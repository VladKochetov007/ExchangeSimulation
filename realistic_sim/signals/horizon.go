package signals

import (
	"exchange_sim/realistic_sim/actors"
	simmath "exchange_sim/realistic_sim/math"
	"sync"
	"time"
)

type TimeWindow struct {
	Duration     time.Duration
	Buffer       *actors.CircularBuffer
	lastUpdate   time.Time
	samplePeriod time.Duration
}

type HorizonTracker struct {
	windows       map[string]map[time.Duration]*TimeWindow
	horizons      []time.Duration
	windowSizes   map[time.Duration]int
	samplePeriods map[time.Duration]time.Duration
	scale         int64
	mu            sync.RWMutex
}

func NewHorizonTracker(horizons []time.Duration, scale int64) *HorizonTracker {
	windowSizes := map[time.Duration]int{
		30 * time.Second: 60,
		3 * time.Minute:  60,
		10 * time.Minute: 60,
		1 * time.Hour:    60,
	}

	samplePeriods := map[time.Duration]time.Duration{
		30 * time.Second: 500 * time.Millisecond,
		3 * time.Minute:  3 * time.Second,
		10 * time.Minute: 10 * time.Second,
		1 * time.Hour:    1 * time.Minute,
	}

	return &HorizonTracker{
		windows:       make(map[string]map[time.Duration]*TimeWindow),
		horizons:      horizons,
		windowSizes:   windowSizes,
		samplePeriods: samplePeriods,
		scale:         scale,
	}
}

func (ht *HorizonTracker) AddPrice(symbol string, price int64, timestamp time.Time) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	if ht.windows[symbol] == nil {
		ht.windows[symbol] = make(map[time.Duration]*TimeWindow)
		for _, horizon := range ht.horizons {
			windowSize := ht.windowSizes[horizon]
			if windowSize == 0 {
				windowSize = 60
			}
			samplePeriod := ht.samplePeriods[horizon]
			if samplePeriod == 0 {
				samplePeriod = horizon / 60
			}
			ht.windows[symbol][horizon] = &TimeWindow{
				Duration:     horizon,
				Buffer:       actors.NewCircularBuffer(windowSize),
				samplePeriod: samplePeriod,
			}
		}
	}

	for _, tw := range ht.windows[symbol] {
		if timestamp.Sub(tw.lastUpdate) >= tw.samplePeriod {
			tw.Buffer.Add(price)
			tw.lastUpdate = timestamp
		}
	}
}

func (ht *HorizonTracker) GetReturn(symbol string, horizon time.Duration) int64 {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	if ht.windows[symbol] == nil {
		return 0
	}

	tw := ht.windows[symbol][horizon]
	if tw == nil || !tw.Buffer.IsFull() {
		return 0
	}

	oldest := tw.Buffer.Get(0)
	newest := tw.Buffer.Latest()

	if oldest == 0 {
		return 0
	}

	return simmath.LogReturn(newest, oldest, ht.scale)
}

func (ht *HorizonTracker) IsReady(symbol string, horizon time.Duration) bool {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	if ht.windows[symbol] == nil {
		return false
	}

	tw := ht.windows[symbol][horizon]
	return tw != nil && tw.Buffer.IsFull()
}
