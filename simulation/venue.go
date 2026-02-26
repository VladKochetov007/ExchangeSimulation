package simulation

import (
	"sync"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// Venue pairs an exchange engine with optional per-channel latency configuration.
type Venue struct {
	Exchange *exchange.Exchange
	Latency  LatencyConfig

	delayed []*DelayedGateway
	mu      sync.Mutex
}

// ConnectClient registers clientID on the exchange and wraps the resulting gateway
// with latency if any LatencyConfig field is non-nil. Returns the (possibly delayed)
// gateway ready for use by actors.
func (v *Venue) ConnectClient(clientID uint64, balances map[string]int64, fee exchange.FeeModel) actor.Gateway {
	gw := v.Exchange.ConnectClient(clientID, balances, fee)
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

// shutdown stops all delayed gateways and shuts down the exchange.
func (v *Venue) shutdown() {
	v.mu.Lock()
	delayed := v.delayed
	v.mu.Unlock()

	for _, d := range delayed {
		d.Stop()
	}
	v.Exchange.Shutdown()
}
