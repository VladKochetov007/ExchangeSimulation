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
	"syscall"

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
	ArmCorpus     []scoreCorpusEntry            `json:"arm_corpus"`
}

const scoreCorpusManifestContract = "v2-r2-sv1d-score-corpus-manifest-v1"

type scoreCorpusEntry struct {
	Arm    string `json:"arm"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type scoreCorpusManifest struct {
	SchemaVersion            int                `json:"schema_version"`
	Contract                 string             `json:"contract"`
	ProbeID                  string             `json:"probe_id"`
	PlanSHA256               string             `json:"plan_sha256"`
	ActivationMetadataPath   string             `json:"activation_metadata_path"`
	ActivationMetadataSHA256 string             `json:"activation_metadata_sha256"`
	ScorePath                string             `json:"score_path"`
	ScoreSHA256              string             `json:"score_sha256"`
	ArmCorpus                []scoreCorpusEntry `json:"arm_corpus"`
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
	treatmentRunDir := flag.String("treatment-run-dir", "", "retained treatment arm evidence directory")
	treatmentRenderedDir := flag.String("treatment-rendered-dir", "", "retained treatment rendered evidence directory")
	modeOffRunDir := flag.String("mode-off-run-dir", "", "retained mode-off arm evidence directory")
	modeOffRenderedDir := flag.String("mode-off-rendered-dir", "", "retained mode-off rendered evidence directory")
	noRosterRunDir := flag.String("no-roster-run-dir", "", "retained no-roster arm evidence directory")
	noRosterRenderedDir := flag.String("no-roster-rendered-dir", "", "retained no-roster rendered evidence directory")
	reviewAttestation := flag.String("review-attestation", "", "externally signed SV1D review attestation")
	reviewReport := flag.String("review-report", "", "externally produced SV1D review report")
	trustedReviewKey := flag.String("trusted-review-key", "", "raw 32-byte trusted Ed25519 public key")
	trustedReviewKeySHA256 := flag.String("trusted-review-key-sha256", "", "externally resolved trusted review key SHA-256")
	treeRevision := flag.String("tree-revision", "", "externally resolved reviewed Git tree revision")
	planSHA256 := flag.String("plan-sha256", "", "externally resolved canonical SV1D plan SHA-256")
	activationMetadata := flag.String("activation-metadata", "", "externally resolved activation run metadata")
	reviewAttestationSHA256 := flag.String("review-attestation-sha256", "", "externally resolved review attestation SHA-256")
	reviewReportSHA256 := flag.String("review-report-sha256", "", "externally resolved review report SHA-256")
	capacityAttestationSHA256 := flag.String("capacity-attestation-sha256", "", "externally resolved capacity attestation SHA-256")
	capacityRecordsSHA256 := flag.String("capacity-records-sha256", "", "externally resolved capacity measurement-records SHA-256")
	capacityRunnerSHA256 := flag.String("capacity-runner-sha256", "", "externally resolved capacity runner SHA-256")
	activationRunnerSHA256 := flag.String("activation-runner-sha256", "", "externally resolved activation runner SHA-256")
	activationMetadataSHA256 := flag.String("activation-metadata-sha256", "", "externally resolved activation metadata SHA-256")
	evidenceSchemaEpoch := flag.Uint("evidence-schema-epoch", 0, "externally resolved evidence schema epoch")
	gomaxprocs := flag.Int("gomaxprocs", 0, "externally resolved GOMAXPROCS envelope")
	gomemlimit := flag.String("gomemlimit", "", "externally resolved GOMEMLIMIT envelope")
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
	corpusManifest := flag.String("corpus-manifest", "", "immutable strict-score corpus manifest output path")
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
		return auditArm(*out, *planPath, *armName, *runDir, *renderedDir, strictProvenanceInputs{
			SourceRevision: *sourceRevision, TreeRevision: *treeRevision, PlanSHA256: *planSHA256,
			ParentRegistrationSHA256: *parentRegistrationSHA256, AmendmentSHA256: *amendmentSHA256,
			ActivationMetadataPath: *activationMetadata, ReviewAttestationSHA256: *reviewAttestationSHA256,
			ReviewReportSHA256: *reviewReportSHA256, CapacityAttestationSHA256: *capacityAttestationSHA256,
			CapacityRecordsSHA256: *capacityRecordsSHA256, CapacityRunnerSHA256: *capacityRunnerSHA256,
			ActivationRunnerSHA256: *activationRunnerSHA256, ActivationMetadataSHA256: *activationMetadataSHA256,
			TrustedReviewKeySHA256: *trustedReviewKeySHA256, EvidenceSchemaEpoch: uint32(*evidenceSchemaEpoch),
			GOMAXPROCS: *gomaxprocs, GOMEMLIMIT: *gomemlimit,
		})
	case "failure":
		return publishFailedArm(*out, *planPath, *armName, *failureReason, strictProvenanceInputs{
			SourceRevision: *sourceRevision, TreeRevision: *treeRevision, PlanSHA256: *planSHA256,
			ParentRegistrationSHA256: *parentRegistrationSHA256, AmendmentSHA256: *amendmentSHA256,
			ActivationMetadataPath: *activationMetadata, ReviewAttestationSHA256: *reviewAttestationSHA256,
			ReviewReportSHA256: *reviewReportSHA256, CapacityAttestationSHA256: *capacityAttestationSHA256,
			CapacityRecordsSHA256: *capacityRecordsSHA256, CapacityRunnerSHA256: *capacityRunnerSHA256,
			ActivationRunnerSHA256: *activationRunnerSHA256, ActivationMetadataSHA256: *activationMetadataSHA256,
			TrustedReviewKeySHA256: *trustedReviewKeySHA256, EvidenceSchemaEpoch: uint32(*evidenceSchemaEpoch),
			GOMAXPROCS: *gomaxprocs, GOMEMLIMIT: *gomemlimit,
		})
	case "score":
		return scoreArms(*out, *corpusManifest, *planPath, *treatmentResult, *modeOffResult, *noRosterResult, scoreArmEvidencePaths{
			"treatment": {RunDir: *treatmentRunDir, RenderedDir: *treatmentRenderedDir},
			"mode-off":  {RunDir: *modeOffRunDir, RenderedDir: *modeOffRenderedDir},
			"no-roster": {RunDir: *noRosterRunDir, RenderedDir: *noRosterRenderedDir},
		}, strictProvenanceInputs{
			SourceRevision: *sourceRevision, TreeRevision: *treeRevision, PlanSHA256: *planSHA256,
			ParentRegistrationSHA256: *parentRegistrationSHA256, AmendmentSHA256: *amendmentSHA256,
			ActivationMetadataPath: *activationMetadata, ReviewAttestationSHA256: *reviewAttestationSHA256,
			ReviewReportSHA256: *reviewReportSHA256, CapacityAttestationSHA256: *capacityAttestationSHA256,
			CapacityRecordsSHA256: *capacityRecordsSHA256, CapacityRunnerSHA256: *capacityRunnerSHA256,
			ActivationRunnerSHA256: *activationRunnerSHA256, ActivationMetadataSHA256: *activationMetadataSHA256,
			TrustedReviewKeySHA256: *trustedReviewKeySHA256, EvidenceSchemaEpoch: uint32(*evidenceSchemaEpoch),
			GOMAXPROCS: *gomaxprocs, GOMEMLIMIT: *gomemlimit,
		})
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

type strictProvenanceInputs struct {
	SourceRevision            string
	TreeRevision              string
	PlanSHA256                string
	ParentRegistrationSHA256  string
	AmendmentSHA256           string
	ActivationMetadataPath    string
	ReviewAttestationSHA256   string
	ReviewReportSHA256        string
	CapacityAttestationSHA256 string
	CapacityRecordsSHA256     string
	CapacityRunnerSHA256      string
	ActivationRunnerSHA256    string
	ActivationMetadataSHA256  string
	TrustedReviewKeySHA256    string
	EvidenceSchemaEpoch       uint32
	GOMAXPROCS                int
	GOMEMLIMIT                string
}

func expectedSV1DProvenance(plan analysis.SV1DProbePlan, spec analysis.SV1DProbeArmSpec, inputs strictProvenanceInputs) analysis.CDFExpectedProvenance {
	return analysis.CDFExpectedProvenance{
		ConfigSHA256: spec.ConfigSHA256, TreatmentConfigSHA256: plan.Treatment.ConfigSHA256,
		ModeOffConfigSHA256:  plan.ModeOff.ConfigSHA256,
		NoRosterConfigSHA256: plan.NoRoster.ConfigSHA256, SourceRevision: inputs.SourceRevision,
		TreeRevision: inputs.TreeRevision, PlanSHA256: inputs.PlanSHA256,
		ParentRegistrationSHA256: inputs.ParentRegistrationSHA256, AmendmentSHA256: inputs.AmendmentSHA256,
		BinarySHA256: spec.BinarySHA256, AnalyzerSHA256: plan.AnalyzerSHA256,
		BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
		RendererSHA256: plan.RendererSHA256, RendererSourceRevision: inputs.SourceRevision,
		RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1",
		RendererTrimpath: true, RendererCGOEnabled: "0",
		ReviewAttestationSHA256: inputs.ReviewAttestationSHA256, ReviewReportSHA256: inputs.ReviewReportSHA256,
		CapacityAttestationSHA256: inputs.CapacityAttestationSHA256, CapacityRecordsSHA256: inputs.CapacityRecordsSHA256,
		CapacityRunnerSHA256: inputs.CapacityRunnerSHA256, ActivationRunnerSHA256: inputs.ActivationRunnerSHA256,
		ActivationMetadataSHA256: inputs.ActivationMetadataSHA256, ActivationMetadataPath: inputs.ActivationMetadataPath,
		TrustedReviewKeySHA256: inputs.TrustedReviewKeySHA256, EvidenceSchemaEpoch: inputs.EvidenceSchemaEpoch,
		GOMAXPROCS: inputs.GOMAXPROCS, GOMEMLIMIT: inputs.GOMEMLIMIT,
	}
}

func armSpecForName(plan analysis.SV1DProbePlan, name string) analysis.SV1DProbeArmSpec {
	switch name {
	case plan.Treatment.Name:
		return plan.Treatment
	case plan.ModeOff.Name:
		return plan.ModeOff
	case plan.NoRoster.Name:
		return plan.NoRoster
	default:
		return analysis.SV1DProbeArmSpec{}
	}
}

func validateStrictProvenanceInputs(plan analysis.SV1DProbePlan, inputs strictProvenanceInputs) error {
	planDigest, err := analysis.SV1DProbePlanSHA256(plan)
	if err != nil {
		return fmt.Errorf("hash canonical probe plan: %w", err)
	}
	if inputs.SourceRevision != plan.Treatment.SourceRevision || plan.ModeOff.SourceRevision != inputs.SourceRevision || plan.NoRoster.SourceRevision != inputs.SourceRevision {
		return fmt.Errorf("strict source revision does not match the registered probe plan")
	}
	if inputs.PlanSHA256 != planDigest {
		return fmt.Errorf("strict plan digest does not match the registered probe plan")
	}
	return nil
}

func auditArm(out, planPath, armName, runDir, renderedDir string, provenanceInputs strictProvenanceInputs) error {
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
	if err := validateStrictProvenanceInputs(document.Plan, provenanceInputs); err != nil {
		return err
	}
	expectedProvenance := expectedSV1DProvenance(document.Plan, spec, provenanceInputs)
	run, err := analysis.Open(runDir)
	if err != nil {
		return err
	}
	result, err := run.AuditSV1DProbeArm(analysis.SV1DProbeArmAuditOptions{
		Spec: spec, Contract: contract, Treatment: treatment, PlanSHA256: document.PlanSHA256,
		Activation: analysis.CDFActivationOptions{
			Contract: contract, EvidenceDir: runDir, RenderedEvidenceDir: renderedDir,
			ExpectedProvenance: expectedProvenance,
		},
	})
	if err != nil {
		return err
	}
	return publishJSON(out, armResultDocument{SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID, Arm: result})
}

func publishFailedArm(out, planPath, armName, reason string, inputs ...strictProvenanceInputs) error {
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
	var expectedProvenance analysis.CDFExpectedProvenance
	if len(inputs) == 0 {
		expectedProvenance = analysis.CDFExpectedProvenance{
			ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision,
			BinarySHA256: spec.BinarySHA256, BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
		}
	} else {
		if err := validateStrictProvenanceInputs(document.Plan, inputs[0]); err != nil {
			return err
		}
		expectedProvenance = expectedSV1DProvenance(document.Plan, spec, inputs[0])
	}
	result := analysis.SV1DProbeArmResult{
		ArmName: spec.Name, ExperimentID: spec.ExperimentID, HypothesisID: spec.HypothesisID,
		ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision, BinarySHA256: spec.BinarySHA256,
		AnalyzerSHA256: spec.AnalyzerSHA256, RendererSHA256: spec.RendererSHA256, PlanSHA256: document.PlanSHA256,
		TreeRevision: expectedProvenance.TreeRevision, ReviewAttestationSHA256: expectedProvenance.ReviewAttestationSHA256,
		ReviewReportSHA256: expectedProvenance.ReviewReportSHA256, CapacityAttestationSHA256: expectedProvenance.CapacityAttestationSHA256,
		CapacityRecordsSHA256: expectedProvenance.CapacityRecordsSHA256, CapacityRunnerSHA256: expectedProvenance.CapacityRunnerSHA256,
		ActivationRunnerSHA256: expectedProvenance.ActivationRunnerSHA256, ActivationMetadataSHA256: expectedProvenance.ActivationMetadataSHA256,
		TrustedReviewKeySHA256: expectedProvenance.TrustedReviewKeySHA256, EvidenceSchemaEpoch: expectedProvenance.EvidenceSchemaEpoch,
		GOMAXPROCS: expectedProvenance.GOMAXPROCS, GOMEMLIMIT: expectedProvenance.GOMEMLIMIT,
		Complete: false, EvidenceValid: false, StrictMechanicsValid: false, TerminalValuationValid: false,
		ActivationSatisfied: false, AntiCheatingSatisfied: false, FailureReasons: []string{reason},
	}
	if err := analysis.ValidateSV1DProbeArmProvenance(result, expectedProvenance); err != nil {
		return err
	}
	return publishJSON(out, armResultDocument{SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID, Arm: result})
}

type scoreArmEvidencePath struct {
	RunDir      string
	RenderedDir string
}

type scoreArmEvidencePaths map[string]scoreArmEvidencePath

func scoreArms(out, corpusManifestPath, planPath, treatmentPath, modeOffPath, noRosterPath string, evidencePaths scoreArmEvidencePaths, provenanceInputs strictProvenanceInputs) error {
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
	if err := validateStrictProvenanceInputs(document.Plan, provenanceInputs); err != nil {
		return err
	}
	strictScoring := provenanceInputs.ActivationMetadataPath != ""
	if strictScoring && corpusManifestPath == "" {
		return fmt.Errorf("strict score requires -corpus-manifest")
	}
	expectedProvenance := expectedSV1DProvenance(document.Plan, document.Plan.Treatment, provenanceInputs)
	if err := analysis.ValidateSV1DActivationLaunchProvenance(provenanceInputs.ActivationMetadataPath, expectedProvenance); err != nil {
		return err
	}
	paths := []string{treatmentPath, modeOffPath, noRosterPath}
	armNames := []string{document.Plan.Treatment.Name, document.Plan.ModeOff.Name, document.Plan.NoRoster.Name}
	var corpusEntries []scoreCorpusEntry
	if strictScoring {
		corpusEntries, err = captureScoreCorpus(provenanceInputs.ActivationMetadataPath, armNames, paths)
		if err != nil {
			return err
		}
	}
	arms := make([]analysis.SV1DProbeArmResult, 0, len(paths))
	for index, path := range paths {
		armName := armNames[index]
		var result armResultDocument
		if strictScoring {
			result, err = readContentAddressedArmResult(path, armName, provenanceInputs.ActivationMetadataPath)
		} else {
			result, err = readArmResult(path)
		}
		if err != nil {
			return err
		}
		if result.SchemaVersion != 1 || result.Contract != armResultContract || result.ProbeID != probeID {
			return fmt.Errorf("arm result %s has an invalid contract identity", path)
		}
		if result.Arm.ArmName != armName {
			return fmt.Errorf("arm result %s is for %q, want %q", path, result.Arm.ArmName, armName)
		}
		if err := analysis.ValidateSV1DProbeArmProvenance(result.Arm, expectedSV1DProvenance(document.Plan, armSpecForName(document.Plan, armName), provenanceInputs)); err != nil {
			return fmt.Errorf("arm result %s has invalid strict provenance: %w", path, err)
		}
		if strictScoring && result.Arm.Complete {
			evidencePath, ok := evidencePaths[armName]
			if !ok {
				return fmt.Errorf("strict score has no retained evidence paths for %s", armName)
			}
			if err := validateStrictScoreEvidencePath(evidencePath, armName); err != nil {
				return err
			}
			freshResult, err := reAuditStrictArm(document.Plan, armName, evidencePath, provenanceInputs)
			if err != nil {
				return fmt.Errorf("re-audit retained %s arm: %w", armName, err)
			}
			if !sameSV1DProbeArmResult(result.Arm, freshResult) {
				return fmt.Errorf("arm result %s differs from its retained-evidence re-audit", path)
			}
			result.Arm = freshResult
		}
		arms = append(arms, result.Arm)
	}
	score := analysis.ScoreSV1DProbe(document.Plan, arms)
	if score.PlanSHA256 == "" {
		return fmt.Errorf("could not derive canonical probe plan digest")
	}
	scoreDocumentPath, err := filepath.Abs(out)
	if err != nil {
		return fmt.Errorf("resolve score output path: %w", err)
	}
	scoreDocumentValue := scoreDocument{SchemaVersion: 1, Contract: scoreContract, ProbeID: probeID, PlanSHA256: score.PlanSHA256, Score: score, Arms: arms, ArmCorpus: corpusEntries}
	if err := publishJSON(scoreDocumentPath, scoreDocumentValue); err != nil {
		return err
	}
	if strictScoring {
		if err := publishScoreCorpusManifest(corpusManifestPath, provenanceInputs.ActivationMetadataPath, scoreDocumentPath, score.PlanSHA256, scoreDocumentValue.ArmCorpus); err != nil {
			return err
		}
	}
	if score.Status != analysis.SV1DProbeStatusPass {
		return fmt.Errorf("probe score is not executable: %s", score.Status)
	}
	return nil
}

func activationOutputRoot(metadataPath string) (string, error) {
	absolute, err := filepath.Abs(metadataPath)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(absolute)
	if filepath.Base(clean) != "activation-run-metadata.json" || filepath.Base(filepath.Dir(clean)) != "provenance" {
		return "", fmt.Errorf("activation metadata path must be provenance/activation-run-metadata.json")
	}
	root := filepath.Dir(filepath.Dir(clean))
	if root == string(filepath.Separator) {
		return "", fmt.Errorf("activation metadata path has no output root")
	}
	return root, nil
}

func relativeScorePath(root, path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, filepath.Clean(absolute))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("path %s is outside output root %s", path, root)
	}
	return filepath.ToSlash(relative), nil
}

func readDirectoryEntriesNoSymlink(path string) ([]os.FileInfo, error) {
	fileDescriptor, absolute, err := openNoSymlink(path, true)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileDescriptor), absolute)
	if file == nil {
		_ = syscall.Close(fileDescriptor)
		return nil, fmt.Errorf("could not wrap directory descriptor for %s", path)
	}
	defer file.Close()
	entries, err := file.Readdir(-1)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func captureScoreCorpus(activationMetadataPath string, armNames, resultPaths []string) ([]scoreCorpusEntry, error) {
	if len(armNames) != 3 || len(resultPaths) != len(armNames) {
		return nil, fmt.Errorf("strict score corpus must contain exactly three arm results")
	}
	outputRoot, err := activationOutputRoot(activationMetadataPath)
	if err != nil {
		return nil, err
	}
	resultRoot := filepath.Join(outputRoot, "provenance", "arm-results")
	entries, err := readDirectoryEntriesNoSymlink(resultRoot)
	if err != nil {
		return nil, fmt.Errorf("read strict score corpus directory: %w", err)
	}
	expectedByBase := make(map[string]string, len(armNames))
	for index, armName := range armNames {
		if armName == "" {
			return nil, fmt.Errorf("strict score corpus has an empty arm name")
		}
		absolute, err := filepath.Abs(resultPaths[index])
		if err != nil {
			return nil, err
		}
		absolute = filepath.Clean(absolute)
		if filepath.Dir(absolute) != resultRoot {
			return nil, fmt.Errorf("arm result %s is outside the strict score corpus directory", resultPaths[index])
		}
		baseName := filepath.Base(absolute)
		if previous, duplicate := expectedByBase[baseName]; duplicate {
			return nil, fmt.Errorf("arm result path %s is assigned to both %s and %s", baseName, previous, armName)
		}
		expectedByBase[baseName] = armName
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("strict score corpus contains symlink %s", entry.Name())
		}
		if !entry.Mode().IsRegular() {
			return nil, fmt.Errorf("strict score corpus contains non-regular entry %s", entry.Name())
		}
		if _, expected := expectedByBase[entry.Name()]; !expected {
			return nil, fmt.Errorf("strict score corpus contains unexpected file %s", entry.Name())
		}
		seen[entry.Name()] = struct{}{}
	}
	if len(seen) != len(expectedByBase) {
		return nil, fmt.Errorf("strict score corpus is missing an expected arm result")
	}
	corpus := make([]scoreCorpusEntry, 0, len(armNames))
	for index, armName := range armNames {
		absolute, err := filepath.Abs(resultPaths[index])
		if err != nil {
			return nil, err
		}
		absolute = filepath.Clean(absolute)
		baseName := filepath.Base(absolute)
		if expectedByBase[baseName] != armName {
			return nil, fmt.Errorf("arm result %s is not assigned to %s", resultPaths[index], armName)
		}
		prefix := armName + "-"
		if !strings.HasPrefix(baseName, prefix) || !strings.HasSuffix(baseName, ".json") {
			return nil, fmt.Errorf("arm result %s is not content addressed for %s", resultPaths[index], armName)
		}
		declaredDigest := strings.TrimSuffix(strings.TrimPrefix(baseName, prefix), ".json")
		if !isHexDigest(declaredDigest) {
			return nil, fmt.Errorf("arm result %s has an invalid content digest", resultPaths[index])
		}
		raw, err := readRegularNoSymlink(absolute)
		if err != nil {
			return nil, fmt.Errorf("read strict score corpus arm %s: %w", armName, err)
		}
		digest := sha256.Sum256(raw)
		actualDigest := hex.EncodeToString(digest[:])
		if actualDigest != declaredDigest {
			return nil, fmt.Errorf("strict score corpus arm %s failed its content-addressed digest", armName)
		}
		document, err := readContentAddressedArmResult(absolute, armName, activationMetadataPath)
		if err != nil {
			return nil, err
		}
		if document.SchemaVersion != 1 || document.Contract != armResultContract || document.ProbeID != probeID || document.Arm.ArmName != armName {
			return nil, fmt.Errorf("strict score corpus arm %s has an invalid arm-result identity", armName)
		}
		relative, err := relativeScorePath(outputRoot, absolute)
		if err != nil {
			return nil, err
		}
		corpus = append(corpus, scoreCorpusEntry{Arm: armName, Path: relative, SHA256: actualDigest, Bytes: int64(len(raw))})
	}
	return corpus, nil
}

func scoreCorpusEntriesEqual(left, right []scoreCorpusEntry) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func publishScoreCorpusManifest(path, activationMetadataPath, scorePath, planSHA256 string, corpus []scoreCorpusEntry) error {
	outputRoot, err := activationOutputRoot(activationMetadataPath)
	if err != nil {
		return err
	}
	manifestPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	expectedManifestPath := filepath.Join(outputRoot, "provenance", "score-corpus-manifest.json")
	if filepath.Clean(manifestPath) != expectedManifestPath {
		return fmt.Errorf("score corpus manifest must be %s", expectedManifestPath)
	}
	metadataPath, err := filepath.Abs(activationMetadataPath)
	if err != nil {
		return err
	}
	metadataRelative, err := relativeScorePath(outputRoot, metadataPath)
	if err != nil {
		return err
	}
	scoreAbsolute, err := filepath.Abs(scorePath)
	if err != nil {
		return err
	}
	scoreRelative, err := relativeScorePath(outputRoot, scoreAbsolute)
	if err != nil {
		return err
	}
	if scoreAbsolute == manifestPath {
		return fmt.Errorf("score and score corpus manifest must be different files")
	}
	metadataRaw, err := readRegularNoSymlink(metadataPath)
	if err != nil {
		return fmt.Errorf("read activation metadata for score corpus: %w", err)
	}
	scoreRaw, err := readRegularNoSymlink(scoreAbsolute)
	if err != nil {
		return fmt.Errorf("read score for score corpus: %w", err)
	}
	metadataDigest := sha256.Sum256(metadataRaw)
	scoreDigest := sha256.Sum256(scoreRaw)
	manifest := scoreCorpusManifest{
		SchemaVersion: 1, Contract: scoreCorpusManifestContract, ProbeID: probeID,
		PlanSHA256: planSHA256, ActivationMetadataPath: metadataRelative,
		ActivationMetadataSHA256: hex.EncodeToString(metadataDigest[:]), ScorePath: scoreRelative,
		ScoreSHA256: hex.EncodeToString(scoreDigest[:]), ArmCorpus: corpus,
	}
	if err := publishJSON(manifestPath, manifest); err != nil {
		return fmt.Errorf("publish score corpus manifest: %w", err)
	}
	return verifyScoreCorpusManifest(manifestPath, activationMetadataPath, scorePath, planSHA256)
}

func verifyScoreCorpusManifest(path, activationMetadataPath, scorePath, expectedPlanSHA256 string) error {
	manifestRaw, err := readRegularNoSymlink(path)
	if err != nil {
		return fmt.Errorf("read score corpus manifest: %w", err)
	}
	var manifest scoreCorpusManifest
	if err := decodeStrictJSON(manifestRaw, &manifest); err != nil {
		return fmt.Errorf("decode score corpus manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Contract != scoreCorpusManifestContract || manifest.ProbeID != probeID || !isHexDigest(manifest.PlanSHA256) || !isHexDigest(manifest.ActivationMetadataSHA256) || !isHexDigest(manifest.ScoreSHA256) {
		return fmt.Errorf("score corpus manifest has an invalid contract identity")
	}
	if expectedPlanSHA256 != "" && manifest.PlanSHA256 != expectedPlanSHA256 {
		return fmt.Errorf("score corpus manifest plan digest does not match expected plan")
	}
	outputRoot, err := activationOutputRoot(activationMetadataPath)
	if err != nil {
		return err
	}
	manifestAbsolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if filepath.Clean(manifestAbsolute) != filepath.Join(outputRoot, "provenance", "score-corpus-manifest.json") {
		return fmt.Errorf("score corpus manifest is outside its registered path")
	}
	metadataAbsolute, err := filepath.Abs(activationMetadataPath)
	if err != nil {
		return err
	}
	metadataRelative, err := relativeScorePath(outputRoot, metadataAbsolute)
	if err != nil || manifest.ActivationMetadataPath != metadataRelative {
		return fmt.Errorf("score corpus manifest activation metadata path is not bound")
	}
	metadataRaw, err := readRegularNoSymlink(metadataAbsolute)
	if err != nil {
		return fmt.Errorf("read score corpus activation metadata: %w", err)
	}
	metadataDigest := sha256.Sum256(metadataRaw)
	if manifest.ActivationMetadataSHA256 != hex.EncodeToString(metadataDigest[:]) {
		return fmt.Errorf("score corpus activation metadata digest mismatch")
	}
	scoreAbsolute, err := filepath.Abs(scorePath)
	if err != nil {
		return err
	}
	scoreRelative, err := relativeScorePath(outputRoot, scoreAbsolute)
	if err != nil || manifest.ScorePath != scoreRelative {
		return fmt.Errorf("score corpus manifest score path is not bound")
	}
	scoreRaw, err := readRegularNoSymlink(scoreAbsolute)
	if err != nil {
		return fmt.Errorf("read score corpus score: %w", err)
	}
	scoreDigest := sha256.Sum256(scoreRaw)
	if manifest.ScoreSHA256 != hex.EncodeToString(scoreDigest[:]) {
		return fmt.Errorf("score corpus score digest mismatch")
	}
	var score scoreDocument
	if err := decodeStrictJSON(scoreRaw, &score); err != nil {
		return fmt.Errorf("decode score corpus score: %w", err)
	}
	if score.SchemaVersion != 1 || score.Contract != scoreContract || score.ProbeID != probeID || score.PlanSHA256 != manifest.PlanSHA256 || !scoreCorpusEntriesEqual(score.ArmCorpus, manifest.ArmCorpus) {
		return fmt.Errorf("score document is not bound to the score corpus manifest")
	}
	if len(manifest.ArmCorpus) != 3 {
		return fmt.Errorf("score corpus manifest must contain exactly three arm results")
	}
	armNames := []string{"treatment", "mode-off", "no-roster"}
	resultPaths := make([]string, len(armNames))
	seenArms := make(map[string]struct{}, len(armNames))
	for _, entry := range manifest.ArmCorpus {
		if _, duplicate := seenArms[entry.Arm]; duplicate {
			return fmt.Errorf("score corpus manifest repeats arm %s", entry.Arm)
		}
		seenArms[entry.Arm] = struct{}{}
		index := -1
		for candidateIndex, armName := range armNames {
			if entry.Arm == armName {
				index = candidateIndex
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("score corpus manifest contains unknown arm %s", entry.Arm)
		}
		if entry.Bytes <= 0 || !isHexDigest(entry.SHA256) {
			return fmt.Errorf("score corpus manifest arm %s has an invalid file identity", entry.Arm)
		}
		if !strings.HasPrefix(entry.Path, "provenance/arm-results/") {
			return fmt.Errorf("score corpus manifest arm %s has an invalid relative path", entry.Arm)
		}
		absolute := filepath.Join(outputRoot, filepath.FromSlash(entry.Path))
		relative, err := relativeScorePath(outputRoot, absolute)
		if err != nil || relative != entry.Path || filepath.Dir(absolute) != filepath.Join(outputRoot, "provenance", "arm-results") {
			return fmt.Errorf("score corpus manifest arm %s escapes its registered directory", entry.Arm)
		}
		baseName := filepath.Base(absolute)
		if !strings.HasPrefix(baseName, entry.Arm+"-") || !strings.HasSuffix(baseName, ".json") || strings.TrimSuffix(strings.TrimPrefix(baseName, entry.Arm+"-"), ".json") != entry.SHA256 {
			return fmt.Errorf("score corpus manifest arm %s path does not contain its digest", entry.Arm)
		}
		resultPaths[index] = absolute
	}
	for index, armName := range armNames {
		if _, ok := seenArms[armName]; !ok || resultPaths[index] == "" {
			return fmt.Errorf("score corpus manifest omits arm %s", armName)
		}
	}
	captured, err := captureScoreCorpus(activationMetadataPath, armNames, resultPaths)
	if err != nil {
		return err
	}
	if !scoreCorpusEntriesEqual(captured, manifest.ArmCorpus) {
		return fmt.Errorf("score corpus manifest arm identities do not match retained files")
	}
	return nil
}

func validateStrictScoreEvidencePath(path scoreArmEvidencePath, armName string) error {
	if path.RunDir == "" || path.RenderedDir == "" {
		return fmt.Errorf("strict score requires retained run and rendered evidence paths for %s", armName)
	}
	if err := validateExistingDirectoryPath(path.RunDir); err != nil {
		return fmt.Errorf("retained run directory for %s: %w", armName, err)
	}
	if err := validateExistingDirectoryPath(path.RenderedDir); err != nil {
		return fmt.Errorf("retained rendered directory for %s: %w", armName, err)
	}
	return nil
}

func reAuditStrictArm(plan analysis.SV1DProbePlan, armName string, evidencePath scoreArmEvidencePath, inputs strictProvenanceInputs) (analysis.SV1DProbeArmResult, error) {
	spec, contract, treatment, err := planArm(plan, armName)
	if err != nil {
		return analysis.SV1DProbeArmResult{}, err
	}
	run, err := analysis.Open(evidencePath.RunDir)
	if err != nil {
		return analysis.SV1DProbeArmResult{}, err
	}
	return run.AuditSV1DProbeArm(analysis.SV1DProbeArmAuditOptions{
		Spec: spec, Contract: contract, Treatment: treatment, PlanSHA256: inputs.PlanSHA256,
		Activation: analysis.CDFActivationOptions{
			Contract: contract, EvidenceDir: evidencePath.RunDir, RenderedEvidenceDir: evidencePath.RenderedDir,
			ExpectedProvenance: expectedSV1DProvenance(plan, spec, inputs),
		},
	})
}

func sameSV1DProbeArmResult(left, right analysis.SV1DProbeArmResult) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftRaw, rightRaw)
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
	fileDescriptor, absolute, err := openNoSymlink(path, false)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileDescriptor), absolute)
	if file == nil {
		_ = syscall.Close(fileDescriptor)
		return nil, fmt.Errorf("could not wrap file descriptor for %s", path)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", path)
	}
	return io.ReadAll(file)
}

func validateExistingDirectoryPath(path string) error {
	fileDescriptor, _, err := openNoSymlink(path, true)
	if err != nil {
		return err
	}
	return syscall.Close(fileDescriptor)
}

func openNoSymlink(path string, directory bool) (int, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return -1, "", err
	}
	if !filepath.IsAbs(absolute) || absolute == string(filepath.Separator) {
		return -1, absolute, fmt.Errorf("path is not a non-root absolute path: %s", path)
	}
	components := strings.Split(strings.TrimPrefix(absolute, string(filepath.Separator)), string(filepath.Separator))
	directoryDescriptor, err := syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, absolute, fmt.Errorf("open path root: %w", err)
	}
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = syscall.Close(directoryDescriptor)
			return -1, absolute, fmt.Errorf("path contains an invalid component: %s", path)
		}
		isFinal := index == len(components)-1
		if isFinal {
			flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
			if directory {
				flags |= syscall.O_DIRECTORY
			}
			fileDescriptor, openErr := syscall.Openat(directoryDescriptor, component, flags, 0)
			_ = syscall.Close(directoryDescriptor)
			if openErr != nil {
				return -1, absolute, openErr
			}
			return fileDescriptor, absolute, nil
		}
		nextDirectoryDescriptor, openErr := syscall.Openat(directoryDescriptor, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = syscall.Close(directoryDescriptor)
			return -1, absolute, openErr
		}
		_ = syscall.Close(directoryDescriptor)
		directoryDescriptor = nextDirectoryDescriptor
	}
	_ = syscall.Close(directoryDescriptor)
	return -1, absolute, fmt.Errorf("path has no final component: %s", path)
}

func readArmResult(path string) (armResultDocument, error) {
	var document armResultDocument
	if err := readStrictJSON(path, &document); err != nil {
		return armResultDocument{}, fmt.Errorf("read arm result %s: %w", path, err)
	}
	return document, nil
}

func readContentAddressedArmResult(path, armName, activationMetadataPath string) (armResultDocument, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return armResultDocument{}, err
	}
	metadataPath, err := filepath.Abs(activationMetadataPath)
	if err != nil {
		return armResultDocument{}, err
	}
	outputRoot := filepath.Dir(filepath.Dir(filepath.Clean(metadataPath)))
	expectedRoot := filepath.Join(outputRoot, "provenance", "arm-results")
	if filepath.Dir(filepath.Clean(absolutePath)) != expectedRoot {
		return armResultDocument{}, fmt.Errorf("arm result %s is outside the activation arm-result root", path)
	}
	baseName := filepath.Base(absolutePath)
	prefix := armName + "-"
	if !strings.HasPrefix(baseName, prefix) || !strings.HasSuffix(baseName, ".json") {
		return armResultDocument{}, fmt.Errorf("arm result %s is not content addressed for %s", path, armName)
	}
	digest := strings.TrimSuffix(strings.TrimPrefix(baseName, prefix), ".json")
	if !isHexDigest(digest) {
		return armResultDocument{}, fmt.Errorf("arm result %s has an invalid content digest", path)
	}
	raw, err := readRegularNoSymlink(absolutePath)
	if err != nil {
		return armResultDocument{}, fmt.Errorf("read retained arm result %s: %w", path, err)
	}
	actualDigest := sha256.Sum256(raw)
	if hex.EncodeToString(actualDigest[:]) != digest {
		return armResultDocument{}, fmt.Errorf("retained arm result %s failed its content-addressed digest", path)
	}
	var document armResultDocument
	if err := decodeStrictJSON(raw, &document); err != nil {
		return armResultDocument{}, fmt.Errorf("decode retained arm result %s: %w", path, err)
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
	raw, err := readRegularNoSymlink(path)
	if err != nil {
		return err
	}
	return decodeStrictJSON(raw, target)
}

func decodeStrictJSON(raw []byte, target any) error {
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
