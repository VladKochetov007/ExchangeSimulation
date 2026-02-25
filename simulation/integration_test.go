package simulation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/logger"
	"exchange_sim/realistic_sim/actors"
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

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(btcusd)

	balances := map[string]int64{
		"BTC": 10000000000,
		"USD": 1000000000000,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}

	gateway := ex.ConnectClient(1, balances, feePlan)
	mm := actors.NewFirstLP(1, gateway, actors.FirstLPConfig{
		Symbol:            "BTCUSD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
		BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	})
	mm.SetInitialState(btcusd)
	mm.UpdateBalances(balances["BTC"], balances["USD"])
	runner.AddActor(mm)

	gateway2 := ex.ConnectClient(2, balances, feePlan)
	taker := actors.NewRandomizedTaker(2, gateway2, actors.RandomizedTakerConfig{
		Symbol:   "BTCUSD",
		Interval: 100 * time.Millisecond,
		MinQty:   exchange.BTCAmount(0.01),
		MaxQty:   exchange.BTCAmount(0.1),
	})
	runner.AddActor(taker)

	logFile, err := os.Create(filepath.Join("testdata", "BTCUSD.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	ex.SetLogger("BTCUSD", logger.New(logFile))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	logFile.Close()

	if _, err := os.Stat(filepath.Join("testdata", "BTCUSD.log")); os.IsNotExist(err) {
		t.Error("BTCUSD.log not created")
	}

	data, err := os.ReadFile(filepath.Join("testdata", "BTCUSD.log"))
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Error("BTCUSD.log is empty")
	}
}
