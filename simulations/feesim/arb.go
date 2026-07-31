package feesim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type BasisArbConfig struct {
	SpotSymbol    string
	PerpSymbol    string
	SpotFeeBps    int64 // taker fee on spot leg
	PerpFeeBps    int64 // taker fee on perp leg
	LotSize       int64
	MaxPosition   int64
	CheckInterval time.Duration
	// Reactive re-evaluates the basis inside HandleEvent on every trade
	// print and snapshot instead of only on the CheckInterval ticker. With
	// polling, reaction time is dominated by ticker phase and a 20x network
	// latency spread produces identical profits (the gen-6 negative result);
	// reactive decisions make reaction time = delivery latency, which is
	// what a latency race is.
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
}

// FeeAwareBasisArb arbitrages spot/perp basis using book mid prices.
// Uses asymmetric thresholds: eagerly unwinds position, reluctantly accumulates.
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
	// hedgePending is the signed perp quantity already sent to flatten the
	// residual but not yet reported filled. Without it each fill re-measures
	// a residual that is already being corrected and fires another hedge, so
	// the strategy overshoots and the exposure flips sign instead of closing.
	hedgePending int64

	spotBid, spotAsk int64
	perpBid, perpAsk int64

	spotSeq      uint64
	perpSeq      uint64
	lastTradeSeq uint64

	subscribed bool
}

func (a *FeeAwareBasisArb) Position() int64    { return a.position }
func (a *FeeAwareBasisArb) Symbol() string     { return a.cfg.SpotSymbol }
func (a *FeeAwareBasisArb) PerpSymbol() string { return a.cfg.PerpSymbol }

func NewFeeAwareBasisArb(id uint64, gw actor.Gateway, cfg BasisArbConfig) *FeeAwareBasisArb {
	a := &FeeAwareBasisArb{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
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
	case actor.EventTrade:
		if a.cfg.Reactive {
			a.onTrade(evt.Data.(actor.TradeEvent))
			a.checkBasis()
		}
	case actor.EventOrderPartialFill, actor.EventOrderFilled:
		a.onFill(evt.Data.(actor.OrderFillEvent))
	}
}

// onFill reconciles believed position against what actually executed. The
// perp leg defines the sign convention (short perp is a positive basis
// position); the spot leg is tracked only to measure the residual between
// them.
func (a *FeeAwareBasisArb) onFill(e actor.OrderFillEvent) {
	signed := e.Qty
	if e.Side == exchange.Sell {
		signed = -signed
	}
	switch e.Symbol {
	case a.cfg.PerpSymbol:
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
			a.SubmitOrder(a.cfg.SpotSymbol, mirror, exchange.Market, 0, e.Qty)
		}
	case a.cfg.SpotSymbol:
		a.spotPos += signed
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

// onTrade folds a trade print into the quote state: the print becomes the
// symbol's mid, spread width kept from the last snapshot. Between 100ms
// snapshots the print is the freshest information there is — and the basis
// dislocation it signals is exactly what the race is run over.
func (a *FeeAwareBasisArb) onTrade(e actor.TradeEvent) {
	price := e.Trade.Price
	switch e.Symbol {
	case a.cfg.SpotSymbol:
		if a.spotBid == 0 || a.spotAsk == 0 {
			return
		}
		half := (a.spotAsk - a.spotBid) / 2
		a.spotBid, a.spotAsk = price-half, price+half
		a.spotSeq++
	case a.cfg.PerpSymbol:
		if a.perpBid == 0 || a.perpAsk == 0 {
			return
		}
		half := (a.perpAsk - a.perpBid) / 2
		a.perpBid, a.perpAsk = price-half, price+half
		a.perpSeq++
	}
}

func (a *FeeAwareBasisArb) onSnapshot(e actor.BookSnapshotEvent) {
	if len(e.Snapshot.Bids) == 0 || len(e.Snapshot.Asks) == 0 {
		return
	}
	bid := e.Snapshot.Bids[0].Price
	ask := e.Snapshot.Asks[0].Price
	switch e.Symbol {
	case a.cfg.SpotSymbol:
		a.spotBid = bid
		a.spotAsk = ask
		a.spotSeq++
	case a.cfg.PerpSymbol:
		a.perpBid = bid
		a.perpAsk = ask
		a.perpSeq++
	}
}

func (a *FeeAwareBasisArb) onTick(_ time.Time) {
	if !a.subscribed {
		types := []exchange.MDType{exchange.MDSnapshot}
		if a.cfg.Reactive {
			types = append(types, exchange.MDTrade)
		}
		a.Subscribe(a.cfg.SpotSymbol, types...)
		a.Subscribe(a.cfg.PerpSymbol, types...)
		a.subscribed = true
	}
	a.checkBasis()
}

func (a *FeeAwareBasisArb) effectiveCost(mid int64) int64 {
	spotFee := a.cfg.SpotFeeBps * mid / 10000
	perpFee := a.cfg.PerpFeeBps * mid / 10000
	halfSpotSpread := (a.spotAsk - a.spotBid) / 2
	halfPerpSpread := (a.perpAsk - a.perpBid) / 2
	return spotFee + perpFee + halfSpotSpread + halfPerpSpread
}

// isUnwinding returns true if the proposed trade direction reduces |position|.
func (a *FeeAwareBasisArb) isUnwinding(basis int64) bool {
	// basis > 0 → wants position++ (short perp / long spot)
	// basis < 0 → wants position-- (long perp / short spot)
	// Unwinding: trade reduces |position|
	return (basis > 0 && a.position < 0) || (basis < 0 && a.position > 0)
}

func (a *FeeAwareBasisArb) checkBasis() {
	if a.spotBid == 0 || a.spotAsk == 0 || a.perpBid == 0 || a.perpAsk == 0 {
		return
	}

	currentSeq := a.spotSeq + a.perpSeq
	if currentSeq == a.lastTradeSeq {
		return
	}

	spotMid := (a.spotBid + a.spotAsk) / 2
	perpMid := (a.perpBid + a.perpAsk) / 2
	basis := perpMid - spotMid
	cost := a.effectiveCost(spotMid)
	if cost == 0 {
		return
	}

	absBasis := basis
	if absBasis < 0 {
		absBasis = -absBasis
	}

	// Asymmetric threshold: eager to unwind (cost/2), reluctant to accumulate
	// (scales up with position: cost * (1 + |position|/MaxPosition)).
	var threshold int64
	if a.isUnwinding(basis) {
		threshold = cost / 2
	} else {
		absPos := a.position
		if absPos < 0 {
			absPos = -absPos
		}
		threshold = cost + cost*absPos/a.cfg.MaxPosition
	}

	if absBasis <= threshold {
		return
	}

	// Exposure the risk cap must respect: filled position plus everything
	// already on the wire. Without the in-flight term a reactive actor can
	// submit hundreds of lots in the latency window before the first fill
	// reports back.
	switch {
	case basis > 0 && a.position+a.inFlight < a.cfg.MaxPosition:
		a.openPair(exchange.Sell, exchange.Buy)
		a.lastTradeSeq = currentSeq

	case basis < 0 && a.position-a.inFlight > -a.cfg.MaxPosition:
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
}
