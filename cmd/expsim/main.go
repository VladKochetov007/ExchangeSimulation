package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"exchange_sim/simulations/feesim"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() (err error) {
	configPath := flag.String("config", "", "path to SimConfig JSON (empty = defaults)")
	duration := flag.Duration("duration", 15*time.Minute, "simulation time to run")
	logDir := flag.String("logdir", "", "override LogDir from config")
	flag.Parse()

	cfg := feesim.DefaultSimConfig()
	if *configPath != "" {
		raw, err := os.ReadFile(*configPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	if *logDir != "" {
		cfg.LogDir = *logDir
	}

	sim, err := feesim.NewSim(*duration, cfg)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, sim.Close())
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	sim.Exchange().StartAutomation(ctx)
	sim.Runner.SetShutdownHook(func() {
		cancel()
		sim.Exchange().StopAutomation()
	})

	started := time.Now()
	if err := sim.Runner.Run(ctx); err != nil {
		return err
	}
	if err := sim.Close(); err != nil {
		return fmt.Errorf("seal evidence: %w", err)
	}
	closed = true
	log.Printf("done: sim=%s wall=%s logs=%s", *duration, time.Since(started).Round(time.Second), cfg.LogDir)

	for i, arb := range sim.RaceArbs {
		log.Printf("race_arb client=%d tier=%.2f reactive=%v final_position=%d",
			arb.ID(), cfg.RaceArbTiers[i], cfg.RaceArbReactive, arb.Position())
	}
	return nil
}
