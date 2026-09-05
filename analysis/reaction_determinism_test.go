package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
)

// reactionLine renders one venue record in the envelope the venues write.
func reactionLine(event string, ts int64, client uint64, symbol string, payload map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": client,
		"event":     event,
		"sim_ts":    ts,
		"data":      map[string]any{"venue_id": "north", "symbol": symbol, "payload": payload},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// A book's records reach the analyzer from more than one file: the symbol comes
// from the record, not the path, and Scan reads files on a worker pool. Two
// records written at the same instant into different files therefore arrive in
// an order that varies between runs of the same evidence.
//
// This fixture is that situation and nothing else: every trade after the
// maker's fill shares one timestamp and they carry different prices, so
// whichever one the horizon search lands on decides the markout.
func reactionTieRun(t *testing.T) *Run {
	t.Helper()
	const symbol = "ABC-USD"
	const horizonNano = int64(1e9)

	var first, second []string
	// A maker fill to measure, and a book change and reacting order so the lag
	// arm has an observation too.
	first = append(first,
		reactionLine("BookDelta", 0, 0, symbol, map[string]any{"seq": 1}),
		reactionLine("OrderAccepted", 1000, 7, symbol, map[string]any{"price": 1000}),
		reactionLine("OrderFill", 0, 7, symbol, map[string]any{
			"price": 1000, "qty": 10, "side": "BUY", "role": "maker",
		}),
	)
	// The horizon lands exactly on this cluster. Prices differ by file and by
	// position, so any tie broken differently changes the reported markout.
	for i := 0; i < 64; i++ {
		first = append(first, reactionLine("Trade", horizonNano, 0, symbol, map[string]any{
			"price": 1000 + i, "qty": 1, "side": "BUY",
		}))
		second = append(second, reactionLine("Trade", horizonNano, 0, symbol, map[string]any{
			"price": 5000 + i, "qty": 1, "side": "SELL",
		}))
	}

	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": first,
		"north/derivatives.jsonl":  second,
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	return run
}

// Identical evidence must produce an identical measurement. It did not: the
// trade tape was sorted on sim_ts alone, which is not a total order over a
// tie-dense tape, so an unstable sort left the horizon search free to pick a
// different trade — and a different price — on every run.
func TestReactionIsReproducibleOverTiedRecords(t *testing.T) {
	run := reactionTieRun(t)
	opts := ReactionOptions{HorizonSeconds: 1, MaxReactionSeconds: 30}

	var want string
	for attempt := 0; attempt < 32; attempt++ {
		reaction, err := run.MeasureReaction(opts)
		if err != nil {
			t.Fatalf("measure reaction: %v", err)
		}
		raw, err := json.Marshal(reaction)
		if err != nil {
			t.Fatalf("marshal reaction: %v", err)
		}
		got := string(raw)
		if attempt == 0 {
			want = got
			continue
		}
		if got != want {
			t.Fatalf("run %d disagrees with run 0 over identical evidence:\n first: %s\nlater: %s",
				attempt, want, got)
		}
	}
}

// The order classes are listed in is part of the artifact, so a tie between two
// medians must not resolve differently per run. The names come from a map,
// whose iteration order Go randomises deliberately.
func TestRestingRoleOrderIsReproducibleOverTiedMedians(t *testing.T) {
	placement := &RestingPlacement{ByRole: map[string]*PlacementStats{}}
	for i := 0; i < 16; i++ {
		placement.ByRole[fmt.Sprintf("role_%02d", i)] = &PlacementStats{
			Orders:        1,
			DistanceTicks: Distribution{Median: 58.5},
		}
	}

	want := placement.RolesByDistance()
	for attempt := 0; attempt < 64; attempt++ {
		got := placement.RolesByDistance()
		if len(got) != len(want) {
			t.Fatalf("run %d returned %d roles, first run returned %d", attempt, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("run %d orders tied medians differently: %v vs %v", attempt, got, want)
			}
		}
	}
}

// symbolless renders a record the way the spot books actually write one: no
// symbol anywhere in the data layer, so the book is only knowable from the file.
func symbolless(event string, ts int64, client uint64, payload map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": client,
		"event":     event,
		"sim_ts":    ts,
		"data":      map[string]any{"venue_id": "north", "payload": payload},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// A venue's spot books are separate markets at unrelated price levels. Keying
// them on a symbol the spot records do not carry put all of them on one price
// tape, so a maker's markout was measured against whichever instrument traded
// next: an ABC/USD fill near 50.00 marked against a CDF/USD trade near 3.00
// reads as a 9400 bps loss that never happened.
func TestReactionKeepsSpotBooksApartWhenRecordsCarryNoSymbol(t *testing.T) {
	const horizonNano = int64(1e9)
	// The maker's own book is named so that it sorts *after* the foreign one.
	// Without that the pooled tape happens to present the right trade first and
	// the bug hides behind alphabetical luck.
	own := []string{
		symbolless("OrderFill", 0, 7, map[string]any{
			"price": 5000000000, "qty": 10, "side": "BUY", "role": "maker",
		}),
		symbolless("Trade", horizonNano, 0, map[string]any{
			"price": 5000500000, "qty": 1, "side": "BUY",
		}),
	}
	// A different market entirely, two orders of magnitude away, trading at the
	// same instant, in a file that sorts first.
	foreign := []string{
		symbolless("Trade", horizonNano, 0, map[string]any{
			"price": 300000000, "qty": 1, "side": "BUY",
		}),
	}

	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ZZZ-USD.jsonl": own,
		"north/spot/AAA-USD.jsonl": foreign,
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	reaction, err := run.MeasureReaction(ReactionOptions{HorizonSeconds: 1, MaxReactionSeconds: 30})
	if err != nil {
		t.Fatalf("measure reaction: %v", err)
	}
	if len(reaction.Adverse) != 1 {
		t.Fatalf("expected one maker row, got %d: %+v", len(reaction.Adverse), reaction.Adverse)
	}
	// The maker bought at 5000000000 and its own book traded at 5000500000: a
	// 1 bps move in the maker's favour, so the markout is -1 bps. Marked
	// against the foreign book at 300000000 it reads about +9400.
	const want = -1.0
	got := reaction.Adverse[0].MeanMarkoutBps
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("markout %.3f bps, want %.3f: the fill was marked against another book", got, want)
	}
}
