package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	root := flag.String("root", "", "retained ME-002 development namespace")
	repo := flag.String("repo", ".", "clean aggregation source checkout")
	source := flag.String("execution-source", "", "pinned ME-002 execution commit")
	output := flag.String("out", "", "new response-surface JSON file")
	flag.Parse()
	if *root == "" || *source == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "melatencysurface: -root, -execution-source and -out are required")
		os.Exit(2)
	}
	surface, err := executionpilot.AggregateLatencyRuns(*root, *repo, *source)
	if err == nil {
		err = executionpilot.WriteLatencySurface(*output, surface)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("replayed %d development worlds and %d paired contrasts\n", surface.ValidWorlds, len(surface.PairedContrasts))
}
