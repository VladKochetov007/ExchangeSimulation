package analysis

import (
	"math"
	"sort"
)

// ImpactCurve is the price response to trade size, measured in size buckets.
//
// The square-root law reported across equities, futures and crypto says the
// response grows as roughly the square root of size, so the exponent of a
// log-log fit is near 0.5. An exponent near one means impact is proportional to
// size, which is what a book of fixed displayed depth produces mechanically: a
// trade eats a known quantity and the price moves in step. Distinguishing the
// two matters because proportional impact makes a large return and a moved
// level the same event.
type ImpactCurve struct {
	// Buckets are ordered by size, each carrying the geometric mean size and
	// the mean signed response.
	Buckets  []ImpactBucket
	Exponent float64
	// R2 of the log-log fit. A low value means the exponent is not describing
	// the data and should not be quoted.
	R2 float64
	N  int
}

// ImpactBucket is one size group's average response.
type ImpactBucket struct {
	MeanSize     float64
	MeanResponse float64
	Count        int
}

// ImpactOptions configures the measurement.
type ImpactOptions struct {
	// HorizonTrades is how many trades ahead the response is measured over.
	HorizonTrades int
	// Buckets is how many size groups to form. Zero uses ten.
	Buckets int
	// Role, when set, keeps only trades whose aggressor belongs to that class.
	// Pooling every participant measures the mix rather than the response.
	Role string
}

// Impact measures the signed price response against trade size.
//
// The response is signed by the aggressor's direction, so a buy that lifts the
// price and a sell that lowers it both count as positive impact. Sizes are
// bucketed by quantile rather than by value, because a heavy-tailed size
// distribution leaves value-spaced buckets almost empty at the top.
func (t *TradeTape) Impact(opts ImpactOptions) ImpactCurve {
	horizon := opts.HorizonTrades
	if horizon < 1 {
		horizon = 10
	}
	buckets := opts.Buckets
	if buckets < 2 {
		buckets = 10
	}
	type observation struct{ size, response float64 }
	observations := make([]observation, 0, len(t.Prices))
	// The reference is the price before the trade, not the trade's own price.
	// A buy executes at the ask while later prices average at the mid, so
	// measuring from the trade price subtracts a half spread from every buy and
	// adds one to every sell. Signed, that is a constant negative offset which
	// does not decay with horizon and swamps the size dependence entirely:
	// measured that way every size bucket read about -0.38 bps, the half spread,
	// from the smallest trade to the largest and out to a thousand trades ahead.
	for i := 1; i+horizon < len(t.Prices); i++ {
		if t.Qtys[i] <= 0 {
			continue
		}
		if opts.Role != "" && (i >= len(t.Roles) || t.Roles[i] != opts.Role) {
			continue
		}
		reference := int64(0)
		if i < len(t.PreMid) {
			reference = t.PreMid[i]
		}
		if reference <= 0 {
			reference = t.Prices[i-1]
		}
		if reference <= 0 {
			continue
		}
		terminal := t.terminalMid(i + horizon)
		if terminal <= 0 {
			continue
		}
		response := 1e4 * math.Log(float64(terminal)/float64(reference))
		observations = append(observations, observation{
			size:     float64(t.Qtys[i]),
			response: float64(t.Signs[i]) * response,
		})
	}
	curve := ImpactCurve{N: len(observations)}
	if len(observations) < buckets*10 {
		return curve
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].size < observations[j].size })

	per := len(observations) / buckets
	var logSizes, logResponses []float64
	for b := 0; b < buckets; b++ {
		lo, hi := b*per, (b+1)*per
		if b == buckets-1 {
			hi = len(observations)
		}
		var sumLogSize, sumResponse float64
		for _, obs := range observations[lo:hi] {
			sumLogSize += math.Log(obs.size)
			sumResponse += obs.response
		}
		count := hi - lo
		bucket := ImpactBucket{
			MeanSize:     math.Exp(sumLogSize / float64(count)),
			MeanResponse: sumResponse / float64(count),
			Count:        count,
		}
		curve.Buckets = append(curve.Buckets, bucket)
		// Only positive responses can enter a log-log fit; a bucket whose mean
		// response is negative carries no exponent information.
		if bucket.MeanResponse > 0 && bucket.MeanSize > 0 {
			logSizes = append(logSizes, math.Log(bucket.MeanSize))
			logResponses = append(logResponses, math.Log(bucket.MeanResponse))
		}
	}
	if len(logSizes) < 3 {
		return curve
	}
	curve.Exponent, curve.R2 = fitLine(logSizes, logResponses)
	return curve
}

// terminalMid is the book midpoint at the end of the measurement horizon.
//
// The terminal must be a mid, not a trade price. A trade at the horizon
// executes at whichever side its own aggressor crossed, so measuring to it
// adds a half spread signed by that trade's direction. Signed flow is
// autocorrelated, so the expectation of that term is the sign correlation at
// the horizon times the half spread — a bias that does not decay with horizon
// and, at the spreads in this campaign, is the same size as the effect being
// measured. It also scales with the spread, which several experiments varied.
func (t *TradeTape) terminalMid(index int) int64 {
	if index < 0 || index >= len(t.Prices) {
		return 0
	}
	if index < len(t.PreMid) && t.PreMid[index] > 0 {
		return t.PreMid[index]
	}
	return 0
}

func fitLine(x, y []float64) (slope, r2 float64) {
	n := float64(len(x))
	var sx, sy, sxx, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	denominator := n*sxx - sx*sx
	if denominator == 0 {
		return math.NaN(), 0
	}
	slope = (n*sxy - sx*sy) / denominator
	intercept := (sy - slope*sx) / n
	meanY := sy / n
	var ssTotal, ssResidual float64
	for i := range x {
		ssTotal += (y[i] - meanY) * (y[i] - meanY)
		residual := y[i] - (intercept + slope*x[i])
		ssResidual += residual * residual
	}
	if ssTotal == 0 {
		return slope, 0
	}
	return slope, 1 - ssResidual/ssTotal
}
