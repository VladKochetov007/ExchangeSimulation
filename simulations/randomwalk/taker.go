package randomwalk

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type TakerConfig struct {
	Symbol       string
	OrderQty     int64
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
	side := exchange.Buy
	if rt.rng.Intn(2) == 1 {
		side = exchange.Sell
	}
	rt.SubmitOrder(rt.cfg.Symbol, side, exchange.Market, 0, rt.cfg.OrderQty)
}
