package simulation

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func newTestGateway() (*exchange.Exchange, actor.Gateway) {
	ex := exchange.NewExchange(10, &RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(inst)
	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	return ex, ex.ConnectNewClient(1, balances, &exchange.FixedFee{})
}

func placeOrder(gw actor.Gateway, reqID uint64) {
	gw.Send(exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   reqID,
			Symbol:      "BTC/USD",
			Side:        exchange.Buy,
			Type:        exchange.LimitOrder,
			Price:       5000000000000,
			Qty:         100000000,
			TimeInForce: exchange.GTC,
		},
	})
}

func TestDelayedGatewayNoLatency(t *testing.T) {
	_, gw := newTestGateway()

	// All nil — NewDelayedGateway still wraps but with zero delays.
	// Test passthrough by just using the raw gateway (no latency, no wrapping needed).
	d := NewDelayedGateway(gw, nil, nil, nil)
	d.Start()
	defer d.Stop()

	start := time.Now()
	placeOrder(d, 1)

	select {
	case resp := <-d.Responses():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got error %v", resp.Error)
		}
		if elapsed > 100*time.Millisecond {
			t.Fatalf("No latency should be fast, took %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestDelayedGatewayRequestLatency(t *testing.T) {
	_, gw := newTestGateway()

	d := NewDelayedGateway(gw, NewConstantLatency(50*time.Millisecond), nil, nil)
	d.Start()
	defer d.Stop()

	start := time.Now()
	placeOrder(d, 1)

	select {
	case resp := <-d.Responses():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got error %v", resp.Error)
		}
		if elapsed < 40*time.Millisecond {
			t.Fatalf("Expected at least 40ms request latency, got %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestDelayedGatewayResponseLatency(t *testing.T) {
	_, gw := newTestGateway()

	d := NewDelayedGateway(gw, nil, NewConstantLatency(30*time.Millisecond), nil)
	d.Start()
	defer d.Stop()

	placeOrder(d, 1)

	start := time.Now()
	select {
	case resp := <-d.Responses():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got error %v", resp.Error)
		}
		if elapsed < 20*time.Millisecond {
			t.Fatalf("Expected at least 20ms response latency, got %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestDelayedGatewayMarketDataLatency(t *testing.T) {
	_, gw := newTestGateway()

	d := NewDelayedGateway(gw, nil, nil, NewConstantLatency(20*time.Millisecond))
	d.Start()
	defer d.Stop()

	// Subscribe via the delayed gateway
	d.Send(exchange.Request{
		Type: exchange.ReqSubscribe,
		QueryReq: &exchange.QueryRequest{RequestID: 1, Symbol: "BTC/USD"},
	})
	<-d.Responses()

	// Place an order directly on the inner gateway to generate market data
	gw.Send(exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   2,
			Symbol:      "BTC/USD",
			Side:        exchange.Sell,
			Type:        exchange.LimitOrder,
			Price:       5000000000000,
			Qty:         100000000,
			TimeInForce: exchange.GTC,
		},
	})

	start := time.Now()
	select {
	case msg := <-d.MarketDataCh():
		elapsed := time.Since(start)
		if msg == nil {
			t.Fatal("Received nil market data")
		}
		if elapsed < 15*time.Millisecond {
			t.Fatalf("Expected at least 15ms market data latency, got %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for market data")
	}
}

func TestDelayedGatewayAllLatencies(t *testing.T) {
	_, gw := newTestGateway()

	d := NewDelayedGateway(gw,
		NewConstantLatency(10*time.Millisecond),
		NewConstantLatency(15*time.Millisecond),
		NewConstantLatency(5*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	start := time.Now()
	placeOrder(d, 1)

	select {
	case resp := <-d.Responses():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got error %v", resp.Error)
		}
		if elapsed < 20*time.Millisecond {
			t.Fatalf("Expected at least 20ms total latency (request+response), got %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestDelayedGatewayImplementsGatewayInterface(t *testing.T) {
	_, gw := newTestGateway()
	d := NewDelayedGateway(gw, nil, nil, nil)
	// Compile-time check: *DelayedGateway must satisfy actor.Gateway
	var _ actor.Gateway = d
	if d.ID() != gw.ID() {
		t.Errorf("ID mismatch: got %d, want %d", d.ID(), gw.ID())
	}
}
