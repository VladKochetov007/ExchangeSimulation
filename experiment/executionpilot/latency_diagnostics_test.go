package executionpilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLatencyDiagnosticsSeparateDeploymentFromSelectedQuote(t *testing.T) {
	surface := LatencySurface{SchemaVersion: 1, AssignedWorlds: 24, ValidWorlds: 24}
	for _, cell := range LatencyCells() {
		id, err := latencyCellID(cell)
		if err != nil {
			t.Fatal(err)
		}
		fill := cell.TargetQty
		if cell.NetworkLatencyNanos == 90_000_000 {
			fill /= 2
		}
		selected := uint64(1)
		if cell.ProcessingDelayNanos > 0 {
			selected = 2
		}
		status := OutcomeFullyFilled
		if fill < cell.TargetQty {
			status = OutcomePartiallyFilled
		}
		surface.Cells = append(surface.Cells, LatencyCellResult{ID: id, Cell: cell,
			FilledFraction: float64(fill) / float64(cell.TargetQty),
			Outcome: ReconstructedOutcome{Status: status, DecisionAt: 1_000_000_000,
				VenueArrivalAt: 1_000_000_000 + cell.NetworkLatencyNanos, DeliveredSnapshotSeq: selected,
				FilledQty: fill, TargetQty: cell.TargetQty, TerminalMarkAvailable: true,
				ActionTiming: &ActionTiming{PublicationToVenueArrivalNanos: cell.NetworkLatencyNanos + cell.ProcessingDelayNanos,
					DecisionToVenueArrivalNanos: cell.NetworkLatencyNanos},
				SelectedOpportunity: &ObservedOpportunity{SelectedSnapshotSeq: selected, QualifyingAtPublication: true}}})
	}
	for _, target := range []int64{50_000_000, 500_000_000} {
		for _, seed := range []int64{12001, 12011, 12017} {
			surface.PairedContrasts = append(surface.PairedContrasts, latencyPairedContrast(target, seed, [4]float64{1, 1, 0.5, 0.5}))
		}
	}
	path := filepath.Join(t.TempDir(), "surface.json")
	raw, err := json.Marshal(surface)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := DiagnoseLatencySurface(path)
	if err != nil {
		t.Fatal(err)
	}
	if !diagnostics.AllDecisionsAtOneSecond || diagnostics.ProcessingMatchedPairs != 12 ||
		diagnostics.ProcessingChangedOrderArrival != 0 || diagnostics.ProcessingChangedSelectedSnapshot != 12 ||
		diagnostics.ProcessingChangedFilledQty != 0 || diagnostics.NetworkMatchedPairs != 12 ||
		diagnostics.NetworkChangedOrderArrival != 12 || diagnostics.NetworkChangedFilledQty != 12 ||
		len(diagnostics.ArmSummaries) != 8 {
		t.Fatalf("timing decomposition=%+v", diagnostics)
	}
}
