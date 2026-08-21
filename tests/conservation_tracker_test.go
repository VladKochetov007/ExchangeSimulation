package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// A balance changed without a logged movement is the one error an audit of the
// log can never find: the log stays self-consistent and is merely incomplete.
// The tracker exists to catch exactly that, by comparing what was recorded
// against what is actually held.
func TestConservationTrackerCatchesAnUnloggedMutation(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000))
	fee := &PercentageFee{MakerBps: 1, TakerBps: 2, InQuote: true}
	ex.ConnectNewClient(1, map[string]int64{"USD": 10_000_000 * USD_PRECISION}, fee)
	ex.ConnectNewClient(2, map[string]int64{"ABC": 100 * BTC_PRECISION}, fee)

	price := int64(50_000) * USD_PRECISION
	if _, reason := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, BTC_PRECISION); reason != "" {
		t.Fatalf("sell rejected: %v", reason)
	}
	if _, reason := InjectLimitOrder(ex, 1, "ABC/USD", Buy, price, BTC_PRECISION); reason != "" {
		t.Fatalf("buy rejected: %v", reason)
	}
	time.Sleep(50 * time.Millisecond)

	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("a correctly settled trade reported violations: %+v", violations)
	}

	// Now move money without recording it, which is what a bug in a settlement
	// path looks like from the outside.
	client := ex.Clients[1]
	client.Balances["USD"] += 12_345

	violations := ex.VerifyConservation()
	if len(violations) != 1 {
		t.Fatalf("violations = %+v, want exactly one for USD", violations)
	}
	if violations[0].Asset != "USD" || violations[0].Gap != 12_345 {
		t.Errorf("violation = %+v, want USD short by 12,345", violations[0])
	}
}

// Borrowing credits cash and creates a liability of the same size. Counting
// both as holdings would double the borrowed amount and make every borrow look
// like value creation.
func TestConservationTrackerCountsDebtAsDebt(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/1000))
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000_000 * USD_PRECISION}, &FixedFee{})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled: true, AutoBorrowSpot: true, DefaultMarginMode: CrossMargin,
		CollateralFactors: map[string]float64{"USD": 1},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION, "ABC": BTC_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION, "ABC": 50_000 * USD_PRECISION}),
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("an idle exchange reported violations: %+v", violations)
	}
}
