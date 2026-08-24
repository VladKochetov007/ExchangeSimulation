package multivenue

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/analysis"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
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
	MakerQuoteSizeDecisions  int64  `json:"maker_quote_size_decisions"`
	MakerRebalanceDecisions  int64  `json:"maker_rebalance_decisions"`
	LiabilityHedgerDecisions int64  `json:"liability_hedger_decisions"`
	LiabilityHedgerPhase     int64  `json:"liability_hedger_phase_nanos"`
	LiabilityHedgerPhaseSet  bool   `json:"liability_hedger_phase_configured"`
	NoiseFlowPhaseDecisions  int64  `json:"noise_flow_phase_decisions"`
	NoiseFlowPhase           int64  `json:"noise_flow_phase_nanos"`
	NoiseFlowPhaseSet        bool   `json:"noise_flow_phase_configured"`
	FundingCarryDecisions    int64  `json:"funding_carry_decisions"`
	TermCarryDecisions       int64  `json:"term_carry_decisions"`
	EvidenceArtifactEvents   int64  `json:"evidence_artifact_events"`
	EvidenceArtifactDigest   string `json:"evidence_artifact_digest"`
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
	remoteRoster := os.Getenv("V21_REMOTE_ROSTER") == "1"
	routerEvidence := os.Getenv("V22_ROUTER") == "1"
	p1QuoteSizeEvidence := os.Getenv("V23_P1_QUOTE_SIZE") == "1"
	p2RebalanceEvidence := os.Getenv("V23_P2_REBALANCE") == "1"
	l0LiabilityHedgerEvidence := os.Getenv("V24_L0_LIABILITY_HEDGER") == "1"
	l1RandomSideControlEvidence := os.Getenv("V24_L1_RANDOM_SIDE_CONTROL") == "1"
	l1PhaseControlEvidence := os.Getenv("V24_L1_PHASE_CONTROL") == "1"
	l1PhaseOffsetEvidence := os.Getenv("V24_L1_PHASE_OFFSET") == "1"
	l1ExplicitZeroPhaseEvidence := os.Getenv("V24_L1_EXPLICIT_ZERO_PHASE") == "1"
	l1P2NoisePhaseEvidence := os.Getenv("V24_L1P2_NOISE_PHASE") != ""
	fundingCarryEvidence := os.Getenv("V25_FUNDING_CARRY") == "1"
	termCarryEvidence := os.Getenv("V25_TERM_CARRY") == "1"
	liabilityHedgerEvidence := l0LiabilityHedgerEvidence || l1RandomSideControlEvidence || l1PhaseControlEvidence || l1PhaseOffsetEvidence || l1ExplicitZeroPhaseEvidence || l1P2NoisePhaseEvidence
	if p1QuoteSizeEvidence {
		// P1 varies only optional raw decision recording. Both sides retain full
		// logs so an identical logger topology cannot mask a recorder effect.
		cfg.LogMode = "full"
		cfg.CrossAssetSpotGraph = true
		cfg.SpotPassiveMakerPostOnly = true
		cfg.SpotPassiveMakerCancelBeforeReplace = true
		cfg.SpotStoikovInventorySizeSkewBps = 5_000
		cfg.RecordMakerQuoteSizeDecisions = os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if p2RebalanceEvidence {
		// Hold P0-C, P1-B, fee topology, and the policy parameters fixed. The
		// child varies only evidence sidecars/recorders, never an economic input.
		cfg.LogMode = "full"
		cfg.CrossAssetSpotGraph = true
		cfg.SpotPassiveMakerPostOnly = true
		cfg.SpotPassiveMakerCancelBeforeReplace = true
		cfg.SpotStoikovInventorySizeSkewBps = 5_000
		cfg.CDFInventoryRebalance = &InventoryRebalanceConfig{
			Enabled: true, Interval: 10 * time.Second, Cooldown: 30 * time.Second,
			RiskBandQty: 10_000_000_000, TargetBandQty: 5_000_000_000, MaxRequestQty: 500_000_000,
			ParticipationBps: 1_000, SlippageBps: 50,
		}
		cfg.RecordMakerInventoryRebalanceDecisions = p2RebalanceEvidence && os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if liabilityHedgerEvidence {
		cfg.LogMode = "full"
		cfg.CrossAssetSpotGraph = true
		// The independent L0/L1/L1-P replay joins records to the preserved
		// participant roster. This is report-only accounting capture, held fixed
		// across evidence ON/OFF; it is not an economic input.
		cfg.StrictPopulationAccounting = true
		cfg.CDFLiabilityHedger = &LiabilityHedgerConfig{
			Enabled: true, Symbol: "CDF/USD", DecisionInterval: 2 * time.Second,
			ObligationInterval: 10 * time.Second, ObligationStepQty: 200_000_000,
			MaxAbsObligationQty: 2_000_000_000, MaxRequestQty: 100_000_000,
		}
		if l1RandomSideControlEvidence {
			cfg.CDFLiabilityHedger.PolicyMode = LiabilityHedgerPolicyRandomSideControl
		}
		if l1PhaseControlEvidence || l1PhaseOffsetEvidence || l1ExplicitZeroPhaseEvidence {
			cfg.CDFLiabilityHedger.PolicyMode = LiabilityHedgerPolicyDeliveryLiability
		}
		if l1PhaseOffsetEvidence {
			cfg.CDFLiabilityHedger.DecisionPhaseOffset = time.Second
		}
		if l1ExplicitZeroPhaseEvidence {
			cfg.CDFLiabilityHedger.DecisionPhaseOffset = 0
		}
		cfg.RecordLiabilityHedgerDecisions = os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if l1P2NoisePhaseEvidence {
		switch os.Getenv("V24_L1P2_NOISE_PHASE") {
		case "absent":
			// Preserve the source-config omission path: zero is the Go default,
			// but no test helper assignment is made.
		case "zero":
			cfg.NoiseFlowDecisionPhaseOffset = 0
		case "one_second":
			cfg.NoiseFlowDecisionPhaseOffset = time.Second
		default:
			t.Fatalf("unknown V24_L1P2_NOISE_PHASE=%q", os.Getenv("V24_L1P2_NOISE_PHASE"))
		}
		cfg.RecordNoiseFlowPhaseDecisions = os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if fundingCarryEvidence {
		// ON and OFF run the identical actor, population, and delayed public
		// feed. Only append-only evidence sinks differ between the children.
		cfg.LogMode = "full"
		cfg.StrictPopulationAccounting = true
		cfg.FundingCarryArbitrageur = &FundingCarryArbitrageurConfig{
			Enabled: true, SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", DecisionPeriod: 2 * time.Second,
			FundingHorizon: 1, MaxFundingAge: 10 * time.Second,
			TakerFeeBps: 0, BorrowAnnualBps: 0, BalanceSheetBps: 0, MarginRiskBps: 0, LegRiskBps: 0, MinNetCarryBps: 0,
			MaxPosition: 100_000_000, LotQty: 10_000_000, MinOrderSize: 100_000, SpotTick: 1_000_000, PerpTick: 1_000_000,
		}
		cfg.LatencyProfiles = map[string]LatencyProfile{"funding_carry_arb": {Model: "constant", Delay: 10 * time.Millisecond}}
		cfg.RecordFundingCarryDecisions = os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if termCarryEvidence {
		// P3 holds the identical actor, delayed feed, and population across ON
		// and OFF. Its explicit one-minute treasury mandate is shorter than the
		// eight-hour minimum term, exercising declared-policy censoring rather
		// than leaking the hidden simulation stop time into the actor. The
		// explicit zero P3d exit floor is evidence-only here: the censored
		// helper never opens a term, but it exercises v3 decision serialization
		// and replay without treating zero as unavailable.
		cfg.LogMode = "full"
		cfg.StrictPopulationAccounting = true
		unwindMinimum := int64(0)
		cfg.TermCarryAllocator = &TermCarryAllocatorConfig{
			Enabled: true, SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", DecisionPeriod: 2 * time.Second,
			CommitmentIntervals: 1, MaxFundingAge: 10 * time.Second,
			// This is an explicit participant mandate, serialized with the P3
			// helper config; it is not the hidden simulation stop time.
			MandateEndAtNano: time.Date(2025, 1, 1, 0, 1, 0, 0, time.UTC).UnixNano(),
			TakerFeeBps:      cfg.TakerFeeBps, LongSpotFundingBps: 0, ShortSpotBorrowBps: 0,
			BalanceSheetBps: 0, MarginRiskBps: 0, LegRiskBps: 0, MinNetCarryBps: 0,
			MaxPosition: 100_000_000, LotQty: 10_000_000, MinOrderSize: 100_000, UnwindMinOrderSize: &unwindMinimum, SpotTick: 1_000_000, PerpTick: 1_000_000,
		}
		cfg.LatencyProfiles = map[string]LatencyProfile{"term_carry_allocator": {Model: "constant", Delay: 10 * time.Millisecond}}
		cfg.RecordTermCarryDecisions = os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if os.Getenv("V21_LOCAL_CACHE") == "1" || remoteFeed || remoteRoster {
		cfg.MakerAnchor = "own_mid"
		cfg.SpotMakerLocalReferenceCache = true
		cfg.LatencyProfiles = map[string]LatencyProfile{
			"spot_maker": {Model: "constant", Delay: 10 * time.Millisecond},
		}
	}
	cfg.RecordMarketDataReceipts = !p1QuoteSizeEvidence && !p2RebalanceEvidence && !liabilityHedgerEvidence && !fundingCarryEvidence && !termCarryEvidence && os.Getenv("V20_EVIDENCE_ON") == "1"
	if p2RebalanceEvidence || liabilityHedgerEvidence || fundingCarryEvidence || termCarryEvidence {
		cfg.RecordMarketDataReceipts = os.Getenv("V20_EVIDENCE_ON") == "1"
	}
	if cfg.RecordMarketDataReceipts {
		if routerEvidence {
			cfg.MarketDataReceiptRoles = []string{"cross_venue_router_tier"}
			cfg.RecordDecisionFrontierVectors = true
		} else if p2RebalanceEvidence || liabilityHedgerEvidence || fundingCarryEvidence || termCarryEvidence {
			cfg.MarketDataReceiptRoles = nil
			if p2RebalanceEvidence {
				cfg.MarketDataReceiptRoles = append(cfg.MarketDataReceiptRoles, "cdf_spot_maker")
			}
			if liabilityHedgerEvidence {
				cfg.MarketDataReceiptRoles = append(cfg.MarketDataReceiptRoles, "liability_hedger")
			}
			if fundingCarryEvidence {
				cfg.MarketDataReceiptRoles = append(cfg.MarketDataReceiptRoles, "funding_carry_arb")
			}
			if termCarryEvidence {
				cfg.MarketDataReceiptRoles = append(cfg.MarketDataReceiptRoles, "term_carry_allocator")
			}
		} else {
			cfg.MarketDataReceiptRoles = []string{"spot_maker"}
		}
		if remoteFeed || remoteRoster {
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
	if remoteRoster {
		cfg.SpotMakerCount = 2
		cfg.RemoteMakerFeeds = []RemoteMakerFeedConfig{
			{TargetVenue: "north", TargetMaker: 1, SourceVenue: "south", Symbol: "ABC/USD", Weight: 0.50, Confidence: 0.80, MaxObservationAge: 2 * time.Second, Latency: LatencyProfile{Model: "constant", Delay: 10 * time.Millisecond}},
			{TargetVenue: "central", TargetMaker: 1, SourceVenue: "north", Symbol: "ABC/USD", Weight: 0.35, Confidence: 0.90, MaxObservationAge: 4 * time.Second, Latency: LatencyProfile{Model: "constant", Delay: 20 * time.Millisecond}},
			{TargetVenue: "south", TargetMaker: 1, SourceVenue: "central", Symbol: "ABC/USD", Weight: 0.45, Confidence: 0.60, MaxObservationAge: 6 * time.Second, Latency: LatencyProfile{Model: "constant", Delay: 30 * time.Millisecond}},
		}
	}
	if routerEvidence {
		cfg.CrossVenueArbTiers = []float64{1}
		cfg.CrossVenueBaseLatency = time.Second
		cfg.CrossVenueArbLotQty = mvBasePrecision / 100
		cfg.CrossVenueArbMaxAttempts = 1
	}
	if liabilityHedgerEvidence || fundingCarryEvidence || termCarryEvidence {
		if err := writeV24RunConfig(output, cfg); err != nil {
			t.Fatalf("write V2-4 independent-replay config: %v", err)
		}
	}
	sim, err := NewSim(2*time.Minute, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if routerEvidence {
		seedV22RouterActivationBooks(t, sim)
	}
	if err := sim.Run(context.Background()); err != nil {
		sim.Close()
		t.Fatal(err)
	}
	if (liabilityHedgerEvidence && cfg.RecordLiabilityHedgerDecisions) || (fundingCarryEvidence && cfg.RecordFundingCarryDecisions) || (termCarryEvidence && cfg.RecordTermCarryDecisions) {
		if err := writeV24AnalysisReport(output, sim); err != nil {
			sim.Close()
			t.Fatalf("write V2-4 independent-replay report: %v", err)
		}
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
		if err != nil || !audit.Valid || (!fundingCarryEvidence && !termCarryEvidence && audit.Decisions == 0) {
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
			componentsPerDecision := int64(2)
			if routerEvidence {
				componentsPerDecision = 3
			}
			if err != nil || !vectors.Valid || vectors.Decisions == 0 || vectors.Components != componentsPerDecision*vectors.Decisions {
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
	if p1QuoteSizeEvidence && cfg.RecordMakerQuoteSizeDecisions {
		result.MakerQuoteSizeDecisions = countRawEvent(t, output, "maker_quote_size_decision")
		if result.MakerQuoteSizeDecisions == 0 {
			t.Fatal("P1 recorder emitted no quote-size decisions")
		}
		artifactRaw, err := os.ReadFile(filepath.Join(output, "evidence-artifact-hash.json"))
		if err != nil {
			t.Fatalf("read P1 evidence artifact digest: %v", err)
		}
		var artifact struct {
			Events int64  `json:"events"`
			Digest string `json:"digest"`
		}
		if err := json.Unmarshal(artifactRaw, &artifact); err != nil || artifact.Events == 0 || artifact.Digest == "" {
			t.Fatalf("decode P1 evidence artifact digest: artifact=%+v err=%v", artifact, err)
		}
		result.EvidenceArtifactEvents = artifact.Events
		result.EvidenceArtifactDigest = artifact.Digest
	}
	if p2RebalanceEvidence && cfg.RecordMakerInventoryRebalanceDecisions {
		result.MakerRebalanceDecisions = countRawEvent(t, output, "maker_inventory_rebalance_decision")
		if result.MakerRebalanceDecisions == 0 {
			t.Fatal("P2 recorder emitted no inventory-rebalance decisions")
		}
	}
	if liabilityHedgerEvidence && cfg.RecordLiabilityHedgerDecisions {
		result.LiabilityHedgerDecisions = countRawEvent(t, output, "liability_hedger_decision")
		if result.LiabilityHedgerDecisions == 0 {
			t.Fatal("V2-4 recorder emitted no liability-hedger decisions")
		}
		run, err := analysis.Open(output)
		if err != nil {
			t.Fatalf("open V2-4 evidence for independent replay: %v", err)
		}
		audit, err := run.MeasureLiabilityHedger()
		if err != nil || !audit.Valid {
			t.Fatalf("invalid V2-4 liability evidence: audit=%+v err=%v", audit, err)
		}
		result.LiabilityHedgerPhase = audit.DecisionPhaseOffsetNanos
		result.LiabilityHedgerPhaseSet = audit.PhaseConfigured
	}
	if l1P2NoisePhaseEvidence && cfg.RecordNoiseFlowPhaseDecisions {
		result.NoiseFlowPhaseDecisions = countRawEvent(t, output, "noise_flow_phase_decision")
		if result.NoiseFlowPhaseDecisions == 0 {
			t.Fatal("L1-P2 recorder emitted no noise-flow phase decisions")
		}
		run, err := analysis.Open(output)
		if err != nil {
			t.Fatalf("open L1-P2 evidence for independent replay: %v", err)
		}
		audit, err := run.MeasureNoiseFlowPhase()
		if err != nil || !audit.Valid {
			t.Fatalf("invalid L1-P2 noise-flow timing evidence: audit=%+v err=%v", audit, err)
		}
		result.NoiseFlowPhase = audit.DecisionPhaseOffsetNanos
		result.NoiseFlowPhaseSet = true
	}
	if fundingCarryEvidence && cfg.RecordFundingCarryDecisions {
		result.FundingCarryDecisions = countRawEvent(t, output, "funding_carry_decision")
		if result.FundingCarryDecisions == 0 {
			t.Fatal("V2-5 P0 recorder emitted no funding-carry decisions")
		}
		run, err := analysis.Open(output)
		if err != nil {
			t.Fatalf("open V2-5 P0 evidence for independent replay: %v", err)
		}
		audit, err := run.MeasureFundingCarry()
		if err != nil || !audit.Valid {
			t.Fatalf("invalid V2-5 P0 funding evidence: audit=%+v err=%v", audit, err)
		}
		if audit.Decisions != result.FundingCarryDecisions {
			t.Fatalf("V2-5 P0 raw/replay decision count mismatch: raw=%d replay=%d", result.FundingCarryDecisions, audit.Decisions)
		}
	}
	if termCarryEvidence && cfg.RecordTermCarryDecisions {
		result.TermCarryDecisions = countRawEvent(t, output, "term_carry_decision")
		if result.TermCarryDecisions == 0 {
			t.Fatal("V2-5 P3 recorder emitted no term-carry decisions")
		}
		run, err := analysis.Open(output)
		if err != nil {
			t.Fatalf("open V2-5 P3 evidence for independent replay: %v", err)
		}
		audit, err := run.MeasureTermCarry()
		if err != nil || !audit.Valid {
			t.Fatalf("invalid V2-5 P3 term-carry evidence: audit=%+v err=%v", audit, err)
		}
		if audit.Decisions != result.TermCarryDecisions {
			t.Fatalf("V2-5 P3 raw/replay decision count mismatch: raw=%d replay=%d", result.TermCarryDecisions, audit.Decisions)
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(encoded))
}

func countRawEvent(t *testing.T, dir, name string) int64 {
	t.Helper()
	var count int64
	err := filepath.WalkDir(filepath.Join(dir, "venues"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			var envelope struct {
				Event string `json:"event"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
				return fmt.Errorf("decode persisted event %s: %w", path, err)
			}
			if envelope.Event == name {
				count++
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("count %s evidence: %v", name, err)
	}
	return count
}

// writeV24AnalysisReport is a test-only adapter for the public multivenue
// command's report boundary. Sim.Run deliberately owns raw evidence while the
// command owns greeks.json; producing this compact equivalent lets the
// independent analyzer join raw V2-4 evidence to the captured role roster.
func writeV24AnalysisReport(dir string, sim *Sim) error {
	report := struct {
		InitialAccounts  []ParticipantAccountSnapshot `json:"initial_accounts"`
		TerminalAccounts []ParticipantAccountSnapshot `json:"terminal_accounts"`
		VenueLedgers     []VenueLedger                `json:"venue_ledgers"`
	}{
		InitialAccounts:  sim.InitialAccounts,
		TerminalAccounts: sim.TerminalAccounts,
		VenueLedgers:     sim.CaptureVenueLedgers(),
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "greeks.json"), raw, 0o644)
}

func writeV24RunConfig(dir string, cfg Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "run-config.json"), raw, 0o644)
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
			results[key] = runV2EvidenceHelper(t, gomax, evidence, true, true, false, false)
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

func TestV21HeterogeneousRosterIsFreshProcessDeterministicAndEvidenceNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV2EvidenceHelper(t, gomax, evidence, true, false, true, false)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("V2-1 heterogeneous roster changes execution with evidence/process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 || left.FrontierVectorDecisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest ||
		left.FrontierVectorDecisions != right.FrontierVectorDecisions || left.FrontierVectorComponents != right.FrontierVectorComponents ||
		left.FrontierVectorDigest != right.FrontierVectorDigest || left.FrontierComponentDigest != right.FrontierComponentDigest {
		t.Fatalf("V2-1 heterogeneous roster evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

func TestV22RouterIsFreshProcessDeterministicAndEvidenceNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV22RouterEvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("V2-2 router changes execution with evidence/process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 || left.FrontierVectorDecisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest ||
		left.FrontierVectorDecisions != right.FrontierVectorDecisions || left.FrontierVectorComponents != right.FrontierVectorComponents ||
		left.FrontierVectorComponents != 3*left.FrontierVectorDecisions ||
		left.FrontierVectorDigest != right.FrontierVectorDigest || left.FrontierComponentDigest != right.FrontierComponentDigest {
		t.Fatalf("V2-2 router evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// P1's recorder is optional persisted evidence rather than an execution
// observation. This fresh-process check holds the P0-C policy and full logger
// topology fixed while varying only the recorder, across host parallelism.
func TestV23P1QuoteSizeEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV23P1EvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("P1 recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.MakerQuoteSizeDecisions == 0 || left.MakerQuoteSizeDecisions != right.MakerQuoteSizeDecisions ||
		left.EvidenceArtifactEvents == 0 || left.EvidenceArtifactEvents != right.EvidenceArtifactEvents ||
		left.EvidenceArtifactDigest == "" || left.EvidenceArtifactDigest != right.EvidenceArtifactDigest {
		t.Fatalf("P1 evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// P2's local action must not read receipt telemetry. This fresh-process
// control holds the economic policy—including the CDF maker fee model—fixed
// while turning only its evidence sidecars and raw-decision recorder on/off.
func TestV23P2RebalanceEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV23P2EvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("P2 recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.MakerRebalanceDecisions == 0 || left.MakerRebalanceDecisions != right.MakerRebalanceDecisions ||
		left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("P2 evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// L0 adds an actor and its deterministic liability stream in both variants;
// this test varies only append-only decision/fill and receipt evidence. It
// protects against a recorder affecting timer order, RNG consumption, or the
// actor-visible public-feed frontier.
func TestV24L0LiabilityHedgerEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV24L0EvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("L0 recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.LiabilityHedgerDecisions == 0 || left.LiabilityHedgerDecisions != right.LiabilityHedgerDecisions ||
		left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("L0 evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// P0 varies only its raw decision/outcome and V2-0 receipt sidecars. The
// funding-carry actor remains present in every child, so the execution hash
// proves the evidence repair cannot change its information, timing, or orders.
func TestV25FundingCarryEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV25FundingCarryEvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("V2-5 P0 recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.FundingCarryDecisions == 0 || left.FundingCarryDecisions != right.FundingCarryDecisions ||
		left.Schedules == 0 || left.Receipts == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("V2-5 P0 evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// P3 uses the same delayed public-feed evidence boundary as P0, but a finite
// term allocator. The helper horizon is intentionally shorter than one
// funding interval: it must record a terminal-censored decision without
// manufacturing an order merely to exercise telemetry. This matrix proves
// those append-only records neither alter execution nor become host-parallel
// dependent before a longer lifecycle run is admitted.
func TestV25TermCarryEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV25TermCarryEvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("V2-5 P3 recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.TermCarryDecisions == 0 || left.TermCarryDecisions != right.TermCarryDecisions ||
		left.Schedules == 0 || left.Receipts == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("V2-5 P3 evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// L1 changes the actor's declared economic side-selection policy, but its
// decision/fill recorder and V2 receipt sidecars must still be append-only.
// This fresh-process matrix therefore exercises the random-side control rather
// than assuming L0's delivery-liability neutrality covers a new RNG stream.
func TestV24L1RandomSideControlEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV24L1EvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("L1 random-control recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.LiabilityHedgerDecisions == 0 || left.LiabilityHedgerDecisions != right.LiabilityHedgerDecisions ||
		left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("L1 random-control evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// L1-P varies only the first liability-hedger decision time. The fresh-process
// matrix proves its additional sidecars remain observational, while the
// independent replay verifies every row carries the configured phase and is
// aligned to the fixed simulation epoch.
func TestV24L1PhaseOffsetEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV24L1PhaseEvidenceHelper(t, gomax, evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("L1-P recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.LiabilityHedgerDecisions == 0 || left.LiabilityHedgerDecisions != right.LiabilityHedgerDecisions ||
		!left.LiabilityHedgerPhaseSet || !right.LiabilityHedgerPhaseSet ||
		left.LiabilityHedgerPhase != int64(time.Second) || left.LiabilityHedgerPhase != right.LiabilityHedgerPhase ||
		left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("L1-P phase evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// Explicit zero is a V2 evidence-schema requirement, but not a scheduling
// change. Compare it with the otherwise identical L1 delivery parent whose
// config predates the field, across fresh processes and GOMAXPROCS values.
func TestV24L1PExplicitZeroPhaseMatchesLegacySchedule(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		results["legacy/g"+gomax] = runV24L1PhaseControlEvidenceHelper(t, gomax, false, false)
		results["zero-off/g"+gomax] = runV24L1PhaseControlEvidenceHelper(t, gomax, true, false)
		results["zero-on/g"+gomax] = runV24L1PhaseControlEvidenceHelper(t, gomax, true, true)
	}
	want := results["legacy/g1"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("explicit zero phase changed legacy execution: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	for _, gomax := range []string{"1", "4"} {
		zero := results["zero-on/g"+gomax]
		if zero.LiabilityHedgerDecisions == 0 || !zero.LiabilityHedgerPhaseSet || zero.LiabilityHedgerPhase != 0 ||
			zero.Schedules == 0 || zero.Receipts == 0 || zero.Decisions == 0 {
			t.Fatalf("explicit zero phase missing required evidence at GOMAXPROCS=%s: %+v", gomax, zero)
		}
	}
	left, right := results["zero-on/g1"], results["zero-on/g4"]
	if left.LiabilityHedgerDecisions != right.LiabilityHedgerDecisions ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("explicit zero phase evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// L1-P2's recorder must not become a hidden causal participant. A one-second
// broad-noise phase changes only the configured first noise tick; evidence
// ON/OFF and fresh process parallelism must leave the execution hash intact.
func TestV24L1P2NoisePhaseEvidenceIsFreshProcessDeterministicAndNeutral(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		for _, evidence := range []bool{false, true} {
			key := "g" + gomax + "/off"
			if evidence {
				key = "g" + gomax + "/on"
			}
			results[key] = runV24L1P2NoisePhaseEvidenceHelper(t, gomax, "one_second", evidence)
		}
	}
	want := results["g1/off"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("L1-P2 recorder changed execution with process setting: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["g1/on"], results["g4/on"]
	if left.NoiseFlowPhaseDecisions == 0 || left.NoiseFlowPhaseDecisions != right.NoiseFlowPhaseDecisions ||
		!left.NoiseFlowPhaseSet || !right.NoiseFlowPhaseSet || left.NoiseFlowPhase != int64(time.Second) || left.NoiseFlowPhase != right.NoiseFlowPhase ||
		left.LiabilityHedgerDecisions == 0 || left.Schedules == 0 || left.Receipts == 0 || left.Decisions == 0 ||
		left.Schedules != right.Schedules || left.Receipts != right.Receipts || left.Decisions != right.Decisions ||
		left.ScheduleDigest != right.ScheduleDigest || left.ReceiptDigest != right.ReceiptDigest || left.DecisionDigest != right.DecisionDigest {
		t.Fatalf("L1-P2 noise phase evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

// A zero phase is an explicit schema value but must preserve RandomTaker's
// legacy ticker representation. The absent source-config path and explicit
// zero configuration are compared across fresh processes and GOMAX settings.
func TestV24L1P2ExplicitZeroNoisePhaseMatchesLegacySchedule(t *testing.T) {
	results := make(map[string]v20HelperResult)
	for _, gomax := range []string{"1", "4"} {
		results["absent/g"+gomax] = runV24L1P2NoisePhaseEvidenceHelper(t, gomax, "absent", false)
		results["zero-off/g"+gomax] = runV24L1P2NoisePhaseEvidenceHelper(t, gomax, "zero", false)
		results["zero-on/g"+gomax] = runV24L1P2NoisePhaseEvidenceHelper(t, gomax, "zero", true)
	}
	want := results["absent/g1"].ExecutionHash
	for key, result := range results {
		if result.ExecutionHash == "" || result.ExecutionHash != want {
			t.Fatalf("explicit zero noise phase changed legacy execution: want %s, %s=%s", want, key, result.ExecutionHash)
		}
	}
	left, right := results["zero-on/g1"], results["zero-on/g4"]
	if left.NoiseFlowPhaseDecisions == 0 || left.NoiseFlowPhaseDecisions != right.NoiseFlowPhaseDecisions ||
		!left.NoiseFlowPhaseSet || !right.NoiseFlowPhaseSet || left.NoiseFlowPhase != 0 || left.NoiseFlowPhase != right.NoiseFlowPhase {
		t.Fatalf("explicit zero noise phase evidence is not fresh-process/GOMAX deterministic: g1=%+v g4=%+v", left, right)
	}
}

func TestV22InstrumentedRouterRegistersEveryCustomLeg(t *testing.T) {
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
	cfg.RecordDecisionFrontierVectors = true
	cfg.CrossVenueArbTiers = []float64{1}
	cfg.CrossVenueBaseLatency = time.Second
	cfg.CrossVenueArbLotQty = mvBasePrecision / 100
	cfg.CrossVenueArbMaxAttempts = 1
	sim, err := NewSim(10*time.Second, cfg)
	if err != nil {
		t.Fatalf("instrumented router failed V2-0 coverage: %v", err)
	}
	defer sim.Close()
	seedV22RouterActivationBooks(t, sim)
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("run instrumented router: %v", err)
	}
	audit, err := analysis.AuditMarketDataReceipts(cfg.LogDir)
	if err != nil || !audit.Valid || audit.Decisions == 0 {
		t.Fatalf("instrumented router scalar evidence = %#v, %v", audit, err)
	}
	vectors, err := analysis.AuditDecisionFrontierVectors(cfg.LogDir)
	if err != nil || !vectors.Valid || vectors.Decisions == 0 || vectors.Components != 3*vectors.Decisions {
		t.Fatalf("instrumented router vector evidence = %#v, %v", vectors, err)
	}
	if len(sim.Routers) != 1 || sim.Routers[0].Report().SubmittedGroups == 0 {
		t.Fatalf("targeted router activation did not submit a qualified group: %#v", sim.Routers)
	}

	// V2-0's generic mutation suite already attacks a receipt ledger. These
	// router-specific mutations prove that a three-venue route cannot lose one
	// frontier, one vector, or advance a component into the future while still
	// passing the independent V3 join.
	decisionsRaw, err := os.ReadFile(filepath.Join(cfg.LogDir, "market-data-decision-vectors-v1.bin"))
	if err != nil {
		t.Fatal(err)
	}
	componentsRaw, err := os.ReadFile(filepath.Join(cfg.LogDir, "market-data-frontier-components-v1.bin"))
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(cfg.LogDir, "market-data-frontier-vectors-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		if err := os.WriteFile(filepath.Join(cfg.LogDir, "market-data-decision-vectors-v1.bin"), decisionsRaw, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.LogDir, "market-data-frontier-components-v1.bin"), componentsRaw, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.LogDir, "market-data-frontier-vectors-v1.json"), manifestRaw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	defer restore()

	future := append([]byte(nil), componentsRaw...)
	decisionAt := binary.BigEndian.Uint64(decisionsRaw[48:56])
	binary.BigEndian.PutUint64(future[32:40], decisionAt+1)
	rewriteV22VectorArtifact(t, cfg.LogDir, decisionsRaw, future)
	mutated, err := analysis.AuditDecisionFrontierVectors(cfg.LogDir)
	if err != nil || mutated.Valid || mutated.FutureComponentUse == 0 {
		t.Fatalf("future router frontier component survived: audit=%#v err=%v", mutated, err)
	}
	restore()

	rewriteV22VectorArtifact(t, cfg.LogDir, decisionsRaw, componentsRaw[:len(componentsRaw)-56])
	mutated, err = analysis.AuditDecisionFrontierVectors(cfg.LogDir)
	if err != nil || mutated.Valid || mutated.MissingDecisionComponents == 0 {
		t.Fatalf("dropped router venue frontier survived: audit=%#v err=%v", mutated, err)
	}
	restore()

	rewriteV22VectorArtifact(t, cfg.LogDir, nil, nil)
	mutated, err = analysis.AuditDecisionFrontierVectors(cfg.LogDir)
	if err != nil || mutated.Valid || mutated.MissingVectorDecision == 0 {
		t.Fatalf("dropped router decision vectors survived: audit=%#v err=%v", mutated, err)
	}
}

func rewriteV22VectorArtifact(t *testing.T, dir string, decisions, components []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "market-data-decision-vectors-v1.bin"), decisions, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "market-data-frontier-components-v1.bin"), components, 0644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "market-data-frontier-vectors-v1.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []struct {
		name string
		raw  []byte
		rows int
	}{
		{name: "decisions", raw: decisions, rows: len(decisions) / simulation.DecisionFrontierVectorRecordBytes},
		{name: "components", raw: components, rows: len(components) / simulation.DecisionFrontierComponentRecordBytes},
	} {
		digest := sha256.Sum256(artifact.raw)
		row, ok := manifest[artifact.name].(map[string]any)
		if !ok {
			t.Fatalf("missing %s artifact row", artifact.name)
		}
		row["records"] = artifact.rows
		row["digest"] = hex.EncodeToString(digest[:])
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, append(encoded, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}

// seedV22RouterActivationBooks creates legal, non-crossed resting liquidity
// before runner start. It is a targeted router-path fixture, not a population
// calibration or an outcome experiment: north offers ABC at 101 while central
// bids at 105, so a router that has received all three public snapshots has
// one all-in FOK comparison to make.
func seedV22RouterActivationBooks(t *testing.T, sim *Sim) {
	t.Helper()
	quotes := []struct{ bid, ask int64 }{
		{bid: 100, ask: 101},
		{bid: 105, ask: 106},
		{bid: 99, ask: 110},
	}
	qty := int64(mvBasePrecision) / 10
	for index, venue := range sim.Venues {
		buyerID, sellerID := uint64(90_001), uint64(90_002)
		buyer := venue.Exchange.ConnectNewClient(buyerID, map[string]int64{"USD": 1_000_000 * mvQuotePrecision}, &exchange.FixedFee{})
		seller := venue.Exchange.ConnectNewClient(sellerID, map[string]int64{"ABC": 10 * mvBasePrecision}, &exchange.FixedFee{})
		for _, quote := range []struct {
			clientID uint64
			side     exchange.Side
			price    int64
		}{
			{clientID: buyerID, side: exchange.Buy, price: quotes[index].bid * mvQuotePrecision},
			{clientID: sellerID, side: exchange.Sell, price: quotes[index].ask * mvQuotePrecision},
		} {
			response := venue.Exchange.PlaceOrder(quote.clientID, &exchange.OrderRequest{
				RequestID: uint64(index + 1), Symbol: "ABC/USD", Side: quote.side, Type: exchange.LimitOrder,
				Price: quote.price, Qty: qty, TimeInForce: exchange.GTC, Visibility: exchange.Normal,
			})
			if !response.Success {
				t.Fatalf("seed %s %s liquidity failed: %#v", venue.ID, quote.side, response)
			}
		}
		drainFixtureGateway(buyer)
		drainFixtureGateway(seller)
		// The fixture accounts are passive resting-liquidity providers, not
		// actors. Disconnect their unused response sessions before runner start
		// so deterministic egress cannot wait for a nonexistent consumer; their
		// legal GTC orders remain on the book.
		venue.Exchange.DisconnectClient(buyerID)
		venue.Exchange.DisconnectClient(sellerID)
	}
}

func drainFixtureGateway(gateway actor.Gateway) {
	for {
		select {
		case <-gateway.Responses():
		case <-gateway.MarketDataCh():
		default:
			return
		}
	}
}

func runV20Helper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	return runV2EvidenceHelper(t, gomax, evidence, false, false, false, false)
}

func runEvidenceHelper(t *testing.T, gomax string, evidence, localCache bool) v20HelperResult {
	return runV2EvidenceHelper(t, gomax, evidence, localCache, false, false, false)
}

func runV22RouterEvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	return runV2EvidenceHelper(t, gomax, evidence, false, false, false, true)
}

func runV23P1EvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V23_P1_QUOTE_SIZE=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("P1 evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode P1 evidence helper output %q: %v", raw, err)
	}
	return result
}

func runV23P2EvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V23_P2_REBALANCE=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("P2 evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode P2 evidence helper output %q: %v", raw, err)
	}
	return result
}

func runV24L0EvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V24_L0_LIABILITY_HEDGER=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("L0 evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode L0 evidence helper output %q: %v", raw, err)
	}
	return result
}

func runV25FundingCarryEvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V25_FUNDING_CARRY=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("V2-5 P0 evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode V2-5 P0 helper output %q: %v", raw, err)
	}
	return result
}

func runV25TermCarryEvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V25_TERM_CARRY=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("V2-5 P3 evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode V2-5 P3 helper output %q: %v", raw, err)
	}
	return result
}

func runV24L1EvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V24_L1_RANDOM_SIDE_CONTROL=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("L1 evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode L1 evidence helper output %q: %v", raw, err)
	}
	return result
}

func runV24L1PhaseEvidenceHelper(t *testing.T, gomax string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V24_L1_PHASE_CONTROL=1", "V24_L1_PHASE_OFFSET=1", "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("L1-P evidence helper GOMAXPROCS=%s evidence=%t: %v\n%s", gomax, evidence, err, raw)
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
		t.Fatalf("decode L1-P evidence helper output %q: %v", raw, err)
	}
	return result
}

func runV24L1PhaseControlEvidenceHelper(t *testing.T, gomax string, explicitZero, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V24_L1_PHASE_CONTROL=1", "GOMAXPROCS="+gomax)
	if explicitZero {
		cmd.Env = append(cmd.Env, "V24_L1_EXPLICIT_ZERO_PHASE=1")
	}
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("L1-P phase-control helper GOMAXPROCS=%s explicitZero=%t evidence=%t: %v\n%s", gomax, explicitZero, evidence, err, raw)
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
		t.Fatalf("decode L1-P phase-control helper output %q: %v", raw, err)
	}
	return result
}

func runV24L1P2NoisePhaseEvidenceHelper(t *testing.T, gomax, phase string, evidence bool) v20HelperResult {
	t.Helper()
	output := filepath.Join(t.TempDir(), "run")
	cmd := exec.Command(os.Args[0], "-test.run=TestV20EvidenceHelper", "--")
	cmd.Env = append(os.Environ(), "V20_EVIDENCE_HELPER=1", "V20_EVIDENCE_OUTPUT="+output, "V24_L1P2_NOISE_PHASE="+phase, "GOMAXPROCS="+gomax)
	if evidence {
		cmd.Env = append(cmd.Env, "V20_EVIDENCE_ON=1")
	}
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("L1-P2 noise phase helper GOMAXPROCS=%s phase=%s evidence=%t: %v\n%s", gomax, phase, evidence, err, raw)
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
		t.Fatalf("decode L1-P2 noise phase helper output %q: %v", raw, err)
	}
	return result
}

func runV2EvidenceHelper(t *testing.T, gomax string, evidence, localCache, remoteFeed, remoteRoster, routerEvidence bool) v20HelperResult {
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
	if remoteRoster {
		cmd.Env = append(cmd.Env, "V21_REMOTE_ROSTER=1")
	}
	if routerEvidence {
		cmd.Env = append(cmd.Env, "V22_ROUTER=1")
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
