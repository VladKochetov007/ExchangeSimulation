package abcusd

import (
	"context"
	"math/rand"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type RandomTakerConfig struct {
	Symbols      []string
	LevelSize    int64
	MinQty       int64 // minimum order qty; orders below this are skipped
	SizeFraction float64
	TakeInterval time.Duration
	Seed         int64
}

type RandomTaker struct {
	*actor.BaseActor
	cfg        RandomTakerConfig
	rng        *rand.Rand
	bestQty    map[string][2]int64 // [0]=bestBidQty [1]=bestAskQty
	subscribed map[string]bool
}

func NewRandomTaker(id uint64, gw actor.Gateway, cfg RandomTakerConfig) *RandomTaker {
	t := &RandomTaker{
		BaseActor:  actor.NewBaseActor(id, gw),
		cfg:        cfg,
		rng:        rand.New(rand.NewSource(cfg.Seed)),
		bestQty:    make(map[string][2]int64, len(cfg.Symbols)),
		subscribed: make(map[string]bool, len(cfg.Symbols)),
	}
	for _, sym := range cfg.Symbols {
		t.bestQty[sym] = [2]int64{cfg.LevelSize, cfg.LevelSize}
	}
	t.SetHandler(t)
	t.AddTicker(cfg.TakeInterval, t.onTick)
	return t
}

func (rt *RandomTaker) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		rt.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	}
}

func (rt *RandomTaker) onSnapshot(e actor.BookSnapshotEvent) {
	qty := rt.bestQty[e.Symbol]
	if len(e.Snapshot.Bids) > 0 {
		qty[0] = e.Snapshot.Bids[0].VisibleQty
	}
	if len(e.Snapshot.Asks) > 0 {
		qty[1] = e.Snapshot.Asks[0].VisibleQty
	}
	rt.bestQty[e.Symbol] = qty
}

func (rt *RandomTaker) onTick(_ time.Time) {
	for _, sym := range rt.cfg.Symbols {
		if !rt.subscribed[sym] {
			rt.Subscribe(sym, exchange.MDSnapshot)
			rt.subscribed[sym] = true
		}
	}

	sym := rt.cfg.Symbols[rt.rng.Intn(len(rt.cfg.Symbols))]

	var side exchange.Side
	var idx int
	if rt.rng.Intn(2) == 0 {
		side = exchange.Buy
		idx = 1 // hits ask
	} else {
		side = exchange.Sell
		idx = 0 // hits bid
	}

	qty := int64(float64(rt.bestQty[sym][idx]) * rt.cfg.SizeFraction)
	if qty >= rt.cfg.MinQty {
		rt.SubmitOrder(sym, side, exchange.Market, 0, qty)
	}
}
