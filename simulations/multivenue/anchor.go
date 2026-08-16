package multivenue

import (
	"sort"
	"sync"
)

// spotIndexProvider publishes the reference price a venue advertises on its
// public index feed.
//
// It deliberately never reads a venue's book directly. The exchange calls its
// price source while holding its own lock, so reaching into another venue would
// risk a lock cycle; and the values it needs are already cached by each venue's
// automation tick.
type spotIndexProvider struct {
	mode string
	// symbols are all the instruments the venue publishes this reference for.
	// The perpetual needs it as much as the spot book: without a reference of
	// its own it can only mirror spot, and a mirrored perpetual has no basis.
	symbols map[string]struct{}

	mu        sync.RWMutex
	venueMids map[string]int64
	value     int64
}

func newSpotIndexProvider(mode string, symbols ...string) *spotIndexProvider {
	set := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		set[symbol] = struct{}{}
	}
	return &spotIndexProvider{mode: mode, symbols: set, venueMids: make(map[string]int64)}
}

// observeVenueMid records one venue's current midpoint.
func (p *spotIndexProvider) observeVenueMid(venueID string, mid int64) {
	if p == nil || mid <= 0 {
		return
	}
	p.mu.Lock()
	p.venueMids[venueID] = mid
	p.mu.Unlock()
}

// observeFundamental records the exogenous value, used only by the idealized
// fundamental anchor.
func (p *spotIndexProvider) observeFundamental(value int64) {
	if p == nil || value <= 0 {
		return
	}
	p.mu.Lock()
	p.value = value
	p.mu.Unlock()
}

// Price returns the published index for a symbol, or zero when the venue should
// publish nothing.
func (p *spotIndexProvider) Price(symbol string) int64 {
	if p == nil {
		return 0
	}
	if _, published := p.symbols[symbol]; !published {
		return 0
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch p.mode {
	case "fundamental":
		return p.value
	case "consensus":
		mids := make([]int64, 0, len(p.venueMids))
		for _, mid := range p.venueMids {
			if mid > 0 {
				mids = append(mids, mid)
			}
		}
		if len(mids) == 0 {
			return 0
		}
		// Median rather than mean: one venue that has run away should not drag
		// the reference the others are quoting around.
		sort.Slice(mids, func(i, j int) bool { return mids[i] < mids[j] })
		return mids[len(mids)/2]
	}
	return 0
}
