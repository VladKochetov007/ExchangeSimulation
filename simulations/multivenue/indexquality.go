package multivenue

import (
	"hash/fnv"
	"math"
	"math/rand"
)

// DegradedIndexConfig makes a venue's published index an imperfect observation.
// The index is built from venue midpoints, so it is already endogenous; lag and
// observation noise model the transport and measurement error a participant
// really faces rather than any hidden reference.
type DegradedIndexConfig struct {
	// LagSamples delays the published value by this many observations.
	LagSamples int `json:"lag_samples"`
	// NoiseBps is the standard deviation of a multiplicative observation error.
	NoiseBps float64 `json:"noise_bps"`
	// Seed keeps the observation error reproducible across runs.
	Seed int64 `json:"seed"`
}

// observationIsImperfect reports whether the configuration actually degrades the
// observation. A nil or all-zero config leaves the feed a perfect oracle.
func (c *DegradedIndexConfig) observationIsImperfect() bool {
	return c != nil && (c.LagSamples > 0 || c.NoiseBps > 0)
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

// ScientificIndexDefaults returns a plausible observation degradation: the
// published consensus is seen a few samples late and imprecisely, so an
// information advantage has to be earned from speed, modelling, order-flow
// inference or inventory rather than from a clean feed.
func ScientificIndexDefaults(seed int64) *DegradedIndexConfig {
	return &DegradedIndexConfig{LagSamples: 5, NoiseBps: 10, Seed: seed}
}
