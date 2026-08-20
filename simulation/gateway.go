package simulation

import (
	"fmt"
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

	// phaseMode replaces scheduler-mode forwarding goroutines with runner
	// pumps. It is enabled before actors start, so latency arrival ordering is
	// model-defined rather than chosen by host goroutine scheduling.
	phaseMode   atomic.Bool
	phaseStopCh chan struct{}
	phaseWG     sync.WaitGroup
	phaseLifeMu sync.Mutex
	phaseMu     sync.Mutex
	phaseResp   []exchange.Response
	phaseMD     []*exchange.MarketDataMsg
}

// Idle reports whether this wrapper and the gateway beneath it have nothing
// queued. Messages already scheduled for a future simulated time are NOT
// counted: they fire only when the clock advances, so waiting for them would
// deadlock the very barrier that is trying to decide whether to advance.
func (d *DelayedGateway) Idle() bool {
	if len(d.requestCh) > 0 || len(d.responseCh) > 0 || len(d.marketDataCh) > 0 {
		return false
	}
	d.phaseMu.Lock()
	phaseQueued := len(d.phaseResp) != 0 || len(d.phaseMD) != 0
	d.phaseMu.Unlock()
	if phaseQueued {
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
		phaseStopCh:       make(chan struct{}),
	}
}

// UseScheduler switches to scheduled delivery at exact simulation times.
// Must be called before Start.
func (d *DelayedGateway) UseScheduler(s *EventScheduler, c types.Clock) {
	d.scheduler = s
	d.clock = c
}

// EnableDeterministicPhases moves scheduler-backed delivery onto the
// simulation runner. It must be enabled before actors start producing work.
// The ordinary scheduled forwarders are stopped and joined so a message can
// never be assigned a delivery sequence by whichever host goroutine runs
// first.
func (d *DelayedGateway) EnableDeterministicPhases() error {
	if d.scheduler == nil || d.clock == nil {
		return fmt.Errorf("simulation: delayed gateway %d needs scheduler and clock for deterministic phases", d.ID())
	}

	d.phaseLifeMu.Lock()
	enabled := d.phaseMode.CompareAndSwap(false, true)
	if enabled {
		close(d.phaseStopCh)
	}
	d.phaseLifeMu.Unlock()
	if enabled {
		d.phaseWG.Wait()
	}
	return nil
}

func (d *DelayedGateway) deterministicPhasesEnabled() bool {
	return d.phaseMode.Load() && d.scheduler != nil && d.clock != nil
}

func (d *DelayedGateway) Start() {
	if !d.running.CompareAndSwap(false, true) {
		return
	}
	if d.scheduler != nil {
		d.phaseLifeMu.Lock()
		phaseMode := d.phaseMode.Load()
		if !phaseMode {
			d.phaseWG.Add(2)
		}
		d.phaseLifeMu.Unlock()
		if phaseMode {
			return
		}
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
		if d.phaseMode.Load() && at <= d.clock.NowUnixNano() {
			d.inner.Send(req)
			return
		}
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
	defer d.phaseWG.Done()
	for {
		select {
		case <-d.stopCh:
			return
		case <-d.phaseStopCh:
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
	defer d.phaseWG.Done()
	for {
		select {
		case <-d.stopCh:
			return
		case <-d.phaseStopCh:
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

// PumpDeterministicPhase drains raw exchange egress and schedules its
// arrival. Responses are intentionally handled before market data, and each
// raw channel remains FIFO. Future arrivals become scheduler callbacks; an
// arrival due now is appended directly because scheduling a same-time event
// after EventScheduler.ProcessUntil has returned would defer it one clock
// step.
func (d *DelayedGateway) PumpDeterministicPhase() bool {
	if !d.deterministicPhasesEnabled() || !d.running.Load() {
		return false
	}
	processed := false
	for {
		select {
		case resp, ok := <-d.inner.Responses():
			if !ok {
				return processed
			}
			d.schedulePhaseResponse(resp)
			processed = true
		default:
			goto marketData
		}
	}

marketData:
	for {
		select {
		case msg, ok := <-d.inner.MarketDataCh():
			if !ok {
				return processed
			}
			d.schedulePhaseMarketData(msg)
			processed = true
		default:
			return processed
		}
	}
}

func (d *DelayedGateway) schedulePhaseResponse(resp exchange.Response) {
	at := d.deliveryTime(&d.respMu, &d.lastRespAt, d.ResponseLatency)
	if at <= d.clock.NowUnixNano() {
		d.phaseMu.Lock()
		d.phaseResp = append(d.phaseResp, resp)
		d.phaseMu.Unlock()
		return
	}
	d.scheduler.Schedule(at, func() {
		if !d.running.Load() {
			return
		}
		d.phaseMu.Lock()
		d.phaseResp = append(d.phaseResp, resp)
		d.phaseMu.Unlock()
	})
}

func (d *DelayedGateway) schedulePhaseMarketData(msg *exchange.MarketDataMsg) {
	at := d.deliveryTime(&d.mdMu, &d.lastMDAt, d.MarketDataLatency)
	if at <= d.clock.NowUnixNano() {
		d.phaseMu.Lock()
		d.phaseMD = append(d.phaseMD, msg)
		d.phaseMu.Unlock()
		return
	}
	d.scheduler.Schedule(at, func() {
		if !d.running.Load() {
			return
		}
		d.phaseMu.Lock()
		d.phaseMD = append(d.phaseMD, msg)
		d.phaseMu.Unlock()
	})
}

// EgressBlocked reports that the gateway holds messages whose delivery time
// has arrived and cannot hand them over because the actor's inbox is full.
//
// This is backpressure, not a stall: the actor consumes on its own clock and
// the messages are delivered as soon as it does. The distinction matters to
// the runner, which would otherwise read a slow consumer as a deadlocked
// simulation and refuse to advance time.
func (d *DelayedGateway) EgressBlocked() bool {
	if !d.deterministicPhasesEnabled() || !d.running.Load() {
		return false
	}
	d.phaseMu.Lock()
	defer d.phaseMu.Unlock()
	if len(d.phaseResp) > 0 && len(d.responseCh) == cap(d.responseCh) {
		return true
	}
	return len(d.phaseMD) > 0 && len(d.marketDataCh) == cap(d.marketDataCh)
}

// PendingDescription reports what this gateway is holding and where.
func (d *DelayedGateway) PendingDescription() string {
	d.phaseMu.Lock()
	resp, md := len(d.phaseResp), len(d.phaseMD)
	d.phaseMu.Unlock()
	return fmt.Sprintf("gw%d[req %d/%d resp %d/%d md %d/%d phaseResp %d phaseMD %d]",
		d.ID(), len(d.requestCh), cap(d.requestCh), len(d.responseCh), cap(d.responseCh),
		len(d.marketDataCh), cap(d.marketDataCh), resp, md)
}

// DrainDeterministicPhaseEgress moves ready delayed messages into the actor
// inbox. A full inbox retains the message instead of silently dropping an
// acknowledgement or fill; the runner will retry it at the same timestamp.
func (d *DelayedGateway) DrainDeterministicPhaseEgress() bool {
	if !d.deterministicPhasesEnabled() || !d.running.Load() {
		return false
	}
	d.phaseMu.Lock()
	defer d.phaseMu.Unlock()
	processed := false
	for len(d.phaseResp) > 0 {
		select {
		case d.responseCh <- d.phaseResp[0]:
			d.phaseResp = d.phaseResp[1:]
			processed = true
		default:
			return processed
		}
	}
	for len(d.phaseMD) > 0 {
		select {
		case d.marketDataCh <- d.phaseMD[0]:
			d.phaseMD = d.phaseMD[1:]
			processed = true
		default:
			return processed
		}
	}
	return processed
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
