package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func setupMarginModeExchange() *Exchange {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(50_000))
	return ex
}

func TestSetMarginMode_Success(t *testing.T) {
	ex := setupMarginModeExchange()

	if err := ex.SetMarginMode(1, IsolatedMargin); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex.Clients[1].MarginMode != IsolatedMargin {
		t.Errorf("expected IsolatedMargin, got %v", ex.Clients[1].MarginMode)
	}
}

func TestSetMarginMode_UnknownClientReturnsError(t *testing.T) {
	ex := setupMarginModeExchange()

	err := ex.SetMarginMode(999, IsolatedMargin)
	if err == nil {
		t.Error("expected error for unknown client")
	}
}

func TestSetMarginMode_FailsWithOpenPositions(t *testing.T) {
	ex := setupMarginModeExchange()

	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(2, "USD", USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, BTCAmount(0.1))
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, BTCAmount(0.1))

	err := ex.SetMarginMode(1, IsolatedMargin)
	if err == nil {
		t.Error("expected error when changing margin mode with open position")
	}
}

func TestAllocateCollateralToPosition_Success(t *testing.T) {
	ex := setupMarginModeExchange()
	_ = ex.SetMarginMode(1, IsolatedMargin)

	perpBefore := ex.Clients[1].PerpBalances["USD"]
	amount := USDAmount(5_000)

	if err := ex.AllocateCollateralToPosition(1, "BTC-PERP", "USD", amount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	isolated := ex.Clients[1].IsolatedPositions["BTC-PERP"]
	if isolated == nil || isolated.Collateral["USD"] != amount {
		t.Errorf("isolated collateral: expected %d, got %v", amount, isolated)
	}
	if ex.Clients[1].PerpBalances["USD"] != perpBefore-amount {
		t.Errorf("perp balance: expected %d, got %d", perpBefore-amount, ex.Clients[1].PerpBalances["USD"])
	}
}

func TestAllocateCollateralToPosition_WrongModeReturnsError(t *testing.T) {
	ex := setupMarginModeExchange()
	// Default mode is CrossMargin

	err := ex.AllocateCollateralToPosition(1, "BTC-PERP", "USD", USDAmount(1_000))
	if err == nil {
		t.Error("expected error when client is not in isolated margin mode")
	}
}

func TestAllocateCollateralToPosition_InsufficientBalanceReturnsError(t *testing.T) {
	ex := setupMarginModeExchange()
	_ = ex.SetMarginMode(1, IsolatedMargin)

	err := ex.AllocateCollateralToPosition(1, "BTC-PERP", "USD", USDAmount(999_999_999))
	if err == nil {
		t.Error("expected error when balance is insufficient")
	}
}

func TestReleaseCollateralFromPosition_Success(t *testing.T) {
	ex := setupMarginModeExchange()
	_ = ex.SetMarginMode(1, IsolatedMargin)

	amount := USDAmount(5_000)
	_ = ex.AllocateCollateralToPosition(1, "BTC-PERP", "USD", amount)
	perpAfterAlloc := ex.Clients[1].PerpBalances["USD"]

	if err := ex.ReleaseCollateralFromPosition(1, "BTC-PERP", "USD", amount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ex.Clients[1].PerpBalances["USD"] != perpAfterAlloc+amount {
		t.Errorf("perp balance after release: expected %d, got %d", perpAfterAlloc+amount, ex.Clients[1].PerpBalances["USD"])
	}
	if ex.Clients[1].IsolatedPositions["BTC-PERP"].Collateral["USD"] != 0 {
		t.Errorf("isolated collateral should be zero after full release")
	}
}

func TestReleaseCollateralFromPosition_InsufficientCollateralReturnsError(t *testing.T) {
	ex := setupMarginModeExchange()
	_ = ex.SetMarginMode(1, IsolatedMargin)

	err := ex.ReleaseCollateralFromPosition(1, "BTC-PERP", "USD", USDAmount(1_000))
	if err == nil {
		t.Error("expected error when releasing more than allocated")
	}
}
