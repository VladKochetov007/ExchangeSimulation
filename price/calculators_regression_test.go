package price

import (
	"testing"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

type regressionIndex struct{ p int64 }

func (r *regressionIndex) Price(string) int64 { return r.p }

func newRegressionBookWithMid(mid int64) *ebook.OrderBook {
	ob := &ebook.OrderBook{
		Symbol: "X",
		Bids:   ebook.NewBook(etypes.Buy),
		Asks:   ebook.NewBook(etypes.Sell),
	}
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: mid - 10, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 2, Price: mid + 10, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	return ob
}

// Integer decay legitimately reaches emaBasis == 0; that must not be treated
// as "uninitialized", or the next sample re-seeds the EMA from one raw print
// and a single spike teleports the mark by its full size.
func TestRegressionEMADoesNotReseedAfterDecayToZero(t *testing.T) {
	idx := &regressionIndex{p: 1_000_000}
	c := NewEMAMarkPrice("X", idx, 600) // alpha = 33

	c.Calculate(newRegressionBookWithMid(1_000_100)) // seed basis +100

	flat := newRegressionBookWithMid(1_000_000)
	for range 3000 {
		c.Calculate(flat) // decay basis to exactly 0
	}
	markAtZero := c.Calculate(flat)

	markAfterSpike := c.Calculate(newRegressionBookWithMid(1_050_000))

	// Smoothed step for a 50000 spike at alpha=33 is ~165; a re-seed jumps 50000.
	if jump := markAfterSpike - markAtZero; jump > 1000 {
		t.Fatalf("single spike moved mark by %d: EMA re-seeded from raw basis", jump)
	}
}

// alpha = 20000/(N+1) floors to 0 for windows >= 19999 samples, freezing the
// EMA at its seed forever; the coefficient must floor at 1 instead.
func TestRegressionEMAAlphaFloorKeepsLargeWindowMoving(t *testing.T) {
	idx := &regressionIndex{p: 1_000_000}
	c := NewEMAMarkPrice("X", idx, 100_000)

	c.Calculate(newRegressionBookWithMid(1_000_100)) // seed basis +100

	// Basis gap must exceed 10000/alpha for the integer EMA step to be >= 1.
	wide := newRegressionBookWithMid(1_020_000) // basis +20000
	var mark int64
	for range 200 {
		mark = c.Calculate(wide)
	}

	if mark-idx.p <= 100 {
		t.Fatalf("mark basis still %d after 200 samples of +20000 basis: alpha floored to zero", mark-idx.p)
	}
}
