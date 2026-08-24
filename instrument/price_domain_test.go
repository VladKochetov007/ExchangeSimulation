package instrument

import (
	"testing"

	etypes "exchange_sim/types"
)

func TestInstrumentPriceDomainsAreExplicit(t *testing.T) {
	spot := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1, 5, 1)
	if spot.PriceDomain().SignPolicy() != etypes.PositivePrice || !spot.ValidatePrice(5) || spot.ValidatePrice(0) || spot.ValidatePrice(-5) {
		t.Fatalf("spot domain = %#v", spot.PriceDomain())
	}

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", 1, 1, 5, 1)
	if perp.PriceDomain().SignPolicy() != etypes.PositivePrice || perp.ValidatePrice(0) {
		t.Fatalf("perp domain = %#v", perp.PriceDomain())
	}

	option := NewEuropeanOption("BTC-100-C", "BTC", "USD", "BTC/USD", 1, 1, 5, 1, 100, 1, true)
	if option.PriceDomain().SignPolicy() != etypes.NonNegativePrice || !option.ValidatePrice(0) || option.ValidatePrice(-5) {
		t.Fatalf("option domain = %#v", option.PriceDomain())
	}

	future := NewExpiringFutures("OIL-FUT", "OIL", "USD", 1, 1, 5, 1, 1)
	if err := future.SetPriceDomain(etypes.SignedPriceDomain(5)); err != nil {
		t.Fatalf("set signed future domain: %v", err)
	}
	for _, price := range []int64{-10, 0, 10} {
		if !future.ValidatePrice(price) {
			t.Fatalf("signed future rejected price %d", price)
		}
	}
	if future.ValidatePrice(-9) {
		t.Fatal("signed future admitted off-tick price")
	}
}

func TestSetPriceDomainRejectsTickMutation(t *testing.T) {
	spot := NewSpotInstrument("BTC/USD", "BTC", "USD", 1, 1, 5, 1)
	if err := spot.SetPriceDomain(etypes.SignedPriceDomain(10)); err == nil {
		t.Fatal("SetPriceDomain accepted a mismatched tick")
	}
	if spot.PriceDomain().SignPolicy() != etypes.PositivePrice {
		t.Fatalf("failed SetPriceDomain mutated existing domain: %#v", spot.PriceDomain())
	}
}
