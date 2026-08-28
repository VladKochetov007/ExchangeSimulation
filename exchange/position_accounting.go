package exchange

import (
	"math"
	"math/big"

	etypes "exchange_sim/types"
)

// positionAccounting stores a signed aggregate cost in the price×quantity
// lattice. It preserves average-cost semantics while avoiding a rational
// denominator that grows after every partial reduction.
type positionAccounting struct {
	size              int64
	basisNumerator    big.Int
	realizedNumerator big.Int
	realizedCash      int64
	// carryNumerator is the sub-cash-unit remainder from closed lifecycles;
	// it survives a flat position until expiry drains it into the ledger.
	carryNumerator int64
}

func newPositionAccounting() *positionAccounting {
	return &positionAccounting{}
}

func (accounting *positionAccounting) clone() *positionAccounting {
	copy := newPositionAccounting()
	copy.size = accounting.size
	copy.basisNumerator.Set(&accounting.basisNumerator)
	copy.realizedNumerator.Set(&accounting.realizedNumerator)
	copy.realizedCash = accounting.realizedCash
	copy.carryNumerator = accounting.carryNumerator
	return copy
}

// applyTrade performs the accounting half of one position transition. Basis
// allocation is deterministic toward zero; PnL cash is the change in the
// cumulative toward-zero exact numerator, so sub-unit event rounding cannot
// accumulate with the number of fills.
func (accounting *positionAccounting) applyTrade(oldSize, quantity, price int64, tradeSide Side, precision int64) (int64, bool) {
	if quantity <= 0 || precision <= 0 || accounting.size != oldSize {
		return 0, false
	}

	delta := quantity
	if tradeSide == Sell {
		delta = -quantity
	}
	if oldSize == 0 {
		accounting.resetCurrentLifecycle()
		accounting.addBasis(delta, price)
		accounting.size = delta
		return 0, true
	}

	oldMagnitude := abs(oldSize)
	deltaMagnitude := abs(delta)
	if oldSize > 0 && delta > 0 || oldSize < 0 && delta < 0 {
		accounting.addBasis(delta, price)
		accounting.size = oldSize + delta
		return 0, true
	}

	newSize := oldSize + delta
	newMagnitude := abs(newSize)
	oldBasis := new(big.Int).Set(&accounting.basisNumerator)
	sameDirection := oldSize > 0 && newSize > 0 || oldSize < 0 && newSize < 0
	var newBasis big.Int
	if sameDirection {
		basisProduct := new(big.Int).Mul(oldBasis, big.NewInt(newMagnitude))
		newBasis.Quo(basisProduct, big.NewInt(oldMagnitude))
	}

	// Exact realized numerator in price×quantity units. For a non-flipping
	// close, newBasis is the deterministic integer-lattice remaining basis. A
	// flip first closes the old position completely, then starts a new basis.
	var tradeValue big.Int
	realizedDelta := delta
	if !sameDirection {
		realizedDelta = -oldSize
	}
	tradeValue.Mul(big.NewInt(realizedDelta), big.NewInt(price))
	var realizedNumerator big.Int
	realizedNumerator.Sub(&newBasis, oldBasis)
	realizedNumerator.Sub(&realizedNumerator, &tradeValue)
	accounting.realizedNumerator.Add(&accounting.realizedNumerator, &realizedNumerator)
	var totalRealizedNumerator big.Int
	totalRealizedNumerator.Add(&accounting.realizedNumerator, big.NewInt(accounting.carryNumerator))
	targetCash, ok := truncateNumerator(&totalRealizedNumerator, precision)
	if !ok {
		panic("position accounting: realized PnL overflows int64")
	}
	realizedCash, ok := etypes.TrySub(targetCash, accounting.realizedCash)
	if !ok {
		panic("position accounting: realized PnL delta overflows int64")
	}
	accounting.realizedCash = targetCash

	if newMagnitude == 0 {
		accounting.finishLifecycle(&totalRealizedNumerator, targetCash, precision)
		return realizedCash, true
	}
	if deltaMagnitude > oldMagnitude {
		accounting.finishLifecycle(&totalRealizedNumerator, targetCash, precision)
		accounting.addBasis(newSize, price)
		accounting.size = newSize
		return realizedCash, true
	}
	accounting.basisNumerator.Set(&newBasis)
	accounting.size = newSize
	return realizedCash, true
}

func (accounting *positionAccounting) addBasis(quantity, price int64) {
	var tradeValue big.Int
	tradeValue.Mul(big.NewInt(quantity), big.NewInt(price))
	accounting.basisNumerator.Add(&accounting.basisNumerator, &tradeValue)
}

func (accounting *positionAccounting) entryPrice() (int64, bool) {
	if accounting.size == 0 {
		return 0, true
	}
	basis := new(big.Int).Set(&accounting.basisNumerator)
	price := new(big.Int).Quo(basis, big.NewInt(accounting.size))
	if !price.IsInt64() {
		return 0, false
	}
	return price.Int64(), true
}

func (accounting *positionAccounting) unrealizedPnL(markPrice, precision int64) (int64, bool) {
	var markedValue big.Int
	markedValue.Mul(big.NewInt(accounting.size), big.NewInt(markPrice))
	markedValue.Sub(&markedValue, &accounting.basisNumerator)
	var totalNumerator big.Int
	totalNumerator.Add(&accounting.realizedNumerator, &markedValue)
	totalNumerator.Add(&totalNumerator, big.NewInt(accounting.carryNumerator))
	targetCash, ok := truncateNumerator(&totalNumerator, precision)
	if !ok {
		return 0, false
	}
	return etypes.TrySub(targetCash, accounting.realizedCash)
}

func (accounting *positionAccounting) liquidationPrice(netBalance, precision int64) (int64, bool) {
	if accounting.size == 0 || precision <= 0 {
		return 0, false
	}
	// Equity is netBalance + trunc(T/precision) - realizedCash, where
	// T=realizedNumerator+size×mark-basis+carry. For q=realizedCash−netBalance,
	// trunc(T/P) <= q ends at (q+1)P−1 when q is non-negative and at qP when
	// q is negative. Solving that integer inequality avoids treating a
	// toward-zero quotient as a valid boundary when T/P is non-integral.
	q := new(big.Int).Sub(big.NewInt(accounting.realizedCash), big.NewInt(netBalance))
	threshold := new(big.Int)
	precisionBig := big.NewInt(precision)
	if q.Sign() >= 0 {
		threshold.Add(q, big.NewInt(1)).Mul(threshold, precisionBig).Sub(threshold, big.NewInt(1))
	} else {
		threshold.Mul(q, precisionBig)
	}
	constant := new(big.Int).Sub(&accounting.realizedNumerator, &accounting.basisNumerator)
	constant.Add(constant, big.NewInt(accounting.carryNumerator))
	numerator := new(big.Int).Sub(threshold, constant)
	denominator := big.NewInt(accounting.size)
	var mark *big.Int
	if accounting.size > 0 {
		mark = floorQuotient(numerator, denominator)
	} else {
		mark = ceilQuotient(numerator, denominator)
	}
	if !mark.IsInt64() {
		return 0, false
	}
	candidate := mark.Int64()
	equityAt := func(markPrice int64) (int64, bool) {
		unrealized, ok := accounting.unrealizedPnL(markPrice, precision)
		if !ok {
			return 0, false
		}
		return etypes.TryAdd(netBalance, unrealized)
	}
	if accounting.size > 0 {
		equity, ok := equityAt(candidate)
		if !ok || equity > 0 {
			return 0, false
		}
		if candidate == math.MaxInt64 {
			return candidate, true
		}
		nextEquity, nextOK := equityAt(candidate + 1)
		if nextOK && nextEquity > 0 {
			return candidate, true
		}
		return 0, false
	}
	equity, ok := equityAt(candidate)
	if !ok || equity > 0 {
		return 0, false
	}
	if candidate == math.MinInt64 {
		return candidate, true
	}
	previousEquity, previousOK := equityAt(candidate - 1)
	if previousOK && previousEquity > 0 {
		return candidate, true
	}
	return 0, false
}

func floorQuotient(numerator, denominator *big.Int) *big.Int {
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 && numerator.Sign() != denominator.Sign() {
		quotient.Sub(quotient, big.NewInt(1))
	}
	return quotient
}

func ceilQuotient(numerator, denominator *big.Int) *big.Int {
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 && numerator.Sign() == denominator.Sign() {
		quotient.Add(quotient, big.NewInt(1))
	}
	return quotient
}

func (accounting *positionAccounting) resetCurrentLifecycle() {
	accounting.size = 0
	accounting.basisNumerator.SetInt64(0)
	accounting.realizedNumerator.SetInt64(0)
	accounting.realizedCash = 0
}

func (accounting *positionAccounting) finishLifecycle(totalNumerator *big.Int, targetCash, precision int64) {
	if precision <= 0 {
		panic("position accounting: invalid lifecycle precision")
	}
	residual := new(big.Int).Sub(totalNumerator, new(big.Int).Mul(big.NewInt(targetCash), big.NewInt(precision)))
	if !residual.IsInt64() {
		panic("position accounting: rounding remainder overflows int64")
	}
	accounting.carryNumerator = residual.Int64()
	accounting.resetCurrentLifecycle()
}

func truncateNumerator(numerator *big.Int, precision int64) (int64, bool) {
	if precision <= 0 {
		return 0, false
	}
	quotient := new(big.Int).Quo(numerator, big.NewInt(precision))
	if !quotient.IsInt64() {
		return 0, false
	}
	return quotient.Int64(), true
}
