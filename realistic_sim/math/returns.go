package math

import "math"

// LogReturn computes ln(price_t / price_t_minus_1) * scale.
func LogReturn(price_t, price_t_minus_1, scale int64) int64 {
	if price_t_minus_1 == 0 {
		return 0
	}
	ratio := float64(price_t) / float64(price_t_minus_1)
	return int64(math.Log(ratio) * float64(scale))
}
