package price

import (
	ebook "exchange_sim/exchange/book"
	etypes "exchange_sim/exchange/types"
)

// MarkPriceCalculator calculates the mark price from an order book.
type MarkPriceCalculator interface {
	Calculate(book *ebook.OrderBook) int64
}

// BookProvider provides read access to order books by symbol.
// *exchange.Exchange satisfies this interface via GetBook.
type BookProvider interface {
	GetBook(symbol string) *ebook.OrderBook
}

// LastPriceCalculator uses the last trade price as mark price.
// Simplest but manipulable by wash trading. Do not use for liquidation triggers.
type LastPriceCalculator struct{}

func NewLastPriceCalculator() *LastPriceCalculator { return &LastPriceCalculator{} }

func (c *LastPriceCalculator) Calculate(book *ebook.OrderBook) int64 {
	return book.GetLastPrice()
}

// MidPriceCalculator uses the mid price between best bid and ask.
type MidPriceCalculator struct{}

func NewMidPriceCalculator() *MidPriceCalculator { return &MidPriceCalculator{} }

func (c *MidPriceCalculator) Calculate(book *ebook.OrderBook) int64 {
	return book.GetMidPrice()
}

// WeightedMidPriceCalculator uses quantity-weighted mid price.
// Weights by available quantity at best levels: thicker side pulls mid toward it.
type WeightedMidPriceCalculator struct{}

func NewWeightedMidPriceCalculator() *WeightedMidPriceCalculator {
	return &WeightedMidPriceCalculator{}
}

func (c *WeightedMidPriceCalculator) Calculate(book *ebook.OrderBook) int64 {
	if book.Bids.Best == nil || book.Asks.Best == nil {
		return book.GetLastPrice()
	}

	bidQty := book.Bids.Best.TotalQty
	askQty := book.Asks.Best.TotalQty
	bidPrice := book.Bids.Best.Price
	askPrice := book.Asks.Best.Price

	if bidQty == 0 && askQty == 0 {
		return bidPrice + (askPrice-bidPrice)/2
	}
	if bidQty == 0 {
		return askPrice
	}
	if askQty == 0 {
		return bidPrice
	}

	totalWeight := bidQty + askQty
	return (bidPrice*askQty + askPrice*bidQty) / totalWeight
}

// --- Index-anchored mark price models ---
//
// All manipulation-resistant models below require a PriceOracle.
// The index is an external spot reference that an attacker cannot move by
// trading the perp book alone.

// BinanceMarkPrice implements the Binance perpetuals mark price model.
//
//	mark = median(index, best_bid, best_ask)
//
// Requires moving two of three inputs to manipulate.
type BinanceMarkPrice struct {
	index  etypes.PriceOracle
	symbol string
}

func NewBinanceMarkPrice(symbol string, index etypes.PriceOracle) *BinanceMarkPrice {
	return &BinanceMarkPrice{symbol: symbol, index: index}
}

func (c *BinanceMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.GetPrice(c.symbol)

	var bid, ask int64
	if book.Bids.Best != nil {
		bid = book.Bids.Best.Price
	}
	if book.Asks.Best != nil {
		ask = book.Asks.Best.Price
	}

	if bid == 0 || ask == 0 || indexPrice == 0 {
		if indexPrice != 0 {
			return indexPrice
		}
		return book.GetMidPrice()
	}

	return median3(bid, ask, indexPrice)
}

// BitMEXMarkPrice implements the BitMEX fair price mark model.
//
//	mark = index + EMA(perp_mid - index)
//
// The EMA smooths transient basis noise. windowSamples is the effective EMA
// window (e.g. 600 for 30 min at 3s sampling).
type BitMEXMarkPrice struct {
	alpha    int64 // 2/(N+1) * 10000, fixed-point
	emaBasis int64
	index    etypes.PriceOracle
	symbol   string
}

func NewBitMEXMarkPrice(symbol string, index etypes.PriceOracle, windowSamples int) *BitMEXMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &BitMEXMarkPrice{
		alpha:  20000 / int64(windowSamples+1),
		index:  index,
		symbol: symbol,
	}
}

func (c *BitMEXMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.GetPrice(c.symbol)
	if indexPrice == 0 {
		return book.GetMidPrice()
	}

	perpMid := book.GetMidPrice()
	if perpMid == 0 {
		return indexPrice
	}

	basis := perpMid - indexPrice
	if c.emaBasis == 0 {
		c.emaBasis = basis
	} else {
		c.emaBasis = (c.alpha*basis + (10000-c.alpha)*c.emaBasis) / 10000
	}

	return indexPrice + c.emaBasis
}

// BybitMarkPrice implements the Bybit mark price model.
//
//	mark = index + clamp(EMA(perp_mid - index), -band, +band)
//
// Adds a hard clamp so the mark can never drift more than bandBps/2 away from
// the index. bandBps is typically the initial margin rate in bps.
type BybitMarkPrice struct {
	alpha    int64
	emaBasis int64
	bandBps  int64 // half-band = bandBps/2 * index / 10000
	index    etypes.PriceOracle
	symbol   string
}

func NewBybitMarkPrice(symbol string, index etypes.PriceOracle, windowSamples int, bandBps int64) *BybitMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &BybitMarkPrice{
		alpha:   20000 / int64(windowSamples+1),
		bandBps: bandBps,
		index:   index,
		symbol:  symbol,
	}
}

func (c *BybitMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.GetPrice(c.symbol)
	if indexPrice == 0 {
		return book.GetMidPrice()
	}

	perpMid := book.GetMidPrice()
	if perpMid == 0 {
		return indexPrice
	}

	basis := perpMid - indexPrice
	if c.emaBasis == 0 {
		c.emaBasis = basis
	} else {
		c.emaBasis = (c.alpha*basis + (10000-c.alpha)*c.emaBasis) / 10000
	}

	// clamp: |mark - index| <= index * bandBps/2 / 10000
	halfBand := indexPrice * c.bandBps / 2 / 10000
	if c.emaBasis > halfBand {
		c.emaBasis = halfBand
	} else if c.emaBasis < -halfBand {
		c.emaBasis = -halfBand
	}

	return indexPrice + c.emaBasis
}

// DydxMarkPrice implements the dYdX oracle-anchored mark price model.
//
//	mark = index + clamp(TWAP(perp_mid - index, window), -band, +band)
//
// Uses a TWAP of the premium over a configurable window of samples.
type DydxMarkPrice struct {
	window  []int64 // rolling window of recent basis samples
	pos     int
	size    int
	bandBps int64
	index   etypes.PriceOracle
	symbol  string
}

func NewDydxMarkPrice(symbol string, index etypes.PriceOracle, windowSamples int, bandBps int64) *DydxMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &DydxMarkPrice{
		window:  make([]int64, windowSamples),
		bandBps: bandBps,
		index:   index,
		symbol:  symbol,
	}
}

func (c *DydxMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.GetPrice(c.symbol)
	if indexPrice == 0 {
		return book.GetMidPrice()
	}

	perpMid := book.GetMidPrice()
	if perpMid == 0 {
		return indexPrice
	}

	// update circular TWAP buffer
	c.window[c.pos] = perpMid - indexPrice
	c.pos = (c.pos + 1) % len(c.window)
	if c.size < len(c.window) {
		c.size++
	}

	twapBasis := int64(0)
	for i := 0; i < c.size; i++ {
		twapBasis += c.window[i]
	}
	twapBasis /= int64(c.size)

	halfBand := indexPrice * c.bandBps / 2 / 10000
	if twapBasis > halfBand {
		twapBasis = halfBand
	} else if twapBasis < -halfBand {
		twapBasis = -halfBand
	}

	return indexPrice + twapBasis
}

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

// median3 returns the median of three int64 values.
func median3(a, b, d int64) int64 {
	if a > b {
		a, b = b, a
	}
	if b > d {
		b, d = d, b
	}
	if a > b {
		b = a
	}
	_ = d
	return b
}
