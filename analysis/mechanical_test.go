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
	return logLine(ts, 0, "BookDelta", map[string]any{
		"price": price, "side": side, "total_qty": totalQty,
		"visible_qty": totalQty, "hidden_qty": 0,
	})
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

func acceptLine(ts int64, clientID uint64, side string, price, qty int64) string {
	return logLine(ts, clientID, "OrderAccepted", map[string]any{
		"order_id": ts*1000 + int64(clientID), "client_id": clientID,
		"side": side, "price": price, "qty": qty, "type": "LIMIT",
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
		wantOK   bool
	}{
		{"buyer inside the touch", true, 10, 102, true},
		{"buyer taking all but one", true, 39, 102, true},
		{"buyer clearing the touch exactly", true, 40, 103, true},
		{"buyer into the second level", true, 60, 103, true},
		{"buyer clearing two levels", true, 100, 104, true},
		{"buyer exhausting the side", true, 500, 0, false},
		{"seller inside the touch", false, 10, 100, true},
		{"seller clearing the touch", false, 40, 99, true},
		{"seller exhausting the side", false, 200, 0, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got, ok := book.ConsumeCounterfactual(testCase.buys, testCase.qty); got != testCase.wantBest || ok != testCase.wantOK {
				t.Errorf("consuming %d gave best (%d, %t), want (%d, %t)", testCase.qty, got, ok, testCase.wantBest, testCase.wantOK)
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
	if bid, ok := book.BestBid(); ok || bid != 0 {
		t.Errorf("best bid = (%d, %t) after the only level was removed", bid, ok)
	}
}

func TestReplayedBookKeepsSignedZeroLevelsSeparateFromAbsence(t *testing.T) {
	book := NewReplayedBook()
	book.Apply("BUY", -1, 10)
	book.Apply("SELL", 0, 10)
	if bid, ok := book.BestBid(); !ok || bid != -1 {
		t.Fatalf("signed best bid = (%d, %t), want (-1, true)", bid, ok)
	}
	if ask, ok := book.BestAsk(); !ok || ask != 0 {
		t.Fatalf("zero best ask = (%d, %t), want (0, true)", ask, ok)
	}
	if mid, ok := book.Mid(); !ok || mid != 0 {
		t.Fatalf("signed midpoint = (%d, %t), want (0, true)", mid, ok)
	}
	if price, ok := book.ConsumeCounterfactual(true, 0); !ok || price != 0 {
		t.Fatalf("zero consumed-side price = (%d, %t), want (0, true)", price, ok)
	}
	if price, ok := book.DeepestVisible(true); !ok || price != 0 {
		t.Fatalf("zero deepest visible price = (%d, %t), want (0, true)", price, ok)
	}
	book.Apply("SELL", 0, 0)
	if ask, ok := book.BestAsk(); ok || ask != 0 {
		t.Fatalf("removed zero ask = (%d, %t), want (0, false)", ask, ok)
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

func TestReplayDetectsDeepQuantityDrift(t *testing.T) {
	path := writeLog(t, []string{
		deltaLine(1, "BUY", 100, 50),
		deltaLine(1, "SELL", 102, 10),
		deltaLine(1, "SELL", 103, 10),
		// The touch is unchanged, but the second level has silently lost one
		// unit. A touch-only audit would incorrectly call this replay exact.
		snapshotEventLine(2, 0, [][2]int64{{100, 50}}, [][2]int64{{102, 10}, {103, 9}}),
	})
	drift, err := ReplayFile(path, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if drift.Checks != 1 || drift.Mismatches != 1 {
		t.Fatalf("deep quantity drift was not detected: %+v", drift)
	}
}

func TestReplayHonorsTruncatedTwentyLevelSnapshotPrefix(t *testing.T) {
	lines := []string{deltaLine(1, "BUY", 100, 50)}
	asks := make([][2]int64, 0, snapshotDepthLimit)
	for index := int64(0); index < snapshotDepthLimit+1; index++ {
		price := 200 + index
		quantity := int64(10 + index)
		lines = append(lines, deltaLine(1, "SELL", price, quantity))
		if index < snapshotDepthLimit {
			asks = append(asks, [2]int64{price, quantity})
		}
	}
	lines = append(lines, snapshotEventLine(2, 0, [][2]int64{{100, 50}}, asks))
	drift, err := ReplayFile(writeLog(t, lines), nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if drift.Checks != 1 || drift.Mismatches != 0 {
		t.Fatalf("valid depth beyond the published prefix was treated as drift: %+v", drift)
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

// A hidden order changes a level without publishing a delta, so a book
// containing one would drift silently for the rest of the run. The replay must
// refuse rather than produce a plausible number.
func TestReplayRefusesHiddenDepth(t *testing.T) {
	path := writeLog(t, []string{
		logLine(1, 0, "BookDelta", map[string]any{
			"price": 100, "side": "BUY", "total_qty": 100, "visible_qty": 40, "hidden_qty": 60,
		}),
	})
	if _, err := ReplayFile(path, nil); err == nil {
		t.Fatal("a delta carrying hidden depth was accepted")
	}
}

// The drift check must never resynchronise the replay from the snapshot it is
// checking against: that would repair a divergence before the next check could
// see it, so the check would pass however broken the replay was.
func TestReplayDoesNotResynchroniseFromSnapshots(t *testing.T) {
	path := writeLog(t, []string{
		deltaLine(1, "BUY", 100, 50),
		deltaLine(1, "SELL", 102, 50),
		// Two snapshots disagreeing with the replay in the same way. If the
		// first one resynchronised the book, the second would agree and only
		// one mismatch would be counted.
		snapshotEventLine(2, 0, [][2]int64{{101, 50}}, [][2]int64{{102, 50}}),
		snapshotEventLine(3, 0, [][2]int64{{101, 50}}, [][2]int64{{102, 50}}),
	})
	drift, err := ReplayFile(path, nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if drift.Mismatches != 2 {
		t.Errorf("mismatches = %d of %d checks, want 2; the snapshot resynchronised the replay",
			drift.Mismatches, drift.Checks)
	}
}

func TestReplayCountsMalformedScoredRecords(t *testing.T) {
	path := writeLog(t, []string{
		`{"sim_ts":1,"event":"BookDelta","data":{"venue_id":"north","payload":{}}}`,
		`{"sim_ts":2,"event":"Trade","data":{"venue_id":"north","payload":"broken"}}`,
		`{"sim_ts":2,"event":"BookSnapshot","data":{"venue_id":"north","payload":{"bids":[{"price":101,"visible_qty":50}],"asks":[]}}}`,
		`{"sim_ts":3,"event":"BookSnapshot","data":{"venue_id":"north","payload":"broken"}}`,
		`not-json`,
	})
	drift, err := ReplayFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if drift.MalformedDeltas != 1 || drift.MalformedTrades != 1 || drift.MalformedSnapshots != 2 || drift.MalformedRecords != 1 {
		t.Fatalf("malformed replay evidence disappeared: %+v", drift)
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

func TestMechanicalImpactReportsSignedReplayAsUndefinedDomain(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", -10_000, 1_000),
		deltaLine(1, "SELL", -9_800, 40),
		deltaLine(1, "SELL", -9_600, 1_000),
		tradeEventLine(2, 1, "BUY", -9_800, 60),
		deltaLine(2, "SELL", -9_800, 0),
		tradeEventLine(3, 2, "BUY", -9_600, 1),
		deltaLine(3, "SELL", -9_600, 999),
	}
	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.Orders == 0 || result.UndefinedDomainOrders == 0 || result.UnmeasurableOrders < result.UndefinedDomainOrders {
		t.Fatalf("signed domain exclusion was not explicit: %+v", result)
	}
}

// Dropping the orders that clear the whole visible side censors the mechanical
// distribution from above, and how often that happens is an outcome of whatever
// an experiment manipulated. A policy must be able to price them instead, so a
// comparison across arms can be bounded rather than silently resampled.
func TestExhaustedOrdersCanBePricedInsteadOfDropped(t *testing.T) {
	lines := []string{
		deltaLine(1, "BUY", 10_000, 1000),
		deltaLine(1, "SELL", 10_200, 10),
		deltaLine(1, "SELL", 10_600, 10),
		tradeEventLine(2, 1, "BUY", 10_600, 20),
		deltaLine(2, "SELL", 10_200, 0),
		deltaLine(2, "SELL", 10_600, 0),
		deltaLine(2, "SELL", 10_800, 1000),
	}
	for i := int64(0); i < 5; i++ {
		lines = append(lines, tradeEventLine(3+i, uint64(10+i), "BUY", 10_800, 1))
	}
	path := writeLog(t, lines)

	dropped, err := MeasureMechanicalImpact(path, MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure dropped: %v", err)
	}
	if dropped.ExhaustedOrders != 1 || dropped.ExhaustedPriced != 0 {
		t.Fatalf("exhausted accounting = %d priced %d, want 1 and 0: %+v",
			dropped.ExhaustedOrders, dropped.ExhaustedPriced, dropped)
	}
	if dropped.MovedOrders != 0 {
		t.Fatalf("the exhausting order was measured anyway: %+v", dropped)
	}

	priced, err := MeasureMechanicalImpact(path, MechanicalOptions{
		HorizonTrades:  1,
		ExhaustedPrice: ExhaustedAtDeepestVisible,
	})
	if err != nil {
		t.Fatalf("measure priced: %v", err)
	}
	if priced.ExhaustedPriced != 1 || priced.MovedOrders != 1 {
		t.Fatalf("priced = %d moved = %d, want 1 and 1: %+v",
			priced.ExhaustedPriced, priced.MovedOrders, priced)
	}
	// preMid 10100; the walk reaches the deepest visible ask at 10600, so the
	// counterfactual mid is 10300.
	wantMechanical := 1e4 * math.Log(10300.0/10100.0)
	if math.Abs(priced.MovedMeanMechanicalBps-wantMechanical) > 1e-6 {
		t.Errorf("mechanical = %.6f, want %.6f", priced.MovedMeanMechanicalBps, wantMechanical)
	}
	if priced.UnmeasurableOrders >= dropped.UnmeasurableOrders {
		t.Errorf("pricing the exhausted order did not reduce the dropped count: %d vs %d",
			priced.UnmeasurableOrders, dropped.UnmeasurableOrders)
	}
}

// The revision term must be the realised response minus the mechanical one,
// computed per order. Defining it as a regression residual would hand the
// whole shared covariance to the mechanical term, which is exactly the
// quantity the measurement exists to estimate.
func TestRevisionIsTheExactRemainderNotAResidual(t *testing.T) {
	const bid, ask, behind = int64(10_000), int64(10_200), int64(10_400)
	lines := []string{
		deltaLine(1, "BUY", bid, 1000),
		deltaLine(1, "SELL", ask, 40),
		deltaLine(1, "SELL", behind, 1000),
		tradeEventLine(2, 1, "BUY", ask, 60),
		// The consumed level is restored in full before the horizon elapses,
		// so the realised move is zero while the mechanical one is not.
		deltaLine(2, "SELL", ask, 40),
	}
	for i := int64(0); i < 5; i++ {
		lines = append(lines, tradeEventLine(3+i, uint64(10+i), "BUY", behind, 1))
	}
	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if result.MovedOrders != 1 {
		t.Fatalf("moved orders = %d, want 1: %+v", result.MovedOrders, result)
	}
	// preMid 10100, counterfactual mid 10200, realised mid back at 10100.
	wantMechanical := 1e4 * math.Log(10200.0/10100.0)
	if math.Abs(result.MovedMeanMechanicalBps-wantMechanical) > 1e-6 {
		t.Errorf("mechanical = %.6f, want %.6f", result.MovedMeanMechanicalBps, wantMechanical)
	}
	if math.Abs(result.MovedMeanActualBps) > 1e-9 {
		t.Errorf("full replenishment left a realised move of %.6f, want 0", result.MovedMeanActualBps)
	}
	// For the order that moved, revision must be exactly minus the mechanical
	// move, since the level came back in full. A residual from a fitted line
	// could not produce this.
	revision := result.MovedMeanActualBps - result.MovedMeanMechanicalBps
	if math.Abs(revision+wantMechanical) > 1e-6 {
		t.Errorf("revision = %.6f, want %.6f (the mechanical move undone)", revision, -wantMechanical)
	}
	// And the whole-sample absolute revision must be non-zero: it is that one
	// order's undoing, averaged over the sample rather than dropped.
	if result.MeanAbsRevisionBps <= 0 {
		t.Error("full replenishment produced no revision component at all")
	}
}

// The variance identity must hold exactly, which it cannot if any term drops
// observations the others keep.
func TestVarianceIdentityHolds(t *testing.T) {
	const bid, ask, behind = int64(10_000), int64(10_200), int64(10_400)
	lines := []string{
		deltaLine(1, "BUY", bid, 100_000),
		deltaLine(1, "SELL", ask, 100_000),
		deltaLine(1, "SELL", behind, 100_000),
	}
	// A mixture: most orders stay inside the touch, some clear it.
	state := uint64(7)
	next := func() uint64 { state = state*6364136223846793005 + 1442695040888963407; return state >> 33 }
	askSize := int64(100_000)
	for i := int64(0); i < 400; i++ {
		size := int64(1 + next()%50)
		lines = append(lines, tradeEventLine(2+i, uint64(i+1), "BUY", ask, size))
		askSize -= size
		if askSize < 100 {
			askSize = 100_000
		}
		lines = append(lines, deltaLine(2+i, "SELL", ask, askSize))
	}
	result, err := MeasureMechanicalImpact(writeLog(t, lines), MechanicalOptions{HorizonTrades: 5})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	identity := result.VarMechanical + result.VarRevision + 2*result.Covariance
	if math.Abs(identity-result.VarActual) > 1e-9*math.Max(1, math.Abs(result.VarActual)) {
		t.Errorf("variance identity broken: mech %.9f + rev %.9f + 2cov %.9f = %.9f, actual %.9f",
			result.VarMechanical, result.VarRevision, 2*result.Covariance, identity, result.VarActual)
	}
}

// A horizon in simulated time must span that much time, which is what makes
// the measurement comparable against the makers' requote interval.
func TestHorizonInSimulatedTimeSpansThatTime(t *testing.T) {
	const bid, ask, behind = int64(10_000), int64(10_200), int64(10_400)
	second := int64(1_000_000_000)
	lines := []string{
		deltaLine(0, "BUY", bid, 1000),
		deltaLine(0, "SELL", ask, 40),
		deltaLine(0, "SELL", behind, 1000),
		tradeEventLine(second, 1, "BUY", ask, 60),
		deltaLine(second, "SELL", ask, 0),
	}
	// Later trades at one-second spacing, with the price moving only after
	// three seconds have passed.
	for i := int64(1); i <= 5; i++ {
		price := behind
		if i >= 3 {
			price = behind + 200
			lines = append(lines, deltaLine(second*(1+i), "SELL", behind, 0))
			lines = append(lines, deltaLine(second*(1+i), "SELL", price, 1000))
		}
		lines = append(lines, tradeEventLine(second*(1+i), uint64(10+i), "BUY", price, 1))
	}
	path := writeLog(t, lines)

	early, err := MeasureMechanicalImpact(path, MechanicalOptions{HorizonNanos: second})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	late, err := MeasureMechanicalImpact(path, MechanicalOptions{HorizonNanos: 4 * second})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if early.MovedOrders != 1 || late.MovedOrders != 1 {
		t.Fatalf("the clearing order was not measured at both horizons: %d, %d",
			early.MovedOrders, late.MovedOrders)
	}
	if late.MovedMeanActualBps <= early.MovedMeanActualBps {
		t.Errorf("a move happening after three seconds was not picked up by the longer horizon: "+
			"1s gave %.4f, 4s gave %.4f", early.MovedMeanActualBps, late.MovedMeanActualBps)
	}
}
