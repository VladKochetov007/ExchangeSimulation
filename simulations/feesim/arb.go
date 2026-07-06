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
}

// FeeAwareBasisArb arbitrages spot/perp basis using book mid prices.
// Uses asymmetric thresholds: eagerly unwinds position, reluctantly accumulates.
type FeeAwareBasisArb struct {
	*actor.BaseActor
	cfg      BasisArbConfig
	position int64 // net lots: +N = N×(short-perp/long-spot)

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
	if evt.Type == actor.EventBookSnapshot {
		a.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
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
		a.Subscribe(a.cfg.SpotSymbol, exchange.MDSnapshot)
		a.Subscribe(a.cfg.PerpSymbol, exchange.MDSnapshot)
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

	switch {
	case basis > 0 && a.position < a.cfg.MaxPosition:
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++
		a.lastTradeSeq = currentSeq

	case basis < 0 && a.position > -a.cfg.MaxPosition:
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--
		a.lastTradeSeq = currentSeq
	}
}
