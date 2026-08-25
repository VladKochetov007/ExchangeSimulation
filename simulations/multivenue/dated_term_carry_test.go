package multivenue

import (
	"context"
	"math/big"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func datedCarryTestConfig(trade bool) DatedTermCarryAllocatorConfig {
	return DatedTermCarryAllocatorConfig{
		Enabled: true, TradeEnabled: trade, SpotSymbol: "ABC/USD", TargetTenor: 8 * time.Hour,
		DecisionPeriod: 2 * time.Second, MaxMarketAge: 10 * time.Second, MinTimeToExpiry: 10 * time.Minute,
		TakerFeeBps: 5, LongSpotFundingBps: 500, ShortSpotBorrowBps: 500,
		BalanceSheetBps: 1, MarginRiskBps: 1, LegRiskBps: 1, SettlementMismatchBps: 2, PostSettlementExitBps: 2, MinNetCarryBps: 1,
		MaxPosition: 10, LotQty: 10, MinOrderSize: 1, SpotTick: 1, FutureTick: 1,
		PassiveExitSliceQty: 1, ExitDeadlineAfterSettlement: time.Hour,
		VenueID: "north", Desk: "dated_term_carry_allocator_1", ClientID: 37,
	}
}

func observeDatedCarryWorld(t *testing.T, allocator *DatedTermCarryAllocator, gateway *fundingCarryStubGateway, listed time.Time, spotBid, spotAsk, futureBid, futureAsk int64) *datedCarryContract {
	t.Helper()
	listedNano := listed.UnixNano()
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 12, Ordinal: 1, DeliveredAt: listed.UnixNano()}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventInstrument, Data: actor.InstrumentEvent{
		Timestamp: listed.UnixNano(), SeqNum: 21,
		Announcement: &exchange.InstrumentAnnouncement{Action: "listed", Symbol: "ABC-FUT-8H", InstrumentType: "FUTURE", Underlying: "ABC/USD", ListedNano: &listedNano, ExpiryNano: listed.Add(8 * time.Hour).UnixNano()},
	}})
	bookAt := listed.Add(time.Second)
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 12, Ordinal: 2, DeliveredAt: bookAt.UnixNano()}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC/USD", Timestamp: bookAt.UnixNano(), SeqNum: 22,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: spotBid, VisibleQty: 100}}, Asks: []exchange.PriceLevel{{Price: spotAsk, VisibleQty: 100}}},
	}})
	gateway.frontier = simulation.MarketDataFrontier{LinkID: 12, Ordinal: 3, DeliveredAt: bookAt.UnixNano(), Digest: [16]byte{4}}
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventBookSnapshot, Data: actor.BookSnapshotEvent{
		Symbol: "ABC-FUT-8H", Timestamp: bookAt.UnixNano(), SeqNum: 23,
		Snapshot: &exchange.BookSnapshot{Bids: []exchange.PriceLevel{{Price: futureBid, VisibleQty: 100}}, Asks: []exchange.PriceLevel{{Price: futureAsk, VisibleQty: 100}}},
	}})
	return allocator.contracts["ABC-FUT-8H"]
}

func TestDatedCarryExactRichAndCheapFinancials(t *testing.T) {
	cfg := datedCarryTestConfig(false)
	tte := int64(8 * time.Hour)
	tests := []struct {
		name                   string
		spotBid, spotAsk       int64
		futureBid, futureAsk   int64
		wantDirection          string
		wantSpot, wantFuture   int64
		wantGrossSpread        string
		wantFinancingDirection string
	}{
		{"rich", 9_999, 10_000, 10_100, 10_101, "RICH_FUTURE", 10_000, 10_100, "100", "LONG_SPOT_CASH_FINANCING"},
		{"cheap", 9_999, 10_000, 9_898, 9_899, "CHEAP_FUTURE", 9_999, 9_899, "100", "SHORT_SPOT_ASSET_BORROW"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spot := fundingCarryBook{hasBid: true, bid: tc.spotBid, hasAsk: true, ask: tc.spotAsk}
			future := fundingCarryBook{hasBid: true, bid: tc.futureBid, hasAsk: true, ask: tc.futureAsk}
			financials, ok := datedCarryBestFinancials(cfg, spot, future, tte)
			if !ok {
				t.Fatal("financials unavailable")
			}
			if financials.direction != tc.wantDirection || financials.spotReference != tc.wantSpot || financials.futureReference != tc.wantFuture || financials.grossSpread.String() != tc.wantGrossSpread || financials.financingDirection != tc.wantFinancingDirection {
				t.Fatalf("financials = %+v", financials)
			}
			costs := new(big.Int).Add(financials.fees, financials.financing)
			for _, cost := range []*big.Int{financials.balance, financials.margin, financials.leg, financials.settlement, financials.exit} {
				costs.Add(costs, cost)
			}
			wantNet := new(big.Int).Sub(financials.gross, costs)
			if financials.net.Cmp(wantNet) != 0 || financials.denominator.Sign() <= 0 {
				t.Fatalf("exact cost identity failed: net=%s want=%s denominator=%s", financials.net, wantNet, financials.denominator)
			}
		})
	}
}

func TestDatedCarryShadowIsEligibleWithoutTargetOrOrders(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []DatedTermCarryDecision
	cfg := datedCarryTestConfig(false)
	cfg.DecisionObserver = func(decision DatedTermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewDatedTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	listed := time.Unix(100, 0)
	observeDatedCarryWorld(t, allocator, gateway, listed, 9_999, 10_000, 10_100, 10_101)
	initialRequests := len(gateway.requests)
	allocator.onTick(listed.Add(2 * time.Second))
	decision := decisions[len(decisions)-1]
	if decision.Action != "SHADOW_ELIGIBLE" || decision.TargetSpot != 0 || decision.TargetFuture != 0 || decision.ProposedTargetSpot != 10 || decision.ProposedTargetFuture != -10 || decision.RequestID != 0 {
		t.Fatalf("shadow eligibility = %+v", decision)
	}
	if len(gateway.requests) != initialRequests || allocator.ownedSymbol != "" || allocator.spotPosition != 0 {
		t.Fatalf("shadow mutated execution state: requests=%d/%d owner=%q spot=%d", initialRequests, len(gateway.requests), allocator.ownedSymbol, allocator.spotPosition)
	}
}

func TestDatedCarryActiveUsesOrdinaryNonAtomicLegsAndSettlesAtZero(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []DatedTermCarryDecision
	var outcomes []DatedTermCarryOutcome
	cfg := datedCarryTestConfig(true)
	cfg.DecisionObserver = func(decision DatedTermCarryDecision) { decisions = append(decisions, decision) }
	cfg.OutcomeObserver = func(outcome DatedTermCarryOutcome) { outcomes = append(outcomes, outcome) }
	allocator := NewDatedTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	listed := time.Unix(100, 0)
	contract := observeDatedCarryWorld(t, allocator, gateway, listed, 9_999, 10_000, 10_100, 10_101)

	allocator.onTick(listed.Add(2 * time.Second))
	spotDecision := decisions[len(decisions)-1]
	if spotDecision.Action != "SUBMIT_ENTRY_SPOT_IOC" || spotDecision.Side != "BUY" || spotDecision.TargetChangedAt == 0 || spotDecision.RequestedQty != 10 {
		t.Fatalf("spot entry = %+v", spotDecision)
	}
	acceptAndFillDatedCarry(t, allocator, spotDecision.RequestID, 41, "ABC/USD", exchange.Buy, 10, listed.Add(3*time.Second))
	if contract.futurePosition != 0 || allocator.spotPosition != 10 {
		t.Fatalf("spot fill fabricated future hedge: spot=%d future=%d", allocator.spotPosition, contract.futurePosition)
	}

	allocator.onTick(listed.Add(4 * time.Second))
	futureDecision := decisions[len(decisions)-1]
	if futureDecision.Action != "SUBMIT_ENTRY_FUTURE_IOC" || futureDecision.Side != "SELL" || futureDecision.RequestedQty != 10 {
		t.Fatalf("future hedge = %+v", futureDecision)
	}
	acceptAndFillDatedCarry(t, allocator, futureDecision.RequestID, 42, contract.symbol, exchange.Sell, 10, listed.Add(5*time.Second))
	allocator.onTick(listed.Add(6 * time.Second))
	if active := decisions[len(decisions)-1]; active.Action != "TERM_ACTIVE" || active.State != datedCarryActive || allocator.spotPosition != 10 || contract.futurePosition != -10 {
		t.Fatalf("matched term = decision %+v spot=%d future=%d", active, allocator.spotPosition, contract.futurePosition)
	}

	zero := int64(0)
	settledAt := listed.Add(8 * time.Hour)
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventInstrument, Data: actor.InstrumentEvent{
		Timestamp: settledAt.UnixNano(), SeqNum: 30,
		Announcement: &exchange.InstrumentAnnouncement{Action: "settled", Symbol: contract.symbol, InstrumentType: "FUTURE", Underlying: "ABC/USD", SettlementPrice: &zero},
	}})
	if contract.futurePosition != 0 || contract.state != datedCarryExitSpot || outcomes[len(outcomes)-1].Event != "CONTRACT_SETTLED" || !outcomes[len(outcomes)-1].HasSettlement || outcomes[len(outcomes)-1].SettlementPrice != 0 {
		t.Fatalf("zero settlement = contract %+v outcome %+v", contract, outcomes[len(outcomes)-1])
	}
	allocator.onTick(settledAt.Add(2 * time.Second))
	exit := decisions[len(decisions)-1]
	if exit.Action != "SUBMIT_EXIT_SPOT_IOC" || exit.Side != "SELL" {
		t.Fatalf("post-settlement exit = %+v", exit)
	}
	acceptAndFillDatedCarry(t, allocator, exit.RequestID, 43, "ABC/USD", exchange.Sell, 10, settledAt.Add(3*time.Second))
	allocator.onTick(settledAt.Add(4 * time.Second))
	closed := decisions[len(decisions)-1]
	if closed.Action != "TERM_CLOSED" || closed.State != datedCarryClosed || allocator.spotPosition != 0 || contract.futurePosition != 0 || allocator.ownedSymbol != "" {
		t.Fatalf("closure = decision %+v spot=%d future=%d owner=%q", closed, allocator.spotPosition, contract.futurePosition, allocator.ownedSymbol)
	}
}

func TestDatedCarrySettlementDoesNotErasePendingLeg(t *testing.T) {
	allocator := NewDatedTermCarryAllocator(1, newFundingCarryStubGateway(), datedCarryTestConfig(true))
	allocator.subscribed = true
	listed := time.Unix(100, 0)
	contract := observeDatedCarryWorld(t, allocator, allocator.Gateway().(*fundingCarryStubGateway), listed, 9_999, 10_000, 10_100, 10_101)
	allocator.onTick(listed.Add(2 * time.Second))
	if allocator.pending == nil {
		t.Fatal("entry request not pending")
	}
	zero := int64(0)
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventInstrument, Data: actor.InstrumentEvent{
		Timestamp: contract.expiryAt, SeqNum: 30,
		Announcement: &exchange.InstrumentAnnouncement{Action: "settled", Symbol: contract.symbol, InstrumentType: "FUTURE", Underlying: "ABC/USD", SettlementPrice: &zero},
	}})
	if allocator.pending == nil {
		t.Fatal("settlement erased in-flight leg")
	}
	acceptAndFillDatedCarry(t, allocator, allocator.pending.requestID, 44, "ABC/USD", exchange.Buy, 10, time.Unix(0, contract.expiryAt+1))
	if contract.state != datedCarryExitSpot || allocator.spotPosition != 10 {
		t.Fatalf("late delivered spot fill was not retained as exit residual: state=%s spot=%d", contract.state, allocator.spotPosition)
	}
}

func TestDatedCarryDoesNotSubmitBeyondEvidenceHorizon(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []DatedTermCarryDecision
	listed := time.Unix(100, 0)
	terminal := listed.Add(2 * time.Second)
	cfg := datedCarryTestConfig(true)
	cfg.TerminalNano = terminal.UnixNano()
	cfg.DecisionObserver = func(decision DatedTermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewDatedTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	observeDatedCarryWorld(t, allocator, gateway, listed, 9_999, 10_000, 10_100, 10_101)
	requestsBefore := len(gateway.requests)
	allocator.onTick(terminal)
	got := decisions[len(decisions)-1]
	if got.Action != "SIMULATION_HORIZON_CENSORED" || got.RequestID != 0 || len(gateway.requests) != requestsBefore || got.SpotPosition != 0 || got.FuturePosition != 0 {
		t.Fatalf("terminal evidence = decision %+v requests %d/%d", got, requestsBefore, len(gateway.requests))
	}
}

func TestDatedCarryDoesNotOpenTermWhoseClosureIsCensored(t *testing.T) {
	gateway := newFundingCarryStubGateway()
	var decisions []DatedTermCarryDecision
	listed := time.Unix(100, 0)
	cfg := datedCarryTestConfig(true)
	cfg.TerminalNano = listed.Add(8*time.Hour + 30*time.Minute).UnixNano()
	cfg.DecisionObserver = func(decision DatedTermCarryDecision) { decisions = append(decisions, decision) }
	allocator := NewDatedTermCarryAllocator(1, gateway, cfg)
	allocator.subscribed = true
	observeDatedCarryWorld(t, allocator, gateway, listed, 9_999, 10_000, 10_100, 10_101)
	requestsBefore := len(gateway.requests)
	allocator.onTick(listed.Add(2 * time.Second))
	got := decisions[len(decisions)-1]
	if got.Action != "TERM_HORIZON_CENSORED" || got.RequestID != 0 || len(gateway.requests) != requestsBefore || got.ProposedTargetSpot != 0 {
		t.Fatalf("term horizon evidence = decision %+v requests %d/%d", got, requestsBefore, len(gateway.requests))
	}
}

func acceptAndFillDatedCarry(t *testing.T, allocator *DatedTermCarryAllocator, requestID, orderID uint64, symbol string, side exchange.Side, quantity int64, at time.Time) {
	t.Helper()
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderAccepted, Data: actor.OrderAcceptedEvent{RequestID: requestID, OrderID: orderID}})
	allocator.HandleEvent(context.Background(), &actor.Event{Type: actor.EventOrderFilled, Data: actor.OrderFillEvent{OrderID: orderID, Symbol: symbol, Side: side, Qty: quantity, Price: 10_000, TradeID: orderID, IsFull: true, Timestamp: at.UnixNano()}})
}
