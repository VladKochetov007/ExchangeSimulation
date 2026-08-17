package analysis

import (
	"math"
	"sort"
	"sync"
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
}

// Tape reads one book's executions.
func (r *Run) Tape(venueID, symbol string) (*TradeTape, error) {
	var files []string
	for _, path := range r.files {
		if pathHasSymbol(path, venueID, symbol) {
			files = append(files, path)
		}
	}
	type payload struct {
		Price int64  `json:"price"`
		Qty   int64  `json:"qty"`
		Side  string `json:"side"`
	}
	type record struct {
		ts    int64
		price int64
		qty   int64
		sign  int8
	}
	var mu sync.Mutex
	var records []record
	err := r.Scan(ScanOptions{Events: []string{"Trade"}, Files: files}, func(event Event) {
		var decoded payload
		if event.Decode(&decoded) != nil || decoded.Price <= 0 {
			return
		}
		sign := int8(1)
		if decoded.Side == "SELL" {
			sign = -1
		}
		mu.Lock()
		records = append(records, record{event.SimTS, decoded.Price, decoded.Qty, sign})
		mu.Unlock()
	})
	if err != nil {
		return nil, err
	}
	// Files are scanned concurrently, so restore time order before differencing.
	sort.Slice(records, func(i, j int) bool { return records[i].ts < records[j].ts })
	tape := &TradeTape{}
	for _, rec := range records {
		tape.Timestamps = append(tape.Timestamps, rec.ts)
		tape.Prices = append(tape.Prices, rec.price)
		tape.Qtys = append(tape.Qtys, rec.qty)
		tape.Signs = append(tape.Signs, rec.sign)
	}
	return tape, nil
}

// LogReturns are successive log price changes in basis points.
func (t *TradeTape) LogReturns() []float64 {
	if len(t.Prices) < 2 {
		return nil
	}
	out := make([]float64, 0, len(t.Prices)-1)
	for i := 1; i < len(t.Prices); i++ {
		if t.Prices[i-1] <= 0 || t.Prices[i] <= 0 {
			continue
		}
		out = append(out, 1e4*math.Log(float64(t.Prices[i])/float64(t.Prices[i-1])))
	}
	return out
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

// HillTailIndex estimates the tail exponent of |returns| from the largest
// observations. Equity and crypto return tails sit near three.
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

	ReturnACF1     float64
	ReturnACFMean5 float64
	AbsReturnACF1  float64
	AbsReturnACF10 float64
	SignACF1       float64
	SignACF10      float64
	SignACF50      float64

	ExcessKurtosis float64
	TailIndex      float64

	ReturnStdBps float64
}

// Facts computes the stylized-fact panel for a tape.
func (t *TradeTape) Facts(maxLag int) StylizedFacts {
	returns := t.LogReturns()
	facts := StylizedFacts{Trades: len(t.Prices)}
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
	facts.TailIndex = HillTailIndex(returns, 0.05)

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
