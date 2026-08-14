package derivsim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// FuturesMMConfig drives a market opener for dated futures: it discovers each
// listed contract and quotes it around the underlying mid, so fresh contracts
// have a book from their first minute. A naive (carry-free) quote is exactly
// what makes the cash-and-carry arb's job observable.
type FuturesMMConfig struct {
	Underlying    string
	SpreadBps     int64
	QuoteQty      int64
	Tick          int64
	QuoteInterval time.Duration
	// SelfAnchored quotes each future around its own last trade (bootstrapped
	// at spot on listing) instead of pegging to the spot mid. This lets the
	// futures price wander on its own flow, so any basis convergence must
	// come from arbitrage rather than from the quoting rule.
	SelfAnchored bool
}

type futQuotes struct {
	bidID, askID       uint64
	bidPrice, askPrice int64
	anchor             int64 // last trade (self-anchored mode)
	// pending gates requoting while submits are in flight (see optionQuotes).
	pendingBid, pendingAsk bool
}

// FuturesMarketMaker quotes every live dated future on its underlying.
type FuturesMarketMaker struct {
	*actor.BaseActor
	cfg        FuturesMMConfig
	set        *contractSet
	quotes     map[string]*futQuotes
	pending    map[uint64]quoteRef
	spotMid    int64
	subscribed bool
}

func NewFuturesMarketMaker(id uint64, gw actor.Gateway, cfg FuturesMMConfig) *FuturesMarketMaker {
	mm := &FuturesMarketMaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		quotes:    make(map[string]*futQuotes),
		pending:   make(map[uint64]quoteRef),
	}
	mm.set.onList = func(c *Contract) {
		if c.Type == "FUTURE" {
			mm.quotes[c.Symbol] = &futQuotes{}
			if cfg.SelfAnchored {
				mm.Subscribe(c.Symbol, exchange.MDTrade)
			}
		}
	}
	mm.set.onSettle = func(c *Contract, _ int64) { delete(mm.quotes, c.Symbol) }
	mm.set.onFill = func(sym string, e actor.OrderFillEvent) {
		if q, ok := mm.quotes[sym]; ok && e.IsFull {
			if e.Side == exchange.Buy {
				q.bidID = 0
			} else {
				q.askID = 0
			}
		}
	}
	mm.set.onAccept = func(_ string, reqID, orderID uint64) {
		ref, ok := mm.pending[reqID]
		if !ok {
			return
		}
		delete(mm.pending, reqID)
		q := mm.quotes[ref.sym]
		if q == nil {
			mm.CancelOrder(orderID)
			return
		}
		if ref.isBid {
			q.bidID = orderID
			q.pendingBid = false
		} else {
			q.askID = orderID
			q.pendingAsk = false
		}
	}
	mm.set.onReject = func(reqID uint64) {
		ref, ok := mm.pending[reqID]
		if !ok {
			return
		}
		delete(mm.pending, reqID)
		if q := mm.quotes[ref.sym]; q != nil {
			if ref.isBid {
				q.pendingBid = false
			} else {
				q.pendingAsk = false
			}
		}
	}
	mm.SetHandler(mm)
	mm.AddTicker(cfg.QuoteInterval, mm.onTick)
	return mm
}

func (mm *FuturesMarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == mm.cfg.Underlying && len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
			mm.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
		}
		return
	case actor.EventTrade:
		e := evt.Data.(actor.TradeEvent)
		if q, ok := mm.quotes[e.Symbol]; ok {
			q.anchor = e.Trade.Price
		}
		return
	}
	mm.set.handle(evt)
}

func (mm *FuturesMarketMaker) onTick(_ time.Time) {
	if !mm.subscribed {
		mm.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		mm.Subscribe(mm.cfg.Underlying, exchange.MDSnapshot)
		mm.subscribed = true
		return
	}
	if mm.spotMid == 0 {
		return
	}
	half := mm.spotMid * mm.cfg.SpreadBps / 10000
	for _, contract := range mm.set.orderedContracts() {
		sym := contract.Symbol
		q := mm.quotes[sym]
		if q == nil {
			continue
		}
		mid := mm.spotMid
		if mm.cfg.SelfAnchored && q.anchor > 0 {
			mid = q.anchor
		}
		bid := alignDown(mid-half, mm.cfg.Tick)
		ask := alignUp(mid+half, mm.cfg.Tick)
		if q.pendingBid || q.pendingAsk {
			continue
		}
		if bid == q.bidPrice && ask == q.askPrice && q.bidID != 0 && q.askID != 0 {
			continue
		}
		if q.bidID != 0 {
			mm.CancelOrder(q.bidID)
			q.bidID = 0
		}
		if q.askID != 0 {
			mm.CancelOrder(q.askID)
			q.askID = 0
		}
		if bid > 0 {
			reqID := mm.SubmitOrder(sym, exchange.Buy, exchange.LimitOrder, bid, mm.cfg.QuoteQty)
			mm.set.trackRequest(reqID, sym)
			mm.pending[reqID] = quoteRef{sym: sym, isBid: true}
			q.pendingBid = true
			q.bidPrice = bid
		}
		reqID := mm.SubmitOrder(sym, exchange.Sell, exchange.LimitOrder, ask, mm.cfg.QuoteQty)
		mm.set.trackRequest(reqID, sym)
		mm.pending[reqID] = quoteRef{sym: sym, isBid: false}
		q.pendingAsk = true
		q.askPrice = ask
	}
}
