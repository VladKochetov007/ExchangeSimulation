package exchange_test

import (
	"os"
	"testing"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
)

// requireAuditFindings gates the reproductions of OPEN audit findings. They
// fail by design: each one demonstrates behaviour this audit argues is wrong
// but which has not been changed. Run them with AUDIT_FINDINGS=1 to see the
// failure; they are skipped otherwise so an open finding does not turn the
// whole suite red. Delete the gate when the finding is fixed.
func requireAuditFindings(t *testing.T) {
	t.Helper()
	if os.Getenv("AUDIT_FINDINGS") == "" {
		t.Skip("open audit finding; set AUDIT_FINDINGS=1 to reproduce")
	}
}

type auditLiquidationRecorder struct{ liquidations int }

func (h *auditLiquidationRecorder) OnMarginCall(*MarginCallEvent)       {}
func (h *auditLiquidationRecorder) OnInsuranceFund(*InsuranceFundEvent) {}
func (h *auditLiquidationRecorder) OnLiquidation(*LiquidationEvent)     { h.liquidations++ }

// auditUnderwaterOnBTC builds an exchange where client 1 is below maintenance
// on BTC-PERP at a mark of 94, with client 2 resting the covering bid.
func auditUnderwaterOnBTC(t *testing.T) (*Exchange, *PerpFutures, *auditLiquidationRecorder) {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	handler := &auditLiquidationRecorder{}
	ex.LiquidationHandler = handler

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(94), BTCAmount(50)); reject != "" {
		t.Fatalf("covering bid rejected: %s", reject)
	}
	return ex, perp, handler
}

// CONTROL: with only the trigger book listed the undercollateralized account
// is liquidated, as the risk engine intends.
func TestAuditControlUnderwaterAccountIsLiquidated(t *testing.T) {
	ex, perp, handler := auditUnderwaterOnBTC(t)
	defer ex.Shutdown()

	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("control: underwater account was not liquidated")
	}
}

// FINDING 1: a second perp in the same quote asset, in which the account holds
// NO position and which has no mark and an empty book, suppresses the
// liquidation entirely. The account's own risk is fully priceable.
func TestAuditUnmarkedZeroExposureBookSuppressesLiquidation(t *testing.T) {
	requireAuditFindings(t)
	ex, perp, handler := auditUnderwaterOnBTC(t)
	defer ex.Shutdown()

	// Freshly listed, never marked, empty book; client 1 has no exposure to it.
	unrelated := NewPerpFutures("ZZZ-PERP", "ZZZ", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(unrelated)

	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("FINDING: liquidation suppressed by an unmarked book the account holds no position in")
	}
}

// FINDING 2: a profile failure attributable to ONE account aborts the whole
// sweep, so every higher-numbered client escapes this mark's liquidation
// check. Client 1 holds an unpriceable option; client 3 holds only the
// fully-priceable perp.
func TestAuditProfileFailureAbortsSweepForOtherAccounts(t *testing.T) {
	requireAuditFindings(t)
	ex, perp, handler := auditUnderwaterOnBTC(t)
	defer ex.Shutdown()

	// Never marked: PositionMark and UnderlyingMark both fail.
	opt := einstrument.NewEuropeanOption("BTC-C-100", "BTC", "USD", "BTC/USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, USDAmount(100), 1<<62, true)
	ex.AddInstrument(opt)

	ex.ConnectNewClient(3, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(3, "USD", USDAmount(100))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-C-100", &Position{
		ClientID: 1, Symbol: "BTC-C-100", PositionSide: PositionBoth,
		Size: -BTCAmount(1), EntryPrice: USDAmount(5),
	})
	pm.InjectPosition(3, "BTC-PERP", &Position{
		ClientID: 3, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("FINDING: client 3 (fully priceable and underwater) escaped liquidation because client 1's profile failed")
	}
}
