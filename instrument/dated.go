package instrument

import (
	etypes "exchange_sim/types"
)

// ExpiringFutures is a cash-settled dated futures contract. It shares the
// perpetual's margin model, settlement, and liquidation semantics (it embeds
// PerpFutures) but pays no funding — IsPerp() reports false, which excludes
// it from the funding sweep — and cash-settles every position at expiry
// against a TWAP of the underlying index observed over ObservationWindowNano.
type ExpiringFutures struct {
	PerpFutures
	expiryNano int64
	// Underlying is the spot symbol whose index settles the contract.
	Underlying string
	// DeliveryFeeBps is charged on settlement notional at expiry.
	DeliveryFeeBps int64

	observer settlementObserver
}

func NewExpiringFutures(symbol, base, quote string, basePrecision, quotePrecision, tickSize, minOrderSize, expiryNano int64) *ExpiringFutures {
	return &ExpiringFutures{
		PerpFutures: *NewPerpFutures(symbol, base, quote, basePrecision, quotePrecision, tickSize, minOrderSize),
		expiryNano:  expiryNano,
		observer:    settlementObserver{windowNano: 60 * 1e9},
	}
}

// SetObservationWindow overrides the settlement TWAP lookback (default 60s).
func (f *ExpiringFutures) SetObservationWindow(windowNano int64) {
	f.observer.windowNano = windowNano
}

func (f *ExpiringFutures) IsPerp() bool           { return false }
func (f *ExpiringFutures) InstrumentType() string { return "FUTURE" }

// Perp exposes the embedded perpetual so the exchange's mark-price and
// liquidation machinery can treat dated futures like any margined book.
func (f *ExpiringFutures) Perp() *PerpFutures { return &f.PerpFutures }

func (f *ExpiringFutures) ExpiryNano() int64        { return f.expiryNano }
func (f *ExpiringFutures) UnderlyingSymbol() string { return f.Underlying }

func (f *ExpiringFutures) ObserveSettlement(price, tsNano int64) {
	f.observer.observe(price, tsNano)
}

// SettlementPrice averages the observations inside the window; with no window
// samples it uses the last declared underlying observation. It never samples a
// funding mark, last trade, or numeric zero at settlement time.
func (f *ExpiringFutures) SettlementPrice() (int64, error) {
	return f.observer.settlementPrice()
}

// ExpiryCashFlow marks the position from entry to settlement: the standard
// futures cash settlement (size signed, long positive).
func (f *ExpiringFutures) ExpiryCashFlow(size, entryPrice, settlementPrice, basePrecision int64) int64 {
	return etypes.MulDiv(size, settlementPrice-entryPrice, basePrecision)
}

func (f *ExpiringFutures) DeliveryFee(size, settlementPrice, basePrecision int64) int64 {
	if f.DeliveryFeeBps <= 0 {
		return 0
	}
	if size < 0 {
		size = -size
	}
	return etypes.MulDiv(size, settlementPrice, basePrecision) * f.DeliveryFeeBps / 10000
}

var _ etypes.Expirable = (*ExpiringFutures)(nil)
var _ etypes.Margined = (*ExpiringFutures)(nil)
var _ etypes.Settleable = (*ExpiringFutures)(nil)
