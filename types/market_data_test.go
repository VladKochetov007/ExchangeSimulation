package types

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"testing"
)

func TestMarketDataFingerprintRetainsReceiptCanonicalEncoding(t *testing.T) {
	tests := []struct {
		name string
		msg  *MarketDataMsg
	}{
		{
			name: "book snapshot",
			msg: &MarketDataMsg{Type: MDSnapshot, Symbol: "ABC-PERP", SeqNum: 41, Timestamp: 123,
				Data: &BookSnapshot{Bids: []PriceLevel{{Price: 100, VisibleQty: 3}}, Asks: []PriceLevel{{Price: 101, VisibleQty: 4}}}},
		},
		{
			name: "zero settlement remains present in the message identity",
			msg:  &MarketDataMsg{Type: MDInstrument, Symbol: InstrumentFeedSymbol, SeqNum: 7, Timestamp: 456, Data: &InstrumentAnnouncement{Action: "settled", Symbol: "OIL-FUT", SettlementPrice: int64Pointer(0)}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MarketDataFingerprint(tc.msg)
			if err != nil {
				t.Fatalf("fingerprint: %v", err)
			}
			raw, err := json.Marshal(struct {
				Type      MDType `json:"type"`
				Symbol    string `json:"symbol"`
				Sequence  uint64 `json:"sequence"`
				Timestamp int64  `json:"timestamp"`
				Data      any    `json:"data"`
			}{tc.msg.Type, tc.msg.Symbol, tc.msg.SeqNum, tc.msg.Timestamp, tc.msg.Data})
			if err != nil {
				t.Fatalf("legacy receipt encoding: %v", err)
			}
			wantHash := sha256.Sum256(raw)
			var want [16]byte
			copy(want[:], wantHash[:])
			if got != want {
				t.Fatalf("fingerprint drifted from receipt encoding: got %x want %x", got, want)
			}
		})
	}

	if _, err := MarketDataFingerprint(&MarketDataMsg{Data: math.Inf(1)}); err == nil {
		t.Fatal("unencodable market-data payload was silently fingerprinted")
	}
}

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

func TestInstrumentAnnouncementListingTimeRoundTrip(t *testing.T) {
	listed := int64(17)
	original := InstrumentAnnouncement{
		Action: "listed", Symbol: "ABC-FUT", InstrumentType: "FUTURE",
		ListedNano: &listed, ExpiryNano: 29,
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var replayed InstrumentAnnouncement
	if err := json.Unmarshal(raw, &replayed); err != nil {
		t.Fatal(err)
	}
	if replayed.ListedNano == nil || *replayed.ListedNano != *original.ListedNano || replayed.ExpiryNano != original.ExpiryNano {
		t.Fatalf("listing lifecycle round trip = %#v, want %#v", replayed, original)
	}
}

func TestInstrumentAnnouncementZeroListingTimeIsPresent(t *testing.T) {
	listed := int64(0)
	raw, err := json.Marshal(InstrumentAnnouncement{Action: "listed", Symbol: "ABC-FUT", InstrumentType: "FUTURE", ListedNano: &listed, ExpiryNano: 1})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if _, present := wire["listed_nano"]; !present {
		t.Fatalf("zero listing time was omitted: %s", raw)
	}
}

func int64Pointer(value int64) *int64 { return &value }
