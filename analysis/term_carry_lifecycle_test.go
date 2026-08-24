package analysis

import (
	"testing"

	"exchange_sim/exchange"
)

func TestTermCarryLifecycleReconstructsClosedLegacyTerm(t *testing.T) {
	var deadline int64
	run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
		makeTermCarryLifecycleV2(fixture)
		deadline = fixture.decisions[len(fixture.decisions)-1].DecisionTime
	})

	result, err := run.MeasureTermCarryLifecycle(TermCarryLifecycleOptions{DeadlineAtNano: deadline})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IntegrityValid {
		t.Fatalf("valid lifecycle rejected: %+v", result.IntegrityFailures)
	}
	if result.SchemaVersion != 1 || result.Arm != "A" || len(result.Terms) != 1 {
		t.Fatalf("unexpected report identity: %+v", result)
	}
	term := result.Terms[0]
	if term.Ownership.Status != LifecycleObserved || term.Activation.Status != LifecycleObserved {
		t.Fatalf("canonical ownership/activation not reconstructed: %+v", term)
	}
	if term.PassiveEligibility.Status != LifecycleNotApplicable || term.PassiveFilledQuantity.Status != LifecycleNotApplicable {
		t.Fatalf("legacy passive endpoints are ambiguous: %+v", term)
	}
	if !term.ProvenClosedByDeadline || term.CloseTransitionCount != 1 || term.Flatness.Status != LifecycleObserved {
		t.Fatalf("strict closure not proven: %+v", term)
	}
	if !term.Conservation.FillChainValid || !term.Conservation.TerminalSpotAgrees || !term.Conservation.TerminalPerpAgrees {
		t.Fatalf("conservation did not close: %+v", term.Conservation)
	}
}

func TestTermCarryLifecycleRequiresRegisteredDeadline(t *testing.T) {
	run := termCarryLifecycleTestRun(t, makeTermCarryLifecycleV2)
	if _, err := run.MeasureTermCarryLifecycle(TermCarryLifecycleOptions{}); err == nil {
		t.Fatal("missing analysis deadline accepted")
	}
}

func TestTermCarryLifecycleReconstructsPassiveClosure(t *testing.T) {
	var deadline int64
	run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
		deadline = makeLifecyclePassiveClosure(fixture)
	})

	result, err := run.MeasureTermCarryLifecycle(TermCarryLifecycleOptions{DeadlineAtNano: deadline})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IntegrityValid {
		t.Fatalf("valid passive lifecycle rejected: %+v", result.IntegrityFailures)
	}
	if result.Arm != "B" || result.Aggregates.EligibleTerms != 1 || result.Aggregates.PassiveAdmittedTerms != 1 {
		t.Fatalf("passive treatment endpoints missing: %+v", result.Aggregates)
	}
	term := result.Terms[0]
	if term.AggressiveEligibility.Status != LifecycleObserved || term.AggressiveEligibility.Eligible {
		t.Fatalf("sub-minimum aggressive condition not reconstructed: %+v", term.AggressiveEligibility)
	}
	if len(term.PassiveOrders) != 2 || term.PassiveFilledQuantity.Quantity != 2*termCarryAuditPolicy().MaxPosition {
		t.Fatalf("passive fills not reconstructed: %+v", term.PassiveOrders)
	}
	if !term.ProvenClosedByDeadline || term.DeadlineState.Status != LifecycleObserved {
		t.Fatalf("passive closure not proven by deadline: %+v", term)
	}
}

func makeLifecyclePassiveClosure(fixture *termCarryLifecycleFixture) int64 {
	fixture.policy.MinOrderSize = fixture.policy.MaxPosition
	makeTermCarryLifecycleV4(fixture)
	deadline := fixture.decisions[2].TermEnd + 10
	fixture.policy.PassiveExit = &termCarryPassiveExitPolicy{SliceQty: fixture.policy.MaxPosition, DeadlineAtNano: deadline}
	for index := range fixture.decisions {
		decision := &fixture.decisions[index]
		slice := fixture.policy.PassiveExit.SliceQty
		decisionDeadline := fixture.policy.PassiveExit.DeadlineAtNano
		decision.PassiveExitSliceQty, decision.PassiveExitDeadlineAtNano = &slice, &decisionDeadline
	}
	makePassive := func(decision *termCarryDecision, leg string) {
		postOnly := true
		decision.Action, decision.Leg = "SUBMIT_"+leg, leg
		decision.OrderType, decision.TimeInForce, decision.PostOnly = exchange.LimitOrder.String(), exchange.GTC.String(), &postOnly
		decision.RequestedQty = fixture.policy.MaxPosition
	}
	perp := &fixture.decisions[3]
	makePassive(perp, "UNWIND_PERP_POST_ONLY")
	perp.LimitPrice, perp.PerpAskQty = perp.PerpBid, fixture.policy.MinOrderSize-1
	spot := &fixture.decisions[4]
	makePassive(spot, "UNWIND_SPOT_POST_ONLY")
	spot.LimitPrice, spot.SpotBidQty = spot.SpotAsk, fixture.policy.MinOrderSize-1

	tail := fixture.decisions[len(fixture.decisions)-1]
	tail.DecisionTime = deadline + 1
	tail.State, tail.Action = "IDLE", "TERM_HORIZON_CENSORED"
	tail.PlanCreatedAt, tail.FirstExposureAt, tail.TermEnd = 0, 0, 0
	fixture.decisions = append(fixture.decisions, tail)
	return deadline
}
