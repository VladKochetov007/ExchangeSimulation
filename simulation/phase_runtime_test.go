package simulation

import (
	"context"
	"reflect"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type phaseOrderActor struct {
	*actor.BaseActor
	start  func()
	events []actor.EventType
}

func newPhaseOrderActor(id uint64, gateway actor.Gateway, start func()) *phaseOrderActor {
	a := &phaseOrderActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		start:     start,
	}
	a.SetHandler(a)
	return a
}

func (a *phaseOrderActor) Start(ctx context.Context) error {
	a.start()
	return a.BaseActor.Start(ctx)
}

func (a *phaseOrderActor) HandleEvent(_ context.Context, event *actor.Event) {
	a.events = append(a.events, event.Type)
}

// This adversarial same-timestamp case exercises the entire phase path. The
// taker fill is enqueued before its acceptance response, while the maker's
// acceptance predates the fill; both actors must observe Accepted before
// Filled after fixed ingress, egress, and actor-inbox ordering.
func TestDeterministicPhasesPreserveOrderLifecycleAtSameTimestamp(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock:               clock,
		TickerFactory:       timers,
		DeterministicPhases: true,
	})
	ex.AddInstrument(exchange.NewSpotInstrument(
		"ABC-USD", "ABC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100,
	))
	mount := NewMount(ex, LatencyConfig{})
	makerGateway := mount.ConnectNewClient(1, map[string]int64{"ABC": exchange.BTC_PRECISION}, &exchange.PercentageFee{})
	takerGateway := mount.ConnectNewClient(2, map[string]int64{"USD": 1_000 * exchange.USD_PRECISION}, &exchange.PercentageFee{})

	var maker *phaseOrderActor
	maker = newPhaseOrderActor(1, makerGateway, func() {
		maker.SubmitOrder("ABC-USD", exchange.Sell, exchange.LimitOrder, 100*exchange.USD_PRECISION, exchange.BTC_PRECISION)
	})
	var taker *phaseOrderActor
	taker = newPhaseOrderActor(2, takerGateway, func() {
		taker.SubmitOrder("ABC-USD", exchange.Buy, exchange.Market, 0, exchange.BTC_PRECISION)
	})

	runner := NewRunner(clock, RunnerConfig{
		Iterations:          1,
		Step:                time.Millisecond,
		DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddIdler(timers)
	runner.AddActor(maker)
	runner.AddActor(taker)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []actor.EventType{actor.EventOrderAccepted, actor.EventOrderFilled}
	if !reflect.DeepEqual(maker.events, want) {
		t.Fatalf("maker lifecycle = %v, want %v", maker.events, want)
	}
	if !reflect.DeepEqual(taker.events, want) {
		t.Fatalf("taker lifecycle = %v, want %v", taker.events, want)
	}
}

// Scheduler-backed latency must use the same phase runtime rather than the
// legacy forwarding goroutines. This covers a full order/fill lifecycle with
// non-zero request, response, and market-data delay; the result is identical
// to a direct mount apart from the modeled arrival timestamps.
func TestDeterministicPhasesSupportScheduledLatency(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock:               clock,
		TickerFactory:       timers,
		DeterministicPhases: true,
	})
	ex.AddInstrument(exchange.NewSpotInstrument(
		"ABC-USD", "ABC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100,
	))
	latency := LatencyConfig{
		Request:    NewConstantLatency(time.Millisecond),
		Response:   NewConstantLatency(time.Millisecond),
		MarketData: NewConstantLatency(time.Millisecond),
		Scheduler:  scheduler,
		Clock:      clock,
	}
	mount := NewMount(ex, latency)
	makerGateway := mount.ConnectNewClient(1, map[string]int64{"ABC": exchange.BTC_PRECISION}, &exchange.PercentageFee{})
	takerGateway := mount.ConnectNewClient(2, map[string]int64{"USD": 1_000 * exchange.USD_PRECISION}, &exchange.PercentageFee{})

	var maker *phaseOrderActor
	maker = newPhaseOrderActor(1, makerGateway, func() {
		maker.SubmitOrder("ABC-USD", exchange.Sell, exchange.LimitOrder, 100*exchange.USD_PRECISION, exchange.BTC_PRECISION)
	})
	var taker *phaseOrderActor
	taker = newPhaseOrderActor(2, takerGateway, func() {
		taker.SubmitOrder("ABC-USD", exchange.Buy, exchange.Market, 0, exchange.BTC_PRECISION)
	})

	runner := NewRunner(clock, RunnerConfig{
		Iterations:          6,
		Step:                time.Millisecond,
		DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddIdler(timers)
	runner.AddActor(maker)
	runner.AddActor(taker)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run with scheduled latency: %v", err)
	}

	want := []actor.EventType{actor.EventOrderAccepted, actor.EventOrderFilled}
	if !reflect.DeepEqual(maker.events, want) {
		t.Fatalf("maker lifecycle = %v, want %v", maker.events, want)
	}
	if !reflect.DeepEqual(taker.events, want) {
		t.Fatalf("taker lifecycle = %v, want %v", taker.events, want)
	}
}

// A runtime reconfiguration can retire a scheduler-backed periodic job after
// its tick has been delivered but before the phase pump receives it. Retiring
// the job must consume that tick as well: otherwise SimTimerFactory retains a
// pending acknowledgement forever and the next fixed-point drain stalls.
func TestDeterministicPhaseRetiresPendingPeriodicJob(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock:                   clock,
		TickerFactory:           timers,
		DeterministicPhases:     true,
		SnapshotInterval:        time.Hour,
		BalanceSnapshotInterval: time.Millisecond,
	})
	mount := NewMount(ex, LatencyConfig{})
	mount.ConnectNewClient(1, nil, &exchange.PercentageFee{})

	// Deliver the old balance tick outside the runner, leaving it in the
	// ticker channel exactly as a concurrent reconfiguration would.
	clock.Advance(time.Millisecond)
	if got := timers.PendingTicks(); got != 1 {
		t.Fatalf("pending ticks before replacement = %d, want 1", got)
	}
	ex.EnableBalanceSnapshots(2 * time.Millisecond)

	runner := NewRunner(clock, RunnerConfig{
		Iterations:          1,
		Step:                time.Millisecond,
		DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddIdler(timers)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run after periodic-job replacement: %v", err)
	}
	if got := timers.PendingTicks(); got != 0 {
		t.Fatalf("pending ticks after replacement = %d, want 0", got)
	}
}
