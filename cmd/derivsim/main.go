package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"exchange_sim/simulations/derivsim"
)

// summary is the per-run result written next to the logs.
type summary struct {
	HedgeTraded   int64                     `json:"dealer_hedge_traded"`
	HedgePosition int64                     `json:"dealer_hedge_position"`
	BasisMeanAbs  float64                   `json:"basis_mean_abs_bps"`
	BasisByBucket map[string][2]interface{} `json:"basis_by_tte_bucket,omitempty"`
	ParityMeanAbs float64                   `json:"parity_mean_abs_bps"`
	ParityTrades  int64                     `json:"parity_trades"`
	BasisSamples  int                       `json:"basis_samples"`
	ParitySamples int                       `json:"parity_samples"`
	Greeks        derivsim.GreekSummary     `json:"dealer_greeks"`
}

func main() {
	configPath := flag.String("config", "", "path to SimConfig JSON")
	duration := flag.Duration("duration", 20*time.Minute, "simulation time")
	logDir := flag.String("logdir", "logs/derivsim", "log directory")
	flag.Parse()

	cfg := derivsim.SimConfig{}
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

	sim, err := derivsim.NewSim(*duration, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer sim.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sim.Exchange().StartAutomation(ctx)
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})
	started := time.Now()
	if err := sim.Runner.Run(ctx); err != nil {
		log.Fatal(err)
	}

	out := summary{
		HedgeTraded:   sim.Dealer.HedgeTraded(),
		HedgePosition: sim.Dealer.HedgePosition(),
	}
	if sim.CarryBot != nil {
		series := sim.CarryBot.BasisSeries()
		out.BasisSamples = len(series)
		out.BasisMeanAbs = meanAbsBasis(series)
		out.BasisByBucket = basisBuckets(series)
	}
	if sim.ParityBot != nil {
		series := sim.ParityBot.ParitySeries()
		out.ParitySamples = len(series)
		var sum float64
		for _, s := range series {
			sum += math.Abs(s.GapBps)
		}
		if len(series) > 0 {
			out.ParityMeanAbs = sum / float64(len(series))
		}
		out.ParityTrades = sim.ParityBot.Trades()
	}
	greekReport, err := derivsim.BuildGreekReport(sim.Dealer.GreekProfiles())
	if err != nil {
		log.Fatal(err)
	}
	out.Greeks = greekReport.Summary

	b, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(filepath.Join(*logDir, "summary.json"), b, 0644); err != nil {
		log.Fatal(err)
	}
	greekBytes, err := json.MarshalIndent(greekReport, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*logDir, "greeks.json"), greekBytes, 0644); err != nil {
		log.Fatal(err)
	}
	log.Printf("done: sim=%s wall=%s logs=%s", *duration, time.Since(started).Round(time.Second), *logDir)
	log.Printf("summary: %s", string(b))
}

func meanAbsBasis(series []derivsim.BasisSample) float64 {
	if len(series) == 0 {
		return 0
	}
	var sum float64
	for _, s := range series {
		sum += math.Abs(s.BasisBps)
	}
	return sum / float64(len(series))
}

// basisBuckets averages |basis| by time-to-expiry bucket — the convergence
// profile: H-F1 predicts the near-expiry buckets shrink.
func basisBuckets(series []derivsim.BasisSample) map[string][2]interface{} {
	type acc struct {
		sum float64
		n   int
	}
	buckets := map[string]*acc{}
	names := []struct {
		name string
		lo   time.Duration
		hi   time.Duration
	}{
		{"0-30s", 0, 30 * time.Second},
		{"30-60s", 30 * time.Second, time.Minute},
		{"1-2m", time.Minute, 2 * time.Minute},
		{"2-5m", 2 * time.Minute, 5 * time.Minute},
	}
	for _, s := range series {
		for _, b := range names {
			if s.TimeToExpiry >= b.lo && s.TimeToExpiry < b.hi {
				if buckets[b.name] == nil {
					buckets[b.name] = &acc{}
				}
				buckets[b.name].sum += math.Abs(s.BasisBps)
				buckets[b.name].n++
			}
		}
	}
	out := map[string][2]interface{}{}
	for name, a := range buckets {
		out[name] = [2]interface{}{a.sum / float64(a.n), a.n}
	}
	return out
}
