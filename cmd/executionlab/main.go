// executionlab runs paired, fixed-seed immediate and TWAP execution worlds.
// It writes JSON Lines so analysis can exclude incomplete parents explicitly.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"exchange_sim/simulations/executionlab"
)

type result struct {
	Seed   int64                        `json:"seed"`
	Policy executionlab.Policy          `json:"policy"`
	Report executionlab.ExecutionReport `json:"report"`
}

func main() {
	seeds := flag.Int("seeds", 20, "number of independent common-random-number seeds")
	seed := flag.Int64("seed", 42, "first random seed")
	policy := flag.String("policy", "both", "immediate, twap, or both")
	duration := flag.Duration("duration", 4*time.Second, "simulated duration per world")
	targetQty := flag.Int64("target-qty", 0, "parent quantity in base fixed-point units (0 uses the scenario default)")
	slices := flag.Int("slices", 0, "TWAP child count (0 uses the scenario default)")
	sliceInterval := flag.Duration("slice-interval", 0, "TWAP child interval (0 uses the scenario default)")
	flag.Parse()
	if *seeds <= 0 {
		fatalf("-seeds must be positive")
	}
	policies, err := parsePolicies(*policy)
	if err != nil {
		fatalf("%v", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	for i := 0; i < *seeds; i++ {
		for _, current := range policies {
			cfg := executionlab.DefaultSimConfig(current)
			cfg.Seed = *seed + int64(i)
			cfg.Duration = *duration
			if *targetQty != 0 {
				cfg.Parent.TargetQty = *targetQty
			}
			if *slices != 0 {
				cfg.Parent.SliceCount = *slices
			}
			if *sliceInterval != 0 {
				cfg.Parent.SliceInterval = *sliceInterval
			}
			sim, newErr := executionlab.NewSim(cfg)
			if newErr != nil {
				fatalf("seed %d %s: %v", cfg.Seed, current, newErr)
			}
			report, runErr := sim.Run(context.Background())
			if runErr != nil {
				fatalf("seed %d %s: %v", cfg.Seed, current, runErr)
			}
			if encodeErr := encoder.Encode(result{Seed: cfg.Seed, Policy: current, Report: report}); encodeErr != nil {
				fatalf("write result: %v", encodeErr)
			}
		}
	}
}

func parsePolicies(value string) ([]executionlab.Policy, error) {
	switch value {
	case "immediate":
		return []executionlab.Policy{executionlab.Immediate}, nil
	case "twap":
		return []executionlab.Policy{executionlab.TWAP}, nil
	case "both":
		return []executionlab.Policy{executionlab.Immediate, executionlab.TWAP}, nil
	default:
		return nil, fmt.Errorf("-policy must be immediate, twap, or both")
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "executionlab: "+format+"\n", args...)
	os.Exit(2)
}
