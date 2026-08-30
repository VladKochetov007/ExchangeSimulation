package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
)

type riskSemanticLiquidationRecorder struct {
	liquidations []uint64
}

func (r *riskSemanticLiquidationRecorder) OnMarginCall(*MarginCallEvent)       {}
func (r *riskSemanticLiquidationRecorder) OnInsuranceFund(*InsuranceFundEvent) {}
func (r *riskSemanticLiquidationRecorder) OnLiquidation(event *LiquidationEvent) {
	r.liquidations = append(r.liquidations, event.ClientID)
}

func newRiskSemanticUnderwaterExchange(t *testing.T) (*Exchange, *PerpFutures, *riskSemanticLiquidationRecorder) {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	recorder := &riskSemanticLiquidationRecorder{}
	ex.LiquidationHandler = recorder

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))

	positions := ex.Positions.(*PositionManager)
	positions.Lock()
	positions.InjectPosition(1, perp.Symbol(), &Position{
		ClientID: 1, Symbol: perp.Symbol(), PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	positions.Unlock()
	if _, reject := InjectLimitOrder(ex, 2, perp.Symbol(), Buy, USDAmount(94), BTCAmount(50)); reject != "" {
		t.Fatalf("covering bid rejected: %s", reject)
	}
	return ex, perp, recorder
}

func TestRiskIgnoresUnmarkedBookWithoutAccountExposure(t *testing.T) {
	ex, perp, recorder := newRiskSemanticUnderwaterExchange(t)
	defer ex.Shutdown()

	// The account has no position in this freshly listed, never-marked book.
	ex.AddInstrument(NewPerpFutures("ZZZ-PERP", "ZZZ", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.CheckLiquidations(perp.Symbol(), perp, USDAmount(94))

	if len(recorder.liquidations) == 0 {
		t.Fatal("liquidation was suppressed by an unmarked book without account exposure")
	}
}

func TestUnpriceableAccountDoesNotAbortOtherLiquidationChecks(t *testing.T) {
	ex, perp, recorder := newRiskSemanticUnderwaterExchange(t)
	defer ex.Shutdown()

	option := einstrument.NewEuropeanOption("BTC-C-100", "BTC", "USD", "BTC/USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, USDAmount(100), 1<<62, true)
	ex.AddInstrument(option)
	ex.ConnectNewClient(3, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(3, "USD", USDAmount(100))

	positions := ex.Positions.(*PositionManager)
	positions.Lock()
	positions.InjectPosition(1, option.Symbol(), &Position{
		ClientID: 1, Symbol: option.Symbol(), PositionSide: PositionBoth,
		Size: -BTCAmount(1), EntryPrice: USDAmount(5),
	})
	positions.InjectPosition(3, perp.Symbol(), &Position{
		ClientID: 3, Symbol: perp.Symbol(), PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	positions.Unlock()

	ex.CheckLiquidations(perp.Symbol(), perp, USDAmount(94))

	if len(recorder.liquidations) == 0 {
		t.Fatal("price failure for client 1 prevented liquidation check for client 3")
	}
}

type semanticConstantMark struct{ price int64 }

func (m *semanticConstantMark) Calculate(*OrderBook) (int64, error) { return m.price, nil }

func crossMarginOrderingCase(t *testing.T, riserSymbol, fallerSymbol string) bool {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	defer ex.Shutdown()

	riser := NewPerpFutures(riserSymbol, "RIS", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	faller := NewPerpFutures(fallerSymbol, "FAL", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(riser)
	ex.AddInstrument(faller)
	riserMark := &semanticConstantMark{price: USDAmount(100)}
	fallerMark := &semanticConstantMark{price: USDAmount(100)}
	recorder := &riskSemanticLiquidationRecorder{}
	ex.ConfigureAutomation(AutomationConfig{
		MarkPriceCalcs: map[string]MarkPriceCalculator{
			riserSymbol:  riserMark,
			fallerSymbol: fallerMark,
		},
		LiquidationHandler: recorder,
	})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(101))
	ex.AddPerpBalance(2, "USD", USDAmount(10_000_000))

	positions := ex.Positions.(*PositionManager)
	positions.Lock()
	for _, symbol := range []string{riserSymbol, fallerSymbol} {
		positions.InjectPosition(1, symbol, &Position{
			ClientID: 1, Symbol: symbol, PositionSide: PositionBoth,
			Size: BTCAmount(10), EntryPrice: USDAmount(100),
		})
	}
	positions.Unlock()
	if _, reject := InjectLimitOrder(ex, 2, fallerSymbol, Buy, USDAmount(70), BTCAmount(50)); reject != "" {
		t.Fatalf("covering bid rejected: %s", reject)
	}

	ex.UpdatePerpPrices()
	if len(recorder.liquidations) != 0 {
		t.Fatalf("setup tick liquidated the account: %d", len(recorder.liquidations))
	}
	riserMark.price = USDAmount(140)
	fallerMark.price = USDAmount(70)
	ex.UpdatePerpPrices()
	return len(recorder.liquidations) != 0
}

func TestCrossMarginRiskUsesOneCoherentMarkSetPerTick(t *testing.T) {
	riserFirst := crossMarginOrderingCase(t, "AAA-PERP", "BBB-PERP")
	fallerFirst := crossMarginOrderingCase(t, "BBB-PERP", "AAA-PERP")

	if riserFirst != fallerFirst {
		t.Fatalf("renaming symbols changed liquidation outcome: riser-first=%v faller-first=%v", riserFirst, fallerFirst)
	}
	if riserFirst {
		t.Fatal("both mark orderings liquidated an account solvent at the refreshed mark set")
	}
}
