package price

import (
	"math"
	"testing"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

func newBook(symbol string) *ebook.OrderBook {
	return &ebook.OrderBook{
		Symbol: symbol,
		Bids:   ebook.NewBook(etypes.Buy),
		Asks:   ebook.NewBook(etypes.Sell),
	}
}

func mustPrice(t testing.TB) func(int64, error) int64 {
	return func(price int64, err error) int64 {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return price
	}
}

func TestLastPriceCalculator(t *testing.T) {
	calc := NewLastPriceCalculator()
	ob := newBook("BTC/USD")
	ob.LastTrade = &etypes.Trade{TradeID: 1, Price: 50000, Qty: 100}
	if price := mustPrice(t)(calc.Calculate(ob)); price != 50000 {
		t.Errorf("want 50000, got %d", price)
	}
}

func TestLastPriceCalculatorNoTrade(t *testing.T) {
	calc := NewLastPriceCalculator()
	if _, err := calc.Calculate(newBook("BTC/USD")); err == nil {
		t.Error("missing last trade did not return an unavailable-price error")
	}
}

func TestMidPriceCalculator(t *testing.T) {
	calc := NewMidPriceCalculator()
	ob := newBook("BTC/USD")
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49900, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 50100, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	if price := mustPrice(t)(calc.Calculate(ob)); price != (49900+50100)/2 {
		t.Errorf("want %d, got %d", (49900+50100)/2, price)
	}
}

func TestMidPriceCalculatorEmptyBook(t *testing.T) {
	calc := NewMidPriceCalculator()
	ob := newBook("BTC/USD")
	ob.LastTrade = &etypes.Trade{Price: 50000}
	if _, err := calc.Calculate(ob); err == nil {
		t.Error("empty book midpoint fell back to last price")
	}
}

func TestWeightedMidPriceCalculator(t *testing.T) {
	calc := NewWeightedMidPriceCalculator()
	ob := newBook("BTC/USD")
	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49900, Qty: 200, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 50100, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})
	// Bid-side depth weights the ask endpoint under the microprice convention.
	bidQty, askQty := int64(200), int64(100)
	expected := (int64(49900)*askQty + int64(50100)*bidQty) / (bidQty + askQty)
	if price := mustPrice(t)(calc.Calculate(ob)); price != expected {
		t.Errorf("want %d, got %d", expected, price)
	}
}

func TestWeightedPriceSignedRangeAndRounding(t *testing.T) {
	tests := []struct {
		name                                   string
		lower, upper, upperWeight, lowerWeight int64
		want                                   int64
	}{
		{name: "ordinary positive", lower: 100, upper: 104, upperWeight: 1, lowerWeight: 1, want: 102},
		{name: "negative truncates toward zero", lower: -20, upper: -10, upperWeight: 1, lowerWeight: 2, want: -16},
		{name: "crosses zero", lower: -1, upper: 0, upperWeight: 1, lowerWeight: 1, want: 0},
		{name: "full signed range", lower: math.MinInt64, upper: math.MaxInt64, upperWeight: 1, lowerWeight: 1, want: 0},
		{name: "full signed range weighted high", lower: math.MinInt64, upper: math.MaxInt64, upperWeight: 2, lowerWeight: 1, want: math.MaxInt64 / 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := weightedPrice(test.lower, test.upper, test.upperWeight, test.lowerWeight)
			if err != nil || got != test.want {
				t.Fatalf("weightedPrice(%d, %d, %d, %d) = (%d, %v), want (%d, nil)", test.lower, test.upper, test.upperWeight, test.lowerWeight, got, err, test.want)
			}
		})
	}
}

func TestOrderBookGetters(t *testing.T) {
	ob := newBook("BTC/USD")
	if _, err := ob.GetBestBid(); err == nil {
		t.Error("empty bids did not return unavailable")
	}
	if _, err := ob.GetBestAsk(); err == nil {
		t.Error("empty asks did not return unavailable")
	}
	if _, err := ob.GetLastPrice(); err == nil {
		t.Error("no trades did not return unavailable")
	}
	if _, err := ob.GetMidPrice(); err == nil {
		t.Error("empty book midpoint did not return unavailable")
	}

	ob.Bids.AddOrder(&etypes.Order{ID: 1, ClientID: 1, Price: 49900, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder})
	ob.Asks.AddOrder(&etypes.Order{ID: 2, ClientID: 1, Price: 50100, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder})

	if got := mustPrice(t)(ob.GetBestBid()); got != 49900 {
		t.Errorf("best bid: want 49900, got %d", got)
	}
	if got := mustPrice(t)(ob.GetBestAsk()); got != 50100 {
		t.Errorf("best ask: want 50100, got %d", got)
	}
	if got := mustPrice(t)(ob.GetMidPrice()); got != (49900+50100)/2 {
		t.Errorf("mid price: want %d, got %d", (49900+50100)/2, got)
	}

	ob.LastTrade = &etypes.Trade{Price: 50000}
	if got := mustPrice(t)(ob.GetLastPrice()); got != 50000 {
		t.Errorf("last price: want 50000, got %d", got)
	}
}
