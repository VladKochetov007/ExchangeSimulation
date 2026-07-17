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
	Seed         int64

	// ImbalanceCoupling tilts order side toward book imbalance (herding flow,
	// queue-reactive style): p(buy) = 0.5 + coupling×imbalance/2 with
	// imbalance ∈ [−1,1] from top-of-book visible qty. 0 = symmetric coin flip.
	ImbalanceCoupling float64

	// ExciteAlpha/ExciteBetaPerSec give Hawkes-lite self-excitation: every
	// observed market trade adds ExciteAlpha to an excitation level that decays
	// at ExciteBetaPerSec; each tick fires 1+floor(excitation) orders (cap 5).
	ExciteAlpha      float64
	ExciteBetaPerSec float64
}

type RandomTaker struct {
	*actor.BaseActor
	cfg        TakerConfig
	rng        *rand.Rand
	imbalance  map[string]float64
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
		decay:     math.Exp(-cfg.ExciteBetaPerSec * cfg.TakeInterval.Seconds()),
	}
	t.SetHandler(t)
	t.AddTicker(cfg.TakeInterval, t.onTick)
	return t
}

func (rt *RandomTaker) stateDependent() bool {
	return rt.cfg.ImbalanceCoupling != 0 || rt.cfg.ExciteAlpha > 0
}

func (rt *RandomTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		e := evt.Data.(actor.BookSnapshotEvent)
		rt.imbalance[e.Symbol] = topImbalance(e.Snapshot)
	case actor.EventTrade:
		if rt.cfg.ExciteAlpha > 0 {
			rt.excitation += rt.cfg.ExciteAlpha
			if rt.excitation > 5 {
				rt.excitation = 5
			}
		}
	}
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

func (rt *RandomTaker) onTick(_ time.Time) {
	if rt.stateDependent() && !rt.subscribed {
		for _, sym := range rt.cfg.Symbols {
			rt.Subscribe(sym, exchange.MDSnapshot, exchange.MDTrade)
		}
		rt.subscribed = true
	}

	orders := 1
	if rt.cfg.ExciteAlpha > 0 {
		orders += int(rt.excitation)
		rt.excitation *= rt.decay
	}
	for i := 0; i < orders; i++ {
		rt.fireOrder()
	}
}

func (rt *RandomTaker) fireOrder() {
	sym := rt.cfg.Symbols[rt.rng.Intn(len(rt.cfg.Symbols))]
	baseQty := rt.cfg.TargetQtys[sym]
	if baseQty == 0 {
		return
	}
	// ±50% randomness around target qty
	qty := baseQty/2 + rt.rng.Int63n(baseQty)

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
	rt.SubmitOrder(sym, side, exchange.Market, 0, qty)
}
