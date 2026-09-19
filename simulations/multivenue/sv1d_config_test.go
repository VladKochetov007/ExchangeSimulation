package multivenue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegisteredSV1DActivationConfigs(t *testing.T) {
	tests := []struct {
		name             string
		wantRoster       int
		wantOneSidedMode bool
		wantDecisionLog  bool
		wantReceiptRoles []string
	}{
		{name: "activation-659-treatment", wantRoster: 4, wantOneSidedMode: true, wantDecisionLog: true, wantReceiptRoles: []string{"cdf_elastic_supplier", "liability_hedger"}},
		{name: "activation-659-mode-off", wantRoster: 4, wantOneSidedMode: false, wantDecisionLog: true, wantReceiptRoles: []string{"cdf_elastic_supplier", "liability_hedger"}},
		{name: "activation-659-no-roster", wantRoster: 0, wantOneSidedMode: false, wantDecisionLog: false, wantReceiptRoles: []string{"liability_hedger"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "research", "configs", "v2-r2-sv1d-activation", test.name+".json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			config, err := DecodeConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			config.LogDir = t.TempDir()
			if err := config.normalize(); err != nil {
				t.Fatalf("normalize registered config: %v", err)
			}
			if config.Seed != 659 || config.EvidenceFormat != binaryRepresentation || config.EvidenceContractVersion != 2 ||
				config.LogMode != "full" || config.R2ExpiryCalendar == nil || len(config.VenueIDs) != 3 ||
				config.RecordMarketDataReceipts != true || !config.StrictPopulationAccounting ||
				!config.StrictRiskContract || config.AutoBorrowSpot == nil || *config.AutoBorrowSpot ||
				!config.CrossAssetSpotGraph || config.CrossAssetCollateralMarks ||
				config.Step != time.Second || config.SnapshotInterval != time.Second ||
				config.AutomationInterval != time.Second || config.QuoteInterval != time.Second ||
				config.NoiseInterval != 2*time.Second || config.GreekInterval != time.Minute ||
				config.CheckpointIntervalSeconds != 60 || config.ElasticSupplierCount != 8 {
				t.Fatalf("common config contract mismatch: seed=%d evidence=%s/%d mode=%s calendar=%v venues=%v", config.Seed, config.EvidenceFormat, config.EvidenceContractVersion, config.LogMode, config.R2ExpiryCalendar != nil, config.VenueIDs)
			}
			if config.VenueRules["north"].MatchingRule != MatchingPriceTime ||
				config.VenueRules["central"].MatchingRule != MatchingProRata ||
				config.VenueRules["south"].MatchingRule != MatchingProRata ||
				config.VenueRules["north"].FundingIntervalSeconds != 8*60*60 ||
				config.VenueRules["central"].FundingIntervalSeconds != 60*60 ||
				config.VenueRules["south"].FundingIntervalSeconds != 2*60*60 {
				t.Fatalf("matching/funding policy mismatch: %+v", config.VenueRules)
			}
			if got := config.R2ExpiryCalendar.Schedules; len(got) != 3 ||
				got[0].Name != "short" || time.Duration(got[0].ListingIntervalNano) != time.Hour || time.Duration(got[0].TimeToExpiryNano) != 2*time.Hour ||
				got[1].Name != "medium" || time.Duration(got[1].ListingIntervalNano) != 3*time.Hour || time.Duration(got[1].TimeToExpiryNano) != 6*time.Hour ||
				got[2].Name != "long" || time.Duration(got[2].ListingIntervalNano) != 6*time.Hour || time.Duration(got[2].TimeToExpiryNano) != 12*time.Hour {
				t.Fatalf("calendar contract mismatch: %+v", got)
			}
			if len(config.ElasticLiquiditySuppliers) != test.wantRoster ||
				config.RecordElasticLiquiditySupplierDecisions != test.wantDecisionLog ||
				!equalStrings(config.MarketDataReceiptRoles, test.wantReceiptRoles) {
				t.Fatalf("roster contract mismatch: roster=%d decisions=%t", len(config.ElasticLiquiditySuppliers), config.RecordElasticLiquiditySupplierDecisions)
			}
			for _, supplier := range config.ElasticLiquiditySuppliers {
				if supplier.QuoteOnOneSidedLocalBook != test.wantOneSidedMode {
					t.Fatalf("supplier %s one-sided mode=%t, want %t", supplier.Role, supplier.QuoteOnOneSidedLocalBook, test.wantOneSidedMode)
				}
			}
		})
	}
}

func TestSV1DManifestRetainsExplicitOneSidedSupplierOption(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "activation-659-treatment", want: true},
		{name: "activation-659-mode-off", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := filepath.Join("..", "..", "research", "configs", "v2-r2-sv1d-activation", test.name+".json")
			raw, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			config, err := DecodeConfig(raw)
			if err != nil {
				t.Fatal(err)
			}
			config.LogDir = t.TempDir()
			sim, err := NewSim(time.Second, config)
			if err != nil {
				t.Fatalf("NewSim: %v", err)
			}
			t.Cleanup(func() {
				if err := sim.Close(); err != nil {
					t.Errorf("close simulation: %v", err)
				}
			})

			manifestRaw, err := os.ReadFile(filepath.Join(config.LogDir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			var manifest struct {
				Config struct {
					ElasticLiquiditySuppliers []map[string]json.RawMessage `json:"elastic_liquidity_suppliers"`
				} `json:"config"`
			}
			if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
				t.Fatalf("decode manifest: %v", err)
			}
			if got := len(manifest.Config.ElasticLiquiditySuppliers); got != len(config.ElasticLiquiditySuppliers) {
				t.Fatalf("manifest supplier count = %d, want %d", got, len(config.ElasticLiquiditySuppliers))
			}
			for index, supplier := range manifest.Config.ElasticLiquiditySuppliers {
				encoded, present := supplier["quote_on_one_sided_local_book"]
				if !present {
					t.Fatalf("supplier %d omitted explicit one-sided option", index)
				}
				var got bool
				if err := json.Unmarshal(encoded, &got); err != nil {
					t.Fatalf("decode supplier %d one-sided option: %v", index, err)
				}
				if got != test.want {
					t.Fatalf("supplier %d one-sided option = %t, want %t", index, got, test.want)
				}
			}
		})
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
