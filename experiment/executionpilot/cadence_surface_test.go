package executionpilot

import (
	"math"
	"testing"
)

func TestCadenceContrastKeepsMixedSigns(t *testing.T) {
	contrast := cadenceContrast(13001, [4]float64{1, 0.6, 0.8, 0.9})
	if contrast.PollAtN1 != -0.4 || math.Abs(contrast.PollAtN90-0.1) > 1e-12 ||
		math.Abs(contrast.NetworkAtP1+0.2) > 1e-12 ||
		math.Abs(contrast.NetworkAtP80-0.3) > 1e-12 ||
		math.Abs(contrast.Interaction-0.5) > 1e-12 {
		t.Fatalf("wrong registered contrasts: %+v", contrast)
	}
}

func TestCadenceContrastsRequireEveryAssignedArm(t *testing.T) {
	cells := make([]CadenceCellResult, 0, 12)
	for _, cell := range CadenceCells() {
		id, err := cadenceCellID(cell)
		if err != nil {
			t.Fatal(err)
		}
		cells = append(cells, CadenceCellResult{ID: id, Cell: cell, FilledFraction: 0.5})
	}
	contrasts, ranges, err := cadenceContrasts(cells)
	if err != nil || len(contrasts) != 3 || len(ranges) != 5 {
		t.Fatalf("complete matrix rejected: %v", err)
	}
	if _, _, err := cadenceContrasts(cells[:11]); err == nil {
		t.Fatal("missing assigned cell accepted")
	}
	cells[11] = cells[10]
	if _, _, err := cadenceContrasts(cells); err == nil {
		t.Fatal("duplicate cell accepted")
	}
}

func TestCadenceCorrectedViewOmitsSampledLifetimeRatio(t *testing.T) {
	ratio := 0.505
	original := CadenceReconstruction{Outcome: ReconstructedOutcome{
		SelectedOpportunity: &ObservedOpportunity{
			SelectedSnapshotSeq: 722, QualifyingAtPublication: true,
			Complete: true, DurationNanos: 200_000_000,
			ActionDelayOverDuration: &ratio,
		},
	}}
	reported, sampled := cadenceReportedReconstruction(original)
	if reported.Outcome.SelectedOpportunity != nil || sampled == nil ||
		sampled.SnapshotSeq != 722 || !sampled.QualifyingAtPublication {
		t.Fatalf("incorrect corrected projection: %+v %+v", reported, sampled)
	}
	if original.Outcome.SelectedOpportunity.ActionDelayOverDuration == nil {
		t.Fatal("corrected projection mutated original analysis")
	}
}

func TestCadenceStoredResultRejectsChangedEndpointAndIdentity(t *testing.T) {
	cell := CadenceCell{NetworkLatencyNanos: 1_000_000,
		PollIntervalNanos: 1_000_000, TargetQty: 500_000_000, Seed: 13001}
	reconstruction := CadenceReconstruction{Outcome: ReconstructedOutcome{FilledQty: 250_000_000}}
	stored := cadenceStoredResult{SchemaVersion: 1, Cell: cell,
		ManifestTypedSHA256: "digest", FilledFraction: 0.5, Reconstruction: reconstruction}
	if err := validateCadenceStoredResult(stored, cell, "digest", reconstruction); err != nil {
		t.Fatal(err)
	}
	stored.FilledFraction = 1
	if err := validateCadenceStoredResult(stored, cell, "digest", reconstruction); err == nil {
		t.Fatal("altered filled fraction accepted")
	}
	stored.FilledFraction = 0.5
	stored.ManifestTypedSHA256 = "other"
	if err := validateCadenceStoredResult(stored, cell, "digest", reconstruction); err == nil {
		t.Fatal("altered manifest identity accepted")
	}
}
