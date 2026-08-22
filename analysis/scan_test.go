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
