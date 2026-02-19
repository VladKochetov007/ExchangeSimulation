package actors

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type TriangleArbConfig struct {
	ActorID          uint64
	BaseSymbol       string // e.g. BCD/USD: sell (fwd) or buy (rev) to close circuit
	CrossSymbol      string // e.g. BCD/ABC: buy (fwd) or sell (rev) BCD with/for ABC
	DirectSymbol     string // e.g. ABC/USD: buy (fwd) or sell (rev) ABC with USD
	BaseInstrument   exchange.Instrument
	CrossInstrument  exchange.Instrument
	DirectInstrument exchange.Instrument
	ThresholdBps     int64
	MaxTradeSize     int64
	TakerFeeBps      int64 // total fee across all 3 legs (e.g. 3 = 1bps × 3)
}

type TriangleArbitrage struct {
	id     uint64
	config TriangleArbConfig

	baseBid  int64
	baseAsk  int64
	crossBid int64
	crossAsk int64
	directBid int64
	directAsk int64

	directReqID uint64
	crossReqID  uint64
	baseReqID   uint64

	directOrderID uint64
	crossOrderID  uint64
	baseOrderID   uint64

	executing    bool
	pendingFills int
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
	case actor.EventOrderAccepted:
		ta.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		ta.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)
	case actor.EventOrderRejected:
		ta.onOrderRejected(event.Data.(actor.OrderRejectedEvent))
	}

	if !ta.executing {
		ta.evaluateArbitrage(ctx, submit)
	}
}

func (ta *TriangleArbitrage) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}
	bid := snap.Snapshot.Bids[0].Price
	ask := snap.Snapshot.Asks[0].Price
	switch snap.Symbol {
	case ta.config.BaseSymbol:
		ta.baseBid, ta.baseAsk = bid, ask
	case ta.config.CrossSymbol:
		ta.crossBid, ta.crossAsk = bid, ask
	case ta.config.DirectSymbol:
		ta.directBid, ta.directAsk = bid, ask
	}
}

func (ta *TriangleArbitrage) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	switch accepted.RequestID {
	case ta.directReqID:
		ta.directOrderID = accepted.OrderID
	case ta.crossReqID:
		ta.crossOrderID = accepted.OrderID
	case ta.baseReqID:
		ta.baseOrderID = accepted.OrderID
	}
}

func (ta *TriangleArbitrage) onOrderRejected(rejected actor.OrderRejectedEvent) {
	// Any rejection aborts the in-flight round; reset so evaluateArbitrage can retry.
	switch rejected.RequestID {
	case ta.directReqID, ta.crossReqID, ta.baseReqID:
		ta.executing = false
		ta.pendingFills = 0
		ta.directOrderID = 0
		ta.crossOrderID = 0
		ta.baseOrderID = 0
	}
}

func (ta *TriangleArbitrage) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	switch fill.OrderID {
	case ta.directOrderID:
		if ta.config.DirectInstrument != nil {
			ctx.OnFill(ta.id, ta.config.DirectSymbol, fill,
				ta.config.DirectInstrument.BasePrecision(),
				ta.config.DirectInstrument.BaseAsset())
		}
	case ta.crossOrderID:
		if ta.config.CrossInstrument != nil {
			precision := ta.config.CrossInstrument.BasePrecision()
			notional := (fill.Qty * fill.Price) / precision
			base := ta.config.CrossInstrument.BaseAsset()
			quote := ta.config.CrossInstrument.QuoteAsset()
			if fill.Side == exchange.Buy {
				ctx.UpdateBalances(base, fill.Qty, 0)
				ctx.UpdateBalances(quote, -notional, 0)
			} else {
				ctx.UpdateBalances(base, -fill.Qty, 0)
				ctx.UpdateBalances(quote, notional, 0)
			}
		}
	case ta.baseOrderID:
		if ta.config.BaseInstrument != nil {
			ctx.OnFill(ta.id, ta.config.BaseSymbol, fill,
				ta.config.BaseInstrument.BasePrecision(),
				ta.config.BaseInstrument.BaseAsset())
		}
	}

	if fill.IsFull {
		switch fill.OrderID {
		case ta.directOrderID, ta.crossOrderID, ta.baseOrderID:
			ta.pendingFills--
			if ta.pendingFills <= 0 {
				ta.executing = false
				ta.pendingFills = 0
				ta.directOrderID = 0
				ta.crossOrderID = 0
				ta.baseOrderID = 0
			}
		}
	}
}

func (ta *TriangleArbitrage) evaluateArbitrage(_ *actor.SharedContext, submit actor.OrderSubmitter) {
	if ta.baseBid == 0 || ta.baseAsk == 0 ||
		ta.crossBid == 0 || ta.crossAsk == 0 ||
		ta.directBid == 0 || ta.directAsk == 0 {
		return
	}

	precision := int64(100_000_000)
	if ta.config.CrossInstrument != nil {
		precision = ta.config.CrossInstrument.BasePrecision()
	}

	minProfit := ta.config.TakerFeeBps + ta.config.ThresholdBps

	// Forward: USD → ABC → base → USD.
	// Execute: buy directAsk, buy crossAsk, sell baseBid.
	// Profit bps = (baseBid×precision − directAsk×crossAsk) × 10000 / (directAsk×crossAsk)
	forwardNumer := ta.baseBid*precision - ta.directAsk*ta.crossAsk
	if forwardNumer*10000 > minProfit*(ta.directAsk*ta.crossAsk) {
		ta.executeArbitrage(false, submit)
		return
	}

	// Reverse: USD → base → ABC → USD.
	// Execute: buy baseAsk, sell crossBid, sell directBid.
	// Profit bps = (directBid×crossBid − baseAsk×precision) × 10000 / (baseAsk×precision)
	reverseNumer := ta.directBid*ta.crossBid - ta.baseAsk*precision
	if reverseNumer*10000 > minProfit*(ta.baseAsk*precision) {
		ta.executeArbitrage(true, submit)
	}
}

func (ta *TriangleArbitrage) executeArbitrage(reverse bool, submit actor.OrderSubmitter) {
	ta.executing = true
	ta.pendingFills = 3

	precision := int64(100_000_000)
	if ta.config.CrossInstrument != nil {
		precision = ta.config.CrossInstrument.BasePrecision()
	}

	if !reverse {
		// USD → ABC → base → USD
		// Buy MaxTradeSize base via crossSymbol; ABC needed = MaxTradeSize * crossAsk / precision
		directQty := ta.config.MaxTradeSize * ta.crossAsk / precision
		if directQty == 0 {
			directQty = 1
		}
		ta.directReqID = submit(ta.config.DirectSymbol, exchange.Buy, exchange.Market, 0, directQty)
		ta.crossReqID = submit(ta.config.CrossSymbol, exchange.Buy, exchange.Market, 0, ta.config.MaxTradeSize)
		ta.baseReqID = submit(ta.config.BaseSymbol, exchange.Sell, exchange.Market, 0, ta.config.MaxTradeSize)
	} else {
		// USD → base → ABC → USD
		// Sell MaxTradeSize base via crossSymbol; ABC received = MaxTradeSize * crossBid / precision
		directQty := ta.config.MaxTradeSize * ta.crossBid / precision
		if directQty == 0 {
			directQty = 1
		}
		ta.baseReqID = submit(ta.config.BaseSymbol, exchange.Buy, exchange.Market, 0, ta.config.MaxTradeSize)
		ta.crossReqID = submit(ta.config.CrossSymbol, exchange.Sell, exchange.Market, 0, ta.config.MaxTradeSize)
		ta.directReqID = submit(ta.config.DirectSymbol, exchange.Sell, exchange.Market, 0, directQty)
	}
}
