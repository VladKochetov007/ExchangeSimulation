package analysis

import (
	"fmt"
	"math"
	"math/big"
)

// exactPositionReplay is an independent reconstruction of the simulator's
// integer cost-basis state. The persisted position update contains the trade
// inputs, so the analyzer can validate both the public rounded entry price and
// the exact marked cash without trusting either the account report or the
// simulator's private store.
type exactPositionReplay struct {
	size              int64
	basisNumerator    big.Int
	realizedNumerator big.Int
	realizedCash      int64
	carryNumerator    int64
}

type exactPositionTrade struct {
	OldSize       int64
	OldEntryPrice int64
	NewSize       int64
	NewEntryPrice int64
	TradeQty      int64
	TradePrice    int64
	TradeSide     string
	PositionSide  string
}

func validPositionSide(side string) bool {
	return side == "BOTH" || side == "LONG" || side == "SHORT"
}

func (replay *exactPositionReplay) apply(trade exactPositionTrade, precision int64) (int64, error) {
	if precision <= 0 {
		return 0, fmt.Errorf("invalid precision %d", precision)
	}
	if trade.TradeQty <= 0 || trade.TradeQty == math.MinInt64 {
		return 0, fmt.Errorf("invalid trade quantity %d", trade.TradeQty)
	}
	if trade.TradeSide != "BUY" && trade.TradeSide != "SELL" {
		return 0, fmt.Errorf("invalid trade side %q", trade.TradeSide)
	}
	if trade.PositionSide != "" && !validPositionSide(trade.PositionSide) {
		return 0, fmt.Errorf("invalid position side %q", trade.PositionSide)
	}
	if trade.OldSize == math.MinInt64 || trade.OldSize != replay.size {
		return 0, fmt.Errorf("old size %d does not follow replay size %d", trade.OldSize, replay.size)
	}
	oldEntry, ok := replay.entryPrice()
	if !ok {
		return 0, fmt.Errorf("old entry price is unrepresentable")
	}
	if trade.OldEntryPrice != oldEntry {
		return 0, fmt.Errorf("old entry price %d does not follow replay entry %d", trade.OldEntryPrice, oldEntry)
	}

	delta := trade.TradeQty
	if trade.TradeSide == "SELL" {
		delta = -delta
	}
	newSize, ok := exactAdd(replay.size, delta)
	if !ok || newSize == math.MinInt64 {
		return 0, fmt.Errorf("new size overflows int64")
	}
	if trade.PositionSide == "LONG" && (trade.OldSize < 0 || newSize < 0) {
		return 0, fmt.Errorf("long position crossed below zero")
	}
	if trade.PositionSide == "SHORT" && (trade.OldSize > 0 || newSize > 0) {
		return 0, fmt.Errorf("short position crossed above zero")
	}
	if replay.size == 0 {
		// A new trade starts a position lifecycle but must retain carry from a
		// previously flat lifecycle until the next terminal drain.
		replay.resetCurrentLifecycle()
		replay.addBasis(delta, trade.TradePrice)
		replay.size = delta
	} else if replay.size > 0 && delta > 0 || replay.size < 0 && delta < 0 {
		// Same-direction trades only extend the aggregate basis; they do not
		// realize PnL or touch the lifecycle carry.
		replay.addBasis(delta, trade.TradePrice)
		replay.size = newSize
	} else {
		realizedCash, ok := replay.applyNonOpeningTrade(delta, newSize, trade.TradePrice, precision)
		if !ok {
			return 0, fmt.Errorf("realized cash or lifecycle remainder is unrepresentable")
		}
		if replay.size != trade.NewSize {
			return realizedCash, fmt.Errorf("new size %d does not follow replay size %d", trade.NewSize, replay.size)
		}
		newEntry, ok := replay.entryPrice()
		if !ok {
			return realizedCash, fmt.Errorf("new entry price is unrepresentable")
		}
		if trade.NewEntryPrice != newEntry {
			return realizedCash, fmt.Errorf("new entry price %d does not follow replay entry %d", trade.NewEntryPrice, newEntry)
		}
		return realizedCash, nil
	}

	if replay.size != trade.NewSize {
		return 0, fmt.Errorf("new size %d does not follow replay size %d", trade.NewSize, replay.size)
	}
	newEntry, ok := replay.entryPrice()
	if !ok {
		return 0, fmt.Errorf("new entry price is unrepresentable")
	}
	if trade.NewEntryPrice != newEntry {
		return 0, fmt.Errorf("new entry price %d does not follow replay entry %d", trade.NewEntryPrice, newEntry)
	}
	return 0, nil
}

func (replay *exactPositionReplay) applyNonOpeningTrade(delta, newSize, price, precision int64) (int64, bool) {
	oldSize := replay.size
	oldMagnitude := exactMagnitude(oldSize)
	deltaMagnitude := exactMagnitude(delta)
	newMagnitude := exactMagnitude(newSize)
	sameDirection := oldSize > 0 && newSize > 0 || oldSize < 0 && newSize < 0
	var newBasis big.Int
	if sameDirection {
		newBasis.Mul(&replay.basisNumerator, big.NewInt(newMagnitude))
		newBasis.Quo(&newBasis, big.NewInt(oldMagnitude))
	}

	realizedDelta := delta
	if !sameDirection {
		realizedDelta = -oldSize
	}
	var tradeValue, realizedDeltaNumerator big.Int
	tradeValue.Mul(big.NewInt(realizedDelta), big.NewInt(price))
	realizedDeltaNumerator.Sub(&newBasis, &replay.basisNumerator)
	realizedDeltaNumerator.Sub(&realizedDeltaNumerator, &tradeValue)
	replay.realizedNumerator.Add(&replay.realizedNumerator, &realizedDeltaNumerator)

	var totalNumerator big.Int
	totalNumerator.Add(&replay.realizedNumerator, big.NewInt(replay.carryNumerator))
	targetCash, ok := truncateExactNumeratorChecked(&totalNumerator, precision)
	if !ok {
		return 0, false
	}
	realizedCash, ok := exactSub(targetCash, replay.realizedCash)
	if !ok {
		return 0, false
	}
	replay.realizedCash = targetCash

	if newMagnitude == 0 {
		return realizedCash, replay.finishLifecycle(&totalNumerator, targetCash, precision)
	}
	if deltaMagnitude > oldMagnitude {
		if !replay.finishLifecycle(&totalNumerator, targetCash, precision) {
			return 0, false
		}
		replay.addBasis(newSize, price)
		replay.size = newSize
		return realizedCash, true
	}
	replay.basisNumerator.Set(&newBasis)
	replay.size = newSize
	return realizedCash, true
}

func (replay *exactPositionReplay) entryPrice() (int64, bool) {
	if replay.size == 0 {
		return 0, true
	}
	var entry big.Int
	entry.Quo(&replay.basisNumerator, big.NewInt(replay.size))
	if !entry.IsInt64() {
		return 0, false
	}
	return entry.Int64(), true
}

func (replay *exactPositionReplay) unrealizedPnL(markPrice, precision int64) (int64, bool) {
	if precision <= 0 {
		return 0, false
	}
	var markedValue, totalNumerator big.Int
	markedValue.Mul(big.NewInt(replay.size), big.NewInt(markPrice))
	markedValue.Sub(&markedValue, &replay.basisNumerator)
	totalNumerator.Add(&replay.realizedNumerator, &markedValue)
	totalNumerator.Add(&totalNumerator, big.NewInt(replay.carryNumerator))
	targetCash, ok := truncateExactNumeratorChecked(&totalNumerator, precision)
	if !ok {
		return 0, false
	}
	return exactSub(targetCash, replay.realizedCash)
}

func (replay *exactPositionReplay) addBasis(quantity, price int64) {
	var value big.Int
	value.Mul(big.NewInt(quantity), big.NewInt(price))
	replay.basisNumerator.Add(&replay.basisNumerator, &value)
}

func (replay *exactPositionReplay) resetCurrentLifecycle() {
	replay.size = 0
	replay.basisNumerator.SetInt64(0)
	replay.realizedNumerator.SetInt64(0)
	replay.realizedCash = 0
}

func (replay *exactPositionReplay) finishLifecycle(totalNumerator *big.Int, targetCash, precision int64) bool {
	var paidNumerator big.Int
	paidNumerator.Mul(big.NewInt(targetCash), big.NewInt(precision))
	paidNumerator.Sub(totalNumerator, &paidNumerator)
	if !paidNumerator.IsInt64() {
		return false
	}
	replay.carryNumerator = paidNumerator.Int64()
	replay.resetCurrentLifecycle()
	return true
}

func exactMagnitude(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func truncateExactNumeratorChecked(numerator *big.Int, precision int64) (int64, bool) {
	if precision <= 0 {
		return 0, false
	}
	var quotient big.Int
	quotient.Quo(numerator, big.NewInt(precision))
	if !quotient.IsInt64() {
		return 0, false
	}
	return quotient.Int64(), true
}
