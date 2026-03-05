package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// CrossPairMMConfig configures a market maker that quotes cross-asset pairs.
// It derives fair value from the ratio of two USD-denominated mid prices.
type CrossPairMMConfig struct {
	CrossSymbols    []string          // e.g. ["DEF-ABC", "GHI-ABC"]
	BaseUSDSymbols  []string          // base USD pair per cross symbol, same index
	QuoteUSDSymbol  string            // shared quote asset USD pair ("ABC-USD")
	QuotePrecision  int64             // precision of the quote asset (BTC_PRECISION for ABC)
	TickSizes       map[string]int64  // tick size per cross symbol
	LevelSizes      map[string]int64  // lot size per level per cross symbol
	Levels          int
	LevelSpacing    int64
	RefreshInterval time.Duration
}

// CrossPairMM quotes cross-asset spot pairs (e.g. DEF-ABC, GHI-ABC) by deriving
// fair value from USD pair mids: crossMid = baseUSDMid * QuotePrecision / quoteUSDMid.
// It requotes whenever the derived mid changes by more than one tick.
type CrossPairMM struct {
	*actor.BaseActor
	cfg        CrossPairMMConfig
	usdMids    map[string]int64          // mid prices for all USD pairs we watch
	mids       map[string]int64          // derived cross mids
	quotedMids map[string]int64          // last cross mid we actually quoted at
	pending    map[string]map[uint64]bool // live order IDs per cross symbol
	reqToSym   map[uint64]string          // reqID → cross symbol
	orderToSym map[uint64]string          // orderID → cross symbol
	subscribed bool
}

func NewCrossPairMM(id uint64, gw actor.Gateway, cfg CrossPairMMConfig) *CrossPairMM {
	mm := &CrossPairMM{
		BaseActor:  actor.NewBaseActor(id, gw),
		cfg:        cfg,
		usdMids:    make(map[string]int64, len(cfg.BaseUSDSymbols)+1),
		mids:       make(map[string]int64, len(cfg.CrossSymbols)),
		quotedMids: make(map[string]int64, len(cfg.CrossSymbols)),
		pending:    make(map[string]map[uint64]bool, len(cfg.CrossSymbols)),
		reqToSym:   make(map[uint64]string),
		orderToSym: make(map[uint64]string),
	}
	for _, cross := range cfg.CrossSymbols {
		mm.pending[cross] = make(map[uint64]bool)
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.RefreshInterval, mm.onTick)
	return mm
}

func (mm *CrossPairMM) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		mm.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		mm.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		mm.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		mm.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (mm *CrossPairMM) onSnapshot(e actor.BookSnapshotEvent) {
	if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
		mm.usdMids[e.Symbol] = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
	}
}

func (mm *CrossPairMM) onAccepted(e actor.OrderAcceptedEvent) {
	sym, ok := mm.reqToSym[e.RequestID]
	if !ok {
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.reqToSym, e.RequestID)
	mm.pending[sym][e.OrderID] = true
	mm.orderToSym[e.OrderID] = sym
}

func (mm *CrossPairMM) onFilled(e actor.OrderFillEvent) {
	sym, ok := mm.orderToSym[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToSym, e.OrderID)
	delete(mm.pending[sym], e.OrderID)
	if !e.IsFull {
		mm.CancelOrder(e.OrderID)
	}
	mm.cancelAllForSym(sym)
}

func (mm *CrossPairMM) onCancelled(e actor.OrderCancelledEvent) {
	sym, ok := mm.orderToSym[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToSym, e.OrderID)
	delete(mm.pending[sym], e.OrderID)
}

func (mm *CrossPairMM) onTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(mm.cfg.QuoteUSDSymbol, exchange.MDSnapshot)
		for _, baseSym := range mm.cfg.BaseUSDSymbols {
			mm.Subscribe(baseSym, exchange.MDSnapshot)
		}
		mm.subscribed = true
	}
	mm.recomputeMids()
	for _, cross := range mm.cfg.CrossSymbols {
		newMid := mm.mids[cross]
		if newMid == 0 {
			continue
		}
		if mm.quotedMids[cross] == newMid && len(mm.pending[cross]) > 0 {
			continue
		}
		mm.cancelAllForSym(cross)
		mm.quote(cross)
	}
}

// recomputeMids derives cross mids from USD pair mids and aligns to tick size.
func (mm *CrossPairMM) recomputeMids() {
	abcMid := mm.usdMids[mm.cfg.QuoteUSDSymbol]
	if abcMid == 0 {
		return
	}
	for i, cross := range mm.cfg.CrossSymbols {
		baseMid := mm.usdMids[mm.cfg.BaseUSDSymbols[i]]
		if baseMid == 0 {
			continue
		}
		tick := mm.cfg.TickSizes[cross]
		raw := baseMid * mm.cfg.QuotePrecision / abcMid
		mm.mids[cross] = (raw / tick) * tick
	}
}

func (mm *CrossPairMM) quote(sym string) {
	mid := mm.mids[sym]
	tick := mm.cfg.TickSizes[sym]
	levelSize := mm.cfg.LevelSizes[sym]
	for k := int64(1); k <= int64(mm.cfg.Levels); k++ {
		offset := (1 + (k-1)*mm.cfg.LevelSpacing) * tick
		bidPrice := mid - offset
		askPrice := mid + offset
		if bidPrice <= 0 {
			continue
		}
		bidReqID := mm.SubmitOrder(sym, exchange.Buy, exchange.LimitOrder, bidPrice, levelSize)
		mm.reqToSym[bidReqID] = sym
		askReqID := mm.SubmitOrder(sym, exchange.Sell, exchange.LimitOrder, askPrice, levelSize)
		mm.reqToSym[askReqID] = sym
	}
	mm.quotedMids[sym] = mid
}

func (mm *CrossPairMM) cancelAllForSym(sym string) {
	for orderID := range mm.pending[sym] {
		mm.CancelOrder(orderID)
		delete(mm.orderToSym, orderID)
		delete(mm.pending[sym], orderID)
	}
	for reqID, s := range mm.reqToSym {
		if s == sym {
			delete(mm.reqToSym, reqID)
		}
	}
}
