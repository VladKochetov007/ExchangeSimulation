package price

import (
	"testing"

	etypes "exchange_sim/types"
)

// --- ShrunkBasisMarkPrice ---

// No basis history: only the floor share of the basis is carried.
func TestShrunkBasisNoHistoryCarriesFloor(t *testing.T) {
	idx := &fixedSource{1_000_000}
	c := NewShrunkBasisMarkPrice("X", idx, 30, 300, 3000, 7000)

	mark := c.Calculate(newRegressionBookWithMid(1_010_000)) // basis +10,000
	if want := int64(1_000_000 + 3000); mark != want {
		t.Fatalf("first-sample mark = %d, want floor-shrunk %d", mark, want)
	}
}

// A steady basis saturates the ratio and is capped at CMax — the mark never
// carries the full basis, keeping at least 30%% index dominance.
func TestShrunkBasisSteadyBasisCapsAtCMax(t *testing.T) {
	idx := &fixedSource{1_000_000}
	c := NewShrunkBasisMarkPrice("X", idx, 30, 300, 3000, 7000)

	book := newRegressionBookWithMid(1_010_000)
	var mark int64
	for range 40 {
		mark = c.Calculate(book)
	}
	if want := int64(1_000_000 + 7000); mark != want {
		t.Fatalf("steady-basis mark = %d, want CMax-shrunk %d (full basis would be 1,010,000)", mark, want)
	}
}

// A basis that collapses relative to its recent maximum shrinks to the floor:
// the ratio |dP|/dPmax self-calibrates per symbol.
func TestShrunkBasisCollapsedBasisShrinksToFloor(t *testing.T) {
	idx := &fixedSource{1_000_000}
	c := NewShrunkBasisMarkPrice("X", idx, 2, 300, 3000, 7000)

	wide := newRegressionBookWithMid(1_010_000) // |dP| = 10,000 history
	for range 5 {
		c.Calculate(wide)
	}
	narrow := newRegressionBookWithMid(1_000_100) // |dP| = 100, ratio 1% -> floor
	mark := c.Calculate(narrow)

	// MA window 2: (10,000 + 100)/2 = 5,050 smoothed basis, floor share 30%.
	if want := int64(1_000_000 + 5050*3000/10000); mark != want {
		t.Fatalf("collapsed-basis mark = %d, want %d", mark, want)
	}
}

func TestShrunkBasisNegativeBasisSymmetric(t *testing.T) {
	idx := &fixedSource{1_000_000}
	c := NewShrunkBasisMarkPrice("X", idx, 30, 300, 3000, 7000)

	book := newRegressionBookWithMid(990_000) // basis -10,000
	var mark int64
	for range 40 {
		mark = c.Calculate(book)
	}
	if want := int64(1_000_000 - 7000); mark != want {
		t.Fatalf("negative steady basis mark = %d, want %d", mark, want)
	}
}

// --- BinanceMedianMarkPrice ---

type fixedClock struct{ now int64 }

func (c *fixedClock) NowUnixNano() int64 { return c.now }
func (c *fixedClock) NowUnix() int64     { return c.now / 1e9 }

func TestBinanceMedianPicksMiddleTerm(t *testing.T) {
	idx := &fixedSource{1_000_000}
	clock := &fixedClock{now: 0}
	// Rate 100 bps, half the interval remaining: P1 = I·(1 + 0.01·0.5) = 1,005,000.
	funding := func() etypes.FundingRate {
		return etypes.FundingRate{Rate: 100, NextFunding: 14400 * 1e9, Interval: 28800}
	}
	c := NewBinanceMedianMarkPrice("X", idx, clock, funding, 30)

	book := newRegressionBookWithMid(1_001_000) // P2 seeds to 1,001,000
	book.LastTrade = &etypes.Trade{Price: 1_020_000}

	mark := c.Calculate(book)
	if want := int64(1_005_000); mark != want {
		t.Fatalf("median mark = %d, want P1 %d (P2=1,001,000 lastTrade=1,020,000)", mark, want)
	}
}

// Documented fallback: no trade print collapses the mark to P2, never to the
// min/max of the remaining pair.
func TestBinanceMedianNoLastTradeCollapsesToP2(t *testing.T) {
	idx := &fixedSource{1_000_000}
	funding := func() etypes.FundingRate {
		return etypes.FundingRate{Rate: 5000, NextFunding: 28800 * 1e9, Interval: 28800}
	}
	c := NewBinanceMedianMarkPrice("X", idx, &fixedClock{}, funding, 30)

	book := newRegressionBookWithMid(1_001_000)
	book.LastTrade = nil

	if mark := c.Calculate(book); mark != 1_001_000 {
		t.Fatalf("no-trade mark = %d, want P2 1,001,000 (P1 would be 1,050,000)", mark)
	}
}

// Last Price Protected: with no index reference the mark follows the
// contract's own last trade.
func TestBinanceMedianNoIndexUsesLastPrice(t *testing.T) {
	c := NewBinanceMedianMarkPrice("X", &fixedSource{0}, &fixedClock{}, nil, 30)

	book := newRegressionBookWithMid(1_001_000)
	book.LastTrade = &etypes.Trade{Price: 999_000}

	if mark := c.Calculate(book); mark != 999_000 {
		t.Fatalf("no-index mark = %d, want last trade 999,000", mark)
	}
}

func TestBinanceMedianNilFundingDegeneratesToP2vsLast(t *testing.T) {
	idx := &fixedSource{1_000_000}
	c := NewBinanceMedianMarkPrice("X", idx, nil, nil, 30)

	book := newRegressionBookWithMid(1_001_000)
	book.LastTrade = &etypes.Trade{Price: 990_000}

	// P1 == P2 = 1,001,000; median(1,001,000, 1,001,000, 990,000) = P2.
	if mark := c.Calculate(book); mark != 1_001_000 {
		t.Fatalf("nil-funding mark = %d, want 1,001,000", mark)
	}
}
