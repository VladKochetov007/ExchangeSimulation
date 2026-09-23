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

func WriteInstructionPlan(repositoryDir, analyzerBinary, path string, cell InstructionCell) error {
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, InstructionEvidenceSchemaID)
	if err != nil {
		return err
	}
	plan, err := LockInstruction(cell, identity)
	if err != nil {
		return err
	}
	return writeExclusiveJSON(path, plan)
}

func readInstructionPlan(path string) (InstructionLockedPlan, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return InstructionLockedPlan{}, "", err
	}
	return DecodeInstructionPlan(raw)
}

func RunInstructionFromPlan(ctx context.Context, repositoryDir, analyzerBinary, planPath, outputDir string) (RunManifest, error) {
	plan, rawDigest, err := readInstructionPlan(planPath)
	if err != nil {
		return RunManifest{}, err
	}
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, InstructionEvidenceSchemaID)
	if err != nil {
		return RunManifest{}, err
	}
	world, err := VerifyInstruction(plan, identity)
	if err != nil {
		return RunManifest{}, err
	}
	if err := os.Mkdir(outputDir, 0700); err != nil {
		return RunManifest{}, fmt.Errorf("instruction pilot: fresh output directory required: %w", err)
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceFile, err := os.OpenFile(evidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return RunManifest{}, err
	}
	recorder := NewInstructionRecorder(evidenceFile)
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
		return RunManifest{}, fmt.Errorf("instruction pilot: incomplete world; preserve failed attempt: %w", runErr)
	}
	if len(reports) != 1 {
		return RunManifest{}, errors.New("instruction pilot: unexpected parent count")
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
	manifest := RunManifest{SchemaVersion: 3, PlanRawSHA256: rawDigest, TypedPlanSHA256: plan.TypedPlanSHA256,
		Identity: identity, Evidence: evidenceIdentity, EvidenceFileSHA256: evidenceDigest, ActorFileSHA256: actorDigest}
	if err := writeExclusiveJSON(filepath.Join(outputDir, ManifestFilename), manifest); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

func AnalyzeInstructionRun(repositoryDir, simulatorBinary, planPath, outputDir string) (ReconstructedOutcome, RunManifest, error) {
	plan, rawDigest, err := readInstructionPlan(planPath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	analyzerBinary, err := os.Executable()
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	identity, err := ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, InstructionEvidenceSchemaID)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if _, err := VerifyInstruction(plan, identity); err != nil {
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
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if manifest.SchemaVersion != 3 || manifest.Identity != identity || manifest.PlanRawSHA256 != rawDigest ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Evidence.SchemaID != InstructionEvidenceSchemaID {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("instruction pilot: run manifest identity mismatch")
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("instruction pilot: evidence file digest mismatch")
	}
	actorPath := filepath.Join(outputDir, ActorFilename)
	actorDigest, err := fileSHA256(actorPath)
	if err != nil || actorDigest != manifest.ActorFileSHA256 {
		return ReconstructedOutcome{}, RunManifest{}, errors.New("instruction pilot: actor report digest mismatch")
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	defer evidenceFile.Close()
	outcome, err := ReconstructInstruction(evidenceFile, manifest.Evidence, plan.EffectiveWorld, plan.Cell.TargetQty)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	if err := compareInstructionActorReport(actorRaw, outcome); err != nil {
		return ReconstructedOutcome{}, RunManifest{}, err
	}
	return outcome, manifest, nil
}

type instructionChildSummary struct {
	RequestID       uint64 `json:"RequestID"`
	OrderID         uint64 `json:"OrderID"`
	SentAt          int64  `json:"SentAt"`
	RequestedQty    int64  `json:"RequestedQty"`
	FilledQty       int64  `json:"FilledQty"`
	Notional        int64  `json:"Notional"`
	QuoteFee        int64  `json:"QuoteFee"`
	FirstFillAt     int64  `json:"FirstFillAt"`
	LastFillAt      int64  `json:"LastFillAt"`
	Rejected        bool   `json:"Rejected"`
	RejectReason    string `json:"RejectReason"`
	CancelRemaining int64  `json:"CancelRemaining"`
}

func compareInstructionActorReport(raw []byte, outcome ReconstructedOutcome) error {
	report, err := decodeLatencyActorReport(raw)
	if err != nil {
		return err
	}
	if err := compareActorReport(outcome, report); err != nil {
		return err
	}
	if report.Policy != string(executionlab.Immediate) || report.Side != "BUY" ||
		report.UnfilledQty != outcome.UnfilledQty || report.SubmittedChildren != boolCount(outcome.OrderSentAt != 0) ||
		report.RejectedChildren != boolCount(outcome.Status == OutcomeRejected) ||
		report.TerminalCancels != boolCount(outcome.CancelledResidual > 0) ||
		report.FirstVenueFillAt != outcome.FirstVenueFillAt || report.LastVenueFillAt != outcome.LastVenueFillAt ||
		report.UnpricedFeeCount != 0 || report.Shortfall != outcome.FilledShortfall ||
		report.ShortfallBps != outcome.FilledShortfallBps ||
		len(report.Children) != report.SubmittedChildren {
		return errors.New("instruction pilot: actor lifecycle summary differs from independent replay")
	}
	if outcome.OrderSentAt == 0 {
		return nil
	}
	if len(report.Children) != 1 || rejectDuplicateJSONKeys(report.Children[0]) != nil {
		return errors.New("instruction pilot: missing or malformed actor child")
	}
	var child instructionChildSummary
	childDecoder := json.NewDecoder(bytes.NewReader(report.Children[0]))
	childDecoder.DisallowUnknownFields()
	if err := childDecoder.Decode(&child); err != nil {
		return err
	}
	if len(jsonObjectFields(report.Children[0])) != reflect.TypeOf(executionlab.ChildReport{}).NumField() ||
		child.RequestID != outcome.RequestID || child.OrderID != outcome.OrderID ||
		child.SentAt != outcome.OrderSentAt || child.RequestedQty != outcome.TargetQty ||
		child.FilledQty != outcome.FilledQty || child.Notional != outcome.Notional ||
		child.QuoteFee != outcome.QuoteFees || child.FirstFillAt != outcome.FirstVenueFillAt ||
		child.LastFillAt != outcome.LastVenueFillAt || child.Rejected != (outcome.Status == OutcomeRejected) ||
		child.RejectReason != outcome.RejectReason || child.CancelRemaining != outcome.CancelledResidual {
		return errors.New("instruction pilot: actor child differs from independent order lifecycle")
	}
	return nil
}

func jsonObjectFields(raw []byte) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}

func WriteInstructionResult(path string, outcome ReconstructedOutcome, manifest RunManifest, cell InstructionCell) error {
	if err := ValidateInstructionCell(cell); err != nil {
		return err
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	return writeExclusiveJSON(path, struct {
		SchemaVersion       int                  `json:"schema_version"`
		Cell                InstructionCell      `json:"cell"`
		ManifestTypedSHA256 string               `json:"manifest_typed_sha256"`
		Outcome             ReconstructedOutcome `json:"outcome"`
	}{3, cell, hex.EncodeToString(manifestDigest[:]), outcome})
}
