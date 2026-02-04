package actor

import (
	"context"
	"exchange_sim/exchange"
	"testing"
	"time"
)

func TestDelayedMakerCreation(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  100 * time.Millisecond,
		OrderCount:  3,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI / 10,
		Visibility:  exchange.Normal,
		IcebergQty:  0,
	}

	maker := NewDelayedMaker(1, gateway, config)
	if maker == nil {
		t.Fatal("Expected delayed maker to be created")
	}
	if maker.Config.Symbol != "BTC/USD" {
		t.Fatalf("Expected symbol BTC/USD, got %s", maker.Config.Symbol)
	}
	if maker.Config.OrderCount != 3 {
		t.Fatalf("Expected order count 3, got %d", maker.Config.OrderCount)
	}
}

func TestDelayedMakerStart(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  50 * time.Millisecond,
		OrderCount:  2,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI / 10,
		Visibility:  exchange.Normal,
		IcebergQty:  0,
	}

	maker := NewDelayedMaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := maker.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer maker.Stop()

	time.Sleep(150 * time.Millisecond)
}

func TestDelayedMakerZeroOrderCount(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  10 * time.Millisecond,
		OrderCount:  0,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI / 10,
		Visibility:  exchange.Normal,
		IcebergQty:  0,
	}

	maker := NewDelayedMaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	maker.Start(ctx)
	defer maker.Stop()

	time.Sleep(50 * time.Millisecond)
}

func TestDelayedMakerContextCancellation(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  200 * time.Millisecond,
		OrderCount:  3,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI / 10,
		Visibility:  exchange.Normal,
		IcebergQty:  0,
	}

	maker := NewDelayedMaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	maker.Start(ctx)
	defer maker.Stop()

	time.Sleep(100 * time.Millisecond)
}

func TestDelayedMakerIcebergOrders(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  10 * time.Millisecond,
		OrderCount:  2,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI,
		Visibility:  exchange.Iceberg,
		IcebergQty:  SATOSHI / 10,
	}

	maker := NewDelayedMaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	maker.Start(ctx)
	defer maker.Stop()

	time.Sleep(50 * time.Millisecond)
}

func TestDelayedMakerOnEvent(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := DelayedMakerConfig{
		Symbol:      "BTC/USD",
		StartDelay:  10 * time.Millisecond,
		OrderCount:  1,
		BasePrice:   exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		PriceSpread: exchange.PriceUSD(100, exchange.DOLLAR_TICK),
		Qty:         SATOSHI / 10,
		Visibility:  exchange.Normal,
		IcebergQty:  0,
	}

	maker := NewDelayedMaker(1, gateway, config)

	event := &Event{
		Type: EventOrderAccepted,
		Data: OrderAcceptedEvent{OrderID: 123, RequestID: 1},
	}
	maker.OnEvent(event)
}
