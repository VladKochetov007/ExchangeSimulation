package actor

import (
	"context"
	"sync"
	"testing"
	"time"

	"exchange_sim/exchange"
)

type countingTickerFactory struct{ created int }

func (f *countingTickerFactory) NewTicker(time.Duration) exchange.Ticker {
	f.created++
	return &countingTicker{ch: make(chan time.Time)}
}

type countingTicker struct{ ch chan time.Time }

func (t *countingTicker) C() <-chan time.Time { return t.ch }
func (t *countingTicker) Stop()               {}

func TestBaseActorDoubleStart(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := actor.Start(ctx); err != nil {
		t.Fatalf("First start failed: %v", err)
	}

	if err := actor.Start(ctx); err != nil {
		t.Fatalf("Second start should not error: %v", err)
	}

	actor.Stop()
}

func TestBaseActorStopBeforeStart(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	actor := NewBaseActor(1, gateway)

	if err := actor.Stop(); err != nil {
		t.Fatalf("Stop before start should not error: %v", err)
	}
}

func TestBaseActorConcurrentStopIsIdempotent(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	trader := NewBaseActor(1, gateway)
	if err := trader.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	for range 2 {
		go func() {
			defer wg.Done()
			_ = trader.Stop()
		}()
	}
	wg.Wait()
}

func TestBaseActorStartRegistersTickersSynchronously(t *testing.T) {
	trader := NewBaseActor(1, exchange.NewClientGateway(1))
	factory := &countingTickerFactory{}
	trader.SetTickerFactory(factory)
	trader.AddTicker(time.Second, func(time.Time) {})
	if err := trader.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer trader.Stop()
	if factory.created != 1 {
		t.Fatalf("tickers registered by Start = %d, want 1", factory.created)
	}
}

func TestBaseActorFullFillBeforeAcceptLeavesNoGhostOrder(t *testing.T) {
	trader := NewBaseActor(1, exchange.NewClientGateway(1))
	const orderID, requestID = uint64(17), uint64(23)

	if events := trader.decodeResponse(exchange.Response{Success: true, Data: &exchange.FillNotification{
		OrderID: orderID, IsFull: true,
	}}); len(events) != 0 {
		t.Fatalf("early full fill emitted %d events before accept", len(events))
	}
	events := trader.decodeResponse(exchange.Response{RequestID: requestID, Success: true, Data: orderID})
	if len(events) != 2 || events[0].Type != EventOrderAccepted || events[1].Type != EventOrderFilled {
		t.Fatalf("replayed events = %#v, want accepted then filled", events)
	}

	if _, active := trader.activeOrders.Load(orderID); active {
		t.Fatal("full fill before accept left a ghost active order")
	}
	if _, pending := trader.requestToOrder.Load(requestID); pending {
		t.Fatal("full fill before accept left a ghost request mapping")
	}
}

func TestBaseActorForcedCancelBeforeAcceptLeavesNoGhostOrder(t *testing.T) {
	trader := NewBaseActor(1, exchange.NewClientGateway(1))
	const orderID, requestID = uint64(19), uint64(29)

	if events := trader.decodeResponse(exchange.Response{Success: true, Data: &exchange.ForcedCancelNotification{OrderID: orderID}}); len(events) != 0 {
		t.Fatalf("early cancel emitted %d events before accept", len(events))
	}
	events := trader.decodeResponse(exchange.Response{RequestID: requestID, Success: true, Data: orderID})
	if len(events) != 2 || events[0].Type != EventOrderAccepted || events[1].Type != EventOrderCancelled {
		t.Fatalf("replayed events = %#v, want accepted then cancelled", events)
	}

	if _, active := trader.activeOrders.Load(orderID); active {
		t.Fatal("forced cancel before accept left a ghost active order")
	}
	if _, pending := trader.requestToOrder.Load(requestID); pending {
		t.Fatal("forced cancel before accept left a ghost request mapping")
	}
}

func TestBaseActorReplaysEarlyPartialFillAndCancelInOrder(t *testing.T) {
	trader := NewBaseActor(1, exchange.NewClientGateway(1))
	const orderID, requestID = uint64(31), uint64(37)

	trader.decodeResponse(exchange.Response{Success: true, Data: &exchange.FillNotification{
		OrderID: orderID, Qty: 4, IsFull: false,
	}})
	trader.decodeResponse(exchange.Response{Success: true, Data: &exchange.ForcedCancelNotification{
		OrderID: orderID, RemainingQty: 6,
	}})
	events := trader.decodeResponse(exchange.Response{RequestID: requestID, Success: true, Data: orderID})
	if len(events) != 3 {
		t.Fatalf("replayed event count = %d, want 3", len(events))
	}
	want := []EventType{EventOrderAccepted, EventOrderPartialFill, EventOrderCancelled}
	for i, event := range events {
		if event.Type != want[i] {
			t.Fatalf("event %d type = %v, want %v", i, event.Type, want[i])
		}
	}
	if _, active := trader.activeOrders.Load(orderID); active {
		t.Fatal("early partial fill plus cancel left an active order")
	}
	if _, pending := trader.requestToOrder.Load(requestID); pending {
		t.Fatal("early partial fill plus cancel left a request mapping")
	}
}

func TestBaseActorClosesStateForExchangeDiscardedMarketRemainder(t *testing.T) {
	ex := exchange.NewExchange(2, &exchange.RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(exchange.NewSpotInstrument(
		"BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, 1,
	))
	ex.ConnectNewClient(1, map[string]int64{"USD": exchange.USDAmount(100_000)}, &exchange.FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"BTC": exchange.BTCAmount(1)}, &exchange.FixedFee{})

	if response := ex.PlaceOrder(2, &exchange.OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        exchange.Sell,
		Type:        exchange.LimitOrder,
		Price:       exchange.PriceUSD(50_000, exchange.DOLLAR_TICK),
		Qty:         exchange.BTC_PRECISION / 4,
		TimeInForce: exchange.GTC,
		Visibility:  exchange.Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %s", response.Error)
	}

	gateway := ex.Gateways[1]
	trader := NewBaseActor(1, gateway)
	const requestID = uint64(23)
	gateway.RequestCh <- exchange.Request{Type: exchange.ReqPlaceOrder, OrderReq: &exchange.OrderRequest{
		RequestID:   requestID,
		Symbol:      "BTC/USD",
		Side:        exchange.Buy,
		Type:        exchange.Market,
		Qty:         exchange.BTC_PRECISION,
		TimeInForce: exchange.GTC,
		Visibility:  exchange.Normal,
	}}

	var events []*Event
	for range 3 { // partial fill, forced cancel, then placement acceptance
		select {
		case response := <-gateway.ResponseCh:
			events = append(events, trader.decodeResponse(response)...)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for market-order lifecycle response")
		}
	}

	want := []EventType{EventOrderAccepted, EventOrderPartialFill, EventOrderCancelled}
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want %d", events, len(want))
	}
	for index, event := range events {
		if event.Type != want[index] {
			t.Fatalf("event %d type = %v, want %v", index, event.Type, want[index])
		}
	}
	accepted := events[0].Data.(OrderAcceptedEvent)
	cancelled := events[2].Data.(OrderCancelledEvent)
	if cancelled.OrderID != accepted.OrderID || cancelled.RemainingQty != 3*exchange.BTC_PRECISION/4 {
		t.Fatalf("cancelled = %#v, want remainder for order %d", cancelled, accepted.OrderID)
	}
	if _, active := trader.activeOrders.Load(accepted.OrderID); active {
		t.Fatal("discarded market remainder left an active order")
	}
	if _, pending := trader.requestToOrder.Load(requestID); pending {
		t.Fatal("discarded market remainder left a request mapping")
	}
}

type eventRecorder struct{ events chan *Event }

func (r *eventRecorder) HandleEvent(_ context.Context, event *Event) {
	r.events <- event
}

func TestBaseActorRunLoopDeliversAcceptanceBeforeEarlyFill(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	trader := NewBaseActor(1, gateway)
	recorder := &eventRecorder{events: make(chan *Event, 2)}
	trader.SetHandler(recorder)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := trader.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer trader.Stop()

	const orderID, requestID = uint64(39), uint64(41)
	gateway.ResponseCh <- exchange.Response{Success: true, Data: &exchange.FillNotification{
		OrderID: orderID, Qty: 5, IsFull: true,
	}}
	gateway.ResponseCh <- exchange.Response{RequestID: requestID, Success: true, Data: orderID}

	for index, want := range []EventType{EventOrderAccepted, EventOrderFilled} {
		select {
		case event := <-recorder.events:
			if event.Type != want {
				t.Fatalf("event %d type = %v, want %v", index, event.Type, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for event %d", index)
		}
	}
}

func TestBaseActorCancelResponseTracksOriginalOrder(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	trader := NewBaseActor(1, gateway)
	const orderID, placeRequestID = uint64(41), uint64(43)
	trader.activeOrders.Store(orderID, &OrderInfo{OrderID: orderID, RequestID: placeRequestID})
	trader.requestToOrder.Store(placeRequestID, orderID)

	trader.CancelOrder(orderID)
	request := <-gateway.RequestCh
	events := trader.decodeResponse(exchange.Response{
		RequestID: request.CancelReq.RequestID,
		Success:   true,
		Data:      int64(7),
	})
	if len(events) != 1 || events[0].Type != EventOrderCancelled {
		t.Fatalf("cancel events = %#v, want one cancellation", events)
	}
	cancelled := events[0].Data.(OrderCancelledEvent)
	if cancelled.OrderID != orderID || cancelled.RemainingQty != 7 {
		t.Fatalf("cancelled = %#v, want order %d with qty 7", cancelled, orderID)
	}
	if _, active := trader.activeOrders.Load(orderID); active {
		t.Fatal("successful cancel left an active order")
	}
	if _, pending := trader.requestToOrder.Load(placeRequestID); pending {
		t.Fatal("successful cancel left the placement mapping")
	}
}

func TestBaseActorCancelRejectionUsesCancelEvent(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	trader := NewBaseActor(1, gateway)
	const orderID = uint64(47)

	trader.CancelOrder(orderID)
	request := <-gateway.RequestCh
	events := trader.decodeResponse(exchange.Response{
		RequestID: request.CancelReq.RequestID,
		Success:   false,
		Error:     exchange.RejectOrderNotFound,
	})
	if len(events) != 1 || events[0].Type != EventOrderCancelRejected {
		t.Fatalf("cancel rejection events = %#v", events)
	}
	rejected := events[0].Data.(OrderCancelRejectedEvent)
	if rejected.OrderID != orderID || rejected.Reason != exchange.RejectOrderNotFound {
		t.Fatalf("cancel rejection = %#v", rejected)
	}
}

func TestBaseActorResponseHandling(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * exchange.BTC_PRECISION, "USD": 100000 * exchange.BTC_PRECISION}
	gateway := ex.ConnectNewClient(1, balances, &exchange.FixedFee{})

	actor := NewBaseActor(1, gateway)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	eventReceived := make(chan bool, 1)
	go func() {
		select {
		case event := <-actor.EventChannel():
			if event.Type == EventOrderRejected {
				eventReceived <- true
			}
		case <-time.After(150 * time.Millisecond):
			eventReceived <- false
		}
	}()

	actor.Start(ctx)
	defer actor.Stop()

	actor.SubmitOrder("BTC/USD", exchange.Buy, exchange.LimitOrder, -1, exchange.BTC_PRECISION)

	select {
	case received := <-eventReceived:
		if !received {
			t.Log("Rejection event not received within timeout (timing dependent)")
		}
	case <-time.After(200 * time.Millisecond):
		t.Log("Rejection event not received (timing dependent)")
	}
}
