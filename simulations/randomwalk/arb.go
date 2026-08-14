package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

type BasisArbConfig struct {
	SpotSymbol      string
	PerpSymbol      string
	SpotTakerFeeBps int64 // charged on the spot market-order leg
	PerpTakerFeeBps int64 // charged on the perp market-order leg
	ThresholdBps    int64 // minimum net edge, after both taker fees
	LotSize         int64 // qty per individual trade
	MaxPosition     int64 // max abs position in lots
}

// BasisArbActor arbitrages the executable spot/perp basis. A basis position
// is recorded only after both legs of a pair have filled in full. Market
// orders can be rejected or partially filled, so a failed pair is explicitly
// unwound before the actor can submit another pair.
//
// position > 0 means short perp / long spot; position < 0 means long perp /
// short spot. It measures completed, matched lots rather than submitted
// intent. The actual fills of a failed pair are kept in basisPair until the
// compensating orders have neutralized them.
type BasisArbActor struct {
	*actor.BaseActor
	cfg BasisArbConfig

	spotBook basisBookTop
	perpBook basisBookTop

	position int64
	pair     *basisPair

	// A coherent snapshot is a decision opportunity only once. Without this
	// guard, a pair which settles before the next 100ms tick can repeatedly
	// trade the same old two-book view.
	lastCheckedSnapshotTimestamp int64
	hasCheckedSnapshot           bool
	subscribed                   bool
}

type basisBookTop struct {
	bid       int64
	ask       int64
	timestamp int64
	received  bool
}

// basisLeg contains one original market order. hedged is the quantity of its
// own fill that has later been offset by compensating orders; preserving this
// per-leg attribution lets a partial pair return exactly to its prior state.
type basisLeg struct {
	symbol string
	side   exchange.Side
	qty    int64

	requestID uint64
	orderID   uint64
	filled    int64
	terminal  bool

	hedged     int64
	hedgeOrder *basisOrder
}

type basisOrder struct {
	requestID uint64
	orderID   uint64
	symbol    string
	side      exchange.Side
	qty       int64
	filled    int64
	terminal  bool

	// source is nil for original orders. A non-nil source makes this a
	// compensating order for that original leg's observed fill.
	source *basisLeg
}

type basisPair struct {
	positionDelta int64
	perp          *basisLeg
	spot          *basisLeg
	recovering    bool
	poisoned      bool
	ordersByReq   map[uint64]*basisOrder
	ordersByID    map[uint64]*basisOrder
}

func NewBasisArbActor(id uint64, gw actor.Gateway, cfg BasisArbConfig) *BasisArbActor {
	a := &BasisArbActor{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
	}
	a.SetHandler(a)
	a.AddTicker(100*time.Millisecond, a.onTick)
	return a
}

func (a *BasisArbActor) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		a.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		a.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderRejected:
		a.onRejected(evt.Data.(actor.OrderRejectedEvent))
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		a.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderCancelled:
		a.onCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

func (a *BasisArbActor) onSnapshot(e actor.BookSnapshotEvent) {
	var book *basisBookTop
	switch e.Symbol {
	case a.cfg.SpotSymbol:
		book = &a.spotBook
	case a.cfg.PerpSymbol:
		book = &a.perpBook
	default:
		return
	}

	// An empty update invalidates the old top. Retaining a previous quote would
	// manufacture executable liquidity after the book has been withdrawn.
	book.bid, book.ask = 0, 0
	if len(e.Snapshot.Bids) > 0 {
		book.bid = e.Snapshot.Bids[0].Price
	}
	if len(e.Snapshot.Asks) > 0 {
		book.ask = e.Snapshot.Asks[0].Price
	}
	book.timestamp = e.Timestamp
	book.received = true
}

func (a *BasisArbActor) onTick(_ time.Time) {
	if !a.subscribed {
		a.Subscribe(a.cfg.SpotSymbol, exchange.MDSnapshot)
		a.Subscribe(a.cfg.PerpSymbol, exchange.MDSnapshot)
		a.subscribed = true
	}
	a.checkBasis()
}

// checkBasis only acts on the best prices a taker can actually execute: sell
// at a bid and buy at an ask. Last trades and book mids are deliberately not
// used because neither is a tradable two-leg price.
func (a *BasisArbActor) checkBasis() {
	if !a.coherentBooks() {
		return
	}

	timestamp := a.spotBook.timestamp
	if a.hasCheckedSnapshot && timestamp == a.lastCheckedSnapshotTimestamp {
		return
	}
	a.lastCheckedSnapshotTimestamp = timestamp
	a.hasCheckedSnapshot = true

	// A new book can make a previously unfilled compensating order executable.
	// It must never start a fresh basis pair until all residual exposure has
	// been offset or explicitly remains unresolved.
	if a.pair != nil {
		if a.pair.poisoned {
			return
		}
		if a.pair.recovering {
			a.tryNeutralize()
		}
		return
	}

	positiveEdge, positiveThreshold, positiveOK := a.netEdge(
		a.perpBook.bid, a.cfg.PerpTakerFeeBps,
		a.spotBook.ask, a.cfg.SpotTakerFeeBps,
	)
	negativeEdge, negativeThreshold, negativeOK := a.netEdge(
		a.spotBook.bid, a.cfg.SpotTakerFeeBps,
		a.perpBook.ask, a.cfg.PerpTakerFeeBps,
	)

	// Closing a completed position uses the same executable opening edge that
	// justified it. Once that edge has decayed below half the configured net
	// threshold, submit the opposite two-leg trade to flatten one lot.
	switch {
	case a.position > 0 && positiveOK && positiveEdge <= positiveThreshold/2:
		a.openPair(-1, exchange.Buy, exchange.Sell)
	case a.position < 0 && negativeOK && negativeEdge <= negativeThreshold/2:
		a.openPair(1, exchange.Sell, exchange.Buy)
	case positiveOK && positiveEdge > positiveThreshold && a.position < a.cfg.MaxPosition:
		a.openPair(1, exchange.Sell, exchange.Buy)
	case negativeOK && negativeEdge > negativeThreshold && a.position > -a.cfg.MaxPosition:
		a.openPair(-1, exchange.Buy, exchange.Sell)
	}
}

func (a *BasisArbActor) coherentBooks() bool {
	return a.cfg.LotSize > 0 &&
		a.cfg.MaxPosition >= 0 &&
		a.cfg.ThresholdBps >= 0 &&
		a.cfg.SpotTakerFeeBps >= 0 &&
		a.cfg.PerpTakerFeeBps >= 0 &&
		a.spotBook.received && a.perpBook.received &&
		a.spotBook.timestamp == a.perpBook.timestamp &&
		a.spotBook.bid > 0 && a.spotBook.ask > 0 &&
		a.perpBook.bid > 0 && a.perpBook.ask > 0
}

// netEdge returns the quote-currency profit for selling LotSize at sellPrice
// and buying it at buyPrice after the exact percentage fees used by the
// exchange. threshold is the configured minimum edge expressed against the
// purchase notional. A false result means the values cannot be represented
// safely, so the actor conservatively does not trade.
func (a *BasisArbActor) netEdge(sellPrice, sellFeeBps, buyPrice, buyFeeBps int64) (edge, threshold int64, ok bool) {
	sellNotional, ok := etypes.TryMulDiv(a.cfg.LotSize, sellPrice, btcPrecision)
	if !ok {
		return 0, 0, false
	}
	buyNotional, ok := etypes.TryMulDiv(a.cfg.LotSize, buyPrice, btcPrecision)
	if !ok {
		return 0, 0, false
	}
	sellFee, ok := etypes.TryMulBps(sellNotional, sellFeeBps)
	if !ok {
		return 0, 0, false
	}
	buyFee, ok := etypes.TryMulBps(buyNotional, buyFeeBps)
	if !ok {
		return 0, 0, false
	}
	netProceeds, ok := etypes.TrySub(sellNotional, sellFee)
	if !ok {
		return 0, 0, false
	}
	cost, ok := etypes.TryAdd(buyNotional, buyFee)
	if !ok {
		return 0, 0, false
	}
	edge, ok = etypes.TrySub(netProceeds, cost)
	if !ok {
		return 0, 0, false
	}
	threshold, ok = etypes.TryMulBps(buyNotional, a.cfg.ThresholdBps)
	return edge, threshold, ok
}

func (a *BasisArbActor) openPair(positionDelta int64, perpSide, spotSide exchange.Side) {
	pair := &basisPair{
		positionDelta: positionDelta,
		ordersByReq:   make(map[uint64]*basisOrder, 2),
		ordersByID:    make(map[uint64]*basisOrder, 2),
	}
	pair.perp = &basisLeg{symbol: a.cfg.PerpSymbol, side: perpSide, qty: a.cfg.LotSize}
	pair.spot = &basisLeg{symbol: a.cfg.SpotSymbol, side: spotSide, qty: a.cfg.LotSize}
	a.pair = pair // establishes the in-flight gate before either Send call.
	a.submitOriginal(pair.perp)
	a.submitOriginal(pair.spot)
}

func (a *BasisArbActor) submitOriginal(leg *basisLeg) {
	requestID := a.SubmitOrder(leg.symbol, leg.side, exchange.Market, 0, leg.qty)
	leg.requestID = requestID
	a.pair.ordersByReq[requestID] = &basisOrder{
		requestID: requestID,
		symbol:    leg.symbol,
		side:      leg.side,
		qty:       leg.qty,
	}
}

func (a *BasisArbActor) onAccepted(e actor.OrderAcceptedEvent) {
	if a.pair == nil {
		return
	}
	order := a.pair.ordersByReq[e.RequestID]
	if order == nil || order.terminal {
		return
	}
	order.orderID = e.OrderID
	a.pair.ordersByID[e.OrderID] = order
	if order.source == nil {
		if leg := a.originalLegFor(order); leg != nil {
			leg.orderID = e.OrderID
		}
	}
}

func (a *BasisArbActor) onRejected(e actor.OrderRejectedEvent) {
	if a.pair == nil {
		return
	}
	order := a.pair.ordersByReq[e.RequestID]
	if order == nil || order.terminal {
		return
	}
	order.terminal = true
	a.finishOrder(order)
}

func (a *BasisArbActor) onFilled(e actor.OrderFillEvent) {
	if a.pair == nil {
		return
	}
	order := a.pair.ordersByID[e.OrderID]
	if order == nil || order.terminal || order.symbol != e.Symbol || order.side != e.Side || e.Qty <= 0 {
		return
	}
	filled, ok := etypes.TryAdd(order.filled, e.Qty)
	if !ok || filled > order.qty {
		// A malformed fill is not an opportunity to invent inventory. Keep the
		// pair blocked for operator diagnosis instead of letting it trade again.
		a.pair.poisoned = true
		return
	}
	order.filled = filled
	if order.source == nil {
		if leg := a.originalLegFor(order); leg != nil {
			leg.filled = filled
		}
	} else {
		hedged, ok := etypes.TryAdd(order.source.hedged, e.Qty)
		if !ok || hedged > order.source.filled {
			a.pair.poisoned = true
			return
		}
		order.source.hedged = hedged
	}
	if e.IsFull {
		order.terminal = true
		a.finishOrder(order)
	}
}

func (a *BasisArbActor) onCancelled(e actor.OrderCancelledEvent) {
	if a.pair == nil {
		return
	}
	order := a.pair.ordersByID[e.OrderID]
	if order == nil || order.terminal {
		return
	}
	order.terminal = true
	a.finishOrder(order)
}

func (a *BasisArbActor) finishOrder(order *basisOrder) {
	delete(a.pair.ordersByReq, order.requestID)
	if order.orderID != 0 {
		delete(a.pair.ordersByID, order.orderID)
	}

	if order.source != nil {
		if order.source.hedgeOrder == order {
			order.source.hedgeOrder = nil
		}
		if a.pair.recovering && a.recoveryComplete() {
			a.pair = nil
		}
		return
	}

	leg := a.originalLegFor(order)
	if leg != nil {
		leg.terminal = true
	}
	if !a.pair.perp.terminal || !a.pair.spot.terminal {
		return
	}
	if a.pair.perp.filled == a.pair.perp.qty && a.pair.spot.filled == a.pair.spot.qty {
		a.position += a.pair.positionDelta
		a.pair = nil
		return
	}

	// The pair did not complete symmetrically. Do not change position; offset
	// every observed fill on its own instrument, then keep the pair blocked
	// until those compensating orders finish.
	a.pair.recovering = true
	a.tryNeutralize()
}

func (a *BasisArbActor) originalLegFor(order *basisOrder) *basisLeg {
	if order == nil || a.pair == nil || order.source != nil {
		return nil
	}
	if order.symbol == a.pair.perp.symbol && order.side == a.pair.perp.side && order.requestID == a.pair.perp.requestID {
		return a.pair.perp
	}
	if order.symbol == a.pair.spot.symbol && order.side == a.pair.spot.side && order.requestID == a.pair.spot.requestID {
		return a.pair.spot
	}
	return nil
}

func (a *BasisArbActor) tryNeutralize() {
	if a.pair == nil || !a.pair.recovering {
		return
	}
	for _, leg := range []*basisLeg{a.pair.perp, a.pair.spot} {
		if leg.hedgeOrder != nil || leg.filled <= leg.hedged {
			continue
		}
		qty := leg.filled - leg.hedged
		side := exchange.Buy
		if leg.side == exchange.Buy {
			side = exchange.Sell
		}
		requestID := a.SubmitOrder(leg.symbol, side, exchange.Market, 0, qty)
		order := &basisOrder{
			requestID: requestID,
			symbol:    leg.symbol,
			side:      side,
			qty:       qty,
			source:    leg,
		}
		leg.hedgeOrder = order
		a.pair.ordersByReq[requestID] = order
	}
	if a.recoveryComplete() {
		a.pair = nil
	}
}

func (a *BasisArbActor) recoveryComplete() bool {
	return a.pair.perp.hedged == a.pair.perp.filled &&
		a.pair.spot.hedged == a.pair.spot.filled &&
		a.pair.perp.hedgeOrder == nil &&
		a.pair.spot.hedgeOrder == nil
}
