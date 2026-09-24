package crossvenue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulations/multivenue"
)

func TestME005TenSecondBinaryFixtureExercisesBothAnalysisArms(t *testing.T) {
	for _, arm := range []string{"OFF", "ON"} {
		t.Run(arm, func(t *testing.T) {
			rawDir := filepath.Join(t.TempDir(), "raw")
			noBorrow := false
			config := multivenue.Config{
				LogDir: rawDir, LogMode: "full", EvidenceFormat: "evstream_v3", EvidenceContractVersion: 2,
				VenueIDs: []string{"north", "south"}, Seed: 42, Step: time.Second,
				SnapshotInterval: time.Second, AutomationInterval: time.Second,
				QuoteInterval: time.Second, NoiseInterval: 2 * time.Second, GreekInterval: time.Minute,
				ShortOptionTenor: time.Hour, LongOptionTenor: 24 * time.Hour,
				ShortFutureTenor: 2 * time.Hour, LongFutureTenor: 30 * time.Hour,
				DealerHedgeMode: "off", AutoBorrowSpot: &noBorrow,
				StrictPopulationAccounting: true,
				CrossVenueBaseLatency:      2 * time.Second, CrossVenueArbLotQty: 40_000_000,
				CrossVenueArbInitialBase:  2 * exchange.BTC_PRECISION,
				CrossVenueArbInitialQuote: 150_000 * exchange.USD_PRECISION,
			}
			if arm == "ON" {
				config.CrossVenueArbTiers = []float64{1}
				config.CrossVenueArbMaxAttempts = 1
				config.RecordMarketDataReceipts = true
				config.MarketDataReceiptRoles = []string{"cross_venue_router_tier"}
				config.RecordDecisionFrontierVectors = true
				config.RecordCrossVenueArbEvaluations = true
			}
			sim, err := multivenue.NewSim(10*time.Second, config)
			if err != nil {
				t.Fatal(err)
			}
			if err := sim.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			reports := make([]multivenue.CrossVenueArbReport, 0, len(sim.Routers))
			for _, router := range sim.Routers {
				reports = append(reports, router.Report())
			}
			ledgers := sim.CaptureVenueLedgers()
			if err := sim.Close(); err != nil {
				t.Fatal(err)
			}
			terminalReport, err := json.Marshal(map[string]any{
				"router_reports": reports, "initial_accounts": sim.InitialAccounts,
				"terminal_accounts": sim.TerminalAccounts, "venue_ledgers": ledgers,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(rawDir, "greeks.json"), terminalReport, 0o644); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(rawDir, "manifest.json")
			manifestRaw, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]json.RawMessage
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatal(err)
			}
			var build map[string]any
			if err := json.Unmarshal(manifest["build"], &build); err != nil {
				t.Fatal(err)
			}
			// Synthetic fixtures may replace Go test's "unknown" build stamp;
			// production run manifests are never modified by this analyzer.
			build["revision"], build["modified"] = strings.Repeat("a", 40), false
			manifest["build"], err = json.Marshal(build)
			if err != nil {
				t.Fatal(err)
			}
			manifestRaw, err = json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, manifestRaw, 0o644); err != nil {
				t.Fatal(err)
			}
			var compactConfig bytes.Buffer
			if err := json.Compact(&compactConfig, manifest["config"]); err != nil {
				t.Fatal(err)
			}
			configHash := sha256.Sum256(compactConfig.Bytes())
			contract := Contract{
				SchemaVersion: 1, Arm: arm, Seed: 42, BackgroundIdentity: "synthetic-ten-second-background",
				SourceRevision: strings.Repeat("a", 40), EffectiveConfigSHA256: fmt.Sprintf("%x", configHash[:]),
				Venues: [2]string{"north", "south"}, Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD",
				HorizonNano: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(10 * time.Second).UnixNano(),
				LotQty:      sim.Config.CrossVenueArbLotQty, BasePrecision: exchange.BTC_PRECISION,
				TakerFeeBps: sim.Config.TakerFeeBps, MaxBookEvidenceAgeNanos: int64(10 * time.Second),
			}
			if arm == "ON" {
				contract.InitialBasePerVenue = config.CrossVenueArbInitialBase
				contract.InitialQuotePerVenue = config.CrossVenueArbInitialQuote
			}
			result, err := AnalyzeCompletedWorld(rawDir, filepath.Join(t.TempDir(), "rendered"), contract)
			if err != nil {
				t.Fatal(err)
			}
			if result.Arm != arm || result.Binding.EventFrames == 0 || result.Router == nil && arm == "ON" ||
				result.Router != nil && arm == "OFF" {
				t.Fatalf("synthetic arm reconstruction = %#v", result)
			}
			if result.Conservation == nil {
				t.Fatal("synthetic arm omitted run-wide conservation")
			}
			if _, err := json.Marshal(result); err != nil {
				t.Fatalf("synthetic machine result is not serializable: %v", err)
			}
		})
	}
}
