package simulation

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"exchange_sim/clock"
	"exchange_sim/types"
)

type LatencyProvider interface {
	Delay() time.Duration
}

// LatencyConfig holds optional per-channel latency. nil field = no delay on that channel.
//
// With only latency providers set, delays are applied as wall-clock sleeps —
// correct for real-time runs but meaningless under a SimulatedClock (a "1ms"
// delay becomes however much sim time passes while the goroutine sleeps).
// Set Scheduler+Clock to deliver messages at exact simulation timestamps
// with per-channel FIFO ordering preserved.
type LatencyConfig struct {
	Request    LatencyProvider
	Response   LatencyProvider
	MarketData LatencyProvider

	// PerClient, when set, builds a private latency source for each client
	// instead of sharing one across every participant on the link.
	//
	// Sharing one random source makes each client's delays depend on how many
	// messages every other client on the same link happened to draw first,
	// which couples participants through the random stream rather than through
	// the market. Two runs that differ anywhere then differ in every delay
	// afterwards. A private stream per client removes that coupling; the link
	// still has one latency distribution, each participant just draws its own
	// sample path from it.
	PerClient func(clientID uint64) (request, response, marketData LatencyProvider)

	// Scheduler delivers delayed messages at exact sim times (required for
	// correct latency under SimulatedClock). Clock must be the same clock the
	// scheduler is bound to.
	Scheduler *EventScheduler
	Clock     types.Clock

	// Telemetry records compact delivery accounting for this link. It is an
	// observation-only sink: it never feeds back into delay sampling, event
	// scheduling, or gateway decisions.
	Telemetry      *LatencyStats
	TelemetryLabel string
}

// LatencyChannel identifies the three independently delayed link paths.
type LatencyChannel string

const (
	LatencyRequest    LatencyChannel = "request"
	LatencyResponse   LatencyChannel = "response"
	LatencyMarketData LatencyChannel = "market_data"
)

// LatencyStats is a compact accounting sink for actual courier delivery. The
// latency arm cannot infer this quantity from a book's later behavioural
// reaction, which also includes an actor's decision clock.
type LatencyStats struct {
	mu   sync.Mutex
	rows map[latencyStatsKey]*latencyStatsRow
}

type latencyStatsKey struct {
	Label   string
	Channel LatencyChannel
}

type latencyStatsRow struct {
	Scheduled   int64
	Delivered   int64
	DrawnNS     int64
	QueueNS     int64
	DeliveredNS int64
}

// LatencySummary is the persisted compact evidence product. Durations are in
// nanoseconds so no precision is discarded before analysis.
type LatencySummary struct {
	Domain string              `json:"domain"`
	Rows   []LatencySummaryRow `json:"rows"`
}

type LatencySummaryRow struct {
	Link                    string  `json:"link"`
	Channel                 string  `json:"channel"`
	Scheduled               int64   `json:"scheduled"`
	Delivered               int64   `json:"delivered"`
	Undelivered             int64   `json:"undelivered"`
	MeanDrawnNanoseconds    float64 `json:"mean_drawn_nanoseconds"`
	MeanQueueNanoseconds    float64 `json:"mean_fifo_queue_nanoseconds"`
	MeanDeliveryNanoseconds float64 `json:"mean_delivery_nanoseconds"`
}

func NewLatencyStats() *LatencyStats {
	return &LatencyStats{rows: make(map[latencyStatsKey]*latencyStatsRow)}
}

type latencyTicket struct {
	stats     *LatencyStats
	key       latencyStatsKey
	sourceAt  int64
	scheduled int64
}

func (s *LatencyStats) scheduled(label string, channel LatencyChannel, sourceAt, drawnAt, scheduledAt int64) latencyTicket {
	if s == nil {
		return latencyTicket{}
	}
	key := latencyStatsKey{Label: label, Channel: channel}
	s.mu.Lock()
	row := s.rows[key]
	if row == nil {
		row = &latencyStatsRow{}
		s.rows[key] = row
	}
	row.Scheduled++
	row.DrawnNS += drawnAt - sourceAt
	row.QueueNS += scheduledAt - drawnAt
	s.mu.Unlock()
	return latencyTicket{stats: s, key: key, sourceAt: sourceAt, scheduled: scheduledAt}
}

func (s *LatencyStats) delivered(ticket latencyTicket, deliveredAt int64) {
	if s == nil || ticket.stats != s {
		return
	}
	s.mu.Lock()
	if row := s.rows[ticket.key]; row != nil {
		row.Delivered++
		row.DeliveredNS += deliveredAt - ticket.sourceAt
	}
	s.mu.Unlock()
}

func (s *LatencyStats) Summary() LatencySummary {
	if s == nil {
		return LatencySummary{Domain: "courier_delivery"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := LatencySummary{Domain: "courier_delivery", Rows: make([]LatencySummaryRow, 0, len(s.rows))}
	for key, row := range s.rows {
		item := LatencySummaryRow{
			Link: key.Label, Channel: string(key.Channel), Scheduled: row.Scheduled,
			Delivered: row.Delivered, Undelivered: row.Scheduled - row.Delivered,
		}
		if row.Scheduled > 0 {
			item.MeanDrawnNanoseconds = float64(row.DrawnNS) / float64(row.Scheduled)
			item.MeanQueueNanoseconds = float64(row.QueueNS) / float64(row.Scheduled)
		}
		if row.Delivered > 0 {
			item.MeanDeliveryNanoseconds = float64(row.DeliveredNS) / float64(row.Delivered)
		}
		result.Rows = append(result.Rows, item)
	}
	sort.Slice(result.Rows, func(i, j int) bool {
		if result.Rows[i].Link != result.Rows[j].Link {
			return result.Rows[i].Link < result.Rows[j].Link
		}
		return result.Rows[i].Channel < result.Rows[j].Channel
	})
	return result
}

// WriteJSON persists the compact latency evidence after all scheduled courier
// work has drained.
func (s *LatencyStats) WriteJSON(path string) error {
	raw, err := json.MarshalIndent(s.Summary(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0644)
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
	if delta <= 0 {
		return u.min
	}
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
//
//	mean   = min + exp(logMu + logSigma²/2)
//	median = min + medianAboveMin
//	p99    ≈ min + exp(logMu + 2.326·logSigma)
//
// Calibrating logSigma by p99/median ratio (tail heaviness):
//
//	0.3 → p99 ≈ 2×  median   tight, stable LAN link
//	0.5 → p99 ≈ 3×  median   moderate, typical co-location
//	1.0 → p99 ≈ 10× median   heavy tail, WAN / congested path
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
//
//	β=1   → half-life 693ms   slow drain, persistent congestion
//	β=10  → half-life  69ms   moderate, burst clears in ~150ms
//	β=100 → half-life   7ms   fast, typical co-located exchange queue
//
// Under steady load ρ orders/s, mean added latency ≈ jumpPerEvent × ρ / β.
// Example: jump=10µs, ρ=1000/s, β=10 → +1ms above minLatency at saturation.
//
// RecordEvent must be called on every order submission. Delay is read-only.
// Under a SimulatedClock, call SetClock so the decay follows simulation time
// instead of wall time.
type HawkesLatency struct {
	minLatency  time.Duration
	alpha       float64
	beta        float64
	excitation  float64
	lastEventNs int64
	clock       types.Clock
	mu          sync.Mutex
}

func NewHawkesLatency(minLatency, jumpPerEvent time.Duration, decayPerSec float64) *HawkesLatency {
	h := &HawkesLatency{
		minLatency: minLatency,
		alpha:      jumpPerEvent.Seconds(),
		beta:       decayPerSec,
		clock:      &clock.RealClock{},
	}
	h.lastEventNs = h.clock.NowUnixNano()
	return h
}

// SetClock switches the decay time source (e.g. to a SimulatedClock).
func (h *HawkesLatency) SetClock(c types.Clock) {
	h.mu.Lock()
	h.clock = c
	h.lastEventNs = c.NowUnixNano()
	h.mu.Unlock()
}

func (h *HawkesLatency) RecordEvent() {
	h.mu.Lock()
	now := h.clock.NowUnixNano()
	dt := float64(now-h.lastEventNs) * 1e-9
	h.excitation = h.excitation*math.Exp(-h.beta*dt) + h.alpha
	h.lastEventNs = now
	h.mu.Unlock()
}

func (h *HawkesLatency) Delay() time.Duration {
	h.mu.Lock()
	now := h.clock.NowUnixNano()
	dt := float64(now-h.lastEventNs) * 1e-9
	exc := h.excitation * math.Exp(-h.beta*dt)
	h.mu.Unlock()
	return h.minLatency + time.Duration(exc*1e9)
}

// LognormalLatency draws delays from a lognormal distribution, which is what
// measured network and matching-engine round trips look like: a tight mode
// with a long right tail rather than a symmetric spread.
//
// A normal draw understates how often a participant is late, because the
// distribution it assumes has no tail. The difference matters for any
// participant whose edge disappears when it is late, since the losses come
// entirely from that tail rather than from the median.
type LognormalLatency struct {
	median time.Duration
	sigma  float64
	cap    time.Duration
	rng    *rand.Rand
}

// NewLognormalLatency builds a lognormal provider. Sigma is the standard
// deviation of the underlying normal, so it sets how heavy the tail is: 0.25
// is a well-behaved link, 1.0 is one where the slowest percent of messages
// arrive many times later than the median. A positive cap truncates the tail,
// which models a client that gives up and retries rather than waiting.
func NewLognormalLatency(median time.Duration, sigma float64, cap time.Duration, seed int64) *LognormalLatency {
	return &LognormalLatency{median: median, sigma: sigma, cap: cap, rng: rand.New(rand.NewSource(seed))}
}

// Delay implements LatencyProvider.
func (l *LognormalLatency) Delay() time.Duration {
	if l.median <= 0 {
		return 0
	}
	if l.sigma <= 0 {
		return l.median
	}
	delay := time.Duration(float64(l.median) * math.Exp(l.rng.NormFloat64()*l.sigma))
	if delay < 0 {
		return 0
	}
	if l.cap > 0 && delay > l.cap {
		return l.cap
	}
	return delay
}

// SpikyLatency is a fast link that occasionally stalls: it returns Base almost
// always and Spike with probability SpikeProbability.
//
// It exists because a participant's behaviour under a rare stall is not the
// same as its behaviour under a slightly wider average. A market maker with a
// fast link that stalls once a minute is picked off during exactly those
// stalls, and no mean latency reproduces that.
type SpikyLatency struct {
	base             LatencyProvider
	spike            LatencyProvider
	spikeProbability float64
	rng              *rand.Rand
}

// NewSpikyLatency composes two providers. Probability outside (0,1) makes the
// provider degenerate to base, which is the harmless reading of "never spikes".
func NewSpikyLatency(base, spike LatencyProvider, probability float64, seed int64) *SpikyLatency {
	return &SpikyLatency{base: base, spike: spike, spikeProbability: probability, rng: rand.New(rand.NewSource(seed))}
}

// Delay implements LatencyProvider.
func (s *SpikyLatency) Delay() time.Duration {
	if s.base == nil {
		return 0
	}
	if s.spike != nil && s.spikeProbability > 0 && s.rng.Float64() < s.spikeProbability {
		return s.spike.Delay()
	}
	return s.base.Delay()
}
