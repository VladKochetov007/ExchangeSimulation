package executionpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	EvidenceFilename = "evidence.evs"
	ManifestFilename = "run-manifest.json"
	ActorFilename    = "actor-report.json"
)

type RunManifest struct {
	SchemaVersion      int              `json:"schema_version"`
	PlanRawSHA256      string           `json:"plan_raw_sha256"`
	TypedPlanSHA256    string           `json:"typed_plan_sha256"`
	Identity           Identity         `json:"identity"`
	Evidence           EvidenceIdentity `json:"evidence"`
	EvidenceFileSHA256 string           `json:"evidence_file_sha256"`
	ActorFileSHA256    string           `json:"actor_file_sha256"`
}

func WritePlan(repositoryDir, analyzerBinary, path string, cell Cell) error {
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, EvidenceSchemaID)
	if err != nil {
		return err
	}
	plan, err := Lock(cell, identity)
	if err != nil {
		return err
	}
	return writeExclusiveJSON(path, plan)
}

func readLockedPlan(path string) (LockedPlan, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LockedPlan{}, "", err
	}
	return DecodePlan(raw)
}

func RunFromPlan(ctx context.Context, repositoryDir, analyzerBinary, planPath, outputDir string) (RunManifest, error) {
	plan, rawDigest, err := readLockedPlan(planPath)
	if err != nil {
		return RunManifest{}, err
	}
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, EvidenceSchemaID)
	if err != nil {
		return RunManifest{}, err
	}
	world, err := Verify(plan, identity)
	if err != nil {
		return RunManifest{}, err
	}
	if err := os.Mkdir(outputDir, 0700); err != nil {
		return RunManifest{}, fmt.Errorf("execution pilot: fresh output directory required: %w", err)
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceFile, err := os.OpenFile(evidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return RunManifest{}, err
	}
	recorder := NewRecorder(evidenceFile)
	world.SetEvidenceObserver(recorder.Record)
	reports, runErr := world.RunMany(ctx)
	var evidenceIdentity EvidenceIdentity
	if runErr == nil {
		evidenceIdentity, runErr = recorder.Finish()
	}
	if syncErr := evidenceFile.Sync(); runErr == nil && syncErr != nil {
		runErr = syncErr
	}
	if closeErr := evidenceFile.Close(); runErr == nil && closeErr != nil {
		runErr = closeErr
	}
	if runErr != nil {
		return RunManifest{}, fmt.Errorf("execution pilot: incomplete world, preserve output as failed attempt: %w", runErr)
	}
	if len(reports) != 1 {
		return RunManifest{}, errors.New("execution pilot: unexpected parent report count")
	}
	actorPath := filepath.Join(outputDir, ActorFilename)
	if err := writeExclusiveJSON(actorPath, reports[0]); err != nil {
		return RunManifest{}, err
	}
	evidenceFileDigest, err := fileSHA256(evidencePath)
	if err != nil {
		return RunManifest{}, err
	}
	actorDigest, err := fileSHA256(actorPath)
	if err != nil {
		return RunManifest{}, err
	}
	manifest := RunManifest{
		SchemaVersion: 1, PlanRawSHA256: rawDigest, TypedPlanSHA256: plan.TypedPlanSHA256,
		Identity: identity, Evidence: evidenceIdentity,
		EvidenceFileSHA256: evidenceFileDigest, ActorFileSHA256: actorDigest,
	}
	if err := writeExclusiveJSON(filepath.Join(outputDir, ManifestFilename), manifest); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

func AnalyzeRun(repositoryDir, simulatorBinary, planPath, outputDir string) (ReconstructedOutcome, RunManifest, error) {
	plan, rawDigest, err := readLockedPlan(planPath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	analyzerBinary, err := os.Executable()
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	identity, err := ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, EvidenceSchemaID)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if plan.Identity != identity {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("execution pilot: analyzer identity differs from locked plan")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(outputDir, ManifestFilename))
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	var manifest RunManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if manifest.SchemaVersion != 1 || manifest.Identity != identity ||
		manifest.PlanRawSHA256 != rawDigest || manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 ||
		manifest.Evidence.SchemaID != EvidenceSchemaID {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("execution pilot: run manifest identity mismatch")
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("execution pilot: evidence file digest mismatch")
	}
	actorPath := filepath.Join(outputDir, ActorFilename)
	actorDigest, err := fileSHA256(actorPath)
	if err != nil || actorDigest != manifest.ActorFileSHA256 {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("execution pilot: actor report file digest mismatch")
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	defer evidenceFile.Close()
	outcome, err := Reconstruct(evidenceFile, manifest.Evidence, plan)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	var actorReport actorSummary
	if err := json.Unmarshal(actorRaw, &actorReport); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if err := compareActorReport(outcome, actorReport); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	return outcome, manifest, nil
}

type actorSummary struct {
	TargetQty            int64   `json:"TargetQty"`
	FilledQty            int64   `json:"FilledQty"`
	DecisionAt           int64   `json:"DecisionAt"`
	DecisionMid          int64   `json:"DecisionMid"`
	Notional             int64   `json:"Notional"`
	QuoteFees            int64   `json:"QuoteFees"`
	TerminalMid          int64   `json:"TerminalMid"`
	TargetShortfallValid bool    `json:"TargetShortfallValid"`
	TargetShortfall      int64   `json:"TargetShortfall"`
	TargetShortfallBps   float64 `json:"TargetShortfallBps"`
}

func compareActorReport(outcome ReconstructedOutcome, report actorSummary) error {
	if outcome.TargetQty != report.TargetQty || outcome.FilledQty != report.FilledQty ||
		outcome.DecisionAt != report.DecisionAt || outcome.DecisionMid != report.DecisionMid ||
		outcome.Notional != report.Notional || outcome.QuoteFees != report.QuoteFees ||
		outcome.TerminalMid != report.TerminalMid ||
		outcome.TargetShortfallDefined != report.TargetShortfallValid {
		return errors.New("execution pilot: actor report differs from independent event reconstruction")
	}
	if outcome.TargetShortfallDefined && (outcome.TargetShortfall != report.TargetShortfall ||
		outcome.TargetShortfallBps != report.TargetShortfallBps) {
		return errors.New("execution pilot: actor target shortfall differs from independent reconstruction")
	}
	return nil
}

func WriteResult(path string, outcome ReconstructedOutcome, manifest RunManifest) error {
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	return writeExclusiveJSON(path, struct {
		SchemaVersion       int                  `json:"schema_version"`
		ManifestTypedSHA256 string               `json:"manifest_typed_sha256"`
		Outcome             ReconstructedOutcome `json:"outcome"`
	}{1, hex.EncodeToString(manifestDigest[:]), outcome})
}

func writeExclusiveJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
