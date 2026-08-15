package feesim

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

func phaseRunDigest(t *testing.T, procs int) [sha256.Size]byte {
	t.Helper()
	previous := runtime.GOMAXPROCS(procs)
	t.Cleanup(func() { runtime.GOMAXPROCS(previous) })

	cfg := DefaultSimConfig()
	cfg.LogDir = t.TempDir()
	cfg.Deterministic = true
	cfg.LatencyMinUs = 1_000
	cfg.LatencyMedianUs = 2_000
	cfg.LatencySigma = 0.25
	cfg.TakerIntervalMs = 20
	cfg.NoiseTraderCount = 4
	cfg.MMBaseIntervalMs = 10
	cfg.MMMaxIntervalMs = 20

	sim, err := NewSim(3*time.Second, cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sim.Exchange().StartAutomation(ctx)
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})
	if err := sim.Runner.Run(ctx); err != nil {
		sim.Close()
		t.Fatalf("Run: %v", err)
	}
	sim.Close()

	var files []string
	if err := filepath.WalkDir(cfg.LogDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk logs: %v", err)
	}
	sort.Strings(files)
	h := sha256.New()
	for _, path := range files {
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		_, _ = h.Write([]byte(filepath.Base(path)))
		_, _ = h.Write(contents)
	}
	var digest [sha256.Size]byte
	copy(digest[:], h.Sum(nil))
	return digest
}

func TestFeesimDeterministicLatencyDigestAcrossGOMAXPROCS(t *testing.T) {
	one := phaseRunDigest(t, 1)
	many := phaseRunDigest(t, 14)
	if one != many {
		t.Fatalf("fixed-seed delayed phase digests differ: GOMAXPROCS=1 %x, GOMAXPROCS=14 %x", one, many)
	}
}

func TestFeesimRaceArbUsesConfiguredLotSize(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.LogDir = t.TempDir()
	cfg.RaceArbTiers = []float64{1}
	cfg.RaceArbLotSize = 7 * abcPrecision / 100

	sim, err := NewSim(time.Millisecond, cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if len(sim.RaceArbs) != 1 {
		t.Fatalf("race arbs = %d, want 1", len(sim.RaceArbs))
	}
	if got := sim.RaceArbs[0].cfg.LotSize; got != cfg.RaceArbLotSize {
		t.Fatalf("race arb lot size = %d, want configured %d", got, cfg.RaceArbLotSize)
	}
}

func TestFeesimRaceTerminalReportsUseStrictMarks(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.LogDir = t.TempDir()
	cfg.Deterministic = true
	cfg.NoiseTraderCount = 4
	cfg.RaceArbTiers = []float64{1, 0.2}
	cfg.RaceArbReactive = true

	sim, err := NewSim(3*time.Second, cfg)
	if err != nil {
		t.Fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	if len(sim.Takers) != cfg.NoiseTraderCount || sim.Taker != sim.Takers[0] {
		t.Fatalf("noise roster = %#v; baseline taker is not roster head", sim.Takers)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sim.Exchange().StartAutomation(ctx)
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})
	if err := sim.Runner.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reports, err := sim.RaceArbTerminalReports()
	if err != nil {
		t.Fatalf("RaceArbTerminalReports: %v", err)
	}
	if len(reports) != 2 || reports[0].Tier != 1 || reports[1].Tier != 0.2 {
		t.Fatalf("tier reports = %#v", reports)
	}
	for _, report := range reports {
		if report.ClientID == 0 || report.InitialEquityUSD <= 0 || report.PassiveTerminalEquityUSD <= 0 || report.TerminalAccount.ReportAsset != "USD" {
			t.Fatalf("invalid strict terminal report: %#v", report)
		}
		if report.Fills.SubmittedPairs == 0 && report.StrategyEquityChangeUSD != 0 {
			t.Fatalf("inactive race tier has strategy PnL %d: %#v", report.StrategyEquityChangeUSD, report)
		}
	}
}
