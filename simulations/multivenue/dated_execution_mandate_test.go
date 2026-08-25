package multivenue

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func datedMandateTestConfig() DatedExecutionMandateConfig {
	return DatedExecutionMandateConfig{
		Enabled: true, Underlying: "ABC/USD", TargetTenor: 8 * time.Hour, Side: exchange.Buy.String(),
		ParentQty: 200, ChildQty: 10, StartDelay: 10 * time.Minute, ExecutionDuration: 2 * time.Hour,
		DecisionPeriod: 5 * time.Minute, MaxMarketAge: 10 * time.Second, SlippageBps: 15, TickSize: 1,
		VenueID: "north", Desk: "dated_execution_mandate_1", ClientID: 31,
	}
}

func deliverDatedMandateListing(t *testing.T, mandate *DatedExecutionMandate, listed time.Time, symbol string, tenor time.Duration, sequence uint64) {
	t.Helper()
	listedNano := listed.UnixNano()
	mandate.HandleEvent(context.Background(), &actor.Event{Type: actor.EventInstrument, Data: actor.InstrumentEvent{
		Timestamp: listed.Add(time.Second).UnixNano(), SeqNum: sequence,
		Announcement: &exchange.InstrumentAnnouncement{
			Action: "listed", Symbol: symbol, InstrumentType: "FUTURE", Underlying: "ABC/USD",
			ListedNano: &listedNano, ExpiryNano: listed.Add(tenor).UnixNano(),
		},
	}})
}

func TestDatedExecutionMandateWorksFiniteParentThroughOrdinaryIOC(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []DatedExecutionMandateDecision
	var outcomes []DatedExecutionMandateOutcome
	cfg := datedMandateTestConfig()
	cfg.DecisionObserver = func(decision DatedExecutionMandateDecision) { decisions = append(decisions, decision) }
	cfg.OutcomeObserver = func(outcome DatedExecutionMandateOutcome) { outcomes = append(outcomes, outcome) }
	mandate := NewDatedExecutionMandate(1, gateway, cfg)
	listed := time.Unix(100, 0)

	mandate.onTick(listed)
	if len(gateway.requests) != 1 || decisions[0].Action != "NOT_SUBSCRIBED" {
		t.Fatalf("initial subscription = requests %+v decisions %+v", gateway.requests, decisions)
	}
	deliverDatedMandateListing(t, mandate, listed, "ABC-FUT-8H", 8*time.Hour, 7)
	deliverDatedMandateListing(t, mandate, listed, "ABC-FUT-72H", 72*time.Hour, 8)
	if len(mandate.contracts) != 1 || mandate.contracts["ABC-FUT-8H"] == nil {
		t.Fatalf("tenor selection = %+v", mandate.contracts)
	}
	if len(gateway.requests) != 2 || gateway.requests[1].QueryReq.Symbol != "ABC-FUT-8H" {
		t.Fatalf("contract subscription = %+v", gateway.requests)
	}

	mandate.onTick(listed.Add(5 * time.Minute))
	if got := decisions[len(decisions)-1]; got.Action != "BEFORE_START" || got.OriginalTenorNanos != int64(8*time.Hour) {
		t.Fatalf("pre-start decision = %+v", got)
	}

	bookAt := listed.Add(10 * time.Minute)
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 9, Ordinal: 3, DeliveredAt: bookAt.UnixNano(), Digest: [16]byte{2}}
	mandate.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC-FUT-8H", Timestamp: bookAt.UnixNano(), SeqNum: 11,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: 99_000, VisibleQty: 20}}, Asks: []exchange.PriceLevel{{Price: 100_000, VisibleQty: 20}}},
	}})
	mandate.onTick(bookAt)
	child := decisions[len(decisions)-1]
	if child.Action != "SUBMIT_CHILD_IOC" || child.Side != "BUY" || child.RequestedQty != 10 || child.LimitPrice <= 100_000 || child.RequestID == 0 {
		t.Fatalf("first child = %+v", child)
	}
	request := gateway.requests[len(gateway.requests)-1]
	if request.OrderReq == nil || request.OrderReq.TimeInForce != exchange.IOC || request.OrderReq.Symbol != "ABC-FUT-8H" || request.OrderReq.Qty != 10 {
		t.Fatalf("ordinary IOC request = %+v", request)
	}
	mandate.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: child.RequestID, OrderID: 41}})
	mandate.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{
		OrderID: 41, Symbol: "ABC-FUT-8H", Side: exchange.Buy, Qty: 10, Price: 100_000, TradeID: 51, IsFull: true, Timestamp: bookAt.Add(time.Millisecond).UnixNano(),
	}})
	if mandate.contracts["ABC-FUT-8H"].filled != 10 || len(outcomes) != 2 || outcomes[1].RemainingQty != 190 {
		t.Fatalf("canonical fill reconstruction = contract %+v outcomes %+v", mandate.contracts["ABC-FUT-8H"], outcomes)
	}
}

func TestDatedExecutionMandateMakesStaleAndExpiredResidualObservable(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []DatedExecutionMandateDecision
	cfg := datedMandateTestConfig()
	cfg.DecisionObserver = func(decision DatedExecutionMandateDecision) { decisions = append(decisions, decision) }
	mandate := NewDatedExecutionMandate(1, gateway, cfg)
	mandate.subscribed = true
	listed := time.Unix(100, 0)
	deliverDatedMandateListing(t, mandate, listed, "ABC-FUT-8H", 8*time.Hour, 7)
	contract := mandate.contracts["ABC-FUT-8H"]
	contract.book = fundingCarryBook{hasSnapshot: true, publishedAt: listed.UnixNano(), sequence: 3, hasAsk: true, ask: 100, askQty: 10}

	mandate.onTick(listed.Add(10 * time.Minute))
	if got := decisions[len(decisions)-1]; got.Action != "BOOK_STALE" || got.MarketAgeNanos != int64(10*time.Minute) {
		t.Fatalf("stale decision = %+v", got)
	}
	mandate.onTick(contractTime(contract.endAt))
	if got := decisions[len(decisions)-1]; got.Action != "PARENT_HORIZON_EXPIRED" || got.RemainingQty != cfg.ParentQty || got.RequestID != 0 {
		t.Fatalf("expired residual decision = %+v", got)
	}
}

func TestDatedExecutionMandateRejectsAmbiguousListingEpoch(t *testing.T) {
	mandate := NewDatedExecutionMandate(1, newFundingCarryStubGateway(), datedMandateTestConfig())
	mandate.HandleEvent(context.Background(), &actor.Event{Type: actor.EventInstrument, Data: actor.InstrumentEvent{
		Timestamp: 10, SeqNum: 1,
		Announcement: &exchange.InstrumentAnnouncement{Action: "listed", Symbol: "ABC-FUT", InstrumentType: "FUTURE", Underlying: "ABC/USD", ExpiryNano: int64(8 * time.Hour)},
	}})
	if len(mandate.contracts) != 0 {
		t.Fatalf("listing without explicit epoch was admitted: %+v", mandate.contracts)
	}
}

func contractTime(nanos int64) time.Time { return time.Unix(0, nanos) }
