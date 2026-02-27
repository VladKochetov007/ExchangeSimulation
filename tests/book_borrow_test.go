package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
	"time"
)

// --- book.insertLimit: middle insertion (l.Prev != nil) ---

func TestInsertLimit_MiddleInsertion(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})

	// To get the "middle insertion" (l.Prev != nil), we need at least 3 levels
	// in the bid book. Place bids at $52k, $50k, then insert $51k between them.
	// For ask book (sell side, ascending): low, high, then insert middle.
	// Place two sells first at different prices, then a middle price.
	p1 := PriceUSD(50_000, DOLLAR_TICK)
	p3 := PriceUSD(52_000, DOLLAR_TICK)
	p2 := PriceUSD(51_000, DOLLAR_TICK)

	_, _ = InjectLimitOrder(ex, 1, "BTC/USD", Sell, p1, BTCAmount(0.1))
	_, _ = InjectLimitOrder(ex, 1, "BTC/USD", Sell, p3, BTCAmount(0.1))
	// Insert p2 between p1 and p3 → triggers l.Prev != nil in ask book
	_, _ = InjectLimitOrder(ex, 1, "BTC/USD", Sell, p2, BTCAmount(0.1))

	ex.RLock()
	book := ex.Books["BTC/USD"]
	askHead := book.Asks.ActiveHead
	ex.RUnlock()

	if askHead == nil || askHead.Price != p1 {
		t.Errorf("expected ask head at $50k, got %v", askHead)
	}
}

// --- book.cancelOrder: nil order (order not in this book) ---

func TestBookCancelOrder_NotFound(t *testing.T) {
	book := &OrderBook{
		Bids: NewBook(Buy),
	}
	// Cancel non-existent orderID — should return nil without panic
	result := book.Bids.CancelOrder(99999)
	if result != nil {
		t.Errorf("expected nil for non-existent order, got %v", result)
	}
}

// --- runBalanceSnapshotLoop: tick fires ---

func TestRunBalanceSnapshotLoop_Ticks(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients:        2,
		BalanceSnapshotInterval: 10 * time.Millisecond,
	})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(1_000)}, &FixedFee{})
	// Wait for at least 2 ticks then shut down
	time.Sleep(30 * time.Millisecond)
	ex.Shutdown()
}

// --- validateCrossMarginCollateral: nil oracle path ---

func TestValidateCrossMarginCollateral_NilOracle(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.PerpBalances["USD"] = USDAmount(100_000)

	bm := NewBorrowingManager(BorrowingConfig{Enabled: true, PriceSource: nil})
	ctx := BorrowContext{Client: client, ClientID: 1}

	err := bm.BorrowMargin(ctx, "USD", USDAmount(1_000), "test")
	if err == nil || err.Error() != "price oracle not configured" {
		t.Errorf("expected 'price oracle not configured', got %v", err)
	}
}

// --- validateCrossMarginCollateral: borrow asset price unavailable ---

func TestValidateCrossMarginCollateral_ZeroBorrowAssetPrice(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.PerpBalances["USD"] = USDAmount(100_000)

	oracle := NewStaticPriceOracle(map[string]int64{}) // no prices at all
	bm := NewBorrowingManager(BorrowingConfig{
		Enabled:           true,
		BorrowRates:       map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0},
		PriceSource:       oracle,
	})
	ctx := BorrowContext{Client: client, ClientID: 1}

	err := bm.BorrowMargin(ctx, "USD", USDAmount(1_000), "test")
	if err == nil || err.Error() != "price unavailable" {
		t.Errorf("expected 'price unavailable', got %v", err)
	}
}

// --- AutoBorrowForSpotTrade: borrow fails → returns (false, err) ---

func TestAutoBorrowForSpotTrade_BorrowFails(t *testing.T) {
	ex := setupBorrowingExchange()
	// Client 3: tiny spot balance, no perp collateral → collateral validation fails
	ex.ConnectClient(3, map[string]int64{"USD": USDAmount(100)}, &FixedFee{})

	// Direct borrow call with no collateral should fail with "insufficient collateral"
	err := ex.BorrowMargin(3, "USD", USDAmount(10_000), "test")
	if err == nil {
		t.Error("expected error when collateral is insufficient")
	}
}

// --- AutoBorrowForPerpTrade: borrow fails → returns (false, err) ---

func TestAutoBorrowForPerpTrade_BorrowFails(t *testing.T) {
	ex := setupBorrowingExchange()
	ex.ConnectClient(3, map[string]int64{}, &FixedFee{})
	// Tiny perp balance, no collateral — borrow should fail
	ex.AddPerpBalance(3, "USD", USDAmount(10))

	err := ex.BorrowMargin(3, "USD", USDAmount(10_000), "test")
	if err == nil {
		t.Error("expected error when collateral is insufficient")
	}
}

// --- Transfer: global logger branch ---

func TestTransfer_WithLogger(t *testing.T) {
	ex := setupTransferExchange()
	ex.SetLogger("_global", &nullLogger{})
	if err := ex.Transfer(1, "spot", "perp", "USD", USDAmount(500)); err != nil {
		t.Fatalf("transfer with logger: %v", err)
	}
}

// --- cancelOrder: logger branch on success ---

func TestCancelOrder_WithLogger(t *testing.T) {
	ex := setupSpotExchange()
	ex.SetLogger("BTC/USD", &nullLogger{})

	orderID, _ := InjectLimitOrder(ex, 1, "BTC/USD", Buy, PriceUSD(48_000, DOLLAR_TICK), BTCAmount(1))
	if orderID == 0 {
		t.Fatal("order placement failed")
	}

	const reqID = uint64(6001)
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: orderID},
	}
	resp := sendRequest(ex.Gateways[1], req, reqID)
	if !resp.Success {
		t.Errorf("cancel with logger should succeed, got error=%v", resp.Error)
	}
}

// --- cancelOrder not owned with logger ---

func TestCancelOrder_NotOwnedWithLogger(t *testing.T) {
	ex := setupSpotExchange()
	ex.SetLogger("BTC/USD", &nullLogger{})

	orderID, _ := InjectLimitOrder(ex, 2, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))

	const reqID = uint64(6002)
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: orderID},
	}
	resp := sendRequest(ex.Gateways[1], req, reqID)
	if resp.Error != RejectOrderNotOwned {
		t.Errorf("expected RejectOrderNotOwned with logger, got %v", resp.Error)
	}
}

// --- calculateCollateralUsed: factor == 0 ---

func TestCalculateCollateralUsed_ZeroFactor(t *testing.T) {
	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	bm := NewBorrowingManager(BorrowingConfig{
		PriceSource:       oracle,
		CollateralFactors: map[string]float64{"USD": 0.0, "default": 0.0},
	})
	result := bm.CalculateCollateralUsed("USD", USDAmount(1_000))
	if result != 0 {
		t.Errorf("expected 0 for zero factor, got %d", result)
	}
}

// --- ReleaseCollateralFromPosition: isolated exists but collateral insufficient ---

func TestReleaseCollateralFromPosition_InsufficientAllocated(t *testing.T) {
	ex := setupMarginModeExchange()
	_ = ex.SetMarginMode(1, IsolatedMargin)

	// Allocate $1k
	_ = ex.AllocateCollateralToPosition(1, "BTC-PERP", "USD", USDAmount(1_000))

	// Try to release $5k (more than allocated)
	err := ex.ReleaseCollateralFromPosition(1, "BTC-PERP", "USD", USDAmount(5_000))
	if err == nil {
		t.Error("expected error when releasing more than allocated collateral")
	}
}

// --- EnablePeriodicSnapshots: running=true, snapshotInterval already nonzero ---

func TestEnablePeriodicSnapshots_AlreadyHasInterval(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: 2,
		SnapshotInterval: 100 * time.Millisecond,
	})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{}, &FixedFee{}) // starts running=true

	// snapshotInterval is already non-zero (100ms) → the `if e.snapshotInterval == 0` is false
	// → no new goroutine started, just updates interval
	ex.EnablePeriodicSnapshots(200 * time.Millisecond)
	ex.Shutdown()
}
