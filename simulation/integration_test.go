package simulation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/logger"
)

func TestSimulationIntegration(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	simClock := NewSimulatedClock(0)
	runner := NewRunner(simClock, RunnerConfig{
		Iterations: 100,
		Step:       time.Millisecond,
	})

	ex := exchange.NewExchange(10, simClock)
	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(btcusd)

	logFile, err := os.Create(filepath.Join("testdata", "BTCUSD.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	ex.SetLogger("BTCUSD", logger.New(logFile))

	v := NewExchangeVenue(ex, LatencyConfig{})
	runner.AddVenue(v)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify simulated clock advanced the expected amount
	expectedNs := int64(100 * time.Millisecond)
	if simClock.NowUnixNano() < expectedNs {
		t.Errorf("Expected sim clock >= %d ns, got %d ns", expectedNs, simClock.NowUnixNano())
	}

	logFile.Close()
	if _, err := os.Stat(filepath.Join("testdata", "BTCUSD.log")); os.IsNotExist(err) {
		t.Error("BTCUSD.log not created")
	}
}

func TestSimulationMultiVenueIntegration(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{
		Duration: 100 * time.Millisecond,
	})

	makeVenue := func(latencyMs int) *Venue {
		ex := exchange.NewExchange(10, &RealClock{})
		inst := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
		ex.AddInstrument(inst)
		return NewExchangeVenue(ex, LatencyConfig{
			Request:  NewConstantLatency(time.Duration(latencyMs) * time.Millisecond),
			Response: NewConstantLatency(time.Duration(latencyMs) * time.Millisecond),
		})
	}

	venueFast := makeVenue(1)
	venueSlow := makeVenue(5)
	runner.AddVenue(venueFast)
	runner.AddVenue(venueSlow)

	balances := map[string]int64{"BTC": 1000000000, "USD": 1000000000000}
	gwFast := venueFast.ConnectClient(1, balances, &exchange.FixedFee{})
	gwSlow := venueSlow.ConnectClient(1, balances, &exchange.FixedFee{})

	if gwFast == nil || gwSlow == nil {
		t.Fatal("ConnectClient returned nil")
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
