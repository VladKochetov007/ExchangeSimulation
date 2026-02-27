package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
	"time"
)

func TestClientNotificationsOnPlaceOrder(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	gateway := ex.Gateways[1]

	go ex.HandleClientRequests(gateway)

	orderReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: orderReq}

	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Fatalf("Order should succeed")
		}
		t.Logf("CLIENT RECEIVES: Order acknowledgment with orderID=%d", resp.Data.(uint64))
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for response")
	}

	select {
	case <-gateway.ResponseCh:
		t.Errorf("❌ UNEXPECTED: Client should NOT receive execution report for resting order")
	case <-time.After(50 * time.Millisecond):
		t.Logf("CORRECT: No execution report sent (order resting on book)")
	}

	gateway.Close()
}

func TestClientNotificationsOnFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	gateway1 := ex.Gateways[1]
	ex.ConnectClient(2, balances, &FixedFee{})
	gateway2 := ex.Gateways[2]

	go ex.HandleClientRequests(gateway1)
	go ex.HandleClientRequests(gateway2)

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	gateway1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}

	select {
	case resp := <-gateway1.ResponseCh:
		if !resp.Success {
			t.Fatalf("Sell order should succeed")
		}
		t.Logf("MAKER receives: Order ack (orderID=%d)", resp.Data.(uint64))
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout")
	}

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         BTC_PRECISION,
		TimeInForce: IOC,
	}
	gateway2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}

	// Taker expects both Order Ack and Fill Notification
	// Since order corresponds to immediate execution, Fill might arrive before Ack
	receivedAck := false
	receivedFill := false

	timeout := time.After(100 * time.Millisecond)
	for i := 0; i < 2; i++ { // Expecting 2 messages: Ack + Fill
		select {
		case resp := <-gateway2.ResponseCh:
			if resp.RequestID == buyReq.RequestID {
				if !resp.Success {
					t.Fatalf("Buy order should succeed")
				}
				receivedAck = true
				t.Logf("TAKER receives: Order ack (orderID=%d)", resp.Data.(uint64))
			} else if resp.RequestID == 0 {
				if _, ok := resp.Data.(*FillNotification); ok {
					receivedFill = true
					t.Logf("TAKER receives: Fill Notification")
				} else {
					t.Errorf("Received unexpected data with ID 0: %T", resp.Data)
				}
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for taker responses")
		}
	}

	if !receivedAck {
		t.Error("Taker did not receive Order Ack")
	}
	if !receivedFill {
		t.Error("Taker did not receive Fill Notification")
	}

	select {
	case resp := <-gateway1.ResponseCh:
		if _, ok := resp.Data.(*FillNotification); ok {
			t.Logf("MAKER receives: Fill notification")
		} else {
			t.Errorf("MAKER received unexpected data: %T", resp.Data)
		}
	case <-time.After(50 * time.Millisecond):
		t.Errorf("❌ MISSING: Maker received NO fill notification")
	}

	// Taker fill check already done above

	gateway1.Close()
	gateway2.Close()
}

func TestClientNotificationsViaMarketData(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	gateway1 := ex.Gateways[1]
	ex.ConnectClient(2, balances, &FixedFee{})
	gateway2 := ex.Gateways[2]

	go ex.HandleClientRequests(gateway1)
	go ex.HandleClientRequests(gateway2)

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
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	gateway1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}
	<-gateway1.ResponseCh

	buyReq := &OrderRequest{
		RequestID:   4,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         BTC_PRECISION,
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
			t.Logf("Maker via MarketData: Trade (price=%d, qty=%d)", trade.Price, trade.Qty)
			tradeReceived1 = true
		}
	case <-time.After(100 * time.Millisecond):
	}

	select {
	case msg := <-gateway2.MarketData:
		if msg.Type == MDTrade {
			trade := msg.Data.(*Trade)
			t.Logf("Taker via MarketData: Trade (price=%d, qty=%d)", trade.Price, trade.Qty)
			tradeReceived2 = true
		}
	case <-time.After(100 * time.Millisecond):
	}

	if !tradeReceived1 {
		t.Logf("Maker did NOT receive trade via market data")
	}
	if !tradeReceived2 {
		t.Logf("Taker did NOT receive trade via market data")
	}

	gateway1.Close()
	gateway2.Close()
}

func TestClientNotificationsOnPartialFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	gateway1 := ex.Gateways[1]
	ex.ConnectClient(2, balances, &FixedFee{})
	gateway2 := ex.Gateways[2]

	go ex.HandleClientRequests(gateway1)
	go ex.HandleClientRequests(gateway2)

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTC_PRECISION / 2,
		TimeInForce: GTC,
	}
	gateway1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}
	<-gateway1.ResponseCh

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	gateway2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}

	// Taker expects both Order Ack and Partial Fill Notification
	// Since order corresponds to immediate execution (limit buy crossing sell), Fill might arrive before Ack
	receivedAck := false
	receivedFill := false

	timeout := time.After(100 * time.Millisecond)
	for i := 0; i < 2; i++ { // Expecting 2 messages: Ack + Fill
		select {
		case resp := <-gateway2.ResponseCh:
			if resp.RequestID == buyReq.RequestID {
				if !resp.Success {
					t.Fatalf("Order should succeed")
				}
				receivedAck = true
				t.Logf("Taker receives: Order ack (orderID=%d)", resp.Data.(uint64))
			} else if resp.RequestID == 0 {
				if _, ok := resp.Data.(*FillNotification); ok {
					receivedFill = true
					t.Logf("Taker receives: Partial Fill Notification")
				} else {
					t.Errorf("Received unexpected data with ID 0: %T", resp.Data)
				}
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for taker responses")
		}
	}

	if !receivedAck {
		t.Error("Taker did not receive Order Ack")
	}
	if !receivedFill {
		t.Error("Taker did not receive Partial Fill Notification")
	}

	select {
	case <-gateway1.ResponseCh:
		t.Logf("WOULD BE GOOD: Maker fill notification")
	case <-time.After(50 * time.Millisecond):
		t.Logf("❌ MISSING: Maker receives NO fill notification")
	}

	gateway1.Close()
	gateway2.Close()
}

func TestClientNotificationsOnReject(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"USD": 1000}
	ex.ConnectClient(1, balances, &FixedFee{})
	gateway := ex.Gateways[1]

	go ex.HandleClientRequests(gateway)

	orderReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: orderReq}

	select {
	case resp := <-gateway.ResponseCh:
		if resp.Success {
			t.Fatalf("Order should be rejected due to insufficient balance")
		}
		t.Logf("CLIENT RECEIVES: Rejection (reason=%v)", resp.Error)
		if resp.Error != RejectInsufficientBalance {
			t.Errorf("Expected RejectInsufficientBalance, got %v", resp.Error)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for rejection")
	}

	gateway.Close()
}
