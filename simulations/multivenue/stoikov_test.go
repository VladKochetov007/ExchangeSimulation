package multivenue

import (
	"context"
	"math"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

func TestCalculateStoikovQuoteInventorySkewsReservation(t *testing.T) {
	base := StoikovInputs{
		Forward: 100, VariancePerSecond: 4, RiskAversion: 0.1,
		FillDecay: 2, InventoryHorizon: 10 * time.Second,
	}
	flat, ok := CalculateStoikovQuote(base)
	if !ok {
		t.Fatal("flat quote invalid")
	}
	if math.Abs(flat.Reservation-100) > 1e-12 {
		t.Fatalf("flat reservation = %f, want 100", flat.Reservation)
	}
	long := base
	long.Inventory = 3
	longQuote, ok := CalculateStoikovQuote(long)
	if !ok {
		t.Fatal("long quote invalid")
	}
	if longQuote.Reservation >= flat.Reservation || longQuote.Bid >= flat.Bid || longQuote.Ask >= flat.Ask {
		t.Fatalf("long inventory did not shift quotes down: flat=%+v long=%+v", flat, longQuote)
	}
}

func TestCalculateStoikovQuoteHonorsMinimumHalfSpread(t *testing.T) {
	quote, ok := CalculateStoikovQuote(StoikovInputs{
		Forward: 100, VariancePerSecond: 0, RiskAversion: 1,
		FillDecay: 1e9, InventoryHorizon: time.Second, MinHalfSpread: 2,
	})
	if !ok {
		t.Fatal("quote invalid")
	}
	if quote.HalfSpread != 2 || quote.Bid != 98 || quote.Ask != 102 {
		t.Fatalf("minimum spread ignored: %+v", quote)
	}
}

func TestCalculateStoikovQuoteRejectsInvalidInputs(t *testing.T) {
	for _, input := range []StoikovInputs{
		{},
		{Forward: 100, RiskAversion: 0.1, FillDecay: 1, InventoryHorizon: time.Second, VariancePerSecond: -1},
		{Forward: 100, RiskAversion: math.NaN(), FillDecay: 1, InventoryHorizon: time.Second},
	} {
		if _, ok := CalculateStoikovQuote(input); ok {
			t.Fatalf("invalid input accepted: %+v", input)
		}
	}
}

func TestQuoteTickRoundingPreservesOrdering(t *testing.T) {
	bid, ok := quoteToBidTicks(100.019, 1_000, 10)
	if !ok || bid != 100_010 {
		t.Fatalf("bid rounding = %d, %v", bid, ok)
	}
	ask, ok := quoteToAskTicks(100.011, 1_000, 10)
	if !ok || ask != 100_020 {
		t.Fatalf("ask rounding = %d, %v", ask, ok)
	}
}

type stoikovStubGateway struct {
	requests   []etypes.Request
	responses  chan etypes.Response
	marketData chan *etypes.MarketDataMsg
}

func newStoikovStubGateway() *stoikovStubGateway {
	return &stoikovStubGateway{
		responses:  make(chan etypes.Response, 8),
		marketData: make(chan *etypes.MarketDataMsg, 8),
	}
}

func (g *stoikovStubGateway) ID() uint64                                 { return 1 }
func (g *stoikovStubGateway) Send(r etypes.Request)                      { g.requests = append(g.requests, r) }
func (g *stoikovStubGateway) Responses() <-chan etypes.Response          { return g.responses }
func (g *stoikovStubGateway) MarketDataCh() <-chan *etypes.MarketDataMsg { return g.marketData }
func (g *stoikovStubGateway) IsRunning() bool                            { return true }

func TestStoikovMarketMakerRequotesAfterInventoryFill(t *testing.T) {
	gw := newStoikovStubGateway()
	mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 100_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: 10, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 1.0 / (100.0 * 100.0), InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.01 * 100, RelativeFillDecay: 2 * 100, MinHalfSpreadTicks: 1,
	})
	now := time.Unix(10, 0)
	mm.onTick(now) // subscribes first
	// The maker subscribes to snapshots for its forward and to trades for its
	// volatility estimate.
	if len(gw.requests) != 2 || gw.requests[0].Type != etypes.ReqSubscribe || gw.requests[1].Type != etypes.ReqSubscribe {
		t.Fatalf("initial tick did not subscribe to snapshots and trades: %+v", gw.requests)
	}
	mm.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: "ABC/USD", Timestamp: now.UnixNano(),
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: 99_990, VisibleQty: 1_000}},
				Asks: []exchange.PriceLevel{{Price: 100_010, VisibleQty: 1_000}},
			},
		},
	})
	mm.onTick(now)
	if len(gw.requests) != 4 {
		t.Fatalf("quote tick requests = %d, want two subscribes + bid + ask", len(gw.requests))
	}
	bidReq, askReq := gw.requests[2].OrderReq, gw.requests[3].OrderReq
	if bidReq.Side != exchange.Buy || askReq.Side != exchange.Sell || bidReq.Price >= askReq.Price {
		t.Fatalf("invalid quote pair: bid=%+v ask=%+v", bidReq, askReq)
	}
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: bidReq.RequestID, OrderID: 10}})
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: askReq.RequestID, OrderID: 11}})
	if mm.bidID != 10 || mm.askID != 11 {
		t.Fatalf("accepts not linked: bid=%d ask=%d", mm.bidID, mm.askID)
	}

	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		Symbol: "ABC/USD", OrderID: 10, Side: exchange.Buy, Qty: 100, IsFull: true,
	}})
	if mm.Inventory() != 100 {
		t.Fatalf("inventory = %d, want 100", mm.Inventory())
	}
	mm.onTick(now)
	if len(gw.requests) != 7 || gw.requests[4].Type != etypes.ReqCancelOrder {
		t.Fatalf("fill must cancel the stale opposite quote and replace pair: %+v", gw.requests)
	}
}

// Inventory enters the control as a fraction of the risk budget, clamped, so a
// position beyond the budget cannot skew the quote without bound. Before the
// clamp the skew was per unit of inventory, and a maker holding 178 units
// multiplied a small per-unit shift into one large enough to move the price
// it was quoting around.
func TestInventoryFractionIsClampedToTheRiskBudget(t *testing.T) {
	mm := &StoikovMarketMaker{cfg: StoikovMMConfig{
		BasePrecision: 1_000, InventoryLimit: 10_000,
	}}
	for _, testCase := range []struct {
		inventory int64
		want      float64
	}{
		{inventory: 0, want: 0},
		{inventory: 5_000, want: 0.5},
		{inventory: 10_000, want: 1},
		{inventory: 40_000, want: 1},
		{inventory: -40_000, want: -1},
	} {
		mm.inventory = testCase.inventory
		if got := mm.inventoryFraction(); got != testCase.want {
			t.Fatalf("inventory %d gave fraction %v, want %v", testCase.inventory, got, testCase.want)
		}
	}

	// With no budget configured the fraction falls back to whole base units
	// rather than dividing by zero.
	unbudgeted := &StoikovMarketMaker{cfg: StoikovMMConfig{BasePrecision: 1_000}}
	unbudgeted.inventory = 500
	if got := unbudgeted.inventoryFraction(); got != 0.5 {
		t.Fatalf("unbudgeted fraction = %v, want 0.5", got)
	}
}
