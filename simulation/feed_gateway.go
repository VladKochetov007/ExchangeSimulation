package simulation

import (
	"fmt"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// FeedOnlyGateway grants a participant component access to a venue's public
// data session without an order-entry path. It is used for V2 remote feeds so
// an observation account cannot become an unrecorded trading account.
type FeedOnlyGateway struct {
	inner actor.Gateway
}

func NewFeedOnlyGateway(inner actor.Gateway) *FeedOnlyGateway {
	if inner == nil {
		panic("simulation: feed-only gateway needs an inner gateway")
	}
	return &FeedOnlyGateway{inner: inner}
}

func (g *FeedOnlyGateway) ID() uint64 { return g.inner.ID() }

func (g *FeedOnlyGateway) Send(request exchange.Request) {
	switch request.Type {
	case exchange.ReqSubscribe, exchange.ReqUnsubscribe:
		g.inner.Send(request)
	default:
		panic(fmt.Sprintf("simulation: feed-only gateway %d rejected %s", g.ID(), request.Type))
	}
}

func (g *FeedOnlyGateway) Responses() <-chan exchange.Response { return g.inner.Responses() }
func (g *FeedOnlyGateway) MarketDataCh() <-chan *exchange.MarketDataMsg {
	return g.inner.MarketDataCh()
}
func (g *FeedOnlyGateway) IsRunning() bool { return g.inner.IsRunning() }

// MarketDataFrontier exposes only immutable evidence metadata from the
// delayed courier beneath this feed session. A V2 remote feed must be delayed;
// panic instead of silently claiming a direct link has an audited frontier.
func (g *FeedOnlyGateway) MarketDataFrontier() MarketDataFrontier {
	delayed, ok := g.inner.(*DelayedGateway)
	if !ok {
		panic("simulation: feed-only gateway has no delayed market-data frontier")
	}
	return delayed.MarketDataFrontier()
}

var _ actor.Gateway = (*FeedOnlyGateway)(nil)
