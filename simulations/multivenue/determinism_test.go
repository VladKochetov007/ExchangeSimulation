package multivenue_test

import (
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
// event digest, which is order-independent across log writers but sensitive to
// any difference in what actually happened. Two of the four runs use more than
// one processor, because simulated time must not depend on how much host
// parallelism is available.
//
// Skipped by default: it builds and runs two binaries and takes a couple of
// minutes. Run it with MULTIVENUE_DETERMINISM=1.
func TestRunsOfOneSeedProduceOneEventStream(t *testing.T) {
	if os.Getenv("MULTIVENUE_DETERMINISM") == "" {
		t.Skip("set MULTIVENUE_DETERMINISM=1 to run the cross-process determinism check")
	}
	repo := repoRoot(t)
	config := filepath.Join(repo, "research/configs/frozen-baseline-2026-08-21.json")
	if _, err := os.Stat(config); err != nil {
		t.Skipf("frozen configuration not present: %v", err)
	}

	dir := t.TempDir()
	sim := filepath.Join(dir, "multivenue")
	analyze := filepath.Join(dir, "mvanalyze")
	build(t, repo, sim, "./cmd/multivenue")
	build(t, repo, analyze, "./cmd/mvanalyze")

	cases := []struct {
		name  string
		procs string
	}{
		{"single-core-a", "1"},
		{"single-core-b", "1"},
		{"multi-core-a", "4"},
		{"multi-core-b", "4"},
	}
	digests := make(map[string]string, len(cases))
	for _, testCase := range cases {
		logDir := filepath.Join(dir, testCase.name)
		run := exec.Command(sim, "-config", config, "-seed", "101", "-duration", "5m", "-logdir", logDir)
		run.Dir = repo
		run.Env = append(os.Environ(), "GOMAXPROCS="+testCase.procs)
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("%s: run failed: %v\n%s", testCase.name, err, out)
		}
		hash := exec.Command(analyze, "-metric", "streamhash", "-json", logDir)
		hash.Dir = repo
		out, err := hash.Output()
		if err != nil {
			t.Fatalf("%s: digest failed: %v", testCase.name, err)
		}
		var payload struct {
			Result struct {
				Events int64  `json:"events"`
				Digest string `json:"digest"`
			} `json:"result"`
		}
		if err := json.Unmarshal(out, &payload); err != nil {
			t.Fatalf("%s: decode digest: %v", testCase.name, err)
		}
		// The run directory differs per case and is not part of the model, so
		// only the digest and the event count are compared.
		digests[testCase.name] = fmt.Sprintf("%d %s", payload.Result.Events, payload.Result.Digest)
	}

	reference := digests[cases[0].name]
	for _, testCase := range cases[1:] {
		if digests[testCase.name] != reference {
			t.Errorf("%s produced a different event stream from %s;\n  %s\n  %s",
				testCase.name, cases[0].name, reference, digests[testCase.name])
		}
	}
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
