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

// quoteRef identifies one side of a symbol's quote. A live bid must not make
// the maker treat a missing ask as quoted, or vice versa.
type quoteRef struct {
	symbol string
	side   exchange.Side
}

var quoteSides = [...]exchange.Side{exchange.Buy, exchange.Sell}

type MarketMaker struct {
	*actor.BaseActor
	cfg          MMConfig
	mids         map[string]int64
	pending      map[quoteRef]map[uint64]bool
	reqToQuote   map[uint64]quoteRef // reqID → quote side
	orderToQuote map[uint64]quoteRef // orderID → quote side
	withdrawn    map[quoteRef]bool   // side has no inventory; wait for an opposite fill
	subscribed   map[string]bool
}

func NewMarketMaker(id uint64, gw actor.Gateway, cfg MMConfig) *MarketMaker {
	mm := &MarketMaker{
		BaseActor:    actor.NewBaseActor(id, gw),
		cfg:          cfg,
		mids:         make(map[string]int64, len(cfg.Symbols)),
		pending:      make(map[quoteRef]map[uint64]bool, len(cfg.Symbols)*len(quoteSides)),
		reqToQuote:   make(map[uint64]quoteRef),
		orderToQuote: make(map[uint64]quoteRef),
		withdrawn:    make(map[quoteRef]bool),
		subscribed:   make(map[string]bool, len(cfg.Symbols)),
	}
	for _, sym := range cfg.Symbols {
		mm.mids[sym] = cfg.BootstrapPrice
		for _, side := range quoteSides {
			mm.pending[quoteRef{symbol: sym, side: side}] = make(map[uint64]bool)
		}
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.RefreshInterval, mm.onTick)
	return mm
}

func (mm *MarketMaker) Mid(sym string) int64 { return mm.mids[sym] }
func (mm *MarketMaker) Symbols() []string    { return mm.cfg.Symbols }

func (mm *MarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
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

func (mm *MarketMaker) onAccepted(e actor.OrderAcceptedEvent) {
	ref, ok := mm.reqToQuote[e.RequestID]
	if !ok {
		// Request was cleared by cancelAllForSym before the accept arrived.
		// The order is live in the book but untracked — cancel it immediately.
		mm.CancelOrder(e.OrderID)
		return
	}
	delete(mm.reqToQuote, e.RequestID)
	mm.pending[ref][e.OrderID] = true
	mm.orderToQuote[e.OrderID] = ref
}

func (mm *MarketMaker) onFilled(e actor.OrderFillEvent) {
	ref, ok := mm.orderToQuote[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToQuote, e.OrderID)
	delete(mm.pending[ref], e.OrderID)
	mm.mids[ref.symbol] = e.Price
	// A buy replenishes base inventory for sells; a sell replenishes quote
	// inventory for buys. Keep a rejected side withdrawn until this happens.
	delete(mm.withdrawn, quoteRef{symbol: ref.symbol, side: oppositeSide(ref.side)})
	if !e.IsFull {
		mm.CancelOrder(e.OrderID) // cancel remaining qty of partially-filled order
	}
	mm.cancelAllForSym(ref.symbol)
	// The cancellation requests precede the replacement orders in the gateway
	// FIFO. Requote now so a same-timestamp taker/snapshot cannot observe an
	// artificial zero-latency liquidity gap.
	mm.quote(ref.symbol)
}

func (mm *MarketMaker) onCancelled(e actor.OrderCancelledEvent) {
	ref, ok := mm.orderToQuote[e.OrderID]
	if !ok {
		return
	}
	delete(mm.orderToQuote, e.OrderID)
	delete(mm.pending[ref], e.OrderID)
	mm.ensureQuoted(ref.symbol)
}

func (mm *MarketMaker) onRejected(e actor.OrderRejectedEvent) {
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

func (mm *MarketMaker) onTick(_ time.Time) {
	for _, sym := range mm.cfg.Symbols {
		if !mm.subscribed[sym] {
			mm.Subscribe(sym, exchange.MDSnapshot)
			mm.subscribed[sym] = true
		}
		mm.ensureQuoted(sym)
	}
}

func (mm *MarketMaker) ensureQuoted(sym string) {
	for _, side := range quoteSides {
		mm.quoteSide(quoteRef{symbol: sym, side: side})
	}
}

func (mm *MarketMaker) hasOutstanding(ref quoteRef) bool {
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

func (mm *MarketMaker) cancelAllForSym(sym string) {
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
	// Clear in-flight requests for this symbol so late accepts become orphans
	// handled by onAccepted, preventing ghost entries in pending.
	for reqID, ref := range mm.reqToQuote {
		if ref.symbol == sym {
			delete(mm.reqToQuote, reqID)
		}
	}
}

func (mm *MarketMaker) quote(sym string) {
	for _, side := range quoteSides {
		mm.quoteSide(quoteRef{symbol: sym, side: side})
	}
}

func (mm *MarketMaker) quoteSide(ref quoteRef) {
	if mm.withdrawn[ref] || mm.hasOutstanding(ref) {
		return
	}

	sym := ref.symbol
	mid := mm.mids[sym]
	for k := int64(1); k <= int64(mm.cfg.Levels); k++ {
		offset := (1 + (k-1)*mm.cfg.LevelSpacing) * mm.cfg.TickSize
		price := mid + offset
		if ref.side == exchange.Buy {
			price = mid - offset
		}
		if price <= 0 {
			continue
		}
		reqID := mm.SubmitOrder(sym, ref.side, exchange.LimitOrder, price, mm.cfg.LevelSize)
		mm.reqToQuote[reqID] = ref
	}
}

func oppositeSide(side exchange.Side) exchange.Side {
	if side == exchange.Buy {
		return exchange.Sell
	}
	return exchange.Buy
}
