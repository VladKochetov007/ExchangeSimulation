package exchange

import "testing"

func TestMatchBuyOrderFullFill(t *testing.T) {
	matcher := NewDefaultMatcher()
	bidBook := newBook(Buy)
	askBook := newBook(Sell)

	sellOrder := getOrder()
	sellOrder.ID = 1
	sellOrder.ClientID = 100
	sellOrder.Price = 100000
	sellOrder.Qty = 100
	askBook.AddOrder(sellOrder)

	buyOrder := getOrder()
	buyOrder.ID = 2
	buyOrder.ClientID = 200
	buyOrder.Side = Buy
	buyOrder.Type = LimitOrder
	buyOrder.Price = 100000
	buyOrder.Qty = 100

	result := matcher.Match(bidBook, askBook, buyOrder)

	if len(result.Executions) != 1 {
		t.Fatalf("Should have 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Qty != 100 {
		t.Errorf("Execution qty should be 100, got %d", result.Executions[0].Qty)
	}
	if buyOrder.Status != Filled {
		t.Errorf("Buy order should be Filled, got %v", buyOrder.Status)
	}
}

func TestMatchPartialFill(t *testing.T) {
	matcher := NewDefaultMatcher()
	bidBook := newBook(Buy)
	askBook := newBook(Sell)

	sellOrder := getOrder()
	sellOrder.ID = 1
	sellOrder.ClientID = 100
	sellOrder.Price = 100000
	sellOrder.Qty = 50
	askBook.AddOrder(sellOrder)

	buyOrder := getOrder()
	buyOrder.ID = 2
	buyOrder.ClientID = 200
	buyOrder.Side = Buy
	buyOrder.Type = LimitOrder
	buyOrder.Price = 100000
	buyOrder.Qty = 100

	result := matcher.Match(bidBook, askBook, buyOrder)

	if len(result.Executions) != 1 {
		t.Fatalf("Should have 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Qty != 50 {
		t.Errorf("Execution qty should be 50, got %d", result.Executions[0].Qty)
	}
	if buyOrder.FilledQty != 50 {
		t.Errorf("Buy order FilledQty should be 50, got %d", buyOrder.FilledQty)
	}
	if buyOrder.Status != PartialFill {
		t.Errorf("Buy order should be PartialFill, got %v", buyOrder.Status)
	}
}

func TestMatchRejectsSelfTrade(t *testing.T) {
	matcher := NewDefaultMatcher()
	bidBook := newBook(Buy)
	askBook := newBook(Sell)

	sellOrder := getOrder()
	sellOrder.ID = 1
	sellOrder.ClientID = 100
	sellOrder.Price = 100000
	sellOrder.Qty = 100
	askBook.AddOrder(sellOrder)

	buyOrder := getOrder()
	buyOrder.ID = 2
	buyOrder.ClientID = 100
	buyOrder.Side = Buy
	buyOrder.Type = LimitOrder
	buyOrder.Price = 100000
	buyOrder.Qty = 100

	result := matcher.Match(bidBook, askBook, buyOrder)

	if len(result.Executions) != 0 {
		t.Errorf("Should have 0 executions (self-trade rejected), got %d", len(result.Executions))
	}
}

func TestMatchMarketOrder(t *testing.T) {
	matcher := NewDefaultMatcher()
	bidBook := newBook(Buy)
	askBook := newBook(Sell)

	sellOrder := getOrder()
	sellOrder.ID = 1
	sellOrder.ClientID = 100
	sellOrder.Price = 100000
	sellOrder.Qty = 100
	askBook.AddOrder(sellOrder)

	buyOrder := getOrder()
	buyOrder.ID = 2
	buyOrder.ClientID = 200
	buyOrder.Side = Buy
	buyOrder.Type = Market
	buyOrder.Qty = 100

	result := matcher.Match(bidBook, askBook, buyOrder)

	if len(result.Executions) != 1 {
		t.Fatalf("Should have 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Price != 100000 {
		t.Errorf("Execution price should be maker price (100000), got %d", result.Executions[0].Price)
	}
}
