package analysis

import (
	"encoding/json"
	"strings"
	"testing"

	etypes "exchange_sim/types"
)

func TestCollectCDFPublicDepthObservationsMeasuresBookModes(t *testing.T) {
	startAt := int64(100)
	terminalAt := int64(110)
	snapshot := func(sequence uint64, at int64, bids, asks []etypes.PriceLevel) Event {
		raw, err := json.Marshal(cdfPublicSnapshotEvidence{
			Bids: bids, Asks: asks, SourceSequence: sequence, PublicBids: bids, PublicAsks: asks,
		})
		if err != nil {
			t.Fatal(err)
		}
		return Event{SimTS: at, Name: "BookSnapshot", VenueID: "north", GlobalSequence: sequence, payload: raw}
	}
	delta := func(sequence uint64, at int64, side string, price, quantity int64) Event {
		raw, err := json.Marshal(cdfBookDeltaEvidence{Side: side, Price: price, VisibleQty: quantity})
		if err != nil {
			t.Fatal(err)
		}
		return Event{SimTS: at, Name: "BookDelta", VenueID: "north", GlobalSequence: sequence, payload: raw}
	}
	events := []Event{
		snapshot(1, 101, []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, []etypes.PriceLevel{{Price: 101, VisibleQty: 10}}),
		delta(2, 103, "SELL", 101, 0),
		snapshot(3, 105, []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, []etypes.PriceLevel{{Price: 101, VisibleQty: 10}}),
	}
	observations, checks := collectCDFPublicDepthObservations(events, []string{"north"}, startAt, terminalAt)
	if len(checks) != 0 {
		t.Fatalf("public depth checks = %+v", checks)
	}
	contract := RegisteredSV1DActivationContract()
	metric := measureCDFVenueConcentration("north", observations["north"], terminalAt, contract)
	if metric.BidOnlyDurationNano != 2 || metric.AskOnlyDurationNano != 0 || metric.EmptyBookDurationNano != 0 ||
		metric.NonTwoSidedDurationNano != 2 || metric.MaxUninterruptedNonTwoSidedDurationNano != 2 || metric.TerminalBookMode != "two_sided" {
		t.Fatalf("availability metric = %+v", metric)
	}
}

func TestValidateCDFObservationCadenceCoversOpeningAndTerminalIntervals(t *testing.T) {
	observations := []cdfDepthObservation{
		{at: 1, globalSequence: 1, snapshot: true},
		{at: 2, globalSequence: 2, snapshot: true},
		{at: 3, globalSequence: 3, snapshot: true},
		{at: 4, globalSequence: 4, snapshot: true},
	}
	if checks := validateCDFObservationCadence("north", observations, 0, 5, 1); len(checks) != 0 {
		t.Fatalf("complete public cadence rejected: %+v", checks)
	}
	covered := prependCDFInitialObservation(observations, CDFActivationContract{
		SimulationStartNano: 0, InitialPublicBookMode: "empty",
	})
	if len(covered) != len(observations)+1 || covered[0].at != 0 || covered[0].bidDepth != 0 || covered[0].askDepth != 0 {
		t.Fatalf("opening state was not reconstructed: %+v", covered)
	}
}

func TestValidateCDFObservationCadenceRejectsMissingOrUnresolvedIntervals(t *testing.T) {
	makeSnapshots := func(times ...int64) []cdfDepthObservation {
		observations := make([]cdfDepthObservation, 0, len(times))
		for sequence, at := range times {
			observations = append(observations, cdfDepthObservation{at: at, globalSequence: uint64(sequence + 1), snapshot: true})
		}
		return observations
	}
	tests := []struct {
		name          string
		observations  []cdfDepthObservation
		wantSubstring string
	}{
		{name: "opening gap", observations: makeSnapshots(2, 3, 4, 5), wantSubstring: "opening observation"},
		{name: "missing snapshot", observations: makeSnapshots(1, 3, 4, 5), wantSubstring: "missing or shifted"},
		{name: "terminal gap", observations: makeSnapshots(1, 2, 3), wantSubstring: "terminal coverage"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checks := validateCDFObservationCadence("north", test.observations, 0, 5, 1)
			if len(checks) == 0 {
				t.Fatal("invalid public cadence was accepted")
			}
			found := false
			for _, check := range checks {
				if strings.Contains(check.Failure, test.wantSubstring) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("checks = %+v; want %q", checks, test.wantSubstring)
			}
		})
	}
}

func TestValidateCDFObservationCadenceIgnoresSameIntervalDeltas(t *testing.T) {
	observations := []cdfDepthObservation{
		{at: 1, globalSequence: 1, snapshot: true},
		{at: 1, globalSequence: 2},
		{at: 2, globalSequence: 3, snapshot: true},
		{at: 3, globalSequence: 4, snapshot: true},
		{at: 4, globalSequence: 5, snapshot: true},
		{at: 4, globalSequence: 6},
	}
	if checks := validateCDFObservationCadence("north", observations, 0, 5, 1); len(checks) != 0 {
		t.Fatalf("same-grid public deltas changed cadence validation: %+v", checks)
	}
}

func TestCollectCDFPublicDepthObservationsFailsClosedOnBoundaryAndProjectionErrors(t *testing.T) {
	makeEvent := func(at int64, name string, payload any) Event {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return Event{SimTS: at, Name: name, VenueID: "north", GlobalSequence: uint64(at), payload: raw}
	}
	validSnapshot := cdfPublicSnapshotEvidence{
		Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, Asks: []etypes.PriceLevel{{Price: 101, VisibleQty: 10}},
		SourceSequence: 1, PublicBids: []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, PublicAsks: []etypes.PriceLevel{{Price: 101, VisibleQty: 10}},
	}
	events := []Event{
		makeEvent(99, "BookDelta", cdfBookDeltaEvidence{Side: "BUY", Price: 99, VisibleQty: 1}),
		makeEvent(100, "BookSnapshot", validSnapshot),
		makeEvent(101, "BookSnapshot", cdfPublicSnapshotEvidence{
			Bids: validSnapshot.Bids, Asks: validSnapshot.Asks, SourceSequence: 2,
			PublicBids: []etypes.PriceLevel{{Price: 99, VisibleQty: 9}}, PublicAsks: validSnapshot.PublicAsks,
		}),
		{SimTS: 102, Name: "BookSnapshot", VenueID: "central", GlobalSequence: 4, payload: func() json.RawMessage {
			raw, err := json.Marshal(validSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}()},
	}
	_, checks := collectCDFPublicDepthObservations(events, []string{"north"}, 100, 110)
	if len(checks) != 3 {
		t.Fatalf("checks = %+v; want three fail-closed findings", checks)
	}
	for _, check := range checks {
		if !strings.Contains(check.Failure, "CDF") && !strings.Contains(check.Failure, "public") {
			t.Fatalf("check is not a public-evidence failure: %+v", check)
		}
	}
}

func TestValidateCDFTerminalValuationRequiresRegisteredEndpoints(t *testing.T) {
	contract := RegisteredSV1DActivationContract()
	run := &Run{Report: Report{
		InitialAccounts: []AccountRow{
			{VenueID: "north", Account: Account{Timestamp: contract.SimulationStartNano}},
			{VenueID: "central", Account: Account{Timestamp: contract.SimulationStartNano}},
			{VenueID: "south", Account: Account{Timestamp: contract.SimulationStartNano}},
		},
		TerminalAccounts: []AccountRow{
			{VenueID: "north", Account: Account{Timestamp: contract.SimulationEndNano}},
			{VenueID: "central", Account: Account{Timestamp: contract.SimulationEndNano}},
			{VenueID: "south", Account: Account{Timestamp: contract.SimulationEndNano}},
		},
	}}
	metadata := cdfActivationMetadata{SimulationStartNano: contract.SimulationStartNano, SimulationEndNano: contract.SimulationEndNano}
	if err := validateCDFTerminalValuation(run, metadata, contract); err != nil {
		t.Fatal(err)
	}
	run.Report.TerminalAccounts[0].Account.Timestamp++
	if err := validateCDFTerminalValuation(run, metadata, contract); err == nil {
		t.Fatal("timestamp-mutated terminal valuation was accepted")
	}
}
