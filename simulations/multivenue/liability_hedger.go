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
	Enabled bool `json:"enabled"`
	// PolicyMode selects the declared L1 side-selection policy. The empty
	// legacy configuration is deliberately equivalent to delivery_liability so
	// retained L0 configurations keep their exact behavior.
	PolicyMode       LiabilityHedgerPolicyMode `json:"policy_mode"`
	Symbol           string                    `json:"symbol"`
	DecisionInterval time.Duration             `json:"decision_interval"`
	// DecisionPhaseOffset moves the first periodic decision by a deterministic
	// offset while preserving DecisionInterval. Zero preserves L0/L1's legacy
	// ticker schedule exactly.
	DecisionPhaseOffset time.Duration                 `json:"decision_phase_offset"`
	ObligationInterval  time.Duration                 `json:"obligation_interval"`
	ObligationStepQty   int64                         `json:"obligation_step_qty"`
	MaxAbsObligationQty int64                         `json:"max_abs_obligation_qty"`
	MaxRequestQty       int64                         `json:"max_request_qty"`
	Seed                int64                         `json:"-"`
	PolicySeed          int64                         `json:"-"`
	VenueID             string                        `json:"-"`
	Hedger              string                        `json:"-"`
	ClientID            uint64                        `json:"-"`
	TerminalNano        int64                         `json:"-"`
	TakerFeeBps         int64                         `json:"-"`
	DecisionObserver    func(LiabilityHedgerDecision) `json:"-"`
	FillObserver        func(LiabilityHedgerFill)     `json:"-"`
}

// LiabilityHedgerPolicyMode is a named economic side-selection policy, not an
// availability sentinel. Both supported modes can represent BUY, SELL, and a
// deferred decision explicitly in the persisted evidence.
type LiabilityHedgerPolicyMode string

const (
	// LiabilityHedgerPolicyDeliveryLiability trades in the direction implied by
	// the signed delivery-liability gap. This is L0's legacy behavior.
	LiabilityHedgerPolicyDeliveryLiability LiabilityHedgerPolicyMode = "delivery_liability"
	// LiabilityHedgerPolicyRandomSideControl is L1's matched activity-generator
	// control. It shares every execution condition with the treatment but draws
	// the side from its independently seeded policy stream.
	LiabilityHedgerPolicyRandomSideControl LiabilityHedgerPolicyMode = "random_side_control"
)

func (c LiabilityHedgerConfig) effectivePolicyMode() (LiabilityHedgerPolicyMode, error) {
	switch c.PolicyMode {
	case "", LiabilityHedgerPolicyDeliveryLiability:
		return LiabilityHedgerPolicyDeliveryLiability, nil
	case LiabilityHedgerPolicyRandomSideControl:
		return LiabilityHedgerPolicyRandomSideControl, nil
	default:
		return "", fmt.Errorf("unsupported policy mode %q", c.PolicyMode)
	}
}

func (c LiabilityHedgerConfig) validate() error {
	if _, err := c.effectivePolicyMode(); err != nil {
		return err
	}
	if c.Symbol == "" {
		return fmt.Errorf("symbol is required")
	}
	if c.DecisionInterval <= 0 || c.ObligationInterval <= 0 {
		return fmt.Errorf("decision and obligation intervals must be positive")
	}
	if c.DecisionPhaseOffset < 0 || c.DecisionPhaseOffset >= c.DecisionInterval {
		return fmt.Errorf("decision phase offset must be in [0, decision interval)")
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
	PolicyMode           string        `json:"policy_mode"`
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
	DecisionPhaseOffset  int64         `json:"decision_phase_offset_nanos"`
	ObligationInterval   int64         `json:"obligation_interval"`
	LastBookSourceTime   int64         `json:"last_book_source_time"`
	LastBookReceivedTime int64         `json:"last_book_received_time"`
	LastBookSequence     uint64        `json:"last_book_sequence"`
	HasSnapshot          bool          `json:"has_snapshot"`
	HasBid               bool          `json:"has_bid"`
	BidPrice             int64         `json:"bid_price"`
	BidVisibleQty        int64         `json:"bid_visible_qty"`
	HasAsk               bool          `json:"has_ask"`
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
	PolicyMode   string `json:"policy_mode"`
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
	HasSnapshot  bool
	SourceTime   int64
	ReceivedTime int64
	Sequence     uint64
	HasBid       bool
	BidPrice     int64
	BidQty       int64
	HasAsk       bool
	AskPrice     int64
	AskQty       int64
}

// LiabilityHedger trades only toward its explicit obligation. It cannot use a
// shared index, an exchange-owned book, a midpoint, or a price-zero fallback.
type LiabilityHedger struct {
	*actor.BaseActor
	cfg       LiabilityHedgerConfig
	mode      LiabilityHedgerPolicyMode
	rng       *rand.Rand
	policyRNG *rand.Rand

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
	mode, err := cfg.effectivePolicyMode()
	if err != nil {
		// Config validation rejects this before a production actor is built. Keep
		// direct unit construction deterministic instead of creating an actor
		// with an implicit random behavior.
		mode = LiabilityHedgerPolicyDeliveryLiability
	}
	h := &LiabilityHedger{
		BaseActor:    actor.NewBaseActor(id, gateway),
		cfg:          cfg,
		mode:         mode,
		rng:          rand.New(rand.NewSource(cfg.Seed)),
		policyRNG:    rand.New(rand.NewSource(cfg.PolicySeed)),
		activeOrders: make(map[uint64]struct{}),
	}
	h.SetHandler(h)
	h.AddTickerWithOffset(cfg.DecisionInterval, cfg.DecisionPhaseOffset, h.onTick)
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
	book := liabilityHedgerBook{HasSnapshot: true, SourceTime: event.Timestamp, Sequence: event.SeqNum}
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) > 0 {
			book.HasBid = true
			book.BidPrice, book.BidQty = event.Snapshot.Bids[0].Price, event.Snapshot.Bids[0].VisibleQty
		}
		if len(event.Snapshot.Asks) > 0 {
			book.HasAsk = true
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
			Symbol: event.Symbol, PolicyMode: string(h.mode), Timestamp: event.Timestamp, OrderID: event.OrderID,
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
	// A request may reach the book on the next runner phase and its response
	// may reach this actor one phase after that. Do not create a final-tail
	// exchange fill that the actor cannot observe and attest before the fixed
	// simulation horizon. This is an explicit policy defer, not a missing-fill
	// exemption in the evidence analyzer.
	if h.terminalRoundTripCensored(now.UnixNano()) {
		decision.ActionOrDeferReason = "SIMULATION_HORIZON_CENSORED"
		decision.OutcomeExpectation = "SIMULATION_HORIZON_CENSORED"
		decision.CensorReason = "terminal_horizon_before_round_trip"
		return decision
	}
	if !h.book.HasSnapshot {
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
	decision.Side = h.selectSide(gap)
	decision.SideEvidence = decision.Side.String()
	if decision.Side == exchange.Buy {
		if !h.book.HasAsk {
			decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.LimitPrice = h.book.AskPrice
	} else {
		if !h.book.HasBid {
			decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.LimitPrice = h.book.BidPrice
	}
	decision.RequestID = h.PeekNextRequestID()
	decision.ActionOrDeferReason = "SUBMIT_IOC"
	decision.OutcomeExpectation = "VENUE_OUTCOME_REQUIRED"
	return decision
}

// selectSide is called only after a nonzero, representable gap has passed the
// terminal and local-snapshot guards. In L1 control mode one independent bit
// is consumed here, before checking the selected executable side; a missing
// selected side remains an explicit defer rather than a hidden alternative
// price or a skipped draw.
func (h *LiabilityHedger) selectSide(gap int64) exchange.Side {
	if h.mode == LiabilityHedgerPolicyRandomSideControl {
		if h.policyRNG.Intn(2) == 0 {
			return exchange.Buy
		}
		return exchange.Sell
	}
	if gap > 0 {
		return exchange.Buy
	}
	return exchange.Sell
}

func (h *LiabilityHedger) terminalRoundTripCensored(now int64) bool {
	if h.cfg.TerminalNano == 0 {
		return false
	}
	deadline, ok := etypes.TryAdd(now, int64(h.cfg.DecisionInterval))
	if !ok {
		return true
	}
	deadline, ok = etypes.TryAdd(deadline, int64(h.cfg.DecisionInterval))
	return !ok || deadline > h.cfg.TerminalNano
}

func (h *LiabilityHedger) baseDecision(now time.Time, action string) LiabilityHedgerDecision {
	return LiabilityHedgerDecision{
		VenueID: h.cfg.VenueID, Hedger: h.cfg.Hedger, ClientID: h.cfg.ClientID,
		Symbol: h.cfg.Symbol, DecisionTime: now.UnixNano(), Enabled: h.cfg.Enabled,
		PolicyMode: string(h.mode),
		Subscribed: h.subscribed, RequestPending: h.pending, ActionOrDeferReason: action,
		ObligationBefore: h.obligation, ObligationAfter: h.obligation,
		ObligationLimit: h.cfg.MaxAbsObligationQty, PositionBefore: h.position,
		DecisionInterval: int64(h.cfg.DecisionInterval), DecisionPhaseOffset: int64(h.cfg.DecisionPhaseOffset),
		ObligationInterval: int64(h.cfg.ObligationInterval),
		LastBookSourceTime: h.book.SourceTime, LastBookReceivedTime: h.book.ReceivedTime,
		LastBookSequence: h.book.Sequence, HasSnapshot: h.book.HasSnapshot,
		HasBid: h.book.HasBid, BidPrice: h.book.BidPrice, BidVisibleQty: h.book.BidQty,
		HasAsk: h.book.HasAsk, AskPrice: h.book.AskPrice, AskVisibleQty: h.book.AskQty,
		TakerFeeBps: h.cfg.TakerFeeBps,
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
