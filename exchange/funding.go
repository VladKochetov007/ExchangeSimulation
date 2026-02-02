package exchange

type PositionManager struct {
	positions map[uint64]map[string]*Position
	clock     Clock
}

func NewPositionManager(clock Clock) *PositionManager {
	return &PositionManager{
		positions: make(map[uint64]map[string]*Position),
		clock:     clock,
	}
}

func (pm *PositionManager) GetPosition(clientID uint64, symbol string) *Position {
	if pm.positions[clientID] == nil {
		return nil
	}
	return pm.positions[clientID][symbol]
}

func (pm *PositionManager) UpdatePosition(clientID uint64, symbol string, qty int64, price int64, side Side) {
	if pm.positions[clientID] == nil {
		pm.positions[clientID] = make(map[string]*Position)
	}

	pos := pm.positions[clientID][symbol]
	if pos == nil {
		pos = &Position{
			ClientID:   clientID,
			Symbol:     symbol,
			Size:       0,
			EntryPrice: 0,
			Margin:     0,
		}
		pm.positions[clientID][symbol] = pos
	}

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

func (pm *PositionManager) SettleFunding(clients map[uint64]*Client, perp *PerpFutures) {
	fundingRate := perp.GetFundingRate()
	for clientID, clientPos := range pm.positions {
		pos := clientPos[perp.Symbol()]
		if pos == nil || pos.Size == 0 {
			continue
		}

		client := clients[clientID]
		if client == nil {
			continue
		}

		positionValue := abs(pos.Size) * pos.EntryPrice / 100000000
		funding := (positionValue * fundingRate.Rate) / 10000

		if pos.Size > 0 {
			client.SubBalance(perp.QuoteAsset(), funding)
		} else {
			client.AddBalance(perp.QuoteAsset(), funding)
		}
	}

	fundingRate.NextFunding = pm.clock.NowUnixNano() + (fundingRate.Interval * 1e9)
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
