package matching

import (
	"fmt"
	"testing"

	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

// Matching priority had no detector anywhere in the audit: no accounting
// invariant is violated by filling the wrong resting order, because the money
// still moves and still balances. A venue that serves a later arrival ahead of
// an earlier one at the same price is nonetheless broken, and every measure of
// queue position, adverse selection and maker profitability computed from such
// a run is meaningless.
//
// These tests state the priority rule as an observable sequence: given a known
// book and one aggressive order, exactly which resting orders fill, in exactly
// what order, and for exactly how much.

// restingOrder is one order to seed, in the sequence it arrives.
type restingOrder struct {
	id       uint64
	clientID uint64
	price    int64
	qty      int64
}

// fillRecord is one execution reduced to what priority is about: whose order
// was hit, at what price, for how much.
type fillRecord struct {
	makerID uint64
	price   int64
	qty     int64
}

func (f fillRecord) String() string {
	return fmt.Sprintf("maker %d @ %d x%d", f.makerID, f.price, f.qty)
}

// seedAndMatch builds an ask book in the given arrival order and runs one
// aggressive buy against it, returning the fills in execution order.
func seedAndMatch(t *testing.T, resting []restingOrder, takerQty, takerLimit int64) []fillRecord {
	t.Helper()
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)
	for _, order := range resting {
		asks.AddOrder(&etypes.Order{
			ID: order.id, ClientID: order.clientID, Price: order.price,
			Qty: order.qty, Side: etypes.Sell, Type: etypes.LimitOrder,
		})
	}
	taker := &etypes.Order{
		ID: 9_000, ClientID: 999, Price: takerLimit, Qty: takerQty,
		Side: etypes.Buy, Type: etypes.LimitOrder,
	}
	result := matcher.Match(bids, asks, taker)
	fills := make([]fillRecord, 0, len(result.Executions))
	for _, exec := range result.Executions {
		fills = append(fills, fillRecord{makerID: exec.MakerOrderID, price: exec.Price, qty: exec.Qty})
	}
	return fills
}

func assertFills(t *testing.T, got, want []fillRecord) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d fills %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fill %d: got %v, want %v (full sequence %v)", i, got[i], want[i], got)
		}
	}
}

// Price priority: the best price is taken first regardless of when it arrived.
func TestPriceTimeMatcherServesBetterPricesFirst(t *testing.T) {
	// Arrival order is deliberately worst-price-first, so a matcher that
	// ignored price and served arrival order would produce the reverse
	// sequence.
	fills := seedAndMatch(t, []restingOrder{
		{id: 1, clientID: 10, price: 102, qty: 10},
		{id: 2, clientID: 20, price: 101, qty: 10},
		{id: 3, clientID: 30, price: 100, qty: 10},
	}, 30, 105)
	assertFills(t, fills, []fillRecord{
		{makerID: 3, price: 100, qty: 10},
		{makerID: 2, price: 101, qty: 10},
		{makerID: 1, price: 102, qty: 10},
	})
}

// Time priority: at one price, the earlier arrival is served first, and the
// queue is consumed strictly in order.
func TestPriceTimeMatcherServesEarlierArrivalsFirstAtOnePrice(t *testing.T) {
	fills := seedAndMatch(t, []restingOrder{
		{id: 1, clientID: 10, price: 100, qty: 10},
		{id: 2, clientID: 20, price: 100, qty: 10},
		{id: 3, clientID: 30, price: 100, qty: 10},
	}, 30, 100)
	assertFills(t, fills, []fillRecord{
		{makerID: 1, price: 100, qty: 10},
		{makerID: 2, price: 100, qty: 10},
		{makerID: 3, price: 100, qty: 10},
	})
}

// A taker that cannot consume the whole queue must stop at the queue position
// it reaches, leaving everything behind it untouched. This is the case a
// priority violation is most visible in: serving the tail first would fill a
// different maker entirely.
func TestPriceTimeMatcherStopsAtTheQueuePositionItReaches(t *testing.T) {
	fills := seedAndMatch(t, []restingOrder{
		{id: 1, clientID: 10, price: 100, qty: 10},
		{id: 2, clientID: 20, price: 100, qty: 10},
		{id: 3, clientID: 30, price: 100, qty: 10},
	}, 15, 100)
	assertFills(t, fills, []fillRecord{
		{makerID: 1, price: 100, qty: 10},
		{makerID: 2, price: 100, qty: 5},
	})
}

// The two rules together, and in the order they apply: price dominates time.
// The late order at the better price is served before the early order at the
// worse one, and within each price the queue order still holds.
func TestPriceTimeMatcherAppliesPriceBeforeTime(t *testing.T) {
	fills := seedAndMatch(t, []restingOrder{
		{id: 1, clientID: 10, price: 101, qty: 10}, // earliest, worse price
		{id: 2, clientID: 20, price: 101, qty: 10},
		{id: 3, clientID: 30, price: 100, qty: 10}, // latest, better price
		{id: 4, clientID: 40, price: 100, qty: 10},
	}, 40, 105)
	assertFills(t, fills, []fillRecord{
		{makerID: 3, price: 100, qty: 10},
		{makerID: 4, price: 100, qty: 10},
		{makerID: 1, price: 101, qty: 10},
		{makerID: 2, price: 101, qty: 10},
	})
}

// A limit that does not reach the far side leaves the book alone: priority
// never causes a fill through the taker's own limit.
func TestPriceTimeMatcherNeverFillsThroughTheTakerLimit(t *testing.T) {
	fills := seedAndMatch(t, []restingOrder{
		{id: 1, clientID: 10, price: 100, qty: 10},
		{id: 2, clientID: 20, price: 101, qty: 10},
	}, 20, 100)
	assertFills(t, fills, []fillRecord{
		{makerID: 1, price: 100, qty: 10},
	})
}
