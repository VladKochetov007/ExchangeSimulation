package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mecadence plan|run|analyze [flags]")
	}
	switch args[0] {
	case "plan":
		flags := flag.NewFlagSet("plan", flag.ContinueOnError)
		repo := flags.String("repo", ".", "clean source checkout")
		analyzer := flags.String("analyzer", "", "pinned analyzer binary")
		output := flags.String("out", "", "new plan file")
		network := flags.Duration("network", 0, "directed feed/request/response delay")
		poll := flags.Duration("poll", 0, "policy tick interval")
		target := flags.Int64("target", 0, "target base units")
		seed := flags.Int64("seed", 0, "development seed")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *analyzer == "" || *output == "" || flags.NArg() != 0 {
			return fmt.Errorf("mecadence plan: -analyzer and -out required; no positional arguments")
		}
		return executionpilot.WriteCadencePlan(*repo, *analyzer, *output, executionpilot.CadenceCell{
			NetworkLatencyNanos: int64(*network), PollIntervalNanos: int64(*poll), TargetQty: *target, Seed: *seed})
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		repo := flags.String("repo", ".", "clean source checkout")
		analyzer := flags.String("analyzer", "", "pinned analyzer binary")
		plan := flags.String("plan", "", "locked plan file")
		output := flags.String("out", "", "fresh output directory")
		timeout := flags.Duration("timeout", 2*time.Minute, "maximum world wall time")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *analyzer == "" || *plan == "" || *output == "" || *timeout <= 0 || flags.NArg() != 0 {
			return fmt.Errorf("mecadence run: -analyzer, -plan, -out and positive -timeout required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		manifest, err := executionpilot.RunCadenceFromPlan(ctx, *repo, *analyzer, *plan, *output)
		if err != nil {
			return err
		}
		fmt.Printf("completed evidence hash %s frames %d\n", manifest.Evidence.ExecutionHash, manifest.Evidence.FrameCount)
		return nil
	case "analyze":
		flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
		repo := flags.String("repo", ".", "clean source checkout")
		simulator := flags.String("simulator", "", "pinned simulator binary")
		plan := flags.String("plan", "", "locked plan file")
		runDir := flags.String("run", "", "completed run directory")
		output := flags.String("out", "", "new analysis result file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *simulator == "" || *plan == "" || *runDir == "" || *output == "" || flags.NArg() != 0 {
			return fmt.Errorf("mecadence analyze: -simulator, -plan, -run and -out required")
		}
		planRaw, err := os.ReadFile(*plan)
		if err != nil {
			return err
		}
		locked, _, err := executionpilot.DecodeCadencePlan(planRaw)
		if err != nil {
			return err
		}
		reconstruction, manifest, err := executionpilot.AnalyzeCadenceRun(*repo, *simulator, *plan, *runDir)
		if err != nil {
			return err
		}
		if err := executionpilot.WriteCadenceResult(*output, reconstruction, manifest, locked.Cell); err != nil {
			return err
		}
		fmt.Printf("reconstructed %s for client %d\n", reconstruction.Outcome.Status, reconstruction.Outcome.ClientID)
		return nil
	default:
		return fmt.Errorf("mecadence: unknown subcommand %q", args[0])
	}
}
