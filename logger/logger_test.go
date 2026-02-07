package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLoggerNil(t *testing.T) {
	log := New(nil)
	if log != nil {
		t.Fatal("expected nil logger when writer is nil")
	}
}

func TestLogEvent(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	event := map[string]any{
		"order_id": uint64(123),
		"price":    int64(5000000),
		"qty":      int64(100),
	}

	log.LogEvent(1000, 42, "OrderAccepted", event)

	line := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["sim_time"].(float64) != 1000 {
		t.Errorf("expected sim_time=1000, got %v", parsed["sim_time"])
	}
	if parsed["client_id"].(float64) != 42 {
		t.Errorf("expected client_id=42, got %v", parsed["client_id"])
	}
	if parsed["event"].(string) != "OrderAccepted" {
		t.Errorf("expected event=OrderAccepted, got %v", parsed["event"])
	}
	if parsed["order_id"].(float64) != 123 {
		t.Errorf("expected order_id=123, got %v", parsed["order_id"])
	}
	if parsed["price"].(float64) != 5000000 {
		t.Errorf("expected price=5000000, got %v", parsed["price"])
	}
	if parsed["qty"].(float64) != 100 {
		t.Errorf("expected qty=100, got %v", parsed["qty"])
	}
}

func TestLogEventNilEvent(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.LogEvent(1000, 42, "TestEvent", nil)

	line := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["sim_time"].(float64) != 1000 {
		t.Errorf("expected sim_time=1000, got %v", parsed["sim_time"])
	}
	if parsed["event"].(string) != "TestEvent" {
		t.Errorf("expected event=TestEvent, got %v", parsed["event"])
	}
}

func TestLogMultipleEvents(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.LogEvent(1000, 1, "Event1", map[string]any{"field1": "value1"})
	log.LogEvent(2000, 2, "Event2", map[string]any{"field2": "value2"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var parsed1 map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &parsed1); err != nil {
		t.Fatalf("failed to parse first line: %v", err)
	}
	if parsed1["event"].(string) != "Event1" {
		t.Errorf("expected Event1, got %v", parsed1["event"])
	}

	var parsed2 map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &parsed2); err != nil {
		t.Fatalf("failed to parse second line: %v", err)
	}
	if parsed2["event"].(string) != "Event2" {
		t.Errorf("expected Event2, got %v", parsed2["event"])
	}
}
