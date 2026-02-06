package math

import (
	"math"
)

type CircularBuffer struct {
	data  []int64
	size  int
	write int
	count int
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data: make([]int64, size),
		size: size,
	}
}

func (cb *CircularBuffer) Add(val int64) {
	cb.data[cb.write] = val
	cb.write = (cb.write + 1) % cb.size
	if cb.count < cb.size {
		cb.count++
	}
}

func (cb *CircularBuffer) Count() int {
	return cb.count
}

func (cb *CircularBuffer) IsFull() bool {
	return cb.count == cb.size
}

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

func (cb *CircularBuffer) Get(index int) int64 {
	if index < 0 || index >= cb.count {
		return 0
	}
	actualIndex := (cb.write - cb.count + index + cb.size) % cb.size
	return cb.data[actualIndex]
}

func (cb *CircularBuffer) Latest() int64 {
	if cb.count == 0 {
		return 0
	}
	return cb.Get(cb.count - 1)
}

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
