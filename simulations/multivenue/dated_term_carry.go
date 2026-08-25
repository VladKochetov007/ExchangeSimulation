package multivenue

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

const (
	datedTermCarryPolicyVersion = "v2_5_p5_dated_term_carry_v1"
	datedTermCarryYearNanos     = int64(365 * 24 * time.Hour)
)

type DatedTermCarryAllocatorConfig struct {
	Enabled                     bool                         `json:"enabled"`
	TradeEnabled                bool                         `json:"trade_enabled"`
	SpotSymbol                  string                       `json:"spot_symbol"`
	TargetTenor                 time.Duration                `json:"target_tenor_nanos"`
	DecisionPeriod              time.Duration                `json:"decision_period_nanos"`
	DecisionPhase               time.Duration                `json:"decision_phase_offset_nanos"`
	MaxMarketAge                time.Duration                `json:"max_market_age_nanos"`
	MinTimeToExpiry             time.Duration                `json:"min_time_to_expiry_nanos"`
	TakerFeeBps                 int64                        `json:"taker_fee_bps"`
	LongSpotFundingBps          int64                        `json:"long_spot_funding_bps"`
	ShortSpotBorrowBps          int64                        `json:"short_spot_borrow_bps"`
	BalanceSheetBps             int64                        `json:"balance_sheet_bps"`
	MarginRiskBps               int64                        `json:"margin_risk_bps"`
	LegRiskBps                  int64                        `json:"leg_risk_bps"`
	SettlementMismatchBps       int64                        `json:"settlement_mismatch_bps"`
	PostSettlementExitBps       int64                        `json:"post_settlement_exit_bps"`
	MinNetCarryBps              int64                        `json:"min_net_carry_bps"`
	MaxPosition                 int64                        `json:"max_position"`
	LotQty                      int64                        `json:"lot_qty"`
	MinOrderSize                int64                        `json:"min_order_size"`
	SpotTick                    int64                        `json:"spot_tick"`
	FutureTick                  int64                        `json:"future_tick"`
	PassiveExitSliceQty         int64                        `json:"passive_exit_slice_qty"`
	ExitDeadlineAfterSettlement time.Duration                `json:"exit_deadline_after_settlement_nanos"`
	TerminalNano                int64                        `json:"-"`
	VenueID                     string                       `json:"-"`
	Desk                        string                       `json:"-"`
	ClientID                    uint64                       `json:"-"`
	DecisionObserver            func(DatedTermCarryDecision) `json:"-"`
	OutcomeObserver             func(DatedTermCarryOutcome)  `json:"-"`
}

func (c DatedTermCarryAllocatorConfig) validate() error {
	if c.SpotSymbol == "" {
		return errors.New("spot symbol is required")
	}
	if c.TargetTenor <= 0 || c.DecisionPeriod <= 0 || c.MaxMarketAge <= 0 || c.MinTimeToExpiry <= 0 || c.MaxPosition <= 0 || c.LotQty <= 0 || c.MinOrderSize <= 0 || c.SpotTick <= 0 || c.FutureTick <= 0 || c.PassiveExitSliceQty <= 0 || c.ExitDeadlineAfterSettlement <= 0 {
		return errors.New("tenor, clocks, size, ticks, and exit policy must be positive")
	}
	if c.DecisionPhase < 0 || c.DecisionPhase >= c.DecisionPeriod {
		return errors.New("decision phase is outside its interval")
	}
	if c.LotQty > c.MaxPosition || c.LotQty < c.MinOrderSize || c.PassiveExitSliceQty < c.MinOrderSize {
		return errors.New("lot, cap, minimum, and passive slice are inconsistent")
	}
	for _, component := range []struct {
		name  string
		value int64
	}{
		{"taker fee", c.TakerFeeBps}, {"long spot financing", c.LongSpotFundingBps}, {"short spot borrow", c.ShortSpotBorrowBps},
		{"balance sheet", c.BalanceSheetBps}, {"margin risk", c.MarginRiskBps}, {"leg risk", c.LegRiskBps},
		{"settlement mismatch", c.SettlementMismatchBps}, {"post-settlement exit", c.PostSettlementExitBps}, {"minimum net carry", c.MinNetCarryBps},
	} {
		if component.value < 0 {
			return fmt.Errorf("%s bps must be non-negative", component.name)
		}
	}
	return nil
}

type DatedTermCarryState string

const (
	datedCarryIdle        DatedTermCarryState = "IDLE"
	datedCarryEntrySpot   DatedTermCarryState = "ENTRY_SPOT"
	datedCarryEntryFuture DatedTermCarryState = "ENTRY_FUTURE"
	datedCarryActive      DatedTermCarryState = "ACTIVE_TERM"
	datedCarryExitSpot    DatedTermCarryState = "POST_SETTLEMENT_EXIT_SPOT"
	datedCarryClosed      DatedTermCarryState = "CLOSED"
)

type DatedTermCarryDecision struct {
	VenueID       string              `json:"venue_id"`
	Desk          string              `json:"desk"`
	ClientID      uint64              `json:"client_id"`
	PolicyVersion string              `json:"policy_version"`
	DecisionTime  int64               `json:"decision_time"`
	Action        string              `json:"action_or_defer_reason"`
	Enabled       bool                `json:"enabled"`
	TradeEnabled  bool                `json:"trade_enabled"`
	Subscribed    bool                `json:"subscribed"`
	State         DatedTermCarryState `json:"state"`

	SpotSymbol           string `json:"spot_symbol"`
	FutureSymbol         string `json:"future_symbol"`
	ListedNano           int64  `json:"listed_nano"`
	ExpiryNano           int64  `json:"expiry_nano"`
	OriginalTenorNanos   int64  `json:"original_tenor_nanos"`
	ListingPublishedAt   int64  `json:"listing_published_at"`
	ListingSequence      uint64 `json:"listing_sequence"`
	ListingFingerprint   string `json:"listing_fingerprint"`
	TimeToExpiryNanos    int64  `json:"time_to_expiry_nanos"`
	SettlementObservedAt int64  `json:"settlement_observed_at"`
	ExitDeadlineAt       int64  `json:"exit_deadline_at"`

	SpotPosition         int64 `json:"spot_position"`
	FuturePosition       int64 `json:"future_position"`
	TargetSpot           int64 `json:"target_spot_position"`
	TargetFuture         int64 `json:"target_future_position"`
	ProposedTargetSpot   int64 `json:"proposed_target_spot_position"`
	ProposedTargetFuture int64 `json:"proposed_target_future_position"`
	TargetChangedAt      int64 `json:"target_changed_at"`

	HasSpotBook       bool   `json:"has_spot_book"`
	SpotPublishedAt   int64  `json:"spot_published_at"`
	SpotSequence      uint64 `json:"spot_sequence"`
	HasSpotBid        bool   `json:"has_spot_bid"`
	SpotBid           int64  `json:"spot_bid"`
	SpotBidQty        int64  `json:"spot_bid_qty"`
	HasSpotAsk        bool   `json:"has_spot_ask"`
	SpotAsk           int64  `json:"spot_ask"`
	SpotAskQty        int64  `json:"spot_ask_qty"`
	HasFutureBook     bool   `json:"has_future_book"`
	FuturePublishedAt int64  `json:"future_published_at"`
	FutureSequence    uint64 `json:"future_sequence"`
	HasFutureBid      bool   `json:"has_future_bid"`
	FutureBid         int64  `json:"future_bid"`
	FutureBidQty      int64  `json:"future_bid_qty"`
	HasFutureAsk      bool   `json:"has_future_ask"`
	FutureAsk         int64  `json:"future_ask"`
	FutureAskQty      int64  `json:"future_ask_qty"`
	SpotAgeNanos      int64  `json:"spot_age_nanos"`
	FutureAgeNanos    int64  `json:"future_age_nanos"`

	Direction                   string `json:"direction"`
	SpotExecutionReference      int64  `json:"spot_execution_reference"`
	FutureExecutionReference    int64  `json:"future_execution_reference"`
	GrossLockedSpreadRaw        string `json:"gross_locked_spread_raw"`
	GrossLockedBpsNumerator     string `json:"gross_locked_bps_numerator"`
	ExecutionFeeBpsNumerator    string `json:"execution_fee_bps_numerator"`
	FinancingBpsNumerator       string `json:"financing_bps_numerator"`
	BalanceSheetBpsNumerator    string `json:"balance_sheet_bps_numerator"`
	MarginRiskBpsNumerator      string `json:"margin_risk_bps_numerator"`
	LegRiskBpsNumerator         string `json:"leg_risk_bps_numerator"`
	SettlementMismatchNumerator string `json:"settlement_mismatch_bps_numerator"`
	PostSettlementExitNumerator string `json:"post_settlement_exit_bps_numerator"`
	NetCarryBpsNumerator        string `json:"net_carry_bps_numerator"`
	MinimumNetBpsNumerator      string `json:"minimum_net_bps_numerator"`
	RationalDenominator         string `json:"rational_denominator"`
	FinancingDirection          string `json:"financing_direction"`

	Leg                         string `json:"leg"`
	Side                        string `json:"side"`
	OrderType                   string `json:"order_type,omitempty"`
	TimeInForce                 string `json:"time_in_force,omitempty"`
	PostOnly                    *bool  `json:"post_only,omitempty"`
	LimitPrice                  int64  `json:"limit_price"`
	RequestedQty                int64  `json:"requested_qty"`
	RequestID                   uint64 `json:"request_id"`
	CancelOrderID               uint64 `json:"cancel_order_id"`
	CancelRequestID             uint64 `json:"cancel_request_id"`
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`
}

type DatedTermCarryOutcome struct {
	VenueID              string              `json:"venue_id"`
	Desk                 string              `json:"desk"`
	ClientID             uint64              `json:"client_id"`
	DecisionTime         int64               `json:"decision_time"`
	ExecutionTime        int64               `json:"execution_time"`
	State                DatedTermCarryState `json:"state"`
	Event                string              `json:"event"`
	Leg                  string              `json:"leg"`
	Symbol               string              `json:"symbol"`
	Side                 string              `json:"side"`
	RequestID            uint64              `json:"request_id"`
	OrderID              uint64              `json:"order_id"`
	TradeID              uint64              `json:"trade_id"`
	Qty                  int64               `json:"qty"`
	Price                int64               `json:"price"`
	FeeAmount            int64               `json:"fee_amount"`
	FeeAsset             string              `json:"fee_asset"`
	RemainingQty         int64               `json:"remaining_qty"`
	RejectReason         string              `json:"reject_reason"`
	CancelRequestID      uint64              `json:"cancel_request_id"`
	SpotPositionBefore   int64               `json:"spot_position_before"`
	SpotPositionAfter    int64               `json:"spot_position_after"`
	FuturePositionBefore int64               `json:"future_position_before"`
	FuturePositionAfter  int64               `json:"future_position_after"`
	HasSettlement        bool                `json:"has_settlement"`
	SettlementPrice      int64               `json:"settlement_price"`
}

type datedCarryFinancials struct {
	direction, financingDirection          string
	spotReference, futureReference         int64
	grossSpread, gross, fees, financing    *big.Int
	balance, margin, leg, settlement, exit *big.Int
	net, minimum, denominator              *big.Int
}

type datedCarryContract struct {
	symbol, underlying   string
	listedAt, expiryAt   int64
	listingPublishedAt   int64
	listingSequence      uint64
	listingFingerprint   string
	book                 fundingCarryBook
	state                DatedTermCarryState
	futurePosition       int64
	direction            int64
	targetChangedAt      int64
	settlementObservedAt int64
	exitDeadlineAt       int64
	closedEmitted        bool
}

type datedCarryPending struct {
	contract, symbol, leg               string
	requestID, orderID, cancelRequestID uint64
	decisionTime                        int64
	state                               DatedTermCarryState
	postOnly                            bool
}

type DatedTermCarryAllocator struct {
	*actor.BaseActor
	cfg          DatedTermCarryAllocatorConfig
	spot         fundingCarryBook
	spotPosition int64
	contracts    map[string]*datedCarryContract
	ownedSymbol  string
	pending      *datedCarryPending
	subscribed   bool
}

func NewDatedTermCarryAllocator(id uint64, gateway actor.Gateway, cfg DatedTermCarryAllocatorConfig) *DatedTermCarryAllocator {
	a := &DatedTermCarryAllocator{BaseActor: actor.NewBaseActor(id, gateway), cfg: cfg, contracts: make(map[string]*datedCarryContract)}
	a.SetHandler(a)
	a.AddTickerWithOffset(cfg.DecisionPeriod, cfg.DecisionPhase, a.onTick)
	return a
}

func (a *DatedTermCarryAllocator) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventInstrument:
		a.observeInstrument(event.Data.(actor.InstrumentEvent))
	case actor.EventBookSnapshot:
		a.observeBook(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		a.onAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		a.onRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		a.onFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		a.onCancelled(event.Data.(actor.OrderCancelledEvent))
	case actor.EventOrderCancelRejected:
		a.onCancelRejected(event.Data.(actor.OrderCancelRejectedEvent))
	}
}

func (a *DatedTermCarryAllocator) observeInstrument(event actor.InstrumentEvent) {
	ann := event.Announcement
	if ann == nil || ann.Underlying != a.cfg.SpotSymbol || ann.InstrumentType != "FUTURE" {
		return
	}
	if ann.Action == "settled" {
		a.observeSettlement(event)
		return
	}
	if ann.Action != "listed" || ann.ListedNano == nil {
		return
	}
	tenor, ok := etypes.TrySub(ann.ExpiryNano, *ann.ListedNano)
	if !ok || tenor != int64(a.cfg.TargetTenor) {
		return
	}
	if _, exists := a.contracts[ann.Symbol]; exists {
		return
	}
	fingerprint, ok := datedInstrumentFingerprint(event)
	if !ok {
		return
	}
	a.contracts[ann.Symbol] = &datedCarryContract{
		symbol: ann.Symbol, underlying: ann.Underlying, listedAt: *ann.ListedNano, expiryAt: ann.ExpiryNano,
		listingPublishedAt: event.Timestamp, listingSequence: event.SeqNum, listingFingerprint: fingerprint, state: datedCarryIdle,
	}
	a.Subscribe(ann.Symbol, exchange.MDSnapshot)
}

func (a *DatedTermCarryAllocator) observeSettlement(event actor.InstrumentEvent) {
	ann := event.Announcement
	contract := a.contracts[ann.Symbol]
	if contract == nil {
		return
	}
	beforeFuture := contract.futurePosition
	contract.futurePosition = 0
	contract.settlementObservedAt = event.Timestamp
	deadline, ok := etypes.TryAdd(event.Timestamp, int64(a.cfg.ExitDeadlineAfterSettlement))
	if !ok {
		panic("dated carry exit deadline overflow")
	}
	contract.exitDeadlineAt = deadline
	pendingSpot := a.pending != nil && a.pending.contract == contract.symbol && a.pending.symbol == a.cfg.SpotSymbol
	if a.ownedSymbol == contract.symbol && (a.spotPosition != 0 || pendingSpot) {
		contract.state = datedCarryExitSpot
	} else {
		contract.state = datedCarryClosed
	}
	// Do not erase an in-flight leg. Exchange cancellation/fill evidence owns
	// its terminal state; a late spot fill remains real residual exposure and a
	// future order must receive its canonical expiry cancellation.
	settlementPrice, hasSettlement := int64(0), ann.SettlementPrice != nil
	if hasSettlement {
		settlementPrice = *ann.SettlementPrice
	}
	a.emitOutcome(DatedTermCarryOutcome{
		VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, ExecutionTime: event.Timestamp,
		State: contract.state, Event: "CONTRACT_SETTLED", Symbol: contract.symbol,
		SpotPositionBefore: a.spotPosition, SpotPositionAfter: a.spotPosition,
		FuturePositionBefore: beforeFuture, FuturePositionAfter: 0, HasSettlement: hasSettlement, SettlementPrice: settlementPrice,
	})
}

func (a *DatedTermCarryAllocator) observeBook(event actor.BookSnapshotEvent) {
	book := fundingCarryBook{hasSnapshot: true, publishedAt: event.Timestamp, sequence: event.SeqNum}
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) > 0 {
			book.hasBid, book.bid, book.bidQty = true, event.Snapshot.Bids[0].Price, event.Snapshot.Bids[0].VisibleQty
		}
		if len(event.Snapshot.Asks) > 0 {
			book.hasAsk, book.ask, book.askQty = true, event.Snapshot.Asks[0].Price, event.Snapshot.Asks[0].VisibleQty
		}
	}
	if event.Symbol == a.cfg.SpotSymbol {
		a.spot = book
		return
	}
	if contract := a.contracts[event.Symbol]; contract != nil {
		contract.book = book
	}
}

func (a *DatedTermCarryAllocator) onTick(now time.Time) {
	if !a.subscribed {
		decision := a.baseDecision(now.UnixNano(), nil, "NOT_SUBSCRIBED")
		a.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		a.Subscribe(a.cfg.SpotSymbol, exchange.MDSnapshot)
		a.subscribed = true
		a.emitDecision(decision)
		return
	}
	contract := a.nextContract()
	if a.cfg.TerminalNano != 0 && now.UnixNano() >= a.cfg.TerminalNano {
		a.emitDecision(a.baseDecision(now.UnixNano(), contract, "SIMULATION_HORIZON_CENSORED"))
		return
	}
	decision := a.decide(now.UnixNano(), contract)
	a.emitDecision(decision)
	if decision.Action == "CANCEL_PASSIVE_EXIT_AT_DEADLINE" {
		requestID := a.CancelOrder(decision.CancelOrderID)
		if requestID != decision.CancelRequestID || a.pending == nil {
			panic("dated carry cancellation request identity changed")
		}
		a.pending.cancelRequestID = requestID
		return
	}
	if decision.RequestID == 0 {
		return
	}
	postOnly := decision.PostOnly != nil && *decision.PostOnly
	a.pending = &datedCarryPending{
		contract: contract.symbol, symbol: datedCarryLegSymbol(a.cfg.SpotSymbol, contract.symbol, decision.Leg), leg: decision.Leg,
		requestID: decision.RequestID, decisionTime: decision.DecisionTime, state: contract.state, postOnly: postOnly,
	}
	var requestID uint64
	if postOnly {
		requestID = a.SubmitPostOnlyOrder(a.pending.symbol, mandateSide(decision.Side), decision.LimitPrice, decision.RequestedQty)
	} else {
		requestID = a.SubmitOrderWithTimeInForce(a.pending.symbol, mandateSide(decision.Side), exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
	}
	if requestID != decision.RequestID {
		panic(fmt.Sprintf("dated carry request ID changed from %d to %d", decision.RequestID, requestID))
	}
}

func (a *DatedTermCarryAllocator) nextContract() *datedCarryContract {
	if a.ownedSymbol != "" {
		return a.contracts[a.ownedSymbol]
	}
	contracts := make([]*datedCarryContract, 0, len(a.contracts))
	for _, contract := range a.contracts {
		if contract.state == datedCarryIdle {
			contracts = append(contracts, contract)
		}
	}
	slices.SortFunc(contracts, func(x, y *datedCarryContract) int {
		if byExpiry := cmp.Compare(x.expiryAt, y.expiryAt); byExpiry != 0 {
			return byExpiry
		}
		return cmp.Compare(x.symbol, y.symbol)
	})
	if len(contracts) == 0 {
		return nil
	}
	return contracts[0]
}

func (a *DatedTermCarryAllocator) decide(now int64, contract *datedCarryContract) DatedTermCarryDecision {
	decision := a.baseDecision(now, contract, "")
	if !a.cfg.Enabled {
		decision.Action = "POLICY_DISABLED"
		return decision
	}
	if contract == nil {
		decision.Action = "NO_ELIGIBLE_CONTRACT"
		return decision
	}
	if a.pending != nil {
		return a.pendingDecision(decision, now)
	}
	switch contract.state {
	case datedCarryIdle:
		return a.openDecision(decision, now, contract)
	case datedCarryEntrySpot:
		spotTarget := contract.direction * a.cfg.MaxPosition
		spotGap, ok := etypes.TrySub(spotTarget, a.spotPosition)
		if !ok {
			decision.Action = "SPOT_TARGET_UNREPRESENTABLE"
			return decision
		}
		if spotGap != 0 {
			return a.spotOrder(decision, contract, spotGap, "ENTRY_SPOT_IOC")
		}
		contract.state = datedCarryEntryFuture
		decision.State = contract.state
		return a.futureOrder(decision, contract)
	case datedCarryEntryFuture:
		if contract.futurePosition == -a.spotPosition && a.spotPosition != 0 {
			if a.spotPosition == contract.direction*a.cfg.MaxPosition {
				contract.state = datedCarryActive
				decision.State, decision.Action = contract.state, "TERM_ACTIVE"
				return decision
			}
			contract.state = datedCarryEntrySpot
			decision.State = contract.state
			spotGap, ok := etypes.TrySub(contract.direction*a.cfg.MaxPosition, a.spotPosition)
			if !ok {
				decision.Action = "SPOT_TARGET_UNREPRESENTABLE"
				return decision
			}
			return a.spotOrder(decision, contract, spotGap, "ENTRY_SPOT_IOC")
		}
		return a.futureOrder(decision, contract)
	case datedCarryActive:
		decision.Action = "TERM_ACTIVE"
		return decision
	case datedCarryExitSpot:
		return a.exitSpotDecision(decision, contract, now)
	case datedCarryClosed:
		if !contract.closedEmitted {
			contract.closedEmitted = true
			decision.Action = "TERM_CLOSED"
			a.ownedSymbol = ""
			return decision
		}
		decision.Action = "TERM_ALREADY_CLOSED"
		return decision
	default:
		decision.Action = "UNKNOWN_LIFECYCLE_STATE"
		return decision
	}
}

func (a *DatedTermCarryAllocator) openDecision(decision DatedTermCarryDecision, now int64, contract *datedCarryContract) DatedTermCarryDecision {
	tte, ok := etypes.TrySub(contract.expiryAt, now)
	if !ok || tte <= 0 {
		decision.Action = "CONTRACT_EXPIRED"
		return decision
	}
	decision.TimeToExpiryNanos = tte
	if tte < int64(a.cfg.MinTimeToExpiry) {
		decision.Action = "TIME_TO_EXPIRY_BELOW_MINIMUM"
		return decision
	}
	spotAge, futureAge, reason := datedCarryBookAges(now, a.spot, contract.book)
	decision.SpotAgeNanos, decision.FutureAgeNanos = spotAge, futureAge
	if reason != "" {
		decision.Action = reason
		return decision
	}
	if spotAge > int64(a.cfg.MaxMarketAge) || futureAge > int64(a.cfg.MaxMarketAge) {
		decision.Action = "BOOK_STALE"
		return decision
	}
	financials, ok := datedCarryBestFinancials(a.cfg, a.spot, contract.book, tte)
	if !ok {
		decision.Action = "EXECUTABLE_TOUCH_UNAVAILABLE"
		return decision
	}
	a.applyFinancials(&decision, financials)
	if financials.net.Cmp(financials.minimum) < 0 {
		decision.Action = "NET_CARRY_BELOW_MINIMUM"
		return decision
	}
	direction := int64(1)
	if financials.direction == "CHEAP_FUTURE" {
		direction = -1
	}
	decision.ProposedTargetSpot, decision.ProposedTargetFuture = direction*a.cfg.MaxPosition, -direction*a.cfg.MaxPosition
	if !a.cfg.TradeEnabled {
		decision.Action = "SHADOW_ELIGIBLE"
		return decision
	}
	contract.direction, contract.targetChangedAt, contract.state = direction, now, datedCarryEntrySpot
	a.ownedSymbol = contract.symbol
	decision.State, decision.TargetChangedAt = contract.state, now
	decision.TargetSpot, decision.TargetFuture = decision.ProposedTargetSpot, decision.ProposedTargetFuture
	return a.spotOrder(decision, contract, direction*a.cfg.MaxPosition, "ENTRY_SPOT_IOC")
}

func (a *DatedTermCarryAllocator) spotOrder(decision DatedTermCarryDecision, contract *datedCarryContract, gap int64, leg string) DatedTermCarryDecision {
	return a.orderFromGap(decision, contract, a.spot, gap, a.cfg.SpotTick, leg, true)
}

func (a *DatedTermCarryAllocator) futureOrder(decision DatedTermCarryDecision, contract *datedCarryContract) DatedTermCarryDecision {
	target, ok := etypes.TrySub(0, a.spotPosition)
	if !ok {
		decision.Action = "FUTURE_TARGET_UNREPRESENTABLE"
		return decision
	}
	gap, ok := etypes.TrySub(target, contract.futurePosition)
	if !ok {
		decision.Action = "FUTURE_GAP_UNREPRESENTABLE"
		return decision
	}
	if gap == 0 {
		contract.state = datedCarryActive
		decision.State, decision.Action = contract.state, "TERM_ACTIVE"
		return decision
	}
	return a.orderFromGap(decision, contract, contract.book, gap, a.cfg.FutureTick, "ENTRY_FUTURE_IOC", false)
}

func (a *DatedTermCarryAllocator) orderFromGap(decision DatedTermCarryDecision, contract *datedCarryContract, book fundingCarryBook, gap, tick int64, leg string, positiveDomain bool) DatedTermCarryDecision {
	quantity, ok := nonnegativeMagnitude(gap)
	if !ok {
		decision.Action = "ORDER_QUANTITY_UNREPRESENTABLE"
		return decision
	}
	quantity = minInt64(quantity, a.cfg.LotQty)
	if quantity < a.cfg.MinOrderSize {
		decision.Action = "ORDER_SIZE_BELOW_MINIMUM"
		return decision
	}
	side := exchange.Buy
	touch, available, hasTouch := book.ask, book.askQty, book.hasAsk
	if gap < 0 {
		side, touch, available, hasTouch = exchange.Sell, book.bid, book.bidQty, book.hasBid
	}
	if !hasTouch {
		decision.Action = "EXECUTABLE_TOUCH_UNAVAILABLE"
		return decision
	}
	if (positiveDomain && touch <= 0) || touch%tick != 0 {
		decision.Action = "EXECUTABLE_TOUCH_OUTSIDE_DOMAIN"
		return decision
	}
	quantity, ok = venueSizedQty(quantity, available, a.cfg.MinOrderSize)
	if !ok {
		decision.Action = "EXECUTABLE_SIZE_UNAVAILABLE"
		return decision
	}
	postOnly := false
	decision.Action, decision.Leg, decision.Side = "SUBMIT_"+leg, leg, side.String()
	decision.OrderType, decision.TimeInForce, decision.PostOnly = exchange.LimitOrder.String(), exchange.IOC.String(), &postOnly
	decision.LimitPrice, decision.RequestedQty, decision.RequestID = touch, quantity, a.PeekNextRequestID()
	return decision
}

func (a *DatedTermCarryAllocator) exitSpotDecision(decision DatedTermCarryDecision, contract *datedCarryContract, now int64) DatedTermCarryDecision {
	if a.spotPosition == 0 {
		contract.state = datedCarryClosed
		decision.State, decision.Action = contract.state, "TERM_CLOSED"
		contract.closedEmitted, a.ownedSymbol = true, ""
		return decision
	}
	if now >= contract.exitDeadlineAt {
		decision.Action = "EXIT_DEADLINE_EXPIRED"
		return decision
	}
	gap, ok := etypes.TrySub(0, a.spotPosition)
	if !ok {
		decision.Action = "EXIT_GAP_UNREPRESENTABLE"
		return decision
	}
	decision = a.orderFromGap(decision, contract, a.spot, gap, a.cfg.SpotTick, "EXIT_SPOT_IOC", true)
	if decision.Action != "EXECUTABLE_SIZE_UNAVAILABLE" {
		return decision
	}
	quantity, ok := nonnegativeMagnitude(gap)
	if !ok {
		decision.Action = "ORDER_QUANTITY_UNREPRESENTABLE"
		return decision
	}
	quantity = minInt64(quantity, a.cfg.PassiveExitSliceQty)
	if quantity < a.cfg.MinOrderSize {
		decision.Action = "PASSIVE_EXIT_SIZE_BELOW_MINIMUM"
		return decision
	}
	side := exchange.Buy
	touch, hasTouch := a.spot.bid, a.spot.hasBid
	if gap < 0 {
		side, touch, hasTouch = exchange.Sell, a.spot.ask, a.spot.hasAsk
	}
	if !hasTouch || touch <= 0 || touch%a.cfg.SpotTick != 0 {
		decision.Action = "PASSIVE_EXIT_REFERENCE_UNAVAILABLE"
		return decision
	}
	postOnly := true
	decision.Action, decision.Leg, decision.Side = "SUBMIT_EXIT_SPOT_POST_ONLY", "EXIT_SPOT_POST_ONLY", side.String()
	decision.OrderType, decision.TimeInForce, decision.PostOnly = exchange.LimitOrder.String(), exchange.GTC.String(), &postOnly
	decision.LimitPrice, decision.RequestedQty, decision.RequestID = touch, quantity, a.PeekNextRequestID()
	return decision
}

func (a *DatedTermCarryAllocator) pendingDecision(decision DatedTermCarryDecision, now int64) DatedTermCarryDecision {
	if a.pending == nil || !a.pending.postOnly {
		decision.Action = "REQUEST_PENDING"
		return decision
	}
	contract := a.contracts[a.pending.contract]
	if contract == nil || now < contract.exitDeadlineAt {
		decision.Action = "PASSIVE_EXIT_RESTING"
		return decision
	}
	if a.pending.orderID == 0 {
		decision.Action = "PASSIVE_EXIT_AWAITING_ACCEPTANCE"
		return decision
	}
	if a.pending.cancelRequestID != 0 {
		decision.Action, decision.CancelOrderID, decision.CancelRequestID = "PASSIVE_EXIT_CANCEL_PENDING", a.pending.orderID, a.pending.cancelRequestID
		return decision
	}
	decision.Action, decision.CancelOrderID, decision.CancelRequestID = "CANCEL_PASSIVE_EXIT_AT_DEADLINE", a.pending.orderID, a.PeekNextRequestID()
	return decision
}

func (a *DatedTermCarryAllocator) baseDecision(now int64, contract *datedCarryContract, action string) DatedTermCarryDecision {
	frontier := fundingCarryFrontier(a.Gateway())
	decision := DatedTermCarryDecision{
		VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, PolicyVersion: datedTermCarryPolicyVersion,
		DecisionTime: now, Action: action, Enabled: a.cfg.Enabled, TradeEnabled: a.cfg.TradeEnabled, Subscribed: a.subscribed,
		SpotSymbol: a.cfg.SpotSymbol, SpotPosition: a.spotPosition,
		HasSpotBook: a.spot.hasSnapshot, SpotPublishedAt: a.spot.publishedAt, SpotSequence: a.spot.sequence,
		HasSpotBid: a.spot.hasBid, SpotBid: a.spot.bid, SpotBidQty: a.spot.bidQty, HasSpotAsk: a.spot.hasAsk, SpotAsk: a.spot.ask, SpotAskQty: a.spot.askQty,
		DecisionFrontierLinkID: frontier.LinkID, DecisionFrontierOrdinal: frontier.Ordinal,
		DecisionFrontierDeliveredAt: frontier.DeliveredAt, DecisionFrontierDigest: fmt.Sprintf("%x", frontier.Digest),
	}
	if contract == nil {
		return decision
	}
	decision.FutureSymbol, decision.ListedNano, decision.ExpiryNano = contract.symbol, contract.listedAt, contract.expiryAt
	decision.OriginalTenorNanos = contract.expiryAt - contract.listedAt
	decision.ListingPublishedAt, decision.ListingSequence = contract.listingPublishedAt, contract.listingSequence
	decision.ListingFingerprint = contract.listingFingerprint
	decision.State, decision.FuturePosition = contract.state, contract.futurePosition
	decision.TargetChangedAt, decision.SettlementObservedAt, decision.ExitDeadlineAt = contract.targetChangedAt, contract.settlementObservedAt, contract.exitDeadlineAt
	if contract.direction != 0 {
		decision.TargetSpot, decision.TargetFuture = contract.direction*a.cfg.MaxPosition, -contract.direction*a.cfg.MaxPosition
	}
	book := contract.book
	decision.HasFutureBook, decision.FuturePublishedAt, decision.FutureSequence = book.hasSnapshot, book.publishedAt, book.sequence
	decision.HasFutureBid, decision.FutureBid, decision.FutureBidQty = book.hasBid, book.bid, book.bidQty
	decision.HasFutureAsk, decision.FutureAsk, decision.FutureAskQty = book.hasAsk, book.ask, book.askQty
	if tte, ok := etypes.TrySub(contract.expiryAt, now); ok {
		decision.TimeToExpiryNanos = tte
	}
	return decision
}

func (a *DatedTermCarryAllocator) applyFinancials(decision *DatedTermCarryDecision, f datedCarryFinancials) {
	decision.Direction, decision.FinancingDirection = f.direction, f.financingDirection
	decision.SpotExecutionReference, decision.FutureExecutionReference = f.spotReference, f.futureReference
	decision.GrossLockedSpreadRaw, decision.GrossLockedBpsNumerator = f.grossSpread.String(), f.gross.String()
	decision.ExecutionFeeBpsNumerator, decision.FinancingBpsNumerator = f.fees.String(), f.financing.String()
	decision.BalanceSheetBpsNumerator, decision.MarginRiskBpsNumerator = f.balance.String(), f.margin.String()
	decision.LegRiskBpsNumerator, decision.SettlementMismatchNumerator = f.leg.String(), f.settlement.String()
	decision.PostSettlementExitNumerator = f.exit.String()
	decision.NetCarryBpsNumerator, decision.MinimumNetBpsNumerator, decision.RationalDenominator = f.net.String(), f.minimum.String(), f.denominator.String()
}

func datedCarryBookAges(now int64, spot, future fundingCarryBook) (int64, int64, string) {
	if !spot.hasSnapshot || !future.hasSnapshot {
		return 0, 0, "BOOK_UNAVAILABLE"
	}
	spotAge, ok := etypes.TrySub(now, spot.publishedAt)
	if !ok || spotAge < 0 {
		return 0, 0, "SPOT_BOOK_PUBLICATION_FUTURE"
	}
	futureAge, ok := etypes.TrySub(now, future.publishedAt)
	if !ok || futureAge < 0 {
		return spotAge, 0, "FUTURE_BOOK_PUBLICATION_FUTURE"
	}
	return spotAge, futureAge, ""
}

func datedCarryBestFinancials(cfg DatedTermCarryAllocatorConfig, spot, future fundingCarryBook, tte int64) (datedCarryFinancials, bool) {
	var candidates []datedCarryFinancials
	if spot.hasAsk && future.hasBid && spot.ask > 0 {
		candidates = append(candidates, datedCarryFinancialsFor(cfg, "RICH_FUTURE", spot.ask, future.bid, tte))
	}
	if spot.hasBid && future.hasAsk && spot.bid > 0 {
		candidates = append(candidates, datedCarryFinancialsFor(cfg, "CHEAP_FUTURE", spot.bid, future.ask, tte))
	}
	if len(candidates) == 0 {
		return datedCarryFinancials{}, false
	}
	slices.SortFunc(candidates, func(a, b datedCarryFinancials) int {
		if byNet := b.net.Cmp(a.net); byNet != 0 {
			return byNet
		}
		return cmp.Compare(a.direction, b.direction)
	})
	return candidates[0], true
}

func datedCarryFinancialsFor(cfg DatedTermCarryAllocatorConfig, direction string, spotReference, futureReference, tte int64) datedCarryFinancials {
	spot := big.NewInt(spotReference)
	denominator := new(big.Int).Mul(new(big.Int).Set(spot), big.NewInt(datedTermCarryYearNanos))
	grossSpread := new(big.Int).Sub(big.NewInt(futureReference), spot)
	financingRate, financingDirection := cfg.LongSpotFundingBps, "LONG_SPOT_CASH_FINANCING"
	if direction == "CHEAP_FUTURE" {
		grossSpread.Neg(grossSpread)
		financingRate, financingDirection = cfg.ShortSpotBorrowBps, "SHORT_SPOT_ASSET_BORROW"
	}
	gross := new(big.Int).Mul(new(big.Int).Set(grossSpread), big.NewInt(10_000))
	gross.Mul(gross, big.NewInt(datedTermCarryYearNanos))
	component := func(bps int64) *big.Int { return new(big.Int).Mul(big.NewInt(bps), denominator) }
	fees := component(4 * cfg.TakerFeeBps)
	financing := new(big.Int).Mul(big.NewInt(financingRate), big.NewInt(tte))
	financing.Mul(financing, spot)
	balance, margin, leg := component(cfg.BalanceSheetBps), component(cfg.MarginRiskBps), component(cfg.LegRiskBps)
	settlement, exit := component(cfg.SettlementMismatchBps), component(cfg.PostSettlementExitBps)
	net := new(big.Int).Set(gross)
	for _, cost := range []*big.Int{fees, financing, balance, margin, leg, settlement, exit} {
		net.Sub(net, cost)
	}
	minimum := component(cfg.MinNetCarryBps)
	return datedCarryFinancials{
		direction: direction, financingDirection: financingDirection, spotReference: spotReference, futureReference: futureReference,
		grossSpread: grossSpread, gross: gross, fees: fees, financing: financing, balance: balance, margin: margin, leg: leg,
		settlement: settlement, exit: exit, net: net, minimum: minimum, denominator: denominator,
	}
}

func (a *DatedTermCarryAllocator) onAccepted(event actor.OrderAcceptedEvent) {
	if a.pending == nil || event.RequestID != a.pending.requestID {
		return
	}
	a.pending.orderID = event.OrderID
	a.emitPendingOutcome("ORDER_ACCEPTED", 0, event.OrderID, 0, 0, 0, 0, "", "", 0)
}

func (a *DatedTermCarryAllocator) onRejected(event actor.OrderRejectedEvent) {
	if a.pending == nil || event.RequestID != a.pending.requestID {
		return
	}
	pending := a.pending
	a.emitPendingOutcome("ORDER_REJECTED", 0, 0, 0, 0, 0, 0, "", string(event.Reason), 0)
	a.pending = nil
	contract := a.contracts[pending.contract]
	if contract != nil && pending.state == datedCarryEntrySpot && a.spotPosition == 0 && contract.futurePosition == 0 {
		contract.state, contract.direction, contract.targetChangedAt = datedCarryIdle, 0, 0
		a.ownedSymbol = ""
	}
}

func (a *DatedTermCarryAllocator) onFill(event actor.OrderFillEvent) {
	if a.pending == nil || event.OrderID != a.pending.orderID || event.Symbol != a.pending.symbol {
		return
	}
	contract := a.contracts[a.pending.contract]
	if contract == nil {
		return
	}
	beforeSpot, beforeFuture := a.spotPosition, contract.futurePosition
	signed := event.Qty
	if event.Side == exchange.Sell {
		signed = -signed
	}
	var ok bool
	if event.Symbol == a.cfg.SpotSymbol {
		a.spotPosition, ok = etypes.TryAdd(a.spotPosition, signed)
		if contract.settlementObservedAt != 0 && a.spotPosition != 0 {
			contract.state = datedCarryExitSpot
		}
	} else {
		contract.futurePosition, ok = etypes.TryAdd(contract.futurePosition, signed)
	}
	if !ok {
		panic("dated carry position overflow")
	}
	a.emitOutcome(DatedTermCarryOutcome{
		VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, DecisionTime: a.pending.decisionTime, ExecutionTime: event.Timestamp,
		State: a.pending.state, Event: "ORDER_FILL", Leg: a.pending.leg, Symbol: event.Symbol, Side: event.Side.String(),
		RequestID: a.pending.requestID, OrderID: event.OrderID, TradeID: event.TradeID, Qty: event.Qty, Price: event.Price, FeeAmount: event.FeeAmount, FeeAsset: event.FeeAsset,
		SpotPositionBefore: beforeSpot, SpotPositionAfter: a.spotPosition, FuturePositionBefore: beforeFuture, FuturePositionAfter: contract.futurePosition,
	})
	if event.IsFull {
		a.pending = nil
	}
}

func (a *DatedTermCarryAllocator) onCancelled(event actor.OrderCancelledEvent) {
	if a.pending == nil || event.OrderID != a.pending.orderID {
		return
	}
	a.emitPendingOutcome("ORDER_CANCELLED", 0, event.OrderID, 0, 0, 0, 0, "", "", event.RemainingQty)
	a.pending = nil
}

func (a *DatedTermCarryAllocator) onCancelRejected(event actor.OrderCancelRejectedEvent) {
	if a.pending == nil || event.OrderID != a.pending.orderID || event.RequestID != a.pending.cancelRequestID {
		return
	}
	a.emitPendingOutcome("ORDER_CANCEL_REJECTED", 0, event.OrderID, 0, 0, 0, 0, "", string(event.Reason), 0)
	a.pending.cancelRequestID = 0
}

func (a *DatedTermCarryAllocator) emitPendingOutcome(event string, executionTime int64, orderID, tradeID uint64, quantity, price, feeAmount int64, feeAsset, reject string, remaining int64) {
	if a.pending == nil {
		return
	}
	contract := a.contracts[a.pending.contract]
	futurePosition := int64(0)
	if contract != nil {
		futurePosition = contract.futurePosition
	}
	a.emitOutcome(DatedTermCarryOutcome{
		VenueID: a.cfg.VenueID, Desk: a.cfg.Desk, ClientID: a.cfg.ClientID, DecisionTime: a.pending.decisionTime, ExecutionTime: executionTime,
		State: a.pending.state, Event: event, Leg: a.pending.leg, Symbol: a.pending.symbol, RequestID: a.pending.requestID,
		OrderID: orderID, TradeID: tradeID, Qty: quantity, Price: price, FeeAmount: feeAmount, FeeAsset: feeAsset, RemainingQty: remaining, RejectReason: reject,
		CancelRequestID: a.pending.cancelRequestID, SpotPositionBefore: a.spotPosition, SpotPositionAfter: a.spotPosition,
		FuturePositionBefore: futurePosition, FuturePositionAfter: futurePosition,
	})
}

func (a *DatedTermCarryAllocator) emitDecision(decision DatedTermCarryDecision) {
	if a.cfg.DecisionObserver != nil {
		a.cfg.DecisionObserver(decision)
	}
}

func (a *DatedTermCarryAllocator) emitOutcome(outcome DatedTermCarryOutcome) {
	if a.cfg.OutcomeObserver != nil {
		a.cfg.OutcomeObserver(outcome)
	}
}

func datedCarryLegSymbol(spot, future, leg string) string {
	if leg == "ENTRY_FUTURE_IOC" {
		return future
	}
	return spot
}
