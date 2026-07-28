package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

type fundEventRecorder struct {
	liquidations int
	fundEvents   []*InsuranceFundEvent
}

func (h *fundEventRecorder) OnMarginCall(*MarginCallEvent) {}
func (h *fundEventRecorder) OnInsuranceFund(e *InsuranceFundEvent) {
	h.fundEvents = append(h.fundEvents, e)
}
func (h *fundEventRecorder) OnLiquidation(*LiquidationEvent) { h.liquidations++ }

// With LiquidationFeeBps set, a liquidation charges the closed account a
// clearance fee on the closed notional and CREDITS the insurance fund — the
// fund was previously write-only-negative (only ever debited on deficits), so
// it could never absorb a cascade.
func TestRegressionLiquidationClearanceFeeCreditsInsuranceFund(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler
	ex.LiquidationFeeBps = 30

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(10_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(94), BTCAmount(10)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("underwater position not liquidated")
	}

	wantFee := MulDiv(BTCAmount(10), USDAmount(94), BTC_PRECISION) * 30 / 10000
	if got := ex.ExchangeBalance.InsuranceFund["USD"]; got != wantFee {
		t.Fatalf("insurance fund = %d, want clearance fee %d credited", got, wantFee)
	}

	// Fee came out of the client: entry 100, close at 94 on 10 BTC = -60
	// realized, leaving 40, minus the fee.
	if got := ex.Clients[1].PerpBalances["USD"]; got != USDAmount(40)-wantFee {
		t.Fatalf("client balance = %d, want %d", got, USDAmount(40)-wantFee)
	}

	var creditEvent *InsuranceFundEvent
	for _, ev := range handler.fundEvents {
		if ev.Reason == "clearance_fee" {
			creditEvent = ev
		}
	}
	if creditEvent == nil {
		t.Fatal("no clearance_fee insurance fund event fired")
	}
	if creditEvent.Delta != wantFee || creditEvent.Balance != wantFee {
		t.Fatalf("fund event delta=%d balance=%d, want %d/%d", creditEvent.Delta, creditEvent.Balance, wantFee, wantFee)
	}
}

// Zero LiquidationFeeBps (the default) must preserve pre-fee economics
// exactly: no fee debit, no fund credit.
func TestRegressionZeroLiquidationFeeKeepsOldEconomics(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(10_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(94), BTCAmount(10)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("underwater position not liquidated")
	}
	if got := ex.ExchangeBalance.InsuranceFund["USD"]; got != 0 {
		t.Fatalf("insurance fund touched with fee disabled: %d", got)
	}
	if got := ex.Clients[1].PerpBalances["USD"]; got != USDAmount(40) {
		t.Fatalf("client balance = %d, want %d untouched by fees", got, USDAmount(40))
	}
}
