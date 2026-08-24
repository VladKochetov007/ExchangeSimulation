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
	if entrySpot.Action != "SUBMIT_ENTRY_SPOT_IOC" || entrySpot.State != termCarryEntrySpot || entrySpot.TermEnd != now.Add(8*time.Hour).UnixNano() || entrySpot.Side != "BUY" || entrySpot.RequestID == 0 {
		t.Fatalf("entry spot decision = %+v", entrySpot)
	}
	acceptAndFillTermCarry(t, allocator, entrySpot.RequestID, 41, "ABC/USD", exchange.Buy, now.Add(2*time.Second))

	allocator.onTick(now.Add(3 * time.Second))
	entryPerp := decisions[len(decisions)-1]
	if entryPerp.Action != "SUBMIT_ENTRY_PERP_IOC" || entryPerp.State != termCarryEntryPerp || entryPerp.Side != "SELL" || entryPerp.TargetSpot != 50 || entryPerp.TargetPerp != -50 {
		t.Fatalf("entry perp decision = %+v", entryPerp)
	}
	acceptAndFillTermCarry(t, allocator, entryPerp.RequestID, 42, "ABC-PERP", exchange.Sell, now.Add(4*time.Second))

	allocator.onTick(now.Add(5 * time.Second))
	if active := decisions[len(decisions)-1]; active.Action != "TERM_ACTIVE" || active.State != termCarryActive || allocator.spotPosition != 50 || allocator.perpPosition != -50 {
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
	allocator.plan = &termCarryPlan{direction: 1, entryAt: now.UnixNano(), termEnd: now.Add(time.Second).UnixNano()}
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
