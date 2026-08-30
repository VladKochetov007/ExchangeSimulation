package exchange

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"
)

// These payloads are marshalled into the ordered execution-stream digest, so
// their encoding is part of the reproducibility attestation. Two properties are
// asserted here, and the first is the one that would silently invalidate every
// published hash if it broke:
//
// The property under test is not "the struct encodes as valid JSON" — it is
// that the struct encodes byte-identically to the map it replaced, which holds
// only while its fields stay in lexicographic order of their JSON names,
// because encoding/json sorts map keys.

// bookDeltaMapForm is the payload exactly as publishBookUpdate built it before
// bookDeltaEvidence replaced it. Kept here as the oracle: without it, "the
// struct matches encoding/json" would be true of any field order.
func bookDeltaMapForm(side string, price, visible, hidden, total int64) map[string]any {
	return map[string]any{
		"side":        side,
		"price":       price,
		"visible_qty": visible,
		"hidden_qty":  hidden,
		"total_qty":   total,
	}
}

func TestBookDeltaEvidenceMatchesTheMapItReplaced(t *testing.T) {
	cases := []struct {
		side                          string
		price, visible, hidden, total int64
	}{
		{"BUY", 0, 0, 0, 0},
		{"SELL", 1, 2, 3, 5},
		{"BUY", -1, -2, -3, -5},
		{"SELL", math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64},
		{"BUY", math.MinInt64, math.MinInt64, math.MinInt64, math.MinInt64},
	}
	for _, tc := range cases {
		wantMap, err := json.Marshal(bookDeltaMapForm(tc.side, tc.price, tc.visible, tc.hidden, tc.total))
		if err != nil {
			t.Fatalf("marshalling the map form failed: %v", err)
		}
		payload := bookDeltaEvidence{
			HiddenQty: tc.hidden, Price: tc.price, Side: tc.side,
			TotalQty: tc.total, VisibleQty: tc.visible,
		}
		gotStruct, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshalling the struct form failed: %v", err)
		}
		if !bytes.Equal(gotStruct, wantMap) {
			t.Fatalf("struct encoding differs from the map it replaced\n  struct: %s\n  map   : %s",
				gotStruct, wantMap)
		}
	}
}

// TestBookDeltaEvidenceFieldOrderIsLexicographic states the contract directly,
// so a reordering shows up as a failure naming the cause rather than as a
// mismatched hash three runs later.
func TestBookDeltaEvidenceFieldOrderIsLexicographic(t *testing.T) {
	encoded, err := json.Marshal(bookDeltaEvidence{})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	want := `{"hidden_qty":0,"price":0,"side":"","total_qty":0,"visible_qty":0}`
	if string(encoded) != want {
		t.Fatalf("field order changed — this is the evidence contract\n  got  %s\n  want %s",
			encoded, want)
	}
}

func TestBookDeltaEvidenceRandomised(t *testing.T) {
	random := rand.New(rand.NewSource(20260901))
	ints := []int64{0, 1, -1, 42, 987654321, math.MaxInt64, math.MinInt64}
	pick := func() int64 { return ints[random.Intn(len(ints))] }

	for range 20000 {
		side := "BUY"
		if random.Intn(2) == 0 {
			side = "SELL"
		}
		delta := bookDeltaEvidence{
			HiddenQty: pick(), Price: pick(), Side: side,
			TotalQty: pick(), VisibleQty: pick(),
		}
		wantMap, _ := json.Marshal(bookDeltaMapForm(
			delta.Side, delta.Price, delta.VisibleQty, delta.HiddenQty, delta.TotalQty))
		gotStruct, err := json.Marshal(delta)
		if err != nil {
			t.Fatalf("marshalling the struct form failed: %v", err)
		}
		if !bytes.Equal(gotStruct, wantMap) {
			t.Fatalf("struct %s, map form %s", gotStruct, wantMap)
		}

	}
}
