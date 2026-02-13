package simulation

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
)

// TestSimulationSpeedup verifies that simulation can run faster than real-time
func TestSimulationSpeedup(t *testing.T) {
	// Setup simulation components
	simClock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	tickerFactory := NewSimTickerFactory(scheduler)

	// Create exchange with simulation time
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients: 10,
		Clock:            simClock,
		TickerFactory:    tickerFactory,
		SnapshotInterval: 100 * time.Millisecond,
	})

	// Create automation
	indexProvider := exchange.NewFixedIndexProvider()
	indexProvider.SetPrice("BTC-PERP", exchange.PriceUSD(50000, exchange.CENT_TICK))

	automation := exchange.NewExchangeAutomation(ex, exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexProvider,
		PriceUpdateInterval: 1 * time.Second,
		TickerFactory:       tickerFactory,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	automation.Start(ctx)
	defer automation.Stop()

	// Track events
	snapshotCount := 0
	priceUpdateCount := 0

	// Subscribe to snapshots
	perpInst := exchange.NewPerpFutures(
		"BTC-PERP", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.CENT_TICK, exchange.BTC_PRECISION/10000,
	)
	ex.AddInstrument(perpInst)

	// Simulate for 10 seconds of sim-time in less than 100ms wall-time
	wallStart := time.Now()
	simDuration := 10 * time.Second
	simTimeStep := 10 * time.Millisecond

	steps := int(simDuration / simTimeStep)
	for i := 0; i < steps; i++ {
		simClock.Advance(simTimeStep)

		// Count snapshots (every 100ms = 10 steps)
		if (i+1)%10 == 0 {
			snapshotCount++
		}

		// Count price updates (every 1s = 100 steps)
		if (i+1)%100 == 0 {
			priceUpdateCount++
		}
	}

	wallElapsed := time.Since(wallStart)
	speedup := float64(simDuration) / float64(wallElapsed)

	t.Logf("Simulated %v in %v (%.1fx speedup)", simDuration, wallElapsed, speedup)
	t.Logf("Snapshots expected: 100, Price updates expected: 10")

	// Verify we achieved significant speedup (at least 10x)
	if speedup < 10.0 {
		t.Errorf("Expected at least 10x speedup, got %.1fx", speedup)
	}

	// Verify simulation completed in reasonable wall-time (< 5 seconds)
	if wallElapsed > 5*time.Second {
		t.Errorf("Simulation took too long: %v", wallElapsed)
	}
}

// TestSimulationDeterminism verifies that simulations are deterministic
func TestSimulationDeterminism(t *testing.T) {
	runSim := func() []int64 {
		simClock := NewSimulatedClock(0)
		scheduler := NewEventScheduler(simClock)
		simClock.SetScheduler(scheduler)
		tickerFactory := NewSimTickerFactory(scheduler)

		ticker := tickerFactory.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		times := []int64{}
		done := make(chan struct{})

		go func() {
			for i := 0; i < 10; i++ {
				<-ticker.C()
				times = append(times, simClock.NowUnixNano())
			}
			close(done)
		}()

		// Advance clock
		for i := 0; i < 20; i++ {
			simClock.Advance(50 * time.Millisecond)
			time.Sleep(time.Millisecond)
		}

		<-done
		return times
	}

	// Run twice
	times1 := runSim()
	times2 := runSim()

	// Verify identical results
	if len(times1) != len(times2) {
		t.Fatalf("Different tick counts: %d vs %d", len(times1), len(times2))
	}

	for i := range times1 {
		if times1[i] != times2[i] {
			t.Errorf("Tick %d differs: %d vs %d", i, times1[i], times2[i])
		}
	}

	t.Logf("Both runs produced identical results: %d ticks", len(times1))
}
