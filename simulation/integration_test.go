package simulation

import (
	"context"
	"os"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestSimulationIntegration(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	config := RunnerConfig{
		UseSimulatedClock: true,
		Duration:          0,
		Iterations:        100,
	}
	runner := NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(btcusd)

	balances := map[string]int64{
		"BTC": 10000000000,
		"USD": 1000000000000,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}

	gateway := ex.ConnectClient(1, balances, feePlan)
	mm := actor.NewFirstLP(1, gateway, actor.FirstLPConfig{
		Symbol:            "BTCUSD",
		SpreadBps:         20,
		LiquidityMultiple: 10,
	})
	runner.AddActor(mm)

	recorderGateway := ex.ConnectClient(999, balances, feePlan)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}
	recorder, err := actor.NewRecorder(999, recorderGateway, actor.RecorderConfig{
		Symbols:             []string{"BTCUSD"},
		OutputDir:           "testdata",
		RecordTrades:        true,
		RecordOrderbook:     true,
		RecordOpenInterest:  false,
		RecordFunding:       false,
		SeparateHiddenFiles: false,
	}, instruments)
	if err != nil {
		t.Fatal(err)
	}
	runner.AddActor(recorder)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat("testdata/BTCUSD_SPOT_trades.csv"); os.IsNotExist(err) {
		t.Error("BTCUSD_SPOT_trades.csv not created")
	}
	if _, err := os.Stat("testdata/BTCUSD_SPOT_orderbook.csv"); os.IsNotExist(err) {
		t.Error("BTCUSD_SPOT_orderbook.csv not created")
	}
}
