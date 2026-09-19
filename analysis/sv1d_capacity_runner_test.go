package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func capacityRunnerSection(t *testing.T, source, start, end string) string {
	t.Helper()
	begin := strings.Index(source, start)
	if begin < 0 {
		t.Fatalf("source section start missing: %q", start)
	}
	finish := strings.Index(source[begin:], end)
	if finish < 0 {
		t.Fatalf("source section end missing: %q", end)
	}
	return source[begin : begin+finish]
}

func readCapacityProducerFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeCapacityProducerFixture(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func capacityProducerConfigFixture(t *testing.T, arm string) (string, map[string]json.RawMessage) {
	t.Helper()
	target, err := filepath.Abs("../research/configs/v2-r2-sv1d-activation/activation-659-" + arm + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(readCapacityProducerFixture(t, target), &config); err != nil {
		t.Fatal(err)
	}
	config["seed"] = json.RawMessage("977")
	config["experiment_id"] = json.RawMessage(fmt.Sprintf("%q", "v2-r2-sv1d-capacity-977-"+arm))
	config["hypothesis_id"] = json.RawMessage(`"V2-R2-SV1D-CAPACITY-ONLY"`)
	config["status"] = json.RawMessage(`"capacity-preflight-only"`)
	config["description"] = json.RawMessage(`"Outcome-ineligible binary-evidence capacity preflight arm"`)
	return target, config
}

func TestSV1DCapacityProductionPaths(t *testing.T) {
	runnerRaw := readCapacityProducerFixture(t, "../scripts/run-v2-r2-sv1d-capacity-preflight.sh")
	source := string(runnerRaw)
	invocation := capacityRunnerSection(t, source, "\tGOMAXPROCS=2 GOMEMLIMIT=4GiB SV1D_CAPACITY_ROOT_DIR=", "\n\tresource_status=$?")
	metadataProducer := capacityRunnerSection(t, source, "\tjq -n \\\n\t\t--arg arm", "\n\trun_metadata_sha256_before=")
	for _, name := range []string{"treatment", "mode-off", "no-roster"} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "retained with spaces")
			if err := os.MkdirAll(filepath.Join(root, "tools"), 0755); err != nil {
				t.Fatal(err)
			}
			fixtureBinary := []byte("independent non-executable simulator identity fixture")
			attestation := SV1DCapacityAttestation{MeasurementRoot: root, RunnerSHA256: sha256DigestHex(runnerRaw), BinarySHA256: sha256DigestHex(fixtureBinary), AnalyzerSHA256: strings.Repeat("b", 64), RendererSHA256: strings.Repeat("c", 64), SourceRevision: "94e63d6abaaac5d790a070f303a08da4445b28af", ProbeID: "v2-r2-sv1d-activation-659"}
			runnerPath := filepath.Join(root, "tools", "capacity-runner-"+attestation.RunnerSHA256+".sh")
			writeCapacityProducerFixture(t, runnerPath, runnerRaw)
			_, config := capacityProducerConfigFixture(t, name)
			configRaw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			arm := SV1DCapacityArm{Name: name, CapacityExperimentID: "v2-r2-sv1d-capacity-977-" + name, CapacityHypothesisID: "V2-R2-SV1D-CAPACITY-ONLY", CapacityConfigSHA256: sha256DigestHex(configRaw)}
			script := `set -euo pipefail
capture_resource() { while [[ "$1" != -- ]]; do shift; done; shift; printf '%s\0' "$@"; }
staged_sv1dresource=capture_resource
output_root=$FIXTURE_ROOT
output_parent=$FIXTURE_ROOT
root_dir=$FIXTURE_ROOT
retained_capacity_runner=$RUNNER_PATH
staged_multivenue=/unexecuted-staging/multivenue
staged_sv1dprobe=/unexecuted-staging/sv1dprobe
staged_evsrender=/unexecuted-staging/evsrender
arm=$ARM_NAME
multivenue_sha256=$SIM_HASH
sv1dprobe_sha256=$ANALYZER_HASH
evsrender_sha256=$RENDERER_HASH
source_revision=$SOURCE_REVISION
arm_dir="$output_root/arms/$arm"
rendered_dir="$output_root/rendered/$arm"
stdout_log="$output_root/logs/$arm.simulator.stdout.log"
stderr_log="$output_root/logs/$arm.simulator.stderr.log"
measurement_path="$output_root/$arm-measurement.json"
declare -A capacity_config_for capacity_experiment_for
capacity_config_for[$arm]="$output_root/configs/capacity-$arm.json"
capacity_experiment_for[$arm]="v2-r2-sv1d-capacity-977-$arm"
` + invocation
			command := exec.Command("/bin/bash", "-c", script)
			command.Env = append(os.Environ(), "FIXTURE_ROOT="+root, "RUNNER_PATH="+runnerPath, "ARM_NAME="+name, "SIM_HASH="+attestation.BinarySHA256, "ANALYZER_HASH="+attestation.AnalyzerSHA256, "RENDERER_HASH="+attestation.RendererSHA256, "SOURCE_REVISION="+attestation.SourceRevision)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			output, err := command.Output()
			if err != nil {
				t.Fatalf("capture failed: %v: %s", err, stderr.String())
			}
			args := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
			measurement := SV1DResourceMeasurement{Command: args}
			if err := validateSV1DCapacityResourceCommand(measurement, attestation, arm); err != nil {
				t.Fatal(err)
			}
			for index := range args {
				mutated := SV1DResourceMeasurement{Command: append([]string(nil), args...)}
				mutated.Command[index] += "-tampered"
				if err := validateSV1DCapacityResourceCommand(mutated, attestation, arm); err == nil {
					t.Fatalf("argv index %d tamper accepted", index)
				}
			}
			t.Logf("%s: production argv verifies and all 13 argument mutations reject; captured tool paths=%q", name, args[6:9])
			if err := os.MkdirAll(args[4], 0755); err != nil {
				t.Fatal(err)
			}
			metadataScript := `set -euo pipefail
hash_file() { case "$1" in "$simulator") printf '%s' "$SIM_HASH";; "$analyzer") printf '%s' "$ANALYZER_HASH";; "$renderer") printf '%s' "$RENDERER_HASH";; *) exit 97;; esac; }
binary_go_version() { printf 'go1.27.0'; }
arm=$ARM_NAME
experiment_id=$EXPERIMENT
hypothesis_id=V2-R2-SV1D-CAPACITY-ONLY
capacity_seed=977
capacity_horizon=5m
simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
config_digest=$CONFIG_HASH
source_revision=$SOURCE_REVISION
simulator=$SIM_PATH
analyzer=$ANALYZER_PATH
renderer=$RENDERER_PATH
arm_dir=$ARM_DIR
log_mode=full
evidence_format=evstream_v3
capacity_contract=v2-r2-sv1d-capacity-runner-v1
capacity_probe_id=v2-r2-sv1d-activation-659
` + metadataProducer
			metadataCommand := exec.Command("/bin/bash", "-c", metadataScript)
			metadataCommand.Env = append(command.Env, "EXPERIMENT="+arm.CapacityExperimentID, "CONFIG_HASH="+arm.CapacityConfigSHA256, "SIM_PATH="+args[6], "ANALYZER_PATH="+args[7], "RENDERER_PATH="+args[8], "ARM_DIR="+args[4])
			if output, err := metadataCommand.CombinedOutput(); err != nil {
				t.Fatalf("metadata producer failed: %v: %s", err, output)
			}
			raw := readCapacityProducerFixture(t, filepath.Join(args[4], "run-metadata.json"))
			t.Logf("actual production metadata JSON: %s", raw)
			snapshot := sv1dCapacityArmSnapshot{armFiles: map[string][]byte{"run-metadata.json": raw}, retainedFiles: map[string][]byte{"simulator": fixtureBinary}}
			if err := verifySV1DCapacityArmRunMetadata(attestation, arm, args[4], snapshot); err != nil {
				t.Fatalf("actual metadata rejected: %v", err)
			}
			assertCapacitySerializationRejectsMutations(t, raw, func(mutated []byte) error {
				snapshot.armFiles["run-metadata.json"] = mutated
				return verifySV1DCapacityArmRunMetadata(attestation, arm, args[4], snapshot)
			})
			snapshot.armFiles["run-metadata.json"] = raw
			snapshot.retainedFiles["simulator"] = []byte("mutated")
			if err := verifySV1DCapacityArmRunMetadata(attestation, arm, args[4], snapshot); err == nil {
				t.Fatal("metadata simulator-byte mutation accepted")
			}
			t.Logf("%s: actual argv and actual jq metadata verified, 13 argv mutations and retained-byte mutation rejected; fixture root contains spaces", name)
		})
	}
}

func TestSV1DCapacityPlanDigestGate(t *testing.T) {
	configRoot := "../research/configs/v2-r2-sv1d-activation"
	triad, err := ValidateSV1DConfigTriadFiles(SV1DConfigPaths{
		Treatment: filepath.Join(configRoot, "activation-659-treatment.json"),
		ModeOff:   filepath.Join(configRoot, "activation-659-mode-off.json"),
		NoRoster:  filepath.Join(configRoot, "activation-659-no-roster.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRegisteredSV1DProbePlan(triad, "67bba6699a39167e1c2f6cf7e432cfcda16aeda4",
		strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	typedDigest, err := SV1DProbePlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	typedRaw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	independentDigest := sha256DigestHex(append([]byte("v2-r2-sv1d-probe-plan-v2\x00"), typedRaw...))
	if typedDigest != independentDigest {
		t.Fatal("typed digest did not match the explicit domain calculation")
	}
	document := struct {
		PlanSHA256 string        `json:"plan_sha256"`
		Plan       SV1DProbePlan `json:"plan"`
	}{typedDigest, plan}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	fileDigest := sha256DigestHex(raw)
	if typedDigest == fileDigest {
		t.Fatal("typed and file digests unexpectedly equal")
	}
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.json")
	if err := os.WriteFile(planPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	runnerRaw, err := os.ReadFile("../scripts/run-v2-r2-sv1d-capacity-preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	var guards []string
	for _, line := range strings.Split(string(runnerRaw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), `[[ "$internal_review_plan_file_sha256" =~`) ||
			strings.HasPrefix(strings.TrimSpace(line), `[[ "$(hash_file "$internal_review_plan")" ==`) {
			guards = append(guards, line)
		}
	}
	if len(guards) != 2 {
		t.Fatalf("found %d raw plan gates", len(guards))
	}
	script := "set -euo pipefail\nhash_file() { sha256sum -- \"$1\" | awk '{print $1}'; }\n" + strings.Join(guards, "\n")
	runGuard := func(digest string) error {
		command := exec.Command("/bin/bash", "-c", script)
		command.Env = append(os.Environ(), "internal_review_plan="+planPath,
			"internal_review_plan_file_sha256="+digest)
		return command.Run()
	}
	if err := runGuard(fileDigest); err != nil {
		t.Fatalf("valid raw digest failed: %v", err)
	}
	if err := runGuard(typedDigest); err == nil {
		t.Fatal("typed digest accepted as raw file digest")
	}
	if err := os.WriteFile(planPath, append(raw, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if err := runGuard(fileDigest); err == nil {
		t.Fatal("raw file tamper accepted")
	}
	formatted, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		PlanSHA256 string        `json:"plan_sha256"`
		Plan       SV1DProbePlan `json:"plan"`
	}
	if err := json.Unmarshal(formatted, &decoded); err != nil {
		t.Fatal(err)
	}
	formattedDigest, err := SV1DProbePlanSHA256(decoded.Plan)
	if err != nil || formattedDigest != typedDigest {
		t.Fatalf("formatting changed typed identity: %v", err)
	}
	plan.Treatment.ConfigSHA256 = strings.Repeat("d", 64)
	mutatedDigest, err := SV1DProbePlanSHA256(plan)
	if err != nil || mutatedDigest == typedDigest {
		t.Fatal("semantic identity mutation preserved typed digest")
	}
	t.Logf("fixture digests: typed=%s raw=%s; valid raw accepted, typed-as-raw rejected, raw tamper rejected, typed formatting stable, semantic mutation detected", typedDigest, fileDigest)
}

func TestSV1DCapacityPrivateConfigBoundary(t *testing.T) {
	source, err := os.ReadFile("../scripts/run-v2-r2-sv1d-capacity-preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "\tidentity_filter='del(.seed,.experiment_id,.hypothesis_id,.status,.description)'")
	if start < 0 {
		t.Fatal("private-entry delta check not found")
	}
	suffix := string(source)[start:]
	end := strings.Index(suffix, "\n\tsource \"$root_dir/scripts/v2-integrated-longrun-r2-contract.sh\"")
	if end < 0 {
		t.Fatal("private-entry delta check end not found")
	}
	actualGuard := suffix[:end]
	targetPath, err := filepath.Abs("../research/configs/v2-r2-sv1d-activation/activation-659-treatment.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	config["experiment_id"] = json.RawMessage(`"v2-r2-sv1d-capacity-977-treatment"`)
	config["hypothesis_id"] = json.RawMessage(`"V2-R2-SV1D-CAPACITY-ONLY"`)
	config["status"] = json.RawMessage(`"capacity-preflight-only"`)
	config["description"] = json.RawMessage(`"Outcome-ineligible binary-evidence capacity preflight arm"`)
	root := t.TempDir()
	config["seed"] = json.RawMessage("977")
	for _, testCase := range []struct {
		name, field, value string
		accepted           bool
	}{
		{"registered", "seed", "977", true},
		{"activation-seed", "seed", "659", false},
		{"reserved-619", "seed", "619", false},
		{"reserved-631", "seed", "631", false},
		{"reserved-641", "seed", "641", false},
		{"string-seed", "seed", `"977"`, false},
		{"null-seed", "seed", "null", false},
		{"wrong-arm", "experiment_id", `"v2-r2-sv1d-capacity-977-mode-off"`, false},
		{"wrong-hypothesis", "hypothesis_id", `"SCIENTIFIC"`, false},
		{"wrong-status", "status", `"development"`, false},
		{"wrong-description", "description", `"activation"`, false},
		{"economic-delta", "log_mode", `"off"`, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			original := config[testCase.field]
			defer func() { config[testCase.field] = original }()
			config[testCase.field] = json.RawMessage(testCase.value)
			configRaw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(root, "capacity-"+testCase.name+".json")
			if err := os.WriteFile(configPath, configRaw, 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("/bin/bash", "-c", "set -euo pipefail\n"+actualGuard)
			command.Env = append(os.Environ(), "target_config="+targetPath, "config="+configPath, "expected_experiment_id=v2-r2-sv1d-capacity-977-treatment")
			output, err := command.CombinedOutput()
			if testCase.accepted {
				if err != nil {
					t.Fatalf("registered seed rejected: %v: %s", err, output)
				}
				t.Log("registered capacity seed accepted by private-entry predicate")
			} else if err == nil {
				t.Fatalf("private-entry pre-start config predicate accepted %s; no arm was launched", testCase.name)
			}
		})
	}
}
