// Command cdf-liquidity-audit audits a rendered evstream_v3 treatment and its
// paired no-CDF control. Rendering is deliberately a separate verified step:
// evsrender checks the binary completion trailer, execution hash, and sidecar
// merge before this analyzer reads the public JSON layout.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"exchange_sim/analysis"
)

func main() {
	treatmentDir := flag.String("treatment", "", "rendered treatment run directory")
	controlDir := flag.String("control", "", "rendered paired control run directory")
	flag.Parse()
	if *treatmentDir == "" || *controlDir == "" {
		fmt.Fprintln(os.Stderr, "usage: cdf-liquidity-audit -treatment DIR -control DIR")
		os.Exit(2)
	}
	treatment, err := analysis.Open(*treatmentDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open treatment: %v\n", err)
		os.Exit(1)
	}
	control, err := analysis.Open(*controlDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open control: %v\n", err)
		os.Exit(1)
	}
	comparison, err := analysis.CompareCDFLiquidityRuns(treatment, control)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit: %v\n", err)
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(comparison, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal audit: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
	if !comparison.Valid {
		os.Exit(1)
	}
}
