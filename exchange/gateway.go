package exchange

import "sync"

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
	Running    bool
	Mu         sync.Mutex
}

func NewClientGateway(clientID uint64) *ClientGateway {
	return &ClientGateway{
		ClientID:   clientID,
		RequestCh:  make(chan Request, RequestChSize),
		ResponseCh: make(chan Response, ResponseChSize),
		MarketData: make(chan *MarketDataMsg, MarketDataChSize),
		Running:    true,
	}
}

func (g *ClientGateway) Close() {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	if g.Running {
		close(g.RequestCh)
		close(g.ResponseCh)
		close(g.MarketData)
		g.Running = false
	}
}
