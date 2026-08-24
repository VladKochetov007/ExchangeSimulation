package multivenue

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// LiabilityHedgerConfig defines one finite-capital participant that hedges a
// bounded external delivery obligation. The obligation is off-exchange motive
// state; every market action still uses an ordinary venue-local account.
type LiabilityHedgerConfig struct {
	Enabled             bool                          `json:"enabled"`
	Symbol              string                        `json:"symbol"`
	DecisionInterval    time.Duration                 `json:"decision_interval"`
	ObligationInterval  time.Duration                 `json:"obligation_interval"`
	ObligationStepQty   int64                         `json:"obligation_step_qty"`
	MaxAbsObligationQty int64                         `json:"max_abs_obligation_qty"`
	MaxRequestQty       int64                         `json:"max_request_qty"`
	Seed                int64                         `json:"-"`
	VenueID             string                        `json:"-"`
	Hedger              string                        `json:"-"`
	ClientID            uint64                        `json:"-"`
	TerminalNano        int64                         `json:"-"`
	TakerFeeBps         int64                         `json:"-"`
	DecisionObserver    func(LiabilityHedgerDecision) `json:"-"`
	FillObserver        func(LiabilityHedgerFill)     `json:"-"`
}

func (c LiabilityHedgerConfig) validate() error {
	if c.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if c.DecisionInterval <= 0 || c.ObligationInterval <= 0 {
		return fmt.Errorf("decision and obligation intervals must be positive")
	}
	if c.ObligationInterval%c.DecisionInterval != 0 {
		return fmt.Errorf("obligation interval must be a multiple of decision interval")
	}
	if c.ObligationStepQty <= 0 || c.MaxAbsObligationQty < c.ObligationStepQty {
		return fmt.Errorf("require positive obligation step no greater than maximum absolute obligation")
	}
	if c.MaxAbsObligationQty > math.MaxInt64-c.ObligationStepQty {
		return fmt.Errorf("obligation bounds leave no safe reflected update")
	}
	if c.MaxRequestQty <= 0 {
		return fmt.Errorf("maximum request quantity must be positive")
	}
	if c.TakerFeeBps < 0 || c.TakerFeeBps > 10_000 {
		return fmt.Errorf("taker fee bps must be in [0,10000]")
	}
	return nil
}

// LiabilityHedgerDecision is L0's pre-ingress evidence. Every evaluation is
// recorded, including disabled and unavailable-price deferrals. SideEvidence
// is deliberately a named string: internal BUY is numerically zero and must
// never become absent under JSON omitempty.
type LiabilityHedgerDecision struct {
	VenueID              string        `json:"venue_id"`
	Hedger               string        `json:"hedger"`
	ClientID             uint64        `json:"client_id"`
	Symbol               string        `json:"symbol"`
	DecisionTime         int64         `json:"decision_time"`
	Enabled              bool          `json:"enabled"`
	Subscribed           bool          `json:"subscribed"`
	RequestPending       bool          `json:"request_pending"`
	ActionOrDeferReason  string        `json:"action_or_defer_reason"`
	ObligationBefore     int64         `json:"obligation_before"`
	ObligationAfter      int64         `json:"obligation_after"`
	ObligationStep       int64         `json:"obligation_step"`
	ObligationLimit      int64         `json:"obligation_limit"`
	PositionBefore       int64         `json:"position_before"`
	HedgeGap             int64         `json:"hedge_gap"`
	DecisionInterval     int64         `json:"decision_interval"`
	ObligationInterval   int64         `json:"obligation_interval"`
	LastBookSourceTime   int64         `json:"last_book_source_time"`
	LastBookReceivedTime int64         `json:"last_book_received_time"`
	LastBookSequence     uint64        `json:"last_book_sequence"`
	BidPrice             int64         `json:"bid_price"`
	BidVisibleQty        int64         `json:"bid_visible_qty"`
	AskPrice             int64         `json:"ask_price"`
	AskVisibleQty        int64         `json:"ask_visible_qty"`
	Side                 exchange.Side `json:"-"`
	SideEvidence         string        `json:"side,omitempty"`
	LimitPrice           int64         `json:"limit_price"`
	RequestedQty         int64         `json:"requested_qty"`
	RequestID            uint64        `json:"request_id,omitempty"`
	TakerFeeBps          int64         `json:"taker_fee_bps"`
	OutcomeExpectation   string        `json:"outcome_expectation"`
	CensorReason         string        `json:"censor_reason,omitempty"`
}

// LiabilityHedgerFill attests the actor-local position change around one
// exchange-confirmed L0 fill. The exchange's fill remains the execution
// source of truth; this row makes the hedging claim independently testable.
type LiabilityHedgerFill struct {
	VenueID      string `json:"venue_id"`
	Hedger       string `json:"hedger"`
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

type liabilityHedgerBook struct {
	SourceTime   int64
	ReceivedTime int64
	Sequence     uint64
	BidPrice     int64
	BidQty       int64
	AskPrice     int64
	AskQty       int64
}

// LiabilityHedger trades only toward its explicit obligation. It cannot use a
// shared index, an exchange-owned book, a midpoint, or a price-zero fallback.
type LiabilityHedger struct {
	*actor.BaseActor
	cfg LiabilityHedgerConfig
	rng *rand.Rand

	book             liabilityHedgerBook
	obligation       int64
	position         int64
	lastUpdate       int64
	pending          bool
	pendingRequestID uint64
	activeOrders     map[uint64]struct{}
	subscribed       bool
}

// NewLiabilityHedger creates a locally informed delivery-liability participant.
func NewLiabilityHedger(id uint64, gateway actor.Gateway, cfg LiabilityHedgerConfig) *LiabilityHedger {
	h := &LiabilityHedger{
		BaseActor:    actor.NewBaseActor(id, gateway),
		cfg:          cfg,
		rng:          rand.New(rand.NewSource(cfg.Seed)),
		activeOrders: make(map[uint64]struct{}),
	}
	h.SetHandler(h)
	h.AddTicker(cfg.DecisionInterval, h.onTick)
	return h
}

// Obligation returns the current signed external delivery obligation.
func (h *LiabilityHedger) Obligation() int64 { return h.obligation }

// Position returns the filled CDF position relative to the actor's opening
// inventory. It intentionally excludes an unfilled request.
func (h *LiabilityHedger) Position() int64 { return h.position }

func (h *LiabilityHedger) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		h.observeSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		h.onAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		h.onRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		h.onFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		h.onCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (h *LiabilityHedger) observeSnapshot(event actor.BookSnapshotEvent) {
	if event.Symbol != h.cfg.Symbol {
		return
	}
	book := liabilityHedgerBook{SourceTime: event.Timestamp, Sequence: event.SeqNum}
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) > 0 {
			book.BidPrice, book.BidQty = event.Snapshot.Bids[0].Price, event.Snapshot.Bids[0].VisibleQty
		}
		if len(event.Snapshot.Asks) > 0 {
			book.AskPrice, book.AskQty = event.Snapshot.Asks[0].Price, event.Snapshot.Asks[0].VisibleQty
		}
	}
	if gateway, ok := h.Gateway().(interface {
		MarketDataFrontier() simulation.MarketDataFrontier
	}); ok {
		book.ReceivedTime = gateway.MarketDataFrontier().DeliveredAt
	}
	h.book = book
}

func (h *LiabilityHedger) onAccepted(event actor.OrderAcceptedEvent) {
	if event.RequestID != h.pendingRequestID {
		return
	}
	h.pendingRequestID = 0
	h.activeOrders[event.OrderID] = struct{}{}
}

func (h *LiabilityHedger) onRejected(event actor.OrderRejectedEvent) {
	if event.RequestID != h.pendingRequestID {
		return
	}
	h.pending, h.pendingRequestID = false, 0
}

func (h *LiabilityHedger) onFill(event actor.OrderFillEvent) {
	if event.Symbol != h.cfg.Symbol {
		return
	}
	if _, active := h.activeOrders[event.OrderID]; !active {
		return
	}
	prePosition := h.position
	if event.Side == exchange.Buy {
		next, ok := etypes.TryAdd(h.position, event.Qty)
		if !ok {
			return
		}
		h.position = next
	} else {
		next, ok := etypes.TrySub(h.position, event.Qty)
		if !ok {
			return
		}
		h.position = next
	}
	if h.cfg.FillObserver != nil {
		h.cfg.FillObserver(LiabilityHedgerFill{
			VenueID: h.cfg.VenueID, Hedger: h.cfg.Hedger, ClientID: h.cfg.ClientID,
			Symbol: event.Symbol, Timestamp: event.Timestamp, OrderID: event.OrderID,
			TradeID: event.TradeID, Side: event.Side.String(), Qty: event.Qty,
			Price: event.Price, FeeAmount: event.FeeAmount, FeeAsset: event.FeeAsset,
			PrePosition: prePosition, PostPosition: h.position,
		})
	}
	if event.IsFull {
		delete(h.activeOrders, event.OrderID)
		h.pending = false
	}
}

func (h *LiabilityHedger) onCancelled(event actor.OrderCancelledEvent) {
	if _, active := h.activeOrders[event.OrderID]; !active {
		return
	}
	delete(h.activeOrders, event.OrderID)
	h.pending = false
}

func (h *LiabilityHedger) onTick(now time.Time) {
	if !h.subscribed {
		decision := h.baseDecision(now, "NOT_SUBSCRIBED")
		h.Subscribe(h.cfg.Symbol, exchange.MDSnapshot)
		h.subscribed = true
		h.emit(decision)
		return
	}
	decision := h.decision(now)
	h.emit(decision)
	if decision.ActionOrDeferReason != "SUBMIT_IOC" {
		return
	}
	h.pending = true
	h.pendingRequestID = decision.RequestID
	h.SubmitOrderWithTimeInForce(h.cfg.Symbol, decision.Side, exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
}

func (h *LiabilityHedger) decision(now time.Time) LiabilityHedgerDecision {
	decision := h.baseDecision(now, "")
	if h.lastUpdate == 0 || now.UnixNano()-h.lastUpdate >= int64(h.cfg.ObligationInterval) {
		step, next, ok := h.nextObligation()
		if !ok {
			decision.ActionOrDeferReason = "OBLIGATION_UPDATE_OVERFLOW"
			return decision
		}
		h.lastUpdate = now.UnixNano()
		h.obligation = next
		decision.ObligationStep = step
		decision.ObligationAfter = next
	}
	decision.PositionBefore = h.position
	gap, ok := etypes.TrySub(h.obligation, h.position)
	if !ok {
		decision.ActionOrDeferReason = "HEDGE_GAP_OVERFLOW"
		return decision
	}
	decision.HedgeGap = gap
	if !h.cfg.Enabled {
		decision.ActionOrDeferReason = "POLICY_DISABLED"
		return decision
	}
	if h.pending {
		decision.ActionOrDeferReason = "REQUEST_PENDING"
		return decision
	}
	if gap == 0 {
		decision.ActionOrDeferReason = "IN_BAND"
		return decision
	}
	if h.book.SourceTime == 0 {
		decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
		return decision
	}
	if h.book.SourceTime > now.UnixNano() {
		decision.ActionOrDeferReason = "LOCAL_BOOK_SOURCE_FUTURE"
		return decision
	}
	quantity, ok := nonnegativeMagnitude(gap)
	if !ok {
		decision.ActionOrDeferReason = "HEDGE_GAP_UNREPRESENTABLE"
		return decision
	}
	decision.RequestedQty = minInt64(quantity, h.cfg.MaxRequestQty)
	if decision.RequestedQty <= 0 {
		decision.ActionOrDeferReason = "ZERO_REQUEST_QUANTITY"
		return decision
	}
	if gap > 0 {
		decision.Side = exchange.Buy
		decision.SideEvidence = exchange.Buy.String()
		decision.LimitPrice = h.book.AskPrice
	} else {
		decision.Side = exchange.Sell
		decision.SideEvidence = exchange.Sell.String()
		decision.LimitPrice = h.book.BidPrice
	}
	if decision.LimitPrice <= 0 {
		decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
		return decision
	}
	decision.RequestID = h.PeekNextRequestID()
	decision.ActionOrDeferReason = "SUBMIT_IOC"
	decision.OutcomeExpectation = "VENUE_OUTCOME_REQUIRED"
	if h.cfg.TerminalNano != 0 && now.UnixNano() >= h.cfg.TerminalNano {
		decision.OutcomeExpectation = "SIMULATION_HORIZON_CENSORED"
		decision.CensorReason = "terminal_horizon_before_venue_ingress"
	}
	return decision
}

func (h *LiabilityHedger) baseDecision(now time.Time, action string) LiabilityHedgerDecision {
	return LiabilityHedgerDecision{
		VenueID: h.cfg.VenueID, Hedger: h.cfg.Hedger, ClientID: h.cfg.ClientID,
		Symbol: h.cfg.Symbol, DecisionTime: now.UnixNano(), Enabled: h.cfg.Enabled,
		Subscribed: h.subscribed, RequestPending: h.pending, ActionOrDeferReason: action,
		ObligationBefore: h.obligation, ObligationAfter: h.obligation,
		ObligationLimit: h.cfg.MaxAbsObligationQty, PositionBefore: h.position,
		DecisionInterval: int64(h.cfg.DecisionInterval), ObligationInterval: int64(h.cfg.ObligationInterval),
		LastBookSourceTime: h.book.SourceTime, LastBookReceivedTime: h.book.ReceivedTime,
		LastBookSequence: h.book.Sequence, BidPrice: h.book.BidPrice, BidVisibleQty: h.book.BidQty,
		AskPrice: h.book.AskPrice, AskVisibleQty: h.book.AskQty, TakerFeeBps: h.cfg.TakerFeeBps,
	}
}

// nextObligation chooses a symmetric signed shock and reflects it at the
// declared bound. It returns the actual signed state transition and target.
func (h *LiabilityHedger) nextObligation() (int64, int64, bool) {
	step := h.cfg.ObligationStepQty
	if h.rng.Intn(2) == 0 {
		step = -step
	}
	next, ok := etypes.TryAdd(h.obligation, step)
	if !ok || next > h.cfg.MaxAbsObligationQty || next < -h.cfg.MaxAbsObligationQty {
		step = -step
		next, ok = etypes.TryAdd(h.obligation, step)
	}
	if !ok || next > h.cfg.MaxAbsObligationQty || next < -h.cfg.MaxAbsObligationQty || step == 0 {
		return 0, h.obligation, false
	}
	return step, next, true
}

func (h *LiabilityHedger) emit(decision LiabilityHedgerDecision) {
	if h.cfg.DecisionObserver != nil {
		h.cfg.DecisionObserver(decision)
	}
}

func nonnegativeMagnitude(value int64) (int64, bool) {
	if value >= 0 {
		return value, true
	}
	if value == math.MinInt64 {
		return 0, false
	}
	return -value, true
}
