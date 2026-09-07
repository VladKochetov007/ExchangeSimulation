package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// H-025. Three parts of this system value the same account, and they do not use
// the same price.
//
//   - MarkedAccount (exchange/valuation.go) reports for telemetry and scoring.
//     It moves no money, and its contract makes the caller supply every
//     conversion explicitly rather than letting the venue invent an FX graph.
//   - buildAccountMarginProfile decides liquidation. It values positions through
//     riskMark and fails closed when no valid mark exists.
//   - validateCrossMarginCollateral decides leverage. It reads
//     BorrowingConfig.PriceSource, which the campaign fills with a static oracle
//     pinned to the bootstrap price (simulations/multivenue/sim.go:2862).
//
// This test puts one account, at one instant, through the two that are callable
// from here and prints the gap. The third is established by reading:
// populationValuationSpec (simulations/multivenue/sim.go:4308) builds the
// scoring spec from a live two-sided ABC/USD mid with a bounded staleness
// window, records which source it used in markSource, and fails closed on a
// non-positive mark — the most disciplined of the three, feeding the decision
// with the least authority.
func TestAuditTheSameAccountIsValuedByThreeDifferentPrices(t *testing.T) {
	const bootstrap = 50_000 * USD_PRECISION
	const live = 25_000 * USD_PRECISION // the market has halved
	const held = 10 * BTC_PRECISION     // 10 ABC of collateral

	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	if err := perp.UpdateFundingRate(live, live); err != nil {
		t.Fatalf("live mark: %v", err)
	}
	// The borrow gate's oracle still says the bootstrap price, exactly as the
	// campaign configures it.
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		DefaultMarginMode: CrossMargin,
		CollateralFactors: map[string]float64{"USD": 1},
		MaxBorrowPerAsset: map[string]int64{"USD": 100_000_000 * USD_PRECISION},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "ABC": BTC_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "ABC": bootstrap}),
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	ex.ConnectNewClient(1, map[string]int64{"ABC": held}, &FixedFee{})

	// What the scoring path reports, given the live mark the campaign's spec
	// builder would supply.
	atLive, err := ex.MarkedAccount(1, etypes.AccountValuationSpec{
		ReportAsset: "USD", ReportPrecision: USD_PRECISION,
		AssetMarks: map[string]etypes.AssetValuationMark{
			"USD": {Price: USD_PRECISION, Precision: USD_PRECISION},
			"ABC": {Price: live, Precision: BTC_PRECISION},
		},
	})
	if err != nil {
		t.Fatalf("marked account at the live mark: %v", err)
	}

	// The same account valued at the price the borrow gate is still using.
	atBootstrap, err := ex.MarkedAccount(1, etypes.AccountValuationSpec{
		ReportAsset: "USD", ReportPrecision: USD_PRECISION,
		AssetMarks: map[string]etypes.AssetValuationMark{
			"USD": {Price: USD_PRECISION, Precision: USD_PRECISION},
			"ABC": {Price: bootstrap, Precision: BTC_PRECISION},
		},
	})
	if err != nil {
		t.Fatalf("marked account at the bootstrap mark: %v", err)
	}

	t.Logf("10 ABC held, market at 25 000, bootstrap 50 000:")
	t.Logf("  scoring equity at the live mark:      %d (%.0f USD)", atLive.Equity, float64(atLive.Equity)/USD_PRECISION)
	t.Logf("  scoring equity at the bootstrap mark: %d (%.0f USD)", atBootstrap.Equity, float64(atBootstrap.Equity)/USD_PRECISION)

	if atLive.Equity >= atBootstrap.Equity {
		t.Fatalf("the scoring path did not track the mark it was given: %d at live, %d at bootstrap",
			atLive.Equity, atBootstrap.Equity)
	}
	// The scoring path is a pure function of the mark it is handed, which is
	// what makes its caller's choice of mark the thing that matters.
	if got, want := atBootstrap.Equity, 2*atLive.Equity; got != want {
		t.Errorf("halving the mark did not halve reported equity: %d then %d", want/2, got)
	}

	// Meanwhile the borrow gate, at the same instant on the same account, is
	// still lending against the bootstrap valuation.
	if err := ex.BorrowMargin(1, "USD", atBootstrap.Equity, "audit"); err != nil {
		t.Errorf("the gate refused a borrow equal to the bootstrap valuation: %v", err)
	}
	if err := ex.BorrowMargin(1, "USD", 1, "audit"); err == nil {
		t.Logf("note: the gate still admits more after lending the full bootstrap valuation")
	}
	t.Logf("  borrow admitted against the same 10 ABC: %.0f USD, which is %.0fx the account's live equity",
		float64(atBootstrap.Equity)/USD_PRECISION,
		float64(atBootstrap.Equity)/float64(atLive.Equity))
}
