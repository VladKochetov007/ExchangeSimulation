package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestMomentumTraderConfig_Defaults(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := MomentumTraderConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		PositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	mt := NewMomentumTrader(1, gateway, config)

	// Check defaults
	if mt.config.MonitorInterval != 1*time.Second {
		t.Errorf("Expected MonitorInterval 1s, got %v", mt.config.MonitorInterval)
	}
	if mt.config.FastWindow != 10 {
		t.Errorf("Expected FastWindow 10, got %d", mt.config.FastWindow)
	}
	if mt.config.SlowWindow != 50 {
		t.Errorf("Expected SlowWindow 50, got %d", mt.config.SlowWindow)
	}
}

func TestMomentumTraderActor_Creation(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := MomentumTraderConfig{
		Symbol:          "BTC/USD",
		Instrument:      inst,
		FastWindow:      5,
		SlowWindow:      20,
		PositionSize:    exchange.BTC_PRECISION,
		MonitorInterval: 500 * time.Millisecond,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	mt := NewMomentumTrader(1, gateway, config)

	if mt.ID() != 1 {
		t.Errorf("Expected ID 1, got %d", mt.ID())
	}
	if mt.config.Symbol != "BTC/USD" {
		t.Errorf("Expected symbol BTC/USD, got %s", mt.config.Symbol)
	}
	if mt.config.FastWindow != 5 {
		t.Errorf("Expected FastWindow 5, got %d", mt.config.FastWindow)
	}
	if mt.config.SlowWindow != 20 {
		t.Errorf("Expected SlowWindow 20, got %d", mt.config.SlowWindow)
	}
}

func TestMomentumTraderActor_Start(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := MomentumTraderConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		FastWindow:   5,
		SlowWindow:   10,
		PositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	mt := NewMomentumTrader(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mt.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start MomentumTrader: %v", err)
	}

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	err = mt.Stop()
	if err != nil {
		t.Fatalf("Failed to stop MomentumTrader: %v", err)
	}
}

func TestMomentumTraderActor_TradeTracking(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := MomentumTraderConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		FastWindow:   3,
		SlowWindow:   5,
		PositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	mt := NewMomentumTrader(1, gateway, config)

	// Simulate some trades to populate buffers
	prices := []int64{
		50000 * exchange.USD_PRECISION,
		50100 * exchange.USD_PRECISION,
		50200 * exchange.USD_PRECISION,
		50300 * exchange.USD_PRECISION,
		50400 * exchange.USD_PRECISION,
	}

	for _, price := range prices {
		mt.onTrade(actor.TradeEvent{
			Symbol: "BTC/USD",
			Trade: &exchange.Trade{
				Price:   price,
				Qty:     exchange.BTC_PRECISION / 10,
				Side:    exchange.Buy,
				TradeID: 1,
			},
		})
	}

	// Check that buffers are populated
	if mt.fastBuffer.Count() != 3 {
		t.Errorf("Expected fastBuffer count 3, got %d", mt.fastBuffer.Count())
	}
	if mt.slowBuffer.Count() != 5 {
		t.Errorf("Expected slowBuffer count 5, got %d", mt.slowBuffer.Count())
	}
}

func TestMomentumTraderActor_SMACalculation(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := MomentumTraderConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		FastWindow:   3,
		SlowWindow:   5,
		PositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	mt := NewMomentumTrader(1, gateway, config)

	// Add known prices
	// Fast SMA (last 3): 50000, 50100, 50200 -> avg = 50100
	// Slow SMA (last 5): 49800, 49900, 50000, 50100, 50200 -> avg = 50000
	prices := []int64{
		49800 * exchange.USD_PRECISION,
		49900 * exchange.USD_PRECISION,
		50000 * exchange.USD_PRECISION,
		50100 * exchange.USD_PRECISION,
		50200 * exchange.USD_PRECISION,
	}

	for _, price := range prices {
		mt.onTrade(actor.TradeEvent{
			Symbol: "BTC/USD",
			Trade: &exchange.Trade{
				Price:   price,
				Qty:     exchange.BTC_PRECISION / 10,
				Side:    exchange.Buy,
				TradeID: 1,
			},
		})
	}

	// Calculate SMAs
	fastSMA := mt.fastBuffer.SMA()
	slowSMA := mt.slowBuffer.SMA()

	// Fast SMA (last 3): 50000, 50100, 50200
	expectedFastSMA := int64((50000*exchange.USD_PRECISION + 50100*exchange.USD_PRECISION + 50200*exchange.USD_PRECISION) / 3)
	if fastSMA != expectedFastSMA {
		t.Errorf("Expected fastSMA %d, got %d", expectedFastSMA, fastSMA)
	}

	// Slow SMA (all 5): 49800, 49900, 50000, 50100, 50200
	expectedSlowSMA := int64((49800*exchange.USD_PRECISION + 49900*exchange.USD_PRECISION +
		50000*exchange.USD_PRECISION + 50100*exchange.USD_PRECISION + 50200*exchange.USD_PRECISION) / 5)
	if slowSMA != expectedSlowSMA {
		t.Errorf("Expected slowSMA %d, got %d", expectedSlowSMA, slowSMA)
	}

	// Fast should be > Slow (uptrend)
	if fastSMA <= slowSMA {
		t.Error("Expected fastSMA > slowSMA for uptrend")
	}
}

func TestMomentumTraderActor_PositionTracking(t *testing.T) {
	inst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := MomentumTraderConfig{
		Symbol:       "BTC/USD",
		Instrument:   inst,
		FastWindow:   3,
		SlowWindow:   5,
		PositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(inst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	mt := NewMomentumTrader(1, gateway, config)

	// Simulate buy fill
	mt.onOrderFilled(actor.OrderFillEvent{
		OrderID: 1,
		Qty:     int64(exchange.BTC_PRECISION),
		Price:   50000 * exchange.USD_PRECISION,
		Side:    exchange.Buy,
		IsFull:  true,
	})

	if mt.position != int64(exchange.BTC_PRECISION) {
		t.Errorf("Expected position %d, got %d", exchange.BTC_PRECISION, mt.position)
	}

	// Simulate sell fill
	mt.onOrderFilled(actor.OrderFillEvent{
		OrderID: 2,
		Qty:     int64(exchange.BTC_PRECISION / 2),
		Price:   50100 * exchange.USD_PRECISION,
		Side:    exchange.Sell,
		IsFull:  true,
	})

	expected := int64(exchange.BTC_PRECISION - exchange.BTC_PRECISION/2)
	if mt.position != expected {
		t.Errorf("Expected position %d, got %d", expected, mt.position)
	}
}
