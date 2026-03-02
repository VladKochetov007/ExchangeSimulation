package exchange_test

import (
	. "exchange_sim/exchange"
	"encoding/json"
	"testing"
)

type testLogger struct {
	events []map[string]any
}

func (t *testLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	entry := map[string]any{
		"sim_time":  simTime,
		"client_id": clientID,
		"event":     eventName,
	}
	if event != nil {
		eventBytes, _ := json.Marshal(event)
		var eventFields map[string]any
		json.Unmarshal(eventBytes, &eventFields)
		for k, v := range eventFields {
			entry[k] = v
		}
	}
	t.events = append(t.events, entry)
}

func TestExchangeLogging(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, 1)

	ex.AddInstrument(btc)

	logger := &testLogger{}
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	ex.ConnectNewClient(1, balances, &FixedFee{})
	ex.ConnectNewClient(2, balances, &FixedFee{})

	gw1 := ex.Gateways[1]
	gw2 := ex.Gateways[2]

	// Place a buy order
	clock.SetTime(1000)
	buyReq := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}
	<-gw1.ResponseCh

	// Verify OrderAccepted and BookDelta were logged
	if len(logger.events) != 2 {
		t.Fatalf("expected 2 events (OrderAccepted + BookDelta), got %d", len(logger.events))
	}
	if logger.events[0]["event"] != "OrderAccepted" {
		t.Errorf("expected OrderAccepted event, got %v", logger.events[0]["event"])
	}
	// Note: OrderAccepted logs the order object, not request details
	if logger.events[0]["order_id"] == nil {
		t.Errorf("expected order_id in OrderAccepted event")
	}

	// Place a sell order that will match
	clock.SetTime(2000)
	sellReq := &OrderRequest{
		RequestID:   2,
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}
	<-gw2.ResponseCh
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	// Should have: OrderAccepted (buy), OrderAccepted (sell), Trade, OrderFill (maker), OrderFill (taker)
	if len(logger.events) < 4 {
		t.Fatalf("expected at least 4 events, got %d", len(logger.events))
	}

	// Find Trade event
	var tradeEvent map[string]any
	for _, evt := range logger.events {
		if evt["event"] == "Trade" {
			tradeEvent = evt
			break
		}
	}

	if tradeEvent == nil {
		t.Fatal("Trade event not found")
	}

	if tradeEvent["price"] != float64(PriceUSD(50000, DOLLAR_TICK)) {
		t.Errorf("expected price=50000, got %v", tradeEvent["price"])
	}
	if tradeEvent["qty"] != float64(BTCAmount(1)) {
		t.Errorf("expected qty=1 BTC, got %v", tradeEvent["qty"])
	}

	// Find OrderFill events
	fillCount := 0
	for _, evt := range logger.events {
		if evt["event"] == "OrderFill" {
			fillCount++
			if evt["role"] == nil {
				t.Error("OrderFill event missing role field")
			}
		}
	}

	if fillCount != 2 {
		t.Errorf("expected 2 OrderFill events, got %d", fillCount)
	}
}

func TestExchangeLoggingRejection(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, 1)

	ex.AddInstrument(btc)

	logger := &testLogger{}
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 0,
		"USD": 0,
	}

	ex.ConnectNewClient(1, balances, &FixedFee{})
	gw1 := ex.Gateways[1]

	// Place order with insufficient balance
	buyReq := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}
	<-gw1.ResponseCh

	if len(logger.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(logger.events))
	}

	if logger.events[0]["event"] != "OrderRejected" {
		t.Errorf("expected OrderRejected event, got %v", logger.events[0]["event"])
	}

	if logger.events[0]["error"] != "INSUFFICIENT_BALANCE" {
		t.Errorf("expected INSUFFICIENT_BALANCE error, got %v", logger.events[0]["error"])
	}
}

func TestExchangeLoggingCancel(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, 1)

	ex.AddInstrument(btc)

	logger := &testLogger{}
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	ex.ConnectNewClient(1, balances, &FixedFee{})
	gw1 := ex.Gateways[1]

	// Place an order
	buyReq := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: buyReq}
	resp := <-gw1.ResponseCh
	orderID := resp.Data.(uint64)

	// Cancel the order
	cancelReq := &CancelRequest{
		RequestID: 2,
		OrderID:   orderID,
	}

	gw1.RequestCh <- Request{Type: ReqCancelOrder, CancelReq: cancelReq}
	<-gw1.ResponseCh

	// Should have: OrderAccepted, BookDelta (post), BookDelta (cancel), OrderCancelled
	if len(logger.events) != 4 {
		t.Fatalf("expected 4 events (OrderAccepted + BookDelta + BookDelta + OrderCancelled), got %d", len(logger.events))
	}

	if logger.events[3]["event"] != "OrderCancelled" {
		t.Errorf("expected OrderCancelled event, got %v", logger.events[3]["event"])
	}

	if logger.events[3]["order_id"] != float64(orderID) {
		t.Errorf("expected order_id=%d, got %v", orderID, logger.events[3]["order_id"])
	}
}
