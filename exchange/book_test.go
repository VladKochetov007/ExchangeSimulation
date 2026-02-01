package exchange

import "testing"

func TestNewBook(t *testing.T) {
	book := newBook(Buy)
	if book.Side != Buy {
		t.Error("Side should be Buy")
	}
	if book.Best != nil {
		t.Error("Best should be nil")
	}
	if len(book.Orders) != 0 {
		t.Error("Orders should be empty")
	}
}

func TestAddOrderCreatesLimit(t *testing.T) {
	book := newBook(Buy)
	order := getOrder()
	order.ID = 1
	order.Price = 100000
	order.Qty = 100

	book.addOrder(order)

	if book.Best == nil {
		t.Fatal("Best should not be nil")
	}
	if book.Best.Price != 100000 {
		t.Errorf("Best price should be 100000, got %d", book.Best.Price)
	}
	if book.Orders[1] != order {
		t.Error("Order should be in Orders map")
	}
}

func TestCancelOrder(t *testing.T) {
	book := newBook(Buy)
	order := getOrder()
	order.ID = 1
	order.Price = 100000
	order.Qty = 100

	book.addOrder(order)
	cancelled := book.cancelOrder(1)

	if cancelled == nil {
		t.Fatal("Should return cancelled order")
	}
	if book.Orders[1] != nil {
		t.Error("Order should be removed from map")
	}
	if book.Best != nil {
		t.Error("Best should be nil after removing only order")
	}
}

func TestBestBidUpdates(t *testing.T) {
	book := newBook(Buy)
	order1 := getOrder()
	order1.ID = 1
	order1.Price = 100000
	order1.Qty = 100

	order2 := getOrder()
	order2.ID = 2
	order2.Price = 110000
	order2.Qty = 100

	book.addOrder(order1)
	if book.Best.Price != 100000 {
		t.Errorf("Best should be 100000, got %d", book.Best.Price)
	}

	book.addOrder(order2)
	if book.Best.Price != 110000 {
		t.Errorf("Best should be 110000 (highest bid), got %d", book.Best.Price)
	}
}

func TestBestAskUpdates(t *testing.T) {
	book := newBook(Sell)
	order1 := getOrder()
	order1.ID = 1
	order1.Price = 110000
	order1.Qty = 100

	order2 := getOrder()
	order2.ID = 2
	order2.Price = 100000
	order2.Qty = 100

	book.addOrder(order1)
	if book.Best.Price != 110000 {
		t.Errorf("Best should be 110000, got %d", book.Best.Price)
	}

	book.addOrder(order2)
	if book.Best.Price != 100000 {
		t.Errorf("Best should be 100000 (lowest ask), got %d", book.Best.Price)
	}
}
