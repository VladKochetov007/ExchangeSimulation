package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"exchange_sim/experiment/repeatedspot"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mespotrun:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return errors.New("usage: mespotrun plan|run|cells [flags]")
	}
	switch arguments[0] {
	case "cells":
		flags := flag.NewFlagSet("cells", flag.ContinueOnError)
		study := flags.String("study", "ME-013", "registered development study: ME-013 or ME-015")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("cells takes only -study")
		}
		switch *study {
		case "ME-013":
			return json.NewEncoder(os.Stdout).Encode(repeatedspot.E0DevelopmentCells())
		case "ME-015":
			return json.NewEncoder(os.Stdout).Encode(repeatedspot.E0LocalReferenceDevelopmentCells())
		default:
			return fmt.Errorf("unknown registered study %q", *study)
		}
	case "plan":
		flags := flag.NewFlagSet("plan", flag.ContinueOnError)
		repository := flags.String("repo", "", "clean pinned E0 source checkout")
		analyzer := flags.String("analyzer", "", "pinned mespotanalyze binary")
		output := flags.String("out", "", "new exclusive plan JSON path")
		composition := flags.String("composition", "", "P, A, M1 or M2")
		quantity := flags.Int64("quote-qty", 0, "maker base atoms per side")
		seed := flags.Int64("seed", 0, "registered E0 development seed")
		referenceMode := flags.String("reference-mode", "", "empty for ME-013; OFF or ON for ME-015")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *repository == "" || *analyzer == "" || *output == "" {
			return errors.New("plan requires -repo, -analyzer, -out, -composition, -quote-qty and -seed")
		}
		return repeatedspot.WriteE0Plan(*repository, *analyzer, *output,
			repeatedspot.E0Cell{Composition: *composition, QuoteQty: *quantity,
				Seed: *seed, ReferenceMode: *referenceMode})
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		repository := flags.String("repo", "", "clean pinned E0 source checkout")
		analyzer := flags.String("analyzer", "", "pinned mespotanalyze binary")
		plan := flags.String("plan", "", "locked plan JSON path")
		output := flags.String("out", "", "fresh run directory")
		timeout := flags.Duration("timeout", 0, "positive real-time process deadline")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *repository == "" || *analyzer == "" || *plan == "" || *output == "" || *timeout <= 0 {
			return errors.New("run requires -repo, -analyzer, -plan, -out and positive -timeout")
		}
		contextWithDeadline, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		started := time.Now()
		manifest, err := repeatedspot.RunE0Plan(contextWithDeadline, *repository, *analyzer, *plan, *output)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "mespotrun: completed E0 cell %s in %s\n", manifest.Cell.ID(), time.Since(started))
		return json.NewEncoder(os.Stdout).Encode(manifest)
	default:
		return fmt.Errorf("unknown mespotrun mode %q", arguments[0])
	}
}
