package price

import (
	"fmt"
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
	// adjusted weight removes the constituent from this pass.
	Adjust(price, median, weight int64) (adjPrice, adjWeight int64)
}

// ClampToMedian caps a constituent's deviation at ThresholdBps of the median
// (the Binance/OKX/Deribit rule): the source stays in the basket but cannot
// pull the index further than the threshold.
type ClampToMedian struct{ ThresholdBps int64 }

func (p ClampToMedian) Adjust(price, median, weight int64) (int64, int64) {
	limit := median * p.ThresholdBps / 10000
	if price > median+limit {
		return median + limit, weight
	}
	if price < median-limit {
		return median - limit, weight
	}
	return price, weight
}

// ExcludeOutliers drops a constituent entirely once it deviates more than
// ThresholdBps from the median (Bybit-style exclusion).
type ExcludeOutliers struct{ ThresholdBps int64 }

func (p ExcludeOutliers) Adjust(price, median, weight int64) (int64, int64) {
	limit := median * p.ThresholdBps / 10000
	if price > median+limit || price < median-limit {
		return price, 0
	}
	return price, weight
}

// BasketSource is one constituent of a BasketIndex.
type BasketSource struct {
	Source etypes.PriceSource
	Weight int64 // relative weight; keep small (basis points or 1..100)
}

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
	type quote struct{ price, weight int64 }
	live := make([]quote, 0, len(b.sources))
	prices := make([]int64, 0, len(b.sources))
	for _, s := range b.sources {
		if s.Source == nil || s.Weight <= 0 {
			continue
		}
		p, err := positiveSourcePrice(s.Source, symbol)
		if err == nil {
			live = append(live, quote{p, s.Weight})
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

	var weightedSum, totalWeight int64
	for _, q := range live {
		price, weight := q.price, q.weight
		if b.policy != nil {
			price, weight = b.policy.Adjust(price, median, weight)
		}
		if weight <= 0 {
			continue
		}
		weightedSum += price * weight
		totalWeight += weight
	}
	if totalWeight == 0 {
		return 0, fmt.Errorf("basket %s has no weighted constituents: %w", symbol, etypes.ErrNoPrice)
	}
	price := weightedSum / totalWeight
	if price <= 0 {
		return 0, fmt.Errorf("basket %s produced non-positive price: %w", symbol, etypes.ErrPriceDomain)
	}
	return price, nil
}
