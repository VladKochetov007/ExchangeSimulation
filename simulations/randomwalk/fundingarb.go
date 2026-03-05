package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// FundingArbConfig configures a carry-trade funding arbitrageur.
//
// Strategy:
//   - On each MDFunding update (every PriceUpdateInterval), check time to next settlement.
//   - Within EntryWindow before settlement: if |rate| > OpenThresholdBps, build position
//     one lot at a time up to MaxPosition.
//   - After each settlement: unwind one lot if |rate| dropped below CloseThresholdBps.
//
// Actors know the next settlement time via FundingRate.NextFunding (Unix ns).
// Time-to-settlement = FundingRate.NextFunding - event.Timestamp.
type FundingArbConfig struct {
	SpotSymbol        string
	PerpSymbol        string
	OpenThresholdBps  int64
	CloseThresholdBps int64
	LotSize           int64
	MaxPosition       int64
	EntryWindow       time.Duration // build position within this window before settlement
}

// FundingArbActor implements a funding carry trade.
// position > 0 means short perp + long spot (collecting positive funding).
// position < 0 means long perp + short spot (collecting negative funding).
type FundingArbActor struct {
	*actor.BaseActor
	cfg         FundingArbConfig
	position    int64
	nextFunding int64 // last known NextFunding timestamp in nanoseconds
}

func NewFundingArbActor(id uint64, gw actor.Gateway, cfg FundingArbConfig) *FundingArbActor {
	a := &FundingArbActor{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
	}
	a.SetHandler(a)
	return a
}

func (a *FundingArbActor) Start(ctx context.Context) error {
	a.Subscribe(a.cfg.PerpSymbol, exchange.MDFunding)
	return a.BaseActor.Start(ctx)
}

func (a *FundingArbActor) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventFundingUpdate {
		a.onFundingUpdate(evt.Data.(actor.FundingUpdateEvent))
	}
}

func (a *FundingArbActor) onFundingUpdate(e actor.FundingUpdateEvent) {
	if e.Symbol != a.cfg.PerpSymbol {
		return
	}
	fr := e.FundingRate
	rate := fr.Rate

	// Detect settlement: NextFunding timestamp advanced to a new 8-hour cycle.
	settlementOccurred := a.nextFunding > 0 && fr.NextFunding > a.nextFunding
	a.nextFunding = fr.NextFunding

	if settlementOccurred {
		// Funding payment was collected. Unwind if the rate is no longer attractive.
		a.unwindIfStale(rate)
		return
	}

	// Time remaining until next settlement (nanoseconds).
	timeToNextNs := fr.NextFunding - e.Timestamp
	if timeToNextNs < 0 || timeToNextNs > int64(a.cfg.EntryWindow) {
		return // outside entry window
	}

	switch {
	case rate > a.cfg.OpenThresholdBps && a.position < a.cfg.MaxPosition:
		// Positive funding: longs pay shorts → short perp + long spot to collect.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++

	case rate < -a.cfg.OpenThresholdBps && a.position > -a.cfg.MaxPosition:
		// Negative funding: shorts pay longs → long perp + short spot to collect.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--
	}
}

func (a *FundingArbActor) unwindIfStale(rate int64) {
	switch {
	case a.position > 0 && rate < a.cfg.CloseThresholdBps:
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--

	case a.position < 0 && rate > -a.cfg.CloseThresholdBps:
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++
	}
}
