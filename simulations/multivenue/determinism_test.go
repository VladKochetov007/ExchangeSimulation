package multivenue_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Determinism regression.
//
// The same commit, configuration and seed must produce the same events. This
// was not true for most of the campaign: runs of one seed produced two or
// three different event streams, and pinning to one core only made the
// disagreement rarer. Every cause found so far was an ordering decision taken
// by something outside the model -- a random latency stream shared between
// participants, a map iterated where the order reached the market -- so this
// test exists to catch the next one.
//
// It runs the simulator twice in fresh processes and compares the canonical
// ordered execution-stream hash. The separately checked persisted-evidence
// digests are unordered because independent log writers do not define one file
// write order. Two of the four runs use more than one processor, because
// simulated time must not depend on how much host parallelism is available.
//
// Skipped by default: it builds and runs two binaries and takes a couple of
// minutes. Run it with MULTIVENUE_DETERMINISM=1.
func TestRunsOfOneSeedProduceOneEventStream(t *testing.T) {
	if os.Getenv("MULTIVENUE_DETERMINISM") == "" {
		t.Skip("set MULTIVENUE_DETERMINISM=1 to run the cross-process determinism check")
	}
	repo := repoRoot(t)
	config := filepath.Join(repo, "research/configs/frozen-baseline-2026-08-22.json")
	if _, err := os.Stat(config); err != nil {
		t.Skipf("frozen configuration not present: %v", err)
	}

	dir := t.TempDir()
	sim := filepath.Join(dir, "multivenue")
	analyze := filepath.Join(dir, "mvanalyze")
	build(t, repo, sim, "./cmd/multivenue")
	build(t, repo, analyze, "./cmd/mvanalyze")

	cases := []struct {
		name    string
		procs   string
		logMode string
	}{
		{"single-core-a", "1", "full"},
		{"single-core-b", "1", "full"},
		{"multi-core", "4", "full"},
		{"logs-off", "4", "none"},
	}
	execution := make(map[string]string, len(cases))
	evidence := make(map[string]string, len(cases))
	evidenceArtifact := make(map[string]string, len(cases))
	latency := make(map[string]string, len(cases))
	for _, testCase := range cases {
		logDir := filepath.Join(dir, testCase.name)
		cfg := checkpointedConfig(t, config, dir, testCase.logMode)
		// Ten seconds covers the one-second maker telemetry path and
		// keeps this fresh-process regression practical in ordinary CI. The
		// campaign's separate 24h acceptance remains the horizon evidence.
		run := exec.Command(sim, "-config", cfg, "-seed", "101", "-duration", "10s", "-logdir", logDir)
		run.Dir = repo
		run.Env = append(os.Environ(), "GOMAXPROCS="+testCase.procs)
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("%s: run failed: %v\n%s", testCase.name, err, out)
		}
		execution[testCase.name] = finalCheckpoint(t, logDir)
		latencyRaw, err := os.ReadFile(filepath.Join(logDir, "latency.json"))
		if err != nil {
			t.Fatalf("%s: read compact latency evidence: %v", testCase.name, err)
		}
		latency[testCase.name] = string(latencyRaw)
		if testCase.logMode == "full" {
			hash := exec.Command(analyze, "-metric", "evidencehash", "-json", logDir)
			hash.Dir = repo
			out, err := hash.Output()
			if err != nil {
				t.Fatalf("%s: evidence digest failed: %v", testCase.name, err)
			}
			var payload struct {
				Result struct {
					Events   int64  `json:"events"`
					Digest   string `json:"digest"`
					Domain   string `json:"domain"`
					Ordering string `json:"ordering"`
				} `json:"result"`
			}
			if err := json.Unmarshal(out, &payload); err != nil {
				t.Fatalf("%s: decode evidence digest: %v", testCase.name, err)
			}
			if payload.Result.Domain != "persisted_evidence" || payload.Result.Ordering != "unordered_multiset" {
				t.Fatalf("%s: wrong evidence domain: %+v", testCase.name, payload.Result)
			}
			evidence[testCase.name] = fmt.Sprintf("%d %s", payload.Result.Events, payload.Result.Digest)
			artifact := exec.Command(analyze, "-metric", "evidenceartifacthash", "-json", logDir)
			artifact.Dir = repo
			artifactOutput, err := artifact.Output()
			if err != nil {
				t.Fatalf("%s: evidence artifact digest failed: %v", testCase.name, err)
			}
			var offline struct {
				Result struct {
					Events   int64  `json:"events"`
					Digest   string `json:"digest"`
					Domain   string `json:"domain"`
					Ordering string `json:"ordering"`
				} `json:"result"`
			}
			if err := json.Unmarshal(artifactOutput, &offline); err != nil {
				t.Fatalf("%s: decode evidence artifact digest: %v", testCase.name, err)
			}
			artifactRaw, err := os.ReadFile(filepath.Join(logDir, "evidence-artifact-hash.json"))
			if err != nil {
				t.Fatalf("%s: read runtime evidence artifact digest: %v", testCase.name, err)
			}
			var runtime struct {
				Events   int64  `json:"events"`
				Digest   string `json:"digest"`
				Domain   string `json:"domain"`
				Ordering string `json:"ordering"`
			}
			if err := json.Unmarshal(artifactRaw, &runtime); err != nil {
				t.Fatalf("%s: decode runtime evidence artifact digest: %v", testCase.name, err)
			}
			if offline.Result.Domain != runtime.Domain || offline.Result.Ordering != runtime.Ordering || offline.Result.Events != runtime.Events || offline.Result.Digest != runtime.Digest {
				t.Fatalf("%s: offline evidence artifact digest %+v does not match runtime %+v", testCase.name, offline.Result, runtime)
			}
			evidenceArtifact[testCase.name] = fmt.Sprintf("%d %s", runtime.Events, runtime.Digest)
		}
	}
	reference := execution[cases[0].name]
	for _, testCase := range cases[1:] {
		if execution[testCase.name] != reference {
			t.Errorf("%s produced a different execution checkpoint from %s;\n  %s\n  %s", testCase.name, cases[0].name, reference, execution[testCase.name])
		}
	}
	if evidence["single-core-a"] != evidence["single-core-b"] || evidence["single-core-a"] != evidence["multi-core"] {
		t.Errorf("persisted evidence digest differs across equivalent full-log executions: %v", evidence)
	}
	if evidenceArtifact["single-core-a"] != evidenceArtifact["single-core-b"] || evidenceArtifact["single-core-a"] != evidenceArtifact["multi-core"] {
		t.Errorf("persisted evidence artifact digest differs across equivalent full-log executions: %v", evidenceArtifact)
	}
	for _, testCase := range cases[1:] {
		if latency[testCase.name] != latency[cases[0].name] {
			t.Errorf("compact latency delivery evidence differs for %s", testCase.name)
		}
	}
}

func checkpointedConfig(t *testing.T, source, dir, logMode string) string {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["checkpoint_interval_seconds"] = float64(60)
	cfg["log_mode"] = logMode
	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config-"+logMode+".json")
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func finalCheckpoint(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "checkpoints.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		t.Fatal("no execution checkpoints")
	}
	last := rows[len(rows)-1]
	if last["domain"] != "execution_observations" || last["ordering"] != "ordered_stream" {
		t.Fatalf("wrong execution checkpoint contract: %+v", last)
	}
	hash, _ := last["execution_stream_hash"].(string)
	legacy, _ := last["rolling_hash"].(string)
	if hash == "" || hash != legacy {
		t.Fatalf("execution hash missing or differs from legacy rolling hash: %+v", last)
	}
	return fmt.Sprintf("%.0f %s", last["event_count"], hash)
}

func build(t *testing.T, repo, output, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, out)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find the module root")
	return ""
}
