package exchange

import etypes "exchange_sim/types"

// VenueBucket names one of the exchange's own balances.
type VenueBucket string

const (
	// VenueFeeRevenue is what the exchange charged for trading, financing and
	// anything else it takes from participants.
	VenueFeeRevenue VenueBucket = "fee_revenue"
	// VenueInsuranceFund absorbs the deficit of a bankrupt account and is paid
	// by liquidation clearance fees. It goes negative when a deficit exceeds
	// what it holds, which is the one place a venue can create money.
	VenueInsuranceFund VenueBucket = "insurance_fund"
)

// VenueBalanceEvent records a movement of the exchange's own money.
//
// It exists because the exchange is a participant in its own market: every
// fee it charges, every unit of interest it collects and every remainder it
// keeps is money that left somebody's account. Without a record of the
// receiving side, a conservation audit has to read the exchange's balance from
// its own summary report, which means the identity closes on that term by
// construction and can never test it.
type VenueBalanceEvent struct {
	Timestamp  int64       `json:"timestamp"`
	Sequence   uint64      `json:"sequence"`
	Bucket     VenueBucket `json:"bucket"`
	Asset      string      `json:"asset"`
	Symbol     string      `json:"symbol,omitempty"`
	TradeID    uint64      `json:"trade_id"`
	Reason     string      `json:"reason"`
	OldBalance int64       `json:"old_balance"`
	NewBalance int64       `json:"new_balance"`
	Delta      int64       `json:"delta"`
}

// VenueBalanceSequenceForReport returns the terminal journal sequence used by
// the simulator's independently captured venue ledger. It is deliberately a
// read-only report adapter; callers cannot mutate the sequence outside the
// venue movement path.
func (e *DefaultExchange) VenueBalanceSequenceForReport() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.venueBalanceSequence
}

// moveVenueBalance is the only way the exchange's own money moves.
//
// Every call is recorded, and the addition is checked: a balance one unit past
// the int64 ceiling would wrap to a large negative number and read as a debt
// rather than as an error, so a wrap stops the venue instead of being
// discovered later as a sign flip.
//
// Caller must hold e.mu.Lock() when mutating exchange state.
func (e *DefaultExchange) moveVenueBalance(bucket VenueBucket, asset string, delta int64, timestamp int64, symbol, reason string) {
	e.moveVenueBalanceWithTradeID(bucket, asset, delta, timestamp, symbol, 0, reason)
}

func (e *DefaultExchange) moveVenueBalanceWithTradeID(bucket VenueBucket, asset string, delta int64, timestamp int64, symbol string, tradeID uint64, reason string) {
	if delta == 0 || asset == "" {
		return
	}
	balances := e.ExchangeBalance.FeeRevenue
	if bucket == VenueInsuranceFund {
		balances = e.ExchangeBalance.InsuranceFund
	}
	old := balances[asset]
	updated := etypes.AddAmount(old, delta)
	balances[asset] = updated
	e.venueBalanceSequence++
	sequence := e.venueBalanceSequence
	e.conservation.recordVenue(asset, delta)

	log := e.getLogger(symbol)
	if log == nil {
		log = e.getLogger("_global")
	}
	if log == nil {
		return
	}
	log.LogEvent(timestamp, 0, "venue_balance_change", VenueBalanceEvent{
		Timestamp: timestamp, Sequence: sequence, Bucket: bucket, Asset: asset, Symbol: symbol,
		TradeID: tradeID, Reason: reason, OldBalance: old, NewBalance: updated, Delta: delta,
	})
}

// MoveVenueBalanceForTest exposes the ledger's single mutation path to tests
// in other packages, which is how the wrap guard is exercised without making
// the path itself public to the rest of the engine.
func (e *DefaultExchange) MoveVenueBalanceForTest(bucket VenueBucket, asset string, delta, timestamp int64, symbol, reason string) {
	e.moveVenueBalance(bucket, asset, delta, timestamp, symbol, reason)
}
