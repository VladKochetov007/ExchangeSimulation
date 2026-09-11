package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
)

const (
	SV1DCapacityPreflightContract  = "v2-r2-sv1d-binary-capacity-preflight-v1"
	SV1DCapacityPreflightPurpose   = "five_minute_sv1d_binary_evidence_capacity_preflight"
	SV1DCapacitySeed               = int64(977)
	SV1DCapacityDurationNano       = uint64(300_000_000_000)
	SV1DCapacityStartNano          = uint64(1_735_689_600_000_000_000)
	SV1DCapacityEndNano            = SV1DCapacityStartNano + SV1DCapacityDurationNano
	SV1DCapacitySampleIntervalNano = uint64(250_000_000)
	SV1DCapacitySafetyReserveBytes = uint64(2 * 1024 * 1024 * 1024)
	SV1DCapacityMinimumGOMAXPROCS  = 1
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
	FilesystemDevice              string
	FilesystemID                  string
	FilesystemType                string
	FilesystemMountID             string
	FilesystemUUID                string
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
	} {
		if !isSV1DHexDigest(value) {
			return fmt.Errorf("SV1D capacity attestation has an invalid %s digest", name)
		}
	}
	if attestation.EvidenceFormat != "evstream_v3" || attestation.EvidenceSchemaEpoch == 0 || attestation.LogMode != "full" || attestation.GOMAXPROCS < SV1DCapacityMinimumGOMAXPROCS || attestation.GOMEMLIMIT == "" || !attestation.SameFilesystem || !absoluteCleanPath(attestation.OutputParent) || !absoluteCleanPath(attestation.MeasurementRoot) || attestation.FilesystemDevice == "" || attestation.FilesystemID == "" || attestation.FilesystemType == "" || attestation.FilesystemMountID == "" || attestation.FilesystemUUID == "" {
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
	return attestation, nil
}

func compareSV1DCapacityExpectation(attestation SV1DCapacityAttestation, expected SV1DCapacityExpectation) error {
	if expected.ProbeID == "" || attestation.ProbeID != expected.ProbeID || attestation.SourceRevision != expected.SourceRevision || attestation.TreeRevision != expected.TreeRevision || attestation.PlanSHA256 != expected.PlanSHA256 || attestation.ReviewAttestationSHA256 != expected.ReviewAttestationSHA256 || attestation.ReviewReportSHA256 != expected.ReviewReportSHA256 {
		return fmt.Errorf("SV1D capacity attestation does not match source, tree, plan, or review identity")
	}
	if attestation.TargetTreatmentConfigSHA256 != expected.TargetTreatmentConfigSHA256 || attestation.TargetModeOffConfigSHA256 != expected.TargetModeOffConfigSHA256 || attestation.TargetNoRosterConfigSHA256 != expected.TargetNoRosterConfigSHA256 || attestation.CapacityTreatmentConfigSHA256 != expected.CapacityTreatmentConfigSHA256 || attestation.CapacityModeOffConfigSHA256 != expected.CapacityModeOffConfigSHA256 || attestation.CapacityNoRosterConfigSHA256 != expected.CapacityNoRosterConfigSHA256 || attestation.CapacityConfigDeltaSHA256 != expected.CapacityConfigDeltaSHA256 {
		return fmt.Errorf("SV1D capacity attestation does not match capacity-only config identities")
	}
	if attestation.BinarySHA256 != expected.BinarySHA256 || attestation.AnalyzerSHA256 != expected.AnalyzerSHA256 || attestation.RendererSHA256 != expected.RendererSHA256 || attestation.RunnerSHA256 != expected.RunnerSHA256 || attestation.MeasurerSHA256 != expected.MeasurerSHA256 || attestation.ResourcePolicySHA256 != expected.ResourcePolicySHA256 {
		return fmt.Errorf("SV1D capacity attestation does not match tool or policy identity")
	}
	if attestation.OutputParent != expected.OutputParent || attestation.MeasurementRoot != expected.MeasurementRoot || attestation.FilesystemDevice != expected.FilesystemDevice || attestation.FilesystemID != expected.FilesystemID || attestation.FilesystemType != expected.FilesystemType || attestation.FilesystemMountID != expected.FilesystemMountID || attestation.FilesystemUUID != expected.FilesystemUUID {
		return fmt.Errorf("SV1D capacity attestation does not match the measured filesystem")
	}
	return nil
}

func validateSV1DCapacityArms(attestation SV1DCapacityAttestation) error {
	if len(attestation.Arms) != 3 {
		return fmt.Errorf("SV1D capacity attestation must contain exactly three arms")
	}
	expectedNames := []string{"treatment", "mode-off", "no-roster"}
	seenConfigHashes := make(map[string]struct{}, len(attestation.Arms))
	for index, arm := range attestation.Arms {
		if arm.Name != expectedNames[index] || arm.CapacityExperimentID == "" || strings.Contains(strings.ToLower(arm.CapacityExperimentID), "holdout") || arm.CapacityHypothesisID == "" || !isSV1DHexDigest(arm.CapacityConfigSHA256) || !arm.Complete || arm.ExitStatus != 0 || arm.SimulationStartNano != SV1DCapacityStartNano || arm.SimulationEndNano != SV1DCapacityEndNano || arm.EventFrames == 0 || arm.StreamFrames < arm.EventFrames || !isSV1DHexDigest(arm.RunMetadataSHA256) || !isSV1DHexDigest(arm.ManifestSHA256) || !isSV1DHexDigest(arm.RunStatusSHA256) || !isSV1DHexDigest(arm.EvidenceManifestSHA256) || !isSV1DHexDigest(arm.BinaryEvidenceAttestationSHA256) || !isSV1DHexDigest(arm.EventsSHA256) || !isSV1DHexDigest(arm.ExecutionStreamHash) || !isSV1DHexDigest(arm.RendererReportSHA256) || !isSV1DHexDigest(arm.RendererAttestationSHA256) || !isSV1DHexDigest(arm.RenderedTreeDigest) || arm.PeakApparentBytes == 0 || arm.PeakAllocatedBytes == 0 || arm.PeakProcessTreeRSSBytes == 0 {
			return fmt.Errorf("SV1D capacity arm %s is incomplete or malformed", expectedNames[index])
		}
		if _, duplicate := seenConfigHashes[arm.CapacityConfigSHA256]; duplicate {
			return fmt.Errorf("SV1D capacity arms reuse a config identity")
		}
		seenConfigHashes[arm.CapacityConfigSHA256] = struct{}{}
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
