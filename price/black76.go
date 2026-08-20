package price

import "math"

// Black76Sensitivity contains local Black-76 sensitivities in the model's
// native units: delta is base units per base unit, gamma is per forward-price
// unit, vega is quote units per 1.00 annualized-volatility move, and theta is
// quote units per calendar year. These values are analytics only and must not
// be used for fixed-point ledger mutation.
type Black76Sensitivity struct {
	Delta float64
	Gamma float64
	Vega  float64
	Theta float64
}

// Black76Premium prices a European option on a forward/futures price with
// zero rate, returning the premium per base unit in quote precision units.
// forward and strike are quote-precision ints; vol is annualized (e.g. 0.8);
// timeToExpiry is in years. Degenerate inputs collapse to intrinsic value.
func Black76Premium(forward, strike int64, vol, timeToExpiry float64, isCall bool) int64 {
	if forward <= 0 || strike <= 0 {
		return 0
	}
	f, k := float64(forward), float64(strike)
	if !finite(vol) || !finite(timeToExpiry) {
		return 0
	}
	if vol <= 0 || timeToExpiry <= 0 {
		return intrinsic(f, k, isCall)
	}
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(f/k) + 0.5*vol*vol*timeToExpiry) / (vol * sqrtT)
	d2 := d1 - vol*sqrtT
	var premium float64
	if isCall {
		premium = f*normCDF(d1) - k*normCDF(d2)
	} else {
		premium = k*normCDF(-d2) - f*normCDF(-d1)
	}
	if premium < 0 {
		premium = 0
	}
	return int64(premium)
}

// Black76Delta returns the option delta in [−1, 1] under the same model.
// Degenerate inputs collapse to the intrinsic hedge (0 or ±1).
func Black76Delta(forward, strike int64, vol, timeToExpiry float64, isCall bool) float64 {
	if forward <= 0 || strike <= 0 {
		return 0
	}
	if !finite(vol) || !finite(timeToExpiry) {
		return 0
	}
	f, k := float64(forward), float64(strike)
	if vol <= 0 || timeToExpiry <= 0 {
		if isCall {
			if f > k {
				return 1
			}
			return 0
		}
		if f < k {
			return -1
		}
		return 0
	}
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(f/k) + 0.5*vol*vol*timeToExpiry) / (vol * sqrtT)
	if isCall {
		return normCDF(d1)
	}
	return normCDF(d1) - 1
}

// Black76Sensitivities returns the analytic Black-76 sensitivities for a
// non-degenerate contract. The boolean is false for invalid, expired, or
// zero-volatility inputs, where gamma, vega, and theta are not meaningful.
func Black76Sensitivities(forward, strike int64, vol, timeToExpiry float64, isCall bool) (Black76Sensitivity, bool) {
	if forward <= 0 || strike <= 0 || vol <= 0 || timeToExpiry <= 0 || !finite(vol) || !finite(timeToExpiry) {
		return Black76Sensitivity{}, false
	}
	f, k := float64(forward), float64(strike)
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(f/k) + 0.5*vol*vol*timeToExpiry) / (vol * sqrtT)
	phi := normPDF(d1)
	delta := normCDF(d1)
	if !isCall {
		delta--
	}
	result := Black76Sensitivity{
		Delta: delta,
		Gamma: phi / (f * vol * sqrtT),
		Vega:  f * phi * sqrtT,
		Theta: -f * phi * vol / (2 * sqrtT),
	}
	if !finite(result.Delta) || !finite(result.Gamma) || !finite(result.Vega) || !finite(result.Theta) {
		return Black76Sensitivity{}, false
	}
	return result, true
}

// Black76Gamma returns d² premium / d forward². Gamma is the same for calls
// and puts. It is zero for expired, zero-volatility, or invalid contracts.
func Black76Gamma(forward, strike int64, vol, timeToExpiry float64) float64 {
	s, ok := Black76Sensitivities(forward, strike, vol, timeToExpiry, true)
	if !ok {
		return 0
	}
	return s.Gamma
}

// Black76Vega returns d premium / d annualized-volatility unit. For example,
// multiplying it by 0.01 approximates the premium change from a one-vol-point
// move. It is identical for calls and puts under Black-76.
func Black76Vega(forward, strike int64, vol, timeToExpiry float64) float64 {
	s, ok := Black76Sensitivities(forward, strike, vol, timeToExpiry, true)
	if !ok {
		return 0
	}
	return s.Vega
}

func intrinsic(f, k float64, isCall bool) int64 {
	var v float64
	if isCall {
		v = f - k
	} else {
		v = k - f
	}
	if v < 0 {
		return 0
	}
	return int64(v)
}

func normCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt2))
}

func normPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}

func finite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// Black76Vanna is d vega / d forward, equivalently d delta / d volatility.
// It is what a dealer's delta hedge misses when volatility moves with the
// underlying, which is the whole reason a vanna-volga desk hedges in options
// rather than only in the underlying.
func Black76Vanna(forward, strike int64, vol, timeToExpiry float64) float64 {
	if forward <= 0 || strike <= 0 || vol <= 0 || timeToExpiry <= 0 || !finite(vol) || !finite(timeToExpiry) {
		return 0
	}
	f, k := float64(forward), float64(strike)
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(f/k) + 0.5*vol*vol*timeToExpiry) / (vol * sqrtT)
	d2 := d1 - vol*sqrtT
	vanna := -normPDF(d1) * d2 / vol
	if !finite(vanna) {
		return 0
	}
	return vanna
}

// Black76Volga is d vega / d volatility: the convexity of the premium in
// volatility. It is zero at the money and largest in the wings, which is why a
// desk that is short wings is short volatility of volatility however flat its
// vega looks.
func Black76Volga(forward, strike int64, vol, timeToExpiry float64) float64 {
	if forward <= 0 || strike <= 0 || vol <= 0 || timeToExpiry <= 0 || !finite(vol) || !finite(timeToExpiry) {
		return 0
	}
	f, k := float64(forward), float64(strike)
	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(f/k) + 0.5*vol*vol*timeToExpiry) / (vol * sqrtT)
	d2 := d1 - vol*sqrtT
	volga := f * normPDF(d1) * sqrtT * d1 * d2 / vol
	if !finite(volga) {
		return 0
	}
	return volga
}
