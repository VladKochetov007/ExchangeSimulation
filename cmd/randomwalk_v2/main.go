package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/logger"
	"exchange_sim/realistic_sim/actors"
	"exchange_sim/simulation"
)

const (
	simDuration    = 300000 * time.Second // ~83 hours (100x longer)
	speedup        = 100.0                // 100x speedup
	simTimeStep    = 10 * time.Millisecond
	bootstrapPrice = 50000
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	startTime := time.Now().UnixNano()
	simClock := simulation.NewSimulatedClock(startTime)

	clientCount := 100
	ex := exchange.NewExchange(clientCount, simClock)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC", "USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.CENT_TICK,
		exchange.BTC_PRECISION/10000,
	)
	ex.AddInstrument(perpInst)

	logDir := "logs/randomwalk_v2"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	logFilePath := fmt.Sprintf("%s/_global.log", logDir)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	globalLogger := logger.New(logFile)
	ex.SetLogger("_global", globalLogger)
	ex.SetLogger("BTC-PERP", globalLogger)

	ex.EnableBalanceSnapshots(10 * time.Second)

	// CRITICAL: Use fixed index but DISABLE FUNDING (CollateralRate = 0)
	// This eliminates any price anchoring from funding arbitrage
	indexProvider := exchange.NewFixedIndexProvider()
	indexProvider.SetPrice("BTC-PERP", exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK))

	automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexProvider,
		PriceUpdateInterval: 3 * time.Second,
		CollateralRate:      0, // ZERO funding - no anchoring to index price
	})

	// 3 Simple makers: just mediators for taker flow
	// High inventory limits - makers should absorb all flow without backing away
	// 1s requote timer - simple, no memory/state complexity
	spreads := []int64{5, 10, 20} // Tight spreads only
	var marketMakers []actor.Actor

	for i, spreadBps := range spreads {
		mmID := uint64(2 + i)
		mmGateway := ex.ConnectClient(mmID, map[string]int64{}, &exchange.FixedFee{})
		ex.AddPerpBalance(mmID, "USD", 10000000*exchange.USD_PRECISION) // $10M capital

		mm := actors.NewSlowMarketMaker(mmID, mmGateway, actors.SlowMarketMakerConfig{
			Symbol:          "BTC-PERP",
			Instrument:      perpInst,
			SpreadBps:       spreadBps,
			QuoteSize:       50 * exchange.BTC_PRECISION / 100, // 0.5 BTC per level
			MaxInventory:    10000 * exchange.BTC_PRECISION,    // 10,000 BTC - effectively unlimited for perps
			RequoteInterval: 1 * time.Second,                    // 1s simple timer
			BootstrapPrice:  exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK),
		})
		marketMakers = append(marketMakers, mm)
	}

	// 10 takers with diverse seeds - ensures true 50/50 by law of large numbers
	// Varying intervals and sizes for realistic flow
	var takers []actor.Actor

	takerConfigs := []struct {
		interval      time.Duration
		minQty, maxQty int64
	}{
		{300 * time.Millisecond, 20, 80},   // Fast, small
		{400 * time.Millisecond, 30, 100},  // Fast, medium
		{500 * time.Millisecond, 40, 120},  // Medium, medium
		{600 * time.Millisecond, 50, 150},  // Medium, large
		{700 * time.Millisecond, 30, 100},  // Slow, medium
		{400 * time.Millisecond, 25, 90},   // Fast, small-medium
		{500 * time.Millisecond, 35, 110},  // Medium, medium
		{600 * time.Millisecond, 45, 130},  // Medium, large
		{350 * time.Millisecond, 20, 70},   // Very fast, small
		{550 * time.Millisecond, 40, 120},  // Medium, medium
	}

	for i, config := range takerConfigs {
		takerID := uint64(10 + i)
		takerGateway := ex.ConnectClient(takerID, map[string]int64{}, &exchange.FixedFee{})
		ex.AddPerpBalance(takerID, "USD", 3000000*exchange.USD_PRECISION) // $3M capital each

		taker := actors.NewRandomizedTaker(takerID, takerGateway, actors.RandomizedTakerConfig{
			Symbol:         "BTC-PERP",
			Interval:       config.interval,
			MinQty:         config.minQty * exchange.BTC_PRECISION / 100,
			MaxQty:         config.maxQty * exchange.BTC_PRECISION / 100,
			BasePrecision:  exchange.BTC_PRECISION,
			QuotePrecision: exchange.USD_PRECISION,
		})
		takers = append(takers, taker)
	}

	ctx, cancel := context.WithTimeout(context.Background(), simDuration)
	defer cancel()

	allActors := marketMakers
	allActors = append(allActors, takers...)

	automation.Start(ctx)
	defer automation.Stop()

	for _, actor := range allActors {
		if err := actor.Start(ctx); err != nil {
			return fmt.Errorf("start actor: %w", err)
		}
	}

	// Simulation clock advancement with periodic logging
	tickInterval := time.Duration(float64(simTimeStep) / speedup)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	wallStart := time.Now()
	lastLogTime := startTime

	for {
		select {
		case <-ctx.Done():
			goto shutdown
		case <-ticker.C:
			simClock.Advance(simTimeStep)

			// Log simulation progress every 30 simulated seconds
			if simClock.NowUnixNano()-lastLogTime >= 30*int64(time.Second) {
				lastLogTime = simClock.NowUnixNano()
				elapsed := time.Duration(simClock.NowUnixNano() - startTime)

				// Get current mid-price from order book
				book := ex.Books["BTC-PERP"]
				if book != nil && book.Bids.Best != nil && book.Asks.Best != nil {
					bestBid := book.Bids.Best.Price
					bestAsk := book.Asks.Best.Price
					midPrice := (bestBid + bestAsk) / 2
					midPriceUSD := float64(midPrice) / float64(exchange.USD_PRECISION)

					fmt.Printf("[%v] Mid: $%.2f | Bid: $%.2f | Ask: $%.2f | Spread: $%.2f\n",
						elapsed.Round(time.Second),
						midPriceUSD,
						float64(bestBid)/float64(exchange.USD_PRECISION),
						float64(bestAsk)/float64(exchange.USD_PRECISION),
						float64(bestAsk-bestBid)/float64(exchange.USD_PRECISION),
					)
				}
			}
		}
	}

shutdown:
	for _, actor := range allActors {
		actor.Stop()
	}
	ex.Shutdown()

	wallElapsed := time.Since(wallStart)
	simElapsed := time.Duration(simClock.NowUnixNano() - startTime)
	actualSpeedup := float64(simElapsed) / float64(wallElapsed)

	fmt.Printf("\n=== Simulation Complete ===\n")
	fmt.Printf("Wall-clock time: %v\n", wallElapsed)
	fmt.Printf("Simulated time: %v\n", simElapsed.Round(time.Second))
	fmt.Printf("Actual speedup: %.2fx\n", actualSpeedup)
	fmt.Printf("Log file: %s\n", logFilePath)

	return nil
}
