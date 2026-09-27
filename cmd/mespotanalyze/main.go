package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/repeatedspot"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mespotanalyze:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
	study := flags.String("study", "ME-013", "registered development study")
	repository := flags.String("repo", "", "clean pinned E0 source checkout")
	simulator := flags.String("simulator", "", "pinned mespotrun binary")
	plan := flags.String("plan", "", "locked plan JSON path")
	runDirectory := flags.String("run", "", "completed run directory")
	output := flags.String("out", "", "new exclusive analysis JSON path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *repository == "" || *simulator == "" || *plan == "" || *runDirectory == "" || *output == "" {
		return errors.New("analyze requires -repo, -simulator, -plan, -run and -out")
	}
	if *study == "ME-016" {
		_, err := repeatedspot.AnalyzeME016Run(*repository, *simulator, *plan, *runDirectory, *output)
		return err
	}
	if *study != "ME-013" && *study != "ME-015" {
		return fmt.Errorf("unknown registered study %q", *study)
	}
	_, err := repeatedspot.AnalyzeE0Run(*repository, *simulator, *plan, *runDirectory, *output)
	return err
}
