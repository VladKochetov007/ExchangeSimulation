package simulation

import (
	"time"

	"exchange_sim/exchange"
)

// SimTickerFactory creates simulation-time tickers backed by EventScheduler
type SimTickerFactory struct {
	scheduler *EventScheduler
}

// NewSimTickerFactory creates a new simulation ticker factory
func NewSimTickerFactory(scheduler *EventScheduler) *SimTickerFactory {
	return &SimTickerFactory{scheduler: scheduler}
}

// NewTicker implements exchange.TickerFactory
func (f *SimTickerFactory) NewTicker(d time.Duration) exchange.Ticker {
	t := &simTicker{
		scheduler: f.scheduler,
		interval:  d.Nanoseconds(),
		ch:        make(chan time.Time, 1), // Buffered to prevent blocking
	}
	t.start()
	return t
}

type simTicker struct {
	scheduler *EventScheduler
	interval  int64
	ch        chan time.Time
	eventID   uint64
	stopped   bool
}

func (t *simTicker) C() <-chan time.Time { return t.ch }

func (t *simTicker) Stop() {
	if !t.stopped && t.eventID != 0 {
		t.scheduler.Cancel(t.eventID)
		t.eventID = 0
		t.stopped = true
		close(t.ch)
	}
}

func (t *simTicker) start() {
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
