package multivenue

import (
	"math"
	"sort"
)

// MispricingStats measures how far a venue's quoted price stays from the
// exogenous fundamental value.
//
// This is the observable for anchoring capacity: informed participants are
// bounded by inventory and funding, so a sustained drift can exhaust them. When
// that happens the quoted price detaches until the informed side recovers, and
// these statistics are how large and how long that excursion was.
//
// Deviations are log ratios, so they are symmetric in direction and comparable
// across price levels.
type MispricingStats struct {
	VenueID string `json:"venue_id"`
	Symbol  string `json:"symbol"`
	Samples int    `json:"samples"`
	// Missing counts samples where the book had no usable two-sided mark.
	Missing int `json:"missing_samples"`

	MeanAbsLogDeviation   float64 `json:"mean_abs_log_deviation"`
	MedianAbsLogDeviation float64 `json:"median_abs_log_deviation"`
	MaxAbsLogDeviation    float64 `json:"max_abs_log_deviation"`
	// FractionBeyondBand is the share of samples further than BandBps from
	// fundamental value, i.e. how much of the run was visibly mispriced.
	BandBps            int64   `json:"band_bps"`
	FractionBeyondBand float64 `json:"fraction_beyond_band"`
	// LongestExcursionSeconds is the longest unbroken stretch beyond the band.
	LongestExcursionSeconds float64 `json:"longest_excursion_seconds"`

	deviations       []float64
	band             float64
	excursionStart   int64
	inExcursion      bool
	longestExcursion int64
	beyondBand       int
}

func newMispricingStats(venueID, symbol string, bandBps int64) *MispricingStats {
	return &MispricingStats{
		VenueID: venueID,
		Symbol:  symbol,
		BandBps: bandBps,
		band:    float64(bandBps) / 10_000.0,
	}
}

// observe records one sample. A missing mark is counted but does not break an
// excursion: an unpriceable book is not evidence that the price returned.
func (m *MispricingStats) observe(timestamp, mark, fundamental int64) {
	if m == nil {
		return
	}
	if mark <= 0 || fundamental <= 0 {
		m.Missing++
		return
	}
	deviation := math.Log(float64(mark) / float64(fundamental))
	if !finite(deviation) {
		m.Missing++
		return
	}
	m.Samples++
	m.deviations = append(m.deviations, math.Abs(deviation))
	if math.Abs(deviation) > m.band {
		m.beyondBand++
		if !m.inExcursion {
			m.inExcursion, m.excursionStart = true, timestamp
		} else if run := timestamp - m.excursionStart; run > m.longestExcursion {
			m.longestExcursion = run
		}
		return
	}
	m.inExcursion = false
}

// finalize computes the summary values. It is called once, after the run.
func (m *MispricingStats) finalize() {
	if m == nil || m.Samples == 0 {
		return
	}
	total := 0.0
	for _, deviation := range m.deviations {
		total += deviation
		if deviation > m.MaxAbsLogDeviation {
			m.MaxAbsLogDeviation = deviation
		}
	}
	m.MeanAbsLogDeviation = total / float64(m.Samples)
	sorted := append([]float64(nil), m.deviations...)
	sort.Float64s(sorted)
	m.MedianAbsLogDeviation = sorted[len(sorted)/2]
	m.FractionBeyondBand = float64(m.beyondBand) / float64(m.Samples)
	m.LongestExcursionSeconds = float64(m.longestExcursion) / 1e9
	m.deviations = nil
}
