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
	if got := future.MarginRequired(3, -20, 1); got != 6 {
		t.Fatalf("negative-price margin = %d, want 6", got)
	}
	if got := future.MarginRequired(3, 0, 1); got != 0 {
		t.Fatalf("zero-price margin = %d, want 0 under declared absolute-notional policy", got)
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
