package simulation

import (
	"testing"

	"exchange_sim/exchange"
)

func TestFeedOnlyGatewayPermitsOnlySubscriptions(t *testing.T) {
	inner := exchange.NewClientGateway(7)
	feed := NewFeedOnlyGateway(inner)
	feed.Send(exchange.Request{Type: exchange.ReqSubscribe, QueryReq: &exchange.QueryRequest{RequestID: 1, Symbol: "ABC/USD"}})
	select {
	case request := <-inner.RequestCh:
		if request.Type != exchange.ReqSubscribe {
			t.Fatalf("feed sent %s", request.Type)
		}
	default:
		t.Fatal("feed subscription did not reach underlying gateway")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("feed-only gateway accepted an order request")
		}
	}()
	feed.Send(exchange.Request{Type: exchange.ReqPlaceOrder, OrderReq: &exchange.OrderRequest{RequestID: 2}})
}
