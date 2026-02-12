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
	simDuration    = 600 * time.Second
	speedup        = 50.0
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
		exchange.CENT_TICK, // $0.01 tick size
		exchange.BTC_PRECISION/10000,
	)
	ex.AddInstrument(perpInst)

	logDir := "logs/randomwalk_stoikov"
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

	indexProvider := exchange.NewFixedIndexProvider()
	indexProvider.SetPrice("BTC-PERP", exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK))

	automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexProvider,
		PriceUpdateInterval: 3 * time.Second,
		CollateralRate:      500,
	})

	// FirstLP for initial liquidity
	lpID := uint64(1)
	lpGateway := ex.ConnectClient(lpID, map[string]int64{}, &exchange.FixedFee{})
	ex.AddPerpBalance(lpID, "USD", 1000000*exchange.USD_PRECISION)

	firstLP := actors.NewFirstLP(lpID, lpGateway, actors.FirstLPConfig{
		Symbol:            "BTC-PERP",
		HalfSpreadBps:     20,
		LiquidityMultiple: 5,
		MonitorInterval:   100 * time.Millisecond,
		MinExitSize:       exchange.BTC_PRECISION / 10,
		BootstrapPrice:    exchange.PriceUSD(bootstrapPrice, exchange.CENT_TICK),
	})

	// Avellaneda-Stoikov Market Makers (5 with different risk aversion)
	var marketMakers []actor.Actor
	gammas := []int64{100, 200, 500, 1000, 2000} // Risk aversion parameters

	for i, gamma := range gammas {
		mmID := uint64(2 + i)
		mmGateway := ex.ConnectClient(mmID, map[string]int64{}, &exchange.FixedFee{})
		ex.AddPerpBalance(mmID, "USD", 1000000*exchange.USD_PRECISION)

		mm := actors.NewAvellanedaStoikov(mmID, mmGateway, actors.AvellanedaStoikovConfig{
			Symbol:           "BTC-PERP",
			Gamma:            gamma,                              // Risk aversion
			K:                150,                                // Order book depth parameter
			T:                3600,                               // Time horizon (1 hour)
			QuoteQty:         2 * exchange.BTC_PRECISION / 100,   // 0.02 BTC
			MaxInventory:     10 * exchange.BTC_PRECISION,        // 10 BTC max
			VolatilityWindow: 20,                                 // 20 price samples for volatility
			RequoteInterval:  500 * time.Millisecond,
		})
		marketMakers = append(marketMakers, mm)
	}

	// Small frequent takers
	intervals := []time.Duration{
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		600 * time.Millisecond,
	}
	var smallTakers []actor.Actor

	for i, interval := range intervals {
		takerID := uint64(10 + i)
		takerGateway := ex.ConnectClient(takerID, map[string]int64{}, &exchange.FixedFee{})
		ex.AddPerpBalance(takerID, "USD", 1000000*exchange.USD_PRECISION)

		taker := actors.NewRandomizedTaker(takerID, takerGateway, actors.RandomizedTakerConfig{
			Symbol:         "BTC-PERP",
			Interval:       interval,
			MinQty:         exchange.BTC_PRECISION / 100,
			MaxQty:         4 * exchange.BTC_PRECISION / 100,
			BasePrecision:  exchange.BTC_PRECISION,
			QuotePrecision: exchange.USD_PRECISION,
		})
		smallTakers = append(smallTakers, taker)
	}

	// Large infrequent takers
	var largeTakers []actor.Actor

	for i := 0; i < 2; i++ {
		takerID := uint64(20 + i)
		takerGateway := ex.ConnectClient(takerID, map[string]int64{}, &exchange.FixedFee{})
		ex.AddPerpBalance(takerID, "USD", 1000000*exchange.USD_PRECISION)

		taker := actors.NewRandomizedTaker(takerID, takerGateway, actors.RandomizedTakerConfig{
			Symbol:         "BTC-PERP",
			Interval:       2 * time.Second,
			MinQty:         4 * exchange.BTC_PRECISION / 100,
			MaxQty:         10 * exchange.BTC_PRECISION / 100,
			BasePrecision:  exchange.BTC_PRECISION,
			QuotePrecision: exchange.USD_PRECISION,
		})
		largeTakers = append(largeTakers, taker)
	}

	ctx, cancel := context.WithTimeout(context.Background(), simDuration)
	defer cancel()

	allActors := []actor.Actor{firstLP}
	allActors = append(allActors, marketMakers...)
	allActors = append(allActors, smallTakers...)
	allActors = append(allActors, largeTakers...)

	automation.Start(ctx)
	defer automation.Stop()

	for _, actor := range allActors {
		if err := actor.Start(ctx); err != nil {
			return fmt.Errorf("start actor: %w", err)
		}
	}

	tickInterval := time.Duration(float64(simTimeStep) / speedup)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	wallStart := time.Now()

	for {
		select {
		case <-ctx.Done():
			goto shutdown
		case <-ticker.C:
			simClock.Advance(simTimeStep)
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

	fmt.Printf("Simulation complete\n")
	fmt.Printf("Wall-clock time: %v\n", wallElapsed)
	fmt.Printf("Simulated time: %v\n", simElapsed.Round(time.Second))
	fmt.Printf("Actual speedup: %.2fx\n", actualSpeedup)
	fmt.Printf("Log file: %s\n", logFilePath)

	return nil
}
