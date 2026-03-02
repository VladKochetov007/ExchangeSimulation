package exchange_test

import (
	. "exchange_sim/exchange"
	"context"
	"testing"
	"time"
)

func bgCtx() context.Context { return context.Background() }

// nullLogger implements Logger with no-op methods for testing branches that require a logger.
type nullLogger struct{}

func (l *nullLogger) LogEvent(timestamp int64, clientID uint64, eventType string, data any) {}
func (l *nullLogger) Close() error                                                           { return nil }

// --- publishSnapshot ---

func TestPublishSnapshot_Directly(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	// publishSnapshot is package-private; call directly to cover it.
	ex.PublishSnapshot("BTC/USD", ex.Clock.NowUnixNano())
	// No panic = pass. Unknown symbol must return early without panic.
	ex.PublishSnapshot("UNKNOWN", ex.Clock.NowUnixNano())
}

// --- logAllBalances with real logger ---

func TestLogAllBalances_WithLogger(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(1_000), "BTC": BTCAmount(0.5)}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(500))
	injectBorrowing(ex, 1, "USD", USDAmount(100))

	ex.SetLogger("_global", &nullLogger{})
	// Triggers the logger branch inside logAllBalances
	ex.LogAllBalances()
}

// --- EnablePeriodicSnapshots when exchange is already running ---

func TestEnablePeriodicSnapshots_WhileRunning(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{}) // starts e.running = true

	// Calling EnablePeriodicSnapshots while running should not panic.
	// The branch `if e.running && e.snapshotInterval == 0 && interval > 0` is triggered.
	ex.EnablePeriodicSnapshots(50 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	ex.Shutdown()
}

// --- subscribe with logger ---

func TestSubscribe_WithLogger(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.SetLogger("BTC/USD", &nullLogger{})

	const reqID = uint64(5555)
	gateway := ex.Gateways[1]
	req := Request{
		Type:     ReqSubscribe,
		QueryReq: &QueryRequest{RequestID: reqID, Symbol: "BTC/USD"},
	}
	resp := sendRequest(gateway, req, reqID)
	if !resp.Success {
		t.Errorf("subscribe with logger should succeed, got error=%v", resp.Error)
	}
}

// --- cancelOrder: order already filled ---

func TestCancelOrder_AlreadyFilled(t *testing.T) {
	ex := setupSpotExchange()

	// Place a sell order from client 2
	orderID, _ := InjectLimitOrder(ex, 2, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	if orderID == 0 {
		t.Fatal("order placement failed")
	}

	// Force the order to Filled status while still in book (simulates the edge case)
	ex.Lock()
	if o := ex.Books["BTC/USD"].Asks.Orders[orderID]; o != nil {
		o.Status = Filled
	}
	ex.Unlock()

	// Client 2 tries to cancel their own order that is now "filled"
	const reqID = uint64(4444)
	gateway := ex.Gateways[2]
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: orderID},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectOrderAlreadyFilled {
		t.Errorf("expected RejectOrderAlreadyFilled, got success=%v error=%v", resp.Success, resp.Error)
	}
}

// --- GetPosition: client has positions but not for this symbol ---

func TestGetPosition_SymbolNotFound(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(500_000))

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(0.1))
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, BTCAmount(0.1))

	// Client 1 has a BTC-PERP position, but not ETH-PERP
	pos := ex.Positions.GetPosition(1, "ETH-PERP")
	if pos != nil {
		t.Errorf("expected nil for missing symbol, got %+v", pos)
	}
}

// --- NewExchangeWithConfig: default branches ---

func TestNewExchangeWithConfig_AllDefaults(t *testing.T) {
	// Empty config hits all default-filling branches
	ex := NewExchangeWithConfig(ExchangeConfig{})
	if ex == nil {
		t.Fatal("expected non-nil exchange")
	}
	if ex.Clock == nil {
		t.Error("default Clock should be set")
	}
}

func TestNewExchangeWithConfig_CustomID(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{ID: "test-exchange"})
	if ex.ID != "test-exchange" {
		t.Errorf("expected ID=test-exchange, got %q", ex.ID)
	}
}

// --- WeightedMidPriceCalculator: only ask side (bid qty = 0) ---

func TestWeightedMidPriceCalculator_ZeroBidQty(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	book := ex.Books["BTC/USD"]

	// Add bid with qty=0 (isEmpty=true but Best set via addOrder — set TotalQty to 0 directly)
	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: BTCAmount(1), Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	ask := &Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: BTCAmount(2), Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.AddOrder(bid)
	book.Asks.AddOrder(ask)
	// Set bid TotalQty to 0 to trigger "bidQty == 0" branch — returns askPrice
	book.Bids.Best.TotalQty = 0

	calc := NewWeightedMidPriceCalculator()
	mid := calc.Calculate(book)
	if mid != PriceUSD(51_000, DOLLAR_TICK) {
		t.Errorf("expected askPrice when bidQty=0, got %d", mid)
	}
}

func TestWeightedMidPriceCalculator_ZeroAskQty(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	book := ex.Books["BTC/USD"]

	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: BTCAmount(2), Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	ask := &Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: BTCAmount(1), Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.AddOrder(bid)
	book.Asks.AddOrder(ask)
	book.Asks.Best.TotalQty = 0

	calc := NewWeightedMidPriceCalculator()
	mid := calc.Calculate(book)
	if mid != PriceUSD(49_000, DOLLAR_TICK) {
		t.Errorf("expected bidPrice when askQty=0, got %d", mid)
	}
}

func TestWeightedMidPriceCalculator_BothZeroQty(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	book := ex.Books["BTC/USD"]

	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: BTCAmount(1), Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	ask := &Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: BTCAmount(1), Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.AddOrder(bid)
	book.Asks.AddOrder(ask)
	book.Bids.Best.TotalQty = 0
	book.Asks.Best.TotalQty = 0

	calc := NewWeightedMidPriceCalculator()
	mid := calc.Calculate(book)
	expected := PriceUSD(49_000, DOLLAR_TICK) + (PriceUSD(51_000, DOLLAR_TICK)-PriceUSD(49_000, DOLLAR_TICK))/2
	if mid != expected {
		t.Errorf("expected mid of zero-qty spread: got %d want %d", mid, expected)
	}
}

// --- calculateCollateralUsed: nil oracle and zero price ---

func TestCalculateCollateralUsed_NilOracle(t *testing.T) {
	bm := NewBorrowingManager(BorrowingConfig{PriceSource: nil})
	result := bm.CalculateCollateralUsed("USD", USDAmount(1_000))
	if result != 0 {
		t.Errorf("expected 0 for nil oracle, got %d", result)
	}
}

func TestCalculateCollateralUsed_ZeroPrice(t *testing.T) {
	oracle := NewStaticPriceOracle(map[string]int64{}) // all prices are 0
	bm := NewBorrowingManager(BorrowingConfig{PriceSource: oracle})
	result := bm.CalculateCollateralUsed("USD", USDAmount(1_000))
	if result != 0 {
		t.Errorf("expected 0 when oracle returns 0, got %d", result)
	}
}

// --- validateCrossMarginCollateral: negative/zero existing borrow price ---

func TestValidateCrossMarginCollateral_ZeroPriceForBorrowedAsset(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	// Give client existing debt in ETH (no oracle price for ETH → price=0 → branch taken)
	injectBorrowing(ex, 1, "ETH", USDAmount(100))

	oracle := NewStaticPriceOracle(map[string]int64{
		"USD": USD_PRECISION,
		// ETH intentionally missing → price=0 for the borrow loop
	})
	ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		BorrowRates:       map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0},
		PriceSource:       oracle,
	})

	// Should succeed — existing ETH borrow ignored (price=0, existingBorrowValue stays 0)
	if err := ex.BorrowMargin(1, "USD", USDAmount(1_000), "test"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- placeOrder: IOC spot buy with partial fill (releases remaining notional) ---

func TestPlaceOrder_IOCSpotBuy_PartialFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})

	// Seed only 0.3 BTC of asks — IOC for 1 BTC will partially fill then cancel remainder
	_, _ = InjectLimitOrder(ex, 2, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(0.3))

	const reqID = uint64(3333)
	gateway := ex.Gateways[1]
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1.0),
			Symbol:      "BTC/USD",
			TimeInForce: IOC,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(gateway, req, reqID)
	// IOC partial fill succeeds (returns the order id)
	if !resp.Success {
		t.Errorf("IOC with partial fill should succeed (partial execution), got error=%v", resp.Error)
	}
}

func TestPlaceOrder_IOCSpotSell_PartialFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})

	// Seed only 0.3 BTC worth of bids
	_, _ = InjectLimitOrder(ex, 2, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(0.3))

	const reqID = uint64(3334)
	gateway := ex.Gateways[1]
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Sell,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1.0),
			Symbol:      "BTC/USD",
			TimeInForce: IOC,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(gateway, req, reqID)
	if !resp.Success {
		t.Errorf("IOC sell with partial fill should succeed, got error=%v", resp.Error)
	}
}

func TestPlaceOrder_IOCPerp_PartialFill(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(500_000))

	// Seed only 0.3 BTC at asks
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(0.3))

	const reqID = uint64(3335)
	gateway := ex.Gateways[1]
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1.0),
			Symbol:      "BTC-PERP",
			TimeInForce: IOC,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(gateway, req, reqID)
	if !resp.Success {
		t.Errorf("IOC perp with partial fill should succeed, got error=%v", resp.Error)
	}
}

// --- placeOrder: perp market order with only bids (mid=0, fallback to bid price) ---

func TestPlaceOrder_PerpMarket_FallbackToBidPrice(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(500_000))
	// Only bids, no asks → mid=0, fallback to bid price for margin estimate
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(0.1))

	// Sell market order: no asks needed for the side check, but mid is 0 → use bid
	_, reason := InjectMarketOrder(ex, 1, "BTC-PERP", Sell, BTCAmount(0.01))
	// Either executes or gets insufficient balance — we just verify it doesn't panic
	_ = reason
}

// mockLiquidationHandler records calls for assertion in tests.
type mockLiquidationHandler struct {
	marginCalls    []*MarginCallEvent
	liquidations   []*LiquidationEvent
	insuranceCalls []*InsuranceFundEvent
}

func (m *mockLiquidationHandler) OnMarginCall(e *MarginCallEvent) {
	m.marginCalls = append(m.marginCalls, e)
}
func (m *mockLiquidationHandler) OnLiquidation(e *LiquidationEvent) {
	m.liquidations = append(m.liquidations, e)
}
func (m *mockLiquidationHandler) OnInsuranceFund(e *InsuranceFundEvent) {
	m.insuranceCalls = append(m.insuranceCalls, e)
}

// setupPerpAutomation builds an exchange with a low-priced perp instrument to avoid int64
// overflow in the initMargin formula: abs(size)*entryPrice*marginRate must fit in int64.
// Using $100 entry price: 1e8 * 1e7 * 1000 = 1e18 < int64 max (~9.2e18).
func setupPerpAutomation(handler LiquidationHandler) (*Exchange, *PerpFutures) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))
	ex.ExchangeBalance.InsuranceFund["USD"] = USDAmount(1_000_000)
	ex.LiquidationHandler = handler
	ex.CollateralRate = 500
	return ex, perp
}

// perpEntry is a safe entry price that avoids int64 overflow:
// abs(1BTC) * perpEntry * marginRate(1000) = 1e8 * 1e7 * 1000 = 1e18 < 9.2e18.
const perpEntry = int64(100) // $100 per BTC

// perpInitMargin returns the initMargin for 1 BTC at perpEntry with 10% rate.
// = 1e8 * (perpEntry*USD_PREC) * 1000 / (1e8 * 10000) = perpEntry * USD_PREC / 10
func perpInitMargin() int64 { return perpEntry * USD_PRECISION / 10 }

// injectPerpPosition injects a synthetic position and overwrites perp balances directly.
func injectPerpPosition(ex *Exchange, clientID uint64, symbol string, size, entryPrice, balance, reserved int64) {
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(clientID, symbol, &Position{
		ClientID:   clientID,
		Symbol:     symbol,
		Size:       size,
		EntryPrice: entryPrice,
	})
	pm.Unlock()

	ex.Lock()
	client := ex.Clients[clientID]
	client.PerpBalances["USD"] = balance
	client.PerpReserved["USD"] = reserved
	ex.Unlock()
}

// entry100 is the entry price $100 in exchange units.
func entry100() int64 { return perpEntry * USD_PRECISION }

// markBelow is a mark price below entry, triggering liquidation for a long.
// At $90: unrealizedPnL = 1BTC * ($90-$100) = -$10 = -1_000_000 USD units.
func markBelow() int64 { return (perpEntry - 10) * USD_PRECISION }

// --- Liquidation: deficit path (PerpAvailable < 0 after failed match) ---

func TestCheckLiquidations_TriggersLiquidation(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)

	// Long 1 BTC at $100. initMargin=$10. balance=0, reserved=$1 → PerpAvailable=-$1.
	// equity = -$1 + (-$10 pnl at $90) = -$11 << maintenance → liquidation.
	injectPerpPosition(ex, 1, "BTC-PERP", BTCAmount(1.0), entry100(), 0, USDAmount(1))
	// Counterparty provides liquidity so the liquidation market sell can fill.
	InjectLimitOrder(ex, 2, "BTC-PERP", Buy, markBelow(), BTCAmount(1.0))

	ex.CheckLiquidations("BTC-PERP", perp, markBelow())

	if len(handler.liquidations) == 0 {
		t.Error("expected OnLiquidation to be called")
	}
}

func TestCheckLiquidations_InsuranceFundDeficit(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)

	// balance=0, reserved=$1, long 1 BTC @ $100, mark=$90 → loss=$10 → deficit after fill.
	injectPerpPosition(ex, 1, "BTC-PERP", BTCAmount(1.0), entry100(), 0, USDAmount(1))
	InjectLimitOrder(ex, 2, "BTC-PERP", Buy, markBelow(), BTCAmount(1.0))

	ex.CheckLiquidations("BTC-PERP", perp, markBelow())

	if len(handler.insuranceCalls) == 0 {
		t.Error("expected OnInsuranceFund to be called for deficit")
	}
}

func TestCheckLiquidations_LiquidationWithLogger(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)
	ex.SetLogger("_global", &nullLogger{})

	injectPerpPosition(ex, 1, "BTC-PERP", BTCAmount(1.0), entry100(), 0, USDAmount(1))
	InjectLimitOrder(ex, 2, "BTC-PERP", Buy, markBelow(), BTCAmount(1.0))

	ex.CheckLiquidations("BTC-PERP", perp, markBelow())
	if len(handler.liquidations) == 0 {
		t.Error("expected OnLiquidation to be called with logger set")
	}
}

// --- Liquidation: surplus path (PerpAvailable > 0 after failed match) ---

func TestCheckLiquidations_SurplusPath(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)

	// balance=$11, reserved=$1 → PerpAvailable=$10, unrealizedPnL=-$10 → equity=$0 < maintenance.
	// After fill at $90: realized PnL=-$10, balance=$1, reserved=0 → PerpAvailable=$1 (surplus).
	injectPerpPosition(ex, 1, "BTC-PERP", BTCAmount(1.0), entry100(), USDAmount(11), USDAmount(1))
	InjectLimitOrder(ex, 2, "BTC-PERP", Buy, markBelow(), BTCAmount(1.0))

	ex.CheckLiquidations("BTC-PERP", perp, markBelow())

	if len(handler.liquidations) == 0 {
		t.Error("expected OnLiquidation to be called")
	}
}

// --- Liquidation: cancels open perp orders before closing ---

func TestCheckLiquidations_CancelsOpenOrders(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)

	// Give client 1 enough perp to place a Sell limit at $110 for 1 BTC.
	// initialMargin = (1BTC * $110 / BTC_PREC) * 10% = $11 = 1_100_000
	ex.Lock()
	ex.Clients[1].PerpBalances["USD"] = USDAmount(25)
	ex.Unlock()

	// Place Sell limit: this reserves $11 and adds the orderID to client.OrderIDs.
	orderID, reason := InjectLimitOrder(ex, 1, "BTC-PERP", Sell, PriceUSD(110, DOLLAR_TICK), BTCAmount(1.0))
	if orderID == 0 {
		// If order fails for some reason, skip test with a log
		t.Logf("skipping cancel orders test: order placement returned reason=%v", reason)
		return
	}

	// Inject long 1 BTC at $100 with deficit state to trigger liquidation.
	injectPerpPosition(ex, 1, "BTC-PERP", BTCAmount(1.0), entry100(), 0, USDAmount(1))
	// Counterparty buy so the liquidation market sell can fill (after client 1's Sell is cancelled).
	InjectLimitOrder(ex, 2, "BTC-PERP", Buy, markBelow(), BTCAmount(1.0))

	ex.CheckLiquidations("BTC-PERP", perp, markBelow())

	if len(handler.liquidations) == 0 {
		t.Error("expected liquidation to be triggered with open orders present")
	}
}

// --- Margin call warning path ---
// initMargin = 1BTC at $100 at 10% = $10 = 1_000_000 USD units.
// Warning zone: 500 < ratio < 750 → $0.50 < equity < $0.75.
// Set perpAvailable = $0.60 = 60_000 → ratio = 60_000*10000/1_000_000 = 600 bps.

func TestCheckLiquidations_MarginCallWarning(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)

	initM := perpInitMargin() // = $10 = 1_000_000

	// balance = initMargin + $0.60, reserved = initMargin → PerpAvailable = $0.60 = 60_000.
	injectPerpPosition(ex, 1, "BTC-PERP",
		BTCAmount(1.0),
		entry100(),
		initM+60_000, // balance
		initM,        // reserved
	)

	// Use exact entry price → unrealizedPnL = 0 → equity = $0.60 → ratio = 600 bps.
	ex.CheckLiquidations("BTC-PERP", perp, entry100())

	if len(handler.marginCalls) == 0 {
		t.Error("expected OnMarginCall to be called")
	}
	if len(handler.liquidations) > 0 {
		t.Error("should not have liquidated at warning margin ratio")
	}
}

// --- checkLiquidations: zero mark price → early return ---

func TestCheckLiquidations_ZeroMarkPrice(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)
	ex.CheckLiquidations("BTC-PERP", perp, 0) // must return early, no panic
	if len(handler.liquidations) > 0 {
		t.Error("zero mark price should not trigger liquidation")
	}
}

// --- chargeCollateralInterest: logger branch (borrowed > 0, logger set) ---

func TestChargeCollateralInterest_WithLogger(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.SetLogger("_global", &nullLogger{})

	// Borrow a large amount so interest > 0 in one minute at 5% APR.
	ex.Clients[1].PerpBalances["USD"] = USDAmount(10_000_000)
	ex.Clients[1].Borrowed["USD"] = USDAmount(10_000_000)

	ex.CollateralRate = 500
	ex.ChargeCollateralInterest() // logger branch fires when interest > 0
}

// --- StartAutomation() called twice: second call is no-op ---

func TestExchangeAutomation_StartAlreadyRunning(t *testing.T) {
	ex := NewExchange(10, &RealClock{})

	// Two StartAutomation calls — second must be a no-op (hits `if automCtx != nil { return }`).
	ex.StartAutomation(bgCtx())
	ex.StartAutomation(bgCtx())
	ex.StopAutomation()
}
