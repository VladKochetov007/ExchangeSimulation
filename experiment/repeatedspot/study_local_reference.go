package repeatedspot

import (
	"fmt"
	"time"

	worldspot "exchange_sim/simulations/repeatedspot"
)

var e0LocalReferenceSeeds = [...]int64{18_101, 18_111, 18_117}

const E0LocalReferenceMaxAge = 15 * time.Second

func E0LocalReferenceDevelopmentCells() []E0Cell {
	compositions := [...]string{"P", "A", "M1", "M2"}
	modes := [...]string{"ON", "OFF"}
	cells := make([]E0Cell, 0, len(compositions)*len(modes)*len(e0LocalReferenceSeeds))
	for _, mode := range modes {
		for _, composition := range compositions {
			for _, seed := range e0LocalReferenceSeeds {
				cells = append(cells, E0Cell{Composition: composition, QuoteQty: e0QuoteSmall,
					Seed: seed, ReferenceMode: mode})
			}
		}
	}
	return cells
}

func validateE0LocalReferenceCell(cell E0Cell) error {
	if cell.ReferenceMode != "ON" && cell.ReferenceMode != "OFF" ||
		cell.Composition != "P" && cell.Composition != "A" && cell.Composition != "M1" && cell.Composition != "M2" ||
		cell.QuoteQty != e0QuoteSmall {
		return fmt.Errorf("repeated spot: cell outside the ME-015 development matrix")
	}
	for _, seed := range e0LocalReferenceSeeds {
		if cell.Seed == seed {
			return nil
		}
	}
	return fmt.Errorf("repeated spot: unregistered ME-015 development seed")
}

func BuildE0LocalReferenceWorld(cell E0Cell) (*worldspot.World, error) {
	if err := validateE0LocalReferenceCell(cell); err != nil {
		return nil, err
	}
	maxAge := time.Duration(0)
	if cell.ReferenceMode == "ON" {
		maxAge = E0LocalReferenceMaxAge
	}
	makerConfig := e0MakerConfig(cell.QuoteQty)
	pure, err := worldspot.NewLocalReferenceFixedMakerPolicy(worldspot.LocalReferenceFixedMakerConfig{
		BoundedFixedMakerConfig: worldspot.BoundedFixedMakerConfig{Maker: makerConfig, SpreadBps: 2},
		MaxAge:                  maxAge,
	})
	if err != nil {
		return nil, err
	}
	stoikov, err := worldspot.NewLocalReferenceStoikovMakerPolicy(worldspot.LocalReferenceStoikovMakerConfig{
		BoundedStoikovMakerConfig: worldspot.BoundedStoikovMakerConfig{
			Maker: makerConfig, QuotePrecision: e0QuotePrecision,
			RelativeRiskAversion: 50, RelativeFillDecay: 20_000,
			InventoryHorizon: 600 * time.Second, MinHalfSpreadTicks: 1,
		}, MaxAge: maxAge,
	})
	if err != nil {
		return nil, err
	}
	policies := [4]worldspot.PolicyDefinition{pure, pure, pure, pure}
	switch cell.Composition {
	case "A":
		policies = [4]worldspot.PolicyDefinition{stoikov, stoikov, stoikov, stoikov}
	case "M1":
		policies = [4]worldspot.PolicyDefinition{pure, stoikov, pure, stoikov}
	case "M2":
		policies = [4]worldspot.PolicyDefinition{stoikov, pure, stoikov, pure}
	}
	takers, roundTrips := e0Streams(cell.Seed)
	config, err := worldspot.DraftE0Config(policies, takers, roundTrips)
	if err != nil {
		return nil, err
	}
	return worldspot.Build(config)
}

func buildRegisteredE0World(cell E0Cell) (*worldspot.World, error) {
	if cell.ReferenceMode != "" {
		return BuildE0LocalReferenceWorld(cell)
	}
	return BuildE0World(cell)
}
