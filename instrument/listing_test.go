package instrument

import "testing"

type listingPriceSource map[string]int64

func (p listingPriceSource) Price(symbol string) int64 { return p[symbol] }

func TestOptionChainListerCapsDistinctStrikesPerExpiry(t *testing.T) {
	lister := &OptionChainLister{
		Underlying:          "ABC/USD",
		Spec:                ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		TenorsNano:          []int64{1_000},
		StrikeStep:          10,
		StrikesPerSide:      1,
		MaxStrikesPerExpiry: 3,
	}
	first := lister.PendingListings(0, listingPriceSource{"ABC/USD": 100})
	if len(first) != 6 { // 3 strikes, call and put each
		t.Fatalf("initial chain = %d options, want 6", len(first))
	}
	// A move to a disjoint grid would previously append three new strikes on
	// every price move. The capped venue keeps its bounded live board.
	second := lister.PendingListings(1, listingPriceSource{"ABC/USD": 200})
	if len(second) != 0 {
		t.Fatalf("capped chain listed %d additional options, want 0", len(second))
	}
}
