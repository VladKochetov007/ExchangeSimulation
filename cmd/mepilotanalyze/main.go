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
	flags := flag.NewFlagSet("mepilotanalyze", flag.ContinueOnError)
	repo := flags.String("repo", ".", "clean source checkout")
	simulator := flags.String("simulator", "", "pinned simulator binary")
	plan := flags.String("plan", "", "locked plan file")
	runDir := flags.String("run", "", "completed run directory")
	output := flags.String("out", "", "new analysis result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *simulator == "" || *plan == "" || *runDir == "" || *output == "" || flags.NArg() != 0 {
		return fmt.Errorf("mepilotanalyze: -simulator, -plan, -run, and -out are required")
	}
	outcome, manifest, err := executionpilot.AnalyzeRun(*repo, *simulator, *plan, *runDir)
	if err != nil {
		return err
	}
	if err := executionpilot.WriteResult(*output, outcome, manifest); err != nil {
		return err
	}
	fmt.Printf("reconstructed %s for client %d\n", outcome.Status, outcome.ClientID)
	return nil
}
