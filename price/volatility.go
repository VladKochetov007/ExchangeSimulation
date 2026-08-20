package price

import "math"

// VolatilityModel supplies the volatility a participant prices an option with.
//
// It is an interface because the volatility a dealer uses is the main thing
// dealers can disagree about, and disagreement is what makes an option book
// trade. A population where every dealer prices from one number quotes one
// price, and the only trades are against participants who do not look at
// price at all.
//
// The strike is passed alongside the forward so a model may vary with
// moneyness. Nothing here builds a smile in: a smile that is written down as a
// parameter is an assumption, not a result. The models below are estimators
// and inventory responses, and any smile is whatever those produce.
type VolatilityModel interface {
	// Volatility returns an annualised volatility for one contract. Zero or
	// negative means the model declines to price it.
	Volatility(forward, strike int64, yearsToExpiry float64, isCall bool) float64
}

// FlatVolatility prices every contract at one number, which is the baseline
// this package's option pricing started from.
type FlatVolatility float64

// Volatility implements VolatilityModel.
func (v FlatVolatility) Volatility(int64, int64, float64, bool) float64 { return float64(v) }

// RealizedVolatility is an exponentially weighted estimate of the underlying's
// own volatility, updated from observed prices.
//
// Two dealers running this with different half-lives disagree after any
// sudden move: the faster one raises its volatility first and quotes wider
// while the slower one is still pricing the calm. That disagreement is
// endogenous, unlike a configured spread between them.
type RealizedVolatility struct {
	// HalfLifeSeconds sets how quickly the estimate forgets. It must be
	// positive for the estimator to update.
	HalfLifeSeconds float64
	// Premium multiplies the estimate before it is quoted. Dealers charge
	// more than realised volatility for carrying gamma, and how much more is
	// a preference rather than a measurement.
	Premium float64
	// Floor and Ceiling bound the estimate. An unbounded estimator quotes a
	// zero premium during a quiet stretch and an unbounded one after a jump.
	Floor   float64
	Ceiling float64

	variance   float64
	lastPrice  int64
	lastNano   int64
	samples    int
	initialSet bool
}

// NewRealizedVolatility seeds an estimator at an initial volatility so it can
// price before it has seen enough of the market to estimate anything.
func NewRealizedVolatility(initial, halfLifeSeconds, premium, floor, ceiling float64) *RealizedVolatility {
	estimator := &RealizedVolatility{
		HalfLifeSeconds: halfLifeSeconds,
		Premium:         premium,
		Floor:           floor,
		Ceiling:         ceiling,
	}
	if initial > 0 {
		estimator.variance = initial * initial
		estimator.initialSet = true
	}
	return estimator
}

const secondsPerYear = 365 * 24 * 60 * 60

// Observe folds one underlying price into the estimate. Out-of-order or
// non-positive observations are ignored, so a caller may feed it every mid it
// sees without filtering first.
func (v *RealizedVolatility) Observe(price, nano int64) {
	if price <= 0 || nano <= 0 {
		return
	}
	if v.lastPrice <= 0 || nano <= v.lastNano {
		v.lastPrice, v.lastNano = price, nano
		return
	}
	elapsedSeconds := float64(nano-v.lastNano) / 1e9
	logReturn := math.Log(float64(price) / float64(v.lastPrice))
	v.lastPrice, v.lastNano = price, nano
	if elapsedSeconds <= 0 || v.HalfLifeSeconds <= 0 {
		return
	}
	// The observation is an annualised variance, so that samples taken at
	// uneven spacing are comparable: a return over two seconds carries twice
	// the variance of one over a second.
	annualised := logReturn * logReturn / elapsedSeconds * secondsPerYear
	weight := 1 - math.Exp(-math.Ln2*elapsedSeconds/v.HalfLifeSeconds)
	if !v.initialSet {
		v.variance = annualised
		v.initialSet = true
	} else {
		v.variance += weight * (annualised - v.variance)
	}
	v.samples++
}

// Samples reports how many returns the estimate has absorbed.
func (v *RealizedVolatility) Samples() int { return v.samples }

// Volatility implements VolatilityModel.
func (v *RealizedVolatility) Volatility(int64, int64, float64, bool) float64 {
	if !v.initialSet || v.variance <= 0 {
		return 0
	}
	volatility := math.Sqrt(v.variance)
	if v.Premium > 0 {
		volatility *= v.Premium
	}
	if v.Floor > 0 && volatility < v.Floor {
		volatility = v.Floor
	}
	if v.Ceiling > 0 && volatility > v.Ceiling {
		volatility = v.Ceiling
	}
	return volatility
}

// VegaExposure reports a dealer's option risk against one contract, in the
// units Black-76 produces: vega per one-unit volatility move, and the two
// second-order terms a vanna-volga dealer prices with.
type VegaExposure struct {
	Vega  float64
	Vanna float64
	Volga float64
}

// InventoryVolatility raises the volatility a dealer prices with as it becomes
// short volatility and lowers it as it becomes long, per contract.
//
// This is where a smile can come from without one being written down. A dealer
// accumulates its inventory unevenly across strikes, because that is where the
// flow went, and it marks up exactly the strikes it is short. Whether the
// result looks like a market smile is a measurement, not a setting.
type InventoryVolatility struct {
	// Base supplies the level the adjustment is applied to.
	Base VolatilityModel
	// VegaAversion is the volatility added per unit of short vega, expressed
	// in volatility points per unit of vega. Zero reproduces Base exactly.
	VegaAversion float64
	// MaxAdjustment bounds the shift in volatility points, so that one large
	// position cannot drive a quote to an absurd level.
	MaxAdjustment float64
	// Exposure reports the dealer's current risk against a contract. It is a
	// callback because the inventory lives in the actor, not in the pricer.
	Exposure func(strike int64, yearsToExpiry float64, isCall bool) VegaExposure
}

// Volatility implements VolatilityModel.
func (v InventoryVolatility) Volatility(forward, strike int64, yearsToExpiry float64, isCall bool) float64 {
	if v.Base == nil {
		return 0
	}
	base := v.Base.Volatility(forward, strike, yearsToExpiry, isCall)
	if base <= 0 || v.VegaAversion == 0 || v.Exposure == nil {
		return base
	}
	exposure := v.Exposure(strike, yearsToExpiry, isCall)
	// A short vega position is negative, and a dealer who is short volatility
	// wants to buy it back higher, so the adjustment carries the opposite sign.
	adjustment := -v.VegaAversion * exposure.Vega
	if v.MaxAdjustment > 0 {
		adjustment = math.Max(-v.MaxAdjustment, math.Min(v.MaxAdjustment, adjustment))
	}
	adjusted := base + adjustment
	if adjusted <= 0 {
		return base
	}
	return adjusted
}
