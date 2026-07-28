package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// Options must be visible to the cross-margin risk engine: a short option
// carries maintenance (MMBps of the underlying plus the premium mark) and its
// mark-to-market moves account equity. Before the PositionMarginer wiring,
// marginCore() resolved nil for options, so short volatility was
// unliquidatable and premium selling unlimited.

func optionRiskExchange(t *testing.T) (*Exchange, *PerpFutures, *EuropeanOption, *bughuntLiquidationHandler) {
	t.Helper()
	ex := NewExchange(3, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	expiry := time.Now().Add(24 * time.Hour).UnixNano()
	strike := PriceUSD(48_000, DOLLAR_TICK)
	opt := NewEuropeanOption("BTC-1-48000-C", "BTC", "USD", "BTC/USD",
		BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, strike, expiry, true)
	ex.AddInstrument(opt)

	handler := &bughuntLiquidationHandler{}
	ex.LiquidationHandler = handler

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(5_000))
	ex.AddPerpBalance(2, "USD", USDAmount(200_000))
	return ex, perp, opt, handler
}

func injectOptionRiskPositions(ex *Exchange, perpSize, optionSize, optionEntry int64) {
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: perpSize, EntryPrice: PriceUSD(50_000, DOLLAR_TICK),
	})
	pm.InjectPosition(1, "BTC-1-48000-C", &Position{
		ClientID: 1, Symbol: "BTC-1-48000-C", PositionSide: PositionBoth,
		Size: optionSize, EntryPrice: optionEntry,
	})
	pm.Unlock()
}

func TestRegressionShortOptionMaintenanceTriggersLiquidation(t *testing.T) {
	ex, perp, opt, handler := optionRiskExchange(t)

	// Short 1 option: MM = 7.5% x $50k underlying + $3k premium = $6,750,
	// against $5,000 equity. The tiny perp leg alone needs only $250.
	injectOptionRiskPositions(ex, BTCAmount(0.1), -BTCAmount(1), USDAmount(3_000))
	opt.SetMarks(USDAmount(50_000), USDAmount(3_000))

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, PriceUSD(50_000, DOLLAR_TICK))

	if handler.liquidations == 0 {
		t.Fatal("short option maintenance invisible to risk engine: $6,750 requirement vs $5,000 equity did not liquidate")
	}
}

func TestRegressionLongOptionCarriesNoMaintenance(t *testing.T) {
	ex, perp, opt, handler := optionRiskExchange(t)

	// Long option at a flat mark: premium is sunk, no maintenance owed. Only
	// the perp's $250 requirement stands against $5,000 equity.
	injectOptionRiskPositions(ex, BTCAmount(0.1), BTCAmount(1), USDAmount(3_000))
	opt.SetMarks(USDAmount(50_000), USDAmount(3_000))

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, PriceUSD(50_000, DOLLAR_TICK))

	if handler.liquidations != 0 {
		t.Fatal("long option charged maintenance: solvent account liquidated")
	}
}

func TestRegressionOptionMarkToMarketMovesEquity(t *testing.T) {
	ex, perp, opt, handler := optionRiskExchange(t)

	// Long option bought at $3k marks down to $100: -$2,900 unrealized loss
	// drops equity to $2,100, below the 1-BTC perp's $2,500 maintenance.
	injectOptionRiskPositions(ex, BTCAmount(1), BTCAmount(1), USDAmount(3_000))
	opt.SetMarks(USDAmount(50_000), USDAmount(100))

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, PriceUSD(50_000, DOLLAR_TICK))

	if handler.liquidations == 0 {
		t.Fatal("option mark-to-market loss invisible to risk engine: equity $2,100 vs $2,500 maintenance did not liquidate")
	}
}
