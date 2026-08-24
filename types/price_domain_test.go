package types

import "testing"

func TestPriceDomainSeparatesNumericDomainFromAvailability(t *testing.T) {
	tests := []struct {
		name   string
		domain PriceDomain
		price  int64
		want   bool
	}{
		{name: "positive admits positive", domain: PositivePriceDomain(5), price: 10, want: true},
		{name: "positive rejects zero", domain: PositivePriceDomain(5), price: 0, want: false},
		{name: "positive rejects negative", domain: PositivePriceDomain(5), price: -10, want: false},
		{name: "non-negative admits zero", domain: NonNegativePriceDomain(5), price: 0, want: true},
		{name: "non-negative rejects negative", domain: NonNegativePriceDomain(5), price: -5, want: false},
		{name: "signed admits negative", domain: SignedPriceDomain(5), price: -10, want: true},
		{name: "signed admits zero", domain: SignedPriceDomain(5), price: 0, want: true},
		{name: "signed admits positive", domain: SignedPriceDomain(5), price: 10, want: true},
		{name: "tick remains explicit", domain: SignedPriceDomain(5), price: -9, want: false},
		{name: "invalid tick rejects safely", domain: SignedPriceDomain(0), price: 0, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.domain.Validate(tc.price); got != tc.want {
				t.Fatalf("Validate(%d) = %t, want %t", tc.price, got, tc.want)
			}
		})
	}
}
