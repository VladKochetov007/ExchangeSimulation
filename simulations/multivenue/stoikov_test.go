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

// A hedge price must land on the hedge instrument's tick grid. Pricing through
// the touch is exactly what knocks it off: a fifty basis point bump on a
// 50,000 price is 250, which is not a multiple of a 1,000 tick, and the venue
// rejects the order outright.
//
// This was silent in the scenario for a long time. The maker made 1,218
// attempts and zero fills, and because a rejection is not a fill the only
// visible symptom was inventory that never came down.
func TestHedgePriceIsAlignedToTheHedgeTick(t *testing.T) {
	const tick = int64(1_000)
	gw := newStoikovStubGateway()
	mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 50_000_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: tick, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 1e-8, InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.1, RelativeFillDecay: 25_000, MinHalfSpreadTicks: 1,
		HedgeSymbol: "ABC-PERP", HedgeBandQty: 10, HedgeSlippageBps: 50, HedgeTickSize: tick,
	})

	// Short, so the hedge is a buy that must round up to stay marketable.
	mm.inventory = -500
	mm.hedgeBid, mm.hedgeBidQty = 49_999_000, 1_000
	mm.hedgeAsk, mm.hedgeAskQty = 50_000_000, 1_000
	mm.hedgeDelta()

	if len(gw.requests) == 0 {
		t.Fatal("no hedge submitted")
	}
	order := gw.requests[len(gw.requests)-1].OrderReq
	if order.Side != exchange.Buy {
		t.Fatalf("hedge side = %v, want a buy against a short", order.Side)
	}
	if order.Price%tick != 0 {
		t.Fatalf("hedge price %d is not a multiple of the %d tick", order.Price, tick)
	}
	if order.Price < mm.hedgeAsk {
		t.Fatalf("hedge price %d is below the ask %d, so it would not cross", order.Price, mm.hedgeAsk)
	}

	// Long: the hedge is a sell and must round down, staying at or below the bid.
	gw.requests = nil
	mm.hedgePending, mm.hedgePosition, mm.inventory = false, 0, 500
	mm.hedgeDelta()
	if len(gw.requests) == 0 {
		t.Fatal("no hedge submitted for a long position")
	}
	sell := gw.requests[len(gw.requests)-1].OrderReq
	if sell.Side != exchange.Sell {
		t.Fatalf("hedge side = %v, want a sell against a long", sell.Side)
	}
	if sell.Price%tick != 0 {
		t.Fatalf("hedge price %d is not a multiple of the %d tick", sell.Price, tick)
	}
	if sell.Price > mm.hedgeBid {
		t.Fatalf("hedge price %d is above the bid %d, so it would not cross", sell.Price, mm.hedgeBid)
	}
}

// A maker that skews its quote away from a reference it only partly anchors to
// displaces the price by more than the skew, because its own midpoint feeds
// back into the reference. Iterating the reference to its fixed point, the
// displacement is the skew divided by the index weight.
//
// Measured in the simulator at four weights, with the perpetual maker's
// inventory held at the same level: 25.0 basis points at weight 1.0, 35.6 at
// 0.7, 49.7 at 0.5 and 83.4 at 0.3, against 25.0, 35.7, 50.0 and 83.3
// predicted.
//
// The skew itself is proportional to inventory until it saturates at the risk
// limit, so the full relation is (skew * min(|q|/limit, 1)) / weight. Raising
// the limit fivefold, which drops the maker from saturation to 30% of its
// budget, moved the premium from 83.4 to 24.1 basis points against 25.0
// predicted.
func TestPartialAnchoringAmplifiesInventorySkew(t *testing.T) {
	const index = int64(50_000) * mvQuotePrecision
	const skewBps = 25.0

	for _, testCase := range []struct {
		weight          float64
		inventoryFactor float64
	}{
		{weight: 1.0, inventoryFactor: 1},
		{weight: 0.7, inventoryFactor: 1},
		{weight: 0.5, inventoryFactor: 1},
		{weight: 0.3, inventoryFactor: 1},
		// Below the risk limit the skew scales with the position.
		{weight: 0.3, inventoryFactor: 0.3},
	} {
		weight := testCase.weight
		mm := &StoikovMarketMaker{cfg: StoikovMMConfig{
			QuotePrecision: mvQuotePrecision, BootstrapPrice: index,
			AnchorToIndex: true, IndexWeight: weight,
		}}
		mm.indexPrice = index

		// Iterate: the maker quotes a fixed skew above its reference, and its
		// midpoint becomes the book midpoint the reference blends in.
		mm.forward = index
		for range 500 {
			reference := float64(mm.referencePrice())
			mm.forward = int64(reference * (1 + testCase.inventoryFactor*skewBps/10_000))
		}

		displacement := 1e4 * float64(mm.forward-index) / float64(index)
		predicted := testCase.inventoryFactor * skewBps / weight
		if diff := displacement - predicted; diff > 0.5 || diff < -0.5 {
			t.Fatalf("weight %.1f at %.0f%% of the risk budget displaced the price by %.2f basis points, want about %.2f",
				weight, 100*testCase.inventoryFactor, displacement, predicted)
		}
	}
}
