package exchange

import (
	"testing"
	"time"
)

func TestClientNotificationsOnPlaceOrder(t *testing.T) {
	ex := NewExchange(10)
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	go ex.handleClientRequests(gateway)

	orderReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: orderReq}

	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Fatalf("Order should succeed")
		}
		t.Logf("✅ CLIENT RECEIVES: Order acknowledgment with orderID=%d", resp.Data.(uint64))
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for response")
	}

	select {
	case <-gateway.ResponseCh:
		t.Errorf("❌ UNEXPECTED: Client should NOT receive execution report for resting order")
	case <-time.After(50 * time.Millisecond):
		t.Logf("✅ CORRECT: No execution report sent (order resting on book)")
	}

	gateway.Close()
}

func TestClientNotificationsOnFill(t *testing.T) {
	ex := NewExchange(10)
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway1 := ex.ConnectClient(1, balances, &FixedFee{})
	gateway2 := ex.ConnectClient(2, balances, &FixedFee{})

	go ex.handleClientRequests(gateway1)
	go ex.handleClientRequests(gateway2)

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	gateway1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}

	select {
	case resp := <-gateway1.ResponseCh:
		if !resp.Success {
			t.Fatalf("Sell order should succeed")
		}
		t.Logf("✅ MAKER receives: Order ack (orderID=%d)", resp.Data.(uint64))
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout")
	}

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}
	gateway2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}

	select {
	case resp := <-gateway2.ResponseCh:
		if !resp.Success {
			t.Fatalf("Buy order should succeed")
		}
		t.Logf("✅ TAKER receives: Order ack (orderID=%d)", resp.Data.(uint64))
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout")
	}

	select {
	case <-gateway1.ResponseCh:
		t.Errorf("❌ GAP: Maker should receive fill notification but doesn't!")
	case <-time.After(50 * time.Millisecond):
		t.Logf("❌ MISSING: Maker receives NO fill notification")
	}

	select {
	case <-gateway2.ResponseCh:
		t.Errorf("❌ GAP: Taker should receive fill notification but doesn't!")
	case <-time.After(50 * time.Millisecond):
		t.Logf("❌ MISSING: Taker receives NO fill notification")
	}

	gateway1.Close()
	gateway2.Close()
}

func TestClientNotificationsViaMarketData(t *testing.T) {
	ex := NewExchange(10)
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway1 := ex.ConnectClient(1, balances, &FixedFee{})
	gateway2 := ex.ConnectClient(2, balances, &FixedFee{})

	go ex.handleClientRequests(gateway1)
	go ex.handleClientRequests(gateway2)

	subReq1 := &QueryRequest{RequestID: 1, Symbol: "BTC/USD"}
	gateway1.RequestCh <- Request{Type: ReqSubscribe, QueryReq: subReq1}
	<-gateway1.ResponseCh

	subReq2 := &QueryRequest{RequestID: 2, Symbol: "BTC/USD"}
	gateway2.RequestCh <- Request{Type: ReqSubscribe, QueryReq: subReq2}
	<-gateway2.ResponseCh

	<-gateway1.MarketData
	<-gateway2.MarketData

	sellReq := &OrderRequest{
		RequestID:   3,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	gateway1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}
	<-gateway1.ResponseCh

	buyReq := &OrderRequest{
		RequestID:   4,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}
	gateway2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}
	<-gateway2.ResponseCh

	tradeReceived1 := false
	tradeReceived2 := false

	select {
	case msg := <-gateway1.MarketData:
		if msg.Type == MDTrade {
			trade := msg.Data.(*Trade)
			t.Logf("✅ Maker via MarketData: Trade (price=%d, qty=%d)", trade.Price, trade.Qty)
			tradeReceived1 = true
		}
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case msg := <-gateway2.MarketData:
		if msg.Type == MDTrade {
			trade := msg.Data.(*Trade)
			t.Logf("✅ Taker via MarketData: Trade (price=%d, qty=%d)", trade.Price, trade.Qty)
			tradeReceived2 = true
		}
	case <-time.After(100 * time.Millisecond):
	}

	if !tradeReceived1 {
		t.Logf("⚠️  Maker did NOT receive trade via market data")
	}
	if !tradeReceived2 {
		t.Logf("⚠️  Taker did NOT receive trade via market data")
	}

	gateway1.Close()
	gateway2.Close()
}

func TestClientNotificationsOnPartialFill(t *testing.T) {
	ex := NewExchange(10)
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway1 := ex.ConnectClient(1, balances, &FixedFee{})
	gateway2 := ex.ConnectClient(2, balances, &FixedFee{})

	go ex.handleClientRequests(gateway1)
	go ex.handleClientRequests(gateway2)

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI / 2,
		TimeInForce: GTC,
	}
	gateway1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}
	<-gateway1.ResponseCh

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	gateway2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}

	select {
	case resp := <-gateway2.ResponseCh:
		if !resp.Success {
			t.Fatalf("Order should succeed")
		}
		t.Logf("✅ Taker receives: Order ack (orderID=%d)", resp.Data.(uint64))
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout")
	}

	select {
	case <-gateway2.ResponseCh:
		t.Logf("✅ WOULD BE GOOD: Partial fill notification")
	case <-time.After(50 * time.Millisecond):
		t.Logf("❌ MISSING: Taker receives NO partial fill notification")
	}

	select {
	case <-gateway1.ResponseCh:
		t.Logf("✅ WOULD BE GOOD: Maker fill notification")
	case <-time.After(50 * time.Millisecond):
		t.Logf("❌ MISSING: Maker receives NO fill notification")
	}

	gateway1.Close()
	gateway2.Close()
}

func TestClientNotificationsOnReject(t *testing.T) {
	ex := NewExchange(10)
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"USD": 1000}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	go ex.handleClientRequests(gateway)

	orderReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000 * SATOSHI,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: orderReq}

	select {
	case resp := <-gateway.ResponseCh:
		if resp.Success {
			t.Fatalf("Order should be rejected due to insufficient balance")
		}
		t.Logf("✅ CLIENT RECEIVES: Rejection (reason=%v)", resp.Error)
		if resp.Error != RejectInsufficientBalance {
			t.Errorf("Expected RejectInsufficientBalance, got %v", resp.Error)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for rejection")
	}

	gateway.Close()
}
