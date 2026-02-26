package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func TestLinkOrder(t *testing.T) {
	limit := GetLimit(100000)
	order := GetOrder()
	order.ID = 1
	order.Qty = 100
	order.FilledQty = 0

	LinkOrder(limit, order)

	if limit.Head != order {
		t.Error("Head should be the linked order")
	}
	if limit.Tail != order {
		t.Error("Tail should be the linked order")
	}
	if limit.TotalQty != 100 {
		t.Errorf("TotalQty should be 100, got %d", limit.TotalQty)
	}
	if limit.OrderCnt != 1 {
		t.Errorf("OrderCnt should be 1, got %d", limit.OrderCnt)
	}
}

func TestLinkMultipleOrders(t *testing.T) {
	limit := GetLimit(100000)
	order1 := GetOrder()
	order1.ID = 1
	order1.Qty = 100
	order2 := GetOrder()
	order2.ID = 2
	order2.Qty = 200

	LinkOrder(limit, order1)
	LinkOrder(limit, order2)

	if limit.Head != order1 {
		t.Error("Head should be first order")
	}
	if limit.Tail != order2 {
		t.Error("Tail should be second order")
	}
	if order1.Next != order2 {
		t.Error("First order Next should point to second")
	}
	if order2.Prev != order1 {
		t.Error("Second order Prev should point to first")
	}
	if limit.TotalQty != 300 {
		t.Errorf("TotalQty should be 300, got %d", limit.TotalQty)
	}
}

func TestUnlinkOrder(t *testing.T) {
	limit := GetLimit(100000)
	order := GetOrder()
	order.ID = 1
	order.Qty = 100

	LinkOrder(limit, order)
	UnlinkOrder(order)

	if limit.Head != nil {
		t.Error("Head should be nil after unlinking")
	}
	if limit.Tail != nil {
		t.Error("Tail should be nil after unlinking")
	}
	if limit.TotalQty != 0 {
		t.Errorf("TotalQty should be 0, got %d", limit.TotalQty)
	}
	if limit.OrderCnt != 0 {
		t.Errorf("OrderCnt should be 0, got %d", limit.OrderCnt)
	}
}

func TestUnlinkMiddleOrder(t *testing.T) {
	limit := GetLimit(100000)
	order1 := GetOrder()
	order1.ID = 1
	order1.Qty = 100
	order2 := GetOrder()
	order2.ID = 2
	order2.Qty = 200
	order3 := GetOrder()
	order3.ID = 3
	order3.Qty = 300

	LinkOrder(limit, order1)
	LinkOrder(limit, order2)
	LinkOrder(limit, order3)
	UnlinkOrder(order2)

	if order1.Next != order3 {
		t.Error("Order1 Next should point to Order3")
	}
	if order3.Prev != order1 {
		t.Error("Order3 Prev should point to Order1")
	}
	if limit.TotalQty != 400 {
		t.Errorf("TotalQty should be 400, got %d", limit.TotalQty)
	}
}
