package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"exchange_sim/analysis"
)

func TestReadStrictJSONRejectsDuplicateAndTrailingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.json")
	for _, raw := range []string{
		`{"schema_version":1,"schema_version":1}`,
		`{"schema_version":1} {"schema_version":1}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		var document planDocument
		if err := readStrictJSON(path, &document); err == nil {
			t.Fatalf("malformed JSON was accepted: %s", raw)
		}
	}
}

func TestReadStrictJSONRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"unexpected":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var document planDocument
	if err := readStrictJSON(path, &document); err == nil {
		t.Fatal("unknown JSON field was accepted")
	}
}

func TestPublishJSONRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "result.json")
	if err := publishJSON(path, map[string]string{"value": "first"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishJSON(path, map[string]string{"value": "second"}); err == nil {
		t.Fatal("existing output was overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || !strings.Contains(string(after), "first") {
		t.Fatalf("existing output changed: before=%q after=%q", before, after)
	}
}

func TestPublishFailedArmBindsPlanAndCannotCertifyCompleteness(t *testing.T) {
	configRoot := filepath.Join("..", "..", "research", "configs", "v2-r2-sv1d-activation")
	triad, err := analysis.ValidateSV1DConfigTriadFiles(analysis.SV1DConfigPaths{
		Treatment: filepath.Join(configRoot, "activation-659-treatment.json"),
		ModeOff:   filepath.Join(configRoot, "activation-659-mode-off.json"),
		NoRoster:  filepath.Join(configRoot, "activation-659-no-roster.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	executableRaw, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	executableDigest := sha256.Sum256(executableRaw)
	plan, err := analysis.BuildRegisteredSV1DProbePlan(triad, strings.Repeat("a", 40), strings.Repeat("b", 64), hex.EncodeToString(executableDigest[:]), strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	planSHA256, err := analysis.SV1DProbePlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	if err := publishJSON(planPath, planDocument{SchemaVersion: 1, Contract: probePlanContract, ProbeID: probeID, PlanSHA256: planSHA256, Plan: plan}); err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(filepath.Dir(planPath), "mode-off-failure.json")
	if err := publishFailedArm(resultPath, planPath, "mode-off", "renderer exited with status 7"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var document armResultDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if document.Arm.Complete || document.Arm.EvidenceValid || document.Arm.TerminalValuationValid || len(document.Arm.FailureReasons) != 1 || document.Arm.FailureReasons[0] != "renderer exited with status 7" {
		t.Fatalf("failed arm was certifying or lost its reason: %+v", document.Arm)
	}
	if document.Arm.PlanSHA256 != planSHA256 || document.Arm.ExperimentID != plan.ModeOff.ExperimentID || document.Arm.ConfigSHA256 != plan.ModeOff.ConfigSHA256 {
		t.Fatalf("failed arm lost immutable plan identity: %+v", document.Arm)
	}
}
