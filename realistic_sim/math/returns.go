package math

// LogReturn computes ln(price_t / price_t_minus_1) * scale.
func LogReturn(price_t, price_t_minus_1, scale int64) int64 {
	if price_t_minus_1 == 0 {
		return 0
	}
	return IntegerLog(price_t, scale) - IntegerLog(price_t_minus_1, scale)
}
