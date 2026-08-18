package analysis

import (
	"encoding/json"
	"math"
	"testing"
)

func tradeLine(ts int64, takerOrderID uint64, price, qty int64) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": 0, "event": "Trade", "sim_ts": ts,
		"data": map[string]any{"venue_id": "north", "payload": map[string]any{
			"price": price, "qty": qty, "side": "BUY", "taker_order_id": takerOrderID,
		}},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// An order that fills entirely at one price never left the touch, whatever its
// size. Counting its fills instead of its prices would call it a sweep.
func TestSweepTreatsSeveralFillsAtOnePriceAsNoSweep(t *testing.T) {
	run := openShapeRun(t, []string{
		tradeLine(1, 7, 100_000, 10),
		tradeLine(1, 7, 100_000, 10),
		tradeLine(1, 7, 100_000, 10),
	})
	sweep, err := run.MeasureSweep(BookShapeOptions{Files: shapeFiles(t, run)})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if sweep.Orders != 1 {
		t.Errorf("orders = %d, want 1", sweep.Orders)
	}
	if sweep.MultiPrice != 0 || sweep.MultiPriceFraction() != 0 {
		t.Errorf("three fills at one price counted as a sweep: %+v", sweep)
	}
	if sweep.FillsPerOrder.Median != 3 {
		t.Errorf("fills per order = %v, want 3", sweep.FillsPerOrder.Median)
	}
	if sweep.PricesPerOrder.Median != 1 {
		t.Errorf("prices per order = %v, want 1", sweep.PricesPerOrder.Median)
	}
	if sweep.SweepBps.Max != 0 {
		t.Errorf("span = %v bps, want 0", sweep.SweepBps.Max)
	}
}

// An order reaching a second price is the event the metric exists to count,
// and the span must be the distance between its extreme fills.
func TestSweepMeasuresTheSpanAcrossPrices(t *testing.T) {
	run := openShapeRun(t, []string{
		tradeLine(1, 7, 100_000, 10),
		tradeLine(1, 7, 100_100, 10), // 10 bps worse
		tradeLine(2, 8, 200_000, 10), // a second order, one price only
	})
	sweep, err := run.MeasureSweep(BookShapeOptions{Files: shapeFiles(t, run)})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if sweep.Orders != 2 || sweep.MultiPrice != 1 {
		t.Fatalf("orders/multi = %d/%d, want 2/1", sweep.Orders, sweep.MultiPrice)
	}
	if got := sweep.MultiPriceFraction(); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("multi-price fraction = %v, want 0.5", got)
	}
	if got := sweep.SweepBpsWhenMulti.Median; math.Abs(got-10) > 1e-6 {
		t.Errorf("span when multi = %v bps, want 10", got)
	}
	// Pooled, the single-price order contributes a zero and halves the median.
	if sweep.SweepBps.Max != sweep.SweepBpsWhenMulti.Max {
		t.Errorf("pooled max %v disagrees with multi-only max %v", sweep.SweepBps.Max, sweep.SweepBpsWhenMulti.Max)
	}
}

// Fills arrive interleaved across concurrent orders and out of price order.
// The span is between the extremes, not between the first and last seen.
func TestSweepSpansExtremesNotArrivalOrder(t *testing.T) {
	run := openShapeRun(t, []string{
		tradeLine(1, 7, 100_200, 10),
		tradeLine(1, 8, 100_000, 10),
		tradeLine(2, 7, 100_000, 10),
		tradeLine(2, 8, 100_000, 10),
		tradeLine(3, 7, 100_100, 10),
	})
	sweep, err := run.MeasureSweep(BookShapeOptions{Files: shapeFiles(t, run)})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if sweep.PricesPerOrder.Max != 3 {
		t.Errorf("max prices per order = %v, want 3", sweep.PricesPerOrder.Max)
	}
	if got := sweep.SweepBpsWhenMulti.Max; math.Abs(got-20) > 1e-6 {
		t.Errorf("span = %v bps, want 20 (100000 to 100200)", got)
	}
	if sweep.MultiPrice != 1 {
		t.Errorf("multi-price orders = %d, want 1 (order 8 stayed at one price)", sweep.MultiPrice)
	}
}

// A run with no trades reports no orders rather than a zero sweep, which would
// otherwise read as "nothing ever walks the book".
func TestSweepOnAnEmptyRunReportsNoOrders(t *testing.T) {
	run := openShapeRun(t, []string{
		snapshotLine(1, [][3]int64{{100, 50, 0}}, [][3]int64{{102, 50, 0}}),
	})
	sweep, err := run.MeasureSweep(BookShapeOptions{Files: shapeFiles(t, run)})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if sweep.Orders != 0 || sweep.PricesPerOrder.N != 0 {
		t.Errorf("empty run reported %+v", sweep)
	}
}

// Order identifiers are allocated per venue and every venue's counter starts
// at one, so two venues' books contain the same identifiers describing
// unrelated orders. Pooling them would invent sweeps that never happened.
func TestSweepDoesNotMergeOrdersAcrossVenues(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/spot/ABC-USD.jsonl": {tradeLine(1, 7, 100_000, 10)},
		"south/spot/ABC-USD.jsonl": {
			// The same identifier at another venue, at a different price.
			`{"client_id":0,"event":"Trade","sim_ts":1,"data":{"venue_id":"south","payload":{"price":200000,"qty":10,"side":"BUY","taker_order_id":7}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	sweep, err := run.MeasureSweep(BookShapeOptions{Files: run.Files(), TickSize: 1})
	if err != nil {
		t.Fatalf("measure: %v", err)
	}
	if sweep.Orders != 2 {
		t.Errorf("orders = %d, want 2 distinct orders across two venues", sweep.Orders)
	}
	if sweep.MultiPrice != 0 {
		t.Errorf("two venues' order 7 were merged into a %.0f-price sweep", sweep.PricesPerOrder.Max)
	}
}
