package repeatedspot

import (
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

const imbalancePrecision = int64(1_000_000)
const imbalanceShiftDenominator = int64(10_000_000_000)

type ImbalanceStoikovMakerConfig struct {
	LocalReferenceStoikovMakerConfig
	SignalGainBps int64         `json:"signal_gain_bps"`
	MaxSignalAge  time.Duration `json:"max_signal_age_ns"`
}

// NewImbalanceStoikovMakerPolicy changes only the quote rule. The shared maker
// still owns all order, response, inventory and cancellation state.
func NewImbalanceStoikovMakerPolicy(config ImbalanceStoikovMakerConfig) (PolicyDefinition, error) {
	if config.SignalGainBps < 0 || config.MaxSignalAge <= 0 {
		return PolicyDefinition{}, fmt.Errorf("repeatedspot: invalid displayed-imbalance signal contract")
	}
	if _, err := NewLocalReferenceStoikovMakerPolicy(config.LocalReferenceStoikovMakerConfig); err != nil {
		return PolicyDefinition{}, err
	}
	return DefinePolicy("bounded_imbalance_stoikov_maker_v1", config,
		func(id uint64, gateway actor.Gateway, _ exchange.TickerFactory, effective ImbalanceStoikovMakerConfig) (actor.Actor, error) {
			return NewRecurringMakerWithInformation(id, gateway, effective.Maker,
				imbalanceStoikovQuoteRule(effective), MakerInformationOptions{
					LocalReferenceMaxAge:  effective.MaxAge,
					EmitReferenceEvidence: true,
					EmitTopDepthEvidence:  true,
				})
		})
}

func imbalanceStoikovQuoteRule(config ImbalanceStoikovMakerConfig) MakerQuoteRule {
	baseRule := stoikovMakerQuoteRule(config.BoundedStoikovMakerConfig)
	return func(input MakerQuoteInput) (MakerQuote, bool) {
		quote, ok := baseRule(input)
		if !ok || config.SignalGainBps == 0 {
			return quote, ok
		}
		if input.TopBidVisibleQty < 0 || input.TopAskVisibleQty < 0 || input.BookSourceAge < 0 {
			return MakerQuote{}, false
		}
		if input.TopBidVisibleQty == 0 || input.TopAskVisibleQty == 0 ||
			input.BookSourceAge >= config.MaxSignalAge {
			return quote, true
		}
		totalQty, ok := types.TryAdd(input.TopBidVisibleQty, input.TopAskVisibleQty)
		if !ok || totalQty <= 0 {
			return MakerQuote{}, false
		}
		difference, ok := types.TrySub(input.TopBidVisibleQty, input.TopAskVisibleQty)
		if !ok {
			return MakerQuote{}, false
		}
		imbalancePPM, ok := types.TryMulDiv(difference, imbalancePrecision, totalQty)
		if !ok {
			return MakerQuote{}, false
		}
		gainPPMBps, ok := types.TryMulDiv(imbalancePPM, config.SignalGainBps, 1)
		if !ok {
			return MakerQuote{}, false
		}
		shift, ok := types.TryMulDiv(input.MidPrice, gainPPMBps, imbalanceShiftDenominator)
		if !ok {
			return MakerQuote{}, false
		}
		shift = shift / config.Maker.TickSize * config.Maker.TickSize
		bid, bidOK := types.TryAdd(quote.BidPrice, shift)
		ask, askOK := types.TryAdd(quote.AskPrice, shift)
		return MakerQuote{BidPrice: bid, AskPrice: ask}, bidOK && askOK && bid > 0 && ask > bid
	}
}
