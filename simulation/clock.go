package simulation

import (
	"sync"
	"sync/atomic"
	"time"
)

type Clock interface {
	NowUnixNano() int64
	NowUnix() int64
}

// Advanceable is implemented by clocks that support deterministic time advancement.
// Runner uses this to drive iteration-based simulations without a type assertion.
type Advanceable interface {
	Advance(d time.Duration)
}

type RealClock struct{}

func (c *RealClock) NowUnixNano() int64 {
	return time.Now().UnixNano()
}

func (c *RealClock) NowUnix() int64 {
	return time.Now().Unix()
}

type SimulatedClock struct {
	// current is read far more often than anything else in the simulator: every
	// order, every log record and every scheduled callback asks for the time.
	// Readers therefore load it atomically instead of taking mu, which cost two
	// atomic read-modify-writes per read through RWMutex. Writers still hold mu,
	// because advancing the clock is a compare-and-set against goal that must
	// stay atomic with respect to other writers; the atomic store only makes the
	// value safely visible to lock-free readers, and a reader observes exactly
	// the same set of values it observed under the read lock.
	current atomic.Int64
	// goal accumulates Advance targets so concurrent Advance calls compose
	// additively (each gets its own disjoint time window) instead of both
	// computing a target from the same base and losing one delta.
	goal      int64
	mu        sync.RWMutex
	scheduler *EventScheduler
}

func NewSimulatedClock(start int64) *SimulatedClock {
	clock := &SimulatedClock{goal: start}
	clock.current.Store(start)
	return clock
}

func (c *SimulatedClock) NowUnixNano() int64 { return c.current.Load() }

func (c *SimulatedClock) NowUnix() int64 { return c.current.Load() / 1e9 }

func (c *SimulatedClock) Advance(delta time.Duration) {
	c.mu.Lock()
	if now := c.current.Load(); c.goal < now {
		c.goal = now
	}
	c.goal += int64(delta)
	target := c.goal
	c.mu.Unlock()

	// Walk simulation time forward event-by-event instead of jumping straight to
	// the target: ProcessUntil advances the clock to each due event's timestamp
	// before firing it, so a callback observes its own scheduled instant (and
	// anything it schedules relative to "now" chains correctly) rather than the
	// end of the whole jump.
	if c.scheduler != nil {
		c.scheduler.ProcessUntil(target)
	}

	// Rest at the requested time even when the last event fired earlier.
	c.mu.Lock()
	if c.current.Load() < target {
		c.current.Store(target)
	}
	c.mu.Unlock()
}

func (c *SimulatedClock) SetTime(t int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current.Store(t)
}

// SetScheduler sets the event scheduler for this clock
// Must be called before Advance() if using event scheduling
func (c *SimulatedClock) SetScheduler(scheduler *EventScheduler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scheduler = scheduler
}
