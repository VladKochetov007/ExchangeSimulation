package crossvenue

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

var routerTreatmentConfigFields = map[string]bool{
	"cross_venue_arb_tiers":              true,
	"cross_venue_arb_lot_qty":            true,
	"cross_venue_arb_max_attempts":       true,
	"cross_venue_arb_initial_base":       true,
	"cross_venue_arb_initial_quote":      true,
	"record_market_data_receipts":        true,
	"market_data_receipt_roles":          true,
	"record_decision_frontier_vectors":   true,
	"record_cross_venue_arb_evaluations": true,
}

// backgroundIdentityFromManifest compares every effective configuration field
// except the registered router treatment and its evidence-only switches.
// It does not assert equal realized RNG streams or equal total system capital.
func backgroundIdentityFromManifest(path, expectedManifestSHA256 string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("ME-005 background: read bound manifest: %w", err)
	}
	manifestHash := sha256.Sum256(raw)
	if hex.EncodeToString(manifestHash[:]) != expectedManifestSHA256 {
		return "", fmt.Errorf("ME-005 background: manifest changed after binding")
	}
	var manifest struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil || len(manifest.Config) == 0 {
		return "", fmt.Errorf("ME-005 background: missing effective config: %v", err)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(manifest.Config, &config); err != nil || len(config) == 0 {
		return "", fmt.Errorf("ME-005 background: malformed effective config: %v", err)
	}
	for field := range routerTreatmentConfigFields {
		delete(config, field)
	}
	for field, value := range config {
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return "", fmt.Errorf("ME-005 background: malformed %s: %w", field, err)
		}
		config[field] = compact.Bytes()
	}
	canonical, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("ME-005 background: encode effective config: %w", err)
	}
	digest := sha256.Sum256(append([]byte("me005-background-config-v1\n"), canonical...))
	return hex.EncodeToString(digest[:]), nil
}
