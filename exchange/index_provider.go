package exchange

// IndexPriceProvider provides index prices for perpetual contracts
// Index price represents the "fair value" reference price
type IndexPriceProvider interface {
	GetIndexPrice(perpSymbol string, timestamp int64) int64
}

// SpotIndexProvider uses a spot instrument on the same exchange as index
// Maps perpetual symbol to corresponding spot symbol
type SpotIndexProvider struct {
	exchange       *Exchange
	perpToSpotMap  map[string]string // perpSymbol -> spotSymbol
	priceCalc      MarkPriceCalculator
}

// NewSpotIndexProvider creates a provider that uses spot prices as index
func NewSpotIndexProvider(exchange *Exchange) *SpotIndexProvider {
	return &SpotIndexProvider{
		exchange:      exchange,
		perpToSpotMap: make(map[string]string),
		priceCalc:     NewMidPriceCalculator(), // Use mid price for spot
	}
}

// MapPerpToSpot maps a perpetual symbol to its spot counterpart
// Example: MapPerpToSpot("BTC-PERP", "BTC/USD")
func (p *SpotIndexProvider) MapPerpToSpot(perpSymbol, spotSymbol string) {
	p.perpToSpotMap[perpSymbol] = spotSymbol
}

func (p *SpotIndexProvider) GetIndexPrice(perpSymbol string, timestamp int64) int64 {
	spotSymbol, ok := p.perpToSpotMap[perpSymbol]
	if !ok {
		return 0
	}

	p.exchange.mu.RLock()
	spotBook, exists := p.exchange.Books[spotSymbol]
	p.exchange.mu.RUnlock()

	if !exists {
		return 0
	}

	return p.priceCalc.Calculate(spotBook)
}

// FixedIndexProvider returns a fixed index price
// Useful for testing and simulations with controlled scenarios
type FixedIndexProvider struct {
	prices map[string]int64 // perpSymbol -> fixedPrice
}

func NewFixedIndexProvider() *FixedIndexProvider {
	return &FixedIndexProvider{
		prices: make(map[string]int64),
	}
}

// SetPrice sets the fixed index price for a symbol
func (p *FixedIndexProvider) SetPrice(symbol string, price int64) {
	p.prices[symbol] = price
}

func (p *FixedIndexProvider) GetIndexPrice(perpSymbol string, timestamp int64) int64 {
	return p.prices[perpSymbol]
}

// DynamicIndexProvider uses a custom function to calculate index price
// Maximum flexibility for custom index calculations
type DynamicIndexProvider struct {
	calculator func(perpSymbol string, timestamp int64) int64
}

func NewDynamicIndexProvider(calculator func(string, int64) int64) *DynamicIndexProvider {
	return &DynamicIndexProvider{
		calculator: calculator,
	}
}

func (p *DynamicIndexProvider) GetIndexPrice(perpSymbol string, timestamp int64) int64 {
	return p.calculator(perpSymbol, timestamp)
}
