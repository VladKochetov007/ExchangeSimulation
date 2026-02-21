package actors

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type InternalFundingArbConfig struct {
	ActorID         uint64
	SpotSymbol      string
	PerpSymbol      string
	SpotInstrument  exchange.Instrument
	PerpInstrument  exchange.Instrument
	MinFundingRate  int64
	ExitFundingRate int64
	MaxPositionSize int64
}

type InternalFundingArb struct {
	id        uint64
	config    InternalFundingArbConfig
	spotOMS   *actor.NettingOMS
	perpOMS   *actor.NettingOMS
	baseAsset string

	spotMid         int64
	perpMid         int64
	fundingRate     int64
	lastNextFunding int64
	isActive        bool
	pendingExit     bool // exit orders submitted, waiting for fills

	pendingRequests map[uint64]string // requestID → symbol
	orderSymbols    map[uint64]string // orderID → symbol
}

func NewInternalFundingArb(config InternalFundingArbConfig) *InternalFundingArb {
	baseAsset := ""
	if config.SpotInstrument != nil {
		baseAsset = config.SpotInstrument.BaseAsset()
	}

	return &InternalFundingArb{
		id:              config.ActorID,
		config:          config,
		spotOMS:         actor.NewNettingOMS(),
		perpOMS:         actor.NewNettingOMS(),
		baseAsset:       baseAsset,
		pendingRequests: make(map[uint64]string),
		orderSymbols:    make(map[uint64]string),
	}
}

func (ifa *InternalFundingArb) GetID() uint64 {
	return ifa.id
}

func (ifa *InternalFundingArb) GetSymbols() []string {
	return []string{ifa.config.SpotSymbol, ifa.config.PerpSymbol}
}

func (ifa *InternalFundingArb) OnEvent(event *actor.Event, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	switch event.Type {
	case actor.EventBookSnapshot:
		ifa.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		ifa.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		ifa.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)
	case actor.EventFundingUpdate:
		ifa.onFundingUpdate(event.Data.(actor.FundingUpdateEvent), ctx)
	case actor.EventOrderRejected:
		ifa.onOrderRejected(event.Data.(actor.OrderRejectedEvent))
	}

	ifa.evaluateStrategy(ctx, submit)
}

func (ifa *InternalFundingArb) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	mid := snap.Snapshot.Bids[0].Price + (snap.Snapshot.Asks[0].Price-snap.Snapshot.Bids[0].Price)/2

	if snap.Symbol == ifa.config.SpotSymbol {
		ifa.spotMid = mid
	} else if snap.Symbol == ifa.config.PerpSymbol {
		ifa.perpMid = mid
	}
}

func (ifa *InternalFundingArb) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	if symbol, ok := ifa.pendingRequests[accepted.RequestID]; ok {
		ifa.orderSymbols[accepted.OrderID] = symbol
		delete(ifa.pendingRequests, accepted.RequestID)
	}
}

func (ifa *InternalFundingArb) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	precision := int64(100_000_000)
	if ifa.config.SpotInstrument != nil {
		precision = ifa.config.SpotInstrument.BasePrecision()
	}

	symbol, ok := ifa.orderSymbols[fill.OrderID]
	if !ok {
		// Fallback: entry is always spot=Buy, perp=Sell
		if fill.Side == exchange.Buy {
			symbol = ifa.config.SpotSymbol
		} else {
			symbol = ifa.config.PerpSymbol
		}
	}
	if fill.IsFull {
		delete(ifa.orderSymbols, fill.OrderID)
	}

	if symbol == ifa.config.SpotSymbol {
		ifa.spotOMS.OnFill(ifa.config.SpotSymbol, fill, precision)
		ctx.OnFill(ifa.id, ifa.config.SpotSymbol, fill, precision, ifa.baseAsset)
	} else {
		ifa.perpOMS.OnFill(ifa.config.PerpSymbol, fill, precision)
		ctx.OnFill(ifa.id, ifa.config.PerpSymbol, fill, precision, ifa.baseAsset)
	}
}

func (ifa *InternalFundingArb) onFundingUpdate(update actor.FundingUpdateEvent, ctx *actor.SharedContext) {
	if update.Symbol != ifa.config.PerpSymbol {
		return
	}
	if ifa.lastNextFunding != 0 && update.FundingRate.NextFunding > ifa.lastNextFunding && ifa.isActive {
		precision := int64(100_000_000)
		if ifa.config.PerpInstrument != nil {
			precision = ifa.config.PerpInstrument.BasePrecision()
		}
		perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)
		if perpPos != 0 {
			absPos := perpPos
			if absPos < 0 {
				absPos = -absPos
			}
			entryPrice := ifa.perpOMS.GetPosition(ifa.config.PerpSymbol).AvgPrice
			fundingAmount := absPos * entryPrice / precision * ifa.fundingRate / 10000
			if perpPos < 0 {
				ctx.UpdateBalances("", 0, fundingAmount)
			} else {
				ctx.UpdateBalances("", 0, -fundingAmount)
			}
		}
	}
	ifa.fundingRate = update.FundingRate.Rate
	ifa.lastNextFunding = update.FundingRate.NextFunding
}

func (ifa *InternalFundingArb) onOrderRejected(rejected actor.OrderRejectedEvent) {
	if _, ok := ifa.pendingRequests[rejected.RequestID]; !ok {
		return
	}
	delete(ifa.pendingRequests, rejected.RequestID)
	if ifa.pendingExit {
		ifa.pendingExit = false // allow exit retry
	} else {
		ifa.isActive = false // entry rejected: allow re-entry
	}
}

func (ifa *InternalFundingArb) evaluateStrategy(ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	if ifa.spotMid == 0 || ifa.perpMid == 0 {
		return
	}

	if !ifa.isActive {
		if ifa.fundingRate >= ifa.config.MinFundingRate {
			ifa.enterPosition(ctx, submit)
		}
	} else {
		if ifa.pendingExit {
			// Check if exit fills have arrived (OMS shows flat position).
			spotFlat := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol) == 0
			perpFlat := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol) == 0
			if spotFlat && perpFlat {
				ifa.isActive = false
				ifa.pendingExit = false
			}
		} else if ifa.fundingRate < ifa.config.ExitFundingRate {
			ifa.exitPosition(ctx, submit)
		}
	}
}

func (ifa *InternalFundingArb) enterPosition(ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	spotPos := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol)
	perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)

	if spotPos != 0 || perpPos != 0 {
		return
	}

	if !ctx.CanSubmitOrder(ifa.id, ifa.config.SpotSymbol, exchange.Buy, ifa.config.MaxPositionSize, ifa.config.MaxPositionSize) {
		return
	}
	if !ctx.CanSubmitOrder(ifa.id, ifa.config.PerpSymbol, exchange.Sell, ifa.config.MaxPositionSize, ifa.config.MaxPositionSize) {
		return
	}

	spotReqID := submit(ifa.config.SpotSymbol, exchange.Buy, exchange.Market, 0, ifa.config.MaxPositionSize)
	perpReqID := submit(ifa.config.PerpSymbol, exchange.Sell, exchange.Market, 0, ifa.config.MaxPositionSize)
	ifa.pendingRequests[spotReqID] = ifa.config.SpotSymbol
	ifa.pendingRequests[perpReqID] = ifa.config.PerpSymbol

	ifa.isActive = true
}

func (ifa *InternalFundingArb) exitPosition(_ *actor.SharedContext, submit actor.OrderSubmitter) {
	spotPos := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol)
	perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)

	if spotPos > 0 {
		reqID := submit(ifa.config.SpotSymbol, exchange.Sell, exchange.Market, 0, spotPos)
		ifa.pendingRequests[reqID] = ifa.config.SpotSymbol
	}

	if perpPos < 0 {
		reqID := submit(ifa.config.PerpSymbol, exchange.Buy, exchange.Market, 0, -perpPos)
		ifa.pendingRequests[reqID] = ifa.config.PerpSymbol
	}

	// isActive stays true; evaluateStrategy clears it once OMS confirms flat.
	ifa.pendingExit = true
}
