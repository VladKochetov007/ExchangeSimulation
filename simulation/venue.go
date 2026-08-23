package simulation

import (
	"fmt"
	"strings"
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
	// phaseMode records that the runner has taken courier work away from
	// goroutines. A client that connects after that point must be created in
	// the same mode, or its messages arrive when the host scheduler happens to
	// run its couriers rather than when simulated time says they should.
	phaseMode bool
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
	request, response, marketData := m.Latency.Request, m.Latency.Response, m.Latency.MarketData
	if m.Latency.PerClient != nil {
		request, response, marketData = m.Latency.PerClient(clientID)
	}
	if request == nil && response == nil && marketData == nil {
		return gw
	}
	d := NewDelayedGateway(gw, request, response, marketData)
	d.SetLatencyTelemetry(m.Latency.Telemetry, m.Latency.TelemetryLabel)
	// A link is a participant session, never a role class: a per-link ordinal
	// must not silently combine observations from two makers on one venue.
	receiptLink := m.Latency.ReceiptLink
	if receiptLink != "" {
		receiptLink = fmt.Sprintf("%s/client/%d", receiptLink, clientID)
	}
	d.SetMarketDataReceiptRecorder(m.Latency.MarketDataReceipts, m.Latency.ReceiptSourceVenue, receiptLink, m.Latency.ReceiptRole)
	if m.Latency.Scheduler != nil && m.Latency.Clock != nil {
		d.UseScheduler(m.Latency.Scheduler, m.Latency.Clock)
	}
	// A scheduler and a simulated clock mean simulated time is authoritative,
	// and a courier that sleeps in real time before delivering is measuring
	// the host rather than the model: the same message is then stamped with
	// whatever simulated instant the process happened to reach, so two runs of
	// one seed disagree. Whenever the link can deliver through the scheduler,
	// it must, from the moment the gateway exists rather than from whenever
	// the runner gets around to switching it over.
	if m.Latency.Scheduler != nil && m.Latency.Clock != nil {
		// Before Start, so the goroutine couriers are never launched at all
		// rather than launched and then stopped.
		if err := d.EnableDeterministicPhases(); err != nil {
			panic(fmt.Sprintf("simulation: client %d cannot join deterministic phases: %v", clientID, err))
		}
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

// EnableDeterministicPhases switches every scheduler-backed wrapper to the
// runner-owned latency courier. It is called before actors start, after all
// clients have connected and their wrappers exist.
func (m *Mount) EnableDeterministicPhases() error {
	m.mu.Lock()
	m.phaseMode = true
	delayed := append([]*DelayedGateway(nil), m.delayed...)
	m.mu.Unlock()
	for _, d := range delayed {
		if err := d.EnableDeterministicPhases(); err != nil {
			return err
		}
	}
	return nil
}

// ValidateDeterministicPhases rejects paths whose event ordering remains under
// goroutine control. Scheduler-backed delayed gateways are valid only after
// EnableDeterministicPhases has transferred their courier work to the runner.
func (m *Mount) ValidateDeterministicPhases() error {
	m.mu.Lock()
	delayed := append([]*DelayedGateway(nil), m.delayed...)
	m.mu.Unlock()
	for _, d := range delayed {
		if !d.deterministicPhasesEnabled() {
			return fmt.Errorf("simulation: delayed gateway %d does not use deterministic phase delivery", d.ID())
		}
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
	processed := false
	if venue, ok := m.Market.(deterministicPhaseVenue); ok {
		processed = venue.PumpDeterministicPhase()
	}
	m.mu.Lock()
	delayed := append([]*DelayedGateway(nil), m.delayed...)
	m.mu.Unlock()
	for _, d := range delayed {
		if d.PumpDeterministicPhase() {
			processed = true
		}
	}
	return processed
}

// DrainDeterministicEgress moves venue responses to actor inboxes in the
// venue-defined deterministic order.
func (m *Mount) DrainDeterministicEgress() bool {
	processed := false
	if venue, ok := m.Market.(deterministicPhaseVenue); ok {
		processed = venue.DrainDeterministicEgress()
	}
	m.mu.Lock()
	delayed := append([]*DelayedGateway(nil), m.delayed...)
	m.mu.Unlock()
	for _, d := range delayed {
		// Exchange ingress in this phase may have generated fresh egress.
		if d.PumpDeterministicPhase() {
			processed = true
		}
		if d.DrainDeterministicPhaseEgress() {
			processed = true
		}
	}
	return processed
}

// EgressBlocked reports whether every message this mount still holds is
// waiting only on a full actor inbox.
func (m *Mount) EgressBlocked() bool {
	m.mu.Lock()
	delayed := append([]*DelayedGateway(nil), m.delayed...)
	m.mu.Unlock()
	blocked := false
	for _, d := range delayed {
		if d.Idle() {
			continue
		}
		if !d.EgressBlocked() {
			return false
		}
		blocked = true
	}
	// The venue's own queues are deliberately not required to be empty. A
	// gateway whose actor inbox is full stops draining, which backs the
	// responses up into the exchange behind it, so the venue reports work
	// pending for exactly the reason the gateway does. Requiring it idle here
	// would classify the commonest case of backpressure as a deadlock.
	return blocked
}

// PendingDescription explains why a mount is not idle, which is the difference
// between a slow consumer and a deadlock when the runner reports a stall.
func (m *Mount) PendingDescription() string {
	m.mu.Lock()
	delayed := append([]*DelayedGateway(nil), m.delayed...)
	m.mu.Unlock()
	parts := make([]string, 0, len(delayed)+1)
	for _, d := range delayed {
		if d.Idle() {
			continue
		}
		parts = append(parts, d.PendingDescription())
	}
	if idler, ok := m.Market.(interface{ Idle() bool }); ok && !idler.Idle() {
		parts = append(parts, "venue")
	}
	return strings.Join(parts, " ")
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
