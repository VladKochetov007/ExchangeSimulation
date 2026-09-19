package analysis

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"exchange_sim/simulations/multivenue"
)

func TestSV1DCapacityProductionStatusSerialization(t *testing.T) {
	source := string(readCapacityProducerFixture(t, "../scripts/run-v2-r2-sv1d-capacity-preflight.sh"))
	producer := capacityRunnerSection(t, source, "\tjq -n \\\n\t\t--argjson exit_status", "\n\tmv -- \"$status_tmp\"")
	for _, name := range []string{"treatment", "mode-off", "no-roster"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			files := make(map[string][]byte)
			for _, artifact := range []string{"run-metadata.json", "manifest.json", "greeks.json", "latency.json", "checkpoints.jsonl", "evidence-manifest.json", "binary-evidence-attestation.json", "market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"} {
				files[artifact] = []byte("fixture identity: " + artifact)
				writeCapacityProducerFixture(t, filepath.Join(root, artifact), files[artifact])
			}
			arm := SV1DCapacityArm{Name: name, CapacityExperimentID: "v2-r2-sv1d-capacity-977-" + name,
				CapacityHypothesisID: "V2-R2-SV1D-CAPACITY-ONLY", RunMetadataSHA256: sha256DigestHex(files["run-metadata.json"]),
				ManifestSHA256: sha256DigestHex(files["manifest.json"]), EvidenceManifestSHA256: sha256DigestHex(files["evidence-manifest.json"]),
				BinaryEvidenceAttestationSHA256: sha256DigestHex(files["binary-evidence-attestation.json"])}
			script := `set -euo pipefail
hash_file() { sha256sum -- "$1" | awk '{print $1}'; }
status=0
arm=$ARM_NAME
experiment_id="v2-r2-sv1d-capacity-977-$arm"
hypothesis_id=V2-R2-SV1D-CAPACITY-ONLY
capacity_horizon=5m
simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
run_metadata_sha256_before=$METADATA_HASH
arm_dir=$FIXTURE_ROOT
status_tmp="$arm_dir/run-status.json"
` + producer
			command := exec.Command("/bin/bash", "-c", script)
			command.Env = append(os.Environ(), "ARM_NAME="+name, "FIXTURE_ROOT="+root, "METADATA_HASH="+arm.RunMetadataSHA256)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("production status serialization: %v: %s", err, output)
			}
			raw := readCapacityProducerFixture(t, filepath.Join(root, "run-status.json"))
			files["run-status.json"] = raw
			snapshot := sv1dCapacityArmSnapshot{armFiles: files}
			if _, err := verifySV1DCapacityArmRunStatus(arm, root, snapshot); err != nil {
				t.Fatalf("actual production status rejected: %v", err)
			}
			for _, artifact := range []string{"market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"} {
				original := files[artifact]
				files[artifact] = []byte("tampered")
				if _, err := verifySV1DCapacityArmRunStatus(arm, root, snapshot); err == nil {
					t.Fatalf("status accepted mutated %s", artifact)
				}
				files[artifact] = original
			}
			assertCapacitySerializationRejectsMutations(t, raw, func(mutated []byte) error {
				files["run-status.json"] = mutated
				_, err := verifySV1DCapacityArmRunStatus(arm, root, snapshot)
				return err
			})
		})
	}
}

func TestSV1DCapacityRealRendererSerialization(t *testing.T) {
	root := t.TempDir()
	attestation := testSV1DCapacityAttestation()
	attestation.OutputParent = root
	attestation.MeasurementRoot = filepath.Join(root, "output")
	attestation.MeasurementRecordsRoot = filepath.Join(root, "measurements")
	filesystem, err := InspectSV1DFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	attestation.FilesystemDevice, attestation.FilesystemID, attestation.FilesystemType = filesystem.Device, filesystem.ID, filesystem.Type
	attestation.FilesystemMountID, attestation.FilesystemUUID = filesystem.MountID, filesystem.UUID
	for _, path := range []string{attestation.MeasurementRoot, attestation.MeasurementRecordsRoot} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeSV1DCapacityMeasurementRecords(t, &attestation)
	for _, arm := range attestation.Arms {
		t.Run(arm.Name, func(t *testing.T) {
			armDir := filepath.Join(attestation.MeasurementRoot, "arms", arm.Name)
			renderedDir := filepath.Join(attestation.MeasurementRoot, "rendered", arm.Name)
			snapshot, err := snapshotSV1DCapacityArm(attestation, arm)
			if err != nil {
				t.Fatal(err)
			}
			report := multivenue.BinaryRenderReport{EventFrames: arm.EventFrames, DictionaryFrames: arm.StreamFrames - arm.EventFrames,
				Routes: 1, ExecutionHash: arm.ExecutionStreamHash, RenderedDigest: arm.RenderedTreeDigest}
			raw, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.armFiles["renderer-report.json"] = raw
			if err := verifySV1DCapacityArmRenderer(attestation, arm, armDir, renderedDir, snapshot); err != nil {
				t.Fatalf("real renderer report failed full renderer verification: %v", err)
			}
			assertCapacitySerializationRejectsMutations(t, raw, func(mutated []byte) error {
				snapshot.armFiles["renderer-report.json"] = mutated
				return verifySV1DCapacityArmRenderer(attestation, arm, armDir, renderedDir, snapshot)
			})
			report.DictionaryFrames = ^uint64(0)
			overflow, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.armFiles["renderer-report.json"] = overflow
			if err := verifySV1DCapacityArmRenderer(attestation, arm, armDir, renderedDir, snapshot); err == nil {
				t.Fatal("overflowing dictionary count accepted")
			}
			report.DictionaryFrames = arm.StreamFrames - arm.EventFrames
			report.Routes = -1
			negativeRoutes, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.armFiles["renderer-report.json"] = negativeRoutes
			if err := verifySV1DCapacityArmRenderer(attestation, arm, armDir, renderedDir, snapshot); err == nil {
				t.Fatal("negative renderer route count accepted")
			}
		})
	}
}

func TestSV1DCapacityRendererRoutesUseProducerIntegerSemantics(t *testing.T) {
	report := multivenue.BinaryRenderReport{
		EventFrames: 1, DictionaryFrames: 0, Routes: 1,
		ExecutionHash: strings.Repeat("a", 64), RenderedDigest: strings.Repeat("b", 64),
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["routes"] = json.RawMessage("18446744073709551615")
	overflow, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var decoded sv1dCapacityRendererReport
	if err := decodeSV1DJSONWithRequiredFields(overflow, &decoded, jsonFieldNames(reflect.TypeOf(decoded), true)...); err == nil {
		t.Fatal("renderer verifier accepted a route count outside producer int range")
	}
}

func assertCapacitySerializationRejectsMutations(t *testing.T, raw []byte, verify func([]byte) error) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for name, original := range fields {
		delete(fields, name)
		missing, _ := json.Marshal(fields)
		if err := verify(missing); err == nil {
			t.Fatalf("missing %s accepted", name)
		}
		fields[name] = json.RawMessage("null")
		null, _ := json.Marshal(fields)
		if err := verify(null); err == nil {
			t.Fatalf("null %s accepted", name)
		}
		fields[name] = original
	}
	fields["unknown_field"] = json.RawMessage(`true`)
	unknown, _ := json.Marshal(fields)
	if err := verify(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field did not fail strict schema: %v", err)
	}
}
