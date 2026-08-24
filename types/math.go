package types

import (
	"math"
	"math/bits"
)

func TryAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func TrySub(a, b int64) (int64, bool) {
	if b == math.MinInt64 {
		return 0, false
	}
	return TryAdd(a, -b)
}

// MulDiv computes (a*b)/c exactly, truncating toward zero like native int64
// division, using a 128-bit intermediate product so qty×price never overflows
// regardless of the instrument's precision configuration. c must be positive
// (it is always a precision constant on money paths). Panics if the true
// quotient does not fit in int64 — an economically impossible input, not a
// rounding concern.
func MulDiv(a, b, c int64) int64 {
	if result, ok := TryMulDiv(a, b, c); ok {
		return result
	}
	panic("MulDiv: quotient overflows int64")
}

// TryMulDiv computes (a*b)/c like MulDiv but reports unrepresentable results
// instead of panicking. Admission paths use it to reject hostile order sizes
// and prices without letting a client crash the venue.
func TryMulDiv(a, b, c int64) (int64, bool) {
	if c <= 0 {
		return 0, false
	}
	negative := (a < 0) != (b < 0)
	hi, lo := bits.Mul64(unsignedAbs(a), unsignedAbs(b))
	uc := uint64(c)
	if hi >= uc {
		return 0, false
	}
	quo, _ := bits.Div64(hi, lo, uc)
	if negative {
		if quo > 1<<63 {
			return 0, false
		}
		if quo == 1<<63 {
			return math.MinInt64, true
		}
		return -int64(quo), true
	}
	if quo > math.MaxInt64 {
		return 0, false
	}
	return int64(quo), true
}

// TryAbsMulDiv computes |a|×|b|/c with a non-negative result, using a
// 128-bit intermediate product. It is for explicitly magnitude-based
// quantities such as futures risk notional and delivery-fee bases; it must not
// be used to turn an unavailable price into a numeric value.
func TryAbsMulDiv(a, b, c int64) (int64, bool) {
	if c <= 0 {
		return 0, false
	}
	hi, lo := bits.Mul64(unsignedAbs(a), unsignedAbs(b))
	uc := uint64(c)
	if hi >= uc {
		return 0, false
	}
	quo, _ := bits.Div64(hi, lo, uc)
	if quo > math.MaxInt64 {
		return 0, false
	}
	return int64(quo), true
}

// AbsMulDiv is TryAbsMulDiv for invariant paths whose inputs have already
// passed venue admission. It panics when the magnitude is unrepresentable.
func AbsMulDiv(a, b, c int64) int64 {
	if result, ok := TryAbsMulDiv(a, b, c); ok {
		return result
	}
	panic("AbsMulDiv: quotient overflows int64")
}

// TryPriceChangeMulDiv computes qty×(toPrice-fromPrice)/precision exactly,
// truncating toward zero. It never forms toPrice-fromPrice as int64: the
// signed difference can span the full uint64 range even though each endpoint
// is an int64. This is the futures PnL primitive for signed settlement prices.
func TryPriceChangeMulDiv(qty, toPrice, fromPrice, precision int64) (int64, bool) {
	if precision <= 0 {
		return 0, false
	}
	differenceNegative := toPrice < fromPrice
	var differenceMagnitude uint64
	if differenceNegative {
		differenceMagnitude = uint64(fromPrice) - uint64(toPrice)
	} else {
		differenceMagnitude = uint64(toPrice) - uint64(fromPrice)
	}
	negative := (qty < 0) != differenceNegative
	hi, lo := bits.Mul64(unsignedAbs(qty), differenceMagnitude)
	uc := uint64(precision)
	if hi >= uc {
		return 0, false
	}
	quo, _ := bits.Div64(hi, lo, uc)
	if negative {
		if quo > 1<<63 {
			return 0, false
		}
		if quo == 1<<63 {
			return math.MinInt64, true
		}
		return -int64(quo), true
	}
	if quo > math.MaxInt64 {
		return 0, false
	}
	return int64(quo), true
}

// PriceChangeMulDiv is TryPriceChangeMulDiv for invariant paths whose inputs
// have already passed venue admission. It panics if the final cash flow cannot
// be represented in int64.
func PriceChangeMulDiv(qty, toPrice, fromPrice, precision int64) int64 {
	if result, ok := TryPriceChangeMulDiv(qty, toPrice, fromPrice, precision); ok {
		return result
	}
	panic("PriceChangeMulDiv: quotient overflows int64")
}

// TryMulBps computes value*bps/10000 without an intermediate int64 overflow.
func TryMulBps(value, bps int64) (int64, bool) {
	return TryMulDiv(value, bps, 10000)
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

// AddAmount adds two amounts and panics if the result is not representable.
//
// Money is held in int64 minor units and Go wraps silently on overflow: a
// balance one unit past the ceiling becomes a large negative number, which
// reads as a debt rather than as an error. Paths where a wrap is a broken
// invariant rather than a hostile input use this, so the failure surfaces
// where it happens instead of as a sign flip in a later audit.
func AddAmount(a, b int64) int64 {
	sum, ok := TryAdd(a, b)
	if !ok {
		panic("AddAmount: sum overflows int64")
	}
	return sum
}
