package analysis

import (
	"encoding/json"
	"math"
	"testing"
)

func TestCrossVenueDispersionMeasuresFreshKnownGap(t *testing.T) {
	books := map[string][]string{}
	for _, item := range []struct {
		venue string
		bid   int64
		ask   int64
	}{
		{venue: "north", bid: 100, ask: 102},
		{venue: "central", bid: 104, ask: 106},
		{venue: "south", bid: 102, ask: 104},
	} {
		file, line := quoteLine(int64(1e9), item.venue, item.venue+"/spot/ABC-USD.jsonl", item.bid, item.ask)
		books[file] = append(books[file], line)
	}
	run, err := Open(writeRun(t, Report{}, books))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureCrossVenueDispersion(CrossVenueDispersionOptions{
		Symbol: "ABC-USD", StalenessNanos: int64(2e9), MinVenues: 3, CapturePositiveObservationTimes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := 1e4 * 4.0 / 101.0
	if result.Evaluated != 1 || result.QuoteUpdates != 3 || math.Abs(result.MidpointRangeBps.Mean-want) > 1e-9 {
		t.Fatalf("dispersion = %#v, want one %.12f-bps observation", result, want)
	}
	if got := result.PositiveObservationTimes; len(got) != 1 || got[0] != int64(1e9) {
		t.Fatalf("positive sampled times = %v, want [1000000000]", got)
	}
}

func TestCrossVenueDispersionRejectsStaleAndOneSidedQuotes(t *testing.T) {
	books := map[string][]string{}
	for _, venue := range []string{"north", "central", "south"} {
		file, line := quoteLine(int64(1e9), venue, venue+"/spot/ABC-USD.jsonl", 100, 102)
		books[file] = append(books[file], line)
	}
	// At t=5 only north refreshed, so central/south are too stale for a
	// three-venue comparison. South's later one-sided publication cannot make
	// absence look like a midpoint either.
	file, line := quoteLine(int64(5e9), "north", "north/spot/ABC-USD.jsonl", 110, 112)
	books[file] = append(books[file], line)
	books["south/spot/ABC-USD.jsonl"] = append(books["south/spot/ABC-USD.jsonl"],
		`{"sim_ts":5000000000,"client_id":0,"event":"BookSnapshot","data":{"venue_id":"south","payload":{"bids":[{"price":110,"visible_qty":100,"hidden_qty":0}],"asks":[]}}}`)
	run, err := Open(writeRun(t, Report{}, books))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureCrossVenueDispersion(CrossVenueDispersionOptions{Symbol: "ABC-USD", StalenessNanos: int64(2e9), MinVenues: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.Evaluated != 1 || result.SkippedInsufficientFresh != 1 || result.QuoteUpdates != 4 {
		t.Fatalf("stale/one-sided handling = %#v", result)
	}
}

func TestCrossVenueDispersionUsesLastPersistedSameTimestampQuote(t *testing.T) {
	books := map[string][]string{}
	for _, item := range []struct {
		venue string
		bid   int64
		ask   int64
	}{
		{venue: "north", bid: 100, ask: 102},
		{venue: "central", bid: 100, ask: 102},
		{venue: "south", bid: 100, ask: 102},
	} {
		file, line := quoteLine(int64(1e9), item.venue, item.venue+"/spot/ABC-USD.jsonl", item.bid, item.ask)
		books[file] = append(books[file], line)
	}
	// Same simulated timestamp, later physical log record. The metric must use
	// this publication, rather than a sort-dependent predecessor, at t=1.
	file, line := quoteLine(int64(1e9), "north", "north/spot/ABC-USD.jsonl", 110, 112)
	books[file] = append(books[file], line)
	run, err := Open(writeRun(t, Report{}, books))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureCrossVenueDispersion(CrossVenueDispersionOptions{Symbol: "ABC-USD", StalenessNanos: int64(2e9), MinVenues: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := 1e4 * 10.0 / 101.0
	if result.Evaluated != 1 || result.QuoteUpdates != 4 || math.Abs(result.MidpointRangeBps.Mean-want) > 1e-9 {
		t.Fatalf("same-timestamp quote order = %#v, want %.12f bps", result, want)
	}
}

func TestCrossVenueDispersionKeepsTargetPhysicalAndDerivativeBooks(t *testing.T) {
	books := map[string][]string{}
	for _, item := range []struct {
		venue string
		bid   int64
		ask   int64
	}{
		{venue: "north", bid: 100, ask: 102},
		{venue: "south", bid: 102, ask: 104},
	} {
		file, line := quoteLine(int64(1e9), item.venue, item.venue+"/spot/ABC-USD.jsonl", item.bid, item.ask)
		books[file] = append(books[file], line)
	}
	books["central/derivatives.jsonl"] = []string{
		`{"sim_ts":1000000000,"client_id":0,"event":"BookSnapshot","data":{"venue_id":"central","payload":{"symbol":"ABC-USD","payload":{"bids":[{"price":104,"visible_qty":100,"hidden_qty":0}],"asks":[{"price":106,"visible_qty":100,"hidden_qty":0}]}}}}`,
	}
	run, err := Open(writeRun(t, Report{}, books))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureCrossVenueDispersion(CrossVenueDispersionOptions{Symbol: "ABC-USD", StalenessNanos: int64(2e9), MinVenues: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.QuoteUpdates != 3 || result.Evaluated != 1 {
		t.Fatalf("physical/derivative target quotes = %#v", result)
	}
}

func TestCrossVenueDispersionIgnoresIrrelevantMalformedSnapshotPayload(t *testing.T) {
	books := map[string][]string{}
	for _, item := range []struct {
		venue string
		bid   int64
		ask   int64
	}{
		{venue: "north", bid: 100, ask: 102},
		{venue: "central", bid: 102, ask: 104},
		{venue: "south", bid: 104, ask: 106},
	} {
		file, line := quoteLine(int64(1e9), item.venue, item.venue+"/spot/ABC-USD.jsonl", item.bid, item.ask)
		books[file] = append(books[file], line)
	}
	baselineRun, err := Open(writeRun(t, Report{}, books))
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := baselineRun.MeasureCrossVenueDispersion(CrossVenueDispersionOptions{Symbol: "ABC-USD", StalenessNanos: int64(2e9), MinVenues: 3})
	if err != nil {
		t.Fatal(err)
	}

	books["north/spot/CDF-USD.jsonl"] = []string{
		`{"sim_ts":1000000000,"client_id":0,"event":"BookSnapshot","data":{"venue_id":"north","payload":"malformed snapshot"}}`,
	}
	run, err := Open(writeRun(t, Report{}, books))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.MeasureCrossVenueDispersion(CrossVenueDispersionOptions{Symbol: "ABC-USD", StalenessNanos: int64(2e9), MinVenues: 3})
	if err != nil {
		t.Fatal(err)
	}
	baselineJSON, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(resultJSON) != string(baselineJSON) {
		t.Fatalf("irrelevant malformed payload changed result\n got: %s\nwant: %s", resultJSON, baselineJSON)
	}
}
