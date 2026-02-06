package signals

import (
	"time"
)

type CrossSectionalSignals struct {
	horizonTracker *HorizonTracker
}

func NewCrossSectionalSignals(horizonTracker *HorizonTracker) *CrossSectionalSignals {
	return &CrossSectionalSignals{
		horizonTracker: horizonTracker,
	}
}

func (cs *CrossSectionalSignals) Calculate(symbols []string, horizon time.Duration) map[string]int64 {
	signals := make(map[string]int64)
	returns := make([]int64, 0, len(symbols))
	validSymbols := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if !cs.horizonTracker.IsReady(symbol, horizon) {
			continue
		}
		ret := cs.horizonTracker.GetReturn(symbol, horizon)
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
