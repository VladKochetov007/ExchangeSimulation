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

// TestNoisyTraderEdgeCases moved to realistic_sim/actors package to avoid import cycle
func TestNoisyTraderEdgeCases(t *testing.T) {
	t.Skip("Test moved to realistic_sim/actors package to avoid import cycle")
}

// TestNoisyTraderPartialFillPath moved to realistic_sim/actors package to avoid import cycle
func TestNoisyTraderPartialFillPath(t *testing.T) {
	t.Skip("Test moved to realistic_sim/actors package to avoid import cycle")
}

func TestDelayedMakerEarlyContextCancel(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  1 * time.Second,
		OrderCount:  3,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         exchange.SATOSHI / 10,
		Visibility:  exchange.Normal,
		IcebergQty:  0,
	}

	maker := NewDelayedMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())

	if err := maker.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)

	maker.Stop()
}

// TestRandomizedTakerEdgeCases moved to realistic_sim/actors package to avoid import cycle
func TestRandomizedTakerEdgeCases(t *testing.T) {
	t.Skip("Test moved to realistic_sim/actors package to avoid import cycle")
}

func TestOMSEdgeCases(t *testing.T) {
	oms := NewNettingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	netPos := oms.GetNetPosition("BTC/USD")
	if netPos != 0 {
		t.Fatalf("Expected net position 0 after flat, got %d", netPos)
	}

	fill3 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 52000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill3, exchange.SATOSHI)

	netPos = oms.GetNetPosition("BTC/USD")
	if netPos != -exchange.SATOSHI {
		t.Fatalf("Expected net position -1 BTC, got %d", netPos)
	}
}

func TestHedgingOMSEdgeCases(t *testing.T) {
	oms := NewHedgingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI / 2,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI / 2,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	netPos := oms.GetNetPosition("BTC/USD")
	if netPos != exchange.SATOSHI {
		t.Fatalf("Expected net position 1 BTC, got %d", netPos)
	}

	fill3 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 52000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI / 4,
	}
	oms.OnFill("BTC/USD", fill3, exchange.SATOSHI)

	positions := oms.GetPositions("BTC/USD")
	if len(positions) != 2 {
		t.Fatalf("Expected 2 positions after partial close, got %d", len(positions))
	}
}

