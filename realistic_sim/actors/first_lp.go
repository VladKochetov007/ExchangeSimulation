package actors

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type ExitStrategyFunc func(exposure, bestBid, bestAsk, bidLiquidity, askLiquidity, liquidityMultiple int64) bool

type FirstLPConfig struct {
	Symbol            string
	HalfSpreadBps     int64
	LiquidityMultiple int64
	MonitorInterval   time.Duration
	MinExitSize       int64
	ExitStrategy      ExitStrategyFunc
	BootstrapPrice    int64
}

type FirstLiquidityProvidingActor struct {
	*actor.BaseActor
	Config FirstLPConfig

	Symbol        string
	Instrument    exchange.Instrument
	BaseAsset     string
	QuoteAsset    string
	BaseBalance   int64
	QuoteBalance  int64
	NetPosition   int64
	AvgEntryPrice int64
	ActiveBidID   uint64
	ActiveAskID   uint64
	BidSize       int64
	AskSize       int64
	lastBidReqID  uint64
	lastAskReqID  uint64
	LastMidPrice  int64
	BestBid       int64
	BestAsk       int64
	BidLiquidity  int64
	AskLiquidity  int64
	monitorTicker *time.Ticker
}

func DefaultExitStrategy(exposure, bestBid, bestAsk, bidLiq, askLiq, multiple int64) bool {
	if exposure == 0 {
		return false
	}

	absExposure := exposure
	if absExposure < 0 {
		absExposure = -absExposure
	}

	threshold := absExposure * multiple

	if exposure > 0 {
		return bidLiq >= threshold
	}
	return askLiq >= threshold
}

func NewFirstLP(id uint64, gateway *exchange.ClientGateway, config FirstLPConfig) *FirstLiquidityProvidingActor {
	if config.LiquidityMultiple == 0 {
		config.LiquidityMultiple = 10
	}
	if config.MonitorInterval == 0 {
		config.MonitorInterval = 100 * time.Millisecond
	}
	if config.ExitStrategy == nil {
		config.ExitStrategy = DefaultExitStrategy
	}

	return &FirstLiquidityProvidingActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		Config:    config,
		Symbol:    config.Symbol,
	}
}

func (f *FirstLiquidityProvidingActor) Start(ctx context.Context) error {
	f.monitorTicker = time.NewTicker(f.Config.MonitorInterval)

	go f.eventLoop(ctx)

	if err := f.BaseActor.Start(ctx); err != nil {
		return err
	}

	f.Subscribe(f.Symbol)
	f.QueryBalance()

	return nil
}

func (f *FirstLiquidityProvidingActor) Stop() error {
	if f.monitorTicker != nil {
		f.monitorTicker.Stop()
	}
	return f.BaseActor.Stop()
}

func (f *FirstLiquidityProvidingActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-f.EventChannel():
			f.OnEvent(event)
		case <-f.monitorTicker.C:
			f.CheckExitConditions()
		}
	}
}

func (f *FirstLiquidityProvidingActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		f.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventBookDelta:
		f.onBookDelta(event.Data.(actor.BookDeltaEvent))
	case actor.EventOrderAccepted:
		f.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		f.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		f.onOrderCancelled(event.Data.(actor.OrderCancelledEvent))
	case actor.EventOrderRejected:
		f.onOrderRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderCancelRejected:
		f.onOrderCancelRejected(event.Data.(actor.OrderCancelRejectedEvent))
	}
}

func (f *FirstLiquidityProvidingActor) SetInitialState(instrument exchange.Instrument) {
	f.Instrument = instrument
	f.BaseAsset = instrument.BaseAsset()
	f.QuoteAsset = instrument.QuoteAsset()
}

func (f *FirstLiquidityProvidingActor) UpdateBalances(baseBalance, quoteBalance int64) {
	f.BaseBalance = baseBalance
	f.QuoteBalance = quoteBalance
}

func (f *FirstLiquidityProvidingActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if snap.Symbol != f.Symbol {
		return
	}

	if len(snap.Snapshot.Bids) > 0 {
		f.BestBid = snap.Snapshot.Bids[0].Price
		f.BidLiquidity = snap.Snapshot.Bids[0].VisibleQty
	}
	if len(snap.Snapshot.Asks) > 0 {
		f.BestAsk = snap.Snapshot.Asks[0].Price
		f.AskLiquidity = snap.Snapshot.Asks[0].VisibleQty
	}

	if f.BestBid > 0 && f.BestAsk > 0 {
		f.LastMidPrice = (f.BestBid + f.BestAsk) / 2
	} else if f.LastMidPrice == 0 && f.Config.BootstrapPrice > 0 {
		f.LastMidPrice = f.Config.BootstrapPrice
	}

	canPlace := f.LastMidPrice > 0 && f.BaseBalance > 0 && f.QuoteBalance > 0 &&
		f.ActiveBidID == 0 && f.ActiveAskID == 0

	if canPlace {
		f.PlaceQuotes()
	}
}

func (f *FirstLiquidityProvidingActor) onBookDelta(delta actor.BookDeltaEvent) {
	if delta.Delta.Side == exchange.Buy {
		if delta.Delta.Price >= f.BestBid {
			f.BestBid = delta.Delta.Price
			f.BidLiquidity = max(delta.Delta.VisibleQty, 0)
		}
	} else {
		if f.BestAsk == 0 || delta.Delta.Price <= f.BestAsk {
			f.BestAsk = delta.Delta.Price
			f.AskLiquidity = max(delta.Delta.VisibleQty, 0)
		}
	}
}

func (f *FirstLiquidityProvidingActor) onOrderFilled(fill actor.OrderFillEvent) {
	if f.Instrument == nil {
		return
	}

	basePrecision := f.Instrument.BasePrecision()

	if fill.Side == exchange.Buy {
		f.UpdatePosition(fill.Qty, fill.Price)
		f.BaseBalance += fill.Qty
		notional := (fill.Qty * fill.Price) / basePrecision
		f.QuoteBalance -= (notional + fill.FeeAmount)
	} else {
		f.UpdatePosition(-fill.Qty, fill.Price)
		f.BaseBalance -= fill.Qty
		notional := (fill.Qty * fill.Price) / basePrecision
		f.QuoteBalance += (notional - fill.FeeAmount)
	}

	f.CancelActiveOrders()

	f.ActiveBidID = 0
	f.ActiveAskID = 0
	f.PlaceQuotes()
}

func (f *FirstLiquidityProvidingActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	if cancelled.OrderID == f.ActiveBidID {
		f.ActiveBidID = 0
	}
	if cancelled.OrderID == f.ActiveAskID {
		f.ActiveAskID = 0
	}
}

func (f *FirstLiquidityProvidingActor) onOrderRejected(rejected actor.OrderRejectedEvent) {
	if rejected.RequestID == f.lastBidReqID {
		f.lastBidReqID = 0
	} else if rejected.RequestID == f.lastAskReqID {
		f.lastAskReqID = 0
	}
}

func (f *FirstLiquidityProvidingActor) onOrderCancelRejected(rejected actor.OrderCancelRejectedEvent) {
	if rejected.OrderID == f.ActiveBidID {
		f.ActiveBidID = 0
	} else if rejected.OrderID == f.ActiveAskID {
		f.ActiveAskID = 0
	}
}

func (f *FirstLiquidityProvidingActor) nextRequestID() uint64 {
	return f.PeekNextRequestID()
}

func (f *FirstLiquidityProvidingActor) PlaceQuotes() {
	if f.LastMidPrice == 0 || f.Instrument == nil {
		return
	}

	basePrecision := f.Instrument.BasePrecision()
	tickSize := f.Instrument.TickSize()

	halfSpread := (f.LastMidPrice * f.Config.HalfSpreadBps) / 10000
	bidPrice := f.LastMidPrice - halfSpread
	askPrice := f.LastMidPrice + halfSpread

	bidPrice = (bidPrice / tickSize) * tickSize
	askPrice = (askPrice / tickSize) * tickSize

	if f.BaseBalance > 0 {
		f.lastAskReqID = f.nextRequestID()
		f.SubmitOrder(f.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, f.BaseBalance)
		f.AskSize = f.BaseBalance
	}
	if f.QuoteBalance > 0 {
		bidQty := (f.QuoteBalance * basePrecision) / bidPrice
		if bidQty > 0 {
			f.lastBidReqID = f.nextRequestID()
			f.SubmitOrder(f.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, bidQty)
			f.BidSize = bidQty
		}
	}
}

func (f *FirstLiquidityProvidingActor) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	if accepted.RequestID == f.lastBidReqID {
		f.ActiveBidID = accepted.OrderID
		f.lastBidReqID = 0
	} else if accepted.RequestID == f.lastAskReqID {
		f.ActiveAskID = accepted.OrderID
		f.lastAskReqID = 0
	}
}

func (f *FirstLiquidityProvidingActor) CancelActiveOrders() {
	if f.ActiveBidID != 0 {
		f.CancelOrder(f.ActiveBidID)
	}
	if f.ActiveAskID != 0 {
		f.CancelOrder(f.ActiveAskID)
	}
}

func (f *FirstLiquidityProvidingActor) CheckExitConditions() {
	absExposure := f.NetPosition
	if absExposure < 0 {
		absExposure = -absExposure
	}

	if absExposure < f.Config.MinExitSize {
		return
	}

	shouldExit := f.Config.ExitStrategy(
		f.NetPosition,
		f.BestBid,
		f.BestAsk,
		f.BidLiquidity,
		f.AskLiquidity,
		f.Config.LiquidityMultiple,
	)

	if shouldExit {
		f.ExecuteExit()
	}
}

func (f *FirstLiquidityProvidingActor) ExecuteExit() {
	if f.NetPosition == 0 {
		return
	}

	f.CancelActiveOrders()

	exitSize := f.NetPosition
	if exitSize < 0 {
		exitSize = -exitSize
	}

	if f.NetPosition > 0 {
		f.SubmitOrder(f.Symbol, exchange.Sell, exchange.Market, 0, exitSize)
	} else {
		f.SubmitOrder(f.Symbol, exchange.Buy, exchange.Market, 0, exitSize)
	}
}

func (f *FirstLiquidityProvidingActor) UpdatePosition(deltaQty int64, price int64) {
	if f.NetPosition == 0 {
		f.NetPosition = deltaQty
		f.AvgEntryPrice = price
		return
	}

	if (f.NetPosition > 0 && deltaQty > 0) || (f.NetPosition < 0 && deltaQty < 0) {
		totalNotional := (f.NetPosition * f.AvgEntryPrice) + (deltaQty * price)
		f.NetPosition += deltaQty
		if f.NetPosition != 0 {
			f.AvgEntryPrice = totalNotional / f.NetPosition
		}
	} else {
		f.NetPosition += deltaQty
		if f.NetPosition == 0 {
			f.AvgEntryPrice = 0
		} else if (f.NetPosition > 0 && deltaQty < 0) || (f.NetPosition < 0 && deltaQty > 0) {
			f.AvgEntryPrice = price
		}
	}
}

func (f *FirstLiquidityProvidingActor) GetPosition() (netPosition, avgEntryPrice int64) {
	return f.NetPosition, f.AvgEntryPrice
}

func (f *FirstLiquidityProvidingActor) GetBalances() (base, quote int64) {
	return f.BaseBalance, f.QuoteBalance
}

func (f *FirstLiquidityProvidingActor) SetMarketState(bestBid, bidLiq, bestAsk, askLiq int64) {
	f.BestBid = bestBid
	f.BidLiquidity = bidLiq
	f.BestAsk = bestAsk
	f.AskLiquidity = askLiq
}
