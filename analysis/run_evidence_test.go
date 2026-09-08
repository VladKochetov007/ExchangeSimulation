package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRejectsUnknownEvidenceFormat(t *testing.T) {
	dir := writeRun(t, Report{}, nil)
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), []byte(`{"evidence_format":"future_format"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "unsupported evidence format") {
		t.Fatalf("unknown evidence format error = %v", err)
	}
}

func TestOpenRejectsConflictingEvidenceDescriptors(t *testing.T) {
	dir := writeRun(t, Report{}, nil)
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), []byte(`{"evidence_format":"evstream_v3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run-metadata.json"), []byte(`{"evidence_format":"jsonl"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "conflicting evidence formats") {
		t.Fatalf("conflicting evidence format error = %v", err)
	}
}
