package repeatedspot

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLocalReferenceMatrixUsesOneSourceAndOnlyChangesTheReferenceRule(t *testing.T) {
	legacyCell := E0DevelopmentCells()[0]
	legacyJSON, err := json.Marshal(legacyCell)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacyJSON) != `{"composition":"P","quote_qty_base_units":10000000,"seed":18001}` ||
		legacyCell.ID() != "P-q10000000-s18001" {
		t.Fatalf("historical r1 cell identity changed: %s, %s", legacyJSON, legacyCell.ID())
	}
	cells := E0LocalReferenceDevelopmentCells()
	if len(cells) != 24 || cells[0] != (E0Cell{Composition: "P", QuoteQty: e0QuoteSmall,
		Seed: e0LocalReferenceSeeds[0], ReferenceMode: "ON"}) {
		t.Fatalf("wrong prospective ME-015 matrix or preflight: %+v", cells)
	}
	seen := make(map[string]bool)
	for _, cell := range cells {
		if seen[cell.ID()] {
			t.Fatalf("duplicate cell %s", cell.ID())
		}
		seen[cell.ID()] = true
		if err := validateE0LocalReferenceCell(cell); err != nil {
			t.Fatal(err)
		}
	}
	on := cells[0]
	off := on
	off.ReferenceMode = "OFF"
	identity := syntheticE0Identity()
	onPlan, err := LockE0Plan(on, identity)
	if err != nil {
		t.Fatal(err)
	}
	offPlan, err := LockE0Plan(off, identity)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		plan E0LockedPlan
		age  int64
	}{
		{onPlan, int64(15 * time.Second)},
		{offPlan, 0},
	} {
		world, err := VerifyE0Plan(entry.plan, identity)
		if err != nil {
			t.Fatal(err)
		}
		world.Close()
		var contract replayContract
		if err := json.Unmarshal(entry.plan.EffectiveWorld, &contract); err != nil {
			t.Fatal(err)
		}
		if len(contract.Participants) != 11 {
			t.Fatalf("successor changed the finite roster: %d", len(contract.Participants))
		}
		for _, maker := range contract.Participants[1:5] {
			if maker.Policy.Name != "bounded_fixed_maker_v3" {
				t.Fatalf("control/treatment used different maker policy types: %s", maker.Policy.Name)
			}
			var parameters struct {
				MaxAge int64 `json:"local_reference_max_age_ns"`
			}
			if err := json.Unmarshal(maker.Policy.Parameters, &parameters); err != nil || parameters.MaxAge != entry.age {
				t.Fatalf("unbound local reference lifetime: %+v, %v", parameters, err)
			}
		}
	}
	mutated := onPlan
	mutated.Cell.ReferenceMode = "OFF"
	if world, err := VerifyE0Plan(mutated, identity); err == nil {
		world.Close()
		t.Fatal("reference-mode mutation passed the locked plan verifier")
	}
	if world, err := BuildE0World(on); err == nil {
		world.Close()
		t.Fatal("legacy r1 world builder accepted a successor assignment")
	}
	invalid := on
	invalid.Seed = 999999
	if _, err := BuildE0LocalReferenceWorld(invalid); err == nil {
		t.Fatal("unregistered/protected seed reached the successor builder")
	}
}
