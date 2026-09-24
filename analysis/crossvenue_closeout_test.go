package analysis

import (
	"math"
	"testing"

	etypes "exchange_sim/types"
)

func TestVenueLocalCloseoutUsesRelevantDepthAndPerLevelFees(t *testing.T) {
	levels := []etypes.PriceLevel{
		{Price: 100, VisibleQty: 2},
		{Price: 102, VisibleQty: 1},
	}
	sale := ValueVenueLocalInventory(VenueLocalCloseoutInput{
		BaseDelta: 3, QuoteDelta: -300, BasePrecision: 1, TakerFeeBps: 100,
		Bids: levels,
	})
	// Sorted bid walk: 1 at 102 and 2 at 100. Fees are 1 and 2.
	if !sale.Available || sale.GrossCashflow != 302 || sale.QuoteFees != 3 || sale.Value != -1 || sale.CoveredQty != 3 {
		t.Fatalf("sale = %#v", sale)
	}
	purchase := ValueVenueLocalInventory(VenueLocalCloseoutInput{
		BaseDelta: -3, QuoteDelta: 310, BasePrecision: 1, TakerFeeBps: 100,
		Asks: levels,
	})
	// Sorted ask walk: 2 at 100 and 1 at 102. Fees are 2 and 1.
	if !purchase.Available || purchase.GrossCashflow != -302 || purchase.QuoteFees != 3 || purchase.Value != 5 {
		t.Fatalf("purchase = %#v", purchase)
	}
}

func TestVenueLocalCloseoutRejectsMissingOrInvalidExecutableDepth(t *testing.T) {
	cases := []struct {
		name   string
		input  VenueLocalCloseoutInput
		reason string
		cover  int64
	}{
		{"empty", VenueLocalCloseoutInput{BaseDelta: 1, BasePrecision: 1}, "NO_EXECUTABLE_SIDE", 0},
		{"partial", VenueLocalCloseoutInput{BaseDelta: -3, BasePrecision: 1, Asks: []etypes.PriceLevel{{Price: 101, VisibleQty: 2}}}, "INSUFFICIENT_VISIBLE_DEPTH", 2},
		{"hidden-not-executable", VenueLocalCloseoutInput{BaseDelta: 1, BasePrecision: 1, Bids: []etypes.PriceLevel{{Price: 100, HiddenQty: 10}}}, "INSUFFICIENT_VISIBLE_DEPTH", 0},
		{"signed-price", VenueLocalCloseoutInput{BaseDelta: 1, BasePrecision: 1, Bids: []etypes.PriceLevel{{Price: -1, VisibleQty: 1}}}, "INVALID_BOOK", 0},
		{"duplicate-level", VenueLocalCloseoutInput{BaseDelta: 1, BasePrecision: 1, Bids: []etypes.PriceLevel{{Price: 100, VisibleQty: 1}, {Price: 100, VisibleQty: 1}}}, "INVALID_BOOK", 1},
		{"invalid-precision", VenueLocalCloseoutInput{BaseDelta: 1, BasePrecision: 0}, "INVALID_CONVENTION", 0},
		{"minimum-quantity", VenueLocalCloseoutInput{BaseDelta: math.MinInt64, BasePrecision: 1}, "QUANTITY_OVERFLOW", 0},
		{"notional-overflow", VenueLocalCloseoutInput{BaseDelta: math.MaxInt64, BasePrecision: 1, Bids: []etypes.PriceLevel{{Price: 2, VisibleQty: math.MaxInt64}}}, "ARITHMETIC_OVERFLOW", 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := ValueVenueLocalInventory(test.input)
			if result.Available || result.Reason != test.reason || result.CoveredQty != test.cover {
				t.Fatalf("closeout = %#v, want reason %s and coverage %d", result, test.reason, test.cover)
			}
		})
	}
}

func TestVenueLocalCloseoutPortfolioFailsClosed(t *testing.T) {
	if _, err := SumVenueLocalCloseouts(map[string]VenueLocalCloseout{
		"north": {Available: true, Value: 12},
		"south": {Reason: "INSUFFICIENT_VISIBLE_DEPTH"},
	}); err == nil {
		t.Fatal("missing local closeout was treated as zero")
	}
	value, err := SumVenueLocalCloseouts(map[string]VenueLocalCloseout{
		"north": {Available: true, Value: -7},
		"south": {Available: true, Value: 12},
	})
	if err != nil || value != 5 {
		t.Fatalf("portfolio = %d, %v", value, err)
	}
	if _, err := SumVenueLocalCloseouts(map[string]VenueLocalCloseout{
		"north": {Available: true, Value: math.MaxInt64},
		"south": {Available: true, Value: 1},
	}); err == nil {
		t.Fatal("portfolio overflow was accepted")
	}
}
