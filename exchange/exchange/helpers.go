package exchange

import ebook "exchange_sim/exchange/book"

// btcPrecision is the number of satoshis per bitcoin.
// Used in collateral calculations. TODO: use instrument.BasePrecision() instead.
const btcPrecision = 100_000_000

func newBook(side Side) *Book { return ebook.NewBook(side) }

func visibleQty(limit *Limit) int64 { return ebook.VisibleQty(limit) }

func logBalanceChange(ex *Exchange, timestamp int64, clientID uint64, symbol, reason string, changes []BalanceDelta) {
	logKey := "_global"
	if symbol != "" {
		logKey = symbol
	}
	log := ex.getLogger(logKey)
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
	return BalanceDelta{Asset: asset, Wallet: "spot", OldBalance: old, NewBalance: new, Delta: new - old}
}

func perpDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{Asset: asset, Wallet: "perp", OldBalance: old, NewBalance: new, Delta: new - old}
}

func reservedSpotDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{Asset: asset, Wallet: "reserved_spot", OldBalance: old, NewBalance: new, Delta: new - old}
}

func reservedPerpDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{Asset: asset, Wallet: "reserved_perp", OldBalance: old, NewBalance: new, Delta: new - old}
}

func borrowedDelta(asset string, old, new int64) BalanceDelta {
	return BalanceDelta{Asset: asset, Wallet: "borrowed", OldBalance: old, NewBalance: new, Delta: new - old}
}

// ReservedSpotDelta builds a BalanceDelta for the reserved_spot wallet.
func ReservedSpotDelta(asset string, old, new int64) BalanceDelta {
	return reservedSpotDelta(asset, old, new)
}
