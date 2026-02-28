package simulation

import (
	"sync"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

// Compile-time proof that *Venue satisfies the types.Venue contract.
var _ types.Venue = (*Venue)(nil)

// Venue pairs a trading venue with optional per-channel latency configuration.
type Venue struct {
	Market  types.Venue
	Latency LatencyConfig

	delayed []*DelayedGateway
	mu      sync.Mutex
}

// NewExchangeVenue creates a Venue backed by an *exchange.Exchange.
func NewExchangeVenue(ex *exchange.Exchange, latency LatencyConfig) *Venue {
	return &Venue{Market: ex, Latency: latency}
}

// ConnectClient registers clientID on the venue and wraps the resulting gateway
// with latency if any LatencyConfig field is non-nil. Returns the (possibly delayed)
// gateway ready for use by actors.
func (v *Venue) ConnectClient(clientID uint64, balances map[string]int64, fee exchange.FeeModel) actor.Gateway {
	gw := v.Market.ConnectClient(clientID, balances, fee)
	if v.Latency.Request == nil && v.Latency.Response == nil && v.Latency.MarketData == nil {
		return gw
	}
	d := NewDelayedGateway(gw, v.Latency.Request, v.Latency.Response, v.Latency.MarketData)
	d.Start()
	v.mu.Lock()
	v.delayed = append(v.delayed, d)
	v.mu.Unlock()
	return d
}

// Shutdown stops all delayed gateways and shuts down the underlying venue.
func (v *Venue) Shutdown() {
	v.mu.Lock()
	delayed := v.delayed
	v.mu.Unlock()

	for _, d := range delayed {
		d.Stop()
	}
	v.Market.Shutdown()
}

// IsRunning delegates to the underlying venue.
func (v *Venue) IsRunning() bool {
	return v.Market.IsRunning()
}
