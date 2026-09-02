package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	etypes "exchange_sim/types"
)

// CDFLiquidityRunAudit is an evidence audit for the V2-R2-SV1 participant
// class. It combines the evidence-only decision/fill stream with the rendered
// CDF/USD execution book and independent account snapshots. Valid means the
// intervention can be reconstructed without guessing; it is not a survival
// score.
type CDFLiquidityRunAudit struct {
	Provenance                                  *CDFLiquidityRunProvenance  `json:"provenance,omitempty"`
	SupplierCount                               int                         `json:"supplier_count"`
	DecisionCount                               int64                       `json:"decision_count"`
	FillCount                                   int64                       `json:"fill_count"`
	SupplierVolumeQty                           int64                       `json:"supplier_volume_qty"`
	TotalTradeCount                             int64                       `json:"total_trade_count"`
	TotalTradeVolumeQty                         int64                       `json:"total_trade_volume_qty"`
	SupplierVolumeShare                         float64                     `json:"supplier_volume_share"`
	SnapshotCount                               int64                       `json:"snapshot_count"`
	BidAbsentSnapshots                          int64                       `json:"bid_absent_snapshots"`
	AskAbsentSnapshots                          int64                       `json:"ask_absent_snapshots"`
	BothAbsentSnapshots                         int64                       `json:"both_absent_snapshots"`
	QualifiedBidAbsentSnapshots                 int64                       `json:"qualified_bid_absent_snapshots"`
	QualifiedAskAbsentSnapshots                 int64                       `json:"qualified_ask_absent_snapshots"`
	QualifiedBothAbsentSnapshots                int64                       `json:"qualified_both_absent_snapshots"`
	BidAbsenceFraction                          float64                     `json:"bid_absence_fraction"`
	AskAbsenceFraction                          float64                     `json:"ask_absence_fraction"`
	QualifiedBidAbsenceFraction                 float64                     `json:"qualified_bid_absence_fraction"`
	QualifiedAskAbsenceFraction                 float64                     `json:"qualified_ask_absence_fraction"`
	SupplierRemovalSnapshotCount                int64                       `json:"supplier_removal_snapshot_count"`
	SupplierRemovalBidAbsentSnapshots           int64                       `json:"supplier_removal_bid_absent_snapshots"`
	SupplierRemovalAskAbsentSnapshots           int64                       `json:"supplier_removal_ask_absent_snapshots"`
	SupplierRemovalBothAbsentSnapshots          int64                       `json:"supplier_removal_both_absent_snapshots"`
	SupplierRemovalQualifiedBidAbsentSnapshots  int64                       `json:"supplier_removal_qualified_bid_absent_snapshots"`
	SupplierRemovalQualifiedAskAbsentSnapshots  int64                       `json:"supplier_removal_qualified_ask_absent_snapshots"`
	SupplierRemovalQualifiedBothAbsentSnapshots int64                       `json:"supplier_removal_qualified_both_absent_snapshots"`
	SupplierRemovalOneSidedSnapshots            int64                       `json:"supplier_removal_one_sided_snapshots"`
	SupplierRemovalInvalidSnapshots             int64                       `json:"supplier_removal_invalid_snapshots"`
	SupplierRemovalBidAbsenceFraction           float64                     `json:"supplier_removal_bid_absence_fraction"`
	SupplierRemovalAskAbsenceFraction           float64                     `json:"supplier_removal_ask_absence_fraction"`
	SupplierRemovalQualifiedBidAbsenceFraction  float64                     `json:"supplier_removal_qualified_bid_absence_fraction"`
	SupplierRemovalQualifiedAskAbsenceFraction  float64                     `json:"supplier_removal_qualified_ask_absence_fraction"`
	SupplierRemovalCounterfactualValid          bool                        `json:"supplier_removal_counterfactual_valid"`
	MinimumExecutableQty                        int64                       `json:"minimum_executable_qty"`
	SupplierInitialEquity                       int64                       `json:"supplier_initial_equity"`
	SupplierTerminalEquity                      int64                       `json:"supplier_terminal_equity"`
	SupplierPnL                                 int64                       `json:"supplier_pnl"`
	AcceptedQuoteCount                          int64                       `json:"accepted_quote_count"`
	CompletedQuoteCount                         int64                       `json:"completed_quote_count"`
	CensoredQuoteCount                          int64                       `json:"censored_quote_count"`
	LiveAcceptedQuoteCount                      int64                       `json:"live_accepted_quote_count"`
	PendingSubmissionCount                      int64                       `json:"pending_submission_count"`
	CancelPendingQuoteCount                     int64                       `json:"cancel_pending_quote_count"`
	MeanQuoteLifetimeNs                         float64                     `json:"mean_quote_lifetime_ns"`
	MaxQuoteLifetimeNs                          int64                       `json:"max_quote_lifetime_ns"`
	MeanObservedTouchShare                      float64                     `json:"mean_observed_touch_share"`
	MaxObservedTouchShare                       float64                     `json:"max_observed_touch_share"`
	SubmitCount                                 int64                       `json:"submit_count"`
	RestCount                                   int64                       `json:"rest_count"`
	CancelCount                                 int64                       `json:"cancel_count"`
	WithdrawCount                               int64                       `json:"withdraw_count"`
	WithdrawalWithoutReplacementCount           int64                       `json:"withdrawal_without_replacement_count"`
	CensoredWithdrawalCount                     int64                       `json:"censored_withdrawal_count"`
	TradingSupplierCount                        int64                       `json:"trading_supplier_count"`
	PnLChangingSupplierCount                    int64                       `json:"pnl_changing_supplier_count"`
	InventoryResponsiveDecisionCount            int64                       `json:"inventory_responsive_decision_count"`
	RiskStateDecisionCount                      int64                       `json:"risk_state_decision_count"`
	RiskLimitTriggeredDecisionCount             int64                       `json:"risk_limit_triggered_decision_count"`
	MaxObservedLossFromInitialQuote             int64                       `json:"max_observed_loss_from_initial_quote"`
	MaxObservedDrawdownQuote                    int64                       `json:"max_observed_drawdown_quote"`
	RealizedPnL                                 int64                       `json:"realized_pnl"`
	UnrealizedPnL                               int64                       `json:"unrealized_pnl"`
	EndowmentRevaluationPnL                     int64                       `json:"endowment_revaluation_pnl"`
	TradingPnL                                  int64                       `json:"trading_pnl"`
	TradingPnLReconciliationResidual            int64                       `json:"trading_pnl_reconciliation_residual"`
	BalanceSnapshotCount                        int64                       `json:"balance_snapshot_count"`
	BalanceReconciliationResidual               int64                       `json:"balance_reconciliation_residual"`
	PnLReconciliationResidual                   int64                       `json:"pnl_reconciliation_residual"`
	MaxBorrowed                                 int64                       `json:"max_borrowed"`
	HistoricalSupplierCount                     int                         `json:"historical_supplier_count"`
	ExpectedHistoricalCount                     int                         `json:"expected_historical_count"`
	SupplierDepthOver75Share                    float64                     `json:"supplier_depth_over_75_share"`
	MaxSupplierDepthShare                       float64                     `json:"max_supplier_depth_share"`
	SupplierTimeWeightedRestingDepthShare       float64                     `json:"supplier_time_weighted_resting_depth_share"`
	Venues                                      []CDFLiquidityVenueAudit    `json:"venues"`
	Suppliers                                   []CDFLiquiditySupplierAudit `json:"suppliers"`
	Checks                                      []CDFLiquidityCheck         `json:"checks,omitempty"`
	Valid                                       bool                        `json:"valid"`

	expectedHistoricalCountPerVenue       int
	lastDepthSnapshotAt                   map[string]int64
	lastDepthTotal                        map[string]int64
	lastSupplierDepthByClient             map[string]map[uint64]int64
	supplierRestingDepthWeightedNumerator float64
	totalRestingDepthWeightedDenominator  float64
	lastEventAt                           int64
	terminalAt                            int64
	cancelRequestedByOrder                map[cdfOrderKey]struct{}
	cancelRequestByOrder                  map[cdfOrderKey]uint64
	cancelDecisionByOrder                 map[cdfOrderKey]cdfCancelRequestState
	quoteRequests                         map[cdfRequestKey]cdfQuoteRequestState
	expectedActionKeys                    map[cdfActionKey]struct{}
	pendingOrderWaits                     []cdfPendingOrderWait
	pendingCancelWaits                    []cdfPendingCancelWait
	staleWithdrawals                      map[cdfOrderKey]cdfStaleWithdrawal
	supplierActions                       []cdfSupplierAction
	restDecisions                         []cdfRestDecision
}

// CDFLiquiditySupplierAudit is the per-participant diagnostic vector required
// by the preregistration. Account equity is the PnL source; position and
// turnover are reconstructed from local evidence.
type CDFLiquiditySupplierAudit struct {
	VenueID                           string  `json:"venue_id"`
	Role                              string  `json:"role"`
	ClientID                          uint64  `json:"client_id"`
	DecisionCount                     int64   `json:"decision_count"`
	FillCount                         int64   `json:"fill_count"`
	FilledQty                         int64   `json:"filled_qty"`
	BuyQty                            int64   `json:"buy_qty"`
	SellQty                           int64   `json:"sell_qty"`
	InitialEquity                     int64   `json:"initial_equity"`
	TerminalEquity                    int64   `json:"terminal_equity"`
	PnL                               int64   `json:"pnl"`
	EndowmentRevaluationPnL           int64   `json:"endowment_revaluation_pnl"`
	TradingPnL                        int64   `json:"trading_pnl"`
	TradingPnLReconciliationResidual  int64   `json:"trading_pnl_reconciliation_residual"`
	MinPosition                       int64   `json:"min_position"`
	MaxPosition                       int64   `json:"max_position"`
	TerminalPosition                  int64   `json:"terminal_position"`
	InventoryLimit                    int64   `json:"inventory_limit"`
	AcceptedQuoteCount                int64   `json:"accepted_quote_count"`
	CompletedQuoteCount               int64   `json:"completed_quote_count"`
	CensoredQuoteCount                int64   `json:"censored_quote_count"`
	LiveAcceptedQuoteCount            int64   `json:"live_accepted_quote_count"`
	PendingSubmissionCount            int64   `json:"pending_submission_count"`
	CancelPendingQuoteCount           int64   `json:"cancel_pending_quote_count"`
	WithdrawCount                     int64   `json:"withdraw_count"`
	CancelCount                       int64   `json:"cancel_count"`
	RestCount                         int64   `json:"rest_count"`
	SubmitCount                       int64   `json:"submit_count"`
	WithdrawalWithoutReplacementCount int64   `json:"withdrawal_without_replacement_count"`
	CensoredWithdrawalCount           int64   `json:"censored_withdrawal_count"`
	InventoryResponsiveDecisionCount  int64   `json:"inventory_responsive_decision_count"`
	RiskStateDecisionCount            int64   `json:"risk_state_decision_count"`
	RiskLimitTriggeredDecisionCount   int64   `json:"risk_limit_triggered_decision_count"`
	MaxObservedLossFromInitialQuote   int64   `json:"max_observed_loss_from_initial_quote"`
	MaxObservedDrawdownQuote          int64   `json:"max_observed_drawdown_quote"`
	MeanQuoteLifetimeNs               float64 `json:"mean_quote_lifetime_ns"`
	MaxQuoteLifetimeNs                int64   `json:"max_quote_lifetime_ns"`
	MeanObservedTouchShare            float64 `json:"mean_observed_touch_share"`
	MaxObservedTouchShare             float64 `json:"max_observed_touch_share"`
	MeanObservationAgeNs              float64 `json:"mean_observation_age_ns"`
	MaxObservationAgeNs               int64   `json:"max_observation_age_ns"`
	ConfiguredMaxPosition             int64   `json:"configured_max_position"`
	ConfiguredMaxInventory            int64   `json:"configured_max_inventory"`
	ConfiguredMaxQuoteQty             int64   `json:"configured_max_quote_qty"`
	ConfiguredMinimumExecutableQty    int64   `json:"configured_minimum_executable_qty"`
	ConfiguredIntervalNs              int64   `json:"configured_interval_ns"`
	ConfiguredMaxLossQuote            int64   `json:"configured_max_loss_quote"`
	ConfiguredMakerFeeBps             int64   `json:"configured_maker_fee_bps"`
	ConfiguredReferencePrice          int64   `json:"configured_reference_price"`
	ConfiguredReferenceHalfLife       int64   `json:"configured_reference_half_life"`
	ConfiguredBaseHolding             int64   `json:"configured_base_holding"`
	ConfiguredElasticityPerPercent    int64   `json:"configured_elasticity_per_percent"`
	SupplierVolumeShare               float64 `json:"supplier_volume_share"`
	// This is liquidity-conditioned concentration: the supplier-depth integral
	// divided by total displayed-depth integral over non-empty intervals. Empty
	// intervals are represented by the separate absence counters.
	TimeWeightedRestingDepthShare float64 `json:"time_weighted_resting_depth_share"`
	MaxQuoteQty                   int64   `json:"max_quote_qty"`
	MaxBorrowed                   int64   `json:"max_borrowed"`
	BorrowEventCount              int64   `json:"borrow_event_count"`
	MaxGrossBaseBalance           int64   `json:"max_gross_base_balance"`
	MaxGrossQuoteBalance          int64   `json:"max_gross_quote_balance"`
	RealizedPnL                   int64   `json:"realized_pnl"`
	UnrealizedPnL                 int64   `json:"unrealized_pnl"`
	BalanceSnapshotCount          int64   `json:"balance_snapshot_count"`
	BalanceReconciliationResidual int64   `json:"balance_reconciliation_residual"`
	PnLReconciliationResidual     int64   `json:"pnl_reconciliation_residual"`
	Valid                         bool    `json:"valid"`

	positionSet                     bool
	lastPosition                    int64
	observationAgeTotal             int64
	observationCount                int64
	touchShareTotal                 float64
	touchShareCount                 int64
	quoteLifetimeTotal              int64
	quoteLifetimeCount              int64
	restingDepthWeightedNumerator   float64
	restingDepthWeightedDenominator float64
	pendingTouchByRequest           map[uint64]float64
	configuredBaseAsset             string
	configuredQuoteAsset            string
	configuredSymbol                string
	configuredMaxQuoteQty           int64
	configuredMinimumExecutableQty  int64
	configuredIntervalNs            int64
	configuredMaxLossQuote          int64
	configuredMakerFeeBps           int64
	configuredMaxPosition           int64
	configuredMaxInventory          int64
	configuredBasePrecision         int64
	configuredReferencePrice        int64
	configuredReferenceHalfLife     int64
	configuredBaseHolding           int64
	configuredElasticityPerPercent  int64
	configuredMaxObservationAge     int64
	configuredDecisionPhaseOffset   int64
	configuredInitialBaseBalance    int64
	configuredInitialQuoteBalance   int64
	configuredQuotePrecision        int64
	lastRiskInitialEquity           int64
	lastRiskEquity                  int64
	lastRiskPeakEquity              int64
	lastRiskLossFromInitial         int64
	lastRiskDrawdown                int64
	lastRiskMarkPrice               int64
	riskStateSeen                   bool
	riskLimitSeen                   bool
	endowmentRevaluationPnL         int64
	tradingPnL                      int64
	entryPrice                      int64
	realizedPnL                     int64
	terminalMark                    int64
	initialAccountSeen              bool
	terminalAccountSeen             bool
	maxBorrowed                     int64
	borrowEventCount                int64
	maxGrossBaseBalance             int64
	maxGrossQuoteBalance            int64
	maxQuoteQty                     int64
	balanceSnapshotCount            int64
	lastBalanceSnapshotAt           int64
	initialSpotNetBalances          map[string]int64
	terminalSpotNetBalances         map[string]int64
	fillNetBalanceDeltas            map[string]int64
	initialMarks                    map[string]int64
	terminalMarks                   map[string]int64
	pendingQuoteByRequest           map[uint64]cdfDecisionEvidence
	lastDecisionAt                  int64
	lastDecisionOrdinal             int64
	lastActionablePosition          int64
	hasActionableDecision           bool
	inventoryChangedSinceActionable bool
	reconstructedReference          int64
	referenceLastValidMarkAt        int64
	referenceInitialized            bool
	referenceMarkSeen               bool
	receiptRequired                 bool
}

// CDFLiquidityVenueAudit separates concentration and side-availability
// diagnostics by venue. Point-in-time concentration is measured over periodic
// public snapshots; time-weighted supplier depth uses the observed book state
// on each left-continuous interval through terminal time.
type CDFLiquidityVenueAudit struct {
	VenueID                                     string  `json:"venue_id"`
	HistoricalSupplierCount                     int     `json:"historical_supplier_count"`
	ExpectedHistoricalCount                     int     `json:"expected_historical_count"`
	SupplierVolumeQty                           int64   `json:"supplier_volume_qty"`
	TotalTradeVolumeQty                         int64   `json:"total_trade_volume_qty"`
	SupplierVolumeShare                         float64 `json:"supplier_volume_share"`
	SnapshotCount                               int64   `json:"snapshot_count"`
	ActiveDepthSnapshotCount                    int64   `json:"active_depth_snapshot_count"`
	SupplierDepthOver75Count                    int64   `json:"supplier_depth_over_75_count"`
	SupplierDepthOver75Fraction                 float64 `json:"supplier_depth_over_75_fraction"`
	MaxSupplierDepthShare                       float64 `json:"max_supplier_depth_share"`
	SupplierTimeWeightedRestingDepthShare       float64 `json:"supplier_time_weighted_resting_depth_share"`
	BidAbsentSnapshots                          int64   `json:"bid_absent_snapshots"`
	AskAbsentSnapshots                          int64   `json:"ask_absent_snapshots"`
	QualifiedBidAbsentSnapshots                 int64   `json:"qualified_bid_absent_snapshots"`
	QualifiedAskAbsentSnapshots                 int64   `json:"qualified_ask_absent_snapshots"`
	SupplierRemovalSnapshotCount                int64   `json:"supplier_removal_snapshot_count"`
	SupplierRemovalBidAbsentSnapshots           int64   `json:"supplier_removal_bid_absent_snapshots"`
	SupplierRemovalAskAbsentSnapshots           int64   `json:"supplier_removal_ask_absent_snapshots"`
	SupplierRemovalBothAbsentSnapshots          int64   `json:"supplier_removal_both_absent_snapshots"`
	SupplierRemovalQualifiedBidAbsentSnapshots  int64   `json:"supplier_removal_qualified_bid_absent_snapshots"`
	SupplierRemovalQualifiedAskAbsentSnapshots  int64   `json:"supplier_removal_qualified_ask_absent_snapshots"`
	SupplierRemovalQualifiedBothAbsentSnapshots int64   `json:"supplier_removal_qualified_both_absent_snapshots"`
	SupplierRemovalOneSidedSnapshots            int64   `json:"supplier_removal_one_sided_snapshots"`
	SupplierRemovalInvalidSnapshots             int64   `json:"supplier_removal_invalid_snapshots"`
	SupplierRemovalBidAbsenceFraction           float64 `json:"supplier_removal_bid_absence_fraction"`
	SupplierRemovalAskAbsenceFraction           float64 `json:"supplier_removal_ask_absence_fraction"`
	SupplierRemovalQualifiedBidAbsenceFraction  float64 `json:"supplier_removal_qualified_bid_absence_fraction"`
	SupplierRemovalQualifiedAskAbsenceFraction  float64 `json:"supplier_removal_qualified_ask_absence_fraction"`
	SupplierRemovalCounterfactualValid          bool    `json:"supplier_removal_counterfactual_valid"`
	MinimumExecutableQty                        int64   `json:"minimum_executable_qty"`
}

type cdfManifest struct {
	Config   json.RawMessage `json:"config"`
	VenueIDs []string        `json:"venue_ids"`
	Build    struct {
		Revision string `json:"revision"`
		Modified bool   `json:"modified"`
		GOOS     string `json:"goos"`
		GOARCH   string `json:"goarch"`
		GOAMD64  string `json:"goamd64"`
	} `json:"build"`
}

type cdfRunConfig struct {
	VenueIDs                  []string            `json:"venue_ids"`
	Seed                      int64               `json:"seed"`
	Step                      int64               `json:"step"`
	LogMode                   string              `json:"log_mode"`
	EvidenceFormat            string              `json:"evidence_format"`
	ExperimentID              string              `json:"experiment_id"`
	HypothesisID              string              `json:"hypothesis_id"`
	ElasticSupplierCount      int                 `json:"elastic_supplier_count"`
	ElasticLiquiditySuppliers []cdfSupplierConfig `json:"elastic_liquidity_suppliers"`
	RecordMarketDataReceipts  bool                `json:"record_market_data_receipts"`
	MarketDataReceiptRoles    []string            `json:"market_data_receipt_roles"`
}

// CDFLiquidityRunProvenance binds a reconstructed run to its immutable input
// and execution metadata. It is separate from economic diagnostics so a valid
// local reconstruction cannot be mistaken for a valid treatment/control pair.
type CDFLiquidityRunProvenance struct {
	ConfigSHA256        string   `json:"config_sha256"`
	SourceRevision      string   `json:"source_revision"`
	SourceModified      bool     `json:"source_modified"`
	BinarySHA256        string   `json:"binary_sha256"`
	BinaryGOOS          string   `json:"binary_goos"`
	BinaryGOARCH        string   `json:"binary_goarch"`
	BinaryGOAMD64       string   `json:"binary_goamd64,omitempty"`
	Seed                int64    `json:"seed"`
	Horizon             string   `json:"horizon"`
	SimulationStartNano int64    `json:"simulation_start_nano"`
	SimulationEndNano   int64    `json:"simulation_end_nano"`
	VenueIDs            []string `json:"venue_ids"`
	ExperimentID        string   `json:"experiment_id"`
	HypothesisID        string   `json:"hypothesis_id"`
	EvidenceFormat      string   `json:"evidence_format"`
	LogMode             string   `json:"log_mode"`
	Valid               bool     `json:"valid"`
	Failure             string   `json:"failure,omitempty"`
}

type CDFLiquidityComparisonProvenance struct {
	Treatment              *CDFLiquidityRunProvenance `json:"treatment"`
	Control                *CDFLiquidityRunProvenance `json:"control"`
	AnalyzerSHA256         string                     `json:"analyzer_sha256,omitempty"`
	AnalyzerSourceRevision string                     `json:"analyzer_source_revision,omitempty"`
	AnalyzerSourceModified bool                       `json:"analyzer_source_modified,omitempty"`
	Valid                  bool                       `json:"valid"`
	Failure                string                     `json:"failure,omitempty"`
}

type cdfRunMetadata struct {
	Seed                int64  `json:"seed"`
	SimulatedHorizon    string `json:"simulated_horizon"`
	SimulationStartNano int64  `json:"simulation_start_nano"`
	SimulationEndNano   int64  `json:"simulation_end_nano"`
	ConfigSHA256        string `json:"config_sha256"`
	BinarySHA256        string `json:"binary_sha256"`
	BinaryGOOS          string `json:"binary_goos"`
	BinaryGOARCH        string `json:"binary_goarch"`
	BinaryGOAMD64       string `json:"binary_goamd64,omitempty"`
	GitRevision         string `json:"git_revision"`
	ConfigExperimentID  string `json:"config_experiment_id"`
	HypothesisID        string `json:"hypothesis_id"`
	LogMode             string `json:"log_mode"`
	EvidenceFormat      string `json:"evidence_format"`
}

type cdfSupplierConfig struct {
	Role                 string `json:"role"`
	Symbol               string `json:"symbol"`
	BaseAsset            string `json:"base_asset"`
	QuoteAsset           string `json:"quote_asset"`
	BasePrecision        int64  `json:"base_precision"`
	QuotePrecision       int64  `json:"quote_precision"`
	InitialBaseBalance   int64  `json:"initial_base_balance"`
	InitialQuoteBalance  int64  `json:"initial_quote_balance"`
	MaxPosition          int64  `json:"max_position"`
	MaxInventory         int64  `json:"max_inventory"`
	MaxQuoteQty          int64  `json:"max_quote_qty"`
	MinimumExecutableQty int64  `json:"minimum_executable_qty"`
	Interval             int64  `json:"interval"`
	MaxLossQuote         int64  `json:"max_loss_quote"`
	MaxObservationAge    int64  `json:"max_observation_age"`
	DecisionPhaseOffset  int64  `json:"decision_phase_offset"`
	ReferencePrice       int64  `json:"reference_price"`
	ReferenceHalfLife    int64  `json:"reference_half_life"`
	BaseHolding          int64  `json:"base_holding"`
	ElasticityPerPercent int64  `json:"elasticity_per_percent"`
	MakerFeeBps          int64  `json:"maker_fee_bps"`
}

func loadCDFRunConfig(run *Run) (cdfRunConfig, error) {
	raw, err := os.ReadFile(filepath.Join(run.Dir, "manifest.json"))
	if err != nil {
		return cdfRunConfig{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest cdfManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return cdfRunConfig{}, fmt.Errorf("decode manifest: %w", err)
	}
	if len(manifest.Config) == 0 || string(manifest.Config) == "null" {
		return cdfRunConfig{}, fmt.Errorf("manifest has no configuration")
	}
	var config cdfRunConfig
	if err := json.Unmarshal(manifest.Config, &config); err != nil {
		return cdfRunConfig{}, fmt.Errorf("decode configuration: %w", err)
	}
	return config, nil
}

type cdfRunIdentity struct {
	provenance       CDFLiquidityRunProvenance
	comparisonConfig string
}

func loadCDFRunIdentity(run *Run) (cdfRunIdentity, error) {
	if run == nil {
		return cdfRunIdentity{}, fmt.Errorf("nil run")
	}
	manifestRaw, err := os.ReadFile(filepath.Join(run.Dir, "manifest.json"))
	if err != nil {
		return cdfRunIdentity{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest cdfManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return cdfRunIdentity{}, fmt.Errorf("decode manifest: %w", err)
	}
	var config cdfRunConfig
	if len(manifest.Config) == 0 || json.Unmarshal(manifest.Config, &config) != nil {
		return cdfRunIdentity{}, fmt.Errorf("manifest has no valid configuration")
	}
	configRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-config.json"))
	if err != nil {
		return cdfRunIdentity{}, fmt.Errorf("read run configuration: %w", err)
	}
	configDigest := sha256.Sum256(configRaw)
	manifestConfigCanonical, err := canonicalJSON(manifest.Config)
	if err != nil {
		return cdfRunIdentity{}, fmt.Errorf("normalize manifest configuration: %w", err)
	}
	runConfigCanonical, err := canonicalJSON(configRaw)
	if err != nil {
		return cdfRunIdentity{}, fmt.Errorf("normalize run configuration: %w", err)
	}
	if manifestConfigCanonical != runConfigCanonical {
		return cdfRunIdentity{}, fmt.Errorf("manifest and copied run configuration differ")
	}
	metadataRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-metadata.json"))
	if err != nil {
		return cdfRunIdentity{}, fmt.Errorf("read run metadata: %w", err)
	}
	var metadata cdfRunMetadata
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		return cdfRunIdentity{}, fmt.Errorf("decode run metadata: %w", err)
	}
	if len(manifest.VenueIDs) == 0 || !sameStrings(manifest.VenueIDs, config.VenueIDs) {
		return cdfRunIdentity{}, fmt.Errorf("manifest and configuration venue IDs differ")
	}
	if manifest.Build.Revision == "" || !isRevision(manifest.Build.Revision) || manifest.Build.Modified {
		return cdfRunIdentity{}, fmt.Errorf("manifest build is missing a clean source revision")
	}
	if manifest.Build.GOOS != "linux" || manifest.Build.GOARCH != "amd64" || manifest.Build.GOAMD64 != "v1" {
		return cdfRunIdentity{}, fmt.Errorf("manifest build is not the registered linux/amd64/v1 target")
	}
	if metadata.ConfigSHA256 != hex.EncodeToString(configDigest[:]) || !isDigest(metadata.ConfigSHA256) || !isDigest(metadata.BinarySHA256) {
		return cdfRunIdentity{}, fmt.Errorf("run metadata does not bind the configuration and binary hashes")
	}
	if metadata.GitRevision != manifest.Build.Revision || !isRevision(metadata.GitRevision) || metadata.Seed != config.Seed || metadata.ConfigExperimentID != config.ExperimentID || metadata.HypothesisID != config.HypothesisID || metadata.LogMode != config.LogMode || metadata.EvidenceFormat != config.EvidenceFormat || metadata.BinaryGOOS != manifest.Build.GOOS || metadata.BinaryGOARCH != manifest.Build.GOARCH || metadata.BinaryGOAMD64 != manifest.Build.GOAMD64 || metadata.SimulationStartNano <= 0 || metadata.SimulationEndNano <= metadata.SimulationStartNano || metadata.SimulatedHorizon == "" {
		return cdfRunIdentity{}, fmt.Errorf("run metadata does not match manifest configuration")
	}
	comparisonConfig, err := canonicalCDFComparisonConfig(configRaw)
	if err != nil {
		return cdfRunIdentity{}, fmt.Errorf("normalize treatment/control configuration: %w", err)
	}
	return cdfRunIdentity{
		provenance: CDFLiquidityRunProvenance{
			ConfigSHA256:        metadata.ConfigSHA256,
			SourceRevision:      metadata.GitRevision,
			SourceModified:      manifest.Build.Modified,
			BinarySHA256:        metadata.BinarySHA256,
			BinaryGOOS:          metadata.BinaryGOOS,
			BinaryGOARCH:        metadata.BinaryGOARCH,
			BinaryGOAMD64:       metadata.BinaryGOAMD64,
			Seed:                metadata.Seed,
			Horizon:             metadata.SimulatedHorizon,
			SimulationStartNano: metadata.SimulationStartNano,
			SimulationEndNano:   metadata.SimulationEndNano,
			VenueIDs:            append([]string(nil), config.VenueIDs...),
			ExperimentID:        config.ExperimentID,
			HypothesisID:        config.HypothesisID,
			EvidenceFormat:      config.EvidenceFormat,
			LogMode:             config.LogMode,
			Valid:               true,
		},
		comparisonConfig: comparisonConfig,
	}, nil
}

func canonicalCDFComparisonConfig(raw []byte) (string, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil || config == nil {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("configuration is not a JSON object")
	}
	// These are the preregistered treatment/control differences: identity
	// labels, the separate CDF roster, its evidence flag, and the extra local
	// feed role needed to audit that roster. Every economic input remains.
	for _, name := range []string{
		"experiment_id", "hypothesis_id", "date", "status", "description",
		"elastic_liquidity_suppliers", "record_elastic_liquidity_supplier_decisions",
	} {
		delete(config, name)
	}
	if rawRoles, exists := config["market_data_receipt_roles"]; exists {
		var roles []string
		if err := json.Unmarshal(rawRoles, &roles); err != nil {
			return "", fmt.Errorf("decode market-data receipt roles: %w", err)
		}
		filteredRoles := roles[:0]
		for _, role := range roles {
			if role != "cdf_elastic_supplier" {
				filteredRoles = append(filteredRoles, role)
			}
		}
		encodedRoles, err := json.Marshal(filteredRoles)
		if err != nil {
			return "", err
		}
		config["market_data_receipt_roles"] = encodedRoles
	}
	canonical, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalJSON(raw []byte) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func sameStrings(left, right []string) bool {
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

func isDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func isRevision(value string) bool {
	return len(value) == 40 && strings.Trim(value, "0123456789abcdef") == ""
}

// CDFLiquidityCheck identifies one concrete reconstruction failure. Any check
// makes the audit invalid even when aggregate counts look plausible.
type CDFLiquidityCheck struct {
	VenueID  string `json:"venue_id"`
	Role     string `json:"role,omitempty"`
	ClientID uint64 `json:"client_id,omitempty"`
	Ordinal  int64  `json:"ordinal,omitempty"`
	Failure  string `json:"failure"`
}

// CDFLiquidityComparison contains a treatment and its paired no-CDF control.
// The control side-absence fraction is a matched development diagnostic, not a
// causal claim beyond the registered seed and horizon.
type CDFLiquidityComparison struct {
	Provenance                  CDFLiquidityComparisonProvenance `json:"provenance"`
	Treatment                   *CDFLiquidityRunAudit            `json:"treatment"`
	Control                     *CDFLiquidityRunAudit            `json:"control"`
	ControlBidAbsenceFraction   float64                          `json:"control_bid_absence_fraction"`
	ControlAskAbsenceFraction   float64                          `json:"control_ask_absence_fraction"`
	TreatmentBidAbsenceFraction float64                          `json:"treatment_bid_absence_fraction"`
	TreatmentAskAbsenceFraction float64                          `json:"treatment_ask_absence_fraction"`
	Valid                       bool                             `json:"valid"`
}

type cdfDecisionEvidence struct {
	Role                   string `json:"role"`
	ClientID               uint64 `json:"client_id"`
	Symbol                 string `json:"symbol"`
	DecisionTime           int64  `json:"decision_time"`
	DecisionPhaseOffset    int64  `json:"decision_phase_offset_nanos"`
	ObservationTime        int64  `json:"observation_time"`
	ObservationAge         int64  `json:"observation_age"`
	ObservationSequence    uint64 `json:"observation_sequence"`
	ObservationLinkID      uint32 `json:"observation_link_id"`
	ObservationOrdinal     uint64 `json:"observation_ordinal"`
	ObservationDeliveredAt int64  `json:"observation_delivered_at"`
	ObservationFingerprint string `json:"observation_fingerprint"`
	ObservationDigest      string `json:"observation_digest"`
	BestBid                int64  `json:"best_bid"`
	BestBidQty             int64  `json:"best_bid_qty"`
	BestAsk                int64  `json:"best_ask"`
	BestAskQty             int64  `json:"best_ask_qty"`
	MarkPrice              int64  `json:"mark_price"`
	RiskMarkPrice          int64  `json:"risk_mark_price"`
	ReferencePrice         int64  `json:"reference_price"`
	Position               int64  `json:"position"`
	TargetPosition         int64  `json:"target_position"`
	InventoryLimit         int64  `json:"inventory_limit"`
	InitialBaseBalance     int64  `json:"initial_base_balance"`
	GrossInventory         int64  `json:"gross_inventory"`
	GrossInventoryLimit    int64  `json:"gross_inventory_limit"`
	Action                 string `json:"action"`
	Reason                 string `json:"reason"`
	Side                   string `json:"side"`
	QuotePrice             int64  `json:"quote_price"`
	QuoteQty               int64  `json:"quote_qty"`
	QuoteOrderID           uint64 `json:"quote_order_id"`
	QuoteRequestID         uint64 `json:"quote_request_id"`
	CancelRequestID        uint64 `json:"cancel_request_id"`
	QuoteSubmittedAt       int64  `json:"quote_submitted_at"`
	QuoteCashAvailable     int64  `json:"quote_cash_available"`
	QuoteCashReserved      int64  `json:"quote_cash_reserved"`
	QuoteCashRequired      int64  `json:"quote_cash_required"`
	InitialEquityQuote     int64  `json:"initial_equity_quote"`
	EquityQuote            int64  `json:"equity_quote"`
	PeakEquityQuote        int64  `json:"peak_equity_quote"`
	LossFromInitialQuote   int64  `json:"loss_from_initial_quote"`
	DrawdownQuote          int64  `json:"drawdown_quote"`
	MaxLossQuote           int64  `json:"max_loss_quote"`
	EquityAvailable        bool   `json:"equity_available"`
	RiskLimitTriggered     bool   `json:"risk_limit_triggered"`
}

type cdfFillEvidence struct {
	Role           string `json:"role"`
	ClientID       uint64 `json:"client_id"`
	Symbol         string `json:"symbol"`
	OrderID        uint64 `json:"order_id"`
	TradeID        uint64 `json:"trade_id"`
	Timestamp      int64  `json:"timestamp"`
	Side           string `json:"side"`
	Price          int64  `json:"price"`
	Qty            int64  `json:"qty"`
	FeeAmount      int64  `json:"fee_amount"`
	FeeAsset       string `json:"fee_asset"`
	IsFull         bool   `json:"is_full"`
	PositionBefore int64  `json:"position_before"`
	PositionAfter  int64  `json:"position_after"`
}

type cdfAcceptedEvidence struct {
	OrderID     uint64 `json:"order_id"`
	ClientID    uint64 `json:"client_id"`
	RequestID   uint64 `json:"request_id"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	PostOnly    bool   `json:"post_only"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
}

type cdfCancelledEvidence struct {
	OrderID      uint64 `json:"order_id"`
	RequestID    uint64 `json:"request_id"`
	RemainingQty int64  `json:"remaining_qty"`
}

type cdfOrderFillEvidence struct {
	OrderID      uint64 `json:"order_id"`
	TradeID      uint64 `json:"trade_id"`
	Side         string `json:"side"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	FilledQty    int64  `json:"filled_qty"`
	RemainingQty int64  `json:"remaining_qty"`
	IsFull       bool   `json:"is_full"`
}

type cdfTradeEvidence struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         string `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
}

type cdfSnapshotEvidence struct {
	Bids []bookLevel `json:"bids"`
	Asks []bookLevel `json:"asks"`
}

type cdfAssetBalanceEvidence struct {
	Asset    string `json:"asset"`
	Free     int64  `json:"free"`
	Locked   int64  `json:"locked"`
	Borrowed int64  `json:"borrowed"`
	Interest int64  `json:"interest"`
	NetAsset int64  `json:"net_asset"`
}

type cdfBalanceSnapshotEvidence struct {
	Timestamp    int64                     `json:"timestamp"`
	ClientID     uint64                    `json:"client_id"`
	SpotBalances []cdfAssetBalanceEvidence `json:"spot_balances"`
	PerpBalances []cdfAssetBalanceEvidence `json:"perp_balances"`
	Borrowed     map[string]int64          `json:"borrowed"`
}

type cdfBorrowEvidence struct {
	ClientID uint64 `json:"client_id"`
	Asset    string `json:"asset"`
	Amount   int64  `json:"amount"`
}

type cdfParticipantKey struct {
	VenueID  string
	ClientID uint64
}

type cdfOrderKey struct {
	VenueID  string
	ClientID uint64
	OrderID  uint64
}

type cdfFillKey struct {
	VenueID  string
	ClientID uint64
	OrderID  uint64
	TradeID  uint64
}

type cdfOrderState struct {
	requestID               uint64
	clientID                uint64
	side                    string
	price                   int64
	acceptedAt              int64
	acceptedSequence        uint64
	acceptedQty             int64
	filledQty               int64
	remainingQty            int64
	closed                  bool
	closedAt                int64
	filled                  bool
	filledAt                int64
	filledOrdinal           int64
	cancelled               bool
	cancelRequestID         uint64
	cancelledSequence       uint64
	cancelRejected          bool
	cancelRejectedRequestID uint64
	cancelRejectedAt        int64
	cancelRejectedOrdinal   int64
	cancelRejectedReason    string
	cancelRequested         bool
	touchShare              float64
	touchShareKnown         bool
	remainingUpdates        []cdfOrderRemainingUpdate
}

type cdfOrderRemainingUpdate struct {
	ordinal      int64
	remainingQty int64
	closed       bool
}

type cdfRestDecision struct {
	key      cdfOrderKey
	role     string
	ordinal  int64
	side     string
	price    int64
	quantity int64
}

func (order *cdfOrderState) remainingAt(ordinal int64) (int64, bool, bool) {
	var latest cdfOrderRemainingUpdate
	found := false
	for _, update := range order.remainingUpdates {
		if update.ordinal <= ordinal && (!found || update.ordinal > latest.ordinal) {
			latest = update
			found = true
		}
	}
	if !found {
		return 0, false, false
	}
	return latest.remainingQty, latest.closed, true
}

type cdfRequestKey struct {
	venueID   string
	clientID  uint64
	requestID uint64
}

type cdfQuoteRequestState struct {
	decisionAt      int64
	decisionOrdinal int64
	acceptedAt      int64
	acceptedOrdinal int64
	rejectedAt      int64
	rejectedOrdinal int64
}

type cdfCancelRequestState struct {
	requestID  uint64
	decisionAt int64
	ordinal    int64
}

type cdfActionKey struct {
	clientID    uint64
	linkID      uint32
	requestID   uint64
	requestType uint8
	orderID     uint64
}

type cdfStaleWithdrawal struct {
	decisionAt      int64
	cancelRequestID uint64
	ordinal         int64
}

type cdfSupplierAction struct {
	key             cdfParticipantKey
	action          string
	orderID         uint64
	cancelRequestID uint64
	decisionAt      int64
	sequence        uint64
	ordinal         int64
}

type cdfPendingCancelWait struct {
	key             cdfOrderKey
	decisionAt      int64
	cancelRequestID uint64
	ordinal         int64
}

type cdfPendingOrderWait struct {
	venueID    string
	clientID   uint64
	requestID  uint64
	decisionAt int64
	ordinal    int64
}

type cdfObservedFill struct {
	fill    cdfFillEvidence
	ordinal int64
}

type cdfCashEvent struct {
	event Event
}

type cdfPendingCashReservation struct {
	side     string
	reserved int64
}

type cdfCashOrder struct {
	remainingReserve int64
}

type cdfQuoteCashLedger struct {
	available      int64
	reserved       int64
	pendingByID    map[uint64]cdfPendingCashReservation
	ordersByID     map[uint64]*cdfCashOrder
	processedFills map[cdfFillKey]struct{}
}

// MeasureCDFLiquidity reconstructs one rendered evstream run. The caller
// should first pass the original run through multivenue.RenderBinaryEvidence;
// this package depends only on the resulting public evidence layout and
// greeks.json.
func (r *Run) MeasureCDFLiquidity() (*CDFLiquidityRunAudit, error) {
	if r == nil {
		return nil, fmt.Errorf("cdf liquidity: nil run")
	}
	result := &CDFLiquidityRunAudit{
		lastDepthSnapshotAt:       make(map[string]int64),
		lastDepthTotal:            make(map[string]int64),
		lastSupplierDepthByClient: make(map[string]map[uint64]int64),
		cancelRequestedByOrder:    make(map[cdfOrderKey]struct{}),
		cancelRequestByOrder:      make(map[cdfOrderKey]uint64),
		cancelDecisionByOrder:     make(map[cdfOrderKey]cdfCancelRequestState),
		quoteRequests:             make(map[cdfRequestKey]cdfQuoteRequestState),
		expectedActionKeys:        make(map[cdfActionKey]struct{}),
		pendingOrderWaits:         make([]cdfPendingOrderWait, 0),
		pendingCancelWaits:        make([]cdfPendingCancelWait, 0),
		staleWithdrawals:          make(map[cdfOrderKey]cdfStaleWithdrawal),
		supplierActions:           make([]cdfSupplierAction, 0),
	}
	config, configErr := loadCDFRunConfig(r)
	if configErr != nil {
		result.addCheck(CDFLiquidityCheck{Failure: "missing or malformed run configuration: " + configErr.Error()})
	}
	if _, statErr := os.Stat(filepath.Join(r.Dir, "run-metadata.json")); statErr == nil {
		identity, identityErr := loadCDFRunIdentity(r)
		if identityErr != nil {
			result.addCheck(CDFLiquidityCheck{Failure: "missing or malformed run provenance: " + identityErr.Error()})
		} else {
			result.Provenance = &identity.provenance
		}
	}
	result.expectedHistoricalCountPerVenue = config.ElasticSupplierCount
	result.ExpectedHistoricalCount = config.ElasticSupplierCount
	var receiptEvidence *cdfMarketDataEvidence
	if config.RecordMarketDataReceipts {
		receiptEvidence, configErr = readCDFMarketDataEvidence(r.Dir)
		if configErr != nil {
			result.addCheck(CDFLiquidityCheck{Failure: "missing or malformed market-data receipt evidence: " + configErr.Error()})
		} else {
			result.terminalAt = receiptEvidence.terminalAt
			if result.Provenance != nil && result.Provenance.SimulationEndNano != receiptEvidence.terminalAt {
				result.addCheck(CDFLiquidityCheck{Failure: "market-data receipt terminal time does not equal provenance simulation end"})
			}
		}
	}
	configByRole := make(map[string]cdfSupplierConfig, len(config.ElasticLiquiditySuppliers))
	for _, supplier := range config.ElasticLiquiditySuppliers {
		if _, exists := configByRole[supplier.Role]; exists {
			result.addCheck(CDFLiquidityCheck{Role: supplier.Role, Failure: "duplicate configured liquidity supplier role"})
			continue
		}
		configByRole[supplier.Role] = supplier
		if supplier.MinimumExecutableQty < 0 {
			result.addCheck(CDFLiquidityCheck{Role: supplier.Role, Failure: "configured minimum executable quantity is negative"})
		} else if supplier.MinimumExecutableQty > 0 {
			if result.MinimumExecutableQty == 0 {
				result.MinimumExecutableQty = supplier.MinimumExecutableQty
			} else if result.MinimumExecutableQty != supplier.MinimumExecutableQty {
				result.addCheck(CDFLiquidityCheck{Role: supplier.Role, Failure: "configured minimum executable quantity is inconsistent across suppliers"})
			}
		}
	}
	states := make(map[cdfParticipantKey]*CDFLiquiditySupplierAudit)
	initial := make(map[cdfParticipantKey]AccountRow)
	terminal := make(map[cdfParticipantKey]AccountRow)
	venueAudits := make(map[string]*CDFLiquidityVenueAudit)
	historicalByVenue := make(map[string]int)
	venueAudit := func(venueID string) *CDFLiquidityVenueAudit {
		if audit := venueAudits[venueID]; audit != nil {
			return audit
		}
		audit := &CDFLiquidityVenueAudit{VenueID: venueID, ExpectedHistoricalCount: config.ElasticSupplierCount, MinimumExecutableQty: result.MinimumExecutableQty}
		venueAudits[venueID] = audit
		return audit
	}
	for _, row := range r.Report.InitialAccounts {
		if isHistoricalElasticSupplierRole(row.Role) {
			result.HistoricalSupplierCount++
			historicalByVenue[row.VenueID]++
			venueAudit(row.VenueID).HistoricalSupplierCount++
		}
		if !isCDFSupplierRole(row.Role) {
			continue
		}
		key := cdfParticipantKey{VenueID: row.VenueID, ClientID: row.ClientID}
		if _, exists := initial[key]; exists {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate initial supplier account"})
			continue
		}
		initial[key] = row
		states[key] = &CDFLiquiditySupplierAudit{
			VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID,
			MinPosition: math.MaxInt64, MaxPosition: math.MinInt64,
			pendingTouchByRequest: make(map[uint64]float64), pendingQuoteByRequest: make(map[uint64]cdfDecisionEvidence),
			initialSpotNetBalances: make(map[string]int64), fillNetBalanceDeltas: make(map[string]int64), initialMarks: cloneMarks(row.Marks), Valid: true, initialAccountSeen: true,
		}
		if err := assignAccountBalances(states[key].initialSpotNetBalances, row.Account.SpotBalances); err != nil {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier initial spot balances are malformed: " + err.Error()})
		}
		if hasNonZeroBalances(row.Account.PerpBalances) {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier initial account has unsupported nonzero perp balances"})
		}
		if hasNonZeroPositions(row.Account.Positions) {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier initial account has unsupported derivative positions"})
		}
		if supplierConfig, configured := configByRole[row.Role]; configured {
			state := states[key]
			state.configuredBaseAsset = supplierConfig.BaseAsset
			state.configuredQuoteAsset = supplierConfig.QuoteAsset
			state.configuredSymbol = supplierConfig.Symbol
			state.configuredMaxPosition = supplierConfig.MaxPosition
			state.configuredMaxInventory = supplierConfig.MaxInventory
			state.configuredBasePrecision = supplierConfig.BasePrecision
			state.configuredQuotePrecision = supplierConfig.QuotePrecision
			state.configuredMaxQuoteQty = supplierConfig.MaxQuoteQty
			state.configuredMinimumExecutableQty = supplierConfig.MinimumExecutableQty
			state.configuredIntervalNs = supplierConfig.Interval
			state.configuredMaxLossQuote = supplierConfig.MaxLossQuote
			state.configuredMakerFeeBps = supplierConfig.MakerFeeBps
			state.configuredReferencePrice = supplierConfig.ReferencePrice
			state.configuredReferenceHalfLife = supplierConfig.ReferenceHalfLife
			state.configuredBaseHolding = supplierConfig.BaseHolding
			state.configuredElasticityPerPercent = supplierConfig.ElasticityPerPercent
			state.reconstructedReference = supplierConfig.ReferencePrice
			state.referenceInitialized = true
			state.configuredMaxObservationAge = supplierConfig.MaxObservationAge
			state.configuredDecisionPhaseOffset = supplierConfig.DecisionPhaseOffset
			state.configuredInitialBaseBalance = supplierConfig.InitialBaseBalance
			state.configuredInitialQuoteBalance = supplierConfig.InitialQuoteBalance
			state.ConfiguredMaxPosition = supplierConfig.MaxPosition
			state.ConfiguredMaxInventory = supplierConfig.MaxInventory
			state.ConfiguredMaxQuoteQty = supplierConfig.MaxQuoteQty
			state.ConfiguredMinimumExecutableQty = supplierConfig.MinimumExecutableQty
			state.ConfiguredIntervalNs = supplierConfig.Interval
			state.ConfiguredMaxLossQuote = supplierConfig.MaxLossQuote
			state.ConfiguredMakerFeeBps = supplierConfig.MakerFeeBps
			state.ConfiguredReferencePrice = supplierConfig.ReferencePrice
			state.ConfiguredReferenceHalfLife = supplierConfig.ReferenceHalfLife
			state.ConfiguredBaseHolding = supplierConfig.BaseHolding
			state.ConfiguredElasticityPerPercent = supplierConfig.ElasticityPerPercent
			state.receiptRequired = config.RecordMarketDataReceipts && containsString(config.MarketDataReceiptRoles, auditRoleClass(row.Role))
			if supplierConfig.InitialBaseBalance <= 0 || supplierConfig.InitialQuoteBalance <= 0 || supplierConfig.BasePrecision <= 0 || supplierConfig.QuotePrecision <= 0 || !accountHasNetBalance(row.Account.SpotBalances, supplierConfig.BaseAsset, supplierConfig.InitialBaseBalance) || !accountHasNetBalance(row.Account.SpotBalances, supplierConfig.QuoteAsset, supplierConfig.InitialQuoteBalance) {
				result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier initial capital does not match registered finite balances"})
			}
		} else {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier is absent from configured liquidity roster"})
		}
	}
	for _, row := range r.Report.TerminalAccounts {
		if !isCDFSupplierRole(row.Role) {
			continue
		}
		key := cdfParticipantKey{VenueID: row.VenueID, ClientID: row.ClientID}
		if _, exists := terminal[key]; exists {
			result.addCheck(CDFLiquidityCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate terminal supplier account"})
			continue
		}
		terminal[key] = row
	}
	result.SupplierCount = len(states)
	for key, row := range initial {
		terminalRow, exists := terminal[key]
		if !exists {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier missing from terminal accounts"})
			continue
		}
		if terminalRow.Role != row.Role {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier role changed between account snapshots"})
			continue
		}
		state := states[key]
		state.InitialEquity = row.Account.Equity
		state.TerminalEquity = terminalRow.Account.Equity
		state.terminalAccountSeen = true
		state.PnL = terminalRow.Account.Equity - row.Account.Equity
		state.terminalSpotNetBalances = make(map[string]int64)
		state.terminalMarks = cloneMarks(terminalRow.Marks)
		if err := assignAccountBalances(state.terminalSpotNetBalances, terminalRow.Account.SpotBalances); err != nil {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier terminal spot balances are malformed: " + err.Error()})
		}
		if hasNonZeroBalances(terminalRow.Account.PerpBalances) {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier terminal account has unsupported nonzero perp balances"})
		}
		if hasNonZeroPositions(terminalRow.Account.Positions) {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier terminal account has unsupported derivative positions"})
		}
		if state.configuredBaseAsset == "" {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier has no configured base asset"})
		} else if mark := terminalRow.Marks[state.configuredBaseAsset]; mark > 0 {
			state.terminalMark = mark
		} else {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "terminal supplier mark is unavailable"})
		}
		var ok bool
		result.SupplierInitialEquity, ok = exactAdd(result.SupplierInitialEquity, state.InitialEquity)
		if !ok {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier initial equity overflow"})
		}
		result.SupplierTerminalEquity, ok = exactAdd(result.SupplierTerminalEquity, state.TerminalEquity)
		if !ok {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "supplier terminal equity overflow"})
		}
	}
	for key, row := range terminal {
		if _, exists := initial[key]; !exists {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: row.Role, ClientID: key.ClientID, Failure: "terminal supplier absent from initial accounts"})
		}
	}
	result.SupplierPnL = result.SupplierTerminalEquity - result.SupplierInitialEquity
	for venueID, count := range historicalByVenue {
		if count != config.ElasticSupplierCount {
			result.addCheck(CDFLiquidityCheck{VenueID: venueID, Failure: fmt.Sprintf("historical elastic supplier count %d does not match configured %d", count, config.ElasticSupplierCount)})
		}
		for role := range configByRole {
			found := false
			for key, state := range states {
				if key.VenueID == venueID && state.Role == role {
					found = true
					break
				}
			}
			if !found {
				result.addCheck(CDFLiquidityCheck{VenueID: venueID, Role: role, Failure: "configured supplier is missing from initial accounts"})
			}
		}
	}
	if len(config.VenueIDs) == 0 {
		result.addCheck(CDFLiquidityCheck{Failure: "run configuration has no venue roster"})
	}
	expectedVenues := make(map[string]struct{}, len(config.VenueIDs))
	for _, venueID := range config.VenueIDs {
		if venueID == "" {
			result.addCheck(CDFLiquidityCheck{Failure: "run configuration has an empty venue ID"})
			continue
		}
		if _, duplicate := expectedVenues[venueID]; duplicate {
			result.addCheck(CDFLiquidityCheck{VenueID: venueID, Failure: "run configuration has duplicate venue ID"})
			continue
		}
		expectedVenues[venueID] = struct{}{}
		if historicalByVenue[venueID] != config.ElasticSupplierCount {
			result.addCheck(CDFLiquidityCheck{VenueID: venueID, Failure: fmt.Sprintf("historical elastic supplier count %d does not match configured %d", historicalByVenue[venueID], config.ElasticSupplierCount)})
		}
		for role := range configByRole {
			found := false
			for key, state := range states {
				if key.VenueID == venueID && state.Role == role {
					found = true
					break
				}
			}
			if !found {
				result.addCheck(CDFLiquidityCheck{VenueID: venueID, Role: role, Failure: "configured supplier is missing from initial accounts"})
			}
		}
	}
	for venueID := range historicalByVenue {
		if _, expected := expectedVenues[venueID]; !expected {
			result.addCheck(CDFLiquidityCheck{VenueID: venueID, Failure: "historical supplier venue is outside the registered venue roster"})
		}
	}
	for key, state := range states {
		if _, expected := expectedVenues[key.VenueID]; !expected {
			result.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "CDF supplier venue is outside the registered venue roster"})
		}
	}
	if config.ElasticSupplierCount > 0 && len(historicalByVenue) == 0 {
		result.addCheck(CDFLiquidityCheck{Failure: "no historical elastic supplier accounts"})
	}
	result.ExpectedHistoricalCount = config.ElasticSupplierCount * len(historicalByVenue)

	generalFiles, bookFiles := make([]string, 0), make([]string, 0)
	for _, path := range r.Files() {
		if logicalEventLogName(path) == "general.jsonl" {
			generalFiles = append(generalFiles, path)
		}
		if logicalEventLogName(path) == "CDF-USD.jsonl" {
			bookFiles = append(bookFiles, path)
		}
	}
	orders := make(map[cdfOrderKey]*cdfOrderState)
	actualFills := make(map[cdfFillKey]cdfOrderFillEvidence)
	observedFills := make(map[cdfFillKey]cdfObservedFill)
	cashEvents := make([]cdfCashEvent, 0)
	if len(generalFiles) > 0 {
		err := r.Scan(ScanOptions{
			Events: []string{"elastic_liquidity_supplier_decision", "elastic_liquidity_supplier_fill", "balance_snapshot", "borrow"},
			Files:  generalFiles, FilesSelected: true, Workers: 1,
		}, func(event Event) {
			if event.SimTS > result.lastEventAt {
				result.lastEventAt = event.SimTS
			}
			switch event.Name {
			case "elastic_liquidity_supplier_decision":
				cashEvents = append(cashEvents, cdfCashEvent{event: event})
				result.processDecision(event, states, receiptEvidence)
			case "elastic_liquidity_supplier_fill":
				cashEvents = append(cashEvents, cdfCashEvent{event: event})
				result.processSupplierFill(event, states, observedFills)
			case "balance_snapshot":
				result.processBalanceSnapshot(event, states)
			case "borrow":
				result.processBorrow(event, states)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("cdf liquidity: scan supplier evidence: %w", err)
		}
	}
	for _, path := range bookFiles {
		err := r.Scan(ScanOptions{
			Events: []string{"Trade", "BookSnapshot", "OrderAccepted", "OrderRejected", "OrderCancelled", "OrderCancelRejected", "OrderFill"},
			Files:  []string{path}, FilesSelected: true, Workers: 1,
		}, func(event Event) {
			if event.SimTS > result.lastEventAt {
				result.lastEventAt = event.SimTS
			}
			cashEvents = append(cashEvents, cdfCashEvent{event: event})
			result.processBookEvent(event, states, orders, actualFills, venueAudits)
		})
		if err != nil {
			return nil, fmt.Errorf("cdf liquidity: scan CDF/USD book %s: %w", path, err)
		}
	}
	result.validateRestDecisionQuantities(orders, states)
	result.validateQuoteCashHeadroom(cashEvents, states)
	if len(bookFiles) == 0 {
		result.addCheck(CDFLiquidityCheck{Failure: "no rendered CDF-USD book evidence"})
	}
	result.validateReceiptActions(receiptEvidence)
	result.validatePendingCancelWaits(orders, states)
	result.validatePendingOrderWaits(states)
	result.validateStaleWithdrawals(orders, states)
	if result.terminalAt == 0 {
		result.terminalAt = result.lastEventAt
	}
	result.measureWithdrawalsWithoutReplacement(states, orders)
	result.accumulateTerminalDepth(states)
	result.reconcileFills(observedFills, actualFills, states)
	result.reconcileSupplierBalances(states)
	result.finalizeOrders(orders, states)
	result.finalizeSuppliers(states)
	result.finalizeVenueAudits(venueAudits)
	if result.SupplierCount > 0 {
		if result.TradingSupplierCount != int64(result.SupplierCount) {
			result.addCheck(CDFLiquidityCheck{Failure: fmt.Sprintf("only %d of %d suppliers traded", result.TradingSupplierCount, result.SupplierCount)})
		}
		if result.CancelCount+result.WithdrawCount == 0 {
			result.addCheck(CDFLiquidityCheck{Failure: "no supplier cancellation, withdrawal, or repricing evidence"})
		}
	}
	if result.TotalTradeVolumeQty > 0 {
		result.SupplierVolumeShare = float64(result.SupplierVolumeQty) / float64(result.TotalTradeVolumeQty)
		if result.SupplierVolumeShare > 0.75 {
			result.addCheck(CDFLiquidityCheck{Failure: "aggregate supplier volume share exceeds 75 percent"})
		}
	}
	if result.SnapshotCount > 0 {
		result.BidAbsenceFraction = float64(result.BidAbsentSnapshots) / float64(result.SnapshotCount)
		result.AskAbsenceFraction = float64(result.AskAbsentSnapshots) / float64(result.SnapshotCount)
		result.QualifiedBidAbsenceFraction = float64(result.QualifiedBidAbsentSnapshots) / float64(result.SnapshotCount)
		result.QualifiedAskAbsenceFraction = float64(result.QualifiedAskAbsentSnapshots) / float64(result.SnapshotCount)
	}
	allSuppliersValid := true
	for _, supplier := range result.Suppliers {
		allSuppliersValid = allSuppliersValid && supplier.Valid
	}
	result.Valid = len(result.Checks) == 0 && allSuppliersValid && (result.SupplierCount == 0 || result.TradingSupplierCount == int64(result.SupplierCount))
	return result, nil
}

// CompareCDFLiquidityRuns applies the paired-control contract without
// interpreting the market-survival outcome. Treatment must contain the
// separate CDF roster, while control must contain none.
func CompareCDFLiquidityRuns(treatment, control *Run) (*CDFLiquidityComparison, error) {
	treatmentAudit, err := treatment.MeasureCDFLiquidity()
	if err != nil {
		return nil, err
	}
	controlAudit, err := control.MeasureCDFLiquidity()
	if err != nil {
		return nil, err
	}
	treatmentIdentity, err := loadCDFRunIdentity(treatment)
	if err != nil {
		return nil, fmt.Errorf("treatment provenance: %w", err)
	}
	controlIdentity, err := loadCDFRunIdentity(control)
	if err != nil {
		return nil, fmt.Errorf("control provenance: %w", err)
	}
	comparison := &CDFLiquidityComparison{
		Provenance: CDFLiquidityComparisonProvenance{
			Treatment: &treatmentIdentity.provenance,
			Control:   &controlIdentity.provenance,
		},
		Treatment: treatmentAudit, Control: controlAudit,
		ControlBidAbsenceFraction:   controlAudit.BidAbsenceFraction,
		ControlAskAbsenceFraction:   controlAudit.AskAbsenceFraction,
		TreatmentBidAbsenceFraction: treatmentAudit.BidAbsenceFraction,
		TreatmentAskAbsenceFraction: treatmentAudit.AskAbsenceFraction,
	}
	comparison.Provenance.Valid = treatmentIdentity.provenance.Valid && controlIdentity.provenance.Valid
	if treatmentIdentity.comparisonConfig != controlIdentity.comparisonConfig {
		comparison.Provenance.Valid = false
		comparison.Provenance.Failure = "treatment/control configurations differ outside the registered CDF roster and evidence identity fields"
	}
	if treatmentIdentity.provenance.Seed != controlIdentity.provenance.Seed || treatmentIdentity.provenance.Horizon != controlIdentity.provenance.Horizon || treatmentIdentity.provenance.SimulationStartNano != controlIdentity.provenance.SimulationStartNano || treatmentIdentity.provenance.SimulationEndNano != controlIdentity.provenance.SimulationEndNano || !sameStrings(treatmentIdentity.provenance.VenueIDs, controlIdentity.provenance.VenueIDs) || treatmentIdentity.provenance.SourceRevision != controlIdentity.provenance.SourceRevision || treatmentIdentity.provenance.BinarySHA256 != controlIdentity.provenance.BinarySHA256 || treatmentIdentity.provenance.BinaryGOOS != controlIdentity.provenance.BinaryGOOS || treatmentIdentity.provenance.BinaryGOARCH != controlIdentity.provenance.BinaryGOARCH || treatmentIdentity.provenance.BinaryGOAMD64 != controlIdentity.provenance.BinaryGOAMD64 {
		comparison.Provenance.Valid = false
		comparison.Provenance.Failure = "treatment/control execution provenance is not paired"
	}
	comparison.Valid = comparison.Provenance.Valid && treatmentAudit.Valid && controlAudit.Valid && treatmentAudit.SupplierCount > 0 && controlAudit.SupplierCount == 0
	return comparison, nil
}

func (r *CDFLiquidityRunAudit) addCheck(check CDFLiquidityCheck) {
	r.Checks = append(r.Checks, check)
}

func (r *CDFLiquidityRunAudit) processDecision(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit, receiptEvidence *cdfMarketDataEvidence) {
	var decision cdfDecisionEvidence
	required := []string{"role", "client_id", "symbol", "decision_time", "decision_phase_offset_nanos", "observation_time", "observation_age", "observation_digest", "best_bid", "best_bid_qty", "best_ask", "best_ask_qty", "mark_price", "reference_price", "position", "target_position", "inventory_limit", "initial_base_balance", "gross_inventory", "gross_inventory_limit", "action", "reason", "quote_cash_available"}
	if err := decodeRequiredJSON(event.Raw(), &decision, required...); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier decision: " + err.Error()})
		return
	}
	key := cdfParticipantKey{VenueID: event.VenueID, ClientID: decision.ClientID}
	state := states[key]
	if state == nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision from unregistered supplier"})
		return
	}
	if state.configuredMaxLossQuote > 0 {
		riskRequired := []string{"risk_mark_price", "quote_cash_reserved", "initial_equity_quote", "equity_quote", "peak_equity_quote", "loss_from_initial_quote", "drawdown_quote", "max_loss_quote", "equity_available", "risk_limit_triggered"}
		if err := decodeRequiredJSON(event.Raw(), &decision, riskRequired...); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier marked-risk state: " + err.Error()})
			return
		}
	}
	r.DecisionCount++
	state.DecisionCount++
	if decision.Action == "submit" || (decision.Action == "withdraw" && decision.QuoteOrderID != 0) {
		r.supplierActions = append(r.supplierActions, cdfSupplierAction{
			key:             key,
			action:          decision.Action,
			orderID:         decision.QuoteOrderID,
			cancelRequestID: decision.CancelRequestID,
			decisionAt:      decision.DecisionTime,
			sequence:        event.Sequence,
			ordinal:         event.Ordinal,
		})
	}
	if event.ClientID != decision.ClientID || decision.Role != state.Role || decision.Symbol != state.configuredSymbol || decision.DecisionTime != event.SimTS {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision identity or timestamp mismatch"})
	}
	if decision.DecisionPhaseOffset != state.configuredDecisionPhaseOffset {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision phase offset disagrees with registered configuration"})
	}
	if state.lastDecisionAt > 0 && (decision.DecisionTime < state.lastDecisionAt || decision.DecisionTime == state.lastDecisionAt && event.Ordinal <= state.lastDecisionOrdinal) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decisions are not in causal timestamp order"})
	}
	state.lastDecisionAt, state.lastDecisionOrdinal = decision.DecisionTime, event.Ordinal
	missingObservation := decision.ObservationTime == 0 && decision.ObservationSequence == 0 && decision.ObservationAge == 0
	initialSubscriptionWait := state.DecisionCount == 1 && decision.Action == "wait" && decision.Reason == "subscribe"
	missingObservationWait := decision.Action == "wait" && (decision.Reason == "stale_or_missing_observation" || decision.Reason == "order_pending" || decision.Reason == "cancel_pending")
	emptyObservationAllowed := missingObservation && (initialSubscriptionWait || missingObservationWait)
	validObservation := (emptyObservationAllowed) || (!missingObservation && decision.ObservationTime > 0 && decision.ObservationSequence > 0 && decision.ObservationTime <= decision.DecisionTime && decision.DecisionTime-decision.ObservationTime == decision.ObservationAge)
	if !validObservation || decision.ObservationAge < 0 || decision.ReferencePrice <= 0 || decision.InventoryLimit <= 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "invalid decision bounds or observation time"})
	}
	r.validateCDFReference(event, decision, state)
	if state.receiptRequired {
		r.validateDecisionObservation(event, decision, emptyObservationAllowed, receiptEvidence)
	}
	staleWithdrawal := isCDFStaleWithdrawal(decision, state.configuredMaxObservationAge)
	if decision.Action == "withdraw" && decision.Reason == "stale_or_missing_observation" && !staleWithdrawal {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "stale withdrawal does not exceed registered delayed-data bound"})
	}
	if state.configuredMaxObservationAge <= 0 || decision.ObservationAge > state.configuredMaxObservationAge && !staleWithdrawal && !missingObservationWait {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision observation age exceeds registered delayed-data bound"})
	}
	if decision.Position < -decision.InventoryLimit || decision.Position > decision.InventoryLimit || decision.TargetPosition < -decision.InventoryLimit || decision.TargetPosition > decision.InventoryLimit {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision exceeds inventory limit"})
	}
	if decision.InitialBaseBalance != state.configuredInitialBaseBalance || decision.GrossInventoryLimit != state.configuredMaxInventory {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision gross inventory contract disagrees with registered configuration"})
	}
	expectedGrossInventory, grossOK := exactAdd(decision.InitialBaseBalance, decision.Position)
	if !grossOK || decision.GrossInventory != expectedGrossInventory || decision.GrossInventory < 0 || decision.GrossInventory > decision.GrossInventoryLimit {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision gross inventory violates finite holding limit"})
	}
	if state.positionSet && state.lastPosition != decision.Position {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision position does not follow prior fill"})
	}
	state.positionSet, state.lastPosition = true, decision.Position
	if state.InventoryLimit != 0 && state.InventoryLimit != decision.InventoryLimit {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "inventory limit changed during run"})
	}
	state.InventoryLimit = decision.InventoryLimit
	if state.configuredMaxPosition <= 0 || decision.InventoryLimit != state.configuredMaxPosition {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "decision inventory limit disagrees with registered configuration"})
	}
	if decision.ObservationAge > state.MaxObservationAgeNs {
		state.MaxObservationAgeNs = decision.ObservationAge
	}
	state.observationAgeTotal, _ = exactAdd(state.observationAgeTotal, decision.ObservationAge)
	state.observationCount++
	if state.receiptRequired {
		r.validateDecisionReceipt(event, decision, receiptEvidence)
	}
	if state.configuredMaxLossQuote > 0 {
		r.validateMarkedRiskDecision(event, decision, state)
	}
	if decision.MarkPrice > 0 {
		expectedTarget, targetOK := expectedCDFTargetPosition(decision.MarkPrice, state.reconstructedReference, state)
		if !targetOK || decision.TargetPosition != expectedTarget {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier target position does not match registered elasticity policy"})
		}
	}
	if decision.Action == "submit" || decision.Action == "rest" {
		expectedSide, expectedQuantity, expected := expectedCDFInventoryQuote(decision, state)
		quantityMatches := expected && decision.QuoteQty == expectedQuantity
		if !expected || decision.Side != expectedSide || !quantityMatches {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier quote does not match its finite inventory target gap"})
		}
		if expected && decision.Side == expectedSide && quantityMatches && state.inventoryChangedSinceActionable && state.hasActionableDecision {
			counterfactualGrossInventory, counterfactualGrossOK := exactAdd(state.configuredInitialBaseBalance, state.lastActionablePosition)
			counterfactualSide, counterfactualQty, counterfactualOK := expectedCDFInventoryQuoteAtWithCash(decision.TargetPosition, state.lastActionablePosition, counterfactualGrossInventory, decision.QuotePrice, decision.QuoteCashAvailable, state)
			counterfactualOK = counterfactualOK && counterfactualGrossOK
			if !counterfactualOK {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "inventory response no-fill counterfactual is not reconstructible"})
			} else if decision.Side != counterfactualSide || decision.QuoteQty != counterfactualQty {
				state.InventoryResponsiveDecisionCount++
				r.InventoryResponsiveDecisionCount++
			}
		}
		state.lastActionablePosition = decision.Position
		state.hasActionableDecision = true
		state.inventoryChangedSinceActionable = false
	}
	switch decision.Action {
	case "submit":
		state.SubmitCount++
		r.SubmitCount++
		if decision.QuoteRequestID == 0 || decision.QuoteOrderID != 0 || decision.ObservationSequence == 0 || !validSide(decision.Side) || decision.QuotePrice <= 0 || decision.QuoteQty <= 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "submit decision has incomplete quote identity"})
			break
		}
		requestKey := cdfRequestKey{venueID: event.VenueID, clientID: decision.ClientID, requestID: decision.QuoteRequestID}
		if _, exists := r.quoteRequests[requestKey]; exists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier order request"})
		} else {
			r.quoteRequests[requestKey] = cdfQuoteRequestState{decisionAt: decision.DecisionTime, decisionOrdinal: event.Ordinal}
		}
		if state.receiptRequired {
			r.expectedActionKeys[cdfActionKey{clientID: decision.ClientID, linkID: decision.ObservationLinkID, requestID: decision.QuoteRequestID, requestType: 2}] = struct{}{}
		}
		if state.configuredMaxQuoteQty <= 0 || decision.QuoteQty > state.configuredMaxQuoteQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "submitted quote exceeds registered maximum quantity"})
		}
		if !quoteMatchesObservedTouch(decision) {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "submitted quote is not the observed local touch"})
		}
		if decision.QuoteQty > state.maxQuoteQty {
			state.maxQuoteQty = decision.QuoteQty
		}
		if decision.Side == "BUY" && state.configuredInitialQuoteBalance > 0 && state.configuredQuotePrecision > 0 {
			expectedRequired, requiredOK := expectedCDFQuoteRequirement(decision.QuotePrice, decision.QuoteQty, state.configuredBasePrecision, state.configuredMakerFeeBps)
			if decision.QuoteCashAvailable <= 0 || decision.QuoteCashRequired <= 0 || !requiredOK || decision.QuoteCashRequired != expectedRequired || decision.QuoteCashRequired > decision.QuoteCashAvailable {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier buy quote exceeds its attested quote-cash headroom"})
			}
		}
		if share, ok := decisionTouchShare(decision); ok {
			state.pendingTouchByRequest[decision.QuoteRequestID] = share
			state.pendingQuoteByRequest[decision.QuoteRequestID] = decision
			state.touchShareTotal += share
			state.touchShareCount++
			if share > state.MaxObservedTouchShare {
				state.MaxObservedTouchShare = share
			}
		} else {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "submitted quote has no positive observed touch depth"})
		}
	case "rest":
		state.RestCount++
		r.RestCount++
		r.restDecisions = append(r.restDecisions, cdfRestDecision{
			key:  cdfOrderKey{VenueID: event.VenueID, ClientID: decision.ClientID, OrderID: decision.QuoteOrderID},
			role: decision.Role, ordinal: event.Ordinal, side: decision.Side, price: decision.QuotePrice, quantity: decision.QuoteQty,
		})
		if decision.QuoteOrderID == 0 || decision.ObservationSequence == 0 || !validSide(decision.Side) || decision.QuotePrice <= 0 || decision.QuoteQty <= 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "rest decision has incomplete quote identity"})
		}
		if state.configuredMaxQuoteQty <= 0 || decision.QuoteQty > state.configuredMaxQuoteQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "resting quote exceeds registered maximum quantity"})
		}
		if !quoteMatchesObservedTouch(decision) {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "resting quote is not the observed local touch"})
		}
		if decision.QuoteQty > state.maxQuoteQty {
			state.maxQuoteQty = decision.QuoteQty
		}
		if share, ok := decisionTouchShare(decision); ok {
			state.touchShareTotal += share
			state.touchShareCount++
			if share > state.MaxObservedTouchShare {
				state.MaxObservedTouchShare = share
			}
		}
	case "cancel", "withdraw":
		if decision.Action == "cancel" {
			state.CancelCount++
			r.CancelCount++
		} else {
			state.WithdrawCount++
			r.WithdrawCount++
		}
		if decision.QuoteOrderID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "withdrawal decision has no order identity"})
		} else {
			orderKey := cdfOrderKey{VenueID: event.VenueID, ClientID: decision.ClientID, OrderID: decision.QuoteOrderID}
			r.cancelRequestedByOrder[orderKey] = struct{}{}
			if decision.CancelRequestID != 0 {
				r.cancelRequestByOrder[orderKey] = decision.CancelRequestID
				r.cancelDecisionByOrder[orderKey] = cdfCancelRequestState{requestID: decision.CancelRequestID, decisionAt: decision.DecisionTime, ordinal: event.Ordinal}
				if state.receiptRequired {
					r.expectedActionKeys[cdfActionKey{clientID: decision.ClientID, linkID: decision.ObservationLinkID, requestID: decision.CancelRequestID, requestType: 3, orderID: decision.QuoteOrderID}] = struct{}{}
				}
			}
			if staleWithdrawal {
				if _, duplicate := r.staleWithdrawals[orderKey]; duplicate {
					r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "duplicate stale withdrawal for supplier order"})
				} else {
					r.staleWithdrawals[orderKey] = cdfStaleWithdrawal{decisionAt: decision.DecisionTime, cancelRequestID: decision.CancelRequestID, ordinal: event.Ordinal}
				}
			}
		}
		if decision.CancelRequestID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "withdrawal decision has no cancellation request identity"})
		}
	case "wait":
		r.validateWaitState(event, decision, state)
	default:
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "unknown supplier action " + decision.Action})
	}
}

func (r *CDFLiquidityRunAudit) validateWaitState(event Event, decision cdfDecisionEvidence, state *CDFLiquiditySupplierAudit) {
	orderKey := cdfOrderKey{VenueID: event.VenueID, ClientID: decision.ClientID, OrderID: decision.QuoteOrderID}
	switch decision.Reason {
	case "subscribe":
		if state.DecisionCount != 1 || decision.QuoteOrderID != 0 || decision.QuoteRequestID != 0 || decision.CancelRequestID != 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "subscription wait has outstanding order state"})
		}
	case "stale_or_missing_observation":
		if decision.QuoteOrderID != 0 || decision.QuoteRequestID != 0 || decision.CancelRequestID != 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "stale-observation wait has outstanding order state"})
		}
	case "inventory_at_target", "one_sided_or_locked_book", "limit_or_touch_unavailable", "quote_cash_limit", "below_minimum_executable_qty":
		if decision.QuoteOrderID != 0 || decision.QuoteRequestID != 0 || decision.CancelRequestID != 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "no-action wait has outstanding order state"})
		}
	case "order_pending":
		if decision.QuoteOrderID != 0 || decision.QuoteRequestID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "order-pending wait has invalid request identity"})
		} else if _, exists := state.pendingQuoteByRequest[decision.QuoteRequestID]; !exists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "order-pending wait has no outstanding submission"})
		} else {
			r.pendingOrderWaits = append(r.pendingOrderWaits, cdfPendingOrderWait{venueID: event.VenueID, clientID: decision.ClientID, requestID: decision.QuoteRequestID, decisionAt: decision.DecisionTime, ordinal: event.Ordinal})
		}
	case "cancel_pending":
		requestedCancel, requestExists := r.cancelRequestByOrder[orderKey]
		if decision.QuoteOrderID == 0 || decision.CancelRequestID == 0 || !requestExists || requestedCancel != decision.CancelRequestID {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "cancel-pending wait has no matching live cancellation"})
		} else {
			r.pendingCancelWaits = append(r.pendingCancelWaits, cdfPendingCancelWait{key: orderKey, decisionAt: decision.DecisionTime, cancelRequestID: decision.CancelRequestID, ordinal: event.Ordinal})
		}
	default:
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "unknown supplier wait reason " + decision.Reason})
	}
}

func (r *CDFLiquidityRunAudit) validateMarkedRiskDecision(event Event, decision cdfDecisionEvidence, state *CDFLiquiditySupplierAudit) {
	wasRiskStateSeen := state.riskStateSeen
	state.RiskStateDecisionCount++
	r.RiskStateDecisionCount++
	state.riskStateSeen = true
	addFailure := func(failure string) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: failure})
	}
	if decision.MaxLossQuote != state.configuredMaxLossQuote || decision.MaxLossQuote <= 0 {
		addFailure("supplier decision maximum loss budget disagrees with registered configuration")
	}
	initialEquity, initialOK := expectedCDFInitialEquity(state)
	if !initialOK || decision.InitialEquityQuote != initialEquity {
		addFailure("supplier decision initial marked equity is not reconstructible from registered endowment")
	}
	if decision.RiskMarkPrice <= 0 {
		addFailure("supplier decision has no positive risk mark")
	}
	if decision.QuoteCashAvailable < 0 || decision.QuoteCashReserved < 0 {
		addFailure("supplier decision exposes negative quote cash state")
	}
	if state.riskLimitSeen && !decision.RiskLimitTriggered {
		addFailure("supplier risk-limit flag is not monotonic")
	}
	if !decision.EquityAvailable {
		if decision.Reason != "equity_unavailable" || decision.RiskLimitTriggered {
			addFailure("unavailable supplier equity has an invalid decision state")
		}
		return
	}
	if decision.Reason == "equity_unavailable" {
		addFailure("available supplier equity is labeled unavailable")
	}
	expectedEquity, equityOK := expectedCDFMarkedEquity(decision, state)
	if !equityOK || decision.EquityQuote != expectedEquity {
		addFailure("supplier decision marked equity does not reconcile to cash, inventory, and risk mark")
		return
	}
	if decision.PeakEquityQuote < decision.EquityQuote {
		addFailure("supplier peak marked equity is below current equity")
	}
	loss, lossOK := positiveQuoteDifference(decision.InitialEquityQuote, decision.EquityQuote)
	drawdown, drawdownOK := positiveQuoteDifference(decision.PeakEquityQuote, decision.EquityQuote)
	if !lossOK || decision.LossFromInitialQuote != loss {
		addFailure("supplier loss from initial equity is not exact")
	}
	if !drawdownOK || decision.DrawdownQuote != drawdown {
		addFailure("supplier marked-equity drawdown is not exact")
	}
	if wasRiskStateSeen {
		if decision.InitialEquityQuote != state.lastRiskInitialEquity || decision.PeakEquityQuote < state.lastRiskPeakEquity {
			addFailure("supplier marked-risk baseline or peak changed during the run")
		}
	}
	thresholdReached := lossOK && drawdownOK && (loss >= state.configuredMaxLossQuote || drawdown >= state.configuredMaxLossQuote)
	if thresholdReached && !decision.RiskLimitTriggered {
		addFailure("supplier risk-limit flag omitted after registered loss budget was reached")
	}
	if decision.RiskLimitTriggered && !thresholdReached {
		addFailure("supplier risk-limit flag was set before the registered loss budget was reached")
	}
	if decision.RiskLimitTriggered && (decision.Action != "wait" && decision.Action != "withdraw" || decision.Reason != "loss_limit") {
		addFailure("supplier risk-limit decision did not withdraw or wait with loss-limit reason")
	}
	if decision.RiskLimitTriggered {
		state.riskLimitSeen = true
		state.RiskLimitTriggeredDecisionCount++
		r.RiskLimitTriggeredDecisionCount++
	}
	state.lastRiskInitialEquity = decision.InitialEquityQuote
	state.lastRiskEquity = decision.EquityQuote
	state.lastRiskPeakEquity = decision.PeakEquityQuote
	state.lastRiskLossFromInitial = decision.LossFromInitialQuote
	state.lastRiskDrawdown = decision.DrawdownQuote
	state.lastRiskMarkPrice = decision.RiskMarkPrice
	if decision.LossFromInitialQuote > state.MaxObservedLossFromInitialQuote {
		state.MaxObservedLossFromInitialQuote = decision.LossFromInitialQuote
	}
	if decision.DrawdownQuote > state.MaxObservedDrawdownQuote {
		state.MaxObservedDrawdownQuote = decision.DrawdownQuote
	}
}

func expectedCDFInitialEquity(state *CDFLiquiditySupplierAudit) (int64, bool) {
	if state.configuredInitialQuoteBalance <= 0 || state.configuredInitialBaseBalance < 0 || state.configuredReferencePrice <= 0 || state.configuredBasePrecision <= 0 {
		return 0, false
	}
	baseNotional, ok := etypes.TryMulDiv(state.configuredInitialBaseBalance, state.configuredReferencePrice, state.configuredBasePrecision)
	if !ok {
		return 0, false
	}
	return exactAdd(state.configuredInitialQuoteBalance, baseNotional)
}

func expectedCDFMarkedEquity(decision cdfDecisionEvidence, state *CDFLiquiditySupplierAudit) (int64, bool) {
	if decision.RiskMarkPrice <= 0 || decision.QuoteCashAvailable < 0 || decision.QuoteCashReserved < 0 || state.configuredBasePrecision <= 0 {
		return 0, false
	}
	grossInventory, ok := exactAdd(state.configuredInitialBaseBalance, decision.Position)
	if !ok || grossInventory < 0 {
		return 0, false
	}
	notional := new(big.Int).Mul(big.NewInt(grossInventory), big.NewInt(decision.RiskMarkPrice))
	notional.Quo(notional, big.NewInt(state.configuredBasePrecision))
	equity := new(big.Int).Add(notional, big.NewInt(decision.QuoteCashAvailable))
	equity.Add(equity, big.NewInt(decision.QuoteCashReserved))
	if !equity.IsInt64() {
		return 0, false
	}
	return equity.Int64(), true
}

func positiveQuoteDifference(larger, smaller int64) (int64, bool) {
	difference := new(big.Int).Sub(big.NewInt(larger), big.NewInt(smaller))
	if difference.Sign() <= 0 {
		return 0, true
	}
	if !difference.IsInt64() {
		return 0, false
	}
	return difference.Int64(), true
}

func (r *CDFLiquidityRunAudit) validateQuoteCashHeadroom(events []cdfCashEvent, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	sort.SliceStable(events, func(left, right int) bool {
		leftEvent, rightEvent := events[left].event, events[right].event
		if leftEvent.Sequence != rightEvent.Sequence {
			return leftEvent.Sequence < rightEvent.Sequence
		}
		leftPriority, rightPriority := cdfCashEventPriority(leftEvent.Name), cdfCashEventPriority(rightEvent.Name)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		if leftEvent.SimTS != rightEvent.SimTS {
			return leftEvent.SimTS < rightEvent.SimTS
		}
		if leftEvent.File != rightEvent.File {
			return leftEvent.File < rightEvent.File
		}
		return leftEvent.Ordinal < rightEvent.Ordinal
	})

	ledgers := make(map[cdfParticipantKey]*cdfQuoteCashLedger, len(states))
	for key, state := range states {
		ledgers[key] = &cdfQuoteCashLedger{
			available:      state.configuredInitialQuoteBalance,
			pendingByID:    make(map[uint64]cdfPendingCashReservation),
			ordersByID:     make(map[uint64]*cdfCashOrder),
			processedFills: make(map[cdfFillKey]struct{}),
		}
	}

	for _, cashEvent := range events {
		event := cashEvent.event
		key := cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}
		ledger := ledgers[key]
		state := states[key]
		if ledger == nil || state == nil {
			continue
		}
		if event.Sequence == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash ledger event has no venue sequence"})
			continue
		}

		switch event.Name {
		case "elastic_liquidity_supplier_decision":
			r.processQuoteCashDecision(event, state, ledger)
		case "OrderAccepted":
			r.processQuoteCashAccepted(event, state, ledger)
		case "OrderRejected":
			r.processQuoteCashRejected(event, state, ledger)
		case "OrderCancelled":
			r.processQuoteCashCancelled(event, state, ledger)
		case "elastic_liquidity_supplier_fill":
			r.processQuoteCashFill(event, state, ledger)
		}
	}
}

func cdfCashEventPriority(eventName string) int {
	switch eventName {
	case "elastic_liquidity_supplier_decision":
		return 0
	case "OrderAccepted", "OrderRejected":
		return 1
	case "elastic_liquidity_supplier_fill":
		return 2
	case "OrderCancelled", "OrderCancelRejected":
		return 3
	default:
		return 4
	}
}

func (r *CDFLiquidityRunAudit) processQuoteCashDecision(event Event, state *CDFLiquiditySupplierAudit, ledger *cdfQuoteCashLedger) {
	var decision cdfDecisionEvidence
	required := []string{"role", "client_id", "action", "quote_cash_available"}
	if state.configuredMaxLossQuote > 0 {
		required = append(required, "quote_cash_reserved")
	}
	if err := decodeRequiredJSON(event.Raw(), &decision, required...); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed quote-cash decision: " + err.Error()})
		return
	}
	if decision.Role != state.Role || decision.ClientID != event.ClientID {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash decision identity mismatch"})
	}
	if decision.QuoteCashAvailable < 0 || decision.QuoteCashAvailable != ledger.available {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier quote-cash headroom does not reconcile to independent ledger"})
	}
	if state.configuredMaxLossQuote > 0 && (decision.QuoteCashReserved < 0 || decision.QuoteCashReserved != ledger.reserved) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier reserved quote cash does not reconcile to independent ledger"})
	}
	if decision.Action != "submit" {
		return
	}
	if decision.QuoteRequestID == 0 || !validSide(decision.Side) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash submission has incomplete request identity"})
		return
	}
	if _, exists := ledger.pendingByID[decision.QuoteRequestID]; exists {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash submission reuses a request identity"})
		return
	}
	reservation := int64(0)
	if decision.Side == "BUY" {
		var ok bool
		reservation, ok = expectedCDFQuoteCashAmount(decision.QuotePrice, decision.QuoteQty, state.configuredBasePrecision, state.configuredMakerFeeBps)
		if !ok || decision.QuoteCashRequired != reservation {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier buy reservation is not independently reconstructible"})
			return
		}
		if reservation > ledger.available {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier buy reservation exceeds independently reconstructed cash"})
			return
		}
		ledger.available -= reservation
		ledger.reserved, _ = exactAdd(ledger.reserved, reservation)
	}
	ledger.pendingByID[decision.QuoteRequestID] = cdfPendingCashReservation{side: decision.Side, reserved: reservation}
}

func (r *CDFLiquidityRunAudit) processQuoteCashAccepted(event Event, state *CDFLiquiditySupplierAudit, ledger *cdfQuoteCashLedger) {
	var accepted cdfAcceptedEvidence
	if err := decodeRequiredJSON(event.Raw(), &accepted, "order_id", "request_id", "side", "price", "qty"); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed quote-cash acceptance: " + err.Error()})
		return
	}
	pending, exists := ledger.pendingByID[accepted.RequestID]
	if !exists {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "accepted quote has no independent cash reservation"})
		return
	}
	if accepted.ClientID != 0 && accepted.ClientID != event.ClientID || accepted.OrderID == 0 || !validSide(accepted.Side) || accepted.Qty <= 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash acceptance identity is invalid"})
		return
	}
	if _, exists := ledger.ordersByID[accepted.OrderID]; exists {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash acceptance reuses an order identity"})
		return
	}
	if pending.side != accepted.Side {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "accepted quote side disagrees with its cash reservation"})
		return
	}
	delete(ledger.pendingByID, accepted.RequestID)
	ledger.ordersByID[accepted.OrderID] = &cdfCashOrder{remainingReserve: pending.reserved}
}

func (r *CDFLiquidityRunAudit) processQuoteCashRejected(event Event, state *CDFLiquiditySupplierAudit, ledger *cdfQuoteCashLedger) {
	var rejected struct {
		RequestID uint64 `json:"request_id"`
	}
	if err := decodeRequiredJSON(event.Raw(), &rejected, "request_id"); err != nil || rejected.RequestID == 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed quote-cash rejection"})
		return
	}
	pending, exists := ledger.pendingByID[rejected.RequestID]
	if !exists {
		return
	}
	delete(ledger.pendingByID, rejected.RequestID)
	if pending.reserved > 0 {
		ledger.available, _ = exactAdd(ledger.available, pending.reserved)
		ledger.reserved, _ = exactAdd(ledger.reserved, -pending.reserved)
	}
}

func (r *CDFLiquidityRunAudit) processQuoteCashCancelled(event Event, state *CDFLiquiditySupplierAudit, ledger *cdfQuoteCashLedger) {
	var cancelled cdfCancelledEvidence
	if err := decodeRequiredJSON(event.Raw(), &cancelled, "order_id", "request_id"); err != nil || cancelled.OrderID == 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed quote-cash cancellation"})
		return
	}
	order, exists := ledger.ordersByID[cancelled.OrderID]
	if !exists {
		return
	}
	if order.remainingReserve > 0 {
		ledger.available, _ = exactAdd(ledger.available, order.remainingReserve)
		ledger.reserved, _ = exactAdd(ledger.reserved, -order.remainingReserve)
	}
	delete(ledger.ordersByID, cancelled.OrderID)
}

func (r *CDFLiquidityRunAudit) processQuoteCashFill(event Event, state *CDFLiquiditySupplierAudit, ledger *cdfQuoteCashLedger) {
	var fill cdfFillEvidence
	if err := decodeRequiredJSON(event.Raw(), &fill, "order_id", "trade_id", "side", "price", "qty", "is_full", "fee_amount", "fee_asset"); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed quote-cash fill: " + err.Error()})
		return
	}
	key := cdfFillKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: fill.OrderID, TradeID: fill.TradeID}
	if _, exists := ledger.processedFills[key]; exists {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash fill is duplicated"})
		return
	}
	ledger.processedFills[key] = struct{}{}
	order := ledger.ordersByID[fill.OrderID]
	if order == nil || fill.Qty <= 0 || fill.Price <= 0 || fill.FeeAmount < 0 || !validSide(fill.Side) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash fill has no valid live reservation"})
		return
	}
	amount, ok := cdfQuoteCashFillAmount(fill, state)
	if !ok {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash fill amount is not reconstructible"})
		return
	}
	if fill.Side == "BUY" {
		if order.remainingReserve < amount {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash fill exceeds independent reservation"})
			return
		}
		order.remainingReserve -= amount
		ledger.reserved, _ = exactAdd(ledger.reserved, -amount)
	} else {
		ledger.available, ok = exactAdd(ledger.available, amount)
		if !ok {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "quote-cash sell proceeds overflow"})
			return
		}
	}
	if fill.IsFull {
		if order.remainingReserve > 0 {
			ledger.available, _ = exactAdd(ledger.available, order.remainingReserve)
			ledger.reserved, _ = exactAdd(ledger.reserved, -order.remainingReserve)
		}
		delete(ledger.ordersByID, fill.OrderID)
	}
}

func expectedCDFQuoteCashAmount(price, quantity, basePrecision, makerFeeBps int64) (int64, bool) {
	if price <= 0 || quantity <= 0 || basePrecision <= 0 || makerFeeBps < 0 {
		return 0, false
	}
	notional, ok := etypes.TryMulDiv(price, quantity, basePrecision)
	if !ok {
		return 0, false
	}
	fee := new(big.Int).Mul(big.NewInt(notional), big.NewInt(makerFeeBps))
	fee.Quo(fee, big.NewInt(10_000))
	if !fee.IsInt64() {
		return 0, false
	}
	return exactAdd(notional, fee.Int64())
}

func cdfQuoteCashFillAmount(fill cdfFillEvidence, state *CDFLiquiditySupplierAudit) (int64, bool) {
	notional, ok := etypes.TryMulDiv(fill.Price, fill.Qty, state.configuredBasePrecision)
	if !ok {
		return 0, false
	}
	if fill.FeeAsset == state.configuredQuoteAsset {
		if fill.Side == "BUY" {
			return exactAdd(notional, fill.FeeAmount)
		}
		if fill.FeeAmount > notional {
			return 0, false
		}
		return notional - fill.FeeAmount, true
	}
	return notional, true
}

func (r *CDFLiquidityRunAudit) validateCDFReference(event Event, decision cdfDecisionEvidence, state *CDFLiquiditySupplierAudit) {
	if state.configuredReferencePrice <= 0 || state.configuredReferenceHalfLife <= 0 {
		return
	}
	if !state.referenceInitialized {
		state.reconstructedReference = state.configuredReferencePrice
		state.referenceInitialized = true
	}

	if decision.MarkPrice > 0 {
		observationUsable := decision.ObservationTime > 0 && decision.ObservationSequence > 0 && decision.ObservationAge >= 0 && decision.ObservationAge <= state.configuredMaxObservationAge
		markActionAllowed := decision.Action == "submit" || decision.Action == "rest" || decision.Action == "cancel" || (decision.Reason == "inventory_at_target" && (decision.Action == "wait" || decision.Action == "withdraw"))
		midpoint := int64(0)
		if decision.BestBid > 0 && decision.BestAsk > 0 && decision.BestBid < decision.BestAsk {
			midpoint = etypes.Midpoint(decision.BestBid, decision.BestAsk)
		}
		if !markActionAllowed || !observationUsable || midpoint <= 0 || decision.MarkPrice != midpoint {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier mark does not match its usable local midpoint"})
		} else {
			if !state.referenceMarkSeen {
				state.referenceLastValidMarkAt = decision.DecisionTime
				state.referenceMarkSeen = true
			} else {
				elapsed := decision.DecisionTime - state.referenceLastValidMarkAt
				if elapsed > 0 {
					revised, ok := advanceCDFReference(state.reconstructedReference, midpoint, elapsed, state.configuredReferenceHalfLife)
					if !ok {
						r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier reference update is outside finite numeric bounds"})
					} else {
						state.reconstructedReference = revised
					}
				}
				state.referenceLastValidMarkAt = decision.DecisionTime
			}
		}
	}
	if decision.ReferencePrice != state.reconstructedReference {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier reference price does not match registered observation update rule"})
	}
}

func (r *CDFLiquidityRunAudit) validatePendingCancelWaits(orders map[cdfOrderKey]*cdfOrderState, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for _, wait := range r.pendingCancelWaits {
		order := orders[wait.key]
		state := states[cdfParticipantKey{VenueID: wait.key.VenueID, ClientID: wait.key.ClientID}]
		role := ""
		if state != nil {
			role = state.Role
		}
		cancel, cancelExists := r.cancelDecisionByOrder[wait.key]
		if order == nil || order.acceptedAt <= 0 || order.acceptedAt > wait.decisionAt || order.closedAt > 0 && order.closedAt <= wait.decisionAt || !order.cancelRequested || !cancelExists || cancel.requestID != wait.cancelRequestID || cancel.decisionAt > wait.decisionAt || cancel.decisionAt < order.acceptedAt {
			r.addCheck(CDFLiquidityCheck{VenueID: wait.key.VenueID, Role: role, ClientID: wait.key.ClientID, Ordinal: wait.ordinal, Failure: "cancel-pending wait has no matching live cancellation"})
		}
	}
}

func (r *CDFLiquidityRunAudit) validatePendingOrderWaits(states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for _, wait := range r.pendingOrderWaits {
		request, requestExists := r.quoteRequests[cdfRequestKey{venueID: wait.venueID, clientID: wait.clientID, requestID: wait.requestID}]
		if !requestExists {
			continue
		}
		if request.acceptedAt > 0 && request.acceptedAt <= wait.decisionAt || request.rejectedAt > 0 && request.rejectedAt <= wait.decisionAt {
			state := states[cdfParticipantKey{VenueID: wait.venueID, ClientID: wait.clientID}]
			role := ""
			if state != nil {
				role = state.Role
			}
			failure := "order-pending wait follows an already accepted order"
			if request.rejectedAt > 0 && request.rejectedAt <= wait.decisionAt && (request.acceptedAt == 0 || request.rejectedAt < request.acceptedAt) {
				failure = "order-pending wait follows an already rejected order"
			}
			r.addCheck(CDFLiquidityCheck{VenueID: wait.venueID, Role: role, ClientID: wait.clientID, Ordinal: wait.ordinal, Failure: failure})
		}
	}
}

func (r *CDFLiquidityRunAudit) validateDecisionReceipt(event Event, decision cdfDecisionEvidence, evidence *cdfMarketDataEvidence) {
	if evidence == nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision has no receipt evidence"})
		return
	}
	requestID := decision.QuoteRequestID
	wantRequestType := uint8(2)
	if decision.Action == "cancel" || decision.Action == "withdraw" {
		requestID = decision.CancelRequestID
		wantRequestType = 3
	}
	if decision.Action != "submit" && decision.Action != "cancel" && decision.Action != "withdraw" {
		return
	}
	var action cdfMarketDataActionRecord
	actionExists := false
	for key, candidate := range evidence.actions {
		if key.ClientID != decision.ClientID || key.RequestID != requestID {
			continue
		}
		link := evidence.links[candidate.LinkID]
		if link.SourceVenue == event.VenueID && link.Role == auditRoleClass(decision.Role) {
			action, actionExists = candidate, true
			if candidate.RequestType == wantRequestType {
				break
			}
		}
	}
	localDigest, digestErr := decodeFrontierDigest(decision.ObservationDigest)
	actionFrontierMatches := actionExists && action.LinkID == decision.ObservationLinkID && action.FrontierOrdinal == decision.ObservationOrdinal && action.FrontierDelivered == decision.ObservationDeliveredAt && digestErr == nil && action.FrontierDigest == localDigest
	symbol, symbolExists := evidence.symbols[action.SymbolID]
	symbolMatches := decision.Action != "submit" && action.SymbolID == 0 || symbolExists && symbol.Symbol == decision.Symbol
	orderMatches := decision.Action != "submit" || action.Price == decision.QuotePrice && action.Qty == decision.QuoteQty
	orderContractMatches := true
	if decision.Action == "submit" {
		expectedSide, sideOK := cdfSideCode(decision.Side)
		orderContractMatches = sideOK && action.Side == expectedSide && action.OrderType == uint8(etypes.LimitOrder) && action.TimeInForce == uint8(etypes.GTC) && action.PostOnly
	}
	if decision.Action != "submit" {
		orderMatches = action.OrderID == decision.QuoteOrderID
	}
	if !actionExists || action.RequestType != wantRequestType || action.ClientID != decision.ClientID || action.DecisionAt != decision.DecisionTime || !actionFrontierMatches || !symbolMatches || !orderMatches || !orderContractMatches {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier action is not reconciled to a market-data gateway boundary"})
	}
	if decision.Action != "submit" {
		return
	}
	var record cdfMarketDataDecisionRecord
	exists := false
	for key, candidate := range evidence.decisions {
		if key.ClientID != decision.ClientID || key.RequestID != decision.QuoteRequestID {
			continue
		}
		link := evidence.links[candidate.LinkID]
		if link.SourceVenue == event.VenueID && link.Role == auditRoleClass(decision.Role) {
			record, exists = candidate, true
			break
		}
	}
	localDigest, digestErr = decodeFrontierDigest(decision.ObservationDigest)
	decisionFrontierMatches := exists && record.LinkID == decision.ObservationLinkID && record.FrontierOrdinal == decision.ObservationOrdinal && record.FrontierDelivered == decision.ObservationDeliveredAt && digestErr == nil && record.FrontierDigest == localDigest
	expectedSide, sideOK := cdfSideCode(decision.Side)
	decisionContractMatches := sideOK && record.Side == expectedSide && record.OrderType == uint8(etypes.LimitOrder) && record.TimeInForce == uint8(etypes.GTC) && record.PostOnly
	if !exists || record.DecisionAt != decision.DecisionTime || record.Price != decision.QuotePrice || record.Qty != decision.QuoteQty || !decisionFrontierMatches || !decisionContractMatches {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier submit is not reconciled to a market-data decision receipt"})
	}
}

func cdfSideCode(side string) (uint8, bool) {
	switch side {
	case "BUY":
		return uint8(etypes.Buy), true
	case "SELL":
		return uint8(etypes.Sell), true
	default:
		return 0, false
	}
}

func (r *CDFLiquidityRunAudit) validateReceiptActions(evidence *cdfMarketDataEvidence) {
	if evidence == nil {
		return
	}
	for key, action := range evidence.actions {
		link := evidence.links[action.LinkID]
		if link.Role != auditRoleClass("cdf_elastic_supplier_1") {
			continue
		}
		if action.RequestType != 2 && action.RequestType != 3 {
			continue
		}
		identity := cdfActionKey{clientID: key.ClientID, linkID: action.LinkID, requestID: key.RequestID, requestType: action.RequestType, orderID: action.OrderID}
		if _, expected := r.expectedActionKeys[identity]; !expected {
			r.addCheck(CDFLiquidityCheck{Role: link.Role, ClientID: key.ClientID, Failure: "market-data gateway action has no matching supplier decision"})
		}
	}
}

func (r *CDFLiquidityRunAudit) validateDecisionObservation(event Event, decision cdfDecisionEvidence, emptyObservationAllowed bool, evidence *cdfMarketDataEvidence) {
	if evidence == nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision has no receipt evidence"})
		return
	}
	link, linkExists := evidence.links[decision.ObservationLinkID]
	if !linkExists || link.SourceVenue != event.VenueID || link.Role != auditRoleClass(decision.Role) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision cites an unknown or unrelated observation link"})
		return
	}
	if emptyObservationAllowed {
		if decision.ObservationLinkID == 0 || decision.ObservationOrdinal != 0 || decision.ObservationDeliveredAt != 0 || decision.ObservationFingerprint != "" || decision.ObservationDigest != "" {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "initial subscription wait has a nonempty observation frontier"})
		}
		return
	}
	if decision.ObservationLinkID == 0 || decision.ObservationOrdinal == 0 || decision.ObservationDeliveredAt <= 0 || decision.ObservationFingerprint == "" || decision.ObservationDigest == "" {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision has an incomplete observation frontier"})
		return
	}
	fingerprintRaw, err := hex.DecodeString(decision.ObservationFingerprint)
	if err != nil || len(fingerprintRaw) != 16 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision has an invalid observation fingerprint"})
		return
	}
	var fingerprint [16]byte
	copy(fingerprint[:], fingerprintRaw)
	digest, digestErr := decodeFrontierDigest(decision.ObservationDigest)
	receipt, receiptExists := evidence.receipts[cdfReceiptLinkOrdinal{LinkID: decision.ObservationLinkID, LinkOrdinal: decision.ObservationOrdinal}]
	latestReceipt, latestReceiptExists := latestCDFReceiptBefore(evidence.receipts, decision.ObservationLinkID, decision.DecisionTime)
	symbol, symbolExists := evidence.symbols[receipt.SymbolID]
	if digestErr != nil || !receiptExists || !latestReceiptExists || latestReceipt.LinkOrdinal != decision.ObservationOrdinal || !symbolExists || receipt.ClientID != decision.ClientID || symbol.Symbol != decision.Symbol || receipt.Sequence != decision.ObservationSequence || receipt.PublishedAt != decision.ObservationTime || receipt.DeliveredAt != decision.ObservationDeliveredAt || receipt.DeliveredAt > decision.DecisionTime || receipt.Fingerprint != fingerprint || receipt.Digest != digest {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: decision.ClientID, Ordinal: event.Ordinal, Failure: "supplier decision frontier does not match its delayed local observation"})
	}
}

func (r *CDFLiquidityRunAudit) processSupplierFill(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit, observed map[cdfFillKey]cdfObservedFill) {
	var fill cdfFillEvidence
	required := []string{"role", "client_id", "symbol", "order_id", "trade_id", "timestamp", "side", "price", "qty", "fee_amount", "fee_asset", "is_full", "position_before", "position_after"}
	if err := decodeRequiredJSON(event.Raw(), &fill, required...); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier fill: " + err.Error()})
		return
	}
	key := cdfParticipantKey{VenueID: event.VenueID, ClientID: fill.ClientID}
	state := states[key]
	if state == nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "fill from unregistered supplier"})
		return
	}
	expectedFee, feeOK := expectedCDFMakerFee(fill.Price, fill.Qty, state.configuredBasePrecision, state.configuredMakerFeeBps)
	if !feeOK || fill.FeeAmount != expectedFee || (expectedFee > 0 && fill.FeeAsset != state.configuredQuoteAsset) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill fee does not match registered maker-fee schedule"})
	}
	r.FillCount++
	state.FillCount++
	if event.ClientID != fill.ClientID || fill.Role != state.Role || fill.Symbol != state.configuredSymbol || fill.Timestamp != event.SimTS || fill.OrderID == 0 || fill.Qty <= 0 || !validSide(fill.Side) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill identity or bounds mismatch"})
	}
	if !state.positionSet {
		state.positionSet, state.lastPosition = true, fill.PositionBefore
	}
	if state.lastPosition != fill.PositionBefore {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill position-before mismatch"})
	}
	expectedAfter := fill.PositionBefore
	if fill.Side == "BUY" {
		expectedAfter, _ = exactAdd(expectedAfter, fill.Qty)
		state.BuyQty, _ = exactAdd(state.BuyQty, fill.Qty)
	} else {
		expectedAfter, _ = exactAdd(expectedAfter, -fill.Qty)
		state.SellQty, _ = exactAdd(state.SellQty, fill.Qty)
	}
	if expectedAfter != fill.PositionAfter || (state.InventoryLimit > 0 && (fill.PositionAfter < -state.InventoryLimit || fill.PositionAfter > state.InventoryLimit)) {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill position-after violates inventory transition"})
	} else if fill.PositionAfter != fill.PositionBefore {
		state.inventoryChangedSinceActionable = true
	}
	if err := updateSupplierPnL(state, fill); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier PnL reconstruction failed: " + err.Error()})
	}
	if err := updateSupplierBalanceDeltas(state, fill); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "supplier cash/base/fee reconciliation failed: " + err.Error()})
	}
	state.lastPosition = fill.PositionAfter
	if fill.PositionAfter < state.MinPosition {
		state.MinPosition = fill.PositionAfter
	}
	if fill.PositionAfter > state.MaxPosition {
		state.MaxPosition = fill.PositionAfter
	}
	state.FilledQty, _ = exactAdd(state.FilledQty, fill.Qty)
	fillKey := cdfFillKey{VenueID: event.VenueID, ClientID: fill.ClientID, OrderID: fill.OrderID, TradeID: fill.TradeID}
	if _, exists := observed[fillKey]; exists {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: fill.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier fill evidence"})
	} else {
		observed[fillKey] = cdfObservedFill{fill: fill, ordinal: event.Ordinal}
	}
}

func updateSupplierBalanceDeltas(state *CDFLiquiditySupplierAudit, fill cdfFillEvidence) error {
	if state.configuredBaseAsset == "" || state.configuredQuoteAsset == "" || state.configuredBasePrecision <= 0 {
		return fmt.Errorf("missing configured asset pair or base precision")
	}
	quoteAmount, ok := quoteProduct(fill.Price, fill.Qty, state.configuredBasePrecision)
	if !ok {
		return fmt.Errorf("quote notional overflows fixed-point range")
	}
	baseDelta, quoteDelta := int64(0), int64(0)
	if fill.Side == "BUY" {
		baseDelta, quoteDelta = fill.Qty, -quoteAmount
	} else if fill.Side == "SELL" {
		baseDelta, quoteDelta = -fill.Qty, quoteAmount
	} else {
		return fmt.Errorf("unknown fill side %q", fill.Side)
	}
	if err := addBalanceDelta(state.fillNetBalanceDeltas, state.configuredBaseAsset, baseDelta); err != nil {
		return err
	}
	if err := addBalanceDelta(state.fillNetBalanceDeltas, state.configuredQuoteAsset, quoteDelta); err != nil {
		return err
	}
	if fill.FeeAmount < 0 {
		return fmt.Errorf("fee amount is negative")
	}
	if fill.FeeAmount == 0 {
		return nil
	}
	if fill.FeeAsset != state.configuredBaseAsset && fill.FeeAsset != state.configuredQuoteAsset {
		return fmt.Errorf("fee asset %q is outside configured pair", fill.FeeAsset)
	}
	return addBalanceDelta(state.fillNetBalanceDeltas, fill.FeeAsset, -fill.FeeAmount)
}

func addBalanceDelta(balances map[string]int64, asset string, delta int64) error {
	updated, ok := exactAdd(balances[asset], delta)
	if !ok {
		return fmt.Errorf("balance delta for %s overflows", asset)
	}
	balances[asset] = updated
	return nil
}

func updateSupplierPnL(state *CDFLiquiditySupplierAudit, fill cdfFillEvidence) error {
	if state.configuredBasePrecision <= 0 {
		return fmt.Errorf("missing positive base precision")
	}
	if fill.Price <= 0 || fill.Qty <= 0 {
		return fmt.Errorf("fill price and quantity must be positive")
	}
	positionBefore := fill.PositionBefore
	quantity := fill.Qty
	if positionBefore == 0 {
		state.entryPrice = fill.Price
	} else if (positionBefore > 0 && fill.Side == "BUY") || (positionBefore < 0 && fill.Side == "SELL") {
		oldQuantity := absInt64(positionBefore)
		combinedQuantity, ok := exactAdd(oldQuantity, quantity)
		if !ok {
			return fmt.Errorf("position quantity overflow")
		}
		weighted := new(big.Int).Mul(big.NewInt(oldQuantity), big.NewInt(state.entryPrice))
		weighted.Add(weighted, new(big.Int).Mul(big.NewInt(quantity), big.NewInt(fill.Price)))
		weighted.Quo(weighted, big.NewInt(combinedQuantity))
		if !weighted.IsInt64() {
			return fmt.Errorf("entry price overflow")
		}
		state.entryPrice = weighted.Int64()
	} else {
		closeQuantity := minInt64(absInt64(positionBefore), quantity)
		priceDifference := fill.Price - state.entryPrice
		if positionBefore < 0 {
			priceDifference = -priceDifference
		}
		pnl, ok := quoteProduct(priceDifference, closeQuantity, state.configuredBasePrecision)
		if !ok {
			return fmt.Errorf("realized PnL overflow")
		}
		state.realizedPnL, ok = exactAdd(state.realizedPnL, pnl)
		if !ok {
			return fmt.Errorf("realized PnL accumulation overflow")
		}
		positionQuantity := absInt64(positionBefore)
		if quantity > positionQuantity {
			state.entryPrice = fill.Price
		} else if quantity == positionQuantity {
			state.entryPrice = 0
		}
	}
	if fill.FeeAsset == state.configuredQuoteAsset && fill.FeeAmount > 0 {
		var ok bool
		state.realizedPnL, ok = exactAdd(state.realizedPnL, -fill.FeeAmount)
		if !ok {
			return fmt.Errorf("fee accumulation overflow")
		}
	}
	return nil
}

func quoteProduct(priceDifference, quantity, basePrecision int64) (int64, bool) {
	if basePrecision <= 0 || quantity < 0 {
		return 0, false
	}
	product := new(big.Int).Mul(big.NewInt(priceDifference), big.NewInt(quantity))
	product.Quo(product, big.NewInt(basePrecision))
	if !product.IsInt64() {
		return 0, false
	}
	return product.Int64(), true
}

func expectedCDFMakerFee(price, quantity, basePrecision, makerFeeBps int64) (int64, bool) {
	if price <= 0 || quantity <= 0 || basePrecision <= 0 || makerFeeBps < 0 {
		return 0, false
	}
	notional, ok := etypes.TryMulDiv(price, quantity, basePrecision)
	if !ok {
		return 0, false
	}
	return etypes.TryMulBps(notional, makerFeeBps)
}

func expectedCDFQuoteRequirement(price, quantity, basePrecision, makerFeeBps int64) (int64, bool) {
	notional, ok := quoteProduct(price, quantity, basePrecision)
	if !ok {
		return 0, false
	}
	fee, ok := expectedCDFMakerFee(price, quantity, basePrecision, makerFeeBps)
	if !ok {
		return 0, false
	}
	return exactAdd(notional, fee)
}

func signedQuoteProduct(priceDifference, signedQuantity, basePrecision int64) (int64, bool) {
	if basePrecision <= 0 {
		return 0, false
	}
	product := new(big.Int).Mul(big.NewInt(priceDifference), big.NewInt(signedQuantity))
	product.Quo(product, big.NewInt(basePrecision))
	if !product.IsInt64() {
		return 0, false
	}
	return product.Int64(), true
}

func absInt64(value int64) int64 {
	if value == math.MinInt64 {
		return math.MaxInt64
	}
	if value < 0 {
		return -value
	}
	return value
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func (r *CDFLiquidityRunAudit) processBookEvent(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit, orders map[cdfOrderKey]*cdfOrderState, actual map[cdfFillKey]cdfOrderFillEvidence, venueAudits map[string]*CDFLiquidityVenueAudit) {
	switch event.Name {
	case "Trade":
		var trade cdfTradeEvidence
		if err := decodeRequiredJSON(event.Raw(), &trade, "trade_id", "price", "qty", "side", "taker_order_id"); err != nil || trade.Qty <= 0 || trade.Price <= 0 || !validSide(trade.Side) || trade.TakerOrderID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed CDF trade evidence"})
			return
		}
		r.TotalTradeCount++
		venue := venueAudits[event.VenueID]
		if venue == nil {
			venue = &CDFLiquidityVenueAudit{VenueID: event.VenueID, ExpectedHistoricalCount: r.expectedHistoricalCountPerVenue}
			venueAudits[event.VenueID] = venue
		}
		var ok bool
		r.TotalTradeVolumeQty, ok = exactAdd(r.TotalTradeVolumeQty, trade.Qty)
		if !ok {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF trade volume overflow"})
		}
		venue.TotalTradeVolumeQty, ok = exactAdd(venue.TotalTradeVolumeQty, trade.Qty)
		if !ok {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "venue CDF trade volume overflow"})
		}
	case "BookSnapshot":
		if event.ClientID != 0 {
			return
		}
		var snapshot cdfSnapshotEvidence
		if err := decodeRequiredJSON(event.Raw(), &snapshot, "bids", "asks"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed CDF book snapshot: " + err.Error()})
			return
		}
		r.SnapshotCount++
		venue := venueAudits[event.VenueID]
		if venue == nil {
			venue = &CDFLiquidityVenueAudit{VenueID: event.VenueID, ExpectedHistoricalCount: r.ExpectedHistoricalCount, MinimumExecutableQty: r.MinimumExecutableQty}
			venueAudits[event.VenueID] = venue
		}
		venue.SnapshotCount++
		if len(snapshot.Bids) == 0 {
			r.BidAbsentSnapshots++
			venue.BidAbsentSnapshots++
		}
		if len(snapshot.Asks) == 0 {
			r.AskAbsentSnapshots++
			venue.AskAbsentSnapshots++
		}
		if len(snapshot.Bids) == 0 && len(snapshot.Asks) == 0 {
			r.BothAbsentSnapshots++
		}
		bidDepth := displayedDepth(snapshot.Bids)
		askDepth := displayedDepth(snapshot.Asks)
		bidQualified := r.MinimumExecutableQty <= 0 || bidDepth >= r.MinimumExecutableQty
		askQualified := r.MinimumExecutableQty <= 0 || askDepth >= r.MinimumExecutableQty
		if !bidQualified {
			r.QualifiedBidAbsentSnapshots++
			venue.QualifiedBidAbsentSnapshots++
		}
		if !askQualified {
			r.QualifiedAskAbsentSnapshots++
			venue.QualifiedAskAbsentSnapshots++
		}
		if !bidQualified && !askQualified {
			r.QualifiedBothAbsentSnapshots++
		}
		totalDisplayed := displayedDepth(snapshot.Bids) + displayedDepth(snapshot.Asks)
		supplierBidDisplayed, supplierAskDisplayed, supplierDepthOK := supplierDisplayedDepthBySide(event.VenueID, orders)
		supplierDisplayed, supplierTotalOK := exactAdd(supplierBidDisplayed, supplierAskDisplayed)
		if !supplierDepthOK || !supplierTotalOK {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "supplier displayed depth cannot be reconstructed safely"})
			r.recordInvalidSupplierRemovalSnapshot(venue)
		} else if supplierBidDisplayed > bidDepth || supplierAskDisplayed > askDepth {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "supplier displayed depth exceeds the corresponding aggregate book side"})
			r.recordInvalidSupplierRemovalSnapshot(venue)
		} else {
			r.recordSupplierRemovalSnapshot(venue, bidDepth-supplierBidDisplayed, askDepth-supplierAskDisplayed)
		}
		if previousAt, seen := r.lastDepthSnapshotAt[event.VenueID]; seen {
			if event.SimTS < previousAt {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF book snapshots are out of timestamp order"})
				return
			}
			if event.SimTS > previousAt {
				r.accumulateDepthInterval(event.VenueID, previousAt, event.SimTS, states)
			}
		}
		if totalDisplayed > 0 {
			venue.ActiveDepthSnapshotCount++
			share := float64(supplierDisplayed) / float64(totalDisplayed)
			if share > venue.MaxSupplierDepthShare {
				venue.MaxSupplierDepthShare = share
			}
			if share > 0.75 {
				venue.SupplierDepthOver75Count++
			}
		}
		r.lastDepthSnapshotAt[event.VenueID] = event.SimTS
		r.lastDepthTotal[event.VenueID] = totalDisplayed
		r.lastSupplierDepthByClient[event.VenueID] = supplierDisplayedDepthByClient(event.VenueID, orders)
	case "OrderAccepted":
		key := cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}
		state := states[key]
		if state == nil {
			return
		}
		var accepted cdfAcceptedEvidence
		if err := decodeRequiredJSON(event.Raw(), &accepted, "order_id", "client_id", "request_id", "side", "type", "time_in_force", "post_only", "price", "qty"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier order acceptance: " + err.Error()})
			return
		}
		if accepted.ClientID != event.ClientID || accepted.OrderID == 0 || accepted.Qty <= 0 || !validSide(accepted.Side) || accepted.Type != "LIMIT" || accepted.TimeInForce != "GTC" || !accepted.PostOnly {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier order acceptance violates passive bounded quote contract"})
			return
		}
		if state.configuredMaxQuoteQty <= 0 || accepted.Qty > state.configuredMaxQuoteQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "accepted quote exceeds registered maximum quantity"})
		}
		requested, requestedOK := state.pendingQuoteByRequest[accepted.RequestID]
		if !requestedOK {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "accepted supplier order has no matching local decision"})
		} else {
			if requested.Side != accepted.Side || requested.QuotePrice != accepted.Price || requested.QuoteQty != accepted.Qty {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "accepted quote disagrees with local decision"})
			}
			delete(state.pendingQuoteByRequest, accepted.RequestID)
		}
		orderKey := cdfOrderKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: accepted.OrderID}
		requestKey := cdfRequestKey{venueID: event.VenueID, clientID: event.ClientID, requestID: accepted.RequestID}
		request, requestExists := r.quoteRequests[requestKey]
		if !requestExists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "accepted supplier order has no causal submit decision"})
		} else {
			if event.SimTS < request.decisionAt {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier order was accepted before its submit decision"})
			}
			request.acceptedAt, request.acceptedOrdinal = event.SimTS, event.Ordinal
			r.quoteRequests[requestKey] = request
		}
		if _, exists := orders[orderKey]; exists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier order acceptance"})
			return
		}
		for existingKey, existingOrder := range orders {
			if existingKey.VenueID == event.VenueID && existingKey.ClientID == event.ClientID && !existingOrder.closed {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier has more than one live accepted order"})
			}
		}
		_, cancelRequested := r.cancelRequestedByOrder[orderKey]
		order := &cdfOrderState{clientID: event.ClientID, side: accepted.Side, price: accepted.Price, requestID: accepted.RequestID, acceptedAt: event.SimTS, acceptedSequence: event.Sequence, acceptedQty: accepted.Qty, remainingQty: accepted.Qty, cancelRequested: cancelRequested, remainingUpdates: []cdfOrderRemainingUpdate{{ordinal: event.Ordinal, remainingQty: accepted.Qty}}}
		if share, ok := state.pendingTouchByRequest[accepted.RequestID]; ok {
			order.touchShare, order.touchShareKnown = share, true
			delete(state.pendingTouchByRequest, accepted.RequestID)
		}
		orders[orderKey] = order
	case "OrderRejected":
		state := states[cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}]
		if state == nil {
			return
		}
		var rejected struct {
			RequestID uint64 `json:"request_id"`
		}
		if err := decodeRequiredJSON(event.Raw(), &rejected, "request_id"); err != nil || rejected.RequestID == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier order rejection"})
			return
		}
		requestKey := cdfRequestKey{venueID: event.VenueID, clientID: event.ClientID, requestID: rejected.RequestID}
		request, requestExists := r.quoteRequests[requestKey]
		if _, exists := state.pendingQuoteByRequest[rejected.RequestID]; !exists && !requestExists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier order rejection has no matching local submission"})
		} else if requestExists {
			if event.SimTS < request.decisionAt {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier order was rejected before its submit decision"})
			}
			request.rejectedAt, request.rejectedOrdinal = event.SimTS, event.Ordinal
			r.quoteRequests[requestKey] = request
		}
		delete(state.pendingQuoteByRequest, rejected.RequestID)
		delete(state.pendingTouchByRequest, rejected.RequestID)
	case "OrderFill":
		key := cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}
		state := states[key]
		if state == nil {
			return
		}
		var fill cdfOrderFillEvidence
		if err := decodeRequiredJSON(event.Raw(), &fill, "order_id", "trade_id", "side", "price", "qty", "filled_qty", "remaining_qty", "is_full"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier order fill: " + err.Error()})
			return
		}
		orderKey := cdfOrderKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: fill.OrderID}
		order := orders[orderKey]
		if order == nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill has no accepted order"})
			return
		}
		if order.closed || fill.Qty <= 0 || fill.FilledQty <= 0 || fill.RemainingQty < 0 || (fill.IsFull != (fill.RemainingQty == 0)) {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "invalid supplier order fill lifecycle"})
			return
		}
		expectedFilledTotal, ok := exactAdd(order.filledQty, fill.Qty)
		if !ok || expectedFilledTotal != fill.FilledQty || fill.FilledQty+fill.RemainingQty != order.acceptedQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier fill quantity does not reconcile to accepted order"})
		}
		order.filledQty, order.remainingQty = fill.FilledQty, fill.RemainingQty
		fillKey := cdfFillKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: fill.OrderID, TradeID: fill.TradeID}
		if _, exists := actual[fillKey]; exists {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "duplicate supplier order fill"})
		} else {
			actual[fillKey] = fill
		}
		if fill.IsFull {
			order.closed, order.closedAt, order.filled = true, event.SimTS, true
			order.filledAt, order.filledOrdinal = event.SimTS, event.Ordinal
		}
		order.remainingUpdates = append(order.remainingUpdates, cdfOrderRemainingUpdate{ordinal: event.Ordinal, remainingQty: order.remainingQty, closed: order.closed})
	case "OrderCancelled":
		state := states[cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}]
		if state == nil {
			return
		}
		var cancelled cdfCancelledEvidence
		if err := decodeRequiredJSON(event.Raw(), &cancelled, "order_id", "request_id", "remaining_qty"); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier cancellation: " + err.Error()})
			return
		}
		key := cdfOrderKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: cancelled.OrderID}
		order := orders[key]
		if order == nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation has no accepted order"})
			return
		}
		cancel, cancelExists := r.cancelDecisionByOrder[key]
		if !cancelExists || cancelled.RequestID != cancel.requestID || event.SimTS < cancel.decisionAt {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation has no matching local request"})
		}
		if order.closed || cancelled.RemainingQty != order.remainingQty {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation does not reconcile order state"})
			return
		}
		order.closed, order.closedAt = true, event.SimTS
		order.cancelled, order.cancelRequestID, order.cancelledSequence = true, cancelled.RequestID, event.Sequence
		order.remainingUpdates = append(order.remainingUpdates, cdfOrderRemainingUpdate{ordinal: event.Ordinal, remainingQty: order.remainingQty, closed: true})
	case "OrderCancelRejected":
		state := states[cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}]
		if state == nil {
			return
		}
		var rejected struct {
			RequestID uint64 `json:"request_id"`
			Success   bool   `json:"success"`
			Error     string `json:"error"`
		}
		if err := decodeRequiredJSON(event.Raw(), &rejected, "request_id", "success", "error"); err != nil || rejected.RequestID == 0 || rejected.Success || rejected.Error == "" {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier cancellation rejection"})
			return
		}
		matched := false
		for key, requestID := range r.cancelRequestByOrder {
			if key.VenueID != event.VenueID || key.ClientID != event.ClientID || requestID != rejected.RequestID {
				continue
			}
			order := orders[key]
			if order == nil {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation rejection has no matching order"})
				matched = true
				continue
			}
			order.cancelRejected, order.cancelRejectedRequestID = true, rejected.RequestID
			order.cancelRejectedAt, order.cancelRejectedOrdinal, order.cancelRejectedReason = event.SimTS, event.Ordinal, rejected.Error
			cancel, cancelExists := r.cancelDecisionByOrder[key]
			if !cancelExists || event.SimTS < cancel.decisionAt {
				r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation rejection has no matching local request"})
			}
			matched = true
		}
		if !matched {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier cancellation rejection has no matching request"})
		}
	}
}

// validateStaleWithdrawals closes the evidence loop across the separate
// decision and book logs. A reason/age pair is not enough: the order named by
// a stale withdrawal must have been accepted, still live at the withdrawal,
// and later closed by the exact cancellation request emitted at that boundary.
func (r *CDFLiquidityRunAudit) validateStaleWithdrawals(orders map[cdfOrderKey]*cdfOrderState, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for key, withdrawal := range r.staleWithdrawals {
		order := orders[key]
		state := states[cdfParticipantKey{VenueID: key.VenueID, ClientID: key.ClientID}]
		role := ""
		if state != nil {
			role = state.Role
		}
		addFailure := func(failure string) {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: role, ClientID: key.ClientID, Ordinal: withdrawal.ordinal, Failure: failure})
		}
		if order == nil {
			addFailure("stale withdrawal has no matching accepted supplier order")
			continue
		}
		if order.acceptedAt <= 0 || order.acceptedAt >= withdrawal.decisionAt {
			addFailure("stale withdrawal does not follow an accepted live supplier order")
		}
		if order.closedAt > 0 && order.closedAt <= withdrawal.decisionAt {
			addFailure("stale withdrawal references a supplier order already closed before the decision")
		}
		matchingCancellation := order.cancelled && order.cancelRequestID == withdrawal.cancelRequestID && order.closedAt > withdrawal.decisionAt
		matchingFillRace := order.filled && order.closedAt > withdrawal.decisionAt && order.cancelRejected && order.cancelRejectedRequestID == withdrawal.cancelRequestID && order.cancelRejectedReason == "ORDER_ALREADY_FILLED" && order.cancelRejectedAt > withdrawal.decisionAt && (order.cancelRejectedAt > order.filledAt || order.cancelRejectedAt == order.filledAt && order.cancelRejectedOrdinal > order.filledOrdinal)
		if !matchingCancellation && !matchingFillRace {
			addFailure("stale withdrawal has no later matching exchange cancellation outcome")
		}
	}
}

// measureWithdrawalsWithoutReplacement distinguishes a genuine liquidity
// withdrawal from an ordinary cancel/requote. A withdrawal qualifies only when
// the supplier remains without a later submit for one complete registered
// decision interval after the exchange confirms removal. Withdrawals too close
// to the terminal boundary are censored instead of being treated as evidence
// of persistent withdrawal.
func (r *CDFLiquidityRunAudit) measureWithdrawalsWithoutReplacement(states map[cdfParticipantKey]*CDFLiquiditySupplierAudit, orders map[cdfOrderKey]*cdfOrderState) {
	if len(r.supplierActions) == 0 {
		return
	}
	sort.SliceStable(r.supplierActions, func(i, j int) bool {
		if r.supplierActions[i].ordinal != r.supplierActions[j].ordinal {
			return r.supplierActions[i].ordinal < r.supplierActions[j].ordinal
		}
		if r.supplierActions[i].key.VenueID != r.supplierActions[j].key.VenueID {
			return r.supplierActions[i].key.VenueID < r.supplierActions[j].key.VenueID
		}
		return r.supplierActions[i].key.ClientID < r.supplierActions[j].key.ClientID
	})
	submits := make(map[cdfParticipantKey][]cdfSupplierAction)
	for _, action := range r.supplierActions {
		if action.action == "submit" {
			submits[action.key] = append(submits[action.key], action)
		}
	}
	for key := range submits {
		sort.SliceStable(submits[key], func(i, j int) bool {
			left, right := submits[key][i], submits[key][j]
			if left.decisionAt != right.decisionAt {
				return left.decisionAt < right.decisionAt
			}
			if left.sequence != 0 && right.sequence != 0 && left.sequence != right.sequence {
				return left.sequence < right.sequence
			}
			return left.ordinal < right.ordinal
		})
	}
	for _, action := range r.supplierActions {
		if action.action != "withdraw" {
			continue
		}
		state := states[action.key]
		if state == nil || state.configuredIntervalNs <= 0 {
			continue
		}
		order := orders[cdfOrderKey{VenueID: action.key.VenueID, ClientID: action.key.ClientID, OrderID: action.orderID}]
		if order == nil || order.acceptedAt <= 0 || !cdfEvidenceAfter(action.decisionAt, action.sequence, order.acceptedAt, order.acceptedSequence) || !order.cancelled || order.cancelRequestID != action.cancelRequestID || !cdfEvidenceAfter(order.closedAt, order.cancelledSequence, action.decisionAt, action.sequence) {
			continue
		}
		deadline, ok := exactAdd(order.closedAt, state.configuredIntervalNs)
		if !ok {
			r.addCheck(CDFLiquidityCheck{VenueID: action.key.VenueID, Role: state.Role, ClientID: action.key.ClientID, Ordinal: action.ordinal, Failure: "withdrawal interval overflows fixed-point timestamp"})
			continue
		}
		if r.terminalAt < deadline {
			state.CensoredWithdrawalCount++
			r.CensoredWithdrawalCount++
			continue
		}
		replacementWithinInterval := false
		for _, candidate := range submits[action.key] {
			if !cdfEvidenceAfter(candidate.decisionAt, candidate.sequence, action.decisionAt, action.sequence) {
				continue
			}
			if candidate.decisionAt <= deadline {
				replacementWithinInterval = true
			}
			break
		}
		if !replacementWithinInterval {
			state.WithdrawalWithoutReplacementCount++
			r.WithdrawalWithoutReplacementCount++
		}
	}
}

// cdfEvidenceAfter orders lifecycle events from different routed files. A
// same-timestamp edge is accepted only when both records carry the
// venue-wide sequence; physical line ordinals are not comparable across
// files.
func cdfEvidenceAfter(laterAt int64, laterSequence uint64, earlierAt int64, earlierSequence uint64) bool {
	if laterAt != earlierAt {
		return laterAt > earlierAt
	}
	return laterSequence != 0 && earlierSequence != 0 && laterSequence > earlierSequence
}

func (r *CDFLiquidityRunAudit) validateRestDecisionQuantities(orders map[cdfOrderKey]*cdfOrderState, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for _, decision := range r.restDecisions {
		state := states[cdfParticipantKey{VenueID: decision.key.VenueID, ClientID: decision.key.ClientID}]
		role := decision.role
		if state != nil {
			role = state.Role
		}
		addFailure := func(failure string) {
			r.addCheck(CDFLiquidityCheck{VenueID: decision.key.VenueID, Role: role, ClientID: decision.key.ClientID, Ordinal: decision.ordinal, Failure: failure})
		}
		order := orders[decision.key]
		if order == nil {
			addFailure("rest decision has no matching accepted supplier order")
			continue
		}
		remaining, closed, found := order.remainingAt(decision.ordinal)
		if !found || closed || remaining <= 0 {
			addFailure("rest decision does not name a live exchange order")
			continue
		}
		if order.side != decision.side || order.price != decision.price || remaining != decision.quantity {
			addFailure("rest decision quantity does not match exchange remaining order state")
		}
	}
}

// accumulateDepthInterval applies the book state observed at the left endpoint
// to [start,end). This makes a snapshot at t authoritative from t until the
// next snapshot, including the interval ending in an empty book and the final
// interval ending at the run terminal time.
func (r *CDFLiquidityRunAudit) accumulateDepthInterval(venueID string, start, end int64, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	if end <= start {
		return
	}
	totalDisplayed := r.lastDepthTotal[venueID]
	if totalDisplayed <= 0 {
		return
	}
	weight := float64(end - start)
	denominator := float64(totalDisplayed) * weight
	supplierDepthByClient := r.lastSupplierDepthByClient[venueID]
	var supplierDisplayed int64
	for _, depth := range supplierDepthByClient {
		updated, ok := exactAdd(supplierDisplayed, depth)
		if !ok {
			return
		}
		supplierDisplayed = updated
	}
	r.supplierRestingDepthWeightedNumerator += float64(supplierDisplayed) * weight
	r.totalRestingDepthWeightedDenominator += denominator
	for key, state := range states {
		if key.VenueID != venueID {
			continue
		}
		state.restingDepthWeightedDenominator += denominator
		state.restingDepthWeightedNumerator += float64(supplierDepthByClient[key.ClientID]) * weight
	}
}

func (r *CDFLiquidityRunAudit) accumulateTerminalDepth(states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	if r.terminalAt <= 0 {
		return
	}
	for venueID, snapshotAt := range r.lastDepthSnapshotAt {
		if r.terminalAt < snapshotAt {
			r.addCheck(CDFLiquidityCheck{VenueID: venueID, Failure: "CDF terminal time precedes the last book snapshot"})
			continue
		}
		r.accumulateDepthInterval(venueID, snapshotAt, r.terminalAt, states)
	}
}

func (r *CDFLiquidityRunAudit) processBalanceSnapshot(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	state := states[cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}]
	if state == nil {
		return
	}
	var snapshot cdfBalanceSnapshotEvidence
	if err := decodeRequiredJSON(event.Raw(), &snapshot, "timestamp", "client_id", "spot_balances", "perp_balances", "borrowed"); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier balance snapshot: " + err.Error()})
		return
	}
	if snapshot.ClientID != event.ClientID || snapshot.Timestamp != event.SimTS || snapshot.Timestamp <= 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshot identity mismatch"})
	}
	if state.lastBalanceSnapshotAt > snapshot.Timestamp {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshots are out of timestamp order"})
	}
	_, spotBorrowed, spotGross, err := validateCDFBalanceRows(snapshot.SpotBalances)
	if err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier spot balance snapshot arithmetic is invalid: " + err.Error()})
		return
	}
	perpNet, perpBorrowed, _, err := validateCDFBalanceRows(snapshot.PerpBalances)
	if err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier perp balance snapshot arithmetic is invalid: " + err.Error()})
		return
	}
	for asset, net := range perpNet {
		if net != 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshot has unsupported nonzero perp balance for " + asset})
		}
	}
	aggregateBorrowed := make(map[string]int64, len(spotBorrowed)+len(perpBorrowed))
	for asset, borrowed := range spotBorrowed {
		if err := addBalanceDelta(aggregateBorrowed, asset, borrowed); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshot borrowed amount overflows"})
		}
	}
	for asset, borrowed := range perpBorrowed {
		if err := addBalanceDelta(aggregateBorrowed, asset, borrowed); err != nil {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshot borrowed amount overflows"})
		}
	}
	for asset, borrowed := range snapshot.Borrowed {
		if borrowed < 0 || aggregateBorrowed[asset] != borrowed {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshot borrowed totals do not reconcile"})
		}
		if borrowed > state.maxBorrowed {
			state.maxBorrowed = borrowed
		}
	}
	for asset, borrowed := range aggregateBorrowed {
		if snapshot.Borrowed[asset] != borrowed {
			r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier balance snapshot omits borrowed asset"})
		}
		if borrowed > state.maxBorrowed {
			state.maxBorrowed = borrowed
		}
	}
	if spotGross[state.configuredBaseAsset] > state.maxGrossBaseBalance {
		state.maxGrossBaseBalance = spotGross[state.configuredBaseAsset]
	}
	if spotGross[state.configuredQuoteAsset] > state.maxGrossQuoteBalance {
		state.maxGrossQuoteBalance = spotGross[state.configuredQuoteAsset]
	}
	state.balanceSnapshotCount++
	state.lastBalanceSnapshotAt = snapshot.Timestamp
}

func validateCDFBalanceRows(rows []cdfAssetBalanceEvidence) (map[string]int64, map[string]int64, map[string]int64, error) {
	netByAsset := make(map[string]int64, len(rows))
	borrowedByAsset := make(map[string]int64, len(rows))
	grossByAsset := make(map[string]int64, len(rows))
	for _, balance := range rows {
		if balance.Asset == "" {
			return nil, nil, nil, fmt.Errorf("asset is empty")
		}
		if _, exists := netByAsset[balance.Asset]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate asset %q", balance.Asset)
		}
		if balance.Borrowed < 0 {
			return nil, nil, nil, fmt.Errorf("borrowed amount for %s is negative", balance.Asset)
		}
		gross, ok := exactAdd(balance.Free, balance.Locked)
		if !ok {
			return nil, nil, nil, fmt.Errorf("gross amount for %s overflows", balance.Asset)
		}
		expectedNet, ok := exactAdd(gross, -balance.Borrowed)
		if !ok || expectedNet != balance.NetAsset {
			return nil, nil, nil, fmt.Errorf("net amount for %s does not equal free plus locked minus borrowed", balance.Asset)
		}
		netByAsset[balance.Asset] = balance.NetAsset
		borrowedByAsset[balance.Asset] = balance.Borrowed
		grossByAsset[balance.Asset] = gross
	}
	return netByAsset, borrowedByAsset, grossByAsset, nil
}

func (r *CDFLiquidityRunAudit) processBorrow(event Event, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	state := states[cdfParticipantKey{VenueID: event.VenueID, ClientID: event.ClientID}]
	if state == nil {
		return
	}
	var borrow cdfBorrowEvidence
	if err := decodeRequiredJSON(event.Raw(), &borrow, "client_id", "asset", "amount"); err != nil {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed supplier borrow evidence: " + err.Error()})
		return
	}
	if borrow.ClientID != event.ClientID || borrow.Amount <= 0 {
		r.addCheck(CDFLiquidityCheck{VenueID: event.VenueID, Role: state.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "supplier borrow evidence has invalid identity or amount"})
		return
	}
	state.borrowEventCount++
}

func (r *CDFLiquidityRunAudit) reconcileSupplierBalances(states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for key, state := range states {
		if state.balanceSnapshotCount == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no balance snapshot evidence"})
		}
		expected := make(map[string]int64, len(state.initialSpotNetBalances))
		for asset, balance := range state.initialSpotNetBalances {
			expected[asset] = balance
		}
		for asset, delta := range state.fillNetBalanceDeltas {
			if err := addBalanceDelta(expected, asset, delta); err != nil {
				r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "expected terminal balance overflows"})
			}
		}
		residual := int64(0)
		assets := make(map[string]struct{}, len(expected)+len(state.terminalSpotNetBalances))
		for asset := range expected {
			assets[asset] = struct{}{}
		}
		for asset := range state.terminalSpotNetBalances {
			assets[asset] = struct{}{}
		}
		for asset := range assets {
			expectedBalance, actualBalance := expected[asset], state.terminalSpotNetBalances[asset]
			if expectedBalance == actualBalance {
				continue
			}
			residual = addAbsoluteDifference(residual, expectedBalance, actualBalance)
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: fmt.Sprintf("terminal %s balance does not reconcile to fills", asset)})
		}
		state.BalanceReconciliationResidual = residual
		state.BalanceSnapshotCount = state.balanceSnapshotCount

		initialValue, initialErr := markedSpotWalletValue(state.initialSpotNetBalances, state.initialMarks, state)
		terminalValue, terminalErr := markedSpotWalletValue(state.terminalSpotNetBalances, state.terminalMarks, state)
		initialEndowmentTerminalValue, endowmentErr := markedSpotWalletValue(state.initialSpotNetBalances, state.terminalMarks, state)
		if initialErr != nil || terminalErr != nil || endowmentErr != nil {
			err := initialErr
			if err == nil {
				err = terminalErr
			}
			if err == nil {
				err = endowmentErr
			}
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier marked wallet reconciliation failed: " + err.Error()})
			continue
		}
		expectedPnL, ok := exactAdd(terminalValue, -initialValue)
		if !ok {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier marked wallet PnL overflows"})
			continue
		}
		residualPnL, ok := exactAdd(state.PnL, -expectedPnL)
		if !ok {
			residualPnL = math.MaxInt64
		}
		state.PnLReconciliationResidual = residualPnL
		if residualPnL != 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: fmt.Sprintf("account equity delta %d disagrees with marked wallet delta %d", state.PnL, expectedPnL)})
		}
		endowmentDelta, endowmentOK := exactAdd(initialEndowmentTerminalValue, -initialValue)
		tradingPnL, tradingOK := exactAdd(terminalValue, -initialEndowmentTerminalValue)
		if !endowmentOK || !tradingOK {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier endowment/trading PnL decomposition overflows"})
			continue
		}
		state.endowmentRevaluationPnL = endowmentDelta
		state.tradingPnL = tradingPnL
	}
}

func markedSpotWalletValue(balances map[string]int64, marks map[string]int64, state *CDFLiquiditySupplierAudit) (int64, error) {
	var total int64
	for asset, balance := range balances {
		if balance == 0 {
			continue
		}
		precision := int64(0)
		switch asset {
		case state.configuredBaseAsset:
			precision = state.configuredBasePrecision
		case state.configuredQuoteAsset:
			precision = state.configuredQuotePrecision
		default:
			return 0, fmt.Errorf("nonzero unconfigured asset %s", asset)
		}
		mark := marks[asset]
		if mark <= 0 || precision <= 0 {
			return 0, fmt.Errorf("asset %s lacks a positive mark and precision", asset)
		}
		value, ok := etypes.TryMulDiv(balance, mark, precision)
		if !ok {
			return 0, fmt.Errorf("asset %s value overflows", asset)
		}
		total, ok = exactAdd(total, value)
		if !ok {
			return 0, fmt.Errorf("wallet value overflows")
		}
	}
	return total, nil
}

func addAbsoluteDifference(total, expected, actual int64) int64 {
	difference := new(big.Int).Sub(big.NewInt(expected), big.NewInt(actual))
	difference.Abs(difference)
	if !difference.IsInt64() {
		return math.MaxInt64
	}
	updated, ok := exactAdd(total, difference.Int64())
	if !ok {
		return math.MaxInt64
	}
	return updated
}

func displayedDepth(levels []bookLevel) int64 {
	var total int64
	for _, level := range levels {
		if level.VisibleQty <= 0 {
			continue
		}
		var ok bool
		total, ok = exactAdd(total, level.VisibleQty)
		if !ok {
			return math.MaxInt64
		}
	}
	return total
}

func (r *CDFLiquidityRunAudit) recordInvalidSupplierRemovalSnapshot(venue *CDFLiquidityVenueAudit) {
	r.SupplierRemovalInvalidSnapshots++
	venue.SupplierRemovalInvalidSnapshots++
}

// recordSupplierRemovalSnapshot projects the same public aggregate book after
// removing every currently resting CDF order. It does not replay matching or
// alter the historical trajectory; it measures whether the retained snapshot
// still has executable depth without the intervention's displayed orders.
func (r *CDFLiquidityRunAudit) recordSupplierRemovalSnapshot(venue *CDFLiquidityVenueAudit, bidDepth, askDepth int64) {
	r.SupplierRemovalSnapshotCount++
	venue.SupplierRemovalSnapshotCount++
	if bidDepth <= 0 {
		r.SupplierRemovalBidAbsentSnapshots++
		venue.SupplierRemovalBidAbsentSnapshots++
	}
	if askDepth <= 0 {
		r.SupplierRemovalAskAbsentSnapshots++
		venue.SupplierRemovalAskAbsentSnapshots++
	}
	if bidDepth <= 0 && askDepth <= 0 {
		r.SupplierRemovalBothAbsentSnapshots++
		venue.SupplierRemovalBothAbsentSnapshots++
	}
	bidQualified := r.MinimumExecutableQty <= 0 || bidDepth >= r.MinimumExecutableQty
	askQualified := r.MinimumExecutableQty <= 0 || askDepth >= r.MinimumExecutableQty
	if !bidQualified {
		r.SupplierRemovalQualifiedBidAbsentSnapshots++
		venue.SupplierRemovalQualifiedBidAbsentSnapshots++
	}
	if !askQualified {
		r.SupplierRemovalQualifiedAskAbsentSnapshots++
		venue.SupplierRemovalQualifiedAskAbsentSnapshots++
	}
	if !bidQualified && !askQualified {
		r.SupplierRemovalQualifiedBothAbsentSnapshots++
		venue.SupplierRemovalQualifiedBothAbsentSnapshots++
	}
	if (bidDepth <= 0) != (askDepth <= 0) {
		r.SupplierRemovalOneSidedSnapshots++
		venue.SupplierRemovalOneSidedSnapshots++
	}
}

func supplierDisplayedDepthBySide(venueID string, orders map[cdfOrderKey]*cdfOrderState) (int64, int64, bool) {
	var bidDepth, askDepth int64
	for key, order := range orders {
		if key.VenueID != venueID || order.closed || order.remainingQty <= 0 {
			continue
		}
		var target *int64
		switch order.side {
		case "BUY":
			target = &bidDepth
		case "SELL":
			target = &askDepth
		default:
			return 0, 0, false
		}
		updated, ok := exactAdd(*target, order.remainingQty)
		if !ok {
			return 0, 0, false
		}
		*target = updated
	}
	return bidDepth, askDepth, true
}

func supplierDisplayedDepthByClient(venueID string, orders map[cdfOrderKey]*cdfOrderState) map[uint64]int64 {
	depthByClient := make(map[uint64]int64)
	for key, order := range orders {
		if key.VenueID != venueID || order.closed || order.remainingQty <= 0 {
			continue
		}
		updated, ok := exactAdd(depthByClient[order.clientID], order.remainingQty)
		if !ok {
			depthByClient[order.clientID] = math.MaxInt64
			continue
		}
		depthByClient[order.clientID] = updated
	}
	return depthByClient
}

func (r *CDFLiquidityRunAudit) reconcileFills(observed map[cdfFillKey]cdfObservedFill, actual map[cdfFillKey]cdfOrderFillEvidence, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	for key, observedFill := range observed {
		actualFill, exists := actual[key]
		if !exists {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, ClientID: key.ClientID, Ordinal: observedFill.ordinal, Failure: "supplier fill evidence has no matching order fill"})
			continue
		}
		fill := observedFill.fill
		if actualFill.Side != fill.Side || actualFill.Price != fill.Price || actualFill.Qty != fill.Qty || actualFill.IsFull != fill.IsFull {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: fill.Role, ClientID: key.ClientID, Ordinal: observedFill.ordinal, Failure: "supplier fill evidence disagrees with order fill"})
		}
	}
	for key := range actual {
		if _, exists := observed[key]; !exists {
			if state := states[cdfParticipantKey{VenueID: key.VenueID, ClientID: key.ClientID}]; state != nil {
				r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier order fill lacks local fill evidence"})
			}
		}
	}
}

func (r *CDFLiquidityRunAudit) finalizeOrders(orders map[cdfOrderKey]*cdfOrderState, states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	keys := make([]cdfOrderKey, 0, len(orders))
	for key := range orders {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VenueID != keys[j].VenueID {
			return keys[i].VenueID < keys[j].VenueID
		}
		if keys[i].ClientID != keys[j].ClientID {
			return keys[i].ClientID < keys[j].ClientID
		}
		return keys[i].OrderID < keys[j].OrderID
	})
	var lifetimeTotal int64
	var lifetimeCount int64
	for _, key := range keys {
		order := orders[key]
		state := states[cdfParticipantKey{VenueID: key.VenueID, ClientID: key.ClientID}]
		if state == nil {
			continue
		}
		state.AcceptedQuoteCount++
		r.AcceptedQuoteCount++
		if !order.closed {
			state.CensoredQuoteCount++
			r.CensoredQuoteCount++
			if order.cancelRequested {
				state.CancelPendingQuoteCount++
				r.CancelPendingQuoteCount++
			} else {
				state.LiveAcceptedQuoteCount++
				r.LiveAcceptedQuoteCount++
			}
			continue
		}
		state.CompletedQuoteCount++
		r.CompletedQuoteCount++
		lifetime := order.closedAt - order.acceptedAt
		if lifetime < 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "negative quote lifetime"})
			continue
		}
		state.quoteLifetimeTotal, _ = exactAdd(state.quoteLifetimeTotal, lifetime)
		state.quoteLifetimeCount++
		if lifetime > state.MaxQuoteLifetimeNs {
			state.MaxQuoteLifetimeNs = lifetime
		}
		if lifetime > r.MaxQuoteLifetimeNs {
			r.MaxQuoteLifetimeNs = lifetime
		}
		lifetimeTotal, _ = exactAdd(lifetimeTotal, lifetime)
		lifetimeCount++
	}
	for _, state := range states {
		for range state.pendingQuoteByRequest {
			state.CensoredQuoteCount++
			r.CensoredQuoteCount++
			state.PendingSubmissionCount++
			r.PendingSubmissionCount++
		}
	}
	if lifetimeCount > 0 {
		r.MeanQuoteLifetimeNs = float64(lifetimeTotal) / float64(lifetimeCount)
	}
}

func (r *CDFLiquidityRunAudit) finalizeSuppliers(states map[cdfParticipantKey]*CDFLiquiditySupplierAudit) {
	keys := make([]cdfParticipantKey, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].VenueID != keys[j].VenueID {
			return keys[i].VenueID < keys[j].VenueID
		}
		return keys[i].ClientID < keys[j].ClientID
	})
	var touchTotal float64
	var touchCount int64
	for _, key := range keys {
		state := states[key]
		if state.MinPosition == math.MaxInt64 {
			state.MinPosition = state.lastPosition
		}
		if state.MaxPosition == math.MinInt64 {
			state.MaxPosition = state.lastPosition
		}
		state.TerminalPosition = state.lastPosition
		if state.observationCount > 0 {
			state.MeanObservationAgeNs = float64(state.observationAgeTotal) / float64(state.observationCount)
		}
		if state.touchShareCount > 0 {
			state.MeanObservedTouchShare = state.touchShareTotal / float64(state.touchShareCount)
			touchTotal += state.touchShareTotal
			touchCount += state.touchShareCount
		}
		if state.restingDepthWeightedDenominator > 0 {
			state.TimeWeightedRestingDepthShare = state.restingDepthWeightedNumerator / state.restingDepthWeightedDenominator
		}
		if state.quoteLifetimeCount > 0 {
			state.MeanQuoteLifetimeNs = float64(state.quoteLifetimeTotal) / float64(state.quoteLifetimeCount)
		}
		if state.FillCount > 0 {
			r.TradingSupplierCount++
		}
		if state.PnL != 0 {
			r.PnLChangingSupplierCount++
		}
		state.RealizedPnL = state.realizedPnL
		if state.lastPosition != 0 && state.entryPrice > 0 && state.terminalMark > 0 {
			unrealized, ok := signedQuoteProduct(state.terminalMark-state.entryPrice, state.lastPosition, state.configuredBasePrecision)
			if !ok {
				r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "unrealized PnL overflow"})
			} else {
				state.UnrealizedPnL = unrealized
			}
		}
		state.EndowmentRevaluationPnL = state.endowmentRevaluationPnL
		state.TradingPnL = state.tradingPnL
		tradingDecomposition, decompositionOK := exactAdd(state.RealizedPnL, state.UnrealizedPnL)
		if decompositionOK {
			state.TradingPnLReconciliationResidual, decompositionOK = exactAdd(state.TradingPnL, -tradingDecomposition)
		}
		if !decompositionOK {
			state.TradingPnLReconciliationResidual = math.MaxInt64
		}
		if absInt64(state.TradingPnLReconciliationResidual) > state.FillCount+2 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "trading PnL decomposition exceeds fixed-point rounding allowance"})
		}
		state.MaxQuoteQty = state.maxQuoteQty
		state.ConfiguredMaxLossQuote = state.configuredMaxLossQuote
		state.ConfiguredIntervalNs = state.configuredIntervalNs
		if state.configuredMaxLossQuote > 0 && !state.riskStateSeen {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no marked-risk decision state"})
		}
		if state.FillCount > 0 && state.InventoryResponsiveDecisionCount == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no inventory-responsive post-fill decision"})
		}
		state.MaxBorrowed = state.maxBorrowed
		state.BorrowEventCount = state.borrowEventCount
		state.MaxGrossBaseBalance = state.maxGrossBaseBalance
		state.MaxGrossQuoteBalance = state.maxGrossQuoteBalance
		if state.configuredMaxInventory <= 0 || state.maxGrossBaseBalance > state.configuredMaxInventory {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier exceeded configured gross base inventory"})
		}
		if state.configuredMakerFeeBps < 0 || state.configuredMakerFeeBps > 10_000 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has invalid configured maker fee"})
		}
		if state.configuredMaxQuoteQty <= 0 || state.maxQuoteQty > state.configuredMaxQuoteQty {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier exceeded configured quote quantity"})
		}
		if state.maxBorrowed > 0 || state.borrowEventCount > 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier used unregistered borrowed capital"})
		}
		baseHoldingInPositionBounds := state.configuredBaseHolding >= -state.configuredMaxPosition && state.configuredBaseHolding <= state.configuredMaxPosition
		if !state.initialAccountSeen || !state.terminalAccountSeen || state.configuredMaxPosition <= 0 || state.configuredMaxInventory <= 0 || state.configuredMaxQuoteQty <= 0 || state.configuredBasePrecision <= 0 || state.configuredQuotePrecision <= 0 || state.configuredMaxObservationAge <= 0 || state.configuredInitialBaseBalance <= 0 || state.configuredInitialQuoteBalance <= 0 || state.configuredReferencePrice <= 0 || state.configuredReferenceHalfLife <= 0 || state.configuredElasticityPerPercent <= 0 || state.configuredMaxLossQuote > 0 && (state.configuredMinimumExecutableQty <= 0 || state.configuredIntervalNs <= 0) || !baseHoldingInPositionBounds {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier lacks complete finite-capital configuration"})
		}
		state.Valid = state.DecisionCount > 0 && state.FillCount > 0 && state.AcceptedQuoteCount > 0 && state.CompletedQuoteCount > 0 && state.initialAccountSeen && state.terminalAccountSeen
		if state.DecisionCount == 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no decision evidence"})
		}
		if state.InventoryLimit <= 0 {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier has no positive inventory limit"})
		}
		if !state.Valid {
			r.addCheck(CDFLiquidityCheck{VenueID: key.VenueID, Role: state.Role, ClientID: key.ClientID, Failure: "supplier activation contract is incomplete"})
		}
		r.RealizedPnL, _ = exactAdd(r.RealizedPnL, state.RealizedPnL)
		r.UnrealizedPnL, _ = exactAdd(r.UnrealizedPnL, state.UnrealizedPnL)
		r.EndowmentRevaluationPnL, _ = exactAdd(r.EndowmentRevaluationPnL, state.EndowmentRevaluationPnL)
		r.TradingPnL, _ = exactAdd(r.TradingPnL, state.TradingPnL)
		r.TradingPnLReconciliationResidual, _ = exactAdd(r.TradingPnLReconciliationResidual, state.TradingPnLReconciliationResidual)
		r.BalanceSnapshotCount, _ = exactAdd(r.BalanceSnapshotCount, state.BalanceSnapshotCount)
		r.BalanceReconciliationResidual, _ = exactAdd(r.BalanceReconciliationResidual, state.BalanceReconciliationResidual)
		r.PnLReconciliationResidual, _ = exactAdd(r.PnLReconciliationResidual, state.PnLReconciliationResidual)
		if state.MaxBorrowed > r.MaxBorrowed {
			r.MaxBorrowed = state.MaxBorrowed
		}
		r.SupplierVolumeQty, _ = exactAdd(r.SupplierVolumeQty, state.FilledQty)
		if state.MaxObservedLossFromInitialQuote > r.MaxObservedLossFromInitialQuote {
			r.MaxObservedLossFromInitialQuote = state.MaxObservedLossFromInitialQuote
		}
		if state.MaxObservedDrawdownQuote > r.MaxObservedDrawdownQuote {
			r.MaxObservedDrawdownQuote = state.MaxObservedDrawdownQuote
		}
		if state.MaxObservedTouchShare > r.MaxObservedTouchShare {
			r.MaxObservedTouchShare = state.MaxObservedTouchShare
		}
		r.Suppliers = append(r.Suppliers, *state)
	}
	if touchCount > 0 {
		r.MeanObservedTouchShare = touchTotal / float64(touchCount)
	}
}

func (r *CDFLiquidityRunAudit) finalizeVenueAudits(venueAudits map[string]*CDFLiquidityVenueAudit) {
	for index := range r.Suppliers {
		supplier := &r.Suppliers[index]
		venue := venueAudits[supplier.VenueID]
		if venue == nil {
			venue = &CDFLiquidityVenueAudit{VenueID: supplier.VenueID, ExpectedHistoricalCount: r.expectedHistoricalCountPerVenue, MinimumExecutableQty: r.MinimumExecutableQty}
			venueAudits[supplier.VenueID] = venue
		}
		venue.SupplierVolumeQty, _ = exactAdd(venue.SupplierVolumeQty, supplier.FilledQty)
		if venue.TotalTradeVolumeQty > 0 {
			supplier.SupplierVolumeShare = float64(supplier.FilledQty) / float64(venue.TotalTradeVolumeQty)
		}
		venue.SupplierTimeWeightedRestingDepthShare += supplier.TimeWeightedRestingDepthShare
	}
	for _, venue := range venueAudits {
		if venue.TotalTradeVolumeQty > 0 {
			venue.SupplierVolumeShare = float64(venue.SupplierVolumeQty) / float64(venue.TotalTradeVolumeQty)
		}
		if venue.ActiveDepthSnapshotCount > 0 {
			venue.SupplierDepthOver75Fraction = float64(venue.SupplierDepthOver75Count) / float64(venue.ActiveDepthSnapshotCount)
		}
		if venue.SupplierRemovalSnapshotCount > 0 {
			venue.SupplierRemovalBidAbsenceFraction = float64(venue.SupplierRemovalBidAbsentSnapshots) / float64(venue.SupplierRemovalSnapshotCount)
			venue.SupplierRemovalAskAbsenceFraction = float64(venue.SupplierRemovalAskAbsentSnapshots) / float64(venue.SupplierRemovalSnapshotCount)
			venue.SupplierRemovalQualifiedBidAbsenceFraction = float64(venue.SupplierRemovalQualifiedBidAbsentSnapshots) / float64(venue.SupplierRemovalSnapshotCount)
			venue.SupplierRemovalQualifiedAskAbsenceFraction = float64(venue.SupplierRemovalQualifiedAskAbsentSnapshots) / float64(venue.SupplierRemovalSnapshotCount)
		}
		venue.SupplierRemovalCounterfactualValid = venue.SupplierRemovalSnapshotCount == venue.SnapshotCount && venue.SupplierRemovalInvalidSnapshots == 0 && venue.SnapshotCount > 0
		if venue.SupplierVolumeShare > 0.75 {
			r.addCheck(CDFLiquidityCheck{VenueID: venue.VenueID, Failure: "supplier volume share exceeds 75 percent"})
		}
		if venue.SupplierDepthOver75Fraction > 0.5 {
			r.addCheck(CDFLiquidityCheck{VenueID: venue.VenueID, Failure: "supplier displayed-depth share exceeds 75 percent for more than half of active snapshots"})
		}
		if venue.HistoricalSupplierCount != venue.ExpectedHistoricalCount {
			r.addCheck(CDFLiquidityCheck{VenueID: venue.VenueID, Failure: fmt.Sprintf("historical supplier count %d does not match configured %d", venue.HistoricalSupplierCount, venue.ExpectedHistoricalCount)})
		}
		r.Venues = append(r.Venues, *venue)
		if venue.MaxSupplierDepthShare > r.MaxSupplierDepthShare {
			r.MaxSupplierDepthShare = venue.MaxSupplierDepthShare
		}
	}
	sort.Slice(r.Venues, func(i, j int) bool { return r.Venues[i].VenueID < r.Venues[j].VenueID })
	var activeSnapshots, over75 int64
	var removalSnapshots, removalInvalid int64
	for _, venue := range r.Venues {
		activeSnapshots += venue.ActiveDepthSnapshotCount
		over75 += venue.SupplierDepthOver75Count
		removalSnapshots += venue.SupplierRemovalSnapshotCount
		removalInvalid += venue.SupplierRemovalInvalidSnapshots
	}
	if activeSnapshots > 0 {
		r.SupplierDepthOver75Share = float64(over75) / float64(activeSnapshots)
	}
	if r.totalRestingDepthWeightedDenominator > 0 {
		r.SupplierTimeWeightedRestingDepthShare = r.supplierRestingDepthWeightedNumerator / r.totalRestingDepthWeightedDenominator
	}
	if r.SupplierRemovalSnapshotCount > 0 {
		r.SupplierRemovalBidAbsenceFraction = float64(r.SupplierRemovalBidAbsentSnapshots) / float64(r.SupplierRemovalSnapshotCount)
		r.SupplierRemovalAskAbsenceFraction = float64(r.SupplierRemovalAskAbsentSnapshots) / float64(r.SupplierRemovalSnapshotCount)
		r.SupplierRemovalQualifiedBidAbsenceFraction = float64(r.SupplierRemovalQualifiedBidAbsentSnapshots) / float64(r.SupplierRemovalSnapshotCount)
		r.SupplierRemovalQualifiedAskAbsenceFraction = float64(r.SupplierRemovalQualifiedAskAbsentSnapshots) / float64(r.SupplierRemovalSnapshotCount)
	}
	r.SupplierRemovalCounterfactualValid = removalSnapshots == r.SnapshotCount && removalInvalid == 0 && r.SnapshotCount > 0
}

func decisionTouchShare(decision cdfDecisionEvidence) (float64, bool) {
	if decision.QuoteQty <= 0 || !validSide(decision.Side) {
		return 0, false
	}
	depth := decision.BestBidQty
	if decision.Side == "SELL" {
		depth = decision.BestAskQty
	}
	if depth <= 0 {
		return 0, false
	}
	return float64(decision.QuoteQty) / float64(depth), true
}

func quoteMatchesObservedTouch(decision cdfDecisionEvidence) bool {
	if !validSide(decision.Side) || decision.QuotePrice <= 0 || decision.QuoteQty <= 0 {
		return false
	}
	if decision.Side == "BUY" {
		return decision.QuotePrice == decision.BestBid && decision.QuoteQty <= decision.BestBidQty
	}
	return decision.QuotePrice == decision.BestAsk && decision.QuoteQty <= decision.BestAskQty
}

func expectedCDFTargetPosition(markPrice, referencePrice int64, state *CDFLiquiditySupplierAudit) (int64, bool) {
	if markPrice <= 0 || referencePrice <= 0 || state.configuredElasticityPerPercent <= 0 {
		return 0, false
	}
	percentAbove := (float64(markPrice)/float64(referencePrice) - 1) * 100
	target := float64(state.configuredBaseHolding) - percentAbove*float64(state.configuredElasticityPerPercent)
	if math.IsNaN(target) || math.IsInf(target, 0) {
		return 0, false
	}
	minimumPosition, maximumPosition := -state.configuredMaxPosition, state.configuredMaxPosition
	if state.configuredMaxInventory > 0 {
		minimumPosition = maxInt64(minimumPosition, -state.configuredInitialBaseBalance)
		maximumPosition = minInt64(maximumPosition, state.configuredMaxInventory-state.configuredInitialBaseBalance)
	}
	return int64(math.Max(float64(minimumPosition), math.Min(float64(maximumPosition), target))), true
}

func expectedCDFInventoryQuote(decision cdfDecisionEvidence, state *CDFLiquiditySupplierAudit) (string, int64, bool) {
	return expectedCDFInventoryQuoteAtWithCash(decision.TargetPosition, decision.Position, decision.GrossInventory, decision.QuotePrice, decision.QuoteCashAvailable, state)
}

func expectedCDFInventoryQuoteAtWithCash(targetPosition, position, grossInventory, quotePrice, quoteCashAvailable int64, state *CDFLiquiditySupplierAudit) (string, int64, bool) {
	gapBig := new(big.Int).Sub(big.NewInt(targetPosition), big.NewInt(position))
	if !gapBig.IsInt64() || gapBig.Sign() == 0 || state.configuredMaxInventory <= 0 || state.configuredMaxQuoteQty <= 0 {
		return "", 0, false
	}
	gap := gapBig.Int64()
	side := "BUY"
	if gap < 0 {
		side = "SELL"
	}
	quantity := absInt64(gap)
	available := int64(0)
	if side == "BUY" {
		available = state.configuredMaxInventory - grossInventory
	} else {
		available = grossInventory
	}
	if available <= 0 {
		return side, 0, false
	}
	if quantity > available {
		quantity = available
	}
	if quantity > state.configuredMaxQuoteQty {
		quantity = state.configuredMaxQuoteQty
	}
	if side == "BUY" && state.configuredInitialQuoteBalance > 0 && state.configuredQuotePrecision > 0 {
		quantity = maxAffordableCDFQuoteQty(quotePrice, state.configuredBasePrecision, state.configuredMakerFeeBps, quoteCashAvailable, quantity)
	}
	if state.configuredMinimumExecutableQty > 0 && quantity < state.configuredMinimumExecutableQty {
		return side, 0, false
	}
	return side, quantity, quantity > 0
}

func maxAffordableCDFQuoteQty(price, basePrecision, makerFeeBps, available, upper int64) int64 {
	if price <= 0 || available <= 0 || upper <= 0 {
		return 0
	}
	low, high := int64(0), upper
	for low < high {
		mid := low + (high-low+1)/2
		required, ok := expectedCDFQuoteRequirement(price, mid, basePrecision, makerFeeBps)
		if ok && required <= available {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return low
}

func advanceCDFReference(previousReference, midpoint, elapsed, halfLife int64) (int64, bool) {
	if previousReference <= 0 || midpoint <= 0 || elapsed <= 0 || halfLife <= 0 {
		return 0, false
	}
	alpha := 1 - math.Exp(-math.Ln2*float64(elapsed)/float64(halfLife))
	revised := float64(previousReference) + alpha*(float64(midpoint)-float64(previousReference))
	if math.IsNaN(revised) || math.IsInf(revised, 0) || revised <= 0 || revised > float64(math.MaxInt64) {
		return 0, false
	}
	return int64(revised), true
}

func validSide(side string) bool { return side == "BUY" || side == "SELL" }

func isCDFStaleWithdrawal(decision cdfDecisionEvidence, configuredMaxAge int64) bool {
	return configuredMaxAge > 0 && decision.Action == "withdraw" && decision.Reason == "stale_or_missing_observation" && decision.ObservationTime > 0 && decision.ObservationSequence > 0 && decision.ObservationAge > configuredMaxAge && decision.DecisionTime-decision.ObservationTime == decision.ObservationAge
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func accountHasNetBalance(balances []Balance, asset string, expected int64) bool {
	for _, balance := range balances {
		if balance.Asset == asset {
			return balance.NetAsset == expected && balance.Borrowed == 0
		}
	}
	return false
}

func assignAccountBalances(target map[string]int64, balances []Balance) error {
	for _, balance := range balances {
		if balance.Asset == "" {
			return fmt.Errorf("asset is empty")
		}
		if _, exists := target[balance.Asset]; exists {
			return fmt.Errorf("duplicate asset %q", balance.Asset)
		}
		if balance.Borrowed < 0 {
			return fmt.Errorf("borrowed amount for %s is negative", balance.Asset)
		}
		target[balance.Asset] = balance.NetAsset
	}
	return nil
}

func hasNonZeroBalances(balances []Balance) bool {
	for _, balance := range balances {
		if balance.NetAsset != 0 || balance.Borrowed != 0 {
			return true
		}
	}
	return false
}

func hasNonZeroPositions(positions []Position) bool {
	for _, position := range positions {
		if position.Size != 0 {
			return true
		}
	}
	return false
}

func cloneMarks(marks map[string]int64) map[string]int64 {
	clone := make(map[string]int64, len(marks))
	for asset, mark := range marks {
		clone[asset] = mark
	}
	return clone
}

func isCDFSupplierRole(role string) bool {
	const prefix = "cdf_elastic_supplier_"
	if !strings.HasPrefix(role, prefix) {
		return false
	}
	_, err := strconv.ParseUint(strings.TrimPrefix(role, prefix), 10, 32)
	return err == nil
}

func isHistoricalElasticSupplierRole(role string) bool {
	const prefix = "elastic_supplier_"
	if !strings.HasPrefix(role, prefix) {
		return false
	}
	_, err := strconv.ParseUint(strings.TrimPrefix(role, prefix), 10, 32)
	return err == nil
}

func auditRoleClass(role string) string {
	index := strings.LastIndex(role, "_")
	if index <= 0 || index == len(role)-1 {
		return role
	}
	if _, err := strconv.ParseUint(role[index+1:], 10, 32); err != nil {
		return role
	}
	return role[:index]
}
