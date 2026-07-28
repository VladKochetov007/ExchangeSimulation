package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// One client that stops draining its ResponseCh must not stall the engine:
// notifications generated under the exchange lock are queued to a per-gateway
// outbox and delivered by a background goroutine, so PlaceOrder returns even
// when a counterparty's channel is full. Before the outbox, sendResponse
// retried inside the write lock and froze every book and client.
func TestRegressionSlowConsumerDoesNotStallEngine(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(inst)

	// Client 1: live gateway with a single-slot response channel, pre-filled
	// and never drained — the slow consumer.
	respCh := make(chan Response, 1)
	gw1 := NewClientGatewayFromChannels(1, make(chan Request, 8), respCh, make(chan *MarketDataMsg, 8))
	ex.Lock()
	ex.Clients[1] = NewClient(1, &FixedFee{})
	ex.Clients[1].Balances = map[string]int64{"USD": USDAmount(1_000_000)}
	ex.Gateways[1] = gw1
	ex.Unlock()
	respCh <- Response{}

	ex.ConnectNewClient(2, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})

	price := PriceUSD(50_000, CENT_TICK)
	placeDirect := func(clientID uint64, side Side, reqID uint64) Response {
		return ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: reqID, Side: side, Type: LimitOrder,
			Price: price, Qty: BTCAmount(1), Symbol: "BTC/USD",
			TimeInForce: GTC, Visibility: Normal,
		})
	}
	if resp := placeDirect(1, Buy, 1); !resp.Success {
		t.Fatalf("maker rejected: %s", resp.Error)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		placeDirect(2, Sell, 2)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("PlaceOrder stalled behind a slow consumer's full ResponseCh")
	}

	// Engine stays responsive while client 1's fill waits in its outbox.
	if resp := placeDirect(2, Sell, 3); !resp.Success {
		t.Fatalf("follow-up order rejected: %s", resp.Error)
	}

	// Once the consumer drains, the queued fill arrives (at-least-once kept).
	<-respCh
	select {
	case resp := <-respCh:
		if _, ok := resp.Data.(*FillNotification); !ok {
			t.Fatalf("expected fill notification after drain, got %#v", resp.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued fill never delivered after the consumer drained")
	}
}

// CancelAllClientOrders iterates books and order maps; the forced-cancel
// notifications must arrive in placement (order-ID) order, not Go's random
// map order, or the event stream differs run to run.
func TestRegressionCancelAllNotificationsAreOrdered(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(inst)

	gw := ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(10_000_000)}, &FixedFee{}).(*ClientGateway)

	const orders = 20
	for i := range orders {
		price := PriceUSD(40_000+float64(i)*10, CENT_TICK)
		if _, reject := InjectLimitOrder(ex, 1, "BTC/USD", Buy, price, BTCAmount(0.1)); reject != "" {
			t.Fatalf("order %d rejected: %s", i, reject)
		}
	}

	if got := ex.CancelAllClientOrders(1); got != orders {
		t.Fatalf("cancelled %d orders, want %d", got, orders)
	}

	var lastID uint64
	for i := range orders {
		select {
		case resp := <-gw.ResponseCh:
			fc, ok := resp.Data.(*ForcedCancelNotification)
			if !ok {
				t.Fatalf("message %d: expected forced cancel, got %#v", i, resp.Data)
			}
			if fc.OrderID <= lastID {
				t.Fatalf("forced cancels out of placement order: %d after %d", fc.OrderID, lastID)
			}
			lastID = fc.OrderID
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d forced cancels delivered", i, orders)
		}
	}
}

// A multi-level sweep publishes one delta per touched level; the publish order
// must be price-sorted, not map order, for a replayable delta stream.
func TestRegressionSweepDeltasArePriceSorted(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(inst)

	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(1000)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(100_000_000)}, &FixedFee{})

	mdGw := NewClientGateway(99)
	ex.MDPublisher.Subscribe(99, "BTC/USD", []MDType{MDDelta}, mdGw)

	drain := func() {
		for {
			select {
			case <-mdGw.MarketData:
			default:
				return
			}
		}
	}

	for range 20 {
		prices := []int64{
			PriceUSD(50_000, CENT_TICK),
			PriceUSD(50_010, CENT_TICK),
			PriceUSD(50_020, CENT_TICK),
		}
		for _, p := range prices {
			if _, reject := InjectLimitOrder(ex, 1, "BTC/USD", Sell, p, BTCAmount(0.1)); reject != "" {
				t.Fatalf("maker rejected: %s", reject)
			}
		}
		drain()

		if _, reject := InjectMarketOrder(ex, 2, "BTC/USD", Buy, BTCAmount(0.3)); reject != "" {
			t.Fatalf("sweep rejected: %s", reject)
		}

		var got []int64
		for len(got) < len(prices) {
			select {
			case msg := <-mdGw.MarketData:
				got = append(got, msg.Data.(*BookDelta).Price)
			case <-time.After(2 * time.Second):
				t.Fatalf("sweep published %d of %d deltas", len(got), len(prices))
			}
		}
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Fatalf("sweep deltas not price-sorted: %v", got)
			}
		}
		drain()
	}
}
