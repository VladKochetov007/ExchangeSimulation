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
	CrossSymbols    []string         // e.g. ["DEF-ABC", "GHI-ABC"]
	BaseUSDSymbols  []string         // base USD pair per cross symbol, same index
	QuoteUSDSymbol  string           // shared quote asset USD pair ("ABC-USD")
	QuotePrecision  int64            // precision of the quote asset (BTC_PRECISION for ABC)
	TickSizes       map[string]int64 // tick size per cross symbol
	LevelSizes      map[string]int64 // lot size per level per cross symbol
	Levels          int
	LevelSpacing    int64
	RefreshInterval time.Duration
}

// CrossPairMM quotes cross-asset spot pairs (e.g. DEF-ABC, GHI-ABC) by deriving
// fair value from USD pair mids: crossMid = baseUSDMid * QuotePrecision / quoteUSDMid.
// It requotes whenever the derived mid changes by more than one tick.
type CrossPairMM struct {
	*actor.BaseActor
	cfg          CrossPairMMConfig
	usdMids      map[string]int64             // mid prices for all USD pairs we watch
	mids         map[string]int64             // derived cross mids
	quotedMids   map[string]int64             // last cross mid we actually quoted at
	pending      map[quoteRef]map[uint64]bool // live order IDs per cross symbol and side
	reqToQuote   map[uint64]quoteRef          // reqID → quote side
	orderToQuote map[uint64]quoteRef          // orderID → quote side
	withdrawn    map[quoteRef]bool            // side has no inventory; wait for an opposite fill
	subscribed   bool
}

func NewCrossPairMM(id uint64, gw actor.Gateway, cfg CrossPairMMConfig) *CrossPairMM {
	mm := &CrossPairMM{
		BaseActor:    actor.NewBaseActor(id, gw),
		cfg:          cfg,
		usdMids:      make(map[string]int64, len(cfg.BaseUSDSymbols)+1),
		mids:         make(map[string]int64, len(cfg.CrossSymbols)),
		quotedMids:   make(map[string]int64, len(cfg.CrossSymbols)),
		pending:      make(map[quoteRef]map[uint64]bool, len(cfg.CrossSymbols)*len(quoteSides)),
		reqToQuote:   make(map[uint64]quoteRef),
		orderToQuote: make(map[uint64]quoteRef),
		withdrawn:    make(map[quoteRef]bool),
	}
	for _, cross := range cfg.CrossSymbols {
		for _, side := range quoteSides {
			mm.pending[quoteRef{symbol: cross, side: side}] = make(map[uint64]bool)
		}
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
	case actor.EventOrderRejected:
		mm.onRejected(evt.Data.(actor.OrderRejectedEvent))
	}
}

func (mm *CrossPairMM) onSnapshot(e actor.BookSnapshotEvent) {
	if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
		mm.usdMids[e.Symbol] = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
	}
}

func (mm *CrossPairMM) onAccepted(e actor.OrderAcceptedEvent) {
	ref, ok := mm.reqToQuote[e.RequestID]
	if !ok {
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.reqToQuote, e.RequestID)
	mm.pending[ref][e.OrderID] = true
	mm.orderToQuote[e.OrderID] = ref
}

func (mm *CrossPairMM) onFilled(e actor.OrderFillEvent) {
	ref, ok := mm.orderToQuote[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToQuote, e.OrderID)
	delete(mm.pending[ref], e.OrderID)
	delete(mm.withdrawn, quoteRef{symbol: ref.symbol, side: oppositeSide(ref.side)})
	if !e.IsFull {
		mm.CancelOrder(e.OrderID)
	}
	mm.cancelAllForSym(ref.symbol)
	mm.quote(ref.symbol)
}

func (mm *CrossPairMM) onCancelled(e actor.OrderCancelledEvent) {
	ref, ok := mm.orderToQuote[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToQuote, e.OrderID)
	delete(mm.pending[ref], e.OrderID)
	mm.ensureQuoted(ref.symbol)
}

func (mm *CrossPairMM) onRejected(e actor.OrderRejectedEvent) {
	ref, ok := mm.reqToQuote[e.RequestID]
	if !ok {
		return
	}
	delete(mm.reqToQuote, e.RequestID)
	// A level can be rejected after earlier levels on the same side were
	// accepted and reserved. Withdrawing the whole side in that case turns a
	// partially funded quote into an empty book at the next reprice.
	if e.Reason == exchange.RejectInsufficientBalance && len(mm.pending[ref]) == 0 {
		mm.withdrawn[ref] = true
	}
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
		if mm.quotedMids[cross] == newMid {
			mm.ensureQuoted(cross)
			continue
		}
		mm.cancelAllForSym(cross)
		mm.quote(cross)
	}
}

func (mm *CrossPairMM) ensureQuoted(sym string) {
	if mm.mids[sym] == 0 {
		return
	}
	for _, side := range quoteSides {
		mm.quoteSide(quoteRef{symbol: sym, side: side})
	}
}

func (mm *CrossPairMM) hasOutstanding(ref quoteRef) bool {
	if len(mm.pending[ref]) > 0 {
		return true
	}
	for _, requestRef := range mm.reqToQuote {
		if requestRef == ref {
			return true
		}
	}
	return false
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
	for _, side := range quoteSides {
		mm.quoteSide(quoteRef{symbol: sym, side: side})
	}
	mm.quotedMids[sym] = mm.mids[sym]
}

func (mm *CrossPairMM) quoteSide(ref quoteRef) {
	if mm.withdrawn[ref] || mm.hasOutstanding(ref) {
		return
	}

	sym := ref.symbol
	mid := mm.mids[sym]
	tick := mm.cfg.TickSizes[sym]
	levelSize := mm.cfg.LevelSizes[sym]
	for k := int64(1); k <= int64(mm.cfg.Levels); k++ {
		offset := (1 + (k-1)*mm.cfg.LevelSpacing) * tick
		price := mid + offset
		if ref.side == exchange.Buy {
			price = mid - offset
		}
		if price <= 0 {
			continue
		}
		reqID := mm.SubmitOrder(sym, ref.side, exchange.LimitOrder, price, levelSize)
		mm.reqToQuote[reqID] = ref
	}
}

func (mm *CrossPairMM) cancelAllForSym(sym string) {
	for ref, orderIDs := range mm.pending {
		if ref.symbol != sym {
			continue
		}
		for orderID := range orderIDs {
			mm.CancelOrder(orderID)
			delete(mm.orderToQuote, orderID)
			delete(orderIDs, orderID)
		}
	}
	for reqID, ref := range mm.reqToQuote {
		if ref.symbol == sym {
			delete(mm.reqToQuote, reqID)
		}
	}
}
