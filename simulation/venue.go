package simulation

import (
	"fmt"
	"sync"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

type deterministicPhaseVenue interface {
	DeterministicPhasesEnabled() bool
	PumpDeterministicPhase() bool
	DrainDeterministicEgress() bool
}

// Compile-time proof that *Mount satisfies the types.Venue contract.
var _ types.Venue = (*Mount)(nil)

// Mount pairs a trading venue with optional per-channel latency configuration.
type Mount struct {
	Market  types.Venue
	Latency LatencyConfig

	delayed []*DelayedGateway
	mu      sync.Mutex
}

// NewMount creates a Mount backed by an *exchange.Exchange.
func NewMount(ex *exchange.Exchange, latency LatencyConfig) *Mount {
	return &Mount{Market: ex, Latency: latency}
}

// ConnectNewClient registers clientID on the venue and wraps the resulting gateway
// with latency if any LatencyConfig field is non-nil. Returns the (possibly delayed)
// gateway ready for use by actors.
func (m *Mount) ConnectNewClient(clientID uint64, balances map[string]int64, fee exchange.FeeModel) actor.Gateway {
	gw := m.Market.ConnectNewClient(clientID, balances, fee)
	if m.Latency.Request == nil && m.Latency.Response == nil && m.Latency.MarketData == nil {
		return gw
	}
	d := NewDelayedGateway(gw, m.Latency.Request, m.Latency.Response, m.Latency.MarketData)
	if m.Latency.Scheduler != nil && m.Latency.Clock != nil {
		d.UseScheduler(m.Latency.Scheduler, m.Latency.Clock)
	}
	d.Start()
	m.mu.Lock()
	m.delayed = append(m.delayed, d)
	m.mu.Unlock()
	return d
}

// Idle reports whether every delayed gateway on this mount has drained. A
// mount with no latency wrapper holds nothing of its own in flight.
func (m *Mount) Idle() bool {
	m.mu.Lock()
	delayed := m.delayed
	m.mu.Unlock()
	for _, d := range delayed {
		if !d.Idle() {
			return false
		}
	}
	// The venue itself holds the request queues of every gateway handed out
	// without a latency wrapper, which no delayed gateway can see.
	if idler, ok := m.Market.(interface{ Idle() bool }); ok {
		return idler.Idle()
	}
	return true
}

// Drain advances opt-in deterministic venue ingress. Regular asynchronous
// venues return false and preserve their production execution path.
func (m *Mount) Drain() bool {
	if drainer, ok := m.Market.(interface{ DrainIngress() bool }); ok {
		return drainer.DrainIngress()
	}
	return false
}

// ValidateDeterministicPhases rejects paths whose event ordering remains under
// goroutine control. Latency wrappers are intentionally out of scope for this
// first phase runtime: their scheduled forwarding goroutines need a separate
// ordered delivery design rather than being silently treated as deterministic.
func (m *Mount) ValidateDeterministicPhases() error {
	if m.Latency.Request != nil || m.Latency.Response != nil || m.Latency.MarketData != nil {
		return fmt.Errorf("simulation: deterministic phases require a direct mount (latency configured)")
	}
	m.mu.Lock()
	delayed := len(m.delayed)
	m.mu.Unlock()
	if delayed != 0 {
		return fmt.Errorf("simulation: deterministic phases require a direct mount (%d delayed gateways)", delayed)
	}
	venue, ok := m.Market.(deterministicPhaseVenue)
	if !ok || !venue.DeterministicPhasesEnabled() {
		return fmt.Errorf("simulation: venue does not opt in to deterministic phases")
	}
	return nil
}

// PumpDeterministicPhase runs due exchange-owned jobs, such as snapshots and
// automation, in the venue's explicit phase order.
func (m *Mount) PumpDeterministicPhase() bool {
	if venue, ok := m.Market.(deterministicPhaseVenue); ok {
		return venue.PumpDeterministicPhase()
	}
	return false
}

// DrainDeterministicEgress moves venue responses to actor inboxes in the
// venue-defined deterministic order.
func (m *Mount) DrainDeterministicEgress() bool {
	if venue, ok := m.Market.(deterministicPhaseVenue); ok {
		return venue.DrainDeterministicEgress()
	}
	return false
}

// Shutdown stops all delayed gateways and shuts down the underlying venue.
func (m *Mount) Shutdown() {
	m.mu.Lock()
	delayed := m.delayed
	m.mu.Unlock()

	for _, d := range delayed {
		d.Stop()
	}
	m.Market.Shutdown()
}

// IsRunning delegates to the underlying venue.
func (m *Mount) IsRunning() bool {
	return m.Market.IsRunning()
}
