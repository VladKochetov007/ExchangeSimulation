package simulation

import (
	"exchange_sim/exchange"
	"testing"
	"time"
)

func TestDelayedGatewayNoLatency(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := LatencyConfig{
		Mode:  LatencyNone,
		Clock: &RealClock{},
	}
	delayedGw := NewDelayedGateway(gateway, config)
	delayedGw.Start()
	defer delayedGw.Stop()

	req := exchange.Request{
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
	}

	start := time.Now()
	delayedGw.RequestCh() <- req

	select {
	case resp := <-delayedGw.ResponseCh():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got error %v", resp.Error)
		}
		if elapsed > 100*time.Millisecond {
			t.Fatalf("No latency mode should be fast, took %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestDelayedGatewayRequestLatency(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := LatencyConfig{
		RequestLatency: NewConstantLatency(50 * time.Millisecond),
		Mode:           LatencyRequest,
		Clock:          &RealClock{},
	}
	delayedGw := NewDelayedGateway(gateway, config)
	delayedGw.Start()
	defer delayedGw.Stop()

	req := exchange.Request{
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
	}

	start := time.Now()
	delayedGw.RequestCh() <- req

	select {
	case resp := <-delayedGw.ResponseCh():
		elapsed := time.Since(start)
		if !resp.Success {
			t.Fatalf("Order should succeed, got error %v", resp.Error)
		}
		if elapsed < 40*time.Millisecond {
			t.Fatalf("Expected at least 40ms latency, got %v", elapsed)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestDelayedGatewayResponseLatency(t *testing.T) {
	ex := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := LatencyConfig{
		ResponseLatency: NewConstantLatency(30 * time.Millisecond),
		Mode:            LatencyResponse,
		Clock:           &RealClock{},
	}
	delayedGw := NewDelayedGateway(gateway, config)
	delayedGw.Start()
	defer delayedGw.Stop()

	req := exchange.Request{
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
	}

	delayedGw.RequestCh() <- req

	start := time.Now()
	select {
	case resp := <-delayedGw.ResponseCh():
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
	ex := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := LatencyConfig{
		MarketDataLatency: NewConstantLatency(20 * time.Millisecond),
		Mode:              LatencyMarketData,
		Clock:             &RealClock{},
	}
	delayedGw := NewDelayedGateway(gateway, config)
	delayedGw.Start()
	defer delayedGw.Stop()

	subReq := exchange.Request{
		Type: exchange.ReqSubscribe,
		QueryReq: &exchange.QueryRequest{
			RequestID: 1,
			Symbol:    "BTC/USD",
		},
	}
	delayedGw.RequestCh() <- subReq

	<-delayedGw.ResponseCh()

	orderReq := exchange.Request{
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
	}
	gateway.RequestCh <- orderReq

	start := time.Now()
	select {
	case msg := <-delayedGw.MarketData():
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
	ex := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000000}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := LatencyConfig{
		RequestLatency:    NewConstantLatency(10 * time.Millisecond),
		ResponseLatency:   NewConstantLatency(15 * time.Millisecond),
		MarketDataLatency: NewConstantLatency(5 * time.Millisecond),
		Mode:              LatencyAll,
		Clock:             &RealClock{},
	}
	delayedGw := NewDelayedGateway(gateway, config)
	delayedGw.Start()
	defer delayedGw.Stop()

	req := exchange.Request{
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
	}

	start := time.Now()
	delayedGw.RequestCh() <- req

	select {
	case resp := <-delayedGw.ResponseCh():
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

const LatencyNone LatencyMode = 0
