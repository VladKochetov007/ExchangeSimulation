package matching

import (
	"math"
	"testing"

	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

func TestProRataSkipsUnrepresentableLevelWithoutMutation(t *testing.T) {
	asks := ebook.NewBook(etypes.Sell)
	first := &etypes.Order{ID: 1, ClientID: 1, Side: etypes.Sell, Type: etypes.LimitOrder, Price: 100, Qty: math.MaxInt64}
	if !asks.AddOrder(first) {
		t.Fatal("initial order was rejected")
	}
	second := &etypes.Order{ID: 2, ClientID: 2, Side: etypes.Sell, Type: etypes.LimitOrder, Price: 100, Qty: 1, Parent: asks.Best, Prev: first}
	first.Next = second
	asks.Best.Tail = second
	asks.Best.OrderCnt++

	incoming := &etypes.Order{ID: 3, ClientID: 3, Side: etypes.Buy, Type: etypes.LimitOrder, Price: 100, Qty: 1}
	result := NewProRataMatcher(&eclock.RealClock{}).Match(ebook.NewBook(etypes.Buy), asks, incoming)

	if len(result.Executions) != 0 || incoming.FilledQty != 0 || incoming.Status != etypes.Open {
		t.Fatalf("unrepresentable level must be untouched: executions=%d filled=%d status=%v", len(result.Executions), incoming.FilledQty, incoming.Status)
	}
	if first.FilledQty != 0 || second.FilledQty != 0 || asks.Best.Head != first || asks.Best.Tail != second || asks.Best.TotalQty != math.MaxInt64 || asks.Best.OrderCnt != 2 {
		t.Fatal("pro-rata allocation changed an unrepresentable live level")
	}
}
