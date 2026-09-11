// Command sv1dprobe is the CLI adapter for the development-only SV1D tri-arm
// contract. It creates a pre-run plan, audits one retained arm, or scores the
// three already audited arm results. Economic logic remains in analysis.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"exchange_sim/analysis"
)

const (
	probePlanContract = "v2-r2-sv1d-probe-plan-v1"
	armResultContract = "v2-r2-sv1d-arm-result-v1"
	scoreContract     = "v2-r2-sv1d-score-v1"
	probeID           = "v2-r2-sv1d-activation-659"
)

type planDocument struct {
	SchemaVersion int                    `json:"schema_version"`
	Contract      string                 `json:"contract"`
	ProbeID       string                 `json:"probe_id"`
	PlanSHA256    string                 `json:"plan_sha256"`
	Plan          analysis.SV1DProbePlan `json:"plan"`
}

type armResultDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Contract      string                      `json:"contract"`
	ProbeID       string                      `json:"probe_id"`
	Arm           analysis.SV1DProbeArmResult `json:"arm"`
}

type scoreDocument struct {
	SchemaVersion int                           `json:"schema_version"`
	Contract      string                        `json:"contract"`
	ProbeID       string                        `json:"probe_id"`
	PlanSHA256    string                        `json:"plan_sha256"`
	Score         analysis.SV1DProbeScore       `json:"score"`
	Arms          []analysis.SV1DProbeArmResult `json:"arms"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sv1dprobe:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "plan, audit, failure, score, verify-review, or verify-capacity")
	out := flag.String("out", "", "new JSON output path")
	planPath := flag.String("plan", "", "pre-run SV1D plan JSON")
	armName := flag.String("arm", "", "treatment, mode-off, or no-roster")
	runDir := flag.String("run-dir", "", "retained arm run directory")
	renderedDir := flag.String("rendered-dir", "", "independently rendered evidence directory")
	treatmentConfig := flag.String("treatment-config", "", "checked-in treatment config")
	modeOffConfig := flag.String("mode-off-config", "", "checked-in mode-off config")
	noRosterConfig := flag.String("no-roster-config", "", "checked-in no-roster config")
	sourceRevision := flag.String("source-revision", "", "externally resolved source revision")
	binarySHA256 := flag.String("binary-sha256", "", "externally resolved simulator binary SHA-256")
	analyzerSHA256 := flag.String("analyzer-sha256", "", "externally resolved sv1dprobe analyzer SHA-256")
	rendererSHA256 := flag.String("renderer-sha256", "", "externally resolved evsrender SHA-256")
	modeOffResult := flag.String("mode-off-result", "", "audited mode-off arm result")
	noRosterResult := flag.String("no-roster-result", "", "audited no-roster arm result")
	treatmentResult := flag.String("treatment-result", "", "audited treatment arm result")
	reviewAttestation := flag.String("review-attestation", "", "externally signed SV1D review attestation")
	reviewReport := flag.String("review-report", "", "externally produced SV1D review report")
	trustedReviewKey := flag.String("trusted-review-key", "", "raw 32-byte trusted Ed25519 public key")
	trustedReviewKeySHA256 := flag.String("trusted-review-key-sha256", "", "externally resolved trusted review key SHA-256")
	treeRevision := flag.String("tree-revision", "", "externally resolved reviewed Git tree revision")
	planSHA256 := flag.String("plan-sha256", "", "externally resolved canonical SV1D plan SHA-256")
	failureReason := flag.String("failure-reason", "", "machine-readable reason for an incomplete arm result")
	parentRegistrationSHA256 := flag.String("parent-registration-sha256", "", "raw parent preregistration SHA-256")
	amendmentSHA256 := flag.String("amendment-sha256", "", "raw SV1D amendment SHA-256")
	capacityAttestation := flag.String("capacity-attestation", "", "measured SV1D capacity attestation")
	capacityTreatmentConfig := flag.String("capacity-treatment-config", "", "capacity-only treatment config")
	capacityModeOffConfig := flag.String("capacity-mode-off-config", "", "capacity-only mode-off config")
	capacityNoRosterConfig := flag.String("capacity-no-roster-config", "", "capacity-only no-roster config")
	capacityConfigDeltaSHA256 := flag.String("capacity-config-delta-sha256", "", "canonical capacity-config delta SHA-256")
	runnerSHA256 := flag.String("runner-sha256", "", "externally resolved capacity-runner SHA-256")
	measurerSHA256 := flag.String("measurer-sha256", "", "externally resolved resource-measurer SHA-256")
	resourcePolicySHA256 := flag.String("resource-policy-sha256", "", "externally resolved resource-policy SHA-256")
	outputParent := flag.String("output-parent", "", "capacity output parent path")
	measurementRoot := flag.String("measurement-root", "", "capacity measurement root path")
	measurementRecordsRoot := flag.String("measurement-records-root", "", "capacity measurement-records root path")
	measurementRecordsSHA256 := flag.String("measurement-records-sha256", "", "canonical measurement-records manifest SHA-256")
	filesystemDevice := flag.String("filesystem-device", "", "measured filesystem device")
	filesystemID := flag.String("filesystem-id", "", "measured filesystem ID")
	filesystemType := flag.String("filesystem-type", "", "measured filesystem type")
	filesystemMountID := flag.String("filesystem-mount-id", "", "measured filesystem mount ID")
	filesystemUUID := flag.String("filesystem-uuid", "", "measured filesystem UUID")
	flag.Parse()

	switch *mode {
	case "plan":
		return createPlan(*out, *treatmentConfig, *modeOffConfig, *noRosterConfig, *sourceRevision, *binarySHA256, *analyzerSHA256, *rendererSHA256)
	case "audit":
		return auditArm(*out, *planPath, *armName, *runDir, *renderedDir)
	case "failure":
		return publishFailedArm(*out, *planPath, *armName, *failureReason)
	case "score":
		return scoreArms(*out, *planPath, *treatmentResult, *modeOffResult, *noRosterResult)
	case "verify-review":
		return verifyReview(reviewVerificationInputs{
			AttestationPath: *reviewAttestation, ReportPath: *reviewReport, TrustedKeyPath: *trustedReviewKey,
			SourceRevision: *sourceRevision, TreeRevision: *treeRevision, PlanSHA256: *planSHA256,
			ParentRegistrationSHA256: *parentRegistrationSHA256, AmendmentSHA256: *amendmentSHA256,
			TreatmentConfigPath: *treatmentConfig, ModeOffConfigPath: *modeOffConfig, NoRosterConfigPath: *noRosterConfig,
			BinarySHA256: *binarySHA256, AnalyzerSHA256: *analyzerSHA256, RendererSHA256: *rendererSHA256,
		})
	case "verify-capacity":
		return verifyCapacity(capacityVerificationInputs{
			AttestationPath: *capacityAttestation,
			SourceRevision:  *sourceRevision, TreeRevision: *treeRevision, PlanSHA256: *planSHA256,
			ReviewAttestationSHA256: *reviewAttestation, ReviewReportSHA256: *reviewReport,
			TrustedReviewKeySHA256: *trustedReviewKeySHA256,
			TreatmentConfigPath:    *treatmentConfig, ModeOffConfigPath: *modeOffConfig, NoRosterConfigPath: *noRosterConfig,
			CapacityTreatmentConfigPath: *capacityTreatmentConfig, CapacityModeOffConfigPath: *capacityModeOffConfig, CapacityNoRosterConfigPath: *capacityNoRosterConfig,
			CapacityConfigDeltaSHA256: *capacityConfigDeltaSHA256,
			BinarySHA256:              *binarySHA256, AnalyzerSHA256: *analyzerSHA256, RendererSHA256: *rendererSHA256,
			RunnerSHA256: *runnerSHA256, MeasurerSHA256: *measurerSHA256, ResourcePolicySHA256: *resourcePolicySHA256,
			OutputParent: *outputParent, MeasurementRoot: *measurementRoot, MeasurementRecordsRoot: *measurementRecordsRoot, MeasurementRecordsSHA256: *measurementRecordsSHA256,
			FilesystemDevice: *filesystemDevice, FilesystemID: *filesystemID, FilesystemType: *filesystemType,
			FilesystemMountID: *filesystemMountID, FilesystemUUID: *filesystemUUID,
		})
	default:
		return fmt.Errorf("-mode must be plan, audit, failure, score, verify-review, or verify-capacity")
	}
}

func createPlan(out, treatmentPath, modeOffPath, noRosterPath, sourceRevision, binarySHA256, analyzerSHA256, rendererSHA256 string) error {
	if out == "" || treatmentPath == "" || modeOffPath == "" || noRosterPath == "" || sourceRevision == "" || binarySHA256 == "" || analyzerSHA256 == "" || rendererSHA256 == "" {
		return fmt.Errorf("plan mode requires -out, all three config paths, -source-revision, -binary-sha256, -analyzer-sha256, and -renderer-sha256")
	}
	if err := verifyCurrentAnalyzer(analyzerSHA256); err != nil {
		return err
	}
	triad, err := analysis.ValidateSV1DConfigTriadFiles(analysis.SV1DConfigPaths{
		Treatment: treatmentPath, ModeOff: modeOffPath, NoRoster: noRosterPath,
	})
	if err != nil {
		return err
	}
	plan, err := analysis.BuildRegisteredSV1DProbePlan(triad, sourceRevision, binarySHA256, analyzerSHA256, rendererSHA256)
	if err != nil {
		return err
	}
	planSHA256, err := analysis.SV1DProbePlanSHA256(plan)
	if err != nil {
		return err
	}
	return publishJSON(out, planDocument{SchemaVersion: 1, Contract: probePlanContract, ProbeID: probeID, PlanSHA256: planSHA256, Plan: plan})
}

func auditArm(out, planPath, armName, runDir, renderedDir string) error {
	if out == "" || planPath == "" || armName == "" || runDir == "" || renderedDir == "" {
		return fmt.Errorf("audit mode requires -out, -plan, -arm, -run-dir, and -rendered-dir")
	}
	document, err := readPlan(planPath)
	if err != nil {
		return err
	}
	spec, contract, treatment, err := planArm(document.Plan, armName)
	if err != nil {
		return err
	}
	if err := verifyCurrentAnalyzer(document.Plan.AnalyzerSHA256); err != nil {
		return err
	}
	run, err := analysis.Open(runDir)
	if err != nil {
		return err
	}
	result, err := run.AuditSV1DProbeArm(analysis.SV1DProbeArmAuditOptions{
		Spec: spec, Contract: contract, Treatment: treatment, PlanSHA256: document.PlanSHA256,
		Activation: analysis.CDFActivationOptions{
			Contract: contract, EvidenceDir: runDir, RenderedEvidenceDir: renderedDir,
			ExpectedProvenance: analysis.CDFExpectedProvenance{
				ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision,
				BinarySHA256: spec.BinarySHA256, BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
				RendererSHA256: spec.RendererSHA256, RendererSourceRevision: spec.SourceRevision,
				RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1",
				RendererTrimpath: true, RendererCGOEnabled: "0",
			},
		},
	})
	if err != nil {
		return err
	}
	return publishJSON(out, armResultDocument{SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID, Arm: result})
}

func publishFailedArm(out, planPath, armName, reason string) error {
	if out == "" || planPath == "" || armName == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("failure mode requires -out, -plan, -arm, and -failure-reason")
	}
	document, err := readPlan(planPath)
	if err != nil {
		return err
	}
	spec, _, _, err := planArm(document.Plan, armName)
	if err != nil {
		return err
	}
	if err := verifyCurrentAnalyzer(document.Plan.AnalyzerSHA256); err != nil {
		return err
	}
	result := analysis.SV1DProbeArmResult{
		ArmName: spec.Name, ExperimentID: spec.ExperimentID, HypothesisID: spec.HypothesisID,
		ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision, BinarySHA256: spec.BinarySHA256,
		AnalyzerSHA256: spec.AnalyzerSHA256, RendererSHA256: spec.RendererSHA256, PlanSHA256: document.PlanSHA256,
		Complete: false, EvidenceValid: false, StrictMechanicsValid: false, TerminalValuationValid: false,
		ActivationSatisfied: false, AntiCheatingSatisfied: false, FailureReasons: []string{reason},
	}
	return publishJSON(out, armResultDocument{SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID, Arm: result})
}

func scoreArms(out, planPath, treatmentPath, modeOffPath, noRosterPath string) error {
	if out == "" || planPath == "" || treatmentPath == "" || modeOffPath == "" || noRosterPath == "" {
		return fmt.Errorf("score mode requires -out, -plan, and all three arm result paths")
	}
	document, err := readPlan(planPath)
	if err != nil {
		return err
	}
	if err := verifyCurrentAnalyzer(document.Plan.AnalyzerSHA256); err != nil {
		return err
	}
	paths := []string{treatmentPath, modeOffPath, noRosterPath}
	arms := make([]analysis.SV1DProbeArmResult, 0, len(paths))
	for _, path := range paths {
		result, err := readArmResult(path)
		if err != nil {
			return err
		}
		if result.SchemaVersion != 1 || result.Contract != armResultContract || result.ProbeID != probeID {
			return fmt.Errorf("arm result %s has an invalid contract identity", path)
		}
		arms = append(arms, result.Arm)
	}
	score := analysis.ScoreSV1DProbe(document.Plan, arms)
	if score.PlanSHA256 == "" {
		return fmt.Errorf("could not derive canonical probe plan digest")
	}
	if err := publishJSON(out, scoreDocument{SchemaVersion: 1, Contract: scoreContract, ProbeID: probeID, PlanSHA256: score.PlanSHA256, Score: score, Arms: arms}); err != nil {
		return err
	}
	if score.Status == analysis.SV1DProbeStatusInvalidEvidence || score.Status == analysis.SV1DProbeStatusIncompleteArm {
		return fmt.Errorf("probe score is not executable: %s", score.Status)
	}
	return nil
}

func verifyCurrentAnalyzer(expectedSHA256 string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve sv1dprobe executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve sv1dprobe executable symlinks: %w", err)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return fmt.Errorf("open sv1dprobe executable: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash sv1dprobe executable: %w", err)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expectedSHA256 {
		return fmt.Errorf("sv1dprobe executable hash %s does not match plan %s", actual, expectedSHA256)
	}
	return nil
}

func readPlan(path string) (planDocument, error) {
	var document planDocument
	if err := readStrictJSON(path, &document); err != nil {
		return planDocument{}, fmt.Errorf("read plan: %w", err)
	}
	if document.SchemaVersion != 1 || document.Contract != probePlanContract || document.ProbeID != probeID || !isHexDigest(document.PlanSHA256) {
		return planDocument{}, fmt.Errorf("plan has an invalid contract identity")
	}
	if err := analysis.ValidateRegisteredSV1DProbePlan(document.Plan); err != nil {
		return planDocument{}, err
	}
	actualPlanSHA256, err := analysis.SV1DProbePlanSHA256(document.Plan)
	if err != nil {
		return planDocument{}, err
	}
	if document.PlanSHA256 != actualPlanSHA256 {
		return planDocument{}, fmt.Errorf("plan digest does not match its canonical typed plan")
	}
	return document, nil
}

func isHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type reviewVerificationInputs struct {
	AttestationPath          string
	ReportPath               string
	TrustedKeyPath           string
	SourceRevision           string
	TreeRevision             string
	PlanSHA256               string
	ParentRegistrationSHA256 string
	AmendmentSHA256          string
	TreatmentConfigPath      string
	ModeOffConfigPath        string
	NoRosterConfigPath       string
	BinarySHA256             string
	AnalyzerSHA256           string
	RendererSHA256           string
}

func verifyReview(inputs reviewVerificationInputs) error {
	if inputs.AttestationPath == "" || inputs.ReportPath == "" || inputs.TrustedKeyPath == "" || inputs.SourceRevision == "" || inputs.TreeRevision == "" || inputs.PlanSHA256 == "" || inputs.ParentRegistrationSHA256 == "" || inputs.AmendmentSHA256 == "" || inputs.TreatmentConfigPath == "" || inputs.ModeOffConfigPath == "" || inputs.NoRosterConfigPath == "" || inputs.BinarySHA256 == "" || inputs.AnalyzerSHA256 == "" || inputs.RendererSHA256 == "" {
		return fmt.Errorf("verify-review mode requires attestation, report, trusted key, source/tree/plan identities, parent/amendment hashes, all config paths, and all tool hashes")
	}
	trustedKey, err := readTrustedReviewKey(inputs.TrustedKeyPath)
	if err != nil {
		return err
	}
	configSHA256 := func(path string) (string, error) {
		raw, err := readRegularNoSymlink(path)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(raw)
		return hex.EncodeToString(digest[:]), nil
	}
	treatmentConfigSHA256, err := configSHA256(inputs.TreatmentConfigPath)
	if err != nil {
		return fmt.Errorf("hash treatment config: %w", err)
	}
	modeOffConfigSHA256, err := configSHA256(inputs.ModeOffConfigPath)
	if err != nil {
		return fmt.Errorf("hash mode-off config: %w", err)
	}
	noRosterConfigSHA256, err := configSHA256(inputs.NoRosterConfigPath)
	if err != nil {
		return fmt.Errorf("hash no-roster config: %w", err)
	}
	_, err = analysis.VerifySV1DReviewAttestation(inputs.AttestationPath, inputs.ReportPath, analysis.SV1DReviewExpectation{
		SourceRevision: inputs.SourceRevision, TreeRevision: inputs.TreeRevision, ProbeID: probeID, PlanSHA256: inputs.PlanSHA256,
		ParentRegistrationSHA256: inputs.ParentRegistrationSHA256, AmendmentSHA256: inputs.AmendmentSHA256,
		TreatmentConfigSHA256: treatmentConfigSHA256, ModeOffConfigSHA256: modeOffConfigSHA256, NoRosterConfigSHA256: noRosterConfigSHA256,
		BinarySHA256: inputs.BinarySHA256, AnalyzerSHA256: inputs.AnalyzerSHA256, RendererSHA256: inputs.RendererSHA256,
		TrustedPublicKey: trustedKey,
	})
	return err
}

func readTrustedReviewKey(path string) (ed25519.PublicKey, error) {
	raw, err := readRegularNoSymlink(path)
	if err != nil {
		return nil, fmt.Errorf("read trusted review key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("trusted review key must be exactly %d raw bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(append([]byte(nil), raw...)), nil
}

func readRegularNoSymlink(path string) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	current := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(absolute, current), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("path contains a symlink: %s", path)
		}
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", path)
	}
	return os.ReadFile(absolute)
}

func readArmResult(path string) (armResultDocument, error) {
	var document armResultDocument
	if err := readStrictJSON(path, &document); err != nil {
		return armResultDocument{}, fmt.Errorf("read arm result %s: %w", path, err)
	}
	return document, nil
}

func planArm(plan analysis.SV1DProbePlan, name string) (analysis.SV1DProbeArmSpec, analysis.CDFActivationContract, bool, error) {
	contract := analysis.RegisteredSV1DActivationContract()
	switch name {
	case "treatment":
		contract.ExperimentID = plan.Treatment.ExperimentID
		contract.HypothesisID = plan.Treatment.HypothesisID
		return plan.Treatment, contract, true, nil
	case "mode-off":
		contract.ExperimentID = plan.ModeOff.ExperimentID
		contract.HypothesisID = plan.ModeOff.HypothesisID
		return plan.ModeOff, contract, false, nil
	case "no-roster":
		contract.ExperimentID = plan.NoRoster.ExperimentID
		contract.HypothesisID = plan.NoRoster.HypothesisID
		return plan.NoRoster, contract, false, nil
	default:
		return analysis.SV1DProbeArmSpec{}, analysis.CDFActivationContract{}, false, fmt.Errorf("unknown arm %q", name)
	}
}

func publishJSON(path string, value any) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	directory := filepath.Dir(absolute)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".sv1dprobe-output-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	raw, err := json.Marshal(value)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(raw, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, absolute); err != nil {
		return fmt.Errorf("publish without overwrite: %w", err)
	}
	removeTemporary = true
	_ = os.Remove(temporaryName)
	return nil
}

func readStrictJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkJSONTokens(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func walkJSONTokens(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key: %s", key)
			}
			seen[key] = struct{}{}
			if err := walkJSONTokens(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkJSONTokens(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
