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

// ZeroFundingCalc returns zero funding rate for simulations testing without funding effects
type ZeroFundingCalc struct{}

func (c *ZeroFundingCalc) Calculate(indexPrice, markPrice int64) int64 {
	return 0
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	startTime := time.Now().UnixNano()
	simClock := simulation.NewSimulatedClock(startTime)

	// Create event scheduler for simulation-time tickers
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)

	// Create ticker factory for simulation mode
	tickerFactory := simulation.NewSimTickerFactory(scheduler)

	clientCount := 100
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients: clientCount,
		Clock:            simClock,
		TickerFactory:    tickerFactory,
		SnapshotInterval: 100 * time.Millisecond,
	})

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC", "USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.CENT_TICK,
		exchange.BTC_PRECISION/10000,
	)
	// Disable funding rates for pure random walk simulation
	perpInst.SetFundingCalculator(&ZeroFundingCalc{})
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
		TickerFactory:       tickerFactory,
	})

	// 3 Simple makers: just mediators for taker flow
	// High inventory limits - makers should absorb all flow without backing away
	// Staggered requote timers - break synchronization for independent price discovery
	// Different EMA decay rates - each maker has different view of fair value
	spreads := []int64{5, 10, 20}
	requoteIntervals := []time.Duration{
		800 * time.Millisecond,  // MM1: 0.8s (fast, tight spread)
		1000 * time.Millisecond, // MM2: 1.0s (medium)
		1200 * time.Millisecond, // MM3: 1.2s (slow, wide spread)
	}
	emaDecays := []float64{
		0.3, // MM1: Fast adaptation (30% weight on new trades)
		0.2, // MM2: Medium adaptation (20% weight)
		0.1, // MM3: Slow adaptation (10% weight)
	}
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
			RequoteInterval: requoteIntervals[i],                // Staggered to prevent synchronized requotes
			BootstrapPrice:  exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK),
			EMADecay:        emaDecays[i], // Different decay rates prevent perfect synchronization
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
		{300 * time.Millisecond, 50, 150},  // Fast, medium-large (always >= 0.5 BTC)
		{400 * time.Millisecond, 60, 180},  // Fast, large
		{500 * time.Millisecond, 70, 200},  // Medium, large
		{600 * time.Millisecond, 80, 220},  // Medium, very large
		{700 * time.Millisecond, 60, 180},  // Slow, large
		{400 * time.Millisecond, 55, 170},  // Fast, medium-large
		{500 * time.Millisecond, 65, 190},  // Medium, large
		{600 * time.Millisecond, 75, 210},  // Medium, very large
		{350 * time.Millisecond, 50, 150},  // Very fast, medium
		{550 * time.Millisecond, 70, 200},  // Medium, large
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

	// Inject ticker factory into all actors before starting them
	for _, a := range allActors {
		// All actors in this sim embed BaseActor, so we can use type assertions
		switch act := a.(type) {
		case *actors.SlowMarketMakerActor:
			act.SetTickerFactory(tickerFactory)
		case *actors.RandomizedTakerActor:
			act.SetTickerFactory(tickerFactory)
		}
	}

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
