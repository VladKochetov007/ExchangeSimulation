package types

import (
	"encoding/json"
	"testing"
)

// A fixed corpus proves the cases its author thought of. The claim here is
// stronger than that: whenever the fast path ACCEPTS an input it must produce
// exactly the bytes encoding/json would, because those bytes are hashed into an
// identity that appears in delivery evidence and decision attestations.
//
// Declining is always safe. Accepting and differing is a silent corruption of
// that identity, so this fuzzes the accept/differ corner specifically.
func FuzzFingerprintFastPathMatchesReflection(f *testing.F) {
	f.Add("ABC-USD", uint8(0), uint64(0), int64(0), int64(0), int64(0), int64(0), false)
	f.Add("ABC/USD", uint8(1), uint64(7), int64(-1), int64(5), int64(0), int64(9), true)
	f.Add("ABC-FUT-1735696801", uint8(2), ^uint64(0), int64(-9223372036854775808), int64(9223372036854775807), int64(-5), int64(3), false)
	f.Add("A<B&C\"D", uint8(1), uint64(3), int64(2), int64(1), int64(1), int64(1), true)
	f.Add("ünïcode", uint8(0), uint64(1), int64(1), int64(1), int64(1), int64(1), false)

	f.Fuzz(func(t *testing.T, symbol string, kind uint8, seq uint64,
		timestamp, price, qty, hidden int64, secondSide bool) {
		side := Buy
		if secondSide {
			side = Sell
		}
		var payload any
		switch kind % 4 {
		case 0:
			payload = BookSnapshot{
				Bids: []PriceLevel{{Price: price, VisibleQty: qty, HiddenQty: hidden}},
				Asks: nil,
			}
		case 1:
			payload = BookDelta{Side: side, Price: price, VisibleQty: qty, HiddenQty: hidden}
		case 2:
			payload = Trade{TradeID: seq, Price: price, Qty: qty, Side: side,
				TakerOrderID: seq, MakerOrderID: seq}
		case 3:
			// No canonical encoder: the fast path must decline, and the
			// fallback must still be correct.
			payload = map[string]any{"unknown": price}
		}
		msg := &MarketDataMsg{Type: MDType(kind % 7), Symbol: symbol,
			SeqNum: seq, Timestamp: timestamp, Data: payload}

		want, err := json.Marshal(struct {
			Type      MDType `json:"type"`
			Symbol    string `json:"symbol"`
			Sequence  uint64 `json:"sequence"`
			Timestamp int64  `json:"timestamp"`
			Data      any    `json:"data"`
		}{msg.Type, msg.Symbol, msg.SeqNum, msg.Timestamp, msg.Data})
		if err != nil {
			return // a payload json cannot encode is not this test's subject
		}

		got, accepted := appendFingerprintJSON(nil, msg)
		if !accepted {
			return // declining is always safe
		}
		if string(got) != string(want) {
			t.Fatalf("fast path accepted an input and produced different bytes\n symbol %q kind %d\n want %s\n  got %s",
				symbol, kind, want, got)
		}
	})
}
