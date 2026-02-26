package exchange

import "sync/atomic"

const (
	RequestChSize    = 10000
	ResponseChSize   = 10000
	MarketDataChSize = 10000
)

type ClientGateway struct {
	ClientID   uint64
	RequestCh  chan Request
	ResponseCh chan Response
	MarketData chan *MarketDataMsg
	running    atomic.Bool
}

func NewClientGateway(clientID uint64) *ClientGateway {
	g := &ClientGateway{
		ClientID:   clientID,
		RequestCh:  make(chan Request, RequestChSize),
		ResponseCh: make(chan Response, ResponseChSize),
		MarketData: make(chan *MarketDataMsg, MarketDataChSize),
	}
	g.running.Store(true)
	return g
}

// NewClientGatewayFromChannels creates a ClientGateway backed by existing channels.
// Used when wrapping channels (e.g. a delayed gateway) behind the ClientGateway interface.
func NewClientGatewayFromChannels(clientID uint64, req chan Request, resp chan Response, md chan *MarketDataMsg) *ClientGateway {
	g := &ClientGateway{
		ClientID:   clientID,
		RequestCh:  req,
		ResponseCh: resp,
		MarketData: md,
	}
	g.running.Store(true)
	return g
}

func (g *ClientGateway) IsRunning() bool {
	return g.running.Load()
}

func (g *ClientGateway) ID() uint64 { return g.ClientID }

// Send submits a request non-blocking. Drops silently if the gateway is closed or the channel is full.
func (g *ClientGateway) Send(req Request) {
	if !g.IsRunning() {
		return
	}
	select {
	case g.RequestCh <- req:
	default:
	}
}

func (g *ClientGateway) Responses() <-chan Response             { return g.ResponseCh }
func (g *ClientGateway) MarketDataCh() <-chan *MarketDataMsg    { return g.MarketData }
func (g *ClientGateway) MarketDataChan() chan *MarketDataMsg     { return g.MarketData }

func (g *ClientGateway) Close() {
	if !g.running.CompareAndSwap(true, false) {
		return
	}
	close(g.RequestCh)
}
