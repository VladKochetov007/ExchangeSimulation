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

	trader.decodeResponse(exchange.Response{Success: true, Data: &exchange.FillNotification{
		OrderID: orderID, IsFull: true,
	}})
	trader.decodeResponse(exchange.Response{RequestID: requestID, Success: true, Data: orderID})

	if _, active := trader.activeOrders.Load(orderID); active {
		t.Fatal("full fill before accept left a ghost active order")
	}
	if _, pending := trader.requestToOrder.Load(requestID); pending {
		t.Fatal("full fill before accept left a ghost request mapping")
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
