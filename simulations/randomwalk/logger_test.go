package randomwalk

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLinesLoggerSnapshotFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	logger, err := NewJSONLinesLogger(path)
	if err != nil {
		t.Fatalf("NewJSONLinesLogger: %v", err)
	}
	logger.filterSnapshots = true
	logger.LogEvent(1, 0, "Trade", map[string]any{"price": 1})
	logger.LogEvent(2, 0, "BookSnapshot", map[string]any{"bids": []any{}})
	logger.Close()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("expected the book snapshot")
	}
	var record struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if record.Event != "BookSnapshot" {
		t.Fatalf("event = %q, want BookSnapshot", record.Event)
	}
	if scanner.Scan() {
		t.Fatal("unexpected non-snapshot event")
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read log: %v", err)
	}
}
