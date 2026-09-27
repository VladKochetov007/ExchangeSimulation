package repeatedspot

import (
	"fmt"
	"strings"
	"testing"

	"exchange_sim/analysis"
	"exchange_sim/experiment/executionpilot"
)

func syntheticE0ReferenceInputs() []E0CellArtifacts {
	window := E0MeasurementWindow()
	identity := executionpilot.Identity{SourceCommit: strings.Repeat("1", 40)}
	evidence := EvidenceIdentity{SchemaID: EvidenceSchemaID, ExecutionHash: strings.Repeat("a", 64), FrameCount: 1}
	inputs := make([]E0CellArtifacts, 0, len(E0LocalReferenceDevelopmentCells()))
	for _, cell := range E0LocalReferenceDevelopmentCells() {
		accounts := make([]AccountResult, 0, 4)
		makers := make([]MakerLiquidityActions, 0, 4)
		for slot := 1; slot <= 4; slot++ {
			actorID := uint64(slot + 1)
			accounts = append(accounts, AccountResult{ActorID: actorID, Role: fmt.Sprintf("maker_slot_%d", slot),
				MakerEnvelope: &MakerEnvelopeSummary{WindowNanos: window.EndAt - window.StartAt},
				RestingDepth:  RestingDepthSummary{WindowNanos: window.EndAt - window.StartAt}})
			makers = append(makers, MakerLiquidityActions{ActorID: actorID,
				DuringMeasurement: map[string]int64{"no_usable_quote": 1}})
		}
		replay := &EconomicReplay{Evidence: evidence, Accounts: accounts,
			InformationAudit:   &analysis.MarketDataReceiptAudit{Valid: true},
			TerminalMarkStatus: "UNAVAILABLE_ONE_SIDED_OR_EMPTY", MeasurementWindow: window,
			Market:            MarketSummary{HorizonNanos: window.EndAt, EmptyNanos: window.EndAt},
			PublicWindowDepth: RestingDepthSummary{WindowNanos: window.EndAt - window.StartAt},
			MakerWindowDepth:  RestingDepthSummary{WindowNanos: window.EndAt - window.StartAt}}
		inputs = append(inputs, E0CellArtifacts{Result: E0AnalyzedResult{Cell: cell, Identity: identity,
			ManifestFileSHA256: strings.Repeat("b", 64), EvidenceFileSHA256: strings.Repeat("c", 64),
			EconomicReconstruction: replay},
			Diagnostic: E0LiquidityDiagnostic{Evidence: evidence, MeasurementStartNanos: window.StartAt,
				MeasurementEndNanos: window.EndAt, WorldEndNanos: window.EndAt,
				ShadowReferenceMaxAgeNanos: int64(E0LocalReferenceMaxAge),
				BookDurations:              BookStateDurations{HorizonNanos: window.EndAt, EmptyNanos: window.EndAt},
				MeasurementBookDurations: BookStateDurations{HorizonNanos: window.EndAt - window.StartAt,
					EmptyNanos: window.EndAt - window.StartAt}, Makers: makers},
			ResultSHA256: strings.Repeat("d", 64), DiagnosticSHA256: strings.Repeat("e", 64)})
	}
	return inputs
}

func TestSummarizeE0LocalReferenceKeepsZeroAndMissingMarks(t *testing.T) {
	inputs := syntheticE0ReferenceInputs()
	surface, err := SummarizeE0LocalReference(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if surface.ValidEconomicCells != 24 || surface.PrimaryContrastStatus != "ESTIMATED_DEVELOPMENT_ONLY" ||
		surface.PrimaryMedianDeltaNanos != 0 || len(surface.PrimaryPairs) != 3 ||
		surface.TreatmentPAGainStatus != "NOT_IDENTIFIED" || surface.Cells[0].MakerGainSumQuoteAtoms != nil ||
		surface.Cells[0].MeasurementBookDurations.TwoSidedNanos != 0 {
		t.Fatalf("zero-opportunity world was lost or misclassified: %+v", surface)
	}
	inputs[1] = inputs[0]
	if _, err := SummarizeE0LocalReference(inputs); err == nil {
		t.Fatal("duplicate cell accepted")
	}
	if _, err := SummarizeE0LocalReference(inputs[:23]); err == nil {
		t.Fatal("incomplete matrix accepted")
	}
}

func TestSummarizeE0LocalReferenceRequiresWindowedIndependentNumerator(t *testing.T) {
	inputs := syntheticE0ReferenceInputs()
	window := E0MeasurementWindow()
	inputs[0].Diagnostic.MeasurementBookDurations.TwoSidedNanos = 100
	inputs[0].Diagnostic.MeasurementBookDurations.EmptyNanos = window.EndAt - window.StartAt - 100
	inputs[0].Result.EconomicReconstruction.PublicWindowDepth.TwoSidedNanos = 100
	inputs[0].Result.EconomicReconstruction.PublicWindowDepth.BidPresentNanos = 100
	inputs[0].Result.EconomicReconstruction.PublicWindowDepth.AskPresentNanos = 100
	inputs[0].Diagnostic.BookDurations.TwoSidedNanos = 100
	inputs[0].Diagnostic.BookDurations.EmptyNanos = window.EndAt - 100
	inputs[0].Result.EconomicReconstruction.Market.TwoSidedNanos = 100
	inputs[0].Result.EconomicReconstruction.Market.EmptyNanos = window.EndAt - 100
	surface, err := SummarizeE0LocalReference(inputs)
	if err != nil || surface.PrimaryPairs[0].OnMinusOffNanos != 100 || surface.PrimaryMaximumDeltaNanos != 100 {
		t.Fatalf("valid exact-window contrast failed: %+v, %v", surface.PrimaryPairs, err)
	}
	inputs[0].Diagnostic.MeasurementBookDurations.TwoSidedNanos++
	inputs[0].Diagnostic.MeasurementBookDurations.EmptyNanos--
	if _, err := SummarizeE0LocalReference(inputs); err == nil {
		t.Fatal("windowed public diagnostic diverged from independent resting depth")
	}
}

func TestSummarizeE0LocalReferenceRejectsFabricatedCachedControl(t *testing.T) {
	inputs := syntheticE0ReferenceInputs()
	for index := range inputs {
		if inputs[index].Result.Cell.ReferenceMode == "OFF" {
			inputs[index].Diagnostic.Makers[0].CachedReferenceDecisions = 1
			break
		}
	}
	if _, err := SummarizeE0LocalReference(inputs); err == nil {
		t.Fatal("OFF arm was credited with cached-reference decisions")
	}
}
