package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

type settableOracle struct{ price int64 }

func (o *settableOracle) Price(string) int64 { return o.price }

func anchorTestExchange(t *testing.T, oracle *settableOracle, explicitCalc MarkPriceCalculator) (*Exchange, *PerpFutures) {
	t.Helper()
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(10_000_000))
	ex.AddPerpBalance(2, "USD", USDAmount(10_000_000))

	config := AutomationConfig{}
	if oracle != nil {
		config.IndexProvider = oracle
	}
	if explicitCalc != nil {
		config.MarkPriceCalc = explicitCalc
	}
	ex.ConfigureAutomation(config)

	// Perp book quotes far above the 50k index: bid 59,990 / ask 60,010.
	if _, reject := InjectLimitOrder(ex, 1, "BTC-PERP", Buy, PriceUSD(59_990, DOLLAR_TICK), BTCAmount(1)); reject != "" {
		t.Fatalf("bid rejected: %s", reject)
	}
	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Sell, PriceUSD(60_010, DOLLAR_TICK), BTCAmount(1)); reject != "" {
		t.Fatalf("ask rejected: %s", reject)
	}
	return ex, perp
}

// With an index configured and no explicit calculator, the DEFAULT mark must
// anchor to the index (ClampedEMA of the basis), not the perp's own mid:
// marking a book at its own mid lets liquidations trade into the price that
// triggers them.
func TestRegressionDefaultMarkAnchorsToIndex(t *testing.T) {
	oracle := &settableOracle{price: PriceUSD(50_000, DOLLAR_TICK)}
	ex, perp := anchorTestExchange(t, oracle, nil)

	ex.UpdatePerpPrices()

	// First sample seeds the basis EMA with mid-index = 10,000, then the
	// ±band/2 clamp caps it: 50,000 * 100bps/2 = 250.
	wantMark := PriceUSD(50_250, DOLLAR_TICK)
	if got := perp.GetFundingRate().MarkPrice; got != wantMark {
		t.Fatalf("default mark = %d, want index-anchored %d (own-mid mark would be %d)",
			got, wantMark, PriceUSD(60_000, DOLLAR_TICK))
	}
	// Basis is real now: index reported alongside must be the oracle's.
	if got := perp.GetFundingRate().IndexPrice; got != oracle.price {
		t.Fatalf("index = %d, want %d", got, oracle.price)
	}
}

// When a configured index goes stale (returns 0), the update must be SKIPPED
// — not silently replaced with the perp's own price, which made basis
// identically zero and hid the outage.
func TestRegressionStaleIndexSkipsMarkUpdate(t *testing.T) {
	oracle := &settableOracle{price: PriceUSD(50_000, DOLLAR_TICK)}
	ex, perp := anchorTestExchange(t, oracle, nil)

	ex.UpdatePerpPrices()
	markBefore := perp.GetFundingRate().MarkPrice

	oracle.price = 0
	ex.UpdatePerpPrices()

	if got := perp.GetFundingRate().MarkPrice; got != markBefore {
		t.Fatalf("stale index still updated mark: %d -> %d", markBefore, got)
	}
}

// A genuine single-venue setup (no index anywhere) keeps the historical
// mid-price behavior: the perp book is the only price there is.
func TestRegressionSingleVenueKeepsMidMark(t *testing.T) {
	ex, perp := anchorTestExchange(t, nil, nil)

	ex.UpdatePerpPrices()

	if got := perp.GetFundingRate().MarkPrice; got != PriceUSD(60_000, DOLLAR_TICK) {
		t.Fatalf("single-venue mark = %d, want book mid %d", got, PriceUSD(60_000, DOLLAR_TICK))
	}
}

// An explicitly injected calculator always wins over the anchored default.
func TestRegressionExplicitCalcOverridesAnchoredDefault(t *testing.T) {
	oracle := &settableOracle{price: PriceUSD(50_000, DOLLAR_TICK)}
	ex, perp := anchorTestExchange(t, oracle, NewMidPriceCalculator())

	ex.UpdatePerpPrices()

	if got := perp.GetFundingRate().MarkPrice; got != PriceUSD(60_000, DOLLAR_TICK) {
		t.Fatalf("explicit mid calculator ignored: mark = %d, want %d", got, PriceUSD(60_000, DOLLAR_TICK))
	}
}
