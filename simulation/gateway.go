package simulation

import (
	"crypto/sha256"
	"encoding/json"
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
	// nextMDReceiptOrdinal is per delayed gateway and is assigned when an
	// ingress message is scheduled. It documents the FIFO order the courier
	// promised independently of how other links interleave at equal times.
	nextMDReceiptOrdinal uint64

	// phaseMode replaces scheduler-mode forwarding goroutines with runner
	// pumps. It is enabled before actors start, so latency arrival ordering is
	// model-defined rather than chosen by host goroutine scheduling.
	phaseMode     atomic.Bool
	phaseStopCh   chan struct{}
	phaseWG       sync.WaitGroup
	phaseLifeMu   sync.Mutex
	phaseMu       sync.Mutex
	phaseResp     []exchange.Response
	phaseMD       []phaseMarketData
	latencyStats  *LatencyStats
	latencyLabel  string
	receiptSink   *MarketDataReceiptRecorder
	receiptSource string
	receiptLink   string
	receiptMu     sync.Mutex
	frontier      MarketDataFrontier
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

// SetLatencyTelemetry installs an observation-only compact delivery sink.
// Call before actors begin producing traffic.
func (d *DelayedGateway) SetLatencyTelemetry(stats *LatencyStats, label string) {
	d.latencyStats = stats
	d.latencyLabel = label
}

// SetMarketDataReceiptRecorder installs the optional individual public-feed
// arrival recorder. It must be configured before traffic starts. The sink is
// observational: it is called only after the courier has delivered a message
// to its actor-facing inbox path.
func (d *DelayedGateway) SetMarketDataReceiptRecorder(sink *MarketDataReceiptRecorder, sourceVenue, link, role string) {
	d.receiptSink = sink
	d.receiptSource = sourceVenue
	d.receiptLink = link
	if sink != nil {
		sink.RegisterLink(sourceVenue, link, role)
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
	d.recordMarketDataDecision(req)
	if d.scheduler != nil {
		at, ticket := d.deliveryTime(&d.reqMu, &d.lastReqAt, d.RequestLatency, LatencyRequest)
		if d.phaseMode.Load() && at <= d.clock.NowUnixNano() {
			d.inner.Send(req)
			d.delivered(ticket)
			return
		}
		d.scheduler.Schedule(at, func() {
			if d.running.Load() {
				d.inner.Send(req)
				d.delivered(ticket)
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
func (d *DelayedGateway) deliveryTime(mu *sync.Mutex, lastAt *int64, lat LatencyProvider, channel LatencyChannel) (int64, latencyTicket) {
	var delay time.Duration
	if lat != nil {
		delay = lat.Delay()
	}
	sourceAt := d.clock.NowUnixNano()
	drawnAt := sourceAt + delay.Nanoseconds()
	at := drawnAt
	mu.Lock()
	if at < *lastAt {
		at = *lastAt
	}
	*lastAt = at
	mu.Unlock()
	return at, d.latencyStats.scheduled(d.latencyLabel, channel, sourceAt, drawnAt, at)
}

func (d *DelayedGateway) delivered(ticket latencyTicket) {
	d.latencyStats.delivered(ticket, d.clock.NowUnixNano())
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
			at, ticket := d.deliveryTime(&d.respMu, &d.lastRespAt, d.ResponseLatency, LatencyResponse)
			d.scheduler.Schedule(at, func() {
				if !d.running.Load() {
					return
				}
				select {
				case d.responseCh <- resp:
					d.delivered(ticket)
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
			at, ticket := d.deliveryTime(&d.mdMu, &d.lastMDAt, d.MarketDataLatency, LatencyMarketData)
			receipt := d.scheduleMarketDataReceipt(msg, at)
			d.scheduler.Schedule(at, func() {
				if !d.running.Load() {
					return
				}
				select {
				case d.marketDataCh <- msg:
					d.delivered(ticket)
					d.recordMarketDataReceipt(receipt)
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
	at, ticket := d.deliveryTime(&d.respMu, &d.lastRespAt, d.ResponseLatency, LatencyResponse)
	if at <= d.clock.NowUnixNano() {
		d.phaseMu.Lock()
		d.phaseResp = append(d.phaseResp, resp)
		d.phaseMu.Unlock()
		d.delivered(ticket)
		return
	}
	d.scheduler.Schedule(at, func() {
		if !d.running.Load() {
			return
		}
		d.phaseMu.Lock()
		d.phaseResp = append(d.phaseResp, resp)
		d.phaseMu.Unlock()
		d.delivered(ticket)
	})
}

func (d *DelayedGateway) schedulePhaseMarketData(msg *exchange.MarketDataMsg) {
	at, ticket := d.deliveryTime(&d.mdMu, &d.lastMDAt, d.MarketDataLatency, LatencyMarketData)
	receipt := d.scheduleMarketDataReceipt(msg, at)
	if at <= d.clock.NowUnixNano() {
		d.phaseMu.Lock()
		d.phaseMD = append(d.phaseMD, phaseMarketData{message: msg, receipt: receipt})
		d.phaseMu.Unlock()
		d.delivered(ticket)
		return
	}
	d.scheduler.Schedule(at, func() {
		if !d.running.Load() {
			return
		}
		d.phaseMu.Lock()
		d.phaseMD = append(d.phaseMD, phaseMarketData{message: msg, receipt: receipt})
		d.phaseMu.Unlock()
		d.delivered(ticket)
	})
}

type phaseMarketData struct {
	message *exchange.MarketDataMsg
	receipt scheduledMarketDataReceipt
}

type scheduledMarketDataReceipt struct {
	message  *exchange.MarketDataMsg
	schedule MarketDataSchedule
}

func (d *DelayedGateway) scheduleMarketDataReceipt(msg *exchange.MarketDataMsg, scheduledAt int64) scheduledMarketDataReceipt {
	if d.receiptSink == nil || msg == nil {
		return scheduledMarketDataReceipt{}
	}
	fingerprint, err := marketDataFingerprint(msg)
	if err != nil {
		d.receiptSink.Fail(fmt.Errorf("fingerprint market-data message: %w", err))
		return scheduledMarketDataReceipt{}
	}
	d.mdMu.Lock()
	d.nextMDReceiptOrdinal++
	ordinal := d.nextMDReceiptOrdinal
	d.mdMu.Unlock()
	schedule := MarketDataSchedule{
		ClientID:    d.ID(),
		SourceVenue: d.receiptSource,
		Link:        d.receiptLink,
		Symbol:      msg.Symbol,
		Type:        msg.Type,
		Sequence:    msg.SeqNum,
		Fingerprint: fingerprint,
		PublishedAt: msg.Timestamp,
		ScheduledAt: scheduledAt,
		LinkOrdinal: ordinal,
	}
	d.receiptSink.RecordSchedule(schedule)
	return scheduledMarketDataReceipt{message: msg, schedule: schedule}
}

func (d *DelayedGateway) recordMarketDataReceipt(ticket scheduledMarketDataReceipt) {
	if d.receiptSink == nil || ticket.message == nil || ticket.schedule.LinkOrdinal == 0 {
		return
	}
	frontier := d.receiptSink.RecordReceipt(MarketDataReceipt{MarketDataSchedule: ticket.schedule, DeliveredAt: d.clock.NowUnixNano()})
	if frontier.LinkID == 0 {
		return
	}
	d.receiptMu.Lock()
	d.frontier = frontier
	d.receiptMu.Unlock()
}

func (d *DelayedGateway) recordMarketDataDecision(req exchange.Request) {
	if d.receiptSink == nil || d.clock == nil || req.Type != exchange.ReqPlaceOrder || req.OrderReq == nil {
		return
	}
	d.receiptMu.Lock()
	frontier := d.frontier
	d.receiptMu.Unlock()
	d.receiptSink.RecordDecision(MarketDataDecision{
		ClientID:    d.ID(),
		SourceVenue: d.receiptSource,
		Link:        d.receiptLink,
		Symbol:      req.OrderReq.Symbol,
		RequestID:   req.OrderReq.RequestID,
		Side:        req.OrderReq.Side,
		OrderType:   req.OrderReq.Type,
		TimeInForce: req.OrderReq.TimeInForce,
		Price:       req.OrderReq.Price,
		Qty:         req.OrderReq.Qty,
		DecisionAt:  d.clock.NowUnixNano(),
		Frontier:    frontier,
	})
}

func marketDataFingerprint(msg *exchange.MarketDataMsg) ([16]byte, error) {
	// Include every actor-visible field. SeqNum alone is not sufficient for
	// directed lifecycle replay, which historically carries zero.
	raw, err := json.Marshal(struct {
		Type      types.MDType `json:"type"`
		Symbol    string       `json:"symbol"`
		Sequence  uint64       `json:"sequence"`
		Timestamp int64        `json:"timestamp"`
		Data      any          `json:"data"`
	}{msg.Type, msg.Symbol, msg.SeqNum, msg.Timestamp, msg.Data})
	if err != nil {
		return [16]byte{}, err
	}
	digest := sha256.Sum256(raw)
	var fingerprint [16]byte
	copy(fingerprint[:], digest[:])
	return fingerprint, nil
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
	processed := false
	received := make([]scheduledMarketDataReceipt, 0)
	for len(d.phaseResp) > 0 {
		select {
		case d.responseCh <- d.phaseResp[0]:
			d.phaseResp = d.phaseResp[1:]
			processed = true
		default:
			d.phaseMu.Unlock()
			return processed
		}
	}
	for len(d.phaseMD) > 0 {
		select {
		case d.marketDataCh <- d.phaseMD[0].message:
			received = append(received, d.phaseMD[0].receipt)
			d.phaseMD = d.phaseMD[1:]
			processed = true
		default:
			d.phaseMu.Unlock()
			for _, ticket := range received {
				d.recordMarketDataReceipt(ticket)
			}
			return processed
		}
	}
	d.phaseMu.Unlock()
	for _, ticket := range received {
		d.recordMarketDataReceipt(ticket)
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
