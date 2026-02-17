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

	spotMid     int64
	perpMid     int64
	fundingRate int64
	isActive    bool
}

func NewInternalFundingArb(config InternalFundingArbConfig) *InternalFundingArb {
	baseAsset := ""
	if config.SpotInstrument != nil {
		baseAsset = config.SpotInstrument.BaseAsset()
	}

	return &InternalFundingArb{
		id:        config.ActorID,
		config:    config,
		spotOMS:   actor.NewNettingOMS(),
		perpOMS:   actor.NewNettingOMS(),
		baseAsset: baseAsset,
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
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		ifa.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)
	case actor.EventFundingUpdate:
		ifa.onFundingUpdate(event.Data.(actor.FundingUpdateEvent))
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

func (ifa *InternalFundingArb) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	precision := int64(100_000_000)
	if ifa.config.SpotInstrument != nil {
		precision = ifa.config.SpotInstrument.BasePrecision()
	}

	if fill.Side == exchange.Buy {
		ifa.spotOMS.OnFill(ifa.config.SpotSymbol, fill, precision)
		ctx.OnFill(ifa.id, ifa.config.SpotSymbol, fill, precision, ifa.baseAsset)
	} else {
		ifa.perpOMS.OnFill(ifa.config.PerpSymbol, fill, precision)
		ctx.OnFill(ifa.id, ifa.config.PerpSymbol, fill, precision, ifa.baseAsset)
	}
}

func (ifa *InternalFundingArb) onFundingUpdate(update actor.FundingUpdateEvent) {
	if update.Symbol == ifa.config.PerpSymbol {
		ifa.fundingRate = update.FundingRate.Rate
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
		if ifa.fundingRate < ifa.config.ExitFundingRate {
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

	submit(ifa.config.SpotSymbol, exchange.Buy, exchange.Market, 0, ifa.config.MaxPositionSize)
	submit(ifa.config.PerpSymbol, exchange.Sell, exchange.Market, 0, ifa.config.MaxPositionSize)

	ifa.isActive = true
}

func (ifa *InternalFundingArb) exitPosition(ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	spotPos := ifa.spotOMS.GetNetPosition(ifa.config.SpotSymbol)
	perpPos := ifa.perpOMS.GetNetPosition(ifa.config.PerpSymbol)

	if spotPos > 0 {
		submit(ifa.config.SpotSymbol, exchange.Sell, exchange.Market, 0, spotPos)
	}

	if perpPos < 0 {
		submit(ifa.config.PerpSymbol, exchange.Buy, exchange.Market, 0, -perpPos)
	}

	ifa.isActive = false
}
