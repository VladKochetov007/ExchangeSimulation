package multivenue

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"exchange_sim/simulations/feesim"
)

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
