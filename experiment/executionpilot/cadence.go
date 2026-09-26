package executionpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"exchange_sim/simulations/executionlab"
)

const CadencePlanSchemaVersion = 1

type CadenceCell struct {
	NetworkLatencyNanos int64 `json:"network_latency_nanos"`
	PollIntervalNanos   int64 `json:"poll_interval_nanos"`
	TargetQty           int64 `json:"target_qty"`
	Seed                int64 `json:"seed"`
}

type CadenceLockedPlan struct {
	SchemaVersion   int             `json:"schema_version"`
	Cell            CadenceCell     `json:"cell"`
	Identity        Identity        `json:"identity"`
	EffectiveWorld  json.RawMessage `json:"effective_world"`
	TypedPlanSHA256 string          `json:"typed_plan_sha256"`
}

type CadenceTiming struct {
	ScheduledFirstGateTickNanos int64  `json:"scheduled_first_gate_tick_nanos"`
	FirstUsableQuoteAtNanos     *int64 `json:"first_usable_quote_at_nanos"`
	RealizedDecisionAtNanos     *int64 `json:"realized_decision_at_nanos"`
	GateWaitNanos               *int64 `json:"gate_wait_nanos"`
	GateAdjustedPollWaitNanos   *int64 `json:"gate_adjusted_poll_wait_nanos"`
	SelectedQuoteAgeNanos       *int64 `json:"selected_quote_age_nanos"`
}

type CadenceReconstruction struct {
	Outcome ReconstructedOutcome `json:"outcome"`
	Timing  CadenceTiming        `json:"timing"`
}

func ValidateCadenceCell(cell CadenceCell) error {
	if !memberInt64(cell.NetworkLatencyNanos, 1_000_000, 90_000_000) ||
		!memberInt64(cell.PollIntervalNanos, 1_000_000, 80_000_000) ||
		cell.TargetQty != 500_000_000 || !memberInt64(cell.Seed, 13001, 13011, 13017) {
		return errors.New("cadence pilot: cell outside prospective ME-002-B development matrix")
	}
	return nil
}

func NewCadenceWorld(cell CadenceCell) (*executionlab.Sim, error) {
	if err := ValidateCadenceCell(cell); err != nil {
		return nil, err
	}
	config := executionlab.DefaultSimConfig(executionlab.Immediate)
	config.Seed = cell.Seed
	config.Parent.TargetQty = cell.TargetQty
	config.Parent.PollInterval = time.Duration(cell.PollIntervalNanos)
	config.RecordSnapshotProjectionEvidence = true
	config.ExecutionLatency = 0
	config.ParentDeployment = &executionlab.ParentDeployment{
		MarketDataLatency: time.Duration(cell.NetworkLatencyNanos),
		RequestLatency:    time.Duration(cell.NetworkLatencyNanos),
		ResponseLatency:   time.Duration(cell.NetworkLatencyNanos),
		ProcessingDelay:   0,
	}
	return executionlab.NewSim(config)
}

func LockCadence(cell CadenceCell, identity Identity) (CadenceLockedPlan, error) {
	if err := ValidateIdentityForSchema(identity, LatencyEvidenceSchemaID); err != nil {
		return CadenceLockedPlan{}, err
	}
	world, err := NewCadenceWorld(cell)
	if err != nil {
		return CadenceLockedPlan{}, err
	}
	effective, err := json.Marshal(world.WorldContract())
	if err != nil {
		return CadenceLockedPlan{}, err
	}
	plan := CadenceLockedPlan{SchemaVersion: CadencePlanSchemaVersion, Cell: cell,
		Identity: identity, EffectiveWorld: effective}
	plan.TypedPlanSHA256, err = cadencePlanDigest(plan)
	return plan, err
}

func VerifyCadence(plan CadenceLockedPlan, actual Identity) (*executionlab.Sim, error) {
	if plan.SchemaVersion != CadencePlanSchemaVersion || plan.Identity != actual {
		return nil, errors.New("cadence pilot: plan version or runtime identity mismatch")
	}
	if err := ValidateIdentityForSchema(actual, LatencyEvidenceSchemaID); err != nil {
		return nil, err
	}
	world, err := NewCadenceWorld(plan.Cell)
	if err != nil {
		return nil, err
	}
	constructed, err := json.Marshal(world.WorldContract())
	if err != nil {
		return nil, err
	}
	var supplied bytes.Buffer
	if err := json.Compact(&supplied, plan.EffectiveWorld); err != nil || !bytes.Equal(supplied.Bytes(), constructed) {
		return nil, errors.New("cadence pilot: effective world differs from constructed world")
	}
	digest, err := cadencePlanDigest(plan)
	if err != nil || digest != plan.TypedPlanSHA256 {
		return nil, errors.New("cadence pilot: typed plan digest mismatch")
	}
	return world, nil
}

func cadencePlanDigest(plan CadenceLockedPlan) (string, error) {
	canonical, err := json.Marshal(struct {
		SchemaVersion  int             `json:"schema_version"`
		Cell           CadenceCell     `json:"cell"`
		Identity       Identity        `json:"identity"`
		EffectiveWorld json.RawMessage `json:"effective_world"`
	}{plan.SchemaVersion, plan.Cell, plan.Identity, plan.EffectiveWorld})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func DecodeCadencePlan(raw []byte) (CadenceLockedPlan, string, error) {
	var plan CadenceLockedPlan
	if err := ValidateStrictJSON(raw); err != nil {
		return plan, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return plan, "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return plan, "", errors.New("cadence pilot: trailing plan content")
	}
	if plan.SchemaVersion != CadencePlanSchemaVersion || len(plan.EffectiveWorld) == 0 || plan.TypedPlanSHA256 == "" {
		return plan, "", errors.New("cadence pilot: incomplete plan")
	}
	digest := sha256.Sum256(raw)
	return plan, hex.EncodeToString(digest[:]), nil
}

func ReconstructCadence(input io.Reader, evidence EvidenceIdentity, effectiveWorld json.RawMessage, cell CadenceCell) (CadenceReconstruction, error) {
	if err := ValidateCadenceCell(cell); err != nil {
		return CadenceReconstruction{}, err
	}
	state, err := reconstructDeployment(input, evidence, effectiveWorld, cell.TargetQty, cell.PollIntervalNanos)
	if err != nil {
		return CadenceReconstruction{}, err
	}
	if state.processingDelay != 0 || state.marketDataLatency != cell.NetworkLatencyNanos ||
		state.requestLatency != cell.NetworkLatencyNanos || state.responseLatency != cell.NetworkLatencyNanos {
		return CadenceReconstruction{}, errors.New("cadence pilot: effective deployment differs from cell")
	}
	gate := state.contract.Parents[0].Config.DecisionAfter
	firstGateTick := ((gate + cell.PollIntervalNanos - 1) / cell.PollIntervalNanos) * cell.PollIntervalNanos
	timing := CadenceTiming{ScheduledFirstGateTickNanos: firstGateTick}
	if state.firstUsableSeen {
		timing.FirstUsableQuoteAtNanos = &state.firstUsableAt
	}
	if state.wasSent {
		if !state.firstUsableSeen || state.result.DecisionAt < firstGateTick ||
			state.result.PublishedSnapshotAt > state.result.DecisionAt {
			return CadenceReconstruction{}, errors.New("cadence pilot: decision precedes usable information or eligible gate")
		}
		eligible := max(state.firstUsableAt, gate)
		if state.result.DecisionAt < eligible {
			return CadenceReconstruction{}, errors.New("cadence pilot: decision precedes usable quote")
		}
		gateWait := max(int64(0), gate-state.firstUsableAt)
		pollWait := state.result.DecisionAt - eligible
		quoteAge := state.result.DecisionAt - state.result.PublishedSnapshotAt
		timing.RealizedDecisionAtNanos = &state.result.DecisionAt
		timing.GateWaitNanos = &gateWait
		timing.GateAdjustedPollWaitNanos = &pollWait
		timing.SelectedQuoteAgeNanos = &quoteAge
	}
	return CadenceReconstruction{Outcome: state.result, Timing: timing}, nil
}

func WriteCadencePlan(repositoryDir, analyzerBinary, path string, cell CadenceCell) error {
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, LatencyEvidenceSchemaID)
	if err != nil {
		return err
	}
	plan, err := LockCadence(cell, identity)
	if err != nil {
		return err
	}
	return writeExclusiveJSON(path, plan)
}

func readCadencePlan(path string) (CadenceLockedPlan, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CadenceLockedPlan{}, "", err
	}
	return DecodeCadencePlan(raw)
}

func RunCadenceFromPlan(ctx context.Context, repositoryDir, analyzerBinary, planPath, outputDir string) (RunManifest, error) {
	plan, rawDigest, err := readCadencePlan(planPath)
	if err != nil {
		return RunManifest{}, err
	}
	identity, err := RuntimeIdentity(repositoryDir, analyzerBinary, LatencyEvidenceSchemaID)
	if err != nil {
		return RunManifest{}, err
	}
	world, err := VerifyCadence(plan, identity)
	if err != nil {
		return RunManifest{}, err
	}
	if err := os.Mkdir(outputDir, 0700); err != nil {
		return RunManifest{}, fmt.Errorf("cadence pilot: fresh output directory required: %w", err)
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
		return RunManifest{}, fmt.Errorf("cadence pilot: incomplete world; preserve failed attempt: %w", runErr)
	}
	if len(reports) != 1 {
		return RunManifest{}, errors.New("cadence pilot: unexpected parent count")
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

func AnalyzeCadenceRun(repositoryDir, simulatorBinary, planPath, outputDir string) (CadenceReconstruction, RunManifest, error) {
	plan, rawDigest, err := readCadencePlan(planPath)
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	analyzerBinary, err := os.Executable()
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	identity, err := ToolIdentity(repositoryDir, simulatorBinary, analyzerBinary, LatencyEvidenceSchemaID)
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	if _, err := VerifyCadence(plan, identity); err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	manifest, err := decodeStrictFile[RunManifest](filepath.Join(outputDir, ManifestFilename))
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	if manifest.SchemaVersion != 2 || manifest.Identity != identity || manifest.PlanRawSHA256 != rawDigest ||
		manifest.TypedPlanSHA256 != plan.TypedPlanSHA256 || manifest.Evidence.SchemaID != LatencyEvidenceSchemaID {
		return CadenceReconstruction{}, RunManifest{}, errors.New("cadence pilot: run manifest identity mismatch")
	}
	evidencePath := filepath.Join(outputDir, EvidenceFilename)
	evidenceDigest, err := fileSHA256(evidencePath)
	if err != nil || evidenceDigest != manifest.EvidenceFileSHA256 {
		return CadenceReconstruction{}, RunManifest{}, errors.New("cadence pilot: evidence file digest mismatch")
	}
	actorPath := filepath.Join(outputDir, ActorFilename)
	actorDigest, err := fileSHA256(actorPath)
	if err != nil || actorDigest != manifest.ActorFileSHA256 {
		return CadenceReconstruction{}, RunManifest{}, errors.New("cadence pilot: actor file digest mismatch")
	}
	evidenceFile, err := os.Open(evidencePath)
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	defer evidenceFile.Close()
	reconstruction, err := ReconstructCadence(evidenceFile, manifest.Evidence, plan.EffectiveWorld, plan.Cell)
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	actorRaw, err := os.ReadFile(actorPath)
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	actorReport, err := decodeLatencyActorReport(actorRaw)
	if err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	outcome := reconstruction.Outcome
	if err := compareActorReport(outcome, actorReport); err != nil {
		return CadenceReconstruction{}, RunManifest{}, err
	}
	if actorReport.Policy != string(executionlab.Immediate) || actorReport.Side != "BUY" ||
		actorReport.UnfilledQty != outcome.UnfilledQty || len(actorReport.Children) != actorReport.SubmittedChildren ||
		actorReport.SubmittedChildren != boolCount(outcome.OrderSentAt != 0) ||
		actorReport.RejectedChildren != boolCount(outcome.Status == OutcomeRejected) ||
		actorReport.TerminalCancels != boolCount(outcome.CancelledResidual > 0 && outcome.Status != OutcomeRejected) {
		return CadenceReconstruction{}, RunManifest{}, errors.New("cadence pilot: actor order lifecycle differs from replay")
	}
	return reconstruction, manifest, nil
}

func WriteCadenceResult(path string, reconstruction CadenceReconstruction, manifest RunManifest, cell CadenceCell) error {
	if err := ValidateCadenceCell(cell); err != nil {
		return err
	}
	if reconstruction.Outcome.LatencyFunnel == nil || manifest.SchemaVersion != 2 ||
		manifest.Evidence.SchemaID != LatencyEvidenceSchemaID || reconstruction.Outcome.TargetQty != cell.TargetQty {
		return errors.New("cadence pilot: result requires valid reconstruction and manifest")
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	return writeExclusiveJSON(path, struct {
		SchemaVersion       int                   `json:"schema_version"`
		Cell                CadenceCell           `json:"cell"`
		ManifestTypedSHA256 string                `json:"manifest_typed_sha256"`
		FilledFraction      float64               `json:"filled_fraction"`
		Reconstruction      CadenceReconstruction `json:"reconstruction"`
	}{1, cell, hex.EncodeToString(manifestDigest[:]),
		float64(reconstruction.Outcome.FilledQty) / float64(cell.TargetQty), reconstruction})
}
