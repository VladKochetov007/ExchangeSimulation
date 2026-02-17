package actor

import (
	"sync"

	"exchange_sim/exchange"
)

type SharedContext struct {
	mu sync.RWMutex

	baseBalances map[string]int64
	quoteBalance int64
	reservedQuote int64

	compositeOMS map[string]*NettingOMS
	actorOMS     map[uint64]map[string]*NettingOMS
}

func NewSharedContext() *SharedContext {
	return &SharedContext{
		baseBalances: make(map[string]int64),
		compositeOMS: make(map[string]*NettingOMS),
		actorOMS:     make(map[uint64]map[string]*NettingOMS),
	}
}

func (sc *SharedContext) InitializeBalances(baseBalances map[string]int64, quoteBalance int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for asset, balance := range baseBalances {
		sc.baseBalances[asset] = balance
	}
	sc.quoteBalance = quoteBalance
}

func (sc *SharedContext) GetBaseBalance(asset string) int64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.baseBalances[asset]
}

func (sc *SharedContext) GetQuoteBalance() int64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.quoteBalance
}

func (sc *SharedContext) GetAvailableQuote() int64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.quoteBalance - sc.reservedQuote
}

func (sc *SharedContext) CanReserveQuote(amount int64) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return (sc.quoteBalance - sc.reservedQuote) >= amount
}

func (sc *SharedContext) ReserveQuote(amount int64) bool {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if (sc.quoteBalance - sc.reservedQuote) < amount {
		return false
	}
	sc.reservedQuote += amount
	return true
}

func (sc *SharedContext) ReleaseQuote(amount int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.reservedQuote -= amount
	if sc.reservedQuote < 0 {
		sc.reservedQuote = 0
	}
}

func (sc *SharedContext) UpdateBalances(baseAsset string, baseDelta int64, quoteDelta int64) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if baseDelta != 0 {
		sc.baseBalances[baseAsset] += baseDelta
	}
	sc.quoteBalance += quoteDelta
}

func (sc *SharedContext) GetCompositeOMS(symbol string) *NettingOMS {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.compositeOMS[symbol] == nil {
		sc.compositeOMS[symbol] = NewNettingOMS()
	}
	return sc.compositeOMS[symbol]
}

func (sc *SharedContext) GetActorOMS(actorID uint64, symbol string) *NettingOMS {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.actorOMS[actorID] == nil {
		sc.actorOMS[actorID] = make(map[string]*NettingOMS)
	}
	if sc.actorOMS[actorID][symbol] == nil {
		sc.actorOMS[actorID][symbol] = NewNettingOMS()
	}
	return sc.actorOMS[actorID][symbol]
}

func (sc *SharedContext) CanSubmitOrder(actorID uint64, symbol string, side exchange.Side, qty int64, maxInventory int64) bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	actorOMS := sc.actorOMS[actorID]
	if actorOMS == nil || actorOMS[symbol] == nil {
		return qty <= maxInventory
	}

	currentPos := actorOMS[symbol].GetNetPosition(symbol)

	if side == exchange.Buy {
		return currentPos+qty <= maxInventory
	}
	return currentPos-qty >= -maxInventory
}

func (sc *SharedContext) OnFill(actorID uint64, symbol string, fill OrderFillEvent, precision int64, baseAsset string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.compositeOMS[symbol] == nil {
		sc.compositeOMS[symbol] = NewNettingOMS()
	}
	sc.compositeOMS[symbol].OnFill(symbol, fill, precision)

	if sc.actorOMS[actorID] == nil {
		sc.actorOMS[actorID] = make(map[string]*NettingOMS)
	}
	if sc.actorOMS[actorID][symbol] == nil {
		sc.actorOMS[actorID][symbol] = NewNettingOMS()
	}
	sc.actorOMS[actorID][symbol].OnFill(symbol, fill, precision)

	notional := (fill.Qty * fill.Price) / precision
	if fill.Side == exchange.Buy {
		sc.baseBalances[baseAsset] += fill.Qty
		sc.quoteBalance -= notional + fill.FeeAmount
	} else {
		sc.baseBalances[baseAsset] -= fill.Qty
		sc.quoteBalance += notional - fill.FeeAmount
	}

	// SharedContext is a local approximation; the exchange is authoritative and
	// guarantees non-negative balances via liquidation. Clamp to match that invariant.
	if sc.quoteBalance < 0 {
		sc.quoteBalance = 0
	}
	if sc.baseBalances[baseAsset] < 0 {
		sc.baseBalances[baseAsset] = 0
	}
}
