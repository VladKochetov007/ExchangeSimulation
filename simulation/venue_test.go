package simulation

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
	"testing"
	"time"
)

func TestMountConnectNewClientNoLatency(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	m := NewMount(ex, LatencyConfig{})
	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gw := m.ConnectNewClient(1, balances, &exchange.FixedFee{})

	if gw == nil {
		t.Fatal("ConnectNewClient returned nil")
	}
	if gw.ID() != 1 {
		t.Errorf("Expected clientID 1, got %d", gw.ID())
	}
	// No latency: no DelayedGateway tracked
	if len(m.delayed) != 0 {
		t.Errorf("Expected no delayed gateways, got %d", len(m.delayed))
	}
	// Should be a raw *exchange.ClientGateway, not a DelayedGateway
	if _, ok := gw.(*exchange.ClientGateway); !ok {
		t.Error("Expected raw *exchange.ClientGateway without latency config")
	}
}

func TestMountConnectNewClientWithLatency(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	m := NewMount(ex, LatencyConfig{Request: NewConstantLatency(30 * time.Millisecond)})
	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gw := m.ConnectNewClient(1, balances, &exchange.FixedFee{})

	if gw == nil {
		t.Fatal("ConnectNewClient returned nil")
	}
	if len(m.delayed) != 1 {
		t.Errorf("Expected 1 delayed gateway, got %d", len(m.delayed))
	}
	if _, ok := gw.(*DelayedGateway); !ok {
		t.Error("Expected *DelayedGateway when latency is configured")
	}

	start := time.Now()
	gw.Send(exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID:   1,
			Symbol:      "BTC/USD",
			Side:        exchange.Buy,
			Type:        exchange.LimitOrder,
			Price:       5000000000000,
			Qty:         100000000,
			TimeInForce: exchange.GTC,
		},
	})

	select {
	case resp := <-gw.Responses():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got %v", resp.Error)
		}
		if elapsed < 25*time.Millisecond {
			t.Fatalf("Expected at least 25ms latency, got %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestMountConnectMultipleClients(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	m := NewMount(ex, LatencyConfig{Response: NewConstantLatency(5 * time.Millisecond)})
	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}

	gw1 := m.ConnectNewClient(1, balances, &exchange.FixedFee{})
	gw2 := m.ConnectNewClient(2, balances, &exchange.FixedFee{})

	if gw1 == nil || gw2 == nil {
		t.Fatal("ConnectNewClient returned nil")
	}
	if gw1.ID() != 1 || gw2.ID() != 2 {
		t.Errorf("Unexpected client IDs: %d, %d", gw1.ID(), gw2.ID())
	}
	if len(m.delayed) != 2 {
		t.Errorf("Expected 2 delayed gateways, got %d", len(m.delayed))
	}
}

func TestMountShutdown(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	m := NewMount(ex, LatencyConfig{Request: NewConstantLatency(1 * time.Millisecond)})
	m.ConnectNewClient(1, map[string]int64{"BTC": 1000000000, "USD": 100000000000000}, &exchange.FixedFee{})
	m.Shutdown() // Must not panic or block
}

func TestMountConnectNewClientReturnsGatewayInterface(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	m := NewMount(ex, LatencyConfig{})
	gw := m.ConnectNewClient(1, map[string]int64{}, &exchange.FixedFee{})
	var _ actor.Gateway = gw // compile-time interface check
}
