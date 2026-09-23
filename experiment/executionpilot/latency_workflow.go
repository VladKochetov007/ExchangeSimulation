package executionpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"exchange_sim/simulations/executionlab"
)

func WriteLatencyPlan(repositoryDir, analyzerBinary, path string, cell LatencyCell) error {
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, LatencyEvidenceSchemaID)
	if err != nil {
		return err
	}
	plan, err := LockLatency(cell, identity)
	if err != nil {
		return err
	}
	return writeExclusiveJSON(path, plan)
}

func readLatencyPlan(path string) (LatencyLockedPlan, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return LatencyLockedPlan{}, "", err
	}
	return DecodeLatencyPlan(raw)
}

func RunLatencyFromPlan(ctx context.Context, repositoryDir, analyzerBinary, planPath, outputDir string) (RunManifest, error) {
	plan, rawDigest, err := readLatencyPlan(planPath)
	if err != nil {
		return RunManifest{}, err
	}
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, LatencyEvidenceSchemaID)
	if err != nil {
		return RunManifest{}, err
	}
	world, err := VerifyLatency(plan, identity)
	if err != nil {
		return RunManifest{}, err
	}
	if err := os.Mkdir(outputDir, 0700); err != nil {
		return RunManifest{}, fmt.Errorf("latency pilot: fresh output directory required: %w", err)
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceFile, err := os.OpenFile(evidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return RunManifest{}, err
	}
	recorder := NewLatencyRecorder(evidenceFile)
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
		return RunManifest{}, fmt.Errorf("latency pilot: incomplete world; preserve failed attempt: %w", runErr)
	}
	if len(reports) != 1 {
		return RunManifest{}, errors.New("latency pilot: unexpected parent count")
	}
	actorPath := filepath.Join(outputDir, ActorFilename)
	if err := writeExclusiveJSON(actorPath, reports[0]); err != nil {
		return RunManifest{}, err
	}
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil {
		return RunManifest{}, err
	}
	actorDigest, err := fileSHA256(actorPath)
	if err != nil {
		return RunManifest{}, err
	}
	manifest := RunManifest{SchemaVersion: 2, PlanRawSHA256: rawDigest, TypedPlanSHA256: plan.TypedPlanSHA256,
		Identity: identity, Evidence: evidenceIdentity, EvidenceFileSHA256: evidenceDigest, ActorFileSHA256: actorDigest}
	if err := writeExclusiveJSON(filepath.Join(outputDir, ManifestFilename), manifest); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

func AnalyzeLatencyRun(repositoryDir, simulatorBinary, planPath, outputDir string) (ReconstructedOutcome, RunManifest, error) {
	plan, rawDigest, err := readLatencyPlan(planPath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	analyzerBinary, err := os.Executable()
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	identity, err := ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, LatencyEvidenceSchemaID)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if _, err := VerifyLatency(plan, identity); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(outputDir, ManifestFilename))
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if err := ValidateStrictJSON(manifestRaw); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	var manifest RunManifest
	manifestDecoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	manifestDecoder.DisallowUnknownFields()
	if err := manifestDecoder.Decode(&manifest); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if manifest.SchemaVersion != 2 || manifest.Identity != identity || manifest.PlanRawSHA256 != rawDigest ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Evidence.SchemaID != LatencyEvidenceSchemaID {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("latency pilot: run manifest identity mismatch")
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("latency pilot: evidence file digest mismatch")
	}
	actorPath := filepath.Join(outputDir, ActorFilename)
	actorDigest, err := fileSHA256(actorPath)
	if err != nil || actorDigest != manifest.ActorFileSHA256 {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("latency pilot: actor report digest mismatch")
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	defer evidenceFile.Close()
	outcome, err := ReconstructLatency(evidenceFile, manifest.Evidence, plan.EffectiveWorld, plan.Cell.TargetQty)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	actorReport, err := decodeLatencyActorReport(actorRaw)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if err := compareActorReport(outcome, actorReport); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if actorReport.Policy != string(executionlab.Immediate) || actorReport.Side != "BUY" ||
		actorReport.UnfilledQty != outcome.UnfilledQty || len(actorReport.Children) != actorReport.SubmittedChildren ||
		actorReport.SubmittedChildren != boolCount(outcome.OrderSentAt != 0) ||
		actorReport.RejectedChildren != boolCount(outcome.Status == OutcomeRejected) ||
		actorReport.TerminalCancels != boolCount(outcome.CancelledResidual > 0 && outcome.Status != OutcomeRejected) {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("latency pilot: actor order lifecycle differs from event reconstruction")
	}
	return outcome, manifest, nil
}

func decodeLatencyActorReport(raw []byte) (actorSummary, error) {
	if err := ValidateStrictJSON(raw); err != nil {
		return actorSummary{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return actorSummary{}, err
	}
	typeOfReport := reflect.TypeOf(executionlab.ExecutionReport{})
	if len(fields) != typeOfReport.NumField() {
		return actorSummary{}, errors.New("latency pilot: actor report has missing or extra fields")
	}
	for index := 0; index < typeOfReport.NumField(); index++ {
		if _, present := fields[typeOfReport.Field(index).Name]; !present {
			return actorSummary{}, errors.New("latency pilot: actor report lacks declared field")
		}
	}
	var report actorSummary
	if err := json.Unmarshal(raw, &report); err != nil {
		return actorSummary{}, err
	}
	return report, nil
}

func boolCount(condition bool) int {
	if condition {
		return 1
	}
	return 0
}

func WriteLatencyResult(path string, outcome ReconstructedOutcome, manifest RunManifest, cell LatencyCell) error {
	if outcome.LatencyFunnel == nil || manifest.SchemaVersion != 2 || manifest.Evidence.SchemaID != LatencyEvidenceSchemaID {
		return errors.New("latency pilot: result requires valid latency reconstruction and manifest")
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	return writeExclusiveJSON(path, struct {
		SchemaVersion       int                  `json:"schema_version"`
		Cell                LatencyCell          `json:"cell"`
		ManifestTypedSHA256 string               `json:"manifest_typed_sha256"`
		Outcome             ReconstructedOutcome `json:"outcome"`
	}{2, cell, hex.EncodeToString(manifestDigest[:]), outcome})
}
