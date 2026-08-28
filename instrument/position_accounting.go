package instrument

import etypes "exchange_sim/types"

func updatePositionWithAccounting(store etypes.PositionStore, requireExact bool, clientID uint64, symbol string, qty, price int64, tradeSide etypes.Side, posSide etypes.PositionSide) (etypes.PositionDelta, etypes.PositionAccountingDelta) {
	if exact, ok := store.(etypes.ExactLinearPositionStore); ok {
		delta, accounting := exact.UpdatePositionWithAccounting(clientID, symbol, qty, price, tradeSide, posSide)
		if !accounting.Valid && requireExact {
			panic("instrument: exact linear position accounting unavailable")
		}
		return delta, accounting
	}
	if requireExact {
		panic("instrument: exact linear position store required")
	}
	return store.UpdatePosition(clientID, symbol, qty, price, tradeSide, posSide), etypes.PositionAccountingDelta{}
}

func preflightPositionAccounting(store etypes.PositionStore, requireExact bool, updates ...positionAccountingUpdate) {
	if !requireExact {
		return
	}
	exact, ok := store.(etypes.ExactLinearPositionStore)
	if !ok {
		panic("instrument: exact linear position store required")
	}
	for _, update := range updates {
		if !exact.CanUpdatePositionWithAccounting(update.clientID, update.symbol, update.qty, update.price, update.tradeSide, update.posSide) {
			panic("instrument: exact linear position transition unavailable")
		}
	}
}

type positionAccountingUpdate struct {
	clientID  uint64
	symbol    string
	qty       int64
	price     int64
	tradeSide etypes.Side
	posSide   etypes.PositionSide
}
