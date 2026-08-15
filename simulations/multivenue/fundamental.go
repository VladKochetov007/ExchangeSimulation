package multivenue

import (
	"math"
	"math/rand"
	"sync"
)

// FundamentalValue is the exogenous value of the traded asset.
//
// Without it the simulated market has no anchor: the only price setters
// estimate their own volatility from a midpoint they themselves produce, so any
// drift feeds back into a wider quote and diverges. A value process gives
// informed participants something to trade against, which is what actually
// holds a market together.
//
// The path is a driftless geometric random walk on a fixed step grid, drawn
// from a seeded generator. It is a function of simulated time alone, so every
// participant that consults it at the same timestamp sees the same value
// regardless of evaluation order.
type FundamentalValue struct {
	start          int64
	stepInterval   int64
	logVolPerStep  float64
	quotePrecision int64

	mu     sync.Mutex
	rng    *rand.Rand
	path   []int64 // path[i] is the value at step i
	startT int64
}

// NewFundamentalValue builds the process. stepInterval and startTimestamp are
// nanoseconds; logVolPerStep is the standard deviation of one step's log return.
func NewFundamentalValue(seed int64, startTimestamp, startPrice, stepInterval, quotePrecision int64, logVolPerStep float64) *FundamentalValue {
	if stepInterval <= 0 {
		stepInterval = int64(1e9)
	}
	return &FundamentalValue{
		start:          startPrice,
		stepInterval:   stepInterval,
		logVolPerStep:  logVolPerStep,
		quotePrecision: quotePrecision,
		rng:            rand.New(rand.NewSource(seed)),
		path:           []int64{startPrice},
		startT:         startTimestamp,
	}
}

// Value returns the fundamental value at a simulated timestamp, extending the
// path as needed. Steps are generated once and never redrawn, so the process is
// deterministic and independent of which participant asks first.
func (f *FundamentalValue) Value(timestamp int64) int64 {
	if f == nil {
		return 0
	}
	step := 0
	if timestamp > f.startT {
		step = int((timestamp - f.startT) / f.stepInterval)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.path) <= step {
		previous := f.path[len(f.path)-1]
		next := previous
		if f.logVolPerStep > 0 && previous > 0 {
			scaled := math.Exp(f.rng.NormFloat64() * f.logVolPerStep)
			if candidate, ok := scaleFixedPoint(previous, scaled); ok {
				next = candidate
			}
		}
		f.path = append(f.path, next)
	}
	return f.path[step]
}

// scaleFixedPoint multiplies a fixed-point price by a positive factor without
// leaving the representable range.
func scaleFixedPoint(price int64, factor float64) (int64, bool) {
	if !finite(factor) || factor <= 0 || price <= 0 {
		return 0, false
	}
	scaled := float64(price) * factor
	if !finite(scaled) || scaled < 1 || scaled > float64(math.MaxInt64/2) {
		return 0, false
	}
	return int64(scaled), true
}

// AlignedValue returns the fundamental value rounded to a tick.
func (f *FundamentalValue) AlignedValue(timestamp, tick int64) int64 {
	value := f.Value(timestamp)
	if tick <= 0 || value <= 0 {
		return value
	}
	return value / tick * tick
}
