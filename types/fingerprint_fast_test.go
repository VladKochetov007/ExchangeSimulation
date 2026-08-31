package types

import (
	"crypto/sha256"
	"encoding/json"
	"math"
	"testing"
)

// The fingerprint is an identity: it feeds delivery evidence and decision
// attestations. A hand-written encoder that is "almost" right changes that
// identity silently, so the only acceptable standard is byte-equality with the
// reflection path, checked over every payload type and every edge case that
// distinguishes the two encoders.
func referenceJSON(t *testing.T, msg *MarketDataMsg) []byte {
	t.Helper()
	raw, err := json.Marshal(struct {
		Type      MDType `json:"type"`
		Symbol    string `json:"symbol"`
		Sequence  uint64 `json:"sequence"`
		Timestamp int64  `json:"timestamp"`
		Data      any    `json:"data"`
	}{msg.Type, msg.Symbol, msg.SeqNum, msg.Timestamp, msg.Data})
	if err != nil {
		t.Fatalf("reference marshal: %v", err)
	}
	return raw
}

func fingerprintCorpus() []*MarketDataMsg {
	levels := []PriceLevel{
		{Price: 1, VisibleQty: 2, HiddenQty: 3},
		{Price: math.MinInt64, VisibleQty: math.MaxInt64, HiddenQty: 0},
	}
	return []*MarketDataMsg{
		{Type: MDSnapshot, Symbol: "ABC-USD", SeqNum: 0, Timestamp: 0,
			Data: BookSnapshot{}},
		{Type: MDSnapshot, Symbol: "ABC/USD", SeqNum: 1, Timestamp: -1,
			Data: BookSnapshot{Bids: levels, Asks: []PriceLevel{}}},
		{Type: MDSnapshot, Symbol: "ABC-FUT-1735696801", SeqNum: math.MaxUint64,
			Timestamp: math.MaxInt64, Data: BookSnapshot{Bids: []PriceLevel{}, Asks: levels}},
		{Type: MDDelta, Symbol: "CDF-USD", SeqNum: 7, Timestamp: 11,
			Data: BookDelta{Side: Sell, Price: -5, VisibleQty: 0, HiddenQty: 7}},
		{Type: MDDelta, Symbol: "CDF-USD", SeqNum: 8, Timestamp: math.MinInt64,
			Data: BookDelta{Side: Buy, Price: math.MaxInt64, VisibleQty: math.MinInt64}},
		{Type: MDTrade, Symbol: "ABC-PERP", SeqNum: 3, Timestamp: 4,
			Data: Trade{TradeID: 9, Price: 1, Qty: 2, Side: Buy, TakerOrderID: 3, MakerOrderID: 4}},
		{Type: MDTrade, Symbol: "ABC-PERP", SeqNum: 4, Timestamp: 5,
			Data: Trade{TradeID: math.MaxUint64, Price: math.MinInt64, Side: Sell}},
	}
}

func TestFastFingerprintIsByteIdenticalToReflection(t *testing.T) {
	for _, msg := range fingerprintCorpus() {
		want := referenceJSON(t, msg)
		got, ok := appendFingerprintJSON(nil, msg)
		if !ok {
			t.Fatalf("fast path declined a payload it implements: %T %+v", msg.Data, msg)
		}
		if string(got) != string(want) {
			t.Fatalf("fast path differs\n want %s\n  got %s", want, got)
		}
		wantDigest := sha256.Sum256(want)
		gotFingerprint, err := MarketDataFingerprint(msg)
		if err != nil {
			t.Fatalf("fingerprint: %v", err)
		}
		var wantFingerprint [16]byte
		copy(wantFingerprint[:], wantDigest[:])
		if gotFingerprint != wantFingerprint {
			t.Fatalf("fingerprint changed for %+v: got %x want %x", msg, gotFingerprint, wantFingerprint)
		}
	}
}

// A symbol needing JSON escaping must take the reflection path rather than be
// encoded by hand, because Go escapes <, > and & in ways a naive appender
// would not reproduce.
func TestFastFingerprintDeclinesEscapedSymbols(t *testing.T) {
	for _, symbol := range []string{`A<B`, `A&B`, `A"B`, "A\\B", "A\tB", "ünïcode"} {
		msg := &MarketDataMsg{Type: MDDelta, Symbol: symbol, Data: BookDelta{Side: Buy}}
		if _, ok := appendFingerprintJSON(nil, msg); ok {
			t.Fatalf("fast path accepted a symbol needing escaping: %q", symbol)
		}
		// The fingerprint must still be correct via the fallback.
		want := sha256.Sum256(referenceJSON(t, msg))
		got, err := MarketDataFingerprint(msg)
		if err != nil {
			t.Fatalf("fingerprint: %v", err)
		}
		var expected [16]byte
		copy(expected[:], want[:])
		if got != expected {
			t.Fatalf("fallback fingerprint wrong for %q", symbol)
		}
	}
}

// A payload with no fast encoder must still fingerprint correctly, which is
// what lets a user add their own market-data type without touching this
// package.
func TestUnknownPayloadStillFingerprints(t *testing.T) {
	msg := &MarketDataMsg{Type: MDFunding, Symbol: "ABC-PERP", SeqNum: 2, Timestamp: 3,
		Data: map[string]any{"rate": 5, "interval": "8h"}}
	if _, ok := appendFingerprintJSON(nil, msg); ok {
		t.Fatal("fast path accepted a payload with no canonical encoder")
	}
	want := sha256.Sum256(referenceJSON(t, msg))
	got, err := MarketDataFingerprint(msg)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	var expected [16]byte
	copy(expected[:], want[:])
	if got != expected {
		t.Fatal("fallback fingerprint differs from reflection")
	}
}
