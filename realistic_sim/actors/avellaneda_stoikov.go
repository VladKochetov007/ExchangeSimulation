package actors

import (
	"context"
	"sync/atomic"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	simmath "exchange_sim/realistic_sim/math"
)

type AvellanedaStoikovConfig struct {
	Symbol           string
	Gamma            int64
	K                int64
	T                int64
	QuoteQty         int64
	MaxInventory     int64
	VolatilityWindow int
	RequoteInterval  time.Duration
}

type AvellanedaStoikovActor struct {
	*actor.BaseActor
	config        AvellanedaStoikovConfig
	instrument    exchange.Instrument
	volatility    *simmath.RollingVolatility
	inventory     int64
	startTime     time.Time
	activeBidID   uint64
	activeAskID   uint64
	lastBidReqID  uint64
	lastAskReqID  uint64
	lastMid       int64
	requestSeq    uint64
	requeueTicker exchange.Ticker
}

func NewAvellanedaStoikov(id uint64, gateway *exchange.ClientGateway, config AvellanedaStoikovConfig) *AvellanedaStoikovActor {
	if config.RequoteInterval == 0 {
		config.RequoteInterval = 500 * time.Millisecond
	}

	return &AvellanedaStoikovActor{
		BaseActor:  actor.NewBaseActor(id, gateway),
		config:     config,
		volatility: simmath.NewRollingVolatility(config.VolatilityWindow, 10000),
	}
}

func (as *AvellanedaStoikovActor) Start(ctx context.Context) error {
	as.startTime = time.Now()
	as.requeueTicker = as.GetTickerFactory().NewTicker(as.config.RequoteInterval)

	go as.eventLoop(ctx)

	if err := as.BaseActor.Start(ctx); err != nil {
		return err
	}

	as.Subscribe(as.config.Symbol)
	return nil
}

func (as *AvellanedaStoikovActor) Stop() error {
	if as.requeueTicker != nil {
		as.requeueTicker.Stop()
	}
	return as.BaseActor.Stop()
}

func (as *AvellanedaStoikovActor) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-as.EventChannel():
			as.OnEvent(event)
		case <-as.requeueTicker.C():
			as.placeQuotes()
		}
	}
}

func (as *AvellanedaStoikovActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		as.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventTrade:
		as.onTrade(event.Data.(actor.TradeEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		as.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderAccepted:
		as.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderCancelled:
		as.onOrderCancelled(event.Data.(actor.OrderCancelledEvent))
	case actor.EventOrderRejected:
		as.onOrderRejected(event.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderCancelRejected:
		as.onOrderCancelRejected(event.Data.(actor.OrderCancelRejectedEvent))
	}
}

func (as *AvellanedaStoikovActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if snap.Symbol != as.config.Symbol {
		return
	}

	if len(snap.Snapshot.Bids) > 0 && len(snap.Snapshot.Asks) > 0 {
		bestBid := snap.Snapshot.Bids[0].Price
		bestAsk := snap.Snapshot.Asks[0].Price
		as.lastMid = bestBid + (bestAsk-bestBid)/2
	}
}

func (as *AvellanedaStoikovActor) onTrade(trade actor.TradeEvent) {
	if trade.Symbol != as.config.Symbol {
		return
	}
	as.volatility.AddPrice(trade.Trade.Price)
}

func (as *AvellanedaStoikovActor) onOrderFilled(fill actor.OrderFillEvent) {
	delta := int64(fill.Qty)
	if fill.Side == exchange.Sell {
		delta = -delta
	}
	as.inventory += delta

	as.cancelActiveQuotes()
	as.activeBidID = 0
	as.activeAskID = 0
}

func (as *AvellanedaStoikovActor) onOrderAccepted(accepted actor.OrderAcceptedEvent) {
	if accepted.RequestID == as.lastBidReqID {
		as.activeBidID = accepted.OrderID
		as.lastBidReqID = 0
	} else if accepted.RequestID == as.lastAskReqID {
		as.activeAskID = accepted.OrderID
		as.lastAskReqID = 0
	}
}

func (as *AvellanedaStoikovActor) onOrderCancelled(cancelled actor.OrderCancelledEvent) {
	if cancelled.OrderID == as.activeBidID {
		as.activeBidID = 0
	}
	if cancelled.OrderID == as.activeAskID {
		as.activeAskID = 0
	}
}

func (as *AvellanedaStoikovActor) onOrderRejected(rejected actor.OrderRejectedEvent) {
	if rejected.RequestID == as.lastBidReqID {
		as.lastBidReqID = 0
	} else if rejected.RequestID == as.lastAskReqID {
		as.lastAskReqID = 0
	}
}

func (as *AvellanedaStoikovActor) onOrderCancelRejected(rejected actor.OrderCancelRejectedEvent) {
	if rejected.OrderID == as.activeBidID {
		as.activeBidID = 0
	} else if rejected.OrderID == as.activeAskID {
		as.activeAskID = 0
	}
}

func (as *AvellanedaStoikovActor) calculateQuotes() (bidPrice, askPrice int64) {
	if as.lastMid == 0 || as.instrument == nil {
		return 0, 0
	}

	if as.volatility.Size() < as.config.VolatilityWindow {
		return 0, 0
	}

	mid := as.lastMid
	basePrecision := int64(as.instrument.BasePrecision())
	quotePrecision := int64(as.instrument.QuotePrecision())
	q := (as.inventory * quotePrecision) / basePrecision
	sigma := as.volatility.Volatility()

	tau := as.config.T - int64(time.Since(as.startTime).Seconds())
	if tau <= 0 {
		tau = 1
	}

	secondsPerYear := int64(31536000)
	scale := int64(10000)

	reservationAdj := ((q * as.config.Gamma / scale) * (sigma * sigma / scale) * tau) / (scale * secondsPerYear)
	r := mid - reservationAdj

	spreadPart1 := ((as.config.Gamma * sigma / scale) * sigma * tau) / (2 * scale * secondsPerYear)

	x := (as.config.Gamma * 10000) / as.config.K
	lnTerm := x - (x*x)/(2*10000) + (x*x*x)/(3*10000*10000)
	spreadPart2 := (10000 * lnTerm) / as.config.Gamma

	delta := spreadPart1 + spreadPart2

	bidPrice = r - delta
	askPrice = r + delta

	tickSize := as.instrument.TickSize()
	bidPrice = (bidPrice / tickSize) * tickSize
	askPrice = (askPrice / tickSize) * tickSize

	return bidPrice, askPrice
}

func (as *AvellanedaStoikovActor) placeQuotes() {
	bidPrice, askPrice := as.calculateQuotes()
	if bidPrice == 0 || askPrice == 0 {
		return
	}

	as.cancelActiveQuotes()

	if as.inventory >= as.config.MaxInventory {
		as.lastAskReqID = atomic.AddUint64(&as.requestSeq, 1)
		as.SubmitOrder(as.config.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, as.config.QuoteQty)
		return
	}

	if as.inventory <= -as.config.MaxInventory {
		as.lastBidReqID = atomic.AddUint64(&as.requestSeq, 1)
		as.SubmitOrder(as.config.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, as.config.QuoteQty)
		return
	}

	as.lastBidReqID = atomic.AddUint64(&as.requestSeq, 1)
	as.SubmitOrder(as.config.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, as.config.QuoteQty)

	as.lastAskReqID = atomic.AddUint64(&as.requestSeq, 1)
	as.SubmitOrder(as.config.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, as.config.QuoteQty)
}

func (as *AvellanedaStoikovActor) cancelActiveQuotes() {
	if as.activeBidID != 0 {
		atomic.AddUint64(&as.requestSeq, 1)
		as.CancelOrder(as.activeBidID)
	}
	if as.activeAskID != 0 {
		atomic.AddUint64(&as.requestSeq, 1)
		as.CancelOrder(as.activeAskID)
	}
}

func (as *AvellanedaStoikovActor) SetInstrument(instrument exchange.Instrument) {
	as.instrument = instrument
}

func (as *AvellanedaStoikovActor) GetInventory() int64 {
	return as.inventory
}
