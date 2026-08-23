package price

import (
	"sync"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

// ShrunkBasisMarkPrice is Bybit's post-2025 mark: index plus a SHRUNK moving
// average of the basis, Mark = I + C·MA(ΔP), where the shrinkage
// C = clamp(|ΔP| / ΔPmax, CMin, CMax) is driven by the newest basis relative
// to its recent maximum (excluding that newest sample). Unlike a hard band it
// degrades continuously — no pinning at a band edge — and the ΔPmax
// normalization self-calibrates per symbol: habitually wide-basis
// instruments get a large denominator and a small C, so heterogeneous
// synthetic instruments need no per-symbol tuning.
type ShrunkBasisMarkPrice struct {
	maWindow []int64 // MA(ΔP) ring
	maPos    int
	maSize   int
	absRing  []int64 // |ΔP| ring backing the ΔPmax lookback
	absPos   int
	absSize  int
	mu       sync.Mutex // see EMAMarkPrice.mu
	cMinBps  int64      // shrinkage floor ×10000 (Bybit: 3000 = 0.3)
	cMaxBps  int64      // shrinkage cap ×10000 (Bybit: 7000 = 0.7)
	index    etypes.PriceSource
	symbol   string
}

// NewShrunkBasisMarkPrice builds the Bybit-style calculator. maSamples is the
// basis MA window, maxSamples the ΔPmax lookback (Bybit's R is unpublished),
// cMinBps/cMaxBps the shrinkage bounds ×10000. Non-positive arguments take
// the documented venue values (30 samples, 300-sample lookback, 0.3/0.7).
func NewShrunkBasisMarkPrice(symbol string, index etypes.PriceSource, maSamples, maxSamples int, cMinBps, cMaxBps int64) *ShrunkBasisMarkPrice {
	if maSamples < 1 {
		maSamples = 30
	}
	if maxSamples < 1 {
		maxSamples = 300
	}
	if cMinBps <= 0 {
		cMinBps = 3000
	}
	if cMaxBps <= 0 || cMaxBps < cMinBps {
		cMaxBps = 7000
	}
	return &ShrunkBasisMarkPrice{
		maWindow: make([]int64, maSamples),
		absRing:  make([]int64, maxSamples),
		cMinBps:  cMinBps,
		cMaxBps:  cMaxBps,
		index:    index,
		symbol:   symbol,
	}
}

func (c *ShrunkBasisMarkPrice) Calculate(book *ebook.OrderBook) (int64, error) {
	indexPrice, err := sourcePrice(c.index, c.symbol)
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

	// ΔPmax over the lookback EXCLUDING the newest sample (per the venue
	// rule): compute before inserting the current basis.
	var basisMax int64
	for i := 0; i < c.absSize; i++ {
		if v := c.absRing[i]; v > basisMax {
			basisMax = v
		}
	}

	absBasis := basis
	if absBasis < 0 {
		absBasis = -absBasis
	}
	c.absRing[c.absPos] = absBasis
	c.absPos = (c.absPos + 1) % len(c.absRing)
	if c.absSize < len(c.absRing) {
		c.absSize++
	}

	c.maWindow[c.maPos] = basis
	c.maPos = (c.maPos + 1) % len(c.maWindow)
	if c.maSize < len(c.maWindow) {
		c.maSize++
	}
	maBasis := int64(0)
	for i := 0; i < c.maSize; i++ {
		maBasis += c.maWindow[i]
	}
	maBasis /= int64(c.maSize)

	// Shrinkage: no history (or a flat one) means no evidence the basis is
	// meaningful — carry only the floor share.
	shrink := c.cMinBps
	if basisMax > 0 {
		shrink = absBasis * 10000 / basisMax
		if shrink < c.cMinBps {
			shrink = c.cMinBps
		} else if shrink > c.cMaxBps {
			shrink = c.cMaxBps
		}
	}

	return indexPrice + maBasis*shrink/10000, nil
}

// BinanceMedianMarkPrice is Binance's documented mark:
// median(P1, P2, last trade), where P1 = I·(1 + r·T_until/T_period) ramps
// the last settled funding rate toward the next settlement and
// P2 = I + MA(basis) is the smoothed-basis price. The documented fallback
// ladder is explicit: a missing last trade collapses to P2 (NOT the median
// of the remaining two, which would silently become min/max), and a missing
// index falls back to the contract's own last price ("Last Price
// Protected").
type BinanceMedianMarkPrice struct {
	basisWindow []int64
	pos         int
	size        int
	mu          sync.Mutex // see EMAMarkPrice.mu
	index       etypes.PriceSource
	clock       etypes.Clock
	// funding supplies the last settled rate (bps per interval), the next
	// settlement time (unix nanos), and the interval (seconds) — injected as
	// a closure so this package needs no instrument dependency. Nil disables
	// the P1 ramp (P1 degenerates to P2).
	funding func() etypes.FundingRate
	symbol  string
}

// NewBinanceMedianMarkPrice builds the median-of-three calculator. maSamples
// is the basis MA window (Binance: 30 one-second samples); non-positive
// takes 30.
func NewBinanceMedianMarkPrice(symbol string, index etypes.PriceSource, clock etypes.Clock, funding func() etypes.FundingRate, maSamples int) *BinanceMedianMarkPrice {
	if maSamples < 1 {
		maSamples = 30
	}
	return &BinanceMedianMarkPrice{
		basisWindow: make([]int64, maSamples),
		index:       index,
		clock:       clock,
		funding:     funding,
		symbol:      symbol,
	}
}

func (c *BinanceMedianMarkPrice) Calculate(book *ebook.OrderBook) (int64, error) {
	indexPrice, indexErr := sourcePrice(c.index, c.symbol)
	if indexErr != nil {
		// Last Price Protected: no stable index reference, fall back to the
		// contract's own last trade only.
		last, lastErr := book.GetLastPrice()
		if lastErr == nil {
			return last, nil
		}
		return 0, indexErr
	}

	c.mu.Lock()
	if perpMid, err := book.GetMidPrice(); err == nil {
		c.basisWindow[c.pos] = perpMid - indexPrice
		c.pos = (c.pos + 1) % len(c.basisWindow)
		if c.size < len(c.basisWindow) {
			c.size++
		}
	}
	maBasis := int64(0)
	if c.size > 0 {
		for i := 0; i < c.size; i++ {
			maBasis += c.basisWindow[i]
		}
		maBasis /= int64(c.size)
	}
	c.mu.Unlock()

	price2 := indexPrice + maBasis

	price1 := price2
	if c.funding != nil && c.clock != nil {
		if fr := c.funding(); fr.Interval > 0 {
			untilNanos := fr.NextFunding - c.clock.NowUnixNano()
			if untilNanos < 0 {
				untilNanos = 0
			}
			periodNanos := fr.Interval * 1e9
			// P1 = I·(1 + r·T_until/T_period), r in bps of the interval.
			price1 = indexPrice + etypes.MulDiv(indexPrice*fr.Rate/10000, untilNanos, periodNanos)
		}
	}

	lastTrade, err := book.GetLastPrice()
	if err != nil {
		// Documented fallback: median collapses to P2 when the contract has
		// no trade print, not to min/max of the remaining pair.
		return price2, nil
	}
	return median3(price1, price2, lastTrade), nil
}
