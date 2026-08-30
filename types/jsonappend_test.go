package types

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

// These encoders exist to feed the ordered execution-stream digest, so the only
// property that matters is byte identity with encoding/json. Equivalence is not
// enough: a differently-but-validly encoded payload changes every published
// hash. Every test here is therefore a differential against the real marshaller.

func requireIdentical(t *testing.T, value JSONAppender) {
	t.Helper()
	want, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%#v) failed: %v", value, err)
	}
	got := value.AppendJSON(nil)
	if !bytes.Equal(got, want) {
		t.Fatalf("encoding differs\n  appended: %s\n  marshal : %s", got, want)
	}
}

// TestAppendJSONStringMatchesMarshal walks the escaping rules directly, because
// the fast path's whole safety argument is that it declines anything it cannot
// reproduce. encoding/json HTML-escapes <, > and & by default, which is the
// case most likely to be missed.
func TestAppendJSONStringMatchesMarshal(t *testing.T) {
	cases := []string{
		"", "USD", "ABC/USD", "ABC-FUT-1735696801", "BOTH", "liquidation_repay",
		`quote"inside`, `back\slash`, "tab\there", "newline\nhere", "carriage\rreturn",
		"<html>", "a&b", "less<than", "greater>than",
		"\x00", "\x1f", "\x7f",
		"unicode é", "emoji 🙂", " line", " para",
		string([]byte{0xff, 0xfe}), // invalid UTF-8
	}
	for _, s := range cases {
		want, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("json.Marshal(%q) failed: %v", s, err)
		}
		got := AppendJSONString(nil, s)
		if !bytes.Equal(got, want) {
			t.Fatalf("string %q\n  appended: %s\n  marshal : %s", s, got, want)
		}
	}
}

func TestAppendJSONIntegersMatchMarshal(t *testing.T) {
	for _, v := range []int64{0, 1, -1, math.MaxInt64, math.MinInt64, 1 << 53} {
		want, _ := json.Marshal(v)
		if got := AppendJSONInt(nil, v); !bytes.Equal(got, want) {
			t.Fatalf("int %d: appended %s, marshal %s", v, got, want)
		}
	}
	for _, v := range []uint64{0, 1, math.MaxUint64, 1 << 63} {
		want, _ := json.Marshal(v)
		if got := AppendJSONUint(nil, v); !bytes.Equal(got, want) {
			t.Fatalf("uint %d: appended %s, marshal %s", v, got, want)
		}
	}
}

// TestBalanceChangeEventNilVersusEmptyChanges pins the distinction encoding/json
// makes and a digest would notice: a nil slice is null, an empty one is [].
func TestBalanceChangeEventNilVersusEmptyChanges(t *testing.T) {
	requireIdentical(t, BalanceChangeEvent{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD",
		Reason: "fill", Changes: nil})
	requireIdentical(t, BalanceChangeEvent{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD",
		Reason: "fill", Changes: []BalanceDelta{}})
}

// TestBalanceChangeEventOmitsEmptyPositionSide covers the omitempty tag, where
// emitting the field anyway would still be valid JSON and still break the hash.
func TestBalanceChangeEventOmitsEmptyPositionSide(t *testing.T) {
	requireIdentical(t, BalanceChangeEvent{Timestamp: 7, ClientID: 9, Symbol: "ABC-PERP",
		PositionSide: "", Reason: "funding", Changes: []BalanceDelta{{Asset: "USD"}}})
	requireIdentical(t, BalanceChangeEvent{Timestamp: 7, ClientID: 9, Symbol: "ABC-PERP",
		PositionSide: "BOTH", Reason: "funding", Changes: []BalanceDelta{{Asset: "USD"}}})
}

func TestBalanceChangeEventZeroValue(t *testing.T) {
	requireIdentical(t, BalanceChangeEvent{})
	requireIdentical(t, BalanceDelta{})
}

// TestBalanceChangeEventRandomised is the broad sweep: random field values
// including the escape-worthy strings and integer extremes, so a divergence in
// any combination shows up rather than only in the shapes I thought to write.
func TestBalanceChangeEventRandomised(t *testing.T) {
	random := rand.New(rand.NewSource(20260830))
	strings := []string{"", "USD", "ABC/USD", `has"quote`, "<tag>", "a&b", "é", "🙂",
		"ctl\x01", "back\\slash", "tab\t"}
	ints := []int64{0, 1, -1, 12345678901234, math.MaxInt64, math.MinInt64}
	uints := []uint64{0, 1, 4294967296, math.MaxUint64}

	pick := func(s []string) string { return s[random.Intn(len(s))] }
	pickI := func() int64 { return ints[random.Intn(len(ints))] }

	for range 20000 {
		event := BalanceChangeEvent{
			Timestamp:    pickI(),
			ClientID:     uints[random.Intn(len(uints))],
			Symbol:       pick(strings),
			PositionSide: pick(strings),
			Reason:       pick(strings),
		}
		switch random.Intn(3) {
		case 0:
			event.Changes = nil
		case 1:
			event.Changes = []BalanceDelta{}
		default:
			count := 1 + random.Intn(3)
			event.Changes = make([]BalanceDelta, count)
			for i := range event.Changes {
				event.Changes[i] = BalanceDelta{
					Asset: pick(strings), Wallet: pick(strings),
					OldBalance: pickI(), NewBalance: pickI(), Delta: pickI(),
				}
			}
		}
		requireIdentical(t, event)
	}
}

// TestAppendJSONPreservesExistingBytes covers the append contract: encoders are
// called with a reused buffer, so one that ignored dst would silently drop
// whatever a caller had already written.
func TestAppendJSONPreservesExistingBytes(t *testing.T) {
	prefix := []byte("PREFIX")
	got := BalanceDelta{Asset: "USD", Delta: 5}.AppendJSON(prefix)
	if !bytes.HasPrefix(got, prefix) {
		t.Fatalf("appender discarded existing bytes: %s", got)
	}
	want, _ := json.Marshal(BalanceDelta{Asset: "USD", Delta: 5})
	if !bytes.Equal(got[len(prefix):], want) {
		t.Fatalf("appended %s, want %s", got[len(prefix):], want)
	}
}

// TestBookPayloadsMatchMarshal covers the market-data payloads that together
// account for a further 19.5 % of hashed bytes. Side goes through
// json.Marshaler, and the slice fields carry the same nil-versus-empty
// distinction as balance changes.
func TestBookPayloadsMatchMarshal(t *testing.T) {
	for _, side := range []Side{Buy, Sell} {
		for _, v := range []int64{0, 1, -1, math.MaxInt64, math.MinInt64} {
			requireIdentical(t, BookDelta{Side: side, Price: v, VisibleQty: v, HiddenQty: v})
		}
	}
	requireIdentical(t, PriceLevel{})
	requireIdentical(t, BookSnapshot{})
	requireIdentical(t, BookSnapshot{Bids: []PriceLevel{}, Asks: nil})
	requireIdentical(t, BookSnapshot{Bids: nil, Asks: []PriceLevel{}})
	requireIdentical(t, BookSnapshot{
		Bids: []PriceLevel{{Price: 1, VisibleQty: 2, HiddenQty: 3}, {Price: math.MinInt64}},
		Asks: []PriceLevel{{Price: math.MaxInt64, VisibleQty: -1}},
	})
}

func TestBookSnapshotRandomised(t *testing.T) {
	random := rand.New(rand.NewSource(20260831))
	ints := []int64{0, 1, -1, 987654321, math.MaxInt64, math.MinInt64}
	for range 20000 {
		build := func() []PriceLevel {
			switch random.Intn(3) {
			case 0:
				return nil
			case 1:
				return []PriceLevel{}
			}
			out := make([]PriceLevel, 1+random.Intn(4))
			for i := range out {
				out[i] = PriceLevel{
					Price:      ints[random.Intn(len(ints))],
					VisibleQty: ints[random.Intn(len(ints))],
					HiddenQty:  ints[random.Intn(len(ints))],
				}
			}
			return out
		}
		requireIdentical(t, BookSnapshot{Bids: build(), Asks: build()})
	}
}
