package matching

import (
	"testing"

	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

func TestMatchBuyOrderFullFill(t *testing.T) {
	matcher := NewDefaultMatcher(&eclock.RealClock{})
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
	matcher := NewDefaultMatcher(&eclock.RealClock{})
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
	matcher := NewDefaultMatcher(&eclock.RealClock{})
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
	matcher := NewDefaultMatcher(&eclock.RealClock{})
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
