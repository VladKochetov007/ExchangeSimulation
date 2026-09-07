package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-022. Collateral interest is charged per client per asset as
// TryMulDiv(borrowed, CollateralRate, collateralInterestDenominator), and the
// result is skipped when it rounds to zero:
//
//	if interest <= 0 { continue }
//
// With the denominator 365*24*3600*10000/60 = 5_256_000_000 and the default 500
// bps, that is interest = borrowed / 10_512_000 per simulated minute, so a debt
// below 10_512_000 quote units — 105.12 USD at USD_PRECISION — is charged
// nothing. The charge runs every minute and the shortfall is not carried
// forward, so the exemption repeats forever.
//
// That makes this a different kind of defect from RT-008 and RT-013. Those move
// at most one quote unit per item. This one forgives the whole charge below a
// threshold, so a borrower who splits a loan into sub-threshold pieces pays
// nothing rather than slightly less.
func TestAuditCollateralInterestIsForgivenBelowAThreshold(t *testing.T) {
	const minutes = 1440 // one simulated day

	// accrue charges `minutes` minutes of interest against `accounts` accounts
	// each holding `perAccount` of debt, and returns the total collected.
	accrue := func(t *testing.T, accounts int, perAccount int64) int64 {
		t.Helper()
		ex := NewExchange(4, &RealClock{})
		// The default 500 bps lives in ConfigureAutomation, not in the
		// constructor, and the campaign reaches it the same way.
		ex.ConfigureAutomation(AutomationConfig{})
		if ex.CollateralRate != 500 {
			t.Fatalf("collateral rate is %d, want the documented default 500", ex.CollateralRate)
		}
		for index := 0; index < accounts; index++ {
			id := uint64(index + 1)
			ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
			ex.Clients[id].Balances["USD"] = perAccount
			ex.Clients[id].Borrowed["USD"] = perAccount
		}
		for minute := 0; minute < minutes; minute++ {
			ex.ChargeCollateralInterest()
		}
		return ex.ExchangeBalance.FeeRevenue["USD"]
	}

	const principal = 1_000 * USD_PRECISION // 1000 USD

	whole := accrue(t, 1, principal)
	split := accrue(t, 10, principal/10) // ten 100 USD loans, each below 105.12

	t.Logf("over %d minutes on %d USD of debt: one account pays %d, ten accounts pay %d",
		minutes, principal/USD_PRECISION, whole, split)
	t.Logf("annualised: one account %.3f%%, ten accounts %.3f%%, against the configured 5.000%%",
		100*float64(whole)*365/float64(principal), 100*float64(split)*365/float64(principal))

	if whole <= 0 {
		t.Fatalf("the undivided loan accrued nothing, so this fixture cannot discriminate")
	}
	if split != 0 {
		t.Errorf("the split arm accrued %d; this test's premise is that sub-threshold debt is charged nothing", split)
	}
	if split >= whole {
		t.Errorf("splitting did not reduce interest: %d then %d", whole, split)
	}
}

// The generalisation that does not depend on an actor being able to open
// several accounts: the delivered interest rate is a function of the size of the
// debt. Below the threshold it is zero; above it, per-minute truncation keeps it
// short of the configured rate by an amount that shrinks as the principal grows.
// A borrower's cost of leverage therefore depends on how much it borrows, in a
// way the configuration does not state.
func TestAuditDeliveredInterestRateDependsOnPrincipal(t *testing.T) {
	const minutes = 1440

	deliveredBps := func(t *testing.T, principal int64) float64 {
		t.Helper()
		ex := NewExchange(4, &RealClock{})
		ex.ConfigureAutomation(AutomationConfig{})
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.Clients[1].Balances["USD"] = principal
		ex.Clients[1].Borrowed["USD"] = principal
		for minute := 0; minute < minutes; minute++ {
			ex.ChargeCollateralInterest()
		}
		charged := ex.ExchangeBalance.FeeRevenue["USD"]
		return 10000 * float64(charged) * 365 / float64(principal)
	}

	previous := -1.0
	for _, usd := range []int64{50, 100, 105, 200, 1_000, 10_000, 100_000, 1_000_000} {
		got := deliveredBps(t, usd*USD_PRECISION)
		t.Logf("%9d USD borrowed -> delivered %.1f bps of the configured 500", usd, got)
		if got > 500.5 {
			t.Errorf("%d USD was charged %.1f bps, above the configured 500", usd, got)
		}
		if got < previous {
			t.Errorf("delivered rate fell from %.1f to %.1f bps as the principal grew: the shortfall should shrink, not grow",
				previous, got)
		}
		previous = got
	}
}

// The boundary itself, stated as a number rather than derived in a comment.
func TestAuditCollateralInterestThreshold(t *testing.T) {
	charged := func(t *testing.T, debt int64) int64 {
		t.Helper()
		ex := NewExchange(4, &RealClock{})
		ex.ConfigureAutomation(AutomationConfig{})
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.Clients[1].Balances["USD"] = debt
		ex.Clients[1].Borrowed["USD"] = debt
		ex.ChargeCollateralInterest()
		return ex.ExchangeBalance.FeeRevenue["USD"]
	}

	// Bisect for the largest debt that is charged nothing in one minute.
	low, high := int64(0), int64(1_000*USD_PRECISION)
	if charged(t, high) == 0 {
		t.Fatalf("even %d units is charged nothing; the search range is wrong", high)
	}
	for low+1 < high {
		mid := low + (high-low)/2
		if charged(t, mid) == 0 {
			low = mid
		} else {
			high = mid
		}
	}
	t.Logf("largest interest-free debt is %d quote units (%.2f USD); the first charged debt is %d (%.2f USD)",
		low, float64(low)/USD_PRECISION, high, float64(high)/USD_PRECISION)

	if got, want := low, int64(10_512_000-1); got != want {
		t.Errorf("interest-free ceiling is %d, want the derived %d", got, want)
	}
}
