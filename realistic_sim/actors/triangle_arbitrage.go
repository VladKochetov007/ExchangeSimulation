package actors

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type TriangleArbConfig struct {
	ActorID          uint64
	BaseSymbol       string // e.g. BCD/USD: Sell this to close the circuit
	CrossSymbol      string // e.g. BCD/ABC: Buy base with ABC
	DirectSymbol     string // e.g. ABC/USD: Buy ABC with USD to open
	BaseInstrument   exchange.Instrument
	CrossInstrument  exchange.Instrument
	DirectInstrument exchange.Instrument
	ThresholdBps     int64
	MaxTradeSize     int64
	TakerFeeBps      int64 // total fee across all 3 legs (e.g. 24 = 8bps × 3)
}

type TriangleArbitrage struct {
	id     uint64
	config TriangleArbConfig

	baseMid   int64
	crossMid  int64
	directMid int64

	// Request IDs assigned when submitting each leg.
	directReqID uint64
	crossReqID  uint64
	baseReqID   uint64

	// Order IDs received after acceptance.
	directOrderID uint64
	crossOrderID  uint64
	baseOrderID   uint64

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
	case actor.EventOrderAccepted:
		ta.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
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

func (ta *TriangleArbitrage) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	switch fill.OrderID {
	case ta.directOrderID:
		// Buy ABC/USD: spend USD, receive ABC.
		if ta.config.DirectInstrument != nil {
			ctx.OnFill(ta.id, ta.config.DirectSymbol, fill,
				ta.config.DirectInstrument.BasePrecision(),
				ta.config.DirectInstrument.BaseAsset())
		}
	case ta.crossOrderID:
		// Buy BCD/ABC: spend ABC (quote), receive BCD (base).
		// SharedContext.quoteBalance tracks the primary quote (USD). ABC lives in
		// baseBalances, so we update both sides manually.
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
		// Sell BCD/USD: spend BCD, receive USD.
		if ta.config.BaseInstrument != nil {
			ctx.OnFill(ta.id, ta.config.BaseSymbol, fill,
				ta.config.BaseInstrument.BasePrecision(),
				ta.config.BaseInstrument.BaseAsset())
		}
	}
}

func (ta *TriangleArbitrage) evaluateArbitrage(_ *actor.SharedContext, submit actor.OrderSubmitter) {
	if ta.baseMid == 0 || ta.crossMid == 0 || ta.directMid == 0 {
		return
	}

	// No-arb identity: baseMid == crossMid * directMid / precision
	//   i.e., BCD/USD == BCD/ABC * ABC/USD
	// Implied ABC/USD from the triangle: baseMid * precision / crossMid
	// Arb fires when the implied price exceeds the actual directMid by more
	// than fees + threshold. Circuit: USD → ABC → BCD → USD.
	precision := int64(100_000_000)
	if ta.config.BaseInstrument != nil {
		precision = ta.config.BaseInstrument.BasePrecision()
	}

	impliedDirect := (ta.baseMid * precision) / ta.crossMid
	profitBps := ((impliedDirect - ta.directMid) * 10000) / ta.directMid

	if profitBps > ta.config.TakerFeeBps+ta.config.ThresholdBps {
		ta.executeArbitrage(submit)
	}
}

func (ta *TriangleArbitrage) executeArbitrage(submit actor.OrderSubmitter) {
	ta.executing = true
	// USD → ABC (buy DirectSymbol) → BCD (buy CrossSymbol) → USD (sell BaseSymbol).
	ta.directReqID = submit(ta.config.DirectSymbol, exchange.Buy, exchange.Market, 0, ta.config.MaxTradeSize)
	ta.crossReqID = submit(ta.config.CrossSymbol, exchange.Buy, exchange.Market, 0, ta.config.MaxTradeSize)
	ta.baseReqID = submit(ta.config.BaseSymbol, exchange.Sell, exchange.Market, 0, ta.config.MaxTradeSize)
	ta.executing = false
}
