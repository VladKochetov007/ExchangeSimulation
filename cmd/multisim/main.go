package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	// Configure the simulation
	config := simulation.MultiSimConfig{
		Exchanges: []simulation.ExchangeConfig{
			{Name: "binance", LatencyMs: 1},
			{Name: "okx", LatencyMs: 5},
			{Name: "bybit", LatencyMs: 3},
		},
		GlobalAssets:       []string{"BTC", "ETH", "SOL", "XRP", "DOGE"},
		QuoteAsset:         "USD",
		OverlapRatio:       0.6, // 60% of pairs on all exchanges
		SpotToFuturesRatio: 0.5, // 50% spot, 50% perp

		LPsPerSymbol:    2, // 2 FirstLPs per symbol
		MMsPerSymbol:    4, // 4 MarketMakers per symbol (spreads: 5/10/20/30 bps)
		TakersPerSymbol: 1, // 1 taker per symbol

		LPSpreadBps:   20,                      // 20 bps spread
		MMSpreadBps:   10,                      // 10 bps spread
		TakerInterval: 500 * time.Millisecond, // Trade every 500ms

		InitialBalances: map[string]int64{
			"BTC":  50 * exchange.BTC_PRECISION,              // 50 BTC (~$5M @ $100k, ~1.7 BTC per actor)
			"ETH":  1250 * exchange.ETH_PRECISION,            // 1250 ETH (~$5M @ $4k, ~43 ETH per actor)
			"SOL":  25000 * exchange.SATOSHI,                 // 25,000 SOL (~$5M @ $200, ~862 SOL per actor)
			"XRP":  1666666 * exchange.SATOSHI,               // 1.67M XRP (~$5M @ $3, ~57K XRP per actor)
			"DOGE": 12500000 * exchange.SATOSHI,              // 12.5M DOGE (~$5M @ $0.40, ~431K DOGE per actor)
			"USD":  5000000 * exchange.USD_PRECISION,         // 5 million USD (~172K per actor)
		},


		Duration:     10 * time.Second,
		LogDir:       "logs",
		SimSpeedup:   50.0,   // 50x speedup
		LPSkewFactor: 0.0005, // 5 bps per unit inventory skew
	}

	// Create and start the runner
	runner, err := simulation.NewMultiExchangeRunner(config)
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}
	defer runner.Close()

	// Print simulation info
	fmt.Println("=== Multi-Exchange Simulation ===")
	fmt.Printf("Exchanges: %d\n", len(config.Exchanges))
	for _, ex := range config.Exchanges {
		fmt.Printf("  - %s (latency: %dms)\n", ex.Name, ex.LatencyMs)
	}
	fmt.Printf("Assets: %v\n", config.GlobalAssets)
	fmt.Printf("Quote: %s\n", config.QuoteAsset)
	fmt.Printf("Overlap: %.0f%%\n", config.OverlapRatio*100)
	fmt.Printf("Actors: %d total\n", runner.ActorCount())
	fmt.Printf("Duration: %v\n", config.Duration)
	fmt.Printf("Log directory: %s/\n", config.LogDir)
	fmt.Println()
	fmt.Println("Starting simulation...")
	fmt.Println("Press Ctrl+C to stop early")
	fmt.Println()

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Run the simulation
	start := time.Now()
	if err := runner.Run(ctx); err != nil {
		return fmt.Errorf("run simulation: %w", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("\nSimulation completed in %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Logs saved to: %s/\n", config.LogDir)

	return nil
}
