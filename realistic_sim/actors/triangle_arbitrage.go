package actors

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type TriangleArbConfig struct {
	ActorID          uint64
	BaseSymbol       string
	CrossSymbol      string
	DirectSymbol     string
	BaseInstrument   exchange.Instrument
	CrossInstrument  exchange.Instrument
	DirectInstrument exchange.Instrument
	ThresholdBps     int64
	MaxTradeSize     int64
}

type TriangleArbitrage struct {
	id     uint64
	config TriangleArbConfig

	baseMid   int64
	crossMid  int64
	directMid int64

	executing bool
}

func NewTriangleArbitrage(config TriangleArbConfig) *TriangleArbitrage {
	return &TriangleArbitrage{
		id:     config.ActorID,
		config: config,
	}
}

func (ta *TriangleArbitrage) GetID() uint64 {
	return ta.id
}

func (ta *TriangleArbitrage) GetSymbols() []string {
	return []string{ta.config.BaseSymbol, ta.config.CrossSymbol, ta.config.DirectSymbol}
}

func (ta *TriangleArbitrage) OnEvent(event *actor.Event, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	switch event.Type {
	case actor.EventBookSnapshot:
		ta.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		ta.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)
	}

	if !ta.executing {
		ta.evaluateArbitrage(ctx, submit)
	}
}

func (ta *TriangleArbitrage) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	mid := snap.Snapshot.Bids[0].Price + (snap.Snapshot.Asks[0].Price-snap.Snapshot.Bids[0].Price)/2

	switch snap.Symbol {
	case ta.config.BaseSymbol:
		ta.baseMid = mid
	case ta.config.CrossSymbol:
		ta.crossMid = mid
	case ta.config.DirectSymbol:
		ta.directMid = mid
	}
}

func (ta *TriangleArbitrage) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	precision := int64(100_000_000)
	if ta.config.BaseInstrument != nil {
		precision = ta.config.BaseInstrument.BasePrecision()
	}

	symbol := ""
	baseAsset := ""

	if fill.Side == exchange.Buy {
		symbol = ta.config.BaseSymbol
		if ta.config.BaseInstrument != nil {
			baseAsset = ta.config.BaseInstrument.BaseAsset()
		}
	} else {
		symbol = ta.config.DirectSymbol
		if ta.config.DirectInstrument != nil {
			baseAsset = ta.config.DirectInstrument.BaseAsset()
		}
	}

	ctx.OnFill(ta.id, symbol, fill, precision, baseAsset)
}

func (ta *TriangleArbitrage) evaluateArbitrage(ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	if ta.baseMid == 0 || ta.crossMid == 0 || ta.directMid == 0 {
		return
	}

	impliedDirect := (ta.baseMid * ta.crossMid) / 100_000_000

	profitBps := ((impliedDirect - ta.directMid) * 10000) / ta.directMid

	takerFeeBps := int64(15)
	if profitBps > (takerFeeBps + ta.config.ThresholdBps) {
		ta.executeArbitrage(ctx, submit)
	}
}

func (ta *TriangleArbitrage) executeArbitrage(ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	ta.executing = true

	submit(ta.config.BaseSymbol, exchange.Buy, exchange.Market, 0, ta.config.MaxTradeSize)
	submit(ta.config.CrossSymbol, exchange.Buy, exchange.Market, 0, ta.config.MaxTradeSize)
	submit(ta.config.DirectSymbol, exchange.Sell, exchange.Market, 0, ta.config.MaxTradeSize)

	ta.executing = false
}
