package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

type fundingCarryStubGateway struct {
	*stoikovStubGateway
	frontier simulation.MarketDataFrontier
}

func newFundingCarryStubGateway() *fundingCarryStubGateway {
	return &fundingCarryStubGateway{stoikovStubGateway: newStoikovStubGateway()}
}

func (g *fundingCarryStubGateway) MarketDataFrontier() simulation.MarketDataFrontier {
	return g.frontier
}

func fundingCarryTestConfig() FundingCarryArbitrageurConfig {
	return FundingCarryArbitrageurConfig{
		Enabled: true, SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", DecisionPeriod: time.Second,
		FundingHorizon: 1, MaxFundingAge: time.Minute, TakerFeeBps: 1, BorrowAnnualBps: 0,
		BalanceSheetBps: 0, MarginRiskBps: 0, LegRiskBps: 0, MinNetCarryBps: 50,
		MaxPosition: 100, LotQty: 50, MinOrderSize: 1, SpotTick: 1, PerpTick: 1,
		VenueID: "north", Desk: "funding_carry_arb_1", ClientID: 9,
	}
}

func observeFundingCarryBooks(t *testing.T, desk *FundingCarryArbitrageur, gateway *fundingCarryStubGateway, now time.Time, spotBid, spotAsk, perpBid, perpAsk, rate int64) {
	t.Helper()
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 1, DeliveredAt: now.UnixNano()}
	desk.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC/USD", Timestamp: now.UnixNano(), SeqNum: 11,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: spotBid, VisibleQty: 1_000}}, Asks: []exchange.PriceLevel{{Price: spotAsk, VisibleQty: 1_000}}},
	}})
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 2, DeliveredAt: now.UnixNano()}
	desk.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 12,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: perpBid, VisibleQty: 1_000}}, Asks: []exchange.PriceLevel{{Price: perpAsk, VisibleQty: 1_000}}},
	}})
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 3, DeliveredAt: now.UnixNano(), Digest: [16]byte{1}}
	desk.HandleEvent(context.Background(), &actor.Event{Type: actor.EventFundingUpdate, Data: actor.FundingUpdateEvent{
		Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 13,
		FundingRate: &exchange.FundingRate{Symbol: "ABC-PERP", Rate: rate, NextFunding: now.Add(8 * time.Hour).UnixNano(), Interval: int64((8 * time.Hour) / time.Second), MarkAvailable: true, MarkPrice: perpBid, IndexAvailable: true, IndexPrice: spotBid},
	}})
}

func TestFundingCarryUsesDelayedDeliveredFundingForPositivePremium(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []FundingCarryDecision
	var outcomes []FundingCarryLegOutcome
	cfg := fundingCarryTestConfig()
	cfg.DecisionObserver = func(decision FundingCarryDecision) { decisions = append(decisions, decision) }
	cfg.OutcomeObserver = func(outcome FundingCarryLegOutcome) { outcomes = append(outcomes, outcome) }
	desk := NewFundingCarryArbitrageur(1, gateway, cfg)
	now := time.Unix(10, 0)

	desk.onTick(now)
	if len(gateway.requests) != 2 || decisions[0].ActionOrDefer != "NOT_SUBSCRIBED" {
		t.Fatalf("initial funding subscriptions/decision = requests=%+v decisions=%+v", gateway.requests, decisions)
	}
	if types := gateway.requests[1].QueryReq.Types; len(types) != 2 || types[0] != exchange.MDSnapshot || types[1] != exchange.MDFunding {
		t.Fatalf("perp subscription omitted funding: %+v", gateway.requests[1])
	}

	observeFundingCarryBooks(t, desk, gateway, now, 100, 101, 102, 103, 100)
	desk.onTick(now.Add(time.Second))
	if len(gateway.requests) != 3 || gateway.requests[2].OrderReq == nil {
		t.Fatalf("positive funding carry did not submit one spot leg: %+v", gateway.requests)
	}
	decision := decisions[len(decisions)-1]
	request := gateway.requests[2].OrderReq
	if decision.ActionOrDefer != "SUBMIT_SPOT_TARGET_IOC" || decision.Side != "BUY" || decision.Leg != "SPOT_TARGET_ADJUSTMENT" ||
		decision.PremiumBps <= 0 || decision.FundingIncomeBps != 100 || decision.NetCarryBps != 96 ||
		decision.FundingSequence != 13 || decision.DecisionFrontierOrdinal != 3 || decision.DecisionFrontierDeliveredAt > decision.DecisionTime ||
		request.RequestID != decision.RequestID || request.Symbol != "ABC/USD" || request.Side != exchange.Buy || request.Price != 101 || request.Qty != 50 || request.TimeInForce != exchange.IOC {
		t.Fatalf("positive funding decision/request mismatch: decision=%+v request=%+v", decision, request)
	}

	desk.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: request.RequestID, OrderID: 44}})
	desk.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{OrderID: 44, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 50, Price: 101, TradeID: 71, IsFull: true, Timestamp: now.Add(2 * time.Second).UnixNano()}})
	desk.onTick(now.Add(3 * time.Second))
	if len(gateway.requests) != 4 || gateway.requests[3].OrderReq == nil {
		t.Fatalf("filled spot leg did not create explicit perp orphan repair: %+v", gateway.requests)
	}
	repair := decisions[len(decisions)-1]
	if repair.ActionOrDefer != "SUBMIT_PERP_ORPHAN_REPAIR_IOC" || repair.Side != "SELL" || repair.Leg != "PERP_ORPHAN_REPAIR" || gateway.requests[3].OrderReq.Symbol != "ABC-PERP" || gateway.requests[3].OrderReq.Side != exchange.Sell {
		t.Fatalf("orphan repair not explicit/correct: decision=%+v request=%+v", repair, gateway.requests[3])
	}
	if len(outcomes) != 2 || outcomes[0].Event != "ORDER_ACCEPTED" || outcomes[1].Event != "ORDER_FILL" || outcomes[1].RequestID != decision.RequestID || outcomes[1].ExecutionTime != now.Add(2*time.Second).UnixNano() || outcomes[1].SpotPositionAfter != 50 {
		t.Fatalf("leg outcomes did not attest non-atomic fill: %+v", outcomes)
	}
}

func TestFundingCarryZeroAndSignMirrorFollowDeclaredEconomicRule(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		spotBid, spotAsk, perpBid, perpAsk, rate int64
		wantAction, wantSide                     string
	}{
		{"zero funding defers", 100, 101, 102, 103, 0, "NET_CARRY_BELOW_MINIMUM", ""},
		{"negative mirror sells spot", 100, 101, 98, 99, -100, "SUBMIT_SPOT_TARGET_IOC", "SELL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := newFundingCarryStubGateway()
			var decisions []FundingCarryDecision
			cfg := fundingCarryTestConfig()
			cfg.DecisionObserver = func(decision FundingCarryDecision) { decisions = append(decisions, decision) }
			desk := NewFundingCarryArbitrageur(1, gateway, cfg)
			now := time.Unix(10, 0)
			desk.subscribed = true
			observeFundingCarryBooks(t, desk, gateway, now, tc.spotBid, tc.spotAsk, tc.perpBid, tc.perpAsk, tc.rate)
			desk.onTick(now.Add(time.Second))
			decision := decisions[len(decisions)-1]
			if decision.ActionOrDefer != tc.wantAction || decision.Side != tc.wantSide {
				t.Fatalf("funding sign decision = %+v, want action=%q side=%q", decision, tc.wantAction, tc.wantSide)
			}
			if tc.wantSide == "" && len(gateway.requests) != 0 {
				t.Fatalf("non-positive net carry submitted: %+v", gateway.requests)
			}
			if tc.wantSide == "SELL" && (len(gateway.requests) != 1 || gateway.requests[0].OrderReq.Side != exchange.Sell || decision.FundingIncomeBps <= 0) {
				t.Fatalf("negative mirror did not choose short spot/long perp: requests=%+v decision=%+v", gateway.requests, decision)
			}
		})
	}
}

func TestFundingCarryRejectsStaleDuplicateAndPresentZeroPriceExplicitly(t *testing.T) {
	now := time.Unix(10, 0)
	t.Run("stale funding", func(t *testing.T) {
		gateway := newFundingCarryStubGateway()
		var decisions []FundingCarryDecision
		cfg := fundingCarryTestConfig()
		cfg.DecisionObserver = func(decision FundingCarryDecision) { decisions = append(decisions, decision) }
		desk := NewFundingCarryArbitrageur(1, gateway, cfg)
		desk.subscribed = true
		observeFundingCarryBooks(t, desk, gateway, now, 100, 101, 102, 103, 100)
		desk.onTick(now.Add(2 * time.Minute))
		if got := decisions[len(decisions)-1].ActionOrDefer; got != "FUNDING_STALE" {
			t.Fatalf("stale funding action=%q", got)
		}
	})
	t.Run("duplicate funding cannot replace frontier", func(t *testing.T) {
		gateway := newFundingCarryStubGateway()
		desk := NewFundingCarryArbitrageur(1, gateway, fundingCarryTestConfig())
		desk.subscribed = true
		observeFundingCarryBooks(t, desk, gateway, now, 100, 101, 102, 103, 100)
		gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 4, DeliveredAt: now.UnixNano()}
		desk.HandleEvent(context.Background(), &actor.Event{Type: actor.EventFundingUpdate, Data: actor.FundingUpdateEvent{
			Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 13,
			FundingRate: &exchange.FundingRate{Rate: -100, NextFunding: now.Add(8 * time.Hour).UnixNano(), Interval: 28_800, MarkAvailable: true, IndexAvailable: true},
		}})
		if desk.funding.rate.Rate != 100 || desk.funding.sequence != 13 {
			t.Fatalf("duplicate funding advanced local cache: %+v", desk.funding)
		}
	})
	t.Run("present zero price is domain error not absence", func(t *testing.T) {
		gateway := newFundingCarryStubGateway()
		var decisions []FundingCarryDecision
		cfg := fundingCarryTestConfig()
		cfg.DecisionObserver = func(decision FundingCarryDecision) { decisions = append(decisions, decision) }
		desk := NewFundingCarryArbitrageur(1, gateway, cfg)
		desk.subscribed = true
		observeFundingCarryBooks(t, desk, gateway, now, 0, 1, 102, 103, 100)
		desk.onTick(now.Add(time.Second))
		decision := decisions[len(decisions)-1]
		if decision.ActionOrDefer != "LOCAL_PRICE_OUTSIDE_DOMAIN" || !decision.HasSpotBid || decision.SpotBid != 0 {
			t.Fatalf("zero present price became unavailable: %+v", decision)
		}
	})
}

func TestFundingCarryFinancialsUseExactSignedCosts(t *testing.T) {
	cfg := fundingCarryTestConfig()
	now := time.Unix(10, 0).UnixNano()
	rate := exchange.FundingRate{Rate: -100, NextFunding: now + int64(8*time.Hour), Interval: 28_800}
	financials, ok := fundingCarryComputeFinancials(cfg, rate, now, -1)
	if !ok || financials.fundingIncome != 100 || financials.takerFees != 4 || financials.netCarry != 96 {
		t.Fatalf("signed funding financials = %+v ok=%t", financials, ok)
	}
}

func TestFundingCarryConfigRequiresExplicitEvidencePath(t *testing.T) {
	policy := fundingCarryTestConfig()
	base := Config{LogDir: t.TempDir(), LogMode: "full", FundingCarryArbitrageur: &policy, RecordFundingCarryDecisions: true}
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("funding carry P0 accepted without delayed public receipt evidence")
	}
	base.RecordMarketDataReceipts = true
	base.MarketDataReceiptRoles = []string{"funding_carry_arb"}
	base.LatencyProfiles = map[string]LatencyProfile{"funding_carry_arb": {Model: "constant", Delay: time.Millisecond}}
	sim, err := NewSim(time.Second, base)
	if err != nil {
		t.Fatalf("funding carry P0 rejected documented receipt path: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.FundingCarryArbs) != 1 {
			t.Fatalf("venue %s funding carry actors = %d, want 1", venue.ID, len(venue.FundingCarryArbs))
		}
	}
}
