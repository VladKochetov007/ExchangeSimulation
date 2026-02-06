package signals

import (
	"time"

	simmath "exchange_sim/realistic_sim/math"
)

type PricePoint struct {
	Timestamp int64
	Price     int64
}

type PriceHistory struct {
	points         []PricePoint
	lookbackPeriod time.Duration
	scale          int64
	maxPoints      int
}

func NewPriceHistory(lookbackPeriod time.Duration, scale int64) *PriceHistory {
	maxPoints := 1000
	return &PriceHistory{
		points:         make([]PricePoint, 0, maxPoints),
		lookbackPeriod: lookbackPeriod,
		scale:          scale,
		maxPoints:      maxPoints,
	}
}

func (ph *PriceHistory) AddPrice(price int64, timestamp int64) {
	ph.points = append(ph.points, PricePoint{
		Timestamp: timestamp,
		Price:     price,
	})

	if len(ph.points) > ph.maxPoints {
		excess := len(ph.points) - ph.maxPoints
		ph.points = ph.points[excess:]
	}
}

func (ph *PriceHistory) GetReturn(currentTime int64) int64 {
	if len(ph.points) == 0 {
		return 0
	}

	cutoff := currentTime - int64(ph.lookbackPeriod/time.Nanosecond)

	oldestIdx := -1
	for i := 0; i < len(ph.points); i++ {
		if ph.points[i].Timestamp >= cutoff {
			oldestIdx = i
			break
		}
	}

	if oldestIdx == -1 {
		return 0
	}

	oldestPrice := ph.points[oldestIdx].Price
	newestPrice := ph.points[len(ph.points)-1].Price

	if oldestPrice == 0 {
		return 0
	}

	ret := simmath.LogReturn(newestPrice, oldestPrice, ph.scale)
	return ret
}

func (ph *PriceHistory) IsReady(currentTime int64) bool {
	if len(ph.points) < 2 {
		return false
	}

	oldestTime := ph.points[0].Timestamp
	timeSpan := currentTime - oldestTime

	return timeSpan >= int64(ph.lookbackPeriod/time.Nanosecond)
}
