package repeatedspot

import (
	"fmt"
	"time"

	worldspot "exchange_sim/simulations/repeatedspot"
)

const (
	me016SignalMaxAge = 10 * time.Second
	me016ReferenceAge = 15 * time.Second
)

var me016DevelopmentSeeds = [...]int64{18_101, 18_111, 18_117}

type ME016Cell struct {
	Composition   string `json:"composition"`
	SignalGainBps int64  `json:"signal_gain_bps"`
	Seed          int64  `json:"seed"`
}

func (cell ME016Cell) ID() string {
	return fmt.Sprintf("ME016-%s-g%d-s%d", cell.Composition, cell.SignalGainBps, cell.Seed)
}

func ME016DevelopmentCells() []ME016Cell {
	cells := make([]ME016Cell, 0, 2*2*len(me016DevelopmentSeeds))
	for _, composition := range [...]string{"M1", "M2"} {
		for _, gain := range [...]int64{0, 2} {
			for _, seed := range me016DevelopmentSeeds {
				cells = append(cells, ME016Cell{Composition: composition, SignalGainBps: gain, Seed: seed})
			}
		}
	}
	return cells
}

func validateME016Cell(cell ME016Cell) error {
	if cell.Composition != "M1" && cell.Composition != "M2" ||
		cell.SignalGainBps != 0 && cell.SignalGainBps != 2 {
		return fmt.Errorf("repeated spot: cell outside the ME-016 development matrix")
	}
	for _, seed := range me016DevelopmentSeeds {
		if cell.Seed == seed {
			return nil
		}
	}
	return fmt.Errorf("repeated spot: unregistered ME-016 development seed")
}

// DraftME016Config is a prospectively fixed study adapter, not a public
// limitation on which policies clients may compose with repeatedspot.Build.
func DraftME016Config(cell ME016Cell) (worldspot.Config, error) {
	if err := validateME016Cell(cell); err != nil {
		return worldspot.Config{}, err
	}
	makerConfig := e0MakerConfig(e0QuoteSmall)
	pure, err := worldspot.NewLocalReferenceFixedMakerPolicy(worldspot.LocalReferenceFixedMakerConfig{
		BoundedFixedMakerConfig: worldspot.BoundedFixedMakerConfig{Maker: makerConfig, SpreadBps: 2},
		MaxAge:                  me016ReferenceAge,
	})
	if err != nil {
		return worldspot.Config{}, err
	}
	signal, err := worldspot.NewImbalanceStoikovMakerPolicy(worldspot.ImbalanceStoikovMakerConfig{
		LocalReferenceStoikovMakerConfig: worldspot.LocalReferenceStoikovMakerConfig{
			BoundedStoikovMakerConfig: worldspot.BoundedStoikovMakerConfig{
				Maker: makerConfig, QuotePrecision: e0QuotePrecision,
				RelativeRiskAversion: 50, RelativeFillDecay: 20_000,
				InventoryHorizon: 600 * time.Second, MinHalfSpreadTicks: 1,
			}, MaxAge: me016ReferenceAge,
		}, SignalGainBps: cell.SignalGainBps, MaxSignalAge: me016SignalMaxAge,
	})
	if err != nil {
		return worldspot.Config{}, err
	}
	policies := [4]worldspot.PolicyDefinition{pure, signal, pure, signal}
	if cell.Composition == "M2" {
		policies = [4]worldspot.PolicyDefinition{signal, pure, signal, pure}
	}
	takers, roundTrips := e0Streams(cell.Seed)
	return worldspot.DraftE0Config(policies, takers, roundTrips)
}

func BuildME016World(cell ME016Cell) (*worldspot.World, error) {
	config, err := DraftME016Config(cell)
	if err != nil {
		return nil, err
	}
	return worldspot.Build(config)
}
