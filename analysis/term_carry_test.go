package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

func termCarryAuditPolicy() termCarryPolicyConfig {
	return termCarryPolicyConfig{
		SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", DecisionPeriod: 1_000_000_000, CommitmentIntervals: 12, MaxFundingAge: 60 * 1_000_000_000,
		TakerFeeBps: 5, LongSpotFundingBps: 500, ShortSpotBorrowBps: 700,
		BalanceSheetBps: 1, MarginRiskBps: 1, LegRiskBps: 1, MinNetCarryBps: 1,
		MaxPosition: 50, LotQty: 50, MinOrderSize: 1, SpotTick: 1, PerpTick: 1,
	}
}

func validTermCarryEntry(t *testing.T, policy termCarryPolicyConfig, rate int64) termCarryDecision {
	t.Helper()
	decision := termCarryDecision{
		VenueID: "north", Desk: "term_carry_allocator_1", ClientID: 9, PolicyVersion: "v2_5_p3_term_carry_v1",
		DecisionTime: 1_000, Enabled: true, Subscribed: true, State: "ENTRY_SPOT", Action: "SUBMIT_ENTRY_SPOT_IOC",
		SpotSymbol: policy.SpotSymbol, PerpSymbol: policy.PerpSymbol, CommitmentIntervals: policy.CommitmentIntervals,
		HasSpotBook: true, SpotPublishedAt: 900, SpotSequence: 11, HasSpotBid: true, SpotBid: 100, SpotBidQty: 50, HasSpotAsk: true, SpotAsk: 101, SpotAskQty: 50,
		HasPerpBook: true, PerpPublishedAt: 900, PerpSequence: 12, HasPerpBid: true, PerpBid: 102, PerpBidQty: 50, HasPerpAsk: true, PerpAsk: 103, PerpAskQty: 50,
		HasFunding: true, FundingRateBps: rate, FundingPublishedAt: 900, FundingSequence: 13, FundingNextAt: 1_000 + 8*60*60*1_000_000_000, FundingIntervalSeconds: 8 * 60 * 60, FundingAgeNanos: 100,
		TargetSpot: policy.MaxPosition, TargetPerp: -policy.MaxPosition, Leg: "ENTRY_SPOT_IOC", Side: "BUY", LimitPrice: 101, RequestedQty: 50, RequestID: 99,
	}
	setTermCarryFinancialEvidence(t, policy, &decision, 1)
	return decision
}

func setTermCarryFinancialEvidence(t *testing.T, policy termCarryPolicyConfig, decision *termCarryDecision, direction int64) {
	t.Helper()
	financials, ok := termCarryAuditFinancials(policy, *decision, direction)
	if !ok {
		t.Fatal("build financials")
	}
	decision.TermEnd = financials.termEnd
	decision.ExpectedFundingBps = financials.funding.String()
	decision.ExecutionFeeBps = financials.fees.String()
	decision.FinancingBpsNumerator = financials.financing.String()
	decision.NetCarryBpsNumerator = financials.net.String()
	decision.RationalDenominator = financials.denominator.String()
	decision.FinancingDirection = financials.direction
}

func TestTermCarryEntryEconomicsRejectsForgedFundingAndFinancing(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 3)
	if err := validateTermCarryEntryEconomics(policy, decision); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
	decision.ExpectedFundingBps = "-36"
	if err := validateTermCarryEntryEconomics(policy, decision); err == nil {
		t.Fatal("reversed funding-sign mutation survived")
	}
	decision = validTermCarryEntry(t, policy, 3)
	decision.FinancingBpsNumerator = "0"
	if err := validateTermCarryEntryEconomics(policy, decision); err == nil {
		t.Fatal("dropped nonzero financing mutation survived")
	}
}

func TestTermCarryEntryEconomicsTreatsPresentZeroFundingAsEconomicInput(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 0)
	decision.Action, decision.RequestID, decision.Leg, decision.Side, decision.RequestedQty = "NET_CARRY_BELOW_MINIMUM", 0, "", "", 0
	if err := validateTermCarryEntryEconomics(policy, decision); err != nil {
		t.Fatalf("present zero funding became invalid/unavailable: %v", err)
	}
	if decision.HasFunding != true || decision.FundingRateBps != 0 {
		t.Fatalf("zero funding presence changed: %+v", decision)
	}
}

func TestTermCarrySubmissionRejectsForgedTouch(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 3)
	if err := validateTermCarrySubmission(policy, decision); err != nil {
		t.Fatalf("valid touch rejected: %v", err)
	}
	decision.LimitPrice = 100
	if err := validateTermCarrySubmission(policy, decision); err == nil {
		t.Fatal("off-touch entry mutation survived")
	}
}

func TestTermCarryV3ExitFloorCannotUndercutVenueMinimum(t *testing.T) {
	zero := int64(0)
	policy := termCarryAuditPolicy()
	policy.LotQty, policy.MinOrderSize, policy.UnwindMinOrderSize = 100, 75, &zero
	decision := validTermCarryEntry(t, policy, 3)
	decision.PolicyVersion, decision.UnwindMinOrderSize = termCarryPolicyV3, &zero

	// The explicit zero is legal as "no additional actor floor", but it cannot
	// undercut the exchange's 75-unit instrument minimum on entry or unwind.
	decision.RequestedQty = 50
	if err := validateTermCarrySubmission(policy, decision); err == nil {
		t.Fatal("entry incorrectly used the explicit unwind minimum")
	}

	decision.Action, decision.State = "SUBMIT_UNWIND_PERP_IOC", "UNWIND_PERP"
	decision.Leg, decision.Side = "UNWIND_PERP_IOC", exchange.Buy.String()
	decision.SpotPosition, decision.PerpPosition = 100, -100
	decision.PerpAskQty = 100
	decision.LimitPrice, decision.RequestedQty, decision.RequestID = decision.PerpAsk, 50, 99
	if err := validateTermCarryPolicyEvidence(policy, decision); err != nil {
		t.Fatalf("valid explicit zero policy rejected: %v", err)
	}
	if err := validateTermCarrySubmission(policy, decision); err == nil {
		t.Fatal("sub-venue-minimum v3 unwind survived")
	}
	decision.RequestedQty = 100
	if err := validateTermCarrySubmission(policy, decision); err != nil {
		t.Fatalf("venue-admissible v3 unwind rejected: %v", err)
	}

	forgedMinimum := int64(75)
	decision.UnwindMinOrderSize = &forgedMinimum
	if err := validateTermCarryPolicyEvidence(policy, decision); err == nil {
		t.Fatal("forged v3 effective exit minimum survived")
	}
	decision.UnwindMinOrderSize = &zero
	decision.RequestedQty = 101
	if err := validateTermCarrySubmission(policy, decision); err == nil {
		t.Fatal("oversized v3 unwind child survived")
	}
}

func TestTermCarryLifecycleReplaysOneFiniteFundingTerm(t *testing.T) {
	run := termCarryLifecycleTestRun(t, nil)
	result, err := run.MeasureTermCarry()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.ActiveTerms != 1 || result.ClosedTerms != 1 || result.OpenTerms != 0 || result.ActiveTermFunding != 1 || result.OutsideTermFunding != 0 || result.PositionContinuityErrors != 0 || result.TerminalPerpMismatches != 0 {
		t.Fatalf("finite term lifecycle replay = %+v", result)
	}
}

func TestTermCarryV2LifecycleUsesCanonicalFirstExposure(t *testing.T) {
	run := termCarryLifecycleTestRun(t, makeTermCarryLifecycleV2)
	result, err := run.MeasureTermCarry()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.ActiveTerms != 1 || result.ClosedTerms != 1 || result.ActiveTermFunding != 1 || result.FirstExposureMismatches != 0 {
		t.Fatalf("v2 first-exposure lifecycle replay = %+v", result)
	}
}

func TestTermCarryV3LifecycleUsesCanonicalFirstExposure(t *testing.T) {
	run := termCarryLifecycleTestRun(t, makeTermCarryLifecycleV3)
	result, err := run.MeasureTermCarry()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.ActiveTerms != 1 || result.ClosedTerms != 1 || result.ActiveTermFunding != 1 || result.FirstExposureMismatches != 0 {
		t.Fatalf("v3 first-exposure lifecycle replay = %+v", result)
	}
}

func TestTermCarryV3LifecycleRejectsForgedExitMinimum(t *testing.T) {
	run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
		makeTermCarryLifecycleV3(fixture)
		forged := int64(1)
		fixture.decisions[3].UnwindMinOrderSize = &forged
	})
	result, err := run.MeasureTermCarry()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.DecisionFieldMismatches == 0 {
		t.Fatalf("forged v3 exit minimum survived: %+v", result)
	}
}

func TestTermCarryV2LifecycleRejectsForgedFirstExposure(t *testing.T) {
	run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
		makeTermCarryLifecycleV2(fixture)
		for index := range fixture.decisions {
			if fixture.decisions[index].FirstExposureAt != 0 {
				fixture.decisions[index].FirstExposureAt++
			}
		}
	})
	result, err := run.MeasureTermCarry()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || result.FirstExposureMismatches == 0 {
		t.Fatalf("forged first exposure survived: %+v", result)
	}
}

func makeTermCarryLifecycleV2(fixture *termCarryLifecycleFixture) {
	firstExposureAt := fixture.outcomes[1].ExecutionTime
	planCreatedAt := fixture.decisions[0].DecisionTime
	for index := range fixture.decisions {
		decision := &fixture.decisions[index]
		decision.PolicyVersion = termCarryPolicyV2
		decision.EntryAt = 0
		decision.PlanCreatedAt = planCreatedAt
		decision.FirstExposureAt = firstExposureAt
	}
	fixture.decisions[0].FirstExposureAt = 0
}

func makeTermCarryLifecycleV3(fixture *termCarryLifecycleFixture) {
	zero := int64(0)
	fixture.policy.UnwindMinOrderSize = &zero
	makeTermCarryLifecycleV2(fixture)
	for index := range fixture.decisions {
		fixture.decisions[index].PolicyVersion = termCarryPolicyV3
		fixture.decisions[index].UnwindMinOrderSize = &zero
	}
}

func TestTermCarryLifecycleDistinguishesProjectionFromOwnership(t *testing.T) {
	t.Run("net carry defer retains a non-owned projected end", func(t *testing.T) {
		run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
			deferred := validTermCarryEntry(t, fixture.policy, 0)
			deferred.DecisionTime = 999
			deferred.FundingAgeNanos = 99
			setTermCarryFinancialEvidence(t, fixture.policy, &deferred, 1)
			deferred.EntryAt = 0
			deferred.State, deferred.Action = "IDLE", "NET_CARRY_BELOW_MINIMUM"
			deferred.TargetSpot, deferred.TargetPerp = 0, 0
			deferred.Leg, deferred.Side, deferred.RequestID, deferred.RequestedQty = "", "", 0, 0
			fixture.decisions = append(fixture.decisions, deferred)
		})
		result, err := run.MeasureTermCarry()
		if err != nil {
			t.Fatal(err)
		}
		if !result.Valid || result.LifecycleViolations != 0 {
			t.Fatalf("non-owned projected term was treated as invalid: %+v", result)
		}
	})

	t.Run("unexpected projected end is rejected", func(t *testing.T) {
		run := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
			deferred := validTermCarryEntry(t, fixture.policy, 0)
			deferred.DecisionTime = 999
			deferred.FundingAgeNanos = 99
			setTermCarryFinancialEvidence(t, fixture.policy, &deferred, 1)
			deferred.EntryAt = 0
			deferred.State, deferred.Action = "IDLE", "ZERO_PREMIUM"
			deferred.TargetSpot, deferred.TargetPerp = 0, 0
			deferred.Leg, deferred.Side, deferred.RequestID, deferred.RequestedQty = "", "", 0, 0
			fixture.decisions = append(fixture.decisions, deferred)
		})
		result, err := run.MeasureTermCarry()
		if err != nil {
			t.Fatal(err)
		}
		if result.Valid || result.LifecycleViolations == 0 {
			t.Fatalf("unexpected projected term survived: %+v", result)
		}
	})
}

func TestTermCarryLifecycleMutationsAreDetected(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*termCarryLifecycleFixture)
		check  func(*TermCarryAudit) bool
	}{
		{
			name: "missing eventual close remains observable",
			mutate: func(fixture *termCarryLifecycleFixture) {
				fixture.decisions = fixture.decisions[:len(fixture.decisions)-1]
			},
			check: func(result *TermCarryAudit) bool { return result.OpenTerms == 1 && result.ClosedTerms == 0 },
		},
		{
			name: "duplicate close is invalid",
			mutate: func(fixture *termCarryLifecycleFixture) {
				duplicate := fixture.decisions[len(fixture.decisions)-1]
				duplicate.DecisionTime++
				fixture.decisions = append(fixture.decisions, duplicate)
			},
			check: func(result *TermCarryAudit) bool { return !result.Valid && result.LifecycleViolations > 0 },
		},
		{
			name: "funding outside active term is invalid",
			mutate: func(fixture *termCarryLifecycleFixture) {
				fixture.settlementAt = fixture.decisions[1].DecisionTime
			},
			check: func(result *TermCarryAudit) bool { return !result.Valid && result.OutsideTermFunding == 1 },
		},
		{
			name: "dropped unwind fill breaks position continuity",
			mutate: func(fixture *termCarryLifecycleFixture) {
				fixture.outcomes = append(fixture.outcomes[:5], fixture.outcomes[6:]...)
			},
			check: func(result *TermCarryAudit) bool { return !result.Valid && result.PositionContinuityErrors > 0 },
		},
		{
			name: "terminal spot balance must match filled inventory",
			mutate: func(fixture *termCarryLifecycleFixture) {
				fixture.terminalSpot++
			},
			check: func(result *TermCarryAudit) bool { return !result.Valid && result.TerminalSpotMismatches == 1 },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := termCarryLifecycleTestRun(t, tc.mutate)
			result, err := run.MeasureTermCarry()
			if err != nil {
				t.Fatal(err)
			}
			if !tc.check(result) {
				t.Fatalf("mutation survived: %+v", result)
			}
		})
	}
}

type termCarryLifecycleFixture struct {
	policy       termCarryPolicyConfig
	decisions    []termCarryDecision
	outcomes     []termCarryOutcome
	settlementAt int64
	initialSpot  int64
	terminalSpot int64
}

// termCarryLifecycleTestRun creates a retained-evidence term without calling
// the allocator. The independent analyzer must reconstruct its own state
// machine, including a funding settlement that occurs while the pair is held.
func termCarryLifecycleTestRun(t *testing.T, mutate func(*termCarryLifecycleFixture)) *Run {
	t.Helper()
	dir := t.TempDir()
	policy := termCarryAuditPolicy()
	entry := validTermCarryEntry(t, policy, 3)
	entry.EntryAt = entry.DecisionTime
	entry.TermEnd = mustTermCarryEnd(t, policy, entry)
	entry.RequestID = 42
	entry.LimitPrice, entry.RequestedQty = entry.SpotAsk, policy.MaxPosition

	entryPerp := entry
	entryPerp.DecisionTime += 2
	entryPerp.State, entryPerp.Action = "ENTRY_PERP", "SUBMIT_ENTRY_PERP_IOC"
	entryPerp.SpotPosition, entryPerp.PerpPosition = policy.MaxPosition, 0
	entryPerp.Leg, entryPerp.Side, entryPerp.LimitPrice, entryPerp.RequestID = "ENTRY_PERP_IOC", exchange.Sell.String(), entryPerp.PerpBid, 43

	active := entry
	active.DecisionTime += 4
	active.State, active.Action, active.Leg, active.Side, active.RequestID, active.RequestedQty = "ACTIVE_TERM", "TERM_ACTIVE", "", "", 0, 0
	active.SpotPosition, active.PerpPosition = policy.MaxPosition, -policy.MaxPosition

	unwindPerp := active
	unwindPerp.DecisionTime = entry.TermEnd + 1
	unwindPerp.State, unwindPerp.Action = "UNWIND_PERP", "SUBMIT_UNWIND_PERP_IOC"
	unwindPerp.TargetSpot, unwindPerp.TargetPerp = 0, 0
	unwindPerp.Leg, unwindPerp.Side, unwindPerp.LimitPrice, unwindPerp.RequestedQty, unwindPerp.RequestID = "UNWIND_PERP_IOC", exchange.Buy.String(), unwindPerp.PerpAsk, policy.MaxPosition, 44

	unwindSpot := unwindPerp
	unwindSpot.DecisionTime += 2
	unwindSpot.State, unwindSpot.Action = "UNWIND_SPOT", "SUBMIT_UNWIND_SPOT_IOC"
	unwindSpot.PerpPosition = 0
	unwindSpot.Leg, unwindSpot.Side, unwindSpot.LimitPrice, unwindSpot.RequestID = "UNWIND_SPOT_IOC", exchange.Sell.String(), unwindSpot.SpotBid, 45

	closed := unwindSpot
	closed.DecisionTime += 2
	closed.State, closed.Action, closed.Leg, closed.Side, closed.RequestID, closed.RequestedQty = "IDLE", "TERM_CLOSED", "", "", 0, 0
	closed.SpotPosition, closed.PerpPosition = 0, 0

	fixture := &termCarryLifecycleFixture{
		policy:    policy,
		decisions: []termCarryDecision{entry, entryPerp, active, unwindPerp, unwindSpot, closed},
		outcomes: []termCarryOutcome{
			termCarryAccepted(entry, 100, 0, 0), termCarryFilled(entry, 100, 0, 0, policy.MaxPosition, 0),
			termCarryAccepted(entryPerp, 101, policy.MaxPosition, 0), termCarryFilled(entryPerp, 101, policy.MaxPosition, 0, policy.MaxPosition, -policy.MaxPosition),
			termCarryAccepted(unwindPerp, 102, policy.MaxPosition, -policy.MaxPosition), termCarryFilled(unwindPerp, 102, policy.MaxPosition, -policy.MaxPosition, policy.MaxPosition, 0),
			termCarryAccepted(unwindSpot, 103, policy.MaxPosition, 0), termCarryFilled(unwindSpot, 103, policy.MaxPosition, 0, 0, 0),
		},
		settlementAt: active.DecisionTime + 1,
		initialSpot:  1_000,
		terminalSpot: 1_000,
	}
	if mutate != nil {
		mutate(fixture)
	}
	writeTermCarryLifecycleManifest(t, dir, fixture)
	writeTermCarryLifecycleReceipts(t, dir, fixture.decisions)
	lines := make([]string, 0, len(fixture.decisions)+2*len(fixture.outcomes)+1)
	for _, decision := range fixture.decisions {
		lines = append(lines, logLine(decision.DecisionTime, decision.ClientID, "term_carry_decision", mustFundingCarryMap(t, decision)))
	}
	for _, outcome := range fixture.outcomes {
		lines = append(lines, logLine(termCarryOutcomeTimestamp(outcome), outcome.ClientID, "term_carry_leg_outcome", mustFundingCarryMap(t, outcome)))
		if outcome.Event == "ORDER_ACCEPTED" {
			order := fundingCarryVenueOrder{RequestID: outcome.RequestID, OrderID: outcome.OrderID, Side: termCarryOutcomeSide(fixture.decisions, outcome.RequestID), Type: exchange.LimitOrder.String(), TimeInForce: exchange.IOC.String(), Price: termCarryOutcomePrice(fixture.decisions, outcome.RequestID), Qty: termCarryOutcomeQty(fixture.decisions, outcome.RequestID)}
			lines = append(lines, logLine(termCarryOutcomeTimestamp(outcome), outcome.ClientID, "OrderAccepted", mustFundingCarryMap(t, order)))
		}
		if outcome.Event == "ORDER_FILL" {
			fill := fundingCarryVenueFill{OrderID: outcome.OrderID, TradeID: outcome.TradeID, Symbol: outcome.Symbol, Side: outcome.Side, Qty: outcome.Qty, Price: outcome.Price, FeeAmount: outcome.FeeAmount, FeeAsset: outcome.FeeAsset}
			lines = append(lines, logLine(outcome.ExecutionTime, outcome.ClientID, "OrderFill", mustFundingCarryMap(t, fill)))
		}
	}
	lines = append(lines, fundingPayLine(fixture.settlementAt, "north", entry.ClientID, 1))
	writeFundingCarryLog(t, dir, lines)
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func mustTermCarryEnd(t *testing.T, policy termCarryPolicyConfig, decision termCarryDecision) int64 {
	t.Helper()
	financials, ok := termCarryAuditFinancials(policy, decision, 1)
	if !ok {
		t.Fatal("term financials unavailable")
	}
	return financials.termEnd
}

func termCarryAccepted(decision termCarryDecision, orderID uint64, spotBefore, perpBefore int64) termCarryOutcome {
	return termCarryOutcome{VenueID: decision.VenueID, Desk: decision.Desk, ClientID: decision.ClientID, DecisionTime: decision.DecisionTime, State: decision.State, Event: "ORDER_ACCEPTED", Leg: decision.Leg, RequestID: decision.RequestID, OrderID: orderID, Symbol: termCarryAuditSymbol(decision), SpotPositionBefore: spotBefore, SpotPositionAfter: spotBefore, PerpPositionBefore: perpBefore, PerpPositionAfter: perpBefore}
}

func termCarryFilled(decision termCarryDecision, orderID uint64, spotBefore, perpBefore, spotAfter, perpAfter int64) termCarryOutcome {
	return termCarryOutcome{VenueID: decision.VenueID, Desk: decision.Desk, ClientID: decision.ClientID, DecisionTime: decision.DecisionTime, ExecutionTime: decision.DecisionTime + 1, State: decision.State, Event: "ORDER_FILL", Leg: decision.Leg, RequestID: decision.RequestID, OrderID: orderID, TradeID: orderID, Symbol: termCarryAuditSymbol(decision), Side: decision.Side, Qty: decision.RequestedQty, Price: decision.LimitPrice, FeeAsset: "USD", SpotPositionBefore: spotBefore, SpotPositionAfter: spotAfter, PerpPositionBefore: perpBefore, PerpPositionAfter: perpAfter}
}

func termCarryOutcomeTimestamp(outcome termCarryOutcome) int64 {
	if outcome.ExecutionTime != 0 {
		return outcome.ExecutionTime
	}
	return outcome.DecisionTime
}

func termCarryOutcomeDecision(decisions []termCarryDecision, requestID uint64) termCarryDecision {
	for _, decision := range decisions {
		if decision.RequestID == requestID {
			return decision
		}
	}
	return termCarryDecision{}
}

func termCarryOutcomeSide(decisions []termCarryDecision, requestID uint64) string {
	return termCarryOutcomeDecision(decisions, requestID).Side
}

func termCarryOutcomePrice(decisions []termCarryDecision, requestID uint64) int64 {
	return termCarryOutcomeDecision(decisions, requestID).LimitPrice
}

func termCarryOutcomeQty(decisions []termCarryDecision, requestID uint64) int64 {
	return termCarryOutcomeDecision(decisions, requestID).RequestedQty
}

func writeTermCarryLifecycleManifest(t *testing.T, dir string, fixture *termCarryLifecycleFixture) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"config": map[string]any{"taker_fee_bps": fixture.policy.TakerFeeBps, "term_carry_allocator": fixture.policy},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	report := Report{
		InitialAccounts: []AccountRow{{VenueID: "north", ClientID: 9, Role: "term_carry_allocator_1", Account: Account{SpotBalances: []Balance{{Asset: "ABC", NetAsset: fixture.initialSpot}}}}},
		TerminalAccounts: []AccountRow{{VenueID: "north", ClientID: 9, Role: "term_carry_allocator_1", Account: Account{
			SpotBalances: []Balance{{Asset: "ABC", NetAsset: fixture.terminalSpot}},
			Positions:    []Position{{Symbol: fixture.policy.PerpSymbol, Size: 0}},
		}}},
	}
	raw, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTermCarryLifecycleReceipts(t *testing.T, dir string, decisions []termCarryDecision) {
	t.Helper()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/term_carry_allocator/client/9"
	recorder.RegisterLink("north", link, "term_carry_allocator")
	schedules := []simulation.MarketDataSchedule{
		{ClientID: 9, SourceVenue: "north", Link: link, Symbol: "ABC/USD", Type: exchange.MDSnapshot, Sequence: 11, Fingerprint: [16]byte{1}, PublishedAt: 900, ScheduledAt: 910, LinkOrdinal: 1},
		{ClientID: 9, SourceVenue: "north", Link: link, Symbol: "ABC-PERP", Type: exchange.MDSnapshot, Sequence: 12, Fingerprint: [16]byte{2}, PublishedAt: 900, ScheduledAt: 920, LinkOrdinal: 2},
		{ClientID: 9, SourceVenue: "north", Link: link, Symbol: "ABC-PERP", Type: exchange.MDFunding, Sequence: 13, Fingerprint: [16]byte{3}, PublishedAt: 900, ScheduledAt: 930, LinkOrdinal: 3},
	}
	var frontier simulation.MarketDataFrontier
	for index, schedule := range schedules {
		recorder.RecordSchedule(schedule)
		frontier = recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: int64(940 + index)})
	}
	for _, decision := range decisions {
		if !termCarrySubmission(decision.Action) {
			continue
		}
		recorder.RecordDecision(simulation.MarketDataDecision{ClientID: decision.ClientID, SourceVenue: "north", Link: link, Symbol: termCarryAuditSymbol(decision), RequestID: decision.RequestID, Side: exchangeSide(decision.Side), OrderType: exchange.LimitOrder, TimeInForce: exchange.IOC, Price: decision.LimitPrice, Qty: decision.RequestedQty, DecisionAt: decision.DecisionTime, Frontier: frontier})
	}
	if err := recorder.Finalize(1_000); err != nil {
		t.Fatal(err)
	}
	for index := range decisions {
		decisions[index].DecisionFrontierLinkID = frontier.LinkID
		decisions[index].DecisionFrontierOrdinal = frontier.Ordinal
		decisions[index].DecisionFrontierDeliveredAt = frontier.DeliveredAt
		decisions[index].DecisionFrontierDigest = fmt.Sprintf("%x", frontier.Digest)
	}
}
