package feesim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type FundingArbConfig struct {
	SpotSymbol  string
	PerpSymbol  string
	SpotFeeBps  int64 // taker fee on spot leg
	PerpFeeBps  int64 // taker fee on perp leg
	LotSize     int64
	MaxPosition int64
	EntryWindow time.Duration // build position within this window before settlement
}

// FeeAwareFundingArb collects funding payments, only entering when expected
// revenue per period exceeds the round-trip fee cost (entry + exit fees).
type FeeAwareFundingArb struct {
	*actor.BaseActor
	cfg         FundingArbConfig
	position    int64
	nextFunding int64 // last known NextFunding timestamp in nanoseconds
}

func NewFeeAwareFundingArb(id uint64, gw actor.Gateway, cfg FundingArbConfig) *FeeAwareFundingArb {
	a := &FeeAwareFundingArb{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
	}
	a.SetHandler(a)
	return a
}

func (a *FeeAwareFundingArb) Start(ctx context.Context) error {
	a.Subscribe(a.cfg.PerpSymbol, exchange.MDFunding)
	return a.BaseActor.Start(ctx)
}

func (a *FeeAwareFundingArb) HandleEvent(_ context.Context, evt *actor.Event) {
	if evt.Type == actor.EventFundingUpdate {
		a.onFundingUpdate(evt.Data.(actor.FundingUpdateEvent))
	}
}

// roundTripCostBps returns the total fee cost in bps for entering AND exiting
// a hedged position (spot + perp on each leg).
func (a *FeeAwareFundingArb) roundTripCostBps() int64 {
	return (a.cfg.SpotFeeBps + a.cfg.PerpFeeBps) * 2
}

func (a *FeeAwareFundingArb) onFundingUpdate(e actor.FundingUpdateEvent) {
	if e.Symbol != a.cfg.PerpSymbol {
		return
	}
	fr := e.FundingRate
	rate := fr.Rate

	// Detect settlement: NextFunding timestamp advanced.
	settlementOccurred := a.nextFunding > 0 && fr.NextFunding > a.nextFunding
	a.nextFunding = fr.NextFunding

	if settlementOccurred {
		a.unwindIfStale(rate)
		return
	}

	// Only enter within the entry window before settlement.
	timeToNextNs := fr.NextFunding - e.Timestamp
	if timeToNextNs < 0 || timeToNextNs > int64(a.cfg.EntryWindow) {
		return
	}

	// Fee-aware: only enter if |rate| exceeds round-trip cost.
	// The funding payment per period is rate × notional / 10000.
	// The cost of entry+exit is roundTripCostBps × notional / 10000.
	minRateBps := a.roundTripCostBps()

	switch {
	case rate > minRateBps && a.position < a.cfg.MaxPosition:
		// Positive funding: longs pay shorts → short perp + long spot to collect.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++

	case rate < -minRateBps && a.position > -a.cfg.MaxPosition:
		// Negative funding: shorts pay longs → long perp + short spot to collect.
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--
	}
}

func (a *FeeAwareFundingArb) unwindIfStale(rate int64) {
	minRateBps := a.roundTripCostBps() / 2 // lower threshold for closing

	switch {
	case a.position > 0 && rate < minRateBps:
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.position--

	case a.position < 0 && rate > -minRateBps:
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.LotSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.LotSize)
		a.position++
	}
}
