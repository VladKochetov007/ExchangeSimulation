package exchange

import (
	"errors"
	"math"
	"testing"
	"time"

	einstrument "exchange_sim/instrument"
)

func newBookPriceExchange(t *testing.T) *DefaultExchange {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	return ex
}

func addBookPriceQuote(t *testing.T, ex *DefaultExchange, side Side, price int64) {
	t.Helper()
	book := ex.Books["ABC/USD"]
	orders := book.Bids
	if side == Sell {
		orders = book.Asks
	}
	order := &Order{ID: uint64(len(orders.Orders) + 1), Side: side, Price: price, Qty: 1, Visibility: Normal}
	if !orders.AddOrder(order) {
		t.Fatalf("add %s quote %d", side, price)
	}
}

func TestBookMidPriceLockedArithmeticAndAbsence(t *testing.T) {
	tests := []struct {
		name    string
		bid     int64
		ask     int64
		want    int64
		wantErr bool
	}{
		{name: "missing book", wantErr: true},
		{name: "empty book", wantErr: true},
		{name: "bid only", bid: 100, wantErr: true},
		{name: "ask only", ask: 101, wantErr: true},
		{name: "ordinary positive", bid: 100, ask: 104, want: 102},
		{name: "odd spread floors", bid: 100, ask: 101, want: 100},
		{name: "equal prices", bid: 101, ask: 101, want: 101},
		{name: "large int64 prices", bid: math.MaxInt64 - 2, ask: math.MaxInt64, want: math.MaxInt64 - 1},
		{name: "crossed book is unusable", bid: 101, ask: 100, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ex := NewExchange(4, &RealClock{})
			defer ex.Shutdown()
			if test.name != "missing book" {
				ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
				if test.bid != 0 {
					addBookPriceQuote(t, ex, Buy, test.bid)
				}
				if test.ask != 0 {
					addBookPriceQuote(t, ex, Sell, test.ask)
				}
			}

			got, err := ex.bookMidPrice("ABC/USD")
			if test.wantErr {
				if !errors.Is(err, ErrNoBookPrice) {
					t.Fatalf("bookMidPrice error = %v, want ErrNoBookPrice", err)
				}
				if got != 0 {
					t.Fatalf("bookMidPrice absent value = %d, want 0 only with error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("bookMidPrice = (%d, %v), want (%d, nil)", got, err, test.want)
			}
		})
	}
}

func TestBookMidPriceArithmeticUsesAdmittedPriceDomain(t *testing.T) {
	inst := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	for _, price := range []int64{1, math.MaxInt64 - 2, math.MaxInt64} {
		if !inst.ValidatePrice(price) {
			t.Fatalf("positive tick-aligned price %d was not admitted", price)
		}
	}
	for _, price := range []int64{0, -1} {
		if inst.ValidatePrice(price) {
			t.Fatalf("non-positive price %d was admitted", price)
		}
	}
}

func TestBookReferencePriceMakesOneSidedPolicyExplicit(t *testing.T) {
	tests := []struct {
		name    string
		bid     int64
		ask     int64
		want    int64
		wantErr bool
	}{
		{name: "empty", wantErr: true},
		{name: "bid reference", bid: 100, want: 100},
		{name: "ask reference", ask: 101, want: 101},
		{name: "two-sided midpoint", bid: 100, ask: 101, want: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ex := newBookPriceExchange(t)
			defer ex.Shutdown()
			if test.bid != 0 {
				addBookPriceQuote(t, ex, Buy, test.bid)
			}
			if test.ask != 0 {
				addBookPriceQuote(t, ex, Sell, test.ask)
			}
			got, err := ex.bookReferencePrice("ABC/USD")
			if test.wantErr {
				if !errors.Is(err, ErrNoBookPrice) {
					t.Fatalf("bookReferencePrice error = %v, want ErrNoBookPrice", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("bookReferencePrice = (%d, %v), want (%d, nil)", got, err, test.want)
			}
		})
	}
}

func TestCheckListingsDefersOptionChainWithoutTrueMidpoint(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	ex.ConfigureAutomation(AutomationConfig{ListingPolicies: []ListingPolicy{&einstrument.OptionChainLister{
		Underlying: "ABC/USD",
		Spec: einstrument.ContractSpec{
			Base: "ABC", Quote: "USD", BasePrecision: 1, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1,
		},
		TenorsNano: []int64{int64(time.Hour)}, StrikeStep: 10, StrikesPerSide: 0,
	}}})

	addBookPriceQuote(t, ex, Buy, 100)
	ex.CheckListings()
	if got := len(ex.ListInstruments("", "")); got != 1 {
		t.Fatalf("one-sided book listed option chain: instruments=%d", got)
	}

	addBookPriceQuote(t, ex, Sell, 102)
	ex.CheckListings()
	if got := len(ex.ListInstruments("", "")); got != 3 {
		t.Fatalf("two-sided book did not release one-strike option chain: instruments=%d", got)
	}
}

func TestDerivativeMarksDeferEmptyBookAndUseDeclaredReferencePolicy(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, time.Now().Add(time.Hour).UnixNano())
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)

	ex.UpdateDerivativeMarks()
	if got := future.SettlementPrice(); got != 0 {
		t.Fatalf("empty underlying produced settlement sample %d", got)
	}

	addBookPriceQuote(t, ex, Buy, 100)
	ex.UpdateDerivativeMarks()
	if got := future.SettlementPrice(); got != 100 {
		t.Fatalf("one-sided declared reference was not observed: got %d, want 100", got)
	}
}

func TestIndexPriceLockedPropagatesMissingUnderlying(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, time.Now().Add(time.Hour).UnixNano())
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)

	ex.mu.RLock()
	price, err := ex.indexPriceLocked(future.Symbol())
	ex.mu.RUnlock()
	if price != 0 || !errors.Is(err, ErrNoBookPrice) {
		t.Fatalf("missing underlying index = (%d, %v), want ErrNoBookPrice", price, err)
	}
}

func TestUpdatePerpPricesDefersUnavailableIndexThenUsesDeclaredReference(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, time.Now().Add(time.Hour).UnixNano())
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)
	ex.ConfigureAutomation(AutomationConfig{})

	ex.UpdatePerpPrices()
	if got := future.GetFundingRate().MarkPrice; got != 0 {
		t.Fatalf("unavailable underlying produced mark %d", got)
	}

	addBookPriceQuote(t, ex, Buy, 100)
	ex.UpdatePerpPrices()
	funding := future.GetFundingRate()
	if funding.IndexPrice != 100 || funding.MarkPrice != 100 {
		t.Fatalf("one-sided declared reference update = index %d mark %d, want 100/100", funding.IndexPrice, funding.MarkPrice)
	}
}
