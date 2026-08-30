package exchange

import (
	"math"
	"math/rand"
	"testing"

	etypes "exchange_sim/types"
)

// The bounded clone omits levels the matcher would never reach. These tests are
// a direct differential of that claim: the bounded clone fed to the matcher must
// produce the same MatchResult as the unbounded clone, for every book and order
// shape. They sit alongside the preview-versus-committed fuzz, which validates
// the preview as a whole; this one isolates the truncation.

func boundTestBook(t *testing.T, askLevels map[int64][]uint64, bidLevels map[int64][]uint64) *OrderBook {
	t.Helper()
	book := &OrderBook{
		Symbol:     "ABC/USD",
		Instrument: NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1),
		Bids:       newBook(Buy),
		Asks:       newBook(Sell),
	}
	id := uint64(1)
	add := func(target *Book, side Side, levels map[int64][]uint64) {
		// Insert in ascending price so ordering does not depend on map iteration.
		prices := make([]int64, 0, len(levels))
		for price := range levels {
			prices = append(prices, price)
		}
		for i := 0; i < len(prices); i++ {
			for j := i + 1; j < len(prices); j++ {
				if prices[j] < prices[i] {
					prices[i], prices[j] = prices[j], prices[i]
				}
			}
		}
		for _, price := range prices {
			for _, clientID := range levels[price] {
				if !target.AddOrder(&etypes.Order{
					ID: id, ClientID: clientID, Price: price, Qty: 10,
					Side: side, Type: etypes.LimitOrder, TimeInForce: etypes.GTC,
				}) {
					t.Fatalf("AddOrder(%d) refused", id)
				}
				id++
			}
		}
	}
	add(book.Asks, Sell, askLevels)
	add(book.Bids, Buy, bidLevels)
	return book
}

// requireBoundedEqualsFull runs the matcher against a bounded clone and an
// unbounded clone of the same book and requires identical results.
func requireBoundedEqualsFull(t *testing.T, book *OrderBook, order *Order, excluded map[uint64]struct{}) {
	t.Helper()
	matcher := NewPriceTimeMatcher()

	runWith := func(bound previewBound) *MatchResult {
		bidBound, askBound := noPreviewBound, noPreviewBound
		if order.Side == Buy {
			askBound = bound
		} else {
			bidBound = bound
		}
		bids, ok := cloneBookForPreviewBounded(book.Bids, excluded, bidBound)
		if !ok {
			return nil
		}
		asks, ok := cloneBookForPreviewBounded(book.Asks, excluded, askBound)
		if !ok {
			return nil
		}
		incoming := *order
		incoming.Prev, incoming.Next, incoming.Parent, incoming.FeeReserved = nil, nil, nil, nil
		return matcher.Match(bids, asks, &incoming)
	}

	full := runWith(noPreviewBound)
	bound, bounded := previewCrossBound(matcher, order)
	if !bounded {
		bound = noPreviewBound
	}
	limited := runWith(bound)

	switch {
	case full == nil && limited == nil:
		return
	case full == nil || limited == nil:
		t.Fatalf("one clone refused and the other did not (full=%v bounded=%v)", full != nil, limited != nil)
	}
	defer releasePreviewExecutions(full.Executions)
	defer releasePreviewExecutions(limited.Executions)

	if full.FullyFilled != limited.FullyFilled {
		t.Fatalf("FullyFilled: full %v, bounded %v", full.FullyFilled, limited.FullyFilled)
	}
	if len(full.Executions) != len(limited.Executions) {
		t.Fatalf("execution count: full %d, bounded %d", len(full.Executions), len(limited.Executions))
	}
	for i := range full.Executions {
		a, b := full.Executions[i], limited.Executions[i]
		if a.MakerOrderID != b.MakerOrderID || a.Qty != b.Qty || a.Price != b.Price ||
			a.MakerFilledQty != b.MakerFilledQty || a.TakerFilledQty != b.TakerFilledQty {
			t.Fatalf("execution %d differs:\n full    %+v\n bounded %+v", i, *a, *b)
		}
	}
}

// TestBoundedCloneAtTheExactPrice is the boundary the truncation turns on: a
// level priced exactly at the incoming order's price crosses and must be kept.
// Off by one here silently drops the only matchable level.
func TestBoundedCloneAtTheExactPrice(t *testing.T) {
	for _, price := range []int64{99, 100, 101} {
		book := boundTestBook(t, map[int64][]uint64{100: {2}}, nil)
		order := &etypes.Order{ID: 500, ClientID: 1, Price: price, Qty: 5,
			Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}
		requireBoundedEqualsFull(t, book, order, nil)
	}
	for _, price := range []int64{99, 100, 101} {
		book := boundTestBook(t, nil, map[int64][]uint64{100: {2}})
		order := &etypes.Order{ID: 500, ClientID: 1, Price: price, Qty: 5,
			Side: Sell, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}
		requireBoundedEqualsFull(t, book, order, nil)
	}
}

// TestBoundedCloneKeepsALevelBehindAnAllExcludedOne covers the interaction the
// truncation could get wrong: the touch holds only excluded orders, so the first
// level the preview can match against is deeper. Truncating on price still keeps
// it, because a deeper level that crosses implies the shallower one crossed too.
func TestBoundedCloneKeepsALevelBehindAnAllExcludedOne(t *testing.T) {
	book := boundTestBook(t, map[int64][]uint64{100: {2}, 101: {3}, 105: {4}}, nil)
	order := &etypes.Order{ID: 500, ClientID: 1, Price: 101, Qty: 25,
		Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}
	// Exclude the whole touch level, so only the 101 level can match.
	requireBoundedEqualsFull(t, book, order, map[uint64]struct{}{1: {}})
	// Exclude the touch and the level behind it: nothing within the bound.
	requireBoundedEqualsFull(t, book, order, map[uint64]struct{}{1: {}, 2: {}})
}

// TestBoundedCloneExtremePrices pins the integer boundaries, where a comparison
// written with the wrong sign or an overflow-prone expression would show up.
func TestBoundedCloneExtremePrices(t *testing.T) {
	for _, level := range []int64{1, math.MaxInt64} {
		for _, price := range []int64{1, math.MaxInt64} {
			book := boundTestBook(t, map[int64][]uint64{level: {2}}, nil)
			order := &etypes.Order{ID: 500, ClientID: 1, Price: price, Qty: 5,
				Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}
			requireBoundedEqualsFull(t, book, order, nil)
		}
	}
}

// TestBoundedCloneSelfCrossOnly covers a crossable level holding only the
// incoming client's own orders. The matcher skips its own orders but must not
// stop there, so the bound must keep the deeper levels it can still reach.
func TestBoundedCloneSelfCrossOnly(t *testing.T) {
	book := boundTestBook(t, map[int64][]uint64{100: {1}, 101: {2}}, nil)
	order := &etypes.Order{ID: 500, ClientID: 1, Price: 101, Qty: 15,
		Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}
	requireBoundedEqualsFull(t, book, order, nil)
}

// TestBoundedCloneRandomised is the broad sweep: random multi-level books,
// random incoming prices spanning inside, at and outside the book, random
// exclusions, both sides, market and limit orders.
func TestBoundedCloneRandomised(t *testing.T) {
	random := rand.New(rand.NewSource(20260830))
	for iteration := 0; iteration < 30000; iteration++ {
		askLevels := map[int64][]uint64{}
		bidLevels := map[int64][]uint64{}
		levelCount := 1 + random.Intn(5)
		for i := 0; i < levelCount; i++ {
			price := int64(100 + i*(1+random.Intn(3)))
			holders := make([]uint64, 0, 3)
			for j := 0; j <= random.Intn(3); j++ {
				holders = append(holders, uint64(1+random.Intn(4)))
			}
			askLevels[price] = holders
		}
		for i := 0; i < random.Intn(4); i++ {
			bidLevels[int64(90-i*2)] = []uint64{uint64(1 + random.Intn(4))}
		}
		book := boundTestBook(t, askLevels, bidLevels)

		side := Buy
		if random.Intn(2) == 0 {
			side = Sell
		}
		orderType := etypes.LimitOrder
		if random.Intn(6) == 0 {
			orderType = etypes.Market
		}
		order := &etypes.Order{
			ID: 9000, ClientID: uint64(1 + random.Intn(4)),
			Price: int64(88 + random.Intn(30)), Qty: int64(1 + random.Intn(40)),
			Side: side, Type: orderType, TimeInForce: etypes.GTC,
		}
		var excluded map[uint64]struct{}
		if random.Intn(3) == 0 {
			excluded = map[uint64]struct{}{}
			for id := uint64(1); id <= 4; id++ {
				if random.Intn(2) == 0 {
					excluded[id] = struct{}{}
				}
			}
		}
		requireBoundedEqualsFull(t, book, order, excluded)
	}
}
