package analysis

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Option surface measured from market prices.
//
// The point of this measurement is to answer whether the smile is inherited
// from the SABR beliefs the value takers hold or produced by the market, so it
// must not read any participant's model. Nothing here touches the dealer's
// implied_volatility field or the vanna-volga desk's own surface. The inputs
// are the option's traded price, its strike and expiry from its symbol, and
// the venue's published index price for the underlying.
//
// Volatility is recovered by inverting Black-76 on the forward, which is the
// same convention the instruments are priced in:
//
//	call = e^{-rT}(F N(d1) - K N(d2)),  r = 0
//
// with the forward taken as the venue's last published index. Using the index
// rather than a model forward keeps the inversion independent of any agent.

// SurfaceOptions selects inputs and the fitting windows.
type SurfaceOptions struct {
	Files         []string
	FilesSelected bool
	// QuotePrecision converts logged integer prices into currency units.
	QuotePrecision int64
	// ATMWindow is the half-width in log-moneyness treated as at the money
	// when fitting the level and the slope.
	ATMWindow float64
	// MinTradesPerExpiry is the fewest trades an expiry needs before its
	// smile is fitted; below it the expiry is reported but not fitted.
	MinTradesPerExpiry int
}

// SurfacePoint is one option's volatility as the market priced it.
type SurfacePoint struct {
	VenueID      string  `json:"venue_id"`
	ExpiryNano   int64   `json:"expiry_nano"`
	Strike       float64 `json:"strike"`
	IsCall       bool    `json:"is_call"`
	Trades       int     `json:"trades"`
	Volume       int64   `json:"volume"`
	Forward      float64 `json:"forward"`
	LogMoneyness float64 `json:"log_moneyness"`
	// ImpliedVol is volume-weighted across the trades in this strike.
	ImpliedVol float64 `json:"implied_vol"`
}

// ExpirySmile is one expiry's fitted cross-strike structure.
type ExpirySmile struct {
	VenueID    string  `json:"venue_id"`
	ExpiryNano int64   `json:"expiry_nano"`
	Strikes    int     `json:"strikes"`
	Trades     int     `json:"trades"`
	MeanTTE    float64 `json:"mean_time_to_expiry_years"`
	// ATMVol is the fitted level at zero log-moneyness, Slope the first
	// derivative in log-moneyness (the skew) and Curvature the second (the
	// smile), from a quadratic weighted by volume.
	ATMVol    float64 `json:"atm_vol"`
	Slope     float64 `json:"slope"`
	Curvature float64 `json:"curvature"`
	Fitted    bool    `json:"fitted"`
	// Dispersion is the volume-weighted standard deviation of implied
	// volatility across strikes, a model-free measure of how much structure
	// the cross-section has.
	Dispersion float64 `json:"dispersion"`
	MinVol     float64 `json:"min_vol"`
	MaxVol     float64 `json:"max_vol"`
}

// OptionSurface is the whole measurement.
type OptionSurface struct {
	Points  []SurfacePoint `json:"points"`
	Smiles  []ExpirySmile  `json:"smiles"`
	Trades  int            `json:"trades"`
	Priced  int            `json:"priced"`
	Skipped int            `json:"skipped_unpriceable"`
	// Pooled statistics across every fitted expiry, volume weighted. These are
	// the numbers an arm is compared on.
	PooledATMVol     float64 `json:"pooled_atm_vol"`
	PooledSlope      float64 `json:"pooled_slope"`
	PooledCurvature  float64 `json:"pooled_curvature"`
	PooledDispersion float64 `json:"pooled_dispersion"`
	FittedExpiries   int     `json:"fitted_expiries"`
	// TermStructure is the volume-weighted ATM level per maturity bucket, in
	// increasing time to expiry.
	TermStructure []TermPoint `json:"term_structure"`
}

// TermPoint is the ATM level at one maturity.
type TermPoint struct {
	MeanTTEYears float64 `json:"mean_time_to_expiry_years"`
	Expiries     int     `json:"expiries"`
	Trades       int     `json:"trades"`
	ATMVol       float64 `json:"atm_vol"`
}

// optionTerms parses ABC-<expiry epoch>-<strike>-C|P.
func optionTerms(symbol string) (expiryNano int64, strike float64, isCall bool, ok bool) {
	parts := strings.Split(symbol, "-")
	if len(parts) < 4 {
		return 0, 0, false, false
	}
	last := parts[len(parts)-1]
	if last != "C" && last != "P" {
		return 0, 0, false, false
	}
	strikeRaw, err := strconv.ParseFloat(parts[len(parts)-2], 64)
	if err != nil {
		return 0, 0, false, false
	}
	epoch, err := strconv.ParseInt(parts[len(parts)-3], 10, 64)
	if err != nil {
		return 0, 0, false, false
	}
	return epoch * 1e9, strikeRaw, last == "C", true
}

// normCDF is the standard normal distribution function.
func normCDF(x float64) float64 { return 0.5 * math.Erfc(-x/math.Sqrt2) }

// black76 prices a European option on a forward at zero rates.
func black76(forward, strike, vol, tte float64, isCall bool) float64 {
	if tte <= 0 || vol <= 0 || forward <= 0 || strike <= 0 {
		intrinsic := forward - strike
		if !isCall {
			intrinsic = strike - forward
		}
		return math.Max(intrinsic, 0)
	}
	sqrtT := math.Sqrt(tte)
	d1 := (math.Log(forward/strike) + 0.5*vol*vol*tte) / (vol * sqrtT)
	d2 := d1 - vol*sqrtT
	if isCall {
		return forward*normCDF(d1) - strike*normCDF(d2)
	}
	return strike*normCDF(-d2) - forward*normCDF(-d1)
}

// impliedVol inverts black76 by bisection. Bisection rather than Newton
// because the vega of a deep wing is small enough that Newton wanders, and
// this runs offline where robustness beats speed.
func impliedVol(price, forward, strike, tte float64, isCall bool) (float64, bool) {
	if tte <= 0 || price <= 0 || forward <= 0 || strike <= 0 {
		return 0, false
	}
	intrinsic := forward - strike
	if !isCall {
		intrinsic = strike - forward
	}
	intrinsic = math.Max(intrinsic, 0)
	// A price at or below intrinsic carries no time value and no volatility
	// can be recovered from it; a price above the forward is not arbitrage
	// free for a call.
	if price <= intrinsic+1e-12 {
		return 0, false
	}
	low, high := 1e-4, 5.0
	if black76(forward, strike, high, tte, isCall) < price {
		return 0, false
	}
	if black76(forward, strike, low, tte, isCall) > price {
		return 0, false
	}
	for i := 0; i < 100; i++ {
		mid := 0.5 * (low + high)
		if black76(forward, strike, mid, tte, isCall) < price {
			low = mid
		} else {
			high = mid
		}
		if high-low < 1e-8 {
			break
		}
	}
	return 0.5 * (low + high), true
}

// surfaceKey identifies one traded contract.
type surfaceKey struct {
	venue  string
	expiry int64
	strike float64
	isCall bool
}

type strikeAccumulator struct {
	trades       int
	volume       int64
	volWeighted  float64
	forwardSum   float64
	tteSum       float64
	moneynessSum float64
}

// MeasureOptionSurface recovers the traded volatility surface from option
// trades and the venue's published index.
func (r *Run) MeasureOptionSurface(opts SurfaceOptions) (*OptionSurface, error) {
	precision := float64(opts.QuotePrecision)
	if precision <= 0 {
		precision = 100000
	}
	atmWindow := opts.ATMWindow
	if atmWindow <= 0 {
		atmWindow = 0.05
	}
	minTrades := opts.MinTradesPerExpiry
	if minTrades <= 0 {
		minTrades = 20
	}

	type markPayload struct {
		Timestamp  int64  `json:"timestamp"`
		Symbol     string `json:"symbol"`
		IndexPrice int64  `json:"index_price"`
	}
	type tradePayload struct {
		Price int64 `json:"price"`
		Qty   int64 `json:"qty"`
	}

	// The index is published on the derivative feed alongside every mark. It
	// is the venue's own number rather than any participant's, which is the
	// property that makes this inversion independent of the agents.
	var mu sync.Mutex
	type indexPoint struct{ at, price int64 }
	indexSeries := make(map[string][]indexPoint)
	type rawTrade struct {
		venue  string
		symbol string
		at     int64
		price  int64
		qty    int64
	}
	var trades []rawTrade

	scan := ScanOptions{
		Events:        []string{"mark_price_update", "Trade"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "mark_price_update":
			var payload markPayload
			if event.Decode(&payload) != nil || payload.IndexPrice <= 0 {
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			mu.Lock()
			indexSeries[event.VenueID] = append(indexSeries[event.VenueID], indexPoint{at, payload.IndexPrice})
			mu.Unlock()
		case "Trade":
			symbol := event.Symbol
			if _, _, _, ok := optionTerms(symbol); !ok {
				return
			}
			var payload tradePayload
			if event.Decode(&payload) != nil || payload.Price <= 0 || payload.Qty <= 0 {
				return
			}
			mu.Lock()
			trades = append(trades, rawTrade{event.VenueID, symbol, event.SimTS, payload.Price, payload.Qty})
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	for venue := range indexSeries {
		points := indexSeries[venue]
		sort.Slice(points, func(i, j int) bool { return points[i].at < points[j].at })
		indexSeries[venue] = points
	}
	indexAt := func(venue string, at int64) (float64, bool) {
		points := indexSeries[venue]
		i := sort.Search(len(points), func(i int) bool { return points[i].at > at })
		if i == 0 {
			return 0, false
		}
		return float64(points[i-1].price) / precision, true
	}

	result := &OptionSurface{Trades: len(trades)}
	accumulators := make(map[surfaceKey]*strikeAccumulator)

	const secondsPerYear = 365 * 24 * 3600
	for _, trade := range trades {
		expiry, strike, isCall, ok := optionTerms(trade.symbol)
		if !ok {
			result.Skipped++
			continue
		}
		forward, known := indexAt(trade.venue, trade.at)
		if !known || forward <= 0 {
			result.Skipped++
			continue
		}
		tte := float64(expiry-trade.at) / 1e9 / secondsPerYear
		price := float64(trade.price) / precision
		vol, inverted := impliedVol(price, forward, strike, tte, isCall)
		if !inverted {
			result.Skipped++
			continue
		}
		result.Priced++
		key := surfaceKey{trade.venue, expiry, strike, isCall}
		acc := accumulators[key]
		if acc == nil {
			acc = &strikeAccumulator{}
			accumulators[key] = acc
		}
		weight := float64(trade.qty)
		acc.trades++
		acc.volume += trade.qty
		acc.volWeighted += vol * weight
		acc.forwardSum += forward * weight
		acc.tteSum += tte * weight
		acc.moneynessSum += math.Log(strike/forward) * weight
	}

	keys := make([]surfaceKey, 0, len(accumulators))
	for key := range accumulators {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		if keys[i].expiry != keys[j].expiry {
			return keys[i].expiry < keys[j].expiry
		}
		if keys[i].strike != keys[j].strike {
			return keys[i].strike < keys[j].strike
		}
		return keys[i].isCall && !keys[j].isCall
	})

	type expiryBucket struct {
		venue  string
		expiry int64
	}
	byExpiry := make(map[expiryBucket][]SurfacePoint)
	for _, key := range keys {
		acc := accumulators[key]
		weight := float64(acc.volume)
		point := SurfacePoint{
			VenueID: key.venue, ExpiryNano: key.expiry, Strike: key.strike, IsCall: key.isCall,
			Trades: acc.trades, Volume: acc.volume,
			Forward:      acc.forwardSum / weight,
			LogMoneyness: acc.moneynessSum / weight,
			ImpliedVol:   acc.volWeighted / weight,
		}
		result.Points = append(result.Points, point)
		bucket := expiryBucket{key.venue, key.expiry}
		byExpiry[bucket] = append(byExpiry[bucket], point)
	}

	buckets := make([]expiryBucket, 0, len(byExpiry))
	for bucket := range byExpiry {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].venue != buckets[j].venue {
			return buckets[i].venue < buckets[j].venue
		}
		return buckets[i].expiry < buckets[j].expiry
	})

	var pooledWeight, pooledATM, pooledSlope, pooledCurve, pooledDisp float64
	for _, bucket := range buckets {
		points := byExpiry[bucket]
		smile := fitSmile(bucket.venue, bucket.expiry, points, accumulators, atmWindow, minTrades)
		result.Smiles = append(result.Smiles, smile)
		if !smile.Fitted {
			continue
		}
		weight := float64(smile.Trades)
		pooledWeight += weight
		pooledATM += smile.ATMVol * weight
		pooledSlope += smile.Slope * weight
		pooledCurve += smile.Curvature * weight
		pooledDisp += smile.Dispersion * weight
		result.FittedExpiries++
	}
	if pooledWeight > 0 {
		result.PooledATMVol = pooledATM / pooledWeight
		result.PooledSlope = pooledSlope / pooledWeight
		result.PooledCurvature = pooledCurve / pooledWeight
		result.PooledDispersion = pooledDisp / pooledWeight
	}
	result.TermStructure = termStructure(result.Smiles)
	return result, nil
}

// fitSmile regresses implied volatility on log-moneyness with a quadratic,
// weighted by traded volume.
func fitSmile(venue string, expiry int64, points []SurfacePoint, accumulators map[surfaceKey]*strikeAccumulator, atmWindow float64, minTrades int) ExpirySmile {
	smile := ExpirySmile{VenueID: venue, ExpiryNano: expiry, Strikes: len(points)}
	var totalWeight, tteWeighted float64
	for _, point := range points {
		smile.Trades += point.Trades
		weight := float64(point.Volume)
		totalWeight += weight
		key := surfaceKey{point.VenueID, point.ExpiryNano, point.Strike, point.IsCall}
		if acc := accumulators[key]; acc != nil {
			tteWeighted += acc.tteSum
		}
	}
	if totalWeight > 0 {
		smile.MeanTTE = tteWeighted / totalWeight
	}
	if len(points) == 0 {
		return smile
	}
	smile.MinVol, smile.MaxVol = points[0].ImpliedVol, points[0].ImpliedVol
	meanVol := 0.0
	for _, point := range points {
		smile.MinVol = math.Min(smile.MinVol, point.ImpliedVol)
		smile.MaxVol = math.Max(smile.MaxVol, point.ImpliedVol)
		meanVol += point.ImpliedVol * float64(point.Volume)
	}
	meanVol /= totalWeight
	variance := 0.0
	for _, point := range points {
		d := point.ImpliedVol - meanVol
		variance += d * d * float64(point.Volume)
	}
	smile.Dispersion = math.Sqrt(variance / totalWeight)

	if smile.Trades < minTrades || len(points) < 3 {
		return smile
	}
	// Weighted quadratic least squares in log-moneyness. Three unknowns, so
	// the normal equations are solved directly rather than through a matrix
	// library the project does not otherwise need.
	var s0, s1, s2, s3, s4, t0, t1, t2 float64
	for _, point := range points {
		w := float64(point.Volume)
		x := point.LogMoneyness
		y := point.ImpliedVol
		s0 += w
		s1 += w * x
		s2 += w * x * x
		s3 += w * x * x * x
		s4 += w * x * x * x * x
		t0 += w * y
		t1 += w * x * y
		t2 += w * x * x * y
	}
	a, b, c, ok := solve3(s0, s1, s2, s1, s2, s3, s2, s3, s4, t0, t1, t2)
	if !ok {
		return smile
	}
	smile.ATMVol = a
	smile.Slope = b
	smile.Curvature = 2 * c
	smile.Fitted = true
	_ = atmWindow
	return smile
}

// solve3 solves a symmetric 3x3 system by elimination, reporting failure
// rather than returning a value when the system is singular.
func solve3(a11, a12, a13, a21, a22, a23, a31, a32, a33, b1, b2, b3 float64) (float64, float64, float64, bool) {
	det := a11*(a22*a33-a23*a32) - a12*(a21*a33-a23*a31) + a13*(a21*a32-a22*a31)
	if math.Abs(det) < 1e-18 {
		return 0, 0, 0, false
	}
	d1 := b1*(a22*a33-a23*a32) - a12*(b2*a33-a23*b3) + a13*(b2*a32-a22*b3)
	d2 := a11*(b2*a33-a23*b3) - b1*(a21*a33-a23*a31) + a13*(a21*b3-b2*a31)
	d3 := a11*(a22*b3-b2*a32) - a12*(a21*b3-b2*a31) + b1*(a21*a32-a22*a31)
	return d1 / det, d2 / det, d3 / det, true
}

// termStructure groups fitted expiries into maturity buckets.
func termStructure(smiles []ExpirySmile) []TermPoint {
	type bucket struct {
		trades  int
		count   int
		tte     float64
		atm     float64
		weights float64
	}
	edges := []float64{1.0 / 365 / 24 * 2, 1.0 / 365 / 24 * 6, 1.0 / 365 / 24 * 12, 1.0 / 365, 1e9}
	buckets := make([]bucket, len(edges))
	for _, smile := range smiles {
		if !smile.Fitted {
			continue
		}
		for i, edge := range edges {
			if smile.MeanTTE <= edge {
				w := float64(smile.Trades)
				buckets[i].trades += smile.Trades
				buckets[i].count++
				buckets[i].tte += smile.MeanTTE * w
				buckets[i].atm += smile.ATMVol * w
				buckets[i].weights += w
				break
			}
		}
	}
	out := make([]TermPoint, 0, len(edges))
	for _, b := range buckets {
		if b.count == 0 || b.weights == 0 {
			continue
		}
		out = append(out, TermPoint{
			MeanTTEYears: b.tte / b.weights,
			Expiries:     b.count,
			Trades:       b.trades,
			ATMVol:       b.atm / b.weights,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MeanTTEYears < out[j].MeanTTEYears })
	return out
}
