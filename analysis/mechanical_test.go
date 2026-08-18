package analysis

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// logLine renders one record in the envelope the venues write.
func logLine(ts int64, clientID uint64, event string, payload map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": clientID, "event": event, "sim_ts": ts,
		"data": map[string]any{"venue_id": "north", "payload": payload},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func deltaLine(ts int64, side string, price, totalQty int64) string {
	return logLine(ts, 0, "BookDelta", map[string]any{"price": price, "side": side, "total_qty": totalQty})
}

func tradeEventLine(ts int64, orderID uint64, side string, price, qty int64) string {
	return logLine(ts, 0, "Trade", map[string]any{
		"price": price, "qty": qty, "side": side, "taker_order_id": orderID,
	})
}

func snapshotEventLine(ts int64, clientID uint64, bids, asks [][2]int64) string {
	level := func(entries [][2]int64) []map[string]int64 {
		out := make([]map[string]int64, 0, len(entries))
		for _, entry := range entries {
			out = append(out, map[string]int64{"price": entry[0], "visible_qty": entry[1], "hidden_qty": 0})
		}
		return out
	}
	return logLine(ts, clientID, "BookSnapshot", map[string]any{
		"bids": level(bids), "asks": level(asks),
	})
}

func writeLog(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ABC-USD.jsonl")
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return path
}

// A ladder with known sizes: consuming less than the touch leaves the best
// price where it was, consuming exactly the touch does too because the level
// is only emptied by consuming MORE than it holds, and consuming past it
// steps to the next price.
func TestConsumeCounterfactualWalksTheLadder(t *testing.T) {
	book := NewReplayedBook()
	book.Apply("SELL", 102, 40)
	book.Apply("SELL", 103, 60)
	book.Apply("SELL", 104, 100)
	book.Apply("BUY", 100, 40)
	book.Apply("BUY", 99, 60)

	for _, testCase := range []struct {
		name     string
		buys     bool
		qty      int64
		wantBest int64
	}{
		{"buyer inside the touch", true, 10, 102},
		{"buyer taking all but one", true, 39, 102},
		{"buyer clearing the touch exactly", true, 40, 103},
		{"buyer into the second level", true, 60, 103},
		{"buyer clearing two levels", true, 100, 104},
		{"buyer exhausting the side", true, 500, 0},
		{"seller inside the touch", false, 10, 100},
		{"seller clearing the touch", false, 40, 99},
		{"seller exhausting the side", false, 200, 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := book.ConsumeCounterfactual(testCase.buys, testCase.qty); got != testCase.wantBest {
				t.Errorf("consuming %d gave best %d, want %d", testCase.qty, got, testCase.wantBest)
			}
		})
	}
}

// A delta carries the absolute size for a price, and zero removes the level.
// Treating it as an increment would accumulate size that was never there.
func TestReplayedBookAppliesAbsoluteSizes(t *testing.T) {
	book := NewReplayedBook()
	book.Apply("BUY", 100, 50)
	book.Apply("BUY", 100, 30)
	if got := book.bids[100]; got != 30 {
		t.Errorf("level size = %d, want 30; the delta was treated as an increment", got)
	}
	book.Apply("BUY", 100, 0)
	if _, present := book.bids[100]; present {
		t.Error("a zero delta left the level in the book")
	}
	if book.BestBid() != 0 {
		t.Errorf("best bid = %d after the only level was removed", book.BestBid())
	}
}

// The replay must notice when the reconstructed touch disagrees with the
// engine's own snapshot, because a book mutation that emits no delta would
// otherwise let the reconstruction drift silently for the rest of the run.
func TestReplayReportsDriftAgainstSnapshots(t *testing.T) {
	agreeing := writeLog(t, []string{
		deltaLine(1, "BUY", 100, 50),
		deltaLine(1, "SELL", 102, 50),
		snapshotEventLine(2, 0, [][2]int64{{100, 50}}, [][2]int64{{102, 50}}),
	})
	drift, err := ReplayFile(agreeing, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if drift.Checks != 1 || drift.Mismatches != 0 {
		t.Errorf("a faithful replay reported %+v", drift)
	}

	// The same log with a book change that emitted no delta: the snapshot
	// shows a better bid than anything the replay was told about.
	drifting := writeLog(t, []string{
		deltaLine(1, "BUY", 100, 50),
		deltaLine(1, "SELL", 102, 50),
		snapshotEventLine(2, 0, [][2]int64{{101, 50}}, [][2]int64{{102, 50}}),
	})
	drift, err = ReplayFile(drifting, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if drift.Mismatches != 1 || drift.MismatchRate() != 1 {
		t.Errorf("a silent book change was not detected: %+v", drift)
	}
}

// A snapshot delivered to a subscribing client is that client's view at the
// moment it connected, not the standing book, so it must not resynchronise the
// replay nor count as a check.
func TestReplayIgnoresSubscriberSnapshots(t *testing.T) {
	path := writeLog(t, []string{
		deltaLine(1, "BUY", 100, 50),
		deltaLine(1, "SELL", 102, 50),
		snapshotEventLine(2, 7, [][2]int64{{90, 1}}, [][2]int64{{110, 1}}),
		snapshotEventLine(3, 0, [][2]int64{{100, 50}}, [][2]int64{{102, 50}}),
	})
	drift, err := ReplayFile(path, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if drift.Checks != 1 {
		t.Errorf("checks = %d, want 1; the subscriber snapshot was used", drift.Checks)
	}
	if drift.Mismatches != 0 {
		t.Errorf("the subscriber snapshot resynchronised the replay: %+v", drift)
	}
}

// The whole decomposition, on a book where the answer is computable by hand.
//
// The book is bid 100 x 40, ask 102 x 40 with 103 x 60 behind it, so the mid
// is 101. A buy of 60 clears the touch and leaves the best ask at 103, giving
// a counterfactual mid of (100+103)/2 = 101 (integer division of 203), so the
// mechanical move is 0.5 rounded down to 0 — which is why the test uses wider
// prices below.
func TestMechanicalImpactRecoversAKnownMove(t *testing.T) {
	// Prices are scaled so integer division of the midpoint is exact.
	const bid, ask, behind = int64(10_000), int64(10_200), int64(10_400)
	lines := []string{
		deltaLine(1, "BUY", bid, 1000),
		deltaLine(1, "SELL", ask, 40),
		deltaLine(1, "SELL", behind, 1000),
	}
	// One aggressive buy of 60 clears the 40 at the touch and steps to `behind`.
	lines = append(lines, tradeEventLine(2, 1, "BUY", ask, 60))
	// Then the book is republished with the consumed level gone, and enough
	// later trades to reach the horizon without changing the price.
	lines = append(lines, deltaLine(2, "SELL", ask, 0))
	for i := int64(0); i < 5; i++ {
		lines = append(lines, tradeEventLine(3+i, uint64(10+i), "BUY", behind, 1))
		lines = append(lines, deltaLine(3+i, "SELL", behind, 1000-(i+1)))
	}

	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Drift.Mismatches != 0 {
		t.Errorf("replay drifted: %+v", result.Drift)
	}
	// preMid = (10000+10200)/2 = 10100. Counterfactual best ask = 10400, so
	// the counterfactual mid = (10000+10400)/2 = 10200.
	wantBps := 1e4 * math.Log(10200.0/10100.0)
	if result.MovedOrders < 1 {
		t.Fatalf("the clearing order was not counted as moving the touch: %+v", result)
	}
	if math.Abs(result.MovedMeanMechanicalBps-wantBps) > 1e-6 {
		t.Errorf("mechanical move = %.6f bps, want %.6f", result.MovedMeanMechanicalBps, wantBps)
	}
}

// An order that stays inside the touch has a mechanical move of exactly zero,
// whatever the price did afterwards. That zero is the point of the design:
// non-sweeping orders enter the average rather than being conditioned away.
func TestMechanicalImpactCountsNonSweepersAsZero(t *testing.T) {
	const bid, ask = int64(10_000), int64(10_200)
	lines := []string{
		deltaLine(1, "BUY", bid, 1000),
		deltaLine(1, "SELL", ask, 1000),
	}
	for i := int64(0); i < 40; i++ {
		lines = append(lines, tradeEventLine(2+i, uint64(i+1), "BUY", ask, 5))
		lines = append(lines, deltaLine(2+i, "SELL", ask, 1000-5*(i+1)))
	}
	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.MovedOrders != 0 {
		t.Errorf("%d orders moved the touch inside a single level", result.MovedOrders)
	}
	if result.ZeroMechanical == 0 {
		t.Error("no order was recorded with a zero mechanical move")
	}
	if result.MeanMechanicalBps != 0 {
		t.Errorf("mean mechanical move = %v, want exactly 0", result.MeanMechanicalBps)
	}
}

// The response is signed by the aggressor, so a sell that pushes the price
// down is positive impact, not negative. Getting this backwards would make the
// mechanical and realised terms anticorrelated and the slope negative.
func TestMechanicalImpactSignsBySideOfAggressor(t *testing.T) {
	const topBid, nextBid, ask = int64(10_200), int64(10_000), int64(10_400)
	lines := []string{
		deltaLine(1, "BUY", topBid, 40),
		deltaLine(1, "BUY", nextBid, 1000),
		deltaLine(1, "SELL", ask, 1000),
	}
	lines = append(lines, tradeEventLine(2, 1, "SELL", topBid, 60))
	lines = append(lines, deltaLine(2, "BUY", topBid, 0))
	for i := int64(0); i < 5; i++ {
		lines = append(lines, tradeEventLine(3+i, uint64(10+i), "SELL", nextBid, 1))
		lines = append(lines, deltaLine(3+i, "BUY", nextBid, 1000-(i+1)))
	}
	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.MovedOrders < 1 {
		t.Fatalf("the clearing sell was not counted: %+v", result)
	}
	// preMid = (10200+10400)/2 = 10300, counterfactual = (10000+10400)/2 = 10200.
	// The price fell, and for a seller that is positive impact.
	wantBps := -1e4 * math.Log(10200.0/10300.0)
	if math.Abs(result.MovedMeanMechanicalBps-wantBps) > 1e-6 {
		t.Errorf("signed mechanical move = %.6f, want %.6f", result.MovedMeanMechanicalBps, wantBps)
	}
	if result.MovedMeanMechanicalBps <= 0 {
		t.Error("a sell that pushed the price down was recorded as negative impact")
	}
}

// An order that would exhaust the visible side has no counterfactual price, so
// it must be reported as unmeasurable rather than treated as a move to zero.
func TestMechanicalImpactExcludesAnExhaustedSide(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", 10_000, 1000),
		deltaLine(1, "SELL", 10_200, 50),
		tradeEventLine(2, 1, "BUY", 10_200, 50),
		deltaLine(2, "SELL", 10_200, 0),
		deltaLine(2, "SELL", 10_400, 1000),
	}
	for i := int64(0); i < 5; i++ {
		lines = append(lines, tradeEventLine(3+i, uint64(10+i), "BUY", 10_400, 1))
	}
	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.UnmeasurableOrders < 1 {
		t.Errorf("an order exhausting the side was measured anyway: %+v", result)
	}
	if result.MovedMeanMechanicalBps < 0 {
		t.Error("an exhausted side produced a move toward price zero")
	}
}
