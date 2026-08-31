package multivenue

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
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
	InitialBaseBalance   int64         `json:"initial_base_balance"`
	InitialQuoteBalance  int64         `json:"initial_quote_balance"`
	Interval             time.Duration `json:"interval"`
	MaxObservationAge    time.Duration `json:"max_observation_age"`
	ReferencePrice       int64         `json:"reference_price"`
	ReferenceHalfLife    time.Duration `json:"reference_half_life"`
	BaseHolding          int64         `json:"base_holding"`
	ElasticityPerPercent int64         `json:"elasticity_per_percent"`
	MaxPosition          int64         `json:"max_position"`
	MaxQuoteQty          int64         `json:"max_quote_qty"`
}

func (s ElasticLiquiditySupplierSpec) validate() error {
	if s.Role == "" || roleClass(s.Role) == s.Role {
		return fmt.Errorf("role must be a numbered liquidity-supplier role, got %q", s.Role)
	}
	if s.Symbol == "" || s.BaseAsset == "" || s.QuoteAsset == "" || s.BaseAsset == s.QuoteAsset {
		return fmt.Errorf("symbol and distinct base/quote assets are required")
	}
	if s.BasePrecision <= 0 || s.InitialBaseBalance <= 0 || s.InitialQuoteBalance <= 0 {
		return fmt.Errorf("initial balances must be positive")
	}
	if s.Interval <= 0 || s.MaxObservationAge <= 0 || s.ReferenceHalfLife <= 0 {
		return fmt.Errorf("interval, observation age, and reference half-life must be positive")
	}
	if s.ReferencePrice <= 0 || s.ElasticityPerPercent <= 0 || s.MaxPosition <= 0 || s.MaxQuoteQty <= 0 {
		return fmt.Errorf("reference, elasticity, position, and quote limits must be positive")
	}
	if s.BaseHolding < -s.MaxPosition || s.BaseHolding > s.MaxPosition {
		return fmt.Errorf("base holding %d exceeds position limit %d", s.BaseHolding, s.MaxPosition)
	}
	if s.InitialBaseBalance < s.MaxPosition {
		return fmt.Errorf("initial base balance %d is below position limit %d", s.InitialBaseBalance, s.MaxPosition)
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
	Role                 string
	ClientID             uint64
	Symbol               string
	BaseAsset            string
	QuoteAsset           string
	BasePrecision        int64
	Interval             time.Duration
	MaxObservationAge    time.Duration
	ReferencePrice       int64
	ReferenceHalfLife    time.Duration
	BaseHolding          int64
	ElasticityPerPercent int64
	MaxPosition          int64
	MaxQuoteQty          int64
	DecisionObserver     func(ElasticLiquiditySupplierDecision)
	FillObserver         func(ElasticLiquiditySupplierFill)
}

// ElasticLiquiditySupplierDecision records the local information and action
// selected at one supplier tick. Account snapshots and exchange fills remain
// the authoritative PnL source; Position and MarkPrice make the local action
// joinable to that economic evidence.
type ElasticLiquiditySupplierDecision struct {
	Role                string `json:"role"`
	ClientID            uint64 `json:"client_id"`
	Symbol              string `json:"symbol"`
	DecisionTime        int64  `json:"decision_time"`
	ObservationTime     int64  `json:"observation_time"`
	ObservationAge      int64  `json:"observation_age"`
	ObservationSequence uint64 `json:"observation_sequence"`
	BestBid             int64  `json:"best_bid"`
	BestBidQty          int64  `json:"best_bid_qty"`
	BestAsk             int64  `json:"best_ask"`
	BestAskQty          int64  `json:"best_ask_qty"`
	MarkPrice           int64  `json:"mark_price"`
	ReferencePrice      int64  `json:"reference_price"`
	Position            int64  `json:"position"`
	TargetPosition      int64  `json:"target_position"`
	InventoryLimit      int64  `json:"inventory_limit"`
	Action              string `json:"action"`
	Reason              string `json:"reason"`
	Side                string `json:"side,omitempty"`
	QuotePrice          int64  `json:"quote_price,omitempty"`
	QuoteQty            int64  `json:"quote_qty,omitempty"`
	QuoteOrderID        uint64 `json:"quote_order_id,omitempty"`
	QuoteRequestID      uint64 `json:"quote_request_id,omitempty"`
	QuoteSubmittedAt    int64  `json:"quote_submitted_at,omitempty"`
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
	orderID     uint64
	requestID   uint64
	side        exchange.Side
	price       int64
	qty         int64
	submittedAt int64
}

// ElasticLiquiditySupplier posts at most one inventory-sensitive passive
// quote. It uses only the delayed local snapshot delivered through its normal
// actor gateway and can withdraw without replacement when its desired side or
// local observation disappears.
type ElasticLiquiditySupplier struct {
	*actor.BaseActor
	cfg                 ElasticLiquiditySupplierConfig
	bestBid             int64
	bestBidQty          int64
	bestAsk             int64
	bestAskQty          int64
	observationTime     int64
	observationSequence uint64
	position            int64
	reference           int64
	lastReferenceUpdate int64
	quote               elasticLiquidityQuote
	pendingRequestID    uint64
	cancelPending       bool
	subscribed          bool
}

func NewElasticLiquiditySupplier(id uint64, gw actor.Gateway, cfg ElasticLiquiditySupplierConfig) *ElasticLiquiditySupplier {
	supplier := &ElasticLiquiditySupplier{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		reference: cfg.ReferencePrice,
	}
	supplier.SetHandler(supplier)
	supplier.AddTicker(cfg.Interval, supplier.onTick)
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
	limit := float64(s.cfg.MaxPosition)
	return int64(math.Max(-limit, math.Min(limit, target)))
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
	}
}

func (s *ElasticLiquiditySupplier) observeFill(event actor.OrderFillEvent) {
	if event.Symbol != s.cfg.Symbol || event.OrderID != s.quote.orderID {
		return
	}
	positionBefore := s.position
	if event.Side == exchange.Buy {
		s.position += event.Qty
	} else {
		s.position -= event.Qty
	}
	if s.cfg.FillObserver != nil {
		s.cfg.FillObserver(ElasticLiquiditySupplierFill{
			Role: s.cfg.Role, ClientID: s.cfg.ClientID, Symbol: event.Symbol,
			OrderID: event.OrderID, TradeID: event.TradeID, Timestamp: event.Timestamp,
			Side: event.Side.String(), Price: event.Price, Qty: event.Qty,
			FeeAmount: event.FeeAmount, FeeAsset: event.FeeAsset, IsFull: event.IsFull,
			PositionBefore: positionBefore, PositionAfter: s.position,
		})
	}
	if event.IsFull {
		s.quote = elasticLiquidityQuote{}
	}
}

func (s *ElasticLiquiditySupplier) observeCancelled(event actor.OrderCancelledEvent) {
	if event.OrderID != s.quote.orderID {
		return
	}
	s.quote = elasticLiquidityQuote{}
	s.cancelPending = false
}

func (s *ElasticLiquiditySupplier) observeCancelRejected(event actor.OrderCancelRejectedEvent) {
	if event.OrderID == s.quote.orderID {
		s.cancelPending = false
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

	if !s.observationUsable(now.UnixNano()) {
		decision.Action, decision.Reason = s.withdrawIfNeeded("stale_or_missing_observation")
		s.emitDecision(decision)
		return
	}
	mid, available := positiveDomainTwoSidedMidpoint(s.bestBid, s.bestAsk)
	if !available || s.bestBid >= s.bestAsk {
		decision.Action, decision.Reason = s.withdrawIfNeeded("one_sided_or_locked_book")
		s.emitDecision(decision)
		return
	}
	s.reviseReference(mid, now.UnixNano())
	decision.MarkPrice, decision.ReferencePrice = mid, s.reference
	target := s.TargetPosition(mid)
	decision.TargetPosition = target
	gap := target - s.position
	if gap == 0 {
		decision.Action, decision.Reason = s.withdrawIfNeeded("inventory_at_target")
		s.emitDecision(decision)
		return
	}

	desiredSide := exchange.Buy
	desiredPrice := s.bestBid
	if gap < 0 {
		desiredSide, desiredPrice = exchange.Sell, s.bestAsk
	}
	quantity := abs64(gap)
	if quantity > s.cfg.MaxQuoteQty {
		quantity = s.cfg.MaxQuoteQty
	}
	if quantity <= 0 || desiredPrice <= 0 {
		decision.Action, decision.Reason = s.withdrawIfNeeded("limit_or_touch_unavailable")
		s.emitDecision(decision)
		return
	}
	decision.Side, decision.QuotePrice, decision.QuoteQty = desiredSide.String(), desiredPrice, quantity
	if s.quote.orderID != 0 && s.quote.side == desiredSide && s.quote.price == desiredPrice && s.quote.qty == quantity {
		decision.Action, decision.Reason = "rest", "quote_unchanged"
		decision.QuoteOrderID, decision.QuoteRequestID, decision.QuoteSubmittedAt = s.quote.orderID, s.quote.requestID, s.quote.submittedAt
		s.emitDecision(decision)
		return
	}
	if s.quote.orderID != 0 {
		s.cancelPending = true
		s.CancelOrder(s.quote.orderID)
		decision.Action, decision.Reason = "cancel", "reprice_for_inventory_or_touch"
		decision.QuoteOrderID = s.quote.orderID
		s.emitDecision(decision)
		return
	}

	requestID := s.SubmitPostOnlyOrder(s.cfg.Symbol, desiredSide, desiredPrice, quantity)
	s.pendingRequestID = requestID
	s.quote = elasticLiquidityQuote{requestID: requestID, side: desiredSide, price: desiredPrice, qty: quantity, submittedAt: now.UnixNano()}
	decision.Action, decision.Reason = "submit", "inventory_target_gap"
	decision.QuoteRequestID, decision.QuoteSubmittedAt = requestID, now.UnixNano()
	s.emitDecision(decision)
}

func (s *ElasticLiquiditySupplier) baseDecision(now int64) ElasticLiquiditySupplierDecision {
	age := int64(0)
	if s.observationTime > 0 && now >= s.observationTime {
		age = now - s.observationTime
	}
	return ElasticLiquiditySupplierDecision{
		Role: s.cfg.Role, ClientID: s.cfg.ClientID, Symbol: s.cfg.Symbol,
		DecisionTime: now, ObservationTime: s.observationTime, ObservationAge: age,
		BestBid: s.bestBid, BestBidQty: s.bestBidQty, BestAsk: s.bestAsk, BestAskQty: s.bestAskQty,
		ObservationSequence: s.observationSequence,
		ReferencePrice:      s.reference,
		Position:            s.position, InventoryLimit: s.cfg.MaxPosition,
		QuoteOrderID: s.quote.orderID, QuoteRequestID: s.quote.requestID,
		QuotePrice: s.quote.price, QuoteQty: s.quote.qty, QuoteSubmittedAt: s.quote.submittedAt,
	}
}

func (s *ElasticLiquiditySupplier) observationUsable(now int64) bool {
	return s.observationTime > 0 && now >= s.observationTime && now-s.observationTime <= int64(s.cfg.MaxObservationAge)
}

func (s *ElasticLiquiditySupplier) withdrawIfNeeded(reason string) (string, string) {
	if s.quote.orderID == 0 {
		return "wait", reason
	}
	s.cancelPending = true
	s.CancelOrder(s.quote.orderID)
	return "withdraw", reason
}

func (s *ElasticLiquiditySupplier) emitDecision(decision ElasticLiquiditySupplierDecision) {
	if s.cfg.DecisionObserver != nil {
		s.cfg.DecisionObserver(decision)
	}
}
