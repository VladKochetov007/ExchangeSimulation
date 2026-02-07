package simulation

import (
	"exchange_sim/exchange"
	"sync/atomic"
	"time"
)

type LatencyMode uint8

const (
	LatencyRequest    LatencyMode = 1 << 0
	LatencyResponse   LatencyMode = 1 << 1
	LatencyMarketData LatencyMode = 1 << 2
	LatencyAll                    = LatencyRequest | LatencyResponse | LatencyMarketData
)

type LatencyConfig struct {
	RequestLatency    LatencyProvider
	ResponseLatency   LatencyProvider
	MarketDataLatency LatencyProvider
	Mode              LatencyMode
	Clock             Clock
}

type DelayedGateway struct {
	gateway      *exchange.ClientGateway
	config       LatencyConfig
	requestCh    chan exchange.Request
	responseCh   chan exchange.Response
	marketDataCh chan *exchange.MarketDataMsg
	running      atomic.Bool
	stopCh       chan struct{}
}

func NewDelayedGateway(gateway *exchange.ClientGateway, config LatencyConfig) *DelayedGateway {
	if config.Clock == nil {
		config.Clock = &RealClock{}
	}
	if config.RequestLatency == nil {
		config.RequestLatency = NewConstantLatency(0)
	}
	if config.ResponseLatency == nil {
		config.ResponseLatency = NewConstantLatency(0)
	}
	if config.MarketDataLatency == nil {
		config.MarketDataLatency = NewConstantLatency(0)
	}

	return &DelayedGateway{
		gateway:      gateway,
		config:       config,
		requestCh:    make(chan exchange.Request, 100),
		responseCh:   make(chan exchange.Response, 100),
		marketDataCh: make(chan *exchange.MarketDataMsg, 1000),
		stopCh:       make(chan struct{}),
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
	if !d.running.CompareAndSwap(true, false) {
		return
	}
	close(d.stopCh)
}

func (d *DelayedGateway) RequestCh() chan<- exchange.Request {
	return d.requestCh
}

func (d *DelayedGateway) ResponseCh() <-chan exchange.Response {
	return d.responseCh
}

func (d *DelayedGateway) MarketData() <-chan *exchange.MarketDataMsg {
	return d.marketDataCh
}

func (d *DelayedGateway) forwardRequests() {
	for {
		select {
		case <-d.stopCh:
			return
		case req := <-d.requestCh:
			if d.config.Mode&LatencyRequest != 0 {
				delay := d.config.RequestLatency.Delay()
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			// Use select/default to avoid panic if gateway is closed
			select {
			case d.gateway.RequestCh <- req:
			default:
				// Gateway closed, silently drop request
			}
		}
	}
}

func (d *DelayedGateway) forwardResponses() {
	for {
		select {
		case <-d.stopCh:
			return
		case resp := <-d.gateway.ResponseCh:
			if d.config.Mode&LatencyResponse != 0 {
				delay := d.config.ResponseLatency.Delay()
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			select {
			case d.responseCh <- resp:
			default:
			}
		}
	}
}

func (d *DelayedGateway) forwardMarketData() {
	for {
		select {
		case <-d.stopCh:
			return
		case msg := <-d.gateway.MarketData:
			if d.config.Mode&LatencyMarketData != 0 {
				delay := d.config.MarketDataLatency.Delay()
				if delay > 0 {
					time.Sleep(delay)
				}
			}
			select {
			case d.marketDataCh <- msg:
			default:
			}
		}
	}
}

// ToClientGateway returns a standard ClientGateway that uses this delayed gateway's channels.
// This allows actors to use the delayed gateway as if it were a direct connection.
func (d *DelayedGateway) ToClientGateway() *exchange.ClientGateway {
	return &exchange.ClientGateway{
		ClientID:   d.gateway.ClientID,
		RequestCh:  d.requestCh,
		ResponseCh: d.responseCh,
		MarketData: d.marketDataCh,
		Running:    true,
	}
}
