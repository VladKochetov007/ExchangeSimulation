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

// PerpExposureHedgerConfig declares a venue-local participant with a bounded
// local exposure mandate and ordinary perpetual orders. The physical state is
// a motive only: it does not write a price, funding rate, exchange balance, or
// counterparty order.
type PerpExposureHedgerConfig struct {
	Enabled bool   `json:"enabled"`
	Symbol  string `json:"symbol"`
	// ExposureMode selects the declared local exposure policy. An empty value is
	// the historical bounded random-walk P2 policy. fixed_liability is the V2-7
	// policy: the participant enters one fixed hedge and holds it, so a later
	// exchange-side liquidation cannot silently turn into a new entry.
	// fixed_directional is a finite-capital one-sided perpetual mandate whose
	// target is held after entry; it has no off-exchange physical offset.
	ExposureMode              string `json:"exposure_mode,omitempty"`
	InitialPhysicalExposure   int64  `json:"initial_physical_exposure,omitempty"`
	InitialTargetPerpPosition int64  `json:"initial_target_perp_position,omitempty"`
	// AutoBorrowPerp is an explicit balance-sheet policy for fixed directional
	// distress screens. It is passed to the venue borrowing contract; the
	// actor never receives an implicit credit exemption.
	AutoBorrowPerp   bool          `json:"auto_borrow_perp,omitempty"`
	DecisionInterval time.Duration `json:"decision_interval"`
	ExposureInterval time.Duration `json:"exposure_interval"`
	ExposureStepQty  int64         `json:"exposure_step_qty"`
	MaxAbsExposure   int64         `json:"max_abs_exposure"`
	MaxRequestQty    int64         `json:"max_request_qty"`
	TickSize         int64         `json:"tick_size"`

	// InitialQuoteBalance and InitialMargin are raw quote-asset quantities for
	// the ordinary venue account. They are declared in the experiment config so
	// the hedger cannot receive an implicit reserve exemption.
	InitialQuoteBalance int64 `json:"initial_quote_balance"`
	InitialMargin       int64 `json:"initial_margin"`

	Seed             int64                            `json:"-"`
	VenueID          string                           `json:"-"`
	Hedger           string                           `json:"-"`
	ClientID         uint64                           `json:"-"`
	TerminalNano     int64                            `json:"-"`
	TakerFeeBps      int64                            `json:"-"`
	DecisionObserver func(PerpExposureHedgerDecision) `json:"-"`
	FillObserver     func(PerpExposureHedgerFill)     `json:"-"`
}

func (c PerpExposureHedgerConfig) validate() error {
	if c.Symbol != "ABC-PERP" {
		return fmt.Errorf("perp exposure hedger must trade ABC-PERP, got %q", c.Symbol)
	}
	if c.ExposureMode != "" && c.ExposureMode != fixedLiabilityExposureMode && c.ExposureMode != fixedDirectionalExposureMode {
		return fmt.Errorf("unsupported exposure mode %q", c.ExposureMode)
	}
	if c.DecisionInterval <= 0 || c.ExposureInterval <= 0 {
		return fmt.Errorf("decision and exposure intervals must be positive")
	}
	if c.ExposureInterval%c.DecisionInterval != 0 {
		return fmt.Errorf("exposure interval must be a multiple of decision interval")
	}
	if c.ExposureStepQty <= 0 || c.MaxAbsExposure < c.ExposureStepQty {
		return fmt.Errorf("require positive exposure step no greater than maximum absolute exposure")
	}
	if c.ExposureMode == fixedLiabilityExposureMode {
		if c.InitialPhysicalExposure == 0 {
			return fmt.Errorf("fixed-liability exposure must be nonzero")
		}
		if c.InitialPhysicalExposure > c.MaxAbsExposure || c.InitialPhysicalExposure < -c.MaxAbsExposure {
			return fmt.Errorf("fixed-liability exposure %d exceeds absolute bound %d", c.InitialPhysicalExposure, c.MaxAbsExposure)
		}
	}
	if c.ExposureMode == fixedDirectionalExposureMode {
		if c.Enabled && c.InitialTargetPerpPosition == 0 {
			return fmt.Errorf("enabled fixed-directional target must be nonzero")
		}
		if c.InitialTargetPerpPosition > c.MaxAbsExposure || c.InitialTargetPerpPosition < -c.MaxAbsExposure {
			return fmt.Errorf("fixed-directional target %d exceeds absolute bound %d", c.InitialTargetPerpPosition, c.MaxAbsExposure)
		}
		if !c.AutoBorrowPerp {
			return fmt.Errorf("fixed-directional policy requires explicit perpetual auto-borrow")
		}
	} else if c.AutoBorrowPerp {
		return fmt.Errorf("perpetual auto-borrow is only valid for fixed-directional policy")
	}
	if c.MaxAbsExposure > math.MaxInt64-c.ExposureStepQty {
		return fmt.Errorf("exposure bounds leave no safe reflected update")
	}
	if c.MaxRequestQty <= 0 || c.TickSize <= 0 {
		return fmt.Errorf("maximum request quantity and tick size must be positive")
	}
	if c.InitialQuoteBalance <= 0 || c.InitialMargin <= 0 {
		return fmt.Errorf("initial quote balance and margin must be positive")
	}
	if c.TakerFeeBps < 0 || c.TakerFeeBps > 10_000 {
		return fmt.Errorf("taker fee bps must be in [0,10000]")
	}
	return nil
}

// PerpExposureHedgerDecision is append-only pre-ingress evidence for one
// complete policy evaluation. Numeric book values are never availability
// sentinels; the explicit presence fields carry that meaning. Side is a named
// string because BUY is a valid zero-valued internal enum.
type PerpExposureHedgerDecision struct {
	VenueID  string `json:"venue_id"`
	Hedger   string `json:"hedger"`
	ClientID uint64 `json:"client_id"`
	// PolicyVersion binds an evidence row to the exact local-exposure
	// contract. Offline auditors must reject a row from an unknown policy
	// rather than silently applying this replay to it.
	PolicyVersion         string `json:"policy_version"`
	ExposureMode          string `json:"exposure_mode,omitempty"`
	Symbol                string `json:"symbol"`
	DecisionTime          int64  `json:"decision_time"`
	Enabled               bool   `json:"enabled"`
	Subscribed            bool   `json:"subscribed"`
	RequestPending        bool   `json:"request_pending"`
	ActionOrDeferReason   string `json:"action_or_defer_reason"`
	PhysicalBefore        int64  `json:"physical_exposure_before"`
	PhysicalAfter         int64  `json:"physical_exposure_after"`
	PhysicalStep          int64  `json:"physical_exposure_step"`
	PhysicalExposureLimit int64  `json:"physical_exposure_limit"`
	FilledPerpPosition    int64  `json:"filled_perp_position"`
	TargetPerpPosition    int64  `json:"target_perp_position"`
	HedgeGap              int64  `json:"hedge_gap"`
	DecisionInterval      int64  `json:"decision_interval"`
	ExposureInterval      int64  `json:"exposure_interval"`

	HasSnapshot     bool   `json:"has_snapshot"`
	BookPublishedAt int64  `json:"book_published_at"`
	BookSequence    uint64 `json:"book_sequence"`
	BookFingerprint string `json:"book_fingerprint"`
	HasBid          bool   `json:"has_bid"`
	BidPrice        int64  `json:"bid_price"`
	BidVisibleQty   int64  `json:"bid_visible_qty"`
	HasAsk          bool   `json:"has_ask"`
	AskPrice        int64  `json:"ask_price"`
	AskVisibleQty   int64  `json:"ask_visible_qty"`

	// DecisionFrontier identifies the complete delayed public-feed prefix used
	// by this action. The cached book identity above must be independently
	// found in this prefix; telemetry is never a policy input.
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`

	Side               string `json:"side"`
	LimitPrice         int64  `json:"limit_price"`
	RequestedQty       int64  `json:"requested_qty"`
	RequestID          uint64 `json:"request_id"`
	TakerFeeBps        int64  `json:"taker_fee_bps"`
	OutcomeExpectation string `json:"outcome_expectation"`
	CensorReason       string `json:"censor_reason"`
}

// PerpExposureHedgerFill attests the submitting actor's local perpetual
// position transition around an exchange-confirmed fill. The generic fill is
// still the execution source of truth.
type PerpExposureHedgerFill struct {
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

type perpExposureBook struct {
	hasSnapshot bool
	publishedAt int64
	sequence    uint64
	fingerprint [16]byte
	hasBid      bool
	bidPrice    int64
	bidQty      int64
	hasAsk      bool
	askPrice    int64
	askQty      int64
}

// PerpExposureHedger works a declared perpetual target using no local
// reference price beyond the most recently delivered executable touch.
type PerpExposureHedger struct {
	*actor.BaseActor
	cfg PerpExposureHedgerConfig
	rng *rand.Rand

	book               perpExposureBook
	physicalExposure   int64
	targetPerpPosition int64
	perpPosition       int64
	lastUpdate         int64
	pending            bool
	pendingRequestID   uint64
	activeOrders       map[uint64]struct{}
	subscribed         bool
	entryComplete      bool
}

const fixedLiabilityExposureMode = "fixed_liability"
const fixedDirectionalExposureMode = "fixed_directional"

// NewPerpExposureHedger constructs an opt-in, locally informed P2 actor.
func NewPerpExposureHedger(id uint64, gateway actor.Gateway, cfg PerpExposureHedgerConfig) *PerpExposureHedger {
	h := &PerpExposureHedger{
		BaseActor:    actor.NewBaseActor(id, gateway),
		cfg:          cfg,
		rng:          rand.New(rand.NewSource(cfg.Seed)),
		activeOrders: make(map[uint64]struct{}),
	}
	if cfg.ExposureMode == fixedLiabilityExposureMode {
		h.physicalExposure = cfg.InitialPhysicalExposure
	} else if cfg.ExposureMode == fixedDirectionalExposureMode {
		h.targetPerpPosition = cfg.InitialTargetPerpPosition
	}
	h.SetHandler(h)
	h.AddTicker(cfg.DecisionInterval, h.onTick)
	return h
}

// PhysicalExposure returns the actor's signed off-exchange exposure motive.
func (h *PerpExposureHedger) PhysicalExposure() int64 { return h.physicalExposure }

// PerpPosition returns the actor-local filled perpetual inventory only.
func (h *PerpExposureHedger) PerpPosition() int64 { return h.perpPosition }

func (h *PerpExposureHedger) HandleEvent(_ context.Context, event *actor.Event) {
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

func (h *PerpExposureHedger) observeSnapshot(event actor.BookSnapshotEvent) {
	if event.Symbol != h.cfg.Symbol {
		return
	}
	book := perpExposureBook{hasSnapshot: true, publishedAt: event.Timestamp, sequence: event.SeqNum}
	if fingerprint, err := etypes.MarketDataFingerprint(&etypes.MarketDataMsg{Type: etypes.MDSnapshot, Symbol: event.Symbol, SeqNum: event.SeqNum, Timestamp: event.Timestamp, Data: event.Snapshot}); err == nil {
		book.fingerprint = fingerprint
	}
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) > 0 {
			book.hasBid = true
			book.bidPrice, book.bidQty = event.Snapshot.Bids[0].Price, event.Snapshot.Bids[0].VisibleQty
		}
		if len(event.Snapshot.Asks) > 0 {
			book.hasAsk = true
			book.askPrice, book.askQty = event.Snapshot.Asks[0].Price, event.Snapshot.Asks[0].VisibleQty
		}
	}
	h.book = book
}

func (h *PerpExposureHedger) onAccepted(event actor.OrderAcceptedEvent) {
	if event.RequestID != h.pendingRequestID {
		return
	}
	h.pendingRequestID = 0
	h.activeOrders[event.OrderID] = struct{}{}
}

func (h *PerpExposureHedger) onRejected(event actor.OrderRejectedEvent) {
	if event.RequestID != h.pendingRequestID {
		return
	}
	h.pending, h.pendingRequestID = false, 0
}

func (h *PerpExposureHedger) onFill(event actor.OrderFillEvent) {
	if event.Symbol != h.cfg.Symbol {
		return
	}
	if _, active := h.activeOrders[event.OrderID]; !active {
		return
	}
	prePosition := h.perpPosition
	var (
		next int64
		ok   bool
	)
	if event.Side == exchange.Buy {
		next, ok = etypes.TryAdd(h.perpPosition, event.Qty)
	} else {
		next, ok = etypes.TrySub(h.perpPosition, event.Qty)
	}
	if !ok {
		return
	}
	h.perpPosition = next
	if h.cfg.ExposureMode == fixedLiabilityExposureMode || h.cfg.ExposureMode == fixedDirectionalExposureMode {
		if target, targetOK := h.targetPosition(); targetOK && h.perpPosition == target {
			h.entryComplete = true
		}
	}
	if h.cfg.FillObserver != nil {
		h.cfg.FillObserver(PerpExposureHedgerFill{
			VenueID: h.cfg.VenueID, Hedger: h.cfg.Hedger, ClientID: h.cfg.ClientID,
			Symbol: event.Symbol, Timestamp: event.Timestamp, OrderID: event.OrderID, TradeID: event.TradeID,
			Side: event.Side.String(), Qty: event.Qty, Price: event.Price, FeeAmount: event.FeeAmount,
			FeeAsset: event.FeeAsset, PrePosition: prePosition, PostPosition: h.perpPosition,
		})
	}
	if event.IsFull {
		delete(h.activeOrders, event.OrderID)
		h.pending = false
	}
}

func (h *PerpExposureHedger) onCancelled(event actor.OrderCancelledEvent) {
	if _, active := h.activeOrders[event.OrderID]; !active {
		return
	}
	delete(h.activeOrders, event.OrderID)
	h.pending = false
}

func (h *PerpExposureHedger) onTick(now time.Time) {
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
	h.pending, h.pendingRequestID = true, decision.RequestID
	requestID := h.SubmitOrderWithTimeInForce(h.cfg.Symbol, namedExposureSide(decision.Side), exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
	if requestID != decision.RequestID {
		panic(fmt.Sprintf("perp exposure request ID changed from %d to %d", decision.RequestID, requestID))
	}
}

func (h *PerpExposureHedger) decision(now time.Time) PerpExposureHedgerDecision {
	decision := h.baseDecision(now, "")
	if h.cfg.ExposureMode != fixedLiabilityExposureMode && h.cfg.ExposureMode != fixedDirectionalExposureMode && (h.lastUpdate == 0 || now.UnixNano()-h.lastUpdate >= int64(h.cfg.ExposureInterval)) {
		step, next, ok := h.nextExposure()
		if !ok {
			decision.ActionOrDeferReason = "PHYSICAL_EXPOSURE_UPDATE_OVERFLOW"
			return decision
		}
		h.lastUpdate, h.physicalExposure = now.UnixNano(), next
		decision.PhysicalStep, decision.PhysicalAfter = step, next
	}
	decision.FilledPerpPosition = h.perpPosition
	target, ok := h.targetPosition()
	if !ok {
		decision.ActionOrDeferReason = "PERP_TARGET_UNREPRESENTABLE"
		return decision
	}
	decision.TargetPerpPosition = target
	gap, ok := etypes.TrySub(target, h.perpPosition)
	if !ok {
		decision.ActionOrDeferReason = "HEDGE_GAP_OVERFLOW"
		return decision
	}
	decision.HedgeGap = gap
	if !h.cfg.Enabled {
		decision.ActionOrDeferReason = "POLICY_DISABLED"
		return decision
	}
	if (h.cfg.ExposureMode == fixedLiabilityExposureMode || h.cfg.ExposureMode == fixedDirectionalExposureMode) && h.entryComplete {
		decision.ActionOrDeferReason = fixedExposureHeldAction(h.cfg.ExposureMode)
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
	if h.terminalRoundTripCensored(now.UnixNano()) {
		decision.ActionOrDeferReason = "SIMULATION_HORIZON_CENSORED"
		decision.OutcomeExpectation, decision.CensorReason = "SIMULATION_HORIZON_CENSORED", "terminal_horizon_before_round_trip"
		return decision
	}
	if !h.book.hasSnapshot {
		decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
		return decision
	}
	if h.book.publishedAt > now.UnixNano() {
		decision.ActionOrDeferReason = "LOCAL_BOOK_PUBLICATION_FUTURE"
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
		decision.Side = exchange.Buy.String()
		if !h.book.hasAsk {
			decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.LimitPrice = h.book.askPrice
	} else {
		decision.Side = exchange.Sell.String()
		if !h.book.hasBid {
			decision.ActionOrDeferReason = "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.LimitPrice = h.book.bidPrice
	}
	if !perpExposurePositiveGridPrice(decision.LimitPrice, h.cfg.TickSize) {
		decision.ActionOrDeferReason = "PERP_PRICE_OUTSIDE_DOMAIN"
		return decision
	}
	decision.RequestID = h.PeekNextRequestID()
	decision.ActionOrDeferReason, decision.OutcomeExpectation = "SUBMIT_IOC", "VENUE_OUTCOME_REQUIRED"
	return decision
}

func (h *PerpExposureHedger) terminalRoundTripCensored(now int64) bool {
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

func (h *PerpExposureHedger) baseDecision(now time.Time, action string) PerpExposureHedgerDecision {
	frontier := perpExposureFrontier(h.Gateway())
	return PerpExposureHedgerDecision{
		VenueID: h.cfg.VenueID, Hedger: h.cfg.Hedger, ClientID: h.cfg.ClientID,
		PolicyVersion: h.policyVersion(), ExposureMode: h.cfg.ExposureMode, Symbol: h.cfg.Symbol,
		DecisionTime: now.UnixNano(), Enabled: h.cfg.Enabled, Subscribed: h.subscribed, RequestPending: h.pending,
		ActionOrDeferReason: action, PhysicalBefore: h.physicalExposure, PhysicalAfter: h.physicalExposure,
		PhysicalExposureLimit: h.cfg.MaxAbsExposure, FilledPerpPosition: h.perpPosition,
		DecisionInterval: int64(h.cfg.DecisionInterval), ExposureInterval: int64(h.cfg.ExposureInterval),
		HasSnapshot: h.book.hasSnapshot, BookPublishedAt: h.book.publishedAt, BookSequence: h.book.sequence,
		BookFingerprint: fmt.Sprintf("%x", h.book.fingerprint),
		HasBid:          h.book.hasBid, BidPrice: h.book.bidPrice, BidVisibleQty: h.book.bidQty,
		HasAsk: h.book.hasAsk, AskPrice: h.book.askPrice, AskVisibleQty: h.book.askQty,
		DecisionFrontierLinkID: frontier.LinkID, DecisionFrontierOrdinal: frontier.Ordinal,
		DecisionFrontierDeliveredAt: frontier.DeliveredAt, DecisionFrontierDigest: fmt.Sprintf("%x", frontier.Digest),
		TakerFeeBps: h.cfg.TakerFeeBps,
	}
}

const perpExposureHedgerPolicyVersion = "v2_5_p2_perp_exposure_v1"
const fixedLiabilityHedgerPolicyVersion = "v2_7_fixed_liability_v1"
const fixedDirectionalHedgerPolicyVersion = "v2_7_fixed_directional_v1"

func (h *PerpExposureHedger) policyVersion() string {
	if h.cfg.ExposureMode == fixedLiabilityExposureMode {
		return fixedLiabilityHedgerPolicyVersion
	}
	if h.cfg.ExposureMode == fixedDirectionalExposureMode {
		return fixedDirectionalHedgerPolicyVersion
	}
	return perpExposureHedgerPolicyVersion
}

func (h *PerpExposureHedger) targetPosition() (int64, bool) {
	if h.cfg.ExposureMode == fixedDirectionalExposureMode {
		return h.targetPerpPosition, true
	}
	return etypes.TrySub(0, h.physicalExposure)
}

func fixedExposureHeldAction(mode string) string {
	if mode == fixedDirectionalExposureMode {
		return "FIXED_DIRECTIONAL_HELD"
	}
	return "FIXED_LIABILITY_HELD"
}

func (h *PerpExposureHedger) nextExposure() (int64, int64, bool) {
	step := h.cfg.ExposureStepQty
	if h.rng.Intn(2) == 0 {
		step = -step
	}
	next, ok := etypes.TryAdd(h.physicalExposure, step)
	if !ok || next > h.cfg.MaxAbsExposure || next < -h.cfg.MaxAbsExposure {
		step = -step
		next, ok = etypes.TryAdd(h.physicalExposure, step)
	}
	if !ok || next > h.cfg.MaxAbsExposure || next < -h.cfg.MaxAbsExposure || step == 0 {
		return 0, h.physicalExposure, false
	}
	return step, next, true
}

func (h *PerpExposureHedger) emit(decision PerpExposureHedgerDecision) {
	if h.cfg.DecisionObserver != nil {
		h.cfg.DecisionObserver(decision)
	}
}

func namedExposureSide(side string) exchange.Side {
	if side == exchange.Sell.String() {
		return exchange.Sell
	}
	return exchange.Buy
}

func perpExposurePositiveGridPrice(price, tick int64) bool {
	return price > 0 && tick > 0 && price%tick == 0
}

func perpExposureFrontier(gateway actor.Gateway) simulation.MarketDataFrontier {
	if source, ok := gateway.(interface {
		MarketDataFrontier() simulation.MarketDataFrontier
	}); ok {
		return source.MarketDataFrontier()
	}
	return simulation.MarketDataFrontier{}
}
