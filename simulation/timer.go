package simulation

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"exchange_sim/exchange"
)

// SimTimerFactory creates simulation-time timers backed by EventScheduler
type SimTimerFactory struct {
	scheduler           *EventScheduler
	mu                  sync.Mutex
	timers              []*simTimer
	deterministicPhases atomic.Bool
}

// PendingTicks counts ticks delivered by the scheduler but not yet acknowledged
// by their consumers. Counting only channel occupancy leaves a hand-off gap:
// a consumer can remove a tick before its own pending-work counter is updated,
// letting a quiescent runner advance simulated time past the tick.
func (f *SimTimerFactory) PendingTicks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.timers {
		n += int(t.pending.Load())
	}
	return n
}

// Idle implements the runner's quiescence contract.
func (f *SimTimerFactory) Idle() bool { return f.PendingTicks() == 0 }

// EnableDeterministicPhases turns otherwise tolerated ticker coalescing into
// a hard runtime error. A phase run may not claim reproducibility after it
// has skipped a model callback.
func (f *SimTimerFactory) EnableDeterministicPhases() {
	f.deterministicPhases.Store(true)
}

// DeterministicPhaseError reports whether any ticker tick was coalesced while
// phase mode was active. The runner checks it at each model-time boundary.
func (f *SimTimerFactory) DeterministicPhaseError() error {
	if !f.deterministicPhases.Load() {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, timer := range f.timers {
		if dropped := timer.dropped.Load(); dropped != 0 {
			return fmt.Errorf("simulation: deterministic phase ticker dropped %d tick(s)", dropped)
		}
	}
	return nil
}

// NewSimTimerFactory creates a new simulation timer factory
func NewSimTimerFactory(scheduler *EventScheduler) *SimTimerFactory {
	return &SimTimerFactory{scheduler: scheduler}
}

// NewTicker implements exchange.TickerFactory
func (f *SimTimerFactory) NewTicker(d time.Duration) exchange.Ticker {
	// Mirror time.NewTicker: a non-positive interval is a programming error,
	// and letting it through would hang ProcessUntil in an infinite loop.
	if d <= 0 {
		panic("simulation: non-positive interval for NewTicker")
	}
	t := &simTimer{
		scheduler:           f.scheduler,
		interval:            d.Nanoseconds(),
		ch:                  make(chan time.Time, 1), // Buffered to prevent blocking
		deterministicPhases: &f.deterministicPhases,
	}
	t.start()
	f.mu.Lock()
	f.timers = append(f.timers, t)
	f.mu.Unlock()
	return t
}

type simTimer struct {
	scheduler *EventScheduler
	interval  int64
	ch        chan time.Time
	mu        sync.Mutex
	eventID   uint64
	stopped   bool
	pending   atomic.Int64
	dropped   atomic.Int64

	deterministicPhases *atomic.Bool
}

func (t *simTimer) C() <-chan time.Time { return t.ch }

// Acknowledge marks one delivered tick as fully processed. It is deliberately
// optional: production tickers keep the small Ticker interface unchanged, and
// consumers only invoke it when the concrete ticker supports it.
func (t *simTimer) Acknowledge() { t.pending.Add(-1) }

// Stop cancels the underlying scheduler event. Like time.Ticker.Stop it does
// NOT close the channel: the tick callback may be mid-fire on another
// goroutine, and a send on a closed channel panics the scheduler.
func (t *simTimer) Stop() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	id := t.eventID
	t.eventID = 0
	t.mu.Unlock()

	if id != 0 {
		t.scheduler.Cancel(id)
	}
}

func (t *simTimer) start() {
	t.eventID = t.scheduler.ScheduleRepeating(t.interval, func() {
		t.mu.Lock()
		if t.stopped {
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()
		// Account before exposing the tick. A receiving goroutine may run as
		// soon as the send completes, so incrementing afterward reintroduces
		// the quiescence hand-off gap this counter is meant to close.
		t.pending.Add(1)
		// Non-blocking send - if channel full, skip this tick.
		select {
		case t.ch <- time.Unix(0, t.scheduler.clock.NowUnixNano()):
		default:
			t.pending.Add(-1)
			if t.deterministicPhases != nil && t.deterministicPhases.Load() {
				t.dropped.Add(1)
			}
		}
	})
}
