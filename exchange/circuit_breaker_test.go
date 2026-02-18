package exchange

import "testing"

// --- PercentBandCircuitBreaker ---

func TestPercentBandCB_ZeroLastPriceAlwaysAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	// Cold start: no trades yet — any price must be accepted.
	for _, newPrice := range []int64{1, 50_000, 1_000_000} {
		if !cb.Allow("X", 0, newPrice) {
			t.Errorf("expected allow for lastPrice=0, newPrice=%d", newPrice)
		}
	}
}

func TestPercentBandCB_SamePriceAlwaysAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	if !cb.Allow("X", 100_00, 100_00) {
		t.Error("same price must always be allowed")
	}
}

func TestPercentBandCB_WithinBandAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	// +4% and −4% are inside the band
	if !cb.Allow("X", last, 104_00) {
		t.Error("4% up should be allowed inside 5% band")
	}
	if !cb.Allow("X", last, 96_00) {
		t.Error("4% down should be allowed inside 5% band")
	}
}

func TestPercentBandCB_AtBoundaryAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	// Exactly 5% deviation: diff*10000 == lastPrice*BandBps → allowed (<=)
	if !cb.Allow("X", last, 105_00) {
		t.Error("exactly at +5% boundary should be allowed")
	}
	if !cb.Allow("X", last, 95_00) {
		t.Error("exactly at −5% boundary should be allowed")
	}
}

func TestPercentBandCB_OverBandRejects(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	// One unit over 5% in either direction
	if cb.Allow("X", last, 105_01) {
		t.Error("just over +5% should be rejected")
	}
	if cb.Allow("X", last, 94_99) {
		t.Error("just over −5% should be rejected")
	}
}

func TestPercentBandCB_LargeDeviationRejects(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	last := int64(100_00)
	if cb.Allow("X", last, 200_00) {
		t.Error("100% up should be rejected")
	}
	if cb.Allow("X", last, 1_00) {
		t.Error("99% down should be rejected")
	}
}

func TestPercentBandCB_ZeroBandOnlySamePricePasses(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 0}
	last := int64(100_00)
	if !cb.Allow("X", last, 100_00) {
		t.Error("same price must pass even at zero band")
	}
	if cb.Allow("X", last, 100_01) {
		t.Error("any deviation must be rejected at zero band")
	}
	if cb.Allow("X", last, 99_99) {
		t.Error("any deviation must be rejected at zero band")
	}
}

func TestPercentBandCB_FullBandAllowsLargeMoves(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 10000} // ±100%
	last := int64(100_00)
	if !cb.Allow("X", last, 199_00) {
		t.Error("99% move should be allowed inside 100% band")
	}
	if !cb.Allow("X", last, 1_00) {
		t.Error("99% drop should be allowed inside 100% band")
	}
}

func TestPercentBandCB_SymbolArgIgnored(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	last := int64(100_00)
	// PercentBandCircuitBreaker is symbol-agnostic; same result for any symbol.
	r1 := cb.Allow("BTC-PERP", last, 104_00)
	r2 := cb.Allow("ETH-PERP", last, 104_00)
	if r1 != r2 {
		t.Error("result must not depend on symbol")
	}
}

// --- CircuitBreakerMatcher ---

type mockCBEngine struct {
	called bool
	result *MatchResult
}

func (m *mockCBEngine) Match(bidBook, askBook *Book, order *Order) *MatchResult {
	m.called = true
	return m.result
}

func (m *mockCBEngine) Priority() Priority { return Priority{Primary: PriorityPrice} }

func makeOrderBook(symbol string, lastPrice, bidPrice, askPrice int64) (*OrderBook, map[string]*OrderBook) {
	ob := &OrderBook{
		Symbol: symbol,
		Bids:   newBook(Buy),
		Asks:   newBook(Sell),
	}
	if lastPrice != 0 {
		ob.LastTrade = &Trade{Price: lastPrice}
	}
	if bidPrice != 0 {
		ob.Bids.Best = &Limit{Price: bidPrice}
	}
	if askPrice != 0 {
		ob.Asks.Best = &Limit{Price: askPrice}
	}
	books := map[string]*OrderBook{symbol: ob}
	return ob, books
}

func TestCBMatcher_AllowedPassesToInner(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	order := &Order{Side: Buy}
	result := m.Match(ob.Bids, ob.Asks, order)

	if !inner.called {
		t.Error("inner matcher must be called when breaker allows")
	}
	if !result.FullyFilled {
		t.Error("inner result must be forwarded")
	}
}

func TestCBMatcher_BlockedDoesNotCallInner(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	// Ask at 200_00 is 100% above last of 100_00, band is 5% → fires.
	ob, books := makeOrderBook("X", 100_00, 0, 200_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	order := &Order{Side: Buy}
	result := m.Match(ob.Bids, ob.Asks, order)

	if inner.called {
		t.Error("inner matcher must NOT be called when breaker fires")
	}
	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("blocked result must be empty")
	}
}

func TestCBMatcher_UnknownBookPassesThrough(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, _ := makeOrderBook("X", 100_00, 0, 200_00)
	// books map is empty — bidBook won't be found.
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, map[string]*OrderBook{})

	order := &Order{Side: Buy}
	m.Match(ob.Bids, ob.Asks, order)

	if !inner.called {
		t.Error("unknown book must bypass breaker and call inner")
	}
}

func TestCBMatcher_NoOpposingBestPassesThrough(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{}}
	// Ask book has no best — nothing to check against.
	ob, books := makeOrderBook("X", 100_00, 99_00, 0)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	order := &Order{Side: Buy}
	m.Match(ob.Bids, ob.Asks, order)

	if !inner.called {
		t.Error("no opposing best must bypass breaker and call inner")
	}
}

func TestCBMatcher_ZeroLastPricePassesThrough(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{}}
	// No last trade — cold start must always allow.
	ob, books := makeOrderBook("X", 0, 0, 999_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	order := &Order{Side: Buy}
	m.Match(ob.Bids, ob.Asks, order)

	if !inner.called {
		t.Error("zero lastPrice must bypass breaker")
	}
}

func TestCBMatcher_BuyOrderChecksAskSide(t *testing.T) {
	// Ask inside band → allow; if breaker checked bid instead it would fire (bid is far).
	inner := &mockCBEngine{result: &MatchResult{}}
	ob, books := makeOrderBook("X", 100_00, 1_00, 101_00) // bid far, ask close
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if !inner.called {
		t.Error("buy order must check ask side — inner should be called when ask is within band")
	}
}

func TestCBMatcher_SellOrderChecksBidSide(t *testing.T) {
	// Bid inside band → allow; if breaker checked ask instead it would fire (ask is far).
	inner := &mockCBEngine{result: &MatchResult{}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 999_00) // bid close, ask far
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	m.Match(ob.Bids, ob.Asks, &Order{Side: Sell})

	if !inner.called {
		t.Error("sell order must check bid side — inner should be called when bid is within band")
	}
}

func TestCBMatcher_SellBlockedByFarBid(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	// Bid is 50% below last → fires for sell order.
	ob, books := makeOrderBook("X", 100_00, 50_00, 101_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books)

	result := m.Match(ob.Bids, ob.Asks, &Order{Side: Sell})

	if inner.called {
		t.Error("inner must not be called when sell breaker fires on far bid")
	}
	if result.FullyFilled {
		t.Error("blocked result must be empty")
	}
}

func TestCBMatcher_PriorityDelegatesToInner(t *testing.T) {
	inner := &mockCBEngine{}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, nil)
	if m.Priority() != inner.Priority() {
		t.Error("Priority must delegate to inner")
	}
}
