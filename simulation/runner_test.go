package simulation

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
)

func TestNewRunnerWithSimulatedClock(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: true,
		Duration:          0,
		Iterations:        0,
	}

	runner := NewRunner(config)
	if runner == nil {
		t.Fatal("NewRunner returned nil")
	}

	if _, ok := runner.clock.(*SimulatedClock); !ok {
		t.Error("Expected SimulatedClock when UseSimulatedClock is true")
	}
}

func TestNewRunnerWithRealClock(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
		Duration:          0,
		Iterations:        0,
	}

	runner := NewRunner(config)
	if runner == nil {
		t.Fatal("NewRunner returned nil")
	}

	if _, ok := runner.clock.(*RealClock); !ok {
		t.Error("Expected RealClock when UseSimulatedClock is false")
	}
}

func TestRunnerExchange(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
	}

	runner := NewRunner(config)
	ex := runner.Exchange()

	if ex == nil {
		t.Error("Exchange() returned nil")
	}
}

func TestRunnerAddActor(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
	}

	runner := NewRunner(config)
	gateway := runner.Exchange().ConnectClient(1, map[string]int64{"USD": 1000000}, &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

	mm := actors.NewFirstLP(1, gateway, actors.FirstLPConfig{
		Symbol:            "BTCUSD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
	})

	runner.AddActor(mm)

	if len(runner.actors) != 1 {
		t.Errorf("Expected 1 actor, got %d", len(runner.actors))
	}
}

func TestRunnerRunWithDuration(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
		Duration:          100 * time.Millisecond,
		Iterations:        0,
	}

	runner := NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(btcusd)

	ctx := context.Background()

	start := time.Now()
	err := runner.Run(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected runtime >= 100ms, got %v", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("Expected runtime < 200ms, got %v", elapsed)
	}
}

func TestRunnerRunWithIterations(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: true,
		Duration:          0,
		Iterations:        10,
	}

	runner := NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(btcusd)

	ctx := context.Background()

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	simClock := runner.clock.(*SimulatedClock)
	elapsed := simClock.NowUnixNano()
	expectedMin := int64(10 * time.Millisecond)

	if elapsed < expectedMin {
		t.Errorf("Expected clock to advance at least %d ns, got %d ns", expectedMin, elapsed)
	}
}

func TestRunnerRunWithContextCancellation(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
		Duration:          10 * time.Second,
		Iterations:        0,
	}

	runner := NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(btcusd)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := runner.Run(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if elapsed > 500*time.Millisecond {
		t.Errorf("Expected quick cancellation, took %v", elapsed)
	}
}

func TestRunnerRunWithActors(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
		Duration:          100 * time.Millisecond,
	}

	runner := NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(btcusd)

	balances := map[string]int64{"BTC": 1000000000, "USD": 1000000000000}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}

	gateway1 := ex.ConnectClient(1, balances, feePlan)
	mm1 := actors.NewFirstLP(1, gateway1, actors.FirstLPConfig{
		Symbol:            "BTCUSD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
	})
	runner.AddActor(mm1)

	gateway2 := ex.ConnectClient(2, balances, feePlan)
	mm2 := actors.NewFirstLP(2, gateway2, actors.FirstLPConfig{
		Symbol:            "BTCUSD",
		HalfSpreadBps:     15, // 0.15% half-spread (was 30 bps / 2)
		LiquidityMultiple: 10,
	})
	runner.AddActor(mm2)

	ctx := context.Background()

	err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

func TestRunnerShutdown(t *testing.T) {
	config := RunnerConfig{
		UseSimulatedClock: false,
		Duration:          10 * time.Second,
	}

	runner := NewRunner(config)
	ex := runner.Exchange()

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	ex.AddInstrument(btcusd)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		runner.Run(ctx)
		done <- true
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Runner did not shut down in time")
	}
}
