package analysis

import "math/bits"

// mulDiv computes a*b/c in 128-bit intermediate precision, truncating toward
// zero, which is what the exchange itself does.
//
// The naive a*b/c overflows int64 for perfectly ordinary inputs here: a price
// difference of a few billion times a position of a few billion is 1e19,
// against an int64 ceiling of 9.2e18. An audit that overflows reports the
// venue as wrong and is itself the thing that is wrong, which is how this
// function came to exist.
func mulDiv(a, b, c int64) int64 {
	if c == 0 {
		return 0
	}
	negative := (a < 0) != (b < 0)
	if c < 0 {
		negative = !negative
		c = -c
	}
	hi, lo := bits.Mul64(absUint(a), absUint(b))
	divisor := uint64(c)
	if hi >= divisor {
		// The quotient does not fit. Saturating is a lie; report zero and let
		// the caller's residual make the problem visible instead.
		return 0
	}
	quotient, _ := bits.Div64(hi, lo, divisor)
	result := int64(quotient)
	if negative {
		return -result
	}
	return result
}

func absUint(value int64) uint64 {
	if value < 0 {
		return uint64(-value)
	}
	return uint64(value)
}
