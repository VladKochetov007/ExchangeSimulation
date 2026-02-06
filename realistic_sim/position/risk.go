package position

import (
	"exchange_sim/exchange"
	"time"
)

type RiskFilter interface {
	ShouldBlock(order *exchange.Order) bool
}

type PnLPoint struct {
	PnL       int64
	Timestamp time.Time
}

type DrawdownFilter struct {
	maxDrawdown     int64
	lookbackPeriod  time.Duration
	penaltyDuration time.Duration
	pnlHistory      []PnLPoint
	blockedUntil    time.Time
	currentPosition int64
}

func NewDrawdownFilter(maxDrawdown int64, lookbackPeriod, penaltyDuration time.Duration) *DrawdownFilter {
	return &DrawdownFilter{
		maxDrawdown:     maxDrawdown,
		lookbackPeriod:  lookbackPeriod,
		penaltyDuration: penaltyDuration,
		pnlHistory:      make([]PnLPoint, 0, 1000),
	}
}

func (df *DrawdownFilter) ShouldBlock(order *exchange.Order) bool {
	if time.Now().Before(df.blockedUntil) {
		return df.isIncreasingPosition(order)
	}
	return false
}

func (df *DrawdownFilter) isIncreasingPosition(order *exchange.Order) bool {
	delta := int64(order.Qty)
	if order.Side == exchange.Sell {
		delta = -delta
	}
	newPosition := df.currentPosition + delta
	if newPosition < 0 {
		newPosition = -newPosition
	}
	if df.currentPosition < 0 {
		df.currentPosition = -df.currentPosition
	}
	return newPosition > df.currentPosition
}

func (df *DrawdownFilter) CheckDrawdown(currentPnL int64, timestamp time.Time) {
	df.pnlHistory = append(df.pnlHistory, PnLPoint{PnL: currentPnL, Timestamp: timestamp})

	cutoff := timestamp.Add(-df.lookbackPeriod)
	start := 0
	for i, point := range df.pnlHistory {
		if point.Timestamp.After(cutoff) {
			start = i
			break
		}
	}
	df.pnlHistory = df.pnlHistory[start:]

	if len(df.pnlHistory) == 0 {
		return
	}

	maxPnL := df.pnlHistory[0].PnL
	for _, point := range df.pnlHistory {
		if point.PnL > maxPnL {
			maxPnL = point.PnL
		}
	}

	drawdown := maxPnL - currentPnL
	if maxPnL > 0 {
		drawdownBps := (drawdown * 10000) / maxPnL
		if drawdownBps > df.maxDrawdown {
			df.blockedUntil = timestamp.Add(df.penaltyDuration)
		}
	}
}

func (df *DrawdownFilter) UpdatePosition(position int64) {
	df.currentPosition = position
}

type PositionFlipFilter struct {
	lastPosition int64
}

func NewPositionFlipFilter() *PositionFlipFilter {
	return &PositionFlipFilter{}
}

func (pf *PositionFlipFilter) ShouldBlock(order *exchange.Order) bool {
	delta := int64(order.Qty)
	if order.Side == exchange.Sell {
		delta = -delta
	}
	newPosition := pf.lastPosition + delta

	if pf.lastPosition*newPosition < 0 {
		return true
	}
	return false
}

func (pf *PositionFlipFilter) UpdatePosition(position int64) {
	pf.lastPosition = position
}

type CompositeFilter struct {
	filters []RiskFilter
}

func NewCompositeFilter(filters ...RiskFilter) *CompositeFilter {
	return &CompositeFilter{filters: filters}
}

func (cf *CompositeFilter) ShouldBlock(order *exchange.Order) bool {
	for _, f := range cf.filters {
		if f.ShouldBlock(order) {
			return true
		}
	}
	return false
}
