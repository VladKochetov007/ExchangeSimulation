package lifecycle

import (
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
)

type StartCondition interface {
	IsSatisfied() bool
	Description() string
}

type AlwaysSatisfied struct{}

func (as *AlwaysSatisfied) IsSatisfied() bool {
	return true
}

func (as *AlwaysSatisfied) Description() string {
	return "Always satisfied"
}

type LiquiditySufficientCondition struct {
	exchange        *exchange.Exchange
	symbol          string
	minBidLiquidity int64
	minAskLiquidity int64
}

func NewLiquiditySufficientCondition(ex *exchange.Exchange, symbol string, minBid, minAsk int64) *LiquiditySufficientCondition {
	return &LiquiditySufficientCondition{
		exchange:        ex,
		symbol:          symbol,
		minBidLiquidity: minBid,
		minAskLiquidity: minAsk,
	}
}

func (lc *LiquiditySufficientCondition) IsSatisfied() bool {
	bidQty, askQty := lc.exchange.GetBestLiquidity(lc.symbol)
	return bidQty >= lc.minBidLiquidity && askQty >= lc.minAskLiquidity
}

func (lc *LiquiditySufficientCondition) Description() string {
	return "Wait for sufficient liquidity"
}

type DataAvailableCondition struct {
	buffer *actors.CircularBuffer
}

func NewDataAvailableCondition(buffer *actors.CircularBuffer) *DataAvailableCondition {
	return &DataAvailableCondition{buffer: buffer}
}

func (dc *DataAvailableCondition) IsSatisfied() bool {
	return dc.buffer.IsFull()
}

func (dc *DataAvailableCondition) Description() string {
	return "Wait for data buffer to fill"
}

type CompositeCondition struct {
	conditions []StartCondition
	requireAll bool
}

func NewCompositeCondition(requireAll bool, conditions ...StartCondition) *CompositeCondition {
	return &CompositeCondition{
		conditions: conditions,
		requireAll: requireAll,
	}
}

func (cc *CompositeCondition) IsSatisfied() bool {
	if cc.requireAll {
		for _, cond := range cc.conditions {
			if !cond.IsSatisfied() {
				return false
			}
		}
		return true
	}

	for _, cond := range cc.conditions {
		if cond.IsSatisfied() {
			return true
		}
	}
	return false
}

func (cc *CompositeCondition) Description() string {
	if cc.requireAll {
		return "Wait for all conditions"
	}
	return "Wait for any condition"
}
