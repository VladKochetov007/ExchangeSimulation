package analysis

import (
	"encoding/json"
	"fmt"
	"testing"
)

func reactionLine(event string, simTimestamp int64, clientID uint64, symbol string, payload map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": clientID,
		"event":     event,
		"sim_ts":    simTimestamp,
		"data":      map[string]any{"venue_id": "north", "symbol": symbol, "payload": payload},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func reactionTieRun(t *testing.T) *Run {
	t.Helper()
	const symbol = "ABC-USD"
	const horizonNano = int64(1e9)

	first := []string{
		reactionLine("BookDelta", 0, 0, symbol, map[string]any{"seq": 1}),
		reactionLine("OrderAccepted", 1000, 7, symbol, map[string]any{"price": 1000}),
		reactionLine("OrderFill", 0, 7, symbol, map[string]any{
			"price": 1000, "qty": 10, "side": "BUY", "role": "maker",
		}),
	}
	second := make([]string, 0, 64)
	for index := 0; index < 64; index++ {
		first = append(first, reactionLine("Trade", horizonNano, 0, symbol, map[string]any{
			"price": 1000 + index, "qty": 1, "side": "BUY",
		}))
		second = append(second, reactionLine("Trade", horizonNano, 0, symbol, map[string]any{
			"price": 5000 + index, "qty": 1, "side": "SELL",
		}))
	}

	run, err := Open(writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": first,
		"north/derivatives.jsonl":  second,
	}))
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	return run
}

func TestReactionIsReproducibleOverTiedRecords(t *testing.T) {
	run := reactionTieRun(t)
	options := ReactionOptions{HorizonSeconds: 1, MaxReactionSeconds: 30}

	var expected string
	for attempt := 0; attempt < 32; attempt++ {
		reaction, err := run.MeasureReaction(options)
		if err != nil {
			t.Fatalf("measure reaction: %v", err)
		}
		raw, err := json.Marshal(reaction)
		if err != nil {
			t.Fatalf("marshal reaction: %v", err)
		}
		actual := string(raw)
		if attempt == 0 {
			expected = actual
			continue
		}
		if actual != expected {
			t.Fatalf("run %d disagrees with run 0 over identical evidence:\nfirst: %s\nlater: %s", attempt, expected, actual)
		}
	}
}

func TestRestingRoleOrderIsReproducibleOverTiedMedians(t *testing.T) {
	placement := &RestingPlacement{ByRole: map[string]*PlacementStats{}}
	for index := 0; index < 16; index++ {
		placement.ByRole[fmt.Sprintf("role_%02d", index)] = &PlacementStats{
			Orders:        1,
			DistanceTicks: Distribution{Median: 58.5},
		}
	}

	expected := placement.RolesByDistance()
	for attempt := 0; attempt < 64; attempt++ {
		actual := placement.RolesByDistance()
		if len(actual) != len(expected) {
			t.Fatalf("run %d returned %d roles, first run returned %d", attempt, len(actual), len(expected))
		}
		for index := range actual {
			if actual[index] != expected[index] {
				t.Fatalf("run %d orders tied medians differently: %v vs %v", attempt, actual, expected)
			}
		}
	}
}
