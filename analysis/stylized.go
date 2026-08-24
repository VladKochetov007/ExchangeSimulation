package analysis

import (
	"math"
	"sort"
	"sync"

	etypes "exchange_sim/types"
)

// TradeTape is one instrument's executions in time order.
//
// Every stylized fact below is a property of this sequence, so it is built once
// and reused rather than rescanned per metric.
type TradeTape struct {
	Timestamps []int64
	Prices     []int64
	Qtys       []int64
	// Signs are the aggressor's direction: +1 when a buy lifted the offer.
	Signs []int8
	// Roles are the participant class of each trade's aggressor, recovered by
	// joining the taker order id to the client that placed it.
	//
	// Impact pooled over every participant measures who trades at each size
	// rather than what a trade does: in one run the response rose to 0.32 bps
	// by 0.05 ABC, turned negative between 0.22 and 0.55 where desks trading
	// against the move concentrate, and rose again to 0.37 at 2.8 ABC. Only a
	// single class gives a curve with one meaning.
	Roles []string
	// PreMid is the book midpoint last published before each trade, which is
	// the only unbiased reference for measuring impact.
	//
	// A trade price sits on one side of the spread, and the side is correlated
	// with the aggressor, so referencing a trade against its own price or
	// against the previous trade subtracts a half spread from every buy. Both
	// were tried: the first left every size bucket reading the half spread and
	// nothing else, the second left a smaller bias that survived because signed
	// order flow is autocorrelated.
	PreMid []int64
	// PreMidAvailable is parallel to PreMid. It makes a present numeric zero
	// midpoint distinct from a trade that had no preceding two-sided snapshot.
	// Older hand-built test tapes that omit this field declare every supplied
	// PreMid entry present; production tapes always populate it explicitly.
	PreMidAvailable []bool
	// TakerOrderIDs identifies the order that crossed, so trades belonging to
	// one aggressive order can be recognised as a single sweep.
	TakerOrderIDs []uint64
}

// SignACFSum is the sum of the trade-sign autocorrelation over lags 1..h-1.
//
// It is the multiplier that converts a per-trade price displacement into the
// response measured over an h-trade horizon: the signed response accumulates
// over the next h trades weighted by how often they share the first trade's
// direction. Assuming the multiplier is h itself asserts that every subsequent
// trade has the same sign, which no traded market does.
func (t *TradeTape) SignACFSum(horizon int) float64 {
	signs := make([]float64, len(t.Signs))
	for i, sign := range t.Signs {
		signs[i] = float64(sign)
	}
	if horizon < 2 {
		return 0
	}
	total := 0.0
	for _, value := range Autocorrelation(signs, horizon-1) {
		total += value
	}
	return total
}

// Tape reads one book's executions.
func (r *Run) Tape(venueID, symbol string) (*TradeTape, error) {
	files := r.BookFiles(venueID, symbol)
	type payload struct {
		Price        int64  `json:"price"`
		Qty          int64  `json:"qty"`
		Side         string `json:"side"`
		TakerOrderID uint64 `json:"taker_order_id"`
	}
	type acceptPayload struct {
		OrderID uint64 `json:"order_id"`
	}
	// Order ownership has to be recovered before the trades are read, because a
	// trade names only the order that crossed.
	owner := map[uint64]string{}
	var ownerMu sync.Mutex
	if err := r.Scan(ScanOptions{Events: []string{"OrderAccepted"}, Files: files, FilesSelected: true}, func(event Event) {
		var accepted acceptPayload
		if event.Decode(&accepted) != nil || accepted.OrderID == 0 {
			return
		}
		role := r.Role(event.VenueID, event.ClientID)
		if role == "" {
			return
		}
		ownerMu.Lock()
		owner[accepted.OrderID] = role
		ownerMu.Unlock()
	}); err != nil {
		return nil, err
	}
	type record struct {
		ts      int64
		price   int64
		qty     int64
		sign    int8
		mid     int64
		role    string
		orderID uint64
	}
	var mu sync.Mutex
	var records []record
	// Snapshots are collected first so each trade can be referenced against the
	// midpoint that preceded it.
	type levels struct {
		Bids []struct {
			Price int64 `json:"price"`
		} `json:"bids"`
		Asks []struct {
			Price int64 `json:"price"`
		} `json:"asks"`
	}
	type snapshotPayload struct {
		Snapshot *levels `json:"snapshot"`
		Bids     []struct {
			Price int64 `json:"price"`
		} `json:"bids"`
		Asks []struct {
			Price int64 `json:"price"`
		} `json:"asks"`
	}
	type midpointObservation struct {
		price int64
		ok    bool
	}
	midAt := map[int64]midpointObservation{}
	var midMu sync.Mutex
	if err := r.Scan(ScanOptions{Events: []string{"BookSnapshot"}, Files: files, FilesSelected: true}, func(event Event) {
		var decoded snapshotPayload
		if event.Decode(&decoded) != nil {
			return
		}
		bids, asks := decoded.Bids, decoded.Asks
		if decoded.Snapshot != nil {
			bids, asks = decoded.Snapshot.Bids, decoded.Snapshot.Asks
		}
		if len(bids) == 0 || len(asks) == 0 {
			return
		}
		if bids[0].Price > asks[0].Price {
			return
		}
		midMu.Lock()
		midAt[event.SimTS] = midpointObservation{price: etypes.Midpoint(bids[0].Price, asks[0].Price), ok: true}
		midMu.Unlock()
	}); err != nil {
		return nil, err
	}
	midKeys := make([]int64, 0, len(midAt))
	for key := range midAt {
		midKeys = append(midKeys, key)
	}
	sort.Slice(midKeys, func(i, j int) bool { return midKeys[i] < midKeys[j] })
	lastMidAtOrBefore := func(ts int64) (int64, bool) {
		index := sort.Search(len(midKeys), func(i int) bool { return midKeys[i] > ts }) - 1
		if index < 0 {
			return 0, false
		}
		observation := midAt[midKeys[index]]
		return observation.price, observation.ok
	}

	err := r.Scan(ScanOptions{Events: []string{"Trade"}, Files: files, FilesSelected: true}, func(event Event) {
		var decoded payload
		if event.Decode(&decoded) != nil {
			return
		}
		sign := int8(1)
		if decoded.Side == "SELL" {
			sign = -1
		}
		mu.Lock()
		records = append(records, record{event.SimTS, decoded.Price, decoded.Qty, sign, 0, owner[decoded.TakerOrderID], decoded.TakerOrderID})
		mu.Unlock()
	})
	if err != nil {
		return nil, err
	}
	// Files are scanned concurrently, so restore time order before differencing.
	//
	// Stable, because the clock granularity is coarser than the trade rate:
	// dozens of trades share one timestamp, and an unstable sort permutes them
	// arbitrarily. The series being differenced would then not be the sequence
	// the matcher produced, and the first-lag bounce is measured on it.
	sort.SliceStable(records, func(i, j int) bool { return records[i].ts < records[j].ts })
	tape := &TradeTape{}
	for _, rec := range records {
		tape.Timestamps = append(tape.Timestamps, rec.ts)
		tape.Prices = append(tape.Prices, rec.price)
		tape.Qtys = append(tape.Qtys, rec.qty)
		tape.Signs = append(tape.Signs, rec.sign)
		mid, midOK := lastMidAtOrBefore(rec.ts)
		tape.PreMid = append(tape.PreMid, mid)
		tape.PreMidAvailable = append(tape.PreMidAvailable, midOK)
		tape.Roles = append(tape.Roles, rec.role)
		tape.TakerOrderIDs = append(tape.TakerOrderIDs, rec.orderID)
	}
	return tape, nil
}

func (t *TradeTape) preMidAt(index int) (int64, bool) {
	if index < 0 || index >= len(t.PreMid) {
		return 0, false
	}
	if len(t.PreMidAvailable) == 0 {
		return t.PreMid[index], true
	}
	if index >= len(t.PreMidAvailable) || !t.PreMidAvailable[index] {
		return 0, false
	}
	return t.PreMid[index], true
}

// LogReturnSeries is a positive-price log-return sample plus the explicit
// accounting required when signed price evidence lies outside that statistic's
// domain. A zero count must never be inferred from an empty return slice.
type LogReturnSeries struct {
	Returns []float64 `json:"returns_bps"`
	// CandidatePairs is the number of adjacent sampled prices considered.
	CandidatePairs int `json:"candidate_pairs"`
	// UndefinedDomainPairs counts present endpoint pairs that cannot enter the
	// current positive-price log-return statistic. It is not missing evidence.
	UndefinedDomainPairs int `json:"undefined_domain_pairs"`
}

func positiveLogReturns(prices []int64) LogReturnSeries {
	if len(prices) < 2 {
		return LogReturnSeries{}
	}
	result := LogReturnSeries{Returns: make([]float64, 0, len(prices)-1)}
	for i := 1; i < len(prices); i++ {
		result.CandidatePairs++
		if prices[i-1] <= 0 || prices[i] <= 0 {
			result.UndefinedDomainPairs++
			continue
		}
		result.Returns = append(result.Returns, 1e4*math.Log(float64(prices[i])/float64(prices[i-1])))
	}
	return result
}

// LogReturnSeries returns successive positive-price log changes in basis
// points. Prices at or below zero remain in the source tape and are accounted
// as undefined because this conventional log-price statistic has no signed
// market domain.
func (t *TradeTape) LogReturnSeries() LogReturnSeries { return positiveLogReturns(t.Prices) }

// LogReturns preserves the legacy slice-only API. New evidence consumers
// should use LogReturnSeries to retain the explicit domain accounting.
func (t *TradeTape) LogReturns() []float64 { return t.LogReturnSeries().Returns }

// timeSampledPrices takes the last trade in each bucket of the given width.
func (t *TradeTape) timeSampledPrices(bucketNanos int64) []int64 {
	if bucketNanos <= 0 || len(t.Prices) < 2 {
		return nil
	}
	prices := make([]int64, 0, len(t.Prices))
	lastBucket := int64(-1)
	for i, ts := range t.Timestamps {
		bucket := ts / bucketNanos
		if bucket != lastBucket && lastBucket >= 0 {
			prices = append(prices, t.Prices[i-1])
		}
		lastBucket = bucket
	}
	prices = append(prices, t.Prices[len(t.Prices)-1])
	return prices
}

// TimeSampledLogReturnSeries takes the last trade in each bucket, then reports
// the positive-price log returns and every signed-domain exclusion. This is the
// clock the empirical panel uses: in trade time the absolute return is pinned
// to the half-spread and can be memoryless even when the underlying process
// has volatility clustering.
func (t *TradeTape) TimeSampledLogReturnSeries(bucketNanos int64) LogReturnSeries {
	return positiveLogReturns(t.timeSampledPrices(bucketNanos))
}

// TimeSampledLogReturns preserves the legacy slice-only API.
func (t *TradeTape) TimeSampledLogReturns(bucketNanos int64) []float64 {
	return t.TimeSampledLogReturnSeries(bucketNanos).Returns
}

// StridedLogReturnSeries samples every stride trades before reporting positive
// log returns and explicit signed-domain exclusions.
func (t *TradeTape) StridedLogReturnSeries(stride int) LogReturnSeries {
	if stride < 1 || len(t.Prices) <= stride {
		return LogReturnSeries{}
	}
	result := LogReturnSeries{Returns: make([]float64, 0, len(t.Prices)/stride)}
	for i := stride; i < len(t.Prices); i += stride {
		result.CandidatePairs++
		if t.Prices[i-stride] <= 0 || t.Prices[i] <= 0 {
			result.UndefinedDomainPairs++
			continue
		}
		result.Returns = append(result.Returns, 1e4*math.Log(float64(t.Prices[i])/float64(t.Prices[i-stride])))
	}
	return result
}

// StridedLogReturns preserves the legacy slice-only API.
func (t *TradeTape) StridedLogReturns(stride int) []float64 {
	return t.StridedLogReturnSeries(stride).Returns
}

// Autocorrelation returns the sample autocorrelation of a series at each lag
// from one to maxLag.
func Autocorrelation(series []float64, maxLag int) []float64 {
	n := len(series)
	if n < 2 || maxLag < 1 {
		return nil
	}
	mean := 0.0
	for _, value := range series {
		mean += value
	}
	mean /= float64(n)
	var variance float64
	for _, value := range series {
		variance += (value - mean) * (value - mean)
	}
	if variance == 0 {
		return make([]float64, maxLag)
	}
	out := make([]float64, maxLag)
	for lag := 1; lag <= maxLag; lag++ {
		if lag >= n {
			break
		}
		var covariance float64
		for i := lag; i < n; i++ {
			covariance += (series[i] - mean) * (series[i-lag] - mean)
		}
		out[lag-1] = covariance / variance
	}
	return out
}

// SignSeries converts trade signs into a float series for autocorrelation.
func (t *TradeTape) SignSeries() []float64 {
	out := make([]float64, len(t.Signs))
	for i, sign := range t.Signs {
		out[i] = float64(sign)
	}
	return out
}

// Kurtosis is the excess kurtosis of a sample. A Gaussian gives zero; real
// return distributions are strongly positive, which is the fat-tail fact.
func Kurtosis(sample []float64) float64 {
	n := float64(len(sample))
	if n < 4 {
		return math.NaN()
	}
	var mean float64
	for _, value := range sample {
		mean += value
	}
	mean /= n
	var m2, m4 float64
	for _, value := range sample {
		d := value - mean
		m2 += d * d
		m4 += d * d * d * d
	}
	m2 /= n
	m4 /= n
	if m2 == 0 {
		return math.NaN()
	}
	return m4/(m2*m2) - 3
}

// HillPlot returns the Hill estimate at a range of tail sizes.
//
// A Hill estimate is only meaningful where it is stable in k. Reading it at one
// arbitrary k reports a number for any sample at all, including samples with no
// tail: on a bounded lattice-valued series this estimator returned 297.9, 39.5
// and 16.9 at tail fractions of 0.005, 0.05 and 0.2 of the same data, and on a
// Pareto-tailed series the choice of fraction reversed which of two arms looked
// better calibrated. Callers should look at the plot, not at a point.
func HillPlot(sample []float64, fractions []float64) map[float64]float64 {
	out := make(map[float64]float64, len(fractions))
	for _, fraction := range fractions {
		out[fraction] = HillTailIndex(sample, fraction)
	}
	return out
}

// HillStability reports the spread of Hill estimates across tail fractions,
// relative to their median. A power-law sample gives a plateau and a small
// spread; a sample with no tail gives a large one.
func HillStability(sample []float64) (median float64, spread float64) {
	fractions := []float64{0.01, 0.02, 0.05, 0.10, 0.20}
	estimates := make([]float64, 0, len(fractions))
	for _, fraction := range fractions {
		if value := HillTailIndex(sample, fraction); !math.IsNaN(value) {
			estimates = append(estimates, value)
		}
	}
	if len(estimates) < 3 {
		return math.NaN(), math.NaN()
	}
	sort.Float64s(estimates)
	median = estimates[len(estimates)/2]
	if median <= 0 {
		return median, math.NaN()
	}
	return median, (estimates[len(estimates)-1] - estimates[0]) / median
}

// HillTailIndex estimates the tail exponent of |returns| from the largest
// observations.
//
// Valid only where the estimate is stable in k; use HillStability to check
// before reporting it. Equity and crypto return tails sit near three.
func HillTailIndex(sample []float64, tailFraction float64) float64 {
	magnitudes := make([]float64, 0, len(sample))
	for _, value := range sample {
		if v := math.Abs(value); v > 0 {
			magnitudes = append(magnitudes, v)
		}
	}
	if len(magnitudes) < 50 {
		return math.NaN()
	}
	sort.Float64s(magnitudes)
	k := int(tailFraction * float64(len(magnitudes)))
	if k < 10 {
		k = 10
	}
	if k >= len(magnitudes) {
		return math.NaN()
	}
	threshold := magnitudes[len(magnitudes)-k-1]
	if threshold <= 0 {
		return math.NaN()
	}
	var sum float64
	for _, value := range magnitudes[len(magnitudes)-k:] {
		sum += math.Log(value / threshold)
	}
	if sum == 0 {
		return math.NaN()
	}
	return float64(k) / sum
}

// StylizedFacts collects the properties a price series must have to resemble a
// traded market.
//
// The reference values are the empirical regularities reported across equities,
// futures and crypto: returns are close to unpredictable, their magnitudes are
// strongly persistent, the distribution is fat tailed, and the sign of order
// flow is long-range correlated because large orders are split.
type StylizedFacts struct {
	Trades int
	// The return-domain counters are present evidence excluded from the
	// conventional positive log-price statistic, never zero-valued returns.
	LogReturnPairs                  int `json:"log_return_pairs"`
	LogReturnUndefinedDomainPairs   int `json:"log_return_undefined_domain_pairs"`
	Sec1ReturnUndefinedDomainPairs  int `json:"sec1_return_undefined_domain_pairs"`
	Sec60ReturnUndefinedDomainPairs int `json:"sec60_return_undefined_domain_pairs"`
	Stride20UndefinedDomainPairs    int `json:"stride20_return_undefined_domain_pairs"`
	Stride100UndefinedDomainPairs   int `json:"stride100_return_undefined_domain_pairs"`

	ReturnACF1     float64
	ReturnACFMean5 float64
	AbsReturnACF1  float64
	AbsReturnACF10 float64
	SignACF1       float64
	SignACF10      float64
	SignACF50      float64

	ExcessKurtosis float64
	TailIndex      float64
	// TailSpread is the relative spread of Hill estimates across tail sizes.
	// Above roughly one the tail index is not estimable and TailIndex should
	// not be reported as a measurement.
	TailSpread float64

	// Strided facts resample the tape every N trades. The empirical values a
	// panel is scored against are measured on time-sampled returns, where the
	// bid-ask bounce no longer dominates the first lag, so a trade-indexed
	// number is not comparable to them.
	Stride20ReturnACF1  float64
	Stride20Kurtosis    float64
	Stride100ReturnACF1 float64
	Stride100Kurtosis   float64

	// Time-sampled facts are the ones comparable to published empirical values.
	Sec1ReturnACF1     float64
	Sec1AbsReturnACF1  float64
	Sec1AbsReturnACF10 float64
	Sec1Kurtosis       float64
	Sec60ReturnACF1    float64
	Sec60AbsReturnACF1 float64
	// Sec60ReturnACFSum is the sum of the first lags of the sixty-second return
	// autocorrelation, and Sec60VarianceRatio is the Bartlett-weighted form a
	// random walk sets to one.
	//
	// A single lag cannot say whether a level wanders or slides: one arm here
	// reported 0.007 at lag one while lags two through twenty ran between 0.11
	// and 0.27, summing to 4.97 and implying a ratio near seven. Reading lag one
	// alone called a strongly trending series stable.
	Sec60ReturnACFSum  float64
	Sec60VarianceRatio float64

	ReturnStdBps float64
}

// Facts computes the stylized-fact panel for a tape.
func (t *TradeTape) Facts(maxLag int) StylizedFacts {
	returnSeries := t.LogReturnSeries()
	returns := returnSeries.Returns
	facts := StylizedFacts{
		Trades:                        len(t.Prices),
		LogReturnPairs:                returnSeries.CandidatePairs,
		LogReturnUndefinedDomainPairs: returnSeries.UndefinedDomainPairs,
	}
	oneSecondSeries := t.TimeSampledLogReturnSeries(1e9)
	oneMinuteSeries := t.TimeSampledLogReturnSeries(60e9)
	stride20Series := t.StridedLogReturnSeries(20)
	stride100Series := t.StridedLogReturnSeries(100)
	facts.Sec1ReturnUndefinedDomainPairs = oneSecondSeries.UndefinedDomainPairs
	facts.Sec60ReturnUndefinedDomainPairs = oneMinuteSeries.UndefinedDomainPairs
	facts.Stride20UndefinedDomainPairs = stride20Series.UndefinedDomainPairs
	facts.Stride100UndefinedDomainPairs = stride100Series.UndefinedDomainPairs
	if len(returns) < 10 {
		return facts
	}
	absReturns := Abs(returns)
	returnACF := Autocorrelation(returns, maxLag)
	absACF := Autocorrelation(absReturns, maxLag)
	signACF := Autocorrelation(t.SignSeries(), maxLag)

	at := func(series []float64, lag int) float64 {
		if lag-1 < len(series) {
			return series[lag-1]
		}
		return math.NaN()
	}
	facts.ReturnACF1 = at(returnACF, 1)
	var sum float64
	count := 0
	for lag := 1; lag <= 5 && lag-1 < len(returnACF); lag++ {
		sum += returnACF[lag-1]
		count++
	}
	if count > 0 {
		facts.ReturnACFMean5 = sum / float64(count)
	}
	facts.AbsReturnACF1 = at(absACF, 1)
	facts.AbsReturnACF10 = at(absACF, 10)
	facts.SignACF1 = at(signACF, 1)
	facts.SignACF10 = at(signACF, 10)
	facts.SignACF50 = at(signACF, 50)
	facts.ExcessKurtosis = Kurtosis(returns)
	facts.TailIndex, facts.TailSpread = HillStability(returns)

	if oneSecond := oneSecondSeries.Returns; len(oneSecond) > 50 {
		facts.Sec1ReturnACF1 = Autocorrelation(oneSecond, 1)[0]
		absOne := Autocorrelation(Abs(oneSecond), 10)
		facts.Sec1AbsReturnACF1, facts.Sec1AbsReturnACF10 = absOne[0], absOne[9]
		facts.Sec1Kurtosis = Kurtosis(oneSecond)
	}
	if oneMinute := oneMinuteSeries.Returns; len(oneMinute) > 50 {
		lags := Autocorrelation(oneMinute, 30)
		facts.Sec60ReturnACF1 = lags[0]
		facts.Sec60AbsReturnACF1 = Autocorrelation(Abs(oneMinute), 1)[0]
		for _, value := range lags[:29] {
			facts.Sec60ReturnACFSum += value
		}
		// The Bartlett-weighted sum is the variance ratio a random walk sets to
		// one, so a value far above it is a trending series however small the
		// first lag happens to be.
		weighted := 0.0
		for index, value := range lags[:29] {
			weighted += (1 - float64(index+1)/30) * value
		}
		facts.Sec60VarianceRatio = 1 + 2*weighted
	}
	for _, sample := range []struct {
		stride int
		series LogReturnSeries
	}{
		{stride: 20, series: stride20Series},
		{stride: 100, series: stride100Series},
	} {
		stride := sample.stride
		stridedSeries := sample.series
		strided := stridedSeries.Returns
		if len(strided) < 50 {
			continue
		}
		acf := Autocorrelation(strided, 1)
		kurt := Kurtosis(strided)
		if stride == 20 {
			facts.Stride20ReturnACF1, facts.Stride20Kurtosis = acf[0], kurt
		} else {
			facts.Stride100ReturnACF1, facts.Stride100Kurtosis = acf[0], kurt
		}
	}

	var mean, variance float64
	for _, value := range returns {
		mean += value
	}
	mean /= float64(len(returns))
	for _, value := range returns {
		variance += (value - mean) * (value - mean)
	}
	facts.ReturnStdBps = math.Sqrt(variance / float64(len(returns)))
	return facts
}
