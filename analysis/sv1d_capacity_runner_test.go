package analysis

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This regression expands the production command through a capture
// function. It never executes the resource wrapper, internal arm, or simulator.
func TestSV1DCapacityProductionPaths(t *testing.T) {
	root := t.TempDir()
	retained := filepath.Join(root, "retained")
	staging := filepath.Join(root, "staging")
	runnerRaw, err := os.ReadFile("../scripts/run-v2-r2-sv1d-capacity-preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	runnerDigest := sha256DigestHex(runnerRaw)
	runnerPath := filepath.Join(retained, "tools", "capacity-runner-"+runnerDigest+".sh")
	if err := os.MkdirAll(filepath.Dir(runnerPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runnerPath, runnerRaw, 0444); err != nil {
		t.Fatal(err)
	}
	simulatorBytes := []byte("reviewer fixture bytes; not executable")
	attestation := SV1DCapacityAttestation{
		MeasurementRoot: retained, RunnerSHA256: runnerDigest,
		BinarySHA256: sha256DigestHex(simulatorBytes), AnalyzerSHA256: strings.Repeat("b", 64),
		RendererSHA256: strings.Repeat("c", 64), SourceRevision: "67bba6699a39167e1c2f6cf7e432cfcda16aeda4",
		ProbeID: "v2-r2-sv1d-activation-659",
	}
	arm := SV1DCapacityArm{Name: "treatment", CapacityExperimentID: "v2-r2-sv1d-capacity-977-treatment",
		CapacityHypothesisID: "V2-R2-SV1D-CAPACITY-ONLY", CapacityConfigSHA256: strings.Repeat("d", 64)}
	source := string(runnerRaw)
	start := strings.Index(source, "\tGOMAXPROCS=2 GOMEMLIMIT=4GiB SV1D_CAPACITY_ROOT_DIR=")
	if start < 0 {
		t.Fatal("production resource invocation not found")
	}
	end := strings.Index(source[start:], "\n\tresource_status=$?")
	if end < 0 {
		t.Fatal("production resource invocation end not found")
	}
	invocation := source[start : start+end]
	for _, assignment := range []string{
		`staged_multivenue="$staging_root/tools/multivenue-$(hash_file "$multivenue_binary")"`,
		`staged_sv1dprobe="$staging_root/tools/sv1dprobe-$(hash_file "$sv1dprobe_binary")"`,
		`staged_evsrender="$staging_root/tools/evsrender-$(hash_file "$evsrender_binary")"`,
	} {
		if !strings.Contains(source, assignment) {
			t.Fatalf("staging definition changed: %s", assignment)
		}
	}
	script := `set -euo pipefail
capture_resource() {
    while [[ "$1" != -- ]]; do shift; done
    shift
    printf '%s\0' "$@"
}
staged_sv1dresource=capture_resource
root_dir=$SOURCE_ROOT
output_root=$RETAINED_ROOT
output_parent=$REPRO_ROOT
retained_capacity_runner=$RUNNER_PATH
staged_multivenue="$STAGING_ROOT/tools/multivenue-$SIM_HASH"
staged_sv1dprobe="$STAGING_ROOT/tools/sv1dprobe-$ANALYZER_HASH"
staged_evsrender="$STAGING_ROOT/tools/evsrender-$RENDERER_HASH"
multivenue_sha256=$SIM_HASH
sv1dprobe_sha256=$ANALYZER_HASH
evsrender_sha256=$RENDERER_HASH
source_revision=$SOURCE_REVISION
arm=treatment
arm_dir="$output_root/arms/$arm"
rendered_dir="$output_root/rendered/$arm"
stdout_log="$output_root/logs/$arm.simulator.stdout.log"
stderr_log="$output_root/logs/$arm.simulator.stderr.log"
measurement_path="$REPRO_ROOT/measurement.json"
declare -A capacity_config_for capacity_experiment_for
capacity_config_for[treatment]="$output_root/configs/capacity-treatment.json"
capacity_experiment_for[treatment]=v2-r2-sv1d-capacity-977-treatment
` + invocation
	command := exec.Command("/bin/bash", "-c", script)
	sourceRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	command.Env = append(os.Environ(), "SOURCE_ROOT="+sourceRoot, "RETAINED_ROOT="+retained,
		"REPRO_ROOT="+root, "RUNNER_PATH="+runnerPath, "STAGING_ROOT="+staging,
		"SIM_HASH="+attestation.BinarySHA256, "ANALYZER_HASH="+attestation.AnalyzerSHA256,
		"RENDERER_HASH="+attestation.RendererSHA256, "SOURCE_REVISION="+attestation.SourceRevision)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("capture only shell: %v: %s", err, stderr.String())
	}
	args := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
	if len(args) != 13 {
		t.Fatalf("captured %d arguments: %q", len(args), args)
	}
	emitted := SV1DResourceMeasurement{Command: args}
	t.Run("retained_path_control", func(t *testing.T) {
		control := emitted
		control.Command = append([]string(nil), args...)
		for _, index := range []int{6, 7, 8} {
			control.Command[index] = filepath.Join(retained, "tools", filepath.Base(args[index]))
		}
		if err := validateSV1DCapacityResourceCommand(control, attestation, arm); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("production_command_should_verify", func(t *testing.T) {
		t.Logf("captured simulator=%s; verifier requires=%s", args[6], filepath.Join(retained, "tools", filepath.Base(args[6])))
		if err := validateSV1DCapacityResourceCommand(emitted, attestation, arm); err != nil {
			t.Fatalf("production command is rejected: %v", err)
		}
	})
	t.Run("production_metadata_should_verify", func(t *testing.T) {
		metadata := sv1dCapacityRunMetadata{
			SchemaVersion: 1, RunnerContract: "v2-r2-sv1d-capacity-runner-v1", ProbeID: attestation.ProbeID,
			CapacityOnly: true, Arm: arm.Name, ExperimentID: arm.CapacityExperimentID,
			ConfigExperimentID: arm.CapacityExperimentID, HypothesisID: arm.CapacityHypothesisID,
			Seed: SV1DCapacitySeed, SimulatedHorizon: "5m", SimulationStartNano: int64(SV1DCapacityStartNano),
			SimulationEndNano: int64(SV1DCapacityEndNano), ConfigSHA256: arm.CapacityConfigSHA256,
			BinarySHA256: attestation.BinarySHA256, GitRevision: attestation.SourceRevision,
			AnalyzerSHA256: attestation.AnalyzerSHA256, RendererSHA256: attestation.RendererSHA256,
			LogMode: "full", EvidenceFormat: "evstream_v3", GOMAXPROCS: SV1DCapacityGOMAXPROCS,
			OutputDir: args[4], BinaryPath: args[6], BinaryGoVersion: "go1.27.0",
			BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
		}
		raw, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		snapshot := sv1dCapacityArmSnapshot{armFiles: map[string][]byte{"run-metadata.json": raw},
			retainedFiles: map[string][]byte{"simulator": simulatorBytes}}
		control := metadata
		control.BinaryPath = filepath.Join(retained, "tools", filepath.Base(args[6]))
		controlRaw, err := json.Marshal(control)
		if err != nil {
			t.Fatal(err)
		}
		controlSnapshot := snapshot
		controlSnapshot.armFiles = map[string][]byte{"run-metadata.json": controlRaw}
		if err := verifySV1DCapacityArmRunMetadata(attestation, arm, args[4], controlSnapshot); err != nil {
			t.Fatalf("retained metadata control failed: %v", err)
		}
		t.Log("retained binary_path control passed")
		if err := verifySV1DCapacityArmRunMetadata(attestation, arm, args[4], snapshot); err != nil {
			t.Fatalf("production binary_path is rejected: %v", err)
		}
	})
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
