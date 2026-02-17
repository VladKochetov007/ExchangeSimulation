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
}

type PureMMSubActor struct {
	id              uint64
	symbol          string
	config          PureMMSubActorConfig
	lastMid         int64
	activeBidID     uint64
	activeAskID     uint64
	hasActiveBid    bool
	hasActiveAsk    bool
	oms             *actor.NettingOMS
}

func NewPureMMSubActor(id uint64, symbol string, config PureMMSubActorConfig) *PureMMSubActor {
	return &PureMMSubActor{
		id:     id,
		symbol: symbol,
		config: config,
		oms:    actor.NewNettingOMS(),
	}
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
		pmm.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)

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

	if pmm.lastMid == 0 {
		pmm.lastMid = mid
		pmm.requote(ctx, submit, mid)
		return
	}

	if abs(mid-pmm.lastMid) >= pmm.config.RequoteThreshold {
		pmm.lastMid = mid
		pmm.hasActiveBid = false
		pmm.hasActiveAsk = false
		pmm.requote(ctx, submit, mid)
	}
}

func (pmm *PureMMSubActor) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	pmm.oms.OnFill(pmm.symbol, fill, pmm.config.Precision)

	baseAsset := extractBaseAsset(pmm.symbol)
	ctx.OnFill(pmm.id, pmm.symbol, fill, pmm.config.Precision, baseAsset)

	if fill.OrderID == pmm.activeBidID {
		pmm.hasActiveBid = false
	}
	if fill.OrderID == pmm.activeAskID {
		pmm.hasActiveAsk = false
	}
}


func (pmm *PureMMSubActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	if cancelled.OrderID == pmm.activeBidID {
		pmm.hasActiveBid = false
	}
	if cancelled.OrderID == pmm.activeAskID {
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
			pmm.activeBidID = submit(pmm.symbol, exchange.Buy, exchange.LimitOrder, bidPrice, pmm.config.QuoteSize)
			pmm.hasActiveBid = true
		}
	}

	if currentPos-pmm.config.QuoteSize >= -pmm.config.MaxInventory && !pmm.hasActiveAsk {
		if ctx.CanSubmitOrder(pmm.id, pmm.symbol, exchange.Sell, pmm.config.QuoteSize, pmm.config.MaxInventory) {
			pmm.activeAskID = submit(pmm.symbol, exchange.Sell, exchange.LimitOrder, askPrice, pmm.config.QuoteSize)
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
