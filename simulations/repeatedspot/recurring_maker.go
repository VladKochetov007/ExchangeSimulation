package repeatedspot

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

// MakerQuoteInput contains only actor-known state. Quote rules may not read an
// exchange book, a shared price oracle, or another actor's private state.
type MakerQuoteInput struct {
	MidPrice              int64
	FilledInventory       int64
	WorkingLimit          int64
	LogVariancePerSecond  float64
	DeliveredTradeSamples uint64
}

type MakerQuote struct {
	BidPrice int64
	AskPrice int64
}

type MakerDecision struct {
	ActorID               uint64  `json:"actor_id"`
	ScheduledAt           int64   `json:"scheduled_at"`
	DecisionAt            int64   `json:"decision_at"`
	BookSeen              bool    `json:"book_seen"`
	LatestBookSourceAt    int64   `json:"latest_book_source_at"`
	LatestBookSequence    uint64  `json:"latest_book_sequence"`
	LatestBookProcessedAt int64   `json:"latest_book_processed_at"`
	BestBid               int64   `json:"best_bid"`
	BestAsk               int64   `json:"best_ask"`
	FilledInventory       int64   `json:"filled_inventory"`
	WorkingLower          int64   `json:"working_lower"`
	WorkingUpper          int64   `json:"working_upper"`
	LogVariancePerSecond  float64 `json:"log_variance_per_second"`
	DeliveredTradeSamples uint64  `json:"delivered_trade_samples"`
	TargetBid             int64   `json:"target_bid"`
	TargetAsk             int64   `json:"target_ask"`
	Action                string  `json:"action"`
}

type MakerObservation struct {
	ActorID        uint64 `json:"actor_id"`
	Kind           string `json:"kind"`
	ProcessedAt    int64  `json:"processed_at"`
	SourceAt       int64  `json:"source_at"`
	SourceSequence uint64 `json:"source_sequence"`
	Symbol         string `json:"symbol"`
	BestBid        int64  `json:"best_bid,omitempty"`
	BestAsk        int64  `json:"best_ask,omitempty"`
	TradeID        uint64 `json:"trade_id,omitempty"`
	TradePrice     int64  `json:"trade_price,omitempty"`
	TradeQty       int64  `json:"trade_qty,omitempty"`
	TradeSide      string `json:"trade_side,omitempty"`
}

type MakerQuoteRule func(MakerQuoteInput) (MakerQuote, bool)

type RecurringMakerConfig struct {
	Symbol                      string        `json:"symbol"`
	QuoteQty                    int64         `json:"quote_qty"`
	MinQuoteQty                 int64         `json:"min_quote_qty"`
	WorkingLimit                int64         `json:"working_limit"`
	TickSize                    int64         `json:"tick_size"`
	QuoteInterval               time.Duration `json:"quote_interval_ns"`
	RequoteBps                  int64         `json:"requote_bps"`
	InitialLogVariancePerSecond float64       `json:"initial_log_variance_per_second"`
	VolatilityHalfLife          time.Duration `json:"volatility_half_life_ns"`
	VolatilitySampleInterval    time.Duration `json:"volatility_sample_interval_ns"`
	MaxLogVarianceMultiple      float64       `json:"max_log_variance_multiple"`
}

// RecurringMaker gives every injected quote rule the same post-only,
// cancel-and-wait lifecycle and the same pending-inclusive inventory guard.
// A cancellation request never creates capacity by itself.
type RecurringMaker struct {
	*actor.BaseActor
	config                RecurringMakerConfig
	quoteRule             MakerQuoteRule
	inventory             *workingInventory
	bestBid               int64
	bestAsk               int64
	latestBookAt          int64
	latestBookSequence    uint64
	latestBookProcessedAt int64
	bookSeen              bool
	activeBid             uint64
	activeAsk             uint64
	quotedBid             int64
	quotedAsk             int64
	cancelRequested       map[uint64]struct{}
	unresolvedEnd         map[uint64]struct{}
	subscribed            bool
	started               atomic.Bool
	fault                 error
	logVariancePerSecond  float64
	lastTradePrice        int64
	lastTradeSourceTime   int64
	tradeSamples          uint64
	decisionObserver      func(MakerDecision)
	observationObserver   func(MakerObservation)
}

func NewRecurringMaker(id uint64, gateway actor.Gateway, config RecurringMakerConfig, quoteRule MakerQuoteRule) (*RecurringMaker, error) {
	if gateway == nil || config.Symbol == "" || config.QuoteQty <= 0 || config.MinQuoteQty <= 0 ||
		config.MinQuoteQty > config.QuoteQty || config.WorkingLimit <= 0 || config.TickSize <= 0 ||
		config.QuoteInterval <= 0 || config.RequoteBps < 0 || quoteRule == nil ||
		math.IsNaN(config.InitialLogVariancePerSecond) || math.IsInf(config.InitialLogVariancePerSecond, 0) ||
		config.InitialLogVariancePerSecond < 0 || config.VolatilityHalfLife < 0 || config.VolatilitySampleInterval < 0 ||
		math.IsNaN(config.MaxLogVarianceMultiple) || math.IsInf(config.MaxLogVarianceMultiple, 0) || config.MaxLogVarianceMultiple < 0 ||
		math.IsInf(config.InitialLogVariancePerSecond*config.MaxLogVarianceMultiple, 0) {
		return nil, fmt.Errorf("repeatedspot: invalid recurring maker contract")
	}
	inventory, err := newWorkingInventory(0, config.WorkingLimit)
	if err != nil {
		return nil, err
	}
	maker := &RecurringMaker{BaseActor: actor.NewBaseActor(id, gateway), config: config,
		quoteRule: quoteRule, inventory: inventory, cancelRequested: make(map[uint64]struct{}),
		unresolvedEnd: make(map[uint64]struct{}), logVariancePerSecond: config.InitialLogVariancePerSecond}
	maker.SetHandler(maker)
	maker.AddTicker(config.QuoteInterval, maker.onTick)
	return maker, nil
}

func (maker *RecurringMaker) Fault() error {
	if maker.fault != nil {
		return maker.fault
	}
	if len(maker.unresolvedEnd) != 0 {
		return fmt.Errorf("repeatedspot: maker %d has an unreconciled rejected cancellation", maker.ID())
	}
	return nil
}

// SetDecisionObserver records every scheduled decision, including no-action
// choices. The callback is observational and must not change actor state.
func (maker *RecurringMaker) SetDecisionObserver(observer func(MakerDecision)) {
	maker.decisionObserver = observer
}

func (maker *RecurringMaker) SetObservationObserver(observer func(MakerObservation)) {
	maker.observationObserver = observer
}

func (maker *RecurringMaker) Start(ctx context.Context) error {
	if !maker.started.CompareAndSwap(false, true) {
		return fmt.Errorf("repeatedspot: recurring maker already started")
	}
	if err := maker.BaseActor.Start(ctx); err != nil {
		return err
	}
	maker.Subscribe(maker.config.Symbol, exchange.MDSnapshot, exchange.MDTrade)
	maker.subscribed = true
	return nil
}

func (maker *RecurringMaker) HandleEvent(_ context.Context, event *actor.Event) {
	if maker.fault != nil {
		return
	}
	switch event.Type {
	case actor.EventBookSnapshot:
		maker.onSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventTrade:
		maker.onTrade(event.Data.(actor.TradeEvent))
	case actor.EventOrderAccepted:
		maker.onAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		maker.fault = maker.inventory.rejected(event.Data.(actor.OrderRejectedEvent).RequestID)
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		maker.onFill(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		maker.onCancelled(event.Data.(actor.OrderCancelledEvent))
	case actor.EventOrderCancelRejected:
		maker.onCancelRejected(event.Data.(actor.OrderCancelRejectedEvent))
	}
}

func (maker *RecurringMaker) onSnapshot(event actor.BookSnapshotEvent) {
	if event.Symbol != maker.config.Symbol {
		return
	}
	if event.Timestamp < 0 || maker.bookSeen && event.Timestamp < maker.latestBookAt {
		maker.fault = fmt.Errorf("repeatedspot: delivered book source time regressed")
		return
	}
	maker.bookSeen = true
	maker.latestBookAt = event.Timestamp
	maker.latestBookSequence = event.SeqNum
	maker.latestBookProcessedAt = maker.localNow(event.Timestamp)
	maker.bestBid, maker.bestAsk = 0, 0
	if event.Snapshot != nil {
		if len(event.Snapshot.Bids) != 0 {
			maker.bestBid = event.Snapshot.Bids[0].Price
		}
		if len(event.Snapshot.Asks) != 0 {
			maker.bestAsk = event.Snapshot.Asks[0].Price
		}
	}
	if maker.observationObserver != nil {
		maker.observationObserver(MakerObservation{ActorID: maker.ID(), Kind: "snapshot",
			ProcessedAt: maker.latestBookProcessedAt, SourceAt: event.Timestamp, SourceSequence: event.SeqNum,
			Symbol: event.Symbol, BestBid: maker.bestBid, BestAsk: maker.bestAsk})
	}
}

func (maker *RecurringMaker) onTrade(event actor.TradeEvent) {
	if event.Symbol != maker.config.Symbol {
		return
	}
	if event.Trade == nil || event.Trade.Price <= 0 || event.Timestamp < 0 {
		maker.fault = fmt.Errorf("repeatedspot: malformed delivered trade")
		return
	}
	if maker.lastTradePrice != 0 && event.Timestamp < maker.lastTradeSourceTime {
		maker.fault = fmt.Errorf("repeatedspot: delivered trade source time regressed")
		return
	}
	if maker.observationObserver != nil {
		maker.observationObserver(MakerObservation{ActorID: maker.ID(), Kind: "trade",
			ProcessedAt: maker.localNow(event.Timestamp), SourceAt: event.Timestamp, SourceSequence: event.SeqNum,
			Symbol: event.Symbol, TradeID: event.Trade.TradeID, TradePrice: event.Trade.Price,
			TradeQty: event.Trade.Qty, TradeSide: event.Trade.Side.String()})
	}
	if maker.lastTradePrice != 0 && event.Timestamp-maker.lastTradeSourceTime < int64(maker.config.VolatilitySampleInterval) {
		return
	}
	if maker.lastTradePrice != 0 && event.Timestamp > maker.lastTradeSourceTime {
		deltaSeconds := float64(event.Timestamp-maker.lastTradeSourceTime) / float64(time.Second)
		logReturn := math.Log(float64(event.Trade.Price) / float64(maker.lastTradePrice))
		instantVariance := logReturn * logReturn / deltaSeconds
		if math.IsNaN(instantVariance) || math.IsInf(instantVariance, 0) {
			maker.fault = fmt.Errorf("repeatedspot: delivered trade variance is not finite")
			return
		}
		alpha := 1.0
		if maker.config.VolatilityHalfLife > 0 {
			alpha = -math.Expm1(-math.Ln2 * deltaSeconds / maker.config.VolatilityHalfLife.Seconds())
		}
		maker.logVariancePerSecond = (1-alpha)*maker.logVariancePerSecond + alpha*instantVariance
		if maker.config.MaxLogVarianceMultiple > 0 && maker.config.InitialLogVariancePerSecond > 0 {
			maximum := maker.config.InitialLogVariancePerSecond * maker.config.MaxLogVarianceMultiple
			if maker.logVariancePerSecond > maximum {
				maker.logVariancePerSecond = maximum
			}
		}
	}
	maker.lastTradePrice = event.Trade.Price
	maker.lastTradeSourceTime = event.Timestamp
	maker.tradeSamples++
}

func (maker *RecurringMaker) onAccepted(event actor.OrderAcceptedEvent) {
	order := maker.inventory.requests[event.RequestID]
	if order == nil {
		maker.fault = fmt.Errorf("repeatedspot: unexpected maker acceptance %d", event.RequestID)
		return
	}
	if err := maker.inventory.accepted(event.RequestID, event.OrderID); err != nil {
		maker.fault = err
		return
	}
	if order.side == exchange.Buy {
		maker.activeBid = event.OrderID
	} else {
		maker.activeAsk = event.OrderID
	}
}

func (maker *RecurringMaker) onFill(event actor.OrderFillEvent) {
	if event.Symbol != maker.config.Symbol {
		maker.fault = fmt.Errorf("repeatedspot: maker fill has wrong symbol %q", event.Symbol)
		return
	}
	if err := maker.inventory.filledOrder(event.OrderID, event.TradeID, event.Side, event.Qty, event.IsFull); err != nil {
		maker.fault = err
		return
	}
	if event.IsFull {
		maker.clearActive(event.OrderID)
		if _, unresolved := maker.unresolvedEnd[event.OrderID]; unresolved {
			delete(maker.unresolvedEnd, event.OrderID)
			delete(maker.cancelRequested, event.OrderID)
		}
	}
}

func (maker *RecurringMaker) onCancelled(event actor.OrderCancelledEvent) {
	if err := maker.inventory.cancelled(event.OrderID, event.RemainingQty); err != nil {
		maker.fault = err
		return
	}
	delete(maker.cancelRequested, event.OrderID)
	delete(maker.unresolvedEnd, event.OrderID)
	maker.clearActive(event.OrderID)
}

func (maker *RecurringMaker) onCancelRejected(event actor.OrderCancelRejectedEvent) {
	if _, requested := maker.cancelRequested[event.OrderID]; !requested {
		maker.fault = fmt.Errorf("repeatedspot: unexpected cancel rejection for order %d", event.OrderID)
		return
	}
	if maker.inventory.orders[event.OrderID] == nil {
		// A full fill or forced cancellation was already delivered. The
		// rejection is the normal race between execution and cancel arrival.
		delete(maker.cancelRequested, event.OrderID)
		return
	}
	switch event.Reason {
	case exchange.RejectRateLimited:
		delete(maker.cancelRequested, event.OrderID)
	case exchange.RejectOrderAlreadyFilled, exchange.RejectOrderNotFound:
		// Wait for the earlier exchange-side terminal event. It may still be
		// travelling through the response queue; do not release risk yet.
		maker.unresolvedEnd[event.OrderID] = struct{}{}
	default:
		maker.fault = fmt.Errorf("repeatedspot: cancel rejected for maker %d order %d: %s", maker.ID(), event.OrderID, event.Reason)
	}
}

func (maker *RecurringMaker) clearActive(orderID uint64) {
	if maker.activeBid == orderID {
		maker.activeBid = 0
	}
	if maker.activeAsk == orderID {
		maker.activeAsk = 0
	}
}

func (maker *RecurringMaker) onTick(scheduled time.Time) {
	action := "fault"
	var quote MakerQuote
	defer func() { maker.observeDecision(scheduled, quote, action) }()
	if maker.fault != nil {
		return
	}
	if !maker.subscribed {
		maker.Subscribe(maker.config.Symbol, exchange.MDSnapshot, exchange.MDTrade)
		maker.subscribed = true
		action = "subscribe"
		return
	}
	if maker.hasPendingPlacement() || len(maker.cancelRequested) != 0 {
		action = "await_response"
		return
	}
	var usable bool
	quote, usable = maker.targetQuote()
	if maker.activeBid != 0 || maker.activeAsk != 0 {
		if usable && maker.matchesCurrentQuote(quote) {
			action = "keep_quotes"
			return
		}
		maker.cancelActive()
		action = "cancel_quotes"
		return
	}
	if usable {
		maker.submitPair(quote)
		action = "evaluate_placements"
		if maker.fault != nil {
			action = "fault"
		}
	} else {
		action = "no_usable_quote"
	}
}

func (maker *RecurringMaker) observeDecision(scheduled time.Time, quote MakerQuote, action string) {
	if maker.decisionObserver == nil {
		return
	}
	decisionAt := maker.localNow(scheduled.UnixNano())
	lower, upper, err := maker.inventory.envelope()
	if err != nil {
		maker.fault = err
		return
	}
	maker.decisionObserver(MakerDecision{
		ActorID: maker.ID(), ScheduledAt: scheduled.UnixNano(), DecisionAt: decisionAt,
		BookSeen: maker.bookSeen, LatestBookSourceAt: maker.latestBookAt,
		LatestBookSequence: maker.latestBookSequence, LatestBookProcessedAt: maker.latestBookProcessedAt,
		BestBid: maker.bestBid, BestAsk: maker.bestAsk,
		FilledInventory: maker.inventory.filled, WorkingLower: lower, WorkingUpper: upper,
		LogVariancePerSecond: maker.logVariancePerSecond, DeliveredTradeSamples: maker.tradeSamples,
		TargetBid: quote.BidPrice, TargetAsk: quote.AskPrice, Action: action,
	})
}

func (maker *RecurringMaker) localNow(fallback int64) int64 {
	if clocked, ok := maker.Gateway().(interface{ NowUnixNano() int64 }); ok {
		return clocked.NowUnixNano()
	}
	return fallback
}

func (maker *RecurringMaker) hasPendingPlacement() bool {
	for _, order := range maker.inventory.requests {
		if order.orderID == 0 {
			return true
		}
	}
	return false
}

func (maker *RecurringMaker) targetQuote() (MakerQuote, bool) {
	if maker.bestBid <= 0 || maker.bestAsk <= maker.bestBid {
		return MakerQuote{}, false
	}
	mid := maker.bestBid + (maker.bestAsk-maker.bestBid)/2
	quote, ok := maker.quoteRule(MakerQuoteInput{MidPrice: mid, FilledInventory: maker.inventory.filled,
		WorkingLimit: maker.config.WorkingLimit, LogVariancePerSecond: maker.logVariancePerSecond,
		DeliveredTradeSamples: maker.tradeSamples})
	if !ok || quote.BidPrice < 0 || quote.AskPrice < 0 ||
		(quote.BidPrice == 0 && quote.AskPrice == 0) ||
		(quote.BidPrice > 0 && quote.AskPrice > 0 && quote.AskPrice <= quote.BidPrice) ||
		quote.BidPrice%maker.config.TickSize != 0 || quote.AskPrice%maker.config.TickSize != 0 {
		return MakerQuote{}, false
	}
	return quote, true
}

func (maker *RecurringMaker) matchesCurrentQuote(quote MakerQuote) bool {
	for _, side := range []struct {
		orderID uint64
		old     int64
		wanted  int64
		kind    exchange.Side
	}{
		{maker.activeBid, maker.quotedBid, quote.BidPrice, exchange.Buy},
		{maker.activeAsk, maker.quotedAsk, quote.AskPrice, exchange.Sell},
	} {
		targetQty := maker.desiredQuantity(side.kind)
		if side.wanted == 0 || targetQty == 0 {
			if side.orderID != 0 {
				return false
			}
			continue
		}
		if side.orderID == 0 {
			return false
		}
		order := maker.inventory.orders[side.orderID]
		if order == nil || order.remaining != targetQty {
			return false
		}
		if side.old == side.wanted {
			continue
		}
		if side.old <= 0 || side.wanted <= 0 {
			return false
		}
		difference := side.old - side.wanted
		if difference < 0 {
			difference = -difference
		}
		if priceMovedAtLeastBps(difference, side.old, maker.config.RequoteBps) {
			return false
		}
	}
	return true
}

func priceMovedAtLeastBps(difference, reference, threshold int64) bool {
	leftHigh, leftLow := bits.Mul64(uint64(difference), 10_000)
	rightHigh, rightLow := bits.Mul64(uint64(reference), uint64(threshold))
	return leftHigh > rightHigh || leftHigh == rightHigh && leftLow >= rightLow
}

func (maker *RecurringMaker) desiredQuantity(side exchange.Side) int64 {
	var capacity int64
	var ok bool
	if side == exchange.Buy {
		capacity, ok = types.TrySub(maker.config.WorkingLimit, maker.inventory.filled)
	} else {
		capacity, ok = types.TryAdd(maker.config.WorkingLimit, maker.inventory.filled)
	}
	if !ok {
		maker.fault = fmt.Errorf("repeatedspot: post-cancel maker capacity overflows")
		return 0
	}
	quantity := min(maker.config.QuoteQty, capacity)
	if quantity < maker.config.MinQuoteQty {
		return 0
	}
	return quantity
}

func (maker *RecurringMaker) cancelActive() {
	for _, orderID := range []uint64{maker.activeBid, maker.activeAsk} {
		if orderID == 0 {
			continue
		}
		maker.CancelOrder(orderID)
		maker.cancelRequested[orderID] = struct{}{}
	}
}

func (maker *RecurringMaker) submitPair(quote MakerQuote) {
	for _, side := range []struct {
		kind  exchange.Side
		price int64
	}{
		{exchange.Buy, quote.BidPrice},
		{exchange.Sell, quote.AskPrice},
	} {
		if side.price == 0 {
			continue
		}
		capacity, err := maker.inventory.available(side.kind)
		if err != nil {
			maker.fault = err
			return
		}
		quantity := min(maker.config.QuoteQty, capacity)
		if quantity < maker.config.MinQuoteQty {
			continue
		}
		requestID := maker.PeekNextRequestID()
		if err := maker.inventory.reserve(requestID, side.kind, quantity); err != nil {
			maker.fault = err
			return
		}
		actualID := maker.SubmitPostOnlyOrder(maker.config.Symbol, side.kind, side.price, quantity)
		if actualID != requestID {
			maker.fault = fmt.Errorf("repeatedspot: maker request identity changed during submission")
			return
		}
		if side.kind == exchange.Buy {
			maker.quotedBid = side.price
		} else {
			maker.quotedAsk = side.price
		}
	}
}
