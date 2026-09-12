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

func TestReadStrictJSONRejectsSymlinkedFile(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	link := filepath.Join(directory, "link.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	var document planDocument
	if err := readStrictJSON(link, &document); err == nil {
		t.Fatal("symlinked JSON file was accepted")
	}
}

func TestReadContentAddressedArmResultRejectsMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "activation")
	metadataPath := filepath.Join(root, "provenance", "activation-run-metadata.json")
	resultRoot := filepath.Join(root, "provenance", "arm-results")
	if err := os.MkdirAll(resultRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(armResultDocument{SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	path := filepath.Join(resultRoot, "treatment-"+hex.EncodeToString(digest[:])+".json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readContentAddressedArmResult(path, "treatment", metadataPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readContentAddressedArmResult(path, "treatment", metadataPath); err == nil {
		t.Fatal("mutated content-addressed arm result was accepted")
	}
}

func TestVerifyScoreCorpusManifestRejectsCorpusAndManifestMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, artifacts scoreCorpusTestArtifacts)
		want   string
	}{
		{
			name: "arm bytes",
			mutate: func(t *testing.T, artifacts scoreCorpusTestArtifacts) {
				raw, err := os.ReadFile(artifacts.armPaths["treatment"])
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(artifacts.armPaths["treatment"], append(raw, 'x'), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "failed its content-addressed digest",
		},
		{
			name: "arm renamed",
			mutate: func(t *testing.T, artifacts scoreCorpusTestArtifacts) {
				oldPath := artifacts.armPaths["treatment"]
				if err := os.Rename(oldPath, filepath.Join(filepath.Dir(oldPath), "renamed-arm.json")); err != nil {
					t.Fatal(err)
				}
			},
			want: "unexpected file",
		},
		{
			name: "extra arm",
			mutate: func(t *testing.T, artifacts scoreCorpusTestArtifacts) {
				if err := os.WriteFile(filepath.Join(artifacts.armRoot, "unexpected.json"), []byte("{}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "unexpected file",
		},
		{
			name: "manifest unknown field",
			mutate: func(t *testing.T, artifacts scoreCorpusTestArtifacts) {
				raw, err := os.ReadFile(artifacts.manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(raw, &fields); err != nil {
					t.Fatal(err)
				}
				fields["unexpected"] = json.RawMessage(`true`)
				mutated, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(artifacts.manifestPath, append(mutated, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "unknown field",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifacts := writeScoreCorpusTestArtifacts(t)
			test.mutate(t, artifacts)
			if err := verifyScoreCorpusManifest(artifacts.manifestPath, artifacts.activationMetadataPath, artifacts.scorePath, artifacts.planSHA256); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("corpus mutation error = %v, want %q", err, test.want)
			}
		})
	}
}

type scoreCorpusTestArtifacts struct {
	activationMetadataPath string
	scorePath              string
	manifestPath           string
	planSHA256             string
	armRoot                string
	armPaths               map[string]string
}

func writeScoreCorpusTestArtifacts(t *testing.T) scoreCorpusTestArtifacts {
	t.Helper()
	root := t.TempDir()
	provenanceRoot := filepath.Join(root, "provenance")
	armRoot := filepath.Join(provenanceRoot, "arm-results")
	if err := os.MkdirAll(armRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	activationMetadataPath := filepath.Join(provenanceRoot, "activation-run-metadata.json")
	if err := os.WriteFile(activationMetadataPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	armNames := []string{"treatment", "mode-off", "no-roster"}
	armPaths := make(map[string]string, len(armNames))
	armResults := make([]analysis.SV1DProbeArmResult, 0, len(armNames))
	for _, armName := range armNames {
		raw, err := json.Marshal(armResultDocument{
			SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID,
			Arm: analysis.SV1DProbeArmResult{ArmName: armName},
		})
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		path := filepath.Join(armRoot, armName+"-"+hex.EncodeToString(digest[:])+".json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		armPaths[armName] = path
		armResults = append(armResults, analysis.SV1DProbeArmResult{ArmName: armName})
	}
	planSHA256 := strings.Repeat("a", 64)
	armPathsInOrder := []string{armPaths[armNames[0]], armPaths[armNames[1]], armPaths[armNames[2]]}
	corpus, err := captureScoreCorpus(activationMetadataPath, armNames, armPathsInOrder)
	if err != nil {
		t.Fatal(err)
	}
	scorePath := filepath.Join(root, "score.json")
	if err := publishJSON(scorePath, scoreDocument{
		SchemaVersion: 1, Contract: scoreContract, ProbeID: probeID, PlanSHA256: planSHA256,
		Score: analysis.SV1DProbeScore{PlanSHA256: planSHA256}, Arms: armResults, ArmCorpus: corpus,
	}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(provenanceRoot, "score-corpus-manifest.json")
	if err := publishScoreCorpusManifest(manifestPath, activationMetadataPath, scorePath, planSHA256, corpus); err != nil {
		t.Fatal(err)
	}
	return scoreCorpusTestArtifacts{
		activationMetadataPath: activationMetadataPath, scorePath: scorePath, manifestPath: manifestPath,
		planSHA256: planSHA256, armRoot: armRoot, armPaths: armPaths,
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
