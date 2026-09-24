package executionpilot

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInstructionPreArrivalComparisonExcludesLaterExecution(t *testing.T) {
	depth := int64(500_000_000)
	ioc := ReconstructedOutcome{DecisionAt: 1_000_000_000, DecisionMid: 5_000_000_000,
		DeliveredSnapshotSeq: 100, CapBoundedAskQty: &depth, OrderSentAt: 1_000_000_000,
		VenueArrivalAt: 1_001_000_000, InitialABC: 100, InitialUSD: 1000}
	fok := ioc
	fok.Status = OutcomeRejected
	fok.FilledQty = 0
	ioc.Status = OutcomePartiallyFilled
	ioc.FilledQty = 480_395_196
	if !reflect.DeepEqual(preArrivalInstruction(ioc), preArrivalInstruction(fok)) {
		t.Fatal("post-arrival treatment response polluted pre-arrival comparison")
	}
	fok.DeliveredSnapshotSeq++
	if reflect.DeepEqual(preArrivalInstruction(ioc), preArrivalInstruction(fok)) {
		t.Fatal("changed selected quote identity passed as aligned")
	}
	fok.DeliveredSnapshotSeq--
	fok.VenueArrivalAt++
	if reflect.DeepEqual(preArrivalInstruction(ioc), preArrivalInstruction(fok)) {
		t.Fatal("changed venue arrival passed as aligned")
	}
}

func TestInstructionDiagnosticsRejectsIncompleteSurface(t *testing.T) {
	path := filepath.Join(t.TempDir(), "surface.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"assigned_worlds":12}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiagnoseInstructionSurface(path); err == nil {
		t.Fatal("incomplete all-assigned surface was diagnosed as valid")
	}
}
