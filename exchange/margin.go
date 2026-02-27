package exchange

import "errors"

func (e *Exchange) SetMarginMode(clientID uint64, mode MarginMode) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	if e.hasOpenPositions(client) {
		return errors.New("cannot change margin mode with open positions")
	}

	client.MarginMode = mode
	return nil
}

func (e *Exchange) hasOpenPositions(client *Client) bool {
	return e.Positions.HasOpenPositions(client.ID)
}

func (e *Exchange) AllocateCollateralToPosition(
	clientID uint64,
	symbol string,
	asset string,
	amount int64,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
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

	timestamp := e.Clock.NowUnixNano()
	oldPerp := client.PerpBalances[asset]
	client.PerpBalances[asset] -= amount
	client.IsolatedPositions[symbol].Collateral[asset] += amount

	logBalanceChange(e, timestamp, clientID, symbol, "allocate_collateral", []BalanceDelta{
		perpDelta(asset, oldPerp, client.PerpBalances[asset]),
	})

	return nil
}

func (e *Exchange) ReleaseCollateralFromPosition(
	clientID uint64,
	symbol string,
	asset string,
	amount int64,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	client := e.Clients[clientID]
	if client == nil {
		return errors.New("unknown client")
	}

	isolated := client.IsolatedPositions[symbol]
	if isolated == nil || isolated.Collateral[asset] < amount {
		return errors.New("insufficient isolated collateral")
	}

	timestamp := e.Clock.NowUnixNano()
	oldPerp := client.PerpBalances[asset]
	isolated.Collateral[asset] -= amount
	client.PerpBalances[asset] += amount

	logBalanceChange(e, timestamp, clientID, symbol, "release_collateral", []BalanceDelta{
		perpDelta(asset, oldPerp, client.PerpBalances[asset]),
	})

	return nil
}
