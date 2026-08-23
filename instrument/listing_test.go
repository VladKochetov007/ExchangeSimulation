package instrument

import (
	"errors"
	"fmt"
	"testing"
)

type listingPriceSource map[string]int64

func (p listingPriceSource) Price(symbol string) (int64, error) {
	price, ok := p[symbol]
	if !ok || price <= 0 {
		return 0, fmt.Errorf("listing price for %s unavailable", symbol)
	}
	return price, nil
}

func TestOptionChainListerCapsDistinctStrikesPerExpiry(t *testing.T) {
	lister := &OptionChainLister{
		Underlying:          "ABC/USD",
		Spec:                ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		TenorsNano:          []int64{1_000},
		StrikeStep:          10,
		StrikesPerSide:      1,
		MaxStrikesPerExpiry: 3,
	}
	first, err := lister.PendingListings(0, listingPriceSource{"ABC/USD": 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 6 { // 3 strikes, call and put each
		t.Fatalf("initial chain = %d options, want 6", len(first))
	}
	// A move to a disjoint grid would previously append three new strikes on
	// every price move. The capped venue keeps its bounded live board.
	second, err := lister.PendingListings(1, listingPriceSource{"ABC/USD": 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("capped chain listed %d additional options, want 0", len(second))
	}
}

type unavailableListingPrice struct{ err error }

func (p unavailableListingPrice) Price(string) (int64, error) { return 0, p.err }

func TestOptionChainListerPropagatesPriceAbsence(t *testing.T) {
	sentinel := errors.New("missing reference")
	lister := &OptionChainLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		TenorsNano: []int64{1_000}, StrikeStep: 10,
	}
	listed, err := lister.PendingListings(0, unavailableListingPrice{err: sentinel})
	if listed != nil || !errors.Is(err, sentinel) {
		t.Fatalf("option price absence = (%#v, %v), want wrapped sentinel", listed, err)
	}
}
