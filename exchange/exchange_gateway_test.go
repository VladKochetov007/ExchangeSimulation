package exchange

import (
	"testing"
	"time"
)

func TestExchangeCancelOrder(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * BTC_PRECISION, "USD": 100000 * USD_PRECISION}
	gateway := ex.ConnectClient(1, balances, &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if !resp.Success {
		t.Fatalf("Failed to place order")
	}
	orderID := resp.Data.(uint64)

	client := ex.Clients[1]
	reservedBefore := client.GetReserved("USD")
	if reservedBefore == 0 {
		t.Errorf("Expected balance to be reserved")
	}

	cancelReq := &CancelRequest{
		RequestID: 2,
		OrderID:   orderID,
	}
	cancelResp := ex.cancelOrder(1, cancelReq)
	if !cancelResp.Success {
		t.Fatalf("Failed to cancel order")
	}

	reservedAfter := client.GetReserved("USD")
	if reservedAfter != 0 {
		t.Errorf("Expected reserved balance to be released, got %d", reservedAfter)
	}

	gateway.Close()
}

func TestCancelOrderUnknownClient(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	cancelReq := &CancelRequest{RequestID: 1, OrderID: 999}
	resp := ex.cancelOrder(999, cancelReq)
	if resp.Success {
		t.Errorf("Should fail for unknown client")
	}
	if resp.Error != RejectUnknownClient {
		t.Errorf("Expected RejectUnknownClient, got %v", resp.Error)
	}
}

func TestCancelOrderNotFound(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	balances := map[string]int64{"BTC": 10 * BTC_PRECISION}
	ex.ConnectClient(1, balances, &FixedFee{})

	cancelReq := &CancelRequest{RequestID: 1, OrderID: 999}
	resp := ex.cancelOrder(1, cancelReq)
	if resp.Success {
		t.Errorf("Should fail for non-existent order")
	}
}

func TestQueryBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	balances := map[string]int64{
		"BTC": 5 * BTC_PRECISION,
		"USD": 10000 * USD_PRECISION,
	}
	ex.ConnectClient(1, balances, &FixedFee{})

	client := ex.Clients[1]
	client.Reserve("USD", 1000*USD_PRECISION)

	req := &QueryRequest{RequestID: 1}
	resp := ex.queryBalance(1, req)

	if !resp.Success {
		t.Fatalf("Query balance should succeed")
	}

	snapshot := resp.Data.(*BalanceSnapshot)
	if len(snapshot.SpotBalances) != 2 {
		t.Errorf("Expected 2 spot balances, got %d", len(snapshot.SpotBalances))
	}

	var usdBalance *AssetBalance
	for i := range snapshot.SpotBalances {
		if snapshot.SpotBalances[i].Asset == "USD" {
			usdBalance = &snapshot.SpotBalances[i]
			break
		}
	}

	if usdBalance == nil {
		t.Fatalf("USD balance not found")
	}

	if usdBalance.Total != 10000*USD_PRECISION {
		t.Errorf("Expected total 10000 USD_PRECISION, got %d", usdBalance.Total)
	}
	if usdBalance.Reserved != 1000*USD_PRECISION {
		t.Errorf("Expected reserved 1000 USD_PRECISION, got %d", usdBalance.Reserved)
	}
	if usdBalance.Available != 9000*USD_PRECISION {
		t.Errorf("Expected available 9000 USD_PRECISION, got %d", usdBalance.Available)
	}
}

func TestQueryBalanceUnknownClient(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	req := &QueryRequest{RequestID: 1}
	resp := ex.queryBalance(999, req)
	if resp.Success {
		t.Errorf("Should fail for unknown client")
	}
	if resp.Error != RejectUnknownClient {
		t.Errorf("Expected RejectUnknownClient, got %v", resp.Error)
	}
}

func TestSubscribeUnsubscribe(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * BTC_PRECISION}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	req := &QueryRequest{
		RequestID: 1,
		Symbol:    "BTC/USD",
	}
	resp := ex.subscribe(1, req, gateway)
	if !resp.Success {
		t.Fatalf("Subscribe should succeed")
	}

	if len(ex.MDPublisher.subscriptions["BTC/USD"]) != 1 {
		t.Errorf("Expected 1 subscription, got %d", len(ex.MDPublisher.subscriptions["BTC/USD"]))
	}

	unsubReq := &QueryRequest{
		RequestID: 2,
		Symbol:    "BTC/USD",
	}
	unsubResp := ex.unsubscribe(1, unsubReq)
	if !unsubResp.Success {
		t.Fatalf("Unsubscribe should succeed")
	}

	if len(ex.MDPublisher.subscriptions["BTC/USD"]) != 0 {
		t.Errorf("Expected 0 subscriptions, got %d", len(ex.MDPublisher.subscriptions["BTC/USD"]))
	}

	gateway.Close()
}

func TestSubscribeUnknownInstrument(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	balances := map[string]int64{"BTC": 10 * BTC_PRECISION}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	req := &QueryRequest{
		RequestID: 1,
		Symbol:    "UNKNOWN/USD",
	}
	resp := ex.subscribe(1, req, gateway)
	if resp.Success {
		t.Errorf("Should fail for unknown instrument")
	}
	if resp.Error != RejectUnknownInstrument {
		t.Errorf("Expected RejectUnknownInstrument, got %v", resp.Error)
	}

	gateway.Close()
}

func TestHandleClientRequestsIntegration(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 10 * BTC_PRECISION,
		"USD": 100000 * USD_PRECISION,
	}
	gateway := ex.ConnectClient(1, balances, &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true})

	go ex.handleClientRequests(gateway)

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
		if !resp.Success {
			t.Errorf("Order should succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for response")
	}

	queryReq := &QueryRequest{RequestID: 2}
	gateway.RequestCh <- Request{Type: ReqQueryBalance, QueryReq: queryReq}

	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Errorf("Query should succeed")
		}
		snapshot := resp.Data.(*BalanceSnapshot)
		if len(snapshot.SpotBalances) == 0 && len(snapshot.PerpBalances) == 0 {
			t.Errorf("Expected balances in snapshot")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for query response")
	}

	gateway.Close()
}

func TestClientRemoveOrder(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.AddOrder(100)
	client.AddOrder(200)
	client.AddOrder(300)

	if len(client.OrderIDs) != 3 {
		t.Fatalf("Expected 3 orders, got %d", len(client.OrderIDs))
	}

	client.RemoveOrder(200)

	if len(client.OrderIDs) != 2 {
		t.Errorf("Expected 2 orders after removal, got %d", len(client.OrderIDs))
	}

	found := false
	for _, id := range client.OrderIDs {
		if id == 200 {
			found = true
		}
	}
	if found {
		t.Errorf("Order 200 should be removed")
	}
}

func TestShutdown(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	balances := map[string]int64{"BTC": 10 * BTC_PRECISION}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	go ex.handleClientRequests(gateway)

	ex.Shutdown()

	if ex.running {
		t.Errorf("Exchange should not be running after shutdown")
	}

	gateway.Close()
}
