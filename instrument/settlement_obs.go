package instrument

import (
	"fmt"
	"sync"

	etypes "exchange_sim/types"
)

// settlementObserver accumulates underlying price samples inside a rolling
// window and freezes their mean as the settlement price on first read.
type settlementObserver struct {
	windowNano   int64
	mu           sync.Mutex
	obs          []settlementObs
	settled      bool
	settledPrice int64
	// lastDeclaredReference is a prior positive observation supplied through
	// ObserveSettlement by the contract's declared underlying-reference path;
	// it is never a trade, book-mid, or numeric-zero fallback.
	lastDeclaredReference    int64
	hasLastDeclaredReference bool
}

type settlementObs struct {
	price int64
	ts    int64
}

func (s *settlementObserver) observe(price, tsNano int64) {
	if price <= 0 {
		return
	}
	s.mu.Lock()
	s.lastDeclaredReference = price
	s.hasLastDeclaredReference = true
	s.obs = append(s.obs, settlementObs{price: price, ts: tsNano})
	cutoff := tsNano - s.windowNano
	trim := 0
	for trim < len(s.obs) && s.obs[trim].ts < cutoff {
		trim++
	}
	s.obs = s.obs[trim:]
	s.mu.Unlock()
}

// settlementPrice freezes and returns the window mean. When the configured
// window has rolled out, its only declared fallback is the last observation
// received through ObserveSettlement from that same underlying-reference
// contract. No observation is an unavailable settlement, not a zero price.
func (s *settlementObserver) settlementPrice() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled {
		return s.settledPrice, nil
	}
	n := int64(len(s.obs))
	if n == 0 {
		if !s.hasLastDeclaredReference || s.lastDeclaredReference <= 0 {
			return 0, fmt.Errorf("settlement observation: %w", etypes.ErrNoPrice)
		}
		s.settledPrice = s.lastDeclaredReference
		s.settled = true
		return s.settledPrice, nil
	}
	// Sum quotient and remainder separately so the mean stays exact for
	// prices near the int64 range.
	var sum, rem int64
	for _, o := range s.obs {
		sum += o.price / n
		rem += o.price % n
	}
	s.settledPrice = sum + rem/n
	if s.settledPrice <= 0 {
		return 0, fmt.Errorf("settlement observation: %w", etypes.ErrNoPrice)
	}
	s.settled = true
	return s.settledPrice, nil
}
