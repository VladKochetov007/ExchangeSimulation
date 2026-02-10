package exchange

type SimplePriceOracle struct {
	exchange      *Exchange
	assetToSymbol map[string]string
}

func NewSimplePriceOracle(exchange *Exchange) *SimplePriceOracle {
	return &SimplePriceOracle{
		exchange:      exchange,
		assetToSymbol: make(map[string]string),
	}
}

func (o *SimplePriceOracle) MapAssetToSymbol(asset, symbol string) {
	o.assetToSymbol[asset] = symbol
}

func (o *SimplePriceOracle) GetPrice(asset string) int64 {
	symbol := o.assetToSymbol[asset]
	if symbol == "" {
		return 0
	}

	o.exchange.mu.RLock()
	defer o.exchange.mu.RUnlock()

	book := o.exchange.Books[symbol]
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

func (o *StaticPriceOracle) GetPrice(asset string) int64 {
	return o.prices[asset]
}
