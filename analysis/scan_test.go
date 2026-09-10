package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRejectsMalformedRelevantEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.jsonl")
	if err := os.WriteFile(path, []byte("{not-json}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run := &Run{files: []string{path}}
	err := run.Scan(ScanOptions{}, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "parse evidence record") {
		t.Fatalf("Scan error = %v, want malformed evidence failure", err)
	}
}

func TestScanRejectsMalformedRelevantDataLayer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken-data.jsonl")
	if err := os.WriteFile(path, []byte("{\"sim_ts\":1,\"client_id\":2,\"event\":\"Trade\",\"data\":\"not-an-object\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run := &Run{files: []string{path}}
	err := run.Scan(ScanOptions{}, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "parse evidence data layer") {
		t.Fatalf("Scan error = %v, want malformed evidence failure", err)
	}
}

func TestScanPreservesRenderedGlobalSequence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "global.jsonl")
	line := `{"sim_ts":1,"client_id":2,"event":"Trade","data":{"venue_id":"north","global_sequence":37,"payload":{"value":3}}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	run := &Run{files: []string{path}}
	var scanned Event
	if err := run.Scan(ScanOptions{Workers: 1}, func(event Event) { scanned = event }); err != nil {
		t.Fatal(err)
	}
	if scanned.GlobalSequence != 37 {
		t.Fatalf("global sequence = %d, want 37", scanned.GlobalSequence)
	}
}

func TestValidateGlobalSequenceAllowsDictionaryGapsAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.jsonl")
	second := filepath.Join(dir, "b.jsonl")
	if err := os.WriteFile(first, []byte(`{"sim_ts":1,"client_id":1,"event":"first","data":{"venue_id":"north","global_sequence":5,"payload":{"value":1}}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(`{"sim_ts":2,"client_id":1,"event":"second","data":{"venue_id":"south","global_sequence":2,"payload":{"value":2}}}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	audit, err := (&Run{files: []string{first, second}}).ValidateGlobalSequence(2, 5)
	if err != nil {
		t.Fatal(err)
	}
	if audit.EventCount != 2 || audit.MaximumSequence != 5 {
		t.Fatalf("global sequence audit = %+v", audit)
	}
}

func TestValidateGlobalSequenceRejectsZeroAndDuplicate(t *testing.T) {
	tests := []struct {
		name  string
		lines string
		want  string
	}{
		{
			name:  "zero",
			lines: `{"sim_ts":1,"event":"first","data":{"venue_id":"north","payload":{}}}` + "\n",
			want:  "no global frame sequence",
		},
		{
			name: "duplicate",
			lines: `{"sim_ts":1,"event":"first","data":{"venue_id":"north","global_sequence":3,"payload":{}}}` + "\n" +
				`{"sim_ts":2,"event":"second","data":{"venue_id":"south","global_sequence":3,"payload":{}}}` + "\n",
			want: "duplicate rendered global frame sequence",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "events.jsonl")
			if err := os.WriteFile(path, []byte(test.lines), 0644); err != nil {
				t.Fatal(err)
			}
			expectedEventFrames := uint64(2)
			if test.name == "zero" {
				expectedEventFrames = 1
			}
			_, err := (&Run{files: []string{path}}).ValidateGlobalSequence(expectedEventFrames, 3)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateGlobalSequenceDoesNotPreallocateFromUntrustedCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	line := `{"sim_ts":1,"event":"first","data":{"venue_id":"north","global_sequence":1,"payload":{}}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := (&Run{files: []string{path}}).ValidateGlobalSequence(^uint64(0), 1)
	if err == nil || !strings.Contains(err.Error(), "rendered event count 1") {
		t.Fatalf("validation error = %v, want bounded count mismatch", err)
	}
}
