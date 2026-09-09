package simulation

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type unreportedPhaseActor struct {
	id          uint64
	gateway     actor.Gateway
	queue       chan int64
	processedAt int64
	enabled     bool
}

func (a *unreportedPhaseActor) ID() uint64 { return a.id }

func (a *unreportedPhaseActor) Gateway() actor.Gateway { return a.gateway }

func (a *unreportedPhaseActor) Start(context.Context) error {
	a.enabled = true
	return nil
}

func (a *unreportedPhaseActor) Stop() error {
	a.enabled = false
	return nil
}

func (a *unreportedPhaseActor) EnableDeterministicPhases() { a.enabled = true }

func (a *unreportedPhaseActor) SupportsDeterministicPhases() bool { return true }

func (a *unreportedPhaseActor) PumpDeterministicPhase(context.Context) bool {
	if !a.enabled {
		return false
	}
	select {
	case processedAt := <-a.queue:
		a.processedAt = processedAt
		return true
	default:
		return false
	}
}

type unreportedDeterministicVenue struct {
	venue       *exchange.DefaultExchange
	clock       *SimulatedClock
	queue       chan struct{}
	processedAt int64
}

type advanceOnlyClock struct {
	now int64
}

type deterministicErrorIdler struct {
	err error
}

func (i *deterministicErrorIdler) Idle() bool { return true }

func (i *deterministicErrorIdler) EnableDeterministicPhases() {}

func (i *deterministicErrorIdler) DeterministicPhaseError() error { return i.err }

func (i *deterministicErrorIdler) DeterministicPhasePending() bool { return false }

func (c *advanceOnlyClock) NowUnixNano() int64 { return c.now }

func (c *advanceOnlyClock) NowUnix() int64 { return c.now / int64(time.Second) }

func (c *advanceOnlyClock) Advance(delta time.Duration) { c.now += int64(delta) }

func TestDeterministicPhasesRejectClockWithoutTimestampHook(t *testing.T) {
	runner := NewRunner(&advanceOnlyClock{}, RunnerConfig{
		Iterations:          1,
		Step:                time.Second,
		DeterministicPhases: true,
	})
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("deterministic phases accepted a clock without timestamp-hook support")
	}
}

func TestDeterministicPhaseErrorIsNotHiddenByReadiness(t *testing.T) {
	wantErr := errors.New("synthetic deterministic phase failure")
	runner := NewRunner(NewSimulatedClock(0), RunnerConfig{
		Iterations:          1,
		Step:                time.Second,
		DeterministicPhases: true,
	})
	runner.AddIdler(&deterministicErrorIdler{err: wantErr})
	if err := runner.Run(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Run error = %v, want %v", err, wantErr)
	}
}

func (v *unreportedDeterministicVenue) ConnectNewClient(clientID uint64, balances map[string]int64, fee exchange.FeeModel) actor.Gateway {
	return v.venue.ConnectNewClient(clientID, balances, fee)
}

func (v *unreportedDeterministicVenue) Shutdown() { v.venue.Shutdown() }

func (v *unreportedDeterministicVenue) IsRunning() bool { return v.venue.IsRunning() }

func (v *unreportedDeterministicVenue) DeterministicPhasesEnabled() bool { return true }

func (v *unreportedDeterministicVenue) PumpDeterministicPhase() bool {
	select {
	case <-v.queue:
		v.processedAt = v.clock.NowUnixNano()
		return true
	default:
		return v.venue.PumpDeterministicPhase()
	}
}

func (v *unreportedDeterministicVenue) DrainDeterministicEgress() bool {
	return v.venue.DrainDeterministicEgress()
}

func TestDeterministicPhasesConservativelyDrainUnreportedExtensions(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)
	baseVenue := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock: clock, TickerFactory: timers, DeterministicPhases: true,
	})
	venue := &unreportedDeterministicVenue{
		venue: baseVenue, clock: clock, queue: make(chan struct{}, 1),
	}
	mount := NewMount(venue, LatencyConfig{})
	extensionActor := &unreportedPhaseActor{
		id: 1, gateway: exchange.NewClientGateway(1), queue: make(chan int64, 1),
	}
	scheduler.Schedule(500, func() {
		extensionActor.queue <- clock.NowUnixNano()
		venue.queue <- struct{}{}
	})
	runner := NewRunner(clock, RunnerConfig{
		Iterations: 1, Step: time.Second, DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddActor(extensionActor)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if extensionActor.processedAt != 500 {
		t.Fatalf("unreported actor work processed at %d, want 500", extensionActor.processedAt)
	}
	if venue.processedAt != 500 {
		t.Fatalf("unreported venue work processed at %d, want 500", venue.processedAt)
	}
}

type embeddedQueuePhaseActor struct {
	*actor.BaseActor
	clock       *SimulatedClock
	queue       chan struct{}
	processedAt int64
}

func (a *embeddedQueuePhaseActor) HandleEvent(context.Context, *actor.Event) {}

func (a *embeddedQueuePhaseActor) PumpDeterministicPhase(ctx context.Context) bool {
	processed := false
	select {
	case <-a.queue:
		a.processedAt = a.clock.NowUnixNano()
		processed = true
	default:
	}
	return a.BaseActor.PumpDeterministicPhase(ctx) || processed
}

func TestEmbeddedBaseActorCanExposeExtensionPhaseReadiness(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock: clock, TickerFactory: timers, DeterministicPhases: true,
	})
	mount := NewMount(ex, LatencyConfig{})
	extensionActor := &embeddedQueuePhaseActor{
		BaseActor: actor.NewBaseActor(1, exchange.NewClientGateway(1)),
		clock:     clock,
		queue:     make(chan struct{}, 1),
	}
	extensionActor.SetHandler(extensionActor)
	extensionActor.SetDeterministicPhasePending(func() bool { return len(extensionActor.queue) != 0 })
	scheduler.Schedule(500, func() { extensionActor.queue <- struct{}{} })
	runner := NewRunner(clock, RunnerConfig{
		Iterations: 1, Step: time.Second, DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddActor(extensionActor)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if extensionActor.processedAt != 500 {
		t.Fatalf("embedded actor work processed at %d, want 500", extensionActor.processedAt)
	}
}

func TestEmbeddedBaseActorWithoutExtensionReadinessFailsClosed(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock: clock, TickerFactory: timers, DeterministicPhases: true,
	})
	mount := NewMount(ex, LatencyConfig{})
	extensionActor := &embeddedQueuePhaseActor{
		BaseActor: actor.NewBaseActor(1, exchange.NewClientGateway(1)),
		clock:     clock,
		queue:     make(chan struct{}, 1),
	}
	extensionActor.SetHandler(extensionActor)
	// Deliberately omit SetDeterministicPhasePending. BaseActor must opt into
	// the conservative runner path rather than letting this hidden queue cross
	// the scheduler timestamp boundary.
	scheduler.Schedule(500, func() { extensionActor.queue <- struct{}{} })
	runner := NewRunner(clock, RunnerConfig{
		Iterations: 1, Step: time.Second, DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddActor(extensionActor)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if extensionActor.processedAt != 500 {
		t.Fatalf("unreported embedded actor work processed at %d, want 500", extensionActor.processedAt)
	}
}

type endpointPhaseActor struct {
	*actor.BaseActor
	clock     *SimulatedClock
	scheduler *EventScheduler
	queue     chan string
	events    []string
}

func (a *endpointPhaseActor) HandleEvent(context.Context, *actor.Event) {}

func (a *endpointPhaseActor) PumpDeterministicPhase(ctx context.Context) bool {
	select {
	case phaseEvent := <-a.queue:
		a.events = append(a.events, phaseEvent)
		a.scheduler.Schedule(a.clock.NowUnixNano(), func() {
			a.events = append(a.events, "endpoint-follow-up")
		})
		a.BaseActor.PumpDeterministicPhase(ctx)
		return true
	default:
		return a.BaseActor.PumpDeterministicPhase(ctx)
	}
}

func TestDeterministicRunnerCompletesEndpointEventPhaseChain(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	phaseActor := &endpointPhaseActor{
		BaseActor: actor.NewBaseActor(1, exchange.NewClientGateway(1)),
		clock:     clock,
		scheduler: scheduler,
		queue:     make(chan string, 1),
	}
	phaseActor.SetHandler(phaseActor)
	scheduler.Schedule(int64(time.Second), func() {
		phaseActor.queue <- "endpoint-phase"
	})
	runner := NewRunner(clock, RunnerConfig{
		Iterations:          1,
		Step:                time.Second,
		DeterministicPhases: true,
	})
	runner.AddActor(phaseActor)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run endpoint phase chain: %v", err)
	}
	if got, want := phaseActor.events, []string{"endpoint-phase", "endpoint-follow-up"}; !equalStrings(got, want) {
		t.Fatalf("endpoint phase events = %v, want %v", got, want)
	}
}

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
	stats := NewLatencyStats()
	latency := LatencyConfig{
		Request:        NewConstantLatency(time.Millisecond),
		Response:       NewConstantLatency(time.Millisecond),
		MarketData:     NewConstantLatency(time.Millisecond),
		Scheduler:      scheduler,
		Clock:          clock,
		Telemetry:      stats,
		TelemetryLabel: "test/latency",
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
	summary := stats.Summary()
	seen := map[string]bool{}
	for _, row := range summary.Rows {
		seen[row.Channel] = true
		if row.Scheduled == 0 || row.Delivered == 0 {
			t.Fatalf("unobserved %s latency row: %+v", row.Channel, row)
		}
		if row.MeanDrawnNanoseconds != float64(time.Millisecond) || row.MeanDeliveryNanoseconds < float64(time.Millisecond) {
			t.Fatalf("wrong %s latency accounting: %+v", row.Channel, row)
		}
	}
	if !seen[string(LatencyRequest)] || !seen[string(LatencyResponse)] {
		t.Fatalf("missing scheduled request/response telemetry: %+v", summary.Rows)
	}
}

type phaseFrontierActor struct {
	*actor.BaseActor
	clock     *SimulatedClock
	tickTime  int64
	clockTime int64
	frontier  MarketDataFrontier
}

func newPhaseFrontierActor(id uint64, gateway actor.Gateway, clock *SimulatedClock) *phaseFrontierActor {
	a := &phaseFrontierActor{BaseActor: actor.NewBaseActor(id, gateway), clock: clock}
	a.SetHandler(a)
	a.AddTickerWithOffset(time.Second, 500*time.Millisecond, a.recordTick)
	return a
}

func (a *phaseFrontierActor) HandleEvent(context.Context, *actor.Event) {}

func (a *phaseFrontierActor) recordTick(tick time.Time) {
	if a.tickTime != 0 {
		return
	}
	a.tickTime = tick.UnixNano()
	a.clockTime = a.clock.NowUnixNano()
	if frontierGateway, ok := a.Gateway().(interface {
		MarketDataFrontier() MarketDataFrontier
	}); ok {
		a.frontier = frontierGateway.MarketDataFrontier()
	}
}

// A phase offset is a real simulated timestamp, not merely an ordering label.
// The courier must therefore record a due market-data receipt before an actor
// tick at that timestamp, even when Runner advances in coarser steps.
func TestDeterministicPhasesKeepOffsetDecisionAndReceiptAtOneTimestamp(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)
	stats := NewLatencyStats()
	recorder, err := NewMarketDataReceiptRecorder(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock: clock, TickerFactory: timers, DeterministicPhases: true,
	})
	mount := NewMount(ex, LatencyConfig{
		MarketData:         NewConstantLatency(0),
		Scheduler:          scheduler,
		Clock:              clock,
		Telemetry:          stats,
		TelemetryLabel:     "test/offset-frontier",
		MarketDataReceipts: recorder,
		ReceiptSourceVenue: "test",
		ReceiptLink:        "test/offset-frontier",
		ReceiptRole:        "offset_frontier",
	})
	gateway := mount.ConnectNewClient(1, nil, &exchange.PercentageFee{})
	inner := ex.Gateways[1]
	scheduler.Schedule((time.Second + 500*time.Millisecond).Nanoseconds(), func() {
		inner.MarketData <- &exchange.MarketDataMsg{
			Type: exchange.MDSnapshot, Symbol: "ABC/USD", SeqNum: 1,
			Data:      &exchange.BookSnapshot{},
			Timestamp: (time.Second + 500*time.Millisecond).Nanoseconds(),
		}
	})
	frontierActor := newPhaseFrontierActor(1, gateway, clock)
	frontierActor.SetTickerFactory(timers)
	runner := NewRunner(clock, RunnerConfig{
		Iterations: 1, Step: 2 * time.Second, DeterministicPhases: true,
	})
	runner.AddMount(mount)
	runner.AddIdler(timers)
	runner.AddActor(frontierActor)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if frontierActor.tickTime != (time.Second+500*time.Millisecond).Nanoseconds() || frontierActor.clockTime != frontierActor.tickTime {
		t.Fatalf("offset tick time=%d clock time=%d, want both 1.5s", frontierActor.tickTime, frontierActor.clockTime)
	}
	if frontierActor.frontier.Ordinal != 1 || frontierActor.frontier.DeliveredAt != frontierActor.tickTime {
		t.Fatalf("offset tick frontier=%+v, want receipt at tick timestamp", frontierActor.frontier)
	}
	if err := recorder.Finalize((2 * time.Second).Nanoseconds()); err != nil {
		t.Fatal(err)
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
