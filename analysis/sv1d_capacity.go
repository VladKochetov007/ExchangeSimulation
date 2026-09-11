package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	SV1DCapacityPreflightContract   = "v2-r2-sv1d-binary-capacity-preflight-v1"
	SV1DCapacityPreflightPurpose    = "five_minute_sv1d_binary_evidence_capacity_preflight"
	SV1DCapacitySeed                = int64(977)
	SV1DCapacityDurationNano        = uint64(300_000_000_000)
	SV1DCapacityStartNano           = uint64(1_735_689_600_000_000_000)
	SV1DCapacityEndNano             = SV1DCapacityStartNano + SV1DCapacityDurationNano
	SV1DCapacitySampleIntervalNano  = uint64(250_000_000)
	SV1DCapacitySafetyReserveBytes  = uint64(2 * 1024 * 1024 * 1024)
	SV1DCapacityGOMAXPROCS          = 2
	SV1DCapacityGOMEMLIMIT          = "4GiB"
	SV1DCapacityEvidenceSchemaEpoch = uint32(4)
)

// SV1DCapacityAttestation is a measured, outcome-ineligible resource record.
// It is intentionally a different contract from the historical full-run JSON
// capacity envelope: a verifier must see completed capacity-only evidence and
// the resource measurements that justify the launch floor.
type SV1DCapacityAttestation struct {
	SchemaVersion                  int               `json:"schema_version"`
	Contract                       string            `json:"contract"`
	ScientificResultEligible       bool              `json:"scientific_result_eligible"`
	Purpose                        string            `json:"purpose"`
	ProbeID                        string            `json:"probe_id"`
	CapacitySeed                   int64             `json:"capacity_seed"`
	Horizon                        string            `json:"horizon"`
	DurationNano                   uint64            `json:"duration_nano"`
	SimulationStartNano            uint64            `json:"simulation_start_nano"`
	SimulationEndNano              uint64            `json:"simulation_end_nano"`
	SourceRevision                 string            `json:"source_revision"`
	TreeRevision                   string            `json:"tree_revision"`
	ReviewAttestationSHA256        string            `json:"review_attestation_sha256"`
	ReviewReportSHA256             string            `json:"review_report_sha256"`
	TrustedReviewKeySHA256         string            `json:"trusted_review_key_sha256"`
	PlanSHA256                     string            `json:"plan_sha256"`
	TargetTreatmentConfigSHA256    string            `json:"target_treatment_config_sha256"`
	TargetModeOffConfigSHA256      string            `json:"target_mode_off_config_sha256"`
	TargetNoRosterConfigSHA256     string            `json:"target_no_roster_config_sha256"`
	CapacityTreatmentConfigSHA256  string            `json:"capacity_treatment_config_sha256"`
	CapacityModeOffConfigSHA256    string            `json:"capacity_mode_off_config_sha256"`
	CapacityNoRosterConfigSHA256   string            `json:"capacity_no_roster_config_sha256"`
	CapacityConfigDeltaSHA256      string            `json:"capacity_config_delta_sha256"`
	BinarySHA256                   string            `json:"binary_sha256"`
	AnalyzerSHA256                 string            `json:"analyzer_sha256"`
	RendererSHA256                 string            `json:"renderer_sha256"`
	RunnerSHA256                   string            `json:"runner_sha256"`
	MeasurerSHA256                 string            `json:"measurer_sha256"`
	ResourcePolicySHA256           string            `json:"resource_policy_sha256"`
	EvidenceFormat                 string            `json:"evidence_format"`
	EvidenceSchemaEpoch            uint32            `json:"evidence_schema_epoch"`
	LogMode                        string            `json:"log_mode"`
	GOMAXPROCS                     int               `json:"gomaxprocs"`
	GOMEMLIMIT                     string            `json:"gomemlimit"`
	OutputParent                   string            `json:"output_parent"`
	MeasurementRoot                string            `json:"measurement_root"`
	MeasurementRecordsRoot         string            `json:"measurement_records_root"`
	MeasurementRecordsSHA256       string            `json:"measurement_records_sha256"`
	FilesystemDevice               string            `json:"filesystem_device"`
	FilesystemID                   string            `json:"filesystem_id"`
	FilesystemType                 string            `json:"filesystem_type"`
	FilesystemMountID              string            `json:"filesystem_mount_id"`
	FilesystemUUID                 string            `json:"filesystem_uuid"`
	SameFilesystem                 bool              `json:"same_filesystem"`
	InitialAvailableBytes          uint64            `json:"initial_available_bytes"`
	MinimumAvailableBytes          uint64            `json:"minimum_available_bytes"`
	FinalAvailableBytes            uint64            `json:"final_available_bytes"`
	PeakApparentBytes              uint64            `json:"peak_apparent_bytes"`
	PeakAllocatedBytes             uint64            `json:"peak_allocated_bytes"`
	PeakFilesystemConsumptionBytes uint64            `json:"peak_filesystem_consumption_bytes"`
	MeasuredPeakBytes              uint64            `json:"measured_peak_bytes"`
	SafetyReserveBytes             uint64            `json:"safety_reserve_bytes"`
	RequiredFreeBytes              uint64            `json:"required_free_bytes"`
	PeakProcessTreeRSSBytes        uint64            `json:"peak_process_tree_rss_bytes"`
	RequiredAvailableMemoryBytes   uint64            `json:"required_available_memory_bytes"`
	PeakCgroupMemoryBytes          uint64            `json:"peak_cgroup_memory_bytes"`
	CgroupMemoryLimitBytes         uint64            `json:"cgroup_memory_limit_bytes"`
	MinimumHostMemAvailableBytes   uint64            `json:"minimum_host_mem_available_bytes"`
	SwapUsedBytes                  uint64            `json:"swap_used_bytes"`
	SampleIntervalNano             uint64            `json:"sample_interval_nano"`
	MaximumSampleGapNano           uint64            `json:"maximum_sample_gap_nano"`
	SampleCount                    uint64            `json:"sample_count"`
	SamplesSHA256                  string            `json:"samples_sha256"`
	OOMEventsDelta                 uint64            `json:"oom_events_delta"`
	OOMKillEventsDelta             uint64            `json:"oom_kill_events_delta"`
	CgroupOOMEventsDelta           uint64            `json:"cgroup_oom_events_delta"`
	CgroupOOMKillEventsDelta       uint64            `json:"cgroup_oom_kill_events_delta"`
	Arms                           []SV1DCapacityArm `json:"arms"`
}

// SV1DCapacityArm is the retained completion and evidence identity for one
// capacity-only counterpart. It contains no outcome score or market metric.
type SV1DCapacityArm struct {
	Name                            string `json:"name"`
	CapacityExperimentID            string `json:"capacity_experiment_id"`
	CapacityHypothesisID            string `json:"capacity_hypothesis_id"`
	CapacityConfigSHA256            string `json:"capacity_config_sha256"`
	Complete                        bool   `json:"complete"`
	ExitStatus                      int    `json:"exit_status"`
	SimulationStartNano             uint64 `json:"simulation_start_nano"`
	SimulationEndNano               uint64 `json:"simulation_end_nano"`
	RunMetadataSHA256               string `json:"run_metadata_sha256"`
	ManifestSHA256                  string `json:"manifest_sha256"`
	RunStatusSHA256                 string `json:"run_status_sha256"`
	EvidenceManifestSHA256          string `json:"evidence_manifest_sha256"`
	BinaryEvidenceAttestationSHA256 string `json:"binary_evidence_attestation_sha256"`
	EventsSHA256                    string `json:"events_sha256"`
	ExecutionStreamHash             string `json:"execution_stream_hash"`
	RendererReportSHA256            string `json:"renderer_report_sha256"`
	RendererAttestationSHA256       string `json:"renderer_attestation_sha256"`
	RenderedTreeDigest              string `json:"rendered_tree_digest"`
	EventFrames                     uint64 `json:"event_frames"`
	StreamFrames                    uint64 `json:"stream_frames"`
	PeakApparentBytes               uint64 `json:"peak_apparent_bytes"`
	PeakAllocatedBytes              uint64 `json:"peak_allocated_bytes"`
	PeakProcessTreeRSSBytes         uint64 `json:"peak_process_tree_rss_bytes"`
	ResourceMeasurementSHA256       string `json:"resource_measurement_sha256"`
	CapacityArmRecordSHA256         string `json:"capacity_arm_record_sha256,omitempty"`
}

// SV1DCapacityExpectation is supplied by the launcher from independently
// resolved files and tools. Empty values are never wildcards.
type SV1DCapacityExpectation struct {
	SourceRevision                string
	TreeRevision                  string
	ProbeID                       string
	PlanSHA256                    string
	ReviewAttestationSHA256       string
	ReviewReportSHA256            string
	TrustedReviewKeySHA256        string
	TargetTreatmentConfigSHA256   string
	TargetModeOffConfigSHA256     string
	TargetNoRosterConfigSHA256    string
	CapacityTreatmentConfigSHA256 string
	CapacityModeOffConfigSHA256   string
	CapacityNoRosterConfigSHA256  string
	CapacityConfigDeltaSHA256     string
	BinarySHA256                  string
	AnalyzerSHA256                string
	RendererSHA256                string
	RunnerSHA256                  string
	MeasurerSHA256                string
	ResourcePolicySHA256          string
	OutputParent                  string
	MeasurementRoot               string
	MeasurementRecordsRoot        string
	MeasurementRecordsSHA256      string
	FilesystemDevice              string
	FilesystemID                  string
	FilesystemType                string
	FilesystemMountID             string
	FilesystemUUID                string
}

type sv1dCapacityMeasurementManifest struct {
	SchemaVersion   int                        `json:"schema_version"`
	Contract        string                     `json:"contract"`
	MeasurementRoot string                     `json:"measurement_root"`
	SampleAggregate sv1dCapacityManifestFile   `json:"sample_aggregate"`
	Files           []sv1dCapacityManifestFile `json:"files"`
}

type sv1dCapacityManifestFile struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type sv1dCapacitySampleAggregateEntry struct {
	Arm    string             `json:"arm"`
	Sample SV1DResourceSample `json:"sample"`
}

type sv1dCapacityRunMetadata struct {
	SchemaVersion            int    `json:"schema_version"`
	RunnerContract           string `json:"runner_contract"`
	ProbeID                  string `json:"probe_id"`
	CapacityOnly             bool   `json:"capacity_only"`
	ScientificResultEligible bool   `json:"scientific_result_eligible"`
	Arm                      string `json:"arm"`
	ExperimentID             string `json:"experiment_id"`
	ConfigExperimentID       string `json:"config_experiment_id"`
	HypothesisID             string `json:"hypothesis_id"`
	Seed                     int64  `json:"seed"`
	SimulatedHorizon         string `json:"simulated_horizon"`
	SimulationStartNano      int64  `json:"simulation_start_nano"`
	SimulationEndNano        int64  `json:"simulation_end_nano"`
	ConfigSHA256             string `json:"config_sha256"`
	BinarySHA256             string `json:"binary_sha256"`
	GitRevision              string `json:"git_revision"`
	BinaryPath               string `json:"binary_path"`
	BinaryGoVersion          string `json:"binary_go_version"`
	BinaryGOOS               string `json:"binary_goos"`
	BinaryGOARCH             string `json:"binary_goarch"`
	BinaryGOAMD64            string `json:"binary_goamd64"`
	AnalyzerSHA256           string `json:"analyzer_sha256"`
	RendererSHA256           string `json:"renderer_sha256"`
	LogMode                  string `json:"log_mode"`
	EvidenceFormat           string `json:"evidence_format"`
	GOMAXPROCS               int    `json:"gomaxprocs"`
	OutputDir                string `json:"output_dir"`
	Holdout                  bool   `json:"holdout"`
}

type sv1dCapacityRunStatus struct {
	SchemaVersion                   int      `json:"schema_version"`
	Contract                        string   `json:"contract"`
	CapacityOnly                    bool     `json:"capacity_only"`
	ScientificResultEligible        bool     `json:"scientific_result_eligible"`
	Cell                            string   `json:"cell"`
	ExperimentID                    string   `json:"experiment_id"`
	ConfigExperimentID              string   `json:"config_experiment_id"`
	HypothesisID                    string   `json:"hypothesis_id"`
	ExitStatus                      int      `json:"exit_status"`
	CompletionVerified              bool     `json:"completion_verified"`
	SimulatedHorizon                string   `json:"simulated_horizon"`
	SimulationStartNano             int64    `json:"simulation_start_nano"`
	SimulationEndNano               int64    `json:"simulation_end_nano"`
	CompletionSentinels             []string `json:"completion_sentinels"`
	RunMetadataSHA256               string   `json:"run_metadata_sha256"`
	ManifestSHA256                  string   `json:"manifest_sha256"`
	GreeksSHA256                    string   `json:"greeks_sha256"`
	LatencySHA256                   string   `json:"latency_sha256"`
	CheckpointsSHA256               string   `json:"checkpoints_sha256"`
	EvidenceManifestSHA256          string   `json:"evidence_manifest_sha256"`
	BinaryEvidenceAttestationSHA256 string   `json:"binary_evidence_attestation_sha256"`
}

type sv1dCapacityRendererReport struct {
	EventFrames         uint64 `json:"event_frames"`
	DictionaryFrames    uint64 `json:"dictionary_frames"`
	StreamFrames        uint64 `json:"stream_frames"`
	ExecutionStreamHash string `json:"execution_stream_hash"`
	Routes              uint64 `json:"routes"`
	RenderedDigest      string `json:"rendered_digest"`
}

type sv1dCapacityArmSnapshot struct {
	armFiles      map[string][]byte
	renderedFiles map[string][]byte
	retainedFiles map[string][]byte
	configRaw     []byte
}

var sv1dCapacityArmArtifactNames = []string{
	"run-config.json", "run-metadata.json", "manifest.json", "run-status.json",
	"evidence-manifest.json", "binary-evidence-attestation.json", "events.evs",
	"renderer-report.json", "greeks.json", "latency.json", "checkpoints.jsonl",
	"market-data-evidence-v2.json", "market-data-schedules-v2.bin",
	"market-data-receipts-v2.bin", "market-data-decisions-v2.bin",
}

func snapshotSV1DCapacityArm(attestation SV1DCapacityAttestation, arm SV1DCapacityArm) (sv1dCapacityArmSnapshot, error) {
	armDir := filepath.Join(attestation.MeasurementRoot, "arms", arm.Name)
	renderedDir := filepath.Join(attestation.MeasurementRoot, "rendered", arm.Name)
	snapshot := sv1dCapacityArmSnapshot{
		armFiles:      make(map[string][]byte, len(sv1dCapacityArmArtifactNames)),
		renderedFiles: make(map[string][]byte),
		retainedFiles: make(map[string][]byte, 1),
	}
	for _, name := range sv1dCapacityArmArtifactNames {
		raw, err := readSV1DRegularFile(filepath.Join(armDir, name))
		if err != nil {
			return sv1dCapacityArmSnapshot{}, fmt.Errorf("capacity arm %s: read %s: %w", arm.Name, name, err)
		}
		snapshot.armFiles[name] = raw
	}
	configPath := filepath.Join(attestation.MeasurementRoot, "configs", "capacity-"+arm.Name+".json")
	configRaw, err := readSV1DRegularFile(configPath)
	if err != nil {
		return sv1dCapacityArmSnapshot{}, fmt.Errorf("capacity arm %s: read retained config: %w", arm.Name, err)
	}
	snapshot.configRaw = configRaw
	binaryPath := filepath.Join(attestation.MeasurementRoot, "tools", "multivenue-"+attestation.BinarySHA256)
	binaryRaw, err := readSV1DRegularFile(binaryPath)
	if err != nil {
		return sv1dCapacityArmSnapshot{}, fmt.Errorf("capacity arm %s: read retained simulator: %w", arm.Name, err)
	}
	snapshot.retainedFiles["simulator"] = binaryRaw
	if err := filepath.WalkDir(renderedDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("capacity arm %s: rendered tree contains a symlink", arm.Name)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(renderedDir, path)
		if err != nil {
			return err
		}
		raw, err := readSV1DRegularFile(path)
		if err != nil {
			return err
		}
		snapshot.renderedFiles[filepath.ToSlash(relative)] = raw
		return nil
	}); err != nil {
		return sv1dCapacityArmSnapshot{}, fmt.Errorf("capacity arm %s: snapshot rendered tree: %w", arm.Name, err)
	}
	return snapshot, nil
}

// ValidateSV1DCapacityAttestation checks intrinsic completeness and formulas.
// It does not trust any source/config/tool identity until Verify... compares
// them with an external expectation.
func ValidateSV1DCapacityAttestation(attestation SV1DCapacityAttestation) error {
	if attestation.SchemaVersion != 1 || attestation.Contract != SV1DCapacityPreflightContract || attestation.ScientificResultEligible || attestation.Purpose != SV1DCapacityPreflightPurpose || attestation.ProbeID == "" || attestation.CapacitySeed != SV1DCapacitySeed || attestation.Horizon != "5m" || attestation.DurationNano != SV1DCapacityDurationNano || attestation.SimulationStartNano != SV1DCapacityStartNano || attestation.SimulationEndNano != SV1DCapacityEndNano {
		return fmt.Errorf("SV1D capacity attestation has an invalid contract, purpose, seed, or horizon")
	}
	if !isCDFHex(attestation.SourceRevision, 20) || !isCDFHex(attestation.TreeRevision, 20) {
		return fmt.Errorf("SV1D capacity attestation has an invalid source or tree revision")
	}
	for name, value := range map[string]string{
		"review attestation":        attestation.ReviewAttestationSHA256,
		"review report":             attestation.ReviewReportSHA256,
		"trusted review key":        attestation.TrustedReviewKeySHA256,
		"plan":                      attestation.PlanSHA256,
		"target treatment config":   attestation.TargetTreatmentConfigSHA256,
		"target mode-off config":    attestation.TargetModeOffConfigSHA256,
		"target no-roster config":   attestation.TargetNoRosterConfigSHA256,
		"capacity treatment config": attestation.CapacityTreatmentConfigSHA256,
		"capacity mode-off config":  attestation.CapacityModeOffConfigSHA256,
		"capacity no-roster config": attestation.CapacityNoRosterConfigSHA256,
		"capacity config delta":     attestation.CapacityConfigDeltaSHA256,
		"simulator":                 attestation.BinarySHA256,
		"analyzer":                  attestation.AnalyzerSHA256,
		"renderer":                  attestation.RendererSHA256,
		"runner":                    attestation.RunnerSHA256,
		"measurer":                  attestation.MeasurerSHA256,
		"resource policy":           attestation.ResourcePolicySHA256,
		"samples":                   attestation.SamplesSHA256,
		"measurement records":       attestation.MeasurementRecordsSHA256,
	} {
		if !isSV1DHexDigest(value) {
			return fmt.Errorf("SV1D capacity attestation has an invalid %s digest", name)
		}
	}
	if attestation.EvidenceFormat != "evstream_v3" || attestation.EvidenceSchemaEpoch != SV1DCapacityEvidenceSchemaEpoch || attestation.LogMode != "full" || attestation.GOMAXPROCS != SV1DCapacityGOMAXPROCS || attestation.GOMEMLIMIT != SV1DCapacityGOMEMLIMIT || !attestation.SameFilesystem || !absoluteCleanPath(attestation.OutputParent) || !absoluteCleanPath(attestation.MeasurementRoot) || !absoluteCleanPath(attestation.MeasurementRecordsRoot) || attestation.FilesystemDevice == "" || attestation.FilesystemID == "" || attestation.FilesystemType == "" || attestation.FilesystemMountID == "" || attestation.FilesystemUUID == "" {
		return fmt.Errorf("SV1D capacity attestation has incomplete runtime or filesystem identity")
	}
	if attestation.MinimumAvailableBytes > attestation.InitialAvailableBytes || attestation.FinalAvailableBytes == 0 || attestation.SafetyReserveBytes < SV1DCapacitySafetyReserveBytes || attestation.PeakApparentBytes == 0 || attestation.PeakAllocatedBytes == 0 || attestation.PeakProcessTreeRSSBytes == 0 || attestation.PeakCgroupMemoryBytes < attestation.PeakProcessTreeRSSBytes || attestation.CgroupMemoryLimitBytes < attestation.PeakCgroupMemoryBytes || attestation.MinimumHostMemAvailableBytes == 0 || attestation.SwapUsedBytes != 0 || attestation.OOMEventsDelta != 0 || attestation.OOMKillEventsDelta != 0 || attestation.CgroupOOMEventsDelta != 0 || attestation.CgroupOOMKillEventsDelta != 0 {
		return fmt.Errorf("SV1D capacity attestation has unsafe resource measurements")
	}
	peakFilesystemConsumption := attestation.InitialAvailableBytes - attestation.MinimumAvailableBytes
	if peakFilesystemConsumption != attestation.PeakFilesystemConsumptionBytes {
		return fmt.Errorf("SV1D capacity filesystem consumption does not match available-space samples")
	}
	measuredPeak := maxCapacityValue(attestation.PeakApparentBytes, attestation.PeakAllocatedBytes, attestation.PeakFilesystemConsumptionBytes)
	if measuredPeak != attestation.MeasuredPeakBytes {
		return fmt.Errorf("SV1D capacity measured peak is not the maximum retained footprint")
	}
	requiredFree, ok := capacityRequiredFree(measuredPeak, attestation.SafetyReserveBytes)
	if !ok || requiredFree != attestation.RequiredFreeBytes {
		return fmt.Errorf("SV1D capacity free-space formula is invalid")
	}
	requiredMemory, ok := capacityRequiredMemory(attestation.PeakProcessTreeRSSBytes)
	if !ok || requiredMemory != attestation.RequiredAvailableMemoryBytes {
		return fmt.Errorf("SV1D capacity memory-headroom formula is invalid")
	}
	if attestation.SampleIntervalNano != SV1DCapacitySampleIntervalNano || attestation.MaximumSampleGapNano == 0 || attestation.MaximumSampleGapNano > attestation.SampleIntervalNano || attestation.SampleCount < 2 {
		return fmt.Errorf("SV1D capacity resource sampling does not cover the measured interval")
	}
	if err := validateSV1DCapacityArms(attestation); err != nil {
		return err
	}
	return nil
}

// VerifySV1DCapacityAttestation combines intrinsic validation with the exact
// identities resolved by the launch policy.
func VerifySV1DCapacityAttestation(path string, expected SV1DCapacityExpectation) (SV1DCapacityAttestation, error) {
	var attestation SV1DCapacityAttestation
	raw, err := readSV1DRegularFile(path)
	if err != nil {
		return attestation, fmt.Errorf("read SV1D capacity attestation: %w", err)
	}
	if err := rejectSV1DDuplicateJSONKeys(raw); err != nil {
		return attestation, fmt.Errorf("SV1D capacity attestation has malformed JSON: %w", err)
	}
	if err := validateSV1DCapacityAttestationJSONPresence(raw); err != nil {
		return attestation, fmt.Errorf("SV1D capacity attestation omits a required field: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attestation); err != nil {
		return attestation, fmt.Errorf("decode SV1D capacity attestation: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return attestation, fmt.Errorf("SV1D capacity attestation has multiple top-level JSON values")
		}
		return attestation, fmt.Errorf("SV1D capacity attestation has trailing JSON: %w", err)
	}
	if err := ValidateSV1DCapacityAttestation(attestation); err != nil {
		return attestation, err
	}
	if err := compareSV1DCapacityExpectation(attestation, expected); err != nil {
		return attestation, err
	}
	if err := verifySV1DCapacityRetainedInputs(attestation); err != nil {
		return attestation, err
	}
	if err := verifySV1DCapacityMeasurementRecords(attestation, expected); err != nil {
		return attestation, err
	}
	return attestation, nil
}

func compareSV1DCapacityExpectation(attestation SV1DCapacityAttestation, expected SV1DCapacityExpectation) error {
	if expected.ProbeID == "" || attestation.ProbeID != expected.ProbeID || attestation.SourceRevision != expected.SourceRevision || attestation.TreeRevision != expected.TreeRevision || attestation.PlanSHA256 != expected.PlanSHA256 || attestation.ReviewAttestationSHA256 != expected.ReviewAttestationSHA256 || attestation.ReviewReportSHA256 != expected.ReviewReportSHA256 || attestation.TrustedReviewKeySHA256 != expected.TrustedReviewKeySHA256 {
		return fmt.Errorf("SV1D capacity attestation does not match source, tree, plan, or review identity")
	}
	if attestation.TargetTreatmentConfigSHA256 != expected.TargetTreatmentConfigSHA256 || attestation.TargetModeOffConfigSHA256 != expected.TargetModeOffConfigSHA256 || attestation.TargetNoRosterConfigSHA256 != expected.TargetNoRosterConfigSHA256 || attestation.CapacityTreatmentConfigSHA256 != expected.CapacityTreatmentConfigSHA256 || attestation.CapacityModeOffConfigSHA256 != expected.CapacityModeOffConfigSHA256 || attestation.CapacityNoRosterConfigSHA256 != expected.CapacityNoRosterConfigSHA256 || attestation.CapacityConfigDeltaSHA256 != expected.CapacityConfigDeltaSHA256 {
		return fmt.Errorf("SV1D capacity attestation does not match capacity-only config identities")
	}
	if attestation.BinarySHA256 != expected.BinarySHA256 || attestation.AnalyzerSHA256 != expected.AnalyzerSHA256 || attestation.RendererSHA256 != expected.RendererSHA256 || attestation.RunnerSHA256 != expected.RunnerSHA256 || attestation.MeasurerSHA256 != expected.MeasurerSHA256 || attestation.ResourcePolicySHA256 != expected.ResourcePolicySHA256 {
		return fmt.Errorf("SV1D capacity attestation does not match tool or policy identity")
	}
	if attestation.OutputParent != expected.OutputParent || attestation.MeasurementRoot != expected.MeasurementRoot || attestation.MeasurementRecordsRoot != expected.MeasurementRecordsRoot || attestation.MeasurementRecordsSHA256 != expected.MeasurementRecordsSHA256 || attestation.FilesystemDevice != expected.FilesystemDevice || attestation.FilesystemID != expected.FilesystemID || attestation.FilesystemType != expected.FilesystemType || attestation.FilesystemMountID != expected.FilesystemMountID || attestation.FilesystemUUID != expected.FilesystemUUID {
		return fmt.Errorf("SV1D capacity attestation does not match the measured filesystem")
	}
	return nil
}

func verifySV1DCapacityMeasurementRecords(attestation SV1DCapacityAttestation, expected SV1DCapacityExpectation) error {
	manifestPath := filepath.Join(expected.MeasurementRecordsRoot, "measurement-records-manifest.json")
	raw, err := readSV1DRegularFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read SV1D capacity measurement manifest: %w", err)
	}
	manifestDigest := sha256DigestHex(raw)
	if manifestDigest != attestation.MeasurementRecordsSHA256 {
		return fmt.Errorf("SV1D capacity measurement manifest digest does not match its attestation")
	}
	var manifest sv1dCapacityMeasurementManifest
	if err := decodeStrictSV1DCapacityJSON(raw, &manifest, jsonFieldNames(reflect.TypeOf(manifest), true)...); err != nil {
		return fmt.Errorf("decode SV1D capacity measurement manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || manifest.Contract != "v2-r2-sv1d-capacity-measurement-records-v1" || manifest.MeasurementRoot != attestation.MeasurementRoot || manifest.SampleAggregate.Path != "all-samples.json" || manifest.SampleAggregate.Bytes == 0 || manifest.SampleAggregate.SHA256 != attestation.SamplesSHA256 || len(manifest.Files) != 7 {
		return fmt.Errorf("SV1D capacity measurement manifest has an invalid identity or file set")
	}
	expectedPaths := []string{
		"treatment-resource-measurement.json", "mode-off-resource-measurement.json", "no-roster-resource-measurement.json",
		"treatment-record.json", "mode-off-record.json", "no-roster-record.json", "all-samples.json",
	}
	expectedArmNames := []string{"treatment", "mode-off", "no-roster"}
	resourceMeasurements := make([]SV1DResourceMeasurement, len(expectedArmNames))
	for index, file := range manifest.Files {
		if file.Path != expectedPaths[index] || file.Bytes == 0 || !isSV1DHexDigest(file.SHA256) || filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "../") || file.Path == ".." {
			return fmt.Errorf("SV1D capacity measurement manifest has an invalid file entry")
		}
		fileRaw, err := readSV1DRegularFile(filepath.Join(expected.MeasurementRecordsRoot, file.Path))
		if err != nil {
			return fmt.Errorf("read SV1D capacity measurement record %s: %w", file.Path, err)
		}
		if uint64(len(fileRaw)) != file.Bytes || sha256DigestHex(fileRaw) != file.SHA256 {
			return fmt.Errorf("SV1D capacity measurement record %s does not match its manifest", file.Path)
		}
		if strings.HasSuffix(file.Path, "-resource-measurement.json") {
			armIndex := index
			if armIndex >= len(expectedArmNames) {
				return fmt.Errorf("SV1D capacity measurement manifest has too many resource records")
			}
			if err := validateSV1DResourceMeasurementJSONPresence(fileRaw); err != nil {
				return fmt.Errorf("validate SV1D resource measurement %s: %w", file.Path, err)
			}
			var measurement SV1DResourceMeasurement
			if err := decodeStrictSV1DCapacityJSON(fileRaw, &measurement, jsonFieldNames(reflect.TypeOf(measurement), false)...); err != nil {
				return fmt.Errorf("decode SV1D resource measurement %s: %w", file.Path, err)
			}
			if err := ValidateSV1DResourceMeasurement(measurement, true); err != nil {
				return fmt.Errorf("validate SV1D resource measurement %s: %w", file.Path, err)
			}
			if measurement.MeasurementRoot != attestation.MeasurementRoot || measurement.OutputParent != attestation.OutputParent || measurement.SampleIntervalNano != SV1DCapacitySampleIntervalNano {
				return fmt.Errorf("SV1D resource measurement %s is not bound to the registered measurement", file.Path)
			}
			if err := validateSV1DCapacityResourceCommand(measurement, attestation, attestation.Arms[armIndex]); err != nil {
				return fmt.Errorf("SV1D resource measurement %s has an unbound command: %w", file.Path, err)
			}
			if !sameSV1DFilesystem(measurement.Filesystem, SV1DFilesystemIdentity{
				Device: attestation.FilesystemDevice, ID: attestation.FilesystemID, Type: attestation.FilesystemType,
				MountID: attestation.FilesystemMountID, UUID: attestation.FilesystemUUID,
			}) {
				return fmt.Errorf("SV1D resource measurement %s is not bound to the attested filesystem", file.Path)
			}
			if file.SHA256 != attestation.Arms[armIndex].ResourceMeasurementSHA256 {
				return fmt.Errorf("SV1D resource measurement %s is not bound to its capacity arm", file.Path)
			}
			resourceMeasurements[armIndex] = measurement
		} else if strings.HasSuffix(file.Path, "-record.json") {
			armIndex := index - len(expectedArmNames)
			if armIndex < 0 || armIndex >= len(expectedArmNames) {
				return fmt.Errorf("SV1D capacity measurement manifest has too many arm records")
			}
			var arm SV1DCapacityArm
			if err := decodeStrictSV1DCapacityJSON(fileRaw, &arm, jsonFieldNames(reflect.TypeOf(arm), false)...); err != nil {
				return fmt.Errorf("decode SV1D capacity arm record %s: %w", file.Path, err)
			}
			expectedArm := attestation.Arms[armIndex]
			if file.SHA256 != expectedArm.CapacityArmRecordSHA256 {
				return fmt.Errorf("SV1D capacity arm record %s is not bound to its attestation", file.Path)
			}
			expectedArm.CapacityArmRecordSHA256 = ""
			if arm != expectedArm {
				return fmt.Errorf("SV1D capacity arm record %s does not match its attestation", file.Path)
			}
		}
	}
	for armIndex := range expectedArmNames {
		if err := verifySV1DCapacityArmArtifacts(attestation, armIndex); err != nil {
			return err
		}
	}
	sampleRaw, err := readSV1DRegularFile(filepath.Join(expected.MeasurementRecordsRoot, manifest.SampleAggregate.Path))
	if err != nil {
		return fmt.Errorf("read SV1D capacity sample aggregate: %w", err)
	}
	if uint64(len(sampleRaw)) == 0 || uint64(len(sampleRaw)) != manifest.SampleAggregate.Bytes || sha256DigestHex(sampleRaw) != manifest.SampleAggregate.SHA256 {
		return fmt.Errorf("SV1D capacity sample aggregate does not match its manifest")
	}
	if err := validateSV1DCapacitySampleAggregateJSONPresence(sampleRaw); err != nil {
		return fmt.Errorf("SV1D capacity sample aggregate is incomplete or malformed: %w", err)
	}
	var samples []sv1dCapacitySampleAggregateEntry
	if err := decodeStrictSV1DCapacityJSON(sampleRaw, &samples); err != nil || len(samples) == 0 {
		return fmt.Errorf("SV1D capacity sample aggregate is incomplete or malformed")
	}
	var expectedSampleCount uint64
	sampleIndex := 0
	for armIndex, armName := range expectedArmNames {
		measurement := resourceMeasurements[armIndex]
		expectedSampleCount += measurement.SampleCount
		if attestation.Arms[armIndex].PeakApparentBytes != measurement.PeakApparentBytes || attestation.Arms[armIndex].PeakAllocatedBytes != measurement.PeakAllocatedBytes || attestation.Arms[armIndex].PeakProcessTreeRSSBytes != measurement.PeakProcessTreeRSSBytes {
			return fmt.Errorf("capacity arm %s does not report its retained resource peaks", armName)
		}
		for _, expectedSample := range measurement.Samples {
			if sampleIndex >= len(samples) || samples[sampleIndex].Arm != armName || samples[sampleIndex].Sample != expectedSample {
				return fmt.Errorf("SV1D capacity sample aggregate differs from the %s resource trace", armName)
			}
			sampleIndex++
		}
	}
	if uint64(len(samples)) != expectedSampleCount || uint64(sampleIndex) != expectedSampleCount || attestation.SampleCount != expectedSampleCount {
		return fmt.Errorf("SV1D capacity sample aggregate count does not match the retained resource traces")
	}
	if err := validateSV1DCapacityResourceAggregates(attestation, resourceMeasurements); err != nil {
		return err
	}
	return nil
}

func validateSV1DCapacityResourceAggregates(attestation SV1DCapacityAttestation, measurements []SV1DResourceMeasurement) error {
	if len(measurements) != 3 {
		return fmt.Errorf("SV1D capacity resource aggregate has an invalid arm count")
	}
	initialAvailable := measurements[0].InitialAvailableBytes
	minimumAvailable := measurements[0].MinimumAvailableBytes
	finalAvailable := measurements[len(measurements)-1].FinalAvailableBytes
	peakApparent := measurements[0].PeakApparentBytes
	peakAllocated := measurements[0].PeakAllocatedBytes
	peakRSS := measurements[0].PeakProcessTreeRSSBytes
	peakCgroup := measurements[0].PeakCgroupMemoryBytes
	cgroupLimit := measurements[0].CgroupMemoryLimitBytes
	minimumHostAvailable := measurements[0].MinimumHostMemAvailableBytes
	maximumSwap := measurements[0].MaximumSwapUsedBytes
	maximumGap := measurements[0].MaximumSampleGapNano
	var sampleCount, oomEvents, oomKillEvents, cgroupOOMEvents, cgroupOOMKillEvents uint64
	for _, measurement := range measurements {
		if measurement.MinimumAvailableBytes < minimumAvailable {
			minimumAvailable = measurement.MinimumAvailableBytes
		}
		if measurement.PeakApparentBytes > peakApparent {
			peakApparent = measurement.PeakApparentBytes
		}
		if measurement.PeakAllocatedBytes > peakAllocated {
			peakAllocated = measurement.PeakAllocatedBytes
		}
		if measurement.PeakProcessTreeRSSBytes > peakRSS {
			peakRSS = measurement.PeakProcessTreeRSSBytes
		}
		if measurement.PeakCgroupMemoryBytes > peakCgroup {
			peakCgroup = measurement.PeakCgroupMemoryBytes
		}
		if measurement.CgroupMemoryLimitBytes < cgroupLimit {
			cgroupLimit = measurement.CgroupMemoryLimitBytes
		}
		if measurement.MinimumHostMemAvailableBytes < minimumHostAvailable {
			minimumHostAvailable = measurement.MinimumHostMemAvailableBytes
		}
		if measurement.MaximumSwapUsedBytes > maximumSwap {
			maximumSwap = measurement.MaximumSwapUsedBytes
		}
		if measurement.MaximumSampleGapNano > maximumGap {
			maximumGap = measurement.MaximumSampleGapNano
		}
		sampleCount += measurement.SampleCount
		oomEvents += measurement.CgroupOOMEventsDelta
		oomKillEvents += measurement.CgroupOOMKillEventsDelta
		cgroupOOMEvents += measurement.CgroupLocalOOMDelta
		cgroupOOMKillEvents += measurement.CgroupLocalOOMKillDelta
	}
	if attestation.InitialAvailableBytes != initialAvailable || attestation.MinimumAvailableBytes != minimumAvailable || attestation.FinalAvailableBytes != finalAvailable || attestation.PeakApparentBytes != peakApparent || attestation.PeakAllocatedBytes != peakAllocated || attestation.PeakProcessTreeRSSBytes != peakRSS || attestation.PeakCgroupMemoryBytes != peakCgroup || attestation.CgroupMemoryLimitBytes != cgroupLimit || attestation.MinimumHostMemAvailableBytes != minimumHostAvailable || attestation.SwapUsedBytes != maximumSwap || attestation.MaximumSampleGapNano != maximumGap || attestation.SampleCount != sampleCount || attestation.OOMEventsDelta != oomEvents || attestation.OOMKillEventsDelta != oomKillEvents || attestation.CgroupOOMEventsDelta != cgroupOOMEvents || attestation.CgroupOOMKillEventsDelta != cgroupOOMKillEvents {
		return fmt.Errorf("SV1D capacity attestation aggregates do not match retained resource measurements")
	}
	if initialAvailable < minimumAvailable || initialAvailable-minimumAvailable != attestation.PeakFilesystemConsumptionBytes {
		return fmt.Errorf("SV1D capacity filesystem consumption does not match retained resource measurements")
	}
	return nil
}

func decodeStrictSV1DCapacityJSON(raw []byte, target any, requiredFields ...string) error {
	if err := rejectSV1DDuplicateJSONKeys(raw); err != nil {
		return err
	}
	if len(requiredFields) > 0 {
		if err := requireSV1DJSONFields(raw, requiredFields...); err != nil {
			return err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple top-level JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func jsonFieldNames(typ reflect.Type, includeOptional bool) []string {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	fields := make([]string, 0, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		parts := strings.Split(jsonTag, ",")
		if !includeOptional && len(parts) > 1 && parts[1] == "omitempty" {
			continue
		}
		fields = append(fields, parts[0])
	}
	return fields
}

func requireSV1DJSONFields(raw []byte, fields ...string) error {
	var object map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil {
		return fmt.Errorf("decode object for required fields: %w", err)
	}
	if object == nil {
		return errors.New("required fields must be in a JSON object")
	}
	for _, field := range fields {
		value, ok := object[field]
		if !ok {
			return fmt.Errorf("missing %s", field)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("null %s", field)
		}
	}
	return nil
}

func validateSV1DCapacityAttestationJSONPresence(raw []byte) error {
	if err := requireSV1DJSONFields(raw, jsonFieldNames(reflect.TypeOf(SV1DCapacityAttestation{}), true)...); err != nil {
		return err
	}
	var envelope struct {
		Arms []json.RawMessage `json:"arms"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode arms: %w", err)
	}
	if len(envelope.Arms) != 3 {
		return fmt.Errorf("arms has %d records, want 3", len(envelope.Arms))
	}
	for index, armRaw := range envelope.Arms {
		if err := requireSV1DJSONFields(armRaw, jsonFieldNames(reflect.TypeOf(SV1DCapacityArm{}), true)...); err != nil {
			return fmt.Errorf("arm %d: %w", index, err)
		}
	}
	return nil
}

func validateSV1DResourceMeasurementJSONPresence(raw []byte) error {
	var envelope struct {
		Samples []json.RawMessage `json:"samples"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode samples: %w", err)
	}
	if envelope.Samples == nil {
		return errors.New("samples is missing or null")
	}
	for index, sampleRaw := range envelope.Samples {
		if err := requireSV1DJSONFields(sampleRaw, jsonFieldNames(reflect.TypeOf(SV1DResourceSample{}), true)...); err != nil {
			return fmt.Errorf("sample %d: %w", index, err)
		}
	}
	return nil
}

func validateSV1DCapacitySampleAggregateJSONPresence(raw []byte) error {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return fmt.Errorf("decode entries: %w", err)
	}
	if entries == nil {
		return errors.New("sample aggregate is not an array")
	}
	for index, entryRaw := range entries {
		if err := requireSV1DJSONFields(entryRaw, "arm", "sample"); err != nil {
			return fmt.Errorf("entry %d: %w", index, err)
		}
		var entry struct {
			Sample json.RawMessage `json:"sample"`
		}
		if err := json.Unmarshal(entryRaw, &entry); err != nil {
			return fmt.Errorf("decode entry %d: %w", index, err)
		}
		if err := requireSV1DJSONFields(entry.Sample, jsonFieldNames(reflect.TypeOf(SV1DResourceSample{}), true)...); err != nil {
			return fmt.Errorf("entry %d sample: %w", index, err)
		}
	}
	return nil
}

func validateSV1DCapacityResourceCommand(measurement SV1DResourceMeasurement, attestation SV1DCapacityAttestation, arm SV1DCapacityArm) error {
	root := attestation.MeasurementRoot
	expectedRunner := filepath.Join(root, "tools", "capacity-runner-"+attestation.RunnerSHA256+".sh")
	if len(measurement.Command) != 13 || measurement.Command[0] != expectedRunner {
		return errors.New("capacity runner command has an invalid wrapper")
	}
	runnerRaw, err := readSV1DRegularFile(expectedRunner)
	if err != nil || sha256DigestHex(runnerRaw) != attestation.RunnerSHA256 {
		return errors.New("capacity runner command is not bound to the retained wrapper")
	}
	expected := []string{
		"--internal-arm",
		arm.Name,
		filepath.Join(root, "configs", "capacity-"+arm.Name+".json"),
		filepath.Join(root, "arms", arm.Name),
		filepath.Join(root, "rendered", arm.Name),
		filepath.Join(root, "tools", "multivenue-"+attestation.BinarySHA256),
		filepath.Join(root, "tools", "sv1dprobe-"+attestation.AnalyzerSHA256),
		filepath.Join(root, "tools", "evsrender-"+attestation.RendererSHA256),
		attestation.SourceRevision,
		arm.CapacityExperimentID,
		filepath.Join(root, "logs", arm.Name+".simulator.stdout.log"),
		filepath.Join(root, "logs", arm.Name+".simulator.stderr.log"),
	}
	for index, value := range expected {
		if measurement.Command[index+1] != value {
			return fmt.Errorf("argument %d does not match the registered arm command", index+1)
		}
	}
	return nil
}

func verifySV1DCapacityArmArtifacts(attestation SV1DCapacityAttestation, armIndex int) error {
	arm := attestation.Arms[armIndex]
	armDir := filepath.Join(attestation.MeasurementRoot, "arms", arm.Name)
	renderedDir := filepath.Join(attestation.MeasurementRoot, "rendered", arm.Name)
	snapshot, err := snapshotSV1DCapacityArm(attestation, arm)
	if err != nil {
		return err
	}
	if err := verifySV1DCapacityArmArtifactDigests(attestation, arm, snapshot, armIndex); err != nil {
		return err
	}
	if err := verifySV1DCapacityArmRunMetadata(attestation, arm, armDir, snapshot); err != nil {
		return err
	}
	status, err := verifySV1DCapacityArmRunStatus(arm, armDir, snapshot)
	if err != nil {
		return err
	}
	evidenceManifest, err := verifySV1DCapacityArmEvidenceManifest(attestation, arm, armDir, snapshot)
	if err != nil {
		return err
	}
	if err := verifySV1DCapacityArmBinaryEvidence(attestation, arm, armDir, snapshot); err != nil {
		return err
	}
	if err := verifySV1DCapacityArmRenderer(attestation, arm, armDir, renderedDir, snapshot); err != nil {
		return err
	}
	if status.RunMetadataSHA256 != arm.RunMetadataSHA256 || status.ManifestSHA256 != arm.ManifestSHA256 || status.EvidenceManifestSHA256 != arm.EvidenceManifestSHA256 || status.BinaryEvidenceAttestationSHA256 != arm.BinaryEvidenceAttestationSHA256 || len(evidenceManifest.FixedFiles) == 0 {
		return fmt.Errorf("SV1D capacity arm %s: cross-artifact identity is incomplete", arm.Name)
	}
	return nil
}

func verifySV1DCapacityArmArtifactDigests(attestation SV1DCapacityAttestation, arm SV1DCapacityArm, snapshot sv1dCapacityArmSnapshot, armIndex int) error {
	artifactDigests := []struct {
		name   string
		digest string
	}{
		{"run-metadata.json", arm.RunMetadataSHA256},
		{"manifest.json", arm.ManifestSHA256},
		{"run-status.json", arm.RunStatusSHA256},
		{"evidence-manifest.json", arm.EvidenceManifestSHA256},
		{"binary-evidence-attestation.json", arm.BinaryEvidenceAttestationSHA256},
		{"events.evs", arm.EventsSHA256},
		{"renderer-report.json", arm.RendererReportSHA256},
	}
	for _, artifact := range artifactDigests {
		raw, ok := snapshot.armFiles[artifact.name]
		if !ok || len(raw) == 0 || sha256DigestHex(raw) != artifact.digest {
			return fmt.Errorf("SV1D capacity arm %s: %s is not bound to its attested digest", arm.Name, artifact.name)
		}
	}
	rendererAttestationRaw, ok := snapshot.renderedFiles["renderer-attestation.json"]
	if !ok || len(rendererAttestationRaw) == 0 || sha256DigestHex(rendererAttestationRaw) != arm.RendererAttestationSHA256 {
		return fmt.Errorf("SV1D capacity arm %s: rendered renderer-attestation.json is not bound to its attested digest", arm.Name)
	}
	configRaw := snapshot.configRaw
	if sha256DigestHex(configRaw) != arm.CapacityConfigSHA256 || sha256DigestHex(configRaw) != sv1dCapacityConfigDigest(attestation, armIndex) {
		return fmt.Errorf("SV1D capacity arm %s: retained config is not bound to its attestation", arm.Name)
	}
	runConfigRaw, ok := snapshot.armFiles["run-config.json"]
	if !ok {
		return fmt.Errorf("SV1D capacity arm %s: executed config is not retained", arm.Name)
	}
	if sha256DigestHex(runConfigRaw) != sha256DigestHex(configRaw) {
		return fmt.Errorf("SV1D capacity arm %s: executed config differs from retained capacity config", arm.Name)
	}
	return nil
}

func verifySV1DCapacityArmRunMetadata(attestation SV1DCapacityAttestation, arm SV1DCapacityArm, armDir string, snapshot sv1dCapacityArmSnapshot) error {
	metadataRaw, ok := snapshot.armFiles["run-metadata.json"]
	if !ok {
		return fmt.Errorf("SV1D capacity arm %s: run metadata is not retained", arm.Name)
	}
	var metadata sv1dCapacityRunMetadata
	if err := decodeSV1DJSONWithRequiredFields(metadataRaw, &metadata, jsonFieldNames(reflect.TypeOf(metadata), true)...); err != nil {
		return fmt.Errorf("SV1D capacity arm %s: decode run metadata: %w", arm.Name, err)
	}
	if metadata.SchemaVersion != 1 || metadata.RunnerContract != "v2-r2-sv1d-capacity-runner-v1" || metadata.ProbeID != attestation.ProbeID || !metadata.CapacityOnly || metadata.ScientificResultEligible || metadata.Arm != arm.Name || metadata.ExperimentID != arm.CapacityExperimentID || metadata.ConfigExperimentID != arm.CapacityExperimentID || metadata.HypothesisID != arm.CapacityHypothesisID || metadata.Seed != SV1DCapacitySeed || metadata.SimulatedHorizon != "5m" || metadata.SimulationStartNano != int64(SV1DCapacityStartNano) || metadata.SimulationEndNano != int64(SV1DCapacityEndNano) || metadata.ConfigSHA256 != arm.CapacityConfigSHA256 || metadata.BinarySHA256 != attestation.BinarySHA256 || metadata.GitRevision != attestation.SourceRevision || metadata.AnalyzerSHA256 != attestation.AnalyzerSHA256 || metadata.RendererSHA256 != attestation.RendererSHA256 || metadata.LogMode != "full" || metadata.EvidenceFormat != "evstream_v3" || metadata.GOMAXPROCS != SV1DCapacityGOMAXPROCS || metadata.OutputDir != armDir || metadata.Holdout {
		return fmt.Errorf("SV1D capacity arm %s: run metadata identity is inconsistent", arm.Name)
	}
	expectedBinaryPath := filepath.Join(attestation.MeasurementRoot, "tools", "multivenue-"+attestation.BinarySHA256)
	if metadata.BinaryPath != expectedBinaryPath || metadata.BinaryGoVersion == "" || metadata.BinaryGOOS != "linux" || metadata.BinaryGOARCH != "amd64" || metadata.BinaryGOAMD64 != "v1" {
		return fmt.Errorf("SV1D capacity arm %s: run metadata has an invalid simulator identity", arm.Name)
	}
	binaryRaw, ok := snapshot.retainedFiles["simulator"]
	if !ok || sha256DigestHex(binaryRaw) != attestation.BinarySHA256 {
		return fmt.Errorf("SV1D capacity arm %s: run metadata simulator is not retained", arm.Name)
	}
	return nil
}

func verifySV1DCapacityArmRunStatus(arm SV1DCapacityArm, armDir string, snapshot sv1dCapacityArmSnapshot) (sv1dCapacityRunStatus, error) {
	statusRaw, ok := snapshot.armFiles["run-status.json"]
	if !ok {
		return sv1dCapacityRunStatus{}, fmt.Errorf("SV1D capacity arm %s: run status is not retained", arm.Name)
	}
	var status sv1dCapacityRunStatus
	if err := decodeSV1DJSONWithRequiredFields(statusRaw, &status,
		"schema_version", "contract", "capacity_only", "scientific_result_eligible", "cell", "experiment_id", "config_experiment_id", "hypothesis_id", "exit_status", "completion_verified", "simulated_horizon", "simulation_start_nano", "simulation_end_nano", "completion_sentinels", "run_metadata_sha256", "manifest_sha256", "greeks_sha256", "latency_sha256", "checkpoints_sha256", "evidence_manifest_sha256", "binary_evidence_attestation_sha256"); err != nil {
		return sv1dCapacityRunStatus{}, fmt.Errorf("SV1D capacity arm %s: decode run status: %w", arm.Name, err)
	}
	if status.SchemaVersion != 1 || status.Contract != "v2-r2-sv1d-capacity-arm-status-v1" || !status.CapacityOnly || status.ScientificResultEligible || status.Cell != arm.Name || status.ExperimentID != arm.CapacityExperimentID || status.ConfigExperimentID != arm.CapacityExperimentID || status.HypothesisID != arm.CapacityHypothesisID || status.ExitStatus != 0 || !status.CompletionVerified || status.SimulatedHorizon != "5m" || status.SimulationStartNano != int64(SV1DCapacityStartNano) || status.SimulationEndNano != int64(SV1DCapacityEndNano) || !sameSV1DStrings(status.CompletionSentinels, []string{"greeks.json", "latency.json"}) || status.RunMetadataSHA256 != arm.RunMetadataSHA256 || status.ManifestSHA256 != arm.ManifestSHA256 || status.EvidenceManifestSHA256 != arm.EvidenceManifestSHA256 || status.BinaryEvidenceAttestationSHA256 != arm.BinaryEvidenceAttestationSHA256 {
		return sv1dCapacityRunStatus{}, fmt.Errorf("SV1D capacity arm %s: run status identity is inconsistent", arm.Name)
	}
	for _, artifact := range []struct {
		name   string
		digest string
	}{
		{"greeks.json", status.GreeksSHA256}, {"latency.json", status.LatencySHA256}, {"checkpoints.jsonl", status.CheckpointsSHA256},
	} {
		if !isSV1DHexDigest(artifact.digest) {
			return sv1dCapacityRunStatus{}, fmt.Errorf("SV1D capacity arm %s: run status has no valid %s digest", arm.Name, artifact.name)
		}
		raw, ok := snapshot.armFiles[artifact.name]
		if !ok || sha256DigestHex(raw) != artifact.digest {
			return sv1dCapacityRunStatus{}, fmt.Errorf("SV1D capacity arm %s: run status %s digest is not retained", arm.Name, artifact.name)
		}
	}
	return status, nil
}

func verifySV1DCapacityArmEvidenceManifest(attestation SV1DCapacityAttestation, arm SV1DCapacityArm, armDir string, snapshot sv1dCapacityArmSnapshot) (cdfEvidenceManifest, error) {
	manifestRaw, ok := snapshot.armFiles["evidence-manifest.json"]
	if !ok {
		return cdfEvidenceManifest{}, fmt.Errorf("SV1D capacity arm %s: evidence manifest is not retained", arm.Name)
	}
	var evidenceManifest cdfEvidenceManifest
	if err := decodeSV1DJSONWithRequiredFields(manifestRaw, &evidenceManifest, "schema_version", "contract", "cell", "log_mode", "evidence_format", "source_revision", "fixed_files", "raw_jsonl_files", "raw_jsonl_bytes", "raw_files"); err != nil {
		return cdfEvidenceManifest{}, fmt.Errorf("SV1D capacity arm %s: decode evidence manifest: %w", arm.Name, err)
	}
	if evidenceManifest.SchemaVersion != 2 || evidenceManifest.Contract != "v2-integrated-longrun-evidence-manifest-v2" || evidenceManifest.Cell != arm.Name || evidenceManifest.LogMode != "full" || evidenceManifest.EvidenceFormat != "evstream_v3" || evidenceManifest.SourceRevision != attestation.SourceRevision || evidenceManifest.RawJSONLFiles != 0 || evidenceManifest.RawJSONLBytes != 0 || len(evidenceManifest.RawFiles) != 0 {
		return cdfEvidenceManifest{}, fmt.Errorf("SV1D capacity arm %s: evidence manifest identity is inconsistent", arm.Name)
	}
	requiredEvidenceFiles := []string{"run-config.json", "run-metadata.json", "manifest.json", "greeks.json", "latency.json", "checkpoints.jsonl", "events.evs", "binary-evidence-attestation.json", "market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"}
	if err := verifySV1DCapacityEvidenceFiles(snapshot.armFiles, evidenceManifest.FixedFiles, requiredEvidenceFiles); err != nil {
		return cdfEvidenceManifest{}, fmt.Errorf("SV1D capacity arm %s: %w", arm.Name, err)
	}
	return evidenceManifest, nil
}

func verifySV1DCapacityArmBinaryEvidence(attestation SV1DCapacityAttestation, arm SV1DCapacityArm, armDir string, snapshot sv1dCapacityArmSnapshot) error {
	binaryRaw, ok := snapshot.armFiles["binary-evidence-attestation.json"]
	if !ok {
		return fmt.Errorf("SV1D capacity arm %s: binary attestation is not retained", arm.Name)
	}
	var binaryAttestation cdfBinaryEvidenceAttestation
	if err := decodeSV1DJSONWithRequiredFields(binaryRaw, &binaryAttestation, "domain", "ordering", "schema_epoch", "event_frames", "stream_frames", "execution_stream_hash", "evidence_only_in_stream"); err != nil {
		return fmt.Errorf("SV1D capacity arm %s: decode binary attestation: %w", arm.Name, err)
	}
	if binaryAttestation.Domain != "canonical_binary_execution_frames" || binaryAttestation.Ordering != "ordered_stream" || binaryAttestation.SchemaEpoch != SV1DCapacityEvidenceSchemaEpoch || binaryAttestation.EventFrames != arm.EventFrames || binaryAttestation.StreamFrames != arm.StreamFrames || binaryAttestation.ExecutionStreamHash != arm.ExecutionStreamHash || !binaryAttestation.EvidenceOnlyIncluded || binaryAttestation.UnencodablePayloads != 0 {
		return fmt.Errorf("SV1D capacity arm %s: binary attestation identity is inconsistent", arm.Name)
	}
	return nil
}

func verifySV1DCapacityArmRenderer(attestation SV1DCapacityAttestation, arm SV1DCapacityArm, armDir, renderedDir string, snapshot sv1dCapacityArmSnapshot) error {
	reportRaw, ok := snapshot.armFiles["renderer-report.json"]
	if !ok {
		return fmt.Errorf("SV1D capacity arm %s: renderer report is not retained", arm.Name)
	}
	var report sv1dCapacityRendererReport
	if err := decodeSV1DJSONWithRequiredFields(reportRaw, &report, jsonFieldNames(reflect.TypeOf(report), true)...); err != nil {
		return fmt.Errorf("SV1D capacity arm %s: decode renderer report: %w", arm.Name, err)
	}
	if report.EventFrames != arm.EventFrames || report.DictionaryFrames+report.EventFrames != report.StreamFrames || report.StreamFrames != arm.StreamFrames || report.ExecutionStreamHash != arm.ExecutionStreamHash || report.Routes == 0 || report.RenderedDigest != arm.RenderedTreeDigest || !isSV1DHexDigest(report.RenderedDigest) {
		return fmt.Errorf("SV1D capacity arm %s: renderer report identity is inconsistent", arm.Name)
	}
	renderedAttestationRaw, ok := snapshot.renderedFiles["rendered-binary-evidence-attestation.json"]
	if !ok {
		return fmt.Errorf("SV1D capacity arm %s: rendered binary attestation is not retained", arm.Name)
	}
	var renderedAttestation cdfRenderedEvidenceAttestation
	if err := decodeSV1DJSONWithRequiredFields(renderedAttestationRaw, &renderedAttestation, "domain", "ordering", "source_execution_stream_hash", "source_event_frames", "source_stream_frames", "rendered_digest", "global_sequence_included"); err != nil {
		return fmt.Errorf("SV1D capacity arm %s: decode rendered binary attestation: %w", arm.Name, err)
	}
	if renderedAttestation.Domain != "rendered_binary_evidence" || renderedAttestation.Ordering != "venue_sequence_files_with_global_frame_identity" || renderedAttestation.SourceExecutionHash != arm.ExecutionStreamHash || renderedAttestation.SourceEventFrames != arm.EventFrames || renderedAttestation.SourceStreamFrames != arm.StreamFrames || renderedAttestation.RenderedDigest != arm.RenderedTreeDigest || !renderedAttestation.GlobalSequenceIncluded {
		return fmt.Errorf("SV1D capacity arm %s: rendered binary attestation identity is inconsistent", arm.Name)
	}
	rendererAttestationRaw, ok := snapshot.renderedFiles["renderer-attestation.json"]
	if !ok {
		return fmt.Errorf("SV1D capacity arm %s: renderer attestation is not retained", arm.Name)
	}
	if err := validateSV1DRendererAttestationRaw(rendererAttestationRaw, renderedAttestationRaw, CDFExpectedProvenance{
		RendererSHA256: attestation.RendererSHA256, RendererSourceRevision: attestation.SourceRevision,
		RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1", RendererTrimpath: true, RendererCGOEnabled: "0",
	}); err != nil {
		return fmt.Errorf("SV1D capacity arm %s: renderer provenance: %w", arm.Name, err)
	}
	if err := validateCDFRenderedGlobalSequenceSnapshot(cdfRenderedEvidenceSnapshot{
		BinaryAttestationRaw:   snapshot.armFiles["binary-evidence-attestation.json"],
		EventsRaw:              snapshot.armFiles["events.evs"],
		RenderedAttestationRaw: renderedAttestationRaw,
		RenderedFiles:          snapshot.renderedFiles,
	}, SV1DCapacityEvidenceSchemaEpoch); err != nil {
		return fmt.Errorf("SV1D capacity arm %s: rendered evidence identity: %w", arm.Name, err)
	}
	return nil
}

func sv1dCapacityConfigDigest(attestation SV1DCapacityAttestation, armIndex int) string {
	switch armIndex {
	case 0:
		return attestation.CapacityTreatmentConfigSHA256
	case 1:
		return attestation.CapacityModeOffConfigSHA256
	case 2:
		return attestation.CapacityNoRosterConfigSHA256
	default:
		return ""
	}
}

func verifySV1DCapacityEvidenceFiles(files map[string][]byte, records []cdfEvidenceManifestRecord, required []string) error {
	byPath := make(map[string]cdfEvidenceManifestRecord, len(records))
	for _, record := range records {
		if record.Path == "" || filepath.IsAbs(record.Path) || filepath.Clean(record.Path) != record.Path || strings.HasPrefix(record.Path, "../") || record.Bytes <= 0 || !isSV1DHexDigest(record.SHA256) {
			return errors.New("evidence manifest contains an invalid fixed-file record")
		}
		if _, duplicate := byPath[record.Path]; duplicate {
			return fmt.Errorf("evidence manifest repeats %s", record.Path)
		}
		byPath[record.Path] = record
		raw, ok := files[record.Path]
		if !ok {
			return fmt.Errorf("evidence manifest fixed file %s is unavailable", record.Path)
		}
		if uint64(len(raw)) != uint64(record.Bytes) || sha256DigestHex(raw) != record.SHA256 {
			return fmt.Errorf("evidence manifest fixed file %s is not retained", record.Path)
		}
	}
	if len(byPath) != len(required) {
		return fmt.Errorf("evidence manifest has %d fixed files, want %d", len(byPath), len(required))
	}
	for _, path := range required {
		if _, ok := byPath[path]; !ok {
			return fmt.Errorf("evidence manifest omits %s", path)
		}
	}
	return nil
}

func decodeSV1DJSONWithRequiredFields(raw []byte, target any, requiredFields ...string) error {
	if err := rejectSV1DDuplicateJSONKeys(raw); err != nil {
		return err
	}
	if err := requireSV1DJSONFields(raw, requiredFields...); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return errors.New("multiple top-level JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func sha256DigestHex(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validateSV1DCapacityArms(attestation SV1DCapacityAttestation) error {
	if len(attestation.Arms) != 3 {
		return fmt.Errorf("SV1D capacity attestation must contain exactly three arms")
	}
	expectedNames := []string{"treatment", "mode-off", "no-roster"}
	seenConfigHashes := make(map[string]struct{}, len(attestation.Arms))
	for index, arm := range attestation.Arms {
		if arm.Name != expectedNames[index] || arm.CapacityExperimentID == "" || strings.Contains(strings.ToLower(arm.CapacityExperimentID), "holdout") || arm.CapacityHypothesisID == "" || !isSV1DHexDigest(arm.CapacityConfigSHA256) || !arm.Complete || arm.ExitStatus != 0 || arm.SimulationStartNano != SV1DCapacityStartNano || arm.SimulationEndNano != SV1DCapacityEndNano || arm.EventFrames == 0 || arm.StreamFrames < arm.EventFrames || !isSV1DHexDigest(arm.RunMetadataSHA256) || !isSV1DHexDigest(arm.ManifestSHA256) || !isSV1DHexDigest(arm.RunStatusSHA256) || !isSV1DHexDigest(arm.EvidenceManifestSHA256) || !isSV1DHexDigest(arm.BinaryEvidenceAttestationSHA256) || !isSV1DHexDigest(arm.EventsSHA256) || !isSV1DHexDigest(arm.ExecutionStreamHash) || !isSV1DHexDigest(arm.RendererReportSHA256) || !isSV1DHexDigest(arm.RendererAttestationSHA256) || !isSV1DHexDigest(arm.RenderedTreeDigest) || !isSV1DHexDigest(arm.ResourceMeasurementSHA256) || !isSV1DHexDigest(arm.CapacityArmRecordSHA256) || arm.PeakApparentBytes == 0 || arm.PeakAllocatedBytes == 0 || arm.PeakProcessTreeRSSBytes == 0 {
			return fmt.Errorf("SV1D capacity arm %s is incomplete or malformed", expectedNames[index])
		}
		if arm.CapacityConfigSHA256 != sv1dCapacityConfigDigest(attestation, index) {
			return fmt.Errorf("SV1D capacity arm %s does not match its top-level config identity", arm.Name)
		}
		if _, duplicate := seenConfigHashes[arm.CapacityConfigSHA256]; duplicate {
			return fmt.Errorf("SV1D capacity arms reuse a config identity")
		}
		seenConfigHashes[arm.CapacityConfigSHA256] = struct{}{}
	}
	return nil
}

func verifySV1DCapacityRetainedInputs(attestation SV1DCapacityAttestation) error {
	root := attestation.MeasurementRoot
	retainedFiles := []struct {
		path   string
		digest string
	}{
		{filepath.Join(root, "tools", "multivenue-"+attestation.BinarySHA256), attestation.BinarySHA256},
		{filepath.Join(root, "tools", "sv1dprobe-"+attestation.AnalyzerSHA256), attestation.AnalyzerSHA256},
		{filepath.Join(root, "tools", "evsrender-"+attestation.RendererSHA256), attestation.RendererSHA256},
		{filepath.Join(root, "tools", "sv1dresource-"+attestation.MeasurerSHA256), attestation.MeasurerSHA256},
		{filepath.Join(root, "tools", "capacity-runner-"+attestation.RunnerSHA256+".sh"), attestation.RunnerSHA256},
		{filepath.Join(root, "resource-policy-v1.json"), attestation.ResourcePolicySHA256},
		{filepath.Join(root, "review", "attestation.json"), attestation.ReviewAttestationSHA256},
		{filepath.Join(root, "review", "report.md"), attestation.ReviewReportSHA256},
		{filepath.Join(root, "review", "trusted-key.raw"), attestation.TrustedReviewKeySHA256},
		{filepath.Join(root, "configs", "target-treatment.json"), attestation.TargetTreatmentConfigSHA256},
		{filepath.Join(root, "configs", "target-mode-off.json"), attestation.TargetModeOffConfigSHA256},
		{filepath.Join(root, "configs", "target-no-roster.json"), attestation.TargetNoRosterConfigSHA256},
		{filepath.Join(root, "configs", "capacity-config-delta.json"), attestation.CapacityConfigDeltaSHA256},
	}
	for _, retained := range retainedFiles {
		if !isSV1DHexDigest(retained.digest) {
			return fmt.Errorf("retained input %s has an invalid attested digest", retained.path)
		}
		raw, err := readSV1DRegularFile(retained.path)
		if err != nil {
			return fmt.Errorf("read retained input %s: %w", retained.path, err)
		}
		if sha256DigestHex(raw) != retained.digest {
			return fmt.Errorf("retained input %s is not bound to its attested digest", retained.path)
		}
	}
	return nil
}

func maxCapacityValue(values ...uint64) uint64 {
	var maximum uint64
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func capacityRequiredFree(measuredPeak, safetyReserve uint64) (uint64, bool) {
	if measuredPeak > math.MaxUint64/2 || measuredPeak > math.MaxUint64-safetyReserve {
		return 0, false
	}
	return maxCapacityValue(measuredPeak*2, measuredPeak+safetyReserve), true
}

func capacityRequiredMemory(peakRSS uint64) (uint64, bool) {
	if peakRSS > (math.MaxUint64-1)/3 || peakRSS > math.MaxUint64-1024*1024*1024 {
		return 0, false
	}
	ceilOnePointFive := (3*peakRSS + 1) / 2
	return maxCapacityValue(ceilOnePointFive, peakRSS+1024*1024*1024), true
}

func absoluteCleanPath(path string) bool {
	return filepath.IsAbs(path) && path != "/" && !strings.Contains(path, "\x00") && filepath.Clean(path) == path
}
