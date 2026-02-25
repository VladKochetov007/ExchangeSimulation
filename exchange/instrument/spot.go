package instrument

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

func (i *SpotInstrument) Symbol() string        { return i.symbol }
func (i *SpotInstrument) BaseAsset() string      { return i.base }
func (i *SpotInstrument) QuoteAsset() string     { return i.quote }
func (i *SpotInstrument) BasePrecision() int64   { return i.basePrecision }
func (i *SpotInstrument) QuotePrecision() int64  { return i.quotePrecision }
func (i *SpotInstrument) TickSize() int64        { return i.tickSize }
func (i *SpotInstrument) MinOrderSize() int64    { return i.minOrderSize }
func (i *SpotInstrument) IsPerp() bool           { return false }
func (i *SpotInstrument) InstrumentType() string { return "SPOT" }

func (i *SpotInstrument) ValidatePrice(price int64) bool {
	return price > 0 && price%i.tickSize == 0
}

func (i *SpotInstrument) ValidateQty(qty int64) bool {
	return qty > 0
}
