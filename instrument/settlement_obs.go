package instrument

import "sync"

// settlementObserver accumulates underlying price samples inside a rolling
// window and freezes their mean as the settlement price on first read.
type settlementObserver struct {
	windowNano   int64
	mu           sync.Mutex
	obs          []settlementObs
	settled      int64
	lastObserved int64
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
	s.lastObserved = price
	s.obs = append(s.obs, settlementObs{price: price, ts: tsNano})
	cutoff := tsNano - s.windowNano
	trim := 0
	for trim < len(s.obs) && s.obs[trim].ts < cutoff {
		trim++
	}
	s.obs = s.obs[trim:]
	s.mu.Unlock()
}

// settlementPrice freezes and returns the window mean; fallback is used when
// no samples were recorded (e.g. the instrument never traded a full window).
func (s *settlementObserver) settlementPrice(fallback int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settled != 0 {
		return s.settled
	}
	n := int64(len(s.obs))
	if n == 0 {
		s.settled = s.lastObserved
		if s.settled == 0 {
			s.settled = fallback
		}
		return s.settled
	}
	// Sum quotient and remainder separately so the mean stays exact for
	// prices near the int64 range.
	var sum, rem int64
	for _, o := range s.obs {
		sum += o.price / n
		rem += o.price % n
	}
	s.settled = sum + rem/n
	return s.settled
}
