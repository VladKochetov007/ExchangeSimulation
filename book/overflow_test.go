package book

import (
	"math"
	"testing"

	etypes "exchange_sim/types"
)

func TestLinkOrderRejectsAggregateOverflowWithoutMutation(t *testing.T) {
	limit := &etypes.Limit{Price: 100}
	first := &etypes.Order{ID: 1, Qty: math.MaxInt64 - 1}
	if !LinkOrder(limit, first) {
		t.Fatal("initial representable order was rejected")
	}
	second := &etypes.Order{ID: 2, Qty: 2}

	if LinkOrder(limit, second) {
		t.Fatal("aggregate overflow must be rejected")
	}
	if limit.Head != first || limit.Tail != first || limit.TotalQty != math.MaxInt64-1 || limit.OrderCnt != 1 {
		t.Fatalf("failed link changed live level: head=%p tail=%p total=%d count=%d", limit.Head, limit.Tail, limit.TotalQty, limit.OrderCnt)
	}
	if second.Parent != nil || second.Prev != nil || second.Next != nil {
		t.Fatal("failed link changed rejected order queue pointers")
	}
}

func TestAddOrderRejectsOverflowWithoutCreatingOrIndexingOrder(t *testing.T) {
	b := NewBook(etypes.Buy)
	first := &etypes.Order{ID: 1, Price: 100, Qty: math.MaxInt64 - 1}
	if !b.AddOrder(first) {
		t.Fatal("initial representable order was rejected")
	}
	second := &etypes.Order{ID: 2, Price: 100, Qty: 2}

	if b.AddOrder(second) {
		t.Fatal("aggregate overflow must be rejected")
	}
	if got := b.Orders[2]; got != nil {
		t.Fatalf("rejected order was indexed: %#v", got)
	}
	if len(b.Limits) != 1 || b.Best == nil || b.Best.TotalQty != math.MaxInt64-1 || b.Best.Head != first || b.Best.Tail != first {
		t.Fatal("rejected order changed live book")
	}
}

func TestNewBookWithCapacityPreservesBookSemantics(t *testing.T) {
	book := NewBookWithCapacity(etypes.Buy, 0, 0)
	orders := []*etypes.Order{
		{ID: 1, Price: 100, Qty: 3},
		{ID: 2, Price: 101, Qty: 2},
		{ID: 3, Price: 100, Qty: 4},
	}
	for _, order := range orders {
		if !book.AddOrder(order) {
			t.Fatalf("AddOrder(%d) failed", order.ID)
		}
	}
	if book.Best == nil || book.Best.Price != 101 || len(book.Orders) != 3 || len(book.Limits) != 2 {
		t.Fatalf("capacity-hinted book state = %#v", book)
	}
	if got := book.CancelOrder(2); got != orders[1] || book.Best == nil || book.Best.Price != 100 {
		t.Fatalf("capacity-hinted cancellation = %#v, best=%#v", got, book.Best)
	}
}

func TestVisibleQtySaturatesUnrepresentableManualLevel(t *testing.T) {
	first := &etypes.Order{ID: 1, Qty: math.MaxInt64, Visibility: etypes.Normal}
	second := &etypes.Order{ID: 2, Qty: 1, Visibility: etypes.Normal, Prev: first}
	first.Next = second
	limit := &etypes.Limit{Price: 100, TotalQty: math.MaxInt64, OrderCnt: 2, Head: first, Tail: second}

	if got := VisibleQty(limit); got != math.MaxInt64 {
		t.Fatalf("visible quantity = %d, want saturation at %d", got, int64(math.MaxInt64))
	}
	b := NewBook(etypes.Buy)
	b.Best, b.ActiveHead, b.ActiveTail = limit, limit, limit
	b.Limits[limit.Price] = limit
	snapshot := b.GetSnapshot()
	if len(snapshot) != 1 || snapshot[0].VisibleQty != math.MaxInt64 || snapshot[0].HiddenQty != 0 {
		t.Fatalf("saturated snapshot = %#v, want non-negative capped depth", snapshot)
	}
}
