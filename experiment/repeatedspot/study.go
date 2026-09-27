package repeatedspot

import (
	"fmt"
	"math"
	"time"

	worldspot "exchange_sim/simulations/repeatedspot"
)

const (
	e0BasePrecision  int64 = 100_000_000
	e0QuotePrecision int64 = 100_000
	e0QuoteSmall     int64 = e0BasePrecision / 10
	e0QuoteLarge     int64 = e0BasePrecision / 5
)

var e0DevelopmentSeeds = [...]int64{18_001, 18_011, 18_017}

// E0Cell is a finite study assignment, not a generic policy-registration type.
// External callers can still compose arbitrary policies with repeatedspot.Build.
type E0Cell struct {
	Composition string `json:"composition"`
	QuoteQty    int64  `json:"quote_qty_base_units"`
	Seed        int64  `json:"seed"`
}

func E0DevelopmentCells() []E0Cell {
	compositions := [...]string{"P", "A", "M1", "M2"}
	quantities := [...]int64{e0QuoteSmall, e0QuoteLarge}
	cells := make([]E0Cell, 0, len(compositions)*len(quantities)*len(e0DevelopmentSeeds))
	for _, composition := range compositions {
		for _, quantity := range quantities {
			for _, seed := range e0DevelopmentSeeds {
				cells = append(cells, E0Cell{Composition: composition, QuoteQty: quantity, Seed: seed})
			}
		}
	}
	return cells
}

func (cell E0Cell) ID() string {
	return fmt.Sprintf("%s-q%d-s%d", cell.Composition, cell.QuoteQty, cell.Seed)
}

func validateE0Cell(cell E0Cell) error {
	if cell.Composition != "P" && cell.Composition != "A" && cell.Composition != "M1" && cell.Composition != "M2" ||
		cell.QuoteQty != e0QuoteSmall && cell.QuoteQty != e0QuoteLarge {
		return fmt.Errorf("repeated spot: cell outside the E0 development matrix")
	}
	for _, seed := range e0DevelopmentSeeds {
		if cell.Seed == seed {
			return nil
		}
	}
	return fmt.Errorf("repeated spot: unregistered E0 development seed")
}

func e0MakerConfig(quoteQty int64) worldspot.RecurringMakerConfig {
	return worldspot.RecurringMakerConfig{
		Symbol: "ABC/USD", QuoteQty: quoteQty, MinQuoteQty: e0BasePrecision / 1_000,
		WorkingLimit: 10 * e0BasePrecision, TickSize: e0QuotePrecision,
		QuoteInterval: 5 * time.Second, RequoteBps: 1,
		InitialLogVariancePerSecond: 1e-8, VolatilityHalfLife: 120 * time.Second,
		VolatilitySampleInterval: 30 * time.Second, MaxLogVarianceMultiple: 4,
	}
}

func e0MakerPolicies(quoteQty int64) (worldspot.PolicyDefinition, worldspot.PolicyDefinition, error) {
	config := e0MakerConfig(quoteQty)
	pure, err := worldspot.NewBoundedFixedMakerPolicy(worldspot.BoundedFixedMakerConfig{
		Maker: config, SpreadBps: 2,
	})
	if err != nil {
		return worldspot.PolicyDefinition{}, worldspot.PolicyDefinition{}, err
	}
	stoikov, err := worldspot.NewBoundedStoikovMakerPolicy(worldspot.BoundedStoikovMakerConfig{
		Maker: config, QuotePrecision: e0QuotePrecision,
		RelativeRiskAversion: 50, RelativeFillDecay: 20_000,
		InventoryHorizon: 600 * time.Second, MinHalfSpreadTicks: 1,
	})
	return pure, stoikov, err
}

func e0Streams(master int64) ([4]int64, [2]int64) {
	var takers [4]int64
	var roundTrips [2]int64
	for index := range takers {
		takers[index] = e0Stream(master, uint64(index+1))
	}
	for index := range roundTrips {
		roundTrips[index] = e0Stream(master, uint64(index+5))
	}
	return takers, roundTrips
}

func e0Stream(master int64, stream uint64) int64 {
	value := uint64(master) + stream*0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	value ^= value >> 31
	seed := int64(value & math.MaxInt64)
	if seed == 0 {
		return 1
	}
	return seed
}

func BuildE0World(cell E0Cell) (*worldspot.World, error) {
	if err := validateE0Cell(cell); err != nil {
		return nil, err
	}
	pure, stoikov, err := e0MakerPolicies(cell.QuoteQty)
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

func E0MeasurementWindow() MeasurementWindow {
	return MeasurementWindow{StartAt: int64(10 * time.Minute), EndAt: int64(55 * time.Minute)}
}
