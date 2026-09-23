package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("meinstructionanalyze", flag.ContinueOnError)
	repo := flags.String("repo", ".", "clean source checkout")
	simulator := flags.String("simulator", "", "pinned simulator binary")
	plan := flags.String("plan", "", "locked plan file")
	runDir := flags.String("run", "", "completed run directory")
	output := flags.String("out", "", "new analysis result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *simulator == "" || *plan == "" || *runDir == "" || *output == "" || flags.NArg() != 0 {
		return fmt.Errorf("meinstructionanalyze: -simulator, -plan, -run and -out required")
	}
	planRaw, err := os.ReadFile(*plan)
	if err != nil {
		return err
	}
	locked, _, err := executionpilot.DecodeInstructionPlan(planRaw)
	if err != nil {
		return err
	}
	outcome, manifest, err := executionpilot.AnalyzeInstructionRun(*repo, *simulator, *plan, *runDir)
	if err != nil {
		return err
	}
	if err := executionpilot.WriteInstructionResult(*output, outcome, manifest, locked.Cell); err != nil {
		return err
	}
	fmt.Printf("reconstructed %s for client %d\n", outcome.Status, outcome.ClientID)
	return nil
}
