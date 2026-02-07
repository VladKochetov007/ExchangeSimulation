package exchange

import (
	"bytes"
	"encoding/json"
	"testing"
)

type bufLogger struct {
	buf    *bytes.Buffer
	events []map[string]any
}

func newBufLogger() *bufLogger {
	return &bufLogger{
		buf:    &bytes.Buffer{},
		events: make([]map[string]any, 0),
	}
}

func (l *bufLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
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

	l.events = append(l.events, entry)
	line, _ := json.Marshal(entry)
	l.buf.Write(line)
	l.buf.Write([]byte("\n"))
}

func (l *bufLogger) getEventsByType(eventType string) []map[string]any {
	result := make([]map[string]any, 0)
	for _, evt := range l.events {
		if evt["event"].(string) == eventType {
			result = append(result, evt)
		}
	}
	return result
}

func TestFullOrderLifecycleLogging(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, SATOSHI)

	ex.AddInstrument(btc)

	logger := newBufLogger()
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	gw1 := ex.Gateways[1]
	gw2 := ex.Gateways[2]

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
	resp := <-gw1.ResponseCh
	orderID := resp.Data.(uint64)

	accepted := logger.getEventsByType("OrderAccepted")
	if len(accepted) != 1 {
		t.Fatalf("expected 1 OrderAccepted event, got %d", len(accepted))
	}

	clock.SetTime(2000)
	sellReq := &OrderRequest{
		RequestID:   2,
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(0.5),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: sellReq}
	<-gw2.ResponseCh
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	trades := logger.getEventsByType("Trade")
	if len(trades) != 1 {
		t.Fatalf("expected 1 Trade event, got %d", len(trades))
	}

	fills := logger.getEventsByType("OrderFill")
	if len(fills) != 2 {
		t.Fatalf("expected 2 OrderFill events, got %d", len(fills))
	}

	var makerFill, takerFill map[string]any
	for _, fill := range fills {
		if fill["role"].(string) == "maker" {
			makerFill = fill
		} else {
			takerFill = fill
		}
	}

	if makerFill == nil || takerFill == nil {
		t.Fatal("missing maker or taker fill event")
	}

	if makerFill["order_id"].(float64) != float64(orderID) {
		t.Errorf("maker fill order_id mismatch")
	}

	clock.SetTime(3000)
	cancelReq := &CancelRequest{
		RequestID: 3,
		OrderID:   orderID,
	}

	gw1.RequestCh <- Request{Type: ReqCancelOrder, CancelReq: cancelReq}
	<-gw1.ResponseCh

	cancelled := logger.getEventsByType("OrderCancelled")
	if len(cancelled) != 1 {
		t.Fatalf("expected 1 OrderCancelled event, got %d", len(cancelled))
	}
}

func TestMarketOrderLogging(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, SATOSHI)

	ex.AddInstrument(btc)

	logger := newBufLogger()
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	gw1 := ex.Gateways[1]
	gw2 := ex.Gateways[2]

	limitReq := &OrderRequest{
		RequestID:   1,
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: limitReq}
	<-gw1.ResponseCh

	marketReq := &OrderRequest{
		RequestID:   2,
		Side:        Buy,
		Type:        Market,
		Qty:         BTCAmount(0.5),
		Symbol:      "BTCUSD",
		TimeInForce: IOC,
		Visibility:  Normal,
	}

	gw2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: marketReq}
	<-gw2.ResponseCh
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	accepted := logger.getEventsByType("OrderAccepted")
	if len(accepted) != 2 {
		t.Fatalf("expected 2 OrderAccepted events, got %d", len(accepted))
	}

	trades := logger.getEventsByType("Trade")
	if len(trades) != 1 {
		t.Fatalf("expected 1 Trade event, got %d", len(trades))
	}
}

func TestIcebergOrderLogging(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, SATOSHI)

	ex.AddInstrument(btc)

	logger := newBufLogger()
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	ex.ConnectClient(1, balances, &FixedFee{})
	gw1 := ex.Gateways[1]

	icebergReq := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(10),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Iceberg,
		IcebergQty:  BTCAmount(1),
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: icebergReq}
	<-gw1.ResponseCh

	accepted := logger.getEventsByType("OrderAccepted")
	if len(accepted) != 1 {
		t.Fatalf("expected 1 OrderAccepted event, got %d", len(accepted))
	}

	evt := accepted[0]
	if evt["visibility"].(string) != "ICEBERG" {
		t.Errorf("expected visibility=ICEBERG, got %v", evt["visibility"])
	}

	if evt["iceberg_qty"].(float64) != float64(BTCAmount(1)) {
		t.Errorf("expected iceberg_qty=%d, got %v", BTCAmount(1), evt["iceberg_qty"])
	}
}

func TestAllRejectReasonsLogged(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, SATOSHI)

	ex.AddInstrument(btc)

	logger := newBufLogger()
	ex.SetLogger("BTCUSD", logger)

	balances := map[string]int64{
		"BTC": 0,
		"USD": 0,
	}

	ex.ConnectClient(1, balances, &FixedFee{})
	gw1 := ex.Gateways[1]

	insufficientReq := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: insufficientReq}
	<-gw1.ResponseCh

	rejected := logger.getEventsByType("OrderRejected")
	if len(rejected) == 0 {
		t.Fatal("expected at least 1 OrderRejected event")
	}

	evt := rejected[0]
	if evt["error"].(string) != "INSUFFICIENT_BALANCE" {
		t.Errorf("expected error=INSUFFICIENT_BALANCE, got %v", evt["error"])
	}

	ex.Clients[1].Balances["USD"] = 1000000 * USD_PRECISION

	invalidPriceReq := &OrderRequest{
		RequestID:   2,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       123,
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: invalidPriceReq}
	<-gw1.ResponseCh

	rejected = logger.getEventsByType("OrderRejected")
	found := false
	for _, evt := range rejected {
		if evt["error"].(string) == "INVALID_PRICE" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected INVALID_PRICE rejection")
	}
}

func TestMultipleSymbolsLogging(t *testing.T) {
	clock := &testClock{}
	ex := NewExchange(10, clock)

	btc := NewSpotInstrument("BTCUSD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION,
		DOLLAR_TICK, SATOSHI)

	eth := NewSpotInstrument("ETHUSD", "ETH", "USD",
		ETH_PRECISION, USD_PRECISION,
		ETH_PRECISION/100, ETH_PRECISION/1000)

	ex.AddInstrument(btc)
	ex.AddInstrument(eth)

	btcLogger := newBufLogger()
	ethLogger := newBufLogger()

	ex.SetLogger("BTCUSD", btcLogger)
	ex.SetLogger("ETHUSD", ethLogger)

	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"ETH": 1000 * ETH_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}

	ex.ConnectClient(1, balances, &FixedFee{})
	gw1 := ex.Gateways[1]

	btcReq := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, DOLLAR_TICK),
		Qty:         BTCAmount(1),
		Symbol:      "BTCUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: btcReq}
	<-gw1.ResponseCh

	ethReq := &OrderRequest{
		RequestID:   2,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       3000 * ETH_PRECISION,
		Qty:         10 * ETH_PRECISION,
		Symbol:      "ETHUSD",
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: ethReq}
	<-gw1.ResponseCh

	if len(btcLogger.events) != 1 {
		t.Errorf("expected 1 BTC event, got %d", len(btcLogger.events))
	}

	if len(ethLogger.events) != 1 {
		t.Errorf("expected 1 ETH event, got %d", len(ethLogger.events))
	}

	if btcLogger.events[0]["symbol"].(string) != "BTCUSD" {
		t.Error("BTC logger got wrong symbol")
	}

	if ethLogger.events[0]["symbol"].(string) != "ETHUSD" {
		t.Error("ETH logger got wrong symbol")
	}
}
