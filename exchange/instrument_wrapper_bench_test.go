package exchange

import (
	"encoding/json"
	"strconv"
	"testing"
)

// The per-symbol wrapper was once suspected of costing allocations that a
// hand-written marshaller could avoid. That rejection was measured across the
// introduction of a census probe which itself allocated on every event it saw,
// so it never tested the wrapper. This re-tests it in isolation.
//
// The candidate is the only mechanism Go offers for bypassing the reflection
// walk of a named type: implementing json.Marshaler. Whether that helps is the
// open question, because the encoder compacts whatever a Marshaler returns,
// and the wrapper's Payload is an interface that must be marshalled by
// reflection either way.

type wrapperCandidate struct {
	Symbol  string `json:"symbol"`
	Payload any    `json:"payload"`
}

func (w wrapperCandidate) MarshalJSON() ([]byte, error) {
	payload, err := json.Marshal(w.Payload)
	if err != nil {
		return nil, err
	}
	dst := make([]byte, 0, len(payload)+len(w.Symbol)+24)
	dst = append(dst, `{"symbol":`...)
	symbol, err := json.Marshal(w.Symbol)
	if err != nil {
		return nil, err
	}
	dst = append(dst, symbol...)
	dst = append(dst, `,"payload":`...)
	dst = append(dst, payload...)
	return append(dst, '}'), nil
}

// A struct payload and a map payload, because both reach the wrapper: typed
// evidence structs from the exchange itself, and map payloads from families
// that have not been converted.
func wrapperFixtures() []any {
	return []any{
		bookDeltaEvidence{
			HiddenQty: 0, Price: 4975000000, Side: "buy",
			TotalQty: 125000000, VisibleQty: 125000000,
		},
		map[string]any{
			"symbol": "ABC-FUT-1", "mark_price": int64(4975000000),
			"method": "index", "stale": false,
		},
	}
}

// Byte-identical output is a precondition: the ordered execution-stream digest
// is taken over these bytes, so a faster encoder that changes them changes the
// trajectory identity.
func TestWrapperCandidateIsByteIdentical(t *testing.T) {
	for _, payload := range wrapperFixtures() {
		want, err := json.Marshal(instrumentLogEvent{Symbol: "ABC-FUT-1", Payload: payload})
		if err != nil {
			t.Fatalf("reflection marshal: %v", err)
		}
		got, err := json.Marshal(wrapperCandidate{Symbol: "ABC-FUT-1", Payload: payload})
		if err != nil {
			t.Fatalf("candidate marshal: %v", err)
		}
		if string(got) != string(want) {
			t.Fatalf("candidate is not byte-identical\n want %s\n  got %s", want, got)
		}
	}
}

func BenchmarkWrapperReflection(b *testing.B) {
	fixtures := wrapperFixtures()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, payload := range fixtures {
			if _, err := json.Marshal(instrumentLogEvent{Symbol: "ABC-FUT-1", Payload: payload}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkWrapperCandidate(b *testing.B) {
	fixtures := wrapperFixtures()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, payload := range fixtures {
			if _, err := json.Marshal(wrapperCandidate{Symbol: "ABC-FUT-1", Payload: payload}); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// The third arm is the only form of the idea that can win: bypass
// encoding/json at the call site entirely, so no encoder buffer and no compact
// pass exist to pay for. It requires the caller to type-switch on an appender
// interface rather than calling json.Marshal, and it requires every payload
// reachable through the wrapper to have one — a map payload has no stable
// hand-written form, so it must fall back.
type jsonAppender interface {
	AppendJSON(dst []byte) []byte
}

func (e bookDeltaEvidence) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"hidden_qty":`...)
	dst = appendInt(dst, e.HiddenQty)
	dst = append(dst, `,"price":`...)
	dst = appendInt(dst, e.Price)
	dst = append(dst, `,"side":"`...)
	dst = append(dst, e.Side...)
	dst = append(dst, `","total_qty":`...)
	dst = appendInt(dst, e.TotalQty)
	dst = append(dst, `,"visible_qty":`...)
	dst = appendInt(dst, e.VisibleQty)
	return append(dst, '}')
}

func appendInt(dst []byte, value int64) []byte {
	return strconv.AppendInt(dst, value, 10)
}

// appendWrapper is what the observe path would call. The symbol is appended
// raw: every symbol in this simulator is drawn from a character set needing no
// escaping, which is an assumption the test below pins.
func appendWrapper(dst []byte, symbol string, payload any) ([]byte, bool) {
	inner, ok := payload.(jsonAppender)
	if !ok {
		return dst, false
	}
	dst = append(dst, `{"symbol":"`...)
	dst = append(dst, symbol...)
	dst = append(dst, `","payload":`...)
	dst = inner.AppendJSON(dst)
	return append(dst, '}'), true
}

func TestAppendWrapperIsByteIdentical(t *testing.T) {
	payload := bookDeltaEvidence{
		HiddenQty: 0, Price: 4975000000, Side: "buy",
		TotalQty: 125000000, VisibleQty: 125000000,
	}
	want, err := json.Marshal(instrumentLogEvent{Symbol: "ABC-FUT-1", Payload: payload})
	if err != nil {
		t.Fatalf("reflection marshal: %v", err)
	}
	got, ok := appendWrapper(nil, "ABC-FUT-1", payload)
	if !ok {
		t.Fatal("appender rejected a payload that implements the interface")
	}
	if string(got) != string(want) {
		t.Fatalf("appender is not byte-identical\n want %s\n  got %s", want, got)
	}
	if _, ok := appendWrapper(nil, "ABC-FUT-1", map[string]any{"a": 1}); ok {
		t.Fatal("appender accepted a map payload it cannot render byte-identically")
	}
}

// Measured against reflection on the SAME single payload, so the comparison is
// not distorted by the map fixture the appender cannot serve.
func BenchmarkWrapperReflectionStructOnly(b *testing.B) {
	payload := bookDeltaEvidence{
		HiddenQty: 0, Price: 4975000000, Side: "buy",
		TotalQty: 125000000, VisibleQty: 125000000,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(instrumentLogEvent{Symbol: "ABC-FUT-1", Payload: payload}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWrapperAppender(b *testing.B) {
	payload := bookDeltaEvidence{
		HiddenQty: 0, Price: 4975000000, Side: "buy",
		TotalQty: 125000000, VisibleQty: 125000000,
	}
	scratch := make([]byte, 0, 256)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, ok := appendWrapper(scratch[:0], "ABC-FUT-1", payload)
		if !ok || len(out) == 0 {
			b.Fatal("appender failed")
		}
	}
}
