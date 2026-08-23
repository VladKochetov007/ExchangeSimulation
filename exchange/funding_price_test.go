package exchange

import (
	"errors"
	"testing"
)

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
