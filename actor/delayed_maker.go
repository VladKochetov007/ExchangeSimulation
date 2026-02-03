package actor

import (
	"context"
	"time"

	"exchange_sim/exchange"
)

type DelayedMakerConfig struct {
	Symbol      string
	StartDelay  time.Duration
	OrderCount  int
	BasePrice   int64
	PriceSpread int64
	Qty         int64
}

type DelayedMakerActor struct {
	*BaseActor
	Config DelayedMakerConfig
}

func NewDelayedMaker(id uint64, gateway *exchange.ClientGateway, config DelayedMakerConfig) *DelayedMakerActor {
	return &DelayedMakerActor{
		BaseActor: NewBaseActor(id, gateway),
		Config:    config,
	}
}

func (a *DelayedMakerActor) Start(ctx context.Context) error {
	a.Subscribe(a.Config.Symbol)
	err := a.BaseActor.Start(ctx)
	if err != nil {
		return err
	}
	a.StartLogic(ctx)
	return nil
}

func (a *DelayedMakerActor) StartLogic(ctx context.Context) {
	go func() {
		// Wait for delay
		select {
		case <-time.After(a.Config.StartDelay):
		case <-ctx.Done():
			return
		}
		a.placeOrders()
	}()
}

func (a *DelayedMakerActor) placeOrders() {
	// Place a few orders around BasePrice
	for i := 0; i < a.Config.OrderCount; i++ {
		// Create a Buy order
		buyPrice := a.Config.BasePrice - (int64(i+1) * a.Config.PriceSpread)
		a.SubmitOrder(a.Config.Symbol, exchange.Buy, exchange.LimitOrder, buyPrice, a.Config.Qty)

		// Create a Sell order
		sellPrice := a.Config.BasePrice + (int64(i+1) * a.Config.PriceSpread)
		a.SubmitOrder(a.Config.Symbol, exchange.Sell, exchange.LimitOrder, sellPrice, a.Config.Qty)
	}
}

func (a *DelayedMakerActor) OnEvent(event *Event) {
	// DelayedMaker doesn't need to react to events for this simulation
}
