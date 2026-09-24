package crossvenue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestME005ResultPathStaysOutsideImmutableEvidence(t *testing.T) {
	root := t.TempDir()
	rawDir, renderedDir := filepath.Join(root, "raw"), filepath.Join(root, "rendered")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResultPath(rawDir, renderedDir, filepath.Join(root, "result.json")); err != nil {
		t.Fatalf("separate new result rejected: %v", err)
	}
	for _, path := range []string{filepath.Join(rawDir, "score.json"), filepath.Join(renderedDir, "score.json")} {
		if err := ValidateResultPath(rawDir, renderedDir, path); err == nil {
			t.Fatalf("result inside evidence was accepted: %s", path)
		}
	}
	alias := filepath.Join(root, "raw-alias")
	if err := os.Symlink(rawDir, alias); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResultPath(rawDir, renderedDir, filepath.Join(alias, "score.json")); err == nil {
		t.Fatal("symlink alias into raw evidence was accepted")
	}
	existing := filepath.Join(root, "existing.json")
	if err := os.WriteFile(existing, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateResultPath(rawDir, renderedDir, existing); err == nil {
		t.Fatal("existing result was accepted for overwrite")
	}
}
