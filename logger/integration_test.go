package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoggerConcurrency(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := map[string]any{
					"goroutine": id,
					"event_num": j,
				}
				log.LogEvent(int64(j), uint64(id), "TestEvent", event)
			}
		}(i)
	}

	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	expected := numGoroutines * eventsPerGoroutine
	if len(lines) != expected {
		t.Errorf("expected %d lines, got %d", expected, len(lines))
	}

	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("failed to parse line: %v", err)
		}
	}
}

func TestLoggerPerformance(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	numEvents := 10000
	event := map[string]any{
		"order_id": uint64(123),
		"price":    int64(5000000),
		"qty":      int64(100),
	}

	start := time.Now()
	for i := 0; i < numEvents; i++ {
		log.LogEvent(int64(i), uint64(i), "OrderAccepted", event)
	}
	elapsed := time.Since(start)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != numEvents {
		t.Errorf("expected %d lines, got %d", numEvents, len(lines))
	}

	avgTime := elapsed / time.Duration(numEvents)
	t.Logf("Average time per event: %v", avgTime)
	if avgTime > 100*time.Microsecond {
		t.Errorf("logging too slow: %v per event", avgTime)
	}
}

func TestLoggerWithComplexEvent(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	event := map[string]any{
		"order_id": uint64(123),
		"nested": map[string]any{
			"field1": "value1",
			"field2": 42,
		},
		"array": []int{1, 2, 3},
		"bool":  true,
		"null":  nil,
	}

	log.LogEvent(1000, 42, "ComplexEvent", event)

	line := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["order_id"].(float64) != 123 {
		t.Errorf("expected order_id=123, got %v", parsed["order_id"])
	}

	nested := parsed["nested"].(map[string]any)
	if nested["field1"].(string) != "value1" {
		t.Errorf("expected nested.field1=value1, got %v", nested["field1"])
	}

	array := parsed["array"].([]any)
	if len(array) != 3 {
		t.Errorf("expected array length 3, got %d", len(array))
	}
}

func TestLoggerEmptyEvents(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.LogEvent(1000, 1, "EmptyEvent", map[string]any{})

	line := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["event"].(string) != "EmptyEvent" {
		t.Errorf("expected event=EmptyEvent, got %v", parsed["event"])
	}
}
