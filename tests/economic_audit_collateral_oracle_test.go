package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-024. The risk engine values derivative exposure through riskMark, which
// tracks each instrument's stored funding mark. The borrow gate does not use
// riskMark at all: it reads BorrowingConfig.PriceSource, and the campaign
// supplies NewStaticPriceOracle with ABC pinned to the 50 000 bootstrap
// (simulations/multivenue/sim.go:2862).
//
// So borrowing power derived from ABC collateral is pinned to the price ABC had
// before the simulation began. This test measures what that means when the
// market moves.
func TestAuditBorrowCollateralIsPricedByAStaticOracle(t *testing.T) {
	const symbol = "ABC-PERP"
	const bootstrap = 50_000 * USD_PRECISION

	// admitted reports the largest USD borrow the gate allows for an account
	// holding `abc` units of ABC, when the instrument's live mark is `liveMark`
	// but the collateral oracle still says `oraclePrice`.
	admitted := func(t *testing.T, abc int64, liveMark, oraclePrice int64) int64 {
		t.Helper()
		build := func() *DefaultExchange {
			ex := NewExchange(4, &RealClock{})
			perp := NewPerpFutures(symbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
			ex.AddInstrument(perp)
			if err := perp.UpdateFundingRate(liveMark, liveMark); err != nil {
				t.Fatalf("live mark: %v", err)
			}
			if err := ex.EnableBorrowing(BorrowingConfig{
				Enabled:           true,
				DefaultMarginMode: CrossMargin,
				CollateralFactors: map[string]float64{"USD": 1},
				MaxBorrowPerAsset: map[string]int64{"USD": 100_000_000 * USD_PRECISION},
				AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "ABC": BTC_PRECISION},
				PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "ABC": oraclePrice}),
			}); err != nil {
				t.Fatalf("enable borrowing: %v", err)
			}
			ex.ConnectNewClient(1, map[string]int64{"ABC": abc}, &FixedFee{})
			return ex
		}
		low, high := int64(0), int64(10_000_000)*USD_PRECISION
		for low+1 < high {
			mid := low + (high-low)/2
			if build().BorrowMargin(1, "USD", mid, "audit") == nil {
				low = mid
			} else {
				high = mid
			}
		}
		return low
	}

	const held = 10 * BTC_PRECISION // 10 ABC

	atBootstrap := admitted(t, held, bootstrap, bootstrap)
	afterCrash := admitted(t, held, bootstrap/2, bootstrap)
	repriced := admitted(t, held, bootstrap/2, bootstrap/2)

	t.Logf("10 ABC collateral, ABC bootstrap 50 000:")
	t.Logf("  market at 50 000, oracle 50 000: borrow %.0f USD", float64(atBootstrap)/USD_PRECISION)
	t.Logf("  market at 25 000, oracle 50 000: borrow %.0f USD", float64(afterCrash)/USD_PRECISION)
	t.Logf("  market at 25 000, oracle 25 000: borrow %.0f USD", float64(repriced)/USD_PRECISION)

	if atBootstrap <= 0 {
		t.Fatalf("the control account could not borrow, so this fixture proves nothing")
	}
	if afterCrash != atBootstrap {
		t.Errorf("the borrow limit now tracks the live market (%d then %d): the staleness this test documents has changed",
			atBootstrap, afterCrash)
	}
	// The counterfactual: an oracle that had repriced would have halved it.
	if repriced >= afterCrash {
		t.Errorf("repricing the oracle did not reduce the limit: %d then %d", afterCrash, repriced)
	}
	t.Logf("a 50%% fall in ABC leaves borrowing power unchanged; an oracle that repriced would admit %.1f%% of it",
		100*float64(repriced)/float64(afterCrash))
}
