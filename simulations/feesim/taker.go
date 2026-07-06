package feesim

import (
	"context"
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
}

type RandomTaker struct {
	*actor.BaseActor
	cfg TakerConfig
	rng *rand.Rand
}

func NewRandomTaker(id uint64, gw actor.Gateway, cfg TakerConfig) *RandomTaker {
	t := &RandomTaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
	}
	t.SetHandler(t)
	t.AddTicker(cfg.TakeInterval, t.onTick)
	return t
}

func (rt *RandomTaker) HandleEvent(_ context.Context, _ *actor.Event) {}

func (rt *RandomTaker) onTick(_ time.Time) {
	sym := rt.cfg.Symbols[rt.rng.Intn(len(rt.cfg.Symbols))]
	baseQty := rt.cfg.TargetQtys[sym]
	if baseQty == 0 {
		return
	}
	// ±50% randomness around target qty
	qty := baseQty/2 + rt.rng.Int63n(baseQty)
	side := exchange.Buy
	if rt.rng.Intn(2) == 1 {
		side = exchange.Sell
	}
	rt.SubmitOrder(sym, side, exchange.Market, 0, qty)
}
