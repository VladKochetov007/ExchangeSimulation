package exchange

import (
	"testing"
	"time"
)

// --- PercentBandCircuitBreaker ---

func TestPercentBandCB_ZeroLastPriceAlwaysAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	for _, newPrice := range []int64{1, 50_000, 1_000_000} {
		if cb.Check("X", 0, newPrice, Buy).Action != CBAllow {
			t.Errorf("expected CBAllow for lastPrice=0, newPrice=%d", newPrice)
		}
	}
}

func TestPercentBandCB_SamePriceAlwaysAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	if cb.Check("X", 100_00, 100_00, Buy).Action != CBAllow {
		t.Error("same price must always be allowed")
	}
}

func TestPercentBandCB_WithinBandAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	// +4% and −4% are inside the band
	if cb.Check("X", last, 104_00, Buy).Action != CBAllow {
		t.Error("4% up should be allowed inside 5% band")
	}
	if cb.Check("X", last, 96_00, Sell).Action != CBAllow {
		t.Error("4% down should be allowed inside 5% band")
	}
}

func TestPercentBandCB_AtBoundaryAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	// Exactly 5% deviation: diff*10000 == lastPrice*BandBps → allowed (not strictly exceeded)
	if cb.Check("X", last, 105_00, Buy).Action != CBAllow {
		t.Error("exactly at +5% boundary should be allowed")
	}
	if cb.Check("X", last, 95_00, Sell).Action != CBAllow {
		t.Error("exactly at −5% boundary should be allowed")
	}
}

func TestPercentBandCB_OverBandRejects(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	// One unit over 5% in either direction
	if cb.Check("X", last, 105_01, Buy).Action == CBAllow {
		t.Error("just over +5% should be rejected")
	}
	if cb.Check("X", last, 94_99, Sell).Action == CBAllow {
		t.Error("just over −5% should be rejected")
	}
}

func TestPercentBandCB_LargeDeviationRejects(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	last := int64(100_00)
	if cb.Check("X", last, 200_00, Buy).Action == CBAllow {
		t.Error("100% up should be rejected")
	}
	if cb.Check("X", last, 1_00, Sell).Action == CBAllow {
		t.Error("99% down should be rejected")
	}
}

func TestPercentBandCB_ZeroBandOnlySamePricePasses(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 0}
	last := int64(100_00)
	if cb.Check("X", last, 100_00, Buy).Action != CBAllow {
		t.Error("same price must pass even at zero band")
	}
	if cb.Check("X", last, 100_01, Buy).Action == CBAllow {
		t.Error("any deviation must be rejected at zero band")
	}
	if cb.Check("X", last, 99_99, Buy).Action == CBAllow {
		t.Error("any deviation must be rejected at zero band")
	}
}

func TestPercentBandCB_FullBandAllowsLargeMoves(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 10000} // ±100%
	last := int64(100_00)
	if cb.Check("X", last, 199_00, Buy).Action != CBAllow {
		t.Error("99% move should be allowed inside 100% band")
	}
	if cb.Check("X", last, 1_00, Sell).Action != CBAllow {
		t.Error("99% drop should be allowed inside 100% band")
	}
}

func TestPercentBandCB_SymbolArgIgnored(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	last := int64(100_00)
	// PercentBandCircuitBreaker is symbol-agnostic; same result for any symbol.
	r1 := cb.Check("BTC-PERP", last, 104_00, Buy).Action
	r2 := cb.Check("ETH-PERP", last, 104_00, Buy).Action
	if r1 != r2 {
		t.Error("result must not depend on symbol")
	}
}

// --- AsymmetricBandCircuitBreaker ---

func TestAsymmetricBandCB_ZeroLastPriceAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 0, 999_00, Buy).Action != CBAllow {
		t.Error("zero lastPrice must always allow")
	}
}

func TestAsymmetricBandCB_BuyOverUpBandRejects(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 10000}
	// ask 6% above last — exceeds up band of 5%
	if cb.Check("X", 100_00, 106_00, Buy).Action == CBAllow {
		t.Error("ask 6% above last must be rejected on buy side")
	}
}

func TestAsymmetricBandCB_BuyWithinUpBandAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 104_00, Buy).Action != CBAllow {
		t.Error("ask 4% above last must be allowed inside 5% up band")
	}
}

func TestAsymmetricBandCB_BuyFavorableDipAllows(t *testing.T) {
	// Ask dropped below last — good for buyer; up band must not fire.
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 90_00, Buy).Action != CBAllow {
		t.Error("ask below last must always allow on buy side")
	}
}

func TestAsymmetricBandCB_SellOverDownBandRejects(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 10000, DownBandBps: 500}
	// bid 6% below last — exceeds down band of 5%
	if cb.Check("X", 100_00, 94_00, Sell).Action == CBAllow {
		t.Error("bid 6% below last must be rejected on sell side")
	}
}

func TestAsymmetricBandCB_SellWithinDownBandAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 96_00, Sell).Action != CBAllow {
		t.Error("bid 4% below last must be allowed inside 5% down band")
	}
}

func TestAsymmetricBandCB_SellFavorableRiseAllows(t *testing.T) {
	// Bid rose above last — good for seller; down band must not fire.
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 110_00, Sell).Action != CBAllow {
		t.Error("bid above last must always allow on sell side")
	}
}

// --- TieredCircuitBreaker ---

var testTiers = []BreakerTier{
	{ThresholdBps: 500, Action: CBReject},
	{ThresholdBps: 1000, Action: CBHalt, HaltDuration: 5 * time.Minute},
}

func TestTieredCB_NoRefPriceAllows(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	// No SetRefPrice called → always allow.
	if cb.Check("X", 100_00, 200_00, Buy).Action != CBAllow {
		t.Error("no ref price must always allow")
	}
}

func TestTieredCB_WithinAllTiersAllows(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	cb.SetRefPrice("X", 100_00)
	// 4% deviation — below first tier threshold of 5%.
	if cb.Check("X", 0, 104_00, Buy).Action != CBAllow {
		t.Error("within all tiers must allow")
	}
}

func TestTieredCB_BreachesMidTierRejects(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	cb.SetRefPrice("X", 100_00)
	// 6% deviation — exceeds 5% reject tier but not 10% halt tier.
	res := cb.Check("X", 0, 106_00, Buy)
	if res.Action != CBReject {
		t.Errorf("6%% deviation must trigger CBReject, got %v", res.Action)
	}
}

func TestTieredCB_BreachesHighestTierHalts(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	cb.SetRefPrice("X", 100_00)
	// 11% deviation — exceeds 10% halt tier.
	res := cb.Check("X", 0, 111_00, Buy)
	if res.Action != CBHalt {
		t.Errorf("11%% deviation must trigger CBHalt, got %v", res.Action)
	}
	if res.HaltDuration != 5*time.Minute {
		t.Errorf("halt duration: want 5m, got %v", res.HaltDuration)
	}
}

// --- CompositeCircuitBreaker ---

type staticBreaker struct{ result CBResult }

func (b *staticBreaker) Check(_ string, _, _ int64, _ Side) CBResult { return b.result }

func TestCompositeCB_AllowAll(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBAllow}},
		&staticBreaker{CBResult{Action: CBAllow}},
	}}
	if c.Check("X", 1, 1, Buy).Action != CBAllow {
		t.Error("all allow → composite must allow")
	}
}

func TestCompositeCB_MostRestrictiveWins(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBAllow}},
		&staticBreaker{CBResult{Action: CBReject}},
	}}
	if c.Check("X", 1, 1, Buy).Action != CBReject {
		t.Error("one reject → composite must reject")
	}
}

func TestCompositeCB_HaltWinsOverReject(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBReject}},
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: time.Minute}},
	}}
	if c.Check("X", 1, 1, Buy).Action != CBHalt {
		t.Error("halt must win over reject")
	}
}

func TestCompositeCB_LongerHaltWins(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: 5 * time.Minute}},
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: 10 * time.Minute}},
	}}
	res := c.Check("X", 1, 1, Buy)
	if res.HaltDuration != 10*time.Minute {
		t.Errorf("longer halt must win: want 10m, got %v", res.HaltDuration)
	}
}

func TestCompositeCB_ForeverHaltWins(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: 0}}, // indefinite
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: time.Hour}},
	}}
	res := c.Check("X", 1, 1, Buy)
	if res.HaltDuration != 0 {
		t.Errorf("indefinite halt must win: want 0, got %v", res.HaltDuration)
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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, map[string]*OrderBook{}, &RealClock{})

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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

	m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if !inner.called {
		t.Error("buy order must check ask side — inner should be called when ask is within band")
	}
}

func TestCBMatcher_SellOrderChecksBidSide(t *testing.T) {
	// Bid inside band → allow; if breaker checked ask instead it would fire (ask is far).
	inner := &mockCBEngine{result: &MatchResult{}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 999_00) // bid close, ask far
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

	m.Match(ob.Bids, ob.Asks, &Order{Side: Sell})

	if !inner.called {
		t.Error("sell order must check bid side — inner should be called when bid is within band")
	}
}

func TestCBMatcher_SellBlockedByFarBid(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	// Bid is 50% below last → fires for sell order.
	ob, books := makeOrderBook("X", 100_00, 50_00, 101_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &RealClock{})

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
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, nil, &RealClock{})
	if m.Priority() != inner.Priority() {
		t.Error("Priority must delegate to inner")
	}
}

// --- CircuitBreakerMatcher halt state ---

func TestCBMatcher_PreTradeHaltOnCBHalt(t *testing.T) {
	// A breaker returning CBHalt must reject the order and set the halt.
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 106_00)
	clk := &testClock{now: 1_000_000_000}
	haltBreaker := &staticBreaker{CBResult{Action: CBHalt, HaltDuration: 5 * time.Minute}}
	m := NewCircuitBreakerMatcher(inner, haltBreaker, books, clk)

	result := m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if inner.called {
		t.Error("inner must not be called when CBHalt fires")
	}
	if result.FullyFilled {
		t.Error("CBHalt result must be empty")
	}
	if !m.IsHalted("X") {
		t.Error("symbol must be halted after CBHalt")
	}
}

func TestCBMatcher_HaltBlocksMatch(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)

	// Impose a halt that expires well in the future.
	m.haltUntil["X"] = clk.now + int64(time.Hour)
	result := m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if inner.called {
		t.Error("inner must not be called while halted")
	}
	if result.FullyFilled {
		t.Error("halted result must be empty")
	}
}

func TestCBMatcher_ExpiredHaltAllows(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 2_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)

	// Halt expired 1 second ago.
	m.haltUntil["X"] = clk.now - int64(time.Second)
	m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if !inner.called {
		t.Error("expired halt must not block the match")
	}
}

func TestCBMatcher_ForeverHaltBlocks(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 999_999_999_999}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)

	m.haltUntil["X"] = haltForever
	m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if inner.called {
		t.Error("indefinite halt must always block")
	}
}

func TestCBMatcher_ClearHalt(t *testing.T) {
	inner := &mockCBEngine{result: &MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)

	m.haltUntil["X"] = haltForever
	m.ClearHalt("X")
	m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if !inner.called {
		t.Error("cleared halt must allow match to proceed")
	}
}

func TestCBMatcher_IsHalted(t *testing.T) {
	_, books := makeOrderBook("X", 0, 0, 0)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(&mockCBEngine{result: &MatchResult{}}, &PercentBandCircuitBreaker{}, books, clk)

	if m.IsHalted("X") {
		t.Error("symbol must not be halted initially")
	}

	m.haltUntil["X"] = haltForever
	if !m.IsHalted("X") {
		t.Error("symbol must be halted after setting haltForever")
	}

	m.ClearHalt("X")
	if m.IsHalted("X") {
		t.Error("symbol must not be halted after ClearHalt")
	}
}

func TestCBMatcher_PostMatchHaltTriggered(t *testing.T) {
	// Inner matcher returns an execution far outside the halt evaluator's band.
	exec := &Execution{Price: 120_00} // 20% from ref
	inner := &mockCBEngine{result: &MatchResult{Executions: []*Execution{exec}}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 10000}, books, clk)

	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 1000, Action: CBHalt, HaltDuration: 5 * time.Minute},
	})
	eval.SetRefPrice("X", 100_00)
	m.SetHaltEvaluator(eval)

	m.Match(ob.Bids, ob.Asks, &Order{Side: Buy})

	if !m.IsHalted("X") {
		t.Error("symbol must be halted after post-match evaluator fires")
	}
}

// --- TieredHaltEvaluator ---

func TestTieredHaltEval_NoRefPriceNoHalt(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: time.Minute},
	})
	halt, _ := eval.Evaluate("X", []*Execution{{Price: 200_00}})
	if halt {
		t.Error("no ref price must not trigger halt")
	}
}

func TestTieredHaltEval_WithinBandNoHalt(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: time.Minute},
	})
	eval.SetRefPrice("X", 100_00)
	// Execution at 4% deviation — within 5% band.
	halt, _ := eval.Evaluate("X", []*Execution{{Price: 104_00}})
	if halt {
		t.Error("execution within band must not trigger halt")
	}
}

func TestTieredHaltEval_ExecOutsideBandHalts(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: 5 * time.Minute},
	})
	eval.SetRefPrice("X", 100_00)
	// Execution at 6% — outside 5% band.
	halt, dur := eval.Evaluate("X", []*Execution{{Price: 106_00}})
	if !halt {
		t.Error("execution outside band must trigger halt")
	}
	if dur != 5*time.Minute {
		t.Errorf("halt duration: want 5m, got %v", dur)
	}
}

func TestTieredHaltEval_MostSevereTierWins(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: time.Minute},
		{ThresholdBps: 1000, Action: CBHalt, HaltDuration: 0}, // indefinite
	})
	eval.SetRefPrice("X", 100_00)
	// Execution at 11% — breaches both tiers; most severe (indefinite) must win.
	halt, dur := eval.Evaluate("X", []*Execution{{Price: 111_00}})
	if !halt {
		t.Error("must halt")
	}
	if dur != 0 {
		t.Errorf("most severe tier (indefinite) must win, got %v", dur)
	}
}
