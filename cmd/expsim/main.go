package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"exchange_sim/simulations/feesim"
)

func main() {
	configPath := flag.String("config", "", "path to SimConfig JSON (empty = defaults)")
	duration := flag.Duration("duration", 15*time.Minute, "simulation time to run")
	logDir := flag.String("logdir", "", "override LogDir from config")
	flag.Parse()

	cfg := feesim.DefaultSimConfig()
	if *configPath != "" {
		raw, err := os.ReadFile(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			log.Fatal(err)
		}
	}
	if *logDir != "" {
		cfg.LogDir = *logDir
	}

	sim, err := feesim.NewSim(*duration, cfg)
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
	log.Printf("done: sim=%s wall=%s logs=%s", *duration, time.Since(started).Round(time.Second), cfg.LogDir)
}
