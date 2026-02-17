package exchange

import "testing"

func setupBorrowingExchange() (*Exchange, *BorrowingManager) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	oracle := NewStaticPriceOracle(map[string]int64{
		"USD": USD_PRECISION,
		"BTC": PriceUSD(50_000, DOLLAR_TICK),
	})
	config := BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		AutoBorrowPerp:    true,
		BorrowRates:       map[string]int64{"USD": 500},
		CollateralFactors: map[string]float64{"USD": 1.0, "BTC": 0.75},
		MaxBorrowPerAsset: map[string]int64{},
		PriceOracle:       oracle,
	}
	bm := NewBorrowingManager(ex, config)

	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	return ex, bm
}

func TestBorrowMargin_IncreasesBalance(t *testing.T) {
	ex, bm := setupBorrowingExchange()

	perpBefore := ex.Clients[1].PerpBalances["USD"]
	borrowedBefore := ex.Clients[1].Borrowed["USD"]

	amount := USDAmount(10_000)
	if err := bm.BorrowMargin(1, "USD", amount, "test"); err != nil {
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
	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	bm := NewBorrowingManager(ex, BorrowingConfig{Enabled: false})

	err := bm.BorrowMargin(1, "USD", USDAmount(1_000), "test")
	if err == nil {
		t.Error("expected error when borrowing is disabled")
	}
}

func TestBorrowMargin_UnknownClientReturnsError(t *testing.T) {
	bm := NewBorrowingManager(NewExchange(10, &RealClock{}), BorrowingConfig{Enabled: true})

	err := bm.BorrowMargin(999, "USD", USDAmount(1_000), "test")
	if err == nil {
		t.Error("expected error for unknown client")
	}
}

func TestBorrowMargin_ExceedsLimitReturnsError(t *testing.T) {
	_, bm := setupBorrowingExchange()
	bm.config.MaxBorrowPerAsset = map[string]int64{"USD": USDAmount(5_000)}

	err := bm.BorrowMargin(1, "USD", USDAmount(10_000), "test")
	if err == nil {
		t.Error("expected error when exceeding max borrow limit")
	}
}

func TestRepayMargin_ReducesDebt(t *testing.T) {
	ex, bm := setupBorrowingExchange()

	amount := USDAmount(20_000)
	_ = bm.BorrowMargin(1, "USD", amount, "test")

	perpBefore := ex.Clients[1].PerpBalances["USD"]
	borrowedBefore := ex.Clients[1].Borrowed["USD"]

	repay := USDAmount(10_000)
	if err := bm.RepayMargin(1, "USD", repay); err != nil {
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
	_, bm := setupBorrowingExchange()

	err := bm.RepayMargin(1, "USD", USDAmount(1_000))
	if err == nil {
		t.Error("expected error when no debt exists")
	}
}

func TestAutoBorrowForSpotTrade_SkipsWhenDisabled(t *testing.T) {
	ex2 := NewExchange(10, &RealClock{})
	ex2.ConnectClient(1, map[string]int64{"USD": USDAmount(1_000)}, &FixedFee{})
	bm := NewBorrowingManager(ex2, BorrowingConfig{Enabled: false, AutoBorrowSpot: true})

	borrowed, err := bm.AutoBorrowForSpotTrade(1, "USD", USDAmount(5_000))
	if err != nil || borrowed {
		t.Errorf("expected (false, nil) when disabled, got (%v, %v)", borrowed, err)
	}
}


func TestAutoBorrowForSpotTrade_NoOpWhenSufficient(t *testing.T) {
	ex, bm := setupBorrowingExchange()
	ex.ConnectClient(2, map[string]int64{"USD": USDAmount(50_000)}, &FixedFee{})

	// Client 2 has $50k spot, needs only $10k
	borrowed, err := bm.AutoBorrowForSpotTrade(2, "USD", USDAmount(10_000))
	if err != nil || borrowed {
		t.Errorf("expected no borrow when balance is sufficient, got (%v, %v)", borrowed, err)
	}
}

func TestAutoBorrowForSpotTrade_BorrowsWhenShortfall(t *testing.T) {
	ex, bm := setupBorrowingExchange()
	// Client 1 has $100k perp but $0 spot. AutoBorrowSpot uses spot balance.
	// So we need a client with low spot balance.
	ex.ConnectClient(3, map[string]int64{"USD": USDAmount(100)}, &FixedFee{})
	// Add enough perp to cover collateral validation
	ex.AddPerpBalance(3, "USD", USDAmount(100_000))

	spotBefore := ex.Clients[3].Balances["USD"]

	borrowed, err := bm.AutoBorrowForSpotTrade(3, "USD", USDAmount(10_000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !borrowed {
		t.Error("expected borrow to occur when spot balance is insufficient")
	}
	_ = spotBefore
}

func TestAutoBorrowForPerpTrade_SkipsWhenDisabled(t *testing.T) {
	ex3 := NewExchange(10, &RealClock{})
	ex3.ConnectClient(1, map[string]int64{}, &FixedFee{})
	bm := NewBorrowingManager(ex3, BorrowingConfig{Enabled: false, AutoBorrowPerp: true})

	borrowed, err := bm.AutoBorrowForPerpTrade(1, "USD", USDAmount(5_000))
	if err != nil || borrowed {
		t.Errorf("expected (false, nil) when disabled, got (%v, %v)", borrowed, err)
	}
}


func TestAutoBorrowForPerpTrade_NoOpWhenSufficient(t *testing.T) {
	_, bm := setupBorrowingExchange()

	// Client 1 has $100k perp, needs only $10k
	borrowed, err := bm.AutoBorrowForPerpTrade(1, "USD", USDAmount(10_000))
	if err != nil || borrowed {
		t.Errorf("expected no borrow when perp balance is sufficient, got (%v, %v)", borrowed, err)
	}
}

func TestAutoBorrowForPerpTrade_BorrowsWhenShortfall(t *testing.T) {
	ex, bm := setupBorrowingExchange()
	ex.ConnectClient(3, map[string]int64{}, &FixedFee{})
	// $9k perp available, needs $10k → shortfall $1k.
	// Collateral validation: $9k covers $1k borrow (9k >= 1k with factor 1.0).
	ex.AddPerpBalance(3, "USD", USDAmount(9_000))

	borrowed, err := bm.AutoBorrowForPerpTrade(3, "USD", USDAmount(10_000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !borrowed {
		t.Error("expected borrow to occur when perp balance is insufficient")
	}
}

func TestBorrowingRate_DefaultKey(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	bm := NewBorrowingManager(ex, BorrowingConfig{
		Enabled:           true,
		BorrowRates:       map[string]int64{"default": 300},
		CollateralFactors: map[string]float64{"default": 1.0},
		PriceOracle:       oracle,
	})

	// BorrowMargin internally calls getRate("USD") → falls through to "default" key
	if err := bm.BorrowMargin(1, "USD", USDAmount(1_000), "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBorrowingRate_HardcodedDefault(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100_000))

	// Empty BorrowRates and CollateralFactors maps → hardcoded defaults (500 bps, 0.75)
	oracle := NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION})
	bm := NewBorrowingManager(ex, BorrowingConfig{
		Enabled:           true,
		BorrowRates:       map[string]int64{},
		CollateralFactors: map[string]float64{},
		PriceOracle:       oracle,
	})

	if err := bm.BorrowMargin(1, "USD", USDAmount(1_000), "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRepayMargin_CapsAtBorrowed(t *testing.T) {
	ex, bm := setupBorrowingExchange()
	_ = bm.BorrowMargin(1, "USD", USDAmount(1_000), "test")

	// Try to repay more than borrowed — should cap at borrowed amount, not error
	if err := bm.RepayMargin(1, "USD", USDAmount(999_999)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex.Clients[1].Borrowed["USD"] != 0 {
		t.Errorf("borrowed should be 0 after capped repay, got %d", ex.Clients[1].Borrowed["USD"])
	}
}
