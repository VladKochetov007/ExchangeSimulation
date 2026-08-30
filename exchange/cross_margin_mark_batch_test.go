package exchange

import (
	"errors"
	"testing"
	"time"

	etypes "exchange_sim/types"
)

func addTwoSidedQuote(t *testing.T, book *OrderBook, bidPrice, askPrice int64) {
	t.Helper()
	if !book.Bids.AddOrder(&Order{ID: 1, Side: Buy, Price: bidPrice, Qty: 1, Visibility: Normal}) {
		t.Fatalf("could not seed bid on %s", book.Symbol)
	}
	if !book.Asks.AddOrder(&Order{ID: 2, Side: Sell, Price: askPrice, Qty: 1, Visibility: Normal}) {
		t.Fatalf("could not seed ask on %s", book.Symbol)
	}
}

func TestUpdatePerpPricesRefreshesOptionAndDatedMarksBeforeRisk(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, clock.now+int64(365*24*time.Hour))
	future.Underlying = spot.Symbol()
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, clock.now+int64(365*24*time.Hour), true)
	for _, instrument := range []Instrument{spot, perp, future, option} {
		ex.AddInstrument(instrument)
	}
	ex.ConfigureAutomation(AutomationConfig{MarkPriceCalc: NewMidPriceCalculator()})
	addTwoSidedQuote(t, ex.Books[spot.Symbol()], 100, 120)
	addTwoSidedQuote(t, ex.Books[perp.Symbol()], 99, 101)
	addTwoSidedQuote(t, ex.Books[future.Symbol()], 100, 102)
	option.SetMarks(90, 7)

	ex.UpdatePerpPrices()

	underlying, err := option.UnderlyingMark()
	if err != nil || underlying != 110 {
		t.Fatalf("option underlying mark = %d, %v; want refreshed 110", underlying, err)
	}
	premium, err := option.MarkPremium()
	if err != nil || premium == 7 {
		t.Fatalf("option premium mark = %d, %v; stale option mark survived the perp risk phase", premium, err)
	}
	funding := future.GetFundingRate()
	if !funding.IndexAvailable || funding.IndexPrice != 110 || !funding.MarkAvailable || funding.MarkPrice != 101 {
		t.Fatalf("dated future mark set = %#v, want index 110 and book mark 101", funding)
	}
}

func TestUpdatePerpPricesClearsStaleOptionMarkWhenUnderlyingUnavailable(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, clock.now+int64(365*24*time.Hour), true)
	ex.AddInstrument(spot)
	ex.AddInstrument(option)
	ex.ConfigureAutomation(AutomationConfig{MarkPriceCalc: NewMidPriceCalculator()})
	addTwoSidedQuote(t, ex.Books[spot.Symbol()], 100, 120)
	option.SetMarks(90, 7)
	ex.UpdatePerpPrices()
	if _, err := option.MarkPremium(); err != nil {
		t.Fatalf("initial option mark unavailable: %v", err)
	}

	ex.Books[spot.Symbol()].Bids.CancelOrder(1)
	ex.Books[spot.Symbol()].Asks.CancelOrder(2)
	ex.UpdatePerpPrices()

	if premium, err := option.MarkPremium(); !errors.Is(err, etypes.ErrNoPrice) || premium != 0 {
		t.Fatalf("stale option premium = (%d, %v), want unavailable", premium, err)
	}
	if _, err := riskMark(option, ex.Books[option.Symbol()]); !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("stale option risk mark error = %v, want ErrNoPrice", err)
	}
}
