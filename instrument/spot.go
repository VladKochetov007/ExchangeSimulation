package instrument

import (
	"fmt"

	etypes "exchange_sim/types"
)

type SpotInstrument struct {
	symbol         string
	base           string
	quote          string
	basePrecision  int64
	quotePrecision int64
	tickSize       int64
	minOrderSize   int64
	priceDomain    etypes.PriceDomain
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
		priceDomain:    etypes.PositivePriceDomain(tickSize),
	}
}

func (i *SpotInstrument) Symbol() string        { return i.symbol }
func (i *SpotInstrument) BaseAsset() string     { return i.base }
func (i *SpotInstrument) QuoteAsset() string    { return i.quote }
func (i *SpotInstrument) BasePrecision() int64  { return i.basePrecision }
func (i *SpotInstrument) QuotePrecision() int64 { return i.quotePrecision }
func (i *SpotInstrument) TickSize() int64       { return i.tickSize }
func (i *SpotInstrument) MinOrderSize() int64   { return i.minOrderSize }
func (i *SpotInstrument) PriceDomain() etypes.PriceDomain {
	return i.priceDomain
}
func (i *SpotInstrument) IsPerp() bool           { return false }
func (i *SpotInstrument) InstrumentType() string { return "SPOT" }

func (i *SpotInstrument) ValidatePrice(price int64) bool {
	return i.PriceDomain().Validate(price)
}

// setPriceDomain changes this instrument's declared signed numeric price
// domain. It is configuration-time only: callers must set a domain using the
// instrument's own tick size, so a sign-policy change cannot also change tick
// admission implicitly.
func (i *SpotInstrument) setPriceDomain(domain etypes.PriceDomain) error {
	if domain.TickSize() != i.tickSize {
		return fmt.Errorf("price domain tick %d does not match instrument tick %d", domain.TickSize(), i.tickSize)
	}
	i.priceDomain = domain
	return nil
}

func (i *SpotInstrument) ValidateQty(qty int64) bool {
	return qty >= i.minOrderSize
}
