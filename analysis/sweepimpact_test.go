package analysis

import (
	"math"
	"testing"
)

// buildTape assembles a tape directly, so a property can be constructed rather
// than hoped for.
func buildTape(orderIDs []uint64, prices, qtys []int64, signs []int8, preMid []int64) *TradeTape {
	tape := &TradeTape{
		Prices: prices, Qtys: qtys, Signs: signs, PreMid: preMid, TakerOrderIDs: orderIDs,
	}
	tape.Timestamps = make([]int64, len(prices))
	for i := range prices {
		tape.Timestamps[i] = int64(i)
	}
	return tape
}

// tapeRows accumulates a synthetic tape one aggressive order at a time.
type tapeRows struct {
	orderIDs             []uint64
	prices, qtys, preMid []int64
	signs                []int8
}

func (r *tapeRows) add(orderID uint64, preMid, price, qty int64) {
	r.orderIDs = append(r.orderIDs, orderID)
	r.preMid = append(r.preMid, preMid)
	r.prices = append(r.prices, price)
	r.qtys = append(r.qtys, qty)
	r.signs = append(r.signs, 1)
}

func (r *tapeRows) tape() *TradeTape {
	return buildTape(r.orderIDs, r.prices, r.qtys, r.signs, r.preMid)
}

// emitOrder writes one aggressive order followed by the trade that carries its
// response. A swept order is the SAME submitted size split across two prices,
// because the measurement sums an order's fills to recover the size that was
// submitted; emitting the full size twice would silently compare swept orders
// against single-price orders half their size.
func emitOrder(rows *tapeRows, orderID uint64, base, size, response int64, sweeps bool) {
	if sweeps {
		rows.add(orderID, base, base, size/2)
		rows.add(orderID, base, base+1, size-size/2)
	} else {
		rows.add(orderID, base, base, size)
	}
	// A filler trade one step later carries the price the response is read
	// from. It belongs to no order, so it is never itself an observation.
	rows.add(0, base, base+response, 0)
}

func TestSweptOrdersNeedsMoreThanOnePrice(t *testing.T) {
	tape := buildTape(
		[]uint64{1, 1, 2, 2, 3},
		[]int64{100, 100, 100, 101, 100},
		[]int64{1, 1, 1, 1, 1},
		[]int8{1, 1, 1, 1, 1},
		[]int64{100, 100, 100, 100, 100},
	)
	swept := tape.SweptOrders()
	if swept[1] {
		t.Error("order filling twice at one price counted as swept")
	}
	if !swept[2] {
		t.Error("order filling at two prices not counted as swept")
	}
	if swept[3] {
		t.Error("single-fill order counted as swept")
	}
	if len(swept) != 1 {
		t.Errorf("swept set = %v, want only order 2", swept)
	}
}

// The comparison must survive the confound it exists to defeat: swept orders
// are larger, and larger orders would look higher-impact under any pooled
// average even when sweeping does nothing. Here impact depends on size ALONE
// while sweeping is only correlated with size, so a correct measurement
// reports no within-bucket gap.
func TestSweepImpactIsNotFooledBySizeCorrelation(t *testing.T) {
	const n = 6000
	var rows tapeRows

	base := int64(1_000_000)
	// A cheap deterministic generator: sweeping is more likely at large sizes
	// but never determined by them, so every bucket holds both classes.
	state := uint64(12345)
	next := func() uint64 { state = state*6364136223846793005 + 1442695040888963407; return state >> 33 }

	for i := 0; i < n; i++ {
		size := int64(10 + int64(next()%1000))
		// Sweep probability rises with size but only from about 30% to about
		// 70%, so both classes are well populated at every size.
		sweeps := int64(next()%100) < 30+(size*40)/1010
		orderID := uint64(i + 1)
		response := size / 10 // depends on size only

		// A swept order is the same submitted size split across two prices,
		// not twice the size: the measurement sums an order's fills, so
		// doubling here would compare swept orders against smaller ones.
		emitOrder(&rows, orderID, base, size, response, sweeps)
	}

	tape := rows.tape()
	result := tape.MeasureSweepImpact(ImpactOptions{HorizonTrades: 1, Buckets: 10})

	// Sizes repeat, so quantile buckets are uneven and the extreme ones can
	// fall below the minimum class size. Most of the range must still be
	// compared, or the confound has not been exercised.
	if result.BucketsCompared < 6 {
		t.Fatalf("only %d of 10 buckets held both classes; the confound was not exercised", result.BucketsCompared)
	}
	spread := result.Buckets[len(result.Buckets)-1].SingleResp - result.Buckets[0].SingleResp
	if spread <= 0 {
		t.Fatalf("the constructed size effect is absent: spread %.4f", spread)
	}
	if math.Abs(result.MeanGapBps) > 0.15*spread {
		t.Errorf("size confound leaked into the gap: gap %.4f against a %.4f response spread across buckets",
			result.MeanGapBps, spread)
	}
}

// When sweeping is entirely determined by size the two cannot be separated by
// size matching, and the measurement must say so by comparing no buckets
// rather than by reporting a gap it cannot attribute.
func TestSweepImpactRefusesAPerfectConfound(t *testing.T) {
	const n = 3000
	var rows tapeRows

	base := int64(1_000_000)
	for i := 0; i < n; i++ {
		size := int64(10 + i)
		sweeps := i >= 2*n/3 // the largest third, and only it, sweeps
		orderID := uint64(i + 1)

		emitOrder(&rows, orderID, base, size, size/10, sweeps)
	}

	tape := rows.tape()
	result := tape.MeasureSweepImpact(ImpactOptions{HorizonTrades: 1, Buckets: 10})

	if result.SweptN == 0 || result.SingleN == 0 {
		t.Fatalf("the design did not produce both classes: swept %d single %d", result.SweptN, result.SingleN)
	}
	// Only buckets straddling the sweep threshold can hold both classes, so
	// the measurement declines to compare almost everywhere. What it must not
	// do is compare across most of the range and report a confident gap.
	if result.BucketsCompared > 2 {
		t.Errorf("%d of 10 buckets held both classes under a perfect confound; "+
			"size matching cannot separate sweeping here and the measurement should decline",
			result.BucketsCompared)
	}
}

// When sweeping genuinely carries extra impact, the measurement must find it,
// and must find it in most buckets rather than as an average over sign flips.
func TestSweepImpactFindsARealGap(t *testing.T) {
	const n = 4000
	var rows tapeRows

	base := int64(1_000_000)
	for i := 0; i < n; i++ {
		size := int64(10 + i%400)
		// Sweeping is independent of size here, so any gap found is the effect.
		sweeps := i%2 == 0
		orderID := uint64(i + 1)
		response := int64(5)
		if sweeps {
			response = 50
		}

		emitOrder(&rows, orderID, base, size, response, sweeps)
	}

	tape := rows.tape()
	result := tape.MeasureSweepImpact(ImpactOptions{HorizonTrades: 1, Buckets: 10})

	if result.MeanGapBps <= 0 {
		t.Fatalf("a constructed sweep effect was not found: gap %.4f", result.MeanGapBps)
	}
	if result.BucketsFavouringSwept < result.BucketsCompared {
		t.Errorf("gap found in only %d of %d buckets; it should be consistent",
			result.BucketsFavouringSwept, result.BucketsCompared)
	}
	if result.SweptN == 0 || result.SingleN == 0 {
		t.Errorf("one class was empty: swept %d single %d", result.SweptN, result.SingleN)
	}
}

// A tape too short to bucket must report nothing rather than a gap computed
// from a handful of observations.
func TestSweepImpactRefusesATinySample(t *testing.T) {
	tape := buildTape(
		[]uint64{1, 2, 3},
		[]int64{100, 101, 102},
		[]int64{1, 1, 1},
		[]int8{1, 1, 1},
		[]int64{100, 100, 100},
	)
	result := tape.MeasureSweepImpact(ImpactOptions{HorizonTrades: 1, Buckets: 10})
	if result.BucketsCompared != 0 || result.MeanGapBps != 0 {
		t.Errorf("tiny sample produced %+v", result)
	}
}
