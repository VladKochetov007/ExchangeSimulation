package exchange

import (
	"sync"
	"sync/atomic"
)

// RequestHandler is implemented by any type that can process venue requests.
// Used with BaseMarket.SetHandler to achieve virtual dispatch that Go's
// embedding alone cannot provide.
type RequestHandler interface {
	HandleRequest(clientID uint64, req Request) Response
}

// BaseMarket handles gateway lifecycle for custom venue implementations.
// Embed it in a struct to get ConnectClient, Shutdown, and IsRunning for free,
// then implement HandleRequest and call SetHandler(self) in the constructor
// to route requests to the embedding type.
type BaseMarket struct {
	mu      sync.Mutex
	clients map[uint64]*ClientGateway
	running atomic.Bool
	handler RequestHandler
}

func NewBaseMarket() *BaseMarket {
	m := &BaseMarket{clients: make(map[uint64]*ClientGateway)}
	m.running.Store(true)
	m.handler = m // default: reject everything
	return m
}

// SetHandler wires the request dispatcher. Call with the embedding struct
// to achieve virtual dispatch:
//
//	m.BaseMarket.SetHandler(m)
func (b *BaseMarket) SetHandler(h RequestHandler) { b.handler = h }

func (b *BaseMarket) ConnectClient(id uint64, _ map[string]int64, _ FeeModel) Gateway {
	gw := NewClientGateway(id)
	b.mu.Lock()
	b.clients[id] = gw
	b.mu.Unlock()
	go b.dispatchLoop(gw)
	return gw
}

func (b *BaseMarket) dispatchLoop(gw *ClientGateway) {
	for req := range gw.RequestCh {
		resp := b.handler.HandleRequest(gw.ClientID, req)
		select {
		case gw.ResponseCh <- resp:
		default:
		}
	}
}

// HandleRequest is the default handler: rejects all requests with RejectUnknownRequest.
func (b *BaseMarket) HandleRequest(_ uint64, req Request) Response {
	reqID := uint64(0)
	if req.OrderReq != nil {
		reqID = req.OrderReq.RequestID
	} else if req.CancelReq != nil {
		reqID = req.CancelReq.RequestID
	} else if req.QueryReq != nil {
		reqID = req.QueryReq.RequestID
	}
	return Response{RequestID: reqID, Success: false, Error: RejectUnknownRequest}
}

func (b *BaseMarket) Shutdown()       { b.running.Store(false) }
func (b *BaseMarket) IsRunning() bool { return b.running.Load() }
