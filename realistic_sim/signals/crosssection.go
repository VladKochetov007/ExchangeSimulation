package signals

import (
	"sync"
	"time"
)

type CrossSectionalSignals struct {
	priceHistories map[string]*PriceHistory
	mu             sync.RWMutex
}

func NewCrossSectionalSignals(lookbackPeriod time.Duration, scale int64) *CrossSectionalSignals {
	return &CrossSectionalSignals{
		priceHistories: make(map[string]*PriceHistory),
	}
}

func (cs *CrossSectionalSignals) AddSymbol(symbol string, lookbackPeriod time.Duration, scale int64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.priceHistories[symbol] = NewPriceHistory(lookbackPeriod, scale)
}

func (cs *CrossSectionalSignals) AddPrice(symbol string, price int64, timestamp int64) {
	cs.mu.RLock()
	ph := cs.priceHistories[symbol]
	cs.mu.RUnlock()

	if ph != nil {
		ph.AddPrice(price, timestamp)
	}
}

func (cs *CrossSectionalSignals) Calculate(symbols []string, currentTime int64) map[string]int64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	signals := make(map[string]int64)
	returns := make([]int64, 0, len(symbols))
	validSymbols := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		ph := cs.priceHistories[symbol]
		if ph == nil || !ph.IsReady(currentTime) {
			continue
		}
		ret := ph.GetReturn(currentTime)
		returns = append(returns, ret)
		validSymbols = append(validSymbols, symbol)
	}

	if len(returns) == 0 {
		return signals
	}

	sum := int64(0)
	for _, ret := range returns {
		sum += ret
	}
	meanReturn := sum / int64(len(returns))

	for i, symbol := range validSymbols {
		signals[symbol] = returns[i] - meanReturn
	}

	return signals
}
