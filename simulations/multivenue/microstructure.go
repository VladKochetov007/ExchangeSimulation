package multivenue

import (
	"math"
	"sort"
)

// MicrostructureStats accumulates the observables behind the Wyart-Bouchaud
// relation between spread and volatility per trade (arXiv:physics/0603084).
//
// That relation says the spread a marginally profitable market maker must
// charge is set by adverse selection, so S should be proportional to the
// volatility measured per trade rather than per unit time. Testing it needs
// three numbers from a run: the time-weighted spread, the volatility of the
// midpoint, and the number of trades.
//
// Samples are taken on the venue's automation tick, so the volatility here is
// per sampling interval; the per-trade figure is recovered by dividing by the
// square root of the trades observed per interval.
type MicrostructureStats struct {
	VenueID string `json:"venue_id"`
	Symbol  string `json:"symbol"`

	Samples int   `json:"samples"`
	Trades  int64 `json:"trades"`
	// MeanSpreadTicks and MeanRelativeSpread are time-weighted over samples
	// where the book was two-sided.
	MeanSpreadTicks    float64 `json:"mean_spread_ticks"`
	MeanRelativeSpread float64 `json:"mean_relative_spread"`
	MedianSpreadTicks  float64 `json:"median_spread_ticks"`
	// SigmaPerSample is the standard deviation of midpoint log returns between
	// consecutive samples; SigmaPerTrade rescales it by the trade rate.
	SigmaPerSample float64 `json:"sigma_per_sample"`
	SigmaPerTrade  float64 `json:"sigma_per_trade"`
	// MeanAbsMakerInventory and MaxAbsMakerInventory summarise how much
	// inventory the spot makers carry. A maker whose quotes do not respond to
	// inventory accumulates monotonically, so this is the observable that says
	// whether the inventory term binds.
	MeanAbsMakerInventory float64 `json:"mean_abs_maker_inventory"`
	MaxAbsMakerInventory  float64 `json:"max_abs_maker_inventory"`
	TradesPerSample       float64 `json:"trades_per_sample"`
	SampleIntervalSecs    float64 `json:"sample_interval_seconds"`

	tickSize    int64
	inventories []float64
	spreads     []float64
	relSpreads  []float64
	logReturns  []float64
	lastMid     int64
	firstTrades int64
	lastTrades  int64
	started     bool
}

func newMicrostructureStats(venueID, symbol string, tickSize int64, sampleIntervalSecs float64) *MicrostructureStats {
	return &MicrostructureStats{
		VenueID: venueID, Symbol: symbol,
		tickSize:           tickSize,
		SampleIntervalSecs: sampleIntervalSecs,
	}
}

// observe records one sample of the book. Samples where the book is not
// two-sided are skipped for the spread, and also break the return series: a
// return spanning an unpriceable gap is not a price change.
func (m *MicrostructureStats) observe(bestBid, bestAsk, cumulativeTrades int64) {
	if m == nil {
		return
	}
	if !m.started {
		m.firstTrades, m.started = cumulativeTrades, true
	}
	m.lastTrades = cumulativeTrades
	if bestBid <= 0 || bestAsk <= 0 || bestAsk <= bestBid {
		m.lastMid = 0
		return
	}
	mid := (bestBid + bestAsk) / 2
	spread := bestAsk - bestBid
	m.Samples++
	if m.tickSize > 0 {
		m.spreads = append(m.spreads, float64(spread)/float64(m.tickSize))
	}
	m.relSpreads = append(m.relSpreads, float64(spread)/float64(mid))
	if m.lastMid > 0 {
		if r := math.Log(float64(mid) / float64(m.lastMid)); finite(r) {
			m.logReturns = append(m.logReturns, r)
		}
	}
	m.lastMid = mid
}

// observeInventory records the absolute inventory of one maker, in base units
// scaled to whole units.
func (m *MicrostructureStats) observeInventory(inventory int64, basePrecision int64) {
	if m == nil || basePrecision <= 0 {
		return
	}
	value := float64(inventory) / float64(basePrecision)
	if value < 0 {
		value = -value
	}
	m.inventories = append(m.inventories, value)
}

func (m *MicrostructureStats) finalize() {
	if m == nil || m.Samples == 0 {
		return
	}
	m.MeanAbsMakerInventory = mean(m.inventories)
	for _, value := range m.inventories {
		if value > m.MaxAbsMakerInventory {
			m.MaxAbsMakerInventory = value
		}
	}
	m.inventories = nil
	m.Trades = m.lastTrades - m.firstTrades
	m.MeanSpreadTicks = mean(m.spreads)
	m.MeanRelativeSpread = mean(m.relSpreads)
	m.MedianSpreadTicks = median(m.spreads)
	m.SigmaPerSample = stddev(m.logReturns)
	m.TradesPerSample = float64(m.Trades) / float64(m.Samples)
	if m.TradesPerSample > 0 {
		// sigma_per_trade = sigma_per_sample / sqrt(trades per sample), the
		// identity behind sigma_1trade = sigma_daily / sqrt(N).
		m.SigmaPerTrade = m.SigmaPerSample / math.Sqrt(m.TradesPerSample)
	}
	m.spreads, m.relSpreads, m.logReturns = nil, nil, nil
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

func stddev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	avg := mean(values)
	total := 0.0
	for _, v := range values {
		total += (v - avg) * (v - avg)
	}
	return math.Sqrt(total / float64(len(values)-1))
}
