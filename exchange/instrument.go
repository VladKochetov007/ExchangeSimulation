package exchange

type Instrument interface {
	Symbol() string
	BaseAsset() string
	QuoteAsset() string
	TickSize() int64
	MinOrderSize() int64
	ValidatePrice(price int64) bool
	ValidateQty(qty int64) bool
	IsPerp() bool
}

type SpotInstrument struct {
	symbol       string
	base         string
	quote        string
	tickSize     int64
	minOrderSize int64
}

func NewSpotInstrument(symbol, base, quote string, tickSize, minOrderSize int64) *SpotInstrument {
	return &SpotInstrument{
		symbol:       symbol,
		base:         base,
		quote:        quote,
		tickSize:     tickSize,
		minOrderSize: minOrderSize,
	}
}

func (i *SpotInstrument) Symbol() string {
	return i.symbol
}

func (i *SpotInstrument) BaseAsset() string {
	return i.base
}

func (i *SpotInstrument) QuoteAsset() string {
	return i.quote
}

func (i *SpotInstrument) TickSize() int64 {
	return i.tickSize
}

func (i *SpotInstrument) MinOrderSize() int64 {
	return i.minOrderSize
}

func (i *SpotInstrument) ValidatePrice(price int64) bool {
	return price > 0 && price%i.tickSize == 0
}

func (i *SpotInstrument) ValidateQty(qty int64) bool {
	return qty > 0
}

func (i *SpotInstrument) IsPerp() bool {
	return false
}

type PerpFutures struct {
	SpotInstrument
	fundingRate *FundingRate
	fundingCalc FundingCalculator
}

func NewPerpFutures(symbol, base, quote string, tickSize, minOrderSize int64) *PerpFutures {
	return &PerpFutures{
		SpotInstrument: SpotInstrument{
			symbol:       symbol,
			base:         base,
			quote:        quote,
			tickSize:     tickSize,
			minOrderSize: minOrderSize,
		},
		fundingRate: &FundingRate{
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
	}
}

func (p *PerpFutures) IsPerp() bool {
	return true
}

func (p *PerpFutures) GetFundingRate() *FundingRate {
	return p.fundingRate
}

func (p *PerpFutures) UpdateFundingRate(indexPrice int64, markPrice int64) {
	p.fundingRate.IndexPrice = indexPrice
	p.fundingRate.MarkPrice = markPrice
	p.fundingRate.Rate = p.fundingCalc.Calculate(indexPrice, markPrice)
}

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
	rate := c.BaseRate + (premium * c.Damping / 100)
	if rate > c.MaxRate {
		return c.MaxRate
	}
	if rate < -c.MaxRate {
		return -c.MaxRate
	}
	return rate
}
