package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{name: "resource policy CPU", mutate: func(attestation *SV1DCapacityAttestation) { attestation.GOMAXPROCS = SV1DCapacityGOMAXPROCS + 1 }},
		{name: "resource policy memory", mutate: func(attestation *SV1DCapacityAttestation) { attestation.GOMEMLIMIT = "8GiB" }},
		{name: "evidence schema epoch", mutate: func(attestation *SV1DCapacityAttestation) { attestation.EvidenceSchemaEpoch++ }},
		{name: "cgroup OOM", mutate: func(attestation *SV1DCapacityAttestation) { attestation.CgroupOOMEventsDelta = 1 }},
		{name: "arm config binding", mutate: func(attestation *SV1DCapacityAttestation) {
			attestation.Arms[0].CapacityConfigSHA256 = attestation.CapacityModeOffConfigSHA256
		}},
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
	attestation.OutputParent = dir
	attestation.MeasurementRoot = filepath.Join(dir, "capacity-output")
	attestation.MeasurementRecordsRoot = filepath.Join(dir, "capacity-measurements")
	if err := os.MkdirAll(attestation.MeasurementRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attestation.MeasurementRecordsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	filesystem, err := InspectSV1DFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	attestation.FilesystemDevice, attestation.FilesystemID, attestation.FilesystemType = filesystem.Device, filesystem.ID, filesystem.Type
	attestation.FilesystemMountID, attestation.FilesystemUUID = filesystem.MountID, filesystem.UUID
	writeSV1DCapacityMeasurementRecords(t, &attestation)
	path := filepath.Join(dir, "capacity.json")
	writeSV1DCapacityAttestation(t, path, attestation)
	if _, err := VerifySV1DCapacityAttestation(path, testSV1DCapacityExpectation(attestation)); err != nil {
		t.Fatal(err)
	}
	tampered := attestation
	tampered.Arms = append([]SV1DCapacityArm(nil), attestation.Arms...)
	tampered.Arms[0].ResourceMeasurementSHA256 = strings.Repeat("0", 64)
	writeSV1DCapacityAttestation(t, path, tampered)
	if _, err := VerifySV1DCapacityAttestation(path, testSV1DCapacityExpectation(attestation)); err == nil {
		t.Fatal("capacity record with an unbound resource measurement was accepted")
	}
	writeSV1DCapacityAttestation(t, path, attestation)
	wrong := testSV1DCapacityExpectation(attestation)
	wrong.BinarySHA256 = strings.Repeat("f", 64)
	if _, err := VerifySV1DCapacityAttestation(path, wrong); err == nil {
		t.Fatal("capacity record with wrong binary identity was accepted")
	}
	wrong = testSV1DCapacityExpectation(attestation)
	wrong.MeasurementRecordsSHA256 = strings.Repeat("f", 64)
	if _, err := VerifySV1DCapacityAttestation(path, wrong); err == nil {
		t.Fatal("capacity record with wrong measurement-record identity was accepted")
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

func TestVerifySV1DCapacityAttestationRejectsOmittedSafetyAndArmFields(t *testing.T) {
	dir := t.TempDir()
	attestation := testSV1DCapacityAttestation()
	attestation.OutputParent = dir
	attestation.MeasurementRoot = filepath.Join(dir, "capacity-output")
	attestation.MeasurementRecordsRoot = filepath.Join(dir, "capacity-measurements")
	if err := os.MkdirAll(attestation.MeasurementRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attestation.MeasurementRecordsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	filesystem, err := InspectSV1DFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	attestation.FilesystemDevice, attestation.FilesystemID, attestation.FilesystemType = filesystem.Device, filesystem.ID, filesystem.Type
	attestation.FilesystemMountID, attestation.FilesystemUUID = filesystem.MountID, filesystem.UUID
	writeSV1DCapacityMeasurementRecords(t, &attestation)
	path := filepath.Join(dir, "capacity.json")
	expectation := testSV1DCapacityExpectation(attestation)

	removeField := func(removeArmField bool, field string) {
		raw, err := json.Marshal(attestation)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		if removeArmField {
			var arms []map[string]json.RawMessage
			if err := json.Unmarshal(document["arms"], &arms); err != nil {
				t.Fatal(err)
			}
			delete(arms[0], field)
			document["arms"], err = json.Marshal(arms)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			delete(document, field)
		}
		mutated, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, mutated, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifySV1DCapacityAttestation(path, expectation); err == nil {
			t.Fatalf("attestation without %s was accepted", field)
		}
	}
	removeField(false, "scientific_result_eligible")
	removeField(false, "cgroup_oom_events_delta")
	removeField(true, "exit_status")
}

func TestSV1DCapacityPresenceChecksRejectMissingAndNullNestedFields(t *testing.T) {
	sample := SV1DResourceSample{}
	measurementRaw, err := json.Marshal(struct {
		Samples []SV1DResourceSample `json:"samples"`
	}{Samples: []SV1DResourceSample{sample}})
	if err != nil {
		t.Fatal(err)
	}
	var measurementDocument map[string]json.RawMessage
	if err := json.Unmarshal(measurementRaw, &measurementDocument); err != nil {
		t.Fatal(err)
	}
	var sampleDocuments []map[string]json.RawMessage
	if err := json.Unmarshal(measurementDocument["samples"], &sampleDocuments); err != nil {
		t.Fatal(err)
	}
	delete(sampleDocuments[0], "available_bytes")
	measurementDocument["samples"], err = json.Marshal(sampleDocuments)
	if err != nil {
		t.Fatal(err)
	}
	measurementRaw, err = json.Marshal(measurementDocument)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSV1DResourceMeasurementJSONPresence(measurementRaw); err == nil {
		t.Fatal("resource measurement accepted a sample with an omitted safety field")
	}

	aggregateRaw, err := json.Marshal([]sv1dCapacitySampleAggregateEntry{{Arm: "treatment", Sample: sample}})
	if err != nil {
		t.Fatal(err)
	}
	var aggregateDocuments []map[string]json.RawMessage
	if err := json.Unmarshal(aggregateRaw, &aggregateDocuments); err != nil {
		t.Fatal(err)
	}
	var aggregateSample map[string]json.RawMessage
	if err := json.Unmarshal(aggregateDocuments[0]["sample"], &aggregateSample); err != nil {
		t.Fatal(err)
	}
	aggregateSample["host_mem_available_bytes"] = json.RawMessage("null")
	aggregateDocuments[0]["sample"], err = json.Marshal(aggregateSample)
	if err != nil {
		t.Fatal(err)
	}
	aggregateRaw, err = json.Marshal(aggregateDocuments)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSV1DCapacitySampleAggregateJSONPresence(aggregateRaw); err == nil {
		t.Fatal("sample aggregate accepted a null safety field")
	}
	if err := requireSV1DJSONFields([]byte(`{"identity":null}`), "identity"); err == nil {
		t.Fatal("required-field checker accepted null")
	}
}

func TestVerifySV1DCapacityAttestationRejectsRetainedToolMutation(t *testing.T) {
	dir := t.TempDir()
	attestation := testSV1DCapacityAttestation()
	attestation.OutputParent = dir
	attestation.MeasurementRoot = filepath.Join(dir, "capacity-output")
	attestation.MeasurementRecordsRoot = filepath.Join(dir, "capacity-measurements")
	if err := os.MkdirAll(attestation.MeasurementRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attestation.MeasurementRecordsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	filesystem, err := InspectSV1DFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	attestation.FilesystemDevice, attestation.FilesystemID, attestation.FilesystemType = filesystem.Device, filesystem.ID, filesystem.Type
	attestation.FilesystemMountID, attestation.FilesystemUUID = filesystem.MountID, filesystem.UUID
	writeSV1DCapacityMeasurementRecords(t, &attestation)
	path := filepath.Join(dir, "capacity.json")
	writeSV1DCapacityAttestation(t, path, attestation)
	if _, err := VerifySV1DCapacityAttestation(path, testSV1DCapacityExpectation(attestation)); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(attestation.MeasurementRoot, "tools", "multivenue-"+attestation.BinarySHA256)
	if err := os.WriteFile(binaryPath, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySV1DCapacityAttestation(path, testSV1DCapacityExpectation(attestation)); err == nil {
		t.Fatal("capacity verifier accepted a mutated retained simulator")
	}
}

func testSV1DCapacityAttestation() SV1DCapacityAttestation {
	digest := func(value byte) string { return fmt.Sprintf("%064x", value) }
	arm := func(name string, index byte, configDigest string) SV1DCapacityArm {
		return SV1DCapacityArm{
			Name: name, CapacityExperimentID: "v2-r2-sv1d-capacity-977-" + name,
			CapacityHypothesisID: "V2-R2-SV1D-CAPACITY-ONLY", CapacityConfigSHA256: configDigest, Complete: true,
			ExitStatus: 0, SimulationStartNano: SV1DCapacityStartNano, SimulationEndNano: SV1DCapacityEndNano,
			RunMetadataSHA256: digest(index + 10), ManifestSHA256: digest(index + 20), RunStatusSHA256: digest(index + 30),
			EvidenceManifestSHA256: digest(index + 40), BinaryEvidenceAttestationSHA256: digest(index + 50), EventsSHA256: digest(index + 60),
			ExecutionStreamHash: digest(index + 70), RendererReportSHA256: digest(index + 80), RendererAttestationSHA256: digest(index + 90),
			RenderedTreeDigest: digest(index + 100), EventFrames: 10, StreamFrames: 11,
			PeakApparentBytes: 4_000, PeakAllocatedBytes: 5_000, PeakProcessTreeRSSBytes: 900,
			ResourceMeasurementSHA256: digest(index + 110), CapacityArmRecordSHA256: digest(index + 120),
		}
	}
	return SV1DCapacityAttestation{
		SchemaVersion: 1, Contract: SV1DCapacityPreflightContract, ScientificResultEligible: false,
		Purpose: SV1DCapacityPreflightPurpose, ProbeID: "v2-r2-sv1d-activation-659", CapacitySeed: SV1DCapacitySeed,
		Horizon: "5m", DurationNano: SV1DCapacityDurationNano, SimulationStartNano: SV1DCapacityStartNano, SimulationEndNano: SV1DCapacityEndNano,
		SourceRevision: strings.Repeat("a", 40), TreeRevision: strings.Repeat("b", 40), ReviewAttestationSHA256: digest('c'), ReviewReportSHA256: digest('d'), TrustedReviewKeySHA256: digest('y'), PlanSHA256: digest('e'),
		TargetTreatmentConfigSHA256: digest('f'), TargetModeOffConfigSHA256: digest('g'), TargetNoRosterConfigSHA256: digest('h'),
		CapacityTreatmentConfigSHA256: digest('i'), CapacityModeOffConfigSHA256: digest('j'), CapacityNoRosterConfigSHA256: digest('k'), CapacityConfigDeltaSHA256: digest('l'),
		BinarySHA256: digest('m'), AnalyzerSHA256: digest('n'), RendererSHA256: digest('o'), RunnerSHA256: digest('p'), MeasurerSHA256: digest('q'), ResourcePolicySHA256: digest('r'),
		EvidenceFormat: "evstream_v3", EvidenceSchemaEpoch: 4, LogMode: "full", GOMAXPROCS: 2, GOMEMLIMIT: "4GiB",
		OutputParent: "/var/lib/sv1d", MeasurementRoot: "/var/lib/sv1d/capacity-977", MeasurementRecordsRoot: "/var/lib/sv1d/capacity-977.measurements", MeasurementRecordsSHA256: digest('z'), FilesystemDevice: "/dev/vda1", FilesystemID: "fs-1", FilesystemType: "ext4", FilesystemMountID: "mount-1", FilesystemUUID: "uuid-1", SameFilesystem: true,
		InitialAvailableBytes: 10_000, MinimumAvailableBytes: 7_000, FinalAvailableBytes: 8_000, PeakApparentBytes: 4_000, PeakAllocatedBytes: 5_000, PeakFilesystemConsumptionBytes: 3_000, MeasuredPeakBytes: 5_000,
		SafetyReserveBytes: SV1DCapacitySafetyReserveBytes, RequiredFreeBytes: 5_000 + SV1DCapacitySafetyReserveBytes, PeakProcessTreeRSSBytes: 1_000, RequiredAvailableMemoryBytes: 1_000 + 1024*1024*1024, PeakCgroupMemoryBytes: 2_000, CgroupMemoryLimitBytes: 3_000, MinimumHostMemAvailableBytes: 2_000,
		SwapUsedBytes: 0, SampleIntervalNano: SV1DCapacitySampleIntervalNano, MaximumSampleGapNano: SV1DCapacitySampleIntervalNano, SampleCount: 3, SamplesSHA256: digest('s'), OOMEventsDelta: 0, OOMKillEventsDelta: 0, CgroupOOMEventsDelta: 0, CgroupOOMKillEventsDelta: 0,
		Arms: []SV1DCapacityArm{arm("treatment", 't', digest('i')), arm("mode-off", 'u', digest('j')), arm("no-roster", 'v', digest('k'))},
	}
}

func testSV1DCapacityExpectation(attestation SV1DCapacityAttestation) SV1DCapacityExpectation {
	return SV1DCapacityExpectation{
		SourceRevision: attestation.SourceRevision, TreeRevision: attestation.TreeRevision, ProbeID: attestation.ProbeID, PlanSHA256: attestation.PlanSHA256,
		ReviewAttestationSHA256: attestation.ReviewAttestationSHA256, ReviewReportSHA256: attestation.ReviewReportSHA256, TrustedReviewKeySHA256: attestation.TrustedReviewKeySHA256,
		TargetTreatmentConfigSHA256: attestation.TargetTreatmentConfigSHA256, TargetModeOffConfigSHA256: attestation.TargetModeOffConfigSHA256, TargetNoRosterConfigSHA256: attestation.TargetNoRosterConfigSHA256,
		CapacityTreatmentConfigSHA256: attestation.CapacityTreatmentConfigSHA256, CapacityModeOffConfigSHA256: attestation.CapacityModeOffConfigSHA256, CapacityNoRosterConfigSHA256: attestation.CapacityNoRosterConfigSHA256, CapacityConfigDeltaSHA256: attestation.CapacityConfigDeltaSHA256,
		BinarySHA256: attestation.BinarySHA256, AnalyzerSHA256: attestation.AnalyzerSHA256, RendererSHA256: attestation.RendererSHA256, RunnerSHA256: attestation.RunnerSHA256, MeasurerSHA256: attestation.MeasurerSHA256, ResourcePolicySHA256: attestation.ResourcePolicySHA256,
		OutputParent: attestation.OutputParent, MeasurementRoot: attestation.MeasurementRoot, MeasurementRecordsRoot: attestation.MeasurementRecordsRoot, MeasurementRecordsSHA256: attestation.MeasurementRecordsSHA256, FilesystemDevice: attestation.FilesystemDevice, FilesystemID: attestation.FilesystemID, FilesystemType: attestation.FilesystemType, FilesystemMountID: attestation.FilesystemMountID, FilesystemUUID: attestation.FilesystemUUID,
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

func writeSV1DCapacityMeasurementRecords(t *testing.T, attestation *SV1DCapacityAttestation) {
	t.Helper()
	root := attestation.MeasurementRecordsRoot
	writeSV1DCapacityRetainedInputs(t, attestation)
	write := func(name string, value any) []byte {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	armNames := []string{"treatment", "mode-off", "no-roster"}
	measurements := make([]SV1DResourceMeasurement, len(armNames))
	for index, armName := range armNames {
		first, err := captureSV1DResourceSample(os.Getpid(), attestation.OutputParent, attestation.MeasurementRoot, "")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
		last, err := captureSV1DResourceSample(os.Getpid(), attestation.OutputParent, attestation.MeasurementRoot, "")
		if err != nil {
			t.Fatal(err)
		}
		for sampleIndex, sample := range []*SV1DResourceSample{&first, &last} {
			sample.ObservedAtUnixNano += int64(index*10 + sampleIndex)
			sample.ApparentBytes = uint64(4_000 + index)
			sample.AllocatedBytes = uint64(5_000 + index)
			if sample.ProcessTreeRSSBytes == 0 {
				sample.ProcessTreeRSSBytes = 900
			}
			if sample.CgroupCurrentBytes < sample.ProcessTreeRSSBytes {
				sample.CgroupCurrentBytes = sample.ProcessTreeRSSBytes + 1
			}
			sample.CgroupMemoryLimitBytes = 1 << 40
			sample.CgroupOOMEvents = 0
			sample.CgroupOOMKillEvents = 0
			sample.CgroupLocalOOMEvents = 0
			sample.CgroupLocalOOMKillEvents = 0
		}
		if first.AvailableBytes > 1 {
			last.AvailableBytes = first.AvailableBytes - 1
		}
		measurement := SV1DResourceMeasurement{
			SchemaVersion: 1, Contract: SV1DResourceMeasurementContract, Command: testSV1DCapacityCommand(*attestation, attestation.Arms[index]), OutputParent: attestation.OutputParent,
			MeasurementRoot: attestation.MeasurementRoot, Filesystem: attestationFilesystem(attestation), SampleIntervalNano: SV1DCapacitySampleIntervalNano, Complete: true,
			ExitStatus: 0, Samples: []SV1DResourceSample{first, last},
		}
		finalizeSV1DResourceMeasurement(&measurement)
		if err := ValidateSV1DResourceMeasurement(measurement, true); err != nil {
			t.Fatal(err)
		}
		measurements[index] = measurement
		resourceRaw := write(fmt.Sprintf("%s-resource-measurement.json", armName), measurement)
		attestation.Arms[index].ResourceMeasurementSHA256 = sha256DigestHex(resourceRaw)
		attestation.Arms[index].PeakApparentBytes = measurement.PeakApparentBytes
		attestation.Arms[index].PeakAllocatedBytes = measurement.PeakAllocatedBytes
		attestation.Arms[index].PeakProcessTreeRSSBytes = measurement.PeakProcessTreeRSSBytes
	}
	writeSV1DCapacityArmArtifacts(t, attestation)
	for index, armName := range armNames {
		arm := attestation.Arms[index]
		arm.CapacityArmRecordSHA256 = ""
		raw := write(fmt.Sprintf("%s-record.json", armName), arm)
		attestation.Arms[index].CapacityArmRecordSHA256 = sha256DigestHex(raw)
	}
	entries := make([]sv1dCapacitySampleAggregateEntry, 0)
	for index, armName := range armNames {
		for _, sample := range measurements[index].Samples {
			entries = append(entries, sv1dCapacitySampleAggregateEntry{Arm: armName, Sample: sample})
		}
	}
	sampleRaw := write("all-samples.json", entries)
	sampleDigest := sha256.Sum256(sampleRaw)
	attestation.SamplesSHA256 = hex.EncodeToString(sampleDigest[:])
	expectedPaths := []string{
		"treatment-resource-measurement.json", "mode-off-resource-measurement.json", "no-roster-resource-measurement.json",
		"treatment-record.json", "mode-off-record.json", "no-roster-record.json", "all-samples.json",
	}
	files := make([]sv1dCapacityManifestFile, 0, len(expectedPaths))
	for _, name := range expectedPaths {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		files = append(files, sv1dCapacityManifestFile{Path: name, Bytes: uint64(len(raw)), SHA256: hex.EncodeToString(digest[:])})
	}
	manifest := sv1dCapacityMeasurementManifest{
		SchemaVersion: 1, Contract: "v2-r2-sv1d-capacity-measurement-records-v1", MeasurementRoot: attestation.MeasurementRoot,
		SampleAggregate: sv1dCapacityManifestFile{Path: "all-samples.json", Bytes: uint64(len(sampleRaw)), SHA256: attestation.SamplesSHA256}, Files: files,
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "measurement-records-manifest.json"), manifestRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256.Sum256(manifestRaw)
	attestation.MeasurementRecordsSHA256 = hex.EncodeToString(manifestDigest[:])

	attestation.InitialAvailableBytes = measurements[0].InitialAvailableBytes
	attestation.MinimumAvailableBytes = measurements[0].MinimumAvailableBytes
	attestation.FinalAvailableBytes = measurements[2].FinalAvailableBytes
	attestation.PeakApparentBytes = 0
	attestation.PeakAllocatedBytes = 0
	attestation.PeakProcessTreeRSSBytes = 0
	attestation.PeakCgroupMemoryBytes = measurements[0].PeakCgroupMemoryBytes
	attestation.CgroupMemoryLimitBytes = measurements[0].CgroupMemoryLimitBytes
	attestation.MinimumHostMemAvailableBytes = measurements[0].MinimumHostMemAvailableBytes
	attestation.MaximumSampleGapNano = measurements[0].MaximumSampleGapNano
	attestation.SampleCount = 0
	for _, measurement := range measurements {
		if measurement.MinimumAvailableBytes < attestation.MinimumAvailableBytes {
			attestation.MinimumAvailableBytes = measurement.MinimumAvailableBytes
		}
		if measurement.PeakApparentBytes > attestation.PeakApparentBytes {
			attestation.PeakApparentBytes = measurement.PeakApparentBytes
		}
		if measurement.PeakAllocatedBytes > attestation.PeakAllocatedBytes {
			attestation.PeakAllocatedBytes = measurement.PeakAllocatedBytes
		}
		if measurement.PeakProcessTreeRSSBytes > attestation.PeakProcessTreeRSSBytes {
			attestation.PeakProcessTreeRSSBytes = measurement.PeakProcessTreeRSSBytes
		}
		if measurement.PeakCgroupMemoryBytes > attestation.PeakCgroupMemoryBytes {
			attestation.PeakCgroupMemoryBytes = measurement.PeakCgroupMemoryBytes
		}
		if measurement.CgroupMemoryLimitBytes < attestation.CgroupMemoryLimitBytes {
			attestation.CgroupMemoryLimitBytes = measurement.CgroupMemoryLimitBytes
		}
		if measurement.MinimumHostMemAvailableBytes < attestation.MinimumHostMemAvailableBytes {
			attestation.MinimumHostMemAvailableBytes = measurement.MinimumHostMemAvailableBytes
		}
		if measurement.MaximumSampleGapNano > attestation.MaximumSampleGapNano {
			attestation.MaximumSampleGapNano = measurement.MaximumSampleGapNano
		}
		attestation.SampleCount += measurement.SampleCount
	}
	attestation.PeakFilesystemConsumptionBytes = attestation.InitialAvailableBytes - attestation.MinimumAvailableBytes
	attestation.MeasuredPeakBytes = maxCapacityValue(attestation.PeakApparentBytes, attestation.PeakAllocatedBytes, attestation.PeakFilesystemConsumptionBytes)
	attestation.RequiredFreeBytes, _ = capacityRequiredFree(attestation.MeasuredPeakBytes, attestation.SafetyReserveBytes)
	attestation.RequiredAvailableMemoryBytes, _ = capacityRequiredMemory(attestation.PeakProcessTreeRSSBytes)
}

func writeSV1DCapacityArmArtifacts(t *testing.T, attestation *SV1DCapacityAttestation) {
	t.Helper()
	armNames := []string{"treatment", "mode-off", "no-roster"}
	for index, armName := range armNames {
		arm := &attestation.Arms[index]
		armDir := filepath.Join(attestation.MeasurementRoot, "arms", armName)
		renderedDir := filepath.Join(attestation.MeasurementRoot, "rendered", armName)
		for _, directory := range []string{armDir, filepath.Join(renderedDir, "venues", "north"), filepath.Join(attestation.MeasurementRoot, "configs"), filepath.Join(attestation.MeasurementRoot, "tools"), filepath.Join(attestation.MeasurementRoot, "logs")} {
			if err := os.MkdirAll(directory, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		configRaw := []byte(`{"arm":"` + armName + `"}`)
		writeArmRaw(t, filepath.Join(attestation.MeasurementRoot, "configs", "capacity-"+armName+".json"), configRaw)
		writeArmRaw(t, filepath.Join(armDir, "run-config.json"), configRaw)
		arm.CapacityConfigSHA256 = sha256DigestHex(configRaw)
		switch index {
		case 0:
			attestation.CapacityTreatmentConfigSHA256 = arm.CapacityConfigSHA256
		case 1:
			attestation.CapacityModeOffConfigSHA256 = arm.CapacityConfigSHA256
		case 2:
			attestation.CapacityNoRosterConfigSHA256 = arm.CapacityConfigSHA256
		}

		metadata := sv1dCapacityRunMetadata{
			SchemaVersion: 1, RunnerContract: "v2-r2-sv1d-capacity-runner-v1", ProbeID: attestation.ProbeID,
			CapacityOnly: true, ScientificResultEligible: false, Arm: armName,
			ExperimentID: arm.CapacityExperimentID, ConfigExperimentID: arm.CapacityExperimentID, HypothesisID: arm.CapacityHypothesisID,
			Seed: SV1DCapacitySeed, SimulatedHorizon: "5m", SimulationStartNano: int64(SV1DCapacityStartNano), SimulationEndNano: int64(SV1DCapacityEndNano),
			ConfigSHA256: arm.CapacityConfigSHA256, BinarySHA256: attestation.BinarySHA256, GitRevision: attestation.SourceRevision,
			BinaryPath: filepath.Join(attestation.MeasurementRoot, "tools", "multivenue-"+attestation.BinarySHA256), BinaryGoVersion: "go1.27.0",
			BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1", AnalyzerSHA256: attestation.AnalyzerSHA256,
			RendererSHA256: attestation.RendererSHA256, LogMode: "full", EvidenceFormat: "evstream_v3", GOMAXPROCS: SV1DCapacityGOMAXPROCS,
			OutputDir: armDir, Holdout: false,
		}
		metadataRaw, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(armDir, "run-metadata.json"), metadataRaw)
		arm.RunMetadataSHA256 = sha256DigestHex(metadataRaw)

		for _, name := range []string{"manifest.json", "greeks.json", "latency.json", "checkpoints.jsonl", "market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"} {
			writeArmRaw(t, filepath.Join(armDir, name), []byte(`{"fixture":true}`))
		}
		arm.ManifestSHA256 = testSV1DFileHash(t, filepath.Join(armDir, "manifest.json"))
		writeStrictCDFBinaryEvidence(t, armDir, []strictCDFFixtureRow{{
			SimTS: 100, ClientID: 7, Event: "capacity_probe", Route: "general.jsonl",
			Data: struct {
				VenueID  string          `json:"venue_id"`
				Sequence uint64          `json:"sequence"`
				Symbol   string          `json:"symbol"`
				Payload  json.RawMessage `json:"payload"`
			}{VenueID: "north", Sequence: 1, Payload: json.RawMessage(`{"value":1}`)},
		}}, SV1DCapacityEvidenceSchemaEpoch)
		binaryRaw, err := os.ReadFile(filepath.Join(armDir, "binary-evidence-attestation.json"))
		if err != nil {
			t.Fatal(err)
		}
		var binaryDocument map[string]json.RawMessage
		if err := json.Unmarshal(binaryRaw, &binaryDocument); err != nil {
			t.Fatal(err)
		}
		delete(binaryDocument, "unencodable_payloads")
		binaryRaw, err = json.Marshal(binaryDocument)
		if err != nil {
			t.Fatal(err)
		}
		binaryRaw = append(binaryRaw, '\n')
		writeArmRaw(t, filepath.Join(armDir, "binary-evidence-attestation.json"), binaryRaw)
		var binaryAttestation cdfBinaryEvidenceAttestation
		if err := json.Unmarshal(binaryRaw, &binaryAttestation); err != nil {
			t.Fatal(err)
		}
		arm.EventFrames = binaryAttestation.EventFrames
		arm.StreamFrames = binaryAttestation.StreamFrames
		arm.ExecutionStreamHash = binaryAttestation.ExecutionStreamHash
		arm.BinaryEvidenceAttestationSHA256 = sha256DigestHex(binaryRaw)
		arm.EventsSHA256 = testSV1DFileHash(t, filepath.Join(armDir, "events.evs"))

		renderedRecord := map[string]any{
			"client_id": uint64(7),
			"data": map[string]any{
				"venue_id": "north", "sequence": uint64(1), "global_sequence": arm.StreamFrames,
				"payload": json.RawMessage(`{"value":1}`),
			},
			"event": "capacity_probe", "sim_ts": int64(100),
		}
		renderedRecordRaw, err := json.Marshal(renderedRecord)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(renderedDir, "venues", "north", "general.jsonl"), append(renderedRecordRaw, '\n'))
		renderedDigest, err := digestRenderedEvidenceDirectory(renderedDir)
		if err != nil {
			t.Fatal(err)
		}
		report := sv1dCapacityRendererReport{EventFrames: arm.EventFrames, DictionaryFrames: arm.StreamFrames - arm.EventFrames, StreamFrames: arm.StreamFrames, ExecutionStreamHash: arm.ExecutionStreamHash, Routes: 1, RenderedDigest: renderedDigest}
		reportRaw, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(armDir, "renderer-report.json"), reportRaw)
		arm.RendererReportSHA256 = sha256DigestHex(reportRaw)
		arm.RenderedTreeDigest = renderedDigest

		renderedEvidence := cdfRenderedEvidenceAttestation{
			Domain: "rendered_binary_evidence", Ordering: "venue_sequence_files_with_global_frame_identity",
			SourceExecutionHash: arm.ExecutionStreamHash, SourceEventFrames: arm.EventFrames, SourceStreamFrames: arm.StreamFrames,
			RenderedDigest: renderedDigest, GlobalSequenceIncluded: true,
		}
		renderedEvidenceRaw, err := json.Marshal(renderedEvidence)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(renderedDir, "rendered-binary-evidence-attestation.json"), renderedEvidenceRaw)
		rendererAttestation := sv1dRendererAttestation{
			SchemaVersion: 1, Contract: "v2-r2-sv1d-renderer-attestation-v1", RendererSHA256: attestation.RendererSHA256,
			RendererSourceRevision: attestation.SourceRevision, RendererSourceModified: false, RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1",
			RendererGoVersion: "go1.27.0", RendererTrimpath: true, RendererCGOEnabled: "0", RenderedAttestationSHA256: sha256DigestHex(renderedEvidenceRaw),
		}
		rendererRaw, err := json.Marshal(rendererAttestation)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(renderedDir, "renderer-attestation.json"), rendererRaw)
		arm.RendererAttestationSHA256 = sha256DigestHex(rendererRaw)

		fixedFiles := []string{"run-config.json", "run-metadata.json", "manifest.json", "greeks.json", "latency.json", "checkpoints.jsonl", "events.evs", "binary-evidence-attestation.json", "market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"}
		fixed := make([]cdfEvidenceManifestRecord, 0, len(fixedFiles))
		for _, name := range fixedFiles {
			path := filepath.Join(armDir, name)
			if name == "run-config.json" {
				path = filepath.Join(attestation.MeasurementRoot, "configs", "capacity-"+armName+".json")
			}
			fixed = append(fixed, cdfEvidenceManifestRecord{Path: name, Bytes: int64(testSV1DFileSize(t, path)), SHA256: testSV1DFileHash(t, path)})
		}
		evidenceManifest := cdfEvidenceManifest{
			SchemaVersion: 2, Contract: "v2-integrated-longrun-evidence-manifest-v2", Cell: armName, LogMode: "full", EvidenceFormat: "evstream_v3",
			SourceRevision: attestation.SourceRevision, FixedFiles: fixed, RawJSONLFiles: 0, RawJSONLBytes: 0, RawFiles: []cdfEvidenceManifestRecord{},
		}
		evidenceRaw, err := json.Marshal(evidenceManifest)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(armDir, "evidence-manifest.json"), evidenceRaw)
		arm.EvidenceManifestSHA256 = sha256DigestHex(evidenceRaw)

		status := sv1dCapacityRunStatus{
			SchemaVersion: 1, Contract: "v2-r2-sv1d-capacity-arm-status-v1", CapacityOnly: true, ScientificResultEligible: false,
			Cell: armName, ExperimentID: arm.CapacityExperimentID, ConfigExperimentID: arm.CapacityExperimentID, HypothesisID: arm.CapacityHypothesisID,
			ExitStatus: 0, CompletionVerified: true, SimulatedHorizon: "5m", SimulationStartNano: int64(SV1DCapacityStartNano), SimulationEndNano: int64(SV1DCapacityEndNano),
			CompletionSentinels: []string{"greeks.json", "latency.json"}, RunMetadataSHA256: arm.RunMetadataSHA256, ManifestSHA256: arm.ManifestSHA256,
			GreeksSHA256: testSV1DFileHash(t, filepath.Join(armDir, "greeks.json")), LatencySHA256: testSV1DFileHash(t, filepath.Join(armDir, "latency.json")),
			CheckpointsSHA256: testSV1DFileHash(t, filepath.Join(armDir, "checkpoints.jsonl")), EvidenceManifestSHA256: arm.EvidenceManifestSHA256,
			BinaryEvidenceAttestationSHA256: arm.BinaryEvidenceAttestationSHA256,
		}
		statusRaw, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		writeArmRaw(t, filepath.Join(armDir, "run-status.json"), statusRaw)
		arm.RunStatusSHA256 = sha256DigestHex(statusRaw)
	}
}

func writeSV1DCapacityRetainedInputs(t *testing.T, attestation *SV1DCapacityAttestation) {
	t.Helper()
	root := attestation.MeasurementRoot
	for _, directory := range []string{
		filepath.Join(root, "configs"), filepath.Join(root, "tools"), filepath.Join(root, "review"),
		filepath.Join(root, "arms"), filepath.Join(root, "rendered"), filepath.Join(root, "logs"),
	} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRetained := func(path string, raw []byte) string {
		writeArmRaw(t, path, raw)
		return sha256DigestHex(raw)
	}
	writeHashedTool := func(prefix string, raw []byte) string {
		digest := sha256DigestHex(raw)
		writeArmRaw(t, filepath.Join(root, "tools", prefix+digest), raw)
		return digest
	}
	attestation.BinarySHA256 = writeHashedTool("multivenue-", []byte("multivenue fixture\n"))
	attestation.AnalyzerSHA256 = writeHashedTool("sv1dprobe-", []byte("sv1dprobe fixture\n"))
	attestation.RendererSHA256 = writeHashedTool("evsrender-", []byte("evsrender fixture\n"))
	attestation.MeasurerSHA256 = writeHashedTool("sv1dresource-", []byte("sv1dresource fixture\n"))
	runnerRaw := []byte("#!/bin/sh\nexit 0\n")
	attestation.RunnerSHA256 = sha256DigestHex(runnerRaw)
	writeArmRaw(t, filepath.Join(root, "tools", "capacity-runner-"+attestation.RunnerSHA256+".sh"), runnerRaw)
	attestation.ResourcePolicySHA256 = writeRetained(filepath.Join(root, "resource-policy-v1.json"), []byte(`{"policy":"fixture"}`))
	attestation.ReviewAttestationSHA256 = writeRetained(filepath.Join(root, "review", "attestation.json"), []byte(`{"review":"fixture"}`))
	attestation.ReviewReportSHA256 = writeRetained(filepath.Join(root, "review", "report.md"), []byte("# fixture review\n"))
	attestation.TrustedReviewKeySHA256 = writeRetained(filepath.Join(root, "review", "trusted-key.raw"), []byte("trusted review key fixture\n"))
	attestation.TargetTreatmentConfigSHA256 = writeRetained(filepath.Join(root, "configs", "target-treatment.json"), []byte(`{"target":"treatment"}`))
	attestation.TargetModeOffConfigSHA256 = writeRetained(filepath.Join(root, "configs", "target-mode-off.json"), []byte(`{"target":"mode-off"}`))
	attestation.TargetNoRosterConfigSHA256 = writeRetained(filepath.Join(root, "configs", "target-no-roster.json"), []byte(`{"target":"no-roster"}`))
	attestation.CapacityConfigDeltaSHA256 = writeRetained(filepath.Join(root, "configs", "capacity-config-delta.json"), []byte(`{"delta":"fixture"}`))
}

func writeArmRaw(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func testSV1DFileHash(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256DigestHex(raw)
}

func testSV1DFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func testSV1DCapacityCommand(attestation SV1DCapacityAttestation, arm SV1DCapacityArm) []string {
	root := attestation.MeasurementRoot
	return []string{
		filepath.Join(root, "tools", "capacity-runner-"+attestation.RunnerSHA256+".sh"), "--internal-arm", arm.Name,
		filepath.Join(root, "configs", "capacity-"+arm.Name+".json"), filepath.Join(root, "arms", arm.Name),
		filepath.Join(root, "rendered", arm.Name), filepath.Join(root, "tools", "multivenue-"+attestation.BinarySHA256),
		filepath.Join(root, "tools", "sv1dprobe-"+attestation.AnalyzerSHA256), filepath.Join(root, "tools", "evsrender-"+attestation.RendererSHA256),
		attestation.SourceRevision, arm.CapacityExperimentID, filepath.Join(root, "logs", arm.Name+".simulator.stdout.log"),
		filepath.Join(root, "logs", arm.Name+".simulator.stderr.log"),
	}
}

func attestationFilesystem(attestation *SV1DCapacityAttestation) SV1DFilesystemIdentity {
	return SV1DFilesystemIdentity{
		Device: attestation.FilesystemDevice, ID: attestation.FilesystemID, Type: attestation.FilesystemType,
		MountID: attestation.FilesystemMountID, UUID: attestation.FilesystemUUID,
	}
}
