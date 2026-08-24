package feesim

import (
	"context"
	"math"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type TakerConfig struct {
	Symbols      []string
	TargetQtys   map[string]int64 // per-symbol base qty per order
	TakeInterval time.Duration
	// DecisionPhaseOffset moves only the first periodic decision. Zero retains
	// the legacy ticker path exactly; a nonzero phase is an explicit causal
	// clock input rather than a scheduler accident.
	DecisionPhaseOffset time.Duration
	Seed                int64
	// VenueID, Role, and ClientID identify optional decision evidence. They are
	// simulator wiring, not strategy inputs, and remain empty for ordinary
	// fee-simulation uses.
	VenueID          string
	Role             string
	ClientID         uint64
	DecisionObserver func(RandomTakerDecision)

	// ImbalanceCoupling tilts order side toward book imbalance (herding flow,
	// queue-reactive style): p(buy) = 0.5 + coupling×imbalance/2 with
	// imbalance ∈ [−1,1] from top-of-book visible qty. 0 = symmetric coin flip.
	ImbalanceCoupling float64

	// ExciteAlpha/ExciteBetaPerSec give Hawkes-lite self-excitation: every
	// observed market trade adds ExciteAlpha to an excitation level that decays
	// at ExciteBetaPerSec; each tick fires 1+floor(excitation) orders (cap 5).
	ExciteAlpha      float64
	ExciteBetaPerSec float64

	// SizeParetoAlpha, when positive, draws order size from a Pareto tail
	// around the target rather than uniformly within a band of it.
	//
	// Uniform sizes far below the quoted depth cannot move a price: an order
	// worth a fraction of the touch is absorbed whole, so the price only ever
	// changes when a maker reprices, and the return distribution collapses to
	// the bid-ask bounce. Measured that way the reference market had an excess
	// kurtosis of -0.60 and a Hill tail index of 39, where traded markets show
	// fat tails and an index near three. A heavy size tail is what lets an
	// occasional order walk the book.
	SizeParetoAlpha float64
	// SizeCapMultiple bounds a drawn size at this multiple of the target, so a
	// tail draw cannot exhaust an account in one order. Zero means fifty.
	SizeCapMultiple float64
}

// RandomTakerDecision records one evaluated broad-flow tick before any venue
// request enters its gateway. SubmittedRequestCount distinguishes an
// evaluated no-order tick from missing evidence. The observer is optional
// and write-only: it must not alter actor state, timing, or random draws.
type RandomTakerDecision struct {
	VenueID               string `json:"venue_id"`
	Role                  string `json:"role"`
	ClientID              uint64 `json:"client_id"`
	DecisionTime          int64  `json:"decision_time"`
	DecisionPhaseOffset   int64  `json:"decision_phase_offset_nanos"`
	Action                string `json:"action"`
	SubmittedRequestCount int64  `json:"submitted_request_count"`
}

type RandomTaker struct {
	*actor.BaseActor
	cfg       TakerConfig
	rng       *rand.Rand
	imbalance map[string]float64
	// depth is the visible quantity on each side of each book, so an order can
	// be bounded by liquidity that exists rather than evaporating against a
	// book that cannot fill it.
	depth      map[string]sideDepth
	excitation float64
	decay      float64
	subscribed bool
}

func NewRandomTaker(id uint64, gw actor.Gateway, cfg TakerConfig) *RandomTaker {
	t := &RandomTaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
		imbalance: make(map[string]float64),
		depth:     make(map[string]sideDepth),
		decay:     math.Exp(-cfg.ExciteBetaPerSec * cfg.TakeInterval.Seconds()),
	}
	t.SetHandler(t)
	t.AddTickerWithOffset(cfg.TakeInterval, cfg.DecisionPhaseOffset, t.onTick)
	return t
}

// sideDepth is the visible quantity resting on each side of one book.
type sideDepth struct{ bid, ask int64 }

// stateDependent reports whether the taker needs market data.
//
// It always does now: an order has to be bounded by the depth facing it. A
// market order larger than the book does not walk it, it evaporates with no
// liquidity, and because the side is drawn before the book is consulted the
// truncation falls on whichever side happens to be thinner. That turns
// unconditionally symmetric flow into flow conditioned on book state: measured
// with a heavy size tail it produced 1557 evaporated orders against 11 with
// uniform sizes, and a 3.8% net selling imbalance out of a requested imbalance
// of 1.3% that is not itself significant.
func (rt *RandomTaker) stateDependent() bool { return true }

func (rt *RandomTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		rt.imbalance[e.Symbol] = topImbalance(e.Snapshot)
		rt.depth[e.Symbol] = visibleDepth(e.Snapshot)
	case actor.EventTrade:
		if rt.cfg.ExciteAlpha > 0 {
			rt.excitation += rt.cfg.ExciteAlpha
			if rt.excitation > 5 {
				rt.excitation = 5
			}
		}
	}
}

// visibleDepth totals the quantity resting on each side of the book.
func visibleDepth(snap *exchange.BookSnapshot) sideDepth {
	var out sideDepth
	for _, level := range snap.Bids {
		out.bid += level.VisibleQty
	}
	for _, level := range snap.Asks {
		out.ask += level.VisibleQty
	}
	return out
}

// topImbalance returns (bidQty−askQty)/(bidQty+askQty) over visible book depth.
func topImbalance(snap *exchange.BookSnapshot) float64 {
	var bid, ask int64
	for _, l := range snap.Bids {
		bid += l.VisibleQty
	}
	for _, l := range snap.Asks {
		ask += l.VisibleQty
	}
	if bid+ask == 0 {
		return 0
	}
	return float64(bid-ask) / float64(bid+ask)
}

func (rt *RandomTaker) onTick(now time.Time) {
	if !rt.subscribed {
		rt.observeDecision(now, "SUBSCRIBE", 0)
		for _, sym := range rt.cfg.Symbols {
			rt.Subscribe(sym, exchange.MDSnapshot, exchange.MDTrade)
		}
		rt.subscribed = true
		// Return rather than trading on the subscribing tick. Firing here means
		// trading a full tick before any maker has quoted, which seeded the
		// first imbalance of the run before a price existed.
		return
	}

	orders := 1
	if rt.cfg.ExciteAlpha > 0 {
		orders += int(rt.excitation)
		rt.excitation *= rt.decay
	}
	// excitation is capped at five, so a tick creates at most six plans. Keep
	// this small pre-ingress buffer on the stack rather than making phase
	// evidence add a per-tick heap allocation to the legacy flow population.
	var plans [6]randomTakerOrderPlan
	planned := 0
	for i := 0; i < orders; i++ {
		if plan, ok := rt.planOrder(); ok {
			plans[planned] = plan
			planned++
		}
	}
	// Emit the complete decision before its planned requests enter the gateway.
	// planOrder consumes the same RNG draws as the legacy fireOrder path, and
	// no gateway response can reach this actor until the callback returns.
	rt.observeDecision(now, "EVALUATE", int64(planned))
	for _, plan := range plans[:planned] {
		rt.submitPlannedOrder(plan)
	}
}

func (rt *RandomTaker) observeDecision(now time.Time, action string, submitted int64) {
	if rt.cfg.DecisionObserver == nil {
		return
	}
	rt.cfg.DecisionObserver(RandomTakerDecision{
		VenueID: rt.cfg.VenueID, Role: rt.cfg.Role, ClientID: rt.cfg.ClientID,
		DecisionTime: now.UnixNano(), DecisionPhaseOffset: int64(rt.cfg.DecisionPhaseOffset),
		Action: action, SubmittedRequestCount: submitted,
	})
}

// drawSize returns one order's quantity: uniform within half the target when no
// tail is configured, and a Pareto draw around it when one is.
func (rt *RandomTaker) drawSize(target int64) int64 {
	if target <= 0 {
		return 0
	}
	if rt.cfg.SizeParetoAlpha <= 0 {
		return target/2 + rt.rng.Int63n(target)
	}
	cap := rt.cfg.SizeCapMultiple
	if cap <= 0 {
		cap = 50
	}
	uniform := rt.rng.Float64()
	if uniform <= 0 {
		uniform = 1e-12
	}
	scaled := float64(target) * math.Pow(uniform, -1/rt.cfg.SizeParetoAlpha)
	if limit := float64(target) * cap; scaled > limit {
		scaled = limit
	}
	if !finiteSize(scaled) {
		return target
	}
	return int64(scaled)
}

func finiteSize(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 1
}

type randomTakerOrderPlan struct {
	symbol string
	side   exchange.Side
	qty    int64
}

// planOrder is the actor-local pre-ingress portion of one random-taker order.
// It intentionally contains every legacy RNG draw; submitPlannedOrder only
// turns that already chosen decision into a gateway request.
func (rt *RandomTaker) planOrder() (randomTakerOrderPlan, bool) {
	sym := rt.cfg.Symbols[rt.rng.Intn(len(rt.cfg.Symbols))]
	baseQty := rt.cfg.TargetQtys[sym]
	if baseQty == 0 {
		return randomTakerOrderPlan{}, false
	}
	qty := rt.drawSize(baseQty)
	if qty <= 0 {
		return randomTakerOrderPlan{}, false
	}

	pBuy := 0.5
	if c := rt.cfg.ImbalanceCoupling; c != 0 {
		pBuy += c * rt.imbalance[sym] / 2
		if pBuy < 0.05 {
			pBuy = 0.05
		} else if pBuy > 0.95 {
			pBuy = 0.95
		}
	}
	side := exchange.Buy
	if rt.rng.Float64() >= pBuy {
		side = exchange.Sell
	}
	// Bound the order by the depth facing it, as every other taker in this
	// scenario does. Without this the executed size is min(drawn, depth), which
	// is a book-conditioned quantity even though the drawn size is not.
	facing := rt.depth[sym].ask
	if side == exchange.Sell {
		facing = rt.depth[sym].bid
	}
	if facing > 0 && qty > facing {
		qty = facing
	}
	if qty <= 0 {
		return randomTakerOrderPlan{}, false
	}
	return randomTakerOrderPlan{symbol: sym, side: side, qty: qty}, true
}

func (rt *RandomTaker) submitPlannedOrder(plan randomTakerOrderPlan) {
	rt.SubmitOrder(plan.symbol, plan.side, exchange.Market, 0, plan.qty)
}
