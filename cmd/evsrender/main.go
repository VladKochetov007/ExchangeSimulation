// Command evsrender reconstructs the routed JSONL evidence layout from a
// completed evstream_v3 run. It writes files below -out and emits only a
// compact attestation report on stdout.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"exchange_sim/simulations/multivenue"
)

func main() {
	inputDir := flag.String("dir", "", "run directory containing events.evs")
	outputDir := flag.String("out", "", "empty directory receiving venues/<venue>/<route>.jsonl")
	flag.Parse()
	if *inputDir == "" || *outputDir == "" {
		fmt.Fprintln(os.Stderr, "evsrender: -dir and -out are required")
		os.Exit(2)
	}
	report, err := multivenue.RenderBinaryEvidence(*inputDir, *outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evsrender: %v\n", err)
		os.Exit(1)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evsrender: marshal report: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}
