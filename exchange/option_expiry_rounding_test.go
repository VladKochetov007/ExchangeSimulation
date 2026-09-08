package exchange

import (
	"math"
	"testing"
	"time"
)

// Integer payout is per position, while the option book is one net contract.
// Unequal position slicing can therefore leave a one-quote-unit truncation;
// the venue ledger must close only that residual.
func TestOptionExpiryRoutesAggregateRoundingResidual(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	const precision = int64(100)
	option := NewEuropeanOption(
		"ABC-ROUND-C", "ABC", "USD", "ABC/USD",
		precision, precision, 1, 1, 50, time.Now().Add(-time.Second).UnixNano(), true,
	)
	option.DeliveryFeeBps = 0
	option.SetMarks(100, 0)
	option.ObserveSettlement(100, clock.NowUnixNano())
	ex.AddInstrument(option)

	for _, clientID := range []uint64{1, 2, 3} {
		ex.ConnectNewClient(clientID, nil, &FixedFee{})
		ex.AddPerpBalance(clientID, "USD", 10)
	}
	// Intrinsic=50 and precision=100: +1 and +1 truncate to zero, while -2
	// truncates to -1. The net position is zero, so the exact book cash flow
	// is zero and the -1 participant residual must be matched by venue +1.
	ex.Positions.UpdatePosition(1, option.Symbol(), 1, 50, Buy, PositionBoth)
	ex.Positions.UpdatePosition(2, option.Symbol(), 1, 50, Buy, PositionBoth)
	ex.Positions.UpdatePosition(3, option.Symbol(), 2, 50, Sell, PositionBoth)

	ex.CheckExpiries()

	if ex.Instruments[option.Symbol()] != nil {
		t.Fatal("option remained listed after expiry")
	}
	if got, want := ex.ExchangeBalance.FeeRevenue["USD"], int64(1); got != want {
		t.Fatalf("option expiry rounding ledger = %d, want %d", got, want)
	}
	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("option expiry rounding was not conserved: %+v", violations)
	}
}

func TestOptionExpiryDoesNotReclassifyUnmatchedNetPositionAsRounding(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()

	const precision = int64(100)
	option := NewEuropeanOption(
		"ABC-UNMATCHED-C", "ABC", "USD", "ABC/USD",
		precision, precision, 1, 1, 50, time.Now().Add(-time.Second).UnixNano(), true,
	)
	option.DeliveryFeeBps = 0
	option.SetMarks(100, 0)
	option.ObserveSettlement(100, clock.NowUnixNano())
	ex.AddInstrument(option)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 10)
	ex.Positions.UpdatePosition(1, option.Symbol(), 2, 50, Buy, PositionBoth)

	ex.CheckExpiries()

	if ex.Instruments[option.Symbol()] == nil || ex.Books[option.Symbol()] == nil {
		t.Fatal("unmatched option was delisted instead of entering settlement-pending")
	}
	pending, ok := ex.settlementPending[option.Symbol()]
	if !ok || pending.State != expiryStateSettlementPending || pending.Attempts != 1 || pending.LastReason == "" {
		t.Fatalf("unmatched option pending state = %#v", pending)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != 0 {
		t.Fatalf("unmatched option cash was hidden as rounding: venue balance %d", got)
	}
	if got, want := ex.Clients[1].PerpBalances["USD"], int64(10); got != want {
		t.Fatalf("unmatched option changed balance = %d, want %d", got, want)
	}
	if got := ex.Positions.GetPosition(1, option.Symbol()); got == nil || got.Size != 2 {
		t.Fatalf("unmatched option position = %#v, want retained size 2", got)
	}
}

func TestOptionExpiryOverflowDefersWithoutMutation(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()
	option := NewEuropeanOption(
		"ABC-OVERFLOW-C", "ABC", "USD", "ABC/USD",
		1, 1, 1, 1, 1, time.Now().Add(-time.Second).UnixNano(), true,
	)
	option.DeliveryFeeBps = 0
	option.ObserveSettlement(3, clock.NowUnixNano())
	ex.AddInstrument(option)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 10)
	ex.Positions.UpdatePosition(1, option.Symbol(), math.MaxInt64, 1, Buy, PositionBoth)

	ex.CheckExpiries()

	if ex.Instruments[option.Symbol()] == nil || ex.Books[option.Symbol()] == nil {
		t.Fatal("overflowing option was delisted instead of entering settlement-pending")
	}
	if got := ex.Clients[1].PerpBalances["USD"]; got != 10 {
		t.Fatalf("overflowing option changed balance = %d, want 10", got)
	}
	if got := ex.Positions.GetPosition(1, option.Symbol()); got == nil || got.Size != math.MaxInt64 {
		t.Fatalf("overflowing option position = %#v, want retained", got)
	}
}
