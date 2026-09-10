package multivenue

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// ElasticLiquiditySupplierSpec is the serializable roster entry for a bounded
// local liquidity participant. It is deliberately separate from
// ElasticSupplierConfig so historical IOC suppliers cannot change when this
// successor class is enabled.
type ElasticLiquiditySupplierSpec struct {
	Role                 string        `json:"role"`
	Symbol               string        `json:"symbol"`
	BaseAsset            string        `json:"base_asset"`
	QuoteAsset           string        `json:"quote_asset"`
	BasePrecision        int64         `json:"base_precision"`
	QuotePrecision       int64         `json:"quote_precision"`
	InitialBaseBalance   int64         `json:"initial_base_balance"`
	InitialQuoteBalance  int64         `json:"initial_quote_balance"`
	Interval             time.Duration `json:"interval"`
	DecisionPhaseOffset  time.Duration `json:"decision_phase_offset,omitempty"`
	MaxObservationAge    time.Duration `json:"max_observation_age"`
	ReferencePrice       int64         `json:"reference_price"`
	ReferenceHalfLife    time.Duration `json:"reference_half_life"`
	BaseHolding          int64         `json:"base_holding"`
	ElasticityPerPercent int64         `json:"elasticity_per_percent"`
	MaxPosition          int64         `json:"max_position"`
	MaxInventory         int64         `json:"max_inventory"`
	MaxQuoteQty          int64         `json:"max_quote_qty"`
	MinimumExecutableQty int64         `json:"minimum_executable_qty,omitempty"`
	// MinimumQualifyingQty is the larger scientific threshold used when a
	// successor claims that a side is materially restored. It is distinct from
	// the exchange admission minimum so a dust-sized legal order cannot prove
	// market survival by itself.
	MinimumQualifyingQty int64 `json:"minimum_qualifying_qty,omitempty"`
	// TickSize is the expected registered CDF/USD price tick. The venue binds
	// the actor to the instrument's actual tick before construction.
	TickSize int64 `json:"tick_size,omitempty"`
	// RegisteredMinimumExecutableQty is the expected minimum order quantity of
	// the registered instrument. The venue checks this declaration against the
	// instrument before constructing a bounded successor supplier, and the
	// analyzer retains it so a replay cannot silently treat an arbitrary
	// quantity as the exchange admission boundary.
	RegisteredMinimumExecutableQty int64 `json:"registered_minimum_executable_qty,omitempty"`
	// QuoteOnOneSidedLocalBook opts into the SV1D successor behavior. False
	// preserves the predecessor's two-sided-only policy.
	QuoteOnOneSidedLocalBook bool `json:"quote_on_one_sided_local_book,omitempty"`
	// MaxLossQuote is a finite quote-denominated loss budget. Zero preserves
	// the historical supplier contract; the SV1B roster must register a
	// positive budget so the participant can withdraw when its marked equity
	// deteriorates.
	MaxLossQuote int64 `json:"max_loss_quote,omitempty"`
	// MakerFeeBps is charged in quote units on passive fills. Zero preserves
	// the historical default; successor rosters may register a positive cost
	// so fee-bearing inventory risk is exercised by the actual exchange.
	MakerFeeBps int64 `json:"maker_fee_bps,omitempty"`
}

func (s ElasticLiquiditySupplierSpec) validate() error {
	if s.Role == "" || roleClass(s.Role) != "cdf_elastic_supplier" {
		return fmt.Errorf("role must be a numbered cdf_elastic_supplier role, got %q", s.Role)
	}
	if s.Symbol == "" || s.BaseAsset == "" || s.QuoteAsset == "" || s.BaseAsset == s.QuoteAsset {
		return fmt.Errorf("symbol and distinct base/quote assets are required")
	}
	if s.BasePrecision <= 0 || s.QuotePrecision <= 0 || s.InitialBaseBalance <= 0 || s.InitialQuoteBalance <= 0 {
		return fmt.Errorf("precisions and initial balances must be positive")
	}
	if s.Interval <= 0 || s.MaxObservationAge <= 0 || s.ReferenceHalfLife <= 0 {
		return fmt.Errorf("interval, observation age, and reference half-life must be positive")
	}
	if s.DecisionPhaseOffset < 0 || s.DecisionPhaseOffset >= s.Interval {
		return fmt.Errorf("decision phase offset must be in [0, interval), got %s for interval %s", s.DecisionPhaseOffset, s.Interval)
	}
	if s.ReferencePrice <= 0 || s.ElasticityPerPercent <= 0 || s.MaxPosition <= 0 || s.MaxInventory <= 0 || s.MaxQuoteQty <= 0 {
		return fmt.Errorf("reference, elasticity, position, inventory, and quote limits must be positive")
	}
	if s.MakerFeeBps < 0 || s.MakerFeeBps > 10_000 {
		return fmt.Errorf("maker fee must be between 0 and 10000 bps, got %d", s.MakerFeeBps)
	}
	if s.MaxLossQuote < 0 {
		return fmt.Errorf("maximum loss budget must not be negative, got %d", s.MaxLossQuote)
	}
	if s.MinimumExecutableQty < 0 || (s.MaxLossQuote > 0 && s.MinimumExecutableQty <= 0) {
		return fmt.Errorf("minimum executable quantity must be positive for bounded-risk rosters and never negative, got %d", s.MinimumExecutableQty)
	}
	if s.MinimumQualifyingQty < 0 {
		return fmt.Errorf("minimum qualifying quantity must not be negative, got %d", s.MinimumQualifyingQty)
	}
	if s.MinimumQualifyingQty > 0 && (s.MinimumExecutableQty <= 0 || s.MinimumQualifyingQty < s.MinimumExecutableQty) {
		return fmt.Errorf("minimum qualifying quantity %d must be at least the executable minimum %d", s.MinimumQualifyingQty, s.MinimumExecutableQty)
	}
	if s.TickSize < 0 {
		return fmt.Errorf("tick size must not be negative, got %d", s.TickSize)
	}
	if s.RegisteredMinimumExecutableQty < 0 {
		return fmt.Errorf("registered minimum executable quantity must not be negative, got %d", s.RegisteredMinimumExecutableQty)
	}
	if s.QuoteOnOneSidedLocalBook && (s.TickSize <= 0 || s.MaxLossQuote <= 0 || s.MinimumExecutableQty <= 0 || s.MinimumQualifyingQty <= s.MinimumExecutableQty) {
		return fmt.Errorf("one-sided local-book mode requires a registered tick, positive loss budget, executable quantity, and larger qualifying quantity")
	}
	if s.QuoteOnOneSidedLocalBook && s.RegisteredMinimumExecutableQty <= 0 {
		return fmt.Errorf("one-sided local-book mode requires the registered executable minimum")
	}
	if s.BaseHolding < -s.MaxPosition || s.BaseHolding > s.MaxPosition {
		return fmt.Errorf("base holding %d exceeds position limit %d", s.BaseHolding, s.MaxPosition)
	}
	if s.InitialBaseBalance < s.MaxPosition {
		return fmt.Errorf("initial base balance %d is below position displacement limit %d", s.InitialBaseBalance, s.MaxPosition)
	}
	if s.InitialBaseBalance > s.MaxInventory {
		return fmt.Errorf("initial base balance %d exceeds gross inventory limit %d", s.InitialBaseBalance, s.MaxInventory)
	}
	maximumBaseBalance := new(big.Int).Add(big.NewInt(s.InitialBaseBalance), big.NewInt(s.MaxPosition))
	if maximumBaseBalance.Cmp(big.NewInt(s.MaxInventory)) > 0 {
		return fmt.Errorf("initial base balance %d plus position limit %d exceeds gross inventory limit %d", s.InitialBaseBalance, s.MaxPosition, s.MaxInventory)
	}
	maximumNotionalBig := new(big.Int).Mul(big.NewInt(s.MaxPosition), big.NewInt(s.ReferencePrice))
	maximumNotionalBig.Quo(maximumNotionalBig, big.NewInt(s.BasePrecision))
	if !maximumNotionalBig.IsInt64() {
		return fmt.Errorf("maximum position notional overflows int64")
	}
	maximumNotional := maximumNotionalBig.Int64()
	if maximumNotional > s.InitialQuoteBalance {
		return fmt.Errorf("initial quote balance %d is below maximum reference notional %d", s.InitialQuoteBalance, maximumNotional)
	}
	return nil
}

// ElasticLiquiditySupplierConfig is the actor-side form of a roster entry.
// The observers are append-only instrumentation and never feed a decision
// back into the actor.
type ElasticLiquiditySupplierConfig struct {
	Role                           string
	ClientID                       uint64
	Symbol                         string
	BaseAsset                      string
	QuoteAsset                     string
	BasePrecision                  int64
	InitialBaseBalance             int64
	QuotePrecision                 int64
	InitialQuoteBalance            int64
	Interval                       time.Duration
	DecisionPhaseOffset            time.Duration
	MaxObservationAge              time.Duration
	ReferencePrice                 int64
	ReferenceHalfLife              time.Duration
	BaseHolding                    int64
	ElasticityPerPercent           int64
	MaxPosition                    int64
	MaxInventory                   int64
	MaxQuoteQty                    int64
	MinimumExecutableQty           int64
	MinimumQualifyingQty           int64
	RegisteredMinimumExecutableQty int64
	TickSize                       int64
	QuoteOnOneSidedLocalBook       bool
	MaxLossQuote                   int64
	MakerFeeBps                    int64
	DecisionObserver               func(ElasticLiquiditySupplierDecision)
	FillObserver                   func(ElasticLiquiditySupplierFill)
	ObservationFrontier            func() simulation.MarketDataFrontier
}

// ElasticLiquiditySupplierDecision records the local information and action
// selected at one supplier tick. Account snapshots and exchange fills remain
// the authoritative PnL source; Position and MarkPrice make the local action
// joinable to that economic evidence.
type ElasticLiquiditySupplierDecision struct {
	Role                           string `json:"role"`
	ClientID                       uint64 `json:"client_id"`
	Symbol                         string `json:"symbol"`
	DecisionTime                   int64  `json:"decision_time"`
	DecisionPhaseOffset            int64  `json:"decision_phase_offset_nanos"`
	ObservationTime                int64  `json:"observation_time"`
	ObservationAge                 int64  `json:"observation_age"`
	ObservationSequence            uint64 `json:"observation_sequence"`
	ObservationLinkID              uint32 `json:"observation_link_id"`
	ObservationOrdinal             uint64 `json:"observation_ordinal"`
	ObservationDeliveredAt         int64  `json:"observation_delivered_at"`
	ObservationFingerprint         string `json:"observation_fingerprint"`
	ObservationDigest              string `json:"observation_digest"`
	BestBid                        int64  `json:"best_bid"`
	BestBidQty                     int64  `json:"best_bid_qty"`
	BestAsk                        int64  `json:"best_ask"`
	BestAskQty                     int64  `json:"best_ask_qty"`
	MarkPrice                      int64  `json:"mark_price"`
	RiskMarkPrice                  int64  `json:"risk_mark_price"`
	LocalBookMode                  string `json:"local_book_mode"`
	QuotePriceSource               string `json:"quote_price_source"`
	RiskMarkSource                 string `json:"risk_mark_source"`
	ReferencePrice                 int64  `json:"reference_price"`
	Position                       int64  `json:"position"`
	TargetPosition                 int64  `json:"target_position"`
	InventoryLimit                 int64  `json:"inventory_limit"`
	InitialBaseBalance             int64  `json:"initial_base_balance"`
	GrossInventory                 int64  `json:"gross_inventory"`
	GrossInventoryLimit            int64  `json:"gross_inventory_limit"`
	Action                         string `json:"action"`
	Reason                         string `json:"reason"`
	Side                           string `json:"side,omitempty"`
	QuotePrice                     int64  `json:"quote_price,omitempty"`
	QuoteQty                       int64  `json:"quote_qty,omitempty"`
	MinimumQualifyingQty           int64  `json:"minimum_qualifying_qty,omitempty"`
	RegisteredMinimumExecutableQty int64  `json:"registered_minimum_executable_qty,omitempty"`
	QuoteOrderID                   uint64 `json:"quote_order_id,omitempty"`
	QuoteRequestID                 uint64 `json:"quote_request_id,omitempty"`
	CancelRequestID                uint64 `json:"cancel_request_id,omitempty"`
	QuoteSubmittedAt               int64  `json:"quote_submitted_at,omitempty"`
	QuoteCashAvailable             int64  `json:"quote_cash_available,omitempty"`
	QuoteCashReserved              int64  `json:"quote_cash_reserved"`
	QuoteCashRequired              int64  `json:"quote_cash_required,omitempty"`
	InitialEquityQuote             int64  `json:"initial_equity_quote"`
	EquityQuote                    int64  `json:"equity_quote"`
	PeakEquityQuote                int64  `json:"peak_equity_quote"`
	LossFromInitialQuote           int64  `json:"loss_from_initial_quote"`
	DrawdownQuote                  int64  `json:"drawdown_quote"`
	MaxLossQuote                   int64  `json:"max_loss_quote"`
	EquityAvailable                bool   `json:"equity_available"`
	RiskLimitTriggered             bool   `json:"risk_limit_triggered"`
}

// ElasticLiquiditySupplierFill joins a fill to the participant's local
// inventory transition. The exchange's ordinary fill record remains the
// source of truth for cash, fees, and realized PnL.
type ElasticLiquiditySupplierFill struct {
	Role           string `json:"role"`
	ClientID       uint64 `json:"client_id"`
	Symbol         string `json:"symbol"`
	OrderID        uint64 `json:"order_id"`
	TradeID        uint64 `json:"trade_id"`
	Timestamp      int64  `json:"timestamp"`
	Side           string `json:"side"`
	Price          int64  `json:"price"`
	Qty            int64  `json:"qty"`
	FeeAmount      int64  `json:"fee_amount"`
	FeeAsset       string `json:"fee_asset"`
	IsFull         bool   `json:"is_full"`
	PositionBefore int64  `json:"position_before"`
	PositionAfter  int64  `json:"position_after"`
}

type elasticLiquidityQuote struct {
	orderID              uint64
	requestID            uint64
	side                 exchange.Side
	price                int64
	qty                  int64
	submittedAt          int64
	observationSequence  uint64
	observationTimestamp int64
	oneSidedObservation  bool
}

// ElasticLiquiditySupplier posts at most one inventory-sensitive passive
// quote. It uses only the delayed local snapshot delivered through its normal
// actor gateway and can withdraw without replacement when its desired side or
// local observation disappears.
type ElasticLiquiditySupplier struct {
	*actor.BaseActor
	cfg                            ElasticLiquiditySupplierConfig
	bestBid                        int64
	bestBidQty                     int64
	bestAsk                        int64
	bestAskQty                     int64
	observationTime                int64
	observationSequence            uint64
	position                       int64
	reference                      int64
	lastReferenceUpdate            int64
	quote                          elasticLiquidityQuote
	pendingRequestID               uint64
	cancelRequestID                uint64
	cancelPending                  bool
	subscribed                     bool
	quoteCashAvailable             int64
	quoteCashReserved              int64
	initialEquityQuote             int64
	equityQuote                    int64
	peakEquityQuote                int64
	lossFromInitialQuote           int64
	drawdownQuote                  int64
	riskMarkPrice                  int64
	equityInitialized              bool
	equityUnavailable              bool
	riskLimitTriggered             bool
	lastClosedObservationSequence  uint64
	lastClosedObservationTimestamp int64
	requiresFreshObservation       bool
}

func NewElasticLiquiditySupplier(id uint64, gw actor.Gateway, cfg ElasticLiquiditySupplierConfig) *ElasticLiquiditySupplier {
	supplier := &ElasticLiquiditySupplier{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		reference: cfg.ReferencePrice,
	}
	if cfg.InitialQuoteBalance > 0 && cfg.QuotePrecision > 0 {
		supplier.quoteCashAvailable = cfg.InitialQuoteBalance
	}
	supplier.initializeMarkedEquity()
	supplier.SetHandler(supplier)
	supplier.AddTickerWithOffset(cfg.Interval, cfg.DecisionPhaseOffset, supplier.onTick)
	return supplier
}

func (s *ElasticLiquiditySupplier) Position() int64 { return s.position }

func (s *ElasticLiquiditySupplier) TargetPosition(price int64) int64 {
	if price <= 0 || s.reference <= 0 {
		return s.cfg.BaseHolding
	}
	percentAbove := (float64(price)/float64(s.reference) - 1) * 100
	target := float64(s.cfg.BaseHolding) - percentAbove*float64(s.cfg.ElasticityPerPercent)
	if !finite(target) {
		return s.cfg.BaseHolding
	}
	minimumPosition, maximumPosition := -s.cfg.MaxPosition, s.cfg.MaxPosition
	if s.cfg.MaxInventory > 0 {
		minimumPosition = maxInt64(minimumPosition, -s.cfg.InitialBaseBalance)
		maximumPosition = minInt64(maximumPosition, s.cfg.MaxInventory-s.cfg.InitialBaseBalance)
	}
	return int64(math.Max(float64(minimumPosition), math.Min(float64(maximumPosition), target)))
}

func (s *ElasticLiquiditySupplier) HandleEvent(_ context.Context, event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		s.observeSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		s.observeAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		s.observeFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		s.observeCancelled(event.Data.(actor.OrderCancelledEvent))
	case actor.EventOrderCancelRejected:
		s.observeCancelRejected(event.Data.(actor.OrderCancelRejectedEvent))
	case actor.EventOrderRejected:
		s.observeRejected(event.Data.(actor.OrderRejectedEvent))
	}
}

func (s *ElasticLiquiditySupplier) observeSnapshot(event actor.BookSnapshotEvent) {
	if event.Symbol != s.cfg.Symbol || event.Snapshot == nil {
		return
	}
	s.bestBid, s.bestBidQty, s.bestAsk, s.bestAskQty, s.observationTime = 0, 0, 0, 0, event.Timestamp
	s.observationSequence = event.SeqNum
	if len(event.Snapshot.Bids) > 0 {
		s.bestBid = event.Snapshot.Bids[0].Price
		s.bestBidQty = event.Snapshot.Bids[0].VisibleQty
	}
	if len(event.Snapshot.Asks) > 0 {
		s.bestAsk = event.Snapshot.Asks[0].Price
		s.bestAskQty = event.Snapshot.Asks[0].VisibleQty
	}
}

func (s *ElasticLiquiditySupplier) observeAccepted(event actor.OrderAcceptedEvent) {
	if event.RequestID != s.pendingRequestID {
		return
	}
	s.quote.orderID, s.quote.requestID = event.OrderID, event.RequestID
	s.pendingRequestID = 0
}

func (s *ElasticLiquiditySupplier) observeRejected(event actor.OrderRejectedEvent) {
	if event.RequestID == s.pendingRequestID {
		s.pendingRequestID = 0
		s.releaseQuoteReservation()
		s.quote = elasticLiquidityQuote{}
	}
}

func (s *ElasticLiquiditySupplier) observeFill(event actor.OrderFillEvent) {
	if event.Symbol != s.cfg.Symbol {
		return
	}
	if event.Qty <= 0 {
		s.rejectFillTransition()
		return
	}
	positionBefore := s.position
	if !s.validRestingFill(event) {
		s.rejectFillTransition()
		return
	}
	if !s.applyPositionDelta(event.Side, event.Qty) {
		s.rejectFillTransition()
		return
	}
	s.updateQuoteRemainingAfterFill(event)
	s.applyQuoteFill(event)
	s.emitFillEvidence(event, positionBefore)
	if event.IsFull {
		s.recordQuoteCloseObservation()
		s.quote = elasticLiquidityQuote{}
		// A full fill wins a concurrent cancellation race: the order no longer
		// exists, so a later cancel rejection must not strand the actor in its
		// cancel-pending state.
		s.cancelPending = false
		s.cancelRequestID = 0
		s.releaseQuoteReservation()
	}
}

func (s *ElasticLiquiditySupplier) validRestingFill(event actor.OrderFillEvent) bool {
	if event.OrderID == 0 || event.OrderID != s.quote.orderID || event.Qty <= 0 || event.Qty > s.quote.qty {
		return false
	}
	if event.Price <= 0 || event.Price != s.quote.price || event.Side != s.quote.side {
		return false
	}
	return event.IsFull == (event.Qty == s.quote.qty)
}

func (s *ElasticLiquiditySupplier) recordQuoteCloseObservation() {
	if !s.quote.oneSidedObservation {
		return
	}
	s.lastClosedObservationSequence = s.observationSequence
	s.lastClosedObservationTimestamp = s.observationTime
	s.requiresFreshObservation = true
}

func (s *ElasticLiquiditySupplier) hasFreshObservationAfterQuoteClose() bool {
	if !s.requiresFreshObservation {
		return true
	}
	if s.observationSequence > s.lastClosedObservationSequence {
		return true
	}
	return s.observationSequence == s.lastClosedObservationSequence && s.observationTime > s.lastClosedObservationTimestamp
}

func (s *ElasticLiquiditySupplier) emitFillEvidence(event actor.OrderFillEvent, positionBefore int64) {
	if s.cfg.FillObserver == nil {
		return
	}
	s.cfg.FillObserver(ElasticLiquiditySupplierFill{
		Role: s.cfg.Role, ClientID: s.cfg.ClientID, Symbol: event.Symbol,
		OrderID: event.OrderID, TradeID: event.TradeID, Timestamp: event.Timestamp,
		Side: event.Side.String(), Price: event.Price, Qty: event.Qty,
		FeeAmount: event.FeeAmount, FeeAsset: event.FeeAsset, IsFull: event.IsFull,
		PositionBefore: positionBefore, PositionAfter: s.position,
	})
}

func (s *ElasticLiquiditySupplier) rejectFillTransition() {
	// A gateway event that cannot be reconciled is not safe to interpret as a
	// partial economic transition. Withdraw on the next decision and keep the
	// local actor fail-closed until an account snapshot can be supplied.
	s.equityUnavailable = true
	s.riskLimitTriggered = true
}

func (s *ElasticLiquiditySupplier) applyPositionDelta(side exchange.Side, quantity int64) bool {
	if quantity <= 0 {
		return false
	}
	delta := quantity
	switch side {
	case exchange.Buy:
	case exchange.Sell:
		delta = -quantity
	default:
		return false
	}
	updatedPosition, ok := etypes.TryAdd(s.position, delta)
	if !ok || !s.positionWithinLimits(updatedPosition) {
		return false
	}
	s.position = updatedPosition
	return true
}

func (s *ElasticLiquiditySupplier) positionWithinLimits(position int64) bool {
	if s.cfg.MaxPosition > 0 && (position < -s.cfg.MaxPosition || position > s.cfg.MaxPosition) {
		return false
	}
	grossInventory, ok := etypes.TryAdd(s.cfg.InitialBaseBalance, position)
	if !ok || grossInventory < 0 {
		return false
	}
	return s.cfg.MaxInventory <= 0 || grossInventory <= s.cfg.MaxInventory
}

func (s *ElasticLiquiditySupplier) updateQuoteRemainingAfterFill(event actor.OrderFillEvent) {
	if event.IsFull || event.Qty >= s.quote.qty {
		s.quote.qty = 0
		return
	}
	if event.Qty > 0 {
		s.quote.qty -= event.Qty
	}
}

func (s *ElasticLiquiditySupplier) observeCancelled(event actor.OrderCancelledEvent) {
	if event.OrderID != s.quote.orderID {
		return
	}
	s.recordQuoteCloseObservation()
	s.quote = elasticLiquidityQuote{}
	s.releaseQuoteReservation()
	s.cancelPending = false
	s.cancelRequestID = 0
}

func (s *ElasticLiquiditySupplier) observeCancelRejected(event actor.OrderCancelRejectedEvent) {
	if event.OrderID == s.quote.orderID {
		s.cancelPending = false
		s.cancelRequestID = 0
	}
}

func (s *ElasticLiquiditySupplier) reviseReference(mid int64, now int64) {
	if mid <= 0 || s.cfg.ReferenceHalfLife <= 0 {
		return
	}
	if s.lastReferenceUpdate == 0 {
		s.lastReferenceUpdate = now
		return
	}
	elapsed := float64(now-s.lastReferenceUpdate) / float64(time.Second)
	s.lastReferenceUpdate = now
	if elapsed <= 0 {
		return
	}
	alpha := 1 - math.Exp(-math.Ln2*elapsed/s.cfg.ReferenceHalfLife.Seconds())
	revised := float64(s.reference) + alpha*(float64(mid)-float64(s.reference))
	if finite(revised) && revised > 0 {
		s.reference = int64(revised)
	}
}

type supplierLocalBookObservation struct {
	anchorPrice    int64
	riskMarkPrice  int64
	localBookMode  string
	riskMarkSource string
	failureReason  string
	ok             bool
}

func (s *ElasticLiquiditySupplier) observeLocalBook() supplierLocalBookObservation {
	mid, twoSided := positiveDomainTwoSidedMidpoint(s.bestBid, s.bestAsk)
	twoSided = twoSided && s.bestBid < s.bestAsk
	if twoSided {
		return supplierLocalBookObservation{
			anchorPrice:    mid,
			riskMarkPrice:  mid,
			localBookMode:  "two_sided",
			riskMarkSource: "two_sided_midpoint",
			ok:             true,
		}
	}
	if !s.cfg.QuoteOnOneSidedLocalBook {
		return supplierLocalBookObservation{failureReason: "one_sided_or_locked_book"}
	}
	if s.validPositiveTickPrice(s.bestBid) && s.bestBidQty > 0 && s.bestAsk == 0 && s.bestAskQty == 0 {
		grossInventory, inventoryOK := s.grossInventory()
		if !inventoryOK {
			return supplierLocalBookObservation{localBookMode: "one_sided", riskMarkSource: "one_sided_bid_unavailable", failureReason: "equity_unavailable"}
		}
		riskSource := "one_sided_bid"
		if grossInventory == 0 {
			riskSource = "one_sided_bid_zero_inventory"
		}
		return supplierLocalBookObservation{
			anchorPrice:    s.bestBid,
			riskMarkPrice:  s.bestBid,
			localBookMode:  "one_sided",
			riskMarkSource: riskSource,
			ok:             true,
		}
	}
	if s.validPositiveTickPrice(s.bestAsk) && s.bestAskQty > 0 && s.bestBid == 0 && s.bestBidQty == 0 {
		grossInventory, inventoryOK := s.grossInventory()
		if !inventoryOK {
			return supplierLocalBookObservation{localBookMode: "one_sided", riskMarkSource: "one_sided_ask_unavailable", failureReason: "equity_unavailable"}
		}
		if grossInventory > 0 {
			return supplierLocalBookObservation{
				anchorPrice:    s.bestAsk,
				localBookMode:  "one_sided",
				riskMarkSource: "one_sided_ask_unavailable",
				failureReason:  "equity_unavailable",
			}
		}
		return supplierLocalBookObservation{
			anchorPrice:    s.bestAsk,
			riskMarkPrice:  s.bestAsk,
			localBookMode:  "one_sided",
			riskMarkSource: "one_sided_ask_zero_inventory",
			ok:             true,
		}
	}
	return supplierLocalBookObservation{failureReason: "one_sided_or_locked_book"}
}

func (s *ElasticLiquiditySupplier) grossInventory() (int64, bool) {
	grossInventory, ok := etypes.TryAdd(s.cfg.InitialBaseBalance, s.position)
	if !ok || grossInventory < 0 {
		return 0, false
	}
	return grossInventory, true
}

func (s *ElasticLiquiditySupplier) oneSidedQuotePrice(side exchange.Side) (int64, bool) {
	if s.cfg.TickSize <= 0 || s.reference <= 0 {
		return 0, false
	}
	var touch int64
	var candidate int64
	switch side {
	case exchange.Sell:
		if s.bestBid <= 0 || s.bestBidQty <= 0 || s.bestAsk != 0 || s.bestAskQty != 0 {
			return 0, false
		}
		touch = s.bestBid
		var touchSum int64
		touchSum, ok := etypes.TryAdd(s.reference, touch)
		if !ok {
			return 0, false
		}
		blended := touchSum / 2
		lower, lowerOK := etypes.TryAdd(touch, s.cfg.TickSize)
		if !lowerOK {
			return 0, false
		}
		candidate = maxInt64(blended, lower)
		return ceilToPositiveTick(candidate, s.cfg.TickSize)
	case exchange.Buy:
		if s.bestAsk <= 0 || s.bestAskQty <= 0 || s.bestBid != 0 || s.bestBidQty != 0 {
			return 0, false
		}
		touch = s.bestAsk
		var touchSum int64
		touchSum, ok := etypes.TryAdd(s.reference, touch)
		if !ok {
			return 0, false
		}
		blended := touchSum / 2
		upper, upperOK := etypes.TrySub(touch, s.cfg.TickSize)
		if !upperOK || upper <= 0 {
			return 0, false
		}
		candidate = minInt64(blended, upper)
		return floorToPositiveTick(candidate, s.cfg.TickSize)
	default:
		return 0, false
	}
}

func (s *ElasticLiquiditySupplier) validPositiveTickPrice(price int64) bool {
	return price > 0 && s.cfg.TickSize > 0 && price%s.cfg.TickSize == 0
}

func floorToPositiveTick(price, tick int64) (int64, bool) {
	if price <= 0 || tick <= 0 {
		return 0, false
	}
	quotient := price / tick
	if quotient <= 0 || quotient > math.MaxInt64/tick {
		return 0, false
	}
	return quotient * tick, true
}

func ceilToPositiveTick(price, tick int64) (int64, bool) {
	if price <= 0 || tick <= 0 {
		return 0, false
	}
	quotient := price / tick
	if price%tick != 0 {
		if quotient == math.MaxInt64 {
			return 0, false
		}
		quotient++
	}
	if quotient <= 0 || quotient > math.MaxInt64/tick {
		return 0, false
	}
	return quotient * tick, true
}

func (s *ElasticLiquiditySupplier) onTick(now time.Time) {
	decision := s.baseDecision(now.UnixNano())
	if !s.subscribed {
		s.Subscribe(s.cfg.Symbol, exchange.MDSnapshot)
		s.subscribed = true
		decision.Action, decision.Reason = "wait", "subscribe"
		s.emitDecision(decision)
		return
	}
	if s.pendingRequestID != 0 {
		decision.Action, decision.Reason = "wait", "order_pending"
		s.emitDecision(decision)
		return
	}
	if s.cancelPending {
		decision.Action, decision.Reason = "wait", "cancel_pending"
		s.emitDecision(decision)
		return
	}
	if s.riskLimitTriggered || s.equityUnavailable {
		reason := "loss_limit"
		if s.equityUnavailable {
			reason = "equity_unavailable"
		}
		decision.Action, decision.Reason = s.withdrawIfNeeded(reason)
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}

	if !s.observationUsable(now.UnixNano()) {
		decision.Action, decision.Reason = s.withdrawIfNeeded("stale_or_missing_observation")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	localBook := s.observeLocalBook()
	if !localBook.ok {
		if localBook.anchorPrice > 0 && localBook.localBookMode == "one_sided" {
			s.reviseReference(localBook.anchorPrice, now.UnixNano())
			decision = s.baseDecision(now.UnixNano())
			decision.MarkPrice, decision.ReferencePrice = localBook.anchorPrice, s.reference
			// The anchor is still a valid input to the participant's inventory
			// target even when the risk mark is unavailable. Do not submit against
			// that state, but keep the intended target auditable.
			decision.TargetPosition = s.TargetPosition(localBook.anchorPrice)
			decision.RiskMarkPrice = localBook.riskMarkPrice
		}
		decision.LocalBookMode, decision.RiskMarkSource = localBook.localBookMode, localBook.riskMarkSource
		reason := localBook.failureReason
		if reason == "" {
			reason = "one_sided_or_locked_book"
		}
		if reason == "equity_unavailable" {
			decision.EquityAvailable = false
		}
		decision.Action, decision.Reason = s.withdrawIfNeeded(reason)
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	s.reviseReference(localBook.anchorPrice, now.UnixNano())
	decision = s.baseDecision(now.UnixNano())
	decision.MarkPrice, decision.ReferencePrice = localBook.anchorPrice, s.reference
	decision.LocalBookMode, decision.RiskMarkSource = localBook.localBookMode, localBook.riskMarkSource
	target := s.TargetPosition(localBook.anchorPrice)
	decision.TargetPosition = target
	if !s.updateMarkedRisk(localBook.riskMarkPrice) {
		decision = s.baseDecision(now.UnixNano())
		decision.MarkPrice, decision.ReferencePrice, decision.TargetPosition = localBook.anchorPrice, s.reference, target
		decision.LocalBookMode, decision.RiskMarkSource = localBook.localBookMode, localBook.riskMarkSource
		decision.Action, decision.Reason = s.withdrawIfNeeded("equity_unavailable")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	decision = s.baseDecision(now.UnixNano())
	decision.MarkPrice, decision.ReferencePrice, decision.TargetPosition = localBook.anchorPrice, s.reference, target
	decision.LocalBookMode, decision.RiskMarkSource = localBook.localBookMode, localBook.riskMarkSource
	if s.riskLimitTriggered {
		decision.Action, decision.Reason = s.withdrawIfNeeded("loss_limit")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	gap, gapOK := etypes.TrySub(target, s.position)
	if !gapOK {
		decision.Action, decision.Reason = s.withdrawIfNeeded("position_gap_overflow")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	if gap == 0 {
		decision.Action, decision.Reason = s.withdrawIfNeeded("inventory_at_target")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}

	desiredSide := exchange.Buy
	desiredPrice := s.bestBid
	decision.QuotePriceSource = "two_sided_touch"
	if gap < 0 {
		desiredSide, desiredPrice = exchange.Sell, s.bestAsk
	}
	if localBook.localBookMode == "one_sided" {
		presentSide := (desiredSide == exchange.Buy && s.bestBid > 0) || (desiredSide == exchange.Sell && s.bestAsk > 0)
		if presentSide && !s.validPositiveTickPrice(desiredPrice) {
			decision.Action, decision.Reason = s.withdrawIfNeeded("limit_or_touch_unavailable")
			decision.CancelRequestID = s.cancelRequestID
			s.emitDecision(decision)
			return
		}
		if !presentSide {
			var quoteOK bool
			desiredPrice, quoteOK = s.oneSidedQuotePrice(desiredSide)
			if !quoteOK {
				decision.Action, decision.Reason = s.withdrawIfNeeded("limit_or_touch_unavailable")
				decision.CancelRequestID = s.cancelRequestID
				s.emitDecision(decision)
				return
			}
			decision.QuotePriceSource = "one_sided_missing_side_blended"
		} else {
			decision.QuotePriceSource = "one_sided_present_touch"
		}
	}
	quantity := abs64(gap)
	if desiredSide == exchange.Buy {
		quantity = minInt64(quantity, s.availableBuyInventory())
	} else {
		quantity = minInt64(quantity, s.availableSellInventory())
	}
	if quantity > s.cfg.MaxQuoteQty {
		quantity = s.cfg.MaxQuoteQty
	}
	if desiredSide == exchange.Buy {
		quantity = minInt64(quantity, s.availableBuyQuote(desiredPrice))
	}
	if quantity <= 0 || desiredPrice <= 0 {
		decision.Action, decision.Reason = s.withdrawIfNeeded("limit_or_touch_unavailable")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	if s.cfg.MinimumExecutableQty > 0 && quantity < s.cfg.MinimumExecutableQty {
		decision.Action, decision.Reason = s.withdrawIfNeeded("below_minimum_executable_qty")
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	decision.Side, decision.QuotePrice, decision.QuoteQty = desiredSide.String(), desiredPrice, quantity
	if desiredSide == exchange.Buy {
		decision.QuoteCashAvailable = s.quoteCashAvailable
		decision.QuoteCashRequired, _ = quoteRequirement(desiredPrice, quantity, s.cfg.BasePrecision, s.cfg.MakerFeeBps)
	}
	if s.quote.orderID != 0 && s.quote.side == desiredSide && s.quote.price == desiredPrice && s.quote.qty == quantity {
		decision.Action, decision.Reason = "rest", "quote_unchanged"
		decision.QuoteOrderID, decision.QuoteRequestID, decision.QuoteSubmittedAt = s.quote.orderID, s.quote.requestID, s.quote.submittedAt
		s.emitDecision(decision)
		return
	}
	if s.quote.orderID != 0 {
		s.cancelPending = true
		s.cancelRequestID = s.CancelOrder(s.quote.orderID)
		decision.Action, decision.Reason = "cancel", "reprice_for_inventory_or_touch"
		decision.QuoteOrderID = s.quote.orderID
		decision.CancelRequestID = s.cancelRequestID
		s.emitDecision(decision)
		return
	}
	if localBook.localBookMode == "one_sided" && !s.hasFreshObservationAfterQuoteClose() {
		decision.Action, decision.Reason = "wait", "awaiting_fresh_observation_after_close"
		s.emitDecision(decision)
		return
	}

	if desiredSide == exchange.Buy && !s.quoteAccountingDisabled() {
		required, ok := quoteRequirement(desiredPrice, quantity, s.cfg.BasePrecision, s.cfg.MakerFeeBps)
		if !ok || required > s.quoteCashAvailable {
			decision.Action, decision.Reason = s.withdrawIfNeeded("quote_cash_limit")
			decision.CancelRequestID = s.cancelRequestID
			s.emitDecision(decision)
			return
		}
		s.quoteCashAvailable -= required
		s.quoteCashReserved += required
	}
	requestID := s.SubmitPostOnlyOrder(s.cfg.Symbol, desiredSide, desiredPrice, quantity)
	s.pendingRequestID = requestID
	s.quote = elasticLiquidityQuote{requestID: requestID, side: desiredSide, price: desiredPrice, qty: quantity, submittedAt: now.UnixNano(), observationSequence: s.observationSequence, observationTimestamp: s.observationTime, oneSidedObservation: localBook.localBookMode == "one_sided"}
	decision.Action, decision.Reason = "submit", "inventory_target_gap"
	decision.QuoteRequestID, decision.QuoteSubmittedAt = requestID, now.UnixNano()
	s.emitDecision(decision)
}

func (s *ElasticLiquiditySupplier) availableBuyInventory() int64 {
	if s.cfg.MaxInventory <= 0 {
		return math.MaxInt64
	}
	grossInventory, ok := s.grossInventory()
	if !ok {
		return 0
	}
	available, ok := etypes.TrySub(s.cfg.MaxInventory, grossInventory)
	if !ok || available < 0 {
		return 0
	}
	return available
}

func (s *ElasticLiquiditySupplier) availableSellInventory() int64 {
	if s.cfg.MaxInventory <= 0 {
		return math.MaxInt64
	}
	grossInventory, ok := s.grossInventory()
	if !ok {
		return 0
	}
	return grossInventory
}

func (s *ElasticLiquiditySupplier) availableBuyQuote(price int64) int64 {
	if s.quoteAccountingDisabled() {
		return math.MaxInt64
	}
	return maxAffordableQuoteQty(price, s.cfg.BasePrecision, s.cfg.MakerFeeBps, s.quoteCashAvailable, s.cfg.MaxQuoteQty)
}

func (s *ElasticLiquiditySupplier) quoteAccountingDisabled() bool {
	return s.cfg.InitialQuoteBalance <= 0 || s.cfg.QuotePrecision <= 0
}

func (s *ElasticLiquiditySupplier) releaseQuoteReservation() bool {
	if s.quoteAccountingDisabled() || s.quoteCashReserved <= 0 {
		return true
	}
	available, ok := etypes.TryAdd(s.quoteCashAvailable, s.quoteCashReserved)
	if !ok {
		s.equityUnavailable = true
		return false
	}
	s.quoteCashAvailable = available
	s.quoteCashReserved = 0
	return true
}

func (s *ElasticLiquiditySupplier) applyQuoteFill(event actor.OrderFillEvent) {
	if s.quoteAccountingDisabled() {
		return
	}
	notional, ok := quoteRequirement(event.Price, event.Qty, s.cfg.BasePrecision, 0)
	if !ok {
		return
	}
	fee := int64(0)
	if event.FeeAsset == s.cfg.QuoteAsset && event.FeeAmount > 0 {
		fee = event.FeeAmount
	}
	if event.Side == exchange.Buy {
		spent, ok := exactQuoteAmount(notional, fee)
		if !ok || spent < 0 || spent > s.quoteCashReserved {
			s.equityUnavailable = true
			return
		}
		s.quoteCashReserved -= spent
	} else if event.Side == exchange.Sell {
		received, ok := exactQuoteAmount(notional, -fee)
		if !ok || received < 0 {
			s.equityUnavailable = true
			return
		}
		available, addOK := etypes.TryAdd(s.quoteCashAvailable, received)
		if !addOK {
			s.equityUnavailable = true
			return
		}
		s.quoteCashAvailable = available
	}
	if event.IsFull {
		s.releaseQuoteReservation()
	}
}

func quoteRequirement(price, quantity, basePrecision, makerFeeBps int64) (int64, bool) {
	if price <= 0 || quantity <= 0 || basePrecision <= 0 || makerFeeBps < 0 {
		return 0, false
	}
	notional := new(big.Int).Mul(big.NewInt(price), big.NewInt(quantity))
	notional.Quo(notional, big.NewInt(basePrecision))
	fee := new(big.Int).Mul(new(big.Int).Set(notional), big.NewInt(makerFeeBps))
	fee.Quo(fee, big.NewInt(10_000))
	notional.Add(notional, fee)
	if !notional.IsInt64() {
		return 0, false
	}
	return notional.Int64(), true
}

func exactQuoteAmount(left, right int64) (int64, bool) {
	amount := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !amount.IsInt64() {
		return 0, false
	}
	return amount.Int64(), true
}

func maxAffordableQuoteQty(price, basePrecision, makerFeeBps, available, upper int64) int64 {
	if available <= 0 || upper <= 0 {
		return 0
	}
	low, high := int64(0), upper
	for low < high {
		mid := low + (high-low+1)/2
		required, ok := quoteRequirement(price, mid, basePrecision, makerFeeBps)
		if ok && required <= available {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
}

func (s *ElasticLiquiditySupplier) baseDecision(now int64) ElasticLiquiditySupplierDecision {
	age := int64(0)
	if s.observationTime > 0 && now >= s.observationTime {
		age = now - s.observationTime
	}
	var frontier simulation.MarketDataFrontier
	if s.cfg.ObservationFrontier != nil {
		frontier = s.cfg.ObservationFrontier()
	}
	fingerprint := ""
	if frontier.Fingerprint != ([16]byte{}) {
		fingerprint = hex.EncodeToString(frontier.Fingerprint[:])
	}
	digest := ""
	if frontier.Digest != ([16]byte{}) {
		digest = hex.EncodeToString(frontier.Digest[:])
	}
	grossInventory, inventoryOK := s.grossInventory()
	if !inventoryOK {
		grossInventory = 0
	}
	return ElasticLiquiditySupplierDecision{
		Role: s.cfg.Role, ClientID: s.cfg.ClientID, Symbol: s.cfg.Symbol,
		DecisionTime: now, ObservationTime: s.observationTime, ObservationAge: age,
		DecisionPhaseOffset: int64(s.cfg.DecisionPhaseOffset),
		BestBid:             s.bestBid, BestBidQty: s.bestBidQty, BestAsk: s.bestAsk, BestAskQty: s.bestAskQty,
		RiskMarkPrice:       s.riskMarkPrice,
		RiskMarkSource:      "",
		ObservationSequence: s.observationSequence, ObservationLinkID: frontier.LinkID,
		ObservationOrdinal: frontier.Ordinal, ObservationDeliveredAt: frontier.DeliveredAt,
		ObservationFingerprint: fingerprint,
		ObservationDigest:      digest,
		ReferencePrice:         s.reference,
		Position:               s.position, InventoryLimit: s.cfg.MaxPosition,
		InitialBaseBalance: s.cfg.InitialBaseBalance, GrossInventory: grossInventory,
		GrossInventoryLimit: s.cfg.MaxInventory,
		QuoteOrderID:        s.quote.orderID, QuoteRequestID: s.quote.requestID,
		QuotePrice: s.quote.price, QuoteQty: s.quote.qty, QuoteSubmittedAt: s.quote.submittedAt,
		CancelRequestID:    s.cancelRequestID,
		QuoteCashAvailable: s.quoteCashAvailable,
		QuoteCashReserved:  s.quoteCashReserved,
		InitialEquityQuote: s.initialEquityQuote, EquityQuote: s.equityQuote,
		PeakEquityQuote: s.peakEquityQuote, LossFromInitialQuote: s.lossFromInitialQuote,
		DrawdownQuote: s.drawdownQuote, MaxLossQuote: s.cfg.MaxLossQuote,
		EquityAvailable:                s.cfg.MaxLossQuote > 0 && s.equityInitialized && !s.equityUnavailable,
		RiskLimitTriggered:             s.riskLimitTriggered,
		MinimumQualifyingQty:           s.cfg.MinimumQualifyingQty,
		RegisteredMinimumExecutableQty: s.cfg.RegisteredMinimumExecutableQty,
	}
}

func (s *ElasticLiquiditySupplier) initializeMarkedEquity() {
	if s.cfg.MaxLossQuote <= 0 {
		return
	}
	equity, ok := s.markedEquityQuote(s.cfg.ReferencePrice)
	if !ok {
		s.equityUnavailable = true
		return
	}
	s.initialEquityQuote, s.equityQuote, s.peakEquityQuote = equity, equity, equity
	s.riskMarkPrice = s.cfg.ReferencePrice
	s.equityInitialized = true
}

func (s *ElasticLiquiditySupplier) updateMarkedRisk(mid int64) bool {
	if s.cfg.MaxLossQuote <= 0 {
		return true
	}
	if s.equityUnavailable {
		return false
	}
	equity, ok := s.markedEquityQuote(mid)
	if !ok {
		s.equityUnavailable = true
		return false
	}
	if !s.equityInitialized {
		s.initialEquityQuote, s.peakEquityQuote = equity, equity
		s.equityInitialized = true
	}
	s.equityQuote = equity
	s.riskMarkPrice = mid
	if equity > s.peakEquityQuote {
		s.peakEquityQuote = equity
	}
	var differenceOK bool
	s.lossFromInitialQuote, differenceOK = positiveDifference(s.initialEquityQuote, equity)
	if !differenceOK {
		s.equityUnavailable = true
		return false
	}
	s.drawdownQuote, differenceOK = positiveDifference(s.peakEquityQuote, equity)
	if !differenceOK {
		s.equityUnavailable = true
		return false
	}
	if s.lossFromInitialQuote >= s.cfg.MaxLossQuote || s.drawdownQuote >= s.cfg.MaxLossQuote {
		s.riskLimitTriggered = true
	}
	return true
}

func (s *ElasticLiquiditySupplier) markedEquityQuote(mid int64) (int64, bool) {
	if mid <= 0 || s.cfg.BasePrecision <= 0 {
		return 0, false
	}
	grossInventory, ok := exactQuoteAmount(s.cfg.InitialBaseBalance, s.position)
	if !ok || grossInventory < 0 {
		return 0, false
	}
	notional := new(big.Int).Mul(big.NewInt(grossInventory), big.NewInt(mid))
	notional.Quo(notional, big.NewInt(s.cfg.BasePrecision))
	equity := new(big.Int).Add(notional, big.NewInt(s.quoteCashAvailable))
	equity.Add(equity, big.NewInt(s.quoteCashReserved))
	if !equity.IsInt64() {
		return 0, false
	}
	return equity.Int64(), true
}

func positiveDifference(larger, smaller int64) (int64, bool) {
	difference := new(big.Int).Sub(big.NewInt(larger), big.NewInt(smaller))
	if difference.Sign() <= 0 {
		return 0, true
	}
	if !difference.IsInt64() {
		return 0, false
	}
	return difference.Int64(), true
}

func (s *ElasticLiquiditySupplier) observationUsable(now int64) bool {
	return s.observationTime > 0 && now >= s.observationTime && now-s.observationTime <= int64(s.cfg.MaxObservationAge)
}

func (s *ElasticLiquiditySupplier) withdrawIfNeeded(reason string) (string, string) {
	if s.quote.orderID == 0 {
		return "wait", reason
	}
	s.cancelPending = true
	s.cancelRequestID = s.CancelOrder(s.quote.orderID)
	return "withdraw", reason
}

func (s *ElasticLiquiditySupplier) emitDecision(decision ElasticLiquiditySupplierDecision) {
	if s.cfg.DecisionObserver != nil {
		s.cfg.DecisionObserver(decision)
	}
}
