package randomwalk

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type TakerConfig struct {
	Symbols       []string
	QuoteNotional int64 // target order value in quote precision units
	BasePrecision int64 // base asset precision (e.g. BTC_PRECISION)
	TakeInterval  time.Duration
	Seed          int64
}

type RandomTaker struct {
	*actor.BaseActor
	cfg       TakerConfig
	rng       *rand.Rand
	midPrices map[string]int64
}

func NewRandomTaker(id uint64, gw actor.Gateway, cfg TakerConfig) *RandomTaker {
	t := &RandomTaker{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		rng:       rand.New(rand.NewSource(cfg.Seed)),
		midPrices: make(map[string]int64, len(cfg.Symbols)),
	}
	t.SetHandler(t)
	t.AddTicker(cfg.TakeInterval, t.onTick)
	return t
}

func (rt *RandomTaker) Start(ctx context.Context) error {
	for _, sym := range rt.cfg.Symbols {
		rt.Subscribe(sym, exchange.MDSnapshot)
	}
	return rt.BaseActor.Start(ctx)
}

func (rt *RandomTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		rt.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	}
}

func (rt *RandomTaker) onSnapshot(e actor.BookSnapshotEvent) {
	if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
		rt.midPrices[e.Symbol] = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
	}
}

func (rt *RandomTaker) onTick(_ time.Time) {
	sym := rt.cfg.Symbols[rt.rng.Intn(len(rt.cfg.Symbols))]
	mid := rt.midPrices[sym]
	if mid == 0 {
		return
	}
	qty := rt.cfg.QuoteNotional * rt.cfg.BasePrecision / mid
	if qty == 0 {
		return
	}
	side := exchange.Buy
	if rt.rng.Intn(2) == 1 {
		side = exchange.Sell
	}
	rt.SubmitOrder(sym, side, exchange.Market, 0, qty)
}
