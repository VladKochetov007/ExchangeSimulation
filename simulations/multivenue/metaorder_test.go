package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// metaGateway records outbound requests without an exchange behind it.
type metaGateway struct {
	requests []etypes.Request
	respCh   chan etypes.Response
	mdCh     chan *etypes.MarketDataMsg
}

func newMetaGateway() *metaGateway {
	return &metaGateway{
		respCh: make(chan etypes.Response, 64),
		mdCh:   make(chan *etypes.MarketDataMsg, 64),
	}
}

func (g *metaGateway) ID() uint64                                 { return 1 }
func (g *metaGateway) Send(req etypes.Request)                    { g.requests = append(g.requests, req) }
func (g *metaGateway) Responses() <-chan etypes.Response          { return g.respCh }
func (g *metaGateway) MarketDataCh() <-chan *etypes.MarketDataMsg { return g.mdCh }
func (g *metaGateway) IsRunning() bool                            { return true }

func (g *metaGateway) orders() []*etypes.OrderRequest {
	var out []*etypes.OrderRequest
	for _, r := range g.requests {
		if r.Type == etypes.ReqPlaceOrder {
			out = append(out, r.OrderReq)
		}
	}
	return out
}

func quoteBook(m *MetaorderTrader, symbol string, bid, ask, size int64) {
	m.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: symbol,
			Snapshot: &etypes.BookSnapshot{
				Bids: []etypes.PriceLevel{{Price: bid, VisibleQty: size}},
				Asks: []etypes.PriceLevel{{Price: ask, VisibleQty: size}},
			},
		},
	})
}

// fillChild acknowledges the most recent child as fully filled.
func fillChild(m *MetaorderTrader, symbol string, qty, price int64) {
	m.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{Symbol: symbol, Qty: qty, Price: price, IsFull: true},
	})
}

// A parent whose residual falls below the venue's minimum order size can never
// be completed by another child: every such child is rejected for INVALID_QTY.
// The agent must retire the parent instead of resubmitting a quantity the
// exchange is guaranteed to refuse.
func TestMetaorderRetiresParentBelowMinimumOrderSize(t *testing.T) {
	const symbol = "ABC/USD"
	gw := newMetaGateway()
	trader := NewMetaorderTrader(1, gw, "v1", MetaorderTraderConfig{
		Symbol:         symbol,
		BasePrecision:  100_000_000,
		TickSize:       1,
		MinQty:         1_000,
		MaxQty:         1_000,
		ParetoAlpha:    2,
		ChildInterval:  time.Millisecond,
		MinChildQty:    900,
		MinOrderSize:   500,
		MaxSlippageBps: 10,
		MaxDuration:    time.Hour,
		Seed:           1,
	})

	now := time.Unix(0, 0)
	trader.onTick(now) // subscribes only
	quoteBook(trader, symbol, 99, 101, 10_000)

	now = now.Add(time.Millisecond)
	trader.onTick(now) // begins the parent
	now = now.Add(time.Millisecond)
	trader.onTick(now) // submits the first child
	placed := gw.orders()
	if len(placed) != 1 {
		t.Fatalf("expected one child, got %d", len(placed))
	}
	// Leaves a residual of 100, below the 500 minimum.
	fillChild(trader, symbol, 900, 101)

	before := len(gw.orders())
	for i := 0; i < 5; i++ {
		now = now.Add(time.Millisecond)
		trader.onTick(now)
	}
	for i, order := range gw.orders()[before:] {
		if order.Qty < 500 {
			t.Fatalf("child %d submitted qty %d below the 500 minimum order size", i, order.Qty)
		}
	}
	records := trader.Records()
	if len(records) == 0 {
		t.Fatal("expected the parent to be retired, got no records")
	}
	if records[0].FilledQty != 900 {
		t.Fatalf("retired parent recorded %d filled, want 900", records[0].FilledQty)
	}
}

// MinOrderSize is optional: leaving it unset must not retire parents early.
func TestMetaorderWithoutMinimumOrderSizeStillWorksTheResidual(t *testing.T) {
	const symbol = "ABC/USD"
	gw := newMetaGateway()
	trader := NewMetaorderTrader(1, gw, "v1", MetaorderTraderConfig{
		Symbol:         symbol,
		BasePrecision:  100_000_000,
		TickSize:       1,
		MinQty:         1_000,
		MaxQty:         1_000,
		ParetoAlpha:    2,
		ChildInterval:  time.Millisecond,
		MinChildQty:    900,
		MaxSlippageBps: 10,
		MaxDuration:    time.Hour,
		Seed:           1,
	})
	now := time.Unix(0, 0)
	trader.onTick(now)
	quoteBook(trader, symbol, 99, 101, 10_000)
	now = now.Add(time.Millisecond)
	trader.onTick(now) // begins the parent
	now = now.Add(time.Millisecond)
	trader.onTick(now) // submits the first child
	fillChild(trader, symbol, 900, 101)

	before := len(gw.orders())
	now = now.Add(time.Millisecond)
	trader.onTick(now)
	if len(gw.orders()) != before+1 {
		t.Fatal("residual child not submitted when no minimum is configured")
	}
	if got := gw.orders()[before].Qty; got != 100 {
		t.Fatalf("residual child qty = %d, want 100", got)
	}
}

var _ = exchange.Buy
