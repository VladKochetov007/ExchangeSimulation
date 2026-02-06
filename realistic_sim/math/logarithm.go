package math

const (
	LN2_SCALED = 6931 // ln(2) * 10000
)

// IntegerLog computes ln(x) * scale using Taylor series approximation.
// Returns scaled logarithm, e.g., IntegerLog(100, 10000) ≈ 46052 (ln(100) ≈ 4.6052).
func IntegerLog(x int64, scale int64) int64 {
	if x <= 0 {
		return 0
	}
	if x == scale {
		return 0
	}

	k := int64(0)
	xScaled := x

	for xScaled >= 2*scale {
		xScaled /= 2
		k++
	}
	for xScaled < scale {
		xScaled *= 2
		k--
	}

	y := xScaled - scale
	y2 := (y * y) / scale
	y3 := (y2 * y) / scale
	y4 := (y3 * y) / scale
	y5 := (y4 * y) / scale

	lnF := y - y2/2 + y3/3 - y4/4 + y5/5

	return k*LN2_SCALED + lnF
}
