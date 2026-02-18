package simulation

import (
	"math/rand"
	"sync/atomic"
	"time"
)

type LatencyProvider interface {
	Delay() time.Duration
}

type ConstantLatency struct {
	delay time.Duration
}

func NewConstantLatency(delay time.Duration) *ConstantLatency {
	return &ConstantLatency{delay: delay}
}

func (c *ConstantLatency) Delay() time.Duration {
	return c.delay
}

type UniformRandomLatency struct {
	min time.Duration
	max time.Duration
	rng *rand.Rand
}

func NewUniformRandomLatency(min, max time.Duration, seed int64) *UniformRandomLatency {
	return &UniformRandomLatency{
		min: min,
		max: max,
		rng: rand.New(rand.NewSource(seed)),
	}
}

func (u *UniformRandomLatency) Delay() time.Duration {
	delta := u.max - u.min
	return u.min + time.Duration(u.rng.Int63n(int64(delta)))
}

type NormalLatency struct {
	mean   time.Duration
	stddev time.Duration
	rng    *rand.Rand
}

func NewNormalLatency(mean, stddev time.Duration, seed int64) *NormalLatency {
	return &NormalLatency{
		mean:   mean,
		stddev: stddev,
		rng:    rand.New(rand.NewSource(seed)),
	}
}

func (n *NormalLatency) Delay() time.Duration {
	delay := time.Duration(n.rng.NormFloat64()*float64(n.stddev) + float64(n.mean))
	if delay < 0 {
		delay = 0
	}
	return delay
}

// LoadScaledLatency scales base latency linearly with the number of in-flight requests.
// Useful for modelling exchange processing queues: as more orders pile up, round-trip
// latency grows. Users call Inc() on order submit and Dec() on acknowledgement.
//
// Effective delay = base + load * perRequest. Both are configurable at construction.
type LoadScaledLatency struct {
	base       time.Duration
	perRequest time.Duration
	load       atomic.Int64
}

func NewLoadScaledLatency(base, perRequest time.Duration) *LoadScaledLatency {
	return &LoadScaledLatency{base: base, perRequest: perRequest}
}

func (l *LoadScaledLatency) Inc() { l.load.Add(1) }
func (l *LoadScaledLatency) Dec() { l.load.Add(-1) }

func (l *LoadScaledLatency) Delay() time.Duration {
	n := l.load.Load()
	if n < 0 {
		n = 0
	}
	return l.base + time.Duration(n)*l.perRequest
}
