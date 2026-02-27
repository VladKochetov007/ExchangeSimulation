package exchange

import ebook "exchange_sim/book"

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

func buildBorrowContext(e *Exchange, client *Client, clientID uint64) BorrowContext {
	timestamp := e.Clock.NowUnixNano()
	return BorrowContext{
		Client:    client,
		ClientID:  clientID,
		Timestamp: timestamp,
		LogBalance: func(reason string, changes []BalanceDelta) {
			logBalanceChange(e, timestamp, clientID, "", reason, changes)
		},
		LogEvent: func(event string, data any) {
			if log := e.getLogger("_global"); log != nil {
				log.LogEvent(timestamp, clientID, event, data)
			}
		},
	}
}

func buildFundingSink(e *Exchange) fundingEventSink {
	return fundingEventSink{
		logBalance: func(timestamp int64, clientID uint64, symbol, reason string, changes []BalanceDelta) {
			logBalanceChange(e, timestamp, clientID, symbol, reason, changes)
		},
		recordRevenue: func(asset string, amount int64) {
			e.ExchangeBalance.FeeRevenue[asset] += amount
		},
	}
}
