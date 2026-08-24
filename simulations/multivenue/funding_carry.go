package multivenue

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// v2 records one exact decision frontier and locates every cached source
// observation within that delivered prefix. Version one incorrectly claimed
// each cached observation was itself the current shared-link frontier.
const fundingCarryPolicyVersion = "v2_5_p0_funding_carry_v2"

const fundingCarryYearNanos int64 = 365 * 24 * int64(time.Hour)

// FundingCarryArbitrageurConfig declares the complete, local economic policy
// for V2-5 P0. It is deliberately distinct from CarryArbitrageurConfig: the
// latter is the retained fixed-basis desk and has no funding input.
type FundingCarryArbitrageurConfig struct {
	Enabled        bool          `json:"enabled"`
	SpotSymbol     string        `json:"spot_symbol"`
	PerpSymbol     string        `json:"perp_symbol"`
	DecisionPeriod time.Duration `json:"decision_period"`

	// FundingHorizon is the number of next delivered funding intervals priced
	// into a decision. P0 uses one; a rate is never extrapolated without an
	// explicit configured interval count.
	FundingHorizon int64         `json:"funding_horizon"`
	MaxFundingAge  time.Duration `json:"max_funding_age"`

	// Every cost is a separately declared non-negative bps estimate. None is a
	// hidden exchange lookup or a price correction.
	TakerFeeBps      int64                        `json:"taker_fee_bps"`
	BorrowAnnualBps  int64                        `json:"borrow_annual_bps"`
	BalanceSheetBps  int64                        `json:"balance_sheet_bps"`
	MarginRiskBps    int64                        `json:"margin_risk_bps"`
	LegRiskBps       int64                        `json:"leg_risk_bps"`
	MinNetCarryBps   int64                        `json:"min_net_carry_bps"`
	MaxPosition      int64                        `json:"max_position"`
	LotQty           int64                        `json:"lot_qty"`
	MinOrderSize     int64                        `json:"min_order_size"`
	SpotTick         int64                        `json:"spot_tick"`
	PerpTick         int64                        `json:"perp_tick"`
	VenueID          string                       `json:"-"`
	Desk             string                       `json:"-"`
	ClientID         uint64                       `json:"-"`
	TerminalNano     int64                        `json:"-"`
	DecisionObserver func(FundingCarryDecision)   `json:"-"`
	OutcomeObserver  func(FundingCarryLegOutcome) `json:"-"`
}

func (c FundingCarryArbitrageurConfig) validate() error {
	if c.SpotSymbol == "" || c.PerpSymbol == "" {
		return fmt.Errorf("spot and perpetual symbols are required")
	}
	if c.DecisionPeriod <= 0 || c.MaxFundingAge <= 0 {
		return fmt.Errorf("decision period and maximum funding age must be positive")
	}
	if c.FundingHorizon <= 0 {
		return fmt.Errorf("funding horizon must be positive")
	}
	if c.MaxPosition <= 0 || c.LotQty <= 0 || c.MinOrderSize < 0 || c.SpotTick <= 0 || c.PerpTick <= 0 {
		return fmt.Errorf("position, lot, and tick policy inputs are invalid")
	}
	for _, component := range []struct {
		name  string
		value int64
	}{
		{"taker fee", c.TakerFeeBps}, {"borrow", c.BorrowAnnualBps},
		{"balance sheet", c.BalanceSheetBps}, {"margin risk", c.MarginRiskBps},
		{"leg risk", c.LegRiskBps}, {"minimum net carry", c.MinNetCarryBps},
	} {
		if component.value < 0 {
			return fmt.Errorf("%s bps must be non-negative", component.name)
		}
	}
	return nil
}

// FundingCarryDecision records every local policy evaluation. Presence fields
// distinguish unavailable observations from a valid numeric zero. P0's crypto
// contract separately rejects present non-positive executable prices.
type FundingCarryDecision struct {
	VenueID       string `json:"venue_id"`
	Desk          string `json:"desk"`
	ClientID      uint64 `json:"client_id"`
	PolicyVersion string `json:"policy_version"`
	DecisionTime  int64  `json:"decision_time"`
	Enabled       bool   `json:"enabled"`
	Subscribed    bool   `json:"subscribed"`
	Pending       bool   `json:"pending"`
	ActionOrDefer string `json:"action_or_defer_reason"`

	SpotSymbol          string `json:"spot_symbol"`
	PerpSymbol          string `json:"perp_symbol"`
	SpotPosition        int64  `json:"spot_position"`
	PerpPosition        int64  `json:"perp_position"`
	DesiredSpotPosition int64  `json:"desired_spot_position"`
	DesiredPerpPosition int64  `json:"desired_perp_position"`

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

	HasFunding             bool   `json:"has_funding"`
	FundingRateBps         int64  `json:"funding_rate_bps"`
	FundingPublishedAt     int64  `json:"funding_published_at"`
	FundingSequence        uint64 `json:"funding_sequence"`
	FundingNextAt          int64  `json:"funding_next_at"`
	FundingIntervalSeconds int64  `json:"funding_interval_seconds"`
	FundingMarkAvailable   bool   `json:"funding_mark_available"`
	FundingMarkPrice       int64  `json:"funding_mark_price"`
	FundingIndexAvailable  bool   `json:"funding_index_available"`
	FundingIndexPrice      int64  `json:"funding_index_price"`
	FundingAgeNanos        int64  `json:"funding_age_nanos"`
	FundingHorizon         int64  `json:"funding_horizon"`
	HoldingNanos           int64  `json:"holding_nanos"`

	// DecisionFrontier is the full actor-local public-feed prefix immediately
	// before this evaluation. The cached book/funding identities above are
	// located independently within this prefix; they are not falsely labelled
	// as the last message delivered on a busy shared link.
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`

	SpotMid             int64 `json:"spot_mid"`
	PerpMid             int64 `json:"perp_mid"`
	PremiumBps          int64 `json:"premium_bps"`
	FundingIncomeBps    int64 `json:"funding_income_bps"`
	TakerFeeCostBps     int64 `json:"taker_fee_cost_bps"`
	BorrowCostBps       int64 `json:"borrow_cost_bps"`
	BalanceSheetCostBps int64 `json:"balance_sheet_cost_bps"`
	MarginRiskCostBps   int64 `json:"margin_risk_cost_bps"`
	LegRiskCostBps      int64 `json:"leg_risk_cost_bps"`
	NetCarryBps         int64 `json:"net_carry_bps"`
	MinNetCarryBps      int64 `json:"min_net_carry_bps"`

	Leg          string `json:"leg"`
	Side         string `json:"side"`
	LimitPrice   int64  `json:"limit_price"`
	RequestedQty int64  `json:"requested_qty"`
	RequestID    uint64 `json:"request_id"`
}

// FundingCarryLegOutcome links an accepted, rejected, filled, or cancelled
// non-atomic leg to the exact request recorded in FundingCarryDecision.
type FundingCarryLegOutcome struct {
	VenueID  string `json:"venue_id"`
	Desk     string `json:"desk"`
	ClientID uint64 `json:"client_id"`
	// DecisionTime is the modeled pre-ingress decision time. ExecutionTime is
	// present only for a fill, whose exchange event carries an exact timestamp.
	// Response and cancellation events have no exchange timestamp in the actor
	// contract, so telemetry never substitutes host wall time.
	DecisionTime       int64  `json:"decision_time"`
	ExecutionTime      int64  `json:"execution_time"`
	Event              string `json:"event"`
	Leg                string `json:"leg"`
	RequestID          uint64 `json:"request_id"`
	OrderID            uint64 `json:"order_id"`
	TradeID            uint64 `json:"trade_id"`
	Symbol             string `json:"symbol"`
	Side               string `json:"side"`
	Qty                int64  `json:"qty"`
	Price              int64  `json:"price"`
	FeeAmount          int64  `json:"fee_amount"`
	FeeAsset           string `json:"fee_asset"`
	RemainingQty       int64  `json:"remaining_qty"`
	RejectReason       string `json:"reject_reason"`
	SpotPositionBefore int64  `json:"spot_position_before"`
	SpotPositionAfter  int64  `json:"spot_position_after"`
	PerpPositionBefore int64  `json:"perp_position_before"`
	PerpPositionAfter  int64  `json:"perp_position_after"`
}

type fundingCarryBook struct {
	hasSnapshot bool
	publishedAt int64
	sequence    uint64
	hasBid      bool
	bid, bidQty int64
	hasAsk      bool
	ask, askQty int64
}

type fundingCarryFunding struct {
	has         bool
	rate        exchange.FundingRate
	publishedAt int64
	sequence    uint64
}

type fundingCarryPending struct {
	requestID    uint64
	orderID      uint64
	leg          string
	symbol       string
	decisionTime int64
}

// FundingCarryArbitrageur is a locally informed, funding-sensitive two-leg
// participant. It submits ordinary IOC legs sequentially; therefore a partial
// fill remains an explicit orphan until a later local decision repairs it.
type FundingCarryArbitrageur struct {
	*actor.BaseActor
	cfg          FundingCarryArbitrageurConfig
	spot         fundingCarryBook
	perp         fundingCarryBook
	funding      fundingCarryFunding
	spotPosition int64
	perpPosition int64
	pending      *fundingCarryPending
	subscribed   bool
}

// NewFundingCarryArbitrageur constructs the opt-in V2-5 P0 desk.
func NewFundingCarryArbitrageur(id uint64, gateway actor.Gateway, cfg FundingCarryArbitrageurConfig) *FundingCarryArbitrageur {
	desk := &FundingCarryArbitrageur{BaseActor: actor.NewBaseActor(id, gateway), cfg: cfg}
	desk.SetHandler(desk)
	desk.AddTicker(cfg.DecisionPeriod, desk.onTick)
	return desk
}

func (d *FundingCarryArbitrageur) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		d.observeBook(event.Data.(actor.BookSnapshotEvent))
	case actor.EventFundingUpdate:
		d.observeFunding(event.Data.(actor.FundingUpdateEvent))
	case actor.EventOrderAccepted:
		d.onAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		d.onRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		d.onFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		d.onCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (d *FundingCarryArbitrageur) observeBook(event actor.BookSnapshotEvent) {
	var target *fundingCarryBook
	switch event.Symbol {
	case d.cfg.SpotSymbol:
		target = &d.spot
	case d.cfg.PerpSymbol:
		target = &d.perp
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

func (d *FundingCarryArbitrageur) observeFunding(event actor.FundingUpdateEvent) {
	if event.Symbol != d.cfg.PerpSymbol || event.FundingRate == nil {
		return
	}
	// Publisher sequence is the message identity. A duplicate or reordered
	// snapshot cannot move this actor's funding frontier backwards or replace
	// the current locally delivered observation.
	if d.funding.has && event.SeqNum != 0 && event.SeqNum <= d.funding.sequence {
		return
	}
	d.funding = fundingCarryFunding{
		has: true, rate: *event.FundingRate, publishedAt: event.Timestamp, sequence: event.SeqNum,
	}
}

func fundingCarryFrontier(gateway actor.Gateway) simulation.MarketDataFrontier {
	if source, ok := gateway.(interface {
		MarketDataFrontier() simulation.MarketDataFrontier
	}); ok {
		return source.MarketDataFrontier()
	}
	return simulation.MarketDataFrontier{}
}

func (d *FundingCarryArbitrageur) onTick(now time.Time) {
	if !d.subscribed {
		decision := d.baseDecision(now, "NOT_SUBSCRIBED")
		d.Subscribe(d.cfg.SpotSymbol, exchange.MDSnapshot)
		d.Subscribe(d.cfg.PerpSymbol, exchange.MDSnapshot, exchange.MDFunding)
		d.subscribed = true
		d.emitDecision(decision)
		return
	}
	decision := d.decision(now)
	d.emitDecision(decision)
	if decision.RequestID == 0 {
		return
	}
	d.pending = &fundingCarryPending{requestID: decision.RequestID, leg: decision.Leg, symbol: mapFundingCarryLegSymbol(d.cfg, decision.Leg), decisionTime: decision.DecisionTime}
	requestID := d.SubmitOrderWithTimeInForce(d.pending.symbol, fundingCarrySide(decision.Side), exchange.LimitOrder, decision.LimitPrice, decision.RequestedQty, exchange.IOC)
	if requestID != decision.RequestID {
		// This is an actor-invariant violation, not a recoverable market state.
		// The pre-ingress evidence must name the exact request that is sent.
		panic(fmt.Sprintf("funding carry request ID changed from %d to %d", decision.RequestID, requestID))
	}
}

func mapFundingCarryLegSymbol(cfg FundingCarryArbitrageurConfig, leg string) string {
	if leg == "PERP_ORPHAN_REPAIR" {
		return cfg.PerpSymbol
	}
	return cfg.SpotSymbol
}

func fundingCarrySide(side string) exchange.Side {
	if side == exchange.Sell.String() {
		return exchange.Sell
	}
	return exchange.Buy
}

func (d *FundingCarryArbitrageur) decision(now time.Time) FundingCarryDecision {
	decision := d.baseDecision(now, "")
	if !d.cfg.Enabled {
		decision.ActionOrDefer = "POLICY_DISABLED"
		return decision
	}
	if d.pending != nil {
		decision.ActionOrDefer = "REQUEST_PENDING"
		return decision
	}
	if d.terminalRoundTripCensored(now.UnixNano()) {
		decision.ActionOrDefer = "SIMULATION_HORIZON_CENSORED"
		return decision
	}
	if mismatch, ok := fundingCarryAdd(d.spotPosition, d.perpPosition); !ok {
		decision.ActionOrDefer = "POSITION_MISMATCH_OVERFLOW"
		return decision
	} else if mismatch != 0 {
		return d.orphanRepair(decision, mismatch)
	}
	if !d.funding.has {
		decision.ActionOrDefer = "FUNDING_UNAVAILABLE"
		return decision
	}
	// SeqNum is actor-visible public message identity. The frontier itself is
	// telemetry metadata and must never become a policy input: toggling receipt
	// recording may not change this desk's trajectory.
	if d.funding.sequence == 0 {
		decision.ActionOrDefer = "FUNDING_IDENTITY_UNAVAILABLE"
		return decision
	}
	if d.funding.publishedAt > now.UnixNano() {
		decision.ActionOrDefer = "FUNDING_PUBLICATION_FUTURE"
		return decision
	}
	age, ok := fundingCarrySub(now.UnixNano(), d.funding.publishedAt)
	if !ok {
		decision.ActionOrDefer = "FUNDING_AGE_UNREPRESENTABLE"
		return decision
	}
	decision.FundingAgeNanos = age
	if age > int64(d.cfg.MaxFundingAge) {
		decision.ActionOrDefer = "FUNDING_STALE"
		return decision
	}
	if !d.funding.rate.MarkAvailable || !d.funding.rate.IndexAvailable {
		decision.ActionOrDefer = "FUNDING_REFERENCE_UNAVAILABLE"
		return decision
	}
	spotMid, perpMid, reason := fundingCarryMids(d.spot, d.perp)
	if reason != "" {
		decision.ActionOrDefer = reason
		return decision
	}
	decision.SpotMid, decision.PerpMid = spotMid, perpMid
	premium, ok := fundingCarryBasisBps(perpMid, spotMid)
	if !ok {
		decision.ActionOrDefer = "PREMIUM_UNREPRESENTABLE"
		return decision
	}
	decision.PremiumBps = premium
	if premium == 0 {
		decision.ActionOrDefer = "ZERO_PREMIUM"
		return decision
	}
	direction := int64(1)
	if premium < 0 {
		direction = -1
	}
	financials, ok := fundingCarryComputeFinancials(d.cfg, d.funding.rate, now.UnixNano(), direction)
	if !ok {
		decision.ActionOrDefer = "FUNDING_HORIZON_UNAVAILABLE"
		return decision
	}
	decision.HoldingNanos = financials.holdingNanos
	decision.FundingIncomeBps = financials.fundingIncome
	decision.TakerFeeCostBps = financials.takerFees
	decision.BorrowCostBps = financials.borrow
	decision.BalanceSheetCostBps = d.cfg.BalanceSheetBps
	decision.MarginRiskCostBps = d.cfg.MarginRiskBps
	decision.LegRiskCostBps = d.cfg.LegRiskBps
	decision.NetCarryBps = financials.netCarry
	if financials.netCarry < d.cfg.MinNetCarryBps {
		decision.ActionOrDefer = "NET_CARRY_BELOW_MINIMUM"
		return decision
	}
	if direction > 0 {
		decision.DesiredSpotPosition = d.cfg.MaxPosition
	} else {
		decision.DesiredSpotPosition = -d.cfg.MaxPosition
	}
	decision.DesiredPerpPosition = -decision.DesiredSpotPosition
	gap, ok := fundingCarrySub(decision.DesiredSpotPosition, d.spotPosition)
	if !ok {
		decision.ActionOrDefer = "TARGET_GAP_UNREPRESENTABLE"
		return decision
	}
	if gap == 0 {
		decision.ActionOrDefer = "AT_TARGET"
		return decision
	}
	return d.submitSpotAdjustment(decision, gap)
}

func (d *FundingCarryArbitrageur) orphanRepair(decision FundingCarryDecision, desiredPerp int64) FundingCarryDecision {
	decision.DesiredSpotPosition = d.spotPosition
	decision.DesiredPerpPosition = -d.spotPosition
	quantity, ok := nonnegativeMagnitude(desiredPerp)
	if !ok {
		decision.ActionOrDefer = "ORPHAN_REPAIR_UNREPRESENTABLE"
		return decision
	}
	decision.Leg = "PERP_ORPHAN_REPAIR"
	decision.RequestedQty = minInt64(quantity, d.cfg.LotQty)
	if decision.RequestedQty <= 0 {
		decision.ActionOrDefer = "ORPHAN_REPAIR_ZERO_QUANTITY"
		return decision
	}
	if desiredPerp > 0 {
		if !d.perp.hasBid {
			decision.ActionOrDefer = "PERP_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.Side, decision.LimitPrice = exchange.Sell.String(), d.perp.bid
	} else {
		if !d.perp.hasAsk {
			decision.ActionOrDefer = "PERP_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.Side, decision.LimitPrice = exchange.Buy.String(), d.perp.ask
	}
	if !fundingCarryPositiveGridPrice(decision.LimitPrice, d.cfg.PerpTick) {
		decision.ActionOrDefer = "PERP_PRICE_OUTSIDE_DOMAIN"
		return decision
	}
	if quantity, ok := venueSizedQty(decision.RequestedQty, fundingCarryAvailable(d.perp, decision.Side), d.cfg.MinOrderSize); !ok {
		decision.RequestedQty = 0
		decision.ActionOrDefer = "PERP_EXECUTABLE_SIZE_UNAVAILABLE"
		return decision
	} else {
		decision.RequestedQty = quantity
	}
	decision.RequestID = d.PeekNextRequestID()
	decision.ActionOrDefer = "SUBMIT_PERP_ORPHAN_REPAIR_IOC"
	return decision
}

func (d *FundingCarryArbitrageur) submitSpotAdjustment(decision FundingCarryDecision, gap int64) FundingCarryDecision {
	quantity, ok := nonnegativeMagnitude(gap)
	if !ok {
		decision.ActionOrDefer = "SPOT_TARGET_UNREPRESENTABLE"
		return decision
	}
	decision.Leg = "SPOT_TARGET_ADJUSTMENT"
	decision.RequestedQty = minInt64(quantity, d.cfg.LotQty)
	if decision.RequestedQty <= 0 {
		decision.ActionOrDefer = "SPOT_TARGET_ZERO_QUANTITY"
		return decision
	}
	if gap > 0 {
		if !d.spot.hasAsk {
			decision.ActionOrDefer = "SPOT_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.Side, decision.LimitPrice = exchange.Buy.String(), d.spot.ask
	} else {
		if !d.spot.hasBid {
			decision.ActionOrDefer = "SPOT_EXECUTABLE_PRICE_UNAVAILABLE"
			return decision
		}
		decision.Side, decision.LimitPrice = exchange.Sell.String(), d.spot.bid
	}
	if !fundingCarryPositiveGridPrice(decision.LimitPrice, d.cfg.SpotTick) {
		decision.ActionOrDefer = "SPOT_PRICE_OUTSIDE_DOMAIN"
		return decision
	}
	if quantity, ok := venueSizedQty(decision.RequestedQty, fundingCarryAvailable(d.spot, decision.Side), d.cfg.MinOrderSize); !ok {
		decision.RequestedQty = 0
		decision.ActionOrDefer = "SPOT_EXECUTABLE_SIZE_UNAVAILABLE"
		return decision
	} else {
		decision.RequestedQty = quantity
	}
	decision.RequestID = d.PeekNextRequestID()
	decision.ActionOrDefer = "SUBMIT_SPOT_TARGET_IOC"
	return decision
}

func fundingCarryAvailable(book fundingCarryBook, side string) int64 {
	if side == exchange.Sell.String() {
		return book.bidQty
	}
	return book.askQty
}

func fundingCarryPositiveGridPrice(price, tick int64) bool {
	return price > 0 && tick > 0 && price%tick == 0
}

func fundingCarryMids(spot, perp fundingCarryBook) (int64, int64, string) {
	if !spot.hasSnapshot || !perp.hasSnapshot || !spot.hasBid || !spot.hasAsk || !perp.hasBid || !perp.hasAsk {
		return 0, 0, "LOCAL_REFERENCE_UNAVAILABLE"
	}
	if spot.bid <= 0 || spot.ask <= 0 || perp.bid <= 0 || perp.ask <= 0 || spot.bid > spot.ask || perp.bid > perp.ask {
		return 0, 0, "LOCAL_PRICE_OUTSIDE_DOMAIN"
	}
	return etypes.Midpoint(spot.bid, spot.ask), etypes.Midpoint(perp.bid, perp.ask), ""
}

type fundingCarryFinancials struct {
	holdingNanos  int64
	fundingIncome int64
	takerFees     int64
	borrow        int64
	netCarry      int64
}

func fundingCarryComputeFinancials(cfg FundingCarryArbitrageurConfig, rate exchange.FundingRate, now, direction int64) (fundingCarryFinancials, bool) {
	if rate.NextFunding <= now || rate.Interval <= 0 || direction == 0 {
		return fundingCarryFinancials{}, false
	}
	holding := new(big.Int).Sub(big.NewInt(rate.NextFunding), big.NewInt(now))
	extraIntervals := new(big.Int).SetInt64(cfg.FundingHorizon - 1)
	extraNanos := new(big.Int).Mul(extraIntervals, big.NewInt(rate.Interval))
	extraNanos.Mul(extraNanos, big.NewInt(int64(time.Second)))
	holding.Add(holding, extraNanos)
	if !holding.IsInt64() || holding.Sign() <= 0 {
		return fundingCarryFinancials{}, false
	}
	income := new(big.Int).Mul(big.NewInt(rate.Rate), big.NewInt(cfg.FundingHorizon))
	income.Mul(income, big.NewInt(direction))
	fees := new(big.Int).Mul(big.NewInt(cfg.TakerFeeBps), big.NewInt(4))
	borrow := new(big.Int).Mul(big.NewInt(cfg.BorrowAnnualBps), holding)
	borrow.Quo(borrow, big.NewInt(fundingCarryYearNanos))
	net := new(big.Int).Sub(income, fees)
	net.Sub(net, borrow)
	net.Sub(net, big.NewInt(cfg.BalanceSheetBps))
	net.Sub(net, big.NewInt(cfg.MarginRiskBps))
	net.Sub(net, big.NewInt(cfg.LegRiskBps))
	if !income.IsInt64() || !fees.IsInt64() || !borrow.IsInt64() || !net.IsInt64() {
		return fundingCarryFinancials{}, false
	}
	return fundingCarryFinancials{holdingNanos: holding.Int64(), fundingIncome: income.Int64(), takerFees: fees.Int64(), borrow: borrow.Int64(), netCarry: net.Int64()}, true
}

func fundingCarryBasisBps(perp, spot int64) (int64, bool) {
	if spot <= 0 || perp <= 0 {
		return 0, false
	}
	numerator := new(big.Int).Sub(big.NewInt(perp), big.NewInt(spot))
	numerator.Mul(numerator, big.NewInt(10_000))
	numerator.Quo(numerator, big.NewInt(spot))
	if !numerator.IsInt64() {
		return 0, false
	}
	return numerator.Int64(), true
}

func fundingCarrySub(left, right int64) (int64, bool) {
	value := new(big.Int).Sub(big.NewInt(left), big.NewInt(right))
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func (d *FundingCarryArbitrageur) onAccepted(event actor.OrderAcceptedEvent) {
	if d.pending == nil || event.RequestID != d.pending.requestID {
		return
	}
	d.pending.orderID = event.OrderID
	d.emitOutcome(FundingCarryLegOutcome{VenueID: d.cfg.VenueID, Desk: d.cfg.Desk, ClientID: d.cfg.ClientID,
		DecisionTime: d.pending.decisionTime, Event: "ORDER_ACCEPTED", Leg: d.pending.leg, RequestID: event.RequestID, OrderID: event.OrderID,
		Symbol: d.pending.symbol, SpotPositionBefore: d.spotPosition, SpotPositionAfter: d.spotPosition, PerpPositionBefore: d.perpPosition, PerpPositionAfter: d.perpPosition})
}

func (d *FundingCarryArbitrageur) onRejected(event actor.OrderRejectedEvent) {
	if d.pending == nil || event.RequestID != d.pending.requestID {
		return
	}
	d.emitOutcome(FundingCarryLegOutcome{VenueID: d.cfg.VenueID, Desk: d.cfg.Desk, ClientID: d.cfg.ClientID,
		DecisionTime: d.pending.decisionTime, Event: "ORDER_REJECTED", Leg: d.pending.leg, RequestID: event.RequestID, Symbol: d.pending.symbol, RejectReason: string(event.Reason),
		SpotPositionBefore: d.spotPosition, SpotPositionAfter: d.spotPosition, PerpPositionBefore: d.perpPosition, PerpPositionAfter: d.perpPosition})
	d.pending = nil
}

func (d *FundingCarryArbitrageur) onFill(event actor.OrderFillEvent) {
	if d.pending == nil || event.OrderID != d.pending.orderID || event.Symbol != d.pending.symbol {
		return
	}
	beforeSpot, beforePerp := d.spotPosition, d.perpPosition
	signedQty := event.Qty
	if event.Side == exchange.Sell {
		signedQty = -signedQty
	}
	var ok bool
	if event.Symbol == d.cfg.SpotSymbol {
		d.spotPosition, ok = fundingCarryAdd(d.spotPosition, signedQty)
	} else {
		d.perpPosition, ok = fundingCarryAdd(d.perpPosition, signedQty)
	}
	if !ok {
		panic("funding carry position overflow")
	}
	d.emitOutcome(FundingCarryLegOutcome{VenueID: d.cfg.VenueID, Desk: d.cfg.Desk, ClientID: d.cfg.ClientID,
		DecisionTime: d.pending.decisionTime, ExecutionTime: event.Timestamp, Event: "ORDER_FILL", Leg: d.pending.leg, RequestID: d.pending.requestID, OrderID: event.OrderID, TradeID: event.TradeID,
		Symbol: event.Symbol, Side: event.Side.String(), Qty: event.Qty, Price: event.Price, FeeAmount: event.FeeAmount, FeeAsset: event.FeeAsset,
		SpotPositionBefore: beforeSpot, SpotPositionAfter: d.spotPosition, PerpPositionBefore: beforePerp, PerpPositionAfter: d.perpPosition})
	if event.IsFull {
		d.pending = nil
	}
}

func (d *FundingCarryArbitrageur) onCancelled(event actor.OrderCancelledEvent) {
	if d.pending == nil || event.OrderID != d.pending.orderID {
		return
	}
	d.emitOutcome(FundingCarryLegOutcome{VenueID: d.cfg.VenueID, Desk: d.cfg.Desk, ClientID: d.cfg.ClientID,
		DecisionTime: d.pending.decisionTime, Event: "ORDER_CANCELLED", Leg: d.pending.leg, RequestID: d.pending.requestID, OrderID: event.OrderID, Symbol: d.pending.symbol, RemainingQty: event.RemainingQty,
		SpotPositionBefore: d.spotPosition, SpotPositionAfter: d.spotPosition, PerpPositionBefore: d.perpPosition, PerpPositionAfter: d.perpPosition})
	d.pending = nil
}

func fundingCarryAdd(left, right int64) (int64, bool) {
	value := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func (d *FundingCarryArbitrageur) terminalRoundTripCensored(now int64) bool {
	if d.cfg.TerminalNano == 0 {
		return false
	}
	deadline, ok := fundingCarryAdd(now, int64(d.cfg.DecisionPeriod))
	if !ok {
		return true
	}
	deadline, ok = fundingCarryAdd(deadline, int64(d.cfg.DecisionPeriod))
	return !ok || deadline > d.cfg.TerminalNano
}

func (d *FundingCarryArbitrageur) baseDecision(now time.Time, action string) FundingCarryDecision {
	frontier := fundingCarryFrontier(d.Gateway())
	decision := FundingCarryDecision{
		VenueID: d.cfg.VenueID, Desk: d.cfg.Desk, ClientID: d.cfg.ClientID, PolicyVersion: fundingCarryPolicyVersion,
		DecisionTime: now.UnixNano(), Enabled: d.cfg.Enabled, Subscribed: d.subscribed, Pending: d.pending != nil, ActionOrDefer: action,
		SpotSymbol: d.cfg.SpotSymbol, PerpSymbol: d.cfg.PerpSymbol, SpotPosition: d.spotPosition, PerpPosition: d.perpPosition,
		HasSpotBook: d.spot.hasSnapshot, SpotPublishedAt: d.spot.publishedAt, SpotSequence: d.spot.sequence,
		HasSpotBid: d.spot.hasBid, SpotBid: d.spot.bid, SpotBidQty: d.spot.bidQty, HasSpotAsk: d.spot.hasAsk, SpotAsk: d.spot.ask, SpotAskQty: d.spot.askQty,
		HasPerpBook: d.perp.hasSnapshot, PerpPublishedAt: d.perp.publishedAt, PerpSequence: d.perp.sequence,
		HasPerpBid: d.perp.hasBid, PerpBid: d.perp.bid, PerpBidQty: d.perp.bidQty, HasPerpAsk: d.perp.hasAsk, PerpAsk: d.perp.ask, PerpAskQty: d.perp.askQty,
		HasFunding: d.funding.has, FundingRateBps: d.funding.rate.Rate, FundingPublishedAt: d.funding.publishedAt, FundingSequence: d.funding.sequence,
		FundingNextAt: d.funding.rate.NextFunding, FundingIntervalSeconds: d.funding.rate.Interval,
		FundingMarkAvailable: d.funding.rate.MarkAvailable, FundingMarkPrice: d.funding.rate.MarkPrice,
		FundingIndexAvailable: d.funding.rate.IndexAvailable, FundingIndexPrice: d.funding.rate.IndexPrice,
		FundingHorizon: d.cfg.FundingHorizon, MinNetCarryBps: d.cfg.MinNetCarryBps,
		DecisionFrontierLinkID: frontier.LinkID, DecisionFrontierOrdinal: frontier.Ordinal,
		DecisionFrontierDeliveredAt: frontier.DeliveredAt, DecisionFrontierDigest: fmt.Sprintf("%x", frontier.Digest),
	}
	return decision
}

func (d *FundingCarryArbitrageur) emitDecision(decision FundingCarryDecision) {
	if d.cfg.DecisionObserver != nil {
		d.cfg.DecisionObserver(decision)
	}
}

func (d *FundingCarryArbitrageur) emitOutcome(outcome FundingCarryLegOutcome) {
	if d.cfg.OutcomeObserver != nil {
		d.cfg.OutcomeObserver(outcome)
	}
}
