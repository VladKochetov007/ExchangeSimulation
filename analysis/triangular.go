package analysis

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// TriangularConfig names the three books of a currency triangle.
//
// Base and Cross share a base asset; Quote is the pair connecting their quote
// currencies. With ABC/USD, CDF/USD and ABC/CDF the implied cross rate is
// ABC/USD divided by CDF/USD, and the deviation is how far the traded ABC/CDF
// price sits from it.
type TriangularConfig struct {
	VenueID    string
	BaseSymbol string // ABC-USD
	QuotePair  string // CDF-USD
	CrossPair  string // ABC-CDF
	// CrossPrecision scales the cross pair's price into the ratio's units.
	CrossPrecision int64
	// BucketNanos groups trades before comparing books, since the three do not
	// print at the same instant. Zero uses one second.
	BucketNanos int64
}

// TriangularDeviationResult is the positive-ratio triangular statistic and
// its declared-domain accounting. A signed trade remains in the evidence scan;
// it is not converted to a missing trade merely because the current ratio is
// undefined at or across zero.
type TriangularDeviationResult struct {
	// DeviationsBps has one positive-domain observation per bucket in which all
	// three books traded.
	DeviationsBps []float64 `json:"deviations_bps"`
	// CompleteBuckets has all three trade observations before application of
	// the current positive-ratio domain.
	CompleteBuckets int `json:"complete_buckets"`
	// UndefinedDomainObservations counts complete buckets excluded because one
	// of the prices is zero or negative. It is not a missing-data count.
	UndefinedDomainObservations int `json:"undefined_domain_observations"`
}

// MeasureTriangularDeviation returns the basis-point deviation of the cross
// rate from the rate implied by the other two books, one observation per bucket
// in which all three books traded. The calculation currently models a positive
// cash-rate triangle; signed levels are retained as explicit undefined-domain
// observations instead of being silently discarded.
func (r *Run) MeasureTriangularDeviation(cfg TriangularConfig) (*TriangularDeviationResult, error) {
	if cfg.CrossPrecision <= 0 {
		return nil, fmt.Errorf("analysis: cross precision must be positive")
	}
	bucket := cfg.BucketNanos
	if bucket <= 0 {
		bucket = int64(1e9)
	}
	last := map[string]map[int64]int64{
		cfg.BaseSymbol: {},
		cfg.QuotePair:  {},
		cfg.CrossPair:  {},
	}
	var files []string
	for _, path := range r.files {
		for _, symbol := range []string{cfg.BaseSymbol, cfg.QuotePair, cfg.CrossPair} {
			if pathHasSymbol(path, cfg.VenueID, symbol) {
				files = append(files, path)
			}
		}
	}
	type tradePayload struct {
		Price int64 `json:"price"`
	}
	var mu sync.Mutex
	err := r.Scan(ScanOptions{Events: []string{"Trade"}, Files: files, FilesSelected: true}, func(event Event) {
		var trade tradePayload
		if event.Decode(&trade) != nil {
			return
		}
		symbol := symbolFromPath(event.File)
		series, ok := last[symbol]
		if !ok {
			return
		}
		mu.Lock()
		series[event.SimTS/bucket] = trade.Price
		mu.Unlock()
	})
	if err != nil {
		return nil, err
	}
	base, quote, cross := last[cfg.BaseSymbol], last[cfg.QuotePair], last[cfg.CrossPair]
	buckets := make([]int64, 0, len(cross))
	for key := range cross {
		if _, ok := base[key]; !ok {
			continue
		}
		if _, ok := quote[key]; !ok {
			continue
		}
		buckets = append(buckets, key)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })
	result := &TriangularDeviationResult{DeviationsBps: make([]float64, 0, len(buckets))}
	for _, key := range buckets {
		result.CompleteBuckets++
		if !positiveTriangleRatePrices(base[key], quote[key], cross[key]) {
			result.UndefinedDomainObservations++
			continue
		}
		implied := float64(base[key]) / float64(quote[key]) * float64(cfg.CrossPrecision)
		result.DeviationsBps = append(result.DeviationsBps, 1e4*(float64(cross[key])/implied-1))
	}
	return result, nil
}

// TriangularDeviation preserves the legacy slice-only API. New evidence
// consumers should use MeasureTriangularDeviation so undefined signed-domain
// observations cannot be mistaken for absent triangle activity.
func (r *Run) TriangularDeviation(cfg TriangularConfig) ([]float64, error) {
	result, err := r.MeasureTriangularDeviation(cfg)
	if err != nil {
		return nil, err
	}
	return result.DeviationsBps, nil
}

// positiveTriangleRatePrices is the current triangle statistic's domain, not
// a presence test. A signed currency pair can be present but cannot enter the
// conventional rate ratio without a separately declared signed-rate model.
func positiveTriangleRatePrices(base, quote, cross int64) bool {
	return base > 0 && quote > 0 && cross > 0
}

// pathHasSymbol reports whether a log file belongs to a venue's symbol book.
func pathHasSymbol(path, venueID, symbol string) bool {
	if venueID != "" && !strings.Contains(path, string(filepath.Separator)+venueID+string(filepath.Separator)) {
		return false
	}
	return symbolFromPath(path) == symbol
}

// symbolFromPath recovers the instrument from a book log's filename.
func symbolFromPath(path string) string {
	return strings.TrimSuffix(logicalEventLogName(path), ".jsonl")
}
