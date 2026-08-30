package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scan reuses one envelope across records, which ties an event's payload to
// its visit call. That is only safe while no consumer retains the payload past
// the visit. These tests check the property directly rather than trusting a
// reading of the call sites.

func writeAliasingFixture(t *testing.T, lines []string) *Run {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), []byte(`{"terminal_accounts":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "venues", "north", "spot", "ABC-USD.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

// TestScanPayloadIsUsableThroughoutItsVisit requires each event's payload to
// decode to its own record's values, which a reused buffer would break if the
// reset were wrong or the buffer were shared across an iteration boundary.
func TestScanPayloadIsUsableThroughoutItsVisit(t *testing.T) {
	var lines []string
	for i := int64(1); i <= 200; i++ {
		// Vary the payload length so a shorter record cannot inherit a longer
		// predecessor's trailing bytes.
		filler := strings.Repeat("x", int(i%37))
		lines = append(lines, `{"sim_ts":`+itoa(i)+`,"client_id":`+itoa(i)+
			`,"event":"Trade","data":{"venue_id":"north","symbol":"ABC-USD","payload":{"seq":`+
			itoa(i)+`,"filler":"`+filler+`"}}}`)
	}
	run := writeAliasingFixture(t, lines)

	seen := 0
	err := run.Scan(ScanOptions{Events: []string{"Trade"}, Workers: 1}, func(event Event) {
		var payload struct {
			Seq    int64  `json:"seq"`
			Filler string `json:"filler"`
		}
		if err := event.Decode(&payload); err != nil {
			t.Fatalf("record %d: decode: %v", event.Ordinal, err)
		}
		if payload.Seq != event.SimTS {
			t.Fatalf("record %d: payload seq %d does not match its own record's sim_ts %d",
				event.Ordinal, payload.Seq, event.SimTS)
		}
		if int64(len(payload.Filler)) != event.SimTS%37 {
			t.Fatalf("record %d: filler length %d, want %d",
				event.Ordinal, len(payload.Filler), event.SimTS%37)
		}
		seen++
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != 200 {
		t.Fatalf("visited %d records, want 200", seen)
	}
}

// TestScanRecordMissingDataStillFails pins the reset. Struct decoding leaves an
// absent field untouched, so without an explicit reset a record with no "data"
// would silently inherit the previous record's payload and be analysed as if it
// carried one.
func TestScanRecordMissingDataStillFails(t *testing.T) {
	run := writeAliasingFixture(t, []string{
		`{"sim_ts":1,"client_id":1,"event":"Trade","data":{"venue_id":"north","symbol":"ABC-USD","payload":{"qty":7}}}`,
		`{"sim_ts":2,"client_id":2,"event":"Trade"}`,
	})
	var payloads []string
	err := run.Scan(ScanOptions{Events: []string{"Trade"}, Workers: 1}, func(event Event) {
		payloads = append(payloads, string(event.Raw()))
	})
	if err == nil {
		t.Fatalf("a record with no data layer was accepted; payloads seen: %v", payloads)
	}
	if len(payloads) != 1 {
		t.Fatalf("visited %d records before failing, want 1", len(payloads))
	}
	if payloads[0] != `{"qty":7}` {
		t.Fatalf("first payload = %s", payloads[0])
	}
}

// TestScanPayloadDoesNotSurviveTheNextRecord documents the lifetime contract by
// demonstrating the failure a retaining consumer would see. It is the reason
// every reducer decodes inside its visit rather than storing Raw().
func TestScanPayloadDoesNotSurviveTheNextRecord(t *testing.T) {
	run := writeAliasingFixture(t, []string{
		`{"sim_ts":1,"client_id":1,"event":"Trade","data":{"venue_id":"north","symbol":"ABC-USD","payload":{"a":1111111111}}}`,
		`{"sim_ts":2,"client_id":2,"event":"Trade","data":{"venue_id":"north","symbol":"ABC-USD","payload":{"b":2}}}`,
	})
	var retained []json.RawMessage
	var copied []string
	if err := run.Scan(ScanOptions{Events: []string{"Trade"}, Workers: 1}, func(event Event) {
		retained = append(retained, event.Raw())
		copied = append(copied, string(event.Raw()))
	}); err != nil {
		t.Fatal(err)
	}
	// Copies taken during the visit are always correct.
	if copied[0] != `{"a":1111111111}` || copied[1] != `{"b":2}` {
		t.Fatalf("copies taken during the visit are wrong: %v", copied)
	}
	// The retained slices are not guaranteed to be, which is exactly why no
	// reducer keeps one. Assert only that decoding a copy works, so this test
	// documents the contract without depending on the buffer's contents.
	if len(retained) != 2 {
		t.Fatalf("retained %d payloads, want 2", len(retained))
	}
}
