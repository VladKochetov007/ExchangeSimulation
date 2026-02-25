package exchange

import "time"

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
// Return CBReject to drop the order silently, or CBHalt to drop it and pause
// the symbol.
type CircuitBreaker interface {
	Check(symbol string, lastPrice, bestOpposing int64, side Side) CBResult
}

// HaltEvaluator is the post-match layer.
// Called after every match with the actual executions; return halt=true to
// pause the symbol for the given duration (0 = indefinite).
type HaltEvaluator interface {
	Evaluate(symbol string, execs []*Execution) (halt bool, duration time.Duration)
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
// Zero lastPrice disables the check (cold start).
//
//	BandBps 500  → ±5%   (NASDAQ LULD style)
//	BandBps 1000 → ±10%  (wider volatile-instrument band)
type PercentBandCircuitBreaker struct {
	BandBps int64
}

func (cb *PercentBandCircuitBreaker) Check(_ string, lastPrice, bestOpposing int64, _ Side) CBResult {
	if lastPrice == 0 || !bandExceeded(lastPrice, bestOpposing, cb.BandBps) {
		return CBResult{Action: CBAllow}
	}
	return CBResult{Action: CBReject}
}

// AsymmetricBandCircuitBreaker applies independent up/down bands.
// Buy orders are checked against UpBandBps (best ask rising too far);
// sell orders are checked against DownBandBps (best bid falling too far).
// Moves in the favourable direction are never rejected.
type AsymmetricBandCircuitBreaker struct {
	UpBandBps   int64
	DownBandBps int64
}

func (cb *AsymmetricBandCircuitBreaker) Check(_ string, lastPrice, bestOpposing int64, side Side) CBResult {
	if lastPrice == 0 {
		return CBResult{Action: CBAllow}
	}
	if side == Buy && bestOpposing > lastPrice && bandExceeded(lastPrice, bestOpposing, cb.UpBandBps) {
		return CBResult{Action: CBReject}
	}
	if side == Sell && bestOpposing < lastPrice && bandExceeded(lastPrice, bestOpposing, cb.DownBandBps) {
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

// TieredCircuitBreaker applies escalating actions as price moves grow, using
// a per-symbol reference price set via SetRefPrice.
// Modelled after CME three-level circuit breakers and Chinese A-share daily limits.
// The most severe tier breached wins; tiers must be sorted ascending by ThresholdBps.
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

func (cb *TieredCircuitBreaker) Check(symbol string, _ int64, bestOpposing int64, _ Side) CBResult {
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
// It checks actual execution prices against tiered thresholds and triggers a
// per-symbol halt when any execution breaches a tier.
// Tiers must be sorted ascending by ThresholdBps.
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

func (e *TieredHaltEvaluator) Evaluate(symbol string, execs []*Execution) (bool, time.Duration) {
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
// restrictive result. Among two CBHalt results, the longer duration wins
// (0 = indefinite, which always dominates).
type CompositeCircuitBreaker struct {
	Breakers []CircuitBreaker
}

func (c *CompositeCircuitBreaker) Check(symbol string, lastPrice, bestOpposing int64, side Side) CBResult {
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

// haltForever encodes an indefinite halt in the haltUntil map.
const haltForever = -1

// CircuitBreakerMatcher wraps an inner MatchingEngine with a pre-trade
// CircuitBreaker and an optional post-match HaltEvaluator.
//
// Pre-trade: before each match the best opposing price is checked.
// CBReject silently drops the order; CBHalt drops it and halts the symbol.
//
// Post-match: the HaltEvaluator (if set) inspects actual executions and may
// trigger a per-symbol halt based on fill prices.
//
// Halted symbols reject all incoming orders until ClearHalt is called or the
// halt duration expires. The reverse index from Book pointer to symbol is
// rebuilt lazily when the books map grows, keeping Match O(1) amortised.
type CircuitBreakerMatcher struct {
	Inner         MatchingEngine
	Breaker       CircuitBreaker
	books         map[string]*OrderBook
	clock         Clock
	haltUntil     map[string]int64
	reverseIdx    map[*Book]string
	haltEvaluator HaltEvaluator
}

func NewCircuitBreakerMatcher(inner MatchingEngine, breaker CircuitBreaker, books map[string]*OrderBook, clock Clock) *CircuitBreakerMatcher {
	return &CircuitBreakerMatcher{
		Inner:      inner,
		Breaker:    breaker,
		books:      books,
		clock:      clock,
		haltUntil:  make(map[string]int64),
		reverseIdx: make(map[*Book]string),
	}
}

func (m *CircuitBreakerMatcher) SetHaltEvaluator(he HaltEvaluator) {
	m.haltEvaluator = he
}

func (m *CircuitBreakerMatcher) IsHalted(symbol string) bool {
	return m.isHaltedAt(symbol, m.clock.NowUnixNano())
}

func (m *CircuitBreakerMatcher) ClearHalt(symbol string) {
	delete(m.haltUntil, symbol)
}

func (m *CircuitBreakerMatcher) Match(bidBook, askBook *Book, order *Order) *MatchResult {
	sym, known := m.symbolFor(bidBook)

	if known {
		if m.isHaltedAt(sym, m.clock.NowUnixNano()) {
			return &MatchResult{}
		}

		ob := m.books[sym]
		lastPrice := ob.GetLastPrice()

		var opposing *Book
		if order.Side == Buy {
			opposing = askBook
		} else {
			opposing = bidBook
		}

		if lastPrice != 0 && opposing.Best != nil {
			res := m.Breaker.Check(sym, lastPrice, opposing.Best.Price, order.Side)
			switch res.Action {
			case CBReject:
				return &MatchResult{}
			case CBHalt:
				m.setHalt(sym, res.HaltDuration)
				return &MatchResult{}
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

func (m *CircuitBreakerMatcher) symbolFor(bidBook *Book) (string, bool) {
	if len(m.reverseIdx) != len(m.books) {
		m.reverseIdx = make(map[*Book]string, len(m.books))
		for sym, ob := range m.books {
			m.reverseIdx[ob.Bids] = sym
		}
	}
	sym, ok := m.reverseIdx[bidBook]
	return sym, ok
}

func (m *CircuitBreakerMatcher) isHaltedAt(sym string, now int64) bool {
	until, ok := m.haltUntil[sym]
	if !ok {
		return false
	}
	if until == haltForever {
		return true
	}
	return now < until
}

func (m *CircuitBreakerMatcher) setHalt(sym string, dur time.Duration) {
	if dur == 0 {
		m.haltUntil[sym] = haltForever
	} else {
		m.haltUntil[sym] = m.clock.NowUnixNano() + dur.Nanoseconds()
	}
}
