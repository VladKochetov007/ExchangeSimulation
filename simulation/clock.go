package simulation

import (
	"sync"
	"time"
)

type Clock interface {
	NowUnixNano() int64
	NowUnix() int64
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
	mu      sync.RWMutex
}

func NewSimulatedClock(start int64) *SimulatedClock {
	return &SimulatedClock{
		current: start,
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
	defer c.mu.Unlock()
	c.current += int64(delta)
}

func (c *SimulatedClock) SetTime(t int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = t
}
