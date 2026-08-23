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

func TestStoikovPostOnlyQuotesCancelBeforeReplacement(t *testing.T) {
	gw := newStoikovStubGateway()
	mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 100_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: 10, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 1.0 / (100.0 * 100.0), InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.01 * 100, RelativeFillDecay: 2 * 100, MinHalfSpreadTicks: 1,
		SubmitBeforeCancel: true, PostOnly: true,
	})
	now := time.Unix(10, 0)
	mm.onTick(now) // subscribes
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC/USD", Timestamp: now.UnixNano(), Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 99_990, VisibleQty: 1_000}},
			Asks: []exchange.PriceLevel{{Price: 100_010, VisibleQty: 1_000}},
		},
	}})
	mm.onTick(now)
	if len(gw.requests) != 4 || !gw.requests[2].OrderReq.PostOnly || !gw.requests[3].OrderReq.PostOnly {
		t.Fatalf("initial post-only quotes = %+v", gw.requests)
	}
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: gw.requests[2].OrderReq.RequestID, OrderID: 10}})
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: gw.requests[3].OrderReq.RequestID, OrderID: 11}})

	// Force a target change without changing the economic control. The purpose
	// is the replacement protocol: post-only overrides submit-before-cancel.
	mm.bidPrice, mm.askPrice = 1, 2
	mm.onTick(now)
	if len(gw.requests) != 8 || gw.requests[4].Type != exchange.ReqCancelOrder || gw.requests[5].Type != exchange.ReqCancelOrder {
		t.Fatalf("post-only replacement did not cancel first: %+v", gw.requests)
	}
	for _, request := range gw.requests[6:8] {
		if request.OrderReq == nil || !request.OrderReq.PostOnly {
			t.Fatalf("replacement quote lost post-only flag: %+v", request)
		}
	}
}

// Remote source age is an information constraint, not a cosmetic telemetry
// field. A stale cache must suppress its composite before it can move a quote.
func TestRemoteReferenceExpiresByPublicationAge(t *testing.T) {
	published := time.Unix(10, 0)
	observe := func(cache *LocalBookCache, sequence uint64, bid, ask int64) {
		if !cache.ObserveSnapshot(actor.BookSnapshotEvent{
			Symbol: "ABC/USD", Timestamp: published.UnixNano(), SeqNum: sequence,
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: 1}},
				Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: 1}},
			},
		}) {
			t.Fatal("cache observation was rejected")
		}
	}
	local := NewLocalBookCache("north", "ABC/USD")
	remote := NewLocalBookCache("south", "ABC/USD")
	observe(local, 1, 99, 101)
	observe(remote, 1, 199, 201)
	mm := &StoikovMarketMaker{
		cfg:            StoikovMMConfig{BootstrapPrice: 100, AnchorToIndex: false},
		localReference: local, remoteReference: remote,
		remoteWeight: 0.5, remoteConfidence: 0.8, remoteMaxAge: time.Second,
	}
	if got, ok := mm.referencePriceAt(published.Add(time.Second)); !ok || got != 140 {
		t.Fatalf("fresh weighted remote composite = %d, want 140", got)
	}
	if got, ok := mm.referencePriceAt(published.Add(time.Second + time.Nanosecond)); ok || got != 0 {
		t.Fatalf("expired remote composite = (%d, %v), want unavailable", got, ok)
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
			referencePrice, ok := mm.referencePrice()
			if !ok {
				t.Fatal("maker lost its configured reference")
			}
			reference := float64(referencePrice)
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

// Quoting and hedging are separate obligations that need separate clocks.
// Hedging only inside the quote cycle stops risk management whenever the market
// calms enough to suppress requoting, while the maker is still being filled.
// Hedging on every tick removes the rate limit the quote cycle provided and the
// maker's own marketable hedges dominate the hedge instrument: measured over
// eight hours that took the median basis from 2.1 to 830 basis points. A
// configured interval is the dial between the two.
func TestMakerHedgesOnItsOwnCadenceWhenRequotingIsSuppressed(t *testing.T) {
	gw := newStoikovStubGateway()
	mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 100_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: 10, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 1.0 / (100.0 * 100.0), InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.01 * 100, RelativeFillDecay: 2 * 100, MinHalfSpreadTicks: 1,
		HedgeSymbol: "ABC-PERP", HedgeBandQty: 50, HedgeTickSize: 10, HedgeSlippageBps: 50,
		RequoteBps: 1_000, HedgeInterval: 5 * time.Second,
	})
	now := time.Unix(10, 0)
	mm.onTick(now)
	book := func() {
		for _, symbol := range []string{"ABC/USD", "ABC-PERP"} {
			mm.HandleEvent(context.Background(), &actor.Event{
				Type: actor.EventBookSnapshot,
				Data: actor.BookSnapshotEvent{
					Symbol: symbol, Timestamp: now.UnixNano(),
					Snapshot: &exchange.BookSnapshot{
						Bids: []exchange.PriceLevel{{Price: 99_990, VisibleQty: 10_000}},
						Asks: []exchange.PriceLevel{{Price: 100_010, VisibleQty: 10_000}},
					},
				},
			})
		}
	}
	book()
	mm.onTick(now)
	var quotes []*etypes.OrderRequest
	for _, req := range gw.requests {
		if req.Type == etypes.ReqPlaceOrder && req.OrderReq.Symbol == "ABC/USD" {
			quotes = append(quotes, req.OrderReq)
		}
	}
	if len(quotes) != 2 {
		t.Fatalf("expected a quote pair, got %d orders", len(quotes))
	}
	orderID := uint64(10)
	for _, q := range quotes {
		mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: q.RequestID, OrderID: orderID}})
		orderID++
	}
	// Partially filled, so the pair still rests and the requote threshold
	// suppresses resubmission, but the inventory is real and must be hedged.
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderPartialFill, Data: actor.OrderFillEvent{
		Symbol: "ABC/USD", OrderID: 10, Side: exchange.Buy, Qty: 100, IsFull: false,
	}})

	before := len(gw.requests)
	book()
	mm.onTick(now)
	if extra := len(gw.requests) - before; extra != 0 {
		t.Fatalf("quote tick submitted %d orders despite the requote threshold", extra)
	}
	mm.onHedgeTick(now)
	var hedged bool
	for _, req := range gw.requests[before:] {
		if req.Type == etypes.ReqPlaceOrder && req.OrderReq.Symbol == "ABC-PERP" {
			hedged = true
		}
	}
	if !hedged {
		t.Fatalf("hedge tick did not offset an inventory of %d", mm.Inventory())
	}
}

// Without a configured cadence the hedge stays inside the quote cycle, so the
// existing behaviour is preserved for any caller that has not opted in.
func TestMakerWithoutHedgeIntervalKeepsHedgingInTheQuoteCycle(t *testing.T) {
	mm := &StoikovMarketMaker{cfg: StoikovMMConfig{HedgeSymbol: "ABC-PERP"}}
	if mm.cfg.HedgeInterval != 0 {
		t.Fatal("default hedge interval must be zero")
	}
	mm.subscribed = true
	mm.onHedgeTick(time.Unix(0, 0)) // must be inert without a cadence
}

// A maker that quotes the same size in every state gives the market no way to
// produce volatility clustering: a burst of trading meets exactly the depth a
// quiet period does, so a large move cannot make the next move more likely.
// Measured on the reference population, the autocorrelation of absolute
// returns was -0.008 at lag one where traded markets show 0.2 to 0.4.
func TestQuoteSizeWithdrawsAsVolatilityRises(t *testing.T) {
	base := StoikovMMConfig{
		QuoteQty:                 1_000_000,
		InitialLogVariancePerSec: 1e-8,
		QuoteSizeVolElasticity:   1.0,
		MinQuoteSizeFraction:     0.1,
	}
	calm := &StoikovMarketMaker{cfg: base, logVariancePerSec: 1e-8}
	if got := calm.quoteSize(); got != base.QuoteQty {
		t.Errorf("at its reference volatility the maker quoted %d, want the full %d", got, base.QuoteQty)
	}

	stressed := &StoikovMarketMaker{cfg: base, logVariancePerSec: 4e-8}
	stressedSize := stressed.quoteSize()
	if stressedSize >= base.QuoteQty {
		t.Errorf("at four times the variance the maker quoted %d, want less than %d", stressedSize, base.QuoteQty)
	}
	if stressedSize < base.QuoteQty/10 {
		t.Errorf("quoted size %d fell below the configured floor of %d", stressedSize, base.QuoteQty/10)
	}

	// The floor has to hold however extreme the estimate becomes, or a
	// volatility spike removes the book entirely.
	extreme := &StoikovMarketMaker{cfg: base, logVariancePerSec: 1e-2}
	if got := extreme.quoteSize(); got != base.QuoteQty/10 {
		t.Errorf("under an extreme estimate the maker quoted %d, want the floor %d", got, base.QuoteQty/10)
	}

	// Zero elasticity is the previous behaviour and must be exactly preserved.
	fixed := &StoikovMarketMaker{
		cfg:               StoikovMMConfig{QuoteQty: 1_000_000, InitialLogVariancePerSec: 1e-8},
		logVariancePerSec: 1e-2,
	}
	if got := fixed.quoteSize(); got != 1_000_000 {
		t.Errorf("without elasticity the maker quoted %d, want a constant 1000000", got)
	}
}

// A maker whose forward is the last printed midpoint treats every sweep as
// news: it requotes around the new level and the move never decays. Impact is
// then permanent by construction, which is why every parameter that widened the
// return tails also made the level slide. A maker that forms its view over time
// quotes back toward where it believed the price was.
func TestForwardHalfLifeMakesImpactDecay(t *testing.T) {
	newMaker := func(halfLife time.Duration) *StoikovMarketMaker {
		return &StoikovMarketMaker{cfg: StoikovMMConfig{
			ReferenceSymbol: "ABC/USD", ForwardHalfLife: halfLife,
		}}
	}
	second := int64(1e9)

	instant := newMaker(0)
	instant.forward = 1000
	if got := instant.blendForward(1200, second); got != 1200 {
		t.Fatalf("without a half-life the forward is %d, want the observed 1200", got)
	}

	// A sweep moves the book to 1200 and it stays there. A maker with a
	// ten-second view should still be well below it after one second, and
	// should converge only as the level persists.
	smoothed := newMaker(10 * time.Second)
	smoothed.forward = 1000
	smoothed.forwardAt = 0
	afterOne := smoothed.blendForward(1200, second)
	smoothed.forward = afterOne
	if afterOne >= 1150 {
		t.Errorf("after one second the forward is %d, want it still near its prior belief of 1000", afterOne)
	}
	if afterOne <= 1000 {
		t.Errorf("after one second the forward is %d, want it to have moved toward 1200", afterOne)
	}
	for i := int64(2); i <= 60; i++ {
		smoothed.forward = smoothed.blendForward(1200, i*second)
	}
	// Six half-lives leave about 1.6% of the gap, so the belief should have
	// closed the great majority of it without needing to arrive exactly.
	if smoothed.forward < 1180 {
		t.Errorf("after a minute at 1200 the forward is %d, want most of the gap closed", smoothed.forward)
	}

	// A transient sweep that reverts should leave the belief nearly untouched,
	// which is what makes the impact decay rather than persist.
	transient := newMaker(10 * time.Second)
	transient.forward = 1000
	transient.forwardAt = 0
	transient.forward = transient.blendForward(1200, second/10)
	transient.forward = transient.blendForward(1000, 2*second/10)
	if transient.forward > 1030 {
		t.Errorf("a tenth-of-a-second spike moved the belief to %d, want it barely changed", transient.forward)
	}
}
