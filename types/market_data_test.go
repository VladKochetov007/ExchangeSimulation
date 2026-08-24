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
		name  string
		price *int64
	}{
		{name: "unavailable"},
		{name: "negative", price: int64Pointer(-20)},
		{name: "zero", price: int64Pointer(0)},
		{name: "positive", price: int64Pointer(20)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := InstrumentAnnouncement{
				Action:          "settled",
				Symbol:          "OIL-FUT",
				InstrumentType:  "FUTURE",
				SettlementPrice: tc.price,
			}
			raw, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var wire map[string]json.RawMessage
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatalf("decode generic wire: %v", err)
			}
			_, priceOnWire := wire["settlement_price"]
			if (tc.price != nil) != priceOnWire {
				t.Fatalf("settlement price presence = %t, want %t: %s", priceOnWire, tc.price != nil, raw)
			}

			var replayed InstrumentAnnouncement
			if err := json.Unmarshal(raw, &replayed); err != nil {
				t.Fatalf("round trip: %v", err)
			}
			if (replayed.SettlementPrice == nil) != (tc.price == nil) {
				t.Fatalf("round trip availability = %#v, want %#v", replayed.SettlementPrice, tc.price)
			}
			if tc.price != nil && *replayed.SettlementPrice != *tc.price {
				t.Fatalf("round trip price = %d, want %d", *replayed.SettlementPrice, *tc.price)
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }
