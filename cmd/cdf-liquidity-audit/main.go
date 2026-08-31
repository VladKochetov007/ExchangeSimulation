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
	"exchange_sim/simulations/multivenue"
)

func main() {
	treatmentDir := flag.String("treatment", "", "original treatment run directory containing events.evs")
	controlDir := flag.String("control", "", "original paired control run directory containing events.evs")
	flag.Parse()
	if *treatmentDir == "" || *controlDir == "" {
		fmt.Fprintln(os.Stderr, "usage: cdf-liquidity-audit -treatment DIR -control DIR")
		os.Exit(2)
	}
	treatmentEvidence, err := os.MkdirTemp("", "cdf-liquidity-treatment-render-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create treatment render directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(treatmentEvidence)
	if _, err := multivenue.RenderBinaryEvidence(*treatmentDir, treatmentEvidence); err != nil {
		fmt.Fprintf(os.Stderr, "render treatment: %v\n", err)
		os.Exit(1)
	}
	controlEvidence, err := os.MkdirTemp("", "cdf-liquidity-control-render-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create control render directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(controlEvidence)
	if _, err := multivenue.RenderBinaryEvidence(*controlDir, controlEvidence); err != nil {
		fmt.Fprintf(os.Stderr, "render control: %v\n", err)
		os.Exit(1)
	}
	treatment, err := analysis.OpenRenderedRun(*treatmentDir, treatmentEvidence)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open treatment: %v\n", err)
		os.Exit(1)
	}
	control, err := analysis.OpenRenderedRun(*controlDir, controlEvidence)
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
