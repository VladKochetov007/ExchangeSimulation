package repeatedspot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
)

func TestEvidenceRoundTripAndCorruption(t *testing.T) {
	var raw bytes.Buffer
	recorder := NewRecorder(&raw)
	recorder.Record(7, 2, "exchange", "balance_change", "ABC/USD", map[string]any{"delta": 3})
	recorder.Record(8, 2, "actor", "order_send", "", map[string]any{"request_id": 4})
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if identity.SchemaID != EvidenceSchemaID || identity.FrameCount != 2 {
		t.Fatalf("wrong identity: %+v", identity)
	}
	var observed []Event
	if err := WalkEvidence(bytes.NewReader(raw.Bytes()), identity, func(event Event) error {
		observed = append(observed, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 || observed[0].Sequence != 1 || observed[1].Sequence != 2 || observed[1].Name != "order_send" {
		t.Fatalf("wrong events: %+v", observed)
	}
	if _, err := recorder.Finish(); err == nil {
		t.Fatal("second finish accepted")
	}

	for name, mutate := range map[string]func() ([]byte, EvidenceIdentity){
		"truncated": func() ([]byte, EvidenceIdentity) { return raw.Bytes()[:raw.Len()-5], identity },
		"wrong hash": func() ([]byte, EvidenceIdentity) {
			changed := identity
			changed.ExecutionHash = hex.EncodeToString(bytes.Repeat([]byte{0}, 32))
			return raw.Bytes(), changed
		},
		"wrong count": func() ([]byte, EvidenceIdentity) {
			changed := identity
			changed.FrameCount++
			return raw.Bytes(), changed
		},
		"flipped byte": func() ([]byte, EvidenceIdentity) {
			changed := bytes.Clone(raw.Bytes())
			changed[len(changed)/2] ^= 1
			return changed, identity
		},
	} {
		t.Run(name, func(t *testing.T) {
			data, expected := mutate()
			if err := WalkEvidence(bytes.NewReader(data), expected, func(Event) error { return nil }); err == nil {
				t.Fatal("corrupted evidence accepted")
			}
		})
	}
}

func TestEvidenceRejectsDuplicateJSONWithMatchingCanonicalHash(t *testing.T) {
	var raw bytes.Buffer
	writer := evstream.NewWriter(&raw, evstream.WriterOptions{SchemaEpoch: evidenceSchemaEpoch})
	malformed := json.RawMessage(`{"source":"exchange","source":"actor","name":"OrderFill","payload":{"qty":1}}`)
	if err := writer.AppendInterning(1, 1, 0, exchange.OpaqueJSON{Value: malformed}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	hash := writer.ExecutionHash()
	identity := EvidenceIdentity{SchemaID: EvidenceSchemaID, ExecutionHash: hex.EncodeToString(hash[:]), FrameCount: writer.Count()}
	if err := WalkEvidence(bytes.NewReader(raw.Bytes()), identity, func(Event) error { return nil }); err == nil {
		t.Fatal("duplicate JSON key accepted despite valid stream digest")
	}
}

func TestEvidenceWriterFailsClosedOnInvalidPayload(t *testing.T) {
	var raw bytes.Buffer
	recorder := NewRecorder(&raw)
	recorder.Record(0, 1, "actor", "decision", "", make(chan int))
	recorder.Record(1, 1, "actor", "decision", "", map[string]int{"valid": 1})
	if _, err := recorder.Finish(); err == nil {
		t.Fatal("writer silently discarded an unencodable event")
	}
}
