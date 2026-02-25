package circuit_breaker

import (
	"testing"
	"time"

	ebook "exchange_sim/exchange/book"
	eclock "exchange_sim/exchange/clock"
	ematching "exchange_sim/exchange/matching"
	etypes "exchange_sim/exchange/types"
)

type testClock struct{ now int64 }

func (c *testClock) NowUnixNano() int64 { return c.now }
func (c *testClock) NowUnix() int64     { return c.now / 1e9 }

// --- PercentBandCircuitBreaker ---

func TestPercentBandCB_ZeroLastPriceAlwaysAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	for _, newPrice := range []int64{1, 50_000, 1_000_000} {
		if cb.Check("X", 0, newPrice, etypes.Buy).Action != CBAllow {
			t.Errorf("expected CBAllow for lastPrice=0, newPrice=%d", newPrice)
		}
	}
}

func TestPercentBandCB_SamePriceAlwaysAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	if cb.Check("X", 100_00, 100_00, etypes.Buy).Action != CBAllow {
		t.Error("same price must always be allowed")
	}
}

func TestPercentBandCB_WithinBandAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	if cb.Check("X", last, 104_00, etypes.Buy).Action != CBAllow {
		t.Error("4% up should be allowed inside 5% band")
	}
	if cb.Check("X", last, 96_00, etypes.Sell).Action != CBAllow {
		t.Error("4% down should be allowed inside 5% band")
	}
}

func TestPercentBandCB_AtBoundaryAllows(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	if cb.Check("X", last, 105_00, etypes.Buy).Action != CBAllow {
		t.Error("exactly at +5% boundary should be allowed")
	}
	if cb.Check("X", last, 95_00, etypes.Sell).Action != CBAllow {
		t.Error("exactly at −5% boundary should be allowed")
	}
}

func TestPercentBandCB_OverBandRejects(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500} // ±5%
	last := int64(100_00)
	if cb.Check("X", last, 105_01, etypes.Buy).Action == CBAllow {
		t.Error("just over +5% should be rejected")
	}
	if cb.Check("X", last, 94_99, etypes.Sell).Action == CBAllow {
		t.Error("just over −5% should be rejected")
	}
}

func TestPercentBandCB_LargeDeviationRejects(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	last := int64(100_00)
	if cb.Check("X", last, 200_00, etypes.Buy).Action == CBAllow {
		t.Error("100% up should be rejected")
	}
	if cb.Check("X", last, 1_00, etypes.Sell).Action == CBAllow {
		t.Error("99% down should be rejected")
	}
}

func TestPercentBandCB_ZeroBandOnlySamePricePasses(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 0}
	last := int64(100_00)
	if cb.Check("X", last, 100_00, etypes.Buy).Action != CBAllow {
		t.Error("same price must pass even at zero band")
	}
	if cb.Check("X", last, 100_01, etypes.Buy).Action == CBAllow {
		t.Error("any deviation must be rejected at zero band")
	}
	if cb.Check("X", last, 99_99, etypes.Buy).Action == CBAllow {
		t.Error("any deviation must be rejected at zero band")
	}
}

func TestPercentBandCB_FullBandAllowsLargeMoves(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 10000} // ±100%
	last := int64(100_00)
	if cb.Check("X", last, 199_00, etypes.Buy).Action != CBAllow {
		t.Error("99% move should be allowed inside 100% band")
	}
	if cb.Check("X", last, 1_00, etypes.Sell).Action != CBAllow {
		t.Error("99% drop should be allowed inside 100% band")
	}
}

func TestPercentBandCB_SymbolArgIgnored(t *testing.T) {
	cb := &PercentBandCircuitBreaker{BandBps: 500}
	last := int64(100_00)
	r1 := cb.Check("BTC-PERP", last, 104_00, etypes.Buy).Action
	r2 := cb.Check("ETH-PERP", last, 104_00, etypes.Buy).Action
	if r1 != r2 {
		t.Error("result must not depend on symbol")
	}
}

// --- AsymmetricBandCircuitBreaker ---

func TestAsymmetricBandCB_ZeroLastPriceAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 0, 999_00, etypes.Buy).Action != CBAllow {
		t.Error("zero lastPrice must always allow")
	}
}

func TestAsymmetricBandCB_BuyOverUpBandRejects(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 10000}
	if cb.Check("X", 100_00, 106_00, etypes.Buy).Action == CBAllow {
		t.Error("ask 6% above last must be rejected on buy side")
	}
}

func TestAsymmetricBandCB_BuyWithinUpBandAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 104_00, etypes.Buy).Action != CBAllow {
		t.Error("ask 4% above last must be allowed inside 5% up band")
	}
}

func TestAsymmetricBandCB_BuyFavorableDipAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 90_00, etypes.Buy).Action != CBAllow {
		t.Error("ask below last must always allow on buy side")
	}
}

func TestAsymmetricBandCB_SellOverDownBandRejects(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 10000, DownBandBps: 500}
	if cb.Check("X", 100_00, 94_00, etypes.Sell).Action == CBAllow {
		t.Error("bid 6% below last must be rejected on sell side")
	}
}

func TestAsymmetricBandCB_SellWithinDownBandAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 96_00, etypes.Sell).Action != CBAllow {
		t.Error("bid 4% below last must be allowed inside 5% down band")
	}
}

func TestAsymmetricBandCB_SellFavorableRiseAllows(t *testing.T) {
	cb := &AsymmetricBandCircuitBreaker{UpBandBps: 500, DownBandBps: 500}
	if cb.Check("X", 100_00, 110_00, etypes.Sell).Action != CBAllow {
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
	if cb.Check("X", 100_00, 200_00, etypes.Buy).Action != CBAllow {
		t.Error("no ref price must always allow")
	}
}

func TestTieredCB_WithinAllTiersAllows(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	cb.SetRefPrice("X", 100_00)
	if cb.Check("X", 0, 104_00, etypes.Buy).Action != CBAllow {
		t.Error("within all tiers must allow")
	}
}

func TestTieredCB_BreachesMidTierRejects(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	cb.SetRefPrice("X", 100_00)
	if res := cb.Check("X", 0, 106_00, etypes.Buy); res.Action != CBReject {
		t.Errorf("6%% deviation must trigger CBReject, got %v", res.Action)
	}
}

func TestTieredCB_BreachesHighestTierHalts(t *testing.T) {
	cb := NewTieredCircuitBreaker(testTiers)
	cb.SetRefPrice("X", 100_00)
	res := cb.Check("X", 0, 111_00, etypes.Buy)
	if res.Action != CBHalt {
		t.Errorf("11%% deviation must trigger CBHalt, got %v", res.Action)
	}
	if res.HaltDuration != 5*time.Minute {
		t.Errorf("halt duration: want 5m, got %v", res.HaltDuration)
	}
}

// --- CompositeCircuitBreaker ---

type staticBreaker struct{ result CBResult }

func (b *staticBreaker) Check(_ string, _, _ int64, _ etypes.Side) CBResult { return b.result }

func TestCompositeCB_AllowAll(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBAllow}},
		&staticBreaker{CBResult{Action: CBAllow}},
	}}
	if c.Check("X", 1, 1, etypes.Buy).Action != CBAllow {
		t.Error("all allow → composite must allow")
	}
}

func TestCompositeCB_MostRestrictiveWins(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBAllow}},
		&staticBreaker{CBResult{Action: CBReject}},
	}}
	if c.Check("X", 1, 1, etypes.Buy).Action != CBReject {
		t.Error("one reject → composite must reject")
	}
}

func TestCompositeCB_HaltWinsOverReject(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBReject}},
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: time.Minute}},
	}}
	if c.Check("X", 1, 1, etypes.Buy).Action != CBHalt {
		t.Error("halt must win over reject")
	}
}

func TestCompositeCB_LongerHaltWins(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: 5 * time.Minute}},
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: 10 * time.Minute}},
	}}
	if res := c.Check("X", 1, 1, etypes.Buy); res.HaltDuration != 10*time.Minute {
		t.Errorf("longer halt must win: want 10m, got %v", res.HaltDuration)
	}
}

func TestCompositeCB_ForeverHaltWins(t *testing.T) {
	c := &CompositeCircuitBreaker{Breakers: []CircuitBreaker{
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: 0}}, // indefinite
		&staticBreaker{CBResult{Action: CBHalt, HaltDuration: time.Hour}},
	}}
	if res := c.Check("X", 1, 1, etypes.Buy); res.HaltDuration != 0 {
		t.Errorf("indefinite halt must win: want 0, got %v", res.HaltDuration)
	}
}

// --- CircuitBreakerMatcher ---

type mockCBEngine struct {
	called bool
	result *ematching.MatchResult
}

func (m *mockCBEngine) Match(bidBook, askBook *ebook.Book, order *etypes.Order) *ematching.MatchResult {
	m.called = true
	return m.result
}

func makeOrderBook(symbol string, lastPrice, bidPrice, askPrice int64) (*ebook.OrderBook, map[string]*ebook.OrderBook) {
	ob := &ebook.OrderBook{
		Symbol: symbol,
		Bids:   ebook.NewBook(etypes.Buy),
		Asks:   ebook.NewBook(etypes.Sell),
	}
	if lastPrice != 0 {
		ob.LastTrade = &etypes.Trade{Price: lastPrice}
	}
	if bidPrice != 0 {
		ob.Bids.Best = &etypes.Limit{Price: bidPrice}
	}
	if askPrice != 0 {
		ob.Asks.Best = &etypes.Limit{Price: askPrice}
	}
	books := map[string]*ebook.OrderBook{symbol: ob}
	return ob, books
}

func TestCBMatcher_AllowedPassesToInner(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	result := m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("inner matcher must be called when breaker allows")
	}
	if !result.FullyFilled {
		t.Error("inner result must be forwarded")
	}
}

func TestCBMatcher_BlockedDoesNotCallInner(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 0, 200_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	result := m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if inner.called {
		t.Error("inner matcher must NOT be called when breaker fires")
	}
	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("blocked result must be empty")
	}
}

func TestCBMatcher_UnknownBookPassesThrough(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, _ := makeOrderBook("X", 100_00, 0, 200_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, map[string]*ebook.OrderBook{}, &eclock.RealClock{})
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("unknown book must bypass breaker and call inner")
	}
}

func TestCBMatcher_NoOpposingBestPassesThrough(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 0)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("no opposing best must bypass breaker and call inner")
	}
}

func TestCBMatcher_ZeroLastPricePassesThrough(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{}}
	ob, books := makeOrderBook("X", 0, 0, 999_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("zero lastPrice must bypass breaker")
	}
}

func TestCBMatcher_BuyOrderChecksAskSide(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{}}
	ob, books := makeOrderBook("X", 100_00, 1_00, 101_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("buy order must check ask side — inner should be called when ask is within band")
	}
}

func TestCBMatcher_SellOrderChecksBidSide(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 999_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Sell})
	if !inner.called {
		t.Error("sell order must check bid side — inner should be called when bid is within band")
	}
}

func TestCBMatcher_SellBlockedByFarBid(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 50_00, 101_00)
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, &eclock.RealClock{})
	result := m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Sell})
	if inner.called {
		t.Error("inner must not be called when sell breaker fires on far bid")
	}
	if result.FullyFilled {
		t.Error("blocked result must be empty")
	}
}

// --- CircuitBreakerMatcher halt state ---

func TestCBMatcher_PreTradeHaltOnCBHalt(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 106_00)
	clk := &testClock{now: 1_000_000_000}
	haltBreaker := &staticBreaker{CBResult{Action: CBHalt, HaltDuration: 5 * time.Minute}}
	m := NewCircuitBreakerMatcher(inner, haltBreaker, books, clk)
	result := m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
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
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)
	m.HaltUntil["X"] = clk.now + int64(time.Hour)
	result := m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if inner.called {
		t.Error("inner must not be called while halted")
	}
	if result.FullyFilled {
		t.Error("halted result must be empty")
	}
}

func TestCBMatcher_ExpiredHaltAllows(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 2_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)
	m.HaltUntil["X"] = clk.now - int64(time.Second)
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("expired halt must not block the match")
	}
}

func TestCBMatcher_ForeverHaltBlocks(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 999_999_999_999}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)
	m.HaltUntil["X"] = HaltForever
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if inner.called {
		t.Error("indefinite halt must always block")
	}
}

func TestCBMatcher_ClearHalt(t *testing.T) {
	inner := &mockCBEngine{result: &ematching.MatchResult{FullyFilled: true}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 500}, books, clk)
	m.HaltUntil["X"] = HaltForever
	m.ClearHalt("X")
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !inner.called {
		t.Error("cleared halt must allow match to proceed")
	}
}

func TestCBMatcher_IsHalted(t *testing.T) {
	_, books := makeOrderBook("X", 0, 0, 0)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(&mockCBEngine{result: &ematching.MatchResult{}}, &PercentBandCircuitBreaker{}, books, clk)
	if m.IsHalted("X") {
		t.Error("symbol must not be halted initially")
	}
	m.HaltUntil["X"] = HaltForever
	if !m.IsHalted("X") {
		t.Error("symbol must be halted after setting HaltForever")
	}
	m.ClearHalt("X")
	if m.IsHalted("X") {
		t.Error("symbol must not be halted after ClearHalt")
	}
}

func TestCBMatcher_PostMatchHaltTriggered(t *testing.T) {
	exec := &etypes.Execution{Price: 120_00}
	inner := &mockCBEngine{result: &ematching.MatchResult{Executions: []*etypes.Execution{exec}}}
	ob, books := makeOrderBook("X", 100_00, 99_00, 101_00)
	clk := &testClock{now: 1_000_000_000}
	m := NewCircuitBreakerMatcher(inner, &PercentBandCircuitBreaker{BandBps: 10000}, books, clk)
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 1000, Action: CBHalt, HaltDuration: 5 * time.Minute},
	})
	eval.SetRefPrice("X", 100_00)
	m.SetHaltEvaluator(eval)
	m.Match(ob.Bids, ob.Asks, &etypes.Order{Side: etypes.Buy})
	if !m.IsHalted("X") {
		t.Error("symbol must be halted after post-match evaluator fires")
	}
}

// --- TieredHaltEvaluator ---

func TestTieredHaltEval_NoRefPriceNoHalt(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: time.Minute},
	})
	halt, _ := eval.Evaluate("X", []*etypes.Execution{{Price: 200_00}})
	if halt {
		t.Error("no ref price must not trigger halt")
	}
}

func TestTieredHaltEval_WithinBandNoHalt(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: time.Minute},
	})
	eval.SetRefPrice("X", 100_00)
	halt, _ := eval.Evaluate("X", []*etypes.Execution{{Price: 104_00}})
	if halt {
		t.Error("execution within band must not trigger halt")
	}
}

func TestTieredHaltEval_ExecOutsideBandHalts(t *testing.T) {
	eval := NewTieredHaltEvaluator([]BreakerTier{
		{ThresholdBps: 500, Action: CBHalt, HaltDuration: 5 * time.Minute},
	})
	eval.SetRefPrice("X", 100_00)
	halt, dur := eval.Evaluate("X", []*etypes.Execution{{Price: 106_00}})
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
	halt, dur := eval.Evaluate("X", []*etypes.Execution{{Price: 111_00}})
	if !halt {
		t.Error("must halt")
	}
	if dur != 0 {
		t.Errorf("most severe tier (indefinite) must win, got %v", dur)
	}
}
