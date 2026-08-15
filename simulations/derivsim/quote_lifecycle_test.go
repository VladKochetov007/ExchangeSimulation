package derivsim

import (
	"context"
	"math"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
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

func TestOptionMMGreekProfileUsesFilledInventoryAndHedge(t *testing.T) {
	gw := newStubGateway()
	mm := NewOptionMarketMaker(1, gw, OptionMMConfig{
		Underlying: "ABC/USD", IV: 0.8,
		QuoteInterval: time.Hour, BasePrecision: 1_000,
	})
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	expiry := now.Add(24 * time.Hour).UnixNano()
	listOption(mm, "ABC-C-100", 10_000, expiry)
	mm.spotMid = 10_000
	mm.quotes["ABC-C-100"].inventory = 2_000 // two contracts long
	mm.hedgePos = -500                       // half a base unit short

	got := mm.GreekProfile(now)
	greeks, ok := eprice.Black76Sensitivities(10_000, 10_000, 0.8, 1.0/365.0, true)
	if !ok {
		t.Fatal("reference Black-76 calculation invalid")
	}
	if math.Abs(got.OptionDelta-2*greeks.Delta) > 1e-12 {
		t.Fatalf("option delta = %.12f, want %.12f", got.OptionDelta, 2*greeks.Delta)
	}
	if math.Abs(got.Gamma-2*greeks.Gamma) > 1e-12 || math.Abs(got.Vega-2*greeks.Vega) > 1e-9 {
		t.Fatalf("profile curvature mismatch: got=%+v reference=%+v", got, greeks)
	}
	if got.HedgeDelta != -0.5 || math.Abs(got.NetDelta-(got.OptionDelta-0.5)) > 1e-12 {
		t.Fatalf("hedge aggregation wrong: %+v", got)
	}
	if got.ModelForward != mm.spotMid || got.ForwardSource != "spot_mid_proxy" {
		t.Fatalf("forward provenance missing: %+v", got)
	}
	if got.Contracts != 1 {
		t.Fatalf("active option contracts = %d, want 1", got.Contracts)
	}
	positions := mm.GreekPositions(now)
	if len(positions) != 1 || positions[0].Symbol != "ABC-C-100" || positions[0].Position != 2_000 {
		t.Fatalf("per-contract profile missing inventory: %+v", positions)
	}
	if math.Abs(positions[0].Delta-2*greeks.Delta) > 1e-12 || positions[0].TimeToExpiryNano != int64(24*time.Hour) {
		t.Fatalf("per-contract Greek values wrong: %+v", positions[0])
	}
}

func TestOptionMMHedgePendingPreventsRepeatedCorrection(t *testing.T) {
	gw := newStubGateway()
	mm := NewOptionMarketMaker(1, gw, OptionMMConfig{Underlying: "ABC/USD", QuoteInterval: time.Hour})
	mm.hedgePending = 100
	mm.hedgeRequests[7] = 100

	mm.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventOrderAccepted,
		Data: actor.OrderAcceptedEvent{RequestID: 7, OrderID: 70},
	})
	if _, exists := mm.hedgeRequests[7]; exists || mm.hedgeOrders[70] != 100 {
		t.Fatalf("accepted hedge was not moved into live state: requests=%v orders=%v", mm.hedgeRequests, mm.hedgeOrders)
	}
	mm.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventOrderPartialFill,
		Data: actor.OrderFillEvent{OrderID: 70, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 40},
	})
	if mm.hedgePos != 40 || mm.hedgePending != 60 {
		t.Fatalf("partial hedge state = position %d pending %d, want 40/60", mm.hedgePos, mm.hedgePending)
	}
	mm.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventOrderCancelled,
		Data: actor.OrderCancelledEvent{OrderID: 70, RemainingQty: 60},
	})
	if mm.hedgePending != 0 {
		t.Fatalf("cancelled hedge left pending delta %d", mm.hedgePending)
	}
}
