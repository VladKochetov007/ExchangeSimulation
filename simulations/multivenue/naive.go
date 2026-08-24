package multivenue

import (
	"context"
	"sort"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// This file holds the "naive" strategies described in publicly written trading
// newsletters, implemented so they can compete against each other and against
// the inventory-managing makers already in the population. None of them is
// given information the others lack: each sees only the books it subscribes to.

// FixedDistanceMakerConfig is the simplest market maker described in Vertox's
// "Market Making - How to start": quote a fixed distance either side of the
// midpoint and requote only once the midpoint has moved far enough to matter.
// It manages inventory by refusing to add beyond a cap rather than by skewing,
// which is exactly what makes it a useful control against the Stoikov maker.
type FixedDistanceMakerConfig struct {
	Symbol string `json:"symbol"`
	// SpreadBps is the half-spread quoted either side of the midpoint.
	SpreadBps int64 `json:"spread_bps"`
	// RequoteBps is how far the midpoint must move before quotes are replaced.
	// Requoting on every tick is what the article warns never gets you filled.
	RequoteBps    int64         `json:"requote_bps"`
	QuoteQty      int64         `json:"quote_qty"`
	MaxInventory  int64         `json:"max_inventory"`
	QuoteInterval time.Duration `json:"quote_interval"`
	TickSize      int64         `json:"-"`
	// PostOnly makes every refreshed quote a named passive venue request.
	PostOnly bool `json:"post_only"`
	// PostOnlyCancelBeforeReplace selects cancellation-before-replacement for
	// the P0 ordering treatment. False preserves the legacy send order.
	PostOnlyCancelBeforeReplace bool `json:"post_only_cancel_before_replace"`
}

type quoteState struct {
	bidID, askID       uint64
	bidPrice, askPrice int64
	pendingBid         bool
	pendingAsk         bool
}

// FixedDistanceMaker quotes a constant spread around the midpoint it observes.
type FixedDistanceMaker struct {
	*actor.BaseActor
	cfg        FixedDistanceMakerConfig
	quotes     quoteState
	pending    map[uint64]bool // request id -> isBid
	bestBid    int64
	bestAsk    int64
	quotedMid  int64
	inventory  int64
	subscribed bool
}

func NewFixedDistanceMaker(id uint64, gw actor.Gateway, cfg FixedDistanceMakerConfig) *FixedDistanceMaker {
	m := newFixedDistanceCore(id, gw, cfg)
	m.SetHandler(m)
	m.AddTicker(cfg.QuoteInterval, m.onTick)
	return m
}

// newFixedDistanceCore builds the shared quoting machinery without registering
// a ticker or a handler, so a strategy that extends it can own both.
func newFixedDistanceCore(id uint64, gw actor.Gateway, cfg FixedDistanceMakerConfig) *FixedDistanceMaker {
	return &FixedDistanceMaker{BaseActor: actor.NewBaseActor(id, gw), cfg: cfg, pending: make(map[uint64]bool)}
}

// Inventory is the maker's signed position in base units.
func (m *FixedDistanceMaker) Inventory() int64 { return m.inventory }

func (m *FixedDistanceMaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol != m.cfg.Symbol {
			return
		}
		m.bestBid, m.bestAsk = 0, 0
		if len(e.Snapshot.Bids) > 0 {
			m.bestBid = e.Snapshot.Bids[0].Price
		}
		if len(e.Snapshot.Asks) > 0 {
			m.bestAsk = e.Snapshot.Asks[0].Price
		}
	case actor.EventOrderAccepted:
		e := evt.Data.(actor.OrderAcceptedEvent)
		isBid, tracked := m.pending[e.RequestID]
		if !tracked {
			m.CancelOrder(e.OrderID)
			return
		}
		delete(m.pending, e.RequestID)
		if isBid {
			m.quotes.bidID, m.quotes.pendingBid = e.OrderID, false
		} else {
			m.quotes.askID, m.quotes.pendingAsk = e.OrderID, false
		}
	case actor.EventOrderRejected:
		e := evt.Data.(actor.OrderRejectedEvent)
		if isBid, tracked := m.pending[e.RequestID]; tracked {
			delete(m.pending, e.RequestID)
			if isBid {
				m.quotes.pendingBid = false
			} else {
				m.quotes.pendingAsk = false
			}
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		e := evt.Data.(actor.OrderFillEvent)
		if e.Side == exchange.Buy {
			m.inventory += e.Qty
		} else {
			m.inventory -= e.Qty
		}
		if e.IsFull {
			m.forget(e.OrderID)
		}
	case actor.EventOrderCancelled:
		m.forget(evt.Data.(actor.OrderCancelledEvent).OrderID)
	}
}

func (m *FixedDistanceMaker) forget(orderID uint64) {
	if orderID == m.quotes.bidID {
		m.quotes.bidID = 0
	}
	if orderID == m.quotes.askID {
		m.quotes.askID = 0
	}
}

func (m *FixedDistanceMaker) onTick(time.Time) {
	if !m.subscribed {
		m.Subscribe(m.cfg.Symbol, exchange.MDSnapshot)
		m.subscribed = true
		return
	}
	if len(m.pending) != 0 {
		return
	}
	mid, available := positiveDomainTwoSidedMidpoint(m.bestBid, m.bestAsk)
	if !available || m.bestAsk == m.bestBid {
		return
	}
	// A maker that has been filled has no quote on that side, and waiting for
	// the mid to move before replacing it leaves the book one-sided for as
	// long as the market is calm — which is exactly when nothing will move the
	// mid. Requote whenever a side it intends to quote is missing.
	if m.quotesIntact(mid) && m.quotedMid > 0 && abs64(mid-m.quotedMid)*10000 < m.cfg.RequoteBps*m.quotedMid {
		return
	}
	m.replace(mid, m.bidTarget(mid), m.askTarget(mid))
}

// quotesIntact reports whether every side this maker wants to show is live.
func (m *FixedDistanceMaker) quotesIntact(mid int64) bool {
	if m.bidTarget(mid) > 0 && m.quotes.bidID == 0 {
		return false
	}
	return !(m.askTarget(mid) > 0 && m.quotes.askID == 0)
}

func (m *FixedDistanceMaker) bidTarget(mid int64) int64 {
	if m.inventory >= m.cfg.MaxInventory {
		return 0
	}
	return alignTo(mid-mid*m.cfg.SpreadBps/10000, m.cfg.TickSize, false)
}

func (m *FixedDistanceMaker) askTarget(mid int64) int64 {
	if m.inventory <= -m.cfg.MaxInventory {
		return 0
	}
	return alignTo(mid+mid*m.cfg.SpreadBps/10000, m.cfg.TickSize, true)
}

// replace keeps the legacy submit-before-cancel order unless the explicit P0
// cancel-before-replace arm is selected for a post-only maker.
func (m *FixedDistanceMaker) replace(mid, bid, ask int64) {
	previousBid, previousAsk := m.quotes.bidID, m.quotes.askID
	if m.cfg.PostOnly && m.cfg.PostOnlyCancelBeforeReplace {
		if previousBid != 0 {
			m.CancelOrder(previousBid)
		}
		if previousAsk != 0 {
			m.CancelOrder(previousAsk)
		}
	}
	m.quotes.bidID, m.quotes.askID = 0, 0
	m.quotedMid = mid
	if bid > 0 {
		reqID := m.submitQuote(exchange.Buy, bid)
		m.pending[reqID] = true
		m.quotes.pendingBid, m.quotes.bidPrice = true, bid
	}
	if ask > bid && ask > 0 {
		reqID := m.submitQuote(exchange.Sell, ask)
		m.pending[reqID] = false
		m.quotes.pendingAsk, m.quotes.askPrice = true, ask
	}
	if !(m.cfg.PostOnly && m.cfg.PostOnlyCancelBeforeReplace) && previousBid != 0 {
		m.CancelOrder(previousBid)
	}
	if !(m.cfg.PostOnly && m.cfg.PostOnlyCancelBeforeReplace) && previousAsk != 0 {
		m.CancelOrder(previousAsk)
	}
}

func (m *FixedDistanceMaker) submitQuote(side exchange.Side, price int64) uint64 {
	if m.cfg.PostOnly {
		return m.SubmitPostOnlyOrder(m.cfg.Symbol, side, price, m.cfg.QuoteQty)
	}
	return m.SubmitOrder(m.cfg.Symbol, side, exchange.LimitOrder, price, m.cfg.QuoteQty)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func alignTo(price, tick int64, up bool) int64 {
	if tick <= 0 || price <= 0 {
		return price
	}
	if up {
		return ((price + tick - 1) / tick) * tick
	}
	return (price / tick) * tick
}

// ImbalanceMakerConfig adds the microstructural alpha Vertox describes: the
// order-book imbalance I = (B - A) / (B + A) at the touch, where B and A are
// the resting sizes. Positive imbalance predicts the midpoint drifting up, so
// the maker leans its quotes rather than quoting symmetrically. This is the
// same fixed-distance maker plus one signal, which is what makes the pair a
// clean test of whether the signal is worth anything.
type ImbalanceMakerConfig struct {
	FixedDistanceMakerConfig
	// LeanBps is how far the pair of quotes is shifted at full imbalance.
	LeanBps int64 `json:"lean_bps"`
	// SuppressAt withdraws the quote on the side the imbalance predicts will be
	// run over. Zero keeps both sides quoted at all times.
	SuppressAt float64 `json:"suppress_at"`
}

// ImbalanceMaker leans its quotes with observed book imbalance.
type ImbalanceMaker struct {
	*FixedDistanceMaker
	cfg       ImbalanceMakerConfig
	bidQty    int64
	askQty    int64
	imbalance float64
}

func NewImbalanceMaker(id uint64, gw actor.Gateway, cfg ImbalanceMakerConfig) *ImbalanceMaker {
	m := &ImbalanceMaker{FixedDistanceMaker: newFixedDistanceCore(id, gw, cfg.FixedDistanceMakerConfig), cfg: cfg}
	m.SetHandler(m)
	m.AddTicker(cfg.QuoteInterval, m.onImbalanceTick)
	return m
}

// Imbalance is the most recent touch imbalance, in [-1, 1].
func (m *ImbalanceMaker) Imbalance() float64 { return m.imbalance }

func (m *ImbalanceMaker) HandleEvent(ctx context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		if e := evt.Data.(actor.BookSnapshotEvent); e.Symbol == m.cfg.Symbol {
			m.bidQty, m.askQty = 0, 0
			if len(e.Snapshot.Bids) > 0 {
				m.bidQty = e.Snapshot.Bids[0].VisibleQty
			}
			if len(e.Snapshot.Asks) > 0 {
				m.askQty = e.Snapshot.Asks[0].VisibleQty
			}
			if total := m.bidQty + m.askQty; total > 0 {
				m.imbalance = float64(m.bidQty-m.askQty) / float64(total)
			} else {
				m.imbalance = 0
			}
		}
	}
	m.FixedDistanceMaker.HandleEvent(ctx, evt)
}

func (m *ImbalanceMaker) onImbalanceTick(time.Time) {
	if !m.subscribed {
		m.Subscribe(m.cfg.Symbol, exchange.MDSnapshot)
		m.subscribed = true
		return
	}
	if len(m.pending) != 0 {
		return
	}
	mid, available := positiveDomainTwoSidedMidpoint(m.bestBid, m.bestAsk)
	if !available || m.bestAsk == m.bestBid {
		return
	}
	if m.quotesIntact(mid) && m.quotedMid > 0 && abs64(mid-m.quotedMid)*10000 < m.cfg.RequoteBps*m.quotedMid {
		return
	}
	// Lean the whole quote pair toward the side the imbalance favours.
	lean := int64(float64(mid*m.cfg.LeanBps/10000) * m.imbalance)
	bid, ask := m.bidTarget(mid), m.askTarget(mid)
	if bid > 0 {
		bid = alignTo(bid+lean, m.cfg.TickSize, false)
	}
	if ask > 0 {
		ask = alignTo(ask+lean, m.cfg.TickSize, true)
	}
	if m.cfg.SuppressAt > 0 {
		// Withdraw the side that is about to be run over.
		if m.imbalance >= m.cfg.SuppressAt {
			ask = 0
		} else if m.imbalance <= -m.cfg.SuppressAt {
			bid = 0
		}
	}
	m.replace(mid, bid, ask)
}

// TriangleArbConfig implements the taker leg of the strategy Quant Arb calls
// TriArb: walk a loop of three books back to where you started and take it when
// the round trip prints more than it costs. The venue lists ABC/USD, CDF/USD
// and ABC/CDF, which is exactly such a loop.
//
// The article's point that the loop must be crossed with fewer than three legs
// only by accident is why all three are sent together: holding one leg and
// waiting is a directional bet, not an arbitrage.
type TriangleArbConfig struct {
	// Legs are the three symbols forming the loop, quoted as base/quote.
	BaseQuote  string `json:"base_quote"`  // ABC/USD
	CrossQuote string `json:"cross_quote"` // CDF/USD
	BaseCross  string `json:"base_cross"`  // ABC/CDF
	// EdgeBps is the round-trip profit required before firing, which must at
	// least cover the taker fee on all three legs.
	EdgeBps int64 `json:"edge_bps"`
	// TakerFeeBps is what each leg of the loop pays. A triangular loop crosses
	// three books, so the round trip costs three times this, and an edge below
	// that cost is not an opportunity. Without it the desk fires on a
	// configured trigger regardless of what trading costs: measured over eight
	// hours it traded identically at 2, 5 and 10 bps of fee, earning 3527 USD,
	// losing 59 and losing 6522, and the cross-rate bound never moved.
	TakerFeeBps int64 `json:"taker_fee_bps"`
	// Legs is how many books the loop crosses. Zero means three.
	Legs          int64         `json:"legs"`
	LotQty        int64         `json:"lot_qty"`
	CheckInterval time.Duration `json:"check_interval"`
	MaxFirings    int64         `json:"max_firings"`
}

type touchPrices struct{ bid, ask int64 }

// TriangleArbTaker crosses all three books when the loop is profitable.
type TriangleArbTaker struct {
	*actor.BaseActor
	cfg        TriangleArbConfig
	books      map[string]*touchPrices
	firings    int64
	subscribed bool
}

func NewTriangleArbTaker(id uint64, gw actor.Gateway, cfg TriangleArbConfig) *TriangleArbTaker {
	t := &TriangleArbTaker{BaseActor: actor.NewBaseActor(id, gw), cfg: cfg, books: map[string]*touchPrices{
		cfg.BaseQuote: {}, cfg.CrossQuote: {}, cfg.BaseCross: {},
	}}
	t.SetHandler(t)
	t.AddTicker(cfg.CheckInterval, t.onTick)
	return t
}

// Firings reports how many loops the desk attempted.
func (t *TriangleArbTaker) Firings() int64 { return t.firings }

func (t *TriangleArbTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type != actor.EventBookSnapshot {
		return
	}
	e := evt.Data.(actor.BookSnapshotEvent)
	book, tracked := t.books[e.Symbol]
	if !tracked {
		return
	}
	book.bid, book.ask = 0, 0
	if len(e.Snapshot.Bids) > 0 {
		book.bid = e.Snapshot.Bids[0].Price
	}
	if len(e.Snapshot.Asks) > 0 {
		book.ask = e.Snapshot.Asks[0].Price
	}
}

func (t *TriangleArbTaker) onTick(time.Time) {
	if !t.subscribed {
		// Symbol order: each Subscribe puts the book's first snapshot into this
		// desk's inbox, so subscribing in map order decides which leg it sees
		// first and therefore what it does with its first tick.
		symbols := make([]string, 0, len(t.books))
		for symbol := range t.books {
			symbols = append(symbols, symbol)
		}
		sort.Strings(symbols)
		for _, symbol := range symbols {
			t.Subscribe(symbol, exchange.MDSnapshot)
		}
		t.subscribed = true
		return
	}
	if t.cfg.MaxFirings > 0 && t.firings >= t.cfg.MaxFirings {
		return
	}
	baseQuote, crossQuote, baseCross := t.books[t.cfg.BaseQuote], t.books[t.cfg.CrossQuote], t.books[t.cfg.BaseCross]
	if !quotable(baseQuote) || !quotable(crossQuote) || !quotable(baseCross) {
		return
	}

	// Forward loop: spend quote on base, sell base for cross, sell cross for
	// quote. Profitable when the implied cross rate beats the direct one.
	implied := float64(baseCross.bid) * float64(crossQuote.bid)
	direct := float64(baseQuote.ask)
	if gainBps(implied/float64(quoteUnit), direct) >= t.requiredEdgeBps() {
		t.fire(t.cfg.BaseQuote, exchange.Buy)
		t.fire(t.cfg.BaseCross, exchange.Sell)
		t.fire(t.cfg.CrossQuote, exchange.Sell)
		t.firings++
		return
	}

	// Reverse loop: buy cross with quote, buy base with cross, sell base for quote.
	impliedReverse := float64(baseCross.ask) * float64(crossQuote.ask)
	if gainBps(float64(baseQuote.bid), impliedReverse/float64(quoteUnit)) >= t.requiredEdgeBps() {
		t.fire(t.cfg.CrossQuote, exchange.Buy)
		t.fire(t.cfg.BaseCross, exchange.Buy)
		t.fire(t.cfg.BaseQuote, exchange.Sell)
		t.firings++
	}
}

// requiredEdgeBps is the gain a loop must show before it is worth firing: the
// configured edge plus the cost of crossing every leg.
func (t *TriangleArbTaker) requiredEdgeBps() float64 {
	legs := t.cfg.Legs
	if legs <= 0 {
		legs = 3
	}
	return float64(t.cfg.EdgeBps + legs*t.cfg.TakerFeeBps)
}

// quoteUnit converts the ABC/CDF price, which is quoted in CDF units, into the
// same scale as a USD price when multiplied by CDF/USD.
const quoteUnit = mvBasePrecision

func quotable(book *touchPrices) bool { return book != nil && book.bid > 0 && book.ask > 0 }

func gainBps(received, paid float64) float64 {
	if paid <= 0 {
		return 0
	}
	return (received/paid - 1) * 10000
}

func (t *TriangleArbTaker) fire(symbol string, side exchange.Side) {
	t.SubmitOrder(symbol, side, exchange.Market, 0, t.cfg.LotQty)
}
