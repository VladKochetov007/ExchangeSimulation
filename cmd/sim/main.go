package main

import (
	"context"
	"fmt"
	"os"

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

	recorderGateway := ex.ConnectClient(999, initialBalances, feePlan)
	recorder, err := actor.NewRecorder(999, recorderGateway, actor.RecorderConfig{
		Symbols:       []string{"BTCUSD", "ETHUSD"},
		TradesPath:    "output/trades.csv",
		SnapshotsPath: "output/snapshots.csv",
	})
	if err != nil {
		return err
	}
	runner.AddActor(recorder)

	fmt.Println("Starting simulation...")
	fmt.Println("Press Ctrl+C to stop")
	return runner.Run(context.Background())
}
