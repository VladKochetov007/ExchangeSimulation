package analysis

import (
	"math"
	"sort"
)

// Distribution summarises a sample without keeping the caller's copy of it.
type Distribution struct {
	N      int
	Median float64
	P75    float64
	P90    float64
	P99    float64
	Max    float64
	Mean   float64
}

// Describe sorts the sample in place and summarises it.
func Describe(sample []float64) Distribution {
	if len(sample) == 0 {
		return Distribution{}
	}
	sort.Float64s(sample)
	sum := 0.0
	for _, value := range sample {
		sum += value
	}
	return Distribution{
		N:      len(sample),
		Median: quantile(sample, 0.5),
		P75:    quantile(sample, 0.75),
		P90:    quantile(sample, 0.90),
		P99:    quantile(sample, 0.99),
		Max:    sample[len(sample)-1],
		Mean:   sum / float64(len(sample)),
	}
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	index := int(q * float64(len(sorted)))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// Abs returns the absolute values of a sample, leaving the input untouched.
func Abs(sample []float64) []float64 {
	out := make([]float64, len(sample))
	for i, value := range sample {
		out[i] = math.Abs(value)
	}
	return out
}
