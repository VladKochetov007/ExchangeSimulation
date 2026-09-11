package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSV1DCapacityAttestationAcceptsMeasuredCompleteRecord(t *testing.T) {
	attestation := testSV1DCapacityAttestation()
	if err := ValidateSV1DCapacityAttestation(attestation); err != nil {
		t.Fatal(err)
	}
	if attestation.RequiredFreeBytes != 2*1024*1024*1024+5_000 || attestation.RequiredAvailableMemoryBytes != 1024*1024*1024+1_000 {
		t.Fatalf("capacity formulas = free %d memory %d", attestation.RequiredFreeBytes, attestation.RequiredAvailableMemoryBytes)
	}
}

func TestValidateSV1DCapacityAttestationRejectsFormulaAndCompletionMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SV1DCapacityAttestation)
	}{
		{name: "scientific eligibility", mutate: func(attestation *SV1DCapacityAttestation) { attestation.ScientificResultEligible = true }},
		{name: "filesystem consumption", mutate: func(attestation *SV1DCapacityAttestation) { attestation.PeakFilesystemConsumptionBytes++ }},
		{name: "measured peak", mutate: func(attestation *SV1DCapacityAttestation) { attestation.MeasuredPeakBytes++ }},
		{name: "free headroom", mutate: func(attestation *SV1DCapacityAttestation) { attestation.RequiredFreeBytes++ }},
		{name: "memory headroom", mutate: func(attestation *SV1DCapacityAttestation) { attestation.RequiredAvailableMemoryBytes++ }},
		{name: "sample gap", mutate: func(attestation *SV1DCapacityAttestation) {
			attestation.MaximumSampleGapNano = attestation.SampleIntervalNano + 1
		}},
		{name: "cgroup OOM", mutate: func(attestation *SV1DCapacityAttestation) { attestation.CgroupOOMEventsDelta = 1 }},
		{name: "arm swap", mutate: func(attestation *SV1DCapacityAttestation) {
			attestation.Arms[0].Name, attestation.Arms[1].Name = attestation.Arms[1].Name, attestation.Arms[0].Name
		}},
		{name: "arm incomplete", mutate: func(attestation *SV1DCapacityAttestation) { attestation.Arms[2].Complete = false }},
		{name: "holdout capacity id", mutate: func(attestation *SV1DCapacityAttestation) { attestation.Arms[0].CapacityExperimentID = "holdout-977" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := testSV1DCapacityAttestation()
			test.mutate(&candidate)
			if err := ValidateSV1DCapacityAttestation(candidate); err == nil {
				t.Fatal("capacity mutation was accepted")
			}
		})
	}
}

func TestVerifySV1DCapacityAttestationBindsExternalExpectationAndRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	attestation := testSV1DCapacityAttestation()
	path := filepath.Join(dir, "capacity.json")
	writeSV1DCapacityAttestation(t, path, attestation)
	if _, err := VerifySV1DCapacityAttestation(path, testSV1DCapacityExpectation(attestation)); err != nil {
		t.Fatal(err)
	}
	wrong := testSV1DCapacityExpectation(attestation)
	wrong.BinarySHA256 = strings.Repeat("f", 64)
	if _, err := VerifySV1DCapacityAttestation(path, wrong); err == nil {
		t.Fatal("capacity record with wrong binary identity was accepted")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unknownRaw := bytes.Replace(raw, []byte("}"), []byte(`,"unexpected":true}`), 1)
	if err := os.WriteFile(path, unknownRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySV1DCapacityAttestation(path, testSV1DCapacityExpectation(attestation)); err == nil {
		t.Fatal("unknown capacity-attestation field was accepted")
	}
}

func testSV1DCapacityAttestation() SV1DCapacityAttestation {
	digest := func(value byte) string { return fmt.Sprintf("%064x", value) }
	arm := func(name string, index byte) SV1DCapacityArm {
		return SV1DCapacityArm{
			Name: name, CapacityExperimentID: "v2-r2-sv1d-capacity-977-" + name,
			CapacityHypothesisID: "V2-R2-SV1D-CAPACITY-ONLY", CapacityConfigSHA256: digest(index), Complete: true,
			ExitStatus: 0, SimulationStartNano: SV1DCapacityStartNano, SimulationEndNano: SV1DCapacityEndNano,
			RunMetadataSHA256: digest(index + 10), ManifestSHA256: digest(index + 20), RunStatusSHA256: digest(index + 30),
			EvidenceManifestSHA256: digest(index + 40), BinaryEvidenceAttestationSHA256: digest(index + 50), EventsSHA256: digest(index + 60),
			ExecutionStreamHash: digest(index + 70), RendererReportSHA256: digest(index + 80), RendererAttestationSHA256: digest(index + 90),
			RenderedTreeDigest: digest(index + 100), EventFrames: 10, StreamFrames: 11,
			PeakApparentBytes: 4_000, PeakAllocatedBytes: 5_000, PeakProcessTreeRSSBytes: 900,
		}
	}
	return SV1DCapacityAttestation{
		SchemaVersion: 1, Contract: SV1DCapacityPreflightContract, ScientificResultEligible: false,
		Purpose: SV1DCapacityPreflightPurpose, ProbeID: "v2-r2-sv1d-activation-659", CapacitySeed: SV1DCapacitySeed,
		Horizon: "5m", DurationNano: SV1DCapacityDurationNano, SimulationStartNano: SV1DCapacityStartNano, SimulationEndNano: SV1DCapacityEndNano,
		SourceRevision: strings.Repeat("a", 40), TreeRevision: strings.Repeat("b", 40), ReviewAttestationSHA256: digest('c'), ReviewReportSHA256: digest('d'), PlanSHA256: digest('e'),
		TargetTreatmentConfigSHA256: digest('f'), TargetModeOffConfigSHA256: digest('g'), TargetNoRosterConfigSHA256: digest('h'),
		CapacityTreatmentConfigSHA256: digest('i'), CapacityModeOffConfigSHA256: digest('j'), CapacityNoRosterConfigSHA256: digest('k'), CapacityConfigDeltaSHA256: digest('l'),
		BinarySHA256: digest('m'), AnalyzerSHA256: digest('n'), RendererSHA256: digest('o'), RunnerSHA256: digest('p'), MeasurerSHA256: digest('q'), ResourcePolicySHA256: digest('r'),
		EvidenceFormat: "evstream_v3", EvidenceSchemaEpoch: 4, LogMode: "full", GOMAXPROCS: 2, GOMEMLIMIT: "4GiB",
		OutputParent: "/var/lib/sv1d", MeasurementRoot: "/var/lib/sv1d/capacity-977", FilesystemDevice: "/dev/vda1", FilesystemID: "fs-1", FilesystemType: "ext4", FilesystemMountID: "mount-1", FilesystemUUID: "uuid-1", SameFilesystem: true,
		InitialAvailableBytes: 10_000, MinimumAvailableBytes: 7_000, FinalAvailableBytes: 8_000, PeakApparentBytes: 4_000, PeakAllocatedBytes: 5_000, PeakFilesystemConsumptionBytes: 3_000, MeasuredPeakBytes: 5_000,
		SafetyReserveBytes: SV1DCapacitySafetyReserveBytes, RequiredFreeBytes: 5_000 + SV1DCapacitySafetyReserveBytes, PeakProcessTreeRSSBytes: 1_000, RequiredAvailableMemoryBytes: 1_000 + 1024*1024*1024, PeakCgroupMemoryBytes: 2_000, CgroupMemoryLimitBytes: 3_000, MinimumHostMemAvailableBytes: 2_000,
		SwapUsedBytes: 0, SampleIntervalNano: SV1DCapacitySampleIntervalNano, MaximumSampleGapNano: SV1DCapacitySampleIntervalNano, SampleCount: 3, SamplesSHA256: digest('s'), OOMEventsDelta: 0, OOMKillEventsDelta: 0, CgroupOOMEventsDelta: 0, CgroupOOMKillEventsDelta: 0,
		Arms: []SV1DCapacityArm{arm("treatment", 't'), arm("mode-off", 'u'), arm("no-roster", 'v')},
	}
}

func testSV1DCapacityExpectation(attestation SV1DCapacityAttestation) SV1DCapacityExpectation {
	return SV1DCapacityExpectation{
		SourceRevision: attestation.SourceRevision, TreeRevision: attestation.TreeRevision, ProbeID: attestation.ProbeID, PlanSHA256: attestation.PlanSHA256,
		ReviewAttestationSHA256: attestation.ReviewAttestationSHA256, ReviewReportSHA256: attestation.ReviewReportSHA256,
		TargetTreatmentConfigSHA256: attestation.TargetTreatmentConfigSHA256, TargetModeOffConfigSHA256: attestation.TargetModeOffConfigSHA256, TargetNoRosterConfigSHA256: attestation.TargetNoRosterConfigSHA256,
		CapacityTreatmentConfigSHA256: attestation.CapacityTreatmentConfigSHA256, CapacityModeOffConfigSHA256: attestation.CapacityModeOffConfigSHA256, CapacityNoRosterConfigSHA256: attestation.CapacityNoRosterConfigSHA256, CapacityConfigDeltaSHA256: attestation.CapacityConfigDeltaSHA256,
		BinarySHA256: attestation.BinarySHA256, AnalyzerSHA256: attestation.AnalyzerSHA256, RendererSHA256: attestation.RendererSHA256, RunnerSHA256: attestation.RunnerSHA256, MeasurerSHA256: attestation.MeasurerSHA256, ResourcePolicySHA256: attestation.ResourcePolicySHA256,
		OutputParent: attestation.OutputParent, MeasurementRoot: attestation.MeasurementRoot, FilesystemDevice: attestation.FilesystemDevice, FilesystemID: attestation.FilesystemID, FilesystemType: attestation.FilesystemType, FilesystemMountID: attestation.FilesystemMountID, FilesystemUUID: attestation.FilesystemUUID,
	}
}

func writeSV1DCapacityAttestation(t *testing.T, path string, attestation SV1DCapacityAttestation) {
	t.Helper()
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
