package multivenue

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestME005ProspectiveConfigsNormalizeWithoutEconomicRun(t *testing.T) {
	expectedHashes := map[string]string{
		"off-1109": "c121ae21036019d97edf1ed0ca60e8587250e8e842582b768f12447455016603",
		"on-1109":  "3a4e6a37f269dba160bafd5bbe354d53d84431f9deca3306a088a589558ab0e2",
		"off-1117": "fb7d72c1d8c0bae6ea8cb005d450df9b02ad7eca6765c45d830e9f5cc148c1b5",
		"on-1117":  "ec5a09120d64880a684086549a7970d24098dcb46f92f4214741515dd11192b9",
	}
	for _, seed := range []int64{1109, 1117} {
		for _, arm := range []string{"off", "on"} {
			cell := fmt.Sprintf("%s-%d", arm, seed)
			t.Run(cell, func(t *testing.T) {
				path := filepath.Join("..", "..", "research", "program", "ideas", "ME-005", fmt.Sprintf("config-%s-%d.json", arm, seed))
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				cfg, err := DecodeConfig(raw)
				if err != nil {
					t.Fatal(err)
				}
				cfg.LogDir = filepath.Join(t.TempDir(), "unused-construction-sink")
				if err := cfg.normalize(); err != nil {
					t.Fatal(err)
				}
				if cfg.Seed != seed || len(cfg.VenueIDs) != 2 || cfg.VenueIDs[0] != "north" || cfg.VenueIDs[1] != "south" ||
					cfg.EvidenceFormat != binaryRepresentation || cfg.EvidenceContractVersion != 2 ||
					cfg.AutoBorrowSpot == nil || *cfg.AutoBorrowSpot || !cfg.StrictPopulationAccounting || !cfg.StrictRiskContract ||
					cfg.CrossVenueArbLotQty != 1_000_000 || cfg.TakerFeeBps != 5 {
					t.Fatalf("ME-005 effective contract differs: %+v", cfg)
				}
				if arm == "on" && (len(cfg.CrossVenueArbTiers) != 1 || cfg.CrossVenueArbMaxAttempts != 1 ||
					cfg.CrossVenueArbInitialBase != 100_000_000 || cfg.CrossVenueArbInitialQuote != 10_000_000_000 ||
					!cfg.RecordMarketDataReceipts || !cfg.RecordDecisionFrontierVectors || !cfg.RecordCrossVenueArbEvaluations) {
					t.Fatalf("ME-005 ON config lost router contract: %+v", cfg)
				}
				if arm == "off" && (len(cfg.CrossVenueArbTiers) != 0 || cfg.CrossVenueArbMaxAttempts != 0 || cfg.RecordCrossVenueArbEvaluations) {
					t.Fatalf("ME-005 OFF config enabled router: %+v", cfg)
				}
				cfg.LogDir = ""
				effective, err := json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if digest := fmt.Sprintf("%x", sha256.Sum256(effective)); digest != expectedHashes[cell] {
					t.Fatalf("effective config changed: %s, want %s", digest, expectedHashes[cell])
				}
			})
		}
	}
}
