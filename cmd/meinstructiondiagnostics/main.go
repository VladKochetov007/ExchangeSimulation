package main

import (
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/executionpilot"
)

func main() {
	surface := flag.String("surface", "", "verified ME-003 response surface JSON")
	output := flag.String("out", "", "new instruction diagnostics JSON")
	flag.Parse()
	if *surface == "" || *output == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "meinstructiondiagnostics: -surface and -out are required")
		os.Exit(2)
	}
	diagnostic, err := executionpilot.DiagnoseInstructionSurface(*surface)
	if err == nil {
		err = executionpilot.WriteInstructionDiagnostics(*output, diagnostic)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("diagnosed %d matched ME-003 pairs\n", len(diagnostic.Pairs))
}
