package repeatedspot

import (
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/analysis"
	"exchange_sim/experiment/executionpilot"
)

func syntheticME016SurfaceArtifacts() map[ME016Cell]me016SurfaceArtifact {
	window := E0MeasurementWindow()
	denominator := window.EndAt - window.StartAt
	identity := executionpilot.Identity{SourceCommit: "source", EvidenceSchemaID: SignalEvidenceSchemaID}
	artifacts := make(map[ME016Cell]me016SurfaceArtifact, len(ME016DevelopmentCells()))
	for _, cell := range ME016DevelopmentCells() {
		twoSided := int64(1_000_000_000_000)
		if cell.SignalGainBps == 2 {
			switch cell.Seed {
			case 18_101:
				twoSided += 5_000_000_000
			case 18_111:
				twoSided -= 2_000_000_000
			}
		}
		makerIDs := []uint64{3, 5}
		if cell.Composition == "M2" {
			makerIDs = []uint64{2, 4}
		}
		audit := &ME016SignalAudit{Window: SignalWindowBookSummary{WindowNanos: denominator,
			BidPresentNanos: twoSided, AskPresentNanos: twoSided, TwoSidedNanos: twoSided,
			BestBidBaseUnitNanos: "100", BestAskBaseUnitNanos: "100",
			MakerBestBidBaseUnitNanos: "100", MakerBestAskBaseUnitNanos: "100",
			SeedBestBidBaseUnitNanos: "0", SeedBestAskBaseUnitNanos: "0",
			OtherBestBidBaseUnitNanos: "0", OtherBestAskBaseUnitNanos: "0",
			SpreadPriceUnitNanos: "100"}}
		for _, id := range makerIDs {
			funnel := SignalMakerFunnel{ClientID: id, BaseQuoteUsable: 1, ShadowGain2Changes: 1}
			if cell.SignalGainBps == 2 {
				funnel.ActualShiftedTargets = 1
			}
			audit.Makers = append(audit.Makers, funnel)
		}
		book := RestingDepthSummary{WindowNanos: denominator, BidPresentNanos: twoSided,
			AskPresentNanos: twoSided, TwoSidedNanos: twoSided,
			BidDepthBaseUnitNanos: "100", AskDepthBaseUnitNanos: "100"}
		replay := &EconomicReplay{ContractSHA256: "contract", Evidence: EvidenceIdentity{SchemaID: SignalEvidenceSchemaID},
			InformationAudit: &analysis.MarketDataReceiptAudit{Valid: true}, MeasurementWindow: window,
			PublicWindowDepth: book, SignalAudit: audit, TerminalMarkStatus: "UNAVAILABLE_ONE_SIDED_OR_EMPTY"}
		for id := uint64(2); id <= 5; id++ {
			replay.Accounts = append(replay.Accounts, AccountResult{ActorID: id, ClientID: id,
				Role:          "maker_slot_" + string(rune('0'+id-1)),
				MakerEnvelope: &MakerEnvelopeSummary{WindowNanos: denominator}})
		}
		manifest := ME016RunManifest{ContractSHA256: "contract", Evidence: replay.Evidence}
		artifacts[cell] = me016SurfaceArtifact{plan: ME016LockedPlan{TypedPlanSHA256: "typed"},
			manifest: manifest, result: ME016AnalyzedResult{SchemaVersion: 1, Cell: cell,
				Identity: identity, EconomicReconstruction: replay},
			planHash: "plan", manifestHash: "manifest", resultHash: "result",
			runResourceHash: "run", analysisHash: "analysis"}
	}
	return artifacts
}

func TestME016SurfacePreservesPairedWorldContrastsAndUnavailableMarks(t *testing.T) {
	surface, err := summarizeME016Artifacts(ME016DevelopmentCells(), syntheticME016SurfaceArtifacts())
	if err != nil {
		t.Fatal(err)
	}
	if surface.ValidEconomicCells != 12 || len(surface.PrimaryM1Pairs) != 3 || len(surface.SensitivityM2Pairs) != 3 ||
		surface.PrimaryMinimumDeltaNanos != -2_000_000_000 || surface.PrimaryMedianDeltaNanos != 0 ||
		surface.PrimaryMaximumDeltaNanos != 5_000_000_000 || surface.TerminalMarksAvailable != 0 {
		t.Fatalf("ME-016 synthetic surface changed: %+v", surface)
	}
}

func TestME016SurfaceRejectsMissingCellAndContradictoryEvidence(t *testing.T) {
	first := ME016DevelopmentCells()[0]
	tests := []struct {
		name   string
		mutate func(map[ME016Cell]me016SurfaceArtifact)
	}{
		{"missing-cell", func(values map[ME016Cell]me016SurfaceArtifact) { delete(values, first) }},
		{"window-mismatch", func(values map[ME016Cell]me016SurfaceArtifact) {
			values[first].result.EconomicReconstruction.PublicWindowDepth.TwoSidedNanos++
		}},
		{"owner-depth-mismatch", func(values map[ME016Cell]me016SurfaceArtifact) {
			values[first].result.EconomicReconstruction.SignalAudit.Window.MakerBestBidBaseUnitNanos = "99"
		}},
		{"gain-zero-actual-shift", func(values map[ME016Cell]me016SurfaceArtifact) {
			values[first].result.EconomicReconstruction.SignalAudit.Makers[0].ActualShiftedTargets = 1
		}},
		{"invented-terminal-gain", func(values map[ME016Cell]me016SurfaceArtifact) {
			gain := int64(1)
			values[first].result.EconomicReconstruction.Accounts[0].BenchmarkGain = &gain
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := syntheticME016SurfaceArtifacts()
			test.mutate(values)
			if _, err := summarizeME016Artifacts(ME016DevelopmentCells(), values); err == nil {
				t.Fatal("contradictory ME-016 surface passed")
			}
		})
	}
}

func TestME016SurfaceRunInventoryRejectsUnregisteredAttempt(t *testing.T) {
	root := t.TempDir()
	for _, cell := range ME016DevelopmentCells() {
		if err := os.MkdirAll(filepath.Join(root, "cells", cell.ID()+"-run"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	control := ME016DevelopmentCells()[0].ID() + "-g1-control-run"
	if err := os.Mkdir(filepath.Join(root, "cells", control), 0700); err != nil {
		t.Fatal(err)
	}
	if err := requireME016RunInventory(root, ME016DevelopmentCells()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "cells", "unregistered-run"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := requireME016RunInventory(root, ME016DevelopmentCells()); err == nil {
		t.Fatal("unregistered economic attempt was silently omitted")
	}
}
