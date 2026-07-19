package feesim

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	etypes "exchange_sim/types"
)

type stubGateway struct {
	requests []etypes.Request
	respCh   chan etypes.Response
	mdCh     chan *etypes.MarketDataMsg
}

func newStubGateway() *stubGateway {
	return &stubGateway{
		respCh: make(chan etypes.Response, 16),
		mdCh:   make(chan *etypes.MarketDataMsg, 16),
	}
}

func (g *stubGateway) ID() uint64                                 { return 1 }
func (g *stubGateway) Send(req etypes.Request)                    { g.requests = append(g.requests, req) }
func (g *stubGateway) Responses() <-chan etypes.Response          { return g.respCh }
func (g *stubGateway) MarketDataCh() <-chan *etypes.MarketDataMsg { return g.mdCh }
func (g *stubGateway) IsRunning() bool                            { return true }

func (g *stubGateway) placed() []*etypes.OrderRequest {
	var out []*etypes.OrderRequest
	for _, r := range g.requests {
		if r.Type == etypes.ReqPlaceOrder {
			out = append(out, r.OrderReq)
		}
	}
	return out
}

// A rejected quote used to leave levelState with price set and no order ID,
// which the refresh guard reads as "accept in flight" — freezing the level
// for the rest of the run.
func TestAdaptiveMMRejectDoesNotFreezeLevel(t *testing.T) {
	gw := newStubGateway()
	mm := NewMarketMaker(1, gw, MMConfig{
		Symbol: "ABC/USD", BootstrapPrice: 1000, Levels: 1,
		LevelSpacing: 1, LevelSize: 10, TickSize: 1,
		BaseInterval: time.Hour, MaxInterval: time.Hour,
	})
	ctx := context.Background()

	mm.subscribed = true
	mm.onBaseTick(time.Now())
	orders := gw.placed()
	if len(orders) != 2 {
		t.Fatalf("want bid+ask, got %d orders", len(orders))
	}

	// Bid accepted, ask rejected.
	mm.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderAccepted,
		Data: actor.OrderAcceptedEvent{OrderID: 11, RequestID: orders[0].RequestID}})
	mm.HandleEvent(ctx, &actor.Event{Type: actor.EventOrderRejected,
		Data: actor.OrderRejectedEvent{RequestID: orders[1].RequestID, Reason: "INSUFFICIENT_BALANCE"}})

	// Next refresh must requote instead of treating the level as in-flight.
	mm.tickCount = 0
	mm.onBaseTick(time.Now())
	if got := len(gw.placed()); got <= 2 {
		t.Fatalf("level frozen after reject: still %d orders placed", got)
	}
}
