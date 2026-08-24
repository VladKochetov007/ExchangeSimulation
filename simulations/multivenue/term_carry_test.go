package multivenue

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func termCarryTestConfig() TermCarryAllocatorConfig {
	return TermCarryAllocatorConfig{
		Enabled: true, SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", DecisionPeriod: time.Second,
		CommitmentIntervals: 1, MaxFundingAge: time.Minute, TakerFeeBps: 1,
		LongSpotFundingBps: 0, ShortSpotBorrowBps: 0, BalanceSheetBps: 0, MarginRiskBps: 0, LegRiskBps: 0, MinNetCarryBps: 1,
		MaxPosition: 50, LotQty: 50, MinOrderSize: 1, SpotTick: 1, PerpTick: 1,
		VenueID: "north", Desk: "term_carry_allocator_1", ClientID: 19,
	}
}

func observeTermCarryBooks(t *testing.T, allocator *TermCarryAllocator, gateway *fundingCarryStubGateway, now time.Time, spotBid, spotAsk, perpBid, perpAsk, rate int64) {
	t.Helper()
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 1, DeliveredAt: now.UnixNano()}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC/USD", Timestamp: now.UnixNano(), SeqNum: 11,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: spotBid, VisibleQty: 1_000}}, Asks: []exchange.PriceLevel{{Price: spotAsk, VisibleQty: 1_000}}},
	}})
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 2, DeliveredAt: now.UnixNano()}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 12,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: perpBid, VisibleQty: 1_000}}, Asks: []exchange.PriceLevel{{Price: perpAsk, VisibleQty: 1_000}}},
	}})
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 7, Ordinal: 3, DeliveredAt: now.UnixNano(), Digest: [16]byte{1}}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventFundingUpdate, Data: actor.FundingUpdateEvent{
		Symbol: "ABC-PERP", Timestamp: now.UnixNano(), SeqNum: 13,
		FundingRate: &exchange.FundingRate{Symbol: "ABC-PERP", Rate: rate, NextFunding: now.Add(8 * time.Hour).UnixNano(), Interval: int64((8 * time.Hour) / time.Second), MarkAvailable: true, MarkPrice: perpBid, IndexAvailable: true, IndexPrice: spotBid},
	}})
}

func acceptAndFillTermCarry(t *testing.T, allocator *TermCarryAllocator, requestID, orderID uint64, symbol string, side exchange.Side, at time.Time) {
	t.Helper()
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: requestID, OrderID: orderID}})
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{OrderID: orderID, Symbol: symbol, Side: side, Qty: 50, Price: 100, TradeID: orderID, IsFull: true, Timestamp: at.UnixNano()}})
}

func TestTermCarryAllocatorCompletesOneExplicitTerm(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	var outcomes []TermCarryLegOutcome
	cfg := termCarryTestConfig()
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	cfg.OutcomeObserver = func(outcome TermCarryLegOutcome) { outcomes = append(outcomes, outcome) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	now := time.Unix(10, 0)
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)

	allocator.onTick(now.Add(time.Second))
	entrySpot := decisions[len(decisions)-1]
	if entrySpot.Action != "SUBMIT_ENTRY_SPOT_IOC" || entrySpot.State != termCarryEntrySpot || entrySpot.PlanCreatedAt != entrySpot.DecisionTime || entrySpot.FirstExposureAt != 0 || entrySpot.TermEnd != now.Add(8*time.Hour).UnixNano() || entrySpot.Side != "BUY" || entrySpot.RequestID == 0 {
		t.Fatalf("entry spot decision = %+v", entrySpot)
	}
	firstFillAt := now.Add(2 * time.Second)
	acceptAndFillTermCarry(t, allocator, entrySpot.RequestID, 41, "ABC/USD", exchange.Buy, firstFillAt)

	allocator.onTick(now.Add(3 * time.Second))
	entryPerp := decisions[len(decisions)-1]
	if entryPerp.Action != "SUBMIT_ENTRY_PERP_IOC" || entryPerp.State != termCarryEntryPerp || entryPerp.PlanCreatedAt != entrySpot.DecisionTime || entryPerp.FirstExposureAt != firstFillAt.UnixNano() || entryPerp.Side != "SELL" || entryPerp.TargetSpot != 50 || entryPerp.TargetPerp != -50 {
		t.Fatalf("entry perp decision = %+v", entryPerp)
	}
	acceptAndFillTermCarry(t, allocator, entryPerp.RequestID, 42, "ABC-PERP", exchange.Sell, now.Add(4*time.Second))

	allocator.onTick(now.Add(5 * time.Second))
	if active := decisions[len(decisions)-1]; active.Action != "TERM_ACTIVE" || active.State != termCarryActive || active.FirstExposureAt != firstFillAt.UnixNano() || allocator.spotPosition != 50 || allocator.perpPosition != -50 {
		t.Fatalf("matched pair did not become active: decision=%+v positions=%d/%d", active, allocator.spotPosition, allocator.perpPosition)
	}

	allocator.onTick(now.Add(8*time.Hour + time.Second))
	unwindPerp := decisions[len(decisions)-1]
	if unwindPerp.Action != "SUBMIT_UNWIND_PERP_IOC" || unwindPerp.State != termCarryUnwindPerp || unwindPerp.Side != "BUY" {
		t.Fatalf("term did not start perp unwind: %+v", unwindPerp)
	}
	acceptAndFillTermCarry(t, allocator, unwindPerp.RequestID, 43, "ABC-PERP", exchange.Buy, now.Add(8*time.Hour+2*time.Second))

	allocator.onTick(now.Add(8*time.Hour + 3*time.Second))
	unwindSpot := decisions[len(decisions)-1]
	if unwindSpot.Action != "SUBMIT_UNWIND_SPOT_IOC" || unwindSpot.State != termCarryUnwindSpot || unwindSpot.Side != "SELL" {
		t.Fatalf("perp close did not start spot unwind: %+v", unwindSpot)
	}
	acceptAndFillTermCarry(t, allocator, unwindSpot.RequestID, 44, "ABC/USD", exchange.Sell, now.Add(8*time.Hour+4*time.Second))

	allocator.onTick(now.Add(8*time.Hour + 5*time.Second))
	if closed := decisions[len(decisions)-1]; closed.Action != "TERM_CLOSED" || closed.State != termCarryIdle || allocator.spotPosition != 0 || allocator.perpPosition != 0 {
		t.Fatalf("term did not close once: decision=%+v positions=%d/%d", closed, allocator.spotPosition, allocator.perpPosition)
	}
	if len(outcomes) != 8 {
		t.Fatalf("outcomes = %d, want accepted/fill for four non-atomic legs", len(outcomes))
	}
}

func TestTermCarryAllocatorRecordsFirstPartialExposureOnce(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	cfg := termCarryTestConfig()
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	now := time.Unix(10, 0)
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)

	allocator.onTick(now.Add(time.Second))
	requestID := allocator.pending.requestID
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: requestID, OrderID: 41}})
	first := now.Add(2 * time.Second)
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderPartialFill, Data: actor.OrderFillEvent{OrderID: 41, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 20, Price: 101, TradeID: 1, Timestamp: first.UnixNano()}})
	if allocator.plan == nil || allocator.plan.firstExposureAt != first.UnixNano() {
		t.Fatalf("first partial fill did not set first exposure: %+v", allocator.plan)
	}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{OrderID: 41, Symbol: "ABC/USD", Side: exchange.Buy, Qty: 30, Price: 101, TradeID: 2, IsFull: true, Timestamp: now.Add(3 * time.Second).UnixNano()}})
	if allocator.plan.firstExposureAt != first.UnixNano() {
		t.Fatalf("later fill changed first exposure: %+v", allocator.plan)
	}
}

func TestTermCarryAllocatorDefersUnavailableUnwindAndRecovers(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	cfg := termCarryTestConfig()
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	now := time.Unix(10, 0)
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)
	allocator.state = termCarryActive
	allocator.plan = &termCarryPlan{direction: 1, planCreatedAt: now.UnixNano(), firstExposureAt: now.UnixNano(), termEnd: now.Add(time.Second).UnixNano()}
	allocator.spotPosition, allocator.perpPosition = 50, -50
	allocator.perp.hasAsk = false

	allocator.onTick(now.Add(2 * time.Second))
	if unavailable := decisions[len(decisions)-1]; unavailable.Action != "UNWIND_PRICE_UNAVAILABLE" || unavailable.RequestID != 0 || allocator.state != termCarryUnwindPerp {
		t.Fatalf("unavailable unwind was not explicit defer: %+v", unavailable)
	}
	allocator.perp.hasAsk, allocator.perp.ask = true, 103
	allocator.onTick(now.Add(3 * time.Second))
	if recovered := decisions[len(decisions)-1]; recovered.Action != "SUBMIT_UNWIND_PERP_IOC" || recovered.RequestID == 0 {
		t.Fatalf("unwind did not deterministically recover: %+v", recovered)
	}
}

func TestTermCarryAllocatorDoesNotCreateTermBeforeExecutableEntry(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	cfg := termCarryTestConfig()
	cfg.MaxPosition, cfg.LotQty, cfg.MinOrderSize = 100, 100, 75
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	now := time.Unix(10, 0)
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)
	allocator.spot.askQty = 50

	allocator.onTick(now.Add(time.Second))
	deferred := decisions[len(decisions)-1]
	if deferred.Action != "EXECUTABLE_SIZE_UNAVAILABLE" || deferred.State != termCarryIdle || deferred.PlanCreatedAt != 0 || deferred.FirstExposureAt != 0 || deferred.TermEnd != 0 || deferred.TargetSpot != 0 || deferred.TargetPerp != 0 || allocator.plan != nil || allocator.state != termCarryIdle {
		t.Fatalf("flat unavailable entry created a term: decision=%+v state=%s plan=%+v", deferred, allocator.state, allocator.plan)
	}

	allocator.spot.askQty = 1_000
	allocator.onTick(now.Add(2 * time.Second))
	if entry := decisions[len(decisions)-1]; entry.Action != "SUBMIT_ENTRY_SPOT_IOC" || entry.State != termCarryEntrySpot || entry.PlanCreatedAt == 0 || entry.FirstExposureAt != 0 || entry.TermEnd <= entry.PlanCreatedAt || allocator.plan == nil {
		t.Fatalf("fresh executable retry did not create a term: decision=%+v state=%s plan=%+v", entry, allocator.state, allocator.plan)
	}
}

func TestTermCarryAllocatorSeparatesEntryAndUnwindMinimums(t *testing.T) {
	zero := int64(0)
	for _, tc := range []struct {
		name           string
		unwindMinimum  *int64
		wantPolicy     string
		wantAction     string
		wantRequestQty int64
	}{
		{
			name:       "legacy policy retains entry floor for unwind",
			wantPolicy: termCarryPolicyVersionV2, wantAction: "EXECUTABLE_SIZE_UNAVAILABLE",
		},
		{
			name:          "explicit zero cannot undercut venue unwind minimum",
			unwindMinimum: &zero, wantPolicy: termCarryPolicyVersionV3,
			wantAction: "EXECUTABLE_SIZE_UNAVAILABLE",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantConfiguredMinimum := int64(0)
			if tc.unwindMinimum != nil {
				wantConfiguredMinimum = *tc.unwindMinimum
			}
			gateway := newFundingCarryStubGateway()
			var decisions []TermCarryDecision
			cfg := termCarryTestConfig()
			cfg.MaxPosition, cfg.LotQty, cfg.MinOrderSize, cfg.UnwindMinOrderSize = 100, 100, 75, tc.unwindMinimum
			cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
			allocator := NewTermCarryAllocator(1, gateway, cfg)
			allocator.subscribed = true
			now := time.Unix(10, 0)
			observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)
			allocator.spot.askQty, allocator.perp.askQty = 50, 50

			// The 50-unit entry remains below the 75-unit entry floor under both
			// policies; the exit exception must never leak into admission.
			allocator.onTick(now.Add(time.Second))
			entry := decisions[len(decisions)-1]
			if entry.Action != "EXECUTABLE_SIZE_UNAVAILABLE" || entry.RequestID != 0 || entry.PolicyVersion != tc.wantPolicy || (tc.unwindMinimum == nil && entry.UnwindMinOrderSize != nil) || (tc.unwindMinimum != nil && (entry.UnwindMinOrderSize == nil || *entry.UnwindMinOrderSize != wantConfiguredMinimum)) {
				t.Fatalf("entry policy leaked or evidence was ambiguous: %+v", entry)
			}

			allocator.state = termCarryActive
			allocator.plan = &termCarryPlan{direction: 1, planCreatedAt: now.UnixNano(), firstExposureAt: now.UnixNano(), termEnd: now.Add(time.Second).UnixNano()}
			allocator.spotPosition, allocator.perpPosition = 50, -50
			allocator.onTick(now.Add(2 * time.Second))
			unwind := decisions[len(decisions)-1]
			if unwind.Action != tc.wantAction || unwind.RequestedQty != tc.wantRequestQty || unwind.PolicyVersion != tc.wantPolicy || (tc.unwindMinimum == nil && unwind.UnwindMinOrderSize != nil) || (tc.unwindMinimum != nil && (unwind.UnwindMinOrderSize == nil || *unwind.UnwindMinOrderSize != wantConfiguredMinimum)) {
				t.Fatalf("unwind minimum policy mismatch: %+v", unwind)
			}
			if tc.unwindMinimum == nil && unwind.RequestID != 0 {
				t.Fatalf("legacy undersized unwind unexpectedly entered: %+v", unwind)
			}
			if tc.unwindMinimum != nil && unwind.RequestID != 0 {
				t.Fatalf("explicit zero bypassed the venue minimum: %+v", unwind)
			}
		})
	}
}

func TestTermCarryAllocatorPartialUnwindRetainsResidual(t *testing.T) {
	zero := int64(0)
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	cfg := termCarryTestConfig()
	cfg.MaxPosition, cfg.LotQty, cfg.MinOrderSize, cfg.UnwindMinOrderSize = 100, 100, 75, &zero
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	now := time.Unix(10, 0)
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)
	allocator.perp.askQty = 75
	allocator.state = termCarryActive
	allocator.plan = &termCarryPlan{direction: 1, planCreatedAt: now.UnixNano(), firstExposureAt: now.UnixNano(), termEnd: now.Add(time.Second).UnixNano()}
	allocator.spotPosition, allocator.perpPosition = 100, -100

	allocator.onTick(now.Add(2 * time.Second))
	first := decisions[len(decisions)-1]
	if first.Action != "SUBMIT_UNWIND_PERP_IOC" || first.RequestedQty != 75 || first.RequestID == 0 {
		t.Fatalf("first bounded unwind = %+v", first)
	}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: first.RequestID, OrderID: 41}})
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderPartialFill, Data: actor.OrderFillEvent{OrderID: 41, Symbol: "ABC-PERP", Side: exchange.Buy, Qty: 20, Price: 103, TradeID: 1, Timestamp: now.Add(3 * time.Second).UnixNano()}})
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelled, Data: actor.OrderCancelledEvent{OrderID: 41, RemainingQty: 30}})
	if allocator.perpPosition != -80 || allocator.state != termCarryUnwindPerp || allocator.plan == nil {
		t.Fatalf("partial unwind was closed or erased: position=%d state=%s plan=%+v", allocator.perpPosition, allocator.state, allocator.plan)
	}
	allocator.onTick(now.Add(4 * time.Second))
	next := decisions[len(decisions)-1]
	if next.Action != "SUBMIT_UNWIND_PERP_IOC" || next.RequestedQty != 75 || next.RequestID == 0 || allocator.state != termCarryUnwindPerp {
		t.Fatalf("residual was not retried at bounded touch: %+v", next)
	}
}

func TestTermCarryAllocatorPassiveExitIsPostOnlyAndDeadlineBounded(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	var outcomes []TermCarryLegOutcome
	now := time.Unix(10, 0)
	cfg := termCarryTestConfig()
	cfg.MaxPosition, cfg.LotQty, cfg.MinOrderSize = 100, 100, 75
	cfg.PassiveExit = &TermCarryPassiveExitConfig{SliceQty: 75, DeadlineAtNano: now.Add(5 * time.Second).UnixNano()}
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	cfg.OutcomeObserver = func(outcome TermCarryLegOutcome) { outcomes = append(outcomes, outcome) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)
	// The contra ask cannot support a legal 75-unit IOC exit, while the local
	// bid is a valid same-side reference for a resting buy-to-cover child.
	allocator.perp.askQty, allocator.perp.bidQty = 50, 1_000
	allocator.state = termCarryActive
	allocator.plan = &termCarryPlan{direction: 1, planCreatedAt: now.UnixNano(), firstExposureAt: now.UnixNano(), termEnd: now.Add(time.Second).UnixNano()}
	allocator.spotPosition, allocator.perpPosition = 100, -100

	allocator.onTick(now.Add(2 * time.Second))
	if len(gateway.requests) != 1 || gateway.requests[0].OrderReq == nil {
		t.Fatalf("passive exit did not submit exactly one ordinary order: %+v", gateway.requests)
	}
	first := decisions[len(decisions)-1]
	order := gateway.requests[0].OrderReq
	if first.PolicyVersion != termCarryPolicyVersionV4 || first.Action != "SUBMIT_UNWIND_PERP_POST_ONLY" || first.Leg != "UNWIND_PERP_POST_ONLY" || first.Side != exchange.Buy.String() || first.RequestedQty != 75 || first.LimitPrice != 102 || first.OrderType != exchange.LimitOrder.String() || first.TimeInForce != exchange.GTC.String() || first.PostOnly == nil || !*first.PostOnly || first.PassiveExitSliceQty == nil || *first.PassiveExitSliceQty != 75 || first.PassiveExitDeadlineAtNano == nil || *first.PassiveExitDeadlineAtNano != cfg.PassiveExit.DeadlineAtNano {
		t.Fatalf("passive-exit decision lost its declared contract: %+v", first)
	}
	if order.RequestID != first.RequestID || order.Symbol != "ABC-PERP" || order.Side != exchange.Buy || order.Price != 102 || order.Qty != 75 || order.TimeInForce != exchange.GTC || !order.PostOnly {
		t.Fatalf("submitted passive exit disagrees with decision: order=%+v decision=%+v", order, first)
	}

	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: first.RequestID, OrderID: 41}})
	allocator.onTick(now.Add(4 * time.Second))
	if resting := decisions[len(decisions)-1]; resting.Action != "PASSIVE_EXIT_RESTING" || resting.RequestID != 0 || resting.CancelRequestID != 0 {
		t.Fatalf("resting passive child was not explicitly observed: %+v", resting)
	}
	if len(gateway.requests) != 1 {
		t.Fatalf("resting passive child created another request: %+v", gateway.requests)
	}

	allocator.onTick(now.Add(5 * time.Second))
	cancel := decisions[len(decisions)-1]
	if cancel.Action != "CANCEL_PASSIVE_EXIT_AT_DEADLINE" || cancel.RequestID != 0 || cancel.CancelOrderID != 41 || cancel.CancelRequestID == 0 || len(gateway.requests) != 2 || gateway.requests[1].CancelReq == nil || gateway.requests[1].CancelReq.OrderID != 41 || gateway.requests[1].CancelReq.RequestID != cancel.CancelRequestID {
		t.Fatalf("deadline cancellation is not exact/order-bound evidence: decision=%+v requests=%+v", cancel, gateway.requests)
	}
	allocator.onTick(now.Add(5*time.Second + time.Nanosecond))
	if pending := decisions[len(decisions)-1]; pending.Action != "PASSIVE_EXIT_CANCEL_PENDING" || pending.CancelOrderID != 41 || pending.CancelRequestID != cancel.CancelRequestID || len(gateway.requests) != 2 {
		t.Fatalf("pending cancellation was resent or became unobservable: decision=%+v requests=%+v", pending, gateway.requests)
	}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelled, Data: actor.OrderCancelledEvent{OrderID: 41, RequestID: cancel.CancelRequestID, RemainingQty: 75}})
	if len(outcomes) == 0 {
		t.Fatal("deadline cancellation emitted no actor outcome")
	}
	lastOutcome := outcomes[len(outcomes)-1]
	if lastOutcome.Event != "ORDER_CANCELLED" || lastOutcome.RequestID != first.RequestID || lastOutcome.CancelRequestID != cancel.CancelRequestID || lastOutcome.RemainingQty != 75 {
		t.Fatalf("cancellation outcome lost placement/cancel identity: %+v", lastOutcome)
	}

	allocator.onTick(now.Add(6 * time.Second))
	if expired := decisions[len(decisions)-1]; expired.Action != "PASSIVE_EXIT_DEADLINE_EXPIRED" || expired.RequestID != 0 || expired.CancelRequestID != 0 || allocator.perpPosition != -100 || allocator.spotPosition != 100 || allocator.pending != nil {
		t.Fatalf("deadline expiry manufactured an exit or new request: %+v positions=%d/%d pending=%+v", expired, allocator.spotPosition, allocator.perpPosition, allocator.pending)
	}
	if len(gateway.requests) != 2 {
		t.Fatalf("expired passive policy submitted another child: %+v", gateway.requests)
	}
}

func TestTermCarryAllocatorRetriesRejectedPassiveExitCancellation(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	now := time.Unix(10, 0)
	cfg := termCarryTestConfig()
	cfg.MaxPosition, cfg.LotQty, cfg.MinOrderSize = 100, 100, 75
	cfg.PassiveExit = &TermCarryPassiveExitConfig{SliceQty: 75, DeadlineAtNano: now.Add(5 * time.Second).UnixNano()}
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)
	allocator.perp.askQty, allocator.perp.bidQty = 50, 1_000
	allocator.state = termCarryActive
	allocator.plan = &termCarryPlan{direction: 1, planCreatedAt: now.UnixNano(), firstExposureAt: now.UnixNano(), termEnd: now.Add(time.Second).UnixNano()}
	allocator.spotPosition, allocator.perpPosition = 100, -100

	allocator.onTick(now.Add(2 * time.Second))
	entry := decisions[len(decisions)-1]
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: entry.RequestID, OrderID: 41}})
	allocator.onTick(now.Add(5 * time.Second))
	firstCancel := decisions[len(decisions)-1]
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderCancelRejected, Data: actor.OrderCancelRejectedEvent{OrderID: 41, RequestID: firstCancel.CancelRequestID, Reason: exchange.RejectOrderNotFound}})
	allocator.onTick(now.Add(6 * time.Second))
	retry := decisions[len(decisions)-1]
	if retry.Action != "CANCEL_PASSIVE_EXIT_AT_DEADLINE" || retry.CancelOrderID != 41 || retry.CancelRequestID == 0 || retry.CancelRequestID == firstCancel.CancelRequestID || len(gateway.requests) != 3 || gateway.requests[2].CancelReq == nil || gateway.requests[2].CancelReq.RequestID != retry.CancelRequestID {
		t.Fatalf("rejected deadline cancellation did not retry deterministically: first=%+v retry=%+v requests=%+v", firstCancel, retry, gateway.requests)
	}
}

func TestTermCarryPassiveExitConfigRejectsIllegalSlice(t *testing.T) {
	cfg := termCarryTestConfig()
	cfg.MinOrderSize = 75
	cfg.PassiveExit = &TermCarryPassiveExitConfig{SliceQty: 74, DeadlineAtNano: time.Unix(10, 0).UnixNano()}
	if err := cfg.validate(); err == nil {
		t.Fatal("term carry accepted a passive child below its venue minimum")
	}
}

func TestTermCarryAllocatorResetsRejectedFlatEntry(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	cfg := termCarryTestConfig()
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	now := time.Unix(10, 0)
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)

	allocator.onTick(now.Add(time.Second))
	entry := decisions[len(decisions)-1]
	if entry.Action != "SUBMIT_ENTRY_SPOT_IOC" {
		t.Fatalf("entry decision = %+v", entry)
	}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderRejected, Data: actor.OrderRejectedEvent{RequestID: entry.RequestID, Reason: exchange.RejectInsufficientBalance}})
	if allocator.pending != nil || allocator.plan != nil || allocator.state != termCarryIdle {
		t.Fatalf("rejected flat entry retained term state: pending=%+v state=%s plan=%+v", allocator.pending, allocator.state, allocator.plan)
	}
}

func TestTermCarryAllocatorUsesDeclaredMandateNotWorldTermination(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []TermCarryDecision
	cfg := termCarryTestConfig()
	cfg.DecisionObserver = func(decision TermCarryDecision) { decisions = append(decisions, decision) }
	now := time.Unix(10, 0)
	cfg.MandateEndAtNano = now.Add(time.Hour).UnixNano()
	allocator := NewTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	observeTermCarryBooks(t, allocator, gateway, now, 100, 101, 102, 103, 100)

	allocator.onTick(now.Add(time.Second))
	censored := decisions[len(decisions)-1]
	if censored.Action != "TERM_HORIZON_CENSORED" || censored.State != termCarryIdle || censored.MandateEndAt != cfg.MandateEndAtNano || allocator.plan != nil {
		t.Fatalf("declared mandate did not censor infeasible term: decision=%+v plan=%+v", censored, allocator.plan)
	}
}

func TestTermCarryFinancialsUseExactDirectionalTermCost(t *testing.T) {
	cfg := termCarryTestConfig()
	cfg.CommitmentIntervals = 12
	cfg.TakerFeeBps = 5
	cfg.LongSpotFundingBps = 500
	cfg.ShortSpotBorrowBps = 700
	cfg.BalanceSheetBps, cfg.MarginRiskBps, cfg.LegRiskBps = 1, 1, 1
	now := time.Unix(10, 0).UnixNano()
	rate := exchange.FundingRate{Rate: 3, NextFunding: now + int64(8*time.Hour), Interval: int64((8 * time.Hour) / time.Second)}
	positive, ok := termCarryComputeFinancials(cfg, rate, now, 1)
	minimum := new(big.Int).Mul(big.NewInt(cfg.MinNetCarryBps), positive.denominator)
	if !ok || positive.net.Cmp(minimum) <= 0 || positive.financing.Sign() <= 0 || positive.fundingIncome.String() != "36" {
		t.Fatalf("positive term financials = %+v ok=%t", positive, ok)
	}
	negative, ok := termCarryComputeFinancials(cfg, rate, now, -1)
	if !ok || negative.financingDirection != "SHORT_SPOT_ASSET_BORROW" || negative.financing.Cmp(positive.financing) <= 0 || negative.fundingIncome.String() != "-36" {
		t.Fatalf("negative directional financing = %+v positive=%+v ok=%t", negative, positive, ok)
	}
}

func TestTermCarryConfigRequiresExplicitEvidencePath(t *testing.T) {
	policy := termCarryTestConfig()
	policy.MinOrderSize = mvBasePrecision / 1_000
	base := Config{
		LogDir: t.TempDir(), LogMode: "full", TakerFeeBps: policy.TakerFeeBps,
		TermCarryAllocator: &policy, RecordTermCarryDecisions: true,
	}
	if _, err := NewSim(time.Second, base); err == nil {
		t.Fatal("term carry accepted without delayed public receipt evidence")
	}
	base.RecordMarketDataReceipts = true
	base.MarketDataReceiptRoles = []string{"term_carry_allocator"}
	base.LatencyProfiles = map[string]LatencyProfile{"term_carry_allocator": {Model: "constant", Delay: time.Millisecond}}
	base.StrictPopulationAccounting = true
	sim, err := NewSim(time.Second, base)
	if err != nil {
		t.Fatalf("term carry rejected documented receipt path: %v", err)
	}
	defer sim.Close()
	for _, venue := range sim.Venues {
		if len(venue.TermCarryAllocators) != 1 {
			t.Fatalf("venue %s term carry allocators = %d, want 1", venue.ID, len(venue.TermCarryAllocators))
		}
		if venue.TermCarryAllocators[0].cfg.MandateEndAtNano != policy.MandateEndAtNano {
			t.Fatalf("venue %s allocator received an undeclared mandate: got %d want %d", venue.ID, venue.TermCarryAllocators[0].cfg.MandateEndAtNano, policy.MandateEndAtNano)
		}
	}
}

func TestTermCarryConfigBindsItsOrderFloorToTheVenue(t *testing.T) {
	for _, minimum := range []int64{1, mvBasePrecision/1_000 + 1} {
		policy := termCarryTestConfig()
		policy.MinOrderSize = minimum
		base := Config{
			LogDir: t.TempDir(), TakerFeeBps: policy.TakerFeeBps, TermCarryAllocator: &policy,
			LatencyProfiles: map[string]LatencyProfile{"term_carry_allocator": {Model: "constant", Delay: time.Millisecond}},
		}
		if _, err := NewSim(time.Second, base); err == nil {
			t.Fatalf("term carry accepted non-venue order floor %d", minimum)
		}
	}
}

func TestTermCarryConfigRejectsNegativeExplicitUnwindMinimum(t *testing.T) {
	negative := int64(-1)
	policy := termCarryTestConfig()
	policy.UnwindMinOrderSize = &negative
	if err := policy.validate(); err == nil {
		t.Fatal("negative explicit unwind minimum was accepted")
	}
}

func TestTermCarryDecisionV3PersistsExplicitZeroUnwindMinimum(t *testing.T) {
	zero := int64(0)
	v3, err := json.Marshal(TermCarryDecision{PolicyVersion: termCarryPolicyVersionV3, UnwindMinOrderSize: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if string(v3) == "" || !containsJSONField(v3, "unwind_min_order_size", "0") {
		t.Fatalf("v3 explicit zero exit floor was not persisted: %s", v3)
	}
	v2, err := json.Marshal(TermCarryDecision{PolicyVersion: termCarryPolicyVersionV2})
	if err != nil {
		t.Fatal(err)
	}
	if containsJSONField(v2, "unwind_min_order_size", "0") {
		t.Fatalf("legacy v2 emitted an absent v3 policy field: %s", v2)
	}
}

func containsJSONField(encoded []byte, field, value string) bool {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return false
	}
	raw, ok := decoded[field]
	return ok && string(raw) == value
}
