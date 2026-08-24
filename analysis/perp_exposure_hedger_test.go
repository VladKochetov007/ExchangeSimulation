package analysis

import (
	"math/rand"
	"testing"
)

func TestPerpExposureDecisionReplayRejectsReversedTargetAndOffTouch(t *testing.T) {
	policy := &perpExposurePolicyConfig{
		Enabled: true, Symbol: "ABC-PERP", DecisionInterval: 2, ExposureInterval: 10,
		ExposureStepQty: 10, MaxAbsExposure: 50, MaxRequestQty: 10, TickSize: 1,
		InitialQuoteBalance: 1, InitialMargin: 1,
	}
	seed := perpExposureFlowSeed(101, 0, 0, 16)
	state := &perpExposureReplayState{rng: rand.New(rand.NewSource(seed)), seenFirst: true, lastTick: 10}
	preview := &perpExposureReplayState{rng: rand.New(rand.NewSource(seed))}
	step, physical, ok := perpExposureNextStep(preview, policy)
	if !ok {
		t.Fatal("test policy has no first state transition")
	}
	target := -physical
	side, price := "SELL", int64(100)
	if target > 0 {
		side, price = "BUY", 101
	}
	decision := perpExposureDecision{
		VenueID: "north", Hedger: "perp_exposure_hedger_1", ClientID: 7,
		PolicyVersion: perpExposurePolicyVersion, Symbol: "ABC-PERP", DecisionTime: 12,
		Enabled: true, Subscribed: true, Action: "SUBMIT_IOC",
		PhysicalBefore: 0, PhysicalAfter: physical, PhysicalStep: step, PhysicalExposureLimit: 50,
		FilledPerpPosition: 0, TargetPerpPosition: target, HedgeGap: target,
		DecisionInterval: 2, ExposureInterval: 10,
		HasSnapshot: true, BookPublishedAt: 11, BookSequence: 1, BookFingerprint: "00000000000000000000000000000001",
		HasBid: true, BidPrice: 100, BidVisibleQty: 10,
		HasAsk: true, AskPrice: 101, AskVisibleQty: 10,
		Side: side, LimitPrice: price, RequestedQty: 10, RequestID: 1,
		TakerFeeBps: 5, OutcomeExpectation: "VENUE_OUTCOME_REQUIRED",
	}
	valid, updated, submitted := validatePerpExposureDecision(decision, state, policy, 5, 100)
	if !valid || !updated || !submitted {
		t.Fatalf("valid P2 decision rejected: valid=%t updated=%t submitted=%t", valid, updated, submitted)
	}

	state = &perpExposureReplayState{rng: rand.New(rand.NewSource(seed)), seenFirst: true, lastTick: 10}
	decision.TargetPerpPosition = -target
	if valid, _, _ := validatePerpExposureDecision(decision, state, policy, 5, 100); valid {
		t.Fatal("reversed target sign survived independent replay")
	}

	state = &perpExposureReplayState{rng: rand.New(rand.NewSource(seed)), seenFirst: true, lastTick: 10}
	decision.TargetPerpPosition = target
	decision.LimitPrice++
	if valid, _, _ := validatePerpExposureDecision(decision, state, policy, 5, 100); valid {
		t.Fatal("off-touch price survived independent replay")
	}
}

func TestPerpExposureFeeAndCounterpartyContracts(t *testing.T) {
	if fee, ok := perpExposureFee(100_000_000, 100_000_000, 5); !ok || fee != 50_000 {
		t.Fatalf("ordinary positive fee = %d, %t", fee, ok)
	}
	if _, ok := perpExposureFee(1, 0, 5); ok {
		t.Fatal("zero positive-domain perpetual price accepted by fee audit")
	}
	key := perpExposureOrderKey{venue: "north", order: 7}
	orders := map[perpExposureOrderKey]perpExposureOrder{
		key:                        {ClientID: 3},
		{venue: "north", order: 8}: {ClientID: 3},
	}
	fill := perpExposureFill{TradeID: 9, Price: 100, Qty: 1}
	trades := []perpExposureTrade{{TradeID: 9, Price: 100, Qty: 1, TakerOrderID: 7, MakerOrderID: 8}}
	if !perpExposureSelfFill(trades, orders, key, fill, 3) {
		t.Fatal("same-client counterparty survived self-fill detector")
	}
	if !perpExposureHasExternalCounterparty(trades, orders, key, fill) {
		t.Fatal("recorded counterparty was not recognized")
	}
	delete(orders, perpExposureOrderKey{venue: "north", order: 8})
	if perpExposureHasExternalCounterparty(trades, orders, key, fill) {
		t.Fatal("unrecorded counterparty was accepted")
	}
}
