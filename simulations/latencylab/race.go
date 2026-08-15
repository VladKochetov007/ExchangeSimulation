// Package latencylab isolates an information-to-execution race. It deliberately
// avoids ecological strategy confounders: two agents receive one public signal
// and attempt the same two-leg FOK conversion with different modeled latency.
package latencylab

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

const (
	alphaName = "alpha"
	betaName  = "beta"

	alphaClientID = uint64(20)
	betaClientID  = uint64(10)

	signalSymbol = "SIG/USD"
	buySymbol    = "ABC-A/USD"
	sellSymbol   = "ABC-B/USD"
	raceQty      = int64(1)
)

type legReport struct {
	Symbol       string                `json:"symbol"`
	Side         exchange.Side         `json:"side"`
	RequestID    uint64                `json:"request_id"`
	OrderID      uint64                `json:"order_id"`
	FilledQty    int64                 `json:"filled_qty"`
	Notional     int64                 `json:"notional"`
	QuoteFees    int64                 `json:"quote_fees"`
	Rejected     bool                  `json:"rejected"`
	RejectReason exchange.RejectReason `json:"reject_reason"`
}

// RacerReport records the observed information time, actual exchange fills,
// and ledger deltas. ObservedCashflow is only a locked conversion cashflow
// when PairComplete is true; incomplete legs remain explicit residual risk.
type RacerReport struct {
	Name             string      `json:"name"`
	ClientID         uint64      `json:"client_id"`
	SignalExchangeAt int64       `json:"signal_exchange_at"`
	SignalObservedAt int64       `json:"signal_observed_at"`
	ReactionLatency  int64       `json:"reaction_latency"`
	Legs             []legReport `json:"legs"`
	PairComplete     bool        `json:"pair_complete"`
	ResidualBase     int64       `json:"residual_base"`
	ObservedCashflow int64       `json:"observed_cashflow"`
	LockedCashflow   int64       `json:"locked_cashflow"`
	AccountUSDDelta  int64       `json:"account_usd_delta"`
	AccountABCDelta  int64       `json:"account_abc_delta"`
	UnpricedFeeCount int         `json:"unpriced_fee_count"`
}

type raceActor struct {
	*actor.BaseActor
	name  string
	clock exchange.Clock
	gw    actor.Gateway

	nextRequestID uint64
	triggered     bool
	report        RacerReport
	byRequest     map[uint64]int
	byOrder       map[uint64]int
}

func newRaceActor(name string, clientID uint64, gw actor.Gateway, clock exchange.Clock) *raceActor {
	r := &raceActor{
		BaseActor: actor.NewBaseActor(clientID, gw),
		name:      name,
		clock:     clock,
		gw:        gw,
		report: RacerReport{
			Name:     name,
			ClientID: clientID,
			Legs: []legReport{
				{Symbol: buySymbol, Side: exchange.Buy},
				{Symbol: sellSymbol, Side: exchange.Sell},
			},
		},
		byRequest:     make(map[uint64]int, 2),
		byOrder:       make(map[uint64]int, 2),
		nextRequestID: 100,
	}
	r.SetHandler(r)
	return r
}

func (r *raceActor) Start(ctx context.Context) error {
	// Subscribe before the actor is registered with the phase pump. The request
	// still travels through the actor's own modeled request latency.
	r.Subscribe(signalSymbol, exchange.MDTrade)
	return r.BaseActor.Start(ctx)
}

func (r *raceActor) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventTrade:
		trade := event.Data.(actor.TradeEvent)
		if trade.Symbol == signalSymbol && !r.triggered {
			r.trigger(trade.Timestamp)
		}
	case actor.EventOrderAccepted:
		accepted := event.Data.(actor.OrderAcceptedEvent)
		if index, ok := r.byRequest[accepted.RequestID]; ok {
			r.report.Legs[index].OrderID = accepted.OrderID
			r.byOrder[accepted.OrderID] = index
		}
	case actor.EventOrderRejected:
		rejected := event.Data.(actor.OrderRejectedEvent)
		if index, ok := r.byRequest[rejected.RequestID]; ok {
			r.report.Legs[index].Rejected = true
			r.report.Legs[index].RejectReason = rejected.Reason
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		r.recordFill(event.Data.(actor.OrderFillEvent))
	}
}

func (r *raceActor) trigger(signalExchangeAt int64) {
	r.triggered = true
	r.report.SignalExchangeAt = signalExchangeAt
	r.report.SignalObservedAt = r.clock.NowUnixNano()
	r.report.ReactionLatency = r.report.SignalObservedAt - signalExchangeAt
	r.submitFOK(0)
	r.submitFOK(1)
}

func (r *raceActor) submitFOK(index int) {
	r.nextRequestID++
	requestID := r.nextRequestID
	leg := &r.report.Legs[index]
	leg.RequestID = requestID
	r.byRequest[requestID] = index
	r.gw.Send(exchange.Request{
		Type: exchange.ReqPlaceOrder,
		OrderReq: &exchange.OrderRequest{
			RequestID: requestID, Symbol: leg.Symbol, Side: leg.Side,
			Type: exchange.Market, Qty: raceQty, TimeInForce: exchange.FOK,
			Visibility: exchange.Normal,
		},
	})
}

func (r *raceActor) recordFill(fill actor.OrderFillEvent) {
	index, ok := r.byOrder[fill.OrderID]
	if !ok {
		return
	}
	leg := &r.report.Legs[index]
	leg.FilledQty += fill.Qty
	leg.Notional += fill.Qty * fill.Price
	if fill.FeeAsset == "USD" {
		leg.QuoteFees += fill.FeeAmount
	} else if fill.FeeAmount != 0 {
		r.report.UnpricedFeeCount++
	}
}

func (r *raceActor) Report() RacerReport {
	report := r.report
	report.Legs = append([]legReport(nil), r.report.Legs...)
	buy := report.Legs[0]
	sell := report.Legs[1]
	report.ResidualBase = buy.FilledQty - sell.FilledQty
	report.ObservedCashflow = sell.Notional - buy.Notional - buy.QuoteFees - sell.QuoteFees
	report.PairComplete = buy.FilledQty == raceQty && sell.FilledQty == raceQty && report.ResidualBase == 0 && report.UnpricedFeeCount == 0
	if report.PairComplete {
		report.LockedCashflow = report.ObservedCashflow
	}
	return report
}

type signalActor struct {
	*actor.BaseActor
	symbol string
	sent   bool
}

func newSignalActor(clientID uint64, gw actor.Gateway) *signalActor {
	s := &signalActor{BaseActor: actor.NewBaseActor(clientID, gw), symbol: signalSymbol}
	s.SetHandler(s)
	s.AddTicker(20*time.Millisecond, s.onTick)
	return s
}

func (s *signalActor) HandleEvent(context.Context, *actor.Event) {}

func (s *signalActor) onTick(_ time.Time) {
	if s.sent {
		return
	}
	s.sent = true
	s.SubmitOrder(s.symbol, exchange.Buy, exchange.Market, 0, raceQty)
}

type passiveActor struct{ *actor.BaseActor }

func newPassiveActor(clientID uint64, gw actor.Gateway) *passiveActor {
	p := &passiveActor{BaseActor: actor.NewBaseActor(clientID, gw)}
	p.SetHandler(p)
	return p
}

func (p *passiveActor) HandleEvent(context.Context, *actor.Event) {}
