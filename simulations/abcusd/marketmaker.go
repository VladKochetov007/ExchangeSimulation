package abcusd

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type MarketMakerConfig struct {
	Symbols         []string
	BootstrapPrice  int64
	SpotSymbol      string
	PerpSymbol      string
	Levels          int
	LevelSpacing    int64
	LevelSize       int64
	TickSize        int64
	RefreshInterval time.Duration
	BasisAlpha      float64
}

type PureMarketMaker struct {
	*actor.BaseActor
	cfg        MarketMakerConfig
	mids       map[string]int64
	pending    map[string]map[uint64]bool
	reqToSym   map[uint64]string
	orderToSym map[uint64]string
	subscribed map[string]bool
}

func NewPureMarketMaker(id uint64, gw actor.Gateway, cfg MarketMakerConfig) *PureMarketMaker {
	mm := &PureMarketMaker{
		BaseActor:  actor.NewBaseActor(id, gw),
		cfg:        cfg,
		mids:       make(map[string]int64, len(cfg.Symbols)),
		pending:    make(map[string]map[uint64]bool, len(cfg.Symbols)),
		reqToSym:   make(map[uint64]string),
		orderToSym: make(map[uint64]string),
		subscribed: make(map[string]bool, len(cfg.Symbols)),
	}
	for _, sym := range cfg.Symbols {
		mm.mids[sym] = cfg.BootstrapPrice
		mm.pending[sym] = make(map[uint64]bool)
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.RefreshInterval, mm.onTick)
	return mm
}

func (mm *PureMarketMaker) Mid(sym string) int64 { return mm.mids[sym] }

func (mm *PureMarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		mm.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled:
		mm.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (mm *PureMarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	sym, ok := mm.reqToSym[e.RequestID]
	if !ok {
		return
	}
	delete(mm.reqToSym, e.RequestID)
	mm.pending[sym][e.OrderID] = true
	mm.orderToSym[e.OrderID] = sym
}

func (mm *PureMarketMaker) onFilled(e actor.OrderFillEvent) {
	sym, ok := mm.orderToSym[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToSym, e.OrderID)
	delete(mm.pending[sym], e.OrderID)

	mm.mids[sym] = e.Price

	if sym == mm.cfg.SpotSymbol && mm.cfg.BasisAlpha > 0 {
		perpMid := mm.mids[mm.cfg.PerpSymbol]
		spotMid := mm.mids[mm.cfg.SpotSymbol]
		adjusted := perpMid + int64(float64(spotMid-perpMid)*mm.cfg.BasisAlpha)
		mm.mids[mm.cfg.PerpSymbol] = (adjusted / mm.cfg.TickSize) * mm.cfg.TickSize
	}

	mm.cancelAllForSym(sym)
	mm.quote(sym)
}

func (mm *PureMarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	sym, ok := mm.orderToSym[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToSym, e.OrderID)
	delete(mm.pending[sym], e.OrderID)
}

func (mm *PureMarketMaker) onTick(_ time.Time) {
	for _, sym := range mm.cfg.Symbols {
		if !mm.subscribed[sym] {
			mm.Subscribe(sym, exchange.MDSnapshot)
			mm.subscribed[sym] = true
		}
		if len(mm.pending[sym]) == 0 {
			mm.quote(sym)
		}
	}
}

func (mm *PureMarketMaker) cancelAllForSym(sym string) {
	for orderID := range mm.pending[sym] {
		mm.CancelOrder(orderID)
	}
}

func (mm *PureMarketMaker) quote(sym string) {
	mid := mm.mids[sym]

	if sym == mm.cfg.PerpSymbol && mm.cfg.BasisAlpha > 0 {
		spotMid := mm.mids[mm.cfg.SpotSymbol]
		adjusted := mid + int64(float64(spotMid-mid)*mm.cfg.BasisAlpha)
		mid = (adjusted / mm.cfg.TickSize) * mm.cfg.TickSize
		mm.mids[sym] = mid
	}

	for k := int64(1); k <= int64(mm.cfg.Levels); k++ {
		offset := (1 + (k-1)*mm.cfg.LevelSpacing) * mm.cfg.TickSize
		bidPrice := mid - offset
		askPrice := mid + offset
		if bidPrice <= 0 {
			continue
		}

		bidReqID := mm.SubmitOrder(sym, exchange.Buy, exchange.LimitOrder, bidPrice, mm.cfg.LevelSize)
		mm.reqToSym[bidReqID] = sym

		askReqID := mm.SubmitOrder(sym, exchange.Sell, exchange.LimitOrder, askPrice, mm.cfg.LevelSize)
		mm.reqToSym[askReqID] = sym
	}
}
