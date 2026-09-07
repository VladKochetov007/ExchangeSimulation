package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// H-027, refined. The original hypothesis was that an option and its hedge are
// valued against two different prices at expiry. Reading the code corrected it
// before the experiment ran, and the correction is the interesting part.
//
// UpdateDerivativeMarks resolves ONE underlying observation per tick through
// derivativeUnderlyingPrice — the spot book — and hands that same number to
// every expirable via ObserveSettlement. A perp, by contrast, is marked at its
// own book. So:
//
//   - hedging an expiring option with a PERP leaves genuine basis risk, because
//     the option settles to spot while the perp stays open at its own mark. That
//     is real economics, not a venue artefact, and is not a defect.
//   - hedging it with a DATED FUTURE expiring at the same instant nets exactly,
//     because both legs settle against the single shared observation.
//
// The second is a load-bearing invariant that nothing else asserts: if the
// shared observation were ever replaced by per-instrument sampling, calendar
// hedges would silently stop netting and only hedgers would pay for it. This
// test pins it.
func TestAuditExpiringContractsShareOneSettlementObservation(t *testing.T) {
	clock := &testClock{now: time.Now().UnixNano()}
	ex := NewExchange(4, clock)

	// A spot book to be the underlying reference, and two expirables on it that
	// reach expiry at the same instant.
	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)
	expiry := clock.now + int64(time.Minute)
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, expiry)
	// NewExpiringFutures takes no underlying, so a bare construction leaves it
	// empty and derivativeUnderlyingPrice falls through to the configured index
	// instead of the spot book. The listing scheduler sets it
	// (instrument/listing.go:97) and the campaign lists dated futures only
	// through that path, so the fixture must do the same or it tests a state the
	// system never reaches.
	future.Underlying = "ABC/USD"
	option := NewEuropeanOption("ABC-C", "ABC", "USD", "ABC/USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, USDAmount(100), expiry, true)
	ex.AddInstrument(future)
	ex.AddInstrument(option)

	// Two participants make a two-sided spot market so the underlying has a
	// usable reference price.
	for _, id := range []uint64{1, 2} {
		ex.ConnectNewClient(id, map[string]int64{
			"USD": 1_000_000 * USD_PRECISION,
			"ABC": 1_000 * BTC_PRECISION,
		}, &FixedFee{})
	}
	if _, reject := InjectLimitOrder(ex, 1, "ABC/USD", Buy, USDAmount(99), BTCAmount(1)); reject != "" {
		t.Fatalf("bid rejected: %s", reject)
	}
	if _, reject := InjectLimitOrder(ex, 2, "ABC/USD", Sell, USDAmount(101), BTCAmount(1)); reject != "" {
		t.Fatalf("ask rejected: %s", reject)
	}

	ex.UpdateDerivativeMarks()

	futurePrice, futureErr := future.SettlementPrice()
	optionPrice, optionErr := option.SettlementPrice()
	if futureErr != nil || optionErr != nil {
		t.Fatalf("no settlement observation: future %v, option %v", futureErr, optionErr)
	}
	t.Logf("one underlying observation, two expirables: future settles at %d, option at %d",
		futurePrice, optionPrice)

	if futurePrice != optionPrice {
		t.Errorf("the two expirables sampled different underlying prices (%d vs %d): a calendar hedge "+
			"between them no longer nets, and only hedgers pay for it", futurePrice, optionPrice)
	}
	// And that shared price is the spot mid, not either derivative's own book.
	if want := USDAmount(100); futurePrice != want {
		t.Errorf("the shared observation is %d, want the spot mid %d", futurePrice, want)
	}
}
