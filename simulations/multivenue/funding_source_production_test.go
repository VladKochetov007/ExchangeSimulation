package multivenue

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/exchange"
)

func fundingSourceFixtureConfig(logDir string) Config {
	return Config{LogDir: logDir, LogMode: "full", EvidenceFormat: binaryRepresentation,
		EvidenceContractVersion: 2, VenueIDs: []string{"N", "S"},
		FundingSourceObservations: &FundingSourceObservationConfig{
			SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP",
			SampleSpacingSeconds: 1, SampleCount: 1,
		},
	}
}

func TestFundingSourceProductionRunnerWritesCanonicalGrid(t *testing.T) {
	logDir := t.TempDir()
	sim, err := NewSim(2*time.Second, fundingSourceFixtureConfig(logDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := sim.Run(context.Background()); err != nil {
		t.Fatalf("short synthetic source fixture: %v", err)
	}
	if err := sim.Close(); err != nil {
		t.Fatal(err)
	}
	stream, err := os.ReadFile(filepath.Join(logDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	for _, venueID := range []string{"N", "S"} {
		replayed, err := ReplayFundingSourceObservations(bytes.NewReader(stream), exchange.FundingWindowSourceConfig{
			VenueID: venueID, SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP",
			FirstBoundaryNano: start, SampleSpacingNano: int64(time.Second), SampleCount: 1,
		}, start+int64(2*time.Second))
		if err != nil || len(replayed) != 3 {
			t.Fatalf("venue %s production source grid: count=%d err=%v", venueID, len(replayed), err)
		}
		for index, record := range replayed {
			if record.Observation.TimestampNano != start+int64(index)*int64(time.Second) || record.EventSeq == 0 {
				t.Fatalf("venue %s source %d: %+v", venueID, index, record)
			}
		}
	}
}

func TestFundingSourceProductionConfigurationFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"json-evidence", func(config *Config) { config.EvidenceFormat = "jsonl"; config.EvidenceContractVersion = 1 }},
		{"prototype-contract", func(config *Config) { config.EvidenceContractVersion = 1 }},
		{"no-log", func(config *Config) { config.LogMode = "none" }},
		{"three-venues", func(config *Config) { config.VenueIDs = []string{"N", "S", "W"} }},
		{"zero-spacing", func(config *Config) { config.FundingSourceObservations.SampleSpacingSeconds = 0 }},
		{"coarse-step", func(config *Config) { config.Step = 2 * time.Second }},
		{"unbound-symbol", func(config *Config) { config.FundingSourceObservations.SpotSymbol = "OTHER/USD" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := fundingSourceFixtureConfig(t.TempDir())
			test.mutate(&config)
			if sim, err := NewSim(time.Second, config); err == nil {
				sim.Close()
				t.Fatal("invalid source contract built a runnable production world")
			}
		})
	}
	t.Run("discard", func(t *testing.T) {
		t.Setenv("EXSIM_BINARY_EVIDENCE", "discard")
		if sim, err := NewSim(time.Second, fundingSourceFixtureConfig(t.TempDir())); err == nil {
			sim.Close()
			t.Fatal("discard mode accepted required source evidence")
		}
	})
}
