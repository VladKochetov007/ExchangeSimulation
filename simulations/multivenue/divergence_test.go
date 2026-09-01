package multivenue

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"exchange_sim/simulations/feesim"
)

type checkpointFailWriter struct {
	bytes.Buffer
	err error
}

func (w *checkpointFailWriter) Write([]byte) (int, error) { return 0, w.err }
func (w *checkpointFailWriter) Close() error              { return nil }

func TestCheckpointSinkReportsTransportFailure(t *testing.T) {
	want := errors.New("injected checkpoint write failure")
	sink := &checkpointSink{
		intervalNano: 1,
		checkpoints:  &checkpointFailWriter{err: want},
		firstEvent:   true,
	}
	sink.observe(1, 1, "event", "north", map[string]int{"n": 1}, "general.jsonl", 1)
	if err := sink.close(); !errors.Is(err, want) {
		t.Fatalf("checkpoint close error = %v, want %v", err, want)
	}
	if err := sink.close(); !errors.Is(err, want) {
		t.Fatalf("repeat checkpoint close error = %v, want %v", err, want)
	}
}

func TestCheckpointSinkClosesAtRegisteredFinalTime(t *testing.T) {
	const second = int64(1_000_000_000)
	dir := t.TempDir()
	sink, err := newCheckpointSink(dir, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	sink.finalSimTime = 3 * second
	sink.observe(1*second, 1, "event", "north", map[string]int{"n": 1}, "general.jsonl", 1)
	sink.observe(2*second, 1, "event", "north", map[string]int{"n": 2}, "general.jsonl", 2)
	sink.observe(3*second, 1, "event", "north", map[string]int{"n": 3}, "general.jsonl", 3)
	sink.observe(3*second, 2, "event", "north", map[string]int{"n": 4}, "general.jsonl", 4)
	if err := sink.close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dir + "/checkpoints.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var records []checkpointRecord
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var record checkpointRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if len(records) != 3 || records[len(records)-2].SimTime != 3*second || records[len(records)-2].Final || !records[len(records)-1].Final || records[len(records)-1].SimTime != 3*second || records[len(records)-2].EventCount != 4 || records[len(records)-2].EventCount != records[len(records)-1].EventCount || records[len(records)-2].ExecutionStreamHash != records[len(records)-1].ExecutionStreamHash {
		t.Fatalf("checkpoint closure = %+v, want ordinary/final duplicate at %d", records, 3*second)
	}
	for index := 1; index < len(records)-1; index++ {
		if records[index-1].SimTime >= records[index].SimTime {
			t.Fatalf("ordinary checkpoints are not strictly increasing: %+v", records)
		}
	}
}

var _ io.WriteCloser = (*checkpointFailWriter)(nil)

func TestMakerTelemetryReusesGlobalCheckpointLogger(t *testing.T) {
	dir := t.TempDir()
	sink, err := newCheckpointSink(dir, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := feesim.NewJSONLinesLogger(dir + "/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	global := venueLogger{venueID: "north", inner: inner, sink: sink}
	makerStateLog := global
	makerStateLog.LogEvent(1, 0, "maker_state", map[string]int{"x": 1})
	makerStateLog.LogEvent(2, 0, "conservation_violation", map[string]int{"x": 2})
	sink.close()
	inner.Close()
	if sink.events != 2 {
		t.Fatalf("checkpoint observations = %d, want 2", sink.events)
	}
	file, err := os.Open(dir + "/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var names []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var row struct {
			Event string `json:"event"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		names = append(names, row.Event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "maker_state" || names[1] != "conservation_violation" {
		t.Fatalf("persisted telemetry events = %v, want one of each", names)
	}
}

func TestMakerQuoteSizeEvidenceBypassesExecutionCheckpoint(t *testing.T) {
	dir := t.TempDir()
	sink, err := newCheckpointSink(dir, 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := feesim.NewJSONLinesLogger(dir + "/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	logger := venueLogger{venueID: "north", inner: inner, sink: sink}
	logger.LogEvidenceOnly(1, 7, "maker_quote_size_decision", MakerQuoteSizeDecision{Maker: "spot_maker_1", ClientID: 7, Symbol: "ABC/USD", DecisionTime: 1, BidRequestID: 1, AskRequestID: 2, BaseVolatilitySize: 10, InventoryLimit: 10, BidPrice: 99, AskPrice: 101, BidQty: 10, AskQty: 10})
	sink.close()
	inner.Close()
	if sink.events != 0 {
		t.Fatalf("evidence-only record changed execution observations: %d", sink.events)
	}
	file, err := os.Open(dir + "/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("persisted maker quote-size evidence is missing")
	}
	var row struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row.Event != "maker_quote_size_decision" {
		t.Fatalf("persisted evidence event = %q", row.Event)
	}
	if scanner.Scan() {
		t.Fatal("persisted maker quote-size evidence has an unexpected extra record")
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestBinaryReplacementKeepsOnlySequencedEvidenceOnlySidecars(t *testing.T) {
	dir := t.TempDir()
	inner, err := feesim.NewJSONLinesLogger(dir + "/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	sink := &checkpointSink{binary: newBinaryEvidence(io.Discard), replaceRaw: true}
	var sequence uint64
	logger := venueLogger{venueID: "north", route: "general.jsonl", inner: inner, sink: sink, sequence: &sequence}
	logger.LogEvent(1, 7, "hashed_event", map[string]int{"value": 1})
	logger.LogEvidenceOnly(2, 8, "sidecar_event", map[string]int{"value": 2})
	if err := sink.close(); err != nil {
		t.Fatal(err)
	}
	if err := inner.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dir + "/evidence.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Event string `json:"event"`
		Data  struct {
			Sequence uint64 `json:"sequence"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(raw), &record); err != nil {
		t.Fatal(err)
	}
	if record.Event != "sidecar_event" || record.Data.Sequence != 2 {
		t.Fatalf("sidecar record = %+v, want sidecar_event sequence 2", record)
	}
}
