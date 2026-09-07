package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-004. A bankrupt account is the classic place for an obligation to be
// extinguished without a payer. The expected numbers here are derived by hand
// from the contract, not read back from the production settlement functions,
// because a function cannot be its own oracle.
//
// Setup: client 1 is long 10 BTC entered at 100 USD with 100 USD of perp cash,
// and separately holds 500 USD in the spot wallet and owes 40 USD of borrowed
// USD. The mark falls to 80 and client 2 supplies the closing liquidity.
//
// By hand:
//
//	realized on close = 10 * (80 - 100)        = -200 USD
//	perp cash         = 100 - 200              = -100 USD  -> deficit 100
//	after write-down  = 0, insurance fund      = -100 USD
//
// What must hold, independently of how the code computes it:
//
//   - the deficit the insurance fund absorbs equals the negative cash exactly;
//   - the borrowed principal is still owed afterwards, because a liquidation
//     that pays a loss does not cancel a loan;
//   - conservation still holds, so both legs of the write-down were recorded.
func TestAuditLiquidationDeficitHasAPayerAndDebtSurvives(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler
	ex.LiquidationFeeBps = 0 // keep the arithmetic exactly the hand calculation

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(10_000))

	// A spot balance and a loan that the liquidation has no business silently
	// consuming or cancelling.
	ex.Clients[1].Balances["USD"] = USDAmount(500)
	ex.Clients[1].Borrowed["USD"] = USDAmount(40)
	spotBefore := ex.Clients[1].Balances["USD"]
	debtBefore := ex.Clients[1].Borrowed["USD"]

	// Those two lines are direct state injection and are themselves unrecorded
	// movements, so the tracker is already off by (spot - debt) before anything
	// happens. The question this test asks is what the LIQUIDATION contributes,
	// so the baseline gap is captured and the assertion is on the change.
	gapBefore := conservationGap(ex, "USD")

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(80), BTCAmount(10)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(80))

	if handler.liquidations == 0 {
		t.Fatal("H-004 not exercised: the underwater position was not liquidated")
	}

	const wantDeficit = 100.0 // USD, hand-derived above
	gotFund := ex.ExchangeBalance.InsuranceFund["USD"]
	if gotFund != -USDAmount(wantDeficit) {
		t.Errorf("insurance fund = %d, want %d: the absorbed deficit must equal the negative cash",
			gotFund, -USDAmount(wantDeficit))
	}
	if got := ex.Clients[1].PerpBalances["USD"]; got != 0 {
		t.Errorf("bankrupt perp cash = %d, want 0 after the write-down", got)
	}

	// The loan is a separate obligation from the trading loss. Liquidation may
	// repay it from available cash, but there was none: the account was already
	// negative. It must therefore still be owed.
	if got := ex.Clients[1].Borrowed["USD"]; got != debtBefore {
		t.Errorf("borrowed principal = %d, want %d: liquidation cancelled a loan without a creditor loss",
			got, debtBefore)
	}

	// Documents the wallet boundary rather than asserting a policy: if this
	// ever changes, the change is deliberate and visible here.
	if got := ex.Clients[1].Balances["USD"]; got != spotBefore {
		t.Errorf("spot balance = %d, want %d: the perp deficit reached the spot wallet", got, spotBefore)
	}

	if got, want := conservationGap(ex, "USD"), gapBefore; got != want {
		t.Errorf("liquidation moved the conservation gap from %d to %d: it mutated a balance without recording it",
			want, got)
	}
}

// conservationGap reports the tracker's held-minus-recorded gap for one asset,
// or zero when the tracker sees no discrepancy.
func conservationGap(ex *DefaultExchange, asset string) int64 {
	for _, v := range ex.VerifyConservation() {
		if v.Asset == asset {
			return v.Gap
		}
	}
	return 0
}
