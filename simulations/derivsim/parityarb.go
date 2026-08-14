package derivsim

import (
	"cmp"
	"context"
	"slices"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// ParityArbConfig drives a put-call parity arbitrageur: for every strike and
// expiry with both a call and a put quoted, it checks C − P = S − K (zero
// rate, spot as the forward proxy) and trades the three-leg conversion or
// reversal when the gap exceeds EdgeBps of spot. The recorded gap series is
// the H-O1 measurement.
type ParityArbConfig struct {
	Underlying    string
	EdgeBps       int64
	LotQty        int64
	MaxTrades     int64
	CheckInterval time.Duration
}

// ParitySample records one parity-gap observation.
type ParitySample struct {
	Strike       int64
	TimeToExpiry time.Duration
	GapBps       float64
}

type optionTop struct {
	bestBid, bestAsk int64
}

// ParityArb monitors C/P pairs and trades conversions when parity breaks.
type ParityArb struct {
	*actor.BaseActor
	cfg        ParityArbConfig
	set        *contractSet
	tops       map[string]*optionTop
	spotMid    int64
	trades     int64
	samples    []ParitySample
	subscribed bool
}

func NewParityArb(id uint64, gw actor.Gateway, cfg ParityArbConfig) *ParityArb {
	a := &ParityArb{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		set:       newContractSet(cfg.Underlying),
		tops:      make(map[string]*optionTop),
	}
	a.set.onList = func(c *Contract) {
		if c.Type == "OPTION" {
			a.tops[c.Symbol] = &optionTop{}
			a.Subscribe(c.Symbol, exchange.MDSnapshot)
		}
	}
	a.set.onSettle = func(c *Contract, _ int64) { delete(a.tops, c.Symbol) }
	a.SetHandler(a)
	a.AddTicker(cfg.CheckInterval, a.onTick)
	return a
}

// ParitySeries returns the recorded parity-gap observations.
func (a *ParityArb) ParitySeries() []ParitySample { return a.samples }
func (a *ParityArb) Trades() int64                { return a.trades }

func (a *ParityArb) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventBookSnapshot {
		e := evt.Data.(actor.BookSnapshotEvent)
		if e.Symbol == a.cfg.Underlying {
			if len(e.Snapshot.Bids) > 0 && len(e.Snapshot.Asks) > 0 {
				a.spotMid = (e.Snapshot.Bids[0].Price + e.Snapshot.Asks[0].Price) / 2
			}
			return
		}
		if top, ok := a.tops[e.Symbol]; ok {
			if len(e.Snapshot.Bids) > 0 {
				top.bestBid = e.Snapshot.Bids[0].Price
			}
			if len(e.Snapshot.Asks) > 0 {
				top.bestAsk = e.Snapshot.Asks[0].Price
			}
		}
		return
	}
	a.set.handle(evt)
}

// pairKey groups a call and put of the same strike and expiry.
type pairKey struct {
	strike int64
	expiry int64
}

func (a *ParityArb) onTick(t time.Time) {
	if !a.subscribed {
		a.Subscribe(exchange.InstrumentFeedSymbol, exchange.MDInstrument)
		a.Subscribe(a.cfg.Underlying, exchange.MDSnapshot)
		a.subscribed = true
		return
	}
	if a.spotMid == 0 {
		return
	}
	now := t.UnixNano()

	pairs := make(map[pairKey][2]*Contract) // [call, put]
	for _, c := range a.set.orderedContracts() {
		if c.Type != "OPTION" {
			continue
		}
		k := pairKey{strike: c.Strike, expiry: c.ExpiryNano}
		pair := pairs[k]
		if c.IsCall {
			pair[0] = c
		} else {
			pair[1] = c
		}
		pairs[k] = pair
	}

	keys := make([]pairKey, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b pairKey) int {
		if byExpiry := cmp.Compare(a.expiry, b.expiry); byExpiry != 0 {
			return byExpiry
		}
		return cmp.Compare(a.strike, b.strike)
	})
	for _, k := range keys {
		pair := pairs[k]
		call, put := pair[0], pair[1]
		if call == nil || put == nil {
			continue
		}
		callTop, putTop := a.tops[call.Symbol], a.tops[put.Symbol]
		if callTop == nil || putTop == nil ||
			callTop.bestBid == 0 || callTop.bestAsk == 0 ||
			putTop.bestBid == 0 || putTop.bestAsk == 0 {
			continue
		}
		callMid := (callTop.bestBid + callTop.bestAsk) / 2
		putMid := (putTop.bestBid + putTop.bestAsk) / 2
		gap := (callMid - putMid) - (a.spotMid - k.strike)
		gapBps := float64(gap) / float64(a.spotMid) * 10000
		a.samples = append(a.samples, ParitySample{
			Strike:       k.strike,
			TimeToExpiry: time.Duration(k.expiry - now),
			GapBps:       gapBps,
		})

		if a.trades >= a.cfg.MaxTrades {
			continue
		}
		edge := a.spotMid * a.cfg.EdgeBps / 10000
		switch {
		case gap > edge:
			// Calls rich: conversion — sell call, buy put, buy spot.
			a.leg(call.Symbol, exchange.Sell)
			a.leg(put.Symbol, exchange.Buy)
			a.leg(a.cfg.Underlying, exchange.Buy)
			a.trades++
		case gap < -edge:
			// Puts rich: reversal — buy call, sell put, sell spot.
			a.leg(call.Symbol, exchange.Buy)
			a.leg(put.Symbol, exchange.Sell)
			a.leg(a.cfg.Underlying, exchange.Sell)
			a.trades++
		}
	}
}

func (a *ParityArb) leg(symbol string, side exchange.Side) {
	reqID := a.SubmitOrder(symbol, side, exchange.Market, 0, a.cfg.LotQty)
	a.set.trackRequest(reqID, symbol)
}
