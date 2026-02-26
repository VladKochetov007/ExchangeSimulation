package simulation

import (
	"time"

	"exchange_sim/exchange"
)

// SimTimerFactory creates simulation-time timers backed by EventScheduler
type SimTimerFactory struct {
	scheduler *EventScheduler
}

// NewSimTimerFactory creates a new simulation timer factory
func NewSimTimerFactory(scheduler *EventScheduler) *SimTimerFactory {
	return &SimTimerFactory{scheduler: scheduler}
}

// NewTicker implements exchange.TickerFactory
func (f *SimTimerFactory) NewTicker(d time.Duration) exchange.Ticker {
	t := &simTimer{
		scheduler: f.scheduler,
		interval:  d.Nanoseconds(),
		ch:        make(chan time.Time, 1), // Buffered to prevent blocking
	}
	t.start()
	return t
}

type simTimer struct {
	scheduler *EventScheduler
	interval  int64
	ch        chan time.Time
	eventID   uint64
	stopped   bool
}

func (t *simTimer) C() <-chan time.Time { return t.ch }

func (t *simTimer) Stop() {
	if !t.stopped && t.eventID != 0 {
		t.scheduler.Cancel(t.eventID)
		t.eventID = 0
		t.stopped = true
		close(t.ch)
	}
}

func (t *simTimer) start() {
	t.eventID = t.scheduler.ScheduleRepeating(t.interval, func() {
		if t.stopped {
			return
		}
		// Non-blocking send - if channel full, skip this tick
		select {
		case t.ch <- time.Unix(0, t.scheduler.clock.NowUnixNano()):
		default:
		}
	})
}
