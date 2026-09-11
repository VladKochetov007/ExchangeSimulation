package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	SV1DProbeStatusInvalidEvidence       = "INVALID_EVIDENCE"
	SV1DProbeStatusIncompleteArm         = "INCOMPLETE_ARM"
	SV1DProbeStatusTreatmentNotActivated = "TREATMENT_NOT_ACTIVATED"
	SV1DProbeStatusAntiCheatingRejected  = "ANTI_CHEATING_REJECTED"
	SV1DProbeStatusNoDirectionalEffect   = "NO_DIRECTIONAL_EFFECT"
	SV1DProbeStatusPass                  = "DEVELOPMENT_PROBE_PASSES"
	sv1dProbePlanHashDomain              = "v2-r2-sv1d-probe-plan-v2\x00"
)

// SV1DProbePlan contains the precommitted comparison boundary for one
// development-only tri-arm probe. It is intentionally a value object: callers
// can define a successor plan without editing an experiment registry.
type SV1DProbePlan struct {
	ExperimentID                            string           `json:"experiment_id"`
	HypothesisID                            string           `json:"hypothesis_id"`
	Seed                                    int64            `json:"seed"`
	Horizon                                 string           `json:"horizon"`
	ProbeDurationNano                       int64            `json:"probe_duration_nano"`
	VenueIDs                                []string         `json:"venue_ids"`
	MaxUninterruptedNonTwoSidedDurationNano int64            `json:"max_uninterrupted_non_two_sided_duration_nano"`
	AnalyzerSHA256                          string           `json:"analyzer_sha256"`
	RendererSHA256                          string           `json:"renderer_sha256"`
	Treatment                               SV1DProbeArmSpec `json:"treatment"`
	ModeOff                                 SV1DProbeArmSpec `json:"mode_off"`
	NoRoster                                SV1DProbeArmSpec `json:"no_roster"`
}

// SV1DProbeArmSpec is the externally committed identity of one arm.
type SV1DProbeArmSpec struct {
	Name           string `json:"name"`
	ExperimentID   string `json:"experiment_id"`
	HypothesisID   string `json:"hypothesis_id"`
	ConfigSHA256   string `json:"config_sha256"`
	SourceRevision string `json:"source_revision"`
	BinarySHA256   string `json:"binary_sha256"`
	AnalyzerSHA256 string `json:"analyzer_sha256"`
	RendererSHA256 string `json:"renderer_sha256"`
}

// SV1DProbeArmResult is the typed, independently reconstructed result of one
// arm. Controls do not need supplier activation; they do need complete,
// strict, identity-bound evidence before they can enter a comparison.
type SV1DProbeArmResult struct {
	ArmName                   string                       `json:"arm_name"`
	ExperimentID              string                       `json:"experiment_id"`
	HypothesisID              string                       `json:"hypothesis_id"`
	ConfigSHA256              string                       `json:"config_sha256"`
	SourceRevision            string                       `json:"source_revision"`
	BinarySHA256              string                       `json:"binary_sha256"`
	AnalyzerSHA256            string                       `json:"analyzer_sha256"`
	RendererSHA256            string                       `json:"renderer_sha256"`
	PlanSHA256                string                       `json:"plan_sha256"`
	TreeRevision              string                       `json:"tree_revision,omitempty"`
	ReviewAttestationSHA256   string                       `json:"review_attestation_sha256,omitempty"`
	ReviewReportSHA256        string                       `json:"review_report_sha256,omitempty"`
	CapacityAttestationSHA256 string                       `json:"capacity_attestation_sha256,omitempty"`
	CapacityRecordsSHA256     string                       `json:"capacity_records_sha256,omitempty"`
	CapacityRunnerSHA256      string                       `json:"capacity_runner_sha256,omitempty"`
	ActivationRunnerSHA256    string                       `json:"activation_runner_sha256,omitempty"`
	ActivationMetadataSHA256  string                       `json:"activation_metadata_sha256,omitempty"`
	TrustedReviewKeySHA256    string                       `json:"trusted_review_key_sha256,omitempty"`
	EvidenceSchemaEpoch       uint32                       `json:"evidence_schema_epoch,omitempty"`
	GOMAXPROCS                int                          `json:"gomaxprocs,omitempty"`
	GOMEMLIMIT                string                       `json:"gomemlimit,omitempty"`
	Complete                  bool                         `json:"complete"`
	EvidenceValid             bool                         `json:"evidence_valid"`
	StrictMechanicsValid      bool                         `json:"strict_mechanics_valid"`
	TerminalValuationValid    bool                         `json:"terminal_valuation_valid"`
	ActivationSatisfied       bool                         `json:"activation_satisfied"`
	AntiCheatingSatisfied     bool                         `json:"anti_cheating_satisfied"`
	Venues                    []CDFVenueConcentrationAudit `json:"venues"`
	FailureReasons            []string                     `json:"failure_reasons,omitempty"`
}

// SV1DProbeArmAuditOptions binds one arm audit to identities supplied by the
// launcher. The expected provenance is intentionally separate from the run;
// a run cannot authenticate its own binary or config identity.
type SV1DProbeArmAuditOptions struct {
	Spec       SV1DProbeArmSpec
	Contract   CDFActivationContract
	Activation CDFActivationOptions
	Treatment  bool
	PlanSHA256 string
}

// AuditSV1DProbeArm produces one scorer input from a production run. Controls
// use the public-book audit, while the treatment additionally has to satisfy
// the finite supplier contract. A complete arm may still fail activation or
// anti-cheating predicates; those are scientific outcomes, not evidence gaps.
func (r *Run) AuditSV1DProbeArm(options SV1DProbeArmAuditOptions) (SV1DProbeArmResult, error) {
	result := SV1DProbeArmResult{
		ArmName: options.Spec.Name, ExperimentID: options.Spec.ExperimentID,
		HypothesisID: options.Spec.HypothesisID, ConfigSHA256: options.Spec.ConfigSHA256,
		SourceRevision: options.Spec.SourceRevision, BinarySHA256: options.Spec.BinarySHA256,
		AnalyzerSHA256: options.Spec.AnalyzerSHA256, RendererSHA256: options.Spec.RendererSHA256,
		PlanSHA256:                options.PlanSHA256,
		TreeRevision:              options.Activation.ExpectedProvenance.TreeRevision,
		ReviewAttestationSHA256:   options.Activation.ExpectedProvenance.ReviewAttestationSHA256,
		ReviewReportSHA256:        options.Activation.ExpectedProvenance.ReviewReportSHA256,
		CapacityAttestationSHA256: options.Activation.ExpectedProvenance.CapacityAttestationSHA256,
		CapacityRecordsSHA256:     options.Activation.ExpectedProvenance.CapacityRecordsSHA256,
		CapacityRunnerSHA256:      options.Activation.ExpectedProvenance.CapacityRunnerSHA256,
		ActivationRunnerSHA256:    options.Activation.ExpectedProvenance.ActivationRunnerSHA256,
		ActivationMetadataSHA256:  options.Activation.ExpectedProvenance.ActivationMetadataSHA256,
		TrustedReviewKeySHA256:    options.Activation.ExpectedProvenance.TrustedReviewKeySHA256,
		EvidenceSchemaEpoch:       options.Activation.ExpectedProvenance.EvidenceSchemaEpoch,
		GOMAXPROCS:                options.Activation.ExpectedProvenance.GOMAXPROCS,
		GOMEMLIMIT:                options.Activation.ExpectedProvenance.GOMEMLIMIT,
	}
	if r == nil {
		return result, fmt.Errorf("SV1D arm audit has a nil run")
	}
	if options.Spec.Name == "" || options.Spec.ExperimentID == "" || options.Spec.HypothesisID == "" ||
		!isSV1DHexDigest(options.Spec.ConfigSHA256) || !isSV1DHexDigest(options.Spec.BinarySHA256) ||
		!isCDFHex(options.Spec.SourceRevision, 20) {
		return result, fmt.Errorf("SV1D arm spec has incomplete immutable identity")
	}
	expected := options.Activation.ExpectedProvenance
	if expected.ConfigSHA256 != options.Spec.ConfigSHA256 || expected.SourceRevision != options.Spec.SourceRevision || expected.BinarySHA256 != options.Spec.BinarySHA256 {
		return result, fmt.Errorf("SV1D arm spec and expected audit provenance disagree")
	}
	if !options.Activation.AllowLegacyJSON && !isSV1DHexDigest(options.PlanSHA256) {
		return result, fmt.Errorf("SV1D strict arm audit requires a canonical plan digest")
	}
	if !options.Activation.AllowLegacyJSON {
		if err := validateSV1DStrictExpectedProvenance(expected); err != nil {
			return result, err
		}
		if expected.PlanSHA256 != options.PlanSHA256 {
			return result, fmt.Errorf("SV1D arm spec and expected plan provenance disagree")
		}
		if !isSV1DHexDigest(options.Spec.AnalyzerSHA256) || !isSV1DHexDigest(options.Spec.RendererSHA256) {
			return result, fmt.Errorf("SV1D arm spec has incomplete successor tool identity")
		}
		if expected.RendererSHA256 != options.Spec.RendererSHA256 || expected.RendererSourceRevision != options.Spec.SourceRevision ||
			expected.RendererSourceModified || expected.RendererGOOS != "linux" || expected.RendererGOARCH != "amd64" ||
			expected.RendererGOAMD64 != "v1" || !expected.RendererTrimpath || expected.RendererCGOEnabled != "0" {
			return result, fmt.Errorf("SV1D arm spec and expected renderer provenance disagree")
		}
		if err := validateSV1DRendererAttestation(options.Activation.RenderedEvidenceDir, expected); err != nil {
			return result, err
		}
	}
	evidenceDir := options.Activation.EvidenceDir
	if evidenceDir == "" {
		evidenceDir = r.Dir
	}
	_, metadata, err := loadCDFActivationIdentity(evidenceDir)
	if err != nil {
		return result, err
	}
	if !options.Activation.AllowLegacyJSON {
		if err := validateSV1DActivationArmProvenance(evidenceDir, options.Activation.RenderedEvidenceDir, options.Spec, options); err != nil {
			return result, err
		}
	}
	terminalErr := validateCDFTerminalValuation(r, metadata, options.Contract)
	result.TerminalValuationValid = terminalErr == nil
	if terminalErr != nil {
		result.FailureReasons = append(result.FailureReasons, terminalErr.Error())
	}
	if options.Treatment {
		audit, err := r.AuditCDFLiquidityActivation(options.Activation)
		if err != nil {
			return result, err
		}
		result.Complete = true
		result.EvidenceValid = audit.EvidenceValid
		result.StrictMechanicsValid = audit.EvidenceValid && !options.Activation.AllowLegacyJSON
		result.ActivationSatisfied = audit.ActivationSatisfied
		result.AntiCheatingSatisfied = audit.AntiCheatingSatisfied
		result.Venues = append([]CDFVenueConcentrationAudit(nil), audit.Venues...)
		result.FailureReasons = appendCDFActivationFailures(result.FailureReasons, audit.Checks)
		return result, nil
	}
	audit, err := r.AuditCDFBookAvailability(options.Activation)
	if err != nil {
		return result, err
	}
	result.Complete = true
	result.EvidenceValid = audit.EvidenceValid
	result.StrictMechanicsValid = audit.StrictMechanicsValid
	result.ActivationSatisfied = false
	result.AntiCheatingSatisfied = true
	result.Venues = append([]CDFVenueConcentrationAudit(nil), audit.Venues...)
	result.FailureReasons = appendCDFActivationFailures(result.FailureReasons, audit.Checks)
	return result, nil
}

func appendCDFActivationFailures(existing []string, checks []CDFActivationCheck) []string {
	for _, check := range checks {
		if check.Failure == "" {
			continue
		}
		existing = append(existing, check.Failure)
	}
	return existing
}

type sv1dRendererAttestation struct {
	SchemaVersion             int    `json:"schema_version"`
	Contract                  string `json:"contract"`
	RendererSHA256            string `json:"renderer_sha256"`
	RendererSourceRevision    string `json:"renderer_source_revision"`
	RendererSourceModified    bool   `json:"renderer_source_modified"`
	RendererGOOS              string `json:"renderer_goos"`
	RendererGOARCH            string `json:"renderer_goarch"`
	RendererGOAMD64           string `json:"renderer_goamd64"`
	RendererGoVersion         string `json:"renderer_go_version"`
	RendererTrimpath          bool   `json:"renderer_trimpath"`
	RendererCGOEnabled        string `json:"renderer_cgo_enabled"`
	RenderedAttestationSHA256 string `json:"rendered_attestation_sha256"`
}

func validateSV1DRendererAttestation(renderedDir string, expected CDFExpectedProvenance) error {
	if renderedDir == "" {
		return fmt.Errorf("SV1D arm audit requires a rendered evidence directory")
	}
	raw, err := readSV1DRegularFile(filepath.Join(renderedDir, "renderer-attestation.json"))
	if err != nil {
		return fmt.Errorf("SV1D renderer attestation: read: %w", err)
	}
	mainRaw, err := readSV1DRegularFile(filepath.Join(renderedDir, "rendered-binary-evidence-attestation.json"))
	if err != nil {
		return fmt.Errorf("SV1D renderer attestation: read rendered evidence attestation: %w", err)
	}
	return validateSV1DRendererAttestationRaw(raw, mainRaw, expected)
}

func validateSV1DRendererAttestationRaw(raw, mainRaw []byte, expected CDFExpectedProvenance) error {
	var attestation sv1dRendererAttestation
	if err := rejectSV1DDuplicateJSONKeys(raw); err != nil {
		return fmt.Errorf("SV1D renderer attestation: malformed JSON: %w", err)
	}
	if err := decodeRequiredJSON(raw, &attestation,
		"schema_version", "contract", "renderer_sha256", "renderer_source_revision",
		"renderer_source_modified", "renderer_goos", "renderer_goarch", "renderer_goamd64",
		"renderer_go_version", "renderer_trimpath", "renderer_cgo_enabled", "rendered_attestation_sha256"); err != nil {
		return fmt.Errorf("SV1D renderer attestation: decode: %w", err)
	}
	if attestation.SchemaVersion != 1 || attestation.Contract != "v2-r2-sv1d-renderer-attestation-v1" ||
		attestation.RendererSHA256 != expected.RendererSHA256 || attestation.RendererSourceRevision != expected.RendererSourceRevision ||
		attestation.RendererSourceModified != expected.RendererSourceModified || attestation.RendererGOOS != expected.RendererGOOS ||
		attestation.RendererGOARCH != expected.RendererGOARCH || attestation.RendererGOAMD64 != expected.RendererGOAMD64 ||
		(attestation.RendererGoVersion == "" || !strings.HasPrefix(attestation.RendererGoVersion, "go1.27")) ||
		(expected.RendererGoVersion != "" && attestation.RendererGoVersion != expected.RendererGoVersion) ||
		attestation.RendererTrimpath != expected.RendererTrimpath ||
		attestation.RendererCGOEnabled != expected.RendererCGOEnabled || !isSV1DHexDigest(attestation.RendererSHA256) ||
		!isCDFHex(attestation.RendererSourceRevision, 20) || !isSV1DHexDigest(attestation.RenderedAttestationSHA256) {
		return fmt.Errorf("SV1D renderer attestation does not match the externally expected clean renderer")
	}
	mainAttestationDigest := sha256.Sum256(mainRaw)
	mainAttestationSHA256 := hex.EncodeToString(mainAttestationDigest[:])
	if mainAttestationSHA256 != attestation.RenderedAttestationSHA256 {
		return fmt.Errorf("SV1D renderer attestation is not bound to rendered evidence")
	}
	return nil
}

const (
	sv1dActivationRunnerContract   = "v2-r2-sv1d-activation-runner-v2"
	sv1dActivationMetadataContract = "v2-r2-sv1d-activation-run-metadata-v2"
)

type sv1dActivationRunMetadata struct {
	SchemaVersion             int      `json:"schema_version"`
	Contract                  string   `json:"contract"`
	DevelopmentOnly           bool     `json:"development_only"`
	ScientificResultEligible  bool     `json:"scientific_result_eligible"`
	SourceRevision            string   `json:"source_revision"`
	TreeRevision              string   `json:"tree_revision"`
	ProbeID                   string   `json:"probe_id"`
	PlanSHA256                string   `json:"plan_sha256"`
	ReviewAttestationSHA256   string   `json:"review_attestation_sha256"`
	ReviewReportSHA256        string   `json:"review_report_sha256"`
	CapacityAttestationSHA256 string   `json:"capacity_attestation_sha256"`
	CapacityRecordsSHA256     string   `json:"capacity_records_sha256"`
	TrustedReviewKeySHA256    string   `json:"trusted_review_key_sha256"`
	CapacityRoot              string   `json:"capacity_root"`
	ActivationRunnerSHA256    string   `json:"activation_runner_sha256"`
	CapacityRunnerSHA256      string   `json:"capacity_runner_sha256"`
	SimulatorSHA256           string   `json:"simulator_sha256"`
	AnalyzerSHA256            string   `json:"analyzer_sha256"`
	RendererSHA256            string   `json:"renderer_sha256"`
	EvidenceFormat            string   `json:"evidence_format"`
	EvidenceSchemaEpoch       uint32   `json:"evidence_schema_epoch"`
	LogMode                   string   `json:"log_mode"`
	GOMAXPROCS                int      `json:"gomaxprocs"`
	GOMEMLIMIT                string   `json:"gomemlimit"`
	OutputRoot                string   `json:"output_root"`
	OutputParent              string   `json:"output_parent"`
	Arms                      []string `json:"arms"`
	HoldoutsConsumed          []string `json:"holdouts_consumed"`
}

type sv1dActivationArmMetadata struct {
	SchemaVersion             int      `json:"schema_version"`
	RunnerContract            string   `json:"runner_contract"`
	ProbeID                   string   `json:"probe_id"`
	Arm                       string   `json:"arm"`
	ExperimentID              string   `json:"experiment_id"`
	ConfigExperimentID        string   `json:"config_experiment_id"`
	HypothesisID              string   `json:"hypothesis_id"`
	Seed                      int64    `json:"seed"`
	SimulatedHorizon          string   `json:"simulated_horizon"`
	SimulationStartNano       int64    `json:"simulation_start_nano"`
	SimulationEndNano         int64    `json:"simulation_end_nano"`
	ConfigSHA256              string   `json:"config_sha256"`
	BinarySHA256              string   `json:"binary_sha256"`
	GitRevision               string   `json:"git_revision"`
	TreeRevision              string   `json:"tree_revision"`
	BinaryPath                string   `json:"binary_path"`
	BinaryGoVersion           string   `json:"binary_go_version"`
	BinaryGOOS                string   `json:"binary_goos"`
	BinaryGOARCH              string   `json:"binary_goarch"`
	BinaryGOAMD64             string   `json:"binary_goamd64"`
	AnalyzerSHA256            string   `json:"analyzer_sha256"`
	RendererSHA256            string   `json:"renderer_sha256"`
	RunnerSHA256              string   `json:"runner_sha256"`
	ReviewAttestationSHA256   string   `json:"review_attestation_sha256"`
	ReviewReportSHA256        string   `json:"review_report_sha256"`
	CapacityAttestationSHA256 string   `json:"capacity_attestation_sha256"`
	CapacityRecordsSHA256     string   `json:"capacity_records_sha256"`
	TrustedReviewKeySHA256    string   `json:"trusted_review_key_sha256"`
	LogMode                   string   `json:"log_mode"`
	EvidenceFormat            string   `json:"evidence_format"`
	EvidenceSchemaEpoch       uint32   `json:"evidence_schema_epoch"`
	GOMAXPROCS                int      `json:"gomaxprocs"`
	GOMEMLIMIT                string   `json:"gomemlimit"`
	OutputDir                 string   `json:"output_dir"`
	Holdout                   bool     `json:"holdout"`
	Command                   []string `json:"command"`
	RawLogPolicy              string   `json:"raw_log_policy"`
}

func validateSV1DStrictExpectedProvenance(expected CDFExpectedProvenance) error {
	if err := expected.validate(); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"analyzer":                     expected.AnalyzerSHA256,
		"renderer":                     expected.RendererSHA256,
		"plan":                         expected.PlanSHA256,
		"treatment config":             expected.TreatmentConfigSHA256,
		"parent registration":          expected.ParentRegistrationSHA256,
		"amendment":                    expected.AmendmentSHA256,
		"mode-off config":              expected.ModeOffConfigSHA256,
		"no-roster config":             expected.NoRosterConfigSHA256,
		"review attestation":           expected.ReviewAttestationSHA256,
		"review report":                expected.ReviewReportSHA256,
		"capacity attestation":         expected.CapacityAttestationSHA256,
		"capacity measurement records": expected.CapacityRecordsSHA256,
		"capacity runner":              expected.CapacityRunnerSHA256,
		"activation runner":            expected.ActivationRunnerSHA256,
		"activation metadata":          expected.ActivationMetadataSHA256,
		"trusted review key":           expected.TrustedReviewKeySHA256,
	} {
		if !isSV1DHexDigest(value) {
			return fmt.Errorf("SV1D strict provenance has an invalid %s digest", name)
		}
	}
	if !isCDFHex(expected.TreeRevision, 20) {
		return fmt.Errorf("SV1D strict provenance has an invalid tree revision")
	}
	if !isCDFHex(expected.RendererSourceRevision, 20) || expected.RendererSourceRevision != expected.SourceRevision {
		return fmt.Errorf("SV1D strict provenance has an invalid renderer source revision")
	}
	if !absoluteCleanPath(expected.ActivationMetadataPath) {
		return fmt.Errorf("SV1D strict provenance has an invalid activation metadata path")
	}
	if expected.EvidenceSchemaEpoch != 4 || expected.GOMAXPROCS != 2 || expected.GOMEMLIMIT != "4GiB" {
		return fmt.Errorf("SV1D strict provenance has an unregistered evidence epoch or runtime envelope")
	}
	if expected.RendererSourceModified || expected.RendererGOOS != "linux" || expected.RendererGOARCH != "amd64" ||
		expected.RendererGOAMD64 != "v1" || !expected.RendererTrimpath || expected.RendererCGOEnabled != "0" {
		return fmt.Errorf("SV1D strict provenance has an unsafe renderer identity")
	}
	return nil
}

func (p CDFExpectedProvenance) hasSV1DStrictProvenance() bool {
	return p.AnalyzerSHA256 != "" || p.TreeRevision != "" || p.PlanSHA256 != "" ||
		p.ReviewAttestationSHA256 != "" || p.ReviewReportSHA256 != "" ||
		p.CapacityAttestationSHA256 != "" || p.CapacityRecordsSHA256 != "" ||
		p.CapacityRunnerSHA256 != "" || p.ActivationRunnerSHA256 != "" ||
		p.ActivationMetadataSHA256 != "" || p.ActivationMetadataPath != "" ||
		p.TrustedReviewKeySHA256 != ""
}

// ValidateSV1DProbeArmProvenance checks the fields that are copied into an arm
// result against launch identities supplied from outside the run. Legacy
// historical results have no successor fields and remain accepted by the
// legacy analyzer; strict successor results must carry every field.
func ValidateSV1DProbeArmProvenance(arm SV1DProbeArmResult, expected CDFExpectedProvenance) error {
	if !expected.hasSV1DStrictProvenance() {
		return nil
	}
	if err := validateSV1DStrictExpectedProvenance(expected); err != nil {
		return err
	}
	if arm.TreeRevision != expected.TreeRevision || arm.PlanSHA256 != expected.PlanSHA256 ||
		arm.ReviewAttestationSHA256 != expected.ReviewAttestationSHA256 || arm.ReviewReportSHA256 != expected.ReviewReportSHA256 ||
		arm.CapacityAttestationSHA256 != expected.CapacityAttestationSHA256 || arm.CapacityRecordsSHA256 != expected.CapacityRecordsSHA256 ||
		arm.CapacityRunnerSHA256 != expected.CapacityRunnerSHA256 || arm.ActivationRunnerSHA256 != expected.ActivationRunnerSHA256 ||
		arm.ActivationMetadataSHA256 != expected.ActivationMetadataSHA256 || arm.TrustedReviewKeySHA256 != expected.TrustedReviewKeySHA256 ||
		arm.EvidenceSchemaEpoch != expected.EvidenceSchemaEpoch || arm.GOMAXPROCS != expected.GOMAXPROCS || arm.GOMEMLIMIT != expected.GOMEMLIMIT {
		return fmt.Errorf("SV1D arm result does not match strict launch provenance")
	}
	return nil
}

func readSV1DActivationMetadata(path string) (sv1dActivationRunMetadata, []byte, error) {
	var metadata sv1dActivationRunMetadata
	raw, err := readSV1DRegularFile(path)
	if err != nil {
		return metadata, nil, fmt.Errorf("read SV1D activation metadata: %w", err)
	}
	fields := []string{
		"schema_version", "contract", "development_only", "scientific_result_eligible",
		"source_revision", "tree_revision", "probe_id", "plan_sha256",
		"review_attestation_sha256", "review_report_sha256", "capacity_attestation_sha256",
		"capacity_records_sha256", "trusted_review_key_sha256", "capacity_root",
		"activation_runner_sha256", "capacity_runner_sha256", "simulator_sha256",
		"analyzer_sha256", "renderer_sha256", "evidence_format", "evidence_schema_epoch",
		"log_mode", "gomaxprocs", "gomemlimit", "output_root", "output_parent",
		"arms", "holdouts_consumed",
	}
	if err := decodeStrictSV1DCapacityJSON(raw, &metadata, fields...); err != nil {
		return metadata, nil, fmt.Errorf("decode SV1D activation metadata: %w", err)
	}
	return metadata, raw, nil
}

func validateSV1DActivationMetadata(metadataPath string, expected CDFExpectedProvenance) error {
	metadata, raw, err := readSV1DActivationMetadata(metadataPath)
	if err != nil {
		return err
	}
	if sha256DigestHex(raw) != expected.ActivationMetadataSHA256 {
		return fmt.Errorf("SV1D activation metadata digest does not match the externally expected launch record")
	}
	cleanMetadataPath := filepath.Clean(metadataPath)
	outputRootFromPath := filepath.Dir(filepath.Dir(cleanMetadataPath))
	if !absoluteCleanPath(cleanMetadataPath) || cleanMetadataPath != expected.ActivationMetadataPath ||
		metadata.OutputRoot != outputRootFromPath {
		return fmt.Errorf("SV1D activation metadata path is not bound to its declared output root")
	}
	if metadata.SchemaVersion != 2 || metadata.Contract != sv1dActivationMetadataContract ||
		!metadata.DevelopmentOnly || metadata.ScientificResultEligible ||
		metadata.SourceRevision != expected.SourceRevision || metadata.TreeRevision != expected.TreeRevision ||
		metadata.ProbeID != "v2-r2-sv1d-activation-659" || metadata.PlanSHA256 != expected.PlanSHA256 ||
		metadata.ReviewAttestationSHA256 != expected.ReviewAttestationSHA256 || metadata.ReviewReportSHA256 != expected.ReviewReportSHA256 ||
		metadata.CapacityAttestationSHA256 != expected.CapacityAttestationSHA256 || metadata.CapacityRecordsSHA256 != expected.CapacityRecordsSHA256 ||
		metadata.TrustedReviewKeySHA256 != expected.TrustedReviewKeySHA256 || metadata.ActivationRunnerSHA256 != expected.ActivationRunnerSHA256 ||
		metadata.CapacityRunnerSHA256 != expected.CapacityRunnerSHA256 || metadata.SimulatorSHA256 != expected.BinarySHA256 ||
		metadata.AnalyzerSHA256 != expected.AnalyzerSHA256 || metadata.RendererSHA256 != expected.RendererSHA256 ||
		metadata.EvidenceFormat != "evstream_v3" || metadata.EvidenceSchemaEpoch != expected.EvidenceSchemaEpoch ||
		metadata.LogMode != "full" || metadata.GOMAXPROCS != expected.GOMAXPROCS || metadata.GOMEMLIMIT != expected.GOMEMLIMIT ||
		len(metadata.Arms) != 3 || !sameSV1DStrings(metadata.Arms, []string{"treatment", "mode-off", "no-roster"}) ||
		len(metadata.HoldoutsConsumed) != 0 {
		return fmt.Errorf("SV1D activation metadata does not match the registered strict launch contract")
	}
	if !absoluteCleanPath(metadata.OutputRoot) || !absoluteCleanPath(metadata.OutputParent) ||
		!absoluteCleanPath(metadata.CapacityRoot) || filepath.Dir(metadata.OutputRoot) != metadata.OutputParent {
		return fmt.Errorf("SV1D activation metadata has an invalid output or capacity path")
	}
	return nil
}

// ValidateSV1DActivationMetadata exposes the strict launch-record check to
// the scorer. A legacy expected identity intentionally remains a no-op so
// historical JSON reconstructions keep their pre-successor contract.
func ValidateSV1DActivationMetadata(metadataPath string, expected CDFExpectedProvenance) error {
	if !expected.hasSV1DStrictProvenance() {
		return nil
	}
	if err := validateSV1DStrictExpectedProvenance(expected); err != nil {
		return err
	}
	return validateSV1DActivationMetadata(metadataPath, expected)
}

// ValidateSV1DActivationLaunchProvenance rechecks the immutable launch bundle
// at scoring time. This prevents a valid arm audit from being combined later
// with a replaced review, capacity record, or retained tool.
func ValidateSV1DActivationLaunchProvenance(metadataPath string, expected CDFExpectedProvenance) error {
	if !expected.hasSV1DStrictProvenance() {
		return nil
	}
	if err := validateSV1DStrictExpectedProvenance(expected); err != nil {
		return err
	}
	metadata, _, err := readSV1DActivationMetadata(metadataPath)
	if err != nil {
		return err
	}
	if err := validateSV1DActivationMetadata(metadataPath, expected); err != nil {
		return err
	}
	outputRoot := metadata.OutputRoot
	if err := validateSV1DReviewRetention(outputRoot, expected); err != nil {
		return err
	}
	if err := validateSV1DCapacityRetention(outputRoot, metadata, expected); err != nil {
		return err
	}
	return nil
}

func validateSV1DReviewRetention(outputRoot string, expected CDFExpectedProvenance) error {
	attestationPath := filepath.Join(outputRoot, "provenance", "review-attestation.json")
	reportPath := filepath.Join(outputRoot, "provenance", "review-report.md")
	keyPath := filepath.Join(outputRoot, "provenance", "trusted-review-key.raw")
	parentPath := filepath.Join(outputRoot, "provenance", "parent-registration.md")
	amendmentPath := filepath.Join(outputRoot, "provenance", "amendment.md")
	configPaths := map[string]string{
		"treatment": filepath.Join(outputRoot, "configs", "target-treatment.json"),
		"mode-off":  filepath.Join(outputRoot, "configs", "target-mode-off.json"),
		"no-roster": filepath.Join(outputRoot, "configs", "target-no-roster.json"),
	}
	for name, path := range map[string]string{
		"review attestation": attestationPath, "review report": reportPath, "trusted review key": keyPath,
		"parent registration": parentPath, "amendment": amendmentPath,
	} {
		raw, err := readSV1DRegularFile(path)
		if err != nil {
			return fmt.Errorf("read retained %s: %w", name, err)
		}
		expectedDigest := ""
		switch name {
		case "review attestation":
			expectedDigest = expected.ReviewAttestationSHA256
		case "review report":
			expectedDigest = expected.ReviewReportSHA256
		case "trusted review key":
			expectedDigest = expected.TrustedReviewKeySHA256
		case "parent registration":
			expectedDigest = expected.ParentRegistrationSHA256
		case "amendment":
			expectedDigest = expected.AmendmentSHA256
		}
		if sha256DigestHex(raw) != expectedDigest {
			return fmt.Errorf("retained %s does not match its strict launch digest", name)
		}
	}
	configDigest := make(map[string]string, len(configPaths))
	for name, path := range configPaths {
		raw, err := readSV1DRegularFile(path)
		if err != nil {
			return fmt.Errorf("read retained %s config: %w", name, err)
		}
		configDigest[name] = sha256DigestHex(raw)
	}
	if configDigest["treatment"] != expected.TreatmentConfigSHA256 || configDigest["mode-off"] != expected.ModeOffConfigSHA256 || configDigest["no-roster"] != expected.NoRosterConfigSHA256 {
		return fmt.Errorf("retained target configs do not match strict launch identities")
	}
	keyRaw, err := readSV1DRegularFile(keyPath)
	if err != nil {
		return err
	}
	if len(keyRaw) != 32 {
		return fmt.Errorf("retained trusted review key has invalid length")
	}
	_, err = VerifySV1DReviewAttestation(attestationPath, reportPath, SV1DReviewExpectation{
		SourceRevision: expected.SourceRevision, TreeRevision: expected.TreeRevision,
		ProbeID: "v2-r2-sv1d-activation-659", PlanSHA256: expected.PlanSHA256,
		ParentRegistrationSHA256: expected.ParentRegistrationSHA256, AmendmentSHA256: expected.AmendmentSHA256,
		TreatmentConfigSHA256: expected.TreatmentConfigSHA256, ModeOffConfigSHA256: expected.ModeOffConfigSHA256,
		NoRosterConfigSHA256: expected.NoRosterConfigSHA256, BinarySHA256: expected.BinarySHA256,
		AnalyzerSHA256: expected.AnalyzerSHA256, RendererSHA256: expected.RendererSHA256,
		TrustedPublicKey: append([]byte(nil), keyRaw...),
	})
	if err != nil {
		return fmt.Errorf("retained SV1D review is not independently valid: %w", err)
	}
	return nil
}

func validateSV1DCapacityRetention(outputRoot string, metadata sv1dActivationRunMetadata, expected CDFExpectedProvenance) error {
	path := filepath.Join(outputRoot, "provenance", "capacity-attestation.json")
	raw, err := readSV1DRegularFile(path)
	if err != nil {
		return fmt.Errorf("read retained capacity attestation: %w", err)
	}
	if sha256DigestHex(raw) != expected.CapacityAttestationSHA256 {
		return fmt.Errorf("retained capacity attestation does not match its strict launch digest")
	}
	var attestation SV1DCapacityAttestation
	if err := validateSV1DCapacityAttestationJSONPresence(raw); err != nil {
		return fmt.Errorf("retained capacity attestation field contract: %w", err)
	}
	if err := decodeStrictSV1DCapacityJSON(raw, &attestation); err != nil {
		return fmt.Errorf("decode retained capacity attestation: %w", err)
	}
	if err := ValidateSV1DCapacityAttestation(attestation); err != nil {
		return fmt.Errorf("retained capacity attestation: %w", err)
	}
	if attestation.SourceRevision != expected.SourceRevision || attestation.TreeRevision != expected.TreeRevision ||
		attestation.PlanSHA256 != expected.PlanSHA256 || attestation.ReviewAttestationSHA256 != expected.ReviewAttestationSHA256 ||
		attestation.ReviewReportSHA256 != expected.ReviewReportSHA256 || attestation.TrustedReviewKeySHA256 != expected.TrustedReviewKeySHA256 ||
		attestation.BinarySHA256 != expected.BinarySHA256 || attestation.AnalyzerSHA256 != expected.AnalyzerSHA256 ||
		attestation.RendererSHA256 != expected.RendererSHA256 || attestation.RunnerSHA256 != expected.CapacityRunnerSHA256 ||
		attestation.MeasurementRecordsSHA256 != expected.CapacityRecordsSHA256 || attestation.OutputParent != metadata.OutputParent {
		return fmt.Errorf("retained capacity attestation does not match strict activation provenance")
	}
	if attestation.TargetTreatmentConfigSHA256 != expected.TreatmentConfigSHA256 || attestation.TargetModeOffConfigSHA256 != expected.ModeOffConfigSHA256 || attestation.TargetNoRosterConfigSHA256 != expected.NoRosterConfigSHA256 {
		return fmt.Errorf("retained capacity attestation target config identities do not match the probe")
	}
	return nil
}

func validateSV1DActivationArmProvenance(evidenceDir, renderedDir string, spec SV1DProbeArmSpec, options SV1DProbeArmAuditOptions) error {
	expected := options.Activation.ExpectedProvenance
	armDirectory, err := filepath.Abs(evidenceDir)
	if err != nil {
		return fmt.Errorf("resolve SV1D arm directory: %w", err)
	}
	outputRoot := filepath.Clean(filepath.Join(armDirectory, "..", ".."))
	metadataPath := filepath.Clean(expected.ActivationMetadataPath)
	if metadataPath != filepath.Join(outputRoot, "provenance", "activation-run-metadata.json") {
		return fmt.Errorf("SV1D activation metadata path is not bound to the arm directory")
	}
	metadata, _, err := readSV1DActivationMetadata(metadataPath)
	if err != nil {
		return err
	}
	if err := validateSV1DActivationMetadata(metadataPath, expected); err != nil {
		return err
	}
	if err := validateSV1DReviewRetention(outputRoot, expected); err != nil {
		return err
	}
	if err := validateSV1DCapacityRetention(outputRoot, metadata, expected); err != nil {
		return err
	}
	armMetadataPath := filepath.Join(armDirectory, "run-metadata.json")
	raw, err := readSV1DRegularFile(armMetadataPath)
	if err != nil {
		return fmt.Errorf("read SV1D arm run metadata: %w", err)
	}
	var armMetadata sv1dActivationArmMetadata
	if err := decodeStrictSV1DCapacityJSON(raw, &armMetadata,
		"schema_version", "runner_contract", "probe_id", "arm", "experiment_id", "config_experiment_id",
		"hypothesis_id", "seed", "simulated_horizon", "simulation_start_nano", "simulation_end_nano",
		"config_sha256", "binary_sha256", "git_revision", "tree_revision", "binary_path",
		"binary_go_version", "binary_goos", "binary_goarch", "binary_goamd64", "analyzer_sha256",
		"renderer_sha256", "runner_sha256", "review_attestation_sha256", "review_report_sha256",
		"capacity_attestation_sha256", "capacity_records_sha256", "trusted_review_key_sha256", "log_mode",
		"evidence_format", "evidence_schema_epoch", "gomaxprocs", "gomemlimit", "output_dir", "holdout",
		"command", "raw_log_policy"); err != nil {
		return fmt.Errorf("decode SV1D arm run metadata: %w", err)
	}
	expectedCommand := []string{"multivenue", "-config", "run-config.json", "-duration", "5m", "-log-mode", "full", "-evidence-format", "evstream_v3"}
	if armMetadata.SchemaVersion != 2 || armMetadata.RunnerContract != sv1dActivationRunnerContract || armMetadata.ProbeID != "v2-r2-sv1d-activation-659" ||
		armMetadata.Arm != spec.Name || armMetadata.ExperimentID != spec.ExperimentID || armMetadata.ConfigExperimentID != spec.ExperimentID ||
		armMetadata.HypothesisID != spec.HypothesisID || armMetadata.Seed != options.Contract.Seed || armMetadata.SimulatedHorizon != options.Contract.Horizon ||
		armMetadata.SimulationStartNano != options.Contract.SimulationStartNano || armMetadata.SimulationEndNano != options.Contract.SimulationEndNano ||
		armMetadata.ConfigSHA256 != spec.ConfigSHA256 || armMetadata.BinarySHA256 != expected.BinarySHA256 || armMetadata.GitRevision != expected.SourceRevision ||
		armMetadata.TreeRevision != expected.TreeRevision || armMetadata.BinaryPath != filepath.Join(outputRoot, "tools", "multivenue-"+expected.BinarySHA256) ||
		!strings.HasPrefix(armMetadata.BinaryGoVersion, "go1.27") || armMetadata.BinaryGOOS != "linux" || armMetadata.BinaryGOARCH != "amd64" || armMetadata.BinaryGOAMD64 != "v1" ||
		armMetadata.AnalyzerSHA256 != expected.AnalyzerSHA256 || armMetadata.RendererSHA256 != expected.RendererSHA256 || armMetadata.RunnerSHA256 != expected.ActivationRunnerSHA256 ||
		armMetadata.ReviewAttestationSHA256 != expected.ReviewAttestationSHA256 || armMetadata.ReviewReportSHA256 != expected.ReviewReportSHA256 ||
		armMetadata.CapacityAttestationSHA256 != expected.CapacityAttestationSHA256 || armMetadata.CapacityRecordsSHA256 != expected.CapacityRecordsSHA256 ||
		armMetadata.TrustedReviewKeySHA256 != expected.TrustedReviewKeySHA256 || armMetadata.LogMode != "full" || armMetadata.EvidenceFormat != "evstream_v3" ||
		armMetadata.EvidenceSchemaEpoch != expected.EvidenceSchemaEpoch || armMetadata.GOMAXPROCS != expected.GOMAXPROCS || armMetadata.GOMEMLIMIT != expected.GOMEMLIMIT ||
		armMetadata.OutputDir != armDirectory || armMetadata.Holdout || !sameSV1DStrings(armMetadata.Command, expectedCommand) ||
		armMetadata.RawLogPolicy != "retain until the complete SV1D arm and tri-arm score have passed independent review" {
		return fmt.Errorf("SV1D arm run metadata does not match the strict activation contract")
	}
	if !absoluteCleanPath(armMetadata.BinaryPath) || !absoluteCleanPath(armMetadata.OutputDir) {
		return fmt.Errorf("SV1D arm run metadata has an invalid binary or output path")
	}
	if renderedDir == "" || !absoluteCleanPath(renderedDir) {
		return fmt.Errorf("SV1D strict arm provenance has an invalid rendered directory")
	}
	for name, digest := range map[string]string{
		"activation runner": expected.ActivationRunnerSHA256,
		"simulator":         expected.BinarySHA256,
		"analyzer":          expected.AnalyzerSHA256,
		"renderer":          expected.RendererSHA256,
	} {
		path := ""
		switch name {
		case "activation runner":
			path = filepath.Join(outputRoot, "provenance", "activation-runner.sh")
		case "simulator":
			path = filepath.Join(outputRoot, "tools", "multivenue-"+expected.BinarySHA256)
		case "analyzer":
			path = filepath.Join(outputRoot, "tools", "sv1dprobe-"+expected.AnalyzerSHA256)
		case "renderer":
			path = filepath.Join(outputRoot, "tools", "evsrender-"+expected.RendererSHA256)
		}
		fileRaw, err := readSV1DRegularFile(path)
		if err != nil {
			return fmt.Errorf("read retained %s: %w", name, err)
		}
		if sha256DigestHex(fileRaw) != digest {
			return fmt.Errorf("retained %s does not match strict activation identity", name)
		}
	}
	return nil
}

// SV1DProbeVenueScore contains the availability measurements used by the
// scorer, retained by venue so aggregate improvement cannot hide a dead venue.
type SV1DProbeVenueScore struct {
	VenueID   string                `json:"venue_id"`
	Treatment SV1DProbeAvailability `json:"treatment"`
	ModeOff   SV1DProbeAvailability `json:"mode_off"`
	NoRoster  SV1DProbeAvailability `json:"no_roster"`
}

type SV1DProbeAvailability struct {
	BidOnlyDurationNano                     int64  `json:"bid_only_duration_nano"`
	AskOnlyDurationNano                     int64  `json:"ask_only_duration_nano"`
	EmptyBookDurationNano                   int64  `json:"empty_book_duration_nano"`
	OneSidedDurationNano                    int64  `json:"one_sided_duration_nano"`
	NonTwoSidedDurationNano                 int64  `json:"non_two_sided_duration_nano"`
	MaxUninterruptedNonTwoSidedDurationNano int64  `json:"max_uninterrupted_non_two_sided_duration_nano"`
	TerminalBookMode                        string `json:"terminal_book_mode"`
}

// SV1DProbeScore is a deterministic vector result rather than a boolean. The
// status is meaningful only together with the component durations and failed
// predicates.
type SV1DProbeScore struct {
	PlanSHA256                       string                `json:"plan_sha256"`
	Status                           string                `json:"status"`
	TreatmentNonTwoSidedDurationNano int64                 `json:"treatment_non_two_sided_duration_nano"`
	ModeOffNonTwoSidedDurationNano   int64                 `json:"mode_off_non_two_sided_duration_nano"`
	NoRosterNonTwoSidedDurationNano  int64                 `json:"no_roster_non_two_sided_duration_nano"`
	TreatmentNonTwoSidedFraction     float64               `json:"treatment_non_two_sided_fraction"`
	ModeOffNonTwoSidedFraction       float64               `json:"mode_off_non_two_sided_fraction"`
	NoRosterNonTwoSidedFraction      float64               `json:"no_roster_non_two_sided_fraction"`
	Venues                           []SV1DProbeVenueScore `json:"venues"`
	FailedPredicates                 []string              `json:"failed_predicates,omitempty"`
}

// ScoreSV1DProbe applies the amendment's precommitted predicates in a fixed
// order. It never infers missing evidence, truncates an arm to its observed
// endpoint, or lets a control's failure masquerade as treatment activation.
func ScoreSV1DProbe(plan SV1DProbePlan, arms []SV1DProbeArmResult) SV1DProbeScore {
	score := SV1DProbeScore{}
	planFailures := validateSV1DProbePlan(plan)
	if len(planFailures) > 0 {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = planFailures
		return score
	}
	planSHA256, err := SV1DProbePlanSHA256(plan)
	if err != nil {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = []string{"probe plan cannot be canonically hashed: " + err.Error()}
		return score
	}
	score.PlanSHA256 = planSHA256

	armByName := make(map[string]SV1DProbeArmResult, len(arms))
	for _, arm := range arms {
		if _, duplicate := armByName[arm.ArmName]; duplicate || arm.ArmName == "" {
			score.Status = SV1DProbeStatusInvalidEvidence
			score.FailedPredicates = []string{"arm names are empty or duplicated"}
			return score
		}
		armByName[arm.ArmName] = arm
	}
	if len(armByName) != 3 {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = []string{"probe must contain exactly treatment, mode-off, and no-roster arms"}
		return score
	}

	specs := map[string]SV1DProbeArmSpec{
		plan.Treatment.Name: plan.Treatment,
		plan.ModeOff.Name:   plan.ModeOff,
		plan.NoRoster.Name:  plan.NoRoster,
	}
	orderedArmNames := []string{plan.Treatment.Name, plan.ModeOff.Name, plan.NoRoster.Name}
	for _, armName := range orderedArmNames {
		arm, ok := armByName[armName]
		if !ok {
			score.Status = SV1DProbeStatusInvalidEvidence
			score.FailedPredicates = append(score.FailedPredicates, "missing arm: "+armName)
			continue
		}
		if failures := validateSV1DProbeArmIdentity(specs[armName], arm); len(failures) > 0 {
			score.Status = SV1DProbeStatusInvalidEvidence
			score.FailedPredicates = append(score.FailedPredicates, failures...)
		}
		if arm.PlanSHA256 != planSHA256 {
			score.Status = SV1DProbeStatusInvalidEvidence
			score.FailedPredicates = append(score.FailedPredicates, "arm plan digest does not match the canonical probe plan: "+armName)
		}
	}
	if score.Status == SV1DProbeStatusInvalidEvidence {
		return score
	}

	availabilityByArm := make(map[string]map[string]SV1DProbeAvailability, 3)
	for _, armName := range orderedArmNames {
		arm := armByName[armName]
		if !arm.Complete {
			score.Status = SV1DProbeStatusIncompleteArm
			score.FailedPredicates = append(score.FailedPredicates, "arm did not reach the registered endpoint: "+armName)
			continue
		}
		if !arm.EvidenceValid || !arm.StrictMechanicsValid {
			score.Status = SV1DProbeStatusInvalidEvidence
			score.FailedPredicates = append(score.FailedPredicates, "arm lacks valid strict evidence: "+armName)
			continue
		}
		if !arm.TerminalValuationValid {
			score.Status = SV1DProbeStatusIncompleteArm
			score.FailedPredicates = append(score.FailedPredicates, "arm lacks strict terminal valuation evidence: "+armName)
			continue
		}
		availability, failures := validateSV1DProbeVenues(plan, arm)
		if len(failures) > 0 {
			score.Status = SV1DProbeStatusInvalidEvidence
			score.FailedPredicates = append(score.FailedPredicates, failures...)
			continue
		}
		availabilityByArm[armName] = availability
	}
	if score.Status == SV1DProbeStatusInvalidEvidence {
		return score
	}
	if score.Status == SV1DProbeStatusIncompleteArm {
		return score
	}

	treatment := armByName[plan.Treatment.Name]
	if !treatment.ActivationSatisfied {
		score.Status = SV1DProbeStatusTreatmentNotActivated
		score.FailedPredicates = []string{"treatment did not satisfy the registered supplier activation contract"}
		return score
	}
	if !treatment.AntiCheatingSatisfied {
		score.Status = SV1DProbeStatusAntiCheatingRejected
		score.FailedPredicates = []string{"treatment failed a registered anti-cheating predicate"}
		return score
	}

	for _, venueID := range plan.VenueIDs {
		treatmentAvailability := availabilityByArm[plan.Treatment.Name][venueID]
		modeOffAvailability := availabilityByArm[plan.ModeOff.Name][venueID]
		noRosterAvailability := availabilityByArm[plan.NoRoster.Name][venueID]
		score.Venues = append(score.Venues, SV1DProbeVenueScore{
			VenueID: venueID, Treatment: treatmentAvailability, ModeOff: modeOffAvailability, NoRoster: noRosterAvailability,
		})
		if treatmentAvailability.MaxUninterruptedNonTwoSidedDurationNano > plan.MaxUninterruptedNonTwoSidedDurationNano {
			score.FailedPredicates = append(score.FailedPredicates, "treatment exceeds per-venue persistence threshold: "+venueID)
		}
		if treatmentAvailability.TerminalBookMode != "two_sided" {
			score.FailedPredicates = append(score.FailedPredicates, "treatment terminal book is not two-sided: "+venueID)
		}
	}

	var ok bool
	score.TreatmentNonTwoSidedDurationNano, ok = sumSV1DNonTwoSidedDuration(plan, availabilityByArm[plan.Treatment.Name])
	if !ok {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = append(score.FailedPredicates, "treatment duration overflows")
		return score
	}
	score.ModeOffNonTwoSidedDurationNano, ok = sumSV1DNonTwoSidedDuration(plan, availabilityByArm[plan.ModeOff.Name])
	if !ok {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = append(score.FailedPredicates, "mode-off duration overflows")
		return score
	}
	score.NoRosterNonTwoSidedDurationNano, ok = sumSV1DNonTwoSidedDuration(plan, availabilityByArm[plan.NoRoster.Name])
	if !ok {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = append(score.FailedPredicates, "no-roster duration overflows")
		return score
	}
	denominatorNano, denominatorOK := checkedSV1DPositiveMultiply(plan.ProbeDurationNano, int64(len(plan.VenueIDs)))
	if !denominatorOK {
		score.Status = SV1DProbeStatusInvalidEvidence
		score.FailedPredicates = append(score.FailedPredicates, "probe duration denominator overflows")
		return score
	}
	denominator := float64(denominatorNano)
	score.TreatmentNonTwoSidedFraction = float64(score.TreatmentNonTwoSidedDurationNano) / denominator
	score.ModeOffNonTwoSidedFraction = float64(score.ModeOffNonTwoSidedDurationNano) / denominator
	score.NoRosterNonTwoSidedFraction = float64(score.NoRosterNonTwoSidedDurationNano) / denominator

	if len(score.FailedPredicates) > 0 || score.TreatmentNonTwoSidedDurationNano >= score.ModeOffNonTwoSidedDurationNano ||
		score.TreatmentNonTwoSidedDurationNano >= score.NoRosterNonTwoSidedDurationNano {
		if score.Status == "" {
			score.Status = SV1DProbeStatusNoDirectionalEffect
		}
		return score
	}
	score.Status = SV1DProbeStatusPass
	return score
}

func validateSV1DProbePlan(plan SV1DProbePlan) []string {
	var failures []string
	if plan.ExperimentID == "" || plan.HypothesisID == "" || plan.Horizon == "" || plan.Seed <= 0 || plan.ProbeDurationNano <= 0 || plan.MaxUninterruptedNonTwoSidedDurationNano <= 0 {
		failures = append(failures, "probe plan has missing identity or positive duration boundary")
	}
	if plan.MaxUninterruptedNonTwoSidedDurationNano > plan.ProbeDurationNano {
		failures = append(failures, "probe persistence threshold exceeds the probe duration")
	}
	if len(plan.VenueIDs) == 0 {
		failures = append(failures, "probe plan has no venues")
	}
	if !isSV1DHexDigest(plan.AnalyzerSHA256) || !isSV1DHexDigest(plan.RendererSHA256) {
		failures = append(failures, "probe plan is missing analyzer or renderer identity")
	}
	seenVenues := make(map[string]struct{}, len(plan.VenueIDs))
	for _, venueID := range plan.VenueIDs {
		if venueID == "" {
			failures = append(failures, "probe plan contains an empty venue")
		} else if _, duplicate := seenVenues[venueID]; duplicate {
			failures = append(failures, "probe plan contains a duplicate venue: "+venueID)
		} else {
			seenVenues[venueID] = struct{}{}
		}
	}
	specs := []SV1DProbeArmSpec{plan.Treatment, plan.ModeOff, plan.NoRoster}
	seenArms := make(map[string]struct{}, len(specs))
	registeredSourceRevision := ""
	for _, spec := range specs {
		if spec.Name == "" || spec.ExperimentID == "" || spec.HypothesisID == "" || !isSV1DHexDigest(spec.ConfigSHA256) || !isSV1DHexDigest(spec.BinarySHA256) || !isSV1DHexDigest(spec.AnalyzerSHA256) || !isSV1DHexDigest(spec.RendererSHA256) || !isCDFHex(spec.SourceRevision, 20) {
			failures = append(failures, "probe arm spec is missing an immutable identity: "+spec.Name)
		}
		if spec.AnalyzerSHA256 != plan.AnalyzerSHA256 || spec.RendererSHA256 != plan.RendererSHA256 {
			failures = append(failures, "probe arm tool identity differs from plan: "+spec.Name)
		}
		if registeredSourceRevision == "" {
			registeredSourceRevision = spec.SourceRevision
		} else if spec.SourceRevision != registeredSourceRevision {
			failures = append(failures, "probe arm source identity differs from plan: "+spec.Name)
		}
		if _, duplicate := seenArms[spec.Name]; duplicate {
			failures = append(failures, "probe plan contains a duplicate arm: "+spec.Name)
		} else {
			seenArms[spec.Name] = struct{}{}
		}
	}
	return failures
}

func validateSV1DProbeArmIdentity(spec SV1DProbeArmSpec, arm SV1DProbeArmResult) []string {
	var failures []string
	if arm.ArmName != spec.Name {
		failures = append(failures, "arm name does not match its plan")
	}
	if arm.ExperimentID != spec.ExperimentID || arm.HypothesisID != spec.HypothesisID || arm.ConfigSHA256 != spec.ConfigSHA256 || arm.SourceRevision != spec.SourceRevision || arm.BinarySHA256 != spec.BinarySHA256 || arm.AnalyzerSHA256 != spec.AnalyzerSHA256 || arm.RendererSHA256 != spec.RendererSHA256 {
		failures = append(failures, "arm provenance does not match its plan: "+spec.Name)
	}
	return failures
}

func validateSV1DProbeVenues(plan SV1DProbePlan, arm SV1DProbeArmResult) (map[string]SV1DProbeAvailability, []string) {
	availability := make(map[string]SV1DProbeAvailability, len(arm.Venues))
	var failures []string
	for _, venue := range arm.Venues {
		if _, duplicate := availability[venue.VenueID]; duplicate || venue.VenueID == "" {
			failures = append(failures, "arm has an empty or duplicate venue metric: "+arm.ArmName)
			continue
		}
		metric := SV1DProbeAvailability{
			BidOnlyDurationNano: venue.BidOnlyDurationNano, AskOnlyDurationNano: venue.AskOnlyDurationNano,
			EmptyBookDurationNano: venue.EmptyBookDurationNano, OneSidedDurationNano: venue.OneSidedDurationNano,
			NonTwoSidedDurationNano:                 venue.NonTwoSidedDurationNano,
			MaxUninterruptedNonTwoSidedDurationNano: venue.MaxUninterruptedNonTwoSidedDurationNano,
			TerminalBookMode:                        venue.TerminalBookMode,
		}
		oneSidedDuration, oneSidedOK := checkedSV1DNonNegativeAdd(metric.BidOnlyDurationNano, metric.AskOnlyDurationNano)
		nonTwoSidedDuration, nonTwoSidedOK := checkedSV1DNonNegativeAdd(oneSidedDuration, metric.EmptyBookDurationNano)
		if metric.BidOnlyDurationNano < 0 || metric.AskOnlyDurationNano < 0 || metric.EmptyBookDurationNano < 0 || metric.OneSidedDurationNano < 0 || metric.NonTwoSidedDurationNano < 0 || !oneSidedOK || !nonTwoSidedOK || oneSidedDuration != metric.OneSidedDurationNano || nonTwoSidedDuration != metric.NonTwoSidedDurationNano || metric.MaxUninterruptedNonTwoSidedDurationNano < 0 || metric.MaxUninterruptedNonTwoSidedDurationNano > metric.NonTwoSidedDurationNano || metric.MaxUninterruptedNonTwoSidedDurationNano > plan.ProbeDurationNano {
			failures = append(failures, "arm has inconsistent or out-of-range venue duration metrics: "+arm.ArmName+"/"+venue.VenueID)
		}
		switch metric.TerminalBookMode {
		case "bid_only", "ask_only", "empty", "two_sided":
		default:
			failures = append(failures, "arm has an unknown terminal book mode: "+arm.ArmName+"/"+venue.VenueID)
		}
		availability[venue.VenueID] = metric
	}
	if len(availability) != len(plan.VenueIDs) {
		failures = append(failures, "arm does not contain exactly one metric for every registered venue: "+arm.ArmName)
	}
	for _, venueID := range plan.VenueIDs {
		if _, found := availability[venueID]; !found {
			failures = append(failures, "arm is missing venue metric: "+arm.ArmName+"/"+venueID)
		}
	}
	return availability, failures
}

func sumSV1DNonTwoSidedDuration(plan SV1DProbePlan, availability map[string]SV1DProbeAvailability) (int64, bool) {
	var total int64
	for _, venueID := range plan.VenueIDs {
		value, ok := availability[venueID]
		if !ok {
			return 0, false
		}
		var addOK bool
		total, addOK = checkedCDFAdd(total, value.NonTwoSidedDurationNano)
		if !addOK || value.NonTwoSidedDurationNano > plan.ProbeDurationNano {
			return 0, false
		}
	}
	return total, true
}

func isSV1DHexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func checkedSV1DNonNegativeAdd(left, right int64) (int64, bool) {
	if left < 0 || right < 0 {
		return 0, false
	}
	return checkedCDFAdd(left, right)
}

func checkedSV1DPositiveMultiply(left, right int64) (int64, bool) {
	if left <= 0 || right <= 0 || left > math.MaxInt64/right {
		return 0, false
	}
	return left * right, true
}

// SV1DConfigPaths identifies the three checked-in probe configurations.
type SV1DConfigPaths struct {
	Treatment string
	ModeOff   string
	NoRoster  string
}

// SV1DConfigIdentity is the raw-file identity used by a launcher to bind an
// arm result to the exact config bytes it executed.
type SV1DConfigIdentity struct {
	ExperimentID string `json:"experiment_id"`
	HypothesisID string `json:"hypothesis_id"`
	ConfigSHA256 string `json:"config_sha256"`
}

type SV1DConfigTriad struct {
	Treatment SV1DConfigIdentity `json:"treatment"`
	ModeOff   SV1DConfigIdentity `json:"mode_off"`
	NoRoster  SV1DConfigIdentity `json:"no_roster"`
}

// BuildRegisteredSV1DProbePlan binds the checked-in config triad to the
// source and binary identities that are resolved before a development probe.
// The resulting plan is immutable for that run; changing any identity creates
// a different plan and therefore cannot silently reuse its score.
func BuildRegisteredSV1DProbePlan(triad SV1DConfigTriad, sourceRevision, binarySHA256, analyzerSHA256, rendererSHA256 string) (SV1DProbePlan, error) {
	if !isCDFHex(sourceRevision, 20) || !isSV1DHexDigest(binarySHA256) || !isSV1DHexDigest(analyzerSHA256) || !isSV1DHexDigest(rendererSHA256) {
		return SV1DProbePlan{}, fmt.Errorf("SV1D plan requires a 40-hex source revision and 64-hex simulator, analyzer, and renderer digests")
	}
	plan := SV1DProbePlan{
		ExperimentID:                            "v2-r2-sv1d-activation-659",
		HypothesisID:                            "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY",
		Seed:                                    659,
		Horizon:                                 "5m",
		ProbeDurationNano:                       300_000_000_000,
		VenueIDs:                                []string{"north", "central", "south"},
		MaxUninterruptedNonTwoSidedDurationNano: 30_000_000_000,
		AnalyzerSHA256:                          analyzerSHA256,
		RendererSHA256:                          rendererSHA256,
		Treatment: SV1DProbeArmSpec{
			Name: "treatment", ExperimentID: triad.Treatment.ExperimentID,
			HypothesisID: triad.Treatment.HypothesisID, ConfigSHA256: triad.Treatment.ConfigSHA256,
			SourceRevision: sourceRevision, BinarySHA256: binarySHA256, AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256,
		},
		ModeOff: SV1DProbeArmSpec{
			Name: "mode-off", ExperimentID: triad.ModeOff.ExperimentID,
			HypothesisID: triad.ModeOff.HypothesisID, ConfigSHA256: triad.ModeOff.ConfigSHA256,
			SourceRevision: sourceRevision, BinarySHA256: binarySHA256, AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256,
		},
		NoRoster: SV1DProbeArmSpec{
			Name: "no-roster", ExperimentID: triad.NoRoster.ExperimentID,
			HypothesisID: triad.NoRoster.HypothesisID, ConfigSHA256: triad.NoRoster.ConfigSHA256,
			SourceRevision: sourceRevision, BinarySHA256: binarySHA256, AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256,
		},
	}
	if err := ValidateRegisteredSV1DProbePlan(plan); err != nil {
		return SV1DProbePlan{}, err
	}
	return plan, nil
}

// ValidateRegisteredSV1DProbePlan applies the fixed development-only probe
// boundary in addition to the structural checks shared by the scorer.
func ValidateRegisteredSV1DProbePlan(plan SV1DProbePlan) error {
	failures := validateSV1DProbePlan(plan)
	if plan.ExperimentID != "v2-r2-sv1d-activation-659" || plan.HypothesisID != "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY" ||
		plan.Seed != 659 || plan.Horizon != "5m" || plan.ProbeDurationNano != 300_000_000_000 ||
		plan.MaxUninterruptedNonTwoSidedDurationNano != 30_000_000_000 ||
		!sameSV1DStrings(plan.VenueIDs, []string{"north", "central", "south"}) {
		failures = append(failures, "probe plan differs from the registered SV1D development boundary")
	}
	registeredArmNames := []string{"treatment", "mode-off", "no-roster"}
	registeredArmExperimentIDs := []string{
		"v2-r2-sv1d-activation-659-treatment",
		"v2-r2-sv1d-activation-659-mode-off",
		"v2-r2-sv1d-activation-659-no-roster",
	}
	registeredArmHypothesisIDs := []string{
		"V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY",
		"V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY-MODE-OFF",
		"V2-R2-SV1D-NO-ROSTER-CONTROL",
	}
	registeredArms := []SV1DProbeArmSpec{plan.Treatment, plan.ModeOff, plan.NoRoster}
	registeredConfigHashes := make(map[string]string, len(registeredArms))
	for index, arm := range registeredArms {
		if arm.Name != registeredArmNames[index] || arm.ExperimentID != registeredArmExperimentIDs[index] || arm.HypothesisID != registeredArmHypothesisIDs[index] {
			failures = append(failures, "registered SV1D arm identity differs from the preregistered triad: "+registeredArmNames[index])
		}
		if index > 0 && arm.BinarySHA256 != registeredArms[0].BinarySHA256 {
			failures = append(failures, "registered SV1D arms do not share one simulator binary identity")
		}
		if index > 0 && (arm.SourceRevision != registeredArms[0].SourceRevision || arm.AnalyzerSHA256 != registeredArms[0].AnalyzerSHA256 || arm.RendererSHA256 != registeredArms[0].RendererSHA256) {
			failures = append(failures, "registered SV1D arms do not share one source/tool identity")
		}
		if _, duplicate := registeredConfigHashes[arm.ConfigSHA256]; duplicate {
			failures = append(failures, "registered SV1D arms do not have distinct config identities")
		}
		registeredConfigHashes[arm.ConfigSHA256] = registeredArmNames[index]
	}
	if len(failures) > 0 {
		return fmt.Errorf("invalid registered SV1D probe plan: %s", failures[0])
	}
	return nil
}

// SV1DProbePlanSHA256 hashes the canonical typed plan. The digest deliberately
// excludes the wrapper document and is therefore stable across JSON formatting
// while remaining sensitive to every plan field and arm identity.
func SV1DProbePlanSHA256(plan SV1DProbePlan) (string, error) {
	raw, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("marshal canonical SV1D probe plan: %w", err)
	}
	hasher := sha256.New()
	if _, err := hasher.Write([]byte(sv1dProbePlanHashDomain)); err != nil {
		return "", fmt.Errorf("write canonical SV1D probe plan hash domain: %w", err)
	}
	if _, err := hasher.Write(raw); err != nil {
		return "", fmt.Errorf("write canonical SV1D probe plan: %w", err)
	}
	digest := hasher.Sum(nil)
	return hex.EncodeToString(digest), nil
}

// ValidateSV1DProbePlan exposes the same fixed validation used by the scorer
// so a launcher can reject a malformed plan before any arm is executed.
func ValidateSV1DProbePlan(plan SV1DProbePlan) error {
	failures := validateSV1DProbePlan(plan)
	if len(failures) > 0 {
		return fmt.Errorf("invalid SV1D probe plan: %s", failures[0])
	}
	return nil
}

func sameSV1DStrings(left, right []string) bool {
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

// ValidateSV1DConfigTriad validates the registered semantic relationships of
// three config documents without trusting their presentation order or hashes.
func ValidateSV1DConfigTriad(treatment, modeOff, noRoster []byte) error {
	_, err := parseSV1DConfigTriad(treatment, modeOff, noRoster)
	return err
}

// ValidateSV1DConfigTriadFiles validates and hashes three exact config files.
// The returned hashes are raw-file hashes because those are the identities
// that a launcher must attest; semantic comparisons use typed JSON values.
func ValidateSV1DConfigTriadFiles(paths SV1DConfigPaths) (SV1DConfigTriad, error) {
	treatment, err := os.ReadFile(paths.Treatment)
	if err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("read treatment config: %w", err)
	}
	modeOff, err := os.ReadFile(paths.ModeOff)
	if err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("read mode-off config: %w", err)
	}
	noRoster, err := os.ReadFile(paths.NoRoster)
	if err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("read no-roster config: %w", err)
	}
	return parseSV1DConfigTriad(treatment, modeOff, noRoster)
}

func parseSV1DConfigTriad(treatmentRaw, modeOffRaw, noRosterRaw []byte) (SV1DConfigTriad, error) {
	treatment, err := decodeSV1DJSONObject(treatmentRaw)
	if err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("decode treatment config: %w", err)
	}
	modeOff, err := decodeSV1DJSONObject(modeOffRaw)
	if err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("decode mode-off config: %w", err)
	}
	noRoster, err := decodeSV1DJSONObject(noRosterRaw)
	if err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("decode no-roster config: %w", err)
	}
	for name, config := range map[string]map[string]any{"treatment": treatment, "mode-off": modeOff, "no-roster": noRoster} {
		if err := validateSV1DCommonConfig(name, config); err != nil {
			return SV1DConfigTriad{}, err
		}
	}
	if err := validateSV1DArmIdentity(treatment, "v2-r2-sv1d-activation-659-treatment", "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY"); err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("treatment identity: %w", err)
	}
	if err := validateSV1DArmIdentity(modeOff, "v2-r2-sv1d-activation-659-mode-off", "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY-MODE-OFF"); err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("mode-off identity: %w", err)
	}
	if err := validateSV1DArmIdentity(noRoster, "v2-r2-sv1d-activation-659-no-roster", "V2-R2-SV1D-NO-ROSTER-CONTROL"); err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("no-roster identity: %w", err)
	}
	if err := validateSV1DSupplierRoster(treatment, true); err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("treatment roster: %w", err)
	}
	if err := validateSV1DSupplierRoster(modeOff, false); err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("mode-off roster: %w", err)
	}
	if err := validateSV1DNoRoster(noRoster); err != nil {
		return SV1DConfigTriad{}, fmt.Errorf("no-roster arm: %w", err)
	}
	identityFilter := func(config map[string]any) any {
		copy := cloneSV1DJSONValue(config).(map[string]any)
		for _, key := range []string{"experiment_id", "hypothesis_id", "description", "date", "status"} {
			delete(copy, key)
		}
		return copy
	}
	treatmentBaseline := cloneSV1DJSONValue(identityFilter(treatment)).(map[string]any)
	modeOffBaseline := cloneSV1DJSONValue(identityFilter(modeOff)).(map[string]any)
	for _, config := range []map[string]any{treatmentBaseline, modeOffBaseline} {
		for _, supplier := range config["elastic_liquidity_suppliers"].([]any) {
			delete(supplier.(map[string]any), "quote_on_one_sided_local_book")
		}
	}
	if !sv1dJSONEqual(treatmentBaseline, modeOffBaseline) {
		return SV1DConfigTriad{}, fmt.Errorf("treatment and mode-off configs differ beyond the registered one-sided option")
	}
	noRosterBaseline := cloneSV1DJSONValue(identityFilter(noRoster)).(map[string]any)
	modeOffBaseline["elastic_liquidity_suppliers"] = []any{}
	modeOffBaseline["record_elastic_liquidity_supplier_decisions"] = false
	modeOffBaseline["market_data_receipt_roles"] = []any{"liability_hedger"}
	noRosterBaseline["elastic_liquidity_suppliers"] = []any{}
	noRosterBaseline["record_elastic_liquidity_supplier_decisions"] = false
	noRosterBaseline["market_data_receipt_roles"] = []any{"liability_hedger"}
	if !sv1dJSONEqual(modeOffBaseline, noRosterBaseline) {
		return SV1DConfigTriad{}, fmt.Errorf("mode-off and no-roster configs differ beyond the registered roster and recorder controls")
	}
	return SV1DConfigTriad{
		Treatment: sv1dConfigIdentity(treatment, treatmentRaw),
		ModeOff:   sv1dConfigIdentity(modeOff, modeOffRaw),
		NoRoster:  sv1dConfigIdentity(noRoster, noRosterRaw),
	}, nil
}

func validateSV1DCommonConfig(name string, config map[string]any) error {
	if err := requireSV1DInt(config, "seed", 659); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for _, requirement := range []struct {
		key      string
		expected string
	}{
		{key: "log_mode", expected: "full"},
		{key: "evidence_format", expected: "evstream_v3"},
	} {
		key, expected := requirement.key, requirement.expected
		if err := requireSV1DString(config, key, expected); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	for _, key := range []string{"record_market_data_receipts", "strict_population_accounting", "strict_risk_contract", "cross_asset_spot_graph"} {
		if err := requireSV1DBool(config, key, true); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	for _, key := range []string{"auto_borrow_spot", "cross_asset_collateral_marks"} {
		if err := requireSV1DBool(config, key, false); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := requireSV1DInt(config, "evidence_contract_version", 2); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := requireSV1DInt(config, "elastic_supplier_count", 8); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	for _, requirement := range []struct {
		key      string
		expected int64
	}{
		{key: "step", expected: 1_000_000_000},
		{key: "snapshot_interval", expected: 1_000_000_000},
		{key: "automation_interval", expected: 1_000_000_000},
		{key: "quote_interval", expected: 1_000_000_000},
		{key: "noise_interval", expected: 2_000_000_000},
		{key: "greek_interval", expected: 60_000_000_000},
		{key: "checkpoint_interval_seconds", expected: 60},
	} {
		if err := requireSV1DInt(config, requirement.key, requirement.expected); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := requireSV1DStringSlice(config, "venue_ids", []string{"north", "central", "south"}); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := requireSV1DStringSlice(config, "market_data_receipt_roles", []string{"cdf_elastic_supplier", "liability_hedger"}); err != nil && name != "no-roster" {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := requireSV1DCanonicalValue(config, "venue_rules", `{"central":{"funding_interval_seconds":3600,"matching_rule":"pro_rata"},"north":{"funding_interval_seconds":28800,"matching_rule":"price_time"},"south":{"funding_interval_seconds":7200,"matching_rule":"pro_rata"}}`); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := requireSV1DCanonicalValue(config, "r2_expiry_calendar", `{"schedules":[{"name":"short","listing_interval_nano":3600000000000,"time_to_expiry_nano":7200000000000},{"name":"medium","listing_interval_nano":10800000000000,"time_to_expiry_nano":21600000000000},{"name":"long","listing_interval_nano":21600000000000,"time_to_expiry_nano":43200000000000}]}`); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func validateSV1DArmIdentity(config map[string]any, experimentID, hypothesisID string) error {
	if err := requireSV1DString(config, "experiment_id", experimentID); err != nil {
		return err
	}
	return requireSV1DString(config, "hypothesis_id", hypothesisID)
}

func validateSV1DSupplierRoster(config map[string]any, oneSided bool) error {
	value, ok := config["elastic_liquidity_suppliers"].([]any)
	if !ok || len(value) != 4 {
		return fmt.Errorf("expected four finite successor suppliers")
	}
	expected := RegisteredSV1DActivationContract().Suppliers
	seenRoles := make(map[string]struct{}, len(value))
	for index, rawSupplier := range value {
		supplier, ok := rawSupplier.(map[string]any)
		if !ok {
			return fmt.Errorf("supplier %d is not an object", index)
		}
		role, ok := supplier["role"].(string)
		if !ok || role != expected[index].Role {
			return fmt.Errorf("supplier %d has unexpected role", index)
		}
		if _, duplicate := seenRoles[role]; duplicate {
			return fmt.Errorf("supplier role is duplicated: %s", role)
		}
		seenRoles[role] = struct{}{}
		oneSidedValue, present := supplier["quote_on_one_sided_local_book"]
		if oneSided {
			if !present || oneSidedValue != true {
				return fmt.Errorf("supplier %s does not enable one-sided quoting", role)
			}
		} else if present && oneSidedValue != false {
			return fmt.Errorf("supplier %s enables one-sided quoting in mode-off", role)
		}
		expectedRaw, err := json.Marshal(expected[index])
		if err != nil {
			return fmt.Errorf("marshal expected supplier %s: %w", role, err)
		}
		expectedValue, err := decodeSV1DJSONObject(expectedRaw)
		if err != nil {
			return fmt.Errorf("decode expected supplier %s: %w", role, err)
		}
		delete(supplier, "quote_on_one_sided_local_book")
		delete(expectedValue, "quote_on_one_sided_local_book")
		if oneSided {
			supplier["quote_on_one_sided_local_book"] = true
			expectedValue["quote_on_one_sided_local_book"] = true
		}
		if !sv1dJSONEqual(supplier, expectedValue) {
			return fmt.Errorf("supplier %s differs from the registered finite economic contract", role)
		}
	}
	return nil
}

func validateSV1DNoRoster(config map[string]any) error {
	value, exists := config["elastic_liquidity_suppliers"]
	if exists && value != nil {
		if suppliers, ok := value.([]any); !ok || len(suppliers) != 0 {
			return fmt.Errorf("no-roster arm has a nonempty successor roster")
		}
	}
	if value, exists := config["record_elastic_liquidity_supplier_decisions"]; exists && value != false {
		return fmt.Errorf("no-roster arm records successor decisions")
	}
	return requireSV1DStringSlice(config, "market_data_receipt_roles", []string{"liability_hedger"})
}

func sv1dConfigIdentity(config map[string]any, raw []byte) SV1DConfigIdentity {
	experimentID, _ := config["experiment_id"].(string)
	hypothesisID, _ := config["hypothesis_id"].(string)
	digest := sha256.Sum256(raw)
	return SV1DConfigIdentity{ExperimentID: experimentID, HypothesisID: hypothesisID, ConfigSHA256: hex.EncodeToString(digest[:])}
}

func decodeSV1DJSONObject(raw []byte) (map[string]any, error) {
	value, err := decodeSV1DJSONValue(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top-level value is not an object")
	}
	return object, nil
}

func decodeSV1DJSONValue(raw []byte) (any, error) {
	if err := rejectSV1DDuplicateJSONKeys(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple top-level JSON values")
		}
		return nil, fmt.Errorf("trailing JSON: %w", err)
	}
	return value, nil
}

func rejectSV1DDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkSV1DJSONTokens(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func walkSV1DJSONTokens(decoder *json.Decoder) error {
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
		seenKeys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seenKeys[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key: %s", key)
			}
			seenKeys[key] = struct{}{}
			if err := walkSV1DJSONTokens(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkSV1DJSONTokens(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter: %q", delimiter)
	}
}

func cloneSV1DJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clone := make(map[string]any, len(typed))
		for key, child := range typed {
			clone[key] = cloneSV1DJSONValue(child)
		}
		return clone
	case []any:
		clone := make([]any, len(typed))
		for index, child := range typed {
			clone[index] = cloneSV1DJSONValue(child)
		}
		return clone
	default:
		return typed
	}
}

func sv1dJSONEqual(left, right any) bool {
	leftCanonical, leftErr := json.Marshal(left)
	rightCanonical, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftCanonical, rightCanonical)
}

func requireSV1DString(config map[string]any, key, expected string) error {
	actual, ok := config[key].(string)
	if !ok || actual != expected {
		return fmt.Errorf("%s must be %q", key, expected)
	}
	return nil
}

func requireSV1DBool(config map[string]any, key string, expected bool) error {
	actual, ok := config[key].(bool)
	if !ok || actual != expected {
		return fmt.Errorf("%s must be %t", key, expected)
	}
	return nil
}

func requireSV1DInt(config map[string]any, key string, expected int64) error {
	actual, ok := config[key].(json.Number)
	if !ok {
		return fmt.Errorf("%s must be integer %d", key, expected)
	}
	parsed, err := actual.Int64()
	if err != nil || parsed != expected {
		return fmt.Errorf("%s must be integer %d", key, expected)
	}
	return nil
}

func requireSV1DStringSlice(config map[string]any, key string, expected []string) error {
	actual, ok := config[key].([]any)
	if !ok || len(actual) != len(expected) {
		return fmt.Errorf("%s has an unexpected string list", key)
	}
	for index, expectedValue := range expected {
		actualValue, ok := actual[index].(string)
		if !ok || actualValue != expectedValue {
			return fmt.Errorf("%s has an unexpected string list", key)
		}
	}
	return nil
}

func requireSV1DCanonicalValue(config map[string]any, key, expectedRaw string) error {
	actual, exists := config[key]
	if !exists {
		return fmt.Errorf("%s is missing", key)
	}
	expected, err := decodeSV1DJSONValue([]byte(expectedRaw))
	if err != nil {
		return fmt.Errorf("decode expected %s: %w", key, err)
	}
	if !sv1dJSONEqual(actual, expected) {
		return fmt.Errorf("%s differs from the registered value", key)
	}
	return nil
}
