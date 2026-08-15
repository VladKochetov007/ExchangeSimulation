// feesimrace runs a deterministic, many-agent basis-latency experiment.
// It reports only exchange-observed fills and strict terminal account marks.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"exchange_sim/simulations/feesim"
)

type result struct {
	Seed         int64                          `json:"seed"`
	Duration     time.Duration                  `json:"duration"`
	NoiseTraders int                            `json:"noise_traders"`
	TierOrder    []float64                      `json:"tier_order"`
	Reports      []feesim.RaceArbTerminalReport `json:"reports"`
}

func main() {
	duration := flag.Duration("duration", 2*time.Minute, "simulated duration")
	seed := flag.Int64("seed", 42, "world seed")
	noiseTraders := flag.Int("noise-traders", 8, "independent random-flow participants")
	tiers := flag.String("tiers", "0.2,1", "comma-separated latency factors in actor/client order")
	logDir := flag.String("logdir", "logs/feesim-race", "raw event-log directory")
	flag.Parse()
	if *duration <= 0 || *noiseTraders <= 0 {
		fatalf("-duration and -noise-traders must be positive")
	}
	tierOrder, err := parseTiers(*tiers)
	if err != nil {
		fatalf("%v", err)
	}

	cfg := feesim.DefaultSimConfig()
	cfg.LogDir = *logDir
	cfg.Seed = *seed
	cfg.Deterministic = true
	cfg.NoiseTraderCount = *noiseTraders
	cfg.TakerIntervalMs = 20
	cfg.MMCount = 2
	cfg.MMBaseIntervalMs = 10
	cfg.MMMaxIntervalMs = 20
	cfg.LatencyMinUs = 1_000
	cfg.LatencyMedianUs = 3_000
	cfg.LatencySigma = 0.25
	cfg.RaceArbTiers = tierOrder
	cfg.RaceArbReactive = true
	cfg.RaceArbHedge = true
	cfg.RaceArbSequential = true

	sim, err := feesim.NewSim(*duration, cfg)
	if err != nil {
		fatalf("NewSim: %v", err)
	}
	defer sim.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sim.Exchange().StartAutomation(ctx)
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})
	if err := sim.Runner.Run(ctx); err != nil {
		fatalf("run: %v", err)
	}
	reports, err := sim.RaceArbTerminalReports()
	if err != nil {
		fatalf("terminal report: %v", err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result{
		Seed: *seed, Duration: *duration, NoiseTraders: *noiseTraders,
		TierOrder: tierOrder, Reports: reports,
	}); err != nil {
		fatalf("write result: %v", err)
	}
}

func parseTiers(value string) ([]float64, error) {
	parts := strings.Split(value, ",")
	if len(parts) < 2 {
		return nil, fmt.Errorf("-tiers needs at least two positive factors")
	}
	tiers := make([]float64, 0, len(parts))
	seen := make(map[float64]struct{}, len(parts))
	for _, part := range parts {
		tier, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || tier <= 0 {
			return nil, fmt.Errorf("invalid latency tier %q", part)
		}
		if _, exists := seen[tier]; exists {
			return nil, fmt.Errorf("duplicate latency tier %g", tier)
		}
		seen[tier] = struct{}{}
		tiers = append(tiers, tier)
	}
	return tiers, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "feesimrace: "+format+"\n", args...)
	os.Exit(2)
}
