package feesim

import (
	"context"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type MidPriceMode int

const (
	MidFromLastTrade   MidPriceMode = iota // mid = latest trade price (any participant)
	MidFromBookMid                         // mid = (bestBid + bestAsk) / 2
	MidFromWeightedMid                     // microprice: bestBid*askQty + bestAsk*bidQty / totalQty
)

type MMConfig struct {
	Symbol         string
	BootstrapPrice int64
	Levels         int
	LevelSpacing   int64
	LevelSize      int64
	TickSize       int64

	// Legacy mode: uniform cancel-all + re-quote-all (active when BaseInterval == 0).
	RefreshInterval time.Duration

	// Adaptive mode: per-level refresh with configurable mid (active when BaseInterval > 0).
	MidPriceMode MidPriceMode
	BaseInterval time.Duration // fastest level refresh (near mid)
	MaxInterval  time.Duration // slowest level refresh (outermost)
}

func (c *MMConfig) isAdaptive() bool { return c.BaseInterval > 0 }

// --- Per-level state for adaptive mode ---

type levelState struct {
	bidOrderID uint64
	askOrderID uint64
	bidPrice   int64 // current resting bid price (0 = no order)
	askPrice   int64 // current resting ask price (0 = no order)
	tickDiv    int   // refresh every N base ticks
}

type inflightOrder struct {
	level int
	isBid bool
}

// --- MarketMaker ---

type MarketMaker struct {
	*actor.BaseActor
	cfg MMConfig
	mid int64

	// Legacy mode state.
	pending  map[uint64]bool
	reqToSym map[uint64]bool

	// Adaptive mode state.
	levels    []levelState
	inflight  map[uint64]inflightOrder
	tickCount int
	lastTrade int64
	bestBid   int64
	bestAsk   int64
	bidTopQty int64
	askTopQty int64

	subscribed bool
}

func NewMarketMaker(id uint64, gw actor.Gateway, cfg MMConfig) *MarketMaker {
	mm := &MarketMaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		mid:       cfg.BootstrapPrice,
	}
	mm.SetHandler(mm)

	if cfg.isAdaptive() {
		mm.levels = make([]levelState, cfg.Levels)
		mm.inflight = make(map[uint64]inflightOrder)
		mm.precomputeTickDivs()
		mm.AddTicker(cfg.BaseInterval, mm.onBaseTick)
	} else {
		mm.pending = make(map[uint64]bool)
		mm.reqToSym = make(map[uint64]bool)
		mm.AddTicker(cfg.RefreshInterval, mm.onTick)
	}
	return mm
}

func (mm *MarketMaker) Mid() int64     { return mm.mid }
func (mm *MarketMaker) Symbol() string { return mm.cfg.Symbol }

// ============================================================
// Event routing
// ============================================================

func (mm *MarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if mm.cfg.isAdaptive() {
		mm.handleAdaptive(evt)
		return
	}
	mm.handleLegacy(evt)
}

// ============================================================
// Legacy mode (unchanged from original)
// ============================================================

func (mm *MarketMaker) handleLegacy(evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		mm.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		mm.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (mm *MarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	if !mm.reqToSym[e.RequestID] {
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.reqToSym, e.RequestID)
	mm.pending[e.OrderID] = true
}

func (mm *MarketMaker) onFilled(e actor.OrderFillEvent) {
	if !mm.pending[e.OrderID] {
		return
	}
	mm.mid = e.Price
	if e.IsFull {
		delete(mm.pending, e.OrderID)
	}
}

func (mm *MarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	delete(mm.pending, e.OrderID)
}

func (mm *MarketMaker) onTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(mm.cfg.Symbol, exchange.MDSnapshot)
		mm.subscribed = true
	}
	mm.cancelAll()
	mm.quote()
}

func (mm *MarketMaker) cancelAll() {
	for orderID := range mm.pending {
		mm.CancelOrder(orderID)
	}
	mm.pending = make(map[uint64]bool)
	mm.reqToSym = make(map[uint64]bool)
}

func (mm *MarketMaker) quote() {
	mid := mm.mid
	for k := int64(1); k <= int64(mm.cfg.Levels); k++ {
		offset := (1 + (k-1)*mm.cfg.LevelSpacing) * mm.cfg.TickSize
		bidPrice := mid - offset
		askPrice := mid + offset
		if bidPrice <= 0 {
			continue
		}
		bidReqID := mm.SubmitOrder(mm.cfg.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, mm.cfg.LevelSize)
		mm.reqToSym[bidReqID] = true
		askReqID := mm.SubmitOrder(mm.cfg.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, mm.cfg.LevelSize)
		mm.reqToSym[askReqID] = true
	}
}

// ============================================================
// Adaptive mode
// ============================================================

func (mm *MarketMaker) handleAdaptive(evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		mm.onAcceptedAdaptive(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		mm.onFilledAdaptive(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelledAdaptive(evt.Data.(actor.OrderCancelledEvent))
	case actor.EventBookSnapshot:
		mm.onBookSnapshot(evt.Data.(actor.BookSnapshotEvent))
	case actor.EventTrade:
		mm.onTradeEvent(evt.Data.(actor.TradeEvent))
	}
}

func (mm *MarketMaker) onAcceptedAdaptive(e actor.OrderAcceptedEvent) {
	info, ok := mm.inflight[e.RequestID]
	if !ok {
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.inflight, e.RequestID)
	lv := &mm.levels[info.level]
	if info.isBid {
		lv.bidOrderID = e.OrderID
	} else {
		lv.askOrderID = e.OrderID
	}
}

func (mm *MarketMaker) onFilledAdaptive(e actor.OrderFillEvent) {
	if !e.IsFull {
		return
	}
	for i := range mm.levels {
		lv := &mm.levels[i]
		if lv.bidOrderID == e.OrderID {
			lv.bidOrderID = 0
			lv.bidPrice = 0
			return
		}
		if lv.askOrderID == e.OrderID {
			lv.askOrderID = 0
			lv.askPrice = 0
			return
		}
	}
}

func (mm *MarketMaker) onCancelledAdaptive(e actor.OrderCancelledEvent) {
	for i := range mm.levels {
		lv := &mm.levels[i]
		if lv.bidOrderID == e.OrderID {
			lv.bidOrderID = 0
			lv.bidPrice = 0
			return
		}
		if lv.askOrderID == e.OrderID {
			lv.askOrderID = 0
			lv.askPrice = 0
			return
		}
	}
}

func (mm *MarketMaker) onBookSnapshot(e actor.BookSnapshotEvent) {
	if e.Symbol != mm.cfg.Symbol {
		return
	}
	snap := e.Snapshot
	if len(snap.Bids) > 0 {
		mm.bestBid = snap.Bids[0].Price
		mm.bidTopQty = snap.Bids[0].VisibleQty
	} else {
		mm.bestBid = 0
		mm.bidTopQty = 0
	}
	if len(snap.Asks) > 0 {
		mm.bestAsk = snap.Asks[0].Price
		mm.askTopQty = snap.Asks[0].VisibleQty
	} else {
		mm.bestAsk = 0
		mm.askTopQty = 0
	}
}

func (mm *MarketMaker) onTradeEvent(e actor.TradeEvent) {
	if e.Symbol != mm.cfg.Symbol {
		return
	}
	mm.lastTrade = e.Trade.Price
}

// --- Adaptive tick ---

func (mm *MarketMaker) onBaseTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(mm.cfg.Symbol, exchange.MDSnapshot, exchange.MDTrade)
		mm.subscribed = true
	}

	mm.mid = mm.computeMid()

	for k := 0; k < mm.cfg.Levels; k++ {
		lv := &mm.levels[k]
		if mm.tickCount%lv.tickDiv != k%lv.tickDiv {
			continue
		}
		mm.refreshLevel(k)
	}
	mm.tickCount++
}

// --- Mid price computation ---

func (mm *MarketMaker) computeMid() int64 {
	tick := mm.cfg.TickSize

	switch mm.cfg.MidPriceMode {
	case MidFromLastTrade:
		if mm.lastTrade > 0 {
			return alignToTick(mm.lastTrade, tick)
		}
		return mm.mid

	case MidFromBookMid:
		return mm.computeBookMid(tick)

	case MidFromWeightedMid:
		return mm.computeWeightedMid(tick)
	}
	return mm.mid
}

func (mm *MarketMaker) computeBookMid(tick int64) int64 {
	hasBid := mm.bestBid > 0
	hasAsk := mm.bestAsk > 0

	switch {
	case hasBid && hasAsk:
		return alignToTick((mm.bestBid+mm.bestAsk)/2, tick)
	case hasAsk:
		return mm.bestAsk
	case hasBid:
		return mm.bestBid
	default:
		return mm.mid
	}
}

func (mm *MarketMaker) computeWeightedMid(tick int64) int64 {
	hasBid := mm.bestBid > 0 && mm.bidTopQty > 0
	hasAsk := mm.bestAsk > 0 && mm.askTopQty > 0

	switch {
	case hasBid && hasAsk:
		// Microprice: bestBid * askWeight + bestAsk * bidWeight.
		// Use normalized weights to avoid int64 overflow (price × qty can exceed 2^63).
		totalQty := mm.bidTopQty + mm.askTopQty
		askWeight := mm.askTopQty * 10000 / totalQty // basis points
		bidWeight := 10000 - askWeight
		raw := (mm.bestBid*askWeight + mm.bestAsk*bidWeight) / 10000
		return alignToTick(raw, tick)
	case hasAsk:
		return mm.bestAsk
	case hasBid:
		return mm.bestBid
	default:
		return mm.mid
	}
}

func alignToTick(price, tick int64) int64 {
	return ((price + tick/2) / tick) * tick
}

// --- Level management ---

func (mm *MarketMaker) precomputeTickDivs() {
	n := mm.cfg.Levels
	baseNs := mm.cfg.BaseInterval.Nanoseconds()
	maxNs := mm.cfg.MaxInterval.Nanoseconds()

	for k := 0; k < n; k++ {
		if n == 1 {
			mm.levels[k].tickDiv = 1
			continue
		}
		intervalNs := baseNs + (maxNs-baseNs)*int64(k)/int64(n-1)
		div := int(math.Round(float64(intervalNs) / float64(baseNs)))
		if div < 1 {
			div = 1
		}
		mm.levels[k].tickDiv = div
	}
}

func (mm *MarketMaker) refreshLevel(k int) {
	lv := &mm.levels[k]

	// Don't refresh while orders from the previous refresh are still in-flight.
	// bidPrice != 0 means we placed a bid but bidOrderID == 0 means accept hasn't arrived.
	if lv.bidPrice != 0 && lv.bidOrderID == 0 {
		return
	}
	if lv.askPrice != 0 && lv.askOrderID == 0 {
		return
	}

	offset := (1 + int64(k)*mm.cfg.LevelSpacing) * mm.cfg.TickSize
	desiredBid := mm.mid - offset
	desiredAsk := mm.mid + offset

	bidUnchanged := desiredBid == lv.bidPrice && lv.bidOrderID != 0
	askUnchanged := desiredAsk == lv.askPrice && lv.askOrderID != 0
	if bidUnchanged && askUnchanged {
		return
	}

	mm.cancelLevelOrders(lv)
	mm.clearInflightForLevel(k)

	if desiredBid > 0 {
		reqID := mm.SubmitOrder(mm.cfg.Symbol, exchange.Buy, exchange.LimitOrder, desiredBid, mm.cfg.LevelSize)
		mm.inflight[reqID] = inflightOrder{level: k, isBid: true}
		lv.bidPrice = desiredBid
	}

	reqID := mm.SubmitOrder(mm.cfg.Symbol, exchange.Sell, exchange.LimitOrder, desiredAsk, mm.cfg.LevelSize)
	mm.inflight[reqID] = inflightOrder{level: k, isBid: false}
	lv.askPrice = desiredAsk
}

func (mm *MarketMaker) cancelLevelOrders(lv *levelState) {
	if lv.bidOrderID != 0 {
		mm.CancelOrder(lv.bidOrderID)
		lv.bidOrderID = 0
	}
	if lv.askOrderID != 0 {
		mm.CancelOrder(lv.askOrderID)
		lv.askOrderID = 0
	}
}

func (mm *MarketMaker) clearInflightForLevel(k int) {
	for reqID, info := range mm.inflight {
		if info.level == k {
			delete(mm.inflight, reqID)
		}
	}
}
