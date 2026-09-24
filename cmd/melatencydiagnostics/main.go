package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	surface := flag.String("surface", "", "verified ME-002 response surface")
	output := flag.String("out", "", "new diagnostics JSON")
	flag.Parse()
	if *surface == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "melatencydiagnostics: -surface and -out required")
		os.Exit(2)
	}
	diagnostics, err := executionpilot.DiagnoseLatencySurface(*surface)
	if err == nil {
		err = executionpilot.WriteLatencyDiagnostics(*output, diagnostics)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("diagnosed %d processing and %d network matched pairs\n", diagnostics.ProcessingMatchedPairs, diagnostics.NetworkMatchedPairs)
}
