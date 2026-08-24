package types

import (
	"encoding/json"
	"testing"
)

// Instrument lifecycle evidence must distinguish a missing settlement source
// from every signed numeric settlement that an instrument contract permits.
// Decode through a generic wire map first so a Go zero-value cannot hide a
// JSON omission behind the same in-memory value.
func TestInstrumentAnnouncementSettlementPriceWireContract(t *testing.T) {
	tests := []struct {
		name      string
		available bool
		price     int64
	}{
		{name: "unavailable", available: false},
		{name: "negative", available: true, price: -20},
		{name: "zero", available: true, price: 0},
		{name: "positive", available: true, price: 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := InstrumentAnnouncement{
				Action:                   "settled",
				Symbol:                   "OIL-FUT",
				InstrumentType:           "FUTURE",
				SettlementPrice:          tc.price,
				SettlementPriceAvailable: tc.available,
			}
			raw, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var wire map[string]json.RawMessage
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatalf("decode generic wire: %v", err)
			}
			for _, field := range []string{"settlement_price", "settlement_price_available"} {
				if _, ok := wire[field]; !ok {
					t.Fatalf("%s missing from lifecycle evidence: %s", field, raw)
				}
			}

			var replayed InstrumentAnnouncement
			if err := json.Unmarshal(raw, &replayed); err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if replayed.SettlementPriceAvailable != tc.available || replayed.SettlementPrice != tc.price {
				t.Fatalf("round trip = %+v, want availability=%t price=%d", replayed, tc.available, tc.price)
			}
		})
	}
}
