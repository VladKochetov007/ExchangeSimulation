package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/logger"
	"exchange_sim/realistic_sim/actors"
	"exchange_sim/simulation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	simDuration := 60 * time.Second
	logDir := "logs"

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}

	startTime := time.Now().UnixNano()
	simClock := simulation.NewSimulatedClock(startTime)
	ex := exchange.NewExchange(100, simClock)

	symbol := "BTC/USD"
	inst := exchange.NewSpotInstrument(
		symbol,
		"BTC", "USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.BTC_PRECISION/10000,
	)
	ex.AddInstrument(inst)

	logFile, err := os.Create(fmt.Sprintf("%s/simulation.log", logDir))
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer logFile.Close()

	simLogger := logger.New(logFile)
	ex.SetLogger(symbol, simLogger)

	clientID := uint64(1)
	allActors := []actor.Actor{}

	lpBalance := map[string]int64{
		"BTC": 20 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	lpGateway := ex.ConnectClient(clientID, lpBalance, &exchange.FixedFee{})
	clientID++

	lp := actors.NewFirstLP(clientID-1, lpGateway, actors.FirstLPConfig{
		Symbol:            symbol,
		HalfSpreadBps:     50,
		LiquidityMultiple: 10,
		MonitorInterval:   100 * time.Millisecond,
		MinExitSize:       exchange.BTC_PRECISION / 10,
		BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	})
	lp.SetInitialState(inst)
	lp.UpdateBalances(lpBalance["BTC"], lpBalance["USD"])
	allActors = append(allActors, lp)

	bootstrapPrice := exchange.PriceUSD(50000, exchange.DOLLAR_TICK)
	mmSpreads := []int64{5, 10, 20, 50}
	for _, spreadBps := range mmSpreads {
		mmBalance := map[string]int64{
			"BTC": 10 * exchange.BTC_PRECISION,
			"USD": 500000 * exchange.USD_PRECISION,
		}
		mmGateway := ex.ConnectClient(clientID, mmBalance, &exchange.FixedFee{})

		mm := actors.NewPureMarketMaker(clientID, mmGateway, actors.PureMarketMakerConfig{
			Symbol:           symbol,
			Instrument:       inst,
			SpreadBps:        spreadBps,
			QuoteSize:        exchange.BTC_PRECISION / 10,
			MaxInventory:     5 * exchange.BTC_PRECISION,
			RequoteThreshold: 5,
			MonitorInterval:  100 * time.Millisecond,
			BootstrapPrice:   bootstrapPrice,
		})
		allActors = append(allActors, mm)
		clientID++
	}

	takerBalance := map[string]int64{
		"BTC": 5 * exchange.BTC_PRECISION,
		"USD": 250000 * exchange.USD_PRECISION,
	}
	takerGateway := ex.ConnectClient(clientID, takerBalance, &exchange.FixedFee{})

	taker := actors.NewRandomizedTaker(clientID, takerGateway, actors.RandomizedTakerConfig{
		Symbol:         symbol,
		Interval:       500 * time.Millisecond,
		MinQty:         exchange.BTC_PRECISION / 100,
		MaxQty:         exchange.BTC_PRECISION / 10,
		BasePrecision:  exchange.BTC_PRECISION,
		QuotePrecision: exchange.USD_PRECISION,
	})
	allActors = append(allActors, taker)
	clientID++

	fmt.Println("=== Simple Exchange Simulation ===")
	fmt.Printf("Symbol: %s\n", symbol)
	fmt.Printf("Actors: 1 LP + %d MMs (spreads: %v bps) + 1 Taker\n", len(mmSpreads), mmSpreads)
	fmt.Printf("Duration: %v\n", simDuration)
	fmt.Printf("Log: %s/simulation.log\n", logDir)
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), simDuration)
	defer cancel()

	for _, a := range allActors {
		if err := a.Start(ctx); err != nil {
			return fmt.Errorf("start actor: %w", err)
		}
	}

	speedup := 10.0
	simTimeStep := 10 * time.Millisecond
	tickInterval := time.Duration(float64(simTimeStep) / speedup)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			goto shutdown
		case <-ticker.C:
			simClock.Advance(simTimeStep)
		}
	}

shutdown:
	for _, a := range allActors {
		a.Stop()
	}
	ex.Shutdown()

	elapsed := time.Since(start)
	simElapsed := time.Duration(simClock.NowUnixNano() - startTime)
	actualSpeedup := float64(simElapsed) / float64(elapsed)

	fmt.Printf("\nSimulation completed in %v wallclock\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Simulated time: %v\n", simElapsed.Round(time.Second))
	fmt.Printf("Speedup: %.1fx\n", actualSpeedup)

	return nil
}
