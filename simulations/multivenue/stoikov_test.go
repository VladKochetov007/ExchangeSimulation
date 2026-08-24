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

func TestInventoryRebalancePlanUsesOnlyLocalContraTouchAndCaps(t *testing.T) {
	gw := newStoikovStubGateway()
	var decisions []MakerInventoryRebalanceDecision
	var fills []MakerInventoryRebalanceFill
	now := time.Unix(10, 0)
	policy := &InventoryRebalanceConfig{
		Enabled: true, Interval: 10 * time.Second, Cooldown: 30 * time.Second,
		RiskBandQty: 1_000, TargetBandQty: 1_000, MaxRequestQty: 200,
		ParticipationBps: 1_000, SlippageBps: 50,
	}
	mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "CDF/USD", TickSize: 10, InventoryRebalance: policy,
		InventoryRebalanceDecisionObserver: func(decision MakerInventoryRebalanceDecision) { decisions = append(decisions, decision) },
		InventoryRebalanceFillObserver:     func(fill MakerInventoryRebalanceFill) { fills = append(fills, fill) },
	})
	mm.subscribed = true
	mm.inventory = 1_500
	mm.rebalanceBook = localRebalanceBook{
		SourceTime: now.UnixNano() - int64(time.Millisecond), ReceivedTime: now.UnixNano() - int64(time.Millisecond), Sequence: 7,
		BidPrice: 1_000, BidQty: 1_000, AskPrice: 1_100, AskQty: 1_000,
	}
	mm.onRebalanceTick(now)
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	got := decisions[0]
	if got.ActionOrDeferReason != "SUBMIT_IOC" || got.Side != exchange.Sell || got.DesiredReduction != 500 || got.ParticipationCap != 100 || got.RequestedQty != 100 || got.LimitPrice != 990 {
		t.Fatalf("unexpected P2 plan: %+v", got)
	}
	if len(gw.requests) != 1 || gw.requests[0].OrderReq == nil {
		t.Fatalf("P2 did not emit one order: %+v", gw.requests)
	}
	order := gw.requests[0].OrderReq
	if order.RequestID != got.RequestID || order.Symbol != "CDF/USD" || order.Side != exchange.Sell || order.TimeInForce != exchange.IOC || order.PostOnly || order.Price != got.LimitPrice || order.Qty != got.RequestedQty {
		t.Fatalf("submitted order disagrees with P2 evidence: order=%+v decision=%+v", order, got)
	}
	if got.CooldownUntil != now.Add(policy.Cooldown).UnixNano() || !mm.rebalancePending {
		t.Fatalf("P2 cooldown/pending state not set at submission: decision=%+v pending=%t", got, mm.rebalancePending)
	}

	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: got.RequestID, OrderID: 44}})
	if mm.rebalancePending || mm.rebalanceRequest != 0 {
		t.Fatalf("accepted P2 IOC left request pending: pending=%t request=%d", mm.rebalancePending, mm.rebalanceRequest)
	}
	if len(gw.requests) != 1 {
		t.Fatalf("accepted P2 IOC was incorrectly cancelled: %+v", gw.requests)
	}
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 44, Symbol: "CDF/USD", Side: exchange.Sell, Qty: 100, Price: 990, TradeID: 8, IsFull: true, FeeAmount: 4, FeeAsset: "USD", Timestamp: now.UnixNano(),
	}})
	if len(fills) != 1 || fills[0].PreInventory != 1_500 || fills[0].PostInventory != 1_400 || fills[0].OrderID != 44 || fills[0].FeeAmount != 4 {
		t.Fatalf("P2 fill transition evidence = %+v", fills)
	}
}

func TestInventoryRebalanceDeferralsAreExplicitAndDoNotSubmit(t *testing.T) {
	now := time.Unix(10, 0)
	base := InventoryRebalanceConfig{
		Enabled: true, Interval: 10 * time.Second, Cooldown: 30 * time.Second,
		RiskBandQty: 1_000, TargetBandQty: 500, MaxRequestQty: 200,
		ParticipationBps: 1_000, SlippageBps: 50,
	}
	newMaker := func() (*StoikovMarketMaker, *stoikovStubGateway, *[]MakerInventoryRebalanceDecision) {
		gw := newStoikovStubGateway()
		decisions := make([]MakerInventoryRebalanceDecision, 0, 1)
		mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
			Symbol: "CDF/USD", TickSize: 10, InventoryRebalance: &base,
			InventoryRebalanceDecisionObserver: func(decision MakerInventoryRebalanceDecision) { decisions = append(decisions, decision) },
		})
		mm.subscribed, mm.inventory = true, 1_500
		return mm, gw, &decisions
	}
	for name, mutate := range map[string]func(*StoikovMarketMaker){
		"missing receipt": func(mm *StoikovMarketMaker) {},
		"stale local book": func(mm *StoikovMarketMaker) {
			mm.rebalanceBook = localRebalanceBook{SourceTime: now.Add(-20 * time.Second).UnixNano(), ReceivedTime: now.Add(-11 * time.Second).UnixNano(), BidPrice: 1_000, BidQty: 1_000, AskPrice: 1_100, AskQty: 1_000}
		},
		"empty contra touch": func(mm *StoikovMarketMaker) {
			mm.rebalanceBook = localRebalanceBook{SourceTime: now.UnixNano(), ReceivedTime: now.UnixNano(), BidPrice: 1_000, AskPrice: 1_100, AskQty: 1_000}
		},
	} {
		t.Run(name, func(t *testing.T) {
			mm, gw, decisions := newMaker()
			mutate(mm)
			mm.onRebalanceTick(now)
			if len(*decisions) != 1 || (*decisions)[0].ActionOrDeferReason == "SUBMIT_IOC" || len(gw.requests) != 0 {
				t.Fatalf("defer was not explicit/non-submitting: decisions=%+v requests=%+v", *decisions, gw.requests)
			}
		})
	}

	mm, gw, decisions := newMaker()
	mm.cfg.InventoryRebalance.Enabled = false
	mm.onRebalanceTick(now)
	if len(*decisions) != 1 || (*decisions)[0].ActionOrDeferReason != "POLICY_DISABLED" || len(gw.requests) != 0 {
		t.Fatalf("disabled control was not explicit/no-action: decisions=%+v requests=%+v", *decisions, gw.requests)
	}
}

func TestInventoryRebalanceBuyRoundsOutwardAndTerminalIsCensored(t *testing.T) {
	gw := newStoikovStubGateway()
	var decisions []MakerInventoryRebalanceDecision
	now := time.Unix(10, 0)
	policy := &InventoryRebalanceConfig{
		Enabled: true, Interval: 10 * time.Second, Cooldown: time.Second,
		RiskBandQty: 1_000, TargetBandQty: 500, MaxRequestQty: 200,
		ParticipationBps: 1_000, SlippageBps: 50,
	}
	mm := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "CDF/USD", TickSize: 10, InventoryRebalance: policy,
		InventoryRebalanceDecisionTerminalNano: now.UnixNano(),
		InventoryRebalanceDecisionObserver:     func(decision MakerInventoryRebalanceDecision) { decisions = append(decisions, decision) },
	})
	mm.subscribed, mm.inventory = true, -1_500
	mm.rebalanceBook = localRebalanceBook{SourceTime: now.UnixNano(), ReceivedTime: now.UnixNano(), BidPrice: 900, BidQty: 1_000, AskPrice: 1_001, AskQty: 1_000}
	mm.onRebalanceTick(now)
	if len(decisions) != 1 {
		t.Fatalf("decisions = %d", len(decisions))
	}
	got := decisions[0]
	if got.Side != exchange.Buy || got.LimitPrice != 1_010 || got.OutcomeExpectation != "SIMULATION_HORIZON_CENSORED" || got.CensorReason != "terminal_horizon_before_venue_ingress" {
		t.Fatalf("buy/censor plan = %+v", got)
	}
	if len(gw.requests) != 1 || gw.requests[0].OrderReq.TimeInForce != exchange.IOC || gw.requests[0].OrderReq.Price != 1_010 {
		t.Fatalf("buy IOC = %+v", gw.requests)
	}
}

func TestInventoryRebalanceValidationRejectsAmbiguousEvidencePath(t *testing.T) {
	base := Config{LogDir: t.TempDir(), LogMode: "full", CrossAssetSpotGraph: true, RecordMakerInventoryRebalanceDecisions: true,
		CDFInventoryRebalance: &InventoryRebalanceConfig{Interval: time.Second, Cooldown: time.Second, RiskBandQty: 10, TargetBandQty: 5, MaxRequestQty: 1, ParticipationBps: 1, SlippageBps: 0}}
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("P2 accepted without independently recorded local feed receipts")
	}
	base.RecordMarketDataReceipts = true
	base.MarketDataReceiptRoles = []string{"cdf_spot_maker"}
	base.LatencyProfiles = map[string]LatencyProfile{"cdf_spot_maker": {Model: "constant", Delay: time.Millisecond}}
	sim, err := NewSim(time.Second, base)
	if err != nil {
		t.Fatalf("P2 rejected documented receipt path: %v", err)
	}
	sim.Close()
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
		SubmitBeforeCancel: true, PostOnly: true, PostOnlyCancelBeforeReplace: true,
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

// Post-only admission and refresh ordering are independently manipulable in
// P0. This test is arm B: legacy submit-before-cancel ordering remains, but
// every replacement is explicitly post-only when it reaches the venue.
func TestStoikovPostOnlyCanKeepLegacyReplacementOrder(t *testing.T) {
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
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: gw.requests[2].OrderReq.RequestID, OrderID: 10}})
	mm.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: gw.requests[3].OrderReq.RequestID, OrderID: 11}})

	mm.bidPrice, mm.askPrice = 1, 2 // force a replacement without changing the policy.
	mm.onTick(now)
	if len(gw.requests) != 8 || gw.requests[4].Type != etypes.ReqPlaceOrder || gw.requests[5].Type != etypes.ReqPlaceOrder || gw.requests[6].Type != etypes.ReqCancelOrder || gw.requests[7].Type != etypes.ReqCancelOrder {
		t.Fatalf("post-only legacy replacement order = %+v", gw.requests)
	}
	for _, request := range gw.requests[4:6] {
		if request.OrderReq == nil || !request.OrderReq.PostOnly {
			t.Fatalf("legacy-order replacement lost post-only admission: %+v", request)
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

func TestStoikovInventoryQuoteSizePlan(t *testing.T) {
	base := StoikovMMConfig{QuoteQty: 1_000, BasePrecision: 1_000, InventoryLimit: 1_000, InventorySizeSkewBps: 5_000}
	tests := []struct {
		name      string
		inventory int64
		baseQty   int64
		limit     int64
		wantBid   int64
		wantAsk   int64
		wantAdj   int64
		ok        bool
	}{
		{name: "flat", wantBid: 1_000, wantAsk: 1_000, ok: true},
		{name: "long half limit", inventory: 500, wantBid: 750, wantAsk: 1_250, wantAdj: 250, ok: true},
		{name: "short half limit", inventory: -500, wantBid: 1_250, wantAsk: 750, wantAdj: 250, ok: true},
		{name: "long clamped", inventory: 2_000, wantBid: 500, wantAsk: 1_500, wantAdj: 500, ok: true},
		{name: "short min int clamped", inventory: math.MinInt64, wantBid: 1_500, wantAsk: 500, wantAdj: 500, ok: true},
		{name: "integer adjustment rounds to zero", inventory: 1, baseQty: 3, limit: 3, wantBid: 3, wantAsk: 3, ok: true},
		{name: "expanded side overflow is refused", inventory: 1_000, baseQty: math.MaxInt64, wantBid: 0, wantAsk: 0, ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			if tc.baseQty != 0 {
				cfg.QuoteQty = tc.baseQty
			}
			if tc.limit != 0 {
				cfg.InventoryLimit = tc.limit
			}
			maker := &StoikovMarketMaker{cfg: cfg, inventory: tc.inventory}
			plan, ok := maker.quoteSizePlan()
			if ok != tc.ok {
				t.Fatalf("quoteSizePlan ok = %t, want %t (plan=%+v)", ok, tc.ok, plan)
			}
			if !ok {
				return
			}
			if plan.BidQty != tc.wantBid || plan.AskQty != tc.wantAsk || plan.Adjustment != tc.wantAdj {
				t.Fatalf("quoteSizePlan = %+v, want bid/ask/adjustment %d/%d/%d", plan, tc.wantBid, tc.wantAsk, tc.wantAdj)
			}
		})
	}
}

func TestStoikovInventorySizeRefreshesWithoutPriceChange(t *testing.T) {
	gw := newStoikovStubGateway()
	maker := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 100_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: 10, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 0, InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.01 * 100, RelativeFillDecay: 2 * 100, MinHalfSpreadTicks: 1,
		InventoryLimit: 100, InventorySizeSkewBps: 5_000,
		SubmitBeforeCancel: true, PostOnly: true, PostOnlyCancelBeforeReplace: true,
	})
	now := time.Unix(10, 0)
	maker.onTick(now) // subscriptions
	maker.HandleEvent(context.Background(), makerSnapshot("ABC/USD", 99_990, 100_010))
	maker.onTick(now)
	if len(gw.requests) != 4 {
		t.Fatalf("initial quote requests = %d, want 4", len(gw.requests))
	}
	initialBid, initialAsk := gw.requests[2].OrderReq, gw.requests[3].OrderReq
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: initialBid.RequestID, OrderID: 10}})
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: initialAsk.RequestID, OrderID: 11}})
	// A partial fill preserves both order IDs. With zero variance the price pair
	// stays unchanged, so this refresh is attributable to the size policy.
	maker.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderPartialFill, Data: actor.OrderFillEvent{
		Symbol: "ABC/USD", OrderID: 10, Side: exchange.Buy, Qty: 50, IsFull: false,
	}})
	maker.onTick(now.Add(time.Second))
	if len(gw.requests) != 8 || gw.requests[4].Type != exchange.ReqCancelOrder || gw.requests[5].Type != exchange.ReqCancelOrder {
		t.Fatalf("size-only refresh did not keep P0-C cancel-before-replace: %+v", gw.requests)
	}
	newBid, newAsk := gw.requests[6].OrderReq, gw.requests[7].OrderReq
	if newBid.Price != initialBid.Price || newAsk.Price != initialAsk.Price {
		t.Fatalf("size-only policy changed prices: initial=%d/%d new=%d/%d", initialBid.Price, initialAsk.Price, newBid.Price, newAsk.Price)
	}
	if newBid.Qty != 75 || newAsk.Qty != 125 {
		t.Fatalf("long inventory quantities = %d/%d, want 75/125", newBid.Qty, newAsk.Qty)
	}
}

func TestStoikovQuoteSizeDecisionPrecedesRequestsAndCarriesIDs(t *testing.T) {
	gw := newStoikovStubGateway()
	var decisions []MakerQuoteSizeDecision
	maker := NewStoikovMarketMaker(1, gw, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 100_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: 10, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 1.0 / (100.0 * 100.0), InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.01 * 100, RelativeFillDecay: 2 * 100, MinHalfSpreadTicks: 1,
		InventoryLimit: 100, PostOnly: true, PostOnlyCancelBeforeReplace: true,
		QuoteSizeDecisionMaker: "spot_maker_1", QuoteSizeDecisionClient: 7,
		QuoteSizeDecisionObserver: func(decision MakerQuoteSizeDecision) {
			if len(gw.requests) != 2 {
				t.Fatalf("decision observed after quote gateway send: %d requests", len(gw.requests))
			}
			decisions = append(decisions, decision)
		},
	})
	now := time.Unix(10, 0)
	maker.onTick(now)
	maker.HandleEvent(context.Background(), makerSnapshot("ABC/USD", 99_990, 100_010))
	maker.onTick(now)
	if len(decisions) != 1 || len(gw.requests) != 4 {
		t.Fatalf("decision/request counts = %d/%d, want 1/4", len(decisions), len(gw.requests))
	}
	got := decisions[0]
	if got.Maker != "spot_maker_1" || got.ClientID != 7 || got.Symbol != "ABC/USD" || got.DecisionTime != now.UnixNano() {
		t.Fatalf("decision identity = %+v", got)
	}
	if got.BidRequestID != gw.requests[2].OrderReq.RequestID || got.AskRequestID != gw.requests[3].OrderReq.RequestID {
		t.Fatalf("decision request IDs = %d/%d, orders = %d/%d", got.BidRequestID, got.AskRequestID, gw.requests[2].OrderReq.RequestID, gw.requests[3].OrderReq.RequestID)
	}
	if got.BidPrice != gw.requests[2].OrderReq.Price || got.AskPrice != gw.requests[3].OrderReq.Price ||
		got.BidQty != gw.requests[2].OrderReq.Qty || got.AskQty != gw.requests[3].OrderReq.Qty || !got.PostOnly || !got.CancelBeforeReplace {
		t.Fatalf("decision did not match emitted P0-C requests: decision=%+v orders=%+v", got, gw.requests[2:])
	}
	if got.OutcomeExpectation != "VENUE_OUTCOME_REQUIRED" || got.CensorReason != "" {
		t.Fatalf("ordinary decision outcome contract = %+v", got)
	}
}

func TestStoikovQuoteSizeDecisionMarksTerminalHorizonCensoring(t *testing.T) {
	gateway := newStoikovStubGateway()
	var decisions []MakerQuoteSizeDecision
	now := time.Unix(10, 0)
	maker := NewStoikovMarketMaker(1, gateway, StoikovMMConfig{
		Symbol: "ABC/USD", ReferenceSymbol: "ABC/USD", BootstrapPrice: 100_000,
		BasePrecision: 1_000, QuotePrecision: 1_000, TickSize: 10, QuoteQty: 100,
		QuoteInterval: time.Second, VolatilityHalfLife: time.Minute,
		InitialLogVariancePerSec: 1.0 / (100.0 * 100.0), InventoryHorizon: time.Minute,
		RelativeRiskAversion: 0.01 * 100, RelativeFillDecay: 2 * 100, MinHalfSpreadTicks: 1,
		InventoryLimit: 100, PostOnly: true, PostOnlyCancelBeforeReplace: true,
		QuoteSizeDecisionTerminalNano: now.UnixNano(),
		QuoteSizeDecisionObserver:     func(decision MakerQuoteSizeDecision) { decisions = append(decisions, decision) },
	})
	maker.onTick(now)
	maker.HandleEvent(context.Background(), makerSnapshot("ABC/USD", 99_990, 100_010))
	maker.onTick(now)
	if len(decisions) != 1 || decisions[0].OutcomeExpectation != "SIMULATION_HORIZON_CENSORED" || decisions[0].CensorReason != "terminal_horizon_before_venue_ingress" {
		t.Fatalf("terminal decision censoring = %+v", decisions)
	}
}
