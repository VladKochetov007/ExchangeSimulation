package price

import "math"

// Black76Premium prices a European option on a forward/futures price with
// zero rate, returning the premium per base unit in quote precision units.
// forward and strike are quote-precision ints; vol is annualized (e.g. 0.8);
// timeToExpiry is in years. Degenerate inputs collapse to intrinsic value.
func Black76Premium(forward, strike int64, vol, timeToExpiry float64, isCall bool) int64 {
	if forward <= 0 || strike <= 0 {
		return 0
	}
	f, k := float64(forward), float64(strike)
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
