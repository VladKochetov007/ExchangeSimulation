package multivenue

import (
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

type v20HelperResult struct {
	ExecutionHash            string `json:"execution_hash"`
	Schedules                int64  `json:"schedules"`
	Receipts                 int64  `json:"receipts"`
	Decisions                int64  `json:"decisions"`
	ScheduleDigest           string `json:"schedule_digest"`
	ReceiptDigest            string `json:"receipt_digest"`
	DecisionDigest           string `json:"decision_digest"`
	FrontierVectorDecisions  int64  `json:"frontier_vector_decisions"`
	FrontierVectorComponents int64  `json:"frontier_vector_components"`
	FrontierVectorDigest     string `json:"frontier_vector_digest"`
	FrontierComponentDigest  string `json:"frontier_component_digest"`
}

// TestV20EvidenceHelper deliberately runs in a fresh test process. Parent test
// invokes it with different GOMAXPROCS and telemetry modes; direct calls are a
// no-op so this helper cannot accidentally become part of normal test state.
func TestV20EvidenceHelper(t *testing.T) {
	if os.Getenv("V20_EVIDENCE_HELPER") != "1" {
		return
	}
	output := os.Getenv("V20_EVIDENCE_OUTPUT")
	if output == "" {
		t.Fatal("missing V20_EVIDENCE_OUTPUT")
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "research", "configs", "frozen-baseline-2026-08-22.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = output
	cfg.LogMode = "none"
	cfg.Seed = 101
	cfg.CheckpointIntervalSeconds = 60
	remoteFeed := os.Getenv("V21_REMOTE_FEED") == "1"
	if os.Getenv("V21_LOCAL_CACHE") == "1" || remoteFeed {
		cfg.MakerAnchor = "own_mid"
		cfg.SpotMakerLocalReferenceCache = true
		cfg.LatencyProfiles = map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: 10 * time.Millisecond},
		}
	}
	cfg.RecordMarketDataReceipts = os.Getenv("V20_EVIDENCE_ON") == "1"
	if cfg.RecordMarketDataReceipts {
		cfg.MarketDataReceiptRoles = []string{"spot_maker"}
		if remoteFeed {
			cfg.MarketDataReceiptRoles = append(cfg.MarketDataReceiptRoles, "v2_remote_feed")
			cfg.RecordDecisionFrontierVectors = true
		}
	}
	if remoteFeed {
		cfg.RemoteMakerFeed = &RemoteMakerFeedConfig{
			TargetVenue: "north", SourceVenue: "south", Symbol: "ABC/USD", Weight: 0.5,
			Latency: LatencyProfile{Model: "constant", Delay: 20 * time.Millisecond},
		}
	}
	sim, err := NewSim(2*time.Minute, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		sim.Close()
		t.Fatal(err)
	}
	sim.Close()
	checkpointRaw, err := os.ReadFile(filepath.Join(output, "checkpoints.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(checkpointRaw)), "\n")
	var checkpoint struct {
		ExecutionHash string `json:"execution_stream_hash"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &checkpoint); err != nil {
		t.Fatal(err)
	}
	result := v20HelperResult{ExecutionHash: checkpoint.ExecutionHash}
	if cfg.RecordMarketDataReceipts {
		audit, err := analysis.AuditMarketDataReceipts(output)
		if err != nil || !audit.Valid || audit.Decisions == 0 {
			t.Fatalf("invalid V2-0 evidence: audit=%+v err=%v", audit, err)
		}
		result.Schedules, result.Receipts, result.Decisions = audit.Schedules, audit.Receipts, audit.Decisions
		var manifest struct {
			Schedules struct {
				Digest string `json:"digest"`
			} `json:"schedules"`
			Receipts struct {
				Digest string `json:"digest"`
			} `json:"receipts"`
			Decisions struct {
				Digest string `json:"digest"`
			} `json:"decisions"`
		}
		manifestRaw, err := os.ReadFile(filepath.Join(output, "market-data-evidence-v2.json"))
		if err != nil || json.Unmarshal(manifestRaw, &manifest) != nil {
			t.Fatalf("read V2-0 evidence manifest: %v", err)
		}
		result.ScheduleDigest, result.ReceiptDigest, result.DecisionDigest = manifest.Schedules.Digest, manifest.Receipts.Digest, manifest.Decisions.Digest
		if cfg.RecordDecisionFrontierVectors {
			vectors, err := analysis.AuditDecisionFrontierVectors(output)
			if err != nil || !vectors.Valid || vectors.Decisions == 0 || vectors.Components != 2*vectors.Decisions {
				t.Fatalf("invalid V2-1 vector evidence: audit=%+v err=%v", vectors, err)
			}
			var vectorManifest struct {
				Decisions struct {
					Digest string `json:"digest"`
				} `json:"decisions"`
				Components struct {
					Digest string `json:"digest"`
				} `json:"components"`
			}
			vectorManifestRaw, err := os.ReadFile(filepath.Join(output, "market-data-frontier-vectors-v1.json"))
			if err != nil || json.Unmarshal(vectorManifestRaw, &vectorManifest) != nil {
				t.Fatalf("read V2-1 vector manifest: %v", err)
			}
			result.FrontierVectorDecisions, result.FrontierVectorComponents = vectors.Decisions, vectors.Components
			result.FrontierVectorDigest, result.FrontierComponentDigest = vectorManifest.Decisions.Digest, vectorManifest.Components.Digest
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(encoded))
}

func TestV20EvidenceDoesNotChangeExecutionAcrossFreshProcesses(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV20Helper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("execution hash changes with evidence/process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("evidence sidecar is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// V2-1a reuses the V2-0 delayed gateway but changes a maker's input path.
// Therefore its own ON/OFF and GOMAXPROCS check is separate from the V2-0
// instrumentation proof: evidence must remain observational in this new
// cache-using world as well.
func TestV21SingleFeedCacheIsFreshProcessDeterministic(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runEvidenceHelper(t, gomax, evidence, true)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("V2-1 cache changes execution with evidence/process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("V2-1 local cache evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// This repeats V2-0's evidence-on/off proof for the first genuinely
// multi-venue local-information path. The off variants are technical controls
// only; production evidence configurations must retain both V2 sidecars.
func TestV21RemoteFeedIsFreshProcessDeterministicAndEvidenceNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV2EvidenceHelper(t, gomax, evidence, true, true)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("V2-1 remote feed changes execution with evidence/process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 || left.FrontierVectorDecisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest ||
		left.FrontierVectorDecisions != right.FrontierVectorDecisions || left.FrontierVectorComponents != right.FrontierVectorComponents ||
		left.FrontierVectorDigest != right.FrontierVectorDigest || left.FrontierComponentDigest != right.FrontierComponentDigest {
		t.Fatalf("V2-1 remote feed evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

func TestV20EvidenceRejectsCustomMountCoverageGap(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "research", "configs", "frozen-baseline-2026-08-22.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LogDir = t.TempDir()
	cfg.LogMode = "none"
	cfg.RecordMarketDataReceipts = true
	cfg.MarketDataReceiptRoles = []string{"cross_venue_router_tier"}
	cfg.CrossVenueArbTiers = []float64{1}
	cfg.CrossVenueBaseLatency = time.Second
	_, err = NewSim(time.Minute, cfg)
	if err == nil || !strings.Contains(err.Error(), "instrumented links") {
		t.Fatalf("custom uninstrumented router mount passed V2-0 evidence coverage: %v", err)
	}
}

func runV20Helper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	return runV2EvidenceHelper(t, gomax, evidence, false, false)
}

func runEvidenceHelper(t *testing.T, gomax string, evidence, localCache bool) v20HelperResult {
	return runV2EvidenceHelper(t, gomax, evidence, localCache, false)
}

func runV2EvidenceHelper(t *testing.T, gomax string, evidence, localCache, remoteFeed bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	if localCache {
		cmd.Env = append(cmd.Env, "V21_LOCAL_CACHE=1")
	}
	if remoteFeed {
		cmd.Env = append(cmd.Env, "V21_REMOTE_FEED=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("V2-0 helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
	}
	var result v20HelperResult
	var encoded string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "{") {
			encoded = line
			break
		}
	}
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode V2-0 helper output %q: %v", raw, err)
	}
	return result
}
