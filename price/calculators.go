package price

import (
	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

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

// Index-anchored mark price models — all require a PriceSource for the external spot reference.

// MedianMarkPrice marks at the median of index, best bid, and best ask.
// Requires moving two of three inputs to manipulate.
type MedianMarkPrice struct {
	index  etypes.PriceSource
	symbol string
}

func NewMedianMarkPrice(symbol string, index etypes.PriceSource) *MedianMarkPrice {
	return &MedianMarkPrice{symbol: symbol, index: index}
}

func (c *MedianMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.Price(c.symbol)

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

// EMAMarkPrice marks at index + EMA(perp_mid - index).
// The EMA smooths transient basis noise. windowSamples is the effective EMA
// window (e.g. 600 for 30 min at 3s sampling).
type EMAMarkPrice struct {
	alpha    int64 // 2/(N+1) * 10000, fixed-point
	emaBasis int64
	index    etypes.PriceSource
	symbol   string
}

func NewEMAMarkPrice(symbol string, index etypes.PriceSource, windowSamples int) *EMAMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &EMAMarkPrice{
		alpha:  20000 / int64(windowSamples+1),
		index:  index,
		symbol: symbol,
	}
}

func (c *EMAMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.Price(c.symbol)
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

// ClampedEMAMarkPrice marks at index + clamp(EMA(perp_mid - index), -band, +band).
// The hard clamp prevents the mark from drifting more than bandBps/2 from the index.
type ClampedEMAMarkPrice struct {
	alpha    int64
	emaBasis int64
	bandBps  int64 // half-band = bandBps/2 * index / 10000
	index    etypes.PriceSource
	symbol   string
}

func NewClampedEMAMarkPrice(symbol string, index etypes.PriceSource, windowSamples int, bandBps int64) *ClampedEMAMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &ClampedEMAMarkPrice{
		alpha:   20000 / int64(windowSamples+1),
		bandBps: bandBps,
		index:   index,
		symbol:  symbol,
	}
}

func (c *ClampedEMAMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.Price(c.symbol)
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

// TWAPMarkPrice marks at index + clamp(TWAP(perp_mid - index, window), -band, +band).
// Uses a rolling TWAP of the basis over a configurable sample window.
type TWAPMarkPrice struct {
	window  []int64 // rolling window of recent basis samples
	pos     int
	size    int
	bandBps int64
	index   etypes.PriceSource
	symbol  string
}

func NewTWAPMarkPrice(symbol string, index etypes.PriceSource, windowSamples int, bandBps int64) *TWAPMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &TWAPMarkPrice{
		window:  make([]int64, windowSamples),
		bandBps: bandBps,
		index:   index,
		symbol:  symbol,
	}
}

func (c *TWAPMarkPrice) Calculate(book *ebook.OrderBook) int64 {
	indexPrice := c.index.Price(c.symbol)
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
