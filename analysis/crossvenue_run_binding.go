package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
)

type CrossVenueRunBindingExpectation struct {
	SourceRevision        string
	EffectiveConfigSHA256 string
	ExecutionStreamHash   string
	RouterEnabled         bool
	Seed                  int64
	Venues                [2]string
	LotQty                int64
	MaxAttempts           int
	TakerFeeBps           int64
}

type CrossVenueRunBinding struct {
	ManifestSHA256        string
	ReportSHA256          string
	EffectiveConfigSHA256 string
	ExecutionStreamHash   string
	EventFrames           uint64
}

type crossVenueBindingManifest struct {
	SchemaVersion int             `json:"schema_version"`
	VenueIDs      []string        `json:"venue_ids"`
	Config        json.RawMessage `json:"config"`
	Build         struct {
		Revision string `json:"revision"`
		Modified bool   `json:"modified"`
	} `json:"build"`
}

type crossVenueBindingConfig struct {
	LogMode                        string    `json:"log_mode"`
	EvidenceFormat                 string    `json:"evidence_format"`
	EvidenceContractVersion        int       `json:"evidence_contract_version"`
	RecordMarketDataReceipts       bool      `json:"record_market_data_receipts"`
	MarketDataReceiptRoles         []string  `json:"market_data_receipt_roles"`
	RecordDecisionFrontierVectors  bool      `json:"record_decision_frontier_vectors"`
	RecordCrossVenueArbEvaluations bool      `json:"record_cross_venue_arb_evaluations"`
	StrictPopulationAccounting     bool      `json:"strict_population_accounting"`
	VenueIDs                       []string  `json:"venue_ids"`
	CrossVenueArbTiers             []float64 `json:"cross_venue_arb_tiers"`
	CrossVenueArbLotQty            int64     `json:"cross_venue_arb_lot_qty"`
	CrossVenueArbMaxAttempts       int       `json:"cross_venue_arb_max_attempts"`
	CrossVenueArbInitialBase       int64     `json:"cross_venue_arb_initial_base"`
	CrossVenueArbInitialQuote      int64     `json:"cross_venue_arb_initial_quote"`
	AutoBorrowSpot                 *bool     `json:"auto_borrow_spot"`
	TakerFeeBps                    int64     `json:"taker_fee_bps"`
	Seed                           int64     `json:"seed"`
}

type crossVenueBinaryAttestation struct {
	Domain               string `json:"domain"`
	Ordering             string `json:"ordering"`
	EventFrames          uint64 `json:"event_frames"`
	ExecutionStreamHash  string `json:"execution_stream_hash"`
	EvidenceOnlyInStream bool   `json:"evidence_only_in_stream"`
}

type crossVenueRenderedAttestation struct {
	Domain                 string `json:"domain"`
	Ordering               string `json:"ordering"`
	SourceExecutionHash    string `json:"source_execution_stream_hash"`
	SourceEventFrames      uint64 `json:"source_event_frames"`
	GlobalSequenceIncluded bool   `json:"global_sequence_included"`
}

// VerifyCrossVenueRunBinding ties a completed raw run to one verified render
// and one preregistered effective config/source. ExecutionStreamHash must be
// the hash returned by a successful complete binary render in this workflow;
// this check does not itself decode the large stream a second time.
func VerifyCrossVenueRunBinding(rawDir, renderedDir string, expected CrossVenueRunBindingExpectation) (CrossVenueRunBinding, error) {
	if rawDir == "" || renderedDir == "" || !crossVenueSourceRevision(expected.SourceRevision) || !crossVenueHexHash(expected.EffectiveConfigSHA256) ||
		!crossVenueHexHash(expected.ExecutionStreamHash) || expected.Venues[0] == "" || expected.Venues[1] == "" ||
		expected.Venues[0] == expected.Venues[1] || expected.Seed == 0 || expected.LotQty <= 0 || expected.TakerFeeBps < 0 ||
		expected.RouterEnabled && expected.MaxAttempts != 1 || !expected.RouterEnabled && expected.MaxAttempts != 0 {
		return CrossVenueRunBinding{}, fmt.Errorf("cross-venue run binding: invalid expected contract")
	}
	manifestRaw, configHash, err := crossVenueVerifyManifest(rawDir, expected)
	if err != nil {
		return CrossVenueRunBinding{}, err
	}
	rawReport, err := crossVenueVerifyReportCopy(rawDir, renderedDir)
	if err != nil {
		return CrossVenueRunBinding{}, err
	}
	eventFrames, err := crossVenueVerifyAttestations(rawDir, renderedDir, expected.ExecutionStreamHash)
	if err != nil {
		return CrossVenueRunBinding{}, err
	}
	manifestHash, reportHash := sha256.Sum256(manifestRaw), sha256.Sum256(rawReport)
	return CrossVenueRunBinding{
		ManifestSHA256: hex.EncodeToString(manifestHash[:]), ReportSHA256: hex.EncodeToString(reportHash[:]),
		EffectiveConfigSHA256: configHash, ExecutionStreamHash: expected.ExecutionStreamHash,
		EventFrames: eventFrames,
	}, nil
}

func crossVenueVerifyManifest(rawDir string, expected CrossVenueRunBindingExpectation) ([]byte, string, error) {
	manifestRaw, err := readSV1DRegularFile(filepath.Join(rawDir, "manifest.json"))
	if err != nil {
		return nil, "", fmt.Errorf("cross-venue run binding: manifest: %w", err)
	}
	var manifest crossVenueBindingManifest
	if err := decodeRequiredJSON(manifestRaw, &manifest, "schema_version", "venue_ids", "config", "build"); err != nil {
		return nil, "", fmt.Errorf("cross-venue run binding: malformed manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 || manifest.Build.Modified || manifest.Build.Revision != expected.SourceRevision ||
		!slices.Equal(manifest.VenueIDs, expected.Venues[:]) {
		return nil, "", fmt.Errorf("cross-venue run binding: source or venue identity differs from preregistration")
	}
	compactConfig := &bytes.Buffer{}
	if err := json.Compact(compactConfig, manifest.Config); err != nil {
		return nil, "", fmt.Errorf("cross-venue run binding: malformed effective config: %w", err)
	}
	configHash := sha256.Sum256(compactConfig.Bytes())
	configHashHex := hex.EncodeToString(configHash[:])
	if configHashHex != expected.EffectiveConfigSHA256 {
		return nil, "", fmt.Errorf("cross-venue run binding: effective config differs from preregistration")
	}
	if err := crossVenueVerifyEffectiveFields(manifest.Config, expected); err != nil {
		return nil, "", err
	}
	return manifestRaw, configHashHex, nil
}

func crossVenueVerifyEffectiveFields(raw json.RawMessage, expected CrossVenueRunBindingExpectation) error {
	var config crossVenueBindingConfig
	required := []string{
		"log_mode", "evidence_format", "evidence_contract_version", "record_market_data_receipts",
		"record_decision_frontier_vectors", "venue_ids",
		"strict_population_accounting",
		"cross_venue_arb_max_attempts", "auto_borrow_spot", "taker_fee_bps", "seed",
	}
	if expected.RouterEnabled {
		required = append(required, "record_cross_venue_arb_evaluations", "cross_venue_arb_tiers", "cross_venue_arb_lot_qty", "cross_venue_arb_initial_base", "cross_venue_arb_initial_quote")
	}
	if err := decodeRequiredJSON(raw, &config, required...); err != nil {
		return fmt.Errorf("cross-venue run binding: malformed effective fields: %w", err)
	}
	if config.LogMode != "full" || config.EvidenceFormat != "evstream_v3" || config.EvidenceContractVersion != 2 ||
		!config.StrictPopulationAccounting || !slices.Equal(config.VenueIDs, expected.Venues[:]) || config.AutoBorrowSpot == nil || *config.AutoBorrowSpot ||
		config.TakerFeeBps != expected.TakerFeeBps || config.Seed != expected.Seed {
		return fmt.Errorf("cross-venue run binding: effective configuration violates first-attempt evidence contract")
	}
	if expected.RouterEnabled {
		if !config.RecordMarketDataReceipts || !config.RecordDecisionFrontierVectors || !config.RecordCrossVenueArbEvaluations ||
			!slices.Contains(config.MarketDataReceiptRoles, "cross_venue_router_tier") ||
			len(config.CrossVenueArbTiers) != 1 || config.CrossVenueArbTiers[0] <= 0 ||
			config.CrossVenueArbLotQty != expected.LotQty || config.CrossVenueArbMaxAttempts != 1 ||
			config.CrossVenueArbInitialBase <= 0 || config.CrossVenueArbInitialQuote <= 0 {
			return fmt.Errorf("cross-venue run binding: router-on evidence or funding contract differs")
		}
	} else if len(config.CrossVenueArbTiers) != 0 || config.RecordCrossVenueArbEvaluations ||
		config.CrossVenueArbMaxAttempts != 0 {
		return fmt.Errorf("cross-venue run binding: router-off arm contains router activation")
	}
	return nil
}

func crossVenueVerifyReportCopy(rawDir, renderedDir string) ([]byte, error) {
	rawReport, err := readSV1DRegularFile(filepath.Join(rawDir, "greeks.json"))
	if err != nil {
		return nil, fmt.Errorf("cross-venue run binding: raw report: %w", err)
	}
	renderedReport, err := readSV1DRegularFile(filepath.Join(renderedDir, "greeks.json"))
	if err != nil {
		return nil, fmt.Errorf("cross-venue run binding: rendered report: %w", err)
	}
	if !bytes.Equal(rawReport, renderedReport) {
		return nil, fmt.Errorf("cross-venue run binding: rendered report is not the raw terminal report")
	}
	return rawReport, nil
}

func crossVenueVerifyAttestations(rawDir, renderedDir, expectedExecutionHash string) (uint64, error) {
	var binary crossVenueBinaryAttestation
	if err := crossVenueReadRequiredJSON(filepath.Join(rawDir, "binary-evidence-attestation.json"), &binary); err != nil {
		return 0, err
	}
	var rendered crossVenueRenderedAttestation
	if err := crossVenueReadRequiredJSON(filepath.Join(renderedDir, "rendered-binary-evidence-attestation.json"), &rendered); err != nil {
		return 0, err
	}
	if binary.Domain != "canonical_binary_execution_frames" || binary.Ordering != "ordered_stream" ||
		!binary.EvidenceOnlyInStream || binary.EventFrames == 0 || rendered.Domain != "rendered_binary_evidence" ||
		rendered.Ordering != "venue_sequence_files_with_global_frame_identity" || !rendered.GlobalSequenceIncluded ||
		rendered.SourceEventFrames != binary.EventFrames || binary.ExecutionStreamHash != expectedExecutionHash ||
		rendered.SourceExecutionHash != expectedExecutionHash {
		return 0, fmt.Errorf("cross-venue run binding: binary and rendered attestations disagree")
	}
	return binary.EventFrames, nil
}

func crossVenueReadRequiredJSON(path string, target any) error {
	raw, err := readSV1DRegularFile(path)
	if err != nil {
		return fmt.Errorf("cross-venue run binding: %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("cross-venue run binding: %s: %w", filepath.Base(path), err)
	}
	return nil
}

func crossVenueHexHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func crossVenueSourceRevision(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
