package multivenue

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The binary sink once wrote a checkpoint claiming zero events and an all-zero
// execution hash, because observe returned before the attestation ran. Nothing
// failed: the file existed, parsed, and carried the right domain and ordering.
// Any two binary runs therefore attested identically, so the divergence locator
// would have called genuinely divergent runs identical.
//
// These tests pin the properties that failure violated, rather than pinning a
// particular digest, which would only pin the current encoder.

func readCheckpoints(t *testing.T, dir string) []checkpointRecord {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "checkpoints.jsonl"))
	if err != nil {
		t.Fatalf("read checkpoints: %v", err)
	}
	var records []checkpointRecord
	decoder := json.NewDecoder(bytes.NewReader(raw))
	for decoder.More() {
		var record checkpointRecord
		if err := decoder.Decode(&record); err != nil {
			t.Fatalf("decode checkpoint: %v", err)
		}
		records = append(records, record)
	}
	return records
}

// feed drives a sink through a fixed event sequence, optionally altering one
// payload so two otherwise identical trajectories can be compared.
func feed(t *testing.T, dir string, alteredAt int) []checkpointRecord {
	t.Helper()
	t.Setenv("EXSIM_BINARY_EVIDENCE", "1")
	sink, err := newCheckpointSink(dir, 1, 0, 0)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if sink == nil {
		t.Fatal("sink was not created")
	}
	for i := 0; i < 2000; i++ {
		quantity := int64(100 + i)
		if i == alteredAt {
			quantity++
		}
		sink.observe(int64(i)*1e6, uint64(i%7), "OrderFill", "venue-a",
			map[string]any{"quantity": quantity, "price": int64(50000)})
	}
	if err := sink.close(); err != nil {
		t.Fatalf("close sink: %v", err)
	}
	return readCheckpoints(t, dir)
}

func TestBinarySinkWritesARealAttestation(t *testing.T) {
	records := feed(t, t.TempDir(), -1)
	if len(records) == 0 {
		t.Fatal("binary mode wrote no checkpoints at all")
	}
	last := records[len(records)-1]
	if last.EventCount != 2000 {
		t.Fatalf("event_count = %d, want 2000 — the attestation is not counting the events it recorded",
			last.EventCount)
	}
	zero := "0000000000000000000000000000000000000000000000000000000000000000"
	if last.ExecutionStreamHash == zero || last.ExecutionStreamHash == "" {
		t.Fatalf("execution_stream_hash = %q — binary mode is attesting to nothing",
			last.ExecutionStreamHash)
	}
	if last.Representation != binaryRepresentation {
		t.Fatalf("representation = %q, want %q — a binary hash must not be mistaken for a JSON one",
			last.Representation, binaryRepresentation)
	}
}

// The property that actually matters: a different trajectory must attest
// differently. A constant hash passes every "is it non-empty" check and is
// still worthless.
func TestBinaryAttestationSeparatesDivergentRuns(t *testing.T) {
	base := feed(t, t.TempDir(), -1)
	same := feed(t, t.TempDir(), -1)
	diverged := feed(t, t.TempDir(), 1500)

	if base[len(base)-1].ExecutionStreamHash != same[len(same)-1].ExecutionStreamHash {
		t.Fatal("two identical binary runs attested differently")
	}
	if base[len(base)-1].ExecutionStreamHash == diverged[len(diverged)-1].ExecutionStreamHash {
		t.Fatal("a run with an altered payload attested identically — the hash does not cover the payload")
	}
}

// A trace the binary path cannot produce must fail the run rather than leave an
// empty file that reads as "nothing happened in the window".
func TestBinarySinkRefusesATraceItCannotProduce(t *testing.T) {
	t.Setenv("EXSIM_BINARY_EVIDENCE", "1")
	if _, err := newCheckpointSink(t.TempDir(), 1, 0, 1e9); err == nil {
		t.Fatal("a trace window was accepted under binary evidence, and would have written an empty file")
	}
}


// An independent reproducer found that a payload the encoder rejects used to
// end the binary stream: the sink recorded 1 of 102 events and dropped the
// other 101 while the simulation ran on. The failure did surface at close, but
// only after a whole run's evidence had already been lost.
func TestBinarySinkSurvivesAnUnencodablePayload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EXSIM_BINARY_EVIDENCE", "1")
	sink, err := newCheckpointSink(dir, 1, 0, 0)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	for i := 0; i < 102; i++ {
		var payload any = map[string]any{"quantity": int64(i)}
		if i == 1 {
			payload = map[string]any{"bad": make(chan int)}
		}
		sink.observe(int64(i)*1e6, 1, "OrderFill", "venue-a", payload)
	}
	if err := sink.close(); err != nil {
		t.Fatalf("close: one bad payload failed the whole run: %v", err)
	}
	records := readCheckpoints(t, dir)
	last := records[len(records)-1]
	if last.EventCount != 102 {
		t.Fatalf("event_count = %d, want 102 — the stream stopped at the bad payload", last.EventCount)
	}
	if last.Unencodable != 1 {
		t.Fatalf("unencodable_payloads = %d, want 1 — the substitution is invisible", last.Unencodable)
	}
}
