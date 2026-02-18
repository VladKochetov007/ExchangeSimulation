package actors

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// PrivateSignalOracle provides a latent "true price" known only to informed traders.
// The simplest implementation is simulation.GBMProcess, but users can inject any signal
// source (e.g. a delayed version of another venue's feed, a proprietary alpha signal).
type PrivateSignalOracle interface {
	GetSignal(symbol string, timestamp int64) int64
}

// InformedTraderConfig configures the informed trader.
type InformedTraderConfig struct {
	Symbol string

	// Oracle is the private signal source. Required.
	Oracle PrivateSignalOracle

	// ThresholdBps: minimum signal deviation from mid before trading, in bps.
	// Prevents noise-trading on tiny signals. Default: 10 (0.1%).
	ThresholdBps int64

	// OrderQty is the size of each market order in base asset units.
	OrderQty int64

	// PollInterval controls how often the actor checks the signal.
	PollInterval time.Duration
}

// InformedTraderActor trades directionally toward the private signal.
//
// When signal > mid + threshold: submit a buy market order (price will rise to signal)
// When signal < mid - threshold: submit a sell market order (price will fall to signal)
//
// This models Kyle (1985) and Glosten-Milgrom (1985) informed traders:
// their order flow is systematically correlated with future price moves, creating
// adverse selection costs for market makers and driving price discovery.
type InformedTraderActor struct {
	*actor.BaseActor
	config  InformedTraderConfig
	mid     int64
	inOrder bool // true while a market order is in flight
}

func NewInformedTrader(id uint64, gateway *exchange.ClientGateway, config InformedTraderConfig) *InformedTraderActor {
	if config.ThresholdBps == 0 {
		config.ThresholdBps = 10
	}
	if config.PollInterval == 0 {
		config.PollInterval = 500 * time.Millisecond
	}
	return &InformedTraderActor{
		BaseActor: actor.NewBaseActor(id, gateway),
		config:    config,
	}
}

func (a *InformedTraderActor) Start(ctx context.Context) error {
	a.Subscribe(a.config.Symbol)
	go a.loop(ctx)
	return a.BaseActor.Start(ctx)
}

func (a *InformedTraderActor) loop(ctx context.Context) {
	ticker := a.GetTickerFactory().NewTicker(a.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-a.EventChannel():
			a.OnEvent(event)
		case <-ticker.C():
			a.checkSignal()
		}
	}
}

func (a *InformedTraderActor) OnEvent(event *actor.Event) {
	switch event.Type {
	case actor.EventBookSnapshot:
		snap := event.Data.(actor.BookSnapshotEvent)
		if len(snap.Snapshot.Bids) > 0 && len(snap.Snapshot.Asks) > 0 {
			a.mid = snap.Snapshot.Bids[0].Price + (snap.Snapshot.Asks[0].Price-snap.Snapshot.Bids[0].Price)/2
		}
	case actor.EventBookDelta:
		delta := event.Data.(actor.BookDeltaEvent)
		if delta.Delta.Side == exchange.Buy && delta.Delta.VisibleQty > 0 {
			a.mid = delta.Delta.Price
		}
	case actor.EventOrderFilled, actor.EventOrderPartialFill:
		if event.Data.(actor.OrderFillEvent).IsFull {
			a.inOrder = false
		}
	case actor.EventOrderCancelled, actor.EventOrderRejected:
		a.inOrder = false
	}
}

func (a *InformedTraderActor) checkSignal() {
	if a.mid == 0 || a.inOrder {
		return
	}

	signal := a.config.Oracle.GetSignal(a.config.Symbol, 0)
	if signal == 0 {
		return
	}

	threshold := a.mid * a.config.ThresholdBps / 10000
	if signal > a.mid+threshold {
		a.inOrder = true
		a.SubmitOrder(a.config.Symbol, exchange.Buy, exchange.Market, 0, a.config.OrderQty)
	} else if signal < a.mid-threshold {
		a.inOrder = true
		a.SubmitOrder(a.config.Symbol, exchange.Sell, exchange.Market, 0, a.config.OrderQty)
	}
}
