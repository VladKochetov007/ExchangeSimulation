package exchange

import "errors"

type MarginModeManager struct {
	exchange *Exchange
}

func NewMarginModeManager(exchange *Exchange) *MarginModeManager {
	return &MarginModeManager{exchange: exchange}
}

func (m *MarginModeManager) SetMarginMode(clientID uint64, mode MarginMode) error {
	m.exchange.mu.Lock()
	defer m.exchange.mu.Unlock()

	client := m.exchange.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	if m.hasOpenPositions(client) {
		return errors.New("cannot change margin mode with open positions")
	}

	client.MarginMode = mode
	return nil
}

func (m *MarginModeManager) hasOpenPositions(client *Client) bool {
	m.exchange.Positions.mu.RLock()
	defer m.exchange.Positions.mu.RUnlock()

	positions := m.exchange.Positions.positions[client.ID]
	if positions == nil {
		return false
	}

	for _, pos := range positions {
		if pos != nil && pos.Size != 0 {
			return true
		}
	}
	return false
}

func (m *MarginModeManager) AllocateCollateralToPosition(
	clientID uint64,
	symbol string,
	asset string,
	amount int64,
) error {
	m.exchange.mu.Lock()
	defer m.exchange.mu.Unlock()

	client := m.exchange.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	if client.MarginMode != IsolatedMargin {
		return errors.New("client not in isolated margin mode")
	}

	if client.PerpAvailable(asset) < amount {
		return errors.New("insufficient perp balance")
	}

	if client.IsolatedPositions[symbol] == nil {
		client.IsolatedPositions[symbol] = &IsolatedPosition{
			Symbol:     symbol,
			Collateral: make(map[string]int64),
			Borrowed:   make(map[string]int64),
		}
	}

	timestamp := m.exchange.Clock.NowUnixNano()
	oldPerp := client.PerpBalances[asset]
	client.PerpBalances[asset] -= amount
	client.IsolatedPositions[symbol].Collateral[asset] += amount

	m.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, symbol, "allocate_collateral", []BalanceDelta{
		perpDelta(asset, oldPerp, client.PerpBalances[asset]),
	})

	return nil
}

func (m *MarginModeManager) ReleaseCollateralFromPosition(
	clientID uint64,
	symbol string,
	asset string,
	amount int64,
) error {
	m.exchange.mu.Lock()
	defer m.exchange.mu.Unlock()

	client := m.exchange.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	isolated := client.IsolatedPositions[symbol]
	if isolated == nil || isolated.Collateral[asset] < amount {
		return errors.New("insufficient isolated collateral")
	}

	timestamp := m.exchange.Clock.NowUnixNano()
	oldPerp := client.PerpBalances[asset]
	isolated.Collateral[asset] -= amount
	client.PerpBalances[asset] += amount

	m.exchange.balanceTracker.LogBalanceChange(timestamp, clientID, symbol, "release_collateral", []BalanceDelta{
		perpDelta(asset, oldPerp, client.PerpBalances[asset]),
	})

	return nil
}
