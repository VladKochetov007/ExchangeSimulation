package multivenue

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/evstream/synthetic"
)

func TestSyntheticCapacityUsesProductionGlobalHashProjection(t *testing.T) {
	header := evstream.AppendFrameHeader(nil, evstream.FrameHeader{
		Length:        evstream.FrameHeaderSize + 32,
		Seq:           41,
		SimTS:         1735689600000000000,
		SchemaID:      evstream.FirstUserSchema,
		SchemaVersion: 1,
		VenueRef:      3,
		ClientID:      7,
	})
	frame := append(header, make([]byte, 32)...)
	frame[evstream.FrameHeaderSize+8] = 1
	frame[evstream.FrameHeaderSize+16] = 1
	production := sha256.New()
	syntheticDigest := sha256.New()
	hashGlobalBinaryExecutionFrame(production, frame)
	synthetic.HashGlobalFrame(syntheticDigest, frame)
	if !bytes.Equal(production.Sum(nil), syntheticDigest.Sum(nil)) {
		t.Fatal("synthetic capacity hash projection diverged from production global sink")
	}
}

type binaryUnencodablePayload struct{}

func (binaryUnencodablePayload) MarshalJSON() ([]byte, error) {
	return nil, errors.New("payload deliberately cannot be encoded")
}

func TestBinarySinkWritesTerminatedStreamAndRealCheckpoint(t *testing.T) {
	t.Setenv("EXSIM_BINARY_EVIDENCE", "file")
	dir := t.TempDir()
	sink, err := newCheckpointSink(dir, 1, 0, 0)
	if err != nil {
		t.Fatalf("new binary sink: %v", err)
	}
	sink.finalSimTime = 2_000_000_000
	sink.observe(1_000_000_000, 7, "event", "north", map[string]int{"value": 1}, "general.jsonl", 1)
	sink.observe(1_500_000_000, 8, "event", "north", map[string]int{"value": 2}, "general.jsonl", 2)
	if err := sink.close(); err != nil {
		t.Fatalf("close binary sink: %v", err)
	}

	stream, err := os.ReadFile(filepath.Join(dir, "events.evs"))
	if err != nil {
		t.Fatalf("read binary stream: %v", err)
	}
	hashFrame, err := binaryHashFrameForContract(sink.binary.hashing)
	if err != nil {
		t.Fatalf("resolve binary hash contract: %v", err)
	}
	reader, err := evstream.NewReader(bytes.NewReader(stream), evstream.ReaderOptions{
		VerifyHash: true,
		HashFrame:  hashFrame,
	})
	if err != nil {
		t.Fatalf("new stream reader: %v", err)
	}
	var events uint64
	if err := reader.Range(func(evstream.Frame) error {
		events++
		return nil
	}); err != nil {
		t.Fatalf("read binary stream: %v", err)
	}
	if events != 2 || !reader.Terminated() {
		t.Fatalf("binary stream events=%d terminated=%t, want 2/true", events, reader.Terminated())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "checkpoints.jsonl"))
	if err != nil {
		t.Fatalf("read checkpoints: %v", err)
	}
	var records []checkpointRecord
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var record checkpointRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode checkpoint: %v", err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		t.Fatal("binary sink wrote no checkpoint")
	}
	last := records[len(records)-1]
	if last.EventCount != 2 || last.Representation != binaryRepresentation || last.ExecutionStreamHash == "" || !last.Final {
		t.Fatalf("checkpoint = %+v, want real binary attestation", last)
	}
}

func TestBinarySinkSubstitutesUnencodablePayloadWithoutDroppingTail(t *testing.T) {
	t.Setenv("EXSIM_BINARY_EVIDENCE", "file")
	var output bytes.Buffer
	sink := &binaryEvidence{writer: evstream.NewWriter(&output, evstream.WriterOptions{})}
	if err := sink.record(1, 1, "bad", "north", binaryUnencodablePayload{}, "general.jsonl", 1); err != nil {
		t.Fatal(err)
	}
	if err := sink.record(2, 1, "good", "north", map[string]int{"value": 2}, "general.jsonl", 2); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if sink.count() != 2 || sink.unencodableCount() != 1 {
		t.Fatalf("binary counts events=%d unencodable=%d, want 2/1", sink.count(), sink.unencodableCount())
	}
	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	var frames uint64
	if err := reader.Range(func(evstream.Frame) error {
		frames++
		return nil
	}); err != nil {
		t.Fatalf("read substitute stream: %v", err)
	}
	if frames != 2 {
		t.Fatalf("read %d event frames, want 2", frames)
	}
}
