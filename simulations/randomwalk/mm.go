package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type MMConfig struct {
	Symbol          string
	BootstrapPrice  int64
	Levels          int
	LevelSpacing    int64
	LevelSize       int64
	TickSize        int64
	RefreshInterval time.Duration
}

type MarketMaker struct {
	*actor.BaseActor
	cfg        MMConfig
	mid        int64
	pending    map[uint64]bool
	reqIDs     map[uint64]bool
	subscribed bool
}

func NewMarketMaker(id uint64, gw actor.Gateway, cfg MMConfig) *MarketMaker {
	mm := &MarketMaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		mid:       cfg.BootstrapPrice,
		pending:   make(map[uint64]bool),
		reqIDs:    make(map[uint64]bool),
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.RefreshInterval, mm.onTick)
	return mm
}

func (mm *MarketMaker) Mid() int64 { return mm.mid }

func (mm *MarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		mm.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled:
		mm.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (mm *MarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	if !mm.reqIDs[e.RequestID] {
		return
	}
	delete(mm.reqIDs, e.RequestID)
	mm.pending[e.OrderID] = true
}

func (mm *MarketMaker) onFilled(e actor.OrderFillEvent) {
	if !mm.pending[e.OrderID] {
		return
	}
	delete(mm.pending, e.OrderID)
	mm.mid = e.Price
	mm.cancelAll()
	mm.quote()
}

func (mm *MarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	delete(mm.pending, e.OrderID)
}

func (mm *MarketMaker) onTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(mm.cfg.Symbol, exchange.MDSnapshot)
		mm.subscribed = true
	}
	if len(mm.pending) == 0 {
		mm.quote()
	}
}

func (mm *MarketMaker) cancelAll() {
	for orderID := range mm.pending {
		mm.CancelOrder(orderID)
		delete(mm.pending, orderID)
	}
}

func (mm *MarketMaker) quote() {
	for k := int64(1); k <= int64(mm.cfg.Levels); k++ {
		offset := (1 + (k-1)*mm.cfg.LevelSpacing) * mm.cfg.TickSize
		bidPrice := mm.mid - offset
		askPrice := mm.mid + offset
		if bidPrice <= 0 {
			continue
		}
		bidReqID := mm.SubmitOrder(mm.cfg.Symbol, exchange.Buy, exchange.LimitOrder, bidPrice, mm.cfg.LevelSize)
		mm.reqIDs[bidReqID] = true
		askReqID := mm.SubmitOrder(mm.cfg.Symbol, exchange.Sell, exchange.LimitOrder, askPrice, mm.cfg.LevelSize)
		mm.reqIDs[askReqID] = true
	}
}
