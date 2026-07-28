package derivsim

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

// stubGateway records outbound requests; responses are fed by the test.
type stubGateway struct {
	requests []etypes.Request
	respCh   chan etypes.Response
	mdCh     chan *etypes.MarketDataMsg
}

func newStubGateway() *stubGateway {
	return &stubGateway{
		respCh: make(chan etypes.Response, 100),
		mdCh:   make(chan *etypes.MarketDataMsg, 100),
	}
}

func (g *stubGateway) ID() uint64                                 { return 1 }
func (g *stubGateway) Send(req etypes.Request)                    { g.requests = append(g.requests, req) }
func (g *stubGateway) Responses() <-chan etypes.Response          { return g.respCh }
func (g *stubGateway) MarketDataCh() <-chan *etypes.MarketDataMsg { return g.mdCh }
func (g *stubGateway) IsRunning() bool                            { return true }

func (g *stubGateway) placedOrders() []*etypes.OrderRequest {
	var out []*etypes.OrderRequest
	for _, r := range g.requests {
		if r.Type == etypes.ReqPlaceOrder {
			out = append(out, r.OrderReq)
		}
	}
	return out
}

func (g *stubGateway) cancels() []*etypes.CancelRequest {
	var out []*etypes.CancelRequest
	for _, r := range g.requests {
		if r.Type == etypes.ReqCancelOrder {
			out = append(out, r.CancelReq)
		}
	}
	return out
}

// acceptAll feeds an accept event for every unacknowledged placed order,
// assigning order IDs starting at nextOrderID. Returns the next free ID.
func acceptAll(mm interface {
	HandleEvent(context.Context, *actor.Event)
}, gw *stubGateway, acked map[uint64]bool, nextOrderID uint64) uint64 {
	for _, req := range gw.placedOrders() {
		if acked[req.RequestID] {
			continue
		}
		acked[req.RequestID] = true
		mm.HandleEvent(context.Background(), &actor.Event{
			Type: actor.EventOrderAccepted,
			Data: actor.OrderAcceptedEvent{OrderID: nextOrderID, RequestID: req.RequestID},
		})
		nextOrderID++
	}
	return nextOrderID
}

func listOption(mm interface {
	HandleEvent(context.Context, *actor.Event)
}, symbol string, strike int64, expiry int64) {
	mm.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventInstrument,
		Data: actor.InstrumentEvent{Announcement: &etypes.InstrumentAnnouncement{
			Action: "listed", Symbol: symbol, InstrumentType: "OPTION",
			Underlying: "ABC/USD", Strike: strike, IsCall: true, ExpiryNano: expiry,
		}},
	})
}

// The full requote lifecycle: quote → accept → price move → cancel + requote.
// Regression for two bugs: order IDs never captured from accepts (quotes were
// never cancelled → zombie accumulation), and the pending gate sticking.
func TestOptionMMRequoteLifecycle(t *testing.T) {
	gw := newStubGateway()
	mm := NewOptionMarketMaker(1, gw, OptionMMConfig{
		Underlying: "ABC/USD", IV: 0.8,
		SpreadBps: 20, QuoteQty: 1000, LotQty: 1000, PremiumTick: 100,
		QuoteInterval: time.Hour, // ticks driven manually
	})

	// Expiry far enough out that ATM theo exceeds the half-spread, so both
	// bid and ask are quoted.
	expiry := time.Now().Add(24 * time.Hour).UnixNano()
	listOption(mm, "ABC-C-100", exchange.USDAmount(100), expiry)

	mm.subscribed = true
	mm.spotMid = exchange.USDAmount(100)
	mm.onQuoteTick(time.Now())

	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("initial quote: want 2 orders, got %d", got)
	}

	acked := map[uint64]bool{}
	nextID := acceptAll(mm, gw, acked, 100)

	// Same price → no churn.
	mm.onQuoteTick(time.Now())
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("unchanged price must not requote, got %d orders", got)
	}

	// Price moves → cancel both stale quotes, submit two fresh ones.
	mm.spotMid = exchange.USDAmount(105)
	mm.onQuoteTick(time.Now())
	if got := len(gw.cancels()); got != 2 {
		t.Fatalf("price move must cancel stale quotes, got %d cancels", got)
	}
	if got := len(gw.placedOrders()); got != 4 {
		t.Fatalf("price move must submit fresh quotes, got %d total orders", got)
	}

	// While the fresh submits are unacknowledged, further ticks must not stack
	// more orders (the in-flight gate).
	mm.spotMid = exchange.USDAmount(110)
	mm.onQuoteTick(time.Now())
	if got := len(gw.placedOrders()); got != 4 {
		t.Fatalf("in-flight gate breached: %d orders", got)
	}

	// Accepts land → the next price move requotes again (gate released).
	nextID = acceptAll(mm, gw, acked, nextID)
	mm.onQuoteTick(time.Now())
	if got := len(gw.placedOrders()); got != 6 {
		t.Fatalf("gate must release after accepts, got %d orders", got)
	}
	if got := len(gw.cancels()); got != 4 {
		t.Fatalf("second requote must cancel previous pair, got %d cancels", got)
	}
	_ = nextID
}

// Futures MM: same lifecycle against the self-anchored quoting path.
func TestFuturesMMRequoteLifecycle(t *testing.T) {
	gw := newStubGateway()
	mm := NewFuturesMarketMaker(1, gw, FuturesMMConfig{
		Underlying: "ABC/USD", SpreadBps: 10, QuoteQty: 1000,
		Tick: 100, QuoteInterval: time.Hour,
	})

	mm.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventInstrument,
		Data: actor.InstrumentEvent{Announcement: &etypes.InstrumentAnnouncement{
			Action: "listed", Symbol: "ABC-FUT-1", InstrumentType: "FUTURE",
			Underlying: "ABC/USD", ExpiryNano: time.Now().Add(time.Hour).UnixNano(),
		}},
	})

	mm.subscribed = true
	mm.spotMid = exchange.USDAmount(100)
	mm.onTick(time.Now())
	if got := len(gw.placedOrders()); got != 2 {
		t.Fatalf("initial quote: want 2 orders, got %d", got)
	}

	acked := map[uint64]bool{}
	acceptAll(mm, gw, acked, 500)

	mm.spotMid = exchange.USDAmount(101)
	mm.onTick(time.Now())
	if got := len(gw.cancels()); got != 2 {
		t.Fatalf("price move must cancel stale quotes, got %d cancels", got)
	}
	if got := len(gw.placedOrders()); got != 4 {
		t.Fatalf("price move must requote, got %d orders", got)
	}
}
