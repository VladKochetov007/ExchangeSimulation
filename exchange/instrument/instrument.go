package instrument

import etypes "exchange_sim/exchange/types"

type SpotInstrument struct {
	symbol         string
	base           string
	quote          string
	basePrecision  int64
	quotePrecision int64
	tickSize       int64
	minOrderSize   int64
}

func NewSpotInstrument(symbol, base, quote string, basePrecision, quotePrecision, tickSize, minOrderSize int64) *SpotInstrument {
	return &SpotInstrument{
		symbol:         symbol,
		base:           base,
		quote:          quote,
		basePrecision:  basePrecision,
		quotePrecision: quotePrecision,
		tickSize:       tickSize,
		minOrderSize:   minOrderSize,
	}
}

func (i *SpotInstrument) Symbol() string         { return i.symbol }
func (i *SpotInstrument) BaseAsset() string       { return i.base }
func (i *SpotInstrument) QuoteAsset() string      { return i.quote }
func (i *SpotInstrument) BasePrecision() int64    { return i.basePrecision }
func (i *SpotInstrument) QuotePrecision() int64   { return i.quotePrecision }
func (i *SpotInstrument) TickSize() int64         { return i.tickSize }
func (i *SpotInstrument) MinOrderSize() int64     { return i.minOrderSize }
func (i *SpotInstrument) IsPerp() bool            { return false }
func (i *SpotInstrument) InstrumentType() string  { return "SPOT" }

func (i *SpotInstrument) ValidatePrice(price int64) bool {
	return price > 0 && price%i.tickSize == 0
}

func (i *SpotInstrument) ValidateQty(qty int64) bool {
	return qty > 0
}

// FundingCalculator computes the funding rate from index and mark prices.
type FundingCalculator interface {
	Calculate(indexPrice, markPrice int64) int64
}

type PerpFutures struct {
	SpotInstrument
	fundingRate *etypes.FundingRate
	fundingCalc FundingCalculator
	// MarginRate is initial margin in bps (e.g. 1000 = 10% = 10x leverage)
	MarginRate int64
	// MaintenanceMarginRate is the minimum margin ratio in bps before liquidation
	MaintenanceMarginRate int64
	// WarningMarginRate triggers a margin call warning before liquidation
	WarningMarginRate int64
}

func NewPerpFutures(symbol, base, quote string, basePrecision, quotePrecision, tickSize, minOrderSize int64) *PerpFutures {
	return &PerpFutures{
		SpotInstrument: SpotInstrument{
			symbol:         symbol,
			base:           base,
			quote:          quote,
			basePrecision:  basePrecision,
			quotePrecision: quotePrecision,
			tickSize:       tickSize,
			minOrderSize:   minOrderSize,
		},
		fundingRate: &etypes.FundingRate{
			Symbol:      symbol,
			Rate:        0,
			NextFunding: 0,
			Interval:    28800,
			MarkPrice:   0,
			IndexPrice:  0,
		},
		fundingCalc: &SimpleFundingCalc{
			BaseRate: 10,
			Damping:  100,
			MaxRate:  75,
		},
		MarginRate:            1000,
		MaintenanceMarginRate: 500,
		WarningMarginRate:     750,
	}
}

func (p *PerpFutures) IsPerp() bool           { return true }
func (p *PerpFutures) InstrumentType() string  { return "PERP" }
func (p *PerpFutures) GetFundingRate() *etypes.FundingRate { return p.fundingRate }

func (p *PerpFutures) SetFundingCalculator(calc FundingCalculator) {
	p.fundingCalc = calc
}

func (p *PerpFutures) UpdateFundingRate(indexPrice int64, markPrice int64) {
	p.fundingRate.IndexPrice = indexPrice
	p.fundingRate.MarkPrice = markPrice
	p.fundingRate.Rate = p.fundingCalc.Calculate(indexPrice, markPrice)
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
	rate := c.BaseRate + (premium * c.Damping / 100)
	if rate > c.MaxRate {
		return c.MaxRate
	}
	if rate < -c.MaxRate {
		return -c.MaxRate
	}
	return rate
}
