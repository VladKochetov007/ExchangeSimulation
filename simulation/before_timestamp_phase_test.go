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

type beforeTimestampFixture struct {
	runner             *Runner
	venue              *exchange.DefaultExchange
	maker              *phaseOrderActor
	taker              *phaseOrderActor
	takerRequestQueued bool
}

func newBeforeTimestampFixture(t *testing.T, arrivalAt time.Duration, drainIntermediate bool,
	hook func(*beforeTimestampFixture, int64) error) *beforeTimestampFixture {
	t.Helper()
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := NewSimTimerFactory(scheduler)
	venue := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock: clock, TickerFactory: timers, DeterministicPhases: true,
	})
	venue.AddInstrument(exchange.NewSpotInstrument(
		"ABC-USD", "ABC", "USD", exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100,
	))
	mount := NewMount(venue, LatencyConfig{})
	makerGateway := mount.ConnectNewClient(1, map[string]int64{"ABC": exchange.BTC_PRECISION}, &exchange.PercentageFee{})
	takerGateway := mount.ConnectNewClient(2, map[string]int64{"USD": 1_000 * exchange.USD_PRECISION}, &exchange.PercentageFee{})

	fixture := &beforeTimestampFixture{venue: venue}
	fixture.maker = newPhaseOrderActor(1, makerGateway, func() {
		fixture.maker.SubmitOrder("ABC-USD", exchange.Sell, exchange.LimitOrder,
			100*exchange.USD_PRECISION, exchange.BTC_PRECISION)
	})
	fixture.taker = newPhaseOrderActor(2, takerGateway, func() {})
	scheduler.Schedule(int64(arrivalAt), func() {
		fixture.takerRequestQueued = true
		fixture.taker.SubmitOrder("ABC-USD", exchange.Buy, exchange.Market, 0, exchange.BTC_PRECISION)
	})
	fixture.runner = NewRunner(clock, RunnerConfig{
		Iterations: 1, Step: time.Millisecond, DeterministicPhases: true,
		DrainIntermediateEvents: drainIntermediate,
		BeforeTimestampPhase:    func(atNano int64) error { return hook(fixture, atNano) },
	})
	fixture.runner.AddMount(mount)
	fixture.runner.AddIdler(timers)
	fixture.runner.AddActor(fixture.maker)
	fixture.runner.AddActor(fixture.taker)
	return fixture
}

func TestBeforeTimestampPhaseSeesPreIngressBook(t *testing.T) {
	var seen []int64
	fixture := newBeforeTimestampFixture(t, time.Millisecond, true, func(fixture *beforeTimestampFixture, atNano int64) error {
		seen = append(seen, atNano)
		_, askQty := fixture.venue.GetBestLiquidity("ABC-USD")
		switch atNano {
		case 0:
			if askQty != 0 || fixture.takerRequestQueued {
				t.Fatal("initial phase observed an unprocessed order")
			}
		case int64(time.Millisecond):
			if !fixture.takerRequestQueued || askQty != exchange.BTC_PRECISION {
				t.Fatalf("same-time taker changed the book before pre-ingress phase: queued=%v ask=%d",
					fixture.takerRequestQueued, askQty)
			}
		default:
			t.Fatalf("unexpected phase instant %d", atNano)
		}
		return nil
	})
	if err := fixture.runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(seen, []int64{0, int64(time.Millisecond)}) {
		t.Fatalf("phase instants = %v", seen)
	}
	_, askQty := fixture.venue.GetBestLiquidity("ABC-USD")
	if askQty != 0 || !reflect.DeepEqual(fixture.taker.events,
		[]actor.EventType{actor.EventOrderAccepted, actor.EventOrderFilled}) {
		t.Fatalf("same-time taker did not execute after phase: ask=%d events=%v", askQty, fixture.taker.events)
	}
}

func TestBeforeTimestampPhaseDrainsEarlierOffGridArrival(t *testing.T) {
	fixture := newBeforeTimestampFixture(t, 500*time.Microsecond, true,
		func(fixture *beforeTimestampFixture, atNano int64) error {
			if atNano == int64(time.Millisecond) {
				_, askQty := fixture.venue.GetBestLiquidity("ABC-USD")
				if !fixture.takerRequestQueued || askQty != 0 || len(fixture.taker.events) == 0 {
					t.Fatalf("earlier off-grid order was not committed before boundary: queued=%v ask=%d events=%v",
						fixture.takerRequestQueued, askQty, fixture.taker.events)
				}
			}
			return nil
		})
	if err := fixture.runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBeforeTimestampPhaseErrorStopsBeforeVenueIngress(t *testing.T) {
	stop := errors.New("joint epoch preflight failed")
	fixture := newBeforeTimestampFixture(t, time.Millisecond, true, func(fixture *beforeTimestampFixture, atNano int64) error {
		if atNano == int64(time.Millisecond) {
			if !fixture.takerRequestQueued {
				t.Fatal("scheduled taker request was not queued")
			}
			return stop
		}
		return nil
	})
	if err := fixture.runner.Run(context.Background()); !errors.Is(err, stop) {
		t.Fatalf("pre-ingress error lost: %v", err)
	}
	if len(fixture.taker.events) != 0 {
		t.Fatalf("taker received a venue outcome after pre-ingress failure: %v", fixture.taker.events)
	}
	_, askQty := fixture.venue.GetBestLiquidity("ABC-USD")
	if askQty != exchange.BTC_PRECISION {
		t.Fatalf("venue ingressed the taker after pre-ingress failure: ask=%d", askQty)
	}
}

func TestBeforeTimestampPhaseRequiresDeterministicRuntime(t *testing.T) {
	runner := NewRunner(NewSimulatedClock(0), RunnerConfig{
		BeforeTimestampPhase: func(int64) error { t.Fatal("hook ran outside deterministic runtime"); return nil },
	})
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("nondeterministic runtime accepted a before-timestamp hook")
	}
}

func TestIntermediateDrainRequiresScheduledClock(t *testing.T) {
	runner := NewRunner(NewSimulatedClock(0), RunnerConfig{
		Iterations: 1, Step: time.Millisecond, DeterministicPhases: true,
		DrainIntermediateEvents: true,
	})
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("intermediate-event drain accepted a clock without a scheduler")
	}
}
