package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFundingIntervalsFromRunConfig(t *testing.T) {
	dir := t.TempDir()
	config := []byte(`{"funding_interval_seconds":28800,"venue_rules":{"central":{"funding_interval_seconds":3600},"north":{"funding_interval_seconds":28800},"south":{"funding_interval_seconds":7200}}}`)
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	intervals, err := loadFundingIntervals(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"central": 3600, "north": 28800, "south": 7200}
	if len(intervals) != len(want) {
		t.Fatalf("intervals = %#v, want %#v", intervals, want)
	}
	for venueID, expected := range want {
		if intervals[venueID] != expected {
			t.Errorf("%s interval = %d, want %d", venueID, intervals[venueID], expected)
		}
	}
}

func TestLoadFundingIntervalsAllowsLegacyRunWithoutConfig(t *testing.T) {
	intervals, err := loadFundingIntervals(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if intervals != nil {
		t.Fatalf("missing run config returned %#v, want nil fallback", intervals)
	}
}

func TestLoadFundingIntervalsRejectsInvalidVenueCadence(t *testing.T) {
	dir := t.TempDir()
	config := []byte(`{"venue_rules":{"north":{"funding_interval_seconds":0}}}`)
	if err := os.WriteFile(filepath.Join(dir, "run-config.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFundingIntervals(dir); err == nil {
		t.Fatal("invalid venue cadence was accepted")
	}
}
