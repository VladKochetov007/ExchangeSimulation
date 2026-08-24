package multivenue

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exchange_sim/analysis"
)

type perpExposureEvidenceProcessResult struct {
	ExecutionHash string `json:"execution_hash"`
	Decisions     int64  `json:"decisions"`
	Receipts      int64  `json:"receipts"`
}

// TestPerpExposureEvidenceProcessHelper executes in a separate process. The
// parent varies only recorder/sidecar presence and host GOMAXPROCS; actor
// config, link, timer, account, and log topology are fixed in every child.
func TestPerpExposureEvidenceProcessHelper(t *testing.T) {
	if os.Getenv("P2_EVIDENCE_PROCESS_HELPER") != "1" {
		return
	}
	dir := os.Getenv("P2_EVIDENCE_PROCESS_OUTPUT")
	if dir == "" {
		t.Fatal("missing P2_EVIDENCE_PROCESS_OUTPUT")
	}
	evidence := os.Getenv("P2_EVIDENCE_PROCESS_ON") == "1"
	policy := PerpExposureHedgerConfig{
		Enabled: true, Symbol: "ABC-PERP", DecisionInterval: 2 * time.Second, ExposureInterval: 10 * time.Second,
		ExposureStepQty: 10_000_000, MaxAbsExposure: 100_000_000, MaxRequestQty: 10_000_000, TickSize: 1_000_000,
		InitialQuoteBalance: 200_000_000 * mvQuotePrecision, InitialMargin: 100_000_000 * mvQuotePrecision,
	}
	cfg := Config{
		LogDir: dir, LogMode: "full", Seed: 101, TakerFeeBps: 5, StrictPopulationAccounting: true,
		CheckpointIntervalSeconds: 1,
		LatencyProfiles:           map[string]LatencyProfile{"perp_exposure_hedger": {Model: "constant", Delay: 40 * time.Millisecond}},
		PerpExposureHedger:        &policy,
	}
	if evidence {
		cfg.RecordMarketDataReceipts = true
		cfg.MarketDataReceiptRoles = []string{"perp_exposure_hedger"}
		cfg.RecordPerpExposureHedgerDecisions = true
	}
	sim, err := NewSim(16*time.Second, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		sim.Close()
		t.Fatal(err)
	}
	report, err := json.Marshal(struct {
		InitialAccounts  []ParticipantAccountSnapshot `json:"initial_accounts"`
		TerminalAccounts []ParticipantAccountSnapshot `json:"terminal_accounts"`
		VenueLedgers     []VenueLedger                `json:"venue_ledgers"`
	}{sim.InitialAccounts, sim.TerminalAccounts, sim.CaptureVenueLedgers()})
	if err != nil {
		sim.Close()
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), report, 0o644); err != nil {
		sim.Close()
		t.Fatal(err)
	}
	sim.Close()
	result := perpExposureEvidenceProcessResult{ExecutionHash: finalExecutionHash(t, dir)}
	if evidence {
		run, err := analysis.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		audit, err := run.MeasurePerpExposureHedger()
		if err != nil || !audit.Valid {
			t.Fatalf("invalid P2 evidence replay: audit=%+v err=%v", audit, err)
		}
		result.Decisions = audit.Decisions
		result.Receipts = audit.ReceiptMatches
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(encoded))
}

func TestPerpExposureEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]perpExposureEvidenceProcessResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runPerpExposureEvidenceProcess(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("P2 evidence/process setting changed execution: want %s, %s=%+v", want, key, result)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.Decisions == 0 || left.Receipts == 0 || left.Decisions != right.Decisions || left.Receipts != right.Receipts {
		t.Fatalf("P2 evidence is not deterministic across fresh processes: g1=%+v g4=%+v", left, right)
	}
}

func runPerpExposureEvidenceProcess(t *testing.T, gomax string, evidence bool) perpExposureEvidenceProcessResult {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestPerpExposureEvidenceProcessHelper", "--")
	cmd.Env = append(os.Environ(), "P2_EVIDENCE_PROCESS_HELPER=1", "P2_EVIDENCE_PROCESS_OUTPUT="+dir, "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "P2_EVIDENCE_PROCESS_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("P2 evidence helper gomax=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
	}
	var encoded string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "{") {
			encoded = line
			break
		}
	}
	var result perpExposureEvidenceProcessResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode P2 evidence helper output %q: %v", raw, err)
	}
	return result
}

func finalExecutionHash(t *testing.T, dir string) string {
	t.Helper()
	file, err := os.Open(filepath.Join(dir, "checkpoints.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var hash string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var checkpoint struct {
			ExecutionHash string `json:"execution_stream_hash"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &checkpoint); err != nil {
			t.Fatal(err)
		}
		hash = checkpoint.ExecutionHash
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("missing terminal execution checkpoint")
	}
	return hash
}
