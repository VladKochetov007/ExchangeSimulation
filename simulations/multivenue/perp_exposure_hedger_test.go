package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func perpExposureTestConfig() PerpExposureHedgerConfig {
	return PerpExposureHedgerConfig{
		Enabled: true, Symbol: "ABC-PERP", DecisionInterval: time.Second, ExposureInterval: 10 * time.Second,
		ExposureStepQty: 10, MaxAbsExposure: 50, MaxRequestQty: 10, TickSize: 1,
		InitialQuoteBalance: 100_000, InitialMargin: 50_000, Seed: 17,
		VenueID: "north", Hedger: "perp_exposure_hedger_1", ClientID: 12, TakerFeeBps: 5,
	}
}

func observePerpExposureBook(h *PerpExposureHedger, gateway *fundingCarryStubGateway, now time.Time, bid, ask int64) {
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 9, Ordinal: 4, DeliveredAt: now.UnixNano(), Digest: [16]byte{9}}
	h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 41,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: bid, VisibleQty: 100}}, Asks: []exchange.PriceLevel{{Price: ask, VisibleQty: 100}}},
	}})
}

func TestPerpExposureHedgerTargetsOppositePhysicalExposureAtLocalTouch(t *testing.T) {
	for _, tc := range []struct {
		name         string
		physical     int64
		wantSide     exchange.Side
		wantPrice    int64
		wantPosition int64
	}{
		{"producer physical long shorts perp", 10, exchange.Sell, 100, -10},
		{"consumer physical short buys perp", -10, exchange.Buy, 101, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := newFundingCarryStubGateway()
			var decisions []PerpExposureHedgerDecision
			var fills []PerpExposureHedgerFill
			cfg := perpExposureTestConfig()
			cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
			cfg.FillObserver = func(fill PerpExposureHedgerFill) { fills = append(fills, fill) }
			h := NewPerpExposureHedger(1, gateway, cfg)
			now := time.Unix(10, 0)
			h.subscribed, h.lastUpdate, h.physicalExposure = true, now.UnixNano(), tc.physical
			observePerpExposureBook(h, gateway, now, 100, 101)

			h.onTick(now.Add(time.Second))
			if len(gateway.requests) != 1 || gateway.requests[0].OrderReq == nil {
				t.Fatalf("perp hedge did not submit exactly one request: %+v", gateway.requests)
			}
			decision := decisions[len(decisions)-1]
			request := gateway.requests[0].OrderReq
			if decision.ActionOrDeferReason != "SUBMIT_IOC" || decision.TargetPerpPosition != -tc.physical ||
				decision.Side != tc.wantSide.String() || decision.LimitPrice != tc.wantPrice || decision.RequestedQty != 10 ||
				decision.DecisionFrontierLinkID != 9 || decision.DecisionFrontierOrdinal != 4 ||
				request.RequestID != decision.RequestID || request.Symbol != "ABC-PERP" || request.Side != tc.wantSide ||
				request.Price != tc.wantPrice || request.Qty != 10 || request.TimeInForce != exchange.IOC {
				t.Fatalf("physical target/local order mismatch: decision=%+v request=%+v", decision, request)
			}

			h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: request.RequestID, OrderID: 77}})
			h.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
				OrderID: 77, Symbol: "ABC-PERP", Side: tc.wantSide, Qty: 10, Price: tc.wantPrice, TradeID: 88, IsFull: true, Timestamp: now.Add(2 * time.Second).UnixNano(),
			}})
			if h.PerpPosition() != tc.wantPosition || len(fills) != 1 || fills[0].PrePosition != 0 || fills[0].PostPosition != tc.wantPosition {
				t.Fatalf("actor did not attest the filled perp transition: position=%d fills=%+v", h.PerpPosition(), fills)
			}
		})
	}
}

func TestPerpExposureHedgerDisabledUpdatesStateButCannotSubmit(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []PerpExposureHedgerDecision
	cfg := perpExposureTestConfig()
	cfg.Enabled = false
	cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
	h := NewPerpExposureHedger(1, gateway, cfg)
	now := time.Unix(10, 0)
	h.subscribed = true
	observePerpExposureBook(h, gateway, now, 100, 101)

	h.onTick(now)
	decision := decisions[len(decisions)-1]
	if decision.ActionOrDeferReason != "POLICY_DISABLED" || decision.PhysicalStep == 0 || decision.PhysicalAfter == 0 ||
		decision.TargetPerpPosition != -decision.PhysicalAfter || len(gateway.requests) != 0 {
		t.Fatalf("disabled actor did not retain auditable state-only behavior: decision=%+v requests=%+v", decision, gateway.requests)
	}
}

func TestPerpExposureHedgerKeepsPresentZeroTouchDistinctFromAbsence(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []PerpExposureHedgerDecision
	cfg := perpExposureTestConfig()
	cfg.DecisionObserver = func(decision PerpExposureHedgerDecision) { decisions = append(decisions, decision) }
	h := NewPerpExposureHedger(1, gateway, cfg)
	now := time.Unix(10, 0)
	h.subscribed, h.lastUpdate, h.physicalExposure = true, now.UnixNano(), 10
	observePerpExposureBook(h, gateway, now, 0, 1)

	h.onTick(now.Add(time.Second))
	decision := decisions[len(decisions)-1]
	if decision.ActionOrDeferReason != "PERP_PRICE_OUTSIDE_DOMAIN" || !decision.HasBid || decision.BidPrice != 0 || len(gateway.requests) != 0 {
		t.Fatalf("present zero bid became absent or tradable: decision=%+v requests=%+v", decision, gateway.requests)
	}
}

func TestPerpExposureHedgerConfigRejectsAmbiguousOrUnsafeContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PerpExposureHedgerConfig)
	}{
		{"wrong symbol", func(c *PerpExposureHedgerConfig) { c.Symbol = "ABC/USD" }},
		{"nonmultiple exposure clock", func(c *PerpExposureHedgerConfig) { c.ExposureInterval = 1500 * time.Millisecond }},
		{"zero prefunding", func(c *PerpExposureHedgerConfig) { c.InitialMargin = 0 }},
		{"zero price grid", func(c *PerpExposureHedgerConfig) { c.TickSize = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := perpExposureTestConfig()
			tc.edit(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("invalid P2 policy accepted")
			}
		})
	}
}
