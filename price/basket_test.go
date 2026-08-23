package price

import (
	"sync"
	"testing"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

type fixedSource struct{ p int64 }

func (s *fixedSource) Price(string) (int64, error) {
	if s.p <= 0 {
		return 0, etypes.ErrNoPrice
	}
	return s.p, nil
}

func basketOf(policy DeviationPolicy, minSources int, prices ...int64) *BasketIndex {
	sources := make([]BasketSource, 0, len(prices))
	for _, p := range prices {
		sources = append(sources, BasketSource{Source: &fixedSource{p}, Weight: 1})
	}
	return NewBasketIndex(sources, policy, minSources)
}

func TestBasketIndexWeightedAverage(t *testing.T) {
	b := NewBasketIndex([]BasketSource{
		{Source: &fixedSource{100}, Weight: 3},
		{Source: &fixedSource{200}, Weight: 1},
	}, nil, 1)
	if got := mustPrice(t)(b.Price("X")); got != 125 {
		t.Fatalf("weighted average = %d, want 125", got)
	}
}

func TestBasketIndexClampsOutlierTowardMedian(t *testing.T) {
	// Median 10000; the 12000 source (+20%) clamps to +1% = 10100.
	b := basketOf(ClampToMedian{ThresholdBps: 100}, 1, 10000, 10000, 12000)
	want := (int64(10000) + 10000 + 10100) / 3
	if got := mustPrice(t)(b.Price("X")); got != want {
		t.Fatalf("clamped index = %d, want %d", got, want)
	}
}

func TestBasketIndexExcludesOutlier(t *testing.T) {
	b := basketOf(ExcludeOutliers{ThresholdBps: 100}, 1, 10000, 10000, 12000)
	if got := mustPrice(t)(b.Price("X")); got != 10000 {
		t.Fatalf("index with outlier excluded = %d, want 10000", got)
	}
}

func TestBasketIndexDeadSourcesDropOut(t *testing.T) {
	b := basketOf(nil, 2, 10000, 0, 10020)
	if got := mustPrice(t)(b.Price("X")); got != 10010 {
		t.Fatalf("index over live sources = %d, want 10010", got)
	}
}

func TestBasketIndexBelowMinSourcesReturnsUnavailable(t *testing.T) {
	b := basketOf(nil, 2, 10000, 0, 0)
	if _, err := b.Price("X"); err == nil {
		t.Fatal("degraded basket returned a price")
	}
	// Median of the pair is 15000; both sources deviate >0.01% and are
	// excluded — a basket with every weight zeroed is also unavailable.
	all := basketOf(ExcludeOutliers{ThresholdBps: 1}, 1, 10000, 20000)
	if _, err := all.Price("X"); err == nil {
		t.Fatal("fully-excluded basket returned a price")
	}
}

// Stateful mark calculators are shared between the automation loop and
// manual UpdatePerpPrices passes; Calculate must be internally synchronized.
// Run with -race: the pre-fix calculators fail here.
func TestMarkCalculatorsConcurrentCalculate(t *testing.T) {
	idx := &fixedSource{1_000_000}
	book := newRegressionBookWithMid(1_000_100)

	calcs := []interface {
		Calculate(*ebook.OrderBook) (int64, error)
	}{
		NewEMAMarkPrice("X", idx, 10),
		NewClampedEMAMarkPrice("X", idx, 10, 600),
		NewTWAPMarkPrice("X", idx, 10, 600),
		NewShrunkBasisMarkPrice("X", idx, 10, 30, 3000, 7000),
		NewBinanceMedianMarkPrice("X", idx, nil, nil, 10),
	}
	var wg sync.WaitGroup
	for _, c := range calcs {
		for range 4 {
			wg.Add(1)
			go func(calc interface {
				Calculate(*ebook.OrderBook) (int64, error)
			}) {
				defer wg.Done()
				for range 500 {
					_, _ = calc.Calculate(book)
				}
			}(c)
		}
	}
	wg.Wait()
}

var _ etypes.PriceSource = (*BasketIndex)(nil)
