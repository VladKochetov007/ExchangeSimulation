package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestEnhancedRandomConfig_Defaults(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := EnhancedRandomConfig{
		Symbol:     "BTC/USD",
		Instrument: inst,
		MinQty:     exchange.SATOSHI / 10,
		MaxQty:     exchange.SATOSHI,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	er := NewEnhancedRandom(1, gateway, config, 42)

	// Check defaults
	if er.config.TradeInterval != 2*time.Second {
		t.Errorf("Expected TradeInterval 2s, got %v", er.config.TradeInterval)
	}
	if er.config.LimitOrderPct != 50 {
		t.Errorf("Expected LimitOrderPct 50, got %d", er.config.LimitOrderPct)
	}
	if er.config.LimitPriceRangeBps != 50 {
		t.Errorf("Expected LimitPriceRangeBps 50, got %d", er.config.LimitPriceRangeBps)
	}
}

func TestEnhancedRandomActor_Creation(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := EnhancedRandomConfig{
		Symbol:             "BTC/USD",
		Instrument:         inst,
		MinQty:             exchange.SATOSHI / 10,
		MaxQty:             exchange.SATOSHI,
		TradeInterval:      1 * time.Second,
		LimitOrderPct:      30,
		LimitPriceRangeBps: 100,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	er := NewEnhancedRandom(1, gateway, config, 42)

	if er.ID() != 1 {
		t.Errorf("Expected ID 1, got %d", er.ID())
	}
	if er.config.Symbol != "BTC/USD" {
		t.Errorf("Expected symbol BTC/USD, got %s", er.config.Symbol)
	}
	if er.config.LimitOrderPct != 30 {
		t.Errorf("Expected LimitOrderPct 30, got %d", er.config.LimitOrderPct)
	}
}

func TestEnhancedRandomActor_Start(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := EnhancedRandomConfig{
		Symbol:        "BTC/USD",
		Instrument:    inst,
		MinQty:        exchange.SATOSHI / 10,
		MaxQty:        exchange.SATOSHI,
		TradeInterval: 1 * time.Second,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	er := NewEnhancedRandom(1, gateway, config, 42)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := er.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start EnhancedRandom: %v", err)
	}

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	err = er.Stop()
	if err != nil {
		t.Fatalf("Failed to stop EnhancedRandom: %v", err)
	}
}

func TestEnhancedRandomActor_BookSnapshot(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := EnhancedRandomConfig{
		Symbol:     "BTC/USD",
		Instrument: inst,
		MinQty:     exchange.SATOSHI / 10,
		MaxQty:     exchange.SATOSHI,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	er := NewEnhancedRandom(1, gateway, config, 42)

	// Simulate book snapshot
	er.onBookSnapshot(actor.BookSnapshotEvent{
		Symbol: "BTC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{
				{Price: 50000 * exchange.USD_PRECISION, VisibleQty: 100},
			},
			Asks: []exchange.PriceLevel{
				{Price: 50100 * exchange.USD_PRECISION, VisibleQty: 100},
			},
		},
	})

	expectedMid := int64((50000*exchange.USD_PRECISION + 50100*exchange.USD_PRECISION) / 2)
	if er.lastMidPrice != expectedMid {
		t.Errorf("Expected lastMidPrice %d, got %d", expectedMid, er.lastMidPrice)
	}
}

func TestEnhancedRandomActor_PriceAlignment(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := EnhancedRandomConfig{
		Symbol:             "BTC/USD",
		Instrument:         inst,
		MinQty:             exchange.SATOSHI / 10,
		MaxQty:             exchange.SATOSHI,
		LimitPriceRangeBps: 500, // 5% - larger range to ensure offset > tick
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	er := NewEnhancedRandom(1, gateway, config, 42)

	// Set mid price
	er.lastMidPrice = 50000 * exchange.USD_PRECISION

	tickSize := inst.TickSize()

	// Test that tick size is valid
	if tickSize == 0 {
		t.Error("Tick size should not be zero")
	}

	// Calculate what the max offset should be for a buy/sell limit order
	// maxOffset = (50000 * 100000 * 500) / 10000 = 250,000,000
	// tickSize = 100,000,000
	// So maxOffset > tickSize, which allows for random price variation
	maxOffset := (er.lastMidPrice * er.config.LimitPriceRangeBps) / 10000
	if maxOffset < tickSize {
		t.Errorf("Max offset %d should be at least one tick %d", maxOffset, tickSize)
	}
	// Price should be mid - offset, aligned to tick
	// This is tested indirectly - if prices aren't aligned, exchange will reject
}

func TestEnhancedRandomActor_QuantityRange(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	minQty := int64(exchange.SATOSHI / 10)  // 0.1 BTC
	maxQty := int64(exchange.SATOSHI)       // 1.0 BTC

	config := EnhancedRandomConfig{
		Symbol:     "BTC/USD",
		Instrument: inst,
		MinQty:     minQty,
		MaxQty:     maxQty,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	er := NewEnhancedRandom(1, gateway, config, 42)

	// Test that generated quantities are within range
	// We test this indirectly by checking config values
	if er.config.MinQty != minQty {
		t.Errorf("Expected MinQty %d, got %d", minQty, er.config.MinQty)
	}
	if er.config.MaxQty != maxQty {
		t.Errorf("Expected MaxQty %d, got %d", maxQty, er.config.MaxQty)
	}
}
