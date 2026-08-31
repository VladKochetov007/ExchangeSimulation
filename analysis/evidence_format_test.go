package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A run whose venue JSONL was replaced by a binary evidence stream still has a
// venues directory, because the evidence-only families are still written there.
// It therefore reads as a valid but much quieter run rather than as an
// unreadable one, and metrics report an absence of findings instead of an
// error. Measured against a JSON run of the same seed, 26 of 32 metrics differed
// while exiting 0 — `viability` reported zero books, which reads as a pass, and
// `conservation` reported 720 broken chain links that do not exist.
func TestOpenRefusesEvidenceItCannotRead(t *testing.T) {
	dir := t.TempDir()
	writeRunFiles(t, dir, `{"schema_version":2,"evidence_format":"evstream_v1"}`)

	_, err := Open(dir)
	if err == nil {
		t.Fatal("a run storing evidence in an unreadable format was opened, and every metric would have reported a quieter run")
	}
	if !strings.Contains(err.Error(), "evstream_v1") {
		t.Fatalf("error does not name the format the caller has to act on: %v", err)
	}
}

// The field is absent on every run written before the binary sink existed, and
// those runs must keep opening.
func TestOpenAcceptsRunsWithoutTheField(t *testing.T) {
	dir := t.TempDir()
	writeRunFiles(t, dir, `{"schema_version":2}`)
	if _, err := Open(dir); err != nil {
		t.Fatalf("a JSONL run was refused: %v", err)
	}
}

// A run predating the manifest entirely cannot be in a replaced format, since
// that format postdates the manifest.
func TestOpenAcceptsRunsWithoutAManifest(t *testing.T) {
	dir := t.TempDir()
	writeRunFiles(t, dir, "")
	if _, err := Open(dir); err != nil {
		t.Fatalf("a run without a manifest was refused: %v", err)
	}
}

func writeRunFiles(t *testing.T, dir, manifest string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	if manifest == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
