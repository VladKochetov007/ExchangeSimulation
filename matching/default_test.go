package matching

import (
	"testing"

	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

func TestMatchBuyOrderFullFill(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)

	sell := &etypes.Order{ID: 1, ClientID: 100, Price: 100000, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder}
	asks.AddOrder(sell)

	buy := &etypes.Order{ID: 2, ClientID: 200, Price: 100000, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder}
	result := matcher.Match(bids, asks, buy)

	if len(result.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Qty != 100 {
		t.Errorf("execution qty: want 100, got %d", result.Executions[0].Qty)
	}
	if buy.Status != etypes.Filled {
		t.Errorf("buy order status: want Filled, got %v", buy.Status)
	}
}

func TestMatchPartialFill(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)

	sell := &etypes.Order{ID: 1, ClientID: 100, Price: 100000, Qty: 50, Side: etypes.Sell, Type: etypes.LimitOrder}
	asks.AddOrder(sell)

	buy := &etypes.Order{ID: 2, ClientID: 200, Price: 100000, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder}
	result := matcher.Match(bids, asks, buy)

	if len(result.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Qty != 50 {
		t.Errorf("execution qty: want 50, got %d", result.Executions[0].Qty)
	}
	if buy.FilledQty != 50 {
		t.Errorf("buy FilledQty: want 50, got %d", buy.FilledQty)
	}
	if buy.Status != etypes.PartialFill {
		t.Errorf("buy status: want PartialFill, got %v", buy.Status)
	}
}

func TestMatchRejectsSelfTrade(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)

	sell := &etypes.Order{ID: 1, ClientID: 100, Price: 100000, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder}
	asks.AddOrder(sell)

	buy := &etypes.Order{ID: 2, ClientID: 100, Price: 100000, Qty: 100, Side: etypes.Buy, Type: etypes.LimitOrder}
	result := matcher.Match(bids, asks, buy)

	if len(result.Executions) != 0 {
		t.Errorf("self-trade: expected 0 executions, got %d", len(result.Executions))
	}
}

func TestMatchMarketOrder(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)

	sell := &etypes.Order{ID: 1, ClientID: 100, Price: 100000, Qty: 100, Side: etypes.Sell, Type: etypes.LimitOrder}
	asks.AddOrder(sell)

	buy := &etypes.Order{ID: 2, ClientID: 200, Qty: 100, Side: etypes.Buy, Type: etypes.Market}
	result := matcher.Match(bids, asks, buy)

	if len(result.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Price != 100000 {
		t.Errorf("execution price: want 100000 (maker), got %d", result.Executions[0].Price)
	}
}

func TestPriceTimeMatcherOrdersAndExecutesSignedPrices(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)

	// A numerically higher negative bid has priority, exactly as for positive
	// prices: -10 is better than -20.
	for _, order := range []*etypes.Order{
		{ID: 1, ClientID: 101, Price: -20, Qty: 1, Side: etypes.Buy, Type: etypes.LimitOrder},
		{ID: 2, ClientID: 102, Price: -10, Qty: 1, Side: etypes.Buy, Type: etypes.LimitOrder},
		{ID: 3, ClientID: 103, Price: -1, Qty: 1, Side: etypes.Buy, Type: etypes.LimitOrder},
	} {
		if !bids.AddOrder(order) {
			t.Fatalf("could not add signed bid %#v", order)
		}
	}
	if bids.Best == nil || bids.Best.Price != -1 {
		t.Fatalf("best negative bid = %#v, want -1", bids.Best)
	}

	// The incoming sell crosses all three negative levels and consumes them in
	// descending numeric order, proving numeric ordering is not positivity
	// dependent.
	sell := &etypes.Order{ID: 4, ClientID: 200, Price: -25, Qty: 3, Side: etypes.Sell, Type: etypes.LimitOrder}
	result := matcher.Match(bids, asks, sell)
	if len(result.Executions) != 3 {
		t.Fatalf("signed executions = %d, want 3", len(result.Executions))
	}
	for i, want := range []int64{-1, -10, -20} {
		if got := result.Executions[i].Price; got != want {
			t.Fatalf("execution %d price = %d, want %d", i, got, want)
		}
	}
}

func TestPriceTimeMatcherCrossesZeroAndPreservesSelfTradePrevention(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)
	if !asks.AddOrder(&etypes.Order{ID: 1, ClientID: 10, Price: 0, Qty: 1, Side: etypes.Sell, Type: etypes.LimitOrder}) {
		t.Fatal("could not add zero ask")
	}
	if !asks.AddOrder(&etypes.Order{ID: 2, ClientID: 11, Price: 1, Qty: 1, Side: etypes.Sell, Type: etypes.LimitOrder}) {
		t.Fatal("could not add positive ask")
	}

	// A zero-priced buy crosses the zero ask, while a self-owned zero ask is
	// still skipped rather than creating a signed-price self trade.
	buy := &etypes.Order{ID: 3, ClientID: 10, Price: 1, Qty: 2, Side: etypes.Buy, Type: etypes.LimitOrder}
	result := matcher.Match(bids, asks, buy)
	if len(result.Executions) != 1 || result.Executions[0].Price != 1 || result.Executions[0].MakerClientID != 11 {
		t.Fatalf("cross-zero executions = %#v, want one non-self fill at 1", result.Executions)
	}
}

func TestPriceTimeMatcherMarketOrderExecutesNegativeLevel(t *testing.T) {
	matcher := NewPriceTimeMatcher(&eclock.RealClock{})
	bids := ebook.NewBook(etypes.Buy)
	asks := ebook.NewBook(etypes.Sell)
	if !asks.AddOrder(&etypes.Order{ID: 1, ClientID: 100, Price: -7, Qty: 1, Side: etypes.Sell, Type: etypes.LimitOrder}) {
		t.Fatal("could not add negative ask")
	}
	result := matcher.Match(bids, asks, &etypes.Order{ID: 2, ClientID: 200, Qty: 1, Side: etypes.Buy, Type: etypes.Market})
	if len(result.Executions) != 1 || result.Executions[0].Price != -7 {
		t.Fatalf("negative market execution = %#v, want price -7", result.Executions)
	}
}
