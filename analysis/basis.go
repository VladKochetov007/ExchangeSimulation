package analysis

import (
	"math"
	"sort"
	"strings"
	"sync"
)

// Basis measurement.
//
// A venue publishes both a mark price and an index price for every derivative
// it lists, so the basis is read straight off the venue's own feed rather than
// joined across books:
//
//	basis_bps = 10000 * (mark - index) / index
//
// For the perpetual that quantity is the premium the preregistration calls the
// perp basis. For a dated future it is the carry that must go to zero at
// expiry, which is what the convergence component asks about.
//
// The mean-reversion estimator is stated here rather than inherited: nothing
// in the frozen configuration defined one, so an AR(1) fit is defined once and
// applied identically to control and treatment. Reading a half-life off it
// assumes the series is AR(1), which is an assumption about the estimator and
// not a finding about the market.

// BasisOptions selects the files to read.
type BasisOptions struct {
	Files         []string
	FilesSelected bool
	// ConvergenceBuckets are the time-to-expiry edges in seconds used to show
	// dated basis against maturity, largest first.
	ConvergenceBuckets []float64
}

// BasisSeriesStats summarises one basis series.
type BasisSeriesStats struct {
	VenueID string `json:"venue_id"`
	Symbol  string `json:"symbol"`
	// Observations is the number of published mark/index pairs.
	Observations int `json:"observations"`
	// UndefinedDomain counts published pairs retained by the evidence scan but
	// excluded from this positive-index BPS statistic. A present zero or signed
	// price is never silently treated as a missing mark.
	UndefinedDomain int     `json:"undefined_domain_observations"`
	MeanBps         float64 `json:"mean_bps"`
	MeanAbsBps      float64 `json:"mean_abs_bps"`
	StdDevBps       float64 `json:"std_dev_bps"`
	MinBps          float64 `json:"min_bps"`
	MaxBps          float64 `json:"max_bps"`
	// AR1 is the lag-one autocorrelation of the series and HalfLifeSeconds the
	// implied time for a deviation to decay by half. A coefficient at or above
	// one means the series does not revert on this sample and the half-life is
	// reported as zero rather than as a negative or infinite number.
	AR1              float64 `json:"ar1"`
	HalfLifeSeconds  float64 `json:"half_life_seconds"`
	HalfLifeDefined  bool    `json:"half_life_defined"`
	MedianStepSecond float64 `json:"median_step_seconds"`
}

// DatedConvergenceBucket is the dated basis at one distance from expiry.
type DatedConvergenceBucket struct {
	// UpperSeconds is the largest time to expiry in this bucket; the bucket
	// runs down to the next edge.
	UpperSeconds float64 `json:"upper_seconds"`
	LowerSeconds float64 `json:"lower_seconds"`
	Observations int     `json:"observations"`
	MeanAbsBps   float64 `json:"mean_abs_bps"`
	MeanBps      float64 `json:"mean_bps"`
}

// Basis is the whole measurement.
type Basis struct {
	Perp  []BasisSeriesStats `json:"perp"`
	Dated []BasisSeriesStats `json:"dated"`
	// PerpPooled and DatedPooled pool every venue's observations, so an arm
	// can be compared on one number per instrument class.
	PerpPooled  BasisSeriesStats `json:"perp_pooled"`
	DatedPooled BasisSeriesStats `json:"dated_pooled"`
	// Convergence is the dated basis against time to expiry. A contract that
	// converges shows the mean absolute basis falling as the buckets approach
	// zero.
	Convergence []DatedConvergenceBucket `json:"convergence"`
	// ConvergenceSlopeBpsPerHour regresses absolute dated basis on time to
	// expiry. Positive means wider when further from expiry, which is what
	// convergence looks like.
	ConvergenceSlopeBpsPerHour  float64 `json:"convergence_slope_bps_per_hour"`
	ConvergenceObservations     int     `json:"convergence_observations"`
	UndefinedDomainObservations int     `json:"undefined_domain_observations"`
}

type basisPoint struct {
	at  int64
	bps float64
	tte float64 // seconds to expiry, dated contracts only
}

// expiryFromSymbol reads the expiry token out of a dated contract's symbol.
// Legacy futures are named ABC-FUT-<epoch>; canonical calendar futures may
// append an underlying identity component after that token.
func expiryFromSymbol(symbol string) (int64, bool) {
	const marker = "-FUT-"
	index := strings.Index(symbol, marker)
	if index < 0 {
		return 0, false
	}
	suffix := symbol[index+len(marker):]
	if separator := strings.IndexByte(suffix, '-'); separator >= 0 {
		suffix = suffix[:separator]
	}
	return expiryNanoFromLabel(suffix)
}

// MeasureBasis reads every published mark and index and reports the basis, its
// persistence, and the dated term structure against time to expiry.
func (r *Run) MeasureBasis(opts BasisOptions) (*Basis, error) {
	type markPayload struct {
		Timestamp  int64  `json:"timestamp"`
		Symbol     string `json:"symbol"`
		MarkPrice  int64  `json:"mark_price"`
		IndexPrice int64  `json:"index_price"`
	}
	var mu sync.Mutex
	series := make(map[markKey][]basisPoint)
	undefined := make(map[markKey]int)

	scan := ScanOptions{
		Events:        []string{"mark_price_update"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		var payload markPayload
		if event.Decode(&payload) != nil || payload.Symbol == "" {
			return
		}
		at := payload.Timestamp
		if at == 0 {
			at = event.SimTS
		}
		key := markKey{event.VenueID, payload.Symbol}
		bps, defined := positiveIndexBasisBPS(payload.MarkPrice, payload.IndexPrice)
		if !defined {
			mu.Lock()
			undefined[key]++
			mu.Unlock()
			return
		}
		point := basisPoint{
			at:  at,
			bps: bps,
			tte: -1,
		}
		if expiry, dated := expiryFromSymbol(payload.Symbol); dated {
			point.tte = float64(expiry-at) / 1e9
		}
		mu.Lock()
		series[key] = append(series[key], point)
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	result := &Basis{}
	var perpPool, datedPool []basisPoint
	keys := make([]markKey, 0, len(series)+len(undefined))
	seenKeys := make(map[markKey]struct{}, len(series)+len(undefined))
	for key := range series {
		seenKeys[key] = struct{}{}
	}
	for key := range undefined {
		seenKeys[key] = struct{}{}
	}
	for key := range seenKeys {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		return keys[i].symbol < keys[j].symbol
	})

	for _, key := range keys {
		points := series[key]
		// The scan is concurrent, so order by time before anything that
		// depends on order.
		sort.Slice(points, func(i, j int) bool { return points[i].at < points[j].at })
		stats := summariseBasis(key, points)
		stats.UndefinedDomain = undefined[key]
		result.UndefinedDomainObservations += stats.UndefinedDomain
		if _, dated := expiryFromSymbol(key.symbol); dated {
			result.Dated = append(result.Dated, stats)
			datedPool = append(datedPool, points...)
			continue
		}
		result.Perp = append(result.Perp, stats)
		perpPool = append(perpPool, points...)
	}

	sort.Slice(perpPool, func(i, j int) bool { return perpPool[i].at < perpPool[j].at })
	sort.Slice(datedPool, func(i, j int) bool { return datedPool[i].at < datedPool[j].at })
	result.PerpPooled = summariseBasis(markKey{"pooled", "perp"}, perpPool)
	result.DatedPooled = summariseBasis(markKey{"pooled", "dated"}, datedPool)
	for _, stats := range result.Perp {
		result.PerpPooled.UndefinedDomain += stats.UndefinedDomain
	}
	for _, stats := range result.Dated {
		result.DatedPooled.UndefinedDomain += stats.UndefinedDomain
	}

	edges := opts.ConvergenceBuckets
	if len(edges) == 0 {
		edges = []float64{21600, 10800, 7200, 3600, 1800, 900, 300, 0}
	}
	result.Convergence, result.ConvergenceSlopeBpsPerHour, result.ConvergenceObservations =
		convergence(datedPool, edges)
	return result, nil
}

// positiveIndexBasisBPS defines the current funding/carry statistic. Its
// positive index denominator and positive mark are a mathematical/economic
// model domain, not an evidence-availability rule. Signed futures need a
// separately declared non-ratio basis statistic at or across zero.
func positiveIndexBasisBPS(mark, index int64) (float64, bool) {
	if mark <= 0 || index <= 0 {
		return 0, false
	}
	return 10000 * float64(mark-index) / float64(index), true
}

// summariseBasis reduces one time-ordered series to its moments and its
// lag-one persistence.
func summariseBasis(key markKey, points []basisPoint) BasisSeriesStats {
	stats := BasisSeriesStats{VenueID: key.venue, Symbol: key.symbol, Observations: len(points)}
	if len(points) == 0 {
		return stats
	}
	sum, sumAbs := 0.0, 0.0
	stats.MinBps, stats.MaxBps = points[0].bps, points[0].bps
	for _, point := range points {
		sum += point.bps
		sumAbs += math.Abs(point.bps)
		stats.MinBps = math.Min(stats.MinBps, point.bps)
		stats.MaxBps = math.Max(stats.MaxBps, point.bps)
	}
	stats.MeanBps = sum / float64(len(points))
	stats.MeanAbsBps = sumAbs / float64(len(points))

	variance := 0.0
	for _, point := range points {
		d := point.bps - stats.MeanBps
		variance += d * d
	}
	if len(points) > 1 {
		stats.StdDevBps = math.Sqrt(variance / float64(len(points)-1))
	}

	// Lag-one autocorrelation. Consecutive observations only; a series with
	// gaps still uses them, which is why the median step is reported beside
	// the half-life rather than assumed.
	if len(points) > 2 && variance > 0 {
		covariance := 0.0
		for i := 1; i < len(points); i++ {
			covariance += (points[i].bps - stats.MeanBps) * (points[i-1].bps - stats.MeanBps)
		}
		stats.AR1 = covariance / variance
	}
	steps := make([]float64, 0, len(points))
	for i := 1; i < len(points); i++ {
		if d := float64(points[i].at-points[i-1].at) / 1e9; d > 0 {
			steps = append(steps, d)
		}
	}
	if len(steps) > 0 {
		sort.Float64s(steps)
		stats.MedianStepSecond = steps[len(steps)/2]
	}
	if stats.AR1 > 0 && stats.AR1 < 1 && stats.MedianStepSecond > 0 {
		stats.HalfLifeSeconds = -math.Ln2 / math.Log(stats.AR1) * stats.MedianStepSecond
		stats.HalfLifeDefined = true
	}
	return stats
}

// convergence buckets the dated basis by time to expiry and regresses its
// magnitude on maturity.
func convergence(points []basisPoint, edges []float64) ([]DatedConvergenceBucket, float64, int) {
	buckets := make([]DatedConvergenceBucket, 0, len(edges))
	for i, upper := range edges {
		lower := 0.0
		if i+1 < len(edges) {
			lower = edges[i+1]
		}
		buckets = append(buckets, DatedConvergenceBucket{UpperSeconds: upper, LowerSeconds: lower})
	}
	sumX, sumY, sumXY, sumXX, n := 0.0, 0.0, 0.0, 0.0, 0
	for _, point := range points {
		if point.tte < 0 {
			continue
		}
		abs := math.Abs(point.bps)
		hours := point.tte / 3600
		sumX += hours
		sumY += abs
		sumXY += hours * abs
		sumXX += hours * hours
		n++
		for i := range buckets {
			if point.tte <= buckets[i].UpperSeconds && point.tte > buckets[i].LowerSeconds {
				buckets[i].Observations++
				buckets[i].MeanAbsBps += abs
				buckets[i].MeanBps += point.bps
				break
			}
		}
	}
	for i := range buckets {
		if buckets[i].Observations > 0 {
			buckets[i].MeanAbsBps /= float64(buckets[i].Observations)
			buckets[i].MeanBps /= float64(buckets[i].Observations)
		}
	}
	slope := 0.0
	if n > 1 {
		denominator := float64(n)*sumXX - sumX*sumX
		if denominator != 0 {
			slope = (float64(n)*sumXY - sumX*sumY) / denominator
		}
	}
	return buckets, slope, n
}
