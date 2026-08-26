// multivenue runs the deterministic three-venue options ecology. By default
// venues remain independent; an opt-in router configuration emits explicit
// non-atomic, venue-qualified cross-venue leg telemetry.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"exchange_sim/simulations/multivenue"
)

type greekOutput struct {
	SchemaVersion    int                                       `json:"schema_version"`
	InitialRisk      map[string]multivenue.VenueRiskSnapshot   `json:"initial_risk"`
	RiskTimeline     map[string][]multivenue.VenueRiskSnapshot `json:"risk_timeline"`
	PreExpiryRisk    map[string][]multivenue.VenueRiskSnapshot `json:"pre_expiry_risk"`
	TerminalRisk     map[string]multivenue.VenueRiskSnapshot   `json:"terminal_risk"`
	Microstructure   []multivenue.MicrostructureStats          `json:"microstructure"`
	Metaorders       []multivenue.MetaorderRecord              `json:"metaorders,omitempty"`
	CarryActivity    []multivenue.CarryActivity                `json:"carry_activity,omitempty"`
	RouterReports    []multivenue.CrossVenueArbReport          `json:"router_reports,omitempty"`
	InitialAccounts  []multivenue.ParticipantAccountSnapshot   `json:"initial_accounts,omitempty"`
	TerminalAccounts []multivenue.ParticipantAccountSnapshot   `json:"terminal_accounts,omitempty"`
	VenueLedgers     []multivenue.VenueLedger                  `json:"venue_ledgers,omitempty"`
	RequestBudgets   []multivenue.RequestBudgetReport          `json:"request_budgets,omitempty"`
	Caveats          []string                                  `json:"caveats"`
}

type runProfiles struct {
	cpu   *os.File
	alloc *os.File
	mutex *os.File
	block *os.File
	trace *os.File
}

// startRunProfiles instruments only the command process around Sim.Run. It
// does not enter the simulator configuration, scheduler, RNG, or actor state.
// Profiled runs are performance observations, never execution evidence.
func startRunProfiles(cpuPath, allocPath, mutexPath, blockPath, tracePath string) (*runProfiles, error) {
	profiles := &runProfiles{}
	var err error
	if cpuPath != "" {
		if profiles.cpu, err = os.Create(cpuPath); err != nil {
			return nil, err
		}
		if err := pprof.StartCPUProfile(profiles.cpu); err != nil {
			_ = profiles.cpu.Close()
			return nil, err
		}
	}
	if allocPath != "" {
		if profiles.alloc, err = os.Create(allocPath); err != nil {
			profiles.Stop()
			return nil, err
		}
		// Keep Go's normal sampling rate: allocation profiles should describe
		// production-like execution rather than turn every allocation into a
		// profiling intervention.
		runtime.MemProfileRate = 512 * 1024
	}
	if mutexPath != "" {
		if profiles.mutex, err = os.Create(mutexPath); err != nil {
			profiles.Stop()
			return nil, err
		}
		runtime.SetMutexProfileFraction(1)
	}
	if blockPath != "" {
		if profiles.block, err = os.Create(blockPath); err != nil {
			profiles.Stop()
			return nil, err
		}
		runtime.SetBlockProfileRate(1)
	}
	if tracePath != "" {
		if profiles.trace, err = os.Create(tracePath); err != nil {
			profiles.Stop()
			return nil, err
		}
		if err := trace.Start(profiles.trace); err != nil {
			profiles.Stop()
			return nil, err
		}
	}
	return profiles, nil
}

func (p *runProfiles) Stop() {
	if p == nil {
		return
	}
	if p.trace != nil {
		trace.Stop()
		_ = p.trace.Close()
		p.trace = nil
	}
	if p.cpu != nil {
		pprof.StopCPUProfile()
		_ = p.cpu.Close()
		p.cpu = nil
	}
	if p.alloc != nil {
		_ = pprof.Lookup("allocs").WriteTo(p.alloc, 0)
		_ = p.alloc.Close()
		p.alloc = nil
	}
	if p.mutex != nil {
		_ = pprof.Lookup("mutex").WriteTo(p.mutex, 0)
		_ = p.mutex.Close()
		p.mutex = nil
		runtime.SetMutexProfileFraction(0)
	}
	if p.block != nil {
		_ = pprof.Lookup("block").WriteTo(p.block, 0)
		_ = p.block.Close()
		p.block = nil
		runtime.SetBlockProfileRate(0)
	}
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() (err error) {
	configPath := flag.String("config", "", "path to multivenue Config JSON")
	duration := flag.Duration("duration", 8*time.Hour, "simulated duration; must be a multiple of the configured step")
	logDir := flag.String("logdir", "logs/multivenue", "output directory")
	seed := flag.Int64("seed", 0, "override simulation seed (0 keeps config/default)")
	hedgeMode := flag.String("dealer-hedge", "", "override dealer hedge mode: on or off")
	logMode := flag.String("log-mode", "", "override raw log mode: full or none")
	checkpointInterval := flag.Int("checkpoint-interval-seconds", -1, "override ordered execution checkpoint interval; negative keeps config")
	cpuProfile := flag.String("cpuprofile", "", "write CPU profile for Sim.Run only")
	allocProfile := flag.String("allocprofile", "", "write sampled allocation profile after Sim.Run")
	mutexProfile := flag.String("mutexprofile", "", "write mutex profile after Sim.Run")
	blockProfile := flag.String("blockprofile", "", "write block profile after Sim.Run")
	traceProfile := flag.String("traceprofile", "", "write Go execution trace for Sim.Run only")
	recordReceipts := flag.Bool("record-market-data-receipts", false, "emit V2 participant-information evidence sidecars")
	receiptRoles := flag.String("market-data-receipt-roles", "", "comma-separated audited role classes; required with -record-market-data-receipts")
	flag.Parse()

	cfg := multivenue.Config{}
	if *configPath != "" {
		raw, readErr := os.ReadFile(*configPath)
		if readErr != nil {
			return readErr
		}
		decoded, decodeErr := multivenue.DecodeConfig(raw)
		if decodeErr != nil {
			return decodeErr
		}
		cfg = decoded
	}
	cfg.LogDir = *logDir
	if *seed != 0 {
		cfg.Seed = *seed
	}
	if *hedgeMode != "" {
		cfg.DealerHedgeMode = *hedgeMode
	}
	if *logMode != "" {
		cfg.LogMode = *logMode
	}
	if *checkpointInterval >= 0 {
		cfg.CheckpointIntervalSeconds = *checkpointInterval
	}
	if *recordReceipts {
		cfg.RecordMarketDataReceipts = true
		if *receiptRoles != "" {
			cfg.MarketDataReceiptRoles = strings.Split(*receiptRoles, ",")
		}
	}

	sim, err := multivenue.NewSim(*duration, cfg)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, sim.Close())
		}
	}()
	profiles, err := startRunProfiles(*cpuProfile, *allocProfile, *mutexProfile, *blockProfile, *traceProfile)
	if err != nil {
		return err
	}
	profilesStopped := false
	defer func() {
		if !profilesStopped {
			profiles.Stop()
		}
	}()

	started := time.Now()
	runErr := sim.Run(context.Background())
	profiles.Stop()
	profilesStopped = true
	if runErr != nil {
		return runErr
	}
	if err := sim.Close(); err != nil {
		return fmt.Errorf("seal evidence: %w", err)
	}
	closed = true
	output := greekOutput{
		SchemaVersion:  6,
		InitialRisk:    make(map[string]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		RiskTimeline:   make(map[string][]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		PreExpiryRisk:  make(map[string][]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		TerminalRisk:   make(map[string]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		Microstructure: make([]multivenue.MicrostructureStats, 0, len(sim.Venues)),
		Caveats: []string{
			"Venues are independently funded. A configured cross-venue router has one local account per venue; it models neither asset transfer nor atomic legs.",
			"Greek timeline rows are recomputed from exchange-owned option positions and the atomic underlying mark paired with each option premium. They are not actor-local quote-cache measurements.",
			"The option model uses flat IV and its stored underlying-mark forward proxy; vega is local model sensitivity, not realized vega PnL.",
			"Terminal marked equity includes wallet debt exactly once, futures-style entry-to-mark PnL, and signed option market value. It is captured after the final phase fixed point and before venue shutdown.",
			"Expiry-pre-settlement rows preserve marked account state. Use the final positive-time-to-expiry timeline row for expiring option gamma and vega because those Greeks are undefined at expiry.",
		},
	}
	for _, venue := range sim.Venues {
		if venue.InitialRisk == nil || venue.TerminalRisk == nil {
			return fmt.Errorf("venue %s missing initial or terminal risk snapshot", venue.ID)
		}
		output.InitialRisk[venue.ID] = *venue.InitialRisk
		output.RiskTimeline[venue.ID] = append([]multivenue.VenueRiskSnapshot(nil), venue.RiskTimeline...)
		output.PreExpiryRisk[venue.ID] = append([]multivenue.VenueRiskSnapshot(nil), venue.PreExpiryRisk...)
		output.TerminalRisk[venue.ID] = *venue.TerminalRisk
		if venue.Microstructure != nil {
			output.Microstructure = append(output.Microstructure, *venue.Microstructure)
		}
		for _, arb := range venue.CarryArbs {
			output.CarryActivity = append(output.CarryActivity, arb.Activity(venue.ID))
		}
		for _, trader := range venue.MetaorderTraders {
			output.Metaorders = append(output.Metaorders, trader.Records()...)
		}
	}
	output.InitialAccounts = append([]multivenue.ParticipantAccountSnapshot(nil), sim.InitialAccounts...)
	output.TerminalAccounts = append([]multivenue.ParticipantAccountSnapshot(nil), sim.TerminalAccounts...)
	output.VenueLedgers = sim.CaptureVenueLedgers()
	output.RequestBudgets = sim.CaptureRequestBudgets()
	for _, router := range sim.Routers {
		output.RouterReports = append(output.RouterReports, router.Report())
	}
	b, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*logDir, "greeks.json"), b, 0644); err != nil {
		return err
	}
	log.Printf("done: sim=%s wall=%s logs=%s", *duration, time.Since(started).Round(time.Second), *logDir)
	return nil
}
