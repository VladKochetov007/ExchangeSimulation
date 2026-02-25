package exchange

type MidPriceOracle struct {
	exchange  *Exchange
	symbolMap map[string]string
}

func NewMidPriceOracle(exchange *Exchange) *MidPriceOracle {
	return &MidPriceOracle{
		exchange:  exchange,
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

	o.exchange.mu.RLock()
	defer o.exchange.mu.RUnlock()

	book := o.exchange.Books[mapped]
	if book == nil {
		return 0
	}
	return book.GetMidPrice()
}

type StaticPriceOracle struct {
	prices map[string]int64
}

func NewStaticPriceOracle(prices map[string]int64) *StaticPriceOracle {
	return &StaticPriceOracle{prices: prices}
}

func (o *StaticPriceOracle) GetPrice(symbol string) int64 {
	return o.prices[symbol]
}

type DynamicPriceOracle struct {
	calculator func(symbol string) int64
}

func NewDynamicPriceOracle(calculator func(string) int64) *DynamicPriceOracle {
	return &DynamicPriceOracle{calculator: calculator}
}

func (o *DynamicPriceOracle) GetPrice(symbol string) int64 {
	return o.calculator(symbol)
}
