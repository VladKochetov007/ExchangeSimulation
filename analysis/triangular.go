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

// TriangularDeviation returns the basis-point deviation of the cross rate from
// the rate implied by the other two books, one observation per bucket in which
// all three books traded.
func (r *Run) TriangularDeviation(cfg TriangularConfig) ([]float64, error) {
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
	err := r.Scan(ScanOptions{Events: []string{"Trade"}, Files: files}, func(event Event) {
		var trade tradePayload
		if event.Decode(&trade) != nil || trade.Price <= 0 {
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
	deviations := make([]float64, 0, len(buckets))
	for _, key := range buckets {
		implied := float64(base[key]) / float64(quote[key]) * float64(cfg.CrossPrecision)
		if implied <= 0 {
			continue
		}
		deviations = append(deviations, 1e4*(float64(cross[key])/implied-1))
	}
	return deviations, nil
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
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}
