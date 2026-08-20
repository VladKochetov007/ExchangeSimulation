package price

import "math"

// SABRVolatility prices each strike from the SABR stochastic-volatility model
// using Hagan's lognormal approximation.
//
// A word of warning about what this does and does not show. A dealer running
// this quotes a smile because the model has one, not because the market
// produced one, so any smile measured in a population where dealers use it is
// an assumption travelling through the book rather than a finding. It is here
// for the opposite purpose: as a pricer that disagrees with a flat-volatility
// dealer in a structured way, so that the two have something to trade, and as
// the harder model a value taker can hold a view with.
//
// Parameters follow the usual reading: Alpha is the overall level, Beta the
// backbone between lognormal and normal dynamics, Rho the correlation between
// the forward and its volatility, which tilts the smile, and Nu the volatility
// of volatility, which sets its curvature.
type SABRVolatility struct {
	Alpha float64
	Beta  float64
	Rho   float64
	Nu    float64
}

// Volatility implements VolatilityModel.
func (s SABRVolatility) Volatility(forward, strike int64, yearsToExpiry float64, _ bool) float64 {
	if forward <= 0 || strike <= 0 || yearsToExpiry <= 0 || s.Alpha <= 0 {
		return 0
	}
	if s.Rho <= -1 || s.Rho >= 1 || s.Beta < 0 || s.Beta > 1 || s.Nu < 0 {
		return 0
	}
	f, k := float64(forward), float64(strike)
	oneMinusBeta := 1 - s.Beta
	fkPow := math.Pow(f*k, oneMinusBeta/2)
	logFK := math.Log(f / k)

	// The correction factor is common to both branches: it is the expansion in
	// time to expiry, and it is what makes the model's smile move with tenor.
	correction := 1 + yearsToExpiry*(
	// term in beta only
	oneMinusBeta*oneMinusBeta*s.Alpha*s.Alpha/(24*fkPow*fkPow)+
		// cross term in rho
		s.Rho*s.Beta*s.Nu*s.Alpha/(4*fkPow)+
		// term in the volatility of volatility
		(2-3*s.Rho*s.Rho)*s.Nu*s.Nu/24)

	var volatility float64
	if math.Abs(logFK) < 1e-9 {
		// At the money the ratio below is zero over zero, so the limit is used.
		volatility = s.Alpha / math.Pow(f, oneMinusBeta) * correction
	} else {
		z := s.Nu / s.Alpha * fkPow * logFK
		// The ratio z over its logarithm tends to one as the volatility of
		// volatility goes to zero, where both are zero. Without the limit a
		// model with no vol-of-vol declines to price every strike away from
		// the money, which is the case a caller is most likely to configure
		// first.
		ratio := 1.0
		if math.Abs(z) > 1e-12 {
			denominator := math.Log((math.Sqrt(1-2*s.Rho*z+z*z) + z - s.Rho) / (1 - s.Rho))
			if denominator == 0 || !finite(denominator) {
				return 0
			}
			ratio = z / denominator
		}
		expansion := 1 +
			oneMinusBeta*oneMinusBeta*logFK*logFK/24 +
			math.Pow(oneMinusBeta*logFK, 4)/1920
		volatility = s.Alpha / (fkPow * expansion) * ratio * correction
	}
	if !finite(volatility) || volatility <= 0 {
		return 0
	}
	return volatility
}
