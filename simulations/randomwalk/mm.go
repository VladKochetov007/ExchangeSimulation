package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type MMConfig struct {
	Symbols         []string
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
	mids       map[string]int64
	pending    map[string]map[uint64]bool
	reqToSym   map[uint64]string // reqID → symbol
	orderToSym map[uint64]string // orderID → symbol
	subscribed map[string]bool
}

func NewMarketMaker(id uint64, gw actor.Gateway, cfg MMConfig) *MarketMaker {
	mm := &MarketMaker{
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

func (mm *MarketMaker) Mid(sym string) int64 { return mm.mids[sym] }

func (mm *MarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventOrderAccepted:
		mm.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		mm.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (mm *MarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	sym, ok := mm.reqToSym[e.RequestID]
	if !ok {
		// Request was cleared by cancelAllForSym before the accept arrived.
		// The order is live in the book but untracked — cancel it immediately.
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.reqToSym, e.RequestID)
	mm.pending[sym][e.OrderID] = true
	mm.orderToSym[e.OrderID] = sym
}

func (mm *MarketMaker) onFilled(e actor.OrderFillEvent) {
	sym, ok := mm.orderToSym[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToSym, e.OrderID)
	delete(mm.pending[sym], e.OrderID)
	mm.mids[sym] = e.Price
	if !e.IsFull {
		mm.CancelOrder(e.OrderID) // cancel remaining qty of partially-filled order
	}
	mm.cancelAllForSym(sym)
	// Timer drives re-quoting; no immediate quote here.
}

func (mm *MarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	sym, ok := mm.orderToSym[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToSym, e.OrderID)
	delete(mm.pending[sym], e.OrderID)
}

func (mm *MarketMaker) onTick(_ time.Time) {
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

func (mm *MarketMaker) cancelAllForSym(sym string) {
	for orderID := range mm.pending[sym] {
		mm.CancelOrder(orderID)
		delete(mm.orderToSym, orderID)
		delete(mm.pending[sym], orderID)
	}
	// Clear in-flight requests for this symbol so late accepts become orphans
	// handled by onAccepted, preventing ghost entries in pending[sym].
	for reqID, s := range mm.reqToSym {
		if s == sym {
			delete(mm.reqToSym, reqID)
		}
	}
}

func (mm *MarketMaker) quote(sym string) {
	mid := mm.mids[sym]
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
