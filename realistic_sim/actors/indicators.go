package actors

import (
	simmath "exchange_sim/realistic_sim/math"
)

type CircularBuffer = simmath.CircularBuffer

func NewCircularBuffer(size int) *CircularBuffer {
	return simmath.NewCircularBuffer(size)
}

// ZScore calculates the z-score of a value given the mean and standard deviation.
// All values are scaled integers (e.g., prices in precision units).
// Returns z-score * 10000 (so z-score of 2.5 returns 25000).
func ZScore(value, mean, stdDev int64) int64 {
	if stdDev == 0 {
		return 0
	}
	diff := value - mean
	// Scale by 10000 to preserve precision
	return (diff * 10000) / stdDev
}

// CalculateRatio calculates the ratio of two prices with scaling.
// Returns (price1 * 10000) / price2 to maintain precision.
func CalculateRatio(price1, price2 int64) int64 {
	if price2 == 0 {
		return 0
	}
	return (price1 * 10000) / price2
}

// BPSChange calculates the percentage change in basis points (1 bps = 0.01%).
// Returns ((newPrice - oldPrice) * 10000) / oldPrice
func BPSChange(oldPrice, newPrice int64) int64 {
	if oldPrice == 0 {
		return 0
	}
	return ((newPrice - oldPrice) * 10000) / oldPrice
}
