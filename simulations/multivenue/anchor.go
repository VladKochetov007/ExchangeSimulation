package multivenue

import (
	"fmt"
	"sort"
	"sync"

	etypes "exchange_sim/types"
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

	mu sync.RWMutex
	// venueMids is keyed by symbol and then venue, because every published
	// symbol needs its own consensus: a market with no reference of its own
	// falls back to its book midpoint and becomes self-referential.
	venueMids map[string]map[string]int64
	value     int64
}

func newSpotIndexProvider(mode string, symbols ...string) *spotIndexProvider {
	set := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		set[symbol] = struct{}{}
	}
	return &spotIndexProvider{mode: mode, symbols: set, venueMids: make(map[string]map[string]int64)}
}

// observeVenueMid records one venue's current midpoint for a symbol.
func (p *spotIndexProvider) observeVenueMid(symbol, venueID string, mid int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.venueMids[symbol] == nil {
		p.venueMids[symbol] = make(map[string]int64)
	}
	p.venueMids[symbol][venueID] = mid
	p.mu.Unlock()
}

// Price returns the published index for a symbol. Unpublished symbols and a
// consensus with no observations are explicitly unavailable.
func (p *spotIndexProvider) Price(symbol string) (int64, error) {
	if p == nil {
		return 0, fmt.Errorf("spot index provider is nil: %w", etypes.ErrNoPrice)
	}
	if _, published := p.symbols[symbol]; !published {
		return 0, fmt.Errorf("spot index does not publish %s: %w", symbol, etypes.ErrNoPrice)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	switch p.mode {
	case "consensus":
		mids := make([]int64, 0, len(p.venueMids[symbol]))
		for _, mid := range p.venueMids[symbol] {
			// Map membership is the receipt/presence state. A zero or negative
			// midpoint is present evidence and must reach the consumer, which
			// can then apply its own positive-index or signed-price domain.
			mids = append(mids, mid)
		}
		if len(mids) == 0 {
			return 0, fmt.Errorf("spot index has no observations for %s: %w", symbol, etypes.ErrNoPrice)
		}
		// Median rather than mean: one venue that has run away should not drag
		// the reference the others are quoting around.
		sort.Slice(mids, func(i, j int) bool { return mids[i] < mids[j] })
		return mids[len(mids)/2], nil
	}
	return 0, fmt.Errorf("unknown spot-index mode %q: %w", p.mode, etypes.ErrNoPrice)
}
