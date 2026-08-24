package instrument

import (
	"fmt"

	etypes "exchange_sim/types"
)

// FundingCalculator computes the funding rate from index and mark prices.
type FundingCalculator interface {
	Calculate(indexPrice, markPrice int64) (int64, error)
}

type SimpleFundingCalc struct {
	BaseRate int64
	Damping  int64
	MaxRate  int64
}

func (c *SimpleFundingCalc) Calculate(indexPrice, markPrice int64) (int64, error) {
	// This percentage-premium formula is a positive-index model. A present
	// zero/negative index is not silently treated as a zero premium; it is an
	// explicit domain error until a signed-price funding model is introduced.
	if indexPrice <= 0 || markPrice <= 0 {
		return 0, fmt.Errorf("simple funding premium: %w", etypes.ErrPriceDomain)
	}
	premium, ok := etypes.TryPriceChangeMulDiv(10_000, markPrice, indexPrice, indexPrice)
	if !ok {
		return 0, fmt.Errorf("simple funding premium: %w", etypes.ErrPriceDomain)
	}
	damped, ok := etypes.TryMulDiv(premium, c.Damping, 100)
	if !ok {
		return 0, fmt.Errorf("simple funding damping: %w", etypes.ErrPriceDomain)
	}
	rate, ok := etypes.TryAdd(c.BaseRate, damped)
	if !ok {
		return 0, fmt.Errorf("simple funding rate: %w", etypes.ErrPriceDomain)
	}
	if rate > c.MaxRate {
		return c.MaxRate, nil
	}
	if rate < -c.MaxRate {
		return -c.MaxRate, nil
	}
	return rate, nil
}
