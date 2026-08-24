package analysis

import "math/big"

// mulDiv computes a*b/c in 128-bit intermediate precision, truncating toward
// zero, which is what the exchange itself does.
//
// The naive a*b/c overflows int64 for perfectly ordinary inputs here: a price
// difference of a few billion times a position of a few billion is 1e19,
// against an int64 ceiling of 9.2e18. An audit that overflows reports the
// venue as wrong and is itself the thing that is wrong, which is how this
// function came to exist.
func mulDiv(a, b, c int64) (int64, bool) {
	if c == 0 {
		return 0, false
	}
	var product, divisor, quotient big.Int
	product.Mul(big.NewInt(a), big.NewInt(b))
	quotient.Quo(&product, divisor.SetInt64(c))
	if !quotient.IsInt64() {
		return 0, false
	}
	return quotient.Int64(), true
}
