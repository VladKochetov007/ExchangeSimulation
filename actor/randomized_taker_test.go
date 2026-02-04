package actor

import (
	"context"
	"exchange_sim/exchange"
	"testing"
	"time"
)

func TestRandomizedTakerCreation(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)
	if taker == nil {
		t.Fatal("Expected randomized taker to be created")
	}
	if taker.Config.Symbol != "BTC/USD" {
		t.Fatalf("Expected symbol BTC/USD, got %s", taker.Config.Symbol)
	}
}

func TestRandomizedTakerDefaultConfig(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol: "BTC/USD",
	}

	taker := NewRandomizedTaker(1, gateway, config)
	if taker.Config.Interval != 2*time.Second {
		t.Fatalf("Expected default interval 2s, got %v", taker.Config.Interval)
	}
	if taker.Config.MinQty != exchange.BTCAmount(0.1) {
		t.Fatalf("Expected default MinQty 0.1 BTC, got %d", taker.Config.MinQty)
	}
	if taker.Config.MaxQty != exchange.BTCAmount(1.0) {
		t.Fatalf("Expected default MaxQty 1.0 BTC, got %d", taker.Config.MaxQty)
	}
	if taker.Config.BasePrecision != exchange.SATOSHI {
		t.Fatalf("Expected default BasePrecision SATOSHI, got %d", taker.Config.BasePrecision)
	}
	if taker.Config.QuotePrecision != exchange.SATOSHI/1000 {
		t.Fatalf("Expected default QuotePrecision SATOSHI/1000, got %d", taker.Config.QuotePrecision)
	}
}

func TestRandomizedTakerStart(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	gateway100 := ex.ConnectClient(100, balances, &exchange.FixedFee{})
	actor100 := NewBaseActor(100, gateway100)
	actor100.SubmitOrder("BTC/USD", exchange.Sell, exchange.LimitOrder, exchange.PriceUSD(50000, exchange.DOLLAR_TICK), SATOSHI)
	<-gateway100.ResponseCh

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI / 10,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := taker.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer taker.Stop()

	time.Sleep(250 * time.Millisecond)
}

func TestRandomizedTakerStop(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	taker.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	if err := taker.Stop(); err != nil {
		t.Fatalf("Failed to stop: %v", err)
	}
}

func TestRandomizedTakerSideFlip(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	if taker.side != exchange.Buy {
		t.Fatalf("Expected initial side to be Buy, got %v", taker.side)
	}

	flipped := taker.flipSide(taker.side)
	if flipped != exchange.Sell {
		t.Fatalf("Expected flipped side to be Sell, got %v", flipped)
	}

	flippedAgain := taker.flipSide(flipped)
	if flippedAgain != exchange.Buy {
		t.Fatalf("Expected flipped side to be Buy, got %v", flippedAgain)
	}
}

func TestRandomizedTakerOnEvent(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	event := &Event{
		Type: EventOrderAccepted,
		Data: OrderAcceptedEvent{OrderID: 123, RequestID: 1},
	}
	taker.OnEvent(event)
}

func TestRandomizedTakerExecuteRandomTrade(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI / 10,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	taker.Start(ctx)
	defer taker.Stop()

	taker.executeRandomTrade()
	time.Sleep(20 * time.Millisecond)
}

func TestRandomizedTakerZeroQtyRange(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI / 10,
		MaxQty:         SATOSHI / 10,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	taker.Start(ctx)
	defer taker.Stop()

	time.Sleep(150 * time.Millisecond)
}

func TestRandomizedTakerNegativeQtyRange(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)
	defer ex.Shutdown()

	balances := map[string]int64{"BTC": 10 * SATOSHI, "USD": 100000 * SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := RandomizedTakerConfig{
		Symbol:         "BTC/USD",
		Interval:       100 * time.Millisecond,
		MinQty:         SATOSHI,
		MaxQty:         SATOSHI / 10,
		BasePrecision:  SATOSHI,
		QuotePrecision: SATOSHI / 1000,
	}

	taker := NewRandomizedTaker(1, gateway, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	taker.Start(ctx)
	defer taker.Stop()

	time.Sleep(150 * time.Millisecond)
}
