package multivenue

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNoiseFlowDecisionPhaseRejectsInvalidOffsets(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "research", "configs", "frozen-baseline-2026-08-22.json"))
	if err != nil {
		t.Fatal(err)
	}
	base, err := DecodeConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	for name, offset := range map[string]time.Duration{
		"negative":    -time.Nanosecond,
		"at interval": 2 * time.Second,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			cfg.LogDir = t.TempDir()
			cfg.NoiseFlowDecisionPhaseOffset = offset
			if _, err := NewSim(time.Second, cfg); err == nil {
				t.Fatalf("phase offset %s unexpectedly accepted", offset)
			}
		})
	}
}
