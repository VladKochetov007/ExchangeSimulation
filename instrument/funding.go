package instrument

// FundingCalculator computes the funding rate from index and mark prices.
type FundingCalculator interface {
	Calculate(indexPrice, markPrice int64) int64
}

type SimpleFundingCalc struct {
	BaseRate int64
	Damping  int64
	MaxRate  int64
}

func (c *SimpleFundingCalc) Calculate(indexPrice, markPrice int64) int64 {
	if indexPrice == 0 {
		return 0
	}
	premium := ((markPrice - indexPrice) * 10000) / indexPrice
	rate := c.BaseRate + (premium*c.Damping/100)
	if rate > c.MaxRate {
		return c.MaxRate
	}
	if rate < -c.MaxRate {
		return -c.MaxRate
	}
	return rate
}
