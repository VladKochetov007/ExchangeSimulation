package derivsim

import (
	"context"
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// OptionLiabilityTakerConfig describes a finite downside-protection mandate.
// The participant has no volatility model: it buys a listed put near a fixed
// moneyness target, at an observed executable ask, until its declared finite
// target is filled. The objective is an off-exchange liability, while all
// observations and execution remain venue-local.
type OptionLiabilityTakerConfig struct {
	Underlying          string                        `json:"underlying"`
	TargetQty           int64                         `json:"target_qty"`
	LotQty              int64                         `json:"lot_qty"`
	TargetStrikeBps     int64                         `json:"target_strike_bps"`
	MaxPremium          int64                         `json:"max_premium"`
	Interval            time.Duration                 `json:"interval"`
	DecisionPhaseOffset time.Duration                 `json:"decision_phase_offset"`
	BasePrecision       int64                         `json:"base_precision"`
	TerminalNano        int64                         `json:"-"`
	VenueID             string                        `json:"-"`
	User                string                        `json:"-"`
	ClientID            uint64                        `json:"-"`
	DecisionObserver    func(OptionLiabilityDecision) `json:"-"`
	FillObserver        func(OptionLiabilityFill)     `json:"-"`
}

// Validate checks only policy inputs. It deliberately does not require a
// listed contract: the participant must be able to wait for future listings.
func (c OptionLiabilityTakerConfig) Validate() error {
	if c.Underlying == "" {
		return fmt.Errorf("underlying is required")
	}
	if c.TargetQty <= 0 || c.LotQty <= 0 || c.LotQty > c.TargetQty {
		return fmt.Errorf("target and lot quantities must be positive with lot <= target")
	}
	if c.TargetStrikeBps <= 0 || c.TargetStrikeBps >= 10_000 {
		return fmt.Errorf("target strike bps must lie strictly inside (0,10000)")
	}
	if c.MaxPremium <= 0 || c.Interval <= 0 {
		return fmt.Errorf("maximum premium and interval must be positive")
	}
	if c.DecisionPhaseOffset < 0 || c.DecisionPhaseOffset >= c.Interval {
		return fmt.Errorf("decision phase offset must be in [0, interval)")
	}
	if c.BasePrecision <= 0 {
		return fmt.Errorf("base precision must be positive")
	}
	return nil
}

// OptionLiabilityDecision is persisted before order ingress. SideEvidence is
// mandatory because exchange.Buy is numerically zero and must not disappear
// from scientific evidence. The book/frontier fields attest the actor's local
// information set without exposing exchange-owned state to the actor.
type OptionLiabilityDecision struct {
	VenueID              string        `json:"venue_id"`
	User                 string        `json:"user"`
	ClientID             uint64        `json:"client_id"`
	Underlying           string        `json:"underlying"`
	DecisionTime         int64         `json:"decision_time"`
	Action               string        `json:"action"`
	Reason               string        `json:"reason"`
	TargetQty            int64         `json:"target_qty"`
	PositionBefore       int64         `json:"position_before"`
	RemainingQty         int64         `json:"remaining_qty"`
	TargetStrikeBps      int64         `json:"target_strike_bps"`
	TargetStrike         int64         `json:"target_strike"`
	OptionSymbol         string        `json:"option_symbol"`
	OptionExpiryNano     int64         `json:"option_expiry_nano"`
	HasUnderlying        bool          `json:"has_underlying"`
	UnderlyingMid        int64         `json:"underlying_mid"`
	UnderlyingSourceTime int64         `json:"underlying_source_time"`
	UnderlyingReceivedAt int64         `json:"underlying_received_at"`
	UnderlyingSequence   uint64        `json:"underlying_sequence"`
	HasAsk               bool          `json:"has_ask"`
	AskPrice             int64         `json:"ask_price"`
	AskVisibleQty        int64         `json:"ask_visible_qty"`
	AskSourceTime        int64         `json:"ask_source_time"`
	AskReceivedAt        int64         `json:"ask_received_at"`
	AskSequence          uint64        `json:"ask_sequence"`
	RequestID            uint64        `json:"request_id"`
	RequestedQty         int64         `json:"requested_qty"`
	Side                 exchange.Side `json:"-"`
	SideEvidence         string        `json:"side"`
	OrderType            string        `json:"order_type"`
	TimeInForce          string        `json:"time_in_force"`
}

// OptionLiabilityFill attests the actor-local position transition around an
// exchange-confirmed fill. Venue fill evidence remains the execution source
// of truth; this row is an independently joinable participant record.
type OptionLiabilityFill struct {
	VenueID      string `json:"venue_id"`
	User         string `json:"user"`
	ClientID     uint64 `json:"client_id"`
	Symbol       string `json:"symbol"`
	Timestamp    int64  `json:"timestamp"`
	OrderID      uint64 `json:"order_id"`
	TradeID      uint64 `json:"trade_id"`
	Side         string `json:"side"`
	Qty          int64  `json:"qty"`
	Price        int64  `json:"price"`
	FeeAmount    int64  `json:"fee_amount"`
	FeeAsset     string `json:"fee_asset"`
	PrePosition  int64  `json:"pre_position"`
	PostPosition int64  `json:"post_position"`
}

type liabilityOptionTouch struct {
	hasAsk     bool
	ask        int64
	askQty     int64
	sourceTime int64
	receivedAt int64
	sequence   uint64
}

type liabilitySpotObservation struct {
	has        bool
	mid        int64
	sourceTime int64
	receivedAt int64
	sequence   uint64
}

// OptionLiabilityTaker is deliberately deterministic: there is no RNG and no
// global/index/model input. Contract selection is stable by symbol after
// filtering to the put nearest the latest delivered 95%-moneyness target.
type OptionLiabilityTaker struct {
	*actor.BaseActor
	cfg        OptionLiabilityTakerConfig
	set        *contractSet
	quotes     map[string]liabilityOptionTouch
	spot       liabilitySpotObservation
	position   int64
	pending    bool
	pendingReq uint64
	pendingSym string
	active     map[uint64]string
	subscribed bool
}

func NewOptionLiabilityTaker(id uint64, gw actor.Gateway, cfg OptionLiabilityTakerConfig) *OptionLiabilityTaker {
	u := &OptionLiabilityTaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		quotes:    make(map[string]liabilityOptionTouch),
		active:    make(map[uint64]string),
	}
	u.set.onList = func(c *Contract) {
		if c.Type == "OPTION" {
			u.Subscribe(c.Symbol, exchange.MDSnapshot)
		}
	}
	u.set.onSettle = func(c *Contract, _ int64) {
		delete(u.quotes, c.Symbol)
	}
	u.set.onAccept = func(sym string, reqID, orderID uint64) {
		if reqID == u.pendingReq {
			u.active[orderID] = sym
		}
	}
	u.set.onReject = func(reqID uint64) {
		if reqID == u.pendingReq {
			u.pending, u.pendingReq, u.pendingSym = false, 0, ""
		}
	}
	u.set.onFill = u.onFill
	u.SetHandler(u)
	u.AddTickerWithOffset(cfg.Interval, cfg.DecisionPhaseOffset, u.onTick)
	return u
}

// Position reports filled option units only; outstanding requests are not
// treated as exposure until the venue confirms a fill.
func (u *OptionLiabilityTaker) Position() int64 { return u.position }

func (u *OptionLiabilityTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventOrderCancelled {
		u.onCancel(evt.Data.(actor.OrderCancelledEvent))
		u.set.handle(evt)
		return
	}
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == u.cfg.Underlying {
			u.observeUnderlying(e)
			return
		}
		touch := liabilityOptionTouch{}
		if e.Snapshot != nil && len(e.Snapshot.Asks) > 0 {
			touch.hasAsk = true
			touch.ask = e.Snapshot.Asks[0].Price
			touch.askQty = e.Snapshot.Asks[0].VisibleQty
		}
		touch.sourceTime, touch.sequence = e.Timestamp, e.SeqNum
		touch.receivedAt = u.receivedAt()
		u.quotes[e.Symbol] = touch
		return
	}
	u.set.handle(evt)
}

func (u *OptionLiabilityTaker) observeUnderlying(e actor.BookSnapshotEvent) {
	obs := liabilitySpotObservation{sourceTime: e.Timestamp, sequence: e.SeqNum, receivedAt: u.receivedAt()}
	if e.Snapshot != nil && len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
		bid, ask := e.Snapshot.Bids[0].Price, e.Snapshot.Asks[0].Price
		if bid <= ask {
			// The option model is positive-domain, but the midpoint arithmetic is
			// still the engine's overflow-safe primitive for signed prices.
			obs.mid = bid + (ask-bid)/2
			obs.has = true
		}
	}
	u.spot = obs
}

func (u *OptionLiabilityTaker) receivedAt() int64 {
	if frontier, ok := u.Gateway().(interface {
		MarketDataFrontier() simulation.MarketDataFrontier
	}); ok {
		return frontier.MarketDataFrontier().DeliveredAt
	}
	return 0
}

func (u *OptionLiabilityTaker) onFill(sym string, e actor.OrderFillEvent) {
	if _, ok := u.active[e.OrderID]; !ok {
		return
	}
	pre := u.position
	if e.Side == exchange.Buy {
		next, ok := etypes.TryAdd(u.position, e.Qty)
		if !ok {
			return
		}
		u.position = next
	} else {
		next, ok := etypes.TrySub(u.position, e.Qty)
		if !ok {
			return
		}
		u.position = next
	}
	if u.cfg.FillObserver != nil {
		u.cfg.FillObserver(OptionLiabilityFill{
			VenueID: u.cfg.VenueID, User: u.cfg.User, ClientID: u.cfg.ClientID,
			Symbol: sym, Timestamp: e.Timestamp, OrderID: e.OrderID, TradeID: e.TradeID,
			Side: e.Side.String(), Qty: e.Qty, Price: e.Price, FeeAmount: e.FeeAmount,
			FeeAsset: e.FeeAsset, PrePosition: pre, PostPosition: u.position,
		})
	}
	if e.IsFull {
		delete(u.active, e.OrderID)
		u.pending, u.pendingReq, u.pendingSym = false, 0, ""
	}
}

func (u *OptionLiabilityTaker) onCancel(e actor.OrderCancelledEvent) {
	if _, ok := u.active[e.OrderID]; ok {
		delete(u.active, e.OrderID)
		u.pending, u.pendingReq, u.pendingSym = false, 0, ""
	}
}

func (u *OptionLiabilityTaker) onTick(now time.Time) {
	if !u.subscribed {
		u.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		u.Subscribe(u.cfg.Underlying, exchange.MDSnapshot)
		u.subscribed = true
		u.emit(u.baseDecision(now, "NOT_SUBSCRIBED"))
		return
	}
	decision := u.decision(now)
	u.emit(decision)
	if decision.Action != "SUBMIT_PUT_IOC" {
		return
	}
	u.pending = true
	u.pendingReq, u.pendingSym = decision.RequestID, decision.OptionSymbol
	reqID := u.SubmitOrderWithTimeInForce(decision.OptionSymbol, exchange.Buy, exchange.LimitOrder, decision.AskPrice, decision.RequestedQty, exchange.IOC)
	if reqID != decision.RequestID {
		// PeekNextRequestID is part of the actor contract; a mismatch is a
		// programming error, not a reason to infer or repair a request identity.
		u.pendingReq = reqID
	}
	u.set.trackRequest(reqID, decision.OptionSymbol)
}

func (u *OptionLiabilityTaker) decision(now time.Time) OptionLiabilityDecision {
	d := u.baseDecision(now, "")
	d.PositionBefore = u.position
	if !u.spot.has || u.spot.mid <= 0 {
		d.Reason = "LOCAL_UNDERLYING_UNAVAILABLE"
		return d
	}
	remaining, ok := etypes.TrySub(u.cfg.TargetQty, u.position)
	if !ok || remaining <= 0 {
		d.Reason = "LIABILITY_TARGET_FILLED"
		return d
	}
	if u.pending {
		d.Reason = "REQUEST_PENDING"
		return d
	}
	if u.terminalRoundTripCensored(now.UnixNano()) {
		d.Reason = "SIMULATION_HORIZON_CENSORED"
		return d
	}
	targetStrike, ok := etypes.TryMulDiv(u.spot.mid, u.cfg.TargetStrikeBps, 10_000)
	if !ok {
		d.Reason = "TARGET_STRIKE_UNREPRESENTABLE"
		return d
	}
	d.TargetStrike = targetStrike
	contract := u.nearestPut(targetStrike, now.UnixNano())
	if contract == nil {
		d.Reason = "NO_LIVE_PUT"
		return d
	}
	d.OptionSymbol, d.OptionExpiryNano = contract.Symbol, contract.ExpiryNano
	touch, ok := u.quotes[contract.Symbol]
	if !ok || !touch.hasAsk || touch.ask <= 0 {
		d.Reason = "OPTION_ASK_UNAVAILABLE"
		return d
	}
	d.HasAsk, d.AskPrice, d.AskVisibleQty = touch.hasAsk, touch.ask, touch.askQty
	d.AskSourceTime, d.AskReceivedAt, d.AskSequence = touch.sourceTime, touch.receivedAt, touch.sequence
	if touch.sourceTime > now.UnixNano() || touch.receivedAt > now.UnixNano() {
		d.Reason = "LOCAL_OPTION_OBSERVATION_FUTURE"
		return d
	}
	if touch.ask > u.cfg.MaxPremium {
		d.Reason = "PREMIUM_BUDGET_EXCEEDED"
		return d
	}
	qty := u.cfg.LotQty
	if qty > remaining {
		qty = remaining
	}
	if touch.askQty > 0 && qty > touch.askQty {
		qty = touch.askQty
	}
	if qty <= 0 {
		d.Reason = "NO_EXECUTABLE_ASK_SIZE"
		return d
	}
	d.Action = "SUBMIT_PUT_IOC"
	d.Reason = "SUBMIT_PUT_IOC"
	d.RequestedQty, d.RequestID = qty, u.PeekNextRequestID()
	d.Side, d.SideEvidence = exchange.Buy, exchange.Buy.String()
	d.OrderType, d.TimeInForce = exchange.LimitOrder.String(), exchange.IOC.String()
	return d
}

func (u *OptionLiabilityTaker) nearestPut(targetStrike, now int64) *Contract {
	var best *Contract
	bestDistance := int64(0)
	for _, c := range u.set.orderedContracts() {
		if c.Type != "OPTION" || c.IsCall || c.ExpiryNano <= now {
			continue
		}
		distance := c.Strike - targetStrike
		if distance < 0 {
			distance = -distance
		}
		if best == nil || distance < bestDistance || (distance == bestDistance && c.Symbol < best.Symbol) {
			best, bestDistance = c, distance
		}
	}
	return best
}

func (u *OptionLiabilityTaker) terminalRoundTripCensored(now int64) bool {
	if u.cfg.TerminalNano == 0 {
		return false
	}
	return now > u.cfg.TerminalNano-int64(2*u.cfg.Interval)
}

func (u *OptionLiabilityTaker) baseDecision(now time.Time, reason string) OptionLiabilityDecision {
	return OptionLiabilityDecision{
		VenueID: u.cfg.VenueID, User: u.cfg.User, ClientID: u.cfg.ClientID,
		Underlying: u.cfg.Underlying, DecisionTime: now.UnixNano(), Action: "DEFER", Reason: reason,
		TargetQty: u.cfg.TargetQty, TargetStrikeBps: u.cfg.TargetStrikeBps,
		PositionBefore: u.position, HasUnderlying: u.spot.has, UnderlyingMid: u.spot.mid,
		UnderlyingSourceTime: u.spot.sourceTime, UnderlyingReceivedAt: u.spot.receivedAt,
		UnderlyingSequence: u.spot.sequence,
	}
}

func (u *OptionLiabilityTaker) emit(d OptionLiabilityDecision) {
	if u.cfg.DecisionObserver != nil {
		u.cfg.DecisionObserver(d)
	}
}
