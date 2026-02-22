package actors

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type PureMMSubActorConfig struct {
	SpreadBps        int64
	QuoteSize        int64
	MaxInventory     int64
	RequoteThreshold int64
	Precision        int64
	BootstrapPrice   int64
	// EMAAlpha is the weight (0–100) applied to each fill price when updating
	// the EMA mid. Higher values = more aggressive price impact per fill.
	// Defaults to 20 if zero.
	EMAAlpha int64
}

type PureMMSubActor struct {
	id           uint64
	symbol       string
	config       PureMMSubActorConfig
	lastMid      int64 // mid used at last requote, for threshold comparison
	emaMid       int64 // EMA of fill prices — drives quoted mid
	cancelFn     func(uint64)
	activeBidID  uint64
	activeAskID  uint64
	lastBidReqID uint64
	lastAskReqID uint64
	hasActiveBid bool
	hasActiveAsk bool
	oms          *actor.NettingOMS
}

func NewPureMMSubActor(id uint64, symbol string, config PureMMSubActorConfig) *PureMMSubActor {
	return &PureMMSubActor{
		id:     id,
		symbol: symbol,
		config: config,
		oms:    actor.NewNettingOMS(),
	}
}

// SetCancelFn wires a cancel callback so the MM can cancel stale quotes before
// repricing. Must be called after the CompositeActor is created.
func (pmm *PureMMSubActor) SetCancelFn(fn func(uint64)) {
	pmm.cancelFn = fn
}

func (pmm *PureMMSubActor) GetID() uint64 {
	return pmm.id
}

func (pmm *PureMMSubActor) GetSymbols() []string {
	return []string{pmm.symbol}
}

func (pmm *PureMMSubActor) OnEvent(event *actor.Event, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	switch event.Type {
	case actor.EventBookSnapshot:
		pmm.onBookSnapshot(event.Data.(actor.BookSnapshotEvent), ctx, submit)

	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		pmm.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx, submit)

	case actor.EventOrderAccepted:
		pmm.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))

	case actor.EventOrderCancelled:
		pmm.onOrderCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (pmm *PureMMSubActor) onBookSnapshot(snap actor.BookSnapshotEvent, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	if snap.Symbol != pmm.symbol {
		return
	}

	var mid int64
	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		if pmm.config.BootstrapPrice > 0 {
			mid = pmm.config.BootstrapPrice
		} else {
			return
		}
	} else {
		mid = snap.Snapshot.Bids[0].Price + (snap.Snapshot.Asks[0].Price-snap.Snapshot.Bids[0].Price)/2
	}

	// Seed EMA from book mid on first observation.
	if pmm.emaMid == 0 {
		pmm.emaMid = mid
	}

	if pmm.lastMid == 0 {
		pmm.lastMid = pmm.emaMid
		pmm.requote(ctx, submit, pmm.emaMid)
		return
	}

	bothDepleted := !pmm.hasActiveBid && !pmm.hasActiveAsk
	if abs(pmm.emaMid-pmm.lastMid) >= pmm.config.RequoteThreshold || bothDepleted {
		pmm.cancelAndRequote(ctx, submit, pmm.emaMid)
	}
}

func (pmm *PureMMSubActor) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	pmm.oms.OnFill(pmm.symbol, fill, pmm.config.Precision)

	baseAsset := extractBaseAsset(pmm.symbol)
	ctx.OnFill(pmm.id, pmm.symbol, fill, pmm.config.Precision, baseAsset)

	// EMA update: shift quoted mid toward fill price on every trade.
	alpha := pmm.config.EMAAlpha
	if alpha <= 0 {
		alpha = 20
	}
	if pmm.emaMid == 0 {
		pmm.emaMid = fill.Price
	} else {
		pmm.emaMid = pmm.emaMid + (fill.Price-pmm.emaMid)*alpha/100
	}

	// Cancel both sides and reprice immediately at the EMA-shifted mid.
	// This is the core price-impact mechanism: every fill moves the book.
	if pmm.lastMid > 0 {
		pmm.cancelAndRequote(ctx, submit, pmm.emaMid)
	}
}

// cancelAndRequote cancels any live quotes and submits fresh ones at newMid.
// Zeroing the active order IDs before sending the cancel request ensures that
// cancel confirmations arriving later do not incorrectly clear the flags for
// the newly placed orders.
func (pmm *PureMMSubActor) cancelAndRequote(ctx *actor.SharedContext, submit actor.OrderSubmitter, newMid int64) {
	if pmm.hasActiveBid {
		if pmm.cancelFn != nil {
			pmm.cancelFn(pmm.activeBidID)
		}
		pmm.activeBidID = 0
		pmm.hasActiveBid = false
	}
	if pmm.hasActiveAsk {
		if pmm.cancelFn != nil {
			pmm.cancelFn(pmm.activeAskID)
		}
		pmm.activeAskID = 0
		pmm.hasActiveAsk = false
	}
	pmm.lastMid = newMid
	pmm.requote(ctx, submit, newMid)
}

func (pmm *PureMMSubActor) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	if accepted.RequestID == pmm.lastBidReqID {
		pmm.activeBidID = accepted.OrderID
	} else if accepted.RequestID == pmm.lastAskReqID {
		pmm.activeAskID = accepted.OrderID
	}
}

func (pmm *PureMMSubActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	// Only clear flags if the cancel matches the currently tracked order.
	// Cancels for old (zeroed) IDs are intentionally ignored here.
	if cancelled.OrderID != 0 && cancelled.OrderID == pmm.activeBidID {
		pmm.hasActiveBid = false
	}
	if cancelled.OrderID != 0 && cancelled.OrderID == pmm.activeAskID {
		pmm.hasActiveAsk = false
	}
}

func (pmm *PureMMSubActor) requote(ctx *actor.SharedContext, submit actor.OrderSubmitter, mid int64) {
	currentPos := pmm.oms.GetNetPosition(pmm.symbol)

	halfSpread := (mid * pmm.config.SpreadBps) / 20000
	if halfSpread == 0 {
		halfSpread = 1
	}

	inventorySkewBps := (currentPos * 100) / pmm.config.MaxInventory
	skew := (mid * inventorySkewBps) / 10000

	if skew > halfSpread {
		skew = halfSpread
	} else if skew < -halfSpread {
		skew = -halfSpread
	}

	bidPrice := mid - halfSpread - skew
	askPrice := mid + halfSpread - skew

	if bidPrice <= 0 {
		bidPrice = 1
	}
	if askPrice <= 0 {
		askPrice = 2
	}

	if currentPos+pmm.config.QuoteSize <= pmm.config.MaxInventory && !pmm.hasActiveBid {
		if ctx.CanSubmitOrder(pmm.id, pmm.symbol, exchange.Buy, pmm.config.QuoteSize, pmm.config.MaxInventory) {
			pmm.lastBidReqID = submit(pmm.symbol, exchange.Buy, exchange.LimitOrder, bidPrice, pmm.config.QuoteSize)
			pmm.hasActiveBid = true
		}
	}

	if currentPos-pmm.config.QuoteSize >= -pmm.config.MaxInventory && !pmm.hasActiveAsk {
		if ctx.CanSubmitOrder(pmm.id, pmm.symbol, exchange.Sell, pmm.config.QuoteSize, pmm.config.MaxInventory) {
			pmm.lastAskReqID = submit(pmm.symbol, exchange.Sell, exchange.LimitOrder, askPrice, pmm.config.QuoteSize)
			pmm.hasActiveAsk = true
		}
	}
}

func extractBaseAsset(symbol string) string {
	for i := 0; i < len(symbol); i++ {
		if symbol[i] == '/' || symbol[i] == '-' {
			return symbol[:i]
		}
	}
	return symbol
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
