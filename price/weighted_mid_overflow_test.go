package price

import (
	"math"
	"testing"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

func TestWeightedMidFallsBackWhenBestLevelWeightsOverflow(t *testing.T) {
	book := &ebook.OrderBook{Bids: ebook.NewBook(etypes.Buy), Asks: ebook.NewBook(etypes.Sell)}
	if !book.Bids.AddOrder(&etypes.Order{ID: 1, Side: etypes.Buy, Type: etypes.LimitOrder, Price: 100, Qty: math.MaxInt64}) {
		t.Fatal("bid was rejected")
	}
	if !book.Asks.AddOrder(&etypes.Order{ID: 2, Side: etypes.Sell, Type: etypes.LimitOrder, Price: 200, Qty: math.MaxInt64}) {
		t.Fatal("ask was rejected")
	}

	if got := NewWeightedMidPriceCalculator().Calculate(book); got != 150 {
		t.Fatalf("weighted midpoint = %d, want ordinary midpoint 150 on aggregate overflow", got)
	}
}
