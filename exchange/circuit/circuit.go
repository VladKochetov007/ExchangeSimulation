package circuit

import (
	"time"

	ebook "exchange_sim/exchange/book"
	ematching "exchange_sim/exchange/matching"
	etypes "exchange_sim/exchange/types"
)

type CBAction uint8

const (
	CBAllow  CBAction = iota
	CBReject          // order rejected; symbol stays open
	CBHalt            // order rejected; symbol halted
)

// CBResult is returned by CircuitBreaker.Check.
// When Action == CBHalt, HaltDuration == 0 means indefinite halt.
type CBResult struct {
	Action       CBAction
	HaltDuration time.Duration
}

// CircuitBreaker is the pre-trade layer.
// lastPrice is the symbol's last traded price; bestOpposing is the best price
// on the side the incoming order would match against.
type CircuitBreaker interface {
	Check(symbol string, lastPrice, bestOpposing int64, side etypes.Side) CBResult
}

// HaltEvaluator is the post-match layer.
// Called after every match with the actual executions; return halt=true to
// pause the symbol for the given duration (0 = indefinite).
type HaltEvaluator interface {
	Evaluate(symbol string, execs []*etypes.Execution) (halt bool, duration time.Duration)
}

// scaleMax guards against int64 overflow: diff*10000 overflows when diff > scaleMax.
const scaleMax = (1<<63 - 1) / 10000

// bandExceeded reports whether abs(price-ref)/ref > bandBps/10000.
// Falls back to float64 when ref is large enough to risk overflow.
func bandExceeded(ref, price, bandBps int64) bool {
	diff := price - ref
	if diff < 0 {
		diff = -diff
	}
	if ref > scaleMax {
		return float64(diff)*10000 > float64(ref)*float64(bandBps)
	}
	return diff*10000 > ref*bandBps
}

// PercentBandCircuitBreaker rejects orders whose best opposing price deviates
// more than BandBps basis points from the last traded price (symmetric).
//
//	BandBps 500  → ±5%   (NASDAQ LULD style)
//	BandBps 1000 → ±10%  (wider volatile-instrument band)
type PercentBandCircuitBreaker struct {
	BandBps int64
}

func (cb *PercentBandCircuitBreaker) Check(_ string, lastPrice, bestOpposing int64, _ etypes.Side) CBResult {
	if lastPrice == 0 || !bandExceeded(lastPrice, bestOpposing, cb.BandBps) {
		return CBResult{Action: CBAllow}
	}
	return CBResult{Action: CBReject}
}

// AsymmetricBandCircuitBreaker applies independent up/down bands.
type AsymmetricBandCircuitBreaker struct {
	UpBandBps   int64
	DownBandBps int64
}

func (cb *AsymmetricBandCircuitBreaker) Check(_ string, lastPrice, bestOpposing int64, side etypes.Side) CBResult {
	if lastPrice == 0 {
		return CBResult{Action: CBAllow}
	}
	if side == etypes.Buy && bestOpposing > lastPrice && bandExceeded(lastPrice, bestOpposing, cb.UpBandBps) {
		return CBResult{Action: CBReject}
	}
	if side == etypes.Sell && bestOpposing < lastPrice && bandExceeded(lastPrice, bestOpposing, cb.DownBandBps) {
		return CBResult{Action: CBReject}
	}
	return CBResult{Action: CBAllow}
}

// BreakerTier defines a price-move threshold and the action to take when
// that threshold is breached. Tiers must be sorted ascending by ThresholdBps.
type BreakerTier struct {
	ThresholdBps int64
	Action       CBAction
	HaltDuration time.Duration // only relevant when Action == CBHalt; 0 = indefinite
}

// TieredCircuitBreaker applies escalating actions as price moves grow.
// Modelled after CME three-level circuit breakers and Chinese A-share daily limits.
type TieredCircuitBreaker struct {
	Tiers     []BreakerTier
	refPrices map[string]int64
}

func NewTieredCircuitBreaker(tiers []BreakerTier) *TieredCircuitBreaker {
	return &TieredCircuitBreaker{Tiers: tiers, refPrices: make(map[string]int64)}
}

func (cb *TieredCircuitBreaker) SetRefPrice(symbol string, price int64) {
	cb.refPrices[symbol] = price
}

func (cb *TieredCircuitBreaker) Check(symbol string, _ int64, bestOpposing int64, _ etypes.Side) CBResult {
	ref := cb.refPrices[symbol]
	if ref == 0 {
		return CBResult{Action: CBAllow}
	}
	for i := len(cb.Tiers) - 1; i >= 0; i-- {
		t := cb.Tiers[i]
		if bandExceeded(ref, bestOpposing, t.ThresholdBps) {
			return CBResult{Action: t.Action, HaltDuration: t.HaltDuration}
		}
	}
	return CBResult{Action: CBAllow}
}

// TieredHaltEvaluator is the post-match counterpart of TieredCircuitBreaker.
type TieredHaltEvaluator struct {
	Tiers     []BreakerTier
	refPrices map[string]int64
}

func NewTieredHaltEvaluator(tiers []BreakerTier) *TieredHaltEvaluator {
	return &TieredHaltEvaluator{Tiers: tiers, refPrices: make(map[string]int64)}
}

func (e *TieredHaltEvaluator) SetRefPrice(symbol string, price int64) {
	e.refPrices[symbol] = price
}

func (e *TieredHaltEvaluator) Evaluate(symbol string, execs []*etypes.Execution) (bool, time.Duration) {
	ref := e.refPrices[symbol]
	if ref == 0 {
		return false, 0
	}
	for i := len(e.Tiers) - 1; i >= 0; i-- {
		t := e.Tiers[i]
		for _, exec := range execs {
			if bandExceeded(ref, exec.Price, t.ThresholdBps) {
				return true, t.HaltDuration
			}
		}
	}
	return false, 0
}

// CompositeCircuitBreaker delegates to multiple breakers and returns the most
// restrictive result.
type CompositeCircuitBreaker struct {
	Breakers []CircuitBreaker
}

func (c *CompositeCircuitBreaker) Check(symbol string, lastPrice, bestOpposing int64, side etypes.Side) CBResult {
	result := CBResult{Action: CBAllow}
	for _, b := range c.Breakers {
		result = moreRestrictive(result, b.Check(symbol, lastPrice, bestOpposing, side))
	}
	return result
}

func moreRestrictive(a, b CBResult) CBResult {
	if b.Action > a.Action {
		return b
	}
	if a.Action > b.Action {
		return a
	}
	if a.Action != CBHalt {
		return a
	}
	// Both CBHalt — 0 (indefinite) dominates; otherwise longer duration wins.
	if a.HaltDuration == 0 {
		return a
	}
	if b.HaltDuration == 0 || b.HaltDuration > a.HaltDuration {
		return b
	}
	return a
}

// HaltForever encodes an indefinite halt in the HaltUntil map.
const HaltForever = int64(-1)

// CircuitBreakerMatcher wraps an inner MatchingEngine with a pre-trade
// CircuitBreaker and an optional post-match HaltEvaluator.
type CircuitBreakerMatcher struct {
	Inner         ematching.MatchingEngine
	Breaker       CircuitBreaker
	books         map[string]*ebook.OrderBook
	clock         etypes.Clock
	HaltUntil     map[string]int64
	reverseIdx    map[*ebook.Book]string
	haltEvaluator HaltEvaluator
}

func NewCircuitBreakerMatcher(inner ematching.MatchingEngine, breaker CircuitBreaker, books map[string]*ebook.OrderBook, clock etypes.Clock) *CircuitBreakerMatcher {
	return &CircuitBreakerMatcher{
		Inner:      inner,
		Breaker:    breaker,
		books:      books,
		clock:      clock,
		HaltUntil:  make(map[string]int64),
		reverseIdx: make(map[*ebook.Book]string),
	}
}

func (m *CircuitBreakerMatcher) SetHaltEvaluator(he HaltEvaluator) {
	m.haltEvaluator = he
}

func (m *CircuitBreakerMatcher) IsHalted(symbol string) bool {
	return m.isHaltedAt(symbol, m.clock.NowUnixNano())
}

func (m *CircuitBreakerMatcher) ClearHalt(symbol string) {
	delete(m.HaltUntil, symbol)
}

func (m *CircuitBreakerMatcher) Match(bidBook, askBook *ebook.Book, order *etypes.Order) *ematching.MatchResult {
	sym, known := m.symbolFor(bidBook)

	if known {
		if m.isHaltedAt(sym, m.clock.NowUnixNano()) {
			return &ematching.MatchResult{}
		}

		ob := m.books[sym]
		lastPrice := ob.GetLastPrice()

		var opposing *ebook.Book
		if order.Side == etypes.Buy {
			opposing = askBook
		} else {
			opposing = bidBook
		}

		if lastPrice != 0 && opposing.Best != nil {
			res := m.Breaker.Check(sym, lastPrice, opposing.Best.Price, order.Side)
			switch res.Action {
			case CBReject:
				return &ematching.MatchResult{}
			case CBHalt:
				m.setHalt(sym, res.HaltDuration)
				return &ematching.MatchResult{}
			}
		}
	}

	result := m.Inner.Match(bidBook, askBook, order)

	if known && m.haltEvaluator != nil && len(result.Executions) > 0 {
		if halt, dur := m.haltEvaluator.Evaluate(sym, result.Executions); halt {
			m.setHalt(sym, dur)
		}
	}

	return result
}

func (m *CircuitBreakerMatcher) symbolFor(bidBook *ebook.Book) (string, bool) {
	if len(m.reverseIdx) != len(m.books) {
		m.reverseIdx = make(map[*ebook.Book]string, len(m.books))
		for sym, ob := range m.books {
			m.reverseIdx[ob.Bids] = sym
		}
	}
	sym, ok := m.reverseIdx[bidBook]
	return sym, ok
}

func (m *CircuitBreakerMatcher) isHaltedAt(sym string, now int64) bool {
	until, ok := m.HaltUntil[sym]
	if !ok {
		return false
	}
	if until == HaltForever {
		return true
	}
	return now < until
}

func (m *CircuitBreakerMatcher) setHalt(sym string, dur time.Duration) {
	if dur == 0 {
		m.HaltUntil[sym] = HaltForever
	} else {
		m.HaltUntil[sym] = m.clock.NowUnixNano() + dur.Nanoseconds()
	}
}
