package feesim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

type BasisArbConfig struct {
	SpotSymbol    string
	PerpSymbol    string
	SpotFeeBps    int64 // taker fee on spot leg
	PerpFeeBps    int64 // taker fee on perp leg
	LotSize       int64
	BasePrecision int64
	MaxPosition   int64
	CheckInterval time.Duration
	// Reactive re-evaluates the basis inside HandleEvent on every book
	// snapshot or delta instead of only on the CheckInterval ticker. With
	// polling, reaction time is dominated by ticker phase and a 20x network
	// latency spread produces identical profits (the gen-6 negative result);
	// reactive decisions make reaction time = delivery latency, while still
	// using an executable displayed touch rather than inferring one from a
	// trade print.
	Reactive bool
	// HedgeResidual flattens the leftover delta between the two legs. The
	// strategy fires two market orders and assumes both fill; into a thin book
	// they fill by different amounts and nothing reconciles the difference, so
	// unhedged directional exposure accumulates until it dwarfs the basis edge
	// the strategy is trying to capture. Production basis arbs hedge exactly
	// this residual.
	HedgeResidual bool
	// SequentialLegs sends the perp leg first and mirrors the SECOND leg to
	// whatever actually filled, instead of firing both blindly and hoping
	// they match. Residual then arises only from the second leg's own partial
	// fills rather than from the mismatch between two independent orders, and
	// the race becomes about who completes both legs first — second-leg
	// latency becomes part of the edge.
	SequentialLegs bool
	// PostSecondLeg rests the mirrored leg as a limit order at the near touch
	// instead of crossing the spread for it. Crossing pays the spread AND
	// arrives in the same instant as every competitor reacting to the same
	// print, so the second legs crowd each other; a resting order is already
	// in the queue before that signal. Unfilled legs are cancelled after
	// SecondLegTimeout and the first leg is unwound, converting an open-ended
	// directional residual into a bounded, decided cost.
	PostSecondLeg    bool
	SecondLegTimeout time.Duration
	// PostImproveTicks posts the second leg this many ticks INSIDE the near
	// touch. Joining the touch queues the order behind whatever the resident
	// market maker already has resting there, so it only fills once that size
	// is exhausted; improving the price buys queue priority at the cost of a
	// tick of edge. TickSize must be set for this to apply.
	PostImproveTicks int64
	TickSize         int64
}

// BasisArbReport is a fill-based account of one strategy's actual activity.
// It intentionally distinguishes execution from submitted orders: an ecology
// result cannot treat a signal, request, or intended pair as a completed arb.
type BasisArbReport struct {
	ClientID             uint64 `json:"client_id"`
	ExecutableSignals    int    `json:"executable_signals"`
	SubmittedPairs       int    `json:"submitted_pairs"`
	SpotBoughtQty        int64  `json:"spot_bought_qty"`
	SpotSoldQty          int64  `json:"spot_sold_qty"`
	PerpBoughtQty        int64  `json:"perp_bought_qty"`
	PerpSoldQty          int64  `json:"perp_sold_qty"`
	SpotNotional         int64  `json:"spot_notional"`
	PerpNotional         int64  `json:"perp_notional"`
	QuoteFees            int64  `json:"quote_fees"`
	UnpricedFeeCount     int    `json:"unpriced_fee_count"`
	ResidualBaseQty      int64  `json:"residual_base_qty"`
	OpenPerpPositionLots int64  `json:"open_perp_position_lots"`
}

// FeeAwareBasisArb arbitrages a spot/perp basis only when the observed best
// prices lock a positive all-in two-leg cashflow for one configured lot.
type FeeAwareBasisArb struct {
	*actor.BaseActor
	cfg BasisArbConfig
	// position is net lots derived from ACTUAL perp-leg fills, not from
	// submissions: a market order into a thin book fills partially (or not at
	// all), so counting intents lets believed inventory drift arbitrarily far
	// from reality and silently voids MaxPosition.
	position int64
	// perpPos and spotPos are signed filled quantities per leg in base units.
	// Delta neutrality means they sum to zero; whatever they actually sum to
	// is the naked directional exposure the strategy is carrying.
	perpPos int64
	spotPos int64
	// inFlight counts submitted-but-unfilled lots so a burst of reactive
	// decisions cannot stack orders past MaxPosition before any fill lands.
	inFlight int64
	// restingLegs tracks posted second legs awaiting fill or timeout, keyed by
	// the request ID so the accept event can attach the exchange order ID.
	restingLegs map[uint64]*restingLeg

	// hedgePending is the signed perp quantity already sent to flatten the
	// residual but not yet reported filled. Without it each fill re-measures
	// a residual that is already being corrected and fires another hedge, so
	// the strategy overshoots and the exposure flips sign instead of closing.
	hedgePending int64

	spotBid, spotAsk int64
	perpBid, perpAsk int64
	spotBook         quoteBook
	perpBook         quoteBook

	spotSeq      uint64
	perpSeq      uint64
	lastTradeSeq uint64

	subscribed bool
	// lastTick is the most recent ticker timestamp, used as the actor's clock
	// so deadlines follow simulation time rather than wall time.
	lastTick int64
	report   BasisArbReport
}

// quoteBook is the actor's public displayed-depth view. A snapshot resets it;
// each subsequent delta changes one price level. It deliberately ignores
// hidden quantity because a taker cannot rely on it for an executable edge.
type quoteBook struct {
	bids map[int64]int64
	asks map[int64]int64
}

func (b *quoteBook) reset(snapshot *exchange.BookSnapshot) {
	b.bids = make(map[int64]int64, len(snapshot.Bids))
	b.asks = make(map[int64]int64, len(snapshot.Asks))
	for _, level := range snapshot.Bids {
		if level.VisibleQty > 0 {
			b.bids[level.Price] = level.VisibleQty
		}
	}
	for _, level := range snapshot.Asks {
		if level.VisibleQty > 0 {
			b.asks[level.Price] = level.VisibleQty
		}
	}
}

func (b *quoteBook) apply(delta *exchange.BookDelta) {
	levels := b.bids
	if delta.Side == exchange.Sell {
		levels = b.asks
	}
	if levels == nil {
		return
	}
	if delta.VisibleQty <= 0 {
		delete(levels, delta.Price)
		return
	}
	levels[delta.Price] = delta.VisibleQty
}

func (b *quoteBook) best() (bid, ask int64, ok bool) {
	for price, qty := range b.bids {
		if qty > 0 && price > bid {
			bid = price
		}
	}
	for price, qty := range b.asks {
		if qty > 0 && (ask == 0 || price < ask) {
			ask = price
		}
	}
	return bid, ask, bid > 0 && ask > 0
}

// now is the actor's view of time: the last tick it observed. Resolution is
// one CheckInterval, which is the same granularity the deadlines use.
func (a *FeeAwareBasisArb) now() int64 { return a.lastTick }

func (a *FeeAwareBasisArb) Position() int64    { return a.position }
func (a *FeeAwareBasisArb) Symbol() string     { return a.cfg.SpotSymbol }
func (a *FeeAwareBasisArb) PerpSymbol() string { return a.cfg.PerpSymbol }

// Report returns a point-in-time, fill-based strategy record. It does not
// claim marked PnL because a caller must supply an explicit terminal mark and
// numeraire for that separate accounting step.
func (a *FeeAwareBasisArb) Report() BasisArbReport {
	report := a.report
	report.ResidualBaseQty = a.residual()
	report.OpenPerpPositionLots = a.position
	return report
}

// restingLeg is a posted second leg: its remaining quantity, the deadline
// after which it is abandoned, and the order ID once the exchange assigns one.
type restingLeg struct {
	orderID   uint64
	side      exchange.Side
	remaining int64
	deadline  int64
	cancelled bool
}

func NewFeeAwareBasisArb(id uint64, gw actor.Gateway, cfg BasisArbConfig) *FeeAwareBasisArb {
	if cfg.LotSize <= 0 || cfg.BasePrecision <= 0 || cfg.MaxPosition <= 0 || cfg.CheckInterval <= 0 {
		panic("feesim: basis arb requires positive lot size, base precision, position cap, and check interval")
	}
	a := &FeeAwareBasisArb{
		BaseActor:   actor.NewBaseActor(id, gw),
		cfg:         cfg,
		restingLegs: make(map[uint64]*restingLeg),
		report:      BasisArbReport{ClientID: id},
	}
	a.SetHandler(a)
	a.AddTicker(cfg.CheckInterval, a.onTick)
	return a
}

func (a *FeeAwareBasisArb) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		a.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
		if a.cfg.Reactive {
			a.checkBasis()
		}
	case actor.EventBookDelta:
		a.onDelta(evt.Data.(actor.BookDeltaEvent))
		if a.cfg.Reactive {
			a.checkBasis()
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		a.onFill(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderAccepted:
		e := evt.Data.(actor.OrderAcceptedEvent)
		if leg := a.restingLegs[e.RequestID]; leg != nil {
			leg.orderID = e.OrderID
		}
	case actor.EventOrderCancelled:
		a.onSecondLegCancelled(evt.Data.(actor.OrderCancelledEvent))
	}
}

// sendSecondLeg either crosses for the mirror leg or posts it at the near
// touch, depending on configuration.
func (a *FeeAwareBasisArb) sendSecondLeg(side exchange.Side, qty int64) {
	if !a.cfg.PostSecondLeg {
		a.SubmitOrder(a.cfg.SpotSymbol, side, exchange.Market, 0, qty)
		return
	}
	// Join the near touch: buying rests at the bid, selling at the ask. The
	// order earns the spread instead of paying it, at the cost of possibly
	// not filling at all — which the timeout sweep then resolves.
	price := a.spotBid
	if side == exchange.Sell {
		price = a.spotAsk
	}
	if improve := a.cfg.PostImproveTicks * a.cfg.TickSize; improve > 0 && price > 0 {
		if side == exchange.Buy {
			price += improve
		} else {
			price -= improve
		}
	}
	if price <= 0 {
		a.SubmitOrder(a.cfg.SpotSymbol, side, exchange.Market, 0, qty)
		return
	}
	reqID := a.SubmitOrder(a.cfg.SpotSymbol, side, exchange.LimitOrder, price, qty)
	a.restingLegs[reqID] = &restingLeg{
		side:      side,
		remaining: qty,
		deadline:  a.now() + int64(a.secondLegTimeout()),
	}
}

func (a *FeeAwareBasisArb) secondLegTimeout() time.Duration {
	if a.cfg.SecondLegTimeout > 0 {
		return a.cfg.SecondLegTimeout
	}
	return a.cfg.CheckInterval
}

// onSecondLegCancelled unwinds the first leg for whatever the posted second
// leg never filled: the strategy decided not to pay up, so the paired
// exposure has to go rather than sit naked.
func (a *FeeAwareBasisArb) onSecondLegCancelled(e actor.OrderCancelledEvent) {
	for reqID, leg := range a.restingLegs {
		if leg.orderID != e.OrderID {
			continue
		}
		delete(a.restingLegs, reqID)
		if e.RemainingQty <= 0 {
			return
		}
		// Unwind the matching first-leg quantity on the perp book.
		unwind := exchange.Sell
		if leg.side == exchange.Sell {
			unwind = exchange.Buy
		}
		a.SubmitOrder(a.cfg.PerpSymbol, unwind, exchange.Market, 0, e.RemainingQty)
		return
	}
}

// sweepSecondLegs abandons posted legs past their deadline. Cancelling drives
// the unwind through onSecondLegCancelled, which knows the unfilled amount.
func (a *FeeAwareBasisArb) sweepSecondLegs() {
	now := a.now()
	for _, leg := range a.restingLegs {
		if leg.cancelled || leg.orderID == 0 || now < leg.deadline {
			continue
		}
		leg.cancelled = true
		a.CancelOrder(leg.orderID)
	}
}

// onFill reconciles believed position against what actually executed. The
// perp leg defines the sign convention (short perp is a positive basis
// position); the spot leg is tracked only to measure the residual between
// them.
func (a *FeeAwareBasisArb) onFill(e actor.OrderFillEvent) {
	notional, ok := etypes.TryMulDiv(e.Qty, e.Price, a.cfg.BasePrecision)
	if !ok {
		panic("feesim: basis arb fill notional overflows")
	}
	if e.FeeAmount != 0 {
		if e.FeeAsset == "USD" {
			a.report.QuoteFees = mustAdd(a.report.QuoteFees, e.FeeAmount, "basis quote fees")
		} else {
			a.report.UnpricedFeeCount++
		}
	}
	signed := e.Qty
	if e.Side == exchange.Sell {
		signed = -signed
	}
	switch e.Symbol {
	case a.cfg.PerpSymbol:
		a.report.PerpNotional = mustAdd(a.report.PerpNotional, notional, "basis perp notional")
		if e.Side == exchange.Buy {
			a.report.PerpBoughtQty = mustAdd(a.report.PerpBoughtQty, e.Qty, "basis perp buy quantity")
		} else {
			a.report.PerpSoldQty = mustAdd(a.report.PerpSoldQty, e.Qty, "basis perp sell quantity")
		}
		a.perpPos += signed
		a.position = -a.perpPos / a.cfg.LotSize
		if a.inFlight > 0 {
			a.inFlight--
		}
		if a.cfg.SequentialLegs {
			// Mirror the second leg to what actually executed. Hedges ride
			// the spot leg in this mode, so a perp fill is always a first
			// leg and mirroring it cannot feed back on itself.
			mirror := exchange.Buy
			if e.Side == exchange.Buy {
				mirror = exchange.Sell
			}
			a.sendSecondLeg(mirror, e.Qty)
		}
	case a.cfg.SpotSymbol:
		a.report.SpotNotional = mustAdd(a.report.SpotNotional, notional, "basis spot notional")
		if e.Side == exchange.Buy {
			a.report.SpotBoughtQty = mustAdd(a.report.SpotBoughtQty, e.Qty, "basis spot buy quantity")
		} else {
			a.report.SpotSoldQty = mustAdd(a.report.SpotSoldQty, e.Qty, "basis spot sell quantity")
		}
		a.spotPos += signed
		for reqID, leg := range a.restingLegs {
			if leg.orderID != e.OrderID {
				continue
			}
			if leg.remaining -= e.Qty; leg.remaining <= 0 || e.IsFull {
				delete(a.restingLegs, reqID)
			}
			break
		}
	default:
		return
	}
	// Pending hedge quantity is retired by fills on whichever leg carries the
	// hedges; fills cannot be attributed to a specific order here, so the
	// nearest fill on that leg is treated as the hedge.
	if e.Symbol == a.hedgeSymbol() {
		a.hedgePending = shrinkToward(a.hedgePending, e.Qty)
	}

	a.hedgeResidual()
}

// residual is the naked delta across both legs, net of hedges already on the
// wire: zero when every pair filled symmetrically, non-zero by exactly the
// amount one leg out-filled the other.
func (a *FeeAwareBasisArb) residual() int64 {
	return a.spotPos + a.perpPos + a.hedgePending
}

// shrinkToward moves v toward zero by up to mag.
func shrinkToward(v, mag int64) int64 {
	if v > 0 {
		if v -= mag; v < 0 {
			return 0
		}
		return v
	}
	if v < 0 {
		if v += mag; v > 0 {
			return 0
		}
		return v
	}
	return 0
}

// hedgeResidual flattens leftover delta on the perp leg, which needs only
// margin rather than spot inventory. It waits for the in-flight pair to
// settle so a half-reported round trip is not mistaken for a residual, and it
// ignores anything below one lot so rounding does not cause perpetual
// hedging.
func (a *FeeAwareBasisArb) hedgeResidual() {
	if !a.cfg.HedgeResidual || a.inFlight > 0 {
		return
	}
	residual := a.residual()
	if residual >= -a.cfg.LotSize && residual <= a.cfg.LotSize {
		return
	}
	side := exchange.Sell
	qty := residual
	if residual < 0 {
		side = exchange.Buy
		qty = -residual
	}
	a.SubmitOrder(a.hedgeSymbol(), side, exchange.Market, 0, qty)
	a.hedgePending -= residual
}

// hedgeSymbol is the leg residual hedges are sent to. With simultaneous legs
// the perp leg takes them, since it needs only margin. With sequential legs
// the spot leg is the one that lags, and putting hedges on the perp leg there
// would mirror each hedge into a fresh spot order and never converge.
func (a *FeeAwareBasisArb) hedgeSymbol() string {
	if a.cfg.SequentialLegs {
		return a.cfg.SpotSymbol
	}
	return a.cfg.PerpSymbol
}

func (a *FeeAwareBasisArb) onSnapshot(e actor.BookSnapshotEvent) {
	switch e.Symbol {
	case a.cfg.SpotSymbol:
		a.spotBook.reset(e.Snapshot)
		a.spotBid, a.spotAsk, _ = a.spotBook.best()
		a.spotSeq++
	case a.cfg.PerpSymbol:
		a.perpBook.reset(e.Snapshot)
		a.perpBid, a.perpAsk, _ = a.perpBook.best()
		a.perpSeq++
	}
}

func (a *FeeAwareBasisArb) onDelta(e actor.BookDeltaEvent) {
	switch e.Symbol {
	case a.cfg.SpotSymbol:
		a.spotBook.apply(e.Delta)
		a.spotBid, a.spotAsk, _ = a.spotBook.best()
		a.spotSeq++
	case a.cfg.PerpSymbol:
		a.perpBook.apply(e.Delta)
		a.perpBid, a.perpAsk, _ = a.perpBook.best()
		a.perpSeq++
	}
}

func (a *FeeAwareBasisArb) onTick(t time.Time) {
	a.lastTick = t.UnixNano()
	if !a.subscribed {
		types := []exchange.MDType{exchange.MDSnapshot, exchange.MDDelta}
		a.Subscribe(a.cfg.SpotSymbol, types...)
		a.Subscribe(a.cfg.PerpSymbol, types...)
		a.subscribed = true
	}
	a.sweepSecondLegs()
	a.checkBasis()
}

// executableEdge returns all-in quote cashflow for one simultaneous pair.
// A perp sell/spot buy is positive only when proceeds net of its fee exceed
// the spot purchase plus its fee; the reverse applies to a perp buy/spot sell.
// This avoids midpoint signals that are guaranteed losses once the spread is
// crossed.
func (a *FeeAwareBasisArb) executableEdge(perpSide exchange.Side) (int64, bool) {
	perpPrice := a.perpBid
	spotPrice := a.spotAsk
	if perpSide == exchange.Buy {
		perpPrice = a.perpAsk
		spotPrice = a.spotBid
	}
	if perpPrice <= 0 || spotPrice <= 0 {
		return 0, false
	}
	perpNotional, ok := etypes.TryMulDiv(a.cfg.LotSize, perpPrice, a.cfg.BasePrecision)
	if !ok {
		return 0, false
	}
	spotNotional, ok := etypes.TryMulDiv(a.cfg.LotSize, spotPrice, a.cfg.BasePrecision)
	if !ok {
		return 0, false
	}
	perpFee, ok := etypes.TryMulBps(perpNotional, a.cfg.PerpFeeBps)
	if !ok {
		return 0, false
	}
	spotFee, ok := etypes.TryMulBps(spotNotional, a.cfg.SpotFeeBps)
	if !ok {
		return 0, false
	}
	if perpSide == exchange.Sell {
		proceeds, ok := etypes.TrySub(perpNotional, perpFee)
		if !ok {
			return 0, false
		}
		cost, ok := etypes.TryAdd(spotNotional, spotFee)
		if !ok {
			return 0, false
		}
		return etypes.TrySub(proceeds, cost)
	}
	proceeds, ok := etypes.TrySub(spotNotional, spotFee)
	if !ok {
		return 0, false
	}
	cost, ok := etypes.TryAdd(perpNotional, perpFee)
	if !ok {
		return 0, false
	}
	return etypes.TrySub(proceeds, cost)
}

func (a *FeeAwareBasisArb) checkBasis() {
	if a.spotBid == 0 || a.spotAsk == 0 || a.perpBid == 0 || a.perpAsk == 0 {
		return
	}

	currentSeq := a.spotSeq + a.perpSeq
	if currentSeq == a.lastTradeSeq {
		return
	}

	// Exposure must include submitted-but-unfilled pairs. Unlike the old
	// midpoint threshold, every entry below is positive at the observed best
	// prices after the configured taker fees.
	if richEdge, ok := a.executableEdge(exchange.Sell); ok && richEdge > 0 && a.position+a.inFlight < a.cfg.MaxPosition {
		a.report.ExecutableSignals++
		a.openPair(exchange.Sell, exchange.Buy)
		a.lastTradeSeq = currentSeq
		return
	}
	if cheapEdge, ok := a.executableEdge(exchange.Buy); ok && cheapEdge > 0 && a.position-a.inFlight > -a.cfg.MaxPosition {
		a.report.ExecutableSignals++
		a.openPair(exchange.Buy, exchange.Sell)
		a.lastTradeSeq = currentSeq
	}
}

// openPair sends the legs. Sequential mode sends only the perp leg and lets
// the fill drive the spot leg; simultaneous mode fires both and accepts
// whatever mismatch the two fills leave behind.
func (a *FeeAwareBasisArb) openPair(perpSide, spotSide exchange.Side) {
	a.SubmitOrder(a.cfg.PerpSymbol, perpSide, exchange.Market, 0, a.cfg.LotSize)
	if !a.cfg.SequentialLegs {
		a.SubmitOrder(a.cfg.SpotSymbol, spotSide, exchange.Market, 0, a.cfg.LotSize)
	}
	a.inFlight++
	a.report.SubmittedPairs++
}

func mustAdd(left, right int64, field string) int64 {
	value, ok := etypes.TryAdd(left, right)
	if !ok {
		panic("feesim: " + field + " overflows")
	}
	return value
}
