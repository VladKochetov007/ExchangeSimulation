package types

// MulDiv computes (a*b)/c without overflowing the intermediate product.
// Exact for |a%c| * |b| < 2^63; truncates toward zero like native division.
// Money-path convention: MulDiv(qty, price, precision) — qty%precision stays
// below base precision, keeping the partial product within int64 range.
func MulDiv(a, b, c int64) int64 {
	return (a/c)*b + (a%c)*b/c
}
