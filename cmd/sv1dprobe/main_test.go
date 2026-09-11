package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadStrictJSONRejectsDuplicateAndTrailingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.json")
	for _, raw := range []string{
		`{"schema_version":1,"schema_version":1}`,
		`{"schema_version":1} {"schema_version":1}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		var document planDocument
		if err := readStrictJSON(path, &document); err == nil {
			t.Fatalf("malformed JSON was accepted: %s", raw)
		}
	}
}

func TestPublishJSONRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "result.json")
	if err := publishJSON(path, map[string]string{"value": "first"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishJSON(path, map[string]string{"value": "second"}); err == nil {
		t.Fatal("existing output was overwritten")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || !strings.Contains(string(after), "first") {
		t.Fatalf("existing output changed: before=%q after=%q", before, after)
	}
}
