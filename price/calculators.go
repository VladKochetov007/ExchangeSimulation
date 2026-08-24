package price

import (
	"sync"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

// LastPriceCalculator uses the last trade price as mark price.
// Simplest but manipulable by wash trading. Do not use for liquidation triggers.
type LastPriceCalculator struct{}

func NewLastPriceCalculator() *LastPriceCalculator { return &LastPriceCalculator{} }

func (c *LastPriceCalculator) Calculate(book *ebook.OrderBook) (int64, error) {
	return book.GetLastPrice()
}

// MidPriceCalculator uses the mid price between best bid and ask.
type MidPriceCalculator struct{}

func NewMidPriceCalculator() *MidPriceCalculator { return &MidPriceCalculator{} }

func (c *MidPriceCalculator) Calculate(book *ebook.OrderBook) (int64, error) {
	return book.GetMidPrice()
}

// WeightedMidPriceCalculator uses quantity-weighted mid price.
// Weights by available quantity at best levels: thicker side pulls mid toward it.
type WeightedMidPriceCalculator struct{}

func NewWeightedMidPriceCalculator() *WeightedMidPriceCalculator {
	return &WeightedMidPriceCalculator{}
}

func (c *WeightedMidPriceCalculator) Calculate(book *ebook.OrderBook) (int64, error) {
	bidPrice, err := book.GetBestBid()
	if err != nil {
		return 0, err
	}
	askPrice, err := book.GetBestAsk()
	if err != nil {
		return 0, err
	}
	if bidPrice > askPrice {
		return 0, etypes.ErrNoPrice
	}
	bidQty := book.Bids.Best.TotalQty
	askQty := book.Asks.Best.TotalQty
	if bidQty < 0 || askQty < 0 {
		return 0, etypes.ErrNoPrice
	}

	if bidQty == 0 && askQty == 0 {
		return etypes.Midpoint(bidPrice, askPrice), nil
	}
	if bidQty == 0 {
		return askPrice, nil
	}
	if askQty == 0 {
		return bidPrice, nil
	}

	// Weighted mid = bid + spread × bidQty/(bidQty+askQty); avoids the
	// price×qty product, which overflows int64 at realistic sizes.
	totalWeight, ok := etypes.TryAdd(bidQty, askQty)
	if !ok || totalWeight <= 0 {
		// Both sides can be individually valid while their combined depth
		// exceeds int64. The ordinary midpoint stays well-defined and avoids
		// allowing an aggregate overflow to move a mark price.
		return etypes.Midpoint(bidPrice, askPrice), nil
	}
	return bidPrice + etypes.MulDiv(askPrice-bidPrice, bidQty, totalWeight), nil
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

func (c *MedianMarkPrice) Calculate(book *ebook.OrderBook) (int64, error) {
	indexPrice, err := positiveSourcePrice(c.index, c.symbol)
	if err != nil {
		return 0, err
	}

	var bid, ask int64
	if book.Bids.Best != nil {
		bid = book.Bids.Best.Price
	}
	if book.Asks.Best != nil {
		ask = book.Asks.Best.Price
	}

	if book.Bids.Best == nil || book.Asks.Best == nil || bid > ask {
		return indexPrice, nil
	}

	return median3(bid, ask, indexPrice), nil
}

// EMAMarkPrice marks at index + EMA(perp_mid - index).
// The EMA smooths transient basis noise. windowSamples is the effective EMA
// window (e.g. 600 for 30 min at 3s sampling).
type EMAMarkPrice struct {
	alpha    int64 // 2/(N+1) * 10000, fixed-point
	emaBasis int64
	// seeded distinguishes "no sample yet" from a basis that legitimately
	// decayed to zero: reusing 0 as the sentinel would re-seed the EMA from one
	// raw print, discarding all smoothing in a single step.
	seeded bool
	// mu guards the EMA state: Calculate may be driven concurrently by the
	// automation price loop and a manual UpdatePerpPrices pass, and the
	// exchange invokes calculators under a read lock, which permits
	// concurrent holders.
	mu     sync.Mutex
	index  etypes.PriceSource
	symbol string
}

func NewEMAMarkPrice(symbol string, index etypes.PriceSource, windowSamples int) *EMAMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &EMAMarkPrice{
		alpha:  emaAlpha(windowSamples),
		index:  index,
		symbol: symbol,
	}
}

// emaAlpha returns 2/(N+1) in fixed-point (×10000), floored at 1 so a very
// large window cannot round the coefficient to zero and freeze the EMA at its
// seed value forever.
func emaAlpha(windowSamples int) int64 {
	if a := 20000 / int64(windowSamples+1); a >= 1 {
		return a
	}
	return 1
}

func (c *EMAMarkPrice) Calculate(book *ebook.OrderBook) (int64, error) {
	indexPrice, err := positiveSourcePrice(c.index, c.symbol)
	if err != nil {
		return 0, err
	}

	perpMid, err := book.GetMidPrice()
	if err != nil {
		return indexPrice, nil
	}

	basis := perpMid - indexPrice
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.seeded {
		c.emaBasis = basis
		c.seeded = true
	} else {
		c.emaBasis = (c.alpha*basis + (10000-c.alpha)*c.emaBasis) / 10000
	}

	return indexPrice + c.emaBasis, nil
}

// ClampedEMAMarkPrice marks at index + clamp(EMA(perp_mid - index), -band, +band).
// The hard clamp prevents the mark from drifting more than bandBps/2 from the index.
type ClampedEMAMarkPrice struct {
	alpha    int64
	emaBasis int64
	seeded   bool
	mu       sync.Mutex // see EMAMarkPrice.mu
	bandBps  int64      // half-band = bandBps/2 * index / 10000
	index    etypes.PriceSource
	symbol   string
}

func NewClampedEMAMarkPrice(symbol string, index etypes.PriceSource, windowSamples int, bandBps int64) *ClampedEMAMarkPrice {
	if windowSamples < 1 {
		windowSamples = 1
	}
	return &ClampedEMAMarkPrice{
		alpha:   emaAlpha(windowSamples),
		bandBps: bandBps,
		index:   index,
		symbol:  symbol,
	}
}

func (c *ClampedEMAMarkPrice) Calculate(book *ebook.OrderBook) (int64, error) {
	indexPrice, err := positiveSourcePrice(c.index, c.symbol)
	if err != nil {
		return 0, err
	}

	perpMid, err := book.GetMidPrice()
	if err != nil {
		return indexPrice, nil
	}

	basis := perpMid - indexPrice
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.seeded {
		c.emaBasis = basis
		c.seeded = true
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

	return indexPrice + c.emaBasis, nil
}

// TWAPMarkPrice marks at index + clamp(TWAP(perp_mid - index, window), -band, +band).
// Uses a rolling TWAP of the basis over a configurable sample window.
type TWAPMarkPrice struct {
	window  []int64 // rolling window of recent basis samples
	pos     int
	size    int
	mu      sync.Mutex // see EMAMarkPrice.mu
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

func (c *TWAPMarkPrice) Calculate(book *ebook.OrderBook) (int64, error) {
	indexPrice, err := positiveSourcePrice(c.index, c.symbol)
	if err != nil {
		return 0, err
	}

	perpMid, err := book.GetMidPrice()
	if err != nil {
		return indexPrice, nil
	}

	// update circular TWAP buffer
	c.mu.Lock()
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
	c.mu.Unlock()

	halfBand := indexPrice * c.bandBps / 2 / 10000
	if twapBasis > halfBand {
		twapBasis = halfBand
	} else if twapBasis < -halfBand {
		twapBasis = -halfBand
	}

	return indexPrice + twapBasis, nil
}
