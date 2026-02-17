package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func main() {
	startTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	simClock := simulation.NewSimulatedClock(startTime.UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	tickerFactory := simulation.NewSimTickerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID:               "microstructure_v1",
		EstimatedClients: 20,
		Clock:            simClock,
		TickerFactory:    tickerFactory,
	})

	marketConfig := CreateMarketConfig()
	for symbol, inst := range marketConfig.Instruments {
		ex.AddInstrument(inst)
		fmt.Printf("Added instrument: %s\n", symbol)
	}

	logDir := "logs/microstructure_v1"
	if err := SetupLogDirectories(logDir); err != nil {
		log.Fatalf("setup log directories: %v", err)
	}

	allSymbols := getAllSymbols(marketConfig.Instruments)
	symbolLoggers, generalLogger, generalLogFile, symbolLogFiles, err := CreateLoggers(logDir, allSymbols)
	if err != nil {
		log.Fatalf("create loggers: %v", err)
	}
	defer generalLogFile.Close()
	for _, f := range symbolLogFiles {
		defer f.Close()
	}

	ex.SetLogger("_global", generalLogger)
	for symbol, symLogger := range symbolLoggers {
		ex.SetLogger(symbol, symLogger)
	}

	ex.EnableBalanceSnapshots(10 * time.Second)

	indexProvider := exchange.NewSpotIndexProvider(ex)
	for symbol, inst := range marketConfig.Instruments {
		if inst.IsPerp() {
			indexProvider.MapPerpToSpot(symbol, inst.BaseAsset()+"/"+inst.QuoteAsset())
		}
	}

	automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexProvider,
		PriceUpdateInterval: 3 * time.Second,
		CollateralRate:      10,
		TickerFactory:       tickerFactory,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	automation.Start(ctx)
	defer automation.Stop()

	fmt.Println("\n=== Liquidity Bootstrap Phase ===")
	bootstrapActors, nextActorID := CreateBootstrapActors(ex, marketConfig, 1)
	fmt.Printf("Starting %d bootstrap actors...\n", len(bootstrapActors))

	for _, a := range bootstrapActors {
		if err := a.Start(ctx); err != nil {
			log.Fatalf("start bootstrap actor: %v", err)
		}
	}

	fmt.Println("Waiting for initial liquidity (2 simulated minutes)...")
	wallStart := time.Now()
	simStart := simClock.NowUnixNano()
	bootstrapDuration := 2 * time.Minute
	speedup := 100.0
	wallTickInterval := 10 * time.Millisecond

	wallTicker := time.NewTicker(wallTickInterval)
	defer wallTicker.Stop()

	bootstrapCtx, bootstrapCancel := context.WithTimeout(ctx, bootstrapDuration)
	defer bootstrapCancel()

	for {
		select {
		case <-bootstrapCtx.Done():
			goto bootstrapComplete
		case <-wallTicker.C:
			simClock.Advance(time.Duration(float64(wallTickInterval) * speedup))
		}
	}

bootstrapComplete:
	fmt.Println("Bootstrap complete, starting main groups...")

	fmt.Println("=== 6-Group Simulation Started ===")
	factory := NewGroupFactory(ex, marketConfig)
	factory.nextActorID = nextActorID

	perpMMGroup := factory.CreatePerpMMGroup()
	spotMMGroup := factory.CreateSpotMMGroup()
	spotTakerGroup := factory.CreateSpotTakerGroup()
	perpTakerGroup := factory.CreatePerpTakerGroup()
	fundingArbGroup := factory.CreateFundingArbGroup()
	triangleArbGroup := factory.CreateTriangleArbGroup()

	allGroups := []*actor.CompositeActor{
		perpMMGroup, spotMMGroup, spotTakerGroup,
		perpTakerGroup, fundingArbGroup, triangleArbGroup,
	}

	fmt.Printf("Groups: %d\n", len(allGroups))
	fmt.Printf("Markets: %d\n", len(marketConfig.Instruments))
	fmt.Println("Duration: 24 hours")

	for i, group := range allGroups {
		if err := group.Start(ctx); err != nil {
			log.Fatalf("start group %d: %v", i, err)
		}
	}

	simDuration := 24 * time.Hour
	simTimeStep := 10 * time.Millisecond

	simCtx, simCancel := context.WithTimeout(ctx, simDuration)
	defer simCancel()

	lastLogTime := simClock.NowUnixNano()

	for {
		select {
		case <-simCtx.Done():
			goto shutdown
		case <-wallTicker.C:
			simClock.Advance(simTimeStep)

			if simClock.NowUnixNano()-lastLogTime >= 5*int64(time.Minute) {
				lastLogTime = simClock.NowUnixNano()
				elapsed := time.Duration(simClock.NowUnixNano() - simStart)
				printGroupStats(allGroups, elapsed)
			}
		}
	}

shutdown:
	fmt.Println("\n=== Shutting Down ===")
	for _, a := range bootstrapActors {
		a.Stop()
	}
	for _, group := range allGroups {
		group.Stop()
	}
	ex.Shutdown()

	wallElapsed := time.Since(wallStart)
	simElapsed := time.Duration(simClock.NowUnixNano() - simStart)
	fmt.Printf("\nWall-clock: %v, Simulated: %v, Speedup: %.2fx\n",
		wallElapsed, simElapsed, float64(simElapsed)/float64(wallElapsed))
	fmt.Printf("Log directory: %s\n", logDir)
}

func printGroupStats(groups []*actor.CompositeActor, elapsed time.Duration) {
	fmt.Printf("\n[%v] Group Balances:\n", elapsed.Round(time.Minute))
	groupNames := []string{"PerpMM", "SpotMM", "SpotTaker", "PerpTaker", "FundingArb", "TriangleArb"}
	for i, group := range groups {
		ctx := group.GetSharedContext()
		quoteBalance := ctx.GetQuoteBalance()
		fmt.Printf("  %-12s: Quote=$%10.2f\n",
			groupNames[i], float64(quoteBalance)/float64(USD_PRECISION))
	}
}
