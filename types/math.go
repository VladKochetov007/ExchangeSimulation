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
