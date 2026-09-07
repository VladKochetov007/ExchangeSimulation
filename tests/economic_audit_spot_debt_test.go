package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// H-026. The exchange has exactly two liquidation entry points, CheckLiquidations
// for perp and dated futures and CheckPositionMarginerLiquidations for options.
// Both walk positions. Spot debt lives in Client.Borrowed and BorrowedSpot and is
// not a position, so neither can see it.
//
// With AutoBorrowSpot enabled — which the campaign sets — a participant short of
// an asset at settlement has it borrowed for them. That is a venue-financed short
// spot position. This test asks what happens when it goes wrong.
func TestAuditBorrowedSpotExposureIsNeverUnwound(t *testing.T) {
	const bootstrap = 50_000 * USD_PRECISION
	const borrowed = 1 * BTC_PRECISION // 1 ABC borrowed and sold

	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	if err := perp.UpdateFundingRate(bootstrap, bootstrap); err != nil {
		t.Fatalf("mark: %v", err)
	}
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		DefaultMarginMode: CrossMargin,
		CollateralFactors: map[string]float64{"USD": 1},
		MaxBorrowPerAsset: map[string]int64{"ABC": 1_000 * BTC_PRECISION},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "ABC": BTC_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "ABC": bootstrap}),
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	ex.ConnectNewClient(1, map[string]int64{"USD": 100_000 * USD_PRECISION}, &FixedFee{})
	ex.LiquidationHandler = &fundEventRecorder{}

	if err := ex.BorrowMargin(1, "ABC", borrowed, "audit_short"); err != nil {
		t.Fatalf("borrow: %v", err)
	}
	// The borrowed ABC is sold: the account is now short 1 ABC against cash.
	ex.Clients[1].Balances["ABC"] -= borrowed
	ex.Clients[1].Balances["USD"] += bootstrap

	marked := func(t *testing.T, abcPrice int64) etypes.MarkedAccountSnapshot {
		t.Helper()
		snapshot, err := ex.MarkedAccount(1, etypes.AccountValuationSpec{
			ReportAsset: "USD", ReportPrecision: USD_PRECISION,
			AssetMarks: map[string]etypes.AssetValuationMark{
				"USD": {Price: USD_PRECISION, Precision: USD_PRECISION},
				"ABC": {Price: abcPrice, Precision: BTC_PRECISION},
			},
		})
		if err != nil {
			t.Fatalf("marked account: %v", err)
		}
		return snapshot
	}

	atEntry := marked(t, bootstrap)
	t.Logf("short 1 ABC financed by the venue, ABC at 50 000: equity %.0f USD",
		float64(atEntry.Equity)/USD_PRECISION)

	// ABC quadruples. The debt is now worth far more than the proceeds.
	const spike = 200_000 * USD_PRECISION
	if err := perp.UpdateFundingRate(spike, spike); err != nil {
		t.Fatalf("spike mark: %v", err)
	}
	afterSpike := marked(t, spike)
	t.Logf("after ABC quadruples to 200 000: equity %.0f USD",
		float64(afterSpike.Equity)/USD_PRECISION)

	if afterSpike.Equity >= 0 {
		t.Fatalf("the account is not underwater, so this fixture cannot test what happens when it is: equity %d",
			afterSpike.Equity)
	}

	debtBefore := ex.Clients[1].Borrowed["ABC"]
	usdBefore := ex.Clients[1].Balances["USD"]
	fundBefore := ex.ExchangeBalance.InsuranceFund["USD"]

	// Both liquidation entry points, invoked against an account with negative
	// net worth.
	ex.CheckLiquidations("ABC-PERP", perp, spike)
	ex.CheckPositionMarginerLiquidations()

	t.Logf("after invoking both liquidation entry points: debt %d (was %d), USD %d (was %d), fund %d (was %d)",
		ex.Clients[1].Borrowed["ABC"], debtBefore,
		ex.Clients[1].Balances["USD"], usdBefore,
		ex.ExchangeBalance.InsuranceFund["USD"], fundBefore)

	if ex.Clients[1].Borrowed["ABC"] != debtBefore {
		t.Errorf("something now unwinds borrowed spot debt: %d then %d; the finding this test documents has changed",
			debtBefore, ex.Clients[1].Borrowed["ABC"])
	}
	if ex.Clients[1].Balances["USD"] != usdBefore {
		t.Errorf("the account's cash moved: %d then %d", usdBefore, ex.Clients[1].Balances["USD"])
	}
	if ex.ExchangeBalance.InsuranceFund["USD"] != fundBefore {
		t.Errorf("the insurance fund absorbed something: %d then %d", fundBefore, ex.ExchangeBalance.InsuranceFund["USD"])
	}
	t.Logf("the account is %.0f USD underwater and no path closes it; the loss sits with the lender unmarked",
		-float64(afterSpike.Equity)/USD_PRECISION)
}
