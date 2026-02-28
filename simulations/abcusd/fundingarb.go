package abcusd

import (
	"context"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type FundingArbConfig struct {
	SpotSymbol        string
	PerpSymbol        string
	OpenThresholdBps  int64
	CloseThresholdBps int64
	PositionSize      int64
}

type FundingArbActor struct {
	*actor.BaseActor
	cfg        FundingArbConfig
	inPosition bool
	direction  int64 // +1 = short perp/long spot, -1 = long perp/short spot
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
	a.Subscribe(a.cfg.SpotSymbol, exchange.MDSnapshot)
	a.Subscribe(a.cfg.PerpSymbol, exchange.MDSnapshot, exchange.MDFunding)
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
	rate := e.FundingRate.Rate

	switch {
	case !a.inPosition && rate > a.cfg.OpenThresholdBps:
		// Positive funding: longs pay shorts → earn by shorting perp + longing spot
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.PositionSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.PositionSize)
		a.inPosition = true
		a.direction = 1

	case !a.inPosition && rate < -a.cfg.OpenThresholdBps:
		// Negative funding: shorts pay longs → earn by longing perp + shorting spot
		a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.PositionSize)
		a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.PositionSize)
		a.inPosition = true
		a.direction = -1

	case a.inPosition && absInt64(rate) < a.cfg.CloseThresholdBps:
		if a.direction > 0 {
			a.SubmitOrder(a.cfg.PerpSymbol, exchange.Buy, exchange.Market, 0, a.cfg.PositionSize)
			a.SubmitOrder(a.cfg.SpotSymbol, exchange.Sell, exchange.Market, 0, a.cfg.PositionSize)
		} else {
			a.SubmitOrder(a.cfg.PerpSymbol, exchange.Sell, exchange.Market, 0, a.cfg.PositionSize)
			a.SubmitOrder(a.cfg.SpotSymbol, exchange.Buy, exchange.Market, 0, a.cfg.PositionSize)
		}
		a.inPosition = false
	}
}

func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
