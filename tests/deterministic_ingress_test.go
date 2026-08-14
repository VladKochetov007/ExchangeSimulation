package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

func TestDeterministicIngressDrainsClientRoundRobin(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{EstimatedClients: 2, Clock: &RealClock{}, DeterministicIngress: true})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))

	balances := map[string]int64{"USD": 1_000_000 * USD_PRECISION}
	g1 := ex.ConnectNewClient(1, balances, &PercentageFee{}).(*ClientGateway)
	g2 := ex.ConnectNewClient(2, balances, &PercentageFee{}).(*ClientGateway)

	request := func(price int64) Request {
		return Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
			Symbol: "ABC/USD", Side: Buy, Type: LimitOrder, Price: price,
			Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
		}}
	}

	// Enqueue client 2 first. The deterministic policy is client-ID
	// round-robin, not channel arrival or goroutine scheduling order.
	g2.Send(request(101 * USD_PRECISION))
	g1.Send(request(100 * USD_PRECISION))
	g2.Send(request(103 * USD_PRECISION))
	g1.Send(request(102 * USD_PRECISION))
	if !ex.DrainIngress() {
		t.Fatal("DrainIngress reported no work")
	}
	if ex.DrainIngress() {
		t.Fatal("DrainIngress reported work after queues drained")
	}

	book := ex.Books["ABC/USD"]
	want := map[uint64]struct {
		clientID uint64
		price    int64
	}{
		2: {clientID: 1, price: 100 * USD_PRECISION},
		3: {clientID: 2, price: 101 * USD_PRECISION},
		4: {clientID: 1, price: 102 * USD_PRECISION},
		5: {clientID: 2, price: 103 * USD_PRECISION},
	}
	for orderID, expected := range want {
		order := book.Bids.Orders[orderID]
		if order == nil {
			t.Fatalf("missing order %d", orderID)
		}
		if order.ClientID != expected.clientID || order.Price != expected.price {
			t.Fatalf("order %d = client %d price %d, want client %d price %d", orderID, order.ClientID, order.Price, expected.clientID, expected.price)
		}
	}
}
