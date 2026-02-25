package instrument

import etypes "exchange_sim/exchange/types"

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
