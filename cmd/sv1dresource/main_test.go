package main

import (
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/analysis"
)

func TestPublishMeasurementRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "measurement.json")
	measurement := analysis.SV1DResourceMeasurement{SchemaVersion: 1, Contract: analysis.SV1DResourceMeasurementContract}
	if err := publishMeasurement(path, measurement); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishMeasurement(path, measurement); err == nil {
		t.Fatal("measurement publisher overwrote an existing record")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing measurement record changed")
	}
}
