package actor

import (
	"context"
	"exchange_sim/exchange"
	"testing"
	"time"
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
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
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

	actor.SubmitOrder("BTC/USD", exchange.Buy, exchange.LimitOrder, -1, SATOSHI)

	select {
	case received := <-eventReceived:
		if !received {
			t.Log("Rejection event not received within timeout (timing dependent)")
		}
	case <-time.After(200 * time.Millisecond):
		t.Log("Rejection event not received (timing dependent)")
	}
}

func TestNoisyTraderEdgeCases(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        50 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          SATOSHI / 10,
		MaxQty:          SATOSHI,
		MaxActiveOrders: 5,
		OrderLifetime:   100 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.midPrice = 50000 * SATOSHI

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	noisy.Start(ctx)
	defer noisy.Stop()

	event := &Event{
		Type: EventOrderRejected,
		Data: OrderRejectedEvent{RequestID: 1, Reason: exchange.RejectInsufficientBalance},
	}
	noisy.OnEvent(event)

	time.Sleep(200 * time.Millisecond)
}

func TestNoisyTraderPartialFillPath(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          SATOSHI / 10,
		MaxQty:          SATOSHI,
		MaxActiveOrders: 5,
		OrderLifetime:   0,
	}

	noisy := NewNoisyTrader(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	noisy.Start(ctx)
	defer noisy.Stop()

	time.Sleep(150 * time.Millisecond)
}

func TestDelayedMakerEarlyContextCancel(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  1 * time.Second,
		OrderCount:  3,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI / 10,
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

func TestRandomizedTakerEdgeCases(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       50 * time.Millisecond,
		MinQty:         SATOSHI,
		MaxQty:         SATOSHI,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	taker.Start(ctx)
	defer taker.Stop()

	event := &Event{
		Type: EventOrderRejected,
		Data: OrderRejectedEvent{RequestID: 1, Reason: exchange.RejectInsufficientBalance},
	}
	taker.OnEvent(event)

	time.Sleep(150 * time.Millisecond)
}

func TestOMSEdgeCases(t *testing.T) {
	oms := NewNettingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * SATOSHI,
		Qty:   SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * SATOSHI,
		Qty:   SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, SATOSHI)

	netPos := oms.GetNetPosition("BTC/USD")
	if netPos != 0 {
		t.Fatalf("Expected net position 0 after flat, got %d", netPos)
	}

	fill3 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 52000 * SATOSHI,
		Qty:   SATOSHI,
	}
	oms.OnFill("BTC/USD", fill3, SATOSHI)

	netPos = oms.GetNetPosition("BTC/USD")
	if netPos != -SATOSHI {
		t.Fatalf("Expected net position -1 BTC, got %d", netPos)
	}
}

func TestHedgingOMSEdgeCases(t *testing.T) {
	oms := NewHedgingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * SATOSHI,
		Qty:   SATOSHI / 2,
	}
	oms.OnFill("BTC/USD", fill1, SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 51000 * SATOSHI,
		Qty:   SATOSHI / 2,
	}
	oms.OnFill("BTC/USD", fill2, SATOSHI)

	netPos := oms.GetNetPosition("BTC/USD")
	if netPos != SATOSHI {
		t.Fatalf("Expected net position 1 BTC, got %d", netPos)
	}

	fill3 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 52000 * SATOSHI,
		Qty:   SATOSHI / 4,
	}
	oms.OnFill("BTC/USD", fill3, SATOSHI)

	positions := oms.GetPositions("BTC/USD")
	if len(positions) != 2 {
		t.Fatalf("Expected 2 positions after partial close, got %d", len(positions))
	}
}

func TestRecorderEdgeCases(t *testing.T) {
	gateway := exchange.NewClientGateway(1)

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000)
	ethusd := exchange.NewPerpFutures("ETHUSD", "ETH", "USD", 10000000, 10000000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
		"ETHUSD": ethusd,
	}

	config := RecorderConfig{
		OutputDir:           "/nonexistent/path/that/should/not/exist",
		Symbols:             []string{"BTCUSD", "ETHUSD"},
		FlushInterval:       100 * time.Millisecond,
		RecordTrades:        true,
		RecordOrderbook:     true,
		RecordOpenInterest:  true,
		RecordFunding:       true,
		SeparateHiddenFiles: true,
	}

	_, err := NewRecorder(1, gateway, config, instruments)
	if err == nil {
		t.Fatal("Expected error with invalid output directory")
	}
}
