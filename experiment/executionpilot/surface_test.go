package executionpilot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSurfaceKeepsPartialFillAndUndefinedContrasts(t *testing.T) {
	seeds := []int64{1009, 1013, 1019}
	cells, err := Cells(seeds)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]CellResult, 0, len(cells))
	for _, cell := range cells {
		id, err := cellID(cell)
		if err != nil {
			t.Fatal(err)
		}
		outcome := ReconstructedOutcome{Status: OutcomeFullyFilled, TargetQty: cell.TargetQty, TargetShortfallDefined: true}
		switch cell.MakerCount {
		case 4:
			outcome.DeliveredFiveAskQty, outcome.TargetShortfallBps = 100_000_000, 8
		case 6:
			outcome.DeliveredFiveAskQty, outcome.TargetShortfallBps = 150_000_000, 6
		case 2:
			outcome.DeliveredFiveAskQty, outcome.TargetShortfallBps = 50_000_000, 12
		}
		if cell.MakerCount == 4 && cell.TargetQty == 500_000_000 && cell.Seed == 1019 {
			outcome.Status = OutcomePartiallyFilled
		}
		classification, admissibility := classifyOutcome(outcome)
		records = append(records, CellResult{ID: id, Cell: cell, Outcome: outcome,
			Classification: classification, Admissibility: admissibility})
	}
	groups, contrasts, err := summarizeCells(records, seeds)
	if err != nil || len(groups) != 9 || len(contrasts) != 6 {
		t.Fatalf("surface summary: groups=%d contrasts=%d err=%v", len(groups), len(contrasts), err)
	}
	if groups[6].Arm != "C0" || groups[6].TargetQty != 500_000_000 ||
		groups[6].AdmissibleCount != 2 || groups[6].FailedMandateCount != 1 {
		t.Fatalf("partial fill disappeared from all-assigned group: %+v", groups[6])
	}
	for _, contrast := range contrasts {
		if !contrast.DepthDefined || !contrast.ShortfallDefined || !contrast.OpportunitySeparated ||
			len(contrast.DepthDifferences) != 3 || len(contrast.ShortfallDifferencesBps) != 3 {
			t.Fatalf("paired contrast unexpectedly undefined: %+v", contrast)
		}
		if contrast.Arm == "Cp" && contrast.MedianShortfallDifferenceBps != -2 ||
			contrast.Arm == "Cm" && contrast.MedianShortfallDifferenceBps != 4 {
			t.Fatalf("unexpected paired median: %+v", contrast)
		}
	}
	records[0].Outcome = ReconstructedOutcome{Status: OutcomeNoObservedOpportunity}
	records[0].Classification, records[0].Admissibility = classifyOutcome(records[0].Outcome)
	groups, contrasts, err = summarizeCells(records, seeds)
	if err != nil || groups[0].NoOpportunityCount != 1 || contrasts[0].DepthDefined || contrasts[0].ShortfallDefined ||
		contrasts[1].DepthDefined || contrasts[1].ShortfallDefined {
		t.Fatalf("missing opportunity was imputed into matched contrasts: groups=%+v contrasts=%+v err=%v", groups[0], contrasts[:2], err)
	}
	unvalued := ReconstructedOutcome{Status: OutcomeFullyFilled, TargetShortfallDefined: false}
	if classification, admissibility := classifyOutcome(unvalued); classification != "VALID_ACTIVE" || admissibility != "NOT_ASSESSABLE" {
		t.Fatalf("unvalued valid execution misclassified: %s/%s", classification, admissibility)
	}
}

func TestSurfaceLoaderReplaysEvidenceAndRejectsAlteredOutcome(t *testing.T) {
	root := t.TempDir()
	cell := Cell{MakerCount: 4, RandomTakerCount: 8, TargetQty: 200_000_000, Seed: 42}
	id, err := cellID(cell)
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"plans", filepath.Join("runs", id), "analysis"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	identity := fixtureIdentity()
	plan, err := Lock(cell, identity)
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(root, "plans", id+".json")
	if err := writeExclusiveJSON(planPath, plan); err != nil {
		t.Fatal(err)
	}
	_, planRawDigest, err := readLockedPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	world, err := NewWorld(cell)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(root, "runs", id)
	evidencePath := filepath.Join(runDir, EvidenceFilename)
	evidenceFile, err := os.Create(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	recorder := NewRecorder(evidenceFile)
	world.SetEvidenceObserver(recorder.Record)
	reports, err := world.RunMany(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	evidenceIdentity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidenceFile.Close(); err != nil {
		t.Fatal(err)
	}
	actorPath := filepath.Join(runDir, ActorFilename)
	if err := writeExclusiveJSON(actorPath, reports[0]); err != nil {
		t.Fatal(err)
	}
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	actorDigest, err := fileSHA256(actorPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := RunManifest{
		SchemaVersion: 1, PlanRawSHA256: planRawDigest, TypedPlanSHA256: plan.TypedPlanSHA256,
		Identity: identity, Evidence: evidenceIdentity,
		EvidenceFileSHA256: evidenceDigest, ActorFileSHA256: actorDigest,
	}
	if err := writeExclusiveJSON(filepath.Join(runDir, ManifestFilename), manifest); err != nil {
		t.Fatal(err)
	}
	evidenceRaw, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := Reconstruct(bytes.NewReader(evidenceRaw), evidenceIdentity, plan)
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(root, "analysis", id+".json")
	if err := WriteResult(resultPath, outcome, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadCellResult(root, cell, identity)
	if err != nil || loaded.Outcome != outcome || loaded.EvidenceFileSHA256 != evidenceDigest {
		t.Fatalf("valid fixture replay failed: loaded=%+v err=%v", loaded, err)
	}
	resultRaw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(resultRaw, []byte(`"filled_qty": 200000000`), []byte(`"filled_qty": 100000000`), 1)
	if bytes.Equal(tampered, resultRaw) {
		t.Fatal("fixture did not contain expected filled quantity")
	}
	if err := os.WriteFile(resultPath, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCellResult(root, cell, identity); err == nil {
		t.Fatal("tampered stored outcome accepted despite independent raw replay")
	}
}
