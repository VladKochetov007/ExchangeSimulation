package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	root := flag.String("root", "", "retained ME-001 development namespace")
	repo := flag.String("repo", ".", "clean aggregation source checkout")
	source := flag.String("execution-source", "", "registered execution commit")
	output := flag.String("out", "", "new response-surface JSON file")
	flag.Parse()
	if *root == "" || *source == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "mepilotsurface: -root, -execution-source and -out are required")
		os.Exit(2)
	}
	surface, err := executionpilot.AggregateRuns(*root, *repo, *source, []int64{1009, 1013, 1019})
	if err == nil {
		err = executionpilot.WriteSurface(*output, surface)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("replayed %d development worlds across %d registered contrasts\n", len(surface.Cells), len(surface.Contrasts))
}
