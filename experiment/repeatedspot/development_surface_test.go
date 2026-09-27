package repeatedspot

import (
	"fmt"
	"strings"
	"testing"

	"exchange_sim/analysis"
	"exchange_sim/experiment/executionpilot"
)

func syntheticE0SurfaceInputs() []E0CellArtifacts {
	window := E0MeasurementWindow()
	identity := executionpilot.Identity{SourceCommit: strings.Repeat("1", 40)}
	evidence := EvidenceIdentity{SchemaID: EvidenceSchemaID, ExecutionHash: strings.Repeat("a", 64), FrameCount: 1}
	inputs := make([]E0CellArtifacts, 0, len(E0DevelopmentCells()))
	for _, cell := range E0DevelopmentCells() {
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
		replay := &EconomicReplay{Evidence: evidence, Accounts: accounts, InformationAudit: &analysis.MarketDataReceiptAudit{Valid: true},
			TerminalMarkStatus: "UNAVAILABLE_ONE_SIDED_OR_EMPTY", MeasurementWindow: window,
			Market: MarketSummary{HorizonNanos: window.EndAt, EmptyNanos: window.EndAt}}
		inputs = append(inputs, E0CellArtifacts{Result: E0AnalyzedResult{Cell: cell, Identity: identity,
			ManifestFileSHA256: strings.Repeat("b", 64), EvidenceFileSHA256: strings.Repeat("c", 64),
			EconomicReconstruction: replay},
			Diagnostic: E0LiquidityDiagnostic{Evidence: evidence, MeasurementStartNanos: window.StartAt,
				MeasurementEndNanos: window.EndAt, WorldEndNanos: window.EndAt, FirstPermanentEmptyNanos: int64Pointer(0),
				BookDurations: BookStateDurations{HorizonNanos: window.EndAt, EmptyNanos: window.EndAt}, Makers: makers},
			ResultSHA256: strings.Repeat("d", 64), DiagnosticSHA256: strings.Repeat("e", 64)})
	}
	return inputs
}

func TestSummarizeE0DevelopmentPreservesUnpriceableCells(t *testing.T) {
	inputs := syntheticE0SurfaceInputs()
	surface, err := SummarizeE0Development(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if surface.ValidEconomicCells != 24 || surface.PrimaryContrastStatus != "NOT_IDENTIFIED" ||
		surface.PrimaryAvailablePairs != 0 || !surface.AllCellsTerminalMarkUnavailable ||
		!surface.AllCellsPermanentlyEmptyBeforeWindow || len(surface.PrimaryPairs) != 3 ||
		surface.Cells[0].MakerNoUsableDecisions != 4 || surface.Cells[0].MakerGainSumQuoteAtoms != nil {
		t.Fatalf("unexpected unpriceable surface: %+v", surface)
	}

	inputs[1] = inputs[0]
	if _, err := SummarizeE0Development(inputs); err == nil {
		t.Fatal("duplicate cell accepted")
	}
	if _, err := SummarizeE0Development(inputs[:23]); err == nil {
		t.Fatal("incomplete matrix accepted")
	}
}

func TestSummarizeE0DevelopmentKeepsAvailablePairDescriptive(t *testing.T) {
	inputs := syntheticE0SurfaceInputs()
	for index := range inputs {
		cell := inputs[index].Result.Cell
		if cell.QuoteQty != e0QuoteSmall || cell.Seed != e0DevelopmentSeeds[0] ||
			(cell.Composition != "P" && cell.Composition != "A") {
			continue
		}
		replay := inputs[index].Result.EconomicReconstruction
		mid := int64(100)
		replay.TerminalMidQuote = &mid
		replay.TerminalMarkStatus = "AVAILABLE_TWO_SIDED_MID"
		replay.Market.EmptyNanos = 0
		replay.Market.TwoSidedNanos = replay.Market.HorizonNanos
		inputs[index].Diagnostic.FirstPermanentEmptyNanos = nil
		inputs[index].Diagnostic.BookDurations.EmptyNanos = 0
		inputs[index].Diagnostic.BookDurations.TwoSidedNanos = replay.Market.HorizonNanos
		for account := range replay.Accounts {
			gain := int64(10)
			if cell.Composition == "A" {
				gain = 20
			}
			replay.Accounts[account].BenchmarkGain = &gain
		}
	}
	surface, err := SummarizeE0Development(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if surface.PrimaryContrastStatus != "NOT_IDENTIFIED" || surface.PrimaryAvailablePairs != 1 ||
		surface.PrimaryPairs[0].DeltaNumeratorQuoteAtoms == nil ||
		*surface.PrimaryPairs[0].DeltaNumeratorQuoteAtoms != "40" ||
		surface.PrimaryPairs[1].DeltaNumeratorQuoteAtoms != nil || surface.AllCellsTerminalMarkUnavailable {
		t.Fatalf("incorrect partial-pair classification: %+v", surface.PrimaryPairs)
	}
}
