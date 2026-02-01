package exchange

import "sync"

const (
	RequestChSize    = 1000
	ResponseChSize   = 1000
	MarketDataChSize = 10000
)

type ClientGateway struct {
	ClientID   uint64
	RequestCh  chan Request
	ResponseCh chan Response
	MarketData chan *MarketDataMsg
	Running    bool
	mu         sync.Mutex
}

func NewClientGateway(clientID uint64) *ClientGateway {
	return &ClientGateway{
		ClientID:   clientID,
		RequestCh:  make(chan Request, RequestChSize),
		ResponseCh: make(chan Response, ResponseChSize),
		MarketData: make(chan *MarketDataMsg, MarketDataChSize),
		Running:    false,
	}
}

func (g *ClientGateway) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.Running {
		close(g.RequestCh)
		close(g.ResponseCh)
		close(g.MarketData)
		g.Running = false
	}
}
