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

func TestTermCarryLifecycleSameTimestampFillUsesReceiptOrder(t *testing.T) {
	var deadline int64
	run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
		deadline = makeLifecyclePassiveClosure(fixture)
		fixture.splitCanonicalLog = true
		passive := fixture.decisions[3]
		fill := fixture.outcomes[5]
		resting := passive
		resting.DecisionTime = fill.ExecutionTime
		resting.Action, resting.Leg, resting.Side = "PASSIVE_EXIT_RESTING", "", ""
		resting.LimitPrice, resting.RequestedQty, resting.RequestID = 0, 0, 0
		resting.OrderType, resting.TimeInForce, resting.PostOnly = "", "", nil
		fixture.decisions = append(fixture.decisions, resting)
	})

	result, err := run.MeasureTermCarryLifecycle(TermCarryLifecycleOptions{DeadlineAtNano: deadline})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IntegrityValid {
		t.Fatalf("same-timestamp decision-before-fill receipt rejected: %+v", result.IntegrityFailures)
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

func TestTermCarryLifecycleAdversarialEvidence(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*termCarryLifecycleFixture) int64
		wantFailure string
	}{
		{
			name: "dropped fill",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				fixture.outcomes = fixture.outcomes[:len(fixture.outcomes)-1]
				return fixture.decisions[len(fixture.decisions)-1].DecisionTime
			},
			wantFailure: "terminal_spot_position_disagreement",
		},
		{
			name: "duplicated fill",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				fixture.outcomes = append(fixture.outcomes, fixture.outcomes[len(fixture.outcomes)-1])
				return fixture.decisions[len(fixture.decisions)-1].DecisionTime
			},
			wantFailure: "duplicate_canonical_fill",
		},
		{
			name: "partial fill misclassified as close",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				fill := &fixture.outcomes[len(fixture.outcomes)-1]
				fill.Qty, fill.SpotPositionAfter = fixture.policy.MaxPosition/2, fixture.policy.MaxPosition/2
				return fixture.decisions[len(fixture.decisions)-1].DecisionTime
			},
			wantFailure: "term_closed_without_prior_flatness",
		},
		{
			name: "forged early close",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				close := &fixture.decisions[len(fixture.decisions)-1]
				close.DecisionTime = fixture.outcomes[len(fixture.outcomes)-1].ExecutionTime
				return close.DecisionTime
			},
			wantFailure: "term_closed_without_prior_flatness",
		},
		{
			name: "duplicate close",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				duplicate := fixture.decisions[len(fixture.decisions)-1]
				duplicate.DecisionTime++
				fixture.decisions = append(fixture.decisions, duplicate)
				return duplicate.DecisionTime
			},
			wantFailure: "duplicate_term_closed_transition",
		},
		{
			name: "cancellation misclassified as closure",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				cancelled := &fixture.outcomes[len(fixture.outcomes)-1]
				cancelled.Event, cancelled.Qty, cancelled.TradeID = "ORDER_CANCELLED", 0, 0
				cancelled.CancelRequestID, cancelled.RemainingQty = 77, fixture.policy.MaxPosition
				cancelled.SpotPositionAfter = cancelled.SpotPositionBefore
				fixture.terminalSpot = fixture.initialSpot + fixture.policy.MaxPosition
				return fixture.decisions[len(fixture.decisions)-1].DecisionTime
			},
			wantFailure: "term_closed_without_prior_flatness",
		},
		{
			name: "wrong cancellation identity",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV4PassiveResidual(fixture, true)
				cancel := &fixture.decisions[len(fixture.decisions)-1]
				cancel.Action, cancel.CancelOrderID, cancel.CancelRequestID = "CANCEL_PASSIVE_EXIT_AT_DEADLINE", 102, 77
				outcome := termCarryOutcome{
					VenueID: cancel.VenueID, Desk: cancel.Desk, ClientID: cancel.ClientID,
					DecisionTime: fixture.decisions[3].DecisionTime, ExecutionTime: cancel.DecisionTime,
					State: "UNWIND_PERP", Event: "ORDER_CANCELLED", Leg: "UNWIND_PERP_POST_ONLY",
					RequestID: fixture.decisions[3].RequestID, OrderID: 102, CancelRequestID: 78,
					Symbol: fixture.policy.PerpSymbol, RemainingQty: fixture.policy.MinOrderSize,
					SpotPositionBefore: fixture.policy.MaxPosition, SpotPositionAfter: fixture.policy.MaxPosition,
					PerpPositionBefore: -fixture.policy.MaxPosition, PerpPositionAfter: -fixture.policy.MaxPosition,
				}
				fixture.outcomes = append(fixture.outcomes, outcome)
				return fixture.policy.PassiveExit.DeadlineAtNano
			},
			wantFailure: "passive_exit_cancellation_chain_mismatch",
		},
		{
			name: "residual erased at deadline",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV4PassiveResidual(fixture, true)
				expired := &fixture.decisions[len(fixture.decisions)-1]
				expired.SpotPosition, expired.PerpPosition = 0, 0
				return fixture.policy.PassiveExit.DeadlineAtNano
			},
			wantFailure: "actor_decision_position_disagreement",
		},
		{
			name: "missing post-deadline funding",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV4PassiveResidual(fixture, true)
				deadline := fixture.policy.PassiveExit.DeadlineAtNano
				fixture.settlementAt = fixture.decisions[2].DecisionTime + 1
				tail := fixture.decisions[len(fixture.decisions)-1]
				tail.DecisionTime, tail.FundingNextAt = deadline+2, deadline+1
				fixture.decisions = append(fixture.decisions, tail)
				return deadline
			},
			wantFailure: "missing_post_deadline_funding",
		},
		{
			name: "funding attributed after real close",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				fixture.settlementAt = fixture.decisions[len(fixture.decisions)-1].DecisionTime + 1
				return fixture.settlementAt
			},
			wantFailure: "funding_attributed_after_close",
		},
		{
			name: "future receipt use",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				fixture.decisions[2].DecisionTime = 941
				return fixture.decisions[len(fixture.decisions)-1].DecisionTime
			},
			wantFailure: "future_receipt",
		},
		{
			name: "terminal position disagreement",
			mutate: func(fixture *termCarryLifecycleFixture) int64 {
				makeTermCarryLifecycleV2(fixture)
				fixture.terminalPerp = 1
				return fixture.decisions[len(fixture.decisions)-1].DecisionTime
			},
			wantFailure: "terminal_perp_position_disagreement",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var deadline int64
			run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
				deadline = tc.mutate(fixture)
			})
			result, err := run.MeasureTermCarryLifecycle(TermCarryLifecycleOptions{DeadlineAtNano: deadline})
			if err != nil {
				t.Fatal(err)
			}
			if result.IntegrityValid || !hasLifecycleFailure(result, tc.wantFailure) {
				t.Fatalf("mutation survived; want %q, got %+v", tc.wantFailure, result.IntegrityFailures)
			}
		})
	}
}

func hasLifecycleFailure(result *TermCarryLifecycleAudit, failure string) bool {
	for _, row := range result.IntegrityFailures {
		if row.Failure == failure {
			return true
		}
	}
	return false
}
