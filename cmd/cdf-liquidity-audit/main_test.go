package main

import (
	"encoding/json"
	"testing"

	"exchange_sim/analysis"
)

func pairedCDFComparisonForProvenanceTest() *analysis.CDFLiquidityComparison {
	return &analysis.CDFLiquidityComparison{
		Provenance: analysis.CDFLiquidityComparisonProvenance{
			Treatment: &analysis.CDFLiquidityRunProvenance{SourceRevision: "raw-revision"},
			Control:   &analysis.CDFLiquidityRunProvenance{SourceRevision: "raw-revision"},
			Valid:     true,
		},
		EvidenceValid:         true,
		ActivationSatisfied:   true,
		AntiCheatingSatisfied: true,
		Valid:                 true,
	}
}

func TestApplyAnalyzerProvenanceLiveRequiresExactSource(t *testing.T) {
	comparison := pairedCDFComparisonForProvenanceTest()
	applyAnalyzerProvenance(comparison, "descendant-revision", false, false)
	if comparison.Valid || comparison.Provenance.Valid {
		t.Fatal("live analysis accepted a descendant analyzer revision")
	}
	if comparison.Provenance.SourceRevisionMode != "pinned_live" || comparison.Provenance.RawSourceRevision != "" {
		t.Fatalf("live provenance mode = %+v", comparison.Provenance)
	}
}

func TestApplyAnalyzerProvenanceAnalysisOnlyReplayAllowsCleanDescendant(t *testing.T) {
	comparison := pairedCDFComparisonForProvenanceTest()
	applyAnalyzerProvenance(comparison, "descendant-revision", false, true)
	if !comparison.Valid || !comparison.Provenance.Valid || !comparison.EvidenceValid {
		t.Fatalf("analysis-only replay was rejected: %+v", comparison.Provenance)
	}
	if comparison.Provenance.SourceRevisionMode != "analysis_only_replay" || comparison.Provenance.RawSourceRevision != "raw-revision" {
		t.Fatalf("replay provenance mode = %+v", comparison.Provenance)
	}
}

func TestApplyAnalyzerProvenanceAnalysisOnlyReplayRejectsModifiedAnalyzer(t *testing.T) {
	comparison := pairedCDFComparisonForProvenanceTest()
	applyAnalyzerProvenance(comparison, "descendant-revision", true, true)
	if comparison.Valid || comparison.Provenance.Valid {
		t.Fatal("analysis-only replay accepted a modified analyzer")
	}
}

func TestCDFComparisonProvenanceRendersExplicitAnalyzerModifiedFlag(t *testing.T) {
	comparison := pairedCDFComparisonForProvenanceTest()
	applyAnalyzerProvenance(comparison, "raw-revision", false, false)
	encoded, err := json.Marshal(comparison)
	if err != nil {
		t.Fatalf("marshal comparison: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal comparison: %v", err)
	}
	var provenance map[string]json.RawMessage
	if err := json.Unmarshal(document["provenance"], &provenance); err != nil {
		t.Fatalf("unmarshal provenance: %v", err)
	}
	modified, ok := provenance["analyzer_source_modified"]
	if !ok || string(modified) != "false" {
		t.Fatalf("analyzer_source_modified = %s, want explicit false", modified)
	}
}
