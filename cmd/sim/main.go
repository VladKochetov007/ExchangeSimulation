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
	os.MkdirAll("output", 0755)

	config := simulation.RunnerConfig{
		UseSimulatedClock: false,
		Duration:          0,
		Iterations:        0,
	}
	runner := simulation.NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewPerpFutures("BTCUSD", "BTC", "USD", 100000000, 1000000)
	ethusd := exchange.NewSpotInstrument("ETHUSD", "ETH", "USD", 10000000, 10000000)
	ex.AddInstrument(btcusd)
	ex.AddInstrument(ethusd)

	initialBalances := map[string]int64{
		"BTC": 10000000000,
		"ETH": 100000000000,
		"USD": 1000000000000,
	}
	feePlan := &exchange.PercentageFee{
		MakerBps: 2,
		TakerBps: 5,
		InQuote:  true,
	}

	for i := uint64(1); i <= 5; i++ {
		gateway := ex.ConnectClient(i, initialBalances, feePlan)
		lp := actor.NewFirstLP(i, gateway, actor.FirstLPConfig{
			Symbol:            "BTCUSD",
			SpreadBps:         20,
			LiquidityMultiple: 10,
			BootstrapPrice:    100000000000, // $100,000 per BTC (100000000 precision)
		})
		lp.SetInitialState(100000000, "BTC", "USD")
		lp.UpdateBalances(initialBalances["BTC"], initialBalances["USD"])
		runner.AddActor(lp)
	}

	// Add taker actors to generate trading activity
	for i := uint64(100); i <= 101; i++ {
		gateway := ex.ConnectClient(i, initialBalances, feePlan)
		taker := actor.NewRandomizedTaker(i, gateway, actor.RandomizedTakerConfig{
			Symbol:   "BTCUSD",
			Interval: 2 * time.Second,
			MinQty:   exchange.BTCAmount(0.05),
			MaxQty:   exchange.BTCAmount(0.5),
		})
		runner.AddActor(taker)
	}

	// Add DelayedMaker actors to provide competition later
	for i := uint64(200); i <= 201; i++ {
		gateway := ex.ConnectClient(i, initialBalances, feePlan)
		maker := actor.NewDelayedMaker(i, gateway, actor.DelayedMakerConfig{
			Symbol:      "BTCUSD",
			StartDelay:  5 * time.Second, // Arrive after 5 seconds
			OrderCount:  5,
			BasePrice:   100000 * 100000000,
			PriceSpread: 50 * 100000000, // Tighter or similar spread
			Qty:         1 * 100000000,
		})
		runner.AddActor(maker)
	}

	// Add IcebergMaker to provide hidden liquidity
	for i := uint64(300); i <= 300; i++ {
		gateway := ex.ConnectClient(i, initialBalances, feePlan)
		maker := actor.NewDelayedMaker(i, gateway, actor.DelayedMakerConfig{
			Symbol:      "BTCUSD",
			StartDelay:  8 * time.Second,
			OrderCount:  3,
			BasePrice:   100000 * 100000000,
			PriceSpread: 20 * 100000000,
			Qty:         5 * 100000000, // 5 BTC
			Visibility:  exchange.Iceberg,
			IcebergQty:  1 * 100000000, // Show 1 BTC
		})
		runner.AddActor(maker)
	}

	// Add NoisyTraders to provide random liquidity around mid-price
	for i := uint64(400); i <= 403; i++ {
		gateway := ex.ConnectClient(i, initialBalances, feePlan)
		noisy := actor.NewNoisyTrader(i, gateway, actor.NoisyTraderConfig{
			Symbol:          "BTCUSD",
			Interval:        1500 * time.Millisecond,
			PriceRangeBps:   100, // +/- 1% from mid
			MinQty:          exchange.BTCAmount(0.1),
			MaxQty:          exchange.BTCAmount(1.0),
			MaxActiveOrders: 3,
			OrderLifetime:   5 * time.Second,
		})
		runner.AddActor(noisy)
	}

	simDuration := 15 * time.Second
	recorderGateway := ex.ConnectClient(999, initialBalances, feePlan)
	recorder, err := actor.NewRecorder(999, recorderGateway, actor.RecorderConfig{
		OutputDir:           "output",
		Symbols:             []string{"BTCUSD", "ETHUSD"},
		FlushInterval:       time.Second,
		SnapshotInterval:    5 * time.Second,
		SnapshotDeltaCount:  50,
		RotationStrategy:    actor.RotationNone,
		RecordTrades:        true,
		RecordOrderbook:     true,
		RecordOpenInterest:  true,
		RecordFunding:       true,
		SeparateHiddenFiles: false,
	}, ex.Instruments)
	if err != nil {
		return err
	}
	runner.AddActor(recorder)

	fmt.Println("Starting simulation...")
	fmt.Printf("Running for %v...\n", simDuration)
	
	ctx, cancel := context.WithTimeout(context.Background(), simDuration)
	defer cancel()
	
	return runner.Run(ctx)
}
