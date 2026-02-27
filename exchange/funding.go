package exchange

import "sync"

type positionKey struct {
	Symbol string
	Side   PositionSide
}

type PositionManager struct {
	positions map[uint64]map[positionKey]*Position
	clock     Clock
	mu        sync.RWMutex
}

func NewPositionManager(clock Clock) *PositionManager {
	return &PositionManager{
		positions: make(map[uint64]map[positionKey]*Position),
		clock:     clock,
	}
}

func (pm *PositionManager) GetPosition(clientID uint64, symbol string) *Position {
	return pm.GetPositionBySide(clientID, symbol, PositionBoth)
}

func (pm *PositionManager) GetPositionBySide(clientID uint64, symbol string, posSide PositionSide) *Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.positions[clientID] == nil {
		return nil
	}
	p := pm.positions[clientID][positionKey{symbol, posSide}]
	if p == nil {
		return nil
	}
	// Return copy to avoid races with callers using the value after lock release
	copy := *p
	return &copy
}

// UpdatePositionWithDelta updates the position and returns old and new state.
func (pm *PositionManager) UpdatePositionWithDelta(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide, exchange *Exchange, reason string) PositionDelta {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[clientID] == nil {
		pm.positions[clientID] = make(map[positionKey]*Position)
	}

	key := positionKey{symbol, posSide}
	pos := pm.positions[clientID][key]
	if pos == nil {
		pos = &Position{ClientID: clientID, Symbol: symbol, PositionSide: posSide}
		pm.positions[clientID][key] = pos
	}

	delta := PositionDelta{OldSize: pos.Size, OldEntryPrice: pos.EntryPrice}

	pm.applyPositionChange(pos, qty, price, tradeSide, posSide)

	delta.NewSize = pos.Size
	delta.NewEntryPrice = pos.EntryPrice

	if exchange != nil {
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
				TradeSide:     tradeSide.String(),
				Reason:        reason,
			})

			openInterest := pm.calculateOpenInterestUnsafe(symbol)
			log.LogEvent(timestamp, 0, "open_interest", OpenInterestEvent{
				Timestamp:    timestamp,
				Symbol:       symbol,
				OpenInterest: openInterest,
			})
		}
	}

	return delta
}

func (pm *PositionManager) UpdatePosition(clientID uint64, symbol string, qty, price int64, side Side) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[clientID] == nil {
		pm.positions[clientID] = make(map[positionKey]*Position)
	}

	key := positionKey{symbol, PositionBoth}
	pos := pm.positions[clientID][key]
	if pos == nil {
		pos = &Position{ClientID: clientID, Symbol: symbol, PositionSide: PositionBoth}
		pm.positions[clientID][key] = pos
	}

	pm.applyPositionChange(pos, qty, price, side, PositionBoth)
}

func (pm *PositionManager) applyPositionChange(pos *Position, qty, price int64, tradeSide Side, posSide PositionSide) {
	if posSide == PositionLong || posSide == PositionShort {
		pm.applyHedgePositionChange(pos, qty, price, tradeSide)
		return
	}
	pm.applyNettingPositionChange(pos, qty, price, tradeSide)
}

func (pm *PositionManager) applyNettingPositionChange(pos *Position, qty, price int64, side Side) {
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

// applyHedgePositionChange accumulates or reduces the hedge-side position independently.
// PositionLong always holds positive size; PositionShort always holds negative size.
func (pm *PositionManager) applyHedgePositionChange(pos *Position, qty, price int64, tradeSide Side) {
	if tradeSide == Buy {
		// Adding to long / reducing short
		if pos.Size < 0 {
			// Reducing: just move towards zero
			pos.Size = min(0, pos.Size+qty)
			if pos.Size == 0 {
				pos.EntryPrice = 0
			}
		} else {
			// Accumulating long
			if pos.Size == 0 {
				pos.EntryPrice = price
			} else {
				totalNotional := pos.Size*pos.EntryPrice + qty*price
				pos.Size += qty
				pos.EntryPrice = totalNotional / pos.Size
				return
			}
			pos.Size += qty
		}
	} else {
		// Adding to short / reducing long
		if pos.Size > 0 {
			// Reducing: just move towards zero
			pos.Size = max(0, pos.Size-qty)
			if pos.Size == 0 {
				pos.EntryPrice = 0
			}
		} else {
			// Accumulating short
			if pos.Size == 0 {
				pos.EntryPrice = price
			} else {
				totalNotional := (-pos.Size)*pos.EntryPrice + qty*price
				pos.Size -= qty
				pos.EntryPrice = totalNotional / (-pos.Size)
				return
			}
			pos.Size -= qty
		}
	}
}

func (pm *PositionManager) CalculateOpenInterest(symbol string) int64 {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.calculateOpenInterestUnsafe(symbol)
}

// calculateOpenInterestUnsafe calculates open interest without locking (caller must hold lock)
func (pm *PositionManager) calculateOpenInterestUnsafe(symbol string) int64 {
	var total int64
	for _, clientPositions := range pm.positions {
		for key, pos := range clientPositions {
			if key.Symbol == symbol && pos.Size != 0 {
				total += abs(pos.Size)
			}
		}
	}
	return total
}

// SettleFunding applies funding payments from/to client PerpBalances.
// Payments are zero-sum: the net flow between longs and shorts routes to/from
// exchange.ExchangeBalance.FeeRevenue so money is conserved.
func (pm *PositionManager) SettleFunding(clients map[uint64]*Client, perp *PerpFutures, exchange *Exchange) {
	fundingRate := perp.GetFundingRate()
	precision := perp.BasePrecision() // satoshis per whole base asset (not TickSize)
	timestamp := pm.clock.NowUnixNano()
	quote := perp.QuoteAsset()

	// netExchangeFlow > 0: exchange received more from longs than it paid to shorts.
	// netExchangeFlow < 0: exchange paid out more to shorts than it received from longs.
	netExchangeFlow := int64(0)

	perpSymbol := perp.Symbol()
	for clientID, clientPos := range pm.positions {
		pos := clientPos[positionKey{perpSymbol, PositionBoth}]
		if pos == nil || pos.Size == 0 {
			continue
		}

		client := clients[clientID]
		if client == nil {
			continue
		}

		positionValue := abs(pos.Size) * pos.EntryPrice / precision
		funding := positionValue * fundingRate.Rate / 10000

		oldBalance := client.PerpBalances[quote]
		if pos.Size > 0 {
			client.PerpBalances[quote] -= funding
			netExchangeFlow += funding
		} else {
			client.PerpBalances[quote] += funding
			netExchangeFlow -= funding
		}

		if exchange != nil {
			logBalanceChange(exchange, timestamp, clientID, perp.Symbol(), "funding_settlement", []BalanceDelta{
				perpDelta(quote, oldBalance, client.PerpBalances[quote]),
			})
		}
	}

	// Route net imbalance to exchange fee revenue (or drain from it if negative).
	// On real exchanges this goes to the insurance fund when the exchange is the residual payer.
	if exchange != nil && netExchangeFlow != 0 {
		exchange.ExchangeBalance.FeeRevenue[quote] += netExchangeFlow
	}

	fundingRate.NextFunding = pm.clock.NowUnixNano() + (fundingRate.Interval * 1e9)
}

// realizedPerpPnL calculates the realized PnL for a perp fill.
// Only non-zero when the trade reduces or closes an existing position.
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
	// closedQty is in base satoshis, priceDiff is in quote precision per full base
	// Result is in quote precision
	priceDiff := tradePrice - oldEntryPrice
	return (closedQty * sign * priceDiff) / basePrecision
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Lock acquires the PositionManager write lock.
func (pm *PositionManager) Lock() { pm.mu.Lock() }

// Unlock releases the PositionManager write lock.
func (pm *PositionManager) Unlock() { pm.mu.Unlock() }

// InjectPosition directly sets a position for testing purposes.
// Caller must hold Lock().
func (pm *PositionManager) InjectPosition(clientID uint64, symbol string, pos *Position) {
	if pm.positions[clientID] == nil {
		pm.positions[clientID] = make(map[positionKey]*Position)
	}
	pm.positions[clientID][positionKey{symbol, pos.PositionSide}] = pos
}

// GetPositions returns all positions for a client keyed by symbol+side, for testing/debugging.
// Caller is responsible for concurrent safety.
func (pm *PositionManager) GetPositions(clientID uint64) map[positionKey]*Position {
	return pm.positions[clientID]
}

// Abs is the exported version of abs for testing.
func Abs(x int64) int64 { return abs(x) }
