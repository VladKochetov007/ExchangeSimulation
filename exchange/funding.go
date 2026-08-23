package exchange

import (
	"fmt"
	"slices"
	"sync"
)

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
	copy := *p
	return &copy
}

// UpdatePosition applies a trade delta and returns old/new state.
// Logging is the caller's responsibility.
func (pm *PositionManager) UpdatePosition(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide) PositionDelta {
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
	return delta
}

// AddPositionMargin increases the tracked margin ledger for a position.
// Implements types.MarginLedger.
func (pm *PositionManager) AddPositionMargin(clientID uint64, symbol string, side PositionSide, amount int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[clientID] == nil {
		return
	}
	if pos := pm.positions[clientID][positionKey{symbol, side}]; pos != nil {
		pos.Margin += amount
	}
}

// ReleasePositionMargin removes and returns the margin share for closing
// closedQty out of a position previously sized oldSize. A full close returns
// the entire remainder so no rounding dust survives the position lifecycle.
// Implements types.MarginLedger.
func (pm *PositionManager) ReleasePositionMargin(clientID uint64, symbol string, side PositionSide, closedQty, oldSize int64) int64 {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.positions[clientID] == nil {
		return 0
	}
	pos := pm.positions[clientID][positionKey{symbol, side}]
	if pos == nil || pos.Margin <= 0 {
		return 0
	}
	absOld := abs(oldSize)
	if closedQty >= absOld {
		release := pos.Margin
		pos.Margin = 0
		return release
	}
	release := MulDiv(pos.Margin, closedQty, absOld)
	pos.Margin -= release
	return release
}

func (pm *PositionManager) HasOpenPositions(clientID uint64) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, pos := range pm.positions[clientID] {
		if pos != nil && pos.Size != 0 {
			return true
		}
	}
	return false
}

func (pm *PositionManager) PositionsForFunding(symbol string, fn func(clientID uint64, pos Position)) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Client-ID order: funding and expiry both settle through this callback,
	// and map order would make the sequence of payments — and the rounding
	// remainder that routes to the exchange — differ between runs.
	clientIDs := make([]uint64, 0, len(pm.positions))
	for clientID := range pm.positions {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)

	for _, clientID := range clientIDs {
		clientPositions := pm.positions[clientID]
		for _, side := range []PositionSide{PositionBoth, PositionLong, PositionShort} {
			pos := clientPositions[positionKey{symbol, side}]
			if pos == nil || pos.Size == 0 {
				continue
			}
			fn(clientID, *pos)
		}
	}
}

func (pm *PositionManager) GetAllPositions(clientID uint64) []Position {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	clientPositions := pm.positions[clientID]
	if len(clientPositions) == 0 {
		return nil
	}
	result := make([]Position, 0, len(clientPositions))
	for _, pos := range clientPositions {
		if pos.Size != 0 {
			result = append(result, *pos)
		}
	}
	return result
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
		// Exact 128-bit weighted average: size × price overflows int64 and
		// exceeds float64's mantissa, and any error here is money.
		pos.EntryPrice = WeightedAverage(abs(pos.Size), pos.EntryPrice, abs(deltaSize), price)
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
				newSize := pos.Size + qty
				pos.EntryPrice = WeightedAverage(pos.Size, pos.EntryPrice, qty, price)
				pos.Size = newSize
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
				newSize := pos.Size - qty
				pos.EntryPrice = WeightedAverage(-pos.Size, pos.EntryPrice, qty, price)
				pos.Size = newSize
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

// fundingEventSink carries the two side-effects settleFunding needs from the exchange.
// Defined here because it references exchange-internal types. Unexported.
type fundingEventSink struct {
	logBalance    func(timestamp int64, clientID uint64, symbol, reason string, changes []BalanceDelta)
	recordRevenue func(asset string, amount int64)
}

// SettleFunding settles funding for perp without logging. A missing shared
// mark is observable to the caller; it is never replaced with an entry-price
// or zero-price estimate. Used in isolated unit tests.
func (pm *PositionManager) SettleFunding(clients map[uint64]*Client, perp *PerpFutures) error {
	if perp == nil {
		return fmt.Errorf("funding settlement: %w", ErrNoBookPrice)
	}
	if !settleFunding(pm, clients, perp, pm.clock, fundingEventSink{}) {
		return fmt.Errorf("funding settlement for %s: %w", perp.Symbol(), ErrNoBookPrice)
	}
	return nil
}

// settleFunding applies funding payments from/to client PerpBalances.
// Payments are zero-sum: net flow between longs and shorts routes to/from exchange revenue.
func settleFunding(store PositionStore, clients map[uint64]*Client, perp *PerpFutures, clock Clock, sink fundingEventSink) bool {
	fundingRate := perp.GetFundingRate()
	if !fundingRate.MarkAvailable || fundingRate.MarkPrice <= 0 {
		// A missing mark is not permission to value funding at each position's
		// entry price. That would turn price absence into a hidden per-account
		// fallback and break the shared-mark funding contract.
		return false
	}
	precision := perp.BasePrecision()
	timestamp := clock.NowUnixNano()
	quote := perp.QuoteAsset()

	// Funding accrues on position value at MARK price (universal perp
	// convention): equal opposite positions pay/receive equal amounts
	// regardless of their entry prices, keeping funding zero-sum.
	markPrice := fundingRate.MarkPrice

	// netExchangeFlow > 0: exchange received more from longs than it paid to shorts.
	// netExchangeFlow < 0: exchange paid out more to shorts than it received from longs.
	netExchangeFlow := int64(0)

	store.PositionsForFunding(perp.Symbol(), func(clientID uint64, pos Position) {
		client := clients[clientID]
		if client == nil {
			return
		}
		positionValue := MulDiv(abs(pos.Size), markPrice, precision)
		funding := positionValue * fundingRate.Rate / 10000

		oldBalance := client.PerpBalances[quote]
		if pos.Size > 0 {
			client.PerpBalances[quote] -= funding
			netExchangeFlow += funding
		} else {
			client.PerpBalances[quote] += funding
			netExchangeFlow -= funding
		}
		if sink.logBalance != nil {
			sink.logBalance(timestamp, clientID, perp.Symbol(), "funding_settlement", []BalanceDelta{
				perpDelta(quote, oldBalance, client.PerpBalances[quote]),
			})
		}
	})

	// Route net imbalance to exchange fee revenue (or drain from it if negative).
	// On real exchanges this goes to the insurance fund when the exchange is the residual payer.
	if sink.recordRevenue != nil && netExchangeFlow != 0 {
		sink.recordRevenue(quote, netExchangeFlow)
	}

	fundingRate.NextFunding = clock.NowUnixNano() + (fundingRate.Interval * 1e9)
	return true
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
	return sign * MulDiv(closedQty, tradePrice-oldEntryPrice, basePrecision)
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
