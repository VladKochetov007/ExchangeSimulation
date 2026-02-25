package price

// MidPriceOracle derives the price for a symbol from the mid price of a mapped book.
type MidPriceOracle struct {
	provider  BookProvider
	symbolMap map[string]string
}

func NewMidPriceOracle(provider BookProvider) *MidPriceOracle {
	return &MidPriceOracle{
		provider:  provider,
		symbolMap: make(map[string]string),
	}
}

func (o *MidPriceOracle) MapSymbol(from, to string) {
	o.symbolMap[from] = to
}

func (o *MidPriceOracle) GetPrice(symbol string) int64 {
	mapped := o.symbolMap[symbol]
	if mapped == "" {
		return 0
	}
	book := o.provider.GetBook(mapped)
	if book == nil {
		return 0
	}
	return book.GetMidPrice()
}

// StaticPriceOracle returns fixed prices, useful for testing.
type StaticPriceOracle struct {
	prices map[string]int64
}

func NewStaticPriceOracle(prices map[string]int64) *StaticPriceOracle {
	return &StaticPriceOracle{prices: prices}
}

func (o *StaticPriceOracle) GetPrice(symbol string) int64 {
	return o.prices[symbol]
}

// DynamicPriceOracle delegates price lookup to a user-supplied function.
type DynamicPriceOracle struct {
	calculator func(symbol string) int64
}

func NewDynamicPriceOracle(calculator func(string) int64) *DynamicPriceOracle {
	return &DynamicPriceOracle{calculator: calculator}
}

func (o *DynamicPriceOracle) GetPrice(symbol string) int64 {
	return o.calculator(symbol)
}
