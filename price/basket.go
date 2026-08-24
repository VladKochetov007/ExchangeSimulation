package price

import (
	"fmt"
	"math/big"
	"slices"

	etypes "exchange_sim/types"
)

// DeviationPolicy decides how a basket constituent that strays from the
// basket median contributes to the index. Venues genuinely disagree here —
// Binance/OKX/Deribit clamp the price toward the median, Bybit cuts the
// source's weight — so the choice is injected, never hardcoded.
type DeviationPolicy interface {
	// Adjust receives a constituent's price, the basket median, and the
	// constituent's weight; it returns the price and weight to use. A zero
	// adjusted weight removes the constituent from this pass. An error is a
	// malformed or unrepresentable index calculation, not source absence.
	Adjust(price, median, weight int64) (adjPrice, adjWeight int64, err error)
}

// ClampToMedian caps a constituent's deviation at ThresholdBps of the median
// (the Binance/OKX/Deribit rule): the source stays in the basket but cannot
// pull the index further than the threshold.
type ClampToMedian struct{ ThresholdBps int64 }

func (p ClampToMedian) Adjust(price, median, weight int64) (int64, int64, error) {
	limit, err := deviationLimit(median, p.ThresholdBps)
	if err != nil {
		return 0, 0, err
	}
	if price > median && price-median > limit {
		adjusted, ok := etypes.TryAdd(median, limit)
		if !ok {
			return 0, 0, fmt.Errorf("clamp upper bound: unrepresentable")
		}
		return adjusted, weight, nil
	}
	if price < median && median-price > limit {
		adjusted, ok := etypes.TrySub(median, limit)
		if !ok {
			return 0, 0, fmt.Errorf("clamp lower bound: unrepresentable")
		}
		return adjusted, weight, nil
	}
	return price, weight, nil
}

// ExcludeOutliers drops a constituent entirely once it deviates more than
// ThresholdBps from the median (Bybit-style exclusion).
type ExcludeOutliers struct{ ThresholdBps int64 }

func (p ExcludeOutliers) Adjust(price, median, weight int64) (int64, int64, error) {
	limit, err := deviationLimit(median, p.ThresholdBps)
	if err != nil {
		return 0, 0, err
	}
	if (price > median && price-median > limit) || (price < median && median-price > limit) {
		return price, 0, nil
	}
	return price, weight, nil
}

func deviationLimit(median, bps int64) (int64, error) {
	if median <= 0 || bps < 0 {
		return 0, fmt.Errorf("invalid positive basket median=%d or threshold_bps=%d", median, bps)
	}
	limit, ok := etypes.TryMulBps(median, bps)
	if !ok {
		return 0, fmt.Errorf("basket deviation limit overflows int64")
	}
	return limit, nil
}

// BasketSource is one constituent of a BasketIndex.
type BasketSource struct {
	Source etypes.PriceSource
	Weight int64 // relative weight; keep small (basis points or 1..100)
}

type basketQuote struct{ price, weight int64 }

// BasketIndex aggregates several price sources into one index the way real
// venues do: unavailable sources drop out, survivors are medianed, the
// deviation policy reins in or removes outliers, and the result is the
// weighted average. Fewer than MinSources usable constituents — or every
// weight zeroed by the policy — is an explicit unavailable result.
type BasketIndex struct {
	sources    []BasketSource
	policy     DeviationPolicy
	minSources int
}

func NewBasketIndex(sources []BasketSource, policy DeviationPolicy, minSources int) *BasketIndex {
	if minSources < 1 {
		minSources = 1
	}
	return &BasketIndex{sources: sources, policy: policy, minSources: minSources}
}

func (b *BasketIndex) Price(symbol string) (int64, error) {
	live := make([]basketQuote, 0, len(b.sources))
	prices := make([]int64, 0, len(b.sources))
	for _, s := range b.sources {
		if s.Source == nil || s.Weight <= 0 {
			continue
		}
		p, err := positiveSourcePrice(s.Source, symbol)
		if err == nil {
			live = append(live, basketQuote{p, s.Weight})
			prices = append(prices, p)
		}
	}
	if len(live) < b.minSources {
		return 0, fmt.Errorf("basket %s has %d usable sources, need %d: %w", symbol, len(live), b.minSources, etypes.ErrNoPrice)
	}

	slices.Sort(prices)
	median := prices[len(prices)/2]
	if len(prices)%2 == 0 {
		lower := prices[len(prices)/2-1]
		upper := prices[len(prices)/2]
		median = etypes.Midpoint(lower, upper)
	}

	adjusted := make([]basketQuote, 0, len(live))
	for _, q := range live {
		price, weight := q.price, q.weight
		if b.policy != nil {
			var err error
			price, weight, err = b.policy.Adjust(price, median, weight)
			if err != nil {
				return 0, fmt.Errorf("basket %s adjustment: %w", symbol, err)
			}
		}
		if weight <= 0 {
			continue
		}
		adjusted = append(adjusted, basketQuote{price, weight})
	}
	price, err := weightedBasketPrice(adjusted)
	if err != nil {
		return 0, fmt.Errorf("basket %s weighted average: %w", symbol, err)
	}
	if price <= 0 {
		return 0, fmt.Errorf("basket %s produced non-positive price: %w", symbol, etypes.ErrPriceDomain)
	}
	return price, nil
}

// weightedBasketPrice uses exact integers because a positive index can still
// overflow a native price×weight intermediate near the signed-price boundary.
// An absent weighted constituent set returns ErrNoPrice. An unrepresentable
// arithmetic result returns its own error; it is not price absence.
func weightedBasketPrice(quotes []basketQuote) (int64, error) {
	var weightedSum, totalWeight, term, price, weight, quotient big.Int
	for _, quote := range quotes {
		if quote.weight <= 0 {
			continue
		}
		price.SetInt64(quote.price)
		weight.SetInt64(quote.weight)
		term.Mul(&price, &weight)
		weightedSum.Add(&weightedSum, &term)
		totalWeight.Add(&totalWeight, &weight)
	}
	if totalWeight.Sign() == 0 {
		return 0, etypes.ErrNoPrice
	}
	quotient.Quo(&weightedSum, &totalWeight)
	if !quotient.IsInt64() {
		return 0, fmt.Errorf("weighted result overflows int64")
	}
	return quotient.Int64(), nil
}
