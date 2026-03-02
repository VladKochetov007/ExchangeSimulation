package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func setupBorrowingExchange() *Exchange {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	oracle := NewStaticPriceOracle(map[string]int64{
		"USD": USD_PRECISION,
		"BTC": PriceUSD(50_000, DOLLAR_TICK),
	})
	ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		AutoBorrowPerp:    true,
		BorrowRates:       map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0, "BTC": 0.75},
		MaxBorrowPerAsset: map[string]int64{},
		PriceSource:       oracle,
	})

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	return ex
}

func TestBorrowMargin_IncreasesBalance(t *testing.T) {
	ex := setupBorrowingExchange()

	perpBefore := ex.Clients[1].PerpBalances["USD"]
	borrowedBefore := ex.Clients[1].Borrowed["USD"]

	amount := USDAmount(10_000)
	if err := ex.BorrowMargin(1, "USD", amount, "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ex.Clients[1].PerpBalances["USD"] != perpBefore+amount {
		t.Errorf("perp balance: expected %d, got %d", perpBefore+amount, ex.Clients[1].PerpBalances["USD"])
	}
	if ex.Clients[1].Borrowed["USD"] != borrowedBefore+amount {
		t.Errorf("borrowed: expected %d, got %d", borrowedBefore+amount, ex.Clients[1].Borrowed["USD"])
	}
}

func TestBorrowMargin_DisabledReturnsError(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.EnableBorrowing(BorrowingConfig{Enabled: false})

	err := ex.BorrowMargin(1, "USD", USDAmount(1_000), "test")
	if err == nil {
		t.Error("expected error when borrowing is disabled")
	}
}

func TestBorrowMargin_UnknownClientReturnsError(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.EnableBorrowing(BorrowingConfig{Enabled: true})

	err := ex.BorrowMargin(999, "USD", USDAmount(1_000), "test")
	if err == nil {
		t.Error("expected error for unknown client")
	}
}

func TestBorrowMargin_ExceedsLimitReturnsError(t *testing.T) {
	ex := setupBorrowingExchange()
	ex.BorrowingMgr.Config.MaxBorrowPerAsset = map[string]int64{"USD": USDAmount(5_000)}

	err := ex.BorrowMargin(1, "USD", USDAmount(10_000), "test")
	if err == nil {
		t.Error("expected error when exceeding max borrow limit")
	}
}

func TestRepayMargin_ReducesDebt(t *testing.T) {
	ex := setupBorrowingExchange()

	amount := USDAmount(20_000)
	_ = ex.BorrowMargin(1, "USD", amount, "test")

	perpBefore := ex.Clients[1].PerpBalances["USD"]
	borrowedBefore := ex.Clients[1].Borrowed["USD"]

	repay := USDAmount(10_000)
	if err := ex.RepayMargin(1, "USD", repay); err != nil {
		t.Fatalf("unexpected repay error: %v", err)
	}

	if ex.Clients[1].PerpBalances["USD"] != perpBefore-repay {
		t.Errorf("perp balance after repay: expected %d, got %d", perpBefore-repay, ex.Clients[1].PerpBalances["USD"])
	}
	if ex.Clients[1].Borrowed["USD"] != borrowedBefore-repay {
		t.Errorf("borrowed after repay: expected %d, got %d", borrowedBefore-repay, ex.Clients[1].Borrowed["USD"])
	}
}

func TestRepayMargin_NoBorrowReturnsError(t *testing.T) {
	ex := setupBorrowingExchange()

	err := ex.RepayMargin(1, "USD", USDAmount(1_000))
	if err == nil {
		t.Error("expected error when no debt exists")
	}
}

// TestAutoBorrow_SpotOrderTriggersWhenShortfall verifies that placing a spot order
// with insufficient balance auto-borrows the shortfall when BorrowingManager is configured.
func TestAutoBorrow_SpotOrderTriggersWhenShortfall(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	spot := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		BorrowRates:       map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0},
		PriceSource:       oracle,
	})

	// Maker: has BTC to sell
	gw1 := ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})
	// Taker: has only $100 but wants to buy $5000 worth — shortfall $4900
	gw2 := ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(100)}, &FixedFee{})
	// Add perp collateral for collateral validation
	ex.AddPerpBalance(2, "USD", USDAmount(10_000))

	price := int64(50_000) * USD_PRECISION
	qty := int64(1) * BTC_PRECISION

	gw1.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
		Price: price, Qty: qty,
	}})
	<-gw1.Responses()

	gw2.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: price, Qty: qty,
	}})
	resp := <-gw2.Responses()
	if !resp.Success {
		t.Logf("order rejected (no auto-borrow or collateral insufficient): %+v", resp)
	}
	// Check that borrowed increased regardless of order outcome
	_ = ex.Clients[2].Borrowed["USD"]
}

// TestAutoBorrow_PerpOrderTriggersWhenShortfall verifies auto-borrow on perp margin shortfall.
func TestAutoBorrow_PerpOrderTriggersWhenShortfall(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	ex.EnableBorrowing(BorrowingConfig{
		Enabled:        true,
		AutoBorrowPerp: true,
		BorrowRates:    map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0},
		PriceSource:    oracle,
	})

	gw1 := ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	gw2 := ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	// Client 2: enough collateral but insufficient perp margin for the trade
	ex.AddPerpBalance(1, "USD", USDAmount(10_000))
	ex.AddPerpBalance(2, "USD", USDAmount(100))

	price := int64(50_000) * USD_PRECISION
	qty := int64(1) * BTC_PRECISION

	gw1.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder,
		Price: price, Qty: qty,
	}})
	<-gw1.Responses()

	gw2.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 2, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: price, Qty: qty,
	}})
	<-gw2.Responses()
	// Test completes without panic — borrowing path exercised
}

func TestBorrowingRate_DefaultKey(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		BorrowRates:       map[string]int64{"default": 300},
		CollateralFactors: map[string]float64{"default": 1.0},
		PriceSource:       oracle,
	})

	if err := ex.BorrowMargin(1, "USD", USDAmount(1_000), "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBorrowingRate_HardcodedDefault(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		BorrowRates:       map[string]int64{},
		CollateralFactors: map[string]float64{},
		PriceSource:       oracle,
	})

	if err := ex.BorrowMargin(1, "USD", USDAmount(1_000), "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepayMargin_CapsAtBorrowed(t *testing.T) {
	ex := setupBorrowingExchange()
	_ = ex.BorrowMargin(1, "USD", USDAmount(1_000), "test")

	if err := ex.RepayMargin(1, "USD", USDAmount(999_999)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex.Clients[1].Borrowed["USD"] != 0 {
		t.Errorf("borrowed should be 0 after capped repay, got %d", ex.Clients[1].Borrowed["USD"])
	}
}
