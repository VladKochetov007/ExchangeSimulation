package simulation

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type LatencyProvider interface {
	Delay() time.Duration
}

// LatencyConfig holds optional per-channel latency. nil field = no delay on that channel.
type LatencyConfig struct {
	Request    LatencyProvider
	Response   LatencyProvider
	MarketData LatencyProvider
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

// LogNormalLatency draws delays from a log-normal distribution with a hard floor.
// log(L − min) ~ N(logMu, logSigma²), so L is strictly above min with a heavy right tail.
// Captures retransmit spikes and GC pauses that the Normal model cannot represent without
// producing impossible negative values.
//
// Constructed from the observable median rather than the log-space mean:
//   mean   = min + exp(logMu + logSigma²/2)
//   median = min + medianAboveMin
//   p99    ≈ min + exp(logMu + 2.326·logSigma)
//
// Calibrating logSigma by p99/median ratio (tail heaviness):
//   0.3 → p99 ≈ 2×  median   tight, stable LAN link
//   0.5 → p99 ≈ 3×  median   moderate, typical co-location
//   1.0 → p99 ≈ 10× median   heavy tail, WAN / congested path
type LogNormalLatency struct {
	min      time.Duration
	logMu    float64
	logSigma float64
	rng      *rand.Rand
	mu       sync.Mutex
}

func NewLogNormalLatency(min, medianAboveMin time.Duration, logSigma float64, seed int64) *LogNormalLatency {
	return &LogNormalLatency{
		min:      min,
		logMu:    math.Log(float64(medianAboveMin)),
		logSigma: logSigma,
		rng:      rand.New(rand.NewSource(seed)),
	}
}

func (l *LogNormalLatency) Delay() time.Duration {
	l.mu.Lock()
	z := l.rng.NormFloat64()
	l.mu.Unlock()
	return l.min + time.Duration(math.Exp(l.logMu+l.logSigma*z))
}

// HawkesLatency models processing latency as a self-exciting process with exponential kernel.
// Each order submission (RecordEvent) injects a spike α that decays at rate β per second:
//
//	R(t)    = R(t_last) · exp(−β · (t − t_last))   between events
//	R(t_n+) = R(t_n−)  + α                          at each event
//	L(t)    = minLatency + R(t)
//
// The exponential kernel admits an O(1) recursive update — no history retained.
// Driven by exogenous orders at mean rate ρ, steady-state excitation converges to:
//
//	E[R∞] = α·ρ/β   (geometric series; always finite since events are external)
//
// Calibrating decayPerSec from half-life: β = ln(2) / halfLife ≈ 0.693 / halfLife.Seconds()
//   β=1   → half-life 693ms   slow drain, persistent congestion
//   β=10  → half-life  69ms   moderate, burst clears in ~150ms
//   β=100 → half-life   7ms   fast, typical co-located exchange queue
//
// Under steady load ρ orders/s, mean added latency ≈ jumpPerEvent × ρ / β.
// Example: jump=10µs, ρ=1000/s, β=10 → +1ms above minLatency at saturation.
//
// RecordEvent must be called on every order submission. Delay is read-only.
type HawkesLatency struct {
	minLatency  time.Duration
	alpha       float64
	beta        float64
	excitation  float64
	lastEventNs int64
	mu          sync.Mutex
}

func NewHawkesLatency(minLatency, jumpPerEvent time.Duration, decayPerSec float64) *HawkesLatency {
	return &HawkesLatency{
		minLatency:  minLatency,
		alpha:       jumpPerEvent.Seconds(),
		beta:        decayPerSec,
		lastEventNs: time.Now().UnixNano(),
	}
}

func (h *HawkesLatency) RecordEvent() {
	now := time.Now().UnixNano()
	h.mu.Lock()
	dt := float64(now-h.lastEventNs) * 1e-9
	h.excitation = h.excitation*math.Exp(-h.beta*dt) + h.alpha
	h.lastEventNs = now
	h.mu.Unlock()
}

func (h *HawkesLatency) Delay() time.Duration {
	now := time.Now().UnixNano()
	h.mu.Lock()
	dt := float64(now-h.lastEventNs) * 1e-9
	exc := h.excitation * math.Exp(-h.beta*dt)
	h.mu.Unlock()
	return h.minLatency + time.Duration(exc*1e9)
}
