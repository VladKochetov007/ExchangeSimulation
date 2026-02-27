package simulation

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// testActor is a minimal Actor implementation for runner tests.
type testActor struct {
	id      uint64
	gateway actor.Gateway
	started bool
	stopped bool
}

func (a *testActor) ID() uint64                    { return a.id }
func (a *testActor) Gateway() actor.Gateway        { return a.gateway }
func (a *testActor) Start(_ context.Context) error { a.started = true; return nil }
func (a *testActor) Stop() error                   { a.stopped = true; return nil }

func newTestVenue() *Venue {
	ex := exchange.NewExchange(10, &RealClock{})
	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex.AddInstrument(btcusd)
	return NewExchangeVenue(ex, LatencyConfig{})
}

func TestNewRunnerWithSimulatedClock(t *testing.T) {
	simClock := NewSimulatedClock(time.Now().UnixNano())
	runner := NewRunner(simClock, RunnerConfig{})

	if runner == nil {
		t.Fatal("NewRunner returned nil")
	}
	if _, ok := runner.clock.(*SimulatedClock); !ok {
		t.Error("Expected SimulatedClock")
	}
}

func TestNewRunnerWithRealClock(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{})

	if runner == nil {
		t.Fatal("NewRunner returned nil")
	}
	if _, ok := runner.clock.(*RealClock); !ok {
		t.Error("Expected RealClock")
	}
}

func TestRunnerAddVenue(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{})
	v := newTestVenue()
	runner.AddVenue(v)

	if len(runner.venues) != 1 {
		t.Errorf("Expected 1 venue, got %d", len(runner.venues))
	}
}

func TestRunnerAddActor(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{})
	v := newTestVenue()
	runner.AddVenue(v)

	gw := v.ConnectClient(1, map[string]int64{"USD": 1000000}, &exchange.FixedFee{})
	a := &testActor{id: 1, gateway: gw}
	runner.AddActor(a)

	if len(runner.actors) != 1 {
		t.Errorf("Expected 1 actor, got %d", len(runner.actors))
	}
}

func TestRunnerRunWithDuration(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{
		Duration: 100 * time.Millisecond,
	})
	runner.AddVenue(newTestVenue())

	start := time.Now()
	err := runner.Run(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected runtime >= 100ms, got %v", elapsed)
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("Expected runtime < 300ms, got %v", elapsed)
	}
}

func TestRunnerRunWithIterations(t *testing.T) {
	simClock := NewSimulatedClock(0)
	runner := NewRunner(simClock, RunnerConfig{
		Iterations: 10,
		Step:       time.Millisecond,
	})
	runner.AddVenue(newTestVenue())

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	elapsed := simClock.NowUnixNano()
	expectedMin := int64(10 * time.Millisecond)
	if elapsed < expectedMin {
		t.Errorf("Expected clock to advance at least %d ns, got %d ns", expectedMin, elapsed)
	}
}

func TestRunnerRunWithContextCancellation(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{
		Duration: 10 * time.Second,
	})
	runner.AddVenue(newTestVenue())

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
	runner := NewRunner(&RealClock{}, RunnerConfig{
		Duration: 100 * time.Millisecond,
	})
	v := newTestVenue()
	runner.AddVenue(v)

	balances := map[string]int64{"BTC": 1000000000, "USD": 1000000000000}
	gw1 := v.ConnectClient(1, balances, &exchange.FixedFee{})
	gw2 := v.ConnectClient(2, balances, &exchange.FixedFee{})

	a1 := &testActor{id: 1, gateway: gw1}
	a2 := &testActor{id: 2, gateway: gw2}
	runner.AddActor(a1)
	runner.AddActor(a2)

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !a1.started || !a2.started {
		t.Error("Actors were not started")
	}
	if !a1.stopped || !a2.stopped {
		t.Error("Actors were not stopped")
	}
}

func TestRunnerShutdown(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{
		Duration: 10 * time.Second,
	})
	runner.AddVenue(newTestVenue())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Runner did not shut down in time")
	}
}

func TestRunnerMultiVenue(t *testing.T) {
	runner := NewRunner(&RealClock{}, RunnerConfig{
		Duration: 100 * time.Millisecond,
	})

	v1 := newTestVenue()
	v2 := newTestVenue()
	runner.AddVenue(v1)
	runner.AddVenue(v2)

	balances := map[string]int64{"BTC": 1000000000, "USD": 1000000000000}
	gw1 := v1.ConnectClient(1, balances, &exchange.FixedFee{})
	gw2 := v2.ConnectClient(1, balances, &exchange.FixedFee{})

	runner.AddActor(&testActor{id: 1, gateway: gw1})
	runner.AddActor(&testActor{id: 2, gateway: gw2})

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}
