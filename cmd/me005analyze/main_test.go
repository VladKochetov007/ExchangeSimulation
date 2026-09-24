package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestME005AnalyzeRequiresPinnedContractBeforeRendering(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("missing analysis inputs accepted")
	}
	root := t.TempDir()
	rawDir := filepath.Join(root, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(root, "contract.json")
	if err := os.WriteFile(contractPath, []byte(`{"schema_version":1,"arm":"ON"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rendered := filepath.Join(root, "rendered")
	if err := run([]string{"-raw", rawDir, "-rendered", rendered,
		"-contract", contractPath, "-out", filepath.Join(root, "result.json")}); err == nil {
		t.Fatal("incomplete contract accepted")
	}
	if _, err := os.Stat(rendered); !os.IsNotExist(err) {
		t.Fatalf("invalid contract created a rendered evidence namespace: %v", err)
	}
}
