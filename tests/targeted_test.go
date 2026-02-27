package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
	"time"
)

// --- MidPriceSource: symbol mapped but no book ---

func TestMidPriceOracle_MappedSymbolNoBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	// Do NOT add the instrument — no book for "BTC/USD"
	o := NewMidPriceOracle(ex)
	o.MapSymbol("BTC", "BTC/USD")
	// symbol is set, but book doesn't exist → book == nil → return 0
	price := o.Price("BTC")
	if price != 0 {
		t.Errorf("expected 0 for missing book, got %d", price)
	}
}

// --- hasOpenPositions: client has positions map but all sizes == 0 ---

func TestSetMarginMode_SuccessWithZeroSizePosition(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(500_000))

	// Inject a zero-size position directly into the positions map (simulates a closed position
	// that has not been removed from the map).
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", Size: 0, EntryPrice: 0,
	})
	pm.Unlock()

	// hasOpenPositions iterates positions, finds Size==0, returns false → SetMarginMode succeeds
	if err := ex.SetMarginMode(1, IsolatedMargin); err != nil {
		t.Errorf("expected success when all positions have size==0: %v", err)
	}
}

// --- VisibleQty: Iceberg order ---

func TestVisibleQty_IcebergOrder(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectClient(1, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})

	const reqID = uint64(9001)
	gateway := ex.Gateways[1]
	// Iceberg sell: total qty 2 BTC, iceberg visible qty 0.5 BTC
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Sell,
			Type:        LimitOrder,
			Price:       PriceUSD(51_000, DOLLAR_TICK),
			Qty:         BTCAmount(2.0),
			Symbol:      "BTC/USD",
			TimeInForce: GTC,
			Visibility:  Iceberg,
			IcebergQty:  BTCAmount(0.5),
		},
	}
	resp := sendRequest(gateway, req, reqID)
	if !resp.Success {
		t.Fatalf("iceberg order should be accepted, got error=%v", resp.Error)
	}

	// VisibleQty(best) returns IcebergQty for iceberg orders
	book := ex.Books["BTC/USD"]
	best := book.Asks.Best
	if best == nil {
		t.Fatal("expected best ask after iceberg order")
	}
	visible := VisibleQty(best)
	if visible != BTCAmount(0.5) {
		t.Errorf("iceberg VisibleQty: expected %d, got %d", BTCAmount(0.5), visible)
	}
}

func TestVisibleQty_IcebergPartialFill(t *testing.T) {
	// When remaining < IcebergQty, VisibleQty returns remaining (not IcebergQty)
	lim := &Limit{}
	order := &Order{
		Visibility: Iceberg,
		Qty:        BTCAmount(2.0),
		FilledQty:  BTCAmount(1.8),  // only 0.2 remaining
		IcebergQty: BTCAmount(0.5),  // iceberg qty is larger than remaining
		Parent:     lim,
	}
	order.Next = nil
	lim.Head = order
	lim.OrderCnt = 1

	visible := VisibleQty(lim)
	expected := BTCAmount(0.2) // remaining < iceberg qty
	if visible != expected {
		t.Errorf("partial iceberg VisibleQty: expected %d, got %d", expected, visible)
	}
}

// --- ConnectClient with balanceSnapshotInterval ---

func TestConnectClient_WithBalanceSnapshotInterval(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients:        2,
		BalanceSnapshotInterval: 50 * time.Millisecond,
	})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	// ConnectClient with non-zero BalanceSnapshotInterval starts the balance snapshot goroutine
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(1_000)}, &FixedFee{})
	time.Sleep(20 * time.Millisecond)
	ex.Shutdown()
}

// --- placeOrder: market buy spot with no asks (no reference price check) ---

func TestPlaceOrder_SpotMarketBuy_NoAsks(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})

	// Empty ask book → market buy has nothing to match against → partial/zero fill but accepted
	_, _ = InjectMarketOrder(ex, 1, "BTC/USD", Buy, BTCAmount(1))
	// Just verifying no panic and the code path for "asks.Best == nil" in spot market buy is exercised.
}

// --- SettleFunding: position with size > 0 (long) already covered; zero pos ---

func TestSettleFunding_SkipsZeroSizePositions(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(100_000), USDAmount(100_000))

	// Open then close a position → Size becomes 0
	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(0.1)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Sell, qty)

	totalBefore := totalMoney(ex, "USD")

	// SettleFunding should skip the zero-size position
	perp.GetFundingRate().NextFunding = 0 // force settlement
	ex.CheckAndSettleFunding()

	totalAfter := totalMoney(ex, "USD")
	if totalBefore != totalAfter {
		t.Errorf("conservation violated: before=%d, after=%d", totalBefore, totalAfter)
	}
}

// --- processExecutions: logger branch for realized PnL ---

func TestProcessExecutions_LoggerPnLBranch(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(500_000))
	ex.SetLogger("_global", &nullLogger{})
	ex.SetLogger("BTC-PERP", &nullLogger{})

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(0.5)

	// Open position
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty)

	// Close at higher price → taker has PnL
	closePrice := PriceUSD(51_000, DOLLAR_TICK)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, closePrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Sell, qty)
	// The realized PnL logger branch in processExecutions is triggered when PnL != 0
}

// --- logAllBalances: borrowed loop with non-zero entry ---

func TestLogAllBalances_WithBorrowedBalance(t *testing.T) {
	ex := setupBorrowingExchange()
	_ = ex.BorrowMargin(1, "USD", USDAmount(1_000), "test")

	ex.SetLogger("_global", &nullLogger{})
	ex.LogAllBalances() // exercises the borrowed map loop
}
