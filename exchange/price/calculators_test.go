package price

import (
	"testing"

	ebook "exchange_sim/exchange/book"
	etypes "exchange_sim/exchange/types"
)

func newBook(symbol string) *ebook.OrderBook {
	return &ebook.OrderBook{
		Symbol: symbol,
		Bids:   ebook.NewBook(etypes.Buy),
		Asks:   ebook.NewBook(etypes.Sell),
	}
}

func TestLastPriceCalculator(t *testing.T) {
	calc := NewLastPriceCalculator()
	ob := newBook("BTC/USD")
	ob.LastTrade = &etypes.Trade{TradeID: 1, Price: 50000, Qty: 100}
	if price := calc.Calculate(ob); price != 50000 {
		t.Errorf("want 50000, got %d", price)
	}
}

func TestLastPriceCalculatorNoTrade(t *testing.T) {
	calc := NewLastPriceCalculator()
	if price := calc.Calculate(newBook("BTC/USD")); price != 0 {
		t.Errorf("want 0, got %d", price)
	}
}

func TestMidPriceCalculator(t *testing.T) {
	calc := NewMidPriceCalculator()
	ob := newBook("BTC/USD")
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49900, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 50100, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	if price := calc.Calculate(ob); price != (49900+50100)/2 {
		t.Errorf("want %d, got %d", (49900+50100)/2, price)
	}
}

func TestMidPriceCalculatorEmptyBook(t *testing.T) {
	calc := NewMidPriceCalculator()
	ob := newBook("BTC/USD")
	ob.LastTrade = &etypes.Trade{Price: 50000}
	if price := calc.Calculate(ob); price != 50000 {
		t.Errorf("empty book must fall back to last price: want 50000, got %d", price)
	}
}

func TestWeightedMidPriceCalculator(t *testing.T) {
	calc := NewWeightedMidPriceCalculator()
	ob := newBook("BTC/USD")
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49900, Qty: 200, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 50100, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	// bid side has more qty → weighted mid pulls toward bid
	bidQty, askQty := int64(200), int64(100)
	expected := (int64(49900)*askQty + int64(50100)*bidQty) / (bidQty + askQty)
	if price := calc.Calculate(ob); price != expected {
		t.Errorf("want %d, got %d", expected, price)
	}
}

func TestOrderBookGetters(t *testing.T) {
	ob := newBook("BTC/USD")
	if ob.GetBestBid() != 0 {
		t.Error("empty bids: want 0")
	}
	if ob.GetBestAsk() != 0 {
		t.Error("empty asks: want 0")
	}
	if ob.GetLastPrice() != 0 {
		t.Error("no trades: want 0")
	}
	if ob.GetMidPrice() != 0 {
		t.Error("empty book: want 0")
	}

	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49900, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 50100, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})

	if got := ob.GetBestBid(); got != 49900 {
		t.Errorf("best bid: want 49900, got %d", got)
	}
	if got := ob.GetBestAsk(); got != 50100 {
		t.Errorf("best ask: want 50100, got %d", got)
	}
	if got := ob.GetMidPrice(); got != (49900+50100)/2 {
		t.Errorf("mid price: want %d, got %d", (49900+50100)/2, got)
	}

	ob.LastTrade = &etypes.Trade{Price: 50000}
	if got := ob.GetLastPrice(); got != 50000 {
		t.Errorf("last price: want 50000, got %d", got)
	}
}
