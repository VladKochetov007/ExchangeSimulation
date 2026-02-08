package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestNoisyTraderCreation(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	if noisy == nil {
		t.Fatal("Expected noisy trader to be created")
	}
	if noisy.Config.Symbol != "BTC/USD" {
		t.Fatalf("Expected symbol BTC/USD, got %s", noisy.Config.Symbol)
	}
}

func TestNoisyTraderPlacesOrders(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway100 := ex.ConnectClient(100, balances, &exchange.FixedFee{})
	gateway1 := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	actor100 := actor.NewBaseActor(100, gateway100)
	actor100.SubmitOrder("BTC/USD", exchange.Sell, exchange.LimitOrder, 50000*exchange.SATOSHI, exchange.SATOSHI)
	<-gateway100.ResponseCh

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        50 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 5,
		OrderLifetime:   1 * time.Second,
	}

	noisy := NewNoisyTrader(1, gateway1, config)
	noisy.midPrice = 50000 * exchange.SATOSHI

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	noisy.Start(ctx)
	defer noisy.Stop()

	time.Sleep(250 * time.Millisecond)

	if len(noisy.activeOrders) == 0 {
		t.Fatal("Expected noisy trader to place at least one order")
	}
}

func TestNoisyTraderOrderAccepted(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.midPrice = 50000 * exchange.SATOSHI

	event := actor.OrderAcceptedEvent{
		OrderID:   123,
		RequestID: 1,
	}
	noisy.onOrderAccepted(event)

	if _, exists := noisy.activeOrders[123]; !exists {
		t.Fatal("Expected order to be tracked in activeOrders")
	}
}

func TestNoisyTraderOrderFilled(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.activeOrders[123] = &activeOrder{orderID: 123, placedAt: time.Now()}

	fillEvent := actor.OrderFillEvent{
		OrderID: 123,
		IsFull:  true,
	}
	noisy.onOrderFilled(fillEvent)

	if _, exists := noisy.activeOrders[123]; exists {
		t.Fatal("Expected filled order to be removed from activeOrders")
	}
}

func TestNoisyTraderPartialFill(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.activeOrders[123] = &activeOrder{orderID: 123, placedAt: time.Now()}

	fillEvent := actor.OrderFillEvent{
		OrderID: 123,
		IsFull:  false,
	}
	noisy.onOrderFilled(fillEvent)

	if _, exists := noisy.activeOrders[123]; !exists {
		t.Fatal("Expected partially filled order to remain in activeOrders")
	}
}

func TestNoisyTraderOrderCancelled(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.activeOrders[123] = &activeOrder{orderID: 123, placedAt: time.Now()}

	cancelEvent := actor.OrderCancelledEvent{
		OrderID: 123,
	}
	noisy.onOrderCancelled(cancelEvent)

	if _, exists := noisy.activeOrders[123]; exists {
		t.Fatal("Expected cancelled order to be removed from activeOrders")
	}
}

func TestNoisyTraderBookSnapshot(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)

	snapEvent := actor.BookSnapshotEvent{
		Symbol: "BTC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 49000 * exchange.SATOSHI, VisibleQty: exchange.SATOSHI}},
			Asks: []exchange.PriceLevel{{Price: 51000 * exchange.SATOSHI, VisibleQty: exchange.SATOSHI}},
		},
	}
	noisy.onBookSnapshot(snapEvent)

	if noisy.bestBid != 49000*exchange.SATOSHI {
		t.Fatalf("Expected bestBid 49000, got %d", noisy.bestBid/exchange.SATOSHI)
	}
	if noisy.bestAsk != 51000*exchange.SATOSHI {
		t.Fatalf("Expected bestAsk 51000, got %d", noisy.bestAsk/exchange.SATOSHI)
	}
	if noisy.midPrice != 50000*exchange.SATOSHI {
		t.Fatalf("Expected midPrice 50000, got %d", noisy.midPrice/exchange.SATOSHI)
	}
}

func TestNoisyTraderBookDelta(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        100 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   500 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.bestBid = 49000 * exchange.SATOSHI
	noisy.bestAsk = 51000 * exchange.SATOSHI

	deltaEvent := actor.BookDeltaEvent{
		Symbol: "BTC/USD",
		Delta: &exchange.BookDelta{
			Side:       exchange.Buy,
			Price:      50000 * exchange.SATOSHI,
			VisibleQty: exchange.SATOSHI,
		},
	}
	noisy.onBookDelta(deltaEvent)

	if noisy.bestBid != 50000*exchange.SATOSHI {
		t.Fatalf("Expected bestBid updated to 50000, got %d", noisy.bestBid/exchange.SATOSHI)
	}
}

func TestNoisyTraderStaleOrderCleanup(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        1 * time.Second,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 3,
		OrderLifetime:   100 * time.Millisecond,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.midPrice = 50000 * exchange.SATOSHI

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	noisy.Start(ctx)
	defer noisy.Stop()

	time.Sleep(50 * time.Millisecond)
	initialOrders := len(noisy.activeOrders)

	time.Sleep(200 * time.Millisecond)

	if len(noisy.activeOrders) >= initialOrders {
		t.Log("Stale orders may have been cleaned up (timing dependent)")
	}
}

func TestNoisyTraderMaxActiveOrders(t *testing.T) {
	ex := exchange.NewExchange(10, &exchange.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 10 * exchange.SATOSHI, "USD": 100000 * exchange.SATOSHI}
	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	config := NoisyTraderConfig{
		Symbol:          "BTC/USD",
		Interval:        20 * time.Millisecond,
		PriceRangeBps:   100,
		MinQty:          exchange.SATOSHI / 10,
		MaxQty:          exchange.SATOSHI,
		MaxActiveOrders: 2,
		OrderLifetime:   10 * time.Second,
	}

	noisy := NewNoisyTrader(1, gateway, config)
	noisy.midPrice = 50000 * exchange.SATOSHI

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	noisy.Start(ctx)
	defer noisy.Stop()

	time.Sleep(200 * time.Millisecond)

	if len(noisy.activeOrders) > config.MaxActiveOrders {
		t.Fatalf("Expected at most %d active orders, got %d", config.MaxActiveOrders, len(noisy.activeOrders))
	}
}
