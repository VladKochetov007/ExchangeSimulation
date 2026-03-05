package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type BasisArbConfig struct {
	SpotSymbol   string
	PerpSymbol   string
	ThresholdBps int64 // price basis (bps) to start opening / stop closing
	LotSize      int64 // qty per individual trade
	MaxPosition  int64 // max abs position in lots
}

// BasisArbActor continuously arbitrages the price basis between a perp and its
// spot reference. It subscribes to trade events on both symbols to track current
// prices, and checks the basis every 100ms to add or reduce its position.
//
// Opening logic (one lot per tick):
//   basis > threshold  → sell perp + buy spot  (positive lot)
//   basis < -threshold → buy perp + sell spot   (negative lot)
//
// Closing logic (one lot per tick): unwind when |basis| < threshold/2.
type BasisArbActor struct {
	*actor.BaseActor
	cfg        BasisArbConfig
	perpPrice  int64
	spotPrice  int64
	position   int64 // net lots: +N = N×(short-perp/long-spot)
	subscribed bool
}

func NewBasisArbActor(id uint64, gw actor.Gateway, cfg BasisArbConfig) *BasisArbActor {
	a := &BasisArbActor{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
	}
	a.SetHandler(a)
	a.AddTicker(100*time.Millisecond, a.onTick)
	return a
}

func (a *BasisArbActor) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventTrade {
		a.onTrade(evt.Data.(actor.TradeEvent))
	}
}

func (a *BasisArbActor) onTrade(e actor.TradeEvent) {
	switch e.Symbol {
	case a.cfg.PerpSymbol:
		a.perpPrice = e.Trade.Price
	case a.cfg.SpotSymbol:
		a.spotPrice = e.Trade.Price
	}
}

func (a *BasisArbActor) onTick(_ time.Time) {
	if !a.subscribed {
		a.Subscribe(a.cfg.SpotSymbol, exchange.MDTrade)
		a.Subscribe(a.cfg.PerpSymbol, exchange.MDTrade)
		a.subscribed = true
	}
	a.checkBasis()
}

func (a *BasisArbActor) checkBasis() {
	if a.perpPrice == 0 || a.spotPrice == 0 {
		return
	}
	basis := a.perpPrice - a.spotPrice
	threshold := a.cfg.ThresholdBps * a.spotPrice / 10000

	switch {
	case basis > threshold && a.position < a.cfg.MaxPosition:
		// Perp premium: sell perp + buy spot to capture the spread.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++

	case basis < -threshold && a.position > -a.cfg.MaxPosition:
		// Spot premium: buy perp + sell spot.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--

	case a.position > 0 && basis < threshold/2:
		// Was short-perp/long-spot; basis has narrowed — reduce position.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--

	case a.position < 0 && basis > -threshold/2:
		// Was long-perp/short-spot; basis has narrowed — reduce position.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
