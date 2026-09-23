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
		return fmt.Errorf("usage: mepilot plan|run [flags]")
	}
	switch args[0] {
	case "plan":
		flags := flag.NewFlagSet("plan", flag.ContinueOnError)
		repo := flags.String("repo", ".", "clean source checkout")
		analyzer := flags.String("analyzer", "", "pinned analyzer binary")
		output := flags.String("out", "", "new plan file")
		makers := flags.Int("makers", 0, "maker count")
		takers := flags.Int("takers", 0, "random-taker count")
		quantity := flags.Int64("target", 0, "target base units")
		seed := flags.Int64("seed", 0, "development seed")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *analyzer == "" || *output == "" || flags.NArg() != 0 {
			return fmt.Errorf("mepilot plan: -analyzer and -out are required; positional arguments forbidden")
		}
		return executionpilot.WritePlan(*repo, *analyzer, *output, executionpilot.Cell{
			MakerCount: *makers, RandomTakerCount: *takers, TargetQty: *quantity, Seed: *seed,
		})
	case "run":
		flags := flag.NewFlagSet("run", flag.ContinueOnError)
		repo := flags.String("repo", ".", "clean source checkout")
		analyzer := flags.String("analyzer", "", "pinned analyzer binary")
		plan := flags.String("plan", "", "locked plan file")
		output := flags.String("out", "", "new output directory")
		timeout := flags.Duration("timeout", time.Minute, "maximum world wall time")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *analyzer == "" || *plan == "" || *output == "" || *timeout <= 0 || flags.NArg() != 0 {
			return fmt.Errorf("mepilot run: -analyzer, -plan, -out and positive -timeout are required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		manifest, err := executionpilot.RunFromPlan(ctx, *repo, *analyzer, *plan, *output)
		if err != nil {
			return err
		}
		fmt.Printf("completed evidence hash %s frames %d\n", manifest.Evidence.ExecutionHash, manifest.Evidence.FrameCount)
		return nil
	default:
		return fmt.Errorf("mepilot: unknown subcommand %q", args[0])
	}
}
