package simulation

import (
	"sync"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

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
	d.Start()
	m.mu.Lock()
	m.delayed = append(m.delayed, d)
	m.mu.Unlock()
	return d
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
