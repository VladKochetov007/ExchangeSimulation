package actors

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
	"math/rand"
	"time"
)

type RandomTakerSubActorConfig struct {
	Interval  time.Duration
	MinQty    int64
	MaxQty    int64
	Precision int64
}

type RandomTakerSubActor struct {
	id           uint64
	symbol       string
	config       RandomTakerSubActorConfig
	lastTradeNs  int64
	rng          *rand.Rand
	oms          *actor.NettingOMS
}

func NewRandomTakerSubActor(id uint64, symbol string, config RandomTakerSubActorConfig, seed int64) *RandomTakerSubActor {
	return &RandomTakerSubActor{
		id:     id,
		symbol: symbol,
		config: config,
		rng:    rand.New(rand.NewSource(seed)),
		oms:    actor.NewNettingOMS(),
	}
}

func (rt *RandomTakerSubActor) GetID() uint64 {
	return rt.id
}

func (rt *RandomTakerSubActor) GetSymbols() []string {
	return []string{rt.symbol}
}

func (rt *RandomTakerSubActor) OnEvent(event *actor.Event, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	switch event.Type {
	case actor.EventBookSnapshot:
		rt.onBookSnapshot(event.Data.(actor.BookSnapshotEvent), ctx, submit)

	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		rt.onOrderFilled(event.Data.(actor.OrderFillEvent), ctx)
	}
}

func (rt *RandomTakerSubActor) onBookSnapshot(snap actor.BookSnapshotEvent, ctx *actor.SharedContext, submit actor.OrderSubmitter) {
	if snap.Symbol != rt.symbol {
		return
	}

	if len(snap.Snapshot.Bids) == 0 || len(snap.Snapshot.Asks) == 0 {
		return
	}

	if rt.lastTradeNs == 0 {
		rt.lastTradeNs = snap.Timestamp
		return
	}

	if snap.Timestamp-rt.lastTradeNs < rt.config.Interval.Nanoseconds() {
		return
	}

	bestAsk := snap.Snapshot.Asks[0].Price

	side := exchange.Buy
	if rt.rng.Intn(2) == 1 {
		side = exchange.Sell
	}

	qty := rt.config.MinQty
	if rt.config.MaxQty > rt.config.MinQty {
		qty = rt.config.MinQty + rt.rng.Int63n(rt.config.MaxQty-rt.config.MinQty)
	}

	if side == exchange.Buy {
		notional := (qty * bestAsk) / rt.config.Precision
		notionalWithFees := notional + (notional * 20 / 10000)
		if notionalWithFees > ctx.GetAvailableQuote() {
			return
		}
	} else {
		if qty > ctx.GetBaseBalance(extractBaseAsset(rt.symbol)) {
			return
		}
	}

	submit(rt.symbol, side, exchange.Market, 0, qty)
	rt.lastTradeNs = snap.Timestamp
}

func (rt *RandomTakerSubActor) onOrderFilled(fill actor.OrderFillEvent, ctx *actor.SharedContext) {
	rt.oms.OnFill(rt.symbol, fill, rt.config.Precision)

	baseAsset := extractBaseAsset(rt.symbol)
	ctx.OnFill(rt.id, rt.symbol, fill, rt.config.Precision, baseAsset)
}
