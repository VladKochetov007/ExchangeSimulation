package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
	"exchange_sim/realistic_sim/lifecycle"
	"exchange_sim/simulation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := defaultConfig()

	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	startNano := int64(1_700_000_000) * 1e9
	clock := simulation.NewSimulatedClock(startNano)

	ex := exchange.NewExchange(256, clock)
	ex.EnablePeriodicSnapshots(cfg.SnapshotInterval)

	instruments := setupInstruments(ex, cfg)

	indexProvider := exchange.NewSpotIndexProvider(ex)
	for _, pair := range cfg.FundingArbPairs {
		indexProvider.MapPerpToSpot(pair.PerpSymbol, pair.SpotSymbol)
	}

	automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexProvider,
		PriceUpdateInterval: 3 * time.Second,
	})

	lm := lifecycle.NewLifecycleManager()

	nextID := uint64(1)

	for _, inst := range instruments {
		for range cfg.FirstLPPerSymbol {
			id := nextID
			nextID++
			gw := connectWithPerp(ex, id, cfg.FirstLPBalance, inst)
			lp := createFirstLP(id, gw, inst, cfg)
			lm.RegisterActor(lp, &lifecycle.AlwaysSatisfied{})
		}
	}

	for _, inst := range instruments {
		for i, spreadBps := range cfg.PureMMSpreads {
			if i >= cfg.PureMMPerSymbol {
				break
			}
			id := nextID
			nextID++
			gw := connectWithPerp(ex, id, cfg.PureMMBalance, inst)
			mm := createPureMM(id, gw, inst, spreadBps)
			cond := lifecycle.NewLiquiditySufficientCondition(ex, inst.Symbol(), inst.MinOrderSize(), inst.MinOrderSize())
			lm.RegisterActor(mm, cond)
		}
	}

	for _, inst := range instruments {
		for range cfg.StoikovPerSymbol {
			id := nextID
			nextID++
			gw := connectWithPerp(ex, id, cfg.StoikovBalance, inst)
			as := createStoikov(id, gw, inst)
			cond := lifecycle.NewLiquiditySufficientCondition(ex, inst.Symbol(), inst.MinOrderSize(), inst.MinOrderSize())
			lm.RegisterActor(as, cond)
		}
	}

	for _, pair := range cfg.FundingArbPairs {
		spotInst := findInstrument(instruments, pair.SpotSymbol)
		perpInst := findInstrument(instruments, pair.PerpSymbol)
		if spotInst == nil || perpInst == nil {
			continue
		}
		id := nextID
		nextID++
		gw := ex.ConnectClient(id, cfg.FundingArbBalance, &exchange.FixedFee{})
		quote := spotInst.QuoteAsset()
		if amt, ok := cfg.FundingArbBalance[quote]; ok {
			ex.AddPerpBalance(id, quote, amt)
		}
		arb := createFundingArb(id, gw, pair, spotInst, perpInst)
		spotCond := lifecycle.NewLiquiditySufficientCondition(ex, pair.SpotSymbol, spotInst.MinOrderSize(), spotInst.MinOrderSize())
		perpCond := lifecycle.NewLiquiditySufficientCondition(ex, pair.PerpSymbol, perpInst.MinOrderSize(), perpInst.MinOrderSize())
		cond := lifecycle.NewCompositeCondition(true, spotCond, perpCond)
		lm.RegisterActor(arb, cond)
	}

	for i, inst := range instruments {
		for j := range cfg.RandomTakerPerSymbol {
			id := nextID
			nextID++
			seed := int64(id)*31 + int64(i)*17 + int64(j)*7
			gw := connectWithPerp(ex, id, cfg.TakerBalance, inst)
			taker := createRandomTaker(id, gw, inst, seed)
			cond := lifecycle.NewLiquiditySufficientCondition(ex, inst.Symbol(), inst.MinOrderSize(), inst.MinOrderSize())
			lm.RegisterActor(taker, cond)
		}
	}

	fmt.Printf("=== Industrial Exchange Simulation ===\n")
	fmt.Printf("Instruments: %d\n", len(instruments))
	for _, inst := range instruments {
		fmt.Printf("  %s (%s)\n", inst.Symbol(), inst.InstrumentType())
	}
	fmt.Printf("Actors: %d\n", int(nextID-1))
	fmt.Printf("Duration: %v sim time (%.0fx speedup = %v real)\n",
		cfg.Duration, cfg.Speedup,
		time.Duration(float64(cfg.Duration)/cfg.Speedup).Round(time.Second))
	fmt.Printf("Log directory: %s\n", cfg.LogDir)
	fmt.Println()
	fmt.Println("Starting simulation... (Ctrl+C to stop)")
	fmt.Println()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down gracefully...")
		cancel()
	}()

	defer ex.Shutdown()

	automation.Start(ctx)
	defer automation.Stop()

	defer lm.StopAll()

	realInterval := 10 * time.Millisecond
	simStep := time.Duration(float64(realInterval) * cfg.Speedup)
	endSimTime := startNano + int64(cfg.Duration)

	ticker := time.NewTicker(realInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Stopped by signal.")
			return nil
		case <-ticker.C:
			clock.Advance(simStep)
			lm.CheckAndStart(ctx)
			if clock.NowUnixNano() >= endSimTime {
				fmt.Printf("Simulation complete. %.1f days simulated.\n", float64(cfg.Duration)/float64(24*time.Hour))
				return nil
			}
		}
	}
}

func setupInstruments(ex *exchange.Exchange, cfg IndustrialConfig) []exchange.Instrument {
	result := make([]exchange.Instrument, 0, len(cfg.Instruments))
	for _, spec := range cfg.Instruments {
		var inst exchange.Instrument
		if spec.IsPerp {
			inst = exchange.NewPerpFutures(
				spec.Symbol, spec.Base, spec.Quote,
				spec.BasePrecision, spec.QuotePrecision,
				spec.TickSize, spec.MinOrderSize,
			)
		} else {
			inst = exchange.NewSpotInstrument(
				spec.Symbol, spec.Base, spec.Quote,
				spec.BasePrecision, spec.QuotePrecision,
				spec.TickSize, spec.MinOrderSize,
			)
		}
		ex.AddInstrument(inst)
		result = append(result, inst)
	}
	return result
}

func findInstrument(instruments []exchange.Instrument, symbol string) exchange.Instrument {
	for _, inst := range instruments {
		if inst.Symbol() == symbol {
			return inst
		}
	}
	return nil
}

func connectWithPerp(ex *exchange.Exchange, id uint64, balances map[string]int64, inst exchange.Instrument) *exchange.ClientGateway {
	gw := ex.ConnectClient(id, balances, &exchange.FixedFee{})
	if inst.IsPerp() {
		quote := inst.QuoteAsset()
		if amt, ok := balances[quote]; ok {
			ex.AddPerpBalance(id, quote, amt)
		}
	}
	return gw
}

func createFirstLP(id uint64, gw *exchange.ClientGateway, inst exchange.Instrument, cfg IndustrialConfig) *actors.FirstLiquidityProvidingActor {
	bootstrapPrice := findBootstrapPrice(cfg.Instruments, inst.Symbol())
	lp := actors.NewFirstLP(id, gw, actors.FirstLPConfig{
		Symbol:          inst.Symbol(),
		HalfSpreadBps:   75,
		MonitorInterval: 50 * time.Millisecond,
		BootstrapPrice:  bootstrapPrice,
	})
	lp.SetInitialState(inst)
	lp.UpdateBalances(cfg.FirstLPBalance[inst.BaseAsset()], cfg.FirstLPBalance[inst.QuoteAsset()])
	return lp
}

func findBootstrapPrice(specs []InstrumentSpec, symbol string) int64 {
	for _, s := range specs {
		if s.Symbol == symbol {
			return s.BootstrapPrice
		}
	}
	return 0
}

func createPureMM(id uint64, gw *exchange.ClientGateway, inst exchange.Instrument, spreadBps int64) actor.Actor {
	return actors.NewPureMarketMaker(id, gw, actors.PureMarketMakerConfig{
		Symbol:           inst.Symbol(),
		Instrument:       inst,
		SpreadBps:        spreadBps,
		QuoteSize:        inst.BasePrecision(),
		MaxInventory:     10 * inst.BasePrecision(),
		RequoteThreshold: 5,
		MonitorInterval:  100 * time.Millisecond,
	})
}

func createStoikov(id uint64, gw *exchange.ClientGateway, inst exchange.Instrument) actor.Actor {
	return actors.NewAvellanedaStoikov(id, gw, actors.AvellanedaStoikovConfig{
		Symbol:           inst.Symbol(),
		Gamma:            5000,
		K:                10000,
		T:                3600,
		QuoteQty:         inst.BasePrecision(),
		MaxInventory:     5 * inst.BasePrecision(),
		VolatilityWindow: 20,
		RequoteInterval:  500 * time.Millisecond,
	})
}

func createFundingArb(id uint64, gw *exchange.ClientGateway, pair FundingArbPairSpec, spotInst, perpInst exchange.Instrument) actor.Actor {
	return actors.NewFundingArbitrage(id, gw, actors.FundingArbConfig{
		SpotSymbol:         pair.SpotSymbol,
		PerpSymbol:         pair.PerpSymbol,
		SpotInstrument:     spotInst,
		PerpInstrument:     perpInst,
		MinFundingRate:     25,
		ExitFundingRate:    5,
		HedgeRatio:         10000,
		MaxPositionSize:    10 * spotInst.BasePrecision(),
		MonitorInterval:    1 * time.Second,
		RebalanceThreshold: 100,
	})
}

func createRandomTaker(id uint64, gw *exchange.ClientGateway, inst exchange.Instrument, seed int64) actor.Actor {
	return actors.NewEnhancedRandom(id, gw, actors.EnhancedRandomConfig{
		Symbol:             inst.Symbol(),
		Instrument:         inst,
		MinQty:             inst.MinOrderSize(),
		MaxQty:             inst.BasePrecision(),
		TradeInterval:      500 * time.Millisecond,
		LimitOrderPct:      50,
		LimitPriceRangeBps: 50,
	}, seed)
}
