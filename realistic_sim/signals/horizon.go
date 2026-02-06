package signals

import (
	simmath "exchange_sim/realistic_sim/math"
)

type PriceHistory struct {
	buffer *simmath.CircularBuffer
	scale  int64
}

func NewPriceHistory(windowSize int, scale int64) *PriceHistory {
	return &PriceHistory{
		buffer: simmath.NewCircularBuffer(windowSize),
		scale:  scale,
	}
}

func (ph *PriceHistory) AddPrice(price int64) {
	ph.buffer.Add(price)
}

func (ph *PriceHistory) GetReturn() int64 {
	if !ph.buffer.IsFull() {
		return 0
	}

	oldest := ph.buffer.Get(0)
	newest := ph.buffer.Latest()

	if oldest == 0 {
		return 0
	}

	return simmath.LogReturn(newest, oldest, ph.scale)
}

func (ph *PriceHistory) IsReady() bool {
	return ph.buffer.IsFull()
}
