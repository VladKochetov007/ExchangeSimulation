package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

var (
	durationFlag = flag.Duration("duration", 25*time.Hour, "simulated duration (0 = infinite)")
	speedupFlag  = flag.Float64("speedup", 0, "sim speedup factor (0 = max CPU tight loop)")
)

func main() {
	flag.Parse()
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
		SnapshotInterval: 5 * time.Second,
	})

	marketConfig := CreateMarketConfig()
	for symbol, inst := range marketConfig.Instruments {
		ex.AddInstrument(inst)
		fmt.Printf("Added instrument: %s\n", symbol)
	}

	logDir := "logs/microstructure_v1"
	fmt.Printf("Log directory: %s\n", logDir)
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

	bootstrapCtx, bootstrapCancel := context.WithCancel(ctx)
	for _, a := range bootstrapActors {
		if err := a.Start(bootstrapCtx); err != nil {
			log.Fatalf("start bootstrap actor: %v", err)
		}
	}

	fmt.Println("Waiting for initial liquidity (2 simulated minutes)...")
	wallStart := time.Now()
	const wallTickInterval = 10 * time.Millisecond
	bootstrapEnd := simClock.NowUnixNano() + int64(2*time.Minute)

	bootstrapTicker := time.NewTicker(wallTickInterval)
	defer bootstrapTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto bootstrapComplete
		case <-bootstrapTicker.C:
			simClock.Advance(time.Duration(float64(wallTickInterval) * 100.0))
			if simClock.NowUnixNano() >= bootstrapEnd {
				goto bootstrapComplete
			}
		}
	}

bootstrapComplete:
	ShutdownBootstrapActors(ex, bootstrapActors)
	bootstrapCancel()
	for _, a := range bootstrapActors {
		a.Stop()
	}
	time.Sleep(5 * time.Millisecond) // let bootstrap goroutines fully exit
	fmt.Println("Bootstrap complete, starting main groups...")
	simStart := simClock.NowUnixNano() // duration counts from here, not from process start

	fmt.Println("=== 8-Group Simulation Started ===")
	factory := NewGroupFactory(ex, marketConfig)
	factory.nextActorID = nextActorID
	factory.nextClientID = nextActorID

	var allGroups []*actor.CompositeActor

	perpMMGroups := factory.CreatePerpMMGroups()
	spotMMGroups := factory.CreateSpotMMGroups()
	spotABCMMGroups := factory.CreateSpotABCMMGroups()
	spotTakerGroups := factory.CreateSpotTakerGroups()
	spotABCTakerGroups := factory.CreateSpotABCTakerGroups()
	perpTakerGroups := factory.CreatePerpTakerGroups()
	fundingArbGroups := factory.CreateFundingArbGroups()
	triangleArbGroups := factory.CreateTriangleArbGroups()

	allGroups = append(allGroups, perpMMGroups...)
	allGroups = append(allGroups, spotMMGroups...)
	allGroups = append(allGroups, spotABCMMGroups...)
	allGroups = append(allGroups, spotTakerGroups...)
	allGroups = append(allGroups, spotABCTakerGroups...)
	allGroups = append(allGroups, perpTakerGroups...)
	allGroups = append(allGroups, fundingArbGroups...)
	allGroups = append(allGroups, triangleArbGroups...)

	fmt.Printf("Total Actors/Groups: %d\n", len(allGroups))

	fmt.Printf("Groups: %d (perp-MM, spot-USD-MM, spot-ABC-MM, spot-USD-taker, spot-ABC-taker, perp-taker, 5×funding-arb, triangle-arb)\n", len(allGroups))
	fmt.Printf("Markets: %d\n", len(marketConfig.Instruments))
	if *durationFlag == 0 {
		fmt.Println("Duration: infinite (Ctrl+C to stop)")
	} else {
		fmt.Printf("Duration: %v simulated\n", *durationFlag)
	}

	for i, group := range allGroups {
		if err := group.Start(ctx); err != nil {
			log.Fatalf("start group %d: %v", i, err)
		}
	}

	simDuration := *durationFlag
	simEndNano := simStart + simDuration.Nanoseconds() // 0 boundary check handled below

	var simCtx context.Context
	var simCancel context.CancelFunc
	if simDuration == 0 || *speedupFlag == 0 {
		// Infinite or tight-loop: context cancelled only on parent cancel or sim-time check
		simCtx, simCancel = context.WithCancel(ctx)
	} else {
		// Fixed speedup: use wall-clock timeout
		wallSimDuration := time.Duration(float64(simDuration) / *speedupFlag)
		simCtx, simCancel = context.WithTimeout(ctx, wallSimDuration)
	}
	defer simCancel()

	lastLogTime := simClock.NowUnixNano()
	const logInterval = int64(5 * time.Minute)

	if *speedupFlag > 0 {
		simStep := time.Duration(float64(wallTickInterval) * *speedupFlag)
		simTicker := time.NewTicker(wallTickInterval)
		defer simTicker.Stop()
		for {
			select {
			case <-simCtx.Done():
				goto shutdown
			case <-simTicker.C:
				simClock.Advance(simStep)
				now := simClock.NowUnixNano()
				if simDuration > 0 && now >= simEndNano {
					goto shutdown
				}
				if now-lastLogTime >= logInterval {
					lastLogTime = now
					printGroupStats(allGroups, time.Duration(now-simStart))
				}
			}
		}
	} else {
		const simStep = 60 * time.Second
		for {
			select {
			case <-simCtx.Done():
				goto shutdown
			default:
				simClock.Advance(simStep)
				time.Sleep(10 * time.Millisecond)
				now := simClock.NowUnixNano()
				if simDuration > 0 && now >= simEndNano {
					goto shutdown
				}
				if now-lastLogTime >= logInterval {
					lastLogTime = now
					printGroupStats(allGroups, time.Duration(now-simStart))
				}
			}
		}
	}

shutdown:
	fmt.Println("\n=== Shutting Down ===")
	cancel()

	wallElapsed := time.Since(wallStart)
	simElapsed := time.Duration(simClock.NowUnixNano() - simStart)
	fmt.Printf("\nWall-clock: %v, Simulated: %v, Speedup: %.2fx\n",
		wallElapsed, simElapsed, float64(simElapsed)/float64(wallElapsed))

	// Log files are written via direct syscalls (no userspace buffer), so os.Exit is safe.
	// Calling automation.Stop() / ex.Shutdown() deadlocks: automation goroutines are stuck
	// waiting on e.mu which Shutdown() also needs. Kill the process cleanly instead.
	generalLogFile.Close()
	for _, f := range symbolLogFiles {
		f.Close()
	}
	os.Exit(0)
}

func printGroupStats(groups []*actor.CompositeActor, elapsed time.Duration) {
	fmt.Printf("\n[%v] Aggregate Balances:\n", elapsed.Round(time.Minute))
	totalUSD := int64(0)
	for _, group := range groups {
		totalUSD += group.GetSharedContext().GetQuoteBalance()
	}
	fmt.Printf("  Total Quote Balance: $%10.2f\n", float64(totalUSD)/float64(USD_PRECISION))
	fmt.Printf("  Active Groups: %d\n", len(groups))
}
