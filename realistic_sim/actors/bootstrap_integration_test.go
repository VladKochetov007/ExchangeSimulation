package actors_test

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
	"exchange_sim/simulation"
)

func TestBootstrapIntegration(t *testing.T) {
	// 1. Setup simulated environment
	simClock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	tickerFactory := simulation.NewSimTickerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock:         simClock,
		TickerFactory: tickerFactory,
	})

	// 2. Add instrument
	inst := exchange.NewSpotInstrument("ABC/USD", "ABC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.CENT_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	// 3. Setup FirstLP
	lpBalances := map[string]int64{
		"ABC": 100 * exchange.BTC_PRECISION,
		"USD": 10_000_000 * exchange.USD_PRECISION,
	}
	lpGateway := ex.ConnectClient(1, lpBalances, &exchange.FixedFee{})
	lp := actors.NewFirstLP(1, lpGateway, actors.FirstLPConfig{
		Symbol:            "ABC/USD",
		HalfSpreadBps:     50,
		LiquidityMultiple: 5, // Small multiple for faster exit in test
		BootstrapPrice:    50000 * exchange.USD_PRECISION,
		MonitorInterval:   100 * time.Millisecond,
	})
	lp.SetInitialState(inst)
	lp.UpdateBalances(lpBalances["ABC"], lpBalances["USD"])

	// 4. Setup a regular MM that will "take over"
	mmBalances := map[string]int64{
		"ABC": 100 * exchange.BTC_PRECISION,
		"USD": 10_000_000 * exchange.USD_PRECISION,
	}
	mmGateway := ex.ConnectClient(2, mmBalances, &exchange.FixedFee{})
	mm := actor.NewCompositeActor(2, mmGateway, []actor.SubActor{
		actors.NewPureMMSubActor(2, "ABC/USD", actors.PureMMSubActorConfig{
			SpreadBps:      10, // Tighter than FirstLP
			QuoteSize:      exchange.BTC_PRECISION * 10,
			MaxInventory:   50 * exchange.BTC_PRECISION,
			Precision:      exchange.BTC_PRECISION,
			BootstrapPrice: 50000 * exchange.USD_PRECISION,
		}),
	})

	// 5. Setup a Small Taker to facilitate trades
	takerBalances := map[string]int64{
		"USD": 1_000_000 * exchange.USD_PRECISION,
	}
	ex.ConnectClient(3, takerBalances, &exchange.FixedFee{})

	// Start Actors
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lp.Start(ctx)
	
	// Give goroutines some time to process
	time.Sleep(100 * time.Millisecond)
	
	// Advance time to let LP place quotes
	simClock.Advance(1 * time.Second)
	time.Sleep(100 * time.Millisecond)
	
	// Verify LP is in the book
	book := ex.Books["ABC/USD"]
	if book.Bids.Best == nil {
		t.Fatal("FirstLP failed to place bids")
	}
	t.Logf("FirstLP Bid: %d, Qty: %d", book.Bids.Best.Price, book.Bids.Best.TotalQty)

	// Create some exposure for FirstLP so it actually wants to exit
	// Taker buys from LP (LP sells)
	_, err := exchange.InjectMarketOrder(ex, 3, "ABC/USD", exchange.Buy, exchange.BTC_PRECISION)
	if err != 0 {
		t.Fatalf("Failed to create LP exposure: %v", err)
	}
	
	// Advance time and let Fill arrive
	simClock.Advance(500 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)

	lpPos, _ := lp.GetPosition()
	if lpPos >= 0 {
		t.Fatalf("FirstLP should have negative position, got %d", lpPos)
	}
	t.Logf("FirstLP Exposure: %d", lpPos)

	// Now start the regular MM
	mm.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	simClock.Advance(1 * time.Second)
	time.Sleep(100 * time.Millisecond)

	// Verify MM is in the book and providing more liquidity
	t.Logf("Book width: best bid price %d", book.Bids.Best.Price)
	
	// The MM should have tighter spread, so it should be at the top
	if book.Bids.Best.TotalQty < exchange.BTC_PRECISION*5 {
		t.Errorf("MM liquidity too low: %d", book.Bids.Best.TotalQty)
	}

	// Wait for FirstLP to monitor and exit
	simClock.Advance(1 * time.Second)
	
	finalPos, _ := lp.GetPosition()
	if finalPos != 0 {
		t.Errorf("FirstLP failed to exit correctly, remaining position: %d", finalPos)
	} else {
		t.Log("FirstLP successfully exited after regular MM appeared")
	}
}
