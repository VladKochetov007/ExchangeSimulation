package simulation

import (
	"sync"
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
	current int64
	// goal accumulates Advance targets so concurrent Advance calls compose
	// additively (each gets its own disjoint time window) instead of both
	// computing a target from the same base and losing one delta.
	goal      int64
	mu        sync.RWMutex
	scheduler *EventScheduler
}

func NewSimulatedClock(start int64) *SimulatedClock {
	return &SimulatedClock{
		current: start,
		goal:    start,
	}
}

func (c *SimulatedClock) NowUnixNano() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *SimulatedClock) NowUnix() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current / 1e9
}

func (c *SimulatedClock) Advance(delta time.Duration) {
	c.mu.Lock()
	if c.goal < c.current {
		c.goal = c.current
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
	if c.current < target {
		c.current = target
	}
	c.mu.Unlock()
}

func (c *SimulatedClock) SetTime(t int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = t
}

// SetScheduler sets the event scheduler for this clock
// Must be called before Advance() if using event scheduling
func (c *SimulatedClock) SetScheduler(scheduler *EventScheduler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scheduler = scheduler
}
