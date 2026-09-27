package repeatedspot

import (
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulations/feesim"
	"exchange_sim/simulations/multivenue"
)

type PercentageFeeConfig struct {
	MakerBps int64 `json:"maker_bps"`
	TakerBps int64 `json:"taker_bps"`
	InQuote  bool  `json:"in_quote"`
}

func NewPercentageFee(config PercentageFeeConfig) (FeeDefinition, error) {
	if config.MakerBps < 0 || config.TakerBps < 0 {
		return FeeDefinition{}, fmt.Errorf("repeatedspot: negative fee rates require an explicit fee model")
	}
	return DefineFee("percentage_v1", config, func(config PercentageFeeConfig) (exchange.FeeModel, error) {
		return &exchange.PercentageFee{MakerBps: config.MakerBps, TakerBps: config.TakerBps, InQuote: config.InQuote}, nil
	})
}

func NewSeedOncePolicy(config SeedOnceConfig) (PolicyDefinition, error) {
	if config.Symbol == "" || config.BidPrice <= 0 || config.AskPrice <= config.BidPrice || config.BidQty <= 0 || config.AskQty <= 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid seed-once policy")
	}
	return DefinePolicy("seed_once_v1", config, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, config SeedOnceConfig) (actor.Actor, error) {
		return NewSeedOnce(id, gateway, config)
	})
}

type FixedMakerConfig struct {
	Symbol                      string        `json:"symbol"`
	SpreadBps                   int64         `json:"spread_bps"`
	RequoteBps                  int64         `json:"requote_bps"`
	QuoteQty                    int64         `json:"quote_qty"`
	MaxInventory                int64         `json:"max_inventory"`
	QuoteInterval               time.Duration `json:"quote_interval_ns"`
	TickSize                    int64         `json:"tick_size"`
	PostOnlyCancelBeforeReplace bool          `json:"post_only_cancel_before_replace"`
}

// NewFixedMakerPolicy adapts the existing recurring fixed-distance maker.
// MaxInventory is its filled-inventory threshold, not a working-order cap.
func NewFixedMakerPolicy(config FixedMakerConfig) (PolicyDefinition, error) {
	if config.Symbol == "" || config.SpreadBps < 0 || config.RequoteBps < 0 || config.QuoteQty <= 0 ||
		config.MaxInventory <= 0 || config.QuoteInterval <= 0 || config.TickSize <= 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid fixed-maker configuration")
	}
	return DefinePolicy("fixed_distance_maker_v1", config, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, c FixedMakerConfig) (actor.Actor, error) {
		return multivenue.NewFixedDistanceMaker(id, gateway, multivenue.FixedDistanceMakerConfig{
			Symbol: c.Symbol, SpreadBps: c.SpreadBps, RequoteBps: c.RequoteBps,
			QuoteQty: c.QuoteQty, MaxInventory: c.MaxInventory, QuoteInterval: c.QuoteInterval,
			TickSize: c.TickSize, PostOnly: true, PostOnlyCancelBeforeReplace: c.PostOnlyCancelBeforeReplace,
		}), nil
	})
}

// StoikovSpotConfig exposes only the existing single-book passive quote path.
// The actor's inventory limit bounds skew but does not cap live/pending orders.
type StoikovSpotConfig struct {
	Symbol                   string        `json:"symbol"`
	VenueID                  string        `json:"venue_id"`
	BootstrapPrice           int64         `json:"bootstrap_price"`
	BasePrecision            int64         `json:"base_precision"`
	QuotePrecision           int64         `json:"quote_precision"`
	TickSize                 int64         `json:"tick_size"`
	QuoteQty                 int64         `json:"quote_qty"`
	QuoteInterval            time.Duration `json:"quote_interval_ns"`
	InventoryLimit           int64         `json:"inventory_limit"`
	InventoryHorizon         time.Duration `json:"inventory_horizon_ns"`
	RelativeRiskAversion     float64       `json:"relative_risk_aversion"`
	RelativeFillDecay        float64       `json:"relative_fill_decay"`
	InitialLogVariancePerSec float64       `json:"initial_log_variance_per_sec"`
	VolatilityHalfLife       time.Duration `json:"volatility_half_life_ns"`
	VolatilitySampleInterval time.Duration `json:"volatility_sample_interval_ns"`
	MaxLogVarianceMultiple   float64       `json:"max_log_variance_multiple"`
	MinHalfSpreadTicks       int64         `json:"min_half_spread_ticks"`
	RequoteBps               int64         `json:"requote_bps"`
	CancelBeforeReplace      bool          `json:"cancel_before_replace"`
}

func NewStoikovSpotPolicy(config StoikovSpotConfig) (PolicyDefinition, error) {
	if config.Symbol == "" || config.VenueID == "" || config.BootstrapPrice <= 0 ||
		config.BasePrecision <= 0 || config.QuotePrecision <= 0 || config.TickSize <= 0 || config.QuoteQty <= 0 ||
		config.QuoteInterval <= 0 || config.InventoryLimit <= 0 || config.InventoryHorizon <= 0 ||
		config.RelativeRiskAversion <= 0 || config.RelativeFillDecay <= 0 || config.InitialLogVariancePerSec < 0 ||
		config.VolatilityHalfLife < 0 || config.VolatilitySampleInterval < 0 || config.MaxLogVarianceMultiple < 0 ||
		config.MinHalfSpreadTicks < 0 || config.RequoteBps < 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid Stoikov spot configuration")
	}
	return DefinePolicy("stoikov_spot_v1", config, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, c StoikovSpotConfig) (actor.Actor, error) {
		return multivenue.NewStoikovMarketMaker(id, gateway, multivenue.StoikovMMConfig{
			Symbol: c.Symbol, ReferenceSymbol: c.Symbol, BootstrapPrice: c.BootstrapPrice,
			BasePrecision: c.BasePrecision, QuotePrecision: c.QuotePrecision, TickSize: c.TickSize,
			QuoteQty: c.QuoteQty, QuoteInterval: c.QuoteInterval, InventoryLimit: c.InventoryLimit,
			InventoryHorizon: c.InventoryHorizon, RelativeRiskAversion: c.RelativeRiskAversion,
			RelativeFillDecay: c.RelativeFillDecay, InitialLogVariancePerSec: c.InitialLogVariancePerSec,
			VolatilityHalfLife: c.VolatilityHalfLife, VolatilitySampleInterval: c.VolatilitySampleInterval,
			MaxLogVarianceMultiple: c.MaxLogVarianceMultiple, MinHalfSpreadTicks: c.MinHalfSpreadTicks,
			RequoteBps: c.RequoteBps, PostOnly: true, PostOnlyCancelBeforeReplace: c.CancelBeforeReplace,
			UseLocalReferenceCache: true, LocalReferenceSourceVenue: c.VenueID,
		}), nil
	})
}

type RandomTakerConfig struct {
	Symbol       string        `json:"symbol"`
	TargetQty    int64         `json:"target_qty"`
	TakeInterval time.Duration `json:"take_interval_ns"`
	Seed         int64         `json:"seed"`
}

func NewRandomTakerPolicy(config RandomTakerConfig) (PolicyDefinition, error) {
	if config.Symbol == "" || config.TargetQty <= 0 || config.TakeInterval <= 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid recurring-taker configuration")
	}
	return DefinePolicy("random_taker_spot_v1", config, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, c RandomTakerConfig) (actor.Actor, error) {
		return feesim.NewRandomTaker(id, gateway, feesim.TakerConfig{
			Symbols: []string{c.Symbol}, TargetQtys: map[string]int64{c.Symbol: c.TargetQty},
			TakeInterval: c.TakeInterval, Seed: c.Seed,
		}), nil
	})
}

type RoundTripConfig struct {
	Symbol          string        `json:"symbol"`
	BasePrecision   int64         `json:"base_precision"`
	LotQty          int64         `json:"lot_qty"`
	MinOrderSize    int64         `json:"min_order_size"`
	Interval        time.Duration `json:"interval_ns"`
	HoldDuration    time.Duration `json:"hold_duration_ns"`
	OpenProbability float64       `json:"open_probability"`
	Seed            int64         `json:"seed"`
}

func NewRoundTripPolicy(config RoundTripConfig) (PolicyDefinition, error) {
	if config.Symbol == "" || config.BasePrecision <= 0 || config.LotQty <= 0 || config.MinOrderSize <= 0 ||
		config.Interval <= 0 || config.HoldDuration <= 0 || config.OpenProbability < 0 || config.OpenProbability > 1 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid round-trip configuration")
	}
	return DefinePolicy("round_trip_spot_v1", config, func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, c RoundTripConfig) (actor.Actor, error) {
		return multivenue.NewRoundTripTrader(id, gateway, multivenue.RoundTripTraderConfig{
			Symbol: c.Symbol, BasePrecision: c.BasePrecision, LotQty: c.LotQty,
			MinOrderSize: c.MinOrderSize, Interval: c.Interval, HoldDuration: c.HoldDuration,
			OpenProbability: c.OpenProbability, Seed: c.Seed,
		}), nil
	})
}
