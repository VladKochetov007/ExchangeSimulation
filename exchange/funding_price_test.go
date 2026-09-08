package exchange

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestFundingScheduleAnchorsAtAutomationStart(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	factory := &recordingTickerFactory{}
	ex := NewExchangeWithConfig(ExchangeConfig{
		Clock:               clock,
		TickerFactory:       factory,
		DeterministicPhases: true,
	})
	defer ex.Shutdown()

	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	perp.GetFundingRate().Interval = 2
	ex.AddInstrument(perp)
	ex.StartAutomation(context.Background())
	ex.StopAutomation()

	funding := perp.GetFundingRate()
	wantFirst := clock.now + int64(2*time.Second)
	if funding.NextFunding != wantFirst {
		t.Fatalf("funding first deadline = %d, want automation-start deadline %d", funding.NextFunding, wantFirst)
	}

	clock.Advance(time.Second)
	ex.CheckAndSettleFunding()
	if funding.NextFunding != wantFirst {
		t.Fatalf("funding deadline moved before its declared interval: got %d want %d", funding.NextFunding, wantFirst)
	}

	funding.MarkPrice = 100
	funding.MarkAvailable = true
	clock.Advance(time.Second)
	ex.CheckAndSettleFunding()
	wantSecond := clock.now + int64(2*time.Second)
	if funding.NextFunding != wantSecond {
		t.Fatalf("funding second deadline = %d, want %d", funding.NextFunding, wantSecond)
	}
}

func TestPerpetualListedAfterAutomationStartsGetsFreshFundingInterval(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	factory := &recordingTickerFactory{}
	ex := NewExchangeWithConfig(ExchangeConfig{
		Clock:               clock,
		TickerFactory:       factory,
		DeterministicPhases: true,
	})
	defer ex.Shutdown()
	ex.StartAutomation(context.Background())
	defer ex.StopAutomation()

	clock.Advance(5 * time.Second)
	perp := NewPerpFutures("LATE-PERP", "LATE", "USD", 1, 1, 1, 1)
	perp.GetFundingRate().Interval = 2
	ex.AddInstrument(perp)

	want := clock.now + int64(2*time.Second)
	if got := perp.GetFundingRate().NextFunding; got != want {
		t.Fatalf("late perpetual first deadline = %d, want listing-relative deadline %d", got, want)
	}
}

func TestDelayedFundingFailsClosedWithoutBoundMarkSnapshot(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	perp.GetFundingRate().Interval = 2
	ex.AddInstrument(perp)
	funding := perp.GetFundingRate()
	funding.MarkPrice = 100
	funding.MarkAvailable = true
	funding.NextFunding = clock.now + int64(2*time.Second)
	scheduledDeadline := funding.NextFunding
	log := &recordingLogger{}
	ex.SetLogger(perp.Symbol(), log)
	clock.Advance(5 * time.Second)
	ex.CheckAndSettleFunding()
	if funding.NextFunding != scheduledDeadline {
		t.Fatalf("late funding advanced deadline without a bound mark snapshot: got %d want %d", funding.NextFunding, scheduledDeadline)
	}
	failed := false
	for _, record := range log.records {
		if record.event != "funding_settlement_failed" {
			continue
		}
		payload, ok := record.data.(map[string]any)
		if ok && payload["reason"] == ErrFundingDeadlineLate.Error() {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("late funding did not emit a fail-closed marker: %#v", log.records)
	}
}

func TestManualFundingDoesNotReanchorFutureDeadline(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	funding := perp.GetFundingRate()
	funding.MarkPrice = 100
	funding.MarkAvailable = true
	funding.NextFunding = clock.now + int64(2*time.Second)
	before := funding.NextFunding

	if err := ex.SettleFunding(perp); !errors.Is(err, ErrFundingNotDue) {
		t.Fatalf("manual funding before deadline = %v, want ErrFundingNotDue", err)
	}
	if funding.NextFunding != before {
		t.Fatalf("manual funding re-anchored future deadline: got %d want %d", funding.NextFunding, before)
	}
}

func TestFundingMissingMarkDefersWithoutEntryPriceFallback(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"USD": 1_000}, &FixedFee{})
	ex.Positions.UpdatePosition(1, perp.Symbol(), 10, 90, Buy, PositionBoth)
	ex.Positions.UpdatePosition(2, perp.Symbol(), 10, 110, Sell, PositionBoth)

	funding := perp.GetFundingRate()
	funding.Rate = 100
	funding.NextFunding = clock.NowUnixNano()
	funding.MarkPrice = 0
	funding.MarkAvailable = false
	beforeOne := ex.Clients[1].PerpBalance("USD")
	beforeTwo := ex.Clients[2].PerpBalance("USD")
	log := &recordingLogger{}
	ex.SetLogger(perp.Symbol(), log)

	// The periodic path keeps its due timestamp and records a visible deferral;
	// it neither performs entry-price funding nor pretends the interval settled.
	ex.CheckAndSettleFunding()
	if funding.NextFunding != clock.NowUnixNano() {
		t.Fatalf("periodic unavailable mark advanced funding schedule: got %d want %d", funding.NextFunding, clock.NowUnixNano())
	}
	deferred := false
	for _, record := range log.records {
		event, ok := record.data.(PriceUnavailableEvent)
		if record.event == "price_unavailable" && ok && event.Operation == "funding_settlement" {
			deferred = true
		}
	}
	if !deferred {
		t.Fatalf("periodic unavailable funding was not observable: %#v", log.records)
	}

	if err := ex.SettleFunding(perp); !errors.Is(err, ErrNoBookPrice) {
		t.Fatalf("manual funding without mark = %v, want ErrNoBookPrice", err)
	}
	if got := ex.Clients[1].PerpBalance("USD"); got != beforeOne {
		t.Fatalf("long balance changed on unavailable mark: got %d want %d", got, beforeOne)
	}
	if got := ex.Clients[2].PerpBalance("USD"); got != beforeTwo {
		t.Fatalf("short balance changed on unavailable mark: got %d want %d", got, beforeTwo)
	}
	if funding.NextFunding != clock.NowUnixNano() {
		t.Fatalf("unavailable mark advanced funding schedule: got %d want %d", funding.NextFunding, clock.NowUnixNano())
	}

	funding.MarkPrice = 100
	funding.MarkAvailable = true
	if err := ex.SettleFunding(perp); err != nil {
		t.Fatalf("manual funding after mark recovery: %v", err)
	}
	if funding.NextFunding <= clock.NowUnixNano() {
		t.Fatalf("successful funding did not advance schedule: %d", funding.NextFunding)
	}
}

func TestFundingArithmeticOverflowLeavesStateUntouched(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{"USD": math.MaxInt64}, &FixedFee{})
	ex.Positions.UpdatePosition(1, perp.Symbol(), math.MaxInt64, 1, Buy, PositionBoth)

	funding := perp.GetFundingRate()
	funding.MarkPrice = math.MaxInt64
	funding.MarkAvailable = true
	funding.Rate = 1
	funding.NextFunding = clock.NowUnixNano()
	beforeBalance := ex.Clients[1].PerpBalance("USD")
	beforeNextFunding := funding.NextFunding

	err := ex.SettleFunding(perp)
	if !errors.Is(err, ErrFundingArithmetic) {
		t.Fatalf("overflow funding error = %v, want ErrFundingArithmetic", err)
	}
	if got := ex.Clients[1].PerpBalance("USD"); got != beforeBalance {
		t.Fatalf("overflow funding changed balance: got %d want %d", got, beforeBalance)
	}
	if funding.NextFunding != beforeNextFunding {
		t.Fatalf("overflow funding advanced schedule: got %d want %d", funding.NextFunding, beforeNextFunding)
	}
}

func TestFundingHedgeModeAccumulatesPaymentsPerClient(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000}, &FixedFee{})
	ex.Clients[1].PerpBalances["USD"] = 1_000
	ex.Positions.UpdatePosition(1, perp.Symbol(), 10, 100, Buy, PositionLong)
	ex.Positions.UpdatePosition(1, perp.Symbol(), 4, 100, Sell, PositionShort)

	funding := perp.GetFundingRate()
	funding.MarkPrice = 100
	funding.MarkAvailable = true
	funding.Rate = 1_000
	funding.NextFunding = clock.NowUnixNano()

	if err := ex.SettleFunding(perp); err != nil {
		t.Fatalf("hedge-mode funding: %v", err)
	}
	// Long pays 100 and short receives 40. Both positions belong to the same
	// client, so the final balance must include both legs rather than letting
	// the second preflight entry overwrite the first.
	if got, want := ex.Clients[1].PerpBalance("USD"), int64(940); got != want {
		t.Fatalf("hedge-mode balance = %d, want %d", got, want)
	}
	if got, want := ex.ExchangeBalance.FeeRevenue["USD"], int64(60); got != want {
		t.Fatalf("hedge-mode venue revenue = %d, want %d", got, want)
	}
}

func TestFundingVenueOverflowLeavesStateUntouched(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000}, &FixedFee{})
	ex.Positions.UpdatePosition(1, perp.Symbol(), 1, 100, Buy, PositionBoth)
	ex.ExchangeBalance.FeeRevenue["USD"] = math.MaxInt64

	funding := perp.GetFundingRate()
	funding.MarkPrice = 100
	funding.MarkAvailable = true
	funding.Rate = 1_000
	funding.NextFunding = clock.NowUnixNano()
	beforeBalance := ex.Clients[1].PerpBalance("USD")
	beforeRevenue := ex.ExchangeBalance.FeeRevenue["USD"]
	beforeNextFunding := funding.NextFunding

	err := ex.SettleFunding(perp)
	if !errors.Is(err, ErrFundingArithmetic) {
		t.Fatalf("venue overflow error = %v, want ErrFundingArithmetic", err)
	}
	if got := ex.Clients[1].PerpBalance("USD"); got != beforeBalance {
		t.Fatalf("venue overflow changed client balance: got %d want %d", got, beforeBalance)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != beforeRevenue {
		t.Fatalf("venue overflow changed venue revenue: got %d want %d", got, beforeRevenue)
	}
	if funding.NextFunding != beforeNextFunding {
		t.Fatalf("venue overflow advanced schedule: got %d want %d", funding.NextFunding, beforeNextFunding)
	}
}

func TestFundingClientDeltaOverflowLeavesStateUntouched(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000}, &FixedFee{})
	ex.Positions.UpdatePosition(1, perp.Symbol(), 1, 10_000, Buy, PositionBoth)

	funding := perp.GetFundingRate()
	funding.MarkPrice = 10_000
	funding.MarkAvailable = true
	// position value / precision is 10,000. With this rate the exact
	// payment is MinInt64: the venue residual can represent it, but debiting
	// the client's zero delta cannot. That client-side failure must not be
	// hidden by the independently successful venue-flow arithmetic.
	funding.Rate = math.MinInt64
	funding.NextFunding = clock.NowUnixNano()
	beforeBalance := ex.Clients[1].PerpBalance("USD")
	beforeRevenue := ex.ExchangeBalance.FeeRevenue["USD"]
	beforeNextFunding := funding.NextFunding

	err := ex.SettleFunding(perp)
	if !errors.Is(err, ErrFundingArithmetic) {
		t.Fatalf("client delta overflow error = %v, want ErrFundingArithmetic", err)
	}
	if got := ex.Clients[1].PerpBalance("USD"); got != beforeBalance {
		t.Fatalf("client delta overflow changed balance: got %d want %d", got, beforeBalance)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != beforeRevenue {
		t.Fatalf("client delta overflow changed venue revenue: got %d want %d", got, beforeRevenue)
	}
	if funding.NextFunding != beforeNextFunding {
		t.Fatalf("client delta overflow advanced schedule: got %d want %d", funding.NextFunding, beforeNextFunding)
	}
}

func TestCollateralInterestArithmeticOverflowLeavesStateUntouched(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000}, &FixedFee{})
	client := ex.Clients[1]
	client.Borrowed["USD"] = math.MaxInt64
	client.PerpBalances["USD"] = 123
	ex.CollateralRate = math.MaxInt64
	ex.ExchangeBalance.FeeRevenue["USD"] = 456

	ex.ChargeCollateralInterest()
	if got := client.Borrowed["USD"]; got != math.MaxInt64 {
		t.Fatalf("arithmetic-overflow interest changed borrowed amount: %d", got)
	}
	if got := client.PerpBalances["USD"]; got != 123 {
		t.Fatalf("arithmetic-overflow interest changed perp balance: %d", got)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != 456 {
		t.Fatalf("arithmetic-overflow interest changed venue revenue: %d", got)
	}
}

func TestCollateralInterestVenueOverflowLeavesStateUntouched(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000}, &FixedFee{})
	client := ex.Clients[1]
	client.Borrowed["USD"] = 5_256_000_000
	client.PerpBalances["USD"] = 1_000
	ex.CollateralRate = 1_000
	ex.ExchangeBalance.FeeRevenue["USD"] = math.MaxInt64
	log := &recordingLogger{}
	ex.SetLogger("_global", log)

	ex.ChargeCollateralInterest()
	if got := client.PerpBalances["USD"]; got != 1_000 {
		t.Fatalf("venue-overflow interest changed perp balance: %d", got)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != math.MaxInt64 {
		t.Fatalf("venue-overflow interest changed venue revenue: %d", got)
	}
	failed := false
	for _, record := range log.records {
		if record.event == "margin_interest_failed" {
			failed = true
			break
		}
	}
	if !failed {
		t.Fatal("venue-overflow interest did not emit a failure event")
	}
}
