package price

import (
	"testing"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

type mockBookProvider struct {
	books map[string]*ebook.OrderBook
}

func (m *mockBookProvider) GetBook(symbol string) *ebook.OrderBook { return m.books[symbol] }

func TestMidPriceOracle_ReturnsZeroForUnmapped(t *testing.T) {
	o := NewMidPriceOracle(&mockBookProvider{books: map[string]*ebook.OrderBook{}})
	if p := o.GetPrice("BTC"); p != 0 {
		t.Errorf("unmapped symbol: want 0, got %d", p)
	}
}

func TestMidPriceOracle_ReturnsZeroForEmptyBook(t *testing.T) {
	ob := &ebook.OrderBook{Bids: ebook.NewBook(etypes.Buy), Asks: ebook.NewBook(etypes.Sell)}
	o := NewMidPriceOracle(&mockBookProvider{books: map[string]*ebook.OrderBook{"BTC/USD": ob}})
	o.MapSymbol("BTC", "BTC/USD")
	if p := o.GetPrice("BTC"); p != 0 {
		t.Errorf("empty book: want 0, got %d", p)
	}
}

func TestMidPriceOracle_ReturnsMidPrice(t *testing.T) {
	ob := &ebook.OrderBook{Bids: ebook.NewBook(etypes.Buy), Asks: ebook.NewBook(etypes.Sell)}
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49000, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 51000, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	o := NewMidPriceOracle(&mockBookProvider{books: map[string]*ebook.OrderBook{"BTC/USD": ob}})
	o.MapSymbol("BTC", "BTC/USD")
	if mid := o.GetPrice("BTC"); mid != (49000+51000)/2 {
		t.Errorf("mid price: want %d, got %d", (49000+51000)/2, mid)
	}
}
