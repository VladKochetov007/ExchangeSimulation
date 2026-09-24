package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func crossVenueBindingFixture(t *testing.T) (string, string, CrossVenueRunBindingExpectation) {
	t.Helper()
	rawDir, renderedDir := filepath.Join(t.TempDir(), "raw"), filepath.Join(t.TempDir(), "rendered")
	for _, directory := range []string{rawDir, renderedDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := map[string]any{
		"log_mode": "full", "evidence_format": "evstream_v3", "evidence_contract_version": 2,
		"record_market_data_receipts": true, "market_data_receipt_roles": []string{"cross_venue_router_tier"},
		"record_decision_frontier_vectors": true, "record_cross_venue_arb_evaluations": true,
		"venue_ids": []string{"north", "south"}, "cross_venue_arb_tiers": []float64{1},
		"cross_venue_arb_lot_qty": 5, "cross_venue_arb_max_attempts": 1,
		"cross_venue_arb_initial_base": 10, "cross_venue_arb_initial_quote": 1000,
		"auto_borrow_spot": false, "taker_fee_bps": 10,
	}
	configRaw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configHash := sha256.Sum256(configRaw)
	expected := CrossVenueRunBindingExpectation{
		SourceRevision: strings.Repeat("a", 40), EffectiveConfigSHA256: hex.EncodeToString(configHash[:]),
		ExecutionStreamHash: strings.Repeat("b", 64), RouterEnabled: true, Venues: [2]string{"north", "south"},
		LotQty: 5, MaxAttempts: 1, TakerFeeBps: 10,
	}
	manifest := map[string]any{
		"schema_version": 2, "venue_ids": []string{"north", "south"}, "config": config,
		"build": map[string]any{"revision": expected.SourceRevision, "modified": false},
	}
	crossVenueWriteBindingJSON(t, filepath.Join(rawDir, "manifest.json"), manifest)
	for _, directory := range []string{rawDir, renderedDir} {
		crossVenueWriteBindingJSON(t, filepath.Join(directory, "greeks.json"), map[string]any{"router_reports": []any{}})
	}
	crossVenueWriteBindingJSON(t, filepath.Join(rawDir, "binary-evidence-attestation.json"), map[string]any{
		"domain": "canonical_binary_execution_frames", "ordering": "ordered_stream",
		"event_frames": 3, "execution_stream_hash": expected.ExecutionStreamHash,
		"evidence_only_in_stream": true,
	})
	crossVenueWriteBindingJSON(t, filepath.Join(renderedDir, "rendered-binary-evidence-attestation.json"), map[string]any{
		"domain": "rendered_binary_evidence", "ordering": "venue_sequence_files_with_global_frame_identity",
		"source_event_frames": 3, "source_execution_stream_hash": expected.ExecutionStreamHash,
		"global_sequence_included": true,
	})
	return rawDir, renderedDir, expected
}

func crossVenueWriteBindingJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCrossVenueRunBindingRequiresExactConfigSourceReportAndStream(t *testing.T) {
	rawDir, renderedDir, expected := crossVenueBindingFixture(t)
	binding, err := VerifyCrossVenueRunBinding(rawDir, renderedDir, expected)
	if err != nil || binding.EventFrames != 3 || binding.ExecutionStreamHash != expected.ExecutionStreamHash ||
		binding.EffectiveConfigSHA256 != expected.EffectiveConfigSHA256 {
		t.Fatalf("valid run binding = %#v, %v", binding, err)
	}
	for _, test := range []struct {
		name   string
		mutate func(string, string, *CrossVenueRunBindingExpectation)
	}{
		{"wrong-source", func(_ string, _ string, expected *CrossVenueRunBindingExpectation) {
			expected.SourceRevision = strings.Repeat("c", 40)
		}},
		{"wrong-config", func(_ string, _ string, expected *CrossVenueRunBindingExpectation) {
			expected.EffectiveConfigSHA256 = strings.Repeat("d", 64)
		}},
		{"borrowing-despite-matching-config-digest", func(rawDir string, _ string, expected *CrossVenueRunBindingExpectation) {
			path := filepath.Join(rawDir, "manifest.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]json.RawMessage
			if err := json.Unmarshal(raw, &manifest); err != nil {
				t.Fatal(err)
			}
			var config map[string]any
			if err := json.Unmarshal(manifest["config"], &config); err != nil {
				t.Fatal(err)
			}
			config["auto_borrow_spot"] = true
			configRaw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			configHash := sha256.Sum256(configRaw)
			expected.EffectiveConfigSHA256 = hex.EncodeToString(configHash[:])
			manifest["config"] = configRaw
			crossVenueWriteBindingJSON(t, path, manifest)
		}},
		{"report-copy", func(_ string, renderedDir string, _ *CrossVenueRunBindingExpectation) {
			if err := os.WriteFile(filepath.Join(renderedDir, "greeks.json"), []byte(`{}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"stream-hash", func(_ string, _ string, expected *CrossVenueRunBindingExpectation) {
			expected.ExecutionStreamHash = strings.Repeat("e", 64)
		}},
		{"source-attestation", func(rawDir string, _ string, _ *CrossVenueRunBindingExpectation) {
			path := filepath.Join(rawDir, "binary-evidence-attestation.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, bytes.Replace(raw, []byte(`"event_frames":3`), []byte(`"event_frames":4`), 1), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			rawDir, renderedDir, expected := crossVenueBindingFixture(t)
			test.mutate(rawDir, renderedDir, &expected)
			if _, err := VerifyCrossVenueRunBinding(rawDir, renderedDir, expected); err == nil {
				t.Fatal("mismatched run identity accepted")
			}
		})
	}
}

func TestCrossVenueRunBindingDistinguishesRouterOffArm(t *testing.T) {
	rawDir, renderedDir, expected := crossVenueBindingFixture(t)
	manifestPath := filepath.Join(rawDir, "manifest.json")
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(manifest["config"], &config); err != nil {
		t.Fatal(err)
	}
	config["cross_venue_arb_tiers"] = nil
	config["cross_venue_arb_max_attempts"] = 0
	config["record_market_data_receipts"] = false
	config["record_decision_frontier_vectors"] = false
	delete(config, "record_cross_venue_arb_evaluations")
	configRaw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configHash := sha256.Sum256(configRaw)
	expected.RouterEnabled, expected.MaxAttempts = false, 0
	expected.EffectiveConfigSHA256 = hex.EncodeToString(configHash[:])
	manifest["config"] = configRaw
	crossVenueWriteBindingJSON(t, manifestPath, manifest)
	if _, err := VerifyCrossVenueRunBinding(rawDir, renderedDir, expected); err != nil {
		t.Fatalf("valid router-off arm was rejected: %v", err)
	}
	expected.RouterEnabled, expected.MaxAttempts = true, 1
	if _, err := VerifyCrossVenueRunBinding(rawDir, renderedDir, expected); err == nil {
		t.Fatal("router-off arm was accepted as router-on")
	}
}
