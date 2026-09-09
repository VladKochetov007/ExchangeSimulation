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
	goal int64
	mu   sync.RWMutex
	// advanceMu serializes scheduler callbacks and timestamp hooks. Concurrent
	// callers still receive additive time windows, but no callback can observe
	// another Advance midway through its own simulated interval.
	advanceMu sync.Mutex
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
	_ = c.advance(delta, nil)
}

// AdvanceWithTimestampHook advances by delta and invokes hook after each
// complete scheduler timestamp. Deterministic runners use this boundary to
// drain delayed courier and actor phases while the clock still equals the
// timestamp that made the work due.
func (c *SimulatedClock) AdvanceWithTimestampHook(delta time.Duration, hook func() error) error {
	return c.advance(delta, hook)
}

func (c *SimulatedClock) advance(delta time.Duration, hook func() error) error {
	c.advanceMu.Lock()
	defer c.advanceMu.Unlock()

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
		if hook == nil {
			c.scheduler.ProcessUntil(target)
		} else if err := c.scheduler.ProcessUntilWithTimestampHook(target, hook); err != nil {
			c.mu.Lock()
			// The scheduler leaves the clock at the failing event timestamp.
			// Reset the accumulated target as well: a caller that records the
			// error and retries must resume from that exact boundary rather than
			// replaying the remainder of a failed advance at a past-due time.
			c.goal = c.current
			c.mu.Unlock()
			return err
		}
	}

	// Rest at the requested time even when the last event fired earlier.
	c.mu.Lock()
	if c.current < target {
		c.current = target
	}
	c.mu.Unlock()
	return nil
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
