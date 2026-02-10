package exchange

import "sync"

type PositionManager struct {
	positions map[uint64]map[string]*Position
	clock     Clock
	mu        sync.RWMutex
}

func NewPositionManager(clock Clock) *PositionManager {
	return &PositionManager{
		positions: make(map[uint64]map[string]*Position),
		clock:     clock,
	}
}

func (pm *PositionManager) GetPosition(clientID uint64, symbol string) *Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.positions[clientID] == nil {
		return nil
	}
	p := pm.positions[clientID][symbol]
	if p == nil {
		return nil
	}
	// Return copy to avoid races with callers using the value after lock release
	copy := *p
	return &copy
}

// PositionDelta contains position state before and after an update.
type PositionDelta struct {
	OldSize       int64
	OldEntryPrice int64
	NewSize       int64
	NewEntryPrice int64
}

// UpdatePositionWithDelta updates the position and returns old and new state.
func (pm *PositionManager) UpdatePositionWithDelta(clientID uint64, symbol string, qty int64, price int64, side Side, exchange *Exchange, reason string) PositionDelta {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[clientID] == nil {
		pm.positions[clientID] = make(map[string]*Position)
	}

	pos := pm.positions[clientID][symbol]
	if pos == nil {
		pos = &Position{ClientID: clientID, Symbol: symbol}
		pm.positions[clientID][symbol] = pos
	}

	delta := PositionDelta{OldSize: pos.Size, OldEntryPrice: pos.EntryPrice}

	pm.applyPositionChange(pos, qty, price, side)

	delta.NewSize = pos.Size
	delta.NewEntryPrice = pos.EntryPrice

	if exchange != nil && exchange.balanceTracker != nil {
		timestamp := pm.clock.NowUnixNano()
		if log := exchange.getLogger("_global"); log != nil {
			log.LogEvent(timestamp, clientID, "position_update", PositionUpdateEvent{
				Timestamp:     timestamp,
				ClientID:      clientID,
				Symbol:        symbol,
				OldSize:       delta.OldSize,
				OldEntryPrice: delta.OldEntryPrice,
				NewSize:       delta.NewSize,
				NewEntryPrice: delta.NewEntryPrice,
				TradeQty:      qty,
				TradePrice:    price,
				TradeSide:     side.String(),
				Reason:        reason,
			})
		}
	}

	return delta
}

func (pm *PositionManager) UpdatePosition(clientID uint64, symbol string, qty int64, price int64, side Side) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[clientID] == nil {
		pm.positions[clientID] = make(map[string]*Position)
	}

	pos := pm.positions[clientID][symbol]
	if pos == nil {
		pos = &Position{ClientID: clientID, Symbol: symbol}
		pm.positions[clientID][symbol] = pos
	}

	pm.applyPositionChange(pos, qty, price, side)
}

func (pm *PositionManager) applyPositionChange(pos *Position, qty int64, price int64, side Side) {
	deltaSize := qty
	if side == Sell {
		deltaSize = -qty
	}

	newSize := pos.Size + deltaSize
	if newSize == 0 {
		pos.Size = 0
		pos.EntryPrice = 0
	} else if pos.Size == 0 {
		pos.Size = newSize
		pos.EntryPrice = price
	} else if (pos.Size > 0 && newSize > pos.Size) || (pos.Size < 0 && newSize < pos.Size) {
		totalNotional := (pos.Size * pos.EntryPrice) + (deltaSize * price)
		pos.EntryPrice = totalNotional / newSize
		pos.Size = newSize
	} else if (pos.Size > 0 && newSize < 0) || (pos.Size < 0 && newSize > 0) {
		pos.EntryPrice = price
		pos.Size = newSize
	} else {
		pos.Size = newSize
	}
}

func (pm *PositionManager) CalculateOpenInterest(symbol string) int64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var total int64
	for _, clientPositions := range pm.positions {
		if pos := clientPositions[symbol]; pos != nil && pos.Size != 0 {
			total += abs(pos.Size)
		}
	}
	return total
}

// SettleFunding applies funding payments from/to client PerpBalances.
func (pm *PositionManager) SettleFunding(clients map[uint64]*Client, perp *PerpFutures, exchange *Exchange) {
	fundingRate := perp.GetFundingRate()
	precision := perp.TickSize()
	timestamp := pm.clock.NowUnixNano()
	quote := perp.QuoteAsset()

	for clientID, clientPos := range pm.positions {
		pos := clientPos[perp.Symbol()]
		if pos == nil || pos.Size == 0 {
			continue
		}

		client := clients[clientID]
		if client == nil {
			continue
		}

		positionValue := abs(pos.Size) * pos.EntryPrice / precision
		funding := (positionValue * fundingRate.Rate) / 10000

		oldBalance := client.PerpBalances[quote]
		if pos.Size > 0 {
			client.PerpBalances[quote] -= funding
		} else {
			client.PerpBalances[quote] += funding
		}

		if exchange != nil && exchange.balanceTracker != nil {
			exchange.balanceTracker.LogBalanceChange(timestamp, clientID, perp.Symbol(), "funding_settlement", []BalanceDelta{
				perpDelta(quote, oldBalance, client.PerpBalances[quote]),
			})
		}
	}

	fundingRate.NextFunding = pm.clock.NowUnixNano() + (fundingRate.Interval * 1e9)
}

// realizedPerpPnL calculates the realized PnL for a perp fill.
// Only non-zero when the trade reduces or closes an existing position.
// Returns PnL in quote asset satoshis (e.g., USD satoshis).
// Prices must be in quote precision (e.g., USD_PRECISION), not base precision.
func realizedPerpPnL(oldSize, oldEntryPrice, tradeQty, tradePrice int64, tradeSide Side, basePrecision int64) int64 {
	if oldSize == 0 {
		return 0
	}
	deltaSize := tradeQty
	if tradeSide == Sell {
		deltaSize = -tradeQty
	}
	// Only realize PnL if this trade reduces the position magnitude
	if (oldSize > 0 && deltaSize >= 0) || (oldSize < 0 && deltaSize <= 0) {
		return 0
	}
	closedQty := abs(deltaSize)
	if closedQty > abs(oldSize) {
		closedQty = abs(oldSize)
	}
	sign := int64(1)
	if oldSize < 0 {
		sign = -1
	}
	// PnL formula: prices are in quotePrecision per full base asset
	// closedQty is in base satoshis, priceDiff is in quote satoshis per full base
	// Result is in quote satoshis
	priceDiff := tradePrice - oldEntryPrice
	return (closedQty * sign * priceDiff) / basePrecision
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
