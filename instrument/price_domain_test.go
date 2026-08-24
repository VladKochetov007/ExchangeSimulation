package instrument

import (
	"errors"
	"math"
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

func TestSignedDatedFutureKeepsCashFlowsSignedAndRiskNotionalNonNegative(t *testing.T) {
	future := NewExpiringFutures("OIL-FUT", "OIL", "USD", 1, 1, 1, 1, 1)
	if err := future.SetPriceDomain(etypes.SignedPriceDomain(1)); err != nil {
		t.Fatalf("set signed future domain: %v", err)
	}
	future.MarginRate = 1_000 // ten percent of absolute traded notional.
	if got, err := future.MarginRequired(3, -20, 1); err != nil || got != 6 {
		t.Fatalf("negative-price margin = (%d, %v), want (6, nil)", got, err)
	}
	if got, err := future.MarginRequired(3, 0, 1); err != nil || got != 0 {
		t.Fatalf("zero-price margin = (%d, %v), want (0, nil) under declared absolute-notional policy", got, err)
	}
	future.DeliveryFeeBps = 1_000
	if got := future.DeliveryFee(-3, -20, 1); got != 6 {
		t.Fatalf("negative settlement delivery fee = %d, want 6", got)
	}

	for _, tc := range []struct {
		name             string
		size, entry, end int64
		want             int64
	}{
		{name: "long positive to negative", size: 1, entry: 20, end: -20, want: -40},
		{name: "short positive to negative", size: -1, entry: 20, end: -20, want: 40},
		{name: "long negative to positive", size: 1, entry: -20, end: 20, want: 40},
		{name: "short negative to positive", size: -1, entry: -20, end: 20, want: -40},
		{name: "long more negative", size: 1, entry: -20, end: -40, want: -20},
		{name: "short less negative", size: -1, entry: -40, end: -20, want: -20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := future.ExpiryCashFlow(tc.size, tc.entry, tc.end, 1); got != tc.want {
				t.Fatalf("ExpiryCashFlow(%d, %d, %d) = %d, want %d", tc.size, tc.entry, tc.end, got, tc.want)
			}
		})
	}
}

func TestMarginRequiredUsesErrorsRatherThanNumericFailureSentinels(t *testing.T) {
	perp := NewPerpFutures("OIL-PERP", "OIL", "USD", 1, 1, 1, 1)
	perp.MarginRate = 1_000

	tests := []struct {
		name                  string
		qty, price, precision int64
		want                  int64
		wantErr               bool
	}{
		{name: "zero price is a valid zero risk input", qty: 3, price: 0, precision: 1, want: 0},
		{name: "negative price uses magnitude", qty: 3, price: -20, precision: 1, want: 6},
		{name: "zero precision is an explicit error", qty: 3, price: 20, precision: 0, wantErr: true},
		{name: "negative quantity is an explicit error", qty: -3, price: 20, precision: 1, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := perp.MarginRequired(tc.qty, tc.price, tc.precision)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MarginRequired(%d, %d, %d) = (%d, nil), want an explicit error", tc.qty, tc.price, tc.precision, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("MarginRequired(%d, %d, %d) = (%d, %v), want (%d, nil)", tc.qty, tc.price, tc.precision, got, err, tc.want)
			}
		})
	}
}

func TestZeroOptionPremiumDoesNotSuppressShortMarketMargin(t *testing.T) {
	option := NewEuropeanOption("OIL-100-C", "OIL", "USD", "OIL/USD", 1, 1, 1, 1, 100, 1, true)
	// A zero premium is a real executable option price. The short's risk is
	// determined by the marked underlying and must not disappear because zero
	// used to be overloaded as a missing market-order reference.
	option.SetMarks(100, 10)
	want := option.MarginForOrder(etypes.Sell, 1, 0, 1)
	if want == 0 {
		t.Fatal("marked short option margin unexpectedly zero")
	}
	if got := option.MarginForMarketOrder(etypes.Sell, 1, 0, 1); got != want {
		t.Fatalf("zero-premium short market margin = %d, want explicit margin policy result %d", got, want)
	}
	if got := option.MarginForMarketOrder(etypes.Buy, 1, 0, 1); got != 0 {
		t.Fatalf("zero-premium long market margin = %d, want paid premium 0", got)
	}
}

func TestSettlementObserverRetainsSignedAndZeroObservations(t *testing.T) {
	future := NewExpiringFutures("OIL-FUT", "OIL", "USD", 1, 1, 1, 1, 1)
	if got, err := future.SettlementPrice(); got != 0 || !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("missing settlement = (%d, %v), want ErrNoPrice", got, err)
	}
	future.ObserveSettlement(-20, 1)
	if got, err := future.SettlementPrice(); err != nil || got != -20 {
		t.Fatalf("negative settlement = (%d, %v), want (-20, nil)", got, err)
	}

	zero := NewExpiringFutures("ZERO-FUT", "OIL", "USD", 1, 1, 1, 1, 1)
	zero.ObserveSettlement(0, 1)
	if got, err := zero.SettlementPrice(); err != nil || got != 0 {
		t.Fatalf("zero settlement = (%d, %v), want (0, nil)", got, err)
	}

	wide := NewExpiringFutures("WIDE-FUT", "OIL", "USD", 1, 1, 1, 1, 1)
	wide.ObserveSettlement(math.MinInt64, 1)
	wide.ObserveSettlement(math.MaxInt64, 2)
	if got, err := wide.SettlementPrice(); err != nil || got != 0 {
		t.Fatalf("wide signed settlement mean = (%d, %v), want (0, nil)", got, err)
	}
}

func TestOptionMarksUseErrorsRatherThanZeroForAbsence(t *testing.T) {
	option := NewEuropeanOption("OIL-0-C", "OIL", "USD", "OIL/USD", 1, 1, 1, 1, 1, 1, true)
	if mark, err := option.PositionMark(); mark != 0 || !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("unmarked premium = (%d, %v), want ErrNoPrice", mark, err)
	}
	if mark, err := option.UnderlyingMark(); mark != 0 || !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("unmarked underlying = (%d, %v), want ErrNoPrice", mark, err)
	}
	option.SetMarks(1, 0)
	if mark, err := option.PositionMark(); mark != 0 || err != nil {
		t.Fatalf("zero premium mark = (%d, %v), want (0, nil)", mark, err)
	}
	if mark, err := option.UnderlyingMark(); mark != 1 || err != nil {
		t.Fatalf("present underlying mark = (%d, %v), want (1, nil)", mark, err)
	}
}

func TestSetPriceDomainRejectsTickMutation(t *testing.T) {
	future := NewExpiringFutures("OIL-FUT", "OIL", "USD", 1, 1, 5, 1, 1)
	if err := future.SetPriceDomain(etypes.SignedPriceDomain(10)); err == nil {
		t.Fatal("SetPriceDomain accepted a mismatched tick")
	}
	if future.PriceDomain().SignPolicy() != etypes.PositivePrice {
		t.Fatalf("failed SetPriceDomain mutated existing domain: %#v", future.PriceDomain())
	}
}
