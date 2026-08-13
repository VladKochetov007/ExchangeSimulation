package exchange

import (
	"sync"
	"sync/atomic"
	"time"
)

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

	// outbox decouples response delivery from the engine: sendResponse can
	// block indefinitely on a slow consumer, and blocking inside the exchange
	// write lock stalls every book and client (the LMAX rule: no external
	// calls inside the business logic). enqueueResponse appends here and a
	// single lazy deliverer goroutine drains in FIFO order, so per-gateway
	// ordering (accept before its fills' successors, cancels in sequence) is
	// exactly the enqueue order.
	outMu      sync.Mutex
	outbox     []Response
	delivering bool

	// closeMu serializes Close against Send. Checking IsRunning and then
	// sending is a time-of-check-to-time-of-use race: Close can retire the
	// gateway and close RequestCh in the window between them, and a send on a
	// closed channel panics rather than merely losing the request.
	closeMu sync.RWMutex
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
	// Read lock: concurrent senders still proceed in parallel, but Close
	// cannot run between the liveness check and the send. The send itself is
	// non-blocking, so holding the lock cannot stall a shutdown.
	g.closeMu.RLock()
	defer g.closeMu.RUnlock()
	if !g.IsRunning() {
		return
	}
	select {
	case g.RequestCh <- req:
	default:
	}
}

// enqueueResponse queues resp for asynchronous FIFO delivery, spawning the
// deliverer if none is active. Safe to call while holding the exchange lock —
// it never blocks on the consumer. Nil-safe like sendResponse.
func (g *ClientGateway) enqueueResponse(resp Response) {
	if g == nil {
		return
	}
	g.outMu.Lock()
	g.outbox = append(g.outbox, resp)
	spawn := !g.delivering
	if spawn {
		g.delivering = true
	}
	g.outMu.Unlock()
	if spawn {
		go g.deliverOutbox()
	}
}

func (g *ClientGateway) deliverOutbox() {
	for {
		g.outMu.Lock()
		if len(g.outbox) == 0 {
			g.delivering = false
			g.outMu.Unlock()
			return
		}
		batch := g.outbox
		g.outbox = nil
		g.outMu.Unlock()
		for _, resp := range batch {
			sendResponse(g, resp)
		}
	}
}

// Idle reports whether nothing is queued in either direction and the outbox
// deliverer has drained. Used by deterministic runners to decide the system
// has settled; scheduled-but-future deliveries are deliberately not counted,
// since those only fire when simulated time advances.
func (g *ClientGateway) Idle() bool {
	if len(g.RequestCh) > 0 || len(g.ResponseCh) > 0 || len(g.MarketData) > 0 {
		return false
	}
	g.outMu.Lock()
	defer g.outMu.Unlock()
	return len(g.outbox) == 0 && !g.delivering
}

func (g *ClientGateway) Responses() <-chan Response          { return g.ResponseCh }
func (g *ClientGateway) MarketDataCh() <-chan *MarketDataMsg { return g.MarketData }
func (g *ClientGateway) MarketDataChan() chan *MarketDataMsg { return g.MarketData }

func (g *ClientGateway) Close() {
	g.closeMu.Lock()
	defer g.closeMu.Unlock()
	if !g.running.CompareAndSwap(true, false) {
		return
	}
	close(g.RequestCh)
}

// sendResponse delivers resp with at-least-once semantics for live gateways:
// the fast path is a direct send; on a full channel it retries while the
// gateway is running (brief backoff so the consumer can drain) and gives up
// only once the gateway closes. One delivery policy for every response type —
// dropped accepts/cancels desynchronize actor order state permanently (the
// ghost-order bug class), and unconditionally blocking sends hang forever on
// gateways whose consumer stopped without closing.
func sendResponse(g *ClientGateway, resp Response) {
	if g == nil {
		return
	}
	select {
	case g.ResponseCh <- resp:
		return
	default:
	}
	for g.IsRunning() {
		select {
		case g.ResponseCh <- resp:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}
