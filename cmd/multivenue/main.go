// multivenue runs the deterministic three-venue options ecology. It does not
// yet enable cross-venue routing: the output is the controlled fragmented
// baseline against which a future per-leg execution state machine is tested.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"exchange_sim/simulations/derivsim"
	"exchange_sim/simulations/multivenue"
)

type greekOutput struct {
	SchemaVersion int                             `json:"schema_version"`
	Venues        map[string]derivsim.GreekReport `json:"venues"`
	Caveats       []string                        `json:"caveats"`
}

func main() {
	configPath := flag.String("config", "", "path to multivenue Config JSON")
	duration := flag.Duration("duration", 8*time.Hour, "simulated duration; must be a multiple of the configured step")
	logDir := flag.String("logdir", "logs/multivenue", "output directory")
	flag.Parse()

	cfg := multivenue.Config{}
	if *configPath != "" {
		raw, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			log.Fatal(err)
		}
	}
	cfg.LogDir = *logDir

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
		SchemaVersion: 1,
		Venues:        make(map[string]derivsim.GreekReport, len(sim.Venues)),
		Caveats: []string{
			"Venues are independently funded. No cross-venue routing, asset transfer, or atomic leg assumption is modeled.",
			"Option reports use a flat IV and spot-mid forward proxy; they are local sensitivities, not realized vega PnL.",
			"Final actor profiles precede expiry. An exchange-owned pre-expiry risk hook remains required for terminal gamma/theta attribution.",
		},
	}
	for _, venue := range sim.Venues {
		report, err := derivsim.BuildGreekReportWithPositions(venue.OptionDealer.GreekProfiles(), venue.OptionDealer.GreekPositionProfiles())
		if err != nil {
			log.Fatalf("venue %s: %v", venue.ID, err)
		}
		output.Venues[venue.ID] = report
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
