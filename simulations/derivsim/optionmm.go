package derivsim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
)

// OptionMMConfig drives a market-opening option dealer: it discovers every
// listed option on Underlying from the instrument feed, quotes around the
// Black-76 theoretical value with an inventory premium skew (Stoikov-Saglam
// style), and delta-hedges its aggregate option inventory in the underlying.
// The hedger is the Frey-Stremme feedback channel: a net short-gamma book
// buys rallies and sells dips, amplifying underlying moves.
type OptionMMConfig struct {
	Underlying string
	IV         float64
	// SpreadBps is the half-spread around theo, in bps of the underlying price.
	SpreadBps int64
	// SkewPerLotBps shifts both quotes against inventory, in bps of the
	// underlying price per lot of net contract inventory.
	SkewPerLotBps int64
	QuoteQty      int64         // per side per contract
	LotQty        int64         // inventory normalization unit
	PremiumTick   int64         // option book tick
	QuoteInterval time.Duration // requote cadence

	// HedgeEnabled turns on delta hedging in the underlying.
	HedgeEnabled  bool
	HedgeInterval time.Duration
	// HedgeBandQty: rebalance only when |target − current| exceeds this.
	HedgeBandQty int64

	BasePrecision int64
}

type optionQuotes struct {
	bidID, askID       uint64
	bidPrice, askPrice int64
	inventory          int64 // signed contracts in base units
	// pendingBid/pendingAsk gate requoting while a submit is in flight:
	// re-submitting before the accept lands would orphan the unacked order
	// as an uncancellable zombie quote.
	pendingBid, pendingAsk bool
}

// quoteRef ties an in-flight quote request to its contract and book side so
// the accept can be routed back into per-contract quote state.
type quoteRef struct {
	sym   string
	isBid bool
}

// OptionMarketMaker opens and quotes every option listed on its underlying.
type OptionMarketMaker struct {
	*actor.BaseActor
	cfg OptionMMConfig

	set        *contractSet
	quotes     map[string]*optionQuotes
	pending    map[uint64]quoteRef
	spotMid    int64
	hedgePos   int64 // underlying inventory from hedge fills
	hedgeQty   int64 // cumulative |hedge| traded, for reporting
	subscribed bool
}

func NewOptionMarketMaker(id uint64, gw actor.Gateway, cfg OptionMMConfig) *OptionMarketMaker {
	mm := &OptionMarketMaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		quotes:    make(map[string]*optionQuotes),
		pending:   make(map[uint64]quoteRef),
	}
	mm.set.onList = func(c *Contract) {
		if c.Type == "OPTION" {
			mm.quotes[c.Symbol] = &optionQuotes{}
		}
	}
	mm.set.onSettle = func(c *Contract, _ int64) { delete(mm.quotes, c.Symbol) }
	mm.set.onFill = mm.onFill
	mm.set.onAccept = mm.onQuoteAccepted
	mm.set.onReject = mm.onQuoteRejected
	mm.SetHandler(mm)
	mm.AddTicker(cfg.QuoteInterval, mm.onQuoteTick)
	if cfg.HedgeEnabled {
		mm.AddTicker(cfg.HedgeInterval, mm.onHedgeTick)
	}
	return mm
}

// HedgeTraded reports cumulative hedge volume (base units) for diagnostics.
func (mm *OptionMarketMaker) HedgeTraded() int64 { return mm.hedgeQty }
func (mm *OptionMarketMaker) HedgePosition() int64 {
	return mm.hedgePos
}

func (mm *OptionMarketMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == mm.cfg.Underlying {
			if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
				mm.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
			}
		}
		return
	}
	mm.set.handle(evt)
}

// onQuoteAccepted records the live order ID for a quote so it can be
// cancelled on requote. An accept for a contract that settled in flight is
// orphan-cancelled immediately.
func (mm *OptionMarketMaker) onQuoteAccepted(_ string, reqID, orderID uint64) {
	ref, ok := mm.pending[reqID]
	if !ok {
		return // hedge order, tracked by fills only
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

func (mm *OptionMarketMaker) onQuoteRejected(reqID uint64) {
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

func (mm *OptionMarketMaker) onFill(sym string, e actor.OrderFillEvent) {
	if sym == mm.cfg.Underlying {
		mm.hedgePos += signedQty(e)
		mm.hedgeQty += e.Qty
		return
	}
	if q, ok := mm.quotes[sym]; ok {
		q.inventory += signedQty(e)
		if e.IsFull {
			if e.Side == exchange.Buy {
				q.bidID = 0
			} else {
				q.askID = 0
			}
		}
	}
}

func (mm *OptionMarketMaker) onQuoteTick(t time.Time) {
	if !mm.subscribed {
		mm.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		mm.Subscribe(mm.cfg.Underlying, exchange.MDSnapshot)
		mm.subscribed = true
		return
	}
	if mm.spotMid == 0 {
		return
	}
	now := t.UnixNano()
	for sym, c := range mm.set.contracts {
		if c.Type != "OPTION" {
			continue
		}
		q := mm.quotes[sym]
		if q == nil {
			continue
		}
		mm.requoteContract(sym, c, q, now)
	}
}

func (mm *OptionMarketMaker) requoteContract(sym string, c *Contract, q *optionQuotes, now int64) {
	yearsLeft := float64(c.ExpiryNano-now) / float64(365*24*time.Hour)
	if yearsLeft <= 0 {
		return
	}
	theo := eprice.Black76Premium(mm.spotMid, c.Strike, mm.cfg.IV, yearsLeft, c.IsCall)
	half := mm.spotMid * mm.cfg.SpreadBps / 10000
	skew := int64(0)
	if mm.cfg.LotQty > 0 {
		skew = mm.spotMid * mm.cfg.SkewPerLotBps / 10000 * q.inventory / mm.cfg.LotQty
	}
	tick := mm.cfg.PremiumTick
	bid := alignDown(theo-half-skew, tick)
	ask := alignUp(theo+half-skew, tick)
	if ask <= 0 {
		return
	}
	if q.pendingBid || q.pendingAsk {
		return
	}
	if bid == q.bidPrice && ask == q.askPrice && q.bidID != 0 && q.askID != 0 {
		return
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

// onHedgeTick trades the underlying toward delta neutrality of the whole book.
func (mm *OptionMarketMaker) onHedgeTick(t time.Time) {
	if mm.spotMid == 0 {
		return
	}
	now := t.UnixNano()
	var netDelta float64
	for sym, c := range mm.set.contracts {
		if c.Type != "OPTION" {
			continue
		}
		q := mm.quotes[sym]
		if q == nil || q.inventory == 0 {
			continue
		}
		yearsLeft := float64(c.ExpiryNano-now) / float64(365*24*time.Hour)
		delta := eprice.Black76Delta(mm.spotMid, c.Strike, mm.cfg.IV, yearsLeft, c.IsCall)
		netDelta += delta * float64(q.inventory)
	}
	target := -int64(netDelta) // hold −Δ of underlying to neutralize
	gap := target - mm.hedgePos
	if gap > mm.cfg.HedgeBandQty {
		reqID := mm.SubmitOrder(mm.cfg.Underlying, exchange.Buy, exchange.Market, 0, gap)
		mm.set.trackRequest(reqID, mm.cfg.Underlying)
	} else if gap < -mm.cfg.HedgeBandQty {
		reqID := mm.SubmitOrder(mm.cfg.Underlying, exchange.Sell, exchange.Market, 0, -gap)
		mm.set.trackRequest(reqID, mm.cfg.Underlying)
	}
}

func alignDown(price, tick int64) int64 {
	if price <= 0 {
		return 0
	}
	return (price / tick) * tick
}

func alignUp(price, tick int64) int64 {
	if price <= 0 {
		return tick
	}
	return ((price + tick - 1) / tick) * tick
}
