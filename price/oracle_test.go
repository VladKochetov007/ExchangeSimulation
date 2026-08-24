package price

import (
	"errors"
	"testing"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

type mockBookProvider struct {
	books map[string]*ebook.OrderBook
}

func TestStaticPriceOracleSeparatesAvailabilityFromSignedValues(t *testing.T) {
	oracle := NewStaticPriceOracle(map[string]int64{
		"NEG":  -20,
		"ZERO": 0,
		"POS":  20,
	})
	for symbol, want := range map[string]int64{"NEG": -20, "ZERO": 0, "POS": 20} {
		got, err := oracle.Price(symbol)
		if err != nil || got != want {
			t.Fatalf("static %s = (%d, %v), want (%d, nil)", symbol, got, err, want)
		}
	}
	if got, err := oracle.Price("MISSING"); got != 0 || !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("missing static price = (%d, %v), want ErrNoPrice", got, err)
	}
}

func TestPositiveSourcePolicyRejectsPresentNonPositiveValuesByDomain(t *testing.T) {
	for _, value := range []int64{-1, 0} {
		source := NewStaticPriceOracle(map[string]int64{"X": value})
		if got, err := sourcePrice(source, "X"); err != nil || got != value {
			t.Fatalf("generic source(%d) = (%d, %v), want present numeric value", value, got, err)
		}
		if got, err := positiveSourcePrice(source, "X"); got != 0 || !errors.Is(err, etypes.ErrPriceDomain) || errors.Is(err, etypes.ErrNoPrice) {
			t.Fatalf("positive source(%d) = (%d, %v), want domain error only", value, got, err)
		}
	}
}

func (m *mockBookProvider) GetBook(symbol string) *ebook.OrderBook { return m.books[symbol] }

func TestMidPriceOracle_ReturnsUnavailableForUnmapped(t *testing.T) {
	o := NewMidPriceOracle(&mockBookProvider{books: map[string]*ebook.OrderBook{}})
	if _, err := o.Price("BTC"); err == nil {
		t.Error("unmapped symbol returned a price")
	}
}

func TestMidPriceOracle_ReturnsUnavailableForEmptyBook(t *testing.T) {
	ob := &ebook.OrderBook{Bids: ebook.NewBook(etypes.Buy), Asks: ebook.NewBook(etypes.Sell)}
	o := NewMidPriceOracle(&mockBookProvider{books: map[string]*ebook.OrderBook{"BTC/USD": ob}})
	o.MapSymbol("BTC", "BTC/USD")
	if _, err := o.Price("BTC"); err == nil {
		t.Error("empty book returned a price")
	}
}

func TestMidPriceOracle_ReturnsMidPrice(t *testing.T) {
	ob := &ebook.OrderBook{Bids: ebook.NewBook(etypes.Buy), Asks: ebook.NewBook(etypes.Sell)}
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49000, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 51000, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	o := NewMidPriceOracle(&mockBookProvider{books: map[string]*ebook.OrderBook{"BTC/USD": ob}})
	o.MapSymbol("BTC", "BTC/USD")
	if mid := mustPrice(t)(o.Price("BTC")); mid != (49000+51000)/2 {
		t.Errorf("mid price: want %d, got %d", (49000+51000)/2, mid)
	}
}
