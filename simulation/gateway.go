package simulation

import (
	"sync"
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

// DelayedGateway wraps any actor.Gateway with independent per-channel latency.
// Nil latency field = passthrough (no delay) for that channel.
// Implements actor.Gateway.
//
// Two delivery modes:
//   - Wall-clock (default): forwarding goroutines sleep Delay() before
//     delivering. Only meaningful with a real-time clock.
//   - Scheduled (UseScheduler): messages are delivered at simClock+Delay()
//     via the EventScheduler, preserving per-channel FIFO order. This is the
//     correct mode under a SimulatedClock, where a wall sleep bears no
//     relation to simulation time.
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

	scheduler *EventScheduler
	clock     types.Clock
	// last scheduled delivery time per channel, so random latency draws can
	// never reorder a session's message stream (network sessions are FIFO).
	reqMu      sync.Mutex
	lastReqAt  int64
	respMu     sync.Mutex
	lastRespAt int64
	mdMu       sync.Mutex
	lastMDAt   int64
}

// Idle reports whether this wrapper and the gateway beneath it have nothing
// queued. Messages already scheduled for a future simulated time are NOT
// counted: they fire only when the clock advances, so waiting for them would
// deadlock the very barrier that is trying to decide whether to advance.
func (d *DelayedGateway) Idle() bool {
	if len(d.requestCh) > 0 || len(d.responseCh) > 0 || len(d.marketDataCh) > 0 {
		return false
	}
	if inner, ok := d.inner.(interface{ Idle() bool }); ok {
		return inner.Idle()
	}
	return true
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

// UseScheduler switches to scheduled delivery at exact simulation times.
// Must be called before Start.
func (d *DelayedGateway) UseScheduler(s *EventScheduler, c types.Clock) {
	d.scheduler = s
	d.clock = c
}

func (d *DelayedGateway) Start() {
	if !d.running.CompareAndSwap(false, true) {
		return
	}
	if d.scheduler != nil {
		go d.scheduleResponses()
		go d.scheduleMarketData()
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
	if d.scheduler != nil {
		at := d.deliveryTime(&d.reqMu, &d.lastReqAt, d.RequestLatency)
		d.scheduler.Schedule(at, func() {
			if d.running.Load() {
				d.inner.Send(req)
			}
		})
		return
	}
	select {
	case d.requestCh <- req:
	default:
	}
}

// deliveryTime draws the channel's latency and returns a monotonically
// non-decreasing delivery timestamp (FIFO within the channel).
func (d *DelayedGateway) deliveryTime(mu *sync.Mutex, lastAt *int64, lat LatencyProvider) int64 {
	var delay time.Duration
	if lat != nil {
		delay = lat.Delay()
	}
	at := d.clock.NowUnixNano() + delay.Nanoseconds()
	mu.Lock()
	if at < *lastAt {
		at = *lastAt
	}
	*lastAt = at
	mu.Unlock()
	return at
}

func (d *DelayedGateway) scheduleResponses() {
	for {
		select {
		case <-d.stopCh:
			return
		case resp, ok := <-d.inner.Responses():
			if !ok {
				return
			}
			at := d.deliveryTime(&d.respMu, &d.lastRespAt, d.ResponseLatency)
			d.scheduler.Schedule(at, func() {
				if !d.running.Load() {
					return
				}
				select {
				case d.responseCh <- resp:
				default:
				}
			})
		}
	}
}

func (d *DelayedGateway) scheduleMarketData() {
	for {
		select {
		case <-d.stopCh:
			return
		case msg, ok := <-d.inner.MarketDataCh():
			if !ok {
				return
			}
			at := d.deliveryTime(&d.mdMu, &d.lastMDAt, d.MarketDataLatency)
			d.scheduler.Schedule(at, func() {
				if !d.running.Load() {
					return
				}
				select {
				case d.marketDataCh <- msg:
				default:
				}
			})
		}
	}
}

func (d *DelayedGateway) Responses() <-chan exchange.Response          { return d.responseCh }
func (d *DelayedGateway) MarketDataCh() <-chan *exchange.MarketDataMsg { return d.marketDataCh }
func (d *DelayedGateway) IsRunning() bool                              { return d.running.Load() }

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
