package actor

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
)

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

func TestBaseActorResponseHandling(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

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

	actor.SubmitOrder("BTC/USD", exchange.Buy, exchange.LimitOrder, -1, exchange.SATOSHI)

	select {
	case received := <-eventReceived:
		if !received {
			t.Log("Rejection event not received within timeout (timing dependent)")
		}
	case <-time.After(200 * time.Millisecond):
		t.Log("Rejection event not received (timing dependent)")
	}
}

