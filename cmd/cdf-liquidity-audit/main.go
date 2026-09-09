// Command cdf-liquidity-audit audits a rendered evstream_v3 treatment and its
// paired no-CDF control. Rendering is deliberately a separate verified step:
// evsrender checks the binary completion trailer, execution hash, and sidecar
// merge before this analyzer reads the public JSON layout.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"exchange_sim/analysis"
	"exchange_sim/simulations/multivenue"
)

func main() {
	treatmentDir := flag.String("treatment", "", "original treatment run directory containing events.evs")
	controlDir := flag.String("control", "", "original paired control run directory containing events.evs")
	analysisOnlyReplay := flag.Bool("analysis-only-replay", false, "rescore retained raw evidence with a clean descendant analyzer without rerunning the simulator")
	flag.Parse()
	if *treatmentDir == "" || *controlDir == "" {
		fmt.Fprintln(os.Stderr, "usage: cdf-liquidity-audit -treatment DIR -control DIR [-analysis-only-replay]")
		os.Exit(2)
	}
	treatmentEvidence, err := os.MkdirTemp("", "cdf-liquidity-treatment-render-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create treatment render directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(treatmentEvidence)
	treatmentRender, err := multivenue.RenderBinaryEvidence(*treatmentDir, treatmentEvidence)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render treatment: %v\n", err)
		os.Exit(1)
	}
	controlEvidence, err := os.MkdirTemp("", "cdf-liquidity-control-render-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create control render directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(controlEvidence)
	controlRender, err := multivenue.RenderBinaryEvidence(*controlDir, controlEvidence)
	if err != nil {
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
	comparison.Provenance.TreatmentExecutionHash = treatmentRender.ExecutionHash
	comparison.Provenance.TreatmentCanonicalHash = treatmentRender.CanonicalHash
	comparison.Provenance.TreatmentFullEvidenceHash = treatmentRender.FullEvidenceHash
	comparison.Provenance.ControlExecutionHash = controlRender.ExecutionHash
	comparison.Provenance.ControlCanonicalHash = controlRender.CanonicalHash
	comparison.Provenance.ControlFullEvidenceHash = controlRender.FullEvidenceHash
	if treatmentRender.ExecutionHash == "" || treatmentRender.CanonicalHash == "" || treatmentRender.FullEvidenceHash == "" ||
		controlRender.ExecutionHash == "" || controlRender.CanonicalHash == "" || controlRender.FullEvidenceHash == "" {
		invalidateComparisonProvenance(comparison, "binary rendering did not return complete execution and canonical evidence hashes")
	}
	analyzerPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "locate analyzer executable: %v\n", err)
		os.Exit(1)
	}
	analyzerRaw, err := os.ReadFile(analyzerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read analyzer executable: %v\n", err)
		os.Exit(1)
	}
	analyzerDigest := sha256.Sum256(analyzerRaw)
	comparison.Provenance.AnalyzerSHA256 = hex.EncodeToString(analyzerDigest[:])
	analyzerRevision, analyzerModified := analyzerBuild()
	applyAnalyzerProvenance(comparison, analyzerRevision, analyzerModified, *analysisOnlyReplay)
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

func applyAnalyzerProvenance(comparison *analysis.CDFLiquidityComparison, analyzerRevision string, analyzerModified, analysisOnlyReplay bool) {
	comparison.Provenance.AnalyzerSourceRevision = analyzerRevision
	comparison.Provenance.AnalyzerSourceModified = analyzerModified
	if analysisOnlyReplay {
		comparison.Provenance.SourceRevisionMode = "analysis_only_replay"
		comparison.Provenance.RawSourceRevision = comparison.Provenance.Treatment.SourceRevision
	} else {
		comparison.Provenance.SourceRevisionMode = "pinned_live"
	}

	isAnalyzerClean := analyzerRevision != "unknown" && !analyzerModified
	isSourcePaired := comparison.Provenance.Treatment != nil && comparison.Provenance.Control != nil &&
		comparison.Provenance.Treatment.SourceRevision == comparison.Provenance.Control.SourceRevision
	if analysisOnlyReplay {
		// A replay may use a descendant analyzer, but it still requires the
		// immutable raw pair to agree and the replacement analyzer to be clean.
		if !isAnalyzerClean || !isSourcePaired {
			invalidateComparisonProvenance(comparison, "analysis-only replay is not bound to a clean analyzer and paired raw source")
		}
		return
	}
	if !isAnalyzerClean || !isSourcePaired || analyzerRevision != comparison.Provenance.Treatment.SourceRevision {
		invalidateComparisonProvenance(comparison, "analyzer binary is not provenance-pinned to the paired clean source revision")
	}
}

func invalidateComparisonProvenance(comparison *analysis.CDFLiquidityComparison, failure string) {
	comparison.Provenance.Valid = false
	comparison.Provenance.Failure = failure
	comparison.EvidenceValid = false
	comparison.ActivationSatisfied = false
	comparison.AntiCheatingSatisfied = false
	comparison.Valid = false
}

func analyzerBuild() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown", true
	}
	revision := "unknown"
	modified := true
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}
