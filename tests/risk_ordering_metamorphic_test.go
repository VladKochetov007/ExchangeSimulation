package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// constantMark is a per-symbol mark calculator whose value the test advances
// between price ticks, standing in for an external index move.
type constantMark struct{ price int64 }

func (c *constantMark) Calculate(*OrderBook) (int64, error) { return c.price, nil }

// auditCrossMarginOrderingCase runs one full price tick over two cross-margined
// perps in the same quote asset: riser's mark goes 100 -> 140, faller's goes
// 100 -> 70. The account is SOLVENT against the fully refreshed mark set.
// Returns whether it was liquidated.
func auditCrossMarginOrderingCase(t *testing.T, riserSymbol, fallerSymbol string) bool {
	t.Helper()

	ex := NewExchange(4, &RealClock{})
	defer ex.Shutdown()

	riser := NewPerpFutures(riserSymbol, "RIS", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	faller := NewPerpFutures(fallerSymbol, "FAL", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(riser)
	ex.AddInstrument(faller)

	riserMark := &constantMark{price: USDAmount(100)}
	fallerMark := &constantMark{price: USDAmount(100)}
	handler := &auditLiquidationRecorder{}
	ex.ConfigureAutomation(AutomationConfig{
		MarkPriceCalcs: map[string]MarkPriceCalculator{
			riserSymbol:  riserMark,
			fallerSymbol: fallerMark,
		},
		LiquidationHandler: handler,
	})

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(101))
	ex.AddPerpBalance(2, "USD", USDAmount(10_000_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	for _, symbol := range []string{riserSymbol, fallerSymbol} {
		pm.InjectPosition(1, symbol, &Position{
			ClientID: 1, Symbol: symbol, PositionSide: PositionBoth,
			Size: BTCAmount(10), EntryPrice: USDAmount(100),
		})
	}
	pm.Unlock()

	// Covering liquidity so a triggered liquidation actually closes.
	if _, reject := InjectLimitOrder(ex, 2, fallerSymbol, Buy, USDAmount(70), BTCAmount(50)); reject != "" {
		t.Fatalf("covering bid rejected: %s", reject)
	}

	// Tick 1 establishes both stored marks at 100. Equity 101 vs maintenance
	// 100: solvent, no liquidation.
	ex.UpdatePerpPrices()
	if handler.liquidations != 0 {
		t.Fatalf("setup tick liquidated the account (%d)", handler.liquidations)
	}

	// Tick 2. Fully refreshed: equity = 101 + 400 - 300 = 201 against a
	// maintenance requirement of 105. The account is solvent.
	riserMark.price = USDAmount(140)
	fallerMark.price = USDAmount(70)
	ex.UpdatePerpPrices()

	return handler.liquidations > 0
}

// Reordering two economically independent mark updates within one price tick
// must not change whether an account is liquidated. It does: the per-symbol
// liquidation sweep runs interleaved with mark application in lexicographic
// symbol order, so a sweep triggered by the faller sees the riser's PREVIOUS
// mark. Renaming the instruments — economically meaningless — flips a solvent
// account into a liquidated one.
func TestAuditSameTickMarkOrderingChangesLiquidationOutcome(t *testing.T) {
	requireAuditFindings(t)
	riserFirst := auditCrossMarginOrderingCase(t, "AAA-PERP", "BBB-PERP")
	fallerFirst := auditCrossMarginOrderingCase(t, "BBB-PERP", "AAA-PERP")

	if riserFirst != fallerFirst {
		t.Fatalf("FINDING: liquidation outcome depends on symbol sort order: riser-first liquidated=%v, faller-first liquidated=%v (account is solvent at the refreshed mark set in both)",
			riserFirst, fallerFirst)
	}
	if riserFirst {
		t.Fatal("both orderings liquidated a solvent account")
	}
}
