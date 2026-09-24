package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	root := flag.String("root", "", "retained ME-003 development namespace")
	repo := flag.String("repo", ".", "clean aggregation source checkout")
	source := flag.String("execution-source", "", "pinned ME-003 execution commit")
	output := flag.String("out", "", "new instruction-response surface JSON file")
	flag.Parse()
	if *root == "" || *source == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "meinstructionsurface: -root, -execution-source and -out are required")
		os.Exit(2)
	}
	surface, err := executionpilot.AggregateInstructionRuns(*root, *repo, *source)
	if err == nil {
		err = executionpilot.WriteInstructionSurface(*output, surface)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("replayed %d development worlds and %d paired contrasts\n", surface.ValidWorlds, len(surface.PairedContrasts))
}
