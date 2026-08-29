package feesim

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

// collectingWriteCloser keeps every byte written so a persisted record can be
// compared exactly.
type collectingWriteCloser struct {
	buffer bytes.Buffer
}

func (c *collectingWriteCloser) Write(p []byte) (int, error) { return c.buffer.Write(p) }
func (c *collectingWriteCloser) Close() error                { return nil }

// venueEnvelope mirrors the data value a venue logger persists. Field order is
// part of the evidence contract.
type venueEnvelope struct {
	VenueID string `json:"venue_id"`
	Payload any    `json:"payload"`
}

// TestLogEncodedEventPersistsTheSameBytesAsLogEvent holds the manual envelope
// assembly to the reflective encoder it replaces, through the real logger.
//
// This is the load-bearing test for the optimization: LogEncodedEvent skips
// encoding/json for the envelope and reuses a payload encoding produced
// elsewhere, so its output has to be proved equal rather than assumed. It also
// checks that the evidence digest agrees, since the digest hashes the persisted
// line and is what an offline evidence-artifact audit re-derives.
func TestLogEncodedEventPersistsTheSameBytesAsLogEvent(t *testing.T) {
	type nested struct {
		Alpha int64  `json:"alpha"`
		Beta  string `json:"beta"`
	}
	payloads := []any{
		nil,
		map[string]any{},
		map[string]any{"a": int64(1), "b": "two"},
		map[string]any{"max": int64(9223372036854775807), "min": int64(-9223372036854775808)},
		map[string]any{"umax": uint64(18446744073709551615)},
		map[string]any{"html": "<script>&</script>"},
		map[string]any{"quote": `he said "hi"`, "backslash": `a\b`},
		map[string]any{"control": "a\nb\tc\r"},
		map[string]any{"unicode": "ábc↦☃"},
		map[string]any{"lineSep": "a\u2028b\u2029c"},
		map[string]any{"nested": nested{Alpha: 3, Beta: "x"}},
		map[string]any{"list": []any{int64(1), "two", nil, true}},
		map[string]any{"float": 1.5, "sci": 1e21, "tiny": 1e-7},
		nested{Alpha: 7, Beta: "seven"},
		[]any{int64(1), int64(2)},
		"scalar string",
		int64(-42),
		true,
	}
	venues := []string{"north", `venue"quote`, "venue<html>&", "vénue", "venue\u2028sep"}
	events := []string{"OrderFill", "balance_change", `event"quote`, "event<html>&", "événement"}
	clients := []uint64{0, 17, 18446744073709551615}
	times := []int64{0, 1735689605000000000, -1, -9223372036854775808, 9223372036854775807}

	for _, venueID := range venues {
		encodedVenue, err := json.Marshal(venueID)
		if err != nil {
			t.Fatal(err)
		}
		prefix := append([]byte(`{"venue_id":`), encodedVenue...)
		prefix = append(prefix, `,"payload":`...)

		for _, eventName := range events {
			for _, clientID := range clients {
				for _, simTime := range times {
					for index, payload := range payloads {
						encodedPayload, err := json.Marshal(payload)
						if err != nil {
							t.Fatalf("payload %d: %v", index, err)
						}

						reference := &collectingWriteCloser{}
						referenceLogger := newJSONLinesLogger(reference, 1<<16)
						referenceLogger.LogEvent(simTime, clientID, eventName,
							venueEnvelope{VenueID: venueID, Payload: payload})
						if err := referenceLogger.Close(); err != nil {
							t.Fatalf("reference close: %v", err)
						}

						candidate := &collectingWriteCloser{}
						candidateLogger := newJSONLinesLogger(candidate, 1<<16)
						candidateLogger.LogEncodedEvent(simTime, clientID, eventName,
							prefix, encodedPayload, []byte("}"))
						if err := candidateLogger.Close(); err != nil {
							t.Fatalf("candidate close: %v", err)
						}

						if !bytes.Equal(reference.buffer.Bytes(), candidate.buffer.Bytes()) {
							t.Fatalf("venue %q event %q client %d time %d payload %d:\n reference %s candidate %s",
								venueID, eventName, clientID, simTime, index,
								reference.buffer.Bytes(), candidate.buffer.Bytes())
						}
						referenceDigest, err := referenceLogger.EvidenceDigest()
						if err != nil {
							t.Fatalf("reference digest: %v", err)
						}
						candidateDigest, err := candidateLogger.EvidenceDigest()
						if err != nil {
							t.Fatalf("candidate digest: %v", err)
						}
						if referenceDigest.Hex() != candidateDigest.Hex() ||
							referenceDigest.Events != candidateDigest.Events {
							t.Fatalf("evidence digest differs for payload %d: %s/%d vs %s/%d",
								index, referenceDigest.Hex(), referenceDigest.Events,
								candidateDigest.Hex(), candidateDigest.Events)
						}
					}
				}
			}
		}
	}
}

// TestLogEncodedEventReusesItsBufferSafely writes many records through one
// logger, which is the shape production uses, so a stale assembly buffer would
// show up as a corrupted line rather than passing unnoticed.
func TestLogEncodedEventReusesItsBufferSafely(t *testing.T) {
	sink := &collectingWriteCloser{}
	logger := newJSONLinesLogger(sink, 1<<16)
	prefix := []byte(`{"venue_id":"north","payload":`)

	var want bytes.Buffer
	for i := 0; i < 500; i++ {
		payload := map[string]any{"index": int64(i), "filler": string(bytes.Repeat([]byte("x"), i))}
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		logger.LogEncodedEvent(int64(i), uint64(i), "OrderFill", prefix, encoded, []byte("}"))

		reference, err := json.Marshal(persistedEvent{
			ClientID: uint64(i), Data: venueEnvelope{VenueID: "north", Payload: payload},
			Event: "OrderFill", SimTS: int64(i),
		})
		if err != nil {
			t.Fatal(err)
		}
		want.Write(reference)
		want.WriteByte('\n')
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !bytes.Equal(want.Bytes(), sink.buffer.Bytes()) {
		t.Fatalf("reused buffer produced different bytes over 500 records")
	}
}

// TestLogEncodedEventPropagatesTransportFailure requires the fast path to fail
// closed exactly as LogEvent does: a write error must be retained, must stop
// further records, and must surface from EvidenceDigest.
func TestLogEncodedEventPropagatesTransportFailure(t *testing.T) {
	failure := io.ErrClosedPipe
	writes := 0
	logger := newJSONLinesLogger(&injectedWriteCloser{write: func(p []byte) (int, error) {
		writes++
		return 0, failure
	}}, 1)
	prefix := []byte(`{"venue_id":"north","payload":`)
	for i := 0; i < 3; i++ {
		logger.LogEncodedEvent(int64(i), 1, "OrderFill", prefix, []byte(`{"a":1}`), []byte("}"))
	}
	if _, err := logger.EvidenceDigest(); err == nil {
		t.Fatal("evidence digest must not be reported after a write failure")
	}
	if writes == 0 {
		t.Fatal("expected at least one attempted write")
	}
}

// TestLogEncodedEventShortWriteIsAFailure guards the short-write branch, which a
// buffered writer can reach on a partially accepting transport.
func TestLogEncodedEventShortWriteIsAFailure(t *testing.T) {
	logger := newJSONLinesLogger(&injectedWriteCloser{write: func(p []byte) (int, error) {
		return len(p) - 1, nil
	}}, 1)
	logger.LogEncodedEvent(1, 1, "OrderFill", []byte(`{"venue_id":"north","payload":`),
		[]byte(`{"a":1}`), []byte("}"))
	if _, err := logger.EvidenceDigest(); err == nil {
		t.Fatal("a short write must be retained as a transport failure")
	}
}
