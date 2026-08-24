package multivenue

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

const (
	termCarryPolicyVersionV2 = "v2_5_p3_term_carry_v2"
	termCarryPolicyVersionV3 = "v2_5_p3_term_carry_v3"
)

// TermCarryAllocatorConfig declares an opt-in, lifecycle-bearing carry
// participant. It is deliberately distinct from FundingCarryArbitrageur: the
// declared commitment is actual actor state with a deterministic unwind, not
// merely an expected-income multiplier.
type TermCarryAllocatorConfig struct {
	Enabled             bool          `json:"enabled"`
	SpotSymbol          string        `json:"spot_symbol"`
	PerpSymbol          string        `json:"perp_symbol"`
	DecisionPeriod      time.Duration `json:"decision_period"`
	CommitmentIntervals int64         `json:"commitment_intervals"`
	MaxFundingAge       time.Duration `json:"max_funding_age"`
	TakerFeeBps         int64         `json:"taker_fee_bps"`
	LongSpotFundingBps  int64         `json:"long_spot_funding_bps"`
	ShortSpotBorrowBps  int64         `json:"short_spot_borrow_bps"`
	BalanceSheetBps     int64         `json:"balance_sheet_bps"`
	MarginRiskBps       int64         `json:"margin_risk_bps"`
	LegRiskBps          int64         `json:"leg_risk_bps"`
	MinNetCarryBps      int64         `json:"min_net_carry_bps"`
	MandateEndAtNano    int64         `json:"mandate_end_at_nano"` // Participant-known deadline; zero means no mandate, never simulator termination.
	MaxPosition         int64         `json:"max_position"`
	LotQty              int64         `json:"lot_qty"`
	MinOrderSize        int64         `json:"min_order_size"`
	// UnwindMinOrderSize is optional so legacy P3 policies retain their
	// entry-sized unwind floor. When present, it is an additional actor
	// materiality floor: zero means no additional floor. It never lowers the
	// instrument's MinOrderSize, which remains the exchange-admissible minimum
	// for every child. It is a quantity policy, never a price-availability
	// sentinel.
	UnwindMinOrderSize *int64                    `json:"unwind_min_order_size,omitempty"`
	SpotTick           int64                     `json:"spot_tick"`
	PerpTick           int64                     `json:"perp_tick"`
	VenueID            string                    `json:"-"`
	Desk               string                    `json:"-"`
	ClientID           uint64                    `json:"-"`
	DecisionObserver   func(TermCarryDecision)   `json:"-"`
	OutcomeObserver    func(TermCarryLegOutcome) `json:"-"`
}

func (c TermCarryAllocatorConfig) validate() error {
	if c.SpotSymbol == "" || c.PerpSymbol == "" {
		return fmt.Errorf("spot and perpetual symbols are required")
	}
	if c.DecisionPeriod <= 0 || c.MaxFundingAge <= 0 || c.CommitmentIntervals <= 0 {
		return fmt.Errorf("decision period, maximum funding age, and commitment intervals must be positive")
	}
	if c.MandateEndAtNano < 0 {
		return fmt.Errorf("mandate end must be non-negative")
	}
	if c.MaxPosition <= 0 || c.LotQty <= 0 || c.MinOrderSize < 0 || c.SpotTick <= 0 || c.PerpTick <= 0 {
		return fmt.Errorf("position, lot, and tick policy inputs are invalid")
	}
	if c.UnwindMinOrderSize != nil && *c.UnwindMinOrderSize < 0 {
		return fmt.Errorf("unwind minimum order size must be non-negative")
	}
	for _, component := range []struct {
		name  string
		value int64
	}{
		{"taker fee", c.TakerFeeBps}, {"long spot financing", c.LongSpotFundingBps},
		{"short spot borrow", c.ShortSpotBorrowBps}, {"balance sheet", c.BalanceSheetBps},
		{"margin risk", c.MarginRiskBps}, {"leg risk", c.LegRiskBps}, {"minimum net carry", c.MinNetCarryBps},
	} {
		if component.value < 0 {
			return fmt.Errorf("%s bps must be non-negative", component.name)
		}
	}
	return nil
}

// TermCarryState is the explicit ownership lifecycle. No state is inferred
// from a numeric position alone: partial non-atomic legs remain observable.
type TermCarryState string

const (
	termCarryIdle       TermCarryState = "IDLE"
	termCarryEntrySpot  TermCarryState = "ENTRY_SPOT"
	termCarryEntryPerp  TermCarryState = "ENTRY_PERP"
	termCarryActive     TermCarryState = "ACTIVE_TERM"
	termCarryUnwindPerp TermCarryState = "UNWIND_PERP"
	termCarryUnwindSpot TermCarryState = "UNWIND_SPOT"
)

// TermCarryDecision attests one policy evaluation before ingress. String
// rational components avoid silently rounding a nonzero term-financing cost
// to integer bps. Numeric zero remains present whenever HasFunding is true.
type TermCarryDecision struct {
	VenueID       string         `json:"venue_id"`
	Desk          string         `json:"desk"`
	ClientID      uint64         `json:"client_id"`
	PolicyVersion string         `json:"policy_version"`
	DecisionTime  int64          `json:"decision_time"`
	Enabled       bool           `json:"enabled"`
	Subscribed    bool           `json:"subscribed"`
	Pending       bool           `json:"pending"`
	State         TermCarryState `json:"state"`
	Action        string         `json:"action_or_defer_reason"`

	SpotSymbol          string `json:"spot_symbol"`
	PerpSymbol          string `json:"perp_symbol"`
	SpotPosition        int64  `json:"spot_position"`
	PerpPosition        int64  `json:"perp_position"`
	TargetSpot          int64  `json:"target_spot_position"`
	TargetPerp          int64  `json:"target_perp_position"`
	PlanCreatedAt       int64  `json:"plan_created_at"`
	FirstExposureAt     int64  `json:"first_exposure_at"`
	TermEnd             int64  `json:"term_end"`
	MandateEndAt        int64  `json:"mandate_end_at"`
	CommitmentIntervals int64  `json:"commitment_intervals"`
	// UnwindMinOrderSize is present only for the explicit v3 policy, including
	// a legitimate zero. Its pointer distinguishes a v2 inherited entry floor
	// from an explicit policy with no additional unwind materiality floor; the
	// exchange minimum remains separately binding.
	UnwindMinOrderSize *int64 `json:"unwind_min_order_size,omitempty"`

	HasSpotBook     bool   `json:"has_spot_book"`
	SpotPublishedAt int64  `json:"spot_published_at"`
	SpotSequence    uint64 `json:"spot_sequence"`
	HasSpotBid      bool   `json:"has_spot_bid"`
	SpotBid         int64  `json:"spot_bid"`
	SpotBidQty      int64  `json:"spot_bid_qty"`
	HasSpotAsk      bool   `json:"has_spot_ask"`
	SpotAsk         int64  `json:"spot_ask"`
	SpotAskQty      int64  `json:"spot_ask_qty"`
	HasPerpBook     bool   `json:"has_perp_book"`
	PerpPublishedAt int64  `json:"perp_published_at"`
	PerpSequence    uint64 `json:"perp_sequence"`
	HasPerpBid      bool   `json:"has_perp_bid"`
	PerpBid         int64  `json:"perp_bid"`
	PerpBidQty      int64  `json:"perp_bid_qty"`
	HasPerpAsk      bool   `json:"has_perp_ask"`
	PerpAsk         int64  `json:"perp_ask"`
	PerpAskQty      int64  `json:"perp_ask_qty"`

	HasFunding                  bool   `json:"has_funding"`
	FundingRateBps              int64  `json:"funding_rate_bps"`
	FundingPublishedAt          int64  `json:"funding_published_at"`
	FundingSequence             uint64 `json:"funding_sequence"`
	FundingNextAt               int64  `json:"funding_next_at"`
	FundingIntervalSeconds      int64  `json:"funding_interval_seconds"`
	FundingAgeNanos             int64  `json:"funding_age_nanos"`
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`

	ExpectedFundingBps    string `json:"expected_funding_bps"`
	ExecutionFeeBps       string `json:"execution_fee_bps"`
	FinancingBpsNumerator string `json:"financing_bps_numerator"`
	NetCarryBpsNumerator  string `json:"net_carry_bps_numerator"`
	RationalDenominator   string `json:"rational_denominator"`
	FinancingDirection    string `json:"financing_direction"`

	Leg          string `json:"leg"`
	Side         string `json:"side"`
	LimitPrice   int64  `json:"limit_price"`
	RequestedQty int64  `json:"requested_qty"`
	RequestID    uint64 `json:"request_id"`
}

// TermCarryLegOutcome links a normal venue response to the lifecycle state
// that sent it. The exchange's canonical OrderFill remains the source of
// truth; this is actor-side attestation for independent lifecycle replay.
type TermCarryLegOutcome struct {
	VenueID            string         `json:"venue_id"`
	Desk               string         `json:"desk"`
	ClientID           uint64         `json:"client_id"`
	DecisionTime       int64          `json:"decision_time"`
	ExecutionTime      int64          `json:"execution_time"`
	State              TermCarryState `json:"state"`
	Event              string         `json:"event"`
	Leg                string         `json:"leg"`
	RequestID          uint64         `json:"request_id"`
	OrderID            uint64         `json:"order_id"`
	TradeID            uint64         `json:"trade_id"`
	Symbol             string         `json:"symbol"`
	Side               string         `json:"side"`
	Qty                int64          `json:"qty"`
	Price              int64          `json:"price"`
	FeeAmount          int64          `json:"fee_amount"`
	FeeAsset           string         `json:"fee_asset"`
	RemainingQty       int64          `json:"remaining_qty"`
	RejectReason       string         `json:"reject_reason"`
	SpotPositionBefore int64          `json:"spot_position_before"`
	SpotPositionAfter  int64          `json:"spot_position_after"`
	PerpPositionBefore int64          `json:"perp_position_before"`
	PerpPositionAfter  int64          `json:"perp_position_after"`
}

type termCarryPlan struct {
	direction       int64
	planCreatedAt   int64
	firstExposureAt int64
	termEnd         int64
}

type termCarryPending struct {
	requestID    uint64
	orderID      uint64
	leg          string
	symbol       string
	decisionTime int64
	state        TermCarryState
}

// TermCarryAllocator is a finite, local-information participant with a
// declared ownership term. It deliberately never reads its evidence recorder
// or receipt sidecars.
type TermCarryAllocator struct {
	*actor.BaseActor
	cfg          TermCarryAllocatorConfig
	spot         fundingCarryBook
	perp         fundingCarryBook
	funding      fundingCarryFunding
	spotPosition int64
	perpPosition int64
	state        TermCarryState
	plan         *termCarryPlan
	pending      *termCarryPending
	subscribed   bool
}

// NewTermCarryAllocator constructs the opt-in P3 participant.
func NewTermCarryAllocator(id uint64, gateway actor.Gateway, cfg TermCarryAllocatorConfig) *TermCarryAllocator {
	allocator := &TermCarryAllocator{BaseActor: actor.NewBaseActor(id, gateway), cfg: cfg, state: termCarryIdle}
	allocator.SetHandler(allocator)
	allocator.AddTicker(cfg.DecisionPeriod, allocator.onTick)
	return allocator
}

func (a *TermCarryAllocator) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		a.observeBook(event.Data.(actor.BookSnapshotEvent))
	case actor.EventFundingUpdate:
		a.observeFunding(event.Data.(actor.FundingUpdateEvent))
	case actor.EventOrderAccepted:
		a.onAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		a.onRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		a.onFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		a.onCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (a *TermCarryAllocator) observeBook(event actor.BookSnapshotEvent) {
	var target *fundingCarryBook
	switch event.Symbol {
	case a.cfg.SpotSymbol:
		target = &a.spot
	case a.cfg.PerpSymbol:
		target = &a.perp
	default:
		return
	}
	book := fundingCarryBook{hasSnapshot: true, publishedAt: event.Timestamp, sequence: event.SeqNum}
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) > 0 {
			book.hasBid, book.bid, book.bidQty = true, event.Snapshot.Bids[0].Price, event.Snapshot.Bids[0].VisibleQty
		}
		if len(event.Snapshot.Asks) > 0 {
			book.hasAsk, book.ask, book.askQty = true, event.Snapshot.Asks[0].Price, event.Snapshot.Asks[0].VisibleQty
		}
	}
	*target = book
}

func (a *TermCarryAllocator) observeFunding(event actor.FundingUpdateEvent) {
	if event.Symbol != a.cfg.PerpSymbol || event.FundingRate == nil {
		return
	}
	if a.funding.has && event.SeqNum != 0 && event.SeqNum <= a.funding.sequence {
		return
	}
	a.funding = fundingCarryFunding{has: true, rate: *event.FundingRate, publishedAt: event.Timestamp, sequence: event.SeqNum}
}

func (a *TermCarryAllocator) onTick(now time.Time) {
	if !a.subscribed {
		decision := a.baseDecision(now, "NOT_SUBSCRIBED")
		a.Subscribe(a.cfg.SpotSymbol, exchange.MDSnapshot)
		a.Subscribe(a.cfg.PerpSymbol, exchange.MDSnapshot, exchange.MDFunding)
		a.subscribed = true
		a.emitDecision(decision)
		return
	}
	decision := a.decision(now)
	a.emitDecision(decision)
	if decision.RequestID == 0 {
		return
	}
	a.pending = &termCarryPending{requestID: decision.RequestID, leg: decision.Leg, symbol: termCarryLegSymbol(a.cfg, decision.Leg), decisionTime: decision.DecisionTime, state: a.state}
	requestID := a.SubmitOrderWithTimeInForce(a.pending.symbol, termCarrySide(decision.Side), exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
	if requestID != decision.RequestID {
		panic(fmt.Sprintf("term carry request ID changed from %d to %d", decision.RequestID, requestID))
	}
}

func (a *TermCarryAllocator) decision(now time.Time) TermCarryDecision {
	decision := a.baseDecision(now, "")
	if !a.cfg.Enabled {
		decision.Action = "POLICY_DISABLED"
		return decision
	}
	if a.pending != nil {
		decision.Action = "REQUEST_PENDING"
		return decision
	}
	switch a.state {
	case termCarryIdle:
		return a.openDecision(decision, now)
	case termCarryEntrySpot, termCarryEntryPerp:
		if a.plan == nil {
			decision.Action = "ENTRY_PLAN_UNAVAILABLE"
			return decision
		}
		if now.UnixNano() >= a.plan.termEnd {
			a.state = termCarryUnwindPerp
			decision.State = a.state
			return a.unwindDecision(decision)
		}
		if a.spotPosition != 0 && a.perpPosition == -a.spotPosition {
			a.state = termCarryActive
			decision.State = a.state
			decision.Action = "TERM_ACTIVE"
			return decision
		}
		a.state = termCarryEntryPerp
		decision.State = a.state
		return a.adjustPerpDecision(decision, "ENTRY_PERP_IOC")
	case termCarryActive:
		if a.plan == nil {
			decision.Action = "ACTIVE_PLAN_UNAVAILABLE"
			return decision
		}
		if now.UnixNano() < a.plan.termEnd {
			decision.Action = "TERM_ACTIVE"
			return decision
		}
		a.state = termCarryUnwindPerp
		decision.State = a.state
		return a.unwindDecision(decision)
	case termCarryUnwindPerp, termCarryUnwindSpot:
		return a.unwindDecision(decision)
	default:
		decision.Action = "UNKNOWN_LIFECYCLE_STATE"
		return decision
	}
}

func (a *TermCarryAllocator) openDecision(decision TermCarryDecision, now time.Time) TermCarryDecision {
	if !a.funding.has {
		decision.Action = "FUNDING_UNAVAILABLE"
		return decision
	}
	if a.funding.sequence == 0 {
		decision.Action = "FUNDING_IDENTITY_UNAVAILABLE"
		return decision
	}
	if a.funding.publishedAt > now.UnixNano() {
		decision.Action = "FUNDING_PUBLICATION_FUTURE"
		return decision
	}
	age, ok := fundingCarrySub(now.UnixNano(), a.funding.publishedAt)
	if !ok {
		decision.Action = "FUNDING_AGE_UNREPRESENTABLE"
		return decision
	}
	decision.FundingAgeNanos = age
	if age > int64(a.cfg.MaxFundingAge) {
		decision.Action = "FUNDING_STALE"
		return decision
	}
	if !a.funding.rate.MarkAvailable || !a.funding.rate.IndexAvailable {
		decision.Action = "FUNDING_REFERENCE_UNAVAILABLE"
		return decision
	}
	spotMid, perpMid, reason := fundingCarryMids(a.spot, a.perp)
	if reason != "" {
		decision.Action = reason
		return decision
	}
	if !termCarryMandateAllowsEntry(a.cfg, now.UnixNano(), a.funding.rate) {
		decision.Action = "TERM_HORIZON_CENSORED"
		return decision
	}
	direction := int64(1)
	if perpMid < spotMid {
		direction = -1
	}
	if perpMid == spotMid {
		decision.Action = "ZERO_PREMIUM"
		return decision
	}
	financials, ok := termCarryComputeFinancials(a.cfg, a.funding.rate, now.UnixNano(), direction)
	if !ok {
		decision.Action = "TERM_FINANCIALS_UNAVAILABLE"
		return decision
	}
	decision.TermEnd = financials.termEnd
	decision.ExpectedFundingBps = financials.fundingIncome.String()
	decision.ExecutionFeeBps = financials.fees.String()
	decision.FinancingBpsNumerator = financials.financing.String()
	decision.NetCarryBpsNumerator = financials.net.String()
	decision.RationalDenominator = financials.denominator.String()
	decision.FinancingDirection = financials.financingDirection
	minimum := new(big.Int).Mul(big.NewInt(a.cfg.MinNetCarryBps), financials.denominator)
	if financials.net.Cmp(minimum) < 0 {
		decision.Action = "NET_CARRY_BELOW_MINIMUM"
		return decision
	}
	if direction > 0 {
		decision.TargetSpot = a.cfg.MaxPosition
	} else {
		decision.TargetSpot = -a.cfg.MaxPosition
	}
	decision.TargetPerp = -decision.TargetSpot
	a.plan = &termCarryPlan{direction: direction, planCreatedAt: now.UnixNano(), termEnd: financials.termEnd}
	a.state = termCarryEntrySpot
	decision.State, decision.PlanCreatedAt, decision.TermEnd = a.state, a.plan.planCreatedAt, a.plan.termEnd
	decision = a.adjustSpotDecision(decision, "ENTRY_SPOT_IOC")
	if !termCarrySubmitsOrder(decision.Action) {
		// A price/size defer has no economic exposure and therefore no ownership
		// term. Retaining the provisional horizon would turn a missing book or
		// zero-fill admission into fictitious carry time. A later tick must
		// recompute its economics from its then-local observations.
		a.resetFlatEntryPlan()
		decision.State, decision.PlanCreatedAt, decision.FirstExposureAt, decision.TermEnd = a.state, 0, 0, 0
		decision.TargetSpot, decision.TargetPerp = 0, 0
	}
	return decision
}

func (a *TermCarryAllocator) adjustSpotDecision(decision TermCarryDecision, action string) TermCarryDecision {
	if a.plan == nil {
		decision.Action = "ENTRY_PLAN_UNAVAILABLE"
		return decision
	}
	target := a.cfg.MaxPosition
	if a.plan.direction < 0 {
		target = -target
	}
	gap, ok := fundingCarrySub(target, a.spotPosition)
	if !ok {
		decision.Action = "SPOT_TARGET_UNREPRESENTABLE"
		return decision
	}
	if gap == 0 {
		a.state = termCarryEntryPerp
		decision.State = a.state
		return a.adjustPerpDecision(decision, "ENTRY_PERP_IOC")
	}
	return a.orderFromGap(decision, a.spot, gap, a.cfg.SpotTick, a.cfg.MinOrderSize, action, "SPOT_ENTRY_PRICE_UNAVAILABLE", "SPOT_ENTRY_PRICE_OUTSIDE_DOMAIN")
}

func (a *TermCarryAllocator) adjustPerpDecision(decision TermCarryDecision, action string) TermCarryDecision {
	target, ok := fundingCarrySub(0, a.spotPosition)
	if !ok {
		decision.Action = "PERP_TARGET_UNREPRESENTABLE"
		return decision
	}
	gap, ok := fundingCarrySub(target, a.perpPosition)
	if !ok {
		decision.Action = "PERP_GAP_UNREPRESENTABLE"
		return decision
	}
	if gap == 0 && a.spotPosition != 0 {
		a.state = termCarryActive
		decision.State = a.state
		decision.Action = "TERM_ACTIVE"
		return decision
	}
	if gap == 0 {
		return a.adjustSpotDecision(decision, "ENTRY_SPOT_IOC")
	}
	return a.orderFromGap(decision, a.perp, gap, a.cfg.PerpTick, a.cfg.MinOrderSize, action, "PERP_ENTRY_PRICE_UNAVAILABLE", "PERP_ENTRY_PRICE_OUTSIDE_DOMAIN")
}

func (a *TermCarryAllocator) unwindDecision(decision TermCarryDecision) TermCarryDecision {
	if a.perpPosition != 0 {
		a.state = termCarryUnwindPerp
		decision.State = a.state
		gap, ok := fundingCarrySub(0, a.perpPosition)
		if !ok {
			decision.Action = "UNWIND_PERP_GAP_UNREPRESENTABLE"
			return decision
		}
		return a.orderFromGap(decision, a.perp, gap, a.cfg.PerpTick, a.unwindMinOrderSize(), "UNWIND_PERP_IOC", "UNWIND_PRICE_UNAVAILABLE", "UNWIND_PRICE_OUTSIDE_DOMAIN")
	}
	if a.spotPosition != 0 {
		a.state = termCarryUnwindSpot
		decision.State = a.state
		gap, ok := fundingCarrySub(0, a.spotPosition)
		if !ok {
			decision.Action = "UNWIND_SPOT_GAP_UNREPRESENTABLE"
			return decision
		}
		return a.orderFromGap(decision, a.spot, gap, a.cfg.SpotTick, a.unwindMinOrderSize(), "UNWIND_SPOT_IOC", "UNWIND_PRICE_UNAVAILABLE", "UNWIND_PRICE_OUTSIDE_DOMAIN")
	}
	a.state, a.plan = termCarryIdle, nil
	decision.State = a.state
	decision.Action = "TERM_CLOSED"
	return decision
}

func (a *TermCarryAllocator) orderFromGap(decision TermCarryDecision, book fundingCarryBook, gap, tick, minOrderSize int64, action, unavailable, outsideDomain string) TermCarryDecision {
	quantity, ok := nonnegativeMagnitude(gap)
	if !ok {
		decision.Action = "ORDER_QUANTITY_UNREPRESENTABLE"
		return decision
	}
	quantity = minInt64(quantity, a.cfg.LotQty)
	if quantity <= 0 {
		decision.Action = "ORDER_ZERO_QUANTITY"
		return decision
	}
	if gap > 0 {
		if !book.hasAsk {
			decision.Action = unavailable
			return decision
		}
		decision.Side, decision.LimitPrice = exchange.Buy.String(), book.ask
	} else {
		if !book.hasBid {
			decision.Action = unavailable
			return decision
		}
		decision.Side, decision.LimitPrice = exchange.Sell.String(), book.bid
	}
	if !fundingCarryPositiveGridPrice(decision.LimitPrice, tick) {
		decision.Action = outsideDomain
		return decision
	}
	if sized, ok := venueSizedQty(quantity, fundingCarryAvailable(book, decision.Side), minOrderSize); ok {
		decision.RequestedQty = sized
	} else {
		decision.Action = "EXECUTABLE_SIZE_UNAVAILABLE"
		return decision
	}
	decision.Leg, decision.RequestID, decision.Action = action, a.PeekNextRequestID(), "SUBMIT_"+action
	return decision
}

func (a *TermCarryAllocator) baseDecision(now time.Time, action string) TermCarryDecision {
	frontier := fundingCarryFrontier(a.Gateway())
	planCreatedAt, firstExposureAt, termEnd := int64(0), int64(0), int64(0)
	if a.plan != nil {
		planCreatedAt, firstExposureAt, termEnd = a.plan.planCreatedAt, a.plan.firstExposureAt, a.plan.termEnd
	}
	targetSpot := int64(0)
	if a.plan != nil && a.state != termCarryUnwindPerp && a.state != termCarryUnwindSpot {
		targetSpot = a.cfg.MaxPosition
		if a.plan.direction < 0 {
			targetSpot = -targetSpot
		}
	}
	decision := TermCarryDecision{
		VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, PolicyVersion: a.policyVersion(),
		DecisionTime: now.UnixNano(), Enabled: a.cfg.Enabled, Subscribed: a.subscribed, Pending: a.pending != nil, State: a.state, Action: action,
		SpotSymbol: a.cfg.SpotSymbol, PerpSymbol: a.cfg.PerpSymbol, SpotPosition: a.spotPosition, PerpPosition: a.perpPosition, TargetSpot: targetSpot, TargetPerp: -targetSpot,
		PlanCreatedAt: planCreatedAt, FirstExposureAt: firstExposureAt, TermEnd: termEnd, MandateEndAt: a.cfg.MandateEndAtNano, CommitmentIntervals: a.cfg.CommitmentIntervals,
		HasSpotBook: a.spot.hasSnapshot, SpotPublishedAt: a.spot.publishedAt, SpotSequence: a.spot.sequence, HasSpotBid: a.spot.hasBid, SpotBid: a.spot.bid, SpotBidQty: a.spot.bidQty, HasSpotAsk: a.spot.hasAsk, SpotAsk: a.spot.ask, SpotAskQty: a.spot.askQty,
		HasPerpBook: a.perp.hasSnapshot, PerpPublishedAt: a.perp.publishedAt, PerpSequence: a.perp.sequence, HasPerpBid: a.perp.hasBid, PerpBid: a.perp.bid, PerpBidQty: a.perp.bidQty, HasPerpAsk: a.perp.hasAsk, PerpAsk: a.perp.ask, PerpAskQty: a.perp.askQty,
		HasFunding: a.funding.has, FundingRateBps: a.funding.rate.Rate, FundingPublishedAt: a.funding.publishedAt, FundingSequence: a.funding.sequence, FundingNextAt: a.funding.rate.NextFunding, FundingIntervalSeconds: a.funding.rate.Interval, FundingAgeNanos: 0,
		DecisionFrontierLinkID: frontier.LinkID, DecisionFrontierOrdinal: frontier.Ordinal, DecisionFrontierDeliveredAt: frontier.DeliveredAt, DecisionFrontierDigest: fmt.Sprintf("%x", frontier.Digest),
	}
	if a.cfg.UnwindMinOrderSize != nil {
		configuredMinimum := *a.cfg.UnwindMinOrderSize
		decision.UnwindMinOrderSize = &configuredMinimum
	}
	return decision
}

func (a *TermCarryAllocator) policyVersion() string {
	if a.cfg.UnwindMinOrderSize != nil {
		return termCarryPolicyVersionV3
	}
	return termCarryPolicyVersionV2
}

func (a *TermCarryAllocator) unwindMinOrderSize() int64 {
	if a.cfg.UnwindMinOrderSize != nil && *a.cfg.UnwindMinOrderSize > a.cfg.MinOrderSize {
		return *a.cfg.UnwindMinOrderSize
	}
	return a.cfg.MinOrderSize
}

func (a *TermCarryAllocator) onAccepted(event actor.OrderAcceptedEvent) {
	if a.pending == nil || event.RequestID != a.pending.requestID {
		return
	}
	a.pending.orderID = event.OrderID
	a.emitOutcome(TermCarryLegOutcome{VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, DecisionTime: a.pending.decisionTime, State: a.pending.state, Event: "ORDER_ACCEPTED", Leg: a.pending.leg, RequestID: event.RequestID, OrderID: event.OrderID, Symbol: a.pending.symbol, SpotPositionBefore: a.spotPosition, SpotPositionAfter: a.spotPosition, PerpPositionBefore: a.perpPosition, PerpPositionAfter: a.perpPosition})
}

func (a *TermCarryAllocator) onRejected(event actor.OrderRejectedEvent) {
	if a.pending == nil || event.RequestID != a.pending.requestID {
		return
	}
	a.emitOutcome(TermCarryLegOutcome{VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, DecisionTime: a.pending.decisionTime, State: a.pending.state, Event: "ORDER_REJECTED", Leg: a.pending.leg, RequestID: event.RequestID, Symbol: a.pending.symbol, RejectReason: string(event.Reason), SpotPositionBefore: a.spotPosition, SpotPositionAfter: a.spotPosition, PerpPositionBefore: a.perpPosition, PerpPositionAfter: a.perpPosition})
	a.pending = nil
	a.resetFlatEntryPlan()
}

func (a *TermCarryAllocator) onFill(event actor.OrderFillEvent) {
	if a.pending == nil || event.OrderID != a.pending.orderID || event.Symbol != a.pending.symbol {
		return
	}
	beforeSpot, beforePerp := a.spotPosition, a.perpPosition
	signed := event.Qty
	if event.Side == exchange.Sell {
		signed = -signed
	}
	var ok bool
	if event.Symbol == a.cfg.SpotSymbol {
		a.spotPosition, ok = fundingCarryAdd(a.spotPosition, signed)
	} else {
		a.perpPosition, ok = fundingCarryAdd(a.perpPosition, signed)
	}
	if !ok {
		panic("term carry position overflow")
	}
	if a.plan != nil && a.plan.firstExposureAt == 0 {
		if event.Timestamp <= 0 {
			panic("term carry first exposure has nonpositive execution time")
		}
		a.plan.firstExposureAt = event.Timestamp
	}
	a.emitOutcome(TermCarryLegOutcome{VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, DecisionTime: a.pending.decisionTime, ExecutionTime: event.Timestamp, State: a.pending.state, Event: "ORDER_FILL", Leg: a.pending.leg, RequestID: a.pending.requestID, OrderID: event.OrderID, TradeID: event.TradeID, Symbol: event.Symbol, Side: event.Side.String(), Qty: event.Qty, Price: event.Price, FeeAmount: event.FeeAmount, FeeAsset: event.FeeAsset, SpotPositionBefore: beforeSpot, SpotPositionAfter: a.spotPosition, PerpPositionBefore: beforePerp, PerpPositionAfter: a.perpPosition})
	if event.IsFull {
		a.pending = nil
	}
}

func (a *TermCarryAllocator) onCancelled(event actor.OrderCancelledEvent) {
	if a.pending == nil || event.OrderID != a.pending.orderID {
		return
	}
	a.emitOutcome(TermCarryLegOutcome{VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, DecisionTime: a.pending.decisionTime, State: a.pending.state, Event: "ORDER_CANCELLED", Leg: a.pending.leg, RequestID: a.pending.requestID, OrderID: event.OrderID, Symbol: a.pending.symbol, RemainingQty: event.RemainingQty, SpotPositionBefore: a.spotPosition, SpotPositionAfter: a.spotPosition, PerpPositionBefore: a.perpPosition, PerpPositionAfter: a.perpPosition})
	a.pending = nil
	a.resetFlatEntryPlan()
}

// resetFlatEntryPlan abandons only a failed, fully flat admission attempt. A
// partially filled leg remains a real exposure and must continue through the
// deterministic hedge/unwind state machine instead of being erased.
func (a *TermCarryAllocator) resetFlatEntryPlan() {
	if a.pending != nil || a.spotPosition != 0 || a.perpPosition != 0 || (a.state != termCarryEntrySpot && a.state != termCarryEntryPerp) {
		return
	}
	a.state, a.plan = termCarryIdle, nil
}

func (a *TermCarryAllocator) emitDecision(decision TermCarryDecision) {
	if a.cfg.DecisionObserver != nil {
		a.cfg.DecisionObserver(decision)
	}
}

func (a *TermCarryAllocator) emitOutcome(outcome TermCarryLegOutcome) {
	if a.cfg.OutcomeObserver != nil {
		a.cfg.OutcomeObserver(outcome)
	}
}

func termCarryLegSymbol(cfg TermCarryAllocatorConfig, leg string) string {
	if leg == "ENTRY_PERP_IOC" || leg == "UNWIND_PERP_IOC" {
		return cfg.PerpSymbol
	}
	return cfg.SpotSymbol
}

func termCarrySide(side string) exchange.Side {
	if side == exchange.Sell.String() {
		return exchange.Sell
	}
	return exchange.Buy
}

func termCarrySubmitsOrder(action string) bool {
	switch action {
	case "SUBMIT_ENTRY_SPOT_IOC", "SUBMIT_ENTRY_PERP_IOC", "SUBMIT_UNWIND_PERP_IOC", "SUBMIT_UNWIND_SPOT_IOC":
		return true
	default:
		return false
	}
}

type termCarryFinancials struct {
	termEnd            int64
	fundingIncome      *big.Int
	fees               *big.Int
	financing          *big.Int
	net                *big.Int
	denominator        *big.Int
	financingDirection string
}

func termCarryComputeFinancials(cfg TermCarryAllocatorConfig, rate exchange.FundingRate, now, direction int64) (termCarryFinancials, bool) {
	if rate.NextFunding <= now || rate.Interval <= 0 || direction == 0 {
		return termCarryFinancials{}, false
	}
	end := new(big.Int).SetInt64(rate.NextFunding)
	extra := new(big.Int).Mul(big.NewInt(cfg.CommitmentIntervals-1), big.NewInt(rate.Interval))
	extra.Mul(extra, big.NewInt(int64(time.Second)))
	end.Add(end, extra)
	if !end.IsInt64() || end.Sign() <= 0 {
		return termCarryFinancials{}, false
	}
	holding := new(big.Int).Sub(end, big.NewInt(now))
	if holding.Sign() <= 0 {
		return termCarryFinancials{}, false
	}
	denominator := big.NewInt(fundingCarryYearNanos)
	funding := new(big.Int).Mul(big.NewInt(rate.Rate), big.NewInt(cfg.CommitmentIntervals))
	funding.Mul(funding, big.NewInt(direction))
	fees := new(big.Int).Mul(big.NewInt(cfg.TakerFeeBps), big.NewInt(4))
	financingRate, financingDirection := cfg.LongSpotFundingBps, "LONG_SPOT_CASH_FINANCING"
	if direction < 0 {
		financingRate, financingDirection = cfg.ShortSpotBorrowBps, "SHORT_SPOT_ASSET_BORROW"
	}
	financing := new(big.Int).Mul(big.NewInt(financingRate), holding)
	net := new(big.Int).Mul(funding, denominator)
	net.Sub(net, new(big.Int).Mul(fees, denominator))
	net.Sub(net, financing)
	fixed := new(big.Int).Add(big.NewInt(cfg.BalanceSheetBps), big.NewInt(cfg.MarginRiskBps))
	fixed.Add(fixed, big.NewInt(cfg.LegRiskBps))
	net.Sub(net, new(big.Int).Mul(fixed, denominator))
	return termCarryFinancials{termEnd: end.Int64(), fundingIncome: funding, fees: fees, financing: financing, net: net, denominator: denominator, financingDirection: financingDirection}, true
}

func termCarryMandateAllowsEntry(cfg TermCarryAllocatorConfig, now int64, rate exchange.FundingRate) bool {
	if cfg.MandateEndAtNano == 0 {
		return true
	}
	financials, ok := termCarryComputeFinancials(cfg, rate, now, 1)
	if !ok {
		return false
	}
	closeBudget := new(big.Int).Mul(big.NewInt(int64(cfg.DecisionPeriod)), big.NewInt(2))
	deadline := new(big.Int).Add(big.NewInt(financials.termEnd), closeBudget)
	return deadline.IsInt64() && deadline.Int64() <= cfg.MandateEndAtNano
}
