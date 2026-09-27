package repeatedspot

import (
	"encoding/json"
	"testing"
)

func TestE0DevelopmentMatrixBindsFiniteRosterAndStreams(t *testing.T) {
	cells := E0DevelopmentCells()
	if len(cells) != 24 {
		t.Fatalf("E0 matrix has %d cells, want 24", len(cells))
	}
	seen := make(map[string]bool)
	for _, cell := range cells {
		if seen[cell.ID()] {
			t.Fatalf("duplicate E0 cell %s", cell.ID())
		}
		seen[cell.ID()] = true
	}
	for _, composition := range []struct {
		name  string
		kinds [4]string
	}{
		{"P", [4]string{"bounded_fixed_maker_v2", "bounded_fixed_maker_v2", "bounded_fixed_maker_v2", "bounded_fixed_maker_v2"}},
		{"A", [4]string{"bounded_stoikov_maker_v2", "bounded_stoikov_maker_v2", "bounded_stoikov_maker_v2", "bounded_stoikov_maker_v2"}},
		{"M1", [4]string{"bounded_fixed_maker_v2", "bounded_stoikov_maker_v2", "bounded_fixed_maker_v2", "bounded_stoikov_maker_v2"}},
		{"M2", [4]string{"bounded_stoikov_maker_v2", "bounded_fixed_maker_v2", "bounded_stoikov_maker_v2", "bounded_fixed_maker_v2"}},
	} {
		for _, quantity := range []int64{e0QuoteSmall, e0QuoteLarge} {
			cell := E0Cell{Composition: composition.name, QuoteQty: quantity, Seed: e0DevelopmentSeeds[0]}
			world, err := BuildE0World(cell)
			if err != nil {
				t.Fatal(err)
			}
			var contract replayContract
			if err := json.Unmarshal(world.ContractJSON(), &contract); err != nil {
				world.Close()
				t.Fatal(err)
			}
			world.Close()
			if len(contract.Participants) != 11 || contract.Iterations != 55*60 || contract.Step != 1_000_000_000 {
				t.Fatalf("wrong E0 roster/horizon for %+v", cell)
			}
			for index, kind := range composition.kinds {
				participant := contract.Participants[index+1]
				if participant.Policy.Name != kind || participant.Balances["ABC"] != 50*e0BasePrecision ||
					participant.Balances["USD"] != 2_500_000*e0QuotePrecision ||
					participant.Latency.RequestNanos != 1_000_000_000 {
					t.Fatalf("maker slot %d differs across declared arms: %+v", index+1, participant)
				}
				parameters, err := parseMakerParameters(participant)
				if err != nil || parameters.quoteQty != quantity || parameters.workingLimit != 10*e0BasePrecision {
					t.Fatalf("maker slot %d has wrong quote scale/cap: %+v, %v", index+1, parameters, err)
				}
			}
		}
	}
	first, err := BuildE0World(E0Cell{Composition: "P", QuoteQty: e0QuoteSmall, Seed: e0DevelopmentSeeds[0]})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildE0World(E0Cell{Composition: "A", QuoteQty: e0QuoteLarge, Seed: e0DevelopmentSeeds[0]})
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	var left, right replayContract
	if err := json.Unmarshal(first.ContractJSON(), &left); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.ContractJSON(), &right); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second.Close()
	for index := 5; index < 11; index++ {
		if string(left.Participants[index].Policy.Parameters) != string(right.Participants[index].Policy.Parameters) {
			t.Fatalf("counterparty %d random stream changed with maker policy/quote scale", index)
		}
	}
}

func TestE0DevelopmentMatrixRejectsUnregisteredAssignment(t *testing.T) {
	for _, cell := range []E0Cell{
		{Composition: "P", QuoteQty: e0QuoteSmall, Seed: 619},
		{Composition: "P", QuoteQty: e0QuoteSmall + 1, Seed: e0DevelopmentSeeds[0]},
		{Composition: "E1", QuoteQty: e0QuoteSmall, Seed: e0DevelopmentSeeds[0]},
	} {
		if _, err := BuildE0World(cell); err == nil {
			t.Fatalf("unregistered E0 cell accepted: %+v", cell)
		}
	}
	for _, seed := range e0DevelopmentSeeds {
		takers, roundTrips := e0Streams(seed)
		all := make(map[int64]bool)
		for _, stream := range append(takers[:], roundTrips[:]...) {
			if stream <= 0 || all[stream] {
				t.Fatalf("duplicate or invalid actor RNG stream for seed %d", seed)
			}
			all[stream] = true
		}
	}
}
