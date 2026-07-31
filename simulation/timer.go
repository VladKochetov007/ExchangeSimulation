package simulation

import (
	"sync"
	"time"

	"exchange_sim/exchange"
)

// SimTimerFactory creates simulation-time timers backed by EventScheduler
type SimTimerFactory struct {
	scheduler *EventScheduler
	mu        sync.Mutex
	timers    []*simTimer
}

// PendingTicks counts ticks delivered into timer channels but not yet
// consumed by their owners. A tick sitting in that buffer is pending work
// that no actor or exchange counter can see, because the receiving goroutine
// has not run yet.
func (f *SimTimerFactory) PendingTicks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.timers {
		n += len(t.ch)
	}
	return n
}

// Idle implements the runner's quiescence contract.
func (f *SimTimerFactory) Idle() bool { return f.PendingTicks() == 0 }

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
		scheduler: f.scheduler,
		interval:  d.Nanoseconds(),
		ch:        make(chan time.Time, 1), // Buffered to prevent blocking
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
}

func (t *simTimer) C() <-chan time.Time { return t.ch }

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
		// Non-blocking send - if channel full, skip this tick
		select {
		case t.ch <- time.Unix(0, t.scheduler.clock.NowUnixNano()):
		default:
		}
	})
}
