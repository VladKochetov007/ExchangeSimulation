package multivenue

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func liabilityHedgerTestConfig() LiabilityHedgerConfig {
	return LiabilityHedgerConfig{
		Enabled: true, Symbol: "CDF/USD", DecisionInterval: 2 * time.Second,
		ObligationInterval: 10 * time.Second, ObligationStepQty: 100,
		MaxAbsObligationQty: 1_000, MaxRequestQty: 100, Seed: 17,
		VenueID: "north", Hedger: "liability_hedger_1", ClientID: 9,
		TakerFeeBps: 5,
	}
}

func TestLiabilityHedgerUsesOnlyRequiredExecutableSide(t *testing.T) {
	gw := newStoikovStubGateway()
	var decisions []LiabilityHedgerDecision
	cfg := liabilityHedgerTestConfig()
	cfg.DecisionObserver = func(decision LiabilityHedgerDecision) { decisions = append(decisions, decision) }
	hedger := NewLiabilityHedger(1, gw, cfg)
	now := time.Unix(10, 0)

	hedger.onTick(now)
	if len(gw.requests) != 1 || gw.requests[0].Type != exchange.ReqSubscribe || decisions[0].Subscribed || decisions[0].ActionOrDeferReason != "NOT_SUBSCRIBED" {
		t.Fatalf("initial subscription decision = requests=%+v decisions=%+v", gw.requests, decisions)
	}

	hedger.obligation, hedger.lastUpdate = 200, now.UnixNano()
	hedger.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "CDF/USD", Timestamp: now.UnixNano(), SeqNum: 7,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: 99, VisibleQty: 1_000}}},
	}})
	hedger.onTick(now.Add(cfg.DecisionInterval))
	if len(gw.requests) != 1 {
		t.Fatalf("missing ask created a request: %+v", gw.requests)
	}
	missing := decisions[len(decisions)-1]
	if missing.ActionOrDeferReason != "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE" || missing.SideEvidence != "BUY" || missing.HasAsk || missing.LimitPrice != 0 {
		t.Fatalf("missing executable ask decision = %+v", missing)
	}
	encoded, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"side":"BUY"`)) {
		t.Fatalf("BUY evidence omitted at explicit unavailable-price boundary: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"has_ask":false`)) {
		t.Fatalf("missing executable ask was not represented explicitly: %s", encoded)
	}

	hedger.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "CDF/USD", Timestamp: now.Add(cfg.DecisionInterval).UnixNano(), SeqNum: 8,
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 99, VisibleQty: 1_000}},
			Asks: []exchange.PriceLevel{{Price: 101, VisibleQty: 1_000}},
		},
	}})
	hedger.onTick(now.Add(2 * cfg.DecisionInterval))
	if len(gw.requests) != 2 {
		t.Fatalf("usable local ask did not submit: %+v", gw.requests)
	}
	request := gw.requests[1].OrderReq
	if request == nil || request.Side != exchange.Buy || request.Price != 101 || request.Qty != 100 || request.TimeInForce != exchange.IOC {
		t.Fatalf("local executable buy request = %+v", request)
	}
}

// A numeric zero and a missing ask are distinct states. CDF/USD itself rejects
// zero at exchange admission, but this actor must never reinterpret a present
// numeric book level as absence before that explicit domain boundary.
func TestLiabilityHedgerDoesNotUseZeroPriceAsMissingSide(t *testing.T) {
	gw := newStoikovStubGateway()
	cfg := liabilityHedgerTestConfig()
	hedger := NewLiabilityHedger(1, gw, cfg)
	now := time.Unix(10, 0)
	hedger.subscribed = true
	hedger.obligation, hedger.lastUpdate = 200, now.UnixNano()
	hedger.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "CDF/USD", Timestamp: 0, SeqNum: 1,
		Snapshot: &exchange.BookSnapshot{Asks: []exchange.PriceLevel{{Price: 0, VisibleQty: 1_000}}},
	}})
	hedger.onTick(now)
	if len(gw.requests) != 1 || gw.requests[0].OrderReq == nil || gw.requests[0].OrderReq.Price != 0 {
		t.Fatalf("present zero-valued ask became an unavailable side: %+v", gw.requests)
	}
}

func TestLiabilityHedgerDefersUnsafeTerminalRoundTrip(t *testing.T) {
	gw := newStoikovStubGateway()
	cfg := liabilityHedgerTestConfig()
	now := time.Unix(10, 0)
	cfg.TerminalNano = now.Add(3 * time.Second).UnixNano()
	var decisions []LiabilityHedgerDecision
	cfg.DecisionObserver = func(decision LiabilityHedgerDecision) { decisions = append(decisions, decision) }
	hedger := NewLiabilityHedger(1, gw, cfg)
	hedger.subscribed = true
	hedger.obligation, hedger.lastUpdate = 200, now.UnixNano()
	hedger.book = liabilityHedgerBook{HasSnapshot: true, SourceTime: now.UnixNano(), Sequence: 1, HasBid: true, BidPrice: 99, BidQty: 1_000, HasAsk: true, AskPrice: 101, AskQty: 1_000}

	hedger.onTick(now)
	if len(gw.requests) != 0 || len(decisions) != 1 {
		t.Fatalf("unsafe terminal tail submitted a request: requests=%+v decisions=%+v", gw.requests, decisions)
	}
	decision := decisions[0]
	if decision.ActionOrDeferReason != "SIMULATION_HORIZON_CENSORED" || decision.OutcomeExpectation != "SIMULATION_HORIZON_CENSORED" || decision.CensorReason != "terminal_horizon_before_round_trip" || decision.RequestID != 0 || decision.RequestedQty != 0 {
		t.Fatalf("terminal defer evidence = %+v", decision)
	}
}

func TestLiabilityHedgerFillReducesExplicitGap(t *testing.T) {
	gw := newStoikovStubGateway()
	var fills []LiabilityHedgerFill
	cfg := liabilityHedgerTestConfig()
	cfg.FillObserver = func(fill LiabilityHedgerFill) { fills = append(fills, fill) }
	hedger := NewLiabilityHedger(1, gw, cfg)
	now := time.Unix(20, 0)
	hedger.subscribed = true
	hedger.obligation, hedger.lastUpdate = 200, now.UnixNano()
	hedger.book = liabilityHedgerBook{HasSnapshot: true, SourceTime: now.UnixNano(), Sequence: 1, HasBid: true, BidPrice: 99, BidQty: 1_000, HasAsk: true, AskPrice: 101, AskQty: 1_000}

	hedger.onTick(now)
	if len(gw.requests) != 1 || gw.requests[0].OrderReq == nil {
		t.Fatalf("initial hedge request = %+v", gw.requests)
	}
	request := gw.requests[0].OrderReq
	hedger.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: request.RequestID, OrderID: 44}})
	hedger.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		Symbol: "CDF/USD", OrderID: 44, TradeID: 73, Side: exchange.Buy, Qty: 100, Price: 101,
		FeeAmount: 5, FeeAsset: "USD", IsFull: true, Timestamp: now.UnixNano(),
	}})
	if hedger.Position() != 100 || hedger.pending {
		t.Fatalf("filled local position/pending = %d/%t", hedger.Position(), hedger.pending)
	}
	if len(fills) != 1 || fills[0].Side != "BUY" || fills[0].PrePosition != 0 || fills[0].PostPosition != 100 {
		t.Fatalf("fill evidence = %+v", fills)
	}
	hedger.onTick(now.Add(cfg.DecisionInterval))
	if len(gw.requests) != 2 || gw.requests[1].OrderReq == nil || gw.requests[1].OrderReq.Qty != 100 {
		t.Fatalf("remaining explicit hedge gap request = %+v", gw.requests)
	}
}

func TestLiabilityHedgerDisabledControlStillEvolvesState(t *testing.T) {
	gw := newStoikovStubGateway()
	var decisions []LiabilityHedgerDecision
	cfg := liabilityHedgerTestConfig()
	cfg.Enabled = false
	cfg.DecisionObserver = func(decision LiabilityHedgerDecision) { decisions = append(decisions, decision) }
	hedger := NewLiabilityHedger(1, gw, cfg)
	now := time.Unix(30, 0)

	hedger.onTick(now)
	hedger.onTick(now.Add(cfg.DecisionInterval))
	hedger.onTick(now.Add(cfg.ObligationInterval + cfg.DecisionInterval))
	if len(gw.requests) != 1 {
		t.Fatalf("disabled control sent market request: %+v", gw.requests)
	}
	if len(decisions) != 3 {
		t.Fatalf("disabled decisions = %d", len(decisions))
	}
	updates := 0
	for _, decision := range decisions[1:] {
		if decision.ActionOrDeferReason != "POLICY_DISABLED" {
			t.Fatalf("disabled decision = %+v", decision)
		}
		if decision.ObligationStep != 0 {
			updates++
		}
	}
	if updates != 2 || hedger.Obligation() == 0 {
		t.Fatalf("disabled state did not evolve: updates=%d obligation=%d decisions=%+v", updates, hedger.Obligation(), decisions)
	}
}

func TestLiabilityHedgerLegacyModeDefaultsToDeliveryLiability(t *testing.T) {
	now := time.Unix(40, 0)
	newHedger := func(mode LiabilityHedgerPolicyMode) (*LiabilityHedger, *stoikovStubGateway) {
		gw := newStoikovStubGateway()
		cfg := liabilityHedgerTestConfig()
		cfg.PolicyMode = mode
		hedger := NewLiabilityHedger(1, gw, cfg)
		hedger.subscribed = true
		hedger.obligation, hedger.lastUpdate = 200, now.UnixNano()
		hedger.book = liabilityHedgerBook{HasSnapshot: true, SourceTime: now.UnixNano(), Sequence: 1, HasBid: true, BidPrice: 99, BidQty: 1_000, HasAsk: true, AskPrice: 101, AskQty: 1_000}
		return hedger, gw
	}
	legacy, legacyGW := newHedger("")
	explicit, explicitGW := newHedger(LiabilityHedgerPolicyDeliveryLiability)
	legacy.onTick(now)
	explicit.onTick(now)
	if len(legacyGW.requests) != 1 || len(explicitGW.requests) != 1 {
		t.Fatalf("legacy/explicit requests = %d/%d", len(legacyGW.requests), len(explicitGW.requests))
	}
	left, right := legacyGW.requests[0].OrderReq, explicitGW.requests[0].OrderReq
	if left == nil || right == nil || left.Side != exchange.Buy || left.Side != right.Side || left.Price != right.Price || left.Qty != right.Qty || legacy.mode != LiabilityHedgerPolicyDeliveryLiability || explicit.mode != LiabilityHedgerPolicyDeliveryLiability {
		t.Fatalf("legacy delivery mode diverged: legacy=%+v explicit=%+v modes=%q/%q", left, right, legacy.mode, explicit.mode)
	}
}

func TestLiabilityHedgerRandomSideControlUsesIndependentDeclaredStream(t *testing.T) {
	gw := newStoikovStubGateway()
	var decisions []LiabilityHedgerDecision
	cfg := liabilityHedgerTestConfig()
	cfg.PolicyMode = LiabilityHedgerPolicyRandomSideControl
	cfg.PolicySeed = 31
	cfg.DecisionObserver = func(decision LiabilityHedgerDecision) { decisions = append(decisions, decision) }
	hedger := NewLiabilityHedger(1, gw, cfg)
	now := time.Unix(50, 0)
	hedger.subscribed = true
	hedger.obligation, hedger.lastUpdate = 200, now.UnixNano()
	// The first independently replayed side has no matching executable level.
	// It must still be recorded as that side and consume exactly one bit.
	want := rand.New(rand.NewSource(cfg.PolicySeed))
	firstBuy := want.Intn(2) == 0
	book := liabilityHedgerBook{HasSnapshot: true, SourceTime: now.UnixNano(), Sequence: 1}
	if firstBuy {
		book.HasBid, book.BidPrice, book.BidQty = true, 99, 1_000
	} else {
		book.HasAsk, book.AskPrice, book.AskQty = true, 101, 1_000
	}
	hedger.book = book
	hedger.onTick(now)
	if len(gw.requests) != 0 || len(decisions) != 1 {
		t.Fatalf("unavailable selected side submitted: requests=%+v decisions=%+v", gw.requests, decisions)
	}
	first := decisions[0]
	wantSide := "SELL"
	if firstBuy {
		wantSide = "BUY"
	}
	if first.PolicyMode != string(LiabilityHedgerPolicyRandomSideControl) || first.SideEvidence != wantSide || first.ActionOrDeferReason != "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE" {
		t.Fatalf("first random-control decision = %+v, want side=%s unavailable", first, wantSide)
	}

	// A two-sided next tick must use the *next* independent bit, not retry the
	// prior unavailable side and not consume the obligation RNG.
	secondBuy := want.Intn(2) == 0
	hedgerBook := liabilityHedgerBook{HasSnapshot: true, SourceTime: now.Add(cfg.DecisionInterval).UnixNano(), Sequence: 2, HasBid: true, BidPrice: 99, BidQty: 1_000, HasAsk: true, AskPrice: 101, AskQty: 1_000}
	hedger.book = hedgerBook
	hedger.onTick(now.Add(cfg.DecisionInterval))
	if len(gw.requests) != 1 || len(decisions) != 2 || gw.requests[0].OrderReq == nil {
		t.Fatalf("second random-control request/decision = requests=%+v decisions=%+v", gw.requests, decisions)
	}
	request := gw.requests[0].OrderReq
	if (secondBuy && request.Side != exchange.Buy) || (!secondBuy && request.Side != exchange.Sell) {
		t.Fatalf("second random side = %s, want buy=%t", request.Side, secondBuy)
	}
	if decisions[1].SideEvidence != request.Side.String() || decisions[1].ActionOrDeferReason != "SUBMIT_IOC" {
		t.Fatalf("second random-control evidence = %+v request=%+v", decisions[1], request)
	}
}

func TestLiabilityHedgerConfigValidation(t *testing.T) {
	valid := liabilityHedgerTestConfig()
	cases := []struct {
		name string
		edit func(*LiabilityHedgerConfig)
	}{
		{"empty symbol", func(c *LiabilityHedgerConfig) { c.Symbol = "" }},
		{"zero request cap", func(c *LiabilityHedgerConfig) { c.MaxRequestQty = 0 }},
		{"nonmultiple obligation interval", func(c *LiabilityHedgerConfig) { c.ObligationInterval = 3 * time.Second }},
		{"step exceeds bound", func(c *LiabilityHedgerConfig) { c.MaxAbsObligationQty = c.ObligationStepQty - 1 }},
		{"unknown policy mode", func(c *LiabilityHedgerConfig) { c.PolicyMode = "unbounded_magic" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.edit(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("invalid liability hedger config was accepted")
			}
		})
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid liability hedger config rejected: %v", err)
	}
}

func TestLiabilityHedgerValidationRequiresReceiptEvidence(t *testing.T) {
	policy := &LiabilityHedgerConfig{
		Enabled: false, Symbol: "CDF/USD", DecisionInterval: time.Second,
		ObligationInterval: 10 * time.Second, ObligationStepQty: 100,
		MaxAbsObligationQty: 1_000, MaxRequestQty: 100,
	}
	base := Config{
		LogDir: t.TempDir(), LogMode: "full", CrossAssetSpotGraph: true,
		RecordLiabilityHedgerDecisions: true, CDFLiabilityHedger: policy,
	}
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("L0 accepted without independently recorded local feed receipts")
	}
	base.RecordMarketDataReceipts = true
	base.MarketDataReceiptRoles = []string{"liability_hedger"}
	base.LatencyProfiles = map[string]LatencyProfile{"liability_hedger": {Model: "constant", Delay: time.Millisecond}}
	sim, err := NewSim(time.Second, base)
	if err != nil {
		t.Fatalf("L0 rejected documented receipt path: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.LiabilityHedgers) != 1 {
			t.Fatalf("venue %s L0 actors = %d, want 1", venue.ID, len(venue.LiabilityHedgers))
		}
		found := false
		for _, participant := range venue.Participants {
			if participant.Role == "liability_hedger_1" {
				found = true
			}
		}
		if !found {
			t.Fatalf("venue %s omitted liability hedger participant record", venue.ID)
		}
	}
}
