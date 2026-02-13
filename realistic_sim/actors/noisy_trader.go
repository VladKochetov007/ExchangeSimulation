package actors

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type NoisyTraderConfig struct {
	Symbol          string
	Interval        time.Duration
	PriceRangeBps   int64
	MinQty          int64
	MaxQty          int64
	MaxActiveOrders int
	OrderLifetime   time.Duration
}

type activeOrder struct {
	orderID  uint64
	placedAt time.Time
}

type NoisyTraderActor struct {
	*actor.BaseActor
	Config       NoisyTraderConfig
	midPrice     int64
	bestBid      int64
	bestAsk      int64
	activeOrders map[uint64]*activeOrder
	rngMu        sync.Mutex
	rng          *rand.Rand
}

func NewNoisyTrader(id uint64, gateway *exchange.ClientGateway, config NoisyTraderConfig) *NoisyTraderActor {
	return &NoisyTraderActor{
		BaseActor:    actor.NewBaseActor(id, gateway),
		Config:       config,
		activeOrders: make(map[uint64]*activeOrder),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano() + int64(id))),
	}
}

func (a *NoisyTraderActor) Start(ctx context.Context) error {
	a.Subscribe(a.Config.Symbol)
	go a.loop(ctx)
	return a.BaseActor.Start(ctx)
}

func (a *NoisyTraderActor) loop(ctx context.Context) {
	tradingTicker := a.GetTickerFactory().NewTicker(a.Config.Interval)
	defer tradingTicker.Stop()

	var cleanupCh <-chan time.Time
	if a.Config.OrderLifetime > 0 {
		cleanupTicker := a.GetTickerFactory().NewTicker(a.Config.OrderLifetime / 2)
		defer cleanupTicker.Stop()
		cleanupCh = cleanupTicker.C()
	}

	a.rngMu.Lock()
	initialDelay := time.Duration(a.rng.Int63n(int64(a.Config.Interval)))
	a.rngMu.Unlock()

	delayTimer := time.NewTimer(initialDelay)
	defer delayTimer.Stop()
	tradingEnabled := false

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-a.EventChannel():
			a.OnEvent(event)
		case <-delayTimer.C:
			tradingEnabled = true
		case <-tradingTicker.C():
			if tradingEnabled && len(a.activeOrders) < a.Config.MaxActiveOrders && a.midPrice > 0 {
				a.placeRandomOrder()
			}
		case <-cleanupCh:
			now := time.Now()
			for orderID, order := range a.activeOrders {
				if now.Sub(order.placedAt) > a.Config.OrderLifetime {
					a.CancelOrder(orderID)
				}
			}
		}
	}
}

func (a *NoisyTraderActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		a.onBookSnapshot(event.Data.(actor.BookSnapshotEvent))
	case actor.EventBookDelta:
		a.onBookDelta(event.Data.(actor.BookDeltaEvent))
	case actor.EventOrderAccepted:
		a.onOrderAccepted(event.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		a.onOrderFilled(event.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		a.onOrderCancelled(event.Data.(actor.OrderCancelledEvent))
	}
}

func (a *NoisyTraderActor) onBookSnapshot(snap actor.BookSnapshotEvent) {
	if len(snap.Snapshot.Bids) > 0 {
		a.bestBid = snap.Snapshot.Bids[0].Price
	}
	if len(snap.Snapshot.Asks) > 0 {
		a.bestAsk = snap.Snapshot.Asks[0].Price
	}
	a.updateMidPrice()
}

func (a *NoisyTraderActor) onBookDelta(delta actor.BookDeltaEvent) {
	if delta.Delta.Side == exchange.Buy && (a.bestBid == 0 || delta.Delta.Price >= a.bestBid) {
		if delta.Delta.VisibleQty > 0 {
			a.bestBid = delta.Delta.Price
		}
	} else if delta.Delta.Side == exchange.Sell && (a.bestAsk == 0 || delta.Delta.Price <= a.bestAsk) {
		if delta.Delta.VisibleQty > 0 {
			a.bestAsk = delta.Delta.Price
		}
	}
	a.updateMidPrice()
}

func (a *NoisyTraderActor) updateMidPrice() {
	if a.bestBid > 0 && a.bestAsk > 0 {
		a.midPrice = (a.bestBid + a.bestAsk) / 2
	}
}

func (a *NoisyTraderActor) onOrderAccepted(event actor.OrderAcceptedEvent) {
	a.activeOrders[event.OrderID] = &activeOrder{
		orderID:  event.OrderID,
		placedAt: time.Now(),
	}
}

func (a *NoisyTraderActor) onOrderFilled(event actor.OrderFillEvent) {
	if event.IsFull {
		delete(a.activeOrders, event.OrderID)
	}
}

func (a *NoisyTraderActor) onOrderCancelled(event actor.OrderCancelledEvent) {
	delete(a.activeOrders, event.OrderID)
}


func (a *NoisyTraderActor) placeRandomOrder() {
	a.rngMu.Lock()
	side := exchange.Buy
	if a.rng.Float64() < 0.5 {
		side = exchange.Sell
	}

	offsetBps := a.rng.Int63n(a.Config.PriceRangeBps*2) - a.Config.PriceRangeBps
	qtyRange := a.Config.MaxQty - a.Config.MinQty
	qty := a.Config.MinQty
	if qtyRange > 0 {
		qty += a.rng.Int63n(qtyRange)
	}
	a.rngMu.Unlock()

	price := a.midPrice + (a.midPrice * offsetBps / 10000)

	if price <= 0 {
		return
	}

	a.SubmitOrder(a.Config.Symbol, side, exchange.LimitOrder, price, qty)
}

