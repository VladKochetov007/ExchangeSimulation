package exchange

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"sync"

	etypes "exchange_sim/types"
)

type positionKey struct {
	Symbol string
	Side   PositionSide
}

type PositionManager struct {
	positions              map[uint64]map[positionKey]*Position
	accounting             map[uint64]map[positionKey]*positionAccounting
	precisions             map[string]int64
	requireExactAccounting bool
	clock                  Clock
	mu                     sync.RWMutex
}

var _ etypes.ExactLinearPositionStore = (*PositionManager)(nil)

func NewPositionManager(clock Clock) *PositionManager {
	return &PositionManager{
		positions:  make(map[uint64]map[positionKey]*Position),
		accounting: make(map[uint64]map[positionKey]*positionAccounting),
		precisions: make(map[string]int64),
		clock:      clock,
	}
}

// SetPositionPrecision records the denomination required to turn exact
// position PnL numerators into quote-asset balance units. The exchange calls
// this before an instrument can receive its first order.
func (pm *PositionManager) SetPositionPrecision(symbol string, precision int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if precision > 0 {
		pm.precisions[symbol] = precision
	}
}

func (pm *PositionManager) ClearPositionPrecision(symbol string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.precisions, symbol)
}

func (pm *PositionManager) SetRequireExactLinearPositionAccounting(required bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.requireExactAccounting = required
}

func (pm *PositionManager) ExactLinearPositionAccountingRequired() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.requireExactAccounting
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
	delta, _ := pm.updatePosition(clientID, symbol, qty, price, tradeSide, posSide)
	return delta
}

// UpdatePositionWithAccounting is the atomic exact transition used by the
// default exchange. The separate accounting result preserves compatibility
// for callers that implement only PositionStore.
func (pm *PositionManager) UpdatePositionWithAccounting(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide) (PositionDelta, etypes.PositionAccountingDelta) {
	return pm.updatePosition(clientID, symbol, qty, price, tradeSide, posSide)
}

func (pm *PositionManager) CanUpdatePositionWithAccounting(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	precision := pm.precisions[symbol]
	if precision <= 0 {
		return false
	}
	var oldSize int64
	if clientPositions := pm.positions[clientID]; clientPositions != nil {
		if pos := clientPositions[positionKey{symbol, posSide}]; pos != nil {
			oldSize = pos.Size
		}
	}
	accounting := pm.accounting[clientID]
	var state *positionAccounting
	if accounting != nil {
		state = accounting[positionKey{symbol, posSide}]
	}
	_, _, _, ok := prepareExactTransition(state, oldSize, qty, price, tradeSide, posSide, precision)
	return ok
}

func (pm *PositionManager) updatePosition(clientID uint64, symbol string, qty, price int64, tradeSide Side, posSide PositionSide) (PositionDelta, etypes.PositionAccountingDelta) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	key := positionKey{symbol, posSide}
	clientPositions := pm.positions[clientID]
	pos := clientPositions[key]
	if pos == nil {
		pos = &Position{ClientID: clientID, Symbol: symbol, PositionSide: posSide}
	}

	delta := PositionDelta{OldSize: pos.Size, OldEntryPrice: pos.EntryPrice}
	accountingDelta := etypes.PositionAccountingDelta{}
	if precision := pm.precisions[symbol]; precision > 0 {
		var accounting *positionAccounting
		if clientAccounting := pm.accounting[clientID]; clientAccounting != nil {
			accounting = clientAccounting[key]
		}
		candidate, exactDelta, entryPrice, exactOK := prepareExactTransition(accounting, pos.Size, qty, price, tradeSide, posSide, precision)
		if pm.requireExactAccounting && !exactOK {
			panic("position manager: exact position transition unavailable")
		}
		if exactOK {
			if clientPositions == nil {
				clientPositions = make(map[positionKey]*Position)
				pm.positions[clientID] = clientPositions
			}
			if clientPositions[key] == nil {
				clientPositions[key] = pos
			}
			if pm.accounting[clientID] == nil {
				pm.accounting[clientID] = make(map[positionKey]*positionAccounting)
			}
			if accounting == nil {
				accounting = newPositionAccounting()
				pm.accounting[clientID][key] = accounting
			}
			*accounting = *candidate
			pos.Size = candidate.size
			pos.EntryPrice = entryPrice
			accountingDelta = exactDelta
			delta.NewSize = pos.Size
			delta.NewEntryPrice = pos.EntryPrice
			return delta, accountingDelta
		}
	}
	if clientPositions == nil {
		clientPositions = make(map[positionKey]*Position)
		pm.positions[clientID] = clientPositions
	}
	if clientPositions[key] == nil {
		clientPositions[key] = pos
	}
	pm.applyPositionChange(pos, qty, price, tradeSide, posSide)
	if accountingDelta.Valid {
		accounting := pm.accounting[clientID][key]
		var ok bool
		pos.EntryPrice, ok = accounting.entryPrice()
		if !ok {
			panic("position accounting: entry price overflows int64")
		}
	}
	delta.NewSize = pos.Size
	delta.NewEntryPrice = pos.EntryPrice
	return delta, accountingDelta
}

func supportsExactPositionTransition(oldSize, quantity int64, tradeSide Side, posSide PositionSide) bool {
	if quantity <= 0 || quantity == math.MinInt64 || oldSize == math.MinInt64 {
		return false
	}
	delta := quantity
	if tradeSide == Sell {
		delta = -quantity
	}
	newSize, ok := etypes.TryAdd(oldSize, delta)
	if !ok || newSize == math.MinInt64 {
		return false
	}
	if posSide == PositionLong && (oldSize < 0 || newSize < 0) {
		return false
	}
	if posSide == PositionShort && (oldSize > 0 || newSize > 0) {
		return false
	}
	return true
}

// prepareExactTransition is a non-mutating dry run. The exact store uses it
// both at admission and immediately before commit so a later representability
// failure cannot leave public position state and its cost basis on different
// lifecycles.
func prepareExactTransition(state *positionAccounting, oldSize, quantity, price int64, tradeSide Side, posSide PositionSide, precision int64) (candidate *positionAccounting, accountingDelta etypes.PositionAccountingDelta, entryPrice int64, ok bool) {
	defer func() {
		if recover() != nil {
			candidate = nil
			accountingDelta = etypes.PositionAccountingDelta{}
			entryPrice = 0
			ok = false
		}
	}()
	if !supportsExactPositionTransition(oldSize, quantity, tradeSide, posSide) {
		return nil, etypes.PositionAccountingDelta{}, 0, false
	}
	if state == nil {
		if oldSize != 0 {
			return nil, etypes.PositionAccountingDelta{}, 0, false
		}
		candidate = newPositionAccounting()
	} else {
		if state.size != oldSize {
			return nil, etypes.PositionAccountingDelta{}, 0, false
		}
		candidate = state.clone()
	}
	realizedPnL, valid := candidate.applyTrade(oldSize, quantity, price, tradeSide, precision)
	if !valid {
		return nil, etypes.PositionAccountingDelta{}, 0, false
	}
	entryPrice, valid = candidate.entryPrice()
	if !valid {
		return nil, etypes.PositionAccountingDelta{}, 0, false
	}
	return candidate, etypes.PositionAccountingDelta{RealizedPnL: realizedPnL, Valid: true}, entryPrice, true
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

// PositionUnrealizedPnL returns the lifecycle-consistent marked value for a
// position. The public EntryPrice remains a rounded display field; this path
// uses the exact aggregate basis and subtracts the already emitted realized
// amount so integer cash rounding cannot accumulate into a conservation drift.
func (pm *PositionManager) PositionUnrealizedPnL(position Position, markPrice, precision int64) (int64, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	accounting := pm.accounting[position.ClientID][positionKey{position.Symbol, position.PositionSide}]
	if accounting == nil || accounting.size != position.Size || precision <= 0 {
		return 0, false
	}
	return accounting.unrealizedPnL(markPrice, precision)
}

// PositionSettlementCashFlow values the still-open portion of a lifecycle at
// settlement. It is the same complement used by marked risk, and therefore
// closes without introducing a second rounding rule.
func (pm *PositionManager) PositionSettlementCashFlow(position Position, settlementPrice, precision int64) (int64, bool) {
	return pm.PositionUnrealizedPnL(position, settlementPrice, precision)
}

// SettlePositionAtPrice atomically values and closes one linear position. The
// caller applies the returned cash to the client only after this store
// transition succeeds, so a custom exact store cannot leave basis and cash on
// different lifecycles.
func (pm *PositionManager) SettlePositionAtPrice(position Position, settlementPrice, precision int64) (int64, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	key := positionKey{position.Symbol, position.PositionSide}
	stored := pm.positions[position.ClientID][key]
	accounting := pm.accounting[position.ClientID][key]
	if stored == nil || accounting == nil || stored.Size != position.Size || accounting.size != position.Size || position.Size == 0 || position.Size == math.MinInt64 {
		return 0, false
	}
	cash, ok := accounting.unrealizedPnL(settlementPrice, precision)
	if !ok {
		return 0, false
	}
	closeSide := Sell
	if position.Size < 0 {
		closeSide = Buy
	}
	candidate, accountingDelta, _, valid := prepareExactTransition(accounting, position.Size, abs(position.Size), settlementPrice, closeSide, position.PositionSide, precision)
	if !valid || accountingDelta.RealizedPnL != cash {
		return 0, false
	}
	*accounting = *candidate
	stored.Size = 0
	stored.EntryPrice = 0
	return cash, true
}

func (pm *PositionManager) CanSettlePositionAtPrice(position Position, settlementPrice, precision int64) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	key := positionKey{position.Symbol, position.PositionSide}
	stored := pm.positions[position.ClientID][key]
	accounting := pm.accounting[position.ClientID][key]
	if stored == nil || accounting == nil || stored.Size != position.Size || accounting.size != position.Size || position.Size == 0 || position.Size == math.MinInt64 {
		return false
	}
	cash, ok := accounting.unrealizedPnL(settlementPrice, precision)
	if !ok {
		return false
	}
	closeSide := Sell
	if position.Size < 0 {
		closeSide = Buy
	}
	_, accountingDelta, _, ok := prepareExactTransition(accounting, position.Size, abs(position.Size), settlementPrice, closeSide, position.PositionSide, precision)
	return ok && accountingDelta.RealizedPnL == cash
}

// PreviewPositionAccountingTerminalization performs the complete non-mutating
// expiry transition, including the carry that closing positions will create.
// The exchange uses the result to validate recipients and ledger arithmetic
// before it cancels orders or changes a client balance.
func (pm *PositionManager) PreviewPositionAccountingTerminalization(symbol string, settlementPrice, precision int64) ([]PositionAccountingRounding, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if precision <= 0 {
		return nil, false
	}
	byClient := make(map[uint64]*big.Int)
	seen := make(map[uint64]map[positionKey]bool)
	for clientID, clientPositions := range pm.positions {
		for key, stored := range clientPositions {
			if key.Symbol != symbol || stored == nil {
				continue
			}
			state := pm.accounting[clientID][key]
			if stored.Size == 0 {
				if state != nil && state.size != 0 {
					return nil, false
				}
				continue
			}
			if state == nil {
				return nil, false
			}
			closeSide := Sell
			if stored.Size < 0 {
				closeSide = Buy
			}
			candidate, accountingDelta, _, ok := prepareExactTransition(state, stored.Size, abs(stored.Size), settlementPrice, closeSide, key.Side, precision)
			if !ok {
				return nil, false
			}
			cash, cashOK := state.unrealizedPnL(settlementPrice, precision)
			if !cashOK || accountingDelta.RealizedPnL != cash {
				return nil, false
			}
			addCarry(byClient, clientID, candidate.carryNumerator)
			if seen[clientID] == nil {
				seen[clientID] = make(map[positionKey]bool)
			}
			seen[clientID][key] = true
		}
	}
	for clientID, clientAccounting := range pm.accounting {
		for key, state := range clientAccounting {
			if key.Symbol != symbol || seen[clientID][key] {
				continue
			}
			if state.size != 0 {
				return nil, false
			}
			addCarry(byClient, clientID, state.carryNumerator)
		}
	}
	return roundingFromNumerators(byClient, precision)
}

func (pm *PositionManager) PositionLiquidationPrice(position Position, netBalance, precision int64) (int64, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	accounting := pm.accounting[position.ClientID][positionKey{position.Symbol, position.PositionSide}]
	if accounting == nil || accounting.size != position.Size {
		return 0, false
	}
	return accounting.liquidationPrice(netBalance, precision)
}

func (pm *PositionManager) DrainPositionAccountingCarry(symbol string, precision int64) ([]PositionAccountingRounding, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	result, ok := pm.roundingForFlatAccounting(symbol, precision)
	if !ok {
		return nil, false
	}
	pm.clearAccountingCarry(symbol)
	return result, true
}

// CommitPositionAccountingCarry is the compare-and-clear half of expiry
// terminalization. The expected result was produced by a non-mutating preview;
// a mismatch leaves carry untouched and prevents client/venue ledger updates
// from being based on a different accounting state.
func (pm *PositionManager) CommitPositionAccountingCarry(symbol string, precision int64, expected []PositionAccountingRounding) ([]PositionAccountingRounding, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	result, ok := pm.roundingForFlatAccounting(symbol, precision)
	if !ok || !slices.Equal(result, expected) {
		return nil, false
	}
	pm.clearAccountingCarry(symbol)
	return result, true
}

func (pm *PositionManager) roundingForFlatAccounting(symbol string, precision int64) ([]PositionAccountingRounding, bool) {
	if precision <= 0 {
		return nil, false
	}
	byClient := make(map[uint64]*big.Int)
	for clientID, clientAccounting := range pm.accounting {
		for key, accounting := range clientAccounting {
			if key.Symbol != symbol {
				continue
			}
			if accounting.size != 0 {
				return nil, false
			}
			if accounting.carryNumerator != 0 {
				addCarry(byClient, clientID, accounting.carryNumerator)
			}
		}
	}
	result, ok := roundingFromNumerators(byClient, precision)
	if !ok {
		return nil, false
	}
	return result, true
}

func (pm *PositionManager) clearAccountingCarry(symbol string) {
	for _, clientAccounting := range pm.accounting {
		for key, accounting := range clientAccounting {
			if key.Symbol == symbol {
				accounting.carryNumerator = 0
			}
		}
	}
}

func addCarry(byClient map[uint64]*big.Int, clientID uint64, carry int64) {
	if carry == 0 {
		return
	}
	if byClient[clientID] == nil {
		byClient[clientID] = new(big.Int)
	}
	byClient[clientID].Add(byClient[clientID], big.NewInt(carry))
}

func roundingFromNumerators(byClient map[uint64]*big.Int, precision int64) ([]PositionAccountingRounding, bool) {
	clientIDs := make([]uint64, 0, len(byClient))
	for clientID := range byClient {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)
	result := make([]PositionAccountingRounding, 0, len(clientIDs))
	for _, clientID := range clientIDs {
		total := byClient[clientID]
		amount, ok := truncateNumerator(total, precision)
		if !ok {
			return nil, false
		}
		remainder := new(big.Int).Sub(total, new(big.Int).Mul(big.NewInt(amount), big.NewInt(precision)))
		if !remainder.IsInt64() {
			return nil, false
		}
		result = append(result, PositionAccountingRounding{ClientID: clientID, Amount: amount, RemainderNumerator: remainder.Int64()})
	}
	return result, true
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
	if !fundingRate.MarkAvailable {
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
		positionValue := etypes.AbsMulDiv(pos.Size, markPrice, precision)
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
	if pm.accounting[clientID] != nil {
		delete(pm.accounting[clientID], positionKey{symbol, pos.PositionSide})
	}
}

// GetPositions returns all positions for a client keyed by symbol+side, for testing/debugging.
// Caller is responsible for concurrent safety.
func (pm *PositionManager) GetPositions(clientID uint64) map[positionKey]*Position {
	return pm.positions[clientID]
}

// Abs is the exported version of abs for testing.
func Abs(x int64) int64 { return abs(x) }
