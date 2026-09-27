package repeatedspot

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"exchange_sim/simulation"
	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestME016MatrixIsFiniteAndRejectsOutOfScopeAssignments(t *testing.T) {
	cells := ME016DevelopmentCells()
	if len(cells) != 12 || cells[0] != (ME016Cell{Composition: "M1", SignalGainBps: 0, Seed: 18_101}) {
		t.Fatalf("wrong ME-016 development matrix or preflight: %+v", cells)
	}
	seen := make(map[string]bool)
	for _, cell := range cells {
		if seen[cell.ID()] {
			t.Fatalf("duplicate ME-016 cell %s", cell.ID())
		}
		seen[cell.ID()] = true
		if _, err := DraftME016Config(cell); err != nil {
			t.Fatalf("registered ME-016 cell rejected: %s: %v", cell.ID(), err)
		}
	}
	for _, invalid := range []ME016Cell{
		{Composition: "A", SignalGainBps: 0, Seed: 18_101},
		{Composition: "M1", SignalGainBps: 3, Seed: 18_101},
		{Composition: "M1", SignalGainBps: 0, Seed: 619},
	} {
		if _, err := DraftME016Config(invalid); err == nil {
			t.Fatalf("unregistered ME-016 assignment accepted: %+v", invalid)
		}
	}
}

func TestME016SignalGainChangesOnlyAssignedPolicyParameters(t *testing.T) {
	for _, composition := range []string{"M1", "M2"} {
		contracts := make([]map[string]any, 2)
		for index, gain := range []int64{0, 2} {
			world, err := BuildME016World(ME016Cell{Composition: composition, SignalGainBps: gain, Seed: 18_101})
			if err != nil {
				t.Fatal(err)
			}
			contract := world.ContractJSON()
			world.Close()
			if err := json.Unmarshal(contract, &contracts[index]); err != nil {
				t.Fatal(err)
			}
			var typed replayContract
			if err := json.Unmarshal(contract, &typed); err != nil {
				t.Fatal(err)
			}
			if typed.Step != int64(time.Second) || typed.Iterations != 3300 || len(typed.Participants) != 11 {
				t.Fatalf("ME-016 changed horizon/roster: %+v", typed)
			}
			signalSlots := map[int]bool{2: true, 4: true}
			if composition == "M2" {
				signalSlots = map[int]bool{1: true, 3: true}
			}
			for slot := 1; slot <= 4; slot++ {
				participant := typed.Participants[slot]
				if signalSlots[slot] && participant.Policy.Name != "bounded_imbalance_stoikov_maker_v1" ||
					!signalSlots[slot] && participant.Policy.Name != "bounded_fixed_maker_v3" {
					t.Fatalf("unexpected ME-016 maker role at slot %d: %s", slot, participant.Policy.Name)
				}
				if participant.Balances["ABC"] != 50*e0BasePrecision ||
					participant.Balances["USD"] != 2_500_000*e0QuotePrecision {
					t.Fatalf("unequal maker resources at slot %d: %+v", slot, participant.Balances)
				}
			}
		}
		participants := contracts[1]["participants"].([]any)
		for _, participantValue := range participants {
			participant := participantValue.(map[string]any)
			policy := participant["policy"].(map[string]any)
			if policy["name"] != "bounded_imbalance_stoikov_maker_v1" {
				continue
			}
			parameters := policy["parameters"].(map[string]any)
			if parameters["signal_gain_bps"] != float64(2) {
				t.Fatalf("treatment signal gain not bound in effective contract: %+v", parameters)
			}
			parameters["signal_gain_bps"] = float64(0)
		}
		if !reflect.DeepEqual(contracts[0], contracts[1]) {
			t.Fatalf("ME-016 %s gain contrast changed more than signal_gain_bps", composition)
		}
	}
}

func TestME016ShortSyntheticFixtureCapturesAndReplaysV5(t *testing.T) {
	for _, gain := range []int64{0, 2} {
		config, err := DraftME016Config(ME016Cell{Composition: "M1", SignalGainBps: gain, Seed: 18_101})
		if err != nil {
			t.Fatal(err)
		}
		config.Iterations = 40 // synthetic contract fixture, not a 55-minute economic cell
		world, err := worldspot.Build(config)
		if err != nil {
			t.Fatal(err)
		}
		directory := t.TempDir()
		receipts, err := simulation.NewMarketDataReceiptRecorder(directory)
		if err != nil {
			t.Fatal(err)
		}
		var raw bytes.Buffer
		capture, err := NewCapture(world, &raw, receipts)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := capture.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if identity.SchemaID != SignalEvidenceSchemaID {
			t.Fatalf("ME-016 policy captured with historical schema: %+v", identity)
		}
		replay, err := ReplayWindow(world.ContractJSON(), bytes.NewReader(raw.Bytes()), identity, directory,
			MeasurementWindow{StartAt: 0, EndAt: int64(40 * time.Second)})
		if err != nil || replay == nil || len(replay.Accounts) != 11 || replay.InformationAudit == nil ||
			!replay.InformationAudit.Valid {
			t.Fatalf("ME-016 synthetic v5 evidence did not reconstruct: %+v, %v", replay, err)
		}
		if replay.SignalAudit == nil || len(replay.SignalAudit.Makers) != 2 ||
			len(replay.SignalAudit.Snapshots) == 0 ||
			replay.SignalAudit.Window.TwoSidedNanos != replay.PublicWindowDepth.TwoSidedNanos {
			t.Fatalf("ME-016 synthetic signal provenance/window missing: %+v", replay.SignalAudit)
		}
		for _, snapshot := range replay.SignalAudit.Snapshots {
			if snapshot.OwnBidQty > snapshot.PublicBidQty || snapshot.OwnAskQty > snapshot.PublicAskQty ||
				snapshot.SourceAt > snapshot.DeliveredAt {
				t.Fatalf("source-time owned depth or delivery invalid: %+v", snapshot)
			}
		}
		if raw.Len() < 8 {
			t.Fatal("synthetic v5 evidence too small for truncation fixture")
		}
		if _, err := ReplayWindow(world.ContractJSON(), bytes.NewReader(raw.Bytes()[:raw.Len()-7]), identity,
			directory, MeasurementWindow{StartAt: 0, EndAt: int64(40 * time.Second)}); err == nil {
			t.Fatal("truncated v5 evidence behind a valid contract and sidecars passed replay")
		}
	}
}
