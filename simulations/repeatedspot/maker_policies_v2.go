package repeatedspot

import (
	"fmt"
	"math"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulations/multivenue"
	"exchange_sim/types"
)

type BoundedFixedMakerConfig struct {
	Maker     RecurringMakerConfig `json:"maker"`
	SpreadBps int64                `json:"spread_bps"`
}

func NewBoundedFixedMakerPolicy(config BoundedFixedMakerConfig) (PolicyDefinition, error) {
	if config.SpreadBps < 0 || config.SpreadBps >= 10_000 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid bounded fixed-maker spread")
	}
	return DefinePolicy("bounded_fixed_maker_v2", config,
		func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, effective BoundedFixedMakerConfig) (actor.Actor, error) {
			return NewRecurringMaker(id, gateway, effective.Maker, fixedMakerQuoteRule(effective.SpreadBps, effective.Maker.TickSize))
		})
}

func fixedMakerQuoteRule(spreadBps, tickSize int64) MakerQuoteRule {
	return func(input MakerQuoteInput) (MakerQuote, bool) {
		halfSpread, ok := types.TryMulBps(input.MidPrice, spreadBps)
		if !ok || halfSpread >= input.MidPrice || tickSize <= 0 {
			return MakerQuote{}, false
		}
		askUnrounded, ok := types.TryAdd(input.MidPrice, halfSpread)
		if !ok {
			return MakerQuote{}, false
		}
		bid := (input.MidPrice - halfSpread) / tickSize * tickSize
		ask := askUnrounded / tickSize * tickSize
		if askUnrounded%tickSize != 0 {
			ask, ok = types.TryAdd(ask, tickSize)
		}
		return MakerQuote{BidPrice: bid, AskPrice: ask}, ok && bid > 0 && ask > bid
	}
}

type BoundedStoikovMakerConfig struct {
	Maker                RecurringMakerConfig `json:"maker"`
	QuotePrecision       int64                `json:"quote_precision"`
	RelativeRiskAversion float64              `json:"relative_risk_aversion"`
	RelativeFillDecay    float64              `json:"relative_fill_decay"`
	InventoryHorizon     time.Duration        `json:"inventory_horizon_ns"`
	MinHalfSpreadTicks   int64                `json:"min_half_spread_ticks"`
}

func NewBoundedStoikovMakerPolicy(config BoundedStoikovMakerConfig) (PolicyDefinition, error) {
	if config.QuotePrecision <= 0 || config.Maker.TickSize <= 0 || config.InventoryHorizon <= 0 ||
		config.MinHalfSpreadTicks <= 0 || config.RelativeRiskAversion <= 0 || config.RelativeFillDecay <= 0 ||
		math.IsNaN(config.RelativeRiskAversion) || math.IsInf(config.RelativeRiskAversion, 0) ||
		math.IsNaN(config.RelativeFillDecay) || math.IsInf(config.RelativeFillDecay, 0) ||
		config.Maker.InitialLogVariancePerSecond <= 0 || config.Maker.VolatilityHalfLife <= 0 ||
		config.Maker.VolatilitySampleInterval <= 0 || config.Maker.MaxLogVarianceMultiple <= 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid bounded Stoikov-maker parameters")
	}
	return DefinePolicy("bounded_stoikov_maker_v2", config,
		func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, effective BoundedStoikovMakerConfig) (actor.Actor, error) {
			return NewRecurringMaker(id, gateway, effective.Maker, stoikovMakerQuoteRule(effective))
		})
}

func stoikovMakerQuoteRule(config BoundedStoikovMakerConfig) MakerQuoteRule {
	return func(input MakerQuoteInput) (MakerQuote, bool) {
		forward := float64(input.MidPrice) / float64(config.QuotePrecision)
		if forward <= 0 || input.WorkingLimit <= 0 {
			return MakerQuote{}, false
		}
		quote, ok := multivenue.CalculateStoikovQuote(multivenue.StoikovInputs{
			Forward:           forward,
			Inventory:         float64(input.FilledInventory) / float64(input.WorkingLimit),
			VariancePerSecond: input.LogVariancePerSecond * forward * forward,
			RiskAversion:      config.RelativeRiskAversion / forward,
			FillDecay:         config.RelativeFillDecay / forward,
			InventoryHorizon:  config.InventoryHorizon,
			MinHalfSpread:     float64(config.MinHalfSpreadTicks) * float64(config.Maker.TickSize) / float64(config.QuotePrecision),
		})
		if !ok {
			return MakerQuote{}, false
		}
		bid, okBid := roundedMakerPrice(quote.Bid, config.QuotePrecision, config.Maker.TickSize, false)
		ask, okAsk := roundedMakerPrice(quote.Ask, config.QuotePrecision, config.Maker.TickSize, true)
		return MakerQuote{BidPrice: bid, AskPrice: ask}, okBid && okAsk && bid > 0 && ask > bid
	}
}

func roundedMakerPrice(price float64, quotePrecision, tickSize int64, roundUp bool) (int64, bool) {
	if math.IsNaN(price) || math.IsInf(price, 0) || price <= 0 || quotePrecision <= 0 || tickSize <= 0 {
		return 0, false
	}
	ticks := price * float64(quotePrecision) / float64(tickSize)
	if roundUp {
		ticks = math.Ceil(ticks)
	} else {
		ticks = math.Floor(ticks)
	}
	if ticks <= 0 || ticks >= float64(math.MaxInt64)/float64(tickSize) {
		return 0, false
	}
	return int64(ticks) * tickSize, true
}
