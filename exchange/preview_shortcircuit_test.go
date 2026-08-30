package exchange

import (
	"testing"

	ematching "exchange_sim/matching"
	etypes "exchange_sim/types"
)

// The preview short-circuit skips building a detached book when the matcher
// promises price-gated execution and the order cannot cross. It is on the order
// admission path, so it is held to the outcome the full preview produces rather
// than to a reading of the matcher.

// uncommittedMatcher wraps the real matcher without promising price gating, so
// the short-circuit must not apply to it.
type uncommittedMatcher struct{ inner *ematching.PriceTimeMatcher }

func (u uncommittedMatcher) Match(bid, ask *Book, order *Order) *MatchResult {
	return u.inner.Match(bid, ask, order)
}

func previewTestBook(t *testing.T, side Side, levels map[int64][]uint64) *OrderBook {
	t.Helper()
	book := &OrderBook{
		Symbol:     "ABC/USD",
		Instrument: NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1),
		Bids:       newBook(Buy),
		Asks:       newBook(Sell),
	}
	target := book.Bids
	if side == Sell {
		target = book.Asks
	}
	id := uint64(1)
	for price, clients := range levels {
		for _, clientID := range clients {
			if !target.AddOrder(&etypes.Order{
				ID: id, ClientID: clientID, Price: price, Qty: 10,
				Side: side, Type: etypes.LimitOrder, TimeInForce: etypes.GTC,
			}) {
				t.Fatalf("AddOrder(%d) refused", id)
			}
			id++
		}
	}
	return book
}

// TestPreviewShortCircuitAgreesWithTheFullPreview requires the skipped path and
// the built path to return the same outcome for every order that reaches it.
func TestPreviewShortCircuitAgreesWithTheFullPreview(t *testing.T) {
	cases := []struct {
		name     string
		askPrice int64
		orderBuy bool
		price    int64
		orderTyp etypes.OrderType
	}{
		{"buy below the touch cannot cross", 100, true, 99, etypes.LimitOrder},
		{"buy at the touch crosses", 100, true, 100, etypes.LimitOrder},
		{"buy above the touch crosses", 100, true, 101, etypes.LimitOrder},
		{"market order always crosses", 100, true, 0, etypes.Market},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			side := Sell
			if !tc.orderBuy {
				side = Buy
			}
			book := previewTestBook(t, side, map[int64][]uint64{tc.askPrice: {2}})
			order := &etypes.Order{
				ID: 99, ClientID: 1, Price: tc.price, Qty: 5,
				Side: Buy, Type: tc.orderTyp, TimeInForce: etypes.GTC,
			}
			if !tc.orderBuy {
				order.Side = Sell
			}

			gated := &DefaultExchange{Matcher: NewPriceTimeMatcher()}
			ungated := &DefaultExchange{Matcher: uncommittedMatcher{inner: NewPriceTimeMatcher()}}

			gatedResult, gatedOK := gated.previewMatchExcluding(book, order, nil)
			ungatedResult, ungatedOK := ungated.previewMatchExcluding(book, order, nil)
			if gatedOK != ungatedOK {
				t.Fatalf("preview acceptance differs: gated %v, ungated %v", gatedOK, ungatedOK)
			}
			if !gatedOK {
				return
			}
			defer releasePreviewExecutions(gatedResult.Executions)
			defer releasePreviewExecutions(ungatedResult.Executions)
			if gatedResult.FullyFilled != ungatedResult.FullyFilled {
				t.Fatalf("FullyFilled differs: gated %v, ungated %v",
					gatedResult.FullyFilled, ungatedResult.FullyFilled)
			}
			if len(gatedResult.Executions) != len(ungatedResult.Executions) {
				t.Fatalf("execution count differs: gated %d, ungated %d",
					len(gatedResult.Executions), len(ungatedResult.Executions))
			}
		})
	}
}

// TestPreviewShortCircuitRequiresThePromise pins the opt-in: a matcher that does
// not promise price gating must still get a real preview, because it may match
// an order that never crosses the touch.
func TestPreviewShortCircuitRequiresThePromise(t *testing.T) {
	book := previewTestBook(t, Sell, map[int64][]uint64{100: {2}})
	order := &etypes.Order{ID: 99, ClientID: 1, Price: 99, Qty: 5,
		Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}

	if previewCannotCross(uncommittedMatcher{inner: NewPriceTimeMatcher()}, book, order, nil) {
		t.Fatal("short-circuit applied to a matcher that made no price-gating promise")
	}
	if !previewCannotCross(NewPriceTimeMatcher(), book, order, nil) {
		t.Fatal("short-circuit did not apply to a matcher that promised price gating")
	}
}

// TestPreviewShortCircuitHonoursTheExclusionSet covers the case the short-circuit
// could get wrong: an excluded order sitting at the touch must not make an order
// look crossable, because the preview would not have matched against it either.
func TestPreviewShortCircuitHonoursTheExclusionSet(t *testing.T) {
	// One ask at 100 (id 1) and one at 105 (id 2).
	book := previewTestBook(t, Sell, map[int64][]uint64{100: {2}})
	if !book.Asks.AddOrder(&etypes.Order{ID: 50, ClientID: 3, Price: 105, Qty: 10,
		Side: Sell, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}) {
		t.Fatal("AddOrder refused")
	}
	matcher := NewPriceTimeMatcher()
	order := &etypes.Order{ID: 99, ClientID: 1, Price: 100, Qty: 5,
		Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}

	// With the touch included the order crosses.
	if previewCannotCross(matcher, book, order, nil) {
		t.Fatal("an order at the touch was reported as unable to cross")
	}
	// Excluding the only order at the touch leaves the 105 level, which the
	// order does not cross.
	excluded := map[uint64]struct{}{1: {}}
	if !previewCannotCross(matcher, book, order, excluded) {
		t.Fatal("excluding the touch did not make the order uncrossable")
	}
	// Excluding everything leaves nothing to match.
	if !previewCannotCross(matcher, book, order, map[uint64]struct{}{1: {}, 50: {}}) {
		t.Fatal("an empty candidate set was not reported as uncrossable")
	}
}

// TestPreviewShortCircuitOnAnEmptyBook covers the degenerate case: nothing to
// match against is trivially uncrossable, and must not be reported as crossing.
func TestPreviewShortCircuitOnAnEmptyBook(t *testing.T) {
	book := previewTestBook(t, Sell, nil)
	order := &etypes.Order{ID: 99, ClientID: 1, Price: 100, Qty: 5,
		Side: Buy, Type: etypes.LimitOrder, TimeInForce: etypes.GTC}
	if !previewCannotCross(NewPriceTimeMatcher(), book, order, nil) {
		t.Fatal("an empty book was not reported as uncrossable")
	}
}

// TestPreviewShortCircuitHandlesMarketOrders covers the case a differential
// audit found the first version missing: a market order crosses any price, so it
// cannot cross only when the opposite side holds no order the preview would have
// copied. The earlier version returned early for every market order and so built
// a detached book to match against nothing.
func TestPreviewShortCircuitHandlesMarketOrders(t *testing.T) {
	matcher := NewPriceTimeMatcher()
	market := func() *etypes.Order {
		return &etypes.Order{ID: 99, ClientID: 1, Qty: 5,
			Side: Buy, Type: etypes.Market, TimeInForce: etypes.IOC}
	}

	// Empty opposite side: nothing to match.
	empty := previewTestBook(t, Sell, nil)
	if !previewCannotCross(matcher, empty, market(), nil) {
		t.Fatal("a market order against an empty side was reported as crossing")
	}

	// Liquidity present: must take the full preview at any price.
	stocked := previewTestBook(t, Sell, map[int64][]uint64{100: {2}})
	if previewCannotCross(matcher, stocked, market(), nil) {
		t.Fatal("a market order against a stocked side was reported as unable to cross")
	}

	// Liquidity present but entirely excluded: nothing the preview would copy.
	if !previewCannotCross(matcher, stocked, market(), map[uint64]struct{}{1: {}}) {
		t.Fatal("a market order against a fully excluded side was reported as crossing")
	}

	// And the outcome still matches the full preview in every case.
	gated := &DefaultExchange{Matcher: matcher}
	ungated := &DefaultExchange{Matcher: uncommittedMatcher{inner: matcher}}
	for _, book := range []*OrderBook{empty, stocked} {
		g, gok := gated.previewMatchExcluding(book, market(), nil)
		u, uok := ungated.previewMatchExcluding(book, market(), nil)
		if gok != uok {
			t.Fatalf("acceptance differs: gated %v ungated %v", gok, uok)
		}
		if gok && g.FullyFilled != u.FullyFilled {
			t.Fatalf("FullyFilled differs: gated %v ungated %v", g.FullyFilled, u.FullyFilled)
		}
		if gok {
			releasePreviewExecutions(g.Executions)
			releasePreviewExecutions(u.Executions)
		}
	}
}
