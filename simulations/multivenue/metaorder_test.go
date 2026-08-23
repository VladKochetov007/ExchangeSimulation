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

// A round-trip desk funded like a noise trader can only ever sell: one lot of
// quote currency costs far more than the noise balance holds, so every buy is
// rejected and the desk becomes one-sided flow instead of the symmetric flow it
// exists to provide.
func TestRoundTripBalancesCoverALongOpen(t *testing.T) {
	const (
		basePrecision  int64 = 100_000_000
		quotePrecision int64 = 1_000_000
		price          int64 = 50_000 * quotePrecision
		lotQty         int64 = basePrecision / 10
	)
	balances := roundTripBalances(lotQty, price, basePrecision, 20, nil)

	oneLotCost := lotQty * price / basePrecision
	if balances["USD"] < oneLotCost {
		t.Fatalf("quote balance %d cannot fund one lot costing %d", balances["USD"], oneLotCost)
	}
	if balances["ABC"] < lotQty {
		t.Fatalf("base balance %d cannot fund one lot of %d", balances["ABC"], lotQty)
	}
	if got, want := balances["USD"], 20*oneLotCost; got != want {
		t.Fatalf("quote balance = %d, want %d lots of headroom", got, want)
	}
	if _, ok := balances["CDF"]; ok {
		t.Fatal("cross asset funded when not requested")
	}
	withCross := roundTripBalances(lotQty, price, basePrecision, 20, []string{"CDF"})
	if withCross["CDF"] != 20*lotQty {
		t.Fatalf("cross asset balance = %d, want %d", withCross["CDF"], 20*lotQty)
	}
}

// Records from several desks are merged into one list in the run report. Without
// a trader identity they cannot be separated, and any sequential measure taken
// across the merged list — the gap between one parent ending and the next
// starting — silently mixes desks and produces overlapping intervals.
func TestMetaorderRecordsCarryTraderIdentity(t *testing.T) {
	const symbol = "ABC/USD"
	cfg := MetaorderTraderConfig{
		Symbol: symbol, BasePrecision: 100_000_000, TickSize: 1,
		MinQty: 1_000, MaxQty: 1_000, ParetoAlpha: 2,
		ChildInterval: time.Millisecond, MinChildQty: 1_000, MinOrderSize: 500,
		MaxSlippageBps: 10, MaxDuration: time.Hour, Seed: 1,
	}
	var records []MetaorderRecord
	for _, id := range []uint64{7, 9} {
		trader := NewMetaorderTrader(id, newMetaGateway(), "v1", cfg)
		now := time.Unix(0, 0)
		trader.onTick(now)
		quoteBook(trader, symbol, 99, 101, 10_000)
		now = now.Add(time.Millisecond)
		trader.onTick(now)
		now = now.Add(time.Millisecond)
		trader.onTick(now)
		fillChild(trader, symbol, 1_000, 101)
		now = now.Add(time.Millisecond)
		trader.onTick(now)
		records = append(records, trader.Records()...)
	}
	if len(records) != 2 {
		t.Fatalf("expected one record per desk, got %d", len(records))
	}
	if records[0].ID != records[1].ID {
		t.Fatalf("precondition failed: record IDs %d and %d should collide across desks",
			records[0].ID, records[1].ID)
	}
	if records[0].TraderID == records[1].TraderID {
		t.Fatalf("records from different desks share trader ID %d", records[0].TraderID)
	}
	if records[0].TraderID != 7 || records[1].TraderID != 9 {
		t.Fatalf("trader IDs = %d, %d; want 7, 9", records[0].TraderID, records[1].TraderID)
	}
}

// A legitimate parent can accumulate a quote notional whose multiplication by
// base precision would overflow int64 even though its weighted execution price
// is ordinary. VWAP must remain a usable positive price rather than wrapping
// negative, and no-fill parents must make absence explicit.
func TestMetaorderVWAPUsesExactWideAccumulator(t *testing.T) {
	const (
		symbol        = "ABC/USD"
		basePrecision = int64(100_000_000)
		price         = int64(5_000_000_000)
		childQty      = int64(500_000_000)
	)
	trader := NewMetaorderTrader(1, newMetaGateway(), "v1", MetaorderTraderConfig{
		Symbol: symbol, BasePrecision: basePrecision, TickSize: 1,
		MinQty: 4 * childQty, MaxQty: 4 * childQty, ParetoAlpha: 2,
		ChildInterval: time.Millisecond, MinChildQty: childQty, MaxDuration: time.Hour,
	})
	trader.parentQty, trader.startMid = 4*childQty, price
	for range 4 {
		fillChild(trader, symbol, childQty, price)
	}
	trader.finish(time.Unix(1, 0).UnixNano(), true)
	records := trader.Records()
	if len(records) != 1 || records[0].VWAP == nil || *records[0].VWAP != price {
		t.Fatalf("wide VWAP record = %#v, want price %d", records, price)
	}

	noFill := NewMetaorderTrader(2, newMetaGateway(), "v1", trader.cfg)
	noFill.parentQty, noFill.startMid = childQty, price
	noFill.finish(time.Unix(1, 0).UnixNano(), false)
	if record := noFill.Records()[0]; record.VWAP != nil {
		t.Fatalf("no-fill VWAP = %d, want unavailable", *record.VWAP)
	}
}
