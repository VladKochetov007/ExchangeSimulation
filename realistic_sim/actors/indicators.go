package actors

import (
	"math"
)

// CircularBuffer implements a fixed-size circular buffer for efficient rolling window calculations.
type CircularBuffer struct {
	data  []int64
	size  int
	write int
	count int
}

// NewCircularBuffer creates a new circular buffer with the specified size.
func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data: make([]int64, size),
		size: size,
	}
}

// Add adds a value to the circular buffer.
func (cb *CircularBuffer) Add(val int64) {
	cb.data[cb.write] = val
	cb.write = (cb.write + 1) % cb.size
	if cb.count < cb.size {
		cb.count++
	}
}

// Count returns the number of elements currently in the buffer.
func (cb *CircularBuffer) Count() int {
	return cb.count
}

// IsFull returns true if the buffer is at capacity.
func (cb *CircularBuffer) IsFull() bool {
	return cb.count == cb.size
}

// SMA calculates the simple moving average of all values in the buffer.
func (cb *CircularBuffer) SMA() int64 {
	if cb.count == 0 {
		return 0
	}
	sum := int64(0)
	for i := 0; i < cb.count; i++ {
		sum += cb.data[i]
	}
	return sum / int64(cb.count)
}

// Get returns the value at the specified index (0 = oldest, count-1 = newest).
func (cb *CircularBuffer) Get(index int) int64 {
	if index < 0 || index >= cb.count {
		return 0
	}
	// Calculate actual index in circular buffer
	actualIndex := (cb.write - cb.count + index + cb.size) % cb.size
	return cb.data[actualIndex]
}

// Latest returns the most recently added value.
func (cb *CircularBuffer) Latest() int64 {
	if cb.count == 0 {
		return 0
	}
	return cb.Get(cb.count - 1)
}

// StdDev calculates the standard deviation of all values in the buffer.
// The mean parameter should be the result of SMA() to avoid recalculating it.
func (cb *CircularBuffer) StdDev(mean int64) int64 {
	if cb.count == 0 {
		return 0
	}

	sumSquaredDiff := int64(0)
	for i := 0; i < cb.count; i++ {
		diff := cb.data[i] - mean
		sumSquaredDiff += diff * diff
	}

	variance := sumSquaredDiff / int64(cb.count)
	return int64(math.Sqrt(float64(variance)))
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
