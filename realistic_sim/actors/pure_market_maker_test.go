package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestPureMarketMakerConfig_Defaults(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.SATOSHI,
		exchange.SATOSHI/1000,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := PureMarketMakerConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		SpreadBps:    20,
		QuoteSize:    exchange.SATOSHI,
		MaxInventory: 10 * exchange.SATOSHI,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.SATOSHI,
		"USD": 1000000 * (exchange.SATOSHI / 1000),
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	pmm := NewPureMarketMaker(1, gateway, config)

	// Check defaults
	if pmm.config.MonitorInterval != 100*time.Millisecond {
		t.Errorf("Expected MonitorInterval 100ms, got %v", pmm.config.MonitorInterval)
	}
	if pmm.config.RequoteThreshold != 5 {
		t.Errorf("Expected RequoteThreshold 5, got %d", pmm.config.RequoteThreshold)
	}
}

func TestPureMarketMakerActor_Creation(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.SATOSHI,
		exchange.SATOSHI/1000,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := PureMarketMakerConfig{
		Symbol:           "BTC/USD",
		Instrument:       inst,
		SpreadBps:        20,
		QuoteSize:        exchange.SATOSHI,
		MaxInventory:     10 * exchange.SATOSHI,
		RequoteThreshold: 10,
		MonitorInterval:  50 * time.Millisecond,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.SATOSHI,
		"USD": 1000000 * (exchange.SATOSHI / 1000),
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	pmm := NewPureMarketMaker(1, gateway, config)

	if pmm.ID() != 1 {
		t.Errorf("Expected ID 1, got %d", pmm.ID())
	}
	if pmm.config.Symbol != "BTC/USD" {
		t.Errorf("Expected symbol BTC/USD, got %s", pmm.config.Symbol)
	}
	if pmm.config.SpreadBps != 20 {
		t.Errorf("Expected SpreadBps 20, got %d", pmm.config.SpreadBps)
	}
}

func TestPureMarketMakerActor_Start(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.SATOSHI,
		exchange.SATOSHI/1000,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := PureMarketMakerConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		SpreadBps:    20,
		QuoteSize:    exchange.SATOSHI,
		MaxInventory: 10 * exchange.SATOSHI,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.SATOSHI,
		"USD": 1000000 * (exchange.SATOSHI / 1000),
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	pmm := NewPureMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := pmm.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start PMM: %v", err)
	}

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	err = pmm.Stop()
	if err != nil {
		t.Fatalf("Failed to stop PMM: %v", err)
	}
}

func TestPureMarketMakerActor_InventoryTracking(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.SATOSHI,
		exchange.SATOSHI/1000,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := PureMarketMakerConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		SpreadBps:    20,
		QuoteSize:    exchange.SATOSHI,
		MaxInventory: 10 * exchange.SATOSHI,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.SATOSHI,
		"USD": 1000000 * (exchange.SATOSHI / 1000),
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	pmm := NewPureMarketMaker(1, gateway, config)

	// Simulate fills
	pmm.onOrderFilled(actor.OrderFillEvent{
		OrderID: 1,
		Qty:     int64(exchange.SATOSHI),
		Price:   50000 * exchange.SATOSHI,
		Side:    exchange.Buy,
		IsFull:  true,
	})

	if pmm.inventory != int64(exchange.SATOSHI) {
		t.Errorf("Expected inventory %d, got %d", exchange.SATOSHI, pmm.inventory)
	}

	pmm.onOrderFilled(actor.OrderFillEvent{
		OrderID: 2,
		Qty:     int64(exchange.SATOSHI / 2),
		Price:   50100 * exchange.SATOSHI,
		Side:    exchange.Sell,
		IsFull:  true,
	})

	expected := int64(exchange.SATOSHI) - int64(exchange.SATOSHI/2)
	if pmm.inventory != expected {
		t.Errorf("Expected inventory %d, got %d", expected, pmm.inventory)
	}
}
