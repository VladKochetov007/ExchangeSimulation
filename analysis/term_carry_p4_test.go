package analysis

import (
	"fmt"
	"testing"
)

func TestP4FinancialsAreIndependentOfActorAttestation(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 3)
	before, ok := p4CandidateFinancials(policy, decision)
	if !ok || !before.eligible {
		t.Fatalf("valid candidate recompute = %+v, %t", before, ok)
	}
	decision.ExpectedFundingBps = "-999999"
	decision.ExecutionFeeBps = "0"
	decision.FinancingBpsNumerator = "0"
	decision.NetCarryBpsNumerator = "999999"
	after, ok := p4CandidateFinancials(policy, decision)
	if !ok || before.funding.Cmp(after.funding) != 0 || before.fees.Cmp(after.fees) != 0 || before.financing.Cmp(after.financing) != 0 || before.net.Cmp(after.net) != 0 {
		t.Fatal("actor-stored arithmetic changed independent recomputation")
	}
	if err := validateTermCarryEntryEconomics(policy, decision); err == nil {
		t.Fatal("forged actor arithmetic was not detected by evidence replay")
	}
}

func TestP4ChainRequiresCanonicalTwoLegExecution(t *testing.T) {
	run := termCarryLifecycleTestRun(t, makeTermCarryLifecycleV4)
	result, err := run.MeasureTermCarryP4Chain()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.DecisionsEvaluated != 1 || len(result.SubmittedCandidates) != 1 {
		t.Fatalf("P4 chain = %+v", result)
	}
	candidate := result.SubmittedCandidates[0]
	if !candidate.SpotAccepted || !candidate.PerpAccepted || candidate.SpotFilledQty != 50 || candidate.PerpFilledQty != 50 || candidate.MatchedExposureQty != 50 || !candidate.ExecutionQualified {
		t.Fatalf("two-leg execution = %+v", candidate)
	}

	mutated := termCarryLifecycleTestRun(t, func(fixture *termCarryLifecycleFixture) {
		makeTermCarryLifecycleV4(fixture)
		fixture.outcomes = append(fixture.outcomes[:3], fixture.outcomes[4:]...)
	})
	broken, err := mutated.MeasureTermCarryP4Chain()
	if err != nil {
		t.Fatal(err)
	}
	if broken.Valid || broken.BaseAudit.Valid {
		t.Fatalf("dropped canonical perpetual fill survived: %+v", broken)
	}
}

func TestP4LinkMutationsCannotBeRescuedByBasis(t *testing.T) {
	a := TermCarryP4Candidate{
		VenueID: "north", DecisionTime: 10, FundingPublishedAt: 9, FundingSequence: 7,
		SpotBid: 99, SpotAsk: 101, PerpBid: 101, PerpAsk: 103, Direction: 1,
		ExpectedFundingBps: "12", Eligible: false,
	}
	b := a
	b.ExpectedFundingBps, b.Eligible = "900", true
	b.TargetSpot, b.TargetPerp = 100_000_000, -100_000_000
	inputs, funding, carry, target := evaluateP4Links(a, b)
	if !inputs || !funding || !carry || !target {
		t.Fatalf("valid link chain = %t/%t/%t/%t", inputs, funding, carry, target)
	}

	mutations := []struct {
		name   string
		mutate func(*TermCarryP4Candidate)
		which  int
	}{
		{"future/different receipt frontier", func(v *TermCarryP4Candidate) { v.FundingPublishedAt++ }, 0},
		{"reversed funding sign", func(v *TermCarryP4Candidate) { v.ExpectedFundingBps = "-900" }, 1},
		{"no exact carry crossing", func(v *TermCarryP4Candidate) { v.Eligible = false }, 2},
		{"target omitted", func(v *TermCarryP4Candidate) { v.TargetSpot, v.TargetPerp = 0, 0 }, 3},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			mutant := b
			tc.mutate(&mutant)
			inputs, funding, carry, target := evaluateP4Links(a, mutant)
			got := []bool{inputs, funding, carry, target}
			if got[tc.which] {
				t.Fatalf("mutation survived: %v", got)
			}
		})
	}
}

func TestP4BasisWindowUsesRegisteredClockCoverageAndDirection(t *testing.T) {
	const second = int64(1_000_000_000)
	t0 := int64(100) * second
	spot := make([]string, 0, 333)
	perp := make([]string, 0, 333)
	for at := t0 - 32*second; at < t0+300*second; at += second {
		spot = append(spot, p4SnapshotLine(at, "north", "ABC/USD", 99, 101))
		perpMid := int64(102)
		if at >= t0 {
			perpMid = 100
		}
		perp = append(perp, p4SnapshotLine(at, "north", "ABC-PERP", perpMid-1, perpMid+1))
	}
	run := writeRunAndOpen(t, map[string][]string{
		"north/spot/ABC-USD.jsonl": spot,
		"north/derivatives.jsonl":  perp,
	})
	window, err := measureP4BasisWindow(run, "north", t0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !window.Measurable || window.PreSamples != 30 || window.PostSamples != 300 || window.PreMeanBps == nil || window.PostMeanBps == nil || window.DeltaBps == nil {
		t.Fatalf("basis window = %+v", window)
	}
	if *window.PreMeanBps != 200 || *window.PostMeanBps != 0 || *window.DeltaBps != -200 {
		t.Fatalf("basis means/delta = %.6f/%.6f/%.6f", *window.PreMeanBps, *window.PostMeanBps, *window.DeltaBps)
	}
	reversed, err := measureP4BasisWindow(run, "north", t0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if reversed.DeltaBps == nil || *reversed.DeltaBps != 200 {
		t.Fatalf("oriented reverse delta = %+v", reversed.DeltaBps)
	}
}

func TestP4BasisWindowFailsClosedForStaleOneSidedAndCensoredEvidence(t *testing.T) {
	const second = int64(1_000_000_000)
	t0 := int64(100) * second
	run := writeRunAndOpen(t, map[string][]string{
		"north/spot/ABC-USD.jsonl": {p4SnapshotLine(t0-3*second, "north", "ABC/USD", 99, 101)},
		"north/derivatives.jsonl":  {p4SnapshotLine(t0-3*second, "north", "ABC-PERP", 101, 103)},
	})
	window, err := measureP4BasisWindow(run, "north", t0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if window.Measurable || window.PreMeanBps != nil || window.PostMeanBps != nil || window.DeltaBps != nil {
		t.Fatalf("stale evidence became numeric basis: %+v", window)
	}
	censored, err := measureP4BasisWindow(run, "north", p4AnalysisCutoff-p4BasisPostNanos+1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !censored.CensoredByCutoff || censored.Measurable {
		t.Fatalf("cutoff mutation survived: %+v", censored)
	}
}

func p4SnapshotLine(ts int64, venue, symbol string, bid, ask int64) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":0,"event":"BookSnapshot","data":{"venue_id":%q,"payload":{"symbol":%q,"bids":[{"price":%d,"visible_qty":10}],"asks":[{"price":%d,"visible_qty":10}]}}}`,
		ts, venue, symbol, bid, ask)
}

func writeRunAndOpen(t *testing.T, logs map[string][]string) *Run {
	t.Helper()
	dir := writeRun(t, Report{}, logs)
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}
