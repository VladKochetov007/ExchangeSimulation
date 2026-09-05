package analysis

import (
	"encoding/json"
	"fmt"
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
