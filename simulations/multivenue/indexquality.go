package multivenue

import (
	"hash/fnv"
	"math"
	"math/rand"
)

// DegradedIndexConfig makes the published index an imperfect observation of the
// value it tracks. Published without degradation the feed is a zero-lag,
// noise-free channel to the exogenous fundamental, so any actor quoting
// directly on it holds perfect information. Lag and observation noise are what
// separate an informed participant from an omniscient one.
type DegradedIndexConfig struct {
	// LagSamples delays the published value by this many observations.
	LagSamples int `json:"lag_samples"`
	// NoiseBps is the standard deviation of a multiplicative observation error.
	NoiseBps float64 `json:"noise_bps"`
	// Seed keeps the observation error reproducible across runs.
	Seed int64 `json:"seed"`
}

// degradedIndex wraps a price source, delaying and perturbing what it reports.
// Each symbol keeps its own history and its own error stream so that degrading
// one contract does not correlate with another.
type degradedIndex struct {
	source  PriceSource
	cfg     DegradedIndexConfig
	history map[string][]int64
	streams map[string]*rand.Rand
}

// PriceSource is the venue-side reference-price interface this file decorates.
type PriceSource interface {
	Price(symbol string) int64
}

func newDegradedIndex(source PriceSource, cfg DegradedIndexConfig) *degradedIndex {
	return &degradedIndex{
		source:  source,
		cfg:     cfg,
		history: make(map[string][]int64),
		streams: make(map[string]*rand.Rand),
	}
}

func (d *degradedIndex) Price(symbol string) int64 {
	value := d.source.Price(symbol)
	if value <= 0 {
		return value
	}
	if d.cfg.LagSamples > 0 {
		history := append(d.history[symbol], value)
		if len(history) > d.cfg.LagSamples+1 {
			history = history[len(history)-(d.cfg.LagSamples+1):]
		}
		d.history[symbol] = history
		value = history[0]
	}
	if d.cfg.NoiseBps > 0 {
		stream, ok := d.streams[symbol]
		if !ok {
			stream = rand.New(rand.NewSource(d.cfg.Seed + symbolSeed(symbol)))
			d.streams[symbol] = stream
		}
		perturbed := float64(value) * (1 + stream.NormFloat64()*d.cfg.NoiseBps/10000)
		if perturbed < 1 {
			perturbed = 1
		}
		value = int64(math.Round(perturbed))
	}
	return value
}

func symbolSeed(symbol string) int64 {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(symbol))
	return int64(digest.Sum64() & math.MaxInt32)
}
