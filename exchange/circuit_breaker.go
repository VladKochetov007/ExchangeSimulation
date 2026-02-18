package exchange

// CircuitBreaker decides whether a fill at newPrice should be allowed given the last price.
// Return false to abort the entire match and reject the incoming order.
type CircuitBreaker interface {
	Allow(symbol string, lastPrice, newPrice int64) bool
}

// PercentBandCircuitBreaker rejects fills that deviate more than BandBps basis points
// from the last traded price. Zero lastPrice disables the check (no trades yet).
type PercentBandCircuitBreaker struct {
	BandBps int64 // e.g. 500 = 5%
}

func (cb *PercentBandCircuitBreaker) Allow(_ string, lastPrice, newPrice int64) bool {
	if lastPrice == 0 {
		return true
	}
	diff := newPrice - lastPrice
	if diff < 0 {
		diff = -diff
	}
	return diff*10000 <= lastPrice*cb.BandBps
}

// CircuitBreakerMatcher wraps an inner MatchingEngine. Before each match it checks
// the best opposing price against the symbol's last trade via the CircuitBreaker.
// If the breaker fires the order is returned unfilled.
//
// books must be the same map reference as Exchange.Books so it reflects live state
// and newly added instruments are automatically included.
//
// Usage:
//
//	ex.Matcher = NewCircuitBreakerMatcher(ex.Matcher, &PercentBandCircuitBreaker{BandBps: 500}, ex.Books)
type CircuitBreakerMatcher struct {
	Inner   MatchingEngine
	Breaker CircuitBreaker
	books   map[string]*OrderBook
}

func NewCircuitBreakerMatcher(inner MatchingEngine, breaker CircuitBreaker, books map[string]*OrderBook) *CircuitBreakerMatcher {
	return &CircuitBreakerMatcher{Inner: inner, Breaker: breaker, books: books}
}

func (m *CircuitBreakerMatcher) Priority() Priority {
	return m.Inner.Priority()
}

func (m *CircuitBreakerMatcher) Match(bidBook, askBook *Book, order *Order) *MatchResult {
	var ob *OrderBook
	for _, candidate := range m.books {
		if candidate.Bids == bidBook {
			ob = candidate
			break
		}
	}

	var candidateBook *Book
	if order.Side == Buy {
		candidateBook = askBook
	} else {
		candidateBook = bidBook
	}

	if ob != nil && candidateBook.Best != nil {
		lastPrice := ob.GetLastPrice()
		if !m.Breaker.Allow(ob.Symbol, lastPrice, candidateBook.Best.Price) {
			return &MatchResult{Executions: nil, FullyFilled: false}
		}
	}

	return m.Inner.Match(bidBook, askBook, order)
}
