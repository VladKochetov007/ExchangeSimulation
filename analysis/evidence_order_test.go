package analysis

import "testing"

func TestEvidenceOrderUsesCanonicalGlobalSequenceAcrossFiles(t *testing.T) {
	prerequisite := eventEvidenceOrder(Event{SimTS: 100, File: "z.jsonl", Ordinal: 9, GlobalSequence: 20})
	use := eventEvidenceOrder(Event{SimTS: 100, File: "a.jsonl", Ordinal: 1, GlobalSequence: 21})
	if !evidenceAfter(use, prerequisite) {
		t.Fatalf("global sequence did not establish cross-file causality")
	}
	if evidenceAfter(prerequisite, use) {
		t.Fatalf("global sequence ordering was reversible")
	}
}

func TestEvidenceOrderFallsBackForHistoricalJSON(t *testing.T) {
	left := evidenceOrder{timestamp: 100, file: "a.jsonl", ordinal: 1}
	right := evidenceOrder{timestamp: 100, file: "z.jsonl", ordinal: 1}
	if !evidenceBefore(left, right) {
		t.Fatalf("historical file-order fallback was not retained")
	}
}

func TestScanPreservesRenderedGlobalSequence(t *testing.T) {
	dir := writeRun(t, Report{}, map[string][]string{
		"north/general.jsonl": {
			`{"sim_ts":100,"client_id":1,"event_seq":42,"event":"probe","data":{"venue_id":"north","payload":{"value":1}}}`,
		},
	})
	run, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var got Event
	if err := run.Scan(ScanOptions{Workers: 1}, func(event Event) { got = event }); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got.GlobalSequence != 42 {
		t.Fatalf("global sequence = %d, want 42", got.GlobalSequence)
	}
}

func TestSizeAtUsesGlobalSequenceAtSameTimestamp(t *testing.T) {
	points := []posPoint{
		{at: 100, size: 10, file: "north.jsonl", ordinal: 7, globalSequence: 20},
		{at: 100, size: 0, file: "south.jsonl", ordinal: 1, globalSequence: 30},
	}
	size, known := sizeAt(points, 100, "control.jsonl", 1, 25)
	if !known || size != 10 {
		t.Fatalf("size at global sequence 25 = (%d, %t), want (10, true)", size, known)
	}
}
