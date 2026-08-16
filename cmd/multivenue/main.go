// multivenue runs the deterministic three-venue options ecology. By default
// venues remain independent; an opt-in router configuration emits explicit
// non-atomic, venue-qualified cross-venue leg telemetry.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"exchange_sim/simulations/multivenue"
)

type greekOutput struct {
	SchemaVersion    int                                       `json:"schema_version"`
	InitialRisk      map[string]multivenue.VenueRiskSnapshot   `json:"initial_risk"`
	RiskTimeline     map[string][]multivenue.VenueRiskSnapshot `json:"risk_timeline"`
	PreExpiryRisk    map[string][]multivenue.VenueRiskSnapshot `json:"pre_expiry_risk"`
	TerminalRisk     map[string]multivenue.VenueRiskSnapshot   `json:"terminal_risk"`
	Mispricing       []multivenue.MispricingStats              `json:"mispricing"`
	Microstructure   []multivenue.MicrostructureStats          `json:"microstructure"`
	Metaorders       []multivenue.MetaorderRecord              `json:"metaorders,omitempty"`
	CarryActivity    []multivenue.CarryActivity                `json:"carry_activity,omitempty"`
	RouterReports    []multivenue.CrossVenueArbReport          `json:"router_reports,omitempty"`
	InitialAccounts  []multivenue.ParticipantAccountSnapshot   `json:"initial_accounts,omitempty"`
	TerminalAccounts []multivenue.ParticipantAccountSnapshot   `json:"terminal_accounts,omitempty"`
	Caveats          []string                                  `json:"caveats"`
}

func main() {
	configPath := flag.String("config", "", "path to multivenue Config JSON")
	duration := flag.Duration("duration", 8*time.Hour, "simulated duration; must be a multiple of the configured step")
	logDir := flag.String("logdir", "logs/multivenue", "output directory")
	seed := flag.Int64("seed", 0, "override simulation seed (0 keeps config/default)")
	hedgeMode := flag.String("dealer-hedge", "", "override dealer hedge mode: on or off")
	flag.Parse()

	cfg := multivenue.Config{}
	if *configPath != "" {
		raw, readErr := os.ReadFile(*configPath)
		if readErr != nil {
			log.Fatal(readErr)
		}
		decoded, decodeErr := multivenue.DecodeConfig(raw)
		if decodeErr != nil {
			log.Fatal(decodeErr)
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

	sim, err := multivenue.NewSim(*duration, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer sim.Close()

	started := time.Now()
	if err := sim.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
	output := greekOutput{
		SchemaVersion:  5,
		InitialRisk:    make(map[string]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		RiskTimeline:   make(map[string][]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		PreExpiryRisk:  make(map[string][]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		TerminalRisk:   make(map[string]multivenue.VenueRiskSnapshot, len(sim.Venues)),
		Mispricing:     make([]multivenue.MispricingStats, 0, len(sim.Venues)),
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
			log.Fatalf("venue %s missing initial or terminal risk snapshot", venue.ID)
		}
		output.InitialRisk[venue.ID] = *venue.InitialRisk
		output.RiskTimeline[venue.ID] = append([]multivenue.VenueRiskSnapshot(nil), venue.RiskTimeline...)
		output.PreExpiryRisk[venue.ID] = append([]multivenue.VenueRiskSnapshot(nil), venue.PreExpiryRisk...)
		output.TerminalRisk[venue.ID] = *venue.TerminalRisk
		if venue.Mispricing != nil {
			output.Mispricing = append(output.Mispricing, *venue.Mispricing)
		}
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
	for _, router := range sim.Routers {
		output.RouterReports = append(output.RouterReports, router.Report())
	}
	b, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*logDir, "greeks.json"), b, 0644); err != nil {
		log.Fatal(err)
	}
	log.Printf("done: sim=%s wall=%s logs=%s", *duration, time.Since(started).Round(time.Second), *logDir)
}
