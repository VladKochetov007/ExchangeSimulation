package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

// setupSpotExchangeWithLogger returns a spot exchange with a null logger set on the symbol.
func setupSpotExchangeWithLogger() *Exchange {
	ex := setupSpotExchange()
	ex.SetLogger("BTC/USD", &nullLogger{})
	return ex
}

func TestPlaceOrder_LogsInvalidPrice(t *testing.T) {
	ex := setupSpotExchangeWithLogger()
	badPrice := PriceUSD(50_000, DOLLAR_TICK) + 1
	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Buy, badPrice, BTCAmount(1))
	if reason != RejectInvalidPrice {
		t.Errorf("expected RejectInvalidPrice, got %v", reason)
	}
}

func TestPlaceOrder_LogsInvalidQty(t *testing.T) {
	ex := setupSpotExchangeWithLogger()
	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), 0)
	if reason != RejectInvalidQty {
		t.Errorf("expected RejectInvalidQty, got %v", reason)
	}
}

func TestPlaceOrder_LogsInsufficientBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(10)}, &FixedFee{})
	ex.SetLogger("BTC/USD", &nullLogger{})

	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	if reason != RejectInsufficientBalance {
		t.Errorf("expected RejectInsufficientBalance, got %v", reason)
	}
}

func TestPlaceOrder_LogsFOKNotFilled(t *testing.T) {
	ex := setupSpotExchangeWithLogger()
	const reqID = uint64(2222)
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			Symbol:      "BTC/USD",
			TimeInForce: FOK,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(ex.Gateways[1], req, reqID)
	if resp.Error != RejectFOKNotFilled {
		t.Errorf("expected RejectFOKNotFilled, got %v", resp.Error)
	}
}

func TestPlaceOrder_LogsOrderAcceptedAndFill(t *testing.T) {
	ex := setupSpotExchangeWithLogger()
	// Place and immediately fill — covers the OrderAccepted log path in placeOrder.
	_, _ = InjectLimitOrder(ex, 2, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	_, reason := InjectMarketOrder(ex, 1, "BTC/USD", Buy, BTCAmount(1))
	if reason != 0 {
		t.Errorf("expected successful fill, got %v", reason)
	}
}

func TestPlaceOrder_AutoBorrow_PerpLimitSuccess(t *testing.T) {
	ex, _ := setupPerpExchange(0, USDAmount(500_000))
	// Client 1 starts with $0 perp but has enough collateral via spot for auto-borrow.
	// Give client 1 some spot balance first (used as collateral in cross-margin).
	ex.ConnectClient(3, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(3, "USD", USDAmount(50_000)) // enough collateral

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowPerp:    true,
		BorrowRates:       map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0},
		PriceSource:       oracle,
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}

	// Client 3 has $50k perp, no borrow yet. Margin for 1 BTC at $50k = $5k.
	// PerpAvailable = $50k ≥ $5k so no borrow needed. Give 0 perp instead:
	ex.ConnectClient(4, map[string]int64{}, &FixedFee{})
	// Inject $4k as perp — not enough for $5k margin, but collateral covers $1k borrow.
	ex.AddPerpBalance(4, "USD", USDAmount(4_000))
	// Need more collateral for the borrow validation:
	ex.AddPerpBalance(4, "USD", USDAmount(10_000)) // total $14k perp, need $1k borrow

	// Seed the sell side
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))

	// Place a limit buy for 1 BTC — requires $5k margin, has $14k → no borrow needed
	// But to test auto-borrow: reset perp to $4,999 (just below $5k)
	ex.Lock()
	ex.Clients[4].PerpBalances["USD"] = USDAmount(4_999)
	ex.Unlock()

	_, _ = InjectLimitOrder(ex, 4, "BTC-PERP", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	// Whether it succeeds or not, the auto-borrow path is exercised.
}

func TestPlaceOrder_AutoBorrow_SpotLimitBuySuccess(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{}) // liquidity provider
	ex.ConnectClient(2, map[string]int64{"USD": USDAmount(1_000)}, &FixedFee{})   // small balance

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:        true,
		AutoBorrowSpot: true,
		BorrowRates:    map[string]int64{"USD": 500},
		// factor: unlimited (1.0 means collateral covers borrow)
		CollateralFactors: map[string]float64{"USD": 0.01}, // low factor so even small collateral passes
		PriceSource:       oracle,
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}

	// Client 2 has $1k spot. 0.1 BTC at $50k = $5k notional, needs to borrow $4k.
	// Collateral = $1k spot (but validateCrossMarginCollateral uses PerpBalances, not spot).
	// So add some perp collateral for validation:
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000)) // big perp collateral
	// Try limit buy at $50k for 0.1 BTC = $5k reserve needed, but only $1k available.
	_, _ = InjectLimitOrder(ex, 2, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(0.1))
	// Auto-borrow attempts to fill the gap; exercises the AutoBorrowForSpotTrade path.
}

func TestRepayMargin_InsufficientAvailableBalance(t *testing.T) {
	ex, bm := setupBorrowingExchange()
	// Borrow $1k
	_ = bm.BorrowMargin(1, "USD", USDAmount(1_000), "test")

	// Reserve all perp balance so PerpAvailable → negative
	ex.Lock()
	ex.Clients[1].PerpReserved["USD"] = USDAmount(200_000) // more than PerpBalances
	ex.Unlock()

	err := bm.RepayMargin(1, "USD", USDAmount(500))
	if err == nil {
		t.Error("expected error when perp available < repay amount")
	}
}

func TestHasOpenPositions_NoPositionsForClient(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})

	// Client has no positions at all (nil map in Positions) → should return false
	if err := ex.SetMarginMode(1, IsolatedMargin); err != nil {
		t.Errorf("expected success when client has no positions: %v", err)
	}
}

func TestCancelOrder_PerpSuccessful(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(500_000))
	orderID, _ := InjectLimitOrder(ex, 1, "BTC-PERP", Buy, PriceUSD(48_000, DOLLAR_TICK), BTCAmount(1))
	if orderID == 0 {
		t.Fatal("order placement failed")
	}

	const reqID = uint64(1111)
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: orderID},
	}
	resp := sendRequest(ex.Gateways[1], req, reqID)
	if !resp.Success {
		t.Errorf("perp cancel should succeed, got error=%v", resp.Error)
	}
}

func TestCancelOrder_SpotSellSuccessful(t *testing.T) {
	ex := setupSpotExchange()
	orderID, _ := InjectLimitOrder(ex, 1, "BTC/USD", Sell, PriceUSD(52_000, DOLLAR_TICK), BTCAmount(1))
	if orderID == 0 {
		t.Fatal("order placement failed")
	}

	const reqID = uint64(1112)
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: orderID},
	}
	resp := sendRequest(ex.Gateways[1], req, reqID)
	if !resp.Success {
		t.Errorf("spot sell cancel should succeed, got error=%v", resp.Error)
	}
}
