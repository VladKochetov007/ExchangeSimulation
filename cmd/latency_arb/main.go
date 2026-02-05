package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("=== Multi-Venue Latency Arbitrage Simulation ===")

	// Create venue registry
	registry := simulation.NewVenueRegistry()

	// Create fast exchange (simulates co-located feed - 1ms latency)
	fastEx := exchange.NewExchange(1000, &simulation.RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	fastEx.AddInstrument(instrument)
	registry.Register("coinbase", fastEx)

	// Create slow exchange (simulates remote feed - 50ms latency)
	slowEx := exchange.NewExchange(1000, &simulation.RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	slowEx.AddInstrument(instrument2)
	registry.Register("binance", slowEx)

	// Initial balances for all participants
	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,      // 100 BTC
		"USD": 10000000 * exchange.USD_PRECISION, // 10 million USD
	}

	// Add market makers to BOTH venues to provide liquidity
	fmt.Println("Setting up market makers on both venues...")

	// Market makers on fast venue (coinbase)
	for i := uint64(100); i <= 101; i++ {
		gateway := fastEx.ConnectClient(i, balances, &exchange.FixedFee{})
		lp := actor.NewFirstLP(i, gateway, actor.FirstLPConfig{
			Symbol:            "BTC/USD",
			SpreadBps:         20,
			LiquidityMultiple: 50,
			BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		})
		lp.SetInitialState(instrument)
		lp.UpdateBalances(balances["BTC"], balances["USD"])

		ctx := context.Background()
		if err := lp.Start(ctx); err != nil {
			return fmt.Errorf("failed to start fast LP: %w", err)
		}
		defer lp.Stop()
	}

	// Market makers on slow venue (binance)
	for i := uint64(200); i <= 201; i++ {
		gateway := slowEx.ConnectClient(i, balances, &exchange.FixedFee{})
		lp := actor.NewFirstLP(i, gateway, actor.FirstLPConfig{
			Symbol:            "BTC/USD",
			SpreadBps:         20,
			LiquidityMultiple: 50,
			BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
		})
		lp.SetInitialState(instrument2)
		lp.UpdateBalances(balances["BTC"], balances["USD"])

		ctx := context.Background()
		if err := lp.Start(ctx); err != nil {
			return fmt.Errorf("failed to start slow LP: %w", err)
		}
		defer lp.Stop()
	}

	// Give market makers time to place orders
	time.Sleep(100 * time.Millisecond)

	// Create multi-venue gateway for arbitrage actor
	arbBalances := map[simulation.VenueID]map[string]int64{
		"coinbase": {
			"BTC": 10 * exchange.BTC_PRECISION,      // 10 BTC
			"USD": 1000000 * exchange.USD_PRECISION, // 1 million USD
		},
		"binance": {
			"BTC": 10 * exchange.BTC_PRECISION,      // 10 BTC
			"USD": 1000000 * exchange.USD_PRECISION, // 1 million USD
		},
	}

	feePlans := map[simulation.VenueID]exchange.FeeModel{
		"coinbase": &exchange.FixedFee{},
		"binance":  &exchange.FixedFee{},
	}

	mgw := simulation.NewMultiVenueGateway(1, registry, arbBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	fmt.Println("Setting up latency arbitrage actor...")

	// Create latency arbitrage actor
	arbActor := simulation.NewLatencyArbitrageActor(1, mgw, simulation.LatencyArbitrageConfig{
		FastVenue:    "coinbase",
		SlowVenue:    "binance",
		Symbol:       "BTC/USD",
		MinProfitBps: 5, // 0.05% minimum profit
		MaxQty:       exchange.BTCAmount(0.1),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := arbActor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start arbitrage actor: %w", err)
	}
	defer arbActor.Stop()

	// Add noisy traders to create price movements on fast venue
	fmt.Println("Adding noisy traders to fast venue to create price movements...")
	for i := uint64(300); i <= 301; i++ {
		gateway := fastEx.ConnectClient(i, balances, &exchange.FixedFee{})
		noisy := actor.NewNoisyTrader(i, gateway, actor.NoisyTraderConfig{
			Symbol:          "BTC/USD",
			Interval:        500 * time.Millisecond,
			PriceRangeBps:   50,
			MinQty:          exchange.BTCAmount(0.01),
			MaxQty:          exchange.BTCAmount(0.05),
			MaxActiveOrders: 2,
			OrderLifetime:   1 * time.Second,
		})

		if err := noisy.Start(ctx); err != nil {
			return fmt.Errorf("failed to start noisy trader: %w", err)
		}
		defer noisy.Stop()
	}

	fmt.Println("\n=== Simulation Running ===")
	fmt.Println("Fast venue (coinbase): ~1ms latency")
	fmt.Println("Slow venue (binance): ~50ms latency (simulated)")
	fmt.Println("Monitoring for arbitrage opportunities...")
	fmt.Println()

	// Wait for simulation to complete
	<-ctx.Done()

	// Print results
	arbitrages, profit := arbActor.Stats()
	fmt.Println("\n=== Simulation Complete ===")
	fmt.Printf("Total arbitrage trades: %d\n", arbitrages)
	fmt.Printf("Total profit (satoshis): %d\n", profit)
	fmt.Printf("Total profit (BTC): %.8f\n", float64(profit)/float64(exchange.BTC_PRECISION))

	// Shutdown exchanges
	fastEx.Shutdown()
	slowEx.Shutdown()

	return nil
}
