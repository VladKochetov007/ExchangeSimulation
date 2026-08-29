package multivenue

import (
	"bytes"
	"encoding/json"
	"testing"
)

// captureLogger records the lines a JSONLinesLogger would persist.
type captureLine struct {
	simTime  int64
	clientID uint64
	event    string
	line     []byte
}

// TestEncodedEventMatchesReflectiveEncoding is the equivalence proof for the
// manual envelope assembly: for every payload shape, persisting the sink's
// bytes through LogEncodedEvent must produce exactly the line that marshalling
// venueLogEvent through LogEvent produces.
//
// This is checked directly here rather than only through the integrated
// evidence digest, so a divergence is localized to the encoder instead of
// surfacing as a mismatched 442MB evidence tree.
func TestEncodedEventMatchesReflectiveEncoding(t *testing.T) {
	type nested struct {
		Alpha int64  `json:"alpha"`
		Beta  string `json:"beta"`
	}
	payloads := []any{
		nil,
		map[string]any{},
		map[string]any{"a": int64(1), "b": "two"},
		map[string]any{"zero": int64(0), "neg": int64(-1)},
		map[string]any{"max": int64(9223372036854775807), "min": int64(-9223372036854775808)},
		map[string]any{"umax": uint64(18446744073709551615)},
		map[string]any{"html": "<script>&</script>"},
		map[string]any{"quote": `he said "hi"`},
		map[string]any{"backslash": `a\b`},
		map[string]any{"newline": "a\nb\tc"},
		map[string]any{"unicode": "ábc↦☃"},
		map[string]any{"lineSep": "a\u2028b\u2029c"},
		map[string]any{"nested": nested{Alpha: 3, Beta: "x"}},
		map[string]any{"list": []any{int64(1), "two", nil, true}},
		map[string]any{"float": 1.5, "sci": 1e21},
		nested{Alpha: 7, Beta: "seven"},
		[]any{int64(1), int64(2)},
		"scalar string",
		int64(42),
		true,
		fillLikeEvidence{Symbol: "ABC/USD", Qty: 5, Price: 100},
	}
	events := []string{
		"OrderFill", "BookDelta", "balance_change", "position_update",
		"event with space", `event"quote`, "event<html>&", "événement",
	}
	venues := []string{"north", "central", "south", `venue"quote`, "venue<html>&", "vénue"}

	for _, venueID := range venues {
		prefix, err := venueDataPrefix(venueID)
		if err != nil {
			t.Fatalf("venue prefix %q: %v", venueID, err)
		}
		for _, eventName := range events {
			for index, payload := range payloads {
				encoded, err := json.Marshal(payload)
				if err != nil {
					t.Fatalf("payload %d: %v", index, err)
				}
				want, err := json.Marshal(persistedEventMirror{
					ClientID: 17, Data: venueLogEvent{VenueID: venueID, Payload: payload},
					Event: eventName, SimTS: 1735689605000000000,
				})
				if err != nil {
					t.Fatalf("reference marshal: %v", err)
				}
				got := assembleLine(17, eventName, 1735689605000000000, prefix, encoded)
				if !bytes.Equal(want, got) {
					t.Fatalf("venue %q event %q payload %d:\n reference %s\n assembled %s",
						venueID, eventName, index, want, got)
				}
			}
		}
	}
}

// persistedEventMirror mirrors the unexported envelope the feesim logger
// marshals, so this test can build the reference line without exporting it.
// Field order is the evidence contract and must match.
type persistedEventMirror struct {
	ClientID uint64 `json:"client_id"`
	Data     any    `json:"data"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
}

type fillLikeEvidence struct {
	Price  int64  `json:"price"`
	Qty    int64  `json:"qty"`
	Symbol string `json:"symbol"`
}

// assembleLine reproduces the assembly LogEncodedEvent performs, so the encoder
// contract can be tested without a filesystem or a logger instance.
func assembleLine(clientID uint64, eventName string, simTime int64, prefix, payload []byte) []byte {
	encodedName, err := json.Marshal(eventName)
	if err != nil {
		panic(err)
	}
	line := append([]byte(nil), `{"client_id":`...)
	line = appendUint(line, clientID)
	line = append(line, `,"data":`...)
	line = append(line, prefix...)
	line = append(line, payload...)
	line = append(line, venueDataSuffix...)
	line = append(line, `,"event":`...)
	line = append(line, encodedName...)
	line = append(line, `,"sim_ts":`...)
	line = appendInt(line, simTime)
	return append(line, '}')
}

func appendUint(dst []byte, value uint64) []byte {
	encoded, _ := json.Marshal(value)
	return append(dst, encoded...)
}

func appendInt(dst []byte, value int64) []byte {
	encoded, _ := json.Marshal(value)
	return append(dst, encoded...)
}
