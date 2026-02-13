package exchange

type BalanceChangeTracker struct {
	exchange *Exchange
}

func (t *BalanceChangeTracker) LogBalanceChange(
	timestamp int64,
	clientID uint64,
	symbol string,
	reason string,
	changes []BalanceDelta,
) {
	// Route to symbol logger if symbol provided, otherwise to _global
	logKey := "_global"
	if symbol != "" {
		logKey = symbol
	}
	log := t.exchange.getLogger(logKey)
	if log == nil {
		return
	}

	log.LogEvent(timestamp, clientID, "balance_change", BalanceChangeEvent{
		Timestamp: timestamp,
		ClientID:  clientID,
		Symbol:    symbol,
		Reason:    reason,
		Changes:   changes,
	})
}

func spotDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{
		Asset:      asset,
		Wallet:     "spot",
		OldBalance: old,
		NewBalance: new,
		Delta:      new - old,
	}
}

func perpDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{
		Asset:      asset,
		Wallet:     "perp",
		OldBalance: old,
		NewBalance: new,
		Delta:      new - old,
	}
}

func reservedSpotDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{
		Asset:      asset,
		Wallet:     "reserved_spot",
		OldBalance: old,
		NewBalance: new,
		Delta:      new - old,
	}
}

func reservedPerpDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{
		Asset:      asset,
		Wallet:     "reserved_perp",
		OldBalance: old,
		NewBalance: new,
		Delta:      new - old,
	}
}

func borrowedDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{
		Asset:      asset,
		Wallet:     "borrowed",
		OldBalance: old,
		NewBalance: new,
		Delta:      new - old,
	}
}
