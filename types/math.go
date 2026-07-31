package types

import (
	"math"
	"math/bits"
)

// MulDiv computes (a*b)/c exactly, truncating toward zero like native int64
// division, using a 128-bit intermediate product so qty×price never overflows
// regardless of the instrument's precision configuration. c must be positive
// (it is always a precision constant on money paths). Panics if the true
// quotient does not fit in int64 — an economically impossible input, not a
// rounding concern.
func MulDiv(a, b, c int64) int64 {
	if c <= 0 {
		// uint64(c) of a negative divisor would silently produce a huge
		// unsigned divisor and a wrong quotient; fail loudly instead.
		panic("MulDiv: divisor must be positive")
	}
	negative := (a < 0) != (b < 0)
	hi, lo := bits.Mul64(unsignedAbs(a), unsignedAbs(b))
	uc := uint64(c)
	if hi >= uc {
		panic("MulDiv: quotient overflows int64")
	}
	quo, _ := bits.Div64(hi, lo, uc)
	if negative {
		if quo > 1<<63 {
			panic("MulDiv: quotient overflows int64")
		}
		if quo == 1<<63 {
			return math.MinInt64
		}
		return -int64(quo)
	}
	if quo > math.MaxInt64 {
		panic("MulDiv: quotient overflows int64")
	}
	return int64(quo)
}

// unsignedAbs returns |x| as uint64, exact even for math.MinInt64.
func unsignedAbs(x int64) uint64 {
	if x < 0 {
		return -uint64(x)
	}
	return uint64(x)
}

// WeightedAverage returns (w1*v1 + w2*v2) / (w1 + w2) computed exactly in
// 128-bit intermediate arithmetic. All arguments must be non-negative and the
// weights must not both be zero.
//
// Position entry prices are the motivating case: size × price reaches ~5e17
// at realistic precisions, well past float64's 53-bit mantissa, so averaging
// in float64 quantizes the result to ~64 units. That error lands directly in
// realized PnL — money the exchange invents or destroys — because each
// account's basis drifts independently of its counterparties'.
func WeightedAverage(w1, v1, w2, v2 int64) int64 {
	if w1 < 0 || v1 < 0 || w2 < 0 || v2 < 0 {
		panic("WeightedAverage: arguments must be non-negative")
	}
	total := uint64(w1) + uint64(w2)
	if total == 0 {
		panic("WeightedAverage: weights sum to zero")
	}
	hi1, lo1 := bits.Mul64(uint64(w1), uint64(v1))
	hi2, lo2 := bits.Mul64(uint64(w2), uint64(v2))
	lo, carry := bits.Add64(lo1, lo2, 0)
	hi, _ := bits.Add64(hi1, hi2, carry)
	if hi >= total {
		panic("WeightedAverage: quotient overflows int64")
	}
	quo, _ := bits.Div64(hi, lo, total)
	if quo > math.MaxInt64 {
		panic("WeightedAverage: quotient overflows int64")
	}
	return int64(quo)
}
