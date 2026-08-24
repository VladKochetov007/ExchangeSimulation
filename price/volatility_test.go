package price

import (
	"math"
	"testing"
)

const nanosPerSecond = int64(1_000_000_000)

// A flat model is the baseline the option dealer started from, and it has to
// stay exactly that: one number for every contract.
func TestFlatVolatilityIgnoresTheContract(t *testing.T) {
	model := FlatVolatility(0.8)
	if got := model.Volatility(50_000, 40_000, 0.25, true); got != 0.8 {
		t.Errorf("volatility = %v, want 0.8", got)
	}
	if got := model.Volatility(50_000, 90_000, 2, false); got != 0.8 {
		t.Errorf("volatility = %v, want 0.8", got)
	}
}

// The estimator has to annualise by the spacing between observations, or a
// participant sampling twice as often reports half the volatility of one
// sampling slowly on the same price path.
func TestRealizedVolatilityIsInvariantToSamplingRate(t *testing.T) {
	measure := func(stepSeconds int64, steps int) float64 {
		estimator := NewRealizedVolatility(0, 3600, 1, 0, 0)
		price := int64(50_000 * 100_000_000)
		nano := nanosPerSecond
		for i := 0; i < steps; i++ {
			// A deterministic alternating move of the same size per second, so
			// the two series have the same volatility per unit of time.
			perStep := math.Sqrt(float64(stepSeconds)) * 0.0002
			direction := 1.0
			if i%2 == 1 {
				direction = -1
			}
			price = int64(float64(price) * math.Exp(direction*perStep))
			nano += stepSeconds * nanosPerSecond
			estimator.Observe(price, nano)
		}
		return estimator.Volatility(0, 0, 0, true)
	}
	fast, slow := measure(1, 4000), measure(4, 1000)
	if fast <= 0 || slow <= 0 {
		t.Fatalf("estimator produced no volatility: fast %v slow %v", fast, slow)
	}
	if ratio := fast / slow; ratio < 0.9 || ratio > 1.1 {
		t.Errorf("sampling rate changed the estimate: fast %v slow %v ratio %v", fast, slow, ratio)
	}
}

// Two dealers with different half-lives must disagree after a jump. That
// disagreement is the whole point: identical pricing means no trade.
func TestRealizedVolatilityHalfLifeCreatesDisagreement(t *testing.T) {
	fast := NewRealizedVolatility(0.5, 60, 1, 0, 0)
	slow := NewRealizedVolatility(0.5, 3600, 1, 0, 0)
	price := int64(50_000 * 100_000_000)
	nano := nanosPerSecond
	for i := 0; i < 120; i++ {
		nano += nanosPerSecond
		move := 0.02
		if i%2 == 1 {
			move = -0.02
		}
		price = int64(float64(price) * math.Exp(move))
		fast.Observe(price, nano)
		slow.Observe(price, nano)
	}
	fastVol, slowVol := fast.Volatility(0, 0, 0, true), slow.Volatility(0, 0, 0, true)
	if fastVol <= slowVol {
		t.Errorf("the faster estimator did not react first: fast %v slow %v", fastVol, slowVol)
	}
}

func TestRealizedVolatilityAppliesPremiumAndBounds(t *testing.T) {
	estimator := NewRealizedVolatility(0.4, 600, 2, 0, 0)
	if got := estimator.Volatility(0, 0, 0, true); math.Abs(got-0.8) > 1e-9 {
		t.Errorf("premium not applied: got %v, want 0.8", got)
	}
	estimator.Ceiling = 0.5
	if got := estimator.Volatility(0, 0, 0, true); got != 0.5 {
		t.Errorf("ceiling not applied: got %v, want 0.5", got)
	}
	estimator.Ceiling = 0
	estimator.Floor = 1.2
	if got := estimator.Volatility(0, 0, 0, true); got != 1.2 {
		t.Errorf("floor not applied: got %v, want 1.2", got)
	}
}

// An estimator that has seen nothing must decline rather than quote zero
// volatility, which would price every option at intrinsic value.
func TestRealizedVolatilityDeclinesBeforeItHasSeenAnything(t *testing.T) {
	estimator := NewRealizedVolatility(0, 600, 1, 0, 0)
	if got := estimator.Volatility(0, 0, 0, true); got != 0 {
		t.Errorf("an unseeded estimator priced at %v, want 0", got)
	}
	estimator.Observe(50_000, nanosPerSecond)
	if got := estimator.Volatility(0, 0, 0, true); got != 0 {
		t.Errorf("one observation is not a return, yet it priced at %v", got)
	}
}

func TestRealizedVolatilityReportsSignedDomainRejections(t *testing.T) {
	estimator := NewRealizedVolatility(0, 600, 1, 0, 0)
	estimator.Observe(100, 1)
	estimator.Observe(0, 2)
	estimator.Observe(-100, 3)
	estimator.Observe(110, 1) // timestamp did not advance.
	nonPositive, outOfOrder := estimator.Rejected()
	if nonPositive != 2 || outOfOrder != 1 {
		t.Fatalf("rejections = non-positive %d, out-of-order %d; want 2, 1", nonPositive, outOfOrder)
	}
	if estimator.Samples() != 0 {
		t.Fatalf("rejected signed prices became samples=%d", estimator.Samples())
	}
}

// The smile must be able to arise from what a dealer is holding rather than
// from a parameter: the strikes it is short are the strikes it marks up.
func TestInventoryVolatilityMarksUpTheStrikesADealerIsShort(t *testing.T) {
	shortStrike := int64(45_000)
	model := InventoryVolatility{
		Base:          FlatVolatility(0.8),
		VegaAversion:  0.001,
		MaxAdjustment: 0.5,
		Exposure: func(strike int64, _ float64, _ bool) VegaExposure {
			if strike == shortStrike {
				return VegaExposure{Vega: -100}
			}
			return VegaExposure{}
		},
	}
	marked := model.Volatility(50_000, shortStrike, 0.25, true)
	flat := model.Volatility(50_000, 55_000, 0.25, true)
	if marked <= flat {
		t.Errorf("the short strike was not marked up: %v against %v", marked, flat)
	}
	if math.Abs(marked-0.9) > 1e-9 {
		t.Errorf("adjustment = %v, want 0.8 plus 0.1", marked)
	}
}

func TestInventoryVolatilityBoundsTheAdjustmentAndStaysPositive(t *testing.T) {
	model := InventoryVolatility{
		Base:          FlatVolatility(0.8),
		VegaAversion:  1,
		MaxAdjustment: 0.3,
		Exposure:      func(int64, float64, bool) VegaExposure { return VegaExposure{Vega: -1000} },
	}
	if got := model.Volatility(50_000, 45_000, 0.25, true); math.Abs(got-1.1) > 1e-9 {
		t.Errorf("adjustment not bounded: got %v, want 1.1", got)
	}
	model.Exposure = func(int64, float64, bool) VegaExposure { return VegaExposure{Vega: 1000} }
	if got := model.Volatility(50_000, 45_000, 0.25, true); got <= 0 {
		t.Errorf("a long dealer priced at %v, which is not a volatility", got)
	}
}

// With no aversion configured the wrapper must be exactly its base, so that
// adding the mechanism to a population changes nothing until it is turned on.
func TestInventoryVolatilityWithoutAversionIsItsBase(t *testing.T) {
	model := InventoryVolatility{
		Base:     FlatVolatility(0.8),
		Exposure: func(int64, float64, bool) VegaExposure { return VegaExposure{Vega: -500} },
	}
	if got := model.Volatility(50_000, 45_000, 0.25, true); got != 0.8 {
		t.Errorf("volatility = %v, want the base 0.8", got)
	}
}
