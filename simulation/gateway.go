package simulation

import (
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// DelayedGateway wraps any actor.Gateway with independent per-channel latency.
// Nil latency field = passthrough (no delay) for that channel.
// Implements actor.Gateway.
type DelayedGateway struct {
	RequestLatency    LatencyProvider
	ResponseLatency   LatencyProvider
	MarketDataLatency LatencyProvider

	inner        actor.Gateway
	requestCh    chan exchange.Request
	responseCh   chan exchange.Response
	marketDataCh chan *exchange.MarketDataMsg
	stopCh       chan struct{}
	running      atomic.Bool
}

func NewDelayedGateway(inner actor.Gateway, reqLat, respLat, mdLat LatencyProvider) *DelayedGateway {
	return &DelayedGateway{
		RequestLatency:    reqLat,
		ResponseLatency:   respLat,
		MarketDataLatency: mdLat,
		inner:             inner,
		requestCh:         make(chan exchange.Request, exchange.RequestChSize),
		responseCh:        make(chan exchange.Response, exchange.ResponseChSize),
		marketDataCh:      make(chan *exchange.MarketDataMsg, exchange.MarketDataChSize),
		stopCh:            make(chan struct{}),
	}
}

func (d *DelayedGateway) Start() {
	if !d.running.CompareAndSwap(false, true) {
		return
	}
	go d.forwardRequests()
	go d.forwardResponses()
	go d.forwardMarketData()
}

func (d *DelayedGateway) Stop() {
	if d.running.CompareAndSwap(true, false) {
		close(d.stopCh)
	}
}

// actor.Gateway implementation

func (d *DelayedGateway) ID() uint64 { return d.inner.ID() }

func (d *DelayedGateway) Send(req exchange.Request) {
	if !d.running.Load() {
		return
	}
	select {
	case d.requestCh <- req:
	default:
	}
}

func (d *DelayedGateway) Responses() <-chan exchange.Response             { return d.responseCh }
func (d *DelayedGateway) MarketDataCh() <-chan *exchange.MarketDataMsg    { return d.marketDataCh }
func (d *DelayedGateway) IsRunning() bool                                 { return d.running.Load() }

func (d *DelayedGateway) forwardRequests() {
	for {
		select {
		case <-d.stopCh:
			return
		case req, ok := <-d.requestCh:
			if !ok {
				return
			}
			if d.RequestLatency != nil {
				time.Sleep(d.RequestLatency.Delay())
			}
			d.inner.Send(req)
		}
	}
}

func (d *DelayedGateway) forwardResponses() {
	for {
		select {
		case <-d.stopCh:
			return
		case resp, ok := <-d.inner.Responses():
			if !ok {
				return
			}
			if d.ResponseLatency != nil {
				time.Sleep(d.ResponseLatency.Delay())
			}
			select {
			case d.responseCh <- resp:
			case <-d.stopCh:
				return
			}
		}
	}
}

func (d *DelayedGateway) forwardMarketData() {
	for {
		select {
		case <-d.stopCh:
			return
		case msg, ok := <-d.inner.MarketDataCh():
			if !ok {
				return
			}
			if d.MarketDataLatency != nil {
				time.Sleep(d.MarketDataLatency.Delay())
			}
			select {
			case d.marketDataCh <- msg:
			case <-d.stopCh:
				return
			}
		}
	}
}
