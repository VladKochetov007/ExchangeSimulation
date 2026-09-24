package crossvenue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestME005BackgroundIdentityExcludesOnlyRegisteredRouterTreatment(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	config := map[string]any{
		"seed": int64(41), "venue_ids": []string{"north", "south"}, "quote_interval": 1_000_000_000,
		"cross_venue_arb_tiers": []float64{}, "cross_venue_arb_max_attempts": 0,
		"record_market_data_receipts": false,
	}
	identity := func() string {
		t.Helper()
		raw, err := json.Marshal(map[string]any{"config": config})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		result, err := backgroundIdentityFromManifest(manifestPath, hex.EncodeToString(digest[:]))
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	off := identity()
	config["cross_venue_arb_tiers"] = []float64{1}
	config["cross_venue_arb_max_attempts"] = 1
	config["record_market_data_receipts"] = true
	config["record_cross_venue_arb_evaluations"] = true
	if on := identity(); on != off {
		t.Fatalf("router-only switch changed background identity: %s vs %s", off, on)
	}
	config["quote_interval"] = 2_000_000_000
	if changed := identity(); changed == off {
		t.Fatal("changed background clock retained identity")
	}
	if _, err := backgroundIdentityFromManifest(manifestPath, off); err == nil {
		t.Fatal("post-binding manifest mutation accepted")
	}
}
