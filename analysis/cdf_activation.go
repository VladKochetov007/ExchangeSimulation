package analysis

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"exchange_sim/evstream"
	etypes "exchange_sim/types"
)

const cdfActivationSymbol = "CDF/USD"
const cdfActivationLogName = "CDF-USD"

// CDFActivationOptions selects the immutable evidence and the scientific
// contract used to audit it. RenderedEvidenceDir is optional for historical
// JSON runs; binary runs point it at an independently verified rendering.
type CDFActivationOptions struct {
	Contract            CDFActivationContract
	EvidenceDir         string
	RenderedEvidenceDir string
	// ExpectedProvenance is supplied by the launcher from independently resolved
	// inputs. Strict audits compare self-reported run metadata to these values.
	ExpectedProvenance CDFExpectedProvenance
	// AllowLegacyJSON is reserved for historical fixture/reconstruction use.
	// A real v2 successor audit must leave this false and provide a rendered
	// binary evidence directory with its source stream present.
	AllowLegacyJSON bool
}

// CDFActivationContract makes the evaluator reusable by successor campaigns
// without teaching the analysis package about a central experiment registry.
type CDFActivationContract struct {
	HypothesisID        string
	ExperimentID        string
	Seed                int64
	Horizon             string
	SimulationStartNano int64
	SimulationEndNano   int64
	// ObservationIntervalNano is the registered public-book sampling cadence.
	// A strict audit may reconstruct the opening empty state only when the first
	// observed transition is exactly one interval after the registered start.
	ObservationIntervalNano           int64
	InitialPublicBookMode             string
	VenueIDs                          []string
	HistoricalSupplierCountPerVenue   int
	Suppliers                         []CDFSupplierContract
	MaximumSupplierVolumeShare        float64
	MaximumSupplierDepthShare         float64
	MaximumDepthDominanceTimeFraction float64
	BinarySchemaEpoch                 uint32
}

// CDFExpectedProvenance is the external identity recorded before a strict run.
// The analyzer does not derive these values from files in the run directory.
type CDFExpectedProvenance struct {
	ConfigSHA256             string
	TreatmentConfigSHA256    string
	ModeOffConfigSHA256      string
	NoRosterConfigSHA256     string
	SourceRevision           string
	TreeRevision             string
	PlanSHA256               string
	ParentRegistrationSHA256 string
	AmendmentSHA256          string
	BinarySHA256             string
	AnalyzerSHA256           string
	BinaryGOOS               string
	BinaryGOARCH             string
	BinaryGOAMD64            string
	// Renderer identity is required by the SV1D successor adapter. Historical
	// CDF audits leave these fields empty because their contract predates the
	// explicit renderer attestation.
	RendererSHA256            string
	RendererSourceRevision    string
	RendererSourceModified    bool
	RendererGOOS              string
	RendererGOARCH            string
	RendererGOAMD64           string
	RendererGoVersion         string
	RendererTrimpath          bool
	RendererCGOEnabled        string
	ReviewAttestationSHA256   string
	ReviewReportSHA256        string
	CapacityAttestationSHA256 string
	CapacityRecordsSHA256     string
	CapacityRunnerSHA256      string
	ActivationRunnerSHA256    string
	ActivationMetadataSHA256  string
	ActivationMetadataPath    string
	TrustedReviewKeySHA256    string
	EvidenceSchemaEpoch       uint32
	GOMAXPROCS                int
	GOMEMLIMIT                string
}

// CDFSupplierContract is one immutable finite-capital roster entry.
type CDFSupplierContract struct {
	Role                           string `json:"role"`
	Symbol                         string `json:"symbol"`
	BaseAsset                      string `json:"base_asset"`
	QuoteAsset                     string `json:"quote_asset"`
	BasePrecision                  int64  `json:"base_precision"`
	QuotePrecision                 int64  `json:"quote_precision"`
	InitialBaseBalance             int64  `json:"initial_base_balance"`
	InitialQuoteBalance            int64  `json:"initial_quote_balance"`
	Interval                       int64  `json:"interval"`
	DecisionPhaseOffset            int64  `json:"decision_phase_offset,omitempty"`
	MaxObservationAge              int64  `json:"max_observation_age"`
	ReferencePrice                 int64  `json:"reference_price"`
	ReferenceHalfLife              int64  `json:"reference_half_life"`
	BaseHolding                    int64  `json:"base_holding"`
	ElasticityPerPercent           int64  `json:"elasticity_per_percent"`
	MaxPosition                    int64  `json:"max_position"`
	MaxInventory                   int64  `json:"max_inventory"`
	MaxQuoteQty                    int64  `json:"max_quote_qty"`
	MinimumExecutableQty           int64  `json:"minimum_executable_qty"`
	MinimumQualifyingQty           int64  `json:"minimum_qualifying_qty"`
	TickSize                       int64  `json:"tick_size"`
	RegisteredMinimumExecutableQty int64  `json:"registered_minimum_executable_qty"`
	QuoteOnOneSidedLocalBook       bool   `json:"quote_on_one_sided_local_book"`
	MaxLossQuote                   int64  `json:"max_loss_quote"`
	MakerFeeBps                    int64  `json:"maker_fee_bps"`
}

// RegisteredSV1DActivationContract returns the preregistered five-minute
// treatment contract. Callers may supply another explicit contract to reuse
// the evaluator for a later successor without modifying this package.
func RegisteredSV1DActivationContract() CDFActivationContract {
	const (
		second = int64(1_000_000_000)
		hour   = 3_600 * second
	)
	common := func(role string, baseBalance, quoteBalance, halfLife, elasticity, maxPosition, maxInventory, maxQuote, maxLoss, phase int64) CDFSupplierContract {
		return CDFSupplierContract{
			Role: role, Symbol: cdfActivationSymbol, BaseAsset: "CDF", QuoteAsset: "USD",
			BasePrecision: 100_000_000, QuotePrecision: 100_000,
			InitialBaseBalance: baseBalance, InitialQuoteBalance: quoteBalance,
			Interval: 2 * second, DecisionPhaseOffset: phase, MaxObservationAge: 60 * second,
			ReferencePrice: 300_000_000, ReferenceHalfLife: halfLife,
			ElasticityPerPercent: elasticity, MaxPosition: maxPosition,
			MaxInventory: maxInventory, MaxQuoteQty: maxQuote,
			MinimumExecutableQty: 100_000, MinimumQualifyingQty: 1_000_000,
			TickSize: 100_000, RegisteredMinimumExecutableQty: 100_000,
			QuoteOnOneSidedLocalBook: true, MaxLossQuote: maxLoss, MakerFeeBps: 5,
		}
	}
	return CDFActivationContract{
		HypothesisID: "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY",
		ExperimentID: "v2-r2-sv1d-activation-659-treatment",
		Seed:         659, Horizon: "5m",
		SimulationStartNano:             1_735_689_600_000_000_000,
		SimulationEndNano:               1_735_689_900_000_000_000,
		ObservationIntervalNano:         second,
		InitialPublicBookMode:           "empty",
		VenueIDs:                        []string{"north", "central", "south"},
		HistoricalSupplierCountPerVenue: 8,
		Suppliers: []CDFSupplierContract{
			common("cdf_elastic_supplier_1", 4_000_000_000, 18_000_000_000, 3*hour, 12_000_000_000, 4_000_000_000, 8_000_000_000, 40_000_000, 3_000_000_000, 0),
			common("cdf_elastic_supplier_2", 5_000_000_000, 21_000_000_000, 4*hour, 15_000_000_000, 5_000_000_000, 10_000_000_000, 50_000_000, 3_600_000_000, 500_000_000),
			common("cdf_elastic_supplier_3", 6_000_000_000, 24_000_000_000, 5*hour, 18_000_000_000, 6_000_000_000, 12_000_000_000, 60_000_000, 4_200_000_000, 1_000_000_000),
			common("cdf_elastic_supplier_4", 7_000_000_000, 27_000_000_000, 6*hour, 21_000_000_000, 7_000_000_000, 14_000_000_000, 70_000_000, 4_800_000_000, 1_500_000_000),
		},
		MaximumSupplierVolumeShare:        0.75,
		MaximumSupplierDepthShare:         0.75,
		MaximumDepthDominanceTimeFraction: 0.50,
		BinarySchemaEpoch:                 4,
	}
}

// CDFActivationAudit separates evidence integrity, mechanism activation, and
// anti-cheating gates so a negative economic result is not mistaken for bad
// evidence. Valid is deliberately the conjunction required for promotion.
type CDFActivationAudit struct {
	Provenance               CDFActivationProvenance      `json:"provenance"`
	SupplierCount            int                          `json:"supplier_count"`
	DecisionCount            int64                        `json:"decision_count"`
	AcceptedOrderCount       int64                        `json:"accepted_order_count"`
	FillCount                int64                        `json:"fill_count"`
	WithdrawalCount          int64                        `json:"withdrawal_count"`
	OneSidedDecisionCount    int64                        `json:"one_sided_decision_count"`
	OneSidedRestorationCount int64                        `json:"one_sided_restoration_count"`
	SupplierVolumeQty        int64                        `json:"supplier_volume_qty"`
	SupplierVolumeNotional   int64                        `json:"supplier_volume_notional_quote"`
	SupplierFeesPaid         int64                        `json:"supplier_fees_paid_quote"`
	TotalVolumeQty           int64                        `json:"total_volume_qty"`
	SupplierVolumeShare      float64                      `json:"supplier_volume_share"`
	VolumeQtyByVenue         map[string]int64             `json:"volume_qty_by_venue"`
	Suppliers                []CDFSupplierActivationAudit `json:"suppliers"`
	Venues                   []CDFVenueConcentrationAudit `json:"venues"`
	Checks                   []CDFActivationCheck         `json:"checks,omitempty"`
	EvidenceValid            bool                         `json:"evidence_valid"`
	ActivationSatisfied      bool                         `json:"activation_satisfied"`
	AntiCheatingSatisfied    bool                         `json:"anti_cheating_satisfied"`
	Valid                    bool                         `json:"valid"`

	strictMechanics     bool
	trades              map[cdfTradeKey]cdfTradeEvidence
	totalVolumeByVenue  map[string]int64
	terminalOrders      map[cdfOrderKey]*cdfOrderState
	liveOrderBySupplier map[cdfParticipantKey]cdfOrderKey
	observedFillGlobal  map[cdfFillKey]uint64
	actualFillGlobal    map[cdfFillKey]uint64
	tradeGlobal         map[cdfTradeKey]uint64
}

type CDFActivationProvenance struct {
	ConfigSHA256        string   `json:"config_sha256"`
	SourceRevision      string   `json:"source_revision"`
	SourceModified      bool     `json:"source_modified"`
	BinarySHA256        string   `json:"binary_sha256"`
	Seed                int64    `json:"seed"`
	Horizon             string   `json:"horizon"`
	SimulationStartNano int64    `json:"simulation_start_nano"`
	SimulationEndNano   int64    `json:"simulation_end_nano"`
	VenueIDs            []string `json:"venue_ids"`
	ExperimentID        string   `json:"experiment_id"`
	HypothesisID        string   `json:"hypothesis_id"`
	EvidenceFormat      string   `json:"evidence_format"`
	LogMode             string   `json:"log_mode"`
}

type CDFSupplierActivationAudit struct {
	VenueID                       string  `json:"venue_id"`
	Role                          string  `json:"role"`
	ClientID                      uint64  `json:"client_id"`
	DecisionCount                 int64   `json:"decision_count"`
	EligibleObservationCount      int64   `json:"eligible_observation_count"`
	AcceptedOrderCount            int64   `json:"accepted_order_count"`
	FillCount                     int64   `json:"fill_count"`
	BalanceSnapshotCount          int64   `json:"balance_snapshot_count"`
	PostFillBalanceSnapshotCount  int64   `json:"post_fill_balance_snapshot_count"`
	PostFillResponsiveCount       int64   `json:"post_fill_responsive_count"`
	TradeCount                    int64   `json:"trade_count"`
	VolumeQty                     int64   `json:"volume_qty"`
	VolumeNotionalQuote           int64   `json:"volume_notional_quote"`
	FeesPaidQuote                 int64   `json:"fees_paid_quote"`
	VenueVolumeDenominatorQty     int64   `json:"venue_volume_denominator_qty"`
	VenueVolumeShare              float64 `json:"venue_volume_share"`
	GlobalVolumeShare             float64 `json:"global_volume_share"`
	FilledOrderCount              int64   `json:"filled_order_count"`
	CancelledOrderCount           int64   `json:"cancelled_order_count"`
	ForcedCancelCount             int64   `json:"forced_cancel_count"`
	CensoredOrderCount            int64   `json:"censored_order_count"`
	CancelRejectedCount           int64   `json:"cancel_rejected_count"`
	SuccessfulWithdrawalCount     int64   `json:"successful_withdrawal_count"`
	RepriceCancelCount            int64   `json:"reprice_cancel_count"`
	CompletedRepriceCount         int64   `json:"completed_reprice_count"`
	TotalQuoteLifetimeNano        int64   `json:"total_quote_lifetime_nano"`
	CensoredQuoteLifetimeNano     int64   `json:"censored_quote_lifetime_nano"`
	MaxQuoteLifetimeNano          int64   `json:"max_quote_lifetime_nano"`
	WithdrawalCount               int64   `json:"withdrawal_count"`
	OpenOrderCount                int64   `json:"open_order_count"`
	OpenOrderQty                  int64   `json:"open_order_qty"`
	InitialEquity                 int64   `json:"initial_equity"`
	TerminalEquity                int64   `json:"terminal_equity"`
	PnL                           int64   `json:"pnl"`
	MinPosition                   int64   `json:"min_position"`
	MaxPosition                   int64   `json:"max_position"`
	MaxGrossInventory             int64   `json:"max_gross_inventory"`
	DepthObservationCount         int64   `json:"depth_observation_count"`
	BidDepthTimeWeightedShare     float64 `json:"bid_depth_time_weighted_share"`
	AskDepthTimeWeightedShare     float64 `json:"ask_depth_time_weighted_share"`
	BidQualifyingDepthShare       float64 `json:"bid_qualifying_depth_share"`
	AskQualifyingDepthShare       float64 `json:"ask_qualifying_depth_share"`
	BidDepthDominanceTimeFraction float64 `json:"bid_depth_dominance_time_fraction"`
	AskDepthDominanceTimeFraction float64 `json:"ask_depth_dominance_time_fraction"`
	BidRemovalDurationNano        int64   `json:"bid_removal_duration_nano"`
	AskRemovalDurationNano        int64   `json:"ask_removal_duration_nano"`
	BidRemovalTimeFraction        float64 `json:"bid_removal_time_fraction"`
	AskRemovalTimeFraction        float64 `json:"ask_removal_time_fraction"`
	EvidenceValid                 bool    `json:"evidence_valid"`
	ActivationSatisfied           bool    `json:"activation_satisfied"`
}

type CDFVenueConcentrationAudit struct {
	VenueID                                 string  `json:"venue_id"`
	SnapshotCount                           int64   `json:"snapshot_count"`
	BidActiveDurationNano                   int64   `json:"bid_active_duration_nano"`
	AskActiveDurationNano                   int64   `json:"ask_active_duration_nano"`
	BidDominantDurationNano                 int64   `json:"bid_dominant_duration_nano"`
	AskDominantDurationNano                 int64   `json:"ask_dominant_duration_nano"`
	BidDominanceTimeFraction                float64 `json:"bid_dominance_time_fraction"`
	AskDominanceTimeFraction                float64 `json:"ask_dominance_time_fraction"`
	BidOnlyDurationNano                     int64   `json:"bid_only_duration_nano"`
	AskOnlyDurationNano                     int64   `json:"ask_only_duration_nano"`
	EmptyBookDurationNano                   int64   `json:"empty_book_duration_nano"`
	OneSidedDurationNano                    int64   `json:"one_sided_duration_nano"`
	NonTwoSidedDurationNano                 int64   `json:"non_two_sided_duration_nano"`
	MaxUninterruptedNonTwoSidedDurationNano int64   `json:"max_uninterrupted_non_two_sided_duration_nano"`
	TerminalBookMode                        string  `json:"terminal_book_mode"`
	ConcentrationSatisfied                  bool    `json:"concentration_satisfied"`
}

type CDFActivationCheck struct {
	VenueID  string `json:"venue_id,omitempty"`
	Role     string `json:"role,omitempty"`
	ClientID uint64 `json:"client_id,omitempty"`
	Ordinal  int64  `json:"ordinal,omitempty"`
	Failure  string `json:"failure"`
}

type cdfActivationConfig struct {
	VenueIDs                                []string              `json:"venue_ids"`
	Seed                                    int64                 `json:"seed"`
	LogMode                                 string                `json:"log_mode"`
	EvidenceFormat                          string                `json:"evidence_format"`
	EvidenceContractVersion                 int                   `json:"evidence_contract_version"`
	ExperimentID                            string                `json:"experiment_id"`
	HypothesisID                            string                `json:"hypothesis_id"`
	ElasticSupplierCount                    int                   `json:"elastic_supplier_count"`
	StrictPopulationAccounting              bool                  `json:"strict_population_accounting"`
	StrictRiskContract                      bool                  `json:"strict_risk_contract"`
	AutoBorrowSpot                          *bool                 `json:"auto_borrow_spot"`
	CrossAssetSpotGraph                     bool                  `json:"cross_asset_spot_graph"`
	CrossAssetCollateralMarks               bool                  `json:"cross_asset_collateral_marks"`
	RecordElasticLiquiditySupplierDecisions bool                  `json:"record_elastic_liquidity_supplier_decisions"`
	RecordMarketDataReceipts                bool                  `json:"record_market_data_receipts"`
	MarketDataReceiptRoles                  []string              `json:"market_data_receipt_roles"`
	ElasticLiquiditySuppliers               []CDFSupplierContract `json:"elastic_liquidity_suppliers"`
}

type cdfActivationManifest struct {
	SchemaVersion int             `json:"schema_version"`
	Config        json.RawMessage `json:"config"`
	VenueIDs      []string        `json:"venue_ids"`
	Build         struct {
		Revision string `json:"revision"`
		Time     string `json:"time"`
		Modified bool   `json:"modified"`
		GOOS     string `json:"goos"`
		GOARCH   string `json:"goarch"`
		GOAMD64  string `json:"goamd64"`
	} `json:"build"`
	Notes []string `json:"notes"`
}

type cdfActivationMetadata struct {
	SchemaVersion               int             `json:"schema_version"`
	RunnerContract              string          `json:"runner_contract"`
	Contract                    string          `json:"contract"`
	ProbeID                     string          `json:"probe_id"`
	Arm                         string          `json:"arm"`
	Mode                        string          `json:"mode"`
	Cell                        string          `json:"cell"`
	ExperimentID                string          `json:"experiment_id"`
	Seed                        int64           `json:"seed"`
	SimulatedHorizon            string          `json:"simulated_horizon"`
	SimulationStartNano         int64           `json:"simulation_start_nano"`
	SimulationEndNano           int64           `json:"simulation_end_nano"`
	ConfigSHA256                string          `json:"config_sha256"`
	BinarySHA256                string          `json:"binary_sha256"`
	TreeRevision                string          `json:"tree_revision"`
	PlanSHA256                  string          `json:"plan_sha256"`
	BinaryPath                  string          `json:"binary_path"`
	BinaryGoVersion             string          `json:"binary_go_version"`
	BinaryGOOS                  string          `json:"binary_goos"`
	BinaryGOARCH                string          `json:"binary_goarch"`
	BinaryGOAMD64               string          `json:"binary_goamd64"`
	GitRevision                 string          `json:"git_revision"`
	ConfigExperimentID          string          `json:"config_experiment_id"`
	HypothesisID                string          `json:"hypothesis_id"`
	AnalyzerSHA256              string          `json:"analyzer_sha256"`
	RendererSHA256              string          `json:"renderer_sha256"`
	RunnerSHA256                string          `json:"runner_sha256"`
	ReviewAttestationSHA256     string          `json:"review_attestation_sha256"`
	ReviewReportSHA256          string          `json:"review_report_sha256"`
	CapacityAttestationSHA256   string          `json:"capacity_attestation_sha256"`
	CapacityRecordsSHA256       string          `json:"capacity_records_sha256"`
	TrustedReviewKeySHA256      string          `json:"trusted_review_key_sha256"`
	LogMode                     string          `json:"log_mode"`
	EvidenceFormat              string          `json:"evidence_format"`
	EvidenceSchemaEpoch         uint32          `json:"evidence_schema_epoch"`
	GOMAXPROCS                  int             `json:"gomaxprocs"`
	GOMEMLIMIT                  string          `json:"gomemlimit"`
	OutputDir                   string          `json:"output_dir"`
	Holdout                     bool            `json:"holdout"`
	Command                     []string        `json:"command"`
	RawLogPolicy                string          `json:"raw_log_policy"`
	VenueIDs                    []string        `json:"venue_ids"`
	CheckpointValidatorPath     string          `json:"checkpoint_validator_path"`
	CheckpointValidatorRevision string          `json:"checkpoint_validator_revision"`
	CheckpointValidatorSHA256   string          `json:"checkpoint_validator_sha256"`
	ReviewAttestationPath       string          `json:"review_attestation_path"`
	ReviewReportPath            string          `json:"review_report_path"`
	ResourcePolicy              json.RawMessage `json:"resource_policy"`
	SourceModified              bool            `json:"-"`
}

type cdfParticipantKey struct {
	venueID  string
	clientID uint64
}

type cdfSupplierState struct {
	audit                  CDFSupplierActivationAudit
	contract               CDFSupplierContract
	lastFillAt             int64
	lastFillPosition       int64
	currentPosition        int64
	fillBaseDelta          int64
	fillQuoteDelta         int64
	exchangeBaseDelta      int64
	exchangeQuoteDelta     int64
	initialBaseBalance     int64
	initialQuoteBalance    int64
	terminalBaseBalance    int64
	terminalQuoteBalance   int64
	reconstructedReference int64
	referenceUpdatedAt     int64
	referenceUpdateSet     bool
	reconstructedRiskMark  int64
	reconstructedEquity    int64
	reconstructedPeak      int64
	equityStateSet         bool
	lastDecision           cdfDecisionEvidence
	hasLastDecision        bool
	fillResponses          []cdfFillResponseWindow
	pendingReprice         bool
	pendingRepriceOrderID  uint64
	pendingRepriceSide     string
	pendingRepricePrice    int64
	pendingRepriceQty      int64
	initialAccountSeen     bool
	terminalAccountSeen    bool
}

type cdfFillResponseWindow struct {
	fillAt                   int64
	fillGlobalSeq            uint64
	positionAfter            int64
	preFillDecision          cdfDecisionEvidence
	preFillKnown             bool
	requiresFreshObservation bool
	responded                bool
}

type cdfDecisionEvidence struct {
	Role                           string `json:"role"`
	ClientID                       uint64 `json:"client_id"`
	Symbol                         string `json:"symbol"`
	DecisionTime                   int64  `json:"decision_time"`
	DecisionPhaseOffset            int64  `json:"decision_phase_offset_nanos"`
	ObservationTime                int64  `json:"observation_time"`
	ObservationAge                 int64  `json:"observation_age"`
	ObservationSequence            uint64 `json:"observation_sequence"`
	ObservationLinkID              uint32 `json:"observation_link_id"`
	ObservationOrdinal             uint64 `json:"observation_ordinal"`
	ObservationDeliveredAt         int64  `json:"observation_delivered_at"`
	ObservationFingerprint         string `json:"observation_fingerprint"`
	ObservationDigest              string `json:"observation_digest"`
	BestBid                        int64  `json:"best_bid"`
	BestBidQty                     int64  `json:"best_bid_qty"`
	BestAsk                        int64  `json:"best_ask"`
	BestAskQty                     int64  `json:"best_ask_qty"`
	MarkPrice                      int64  `json:"mark_price"`
	RiskMarkPrice                  int64  `json:"risk_mark_price"`
	RiskMarkCurrent                bool   `json:"risk_mark_current"`
	LocalBookMode                  string `json:"local_book_mode"`
	QuotePriceSource               string `json:"quote_price_source"`
	RiskMarkSource                 string `json:"risk_mark_source"`
	ReferencePrice                 int64  `json:"reference_price"`
	Position                       int64  `json:"position"`
	TargetPosition                 int64  `json:"target_position"`
	InventoryLimit                 int64  `json:"inventory_limit"`
	InitialBaseBalance             int64  `json:"initial_base_balance"`
	GrossInventory                 int64  `json:"gross_inventory"`
	GrossInventoryLimit            int64  `json:"gross_inventory_limit"`
	Action                         string `json:"action"`
	Reason                         string `json:"reason"`
	Side                           string `json:"side"`
	QuotePrice                     int64  `json:"quote_price"`
	QuoteQty                       int64  `json:"quote_qty"`
	MinimumQualifyingQty           int64  `json:"minimum_qualifying_qty"`
	RegisteredMinimumExecutableQty int64  `json:"registered_minimum_executable_qty"`
	QuoteOrderID                   uint64 `json:"quote_order_id"`
	QuoteRequestID                 uint64 `json:"quote_request_id"`
	CancelRequestID                uint64 `json:"cancel_request_id"`
	ReplacesOrderID                uint64 `json:"replaces_order_id"`
	QuoteSubmittedAt               int64  `json:"quote_submitted_at"`
	QuoteCashAvailable             int64  `json:"quote_cash_available"`
	QuoteCashReserved              int64  `json:"quote_cash_reserved"`
	QuoteCashRequired              int64  `json:"quote_cash_required"`
	InitialEquityQuote             int64  `json:"initial_equity_quote"`
	EquityQuote                    int64  `json:"equity_quote"`
	PeakEquityQuote                int64  `json:"peak_equity_quote"`
	LossFromInitialQuote           int64  `json:"loss_from_initial_quote"`
	DrawdownQuote                  int64  `json:"drawdown_quote"`
	MaxLossQuote                   int64  `json:"max_loss_quote"`
	EquityAvailable                bool   `json:"equity_available"`
	RiskLimitTriggered             bool   `json:"risk_limit_triggered"`
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

type cdfOrderFillEvidence struct {
	OrderID      uint64 `json:"order_id"`
	TradeID      uint64 `json:"trade_id"`
	Side         string `json:"side"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	FeeAmount    int64  `json:"fee_amount"`
	FeeAsset     string `json:"fee_asset"`
	FilledQty    int64  `json:"filled_qty"`
	RemainingQty int64  `json:"remaining_qty"`
	IsFull       bool   `json:"is_full"`
}

type cdfCancelledEvidence struct {
	OrderID      uint64 `json:"order_id"`
	RequestID    uint64 `json:"request_id"`
	RemainingQty int64  `json:"remaining_qty"`
	Reason       string `json:"reason"`
}

type cdfCancelRejectedEvidence struct {
	OrderID   uint64 `json:"order_id"`
	RequestID uint64 `json:"request_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error"`
}

type cdfRejectedEvidence struct {
	RequestID uint64 `json:"request_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error"`
}

type cdfTradeEvidence struct {
	TradeID        uint64 `json:"trade_id"`
	Price          int64  `json:"price"`
	Qty            int64  `json:"qty"`
	Side           string `json:"side"`
	TakerOrderID   uint64 `json:"taker_order_id"`
	MakerOrderID   uint64 `json:"maker_order_id"`
	globalSequence uint64
}

type cdfBalanceEvidence struct {
	Asset    string `json:"asset"`
	Free     int64  `json:"free"`
	Locked   int64  `json:"locked"`
	Borrowed int64  `json:"borrowed"`
	Interest int64  `json:"interest"`
	NetAsset int64  `json:"net_asset"`
}

type cdfBalanceSnapshotEvidence struct {
	Timestamp    int64                `json:"timestamp"`
	ClientID     uint64               `json:"client_id"`
	SpotBalances []cdfBalanceEvidence `json:"spot_balances"`
	PerpBalances []cdfBalanceEvidence `json:"perp_balances"`
	Borrowed     map[string]int64     `json:"borrowed"`
}

type cdfBorrowEvidence struct {
	ClientID uint64 `json:"client_id"`
	Asset    string `json:"asset"`
	Amount   int64  `json:"amount"`
}

type cdfPublicSnapshotEvidence struct {
	Bids           []etypes.PriceLevel `json:"bids"`
	Asks           []etypes.PriceLevel `json:"asks"`
	SourceSequence uint64              `json:"source_sequence"`
	PublicBids     []etypes.PriceLevel `json:"public_bids"`
	PublicAsks     []etypes.PriceLevel `json:"public_asks"`
}

type cdfBookDeltaEvidence struct {
	Side       string `json:"side"`
	Price      int64  `json:"price"`
	VisibleQty int64  `json:"visible_qty"`
	HiddenQty  int64  `json:"hidden_qty"`
}

type cdfPublicDepthState struct {
	initialized bool
	bids        map[int64]int64
	asks        map[int64]int64
}

type cdfPendingDepthObservation struct {
	event           Event
	state           cdfPublicDepthState
	causalOrders    map[cdfOrderKey]int64
	side            string
	price           int64
	previousVisible int64
	newVisible      int64
	isSnapshot      bool
}

type cdfReceiptKey struct {
	clientID uint64
	linkID   uint32
	ordinal  uint64
}

type cdfReceiptProof struct {
	sourceVenue string
	role        string
	symbol      string
	record      observationRecord
	digest      [16]byte
}

type cdfRequestKey struct {
	venueID   string
	clientID  uint64
	requestID uint64
}

type cdfOrderKey struct {
	venueID  string
	clientID uint64
	orderID  uint64
}

type cdfTradeKey struct {
	venueID string
	tradeID uint64
}

type cdfFillKey struct {
	venueID  string
	clientID uint64
	orderID  uint64
	tradeID  uint64
}

type cdfGatewayDecision struct {
	sourceVenue string
	symbol      string
	record      decisionRecord
}

type cdfReceiptIndex struct {
	receipts  map[cdfReceiptKey]cdfReceiptProof
	decisions map[cdfRequestKey]cdfGatewayDecision
}

type cdfSnapshotKey struct {
	venueID     string
	sequence    uint64
	fingerprint [16]byte
}

type cdfSnapshotProof struct {
	publishedAt    int64
	globalSequence uint64
	bids           []etypes.PriceLevel
	asks           []etypes.PriceLevel
}

type cdfSubmission struct {
	event    Event
	decision cdfDecisionEvidence
	accepted bool
	rejected bool
}

type cdfWithdrawal struct {
	event          Event
	decision       cdfDecisionEvidence
	closed         bool
	cancelRejected bool
}

type cdfOrderState struct {
	requestID         uint64
	side              string
	price             int64
	originalQty       int64
	filledQty         int64
	remainingQty      int64
	acceptedAt        int64
	acceptedGlobalSeq uint64
	oneSidedCandidate bool
	minimumQualifying int64
	restored          bool
	decisionAction    string
	decisionReason    string
	fillCount         int64
	firstFillAt       int64
	terminalState     string
}

type cdfDepthObservation struct {
	at                 int64
	globalSequence     uint64
	snapshot           bool
	bidDepth           int64
	askDepth           int64
	supplierBid        int64
	supplierAsk        int64
	supplierDepthByKey map[cdfParticipantKey]cdfSupplierDepth
}

type cdfSupplierDepth struct {
	bid int64
	ask int64
}

// AuditCDFLiquidityActivation validates a complete treatment run without
// interpreting holdouts or changing the simulator trajectory.
func (r *Run) AuditCDFLiquidityActivation(options CDFActivationOptions) (*CDFActivationAudit, error) {
	if r == nil {
		return nil, fmt.Errorf("cdf activation: nil run")
	}
	if err := options.Contract.validate(); err != nil {
		return nil, fmt.Errorf("cdf activation contract: %w", err)
	}
	evidenceDir := options.EvidenceDir
	if evidenceDir == "" {
		evidenceDir = r.Dir
	}
	if !options.AllowLegacyJSON && !sameCDFPath(r.Dir, evidenceDir) {
		return nil, fmt.Errorf("cdf activation: report and evidence directories must be identical in strict mode")
	}
	scanRun, err := cdfActivationScanRun(r, options.RenderedEvidenceDir)
	if err != nil {
		return nil, err
	}
	result := &CDFActivationAudit{
		trades:              make(map[cdfTradeKey]cdfTradeEvidence),
		totalVolumeByVenue:  make(map[string]int64),
		terminalOrders:      make(map[cdfOrderKey]*cdfOrderState),
		liveOrderBySupplier: make(map[cdfParticipantKey]cdfOrderKey),
		observedFillGlobal:  make(map[cdfFillKey]uint64),
		actualFillGlobal:    make(map[cdfFillKey]uint64),
		tradeGlobal:         make(map[cdfTradeKey]uint64),
	}
	config, metadata, err := loadCDFActivationIdentity(evidenceDir, !options.AllowLegacyJSON)
	if err != nil {
		return nil, err
	}
	strictMechanics := config.EvidenceFormat == "evstream_v3" && config.EvidenceContractVersion >= 2 && !options.AllowLegacyJSON
	if strictMechanics {
		if err := options.ExpectedProvenance.validate(); err != nil {
			return nil, fmt.Errorf("cdf activation expected provenance: %w", err)
		}
		if err := validateCDFExpectedProvenance(metadata, options.ExpectedProvenance); err != nil {
			return nil, err
		}
	}
	if config.EvidenceFormat == "evstream_v3" && config.EvidenceContractVersion >= 2 {
		eventsPath := filepath.Join(evidenceDir, "events.evs")
		_, eventsErr := os.Stat(eventsPath)
		sourceStreamPresent := eventsErr == nil
		if eventsErr != nil && !os.IsNotExist(eventsErr) {
			return nil, fmt.Errorf("cdf activation: inspect binary evidence stream: %w", eventsErr)
		}
		if !sourceStreamPresent {
			if !options.AllowLegacyJSON {
				return nil, fmt.Errorf("cdf activation: v2 audit requires the canonical binary stream")
			}
		} else {
			if options.RenderedEvidenceDir == "" {
				return nil, fmt.Errorf("cdf activation: v2 audit requires independently rendered binary evidence")
			}
			if err := validateCDFCompletionArtifacts(evidenceDir, metadata, options.Contract.BinarySchemaEpoch); err != nil {
				return nil, err
			}
			if err := validateCDFRenderedGlobalSequence(scanRun, evidenceDir, options.RenderedEvidenceDir, options.Contract.BinarySchemaEpoch); err != nil {
				return nil, err
			}
		}
	}
	result.strictMechanics = strictMechanics
	result.Provenance = CDFActivationProvenance{
		ConfigSHA256: metadata.ConfigSHA256, SourceRevision: metadata.GitRevision,
		SourceModified: metadata.SourceModified, BinarySHA256: metadata.BinarySHA256, Seed: metadata.Seed,
		Horizon: metadata.SimulatedHorizon, SimulationStartNano: metadata.SimulationStartNano,
		SimulationEndNano: metadata.SimulationEndNano, VenueIDs: append([]string(nil), config.VenueIDs...),
		ExperimentID: config.ExperimentID, HypothesisID: config.HypothesisID,
		EvidenceFormat: config.EvidenceFormat, LogMode: config.LogMode,
	}
	result.validateConfiguration(config, metadata, options.Contract)
	receipts, receiptAudit, err := loadCDFReceiptIndex(evidenceDir)
	if err != nil {
		return nil, fmt.Errorf("cdf activation receipts: %w", err)
	}
	if !receiptAudit.Valid {
		if strictMechanics {
			return nil, fmt.Errorf("cdf activation: market-data receipt contract is invalid")
		}
		result.addCheck(CDFActivationCheck{Failure: "market-data receipt contract is invalid"})
	}
	states := result.indexCDFAccounts(r.Report, config, options.Contract)
	snapshots, depth, err := result.indexCDFSnapshots(scanRun)
	if err != nil {
		return nil, err
	}
	submissions := make(map[cdfRequestKey]*cdfSubmission)
	withdrawals := make(map[cdfRequestKey]*cdfWithdrawal)
	observedFills := make(map[cdfFillKey]cdfFillEvidence)
	actualFills := make(map[cdfFillKey]cdfOrderFillEvidence)
	orders := make(map[cdfOrderKey]*cdfOrderState)
	if result.strictMechanics {
		orderedEvents, err := collectCDFOrderedEvents(scanRun)
		if err != nil {
			return nil, err
		}
		if err := result.scanCDFOrdered(orderedEvents, states, receipts, snapshots, submissions, withdrawals, observedFills, orders, actualFills, depth); err != nil {
			return nil, err
		}
	} else {
		if err := result.scanCDFGeneral(scanRun, states, receipts, snapshots, submissions, withdrawals, observedFills); err != nil {
			return nil, err
		}
		if err := result.scanCDFBooks(scanRun, states, submissions, withdrawals, orders, actualFills, depth); err != nil {
			return nil, err
		}
	}
	result.reconcileCDFFills(states, observedFills, actualFills, orders, metadata.SimulationEndNano)
	result.finalizeCDFActivation(states, submissions, withdrawals, depth, metadata.SimulationEndNano, options.Contract)
	return result, nil
}

func (c CDFActivationContract) validate() error {
	if c.HypothesisID == "" || c.ExperimentID == "" || c.Seed == 0 || c.Horizon == "" {
		return fmt.Errorf("identity, seed, and horizon are required")
	}
	if c.SimulationStartNano <= 0 || c.SimulationEndNano <= c.SimulationStartNano || c.ObservationIntervalNano <= 0 {
		return fmt.Errorf("invalid simulation interval")
	}
	if c.InitialPublicBookMode != "empty" {
		return fmt.Errorf("initial public CDF book mode must be empty")
	}
	if len(c.VenueIDs) == 0 || c.HistoricalSupplierCountPerVenue < 0 || len(c.Suppliers) == 0 {
		return fmt.Errorf("venue and supplier rosters are required")
	}
	if c.BinarySchemaEpoch == 0 {
		return fmt.Errorf("binary schema epoch is required")
	}
	if c.MaximumSupplierVolumeShare <= 0 || c.MaximumSupplierVolumeShare > 1 ||
		c.MaximumSupplierDepthShare <= 0 || c.MaximumSupplierDepthShare > 1 ||
		c.MaximumDepthDominanceTimeFraction <= 0 || c.MaximumDepthDominanceTimeFraction > 1 {
		return fmt.Errorf("concentration thresholds must be in (0,1]")
	}
	venues := make(map[string]struct{}, len(c.VenueIDs))
	for _, venueID := range c.VenueIDs {
		if venueID == "" {
			return fmt.Errorf("empty venue ID")
		}
		if _, duplicate := venues[venueID]; duplicate {
			return fmt.Errorf("duplicate venue ID %q", venueID)
		}
		venues[venueID] = struct{}{}
	}
	roles := make(map[string]struct{}, len(c.Suppliers))
	for _, supplier := range c.Suppliers {
		if !isNumberedRole(supplier.Role, "cdf_elastic_supplier_") || supplier.Symbol != cdfActivationSymbol ||
			supplier.BaseAsset != "CDF" || supplier.QuoteAsset != "USD" || supplier.BasePrecision <= 0 ||
			supplier.QuotePrecision <= 0 || supplier.InitialBaseBalance <= 0 || supplier.InitialQuoteBalance <= 0 ||
			supplier.Interval <= 0 || supplier.MaxObservationAge <= 0 || supplier.ReferencePrice <= 0 ||
			supplier.ReferenceHalfLife <= 0 || supplier.MaxPosition <= 0 || supplier.MaxInventory <= 0 ||
			supplier.MaxQuoteQty <= 0 || supplier.MinimumExecutableQty <= 0 ||
			supplier.MinimumQualifyingQty <= supplier.MinimumExecutableQty || supplier.TickSize <= 0 ||
			supplier.RegisteredMinimumExecutableQty != supplier.MinimumExecutableQty ||
			!supplier.QuoteOnOneSidedLocalBook || supplier.MaxLossQuote <= 0 || supplier.MakerFeeBps < 0 {
			return fmt.Errorf("invalid finite supplier contract for %q", supplier.Role)
		}
		if _, duplicate := roles[supplier.Role]; duplicate {
			return fmt.Errorf("duplicate supplier role %q", supplier.Role)
		}
		roles[supplier.Role] = struct{}{}
	}
	return nil
}

func cdfActivationScanRun(run *Run, renderedDir string) (*Run, error) {
	if renderedDir == "" {
		return run, nil
	}
	clone := *run
	clone.files = nil
	venueRoot := filepath.Join(renderedDir, "venues")
	if err := filepath.WalkDir(venueRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".jsonl") {
			clone.files = append(clone.files, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("cdf activation: index rendered evidence: %w", err)
	}
	sort.Strings(clone.files)
	return &clone, nil
}

var cdfStrictActivationConfigRequiredFields = []string{
	"venue_ids", "seed", "log_mode", "evidence_format", "evidence_contract_version",
	"experiment_id", "hypothesis_id", "strict_population_accounting", "strict_risk_contract",
	"auto_borrow_spot", "cross_asset_spot_graph", "cross_asset_collateral_marks",
	"record_market_data_receipts", "market_data_receipt_roles",
}

var cdfStrictActivationMetadataRequiredFields = []string{
	"schema_version", "runner_contract", "probe_id", "arm", "experiment_id", "config_experiment_id",
	"hypothesis_id", "seed", "simulated_horizon", "simulation_start_nano", "simulation_end_nano",
	"config_sha256", "binary_sha256", "git_revision", "tree_revision", "binary_path", "binary_go_version",
	"binary_goos", "binary_goarch", "binary_goamd64", "analyzer_sha256", "renderer_sha256", "runner_sha256",
	"review_attestation_sha256", "review_report_sha256", "capacity_attestation_sha256", "capacity_records_sha256",
	"trusted_review_key_sha256", "log_mode", "evidence_format", "evidence_schema_epoch", "gomaxprocs",
	"gomemlimit", "output_dir", "holdout", "command", "raw_log_policy",
}

var cdfKnownActivationConfigFields = func() map[string]struct{} {
	fields := strings.Fields(`
		log_dir experiment_id hypothesis_id date status description log_mode evidence_format evidence_contract_version
		dated_future_delivery_fee_policy record_market_data_receipts market_data_receipt_roles
		record_decision_frontier_vectors record_maker_quote_size_decisions record_maker_inventory_rebalance_decisions
		record_perp_maker_replenishment_decisions record_liability_hedger_decisions record_noise_flow_phase_decisions
		record_funding_carry_decisions record_term_carry_decisions record_dated_execution_mandate_decisions
		record_dated_term_carry_decisions record_perp_exposure_hedger_decisions record_option_liability_user_decisions
		record_elastic_liquidity_supplier_decisions checkpoint_interval_seconds trace_from_nano trace_to_nano
		seed venue_ids strict_population_accounting strict_risk_contract auto_borrow_spot venue_rules
		cross_asset_spot_graph cross_asset_collateral_marks step snapshot_interval automation_interval quote_interval
		noise_interval noise_flow_decision_phase_offset greek_interval noise_trader_count option_flow_count
		stoikov_max_variance_multiple stoikov_volatility_sample_interval spot_tick_quote_units maker_anchor
		spot_maker_local_reference_cache remote_maker_feed remote_maker_feeds round_trip_trader_count round_trip_hold
		round_trip_lot_qty maker_forward_half_life maker_quote_size_vol_elasticity maker_min_quote_size_fraction
		elastic_supplier_reference_half_life noise_order_qty noise_target_qty_by_symbol noise_funding_lots
		noise_size_pareto_alpha noise_size_cap_multiple noise_imbalance_coupling noise_excite_alpha noise_excite_beta_per_sec
		round_trip_inventory_lots elastic_supplier_count elastic_supplier_units_per_percent carry_arbitrageur_count
		carry_entry_bps carry_exit_bps carry_max_position carry_lot_qty perp_maker_inventory_limit perp_maker_replenish_below_bps
		funding_interval_seconds funding_max_rate_bps option_dealer_count option_liability_user dated_carry_arb_count
		parity_arb_count dated_carry_edge_bps dated_carry_slippage_bps dated_carry_check_interval parity_edge_bps
		dated_carry_scale_edge futures_maker_count option_flow_include_futures futures_maker_self_anchored degraded_index
		taker_fee_bps rate_limit_tiers fixed_distance_maker_count fixed_distance_maker imbalance_maker_count imbalance_maker
		triangle_arb_count triangle_arb bootstrap_depth_count bootstrap_depth spot_maker_requote_bps spot_maker_requote_bps_tiers
		spot_maker_submit_before_cancel spot_passive_maker_post_only spot_passive_maker_cancel_before_replace spot_maker_count
		maker_quote_qty maker_hedge_symbol maker_hedge_band_qty maker_hedge_slippage_bps maker_inventory_limit
		maker_min_half_spread_ticks maker_hedge_interval maker_inventory_skew_bps spot_stoikov_inventory_size_skew_bps
		cdf_inventory_rebalance cdf_liability_hedger perp_exposure_hedger funding_carry_arbitrageur term_carry_allocator
		dated_future_execution_mandate dated_term_carry_allocator maker_index_weight latent_liquidity_count latent_liquidity
		metaorder_trader_count metaorder_traders short_option_tenor long_option_tenor short_future_tenor long_future_tenor
		r2_expiry_calendar option_iv strikes_per_side strike_step_usd option_max_strikes_per_expiry stoikov_risk_aversion
		stoikov_fill_decay stoikov_variance_per_second stoikov_inventory_horizon stoikov_volatility_half_life
		option_buy_probability future_flow_count future_flow_lot_qty future_flow_interval vanna_volga_desk_count
		vanna_volga_vega_tolerance vanna_volga_vanna_tolerance vanna_volga_volga_tolerance vanna_volga_lot_qty
		vanna_volga_max_contracts vanna_volga_interval vanna_volga_vol latency_profiles default_latency_profile
		 elastic_supplier_symbols elastic_liquidity_suppliers fixed_distance_maker_symbols imbalance_maker_symbols
		option_dealer_vol option_dealer_hedge_policies option_dealer_hedge_interval_seconds option_value_taker_count
		option_value_taker_edge_bps option_value_taker_lot_qty option_value_taker_max_position option_value_taker_interval
		option_value_taker_vol dealer_hedge_mode cross_venue_arb_tiers cross_venue_base_latency cross_venue_arb_lot_qty
		cross_venue_arb_max_attempts`)
	known := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		known[field] = struct{}{}
	}
	return known
}()

func loadCDFActivationIdentity(dir string, strict bool) (cdfActivationConfig, cdfActivationMetadata, error) {
	readArtifact := func(name string) ([]byte, error) {
		path := filepath.Join(dir, name)
		if strict {
			return readSV1DRegularFile(path)
		}
		return os.ReadFile(path)
	}
	manifestRaw, err := readArtifact("manifest.json")
	if err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: read manifest: %w", err)
	}
	var manifest cdfActivationManifest
	if strict {
		if err := decodeSV1DJSONWithRequiredFields(manifestRaw, &manifest, "schema_version", "config", "venue_ids", "build", "notes"); err != nil {
			return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: decode manifest: %w", err)
		}
		var manifestFields map[string]json.RawMessage
		if err := decodeSV1DJSONWithRequiredFields(manifestRaw, &manifestFields); err != nil {
			return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: inspect manifest: %w", err)
		}
		if err := decodeSV1DJSONWithRequiredFields(manifestFields["build"], &manifest.Build, "revision", "time", "modified", "goos", "goarch", "goamd64"); err != nil {
			return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: manifest build: %w", err)
		}
	} else if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: decode manifest: %w", err)
	}
	if len(manifest.Config) == 0 || string(manifest.Config) == "null" {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: manifest has no config")
	}
	config, err := decodeCDFActivationConfig(manifest.Config, strict)
	if err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: decode config: %w", err)
	}
	runConfigRaw, err := readArtifact("run-config.json")
	if err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: read run config: %w", err)
	}
	if _, err := decodeCDFActivationConfig(runConfigRaw, strict); err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: decode copied run config: %w", err)
	}
	manifestCanonical, err := canonicalCDFActivationJSON(manifest.Config)
	if err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, err
	}
	runCanonical, err := canonicalCDFActivationJSON(runConfigRaw)
	if err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, err
	}
	if manifestCanonical != runCanonical {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: manifest and copied run config differ")
	}
	metadataRaw, err := readArtifact("run-metadata.json")
	if err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: read run metadata: %w", err)
	}
	var metadata cdfActivationMetadata
	if strict {
		if err := decodeSV1DJSONWithRequiredFields(metadataRaw, &metadata, cdfStrictActivationMetadataRequiredFields...); err != nil {
			return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: decode run metadata: %w", err)
		}
	} else if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: decode run metadata: %w", err)
	}
	configDigest := sha256.Sum256(runConfigRaw)
	if metadata.ConfigSHA256 != hex.EncodeToString(configDigest[:]) {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: metadata config hash mismatch")
	}
	if strict && (manifest.SchemaVersion != 2 || metadata.SchemaVersion != 2 || metadata.RunnerContract != "v2-r2-sv1d-activation-runner-v2" ||
		metadata.ProbeID != "v2-r2-sv1d-activation-659" || metadata.EvidenceFormat != "evstream_v3" || metadata.EvidenceSchemaEpoch != 4 ||
		metadata.GOMAXPROCS != 2 || metadata.GOMEMLIMIT != "4GiB" || metadata.Holdout) {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: strict run metadata contract is invalid")
	}
	if !isCDFHex(metadata.ConfigSHA256, sha256.Size) || !isCDFHex(metadata.BinarySHA256, sha256.Size) ||
		!isCDFHex(manifest.Build.Revision, 20) || manifest.Build.Modified ||
		manifest.Build.GOOS != "linux" || manifest.Build.GOARCH != "amd64" || manifest.Build.GOAMD64 != "v1" {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: build provenance is not clean linux/amd64/v1")
	}
	if metadata.GitRevision != manifest.Build.Revision || metadata.BinaryGOOS != manifest.Build.GOOS ||
		metadata.BinaryGOARCH != manifest.Build.GOARCH || metadata.BinaryGOAMD64 != manifest.Build.GOAMD64 ||
		metadata.Seed != config.Seed || metadata.ConfigExperimentID != config.ExperimentID ||
		metadata.HypothesisID != config.HypothesisID || metadata.LogMode != config.LogMode ||
		metadata.EvidenceFormat != config.EvidenceFormat || !sameCDFStrings(manifest.VenueIDs, config.VenueIDs) {
		return cdfActivationConfig{}, cdfActivationMetadata{}, fmt.Errorf("cdf activation: metadata, manifest, and config identities disagree")
	}
	metadata.SourceModified = manifest.Build.Modified
	return config, metadata, nil
}

func decodeCDFActivationConfig(raw []byte, strict bool) (cdfActivationConfig, error) {
	if !strict {
		var config cdfActivationConfig
		if err := json.Unmarshal(raw, &config); err != nil {
			return cdfActivationConfig{}, err
		}
		return config, nil
	}
	var fields map[string]json.RawMessage
	if err := decodeSV1DJSONWithRequiredFields(raw, &fields, cdfStrictActivationConfigRequiredFields...); err != nil {
		return cdfActivationConfig{}, err
	}
	for field := range fields {
		if _, known := cdfKnownActivationConfigFields[field]; !known {
			return cdfActivationConfig{}, fmt.Errorf("unknown simulator config field %q", field)
		}
	}
	var config cdfActivationConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return cdfActivationConfig{}, err
	}
	suppliersRaw, present := fields["elastic_liquidity_suppliers"]
	if !present || string(bytes.TrimSpace(suppliersRaw)) == "null" {
		return config, nil
	}
	var supplierRows []json.RawMessage
	if err := decodeSV1DJSONWithRequiredFields(suppliersRaw, &supplierRows); err != nil {
		return cdfActivationConfig{}, fmt.Errorf("decode elastic supplier roster: %w", err)
	}
	for index, supplierRaw := range supplierRows {
		var supplier CDFSupplierContract
		if err := decodeSV1DJSONWithRequiredFields(supplierRaw, &supplier,
			"role", "symbol", "base_asset", "quote_asset", "base_precision", "quote_precision",
			"initial_base_balance", "initial_quote_balance", "interval", "max_observation_age", "reference_price",
			"reference_half_life", "base_holding", "elasticity_per_percent", "max_position", "max_inventory",
			"max_quote_qty", "minimum_executable_qty", "minimum_qualifying_qty", "tick_size",
			"registered_minimum_executable_qty", "quote_on_one_sided_local_book", "max_loss_quote", "maker_fee_bps"); err != nil {
			return cdfActivationConfig{}, fmt.Errorf("decode elastic supplier %d: %w", index, err)
		}
	}
	return config, nil
}

func (p CDFExpectedProvenance) validate() error {
	if !isCDFHex(p.ConfigSHA256, sha256.Size) || !isCDFHex(p.BinarySHA256, sha256.Size) ||
		!isCDFHex(p.SourceRevision, 20) {
		return fmt.Errorf("config, source, and binary identities must be hexadecimal digests")
	}
	if p.BinaryGOOS != "linux" || p.BinaryGOARCH != "amd64" || p.BinaryGOAMD64 != "v1" {
		return fmt.Errorf("binary identity must be linux/amd64/v1")
	}
	return nil
}

func validateCDFExpectedProvenance(metadata cdfActivationMetadata, expected CDFExpectedProvenance) error {
	if metadata.SourceModified || metadata.ConfigSHA256 != expected.ConfigSHA256 ||
		metadata.GitRevision != expected.SourceRevision || metadata.BinarySHA256 != expected.BinarySHA256 ||
		metadata.BinaryGOOS != expected.BinaryGOOS || metadata.BinaryGOARCH != expected.BinaryGOARCH ||
		metadata.BinaryGOAMD64 != expected.BinaryGOAMD64 {
		return fmt.Errorf("cdf activation: run provenance does not match the externally expected clean build identity")
	}
	return nil
}

type cdfBinaryEvidenceAttestation struct {
	Domain               string `json:"domain"`
	Ordering             string `json:"ordering"`
	SchemaEpoch          uint32 `json:"schema_epoch"`
	EventFrames          uint64 `json:"event_frames"`
	StreamFrames         uint64 `json:"stream_frames"`
	ExecutionStreamHash  string `json:"execution_stream_hash"`
	EvidenceOnlyIncluded bool   `json:"evidence_only_in_stream"`
	UnencodablePayloads  uint64 `json:"unencodable_payloads"`
}

type cdfRenderedEvidenceAttestation struct {
	Domain                 string `json:"domain"`
	Ordering               string `json:"ordering"`
	SourceExecutionHash    string `json:"source_execution_stream_hash"`
	SourceEventFrames      uint64 `json:"source_event_frames"`
	SourceStreamFrames     uint64 `json:"source_stream_frames"`
	RenderedDigest         string `json:"rendered_digest"`
	GlobalSequenceIncluded bool   `json:"global_sequence_included"`
}

type cdfRunStatus struct {
	SchemaVersion          int      `json:"schema_version"`
	Contract               string   `json:"contract"`
	ExitStatus             int      `json:"exit_status"`
	CompletionVerified     bool     `json:"completion_verified"`
	SimulatedHorizon       string   `json:"simulated_horizon"`
	SimulationStartNano    int64    `json:"simulation_start_nano"`
	SimulationEndNano      int64    `json:"simulation_end_nano"`
	RunMetadataSHA256      string   `json:"run_metadata_sha256"`
	ManifestSHA256         string   `json:"manifest_sha256"`
	GreeksSHA256           string   `json:"greeks_sha256"`
	LatencySHA256          string   `json:"latency_sha256"`
	CheckpointsSHA256      string   `json:"checkpoints_sha256"`
	EvidenceManifestSHA    string   `json:"evidence_manifest_sha256"`
	BinaryAttestationSHA   string   `json:"binary_evidence_attestation_sha256"`
	MarketDataEvidenceSHA  string   `json:"market_data_evidence_sha256,omitempty"`
	MarketDataSchedulesSHA string   `json:"market_data_schedules_sha256,omitempty"`
	MarketDataReceiptsSHA  string   `json:"market_data_receipts_sha256,omitempty"`
	MarketDataDecisionsSHA string   `json:"market_data_decisions_sha256,omitempty"`
	CompletionSentinels    []string `json:"completion_sentinels"`
}

type cdfGreeksSidecar struct {
	SchemaVersion              int               `json:"schema_version"`
	InitialAccounts            []json.RawMessage `json:"initial_accounts"`
	TerminalAccounts           []json.RawMessage `json:"terminal_accounts"`
	InitialRisk                json.RawMessage   `json:"initial_risk"`
	TerminalRisk               json.RawMessage   `json:"terminal_risk"`
	RiskTimeline               json.RawMessage   `json:"risk_timeline"`
	PreExpiryRisk              json.RawMessage   `json:"pre_expiry_risk"`
	Microstructure             []json.RawMessage `json:"microstructure"`
	Metaorders                 []json.RawMessage `json:"metaorders"`
	CarryActivity              []json.RawMessage `json:"carry_activity"`
	RouterReports              []json.RawMessage `json:"router_reports"`
	VenueLedgers               []json.RawMessage `json:"venue_ledgers"`
	RequestBudgets             []json.RawMessage `json:"request_budgets"`
	Caveats                    []string          `json:"caveats"`
	ReportStatus               json.RawMessage   `json:"report_status"`
	TerminalValuationAvailable *bool             `json:"terminal_valuation_available"`
}

type cdfLatencySidecar struct {
	Domain string          `json:"domain"`
	Rows   []cdfLatencyRow `json:"rows"`
}

type cdfLatencyRow struct {
	Link                    string  `json:"link"`
	Channel                 string  `json:"channel"`
	Scheduled               int64   `json:"scheduled"`
	Delivered               int64   `json:"delivered"`
	Undelivered             int64   `json:"undelivered"`
	MeanDrawnNanoseconds    float64 `json:"mean_drawn_nanoseconds"`
	MeanQueueNanoseconds    float64 `json:"mean_fifo_queue_nanoseconds"`
	MeanDeliveryNanoseconds float64 `json:"mean_delivery_nanoseconds"`
}

type cdfCheckpointSidecar struct {
	Domain              string `json:"domain"`
	Ordering            string `json:"ordering"`
	SimTime             int64  `json:"sim_time"`
	EventCount          int64  `json:"event_count"`
	ExecutionStreamHash string `json:"execution_stream_hash"`
	Rolling             string `json:"rolling_hash"`
	Representation      string `json:"representation"`
	Unencodable         int64  `json:"unencodable_payloads"`
}

type cdfEvidenceManifestRecord struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type cdfEvidenceManifest struct {
	SchemaVersion  int                         `json:"schema_version"`
	Contract       string                      `json:"contract"`
	Cell           string                      `json:"cell"`
	LogMode        string                      `json:"log_mode"`
	EvidenceFormat string                      `json:"evidence_format"`
	SourceRevision string                      `json:"source_revision"`
	FixedFiles     []cdfEvidenceManifestRecord `json:"fixed_files"`
	RawJSONLFiles  int                         `json:"raw_jsonl_files"`
	RawJSONLBytes  int64                       `json:"raw_jsonl_bytes"`
	RawFiles       []cdfEvidenceManifestRecord `json:"raw_files"`
}

func validateCDFCompletionArtifacts(dir string, metadata cdfActivationMetadata, expectedSchemaEpoch uint32) error {
	raw, err := readSV1DRegularFile(filepath.Join(dir, "run-status.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read run status: %w", err)
	}
	var status cdfRunStatus
	if err := decodeSV1DJSONWithRequiredFields(raw, &status,
		"schema_version", "contract", "exit_status", "completion_verified", "simulated_horizon",
		"simulation_start_nano", "simulation_end_nano", "run_metadata_sha256", "manifest_sha256",
		"greeks_sha256", "latency_sha256", "checkpoints_sha256", "evidence_manifest_sha256",
		"binary_evidence_attestation_sha256", "market_data_evidence_sha256", "market_data_schedules_sha256",
		"market_data_receipts_sha256", "market_data_decisions_sha256", "completion_sentinels"); err != nil {
		return fmt.Errorf("cdf activation: decode run status: %w", err)
	}
	if status.SchemaVersion != 1 || status.Contract != "v2-r2-sv1d-arm-status-v2" || status.ExitStatus != 0 || !status.CompletionVerified || status.SimulatedHorizon != metadata.SimulatedHorizon ||
		status.SimulationStartNano != metadata.SimulationStartNano || status.SimulationEndNano != metadata.SimulationEndNano {
		return fmt.Errorf("cdf activation: run status does not attest a complete registered horizon")
	}
	if !sameCDFStrings(status.CompletionSentinels, []string{"greeks.json", "latency.json"}) {
		return fmt.Errorf("cdf activation: run status completion sentinels are incomplete")
	}
	checks := []struct {
		name string
		path string
		want string
	}{
		{"run metadata", "run-metadata.json", status.RunMetadataSHA256},
		{"manifest", "manifest.json", status.ManifestSHA256},
		{"greeks", "greeks.json", status.GreeksSHA256},
		{"latency", "latency.json", status.LatencySHA256},
		{"checkpoints", "checkpoints.jsonl", status.CheckpointsSHA256},
		{"evidence manifest", "evidence-manifest.json", status.EvidenceManifestSHA},
		{"binary evidence attestation", "binary-evidence-attestation.json", status.BinaryAttestationSHA},
		{"market-data evidence manifest", "market-data-evidence-v2.json", status.MarketDataEvidenceSHA},
		{"market-data schedules", "market-data-schedules-v2.bin", status.MarketDataSchedulesSHA},
		{"market-data receipts", "market-data-receipts-v2.bin", status.MarketDataReceiptsSHA},
		{"market-data decisions", "market-data-decisions-v2.bin", status.MarketDataDecisionsSHA},
	}
	for _, check := range checks {
		if !isCDFHex(check.want, sha256.Size) {
			return fmt.Errorf("cdf activation: run status has no valid %s hash", check.name)
		}
		actual, err := sha256File(filepath.Join(dir, check.path))
		if err != nil {
			return fmt.Errorf("cdf activation: hash %s: %w", check.name, err)
		}
		if actual != check.want {
			return fmt.Errorf("cdf activation: run status %s hash mismatch", check.name)
		}
	}
	if err := validateCDFCompletionSidecars(dir, metadata, expectedSchemaEpoch); err != nil {
		return err
	}
	if metadata.BinaryPath == "" {
		return fmt.Errorf("cdf activation: run metadata has no simulator binary path")
	}
	binaryDigest, err := sha256File(metadata.BinaryPath)
	if err != nil {
		return fmt.Errorf("cdf activation: hash simulator binary: %w", err)
	}
	if binaryDigest != metadata.BinarySHA256 {
		return fmt.Errorf("cdf activation: simulator binary hash mismatch")
	}
	return nil
}

func validateCDFCompletionSidecars(dir string, metadata cdfActivationMetadata, expectedSchemaEpoch uint32) error {
	greeksRaw, err := readSV1DRegularFile(filepath.Join(dir, "greeks.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read greeks sidecar: %w", err)
	}
	var greeks cdfGreeksSidecar
	if err := decodeSV1DJSONWithRequiredFields(greeksRaw, &greeks,
		"schema_version", "initial_accounts", "terminal_accounts", "initial_risk",
		"terminal_risk", "risk_timeline", "microstructure"); err != nil {
		return fmt.Errorf("cdf activation: decode greeks sidecar: %w", err)
	}
	if greeks.SchemaVersion < 1 || len(greeks.InitialAccounts) == 0 || len(greeks.TerminalAccounts) == 0 ||
		!cdfJSONObject(greeks.InitialRisk) || !cdfJSONObject(greeks.TerminalRisk) ||
		!cdfJSONObject(greeks.RiskTimeline) || len(greeks.Microstructure) == 0 {
		return fmt.Errorf("cdf activation: greeks sidecar is structurally incomplete")
	}

	latencyRaw, err := readSV1DRegularFile(filepath.Join(dir, "latency.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read latency sidecar: %w", err)
	}
	var latency cdfLatencySidecar
	if err := decodeSV1DJSONWithRequiredFields(latencyRaw, &latency, "domain", "rows"); err != nil || latency.Domain != "courier_delivery" || len(latency.Rows) == 0 {
		return fmt.Errorf("cdf activation: latency sidecar is structurally incomplete")
	}

	attestationRaw, err := readSV1DRegularFile(filepath.Join(dir, "binary-evidence-attestation.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read binary evidence attestation: %w", err)
	}
	var attestation cdfBinaryEvidenceAttestation
	if err := decodeSV1DJSONWithRequiredFields(attestationRaw, &attestation,
		"domain", "ordering", "schema_epoch", "event_frames", "stream_frames", "execution_stream_hash", "evidence_only_in_stream"); err != nil {
		return fmt.Errorf("cdf activation: decode binary evidence attestation: %w", err)
	}
	if attestation.Domain != "canonical_binary_execution_frames" || attestation.Ordering != "ordered_stream" ||
		attestation.SchemaEpoch != expectedSchemaEpoch || expectedSchemaEpoch == 0 ||
		!isCDFHex(attestation.ExecutionStreamHash, sha256.Size) || attestation.EventFrames == 0 ||
		attestation.StreamFrames < attestation.EventFrames || !attestation.EvidenceOnlyIncluded ||
		attestation.UnencodablePayloads != 0 {
		return fmt.Errorf("cdf activation: binary evidence attestation is not a complete v2 successor attestation")
	}

	checkpointRaw, err := readSV1DRegularFile(filepath.Join(dir, "checkpoints.jsonl"))
	if err != nil {
		return fmt.Errorf("cdf activation: open checkpoint sidecar: %w", err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(checkpointRaw))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	checkpointCount := 0
	checkpointsByEventCount := make(map[uint64]string)
	var previousSimTime, previousEventCount int64
	for scanner.Scan() {
		line := scanner.Bytes()
		var checkpoint cdfCheckpointSidecar
		if err := decodeSV1DJSONWithRequiredFields(line, &checkpoint,
			"domain", "ordering", "sim_time", "event_count", "execution_stream_hash", "rolling_hash", "representation"); err != nil || checkpoint.Domain != "execution_observations" ||
			checkpoint.Ordering != "ordered_stream" || checkpoint.SimTime <= 0 || checkpoint.EventCount <= 0 ||
			checkpoint.Representation != "evstream_v3" || checkpoint.Unencodable != 0 ||
			checkpoint.Rolling != checkpoint.ExecutionStreamHash || !isCDFHex(checkpoint.ExecutionStreamHash, sha256.Size) ||
			(checkpointCount > 0 && (checkpoint.SimTime <= previousSimTime || checkpoint.EventCount <= previousEventCount)) {
			return fmt.Errorf("cdf activation: checkpoint sidecar contains an invalid sequence")
		}
		previousSimTime, previousEventCount = checkpoint.SimTime, checkpoint.EventCount
		if uint64(checkpoint.EventCount) > attestation.EventFrames {
			return fmt.Errorf("cdf activation: checkpoint event count exceeds binary attestation")
		}
		checkpointKey := uint64(checkpoint.EventCount)
		if _, duplicate := checkpointsByEventCount[checkpointKey]; duplicate {
			return fmt.Errorf("cdf activation: checkpoint event count is repeated")
		}
		checkpointsByEventCount[checkpointKey] = checkpoint.ExecutionStreamHash
		checkpointCount++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("cdf activation: scan checkpoint sidecar: %w", err)
	}
	if checkpointCount == 0 || previousSimTime != metadata.SimulationEndNano ||
		previousEventCount != int64(attestation.EventFrames) {
		return fmt.Errorf("cdf activation: checkpoint sidecar does not attest the registered terminal horizon")
	}
	var terminalCheckpoint cdfCheckpointSidecar
	checkpointLines := bytes.Split(bytes.TrimSpace(checkpointRaw), []byte{'\n'})
	if len(checkpointLines) == 0 || decodeSV1DJSONWithRequiredFields(checkpointLines[len(checkpointLines)-1], &terminalCheckpoint,
		"domain", "ordering", "sim_time", "event_count", "execution_stream_hash", "rolling_hash", "representation") != nil ||
		terminalCheckpoint.ExecutionStreamHash != attestation.ExecutionStreamHash {
		return fmt.Errorf("cdf activation: terminal checkpoint is not bound to the binary attestation")
	}
	if err := validateCDFCheckpointPrefixes(dir, checkpointsByEventCount, attestation, expectedSchemaEpoch); err != nil {
		return err
	}

	manifestRaw, err := readSV1DRegularFile(filepath.Join(dir, "evidence-manifest.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read evidence manifest sidecar: %w", err)
	}
	var evidenceManifest cdfEvidenceManifest
	if err := decodeSV1DJSONWithRequiredFields(manifestRaw, &evidenceManifest,
		"schema_version", "contract", "cell", "log_mode", "evidence_format", "source_revision", "fixed_files", "raw_jsonl_files", "raw_jsonl_bytes", "raw_files"); err != nil || evidenceManifest.SchemaVersion != 2 ||
		evidenceManifest.Contract != "v2-integrated-longrun-evidence-manifest-v2" ||
		evidenceManifest.EvidenceFormat != "evstream_v3" || evidenceManifest.LogMode != metadata.LogMode ||
		evidenceManifest.SourceRevision != metadata.GitRevision || len(evidenceManifest.FixedFiles) == 0 {
		return fmt.Errorf("cdf activation: evidence manifest sidecar is structurally incomplete")
	}
	fixed := make(map[string]cdfEvidenceManifestRecord, len(evidenceManifest.FixedFiles))
	for _, record := range evidenceManifest.FixedFiles {
		if record.Path == "" || filepath.IsAbs(record.Path) || filepath.Clean(record.Path) != record.Path ||
			strings.HasPrefix(record.Path, "../") || record.Bytes <= 0 || !isCDFHex(record.SHA256, sha256.Size) {
			return fmt.Errorf("cdf activation: evidence manifest contains an invalid fixed-file record")
		}
		if _, duplicate := fixed[record.Path]; duplicate {
			return fmt.Errorf("cdf activation: evidence manifest repeats fixed file %q", record.Path)
		}
		fixed[record.Path] = record
		path := filepath.Join(dir, filepath.FromSlash(record.Path))
		actual, actualBytes, err := hashRegularFile(path)
		if err != nil {
			return fmt.Errorf("cdf activation: evidence manifest fixed file %q is unavailable", record.Path)
		}
		if actualBytes != record.Bytes {
			return fmt.Errorf("cdf activation: evidence manifest byte count mismatch for %q", record.Path)
		}
		if actual != record.SHA256 {
			return fmt.Errorf("cdf activation: evidence manifest digest mismatch for %q", record.Path)
		}
	}
	requiredFiles := []string{
		"run-config.json", "run-metadata.json", "manifest.json", "greeks.json", "latency.json",
		"checkpoints.jsonl", "events.evs", "binary-evidence-attestation.json",
		"market-data-evidence-v2.json", "market-data-schedules-v2.bin",
		"market-data-receipts-v2.bin", "market-data-decisions-v2.bin",
	}
	for _, required := range requiredFiles {
		if _, exists := fixed[required]; !exists {
			return fmt.Errorf("cdf activation: evidence manifest omits required file %q", required)
		}
	}
	if len(fixed) != len(requiredFiles) || evidenceManifest.RawJSONLFiles != 0 || evidenceManifest.RawJSONLBytes != 0 || len(evidenceManifest.RawFiles) != 0 {
		return fmt.Errorf("cdf activation: v2 binary evidence manifest contains legacy raw JSONL evidence")
	}
	venuesDir := filepath.Join(dir, "venues")
	if info, err := os.Lstat(venuesDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("cdf activation: binary evidence venue namespace is a symlink")
		}
		if err := filepath.WalkDir(venuesDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("cdf activation: binary evidence venue namespace contains a symlink")
			}
			if !entry.IsDir() {
				return fmt.Errorf("cdf activation: binary evidence venue namespace contains legacy raw file %q", filepath.Base(path))
			}
			return nil
		}); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cdf activation: inspect binary evidence venue namespace: %w", err)
	}
	return nil
}

func validateCDFCheckpointPrefixes(dir string, checkpoints map[uint64]string, attestation cdfBinaryEvidenceAttestation, expectedSchemaEpoch uint32) error {
	fileDescriptor, absolute, err := openSV1DNoSymlink(filepath.Join(dir, "events.evs"), false)
	if err != nil {
		return fmt.Errorf("cdf activation: open binary evidence for checkpoint validation: %w", err)
	}
	file := os.NewFile(uintptr(fileDescriptor), absolute)
	if file == nil {
		_ = syscall.Close(fileDescriptor)
		return fmt.Errorf("cdf activation: wrap binary evidence for checkpoint validation")
	}
	defer file.Close()
	reader, err := evstream.NewReader(file, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return fmt.Errorf("cdf activation: read binary evidence for checkpoint validation: %w", err)
	}
	if reader.Codec() != evstream.CodecNone || reader.SchemaEpoch() != expectedSchemaEpoch {
		return fmt.Errorf("cdf activation: checkpoint source codec or schema epoch is not registered")
	}
	matched := make(map[uint64]struct{}, len(checkpoints))
	var eventFrames uint64
	if err := reader.Range(func(_ evstream.Frame) error {
		eventFrames++
		if expected, ok := checkpoints[eventFrames]; ok {
			actual := reader.ExecutionHash()
			if hex.EncodeToString(actual[:]) != expected {
				return fmt.Errorf("cdf activation: checkpoint hash does not match binary prefix at event count %d", eventFrames)
			}
			matched[eventFrames] = struct{}{}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("cdf activation: validate checkpoint binary prefixes: %w", err)
	}
	if !reader.Terminated() || eventFrames != attestation.EventFrames || reader.Count() != attestation.StreamFrames {
		return fmt.Errorf("cdf activation: checkpoint source does not match binary attestation")
	}
	if len(matched) != len(checkpoints) {
		return fmt.Errorf("cdf activation: checkpoint stream contains a prefix not present in binary evidence")
	}
	return nil
}

func cdfJSONNumberAtLeast(raw json.RawMessage, minimum int64) bool {
	var value int64
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value >= minimum
}

func cdfJSONArray(raw json.RawMessage) bool {
	var value []json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value != nil
}

func cdfJSONArrayNonEmpty(raw json.RawMessage) bool {
	var value []json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && len(value) > 0
}

func cdfJSONObject(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &value) == nil && value != nil
}

func sha256File(path string) (string, error) {
	digest, _, err := hashRegularFile(path)
	return digest, err
}

func hashRegularFile(path string) (string, int64, error) {
	fileDescriptor, absolute, err := openSV1DNoSymlink(path, false)
	if err != nil {
		return "", 0, err
	}
	file := os.NewFile(uintptr(fileDescriptor), absolute)
	if file == nil {
		_ = syscall.Close(fileDescriptor)
		return "", 0, fmt.Errorf("could not wrap file descriptor for %s", path)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("path is not a regular file: %s", path)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", 0, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return hex.EncodeToString(digest[:]), info.Size(), nil
}

type cdfEvidenceFrameIdentity struct {
	globalSequence uint64
	localSequence  uint64
	simTS          int64
	clientID       uint64
	venueID        string
	route          string
	eventName      string
	payloadDigest  [sha256.Size]byte
}

type cdfRenderedEvidenceSnapshot struct {
	BinaryAttestationRaw   []byte
	EventsRaw              []byte
	RenderedAttestationRaw []byte
	RenderedFiles          map[string][]byte
}

func validateCDFRenderedGlobalSequence(renderedRun *Run, evidenceDir, renderedDir string, expectedSchemaEpoch uint32) error {
	_ = renderedRun
	binaryAttestationRaw, err := readSV1DRegularFile(filepath.Join(evidenceDir, "binary-evidence-attestation.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read binary evidence attestation: %w", err)
	}
	eventsRaw, err := readSV1DRegularFile(filepath.Join(evidenceDir, "events.evs"))
	if err != nil {
		return fmt.Errorf("cdf activation: read binary evidence source: %w", err)
	}
	renderedAttestationRaw, err := readSV1DRegularFile(filepath.Join(renderedDir, "rendered-binary-evidence-attestation.json"))
	if err != nil {
		return fmt.Errorf("cdf activation: read rendered evidence attestation: %w", err)
	}
	renderedFiles, err := snapshotCDFRenderedFiles(renderedDir)
	if err != nil {
		return err
	}
	return validateCDFRenderedGlobalSequenceSnapshot(cdfRenderedEvidenceSnapshot{
		BinaryAttestationRaw: binaryAttestationRaw, EventsRaw: eventsRaw,
		RenderedAttestationRaw: renderedAttestationRaw, RenderedFiles: renderedFiles,
	}, expectedSchemaEpoch)
}

func snapshotCDFRenderedFiles(renderedDir string) (map[string][]byte, error) {
	files := make(map[string][]byte)
	if err := filepath.WalkDir(renderedDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cdf activation: rendered evidence contains a symlink")
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(renderedDir, path)
		if err != nil {
			return err
		}
		raw, err := readSV1DRegularFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = raw
		return nil
	}); err != nil {
		return nil, fmt.Errorf("cdf activation: snapshot rendered evidence: %w", err)
	}
	return files, nil
}

func validateCDFRenderedGlobalSequenceSnapshot(snapshot cdfRenderedEvidenceSnapshot, expectedSchemaEpoch uint32) error {
	var attestation cdfBinaryEvidenceAttestation
	if err := decodeSV1DJSONWithRequiredFields(snapshot.BinaryAttestationRaw, &attestation,
		"domain", "ordering", "schema_epoch", "event_frames", "stream_frames",
		"execution_stream_hash", "evidence_only_in_stream"); err != nil {
		return fmt.Errorf("cdf activation: decode binary evidence attestation: %w", err)
	}
	if attestation.Domain != "canonical_binary_execution_frames" || attestation.Ordering != "ordered_stream" ||
		!isCDFHex(attestation.ExecutionStreamHash, sha256.Size) || attestation.EventFrames == 0 ||
		attestation.StreamFrames < attestation.EventFrames || !attestation.EvidenceOnlyIncluded ||
		attestation.UnencodablePayloads != 0 || expectedSchemaEpoch == 0 || attestation.SchemaEpoch != expectedSchemaEpoch {
		return fmt.Errorf("cdf activation: binary evidence attestation is not a complete v2 successor attestation")
	}
	sourceReader, err := evstream.NewReader(bytes.NewReader(snapshot.EventsRaw), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return fmt.Errorf("cdf activation: read binary evidence source: %w", err)
	}
	if sourceReader.Codec() != evstream.CodecNone || sourceReader.SchemaEpoch() != expectedSchemaEpoch {
		return fmt.Errorf("cdf activation: binary evidence source codec or schema epoch is not the registered contract")
	}
	sourceByGlobal := make(map[uint64]cdfEvidenceFrameIdentity)
	var sourceEventCount uint64
	if err := sourceReader.Range(func(frame evstream.Frame) error {
		if len(frame.Payload) < 16+sha256.Size {
			return fmt.Errorf("cdf activation: source frame %d payload is shorter than the epoch-4 envelope", frame.Header.Seq)
		}
		routeRef := binary.LittleEndian.Uint32(frame.Payload[0:4])
		eventRef := binary.LittleEndian.Uint32(frame.Payload[4:8])
		localSequence := binary.LittleEndian.Uint64(frame.Payload[8:16])
		var payloadDigest [sha256.Size]byte
		copy(payloadDigest[:], frame.Payload[16:16+sha256.Size])
		if routeRef == 0 || eventRef == 0 || localSequence == 0 || frame.Header.Seq == 0 || frame.Venue == "" {
			return fmt.Errorf("cdf activation: source frame %d has incomplete identity envelope", frame.Header.Seq)
		}
		route, routeOK := sourceReader.Lookup(routeRef)
		eventName, eventOK := sourceReader.Lookup(eventRef)
		if !routeOK || !eventOK || route == "" || eventName == "" {
			return fmt.Errorf("cdf activation: source frame %d has unresolved route or event identity", frame.Header.Seq)
		}
		if _, duplicate := sourceByGlobal[frame.Header.Seq]; duplicate {
			return fmt.Errorf("cdf activation: source has duplicate global frame sequence %d", frame.Header.Seq)
		}
		sourceByGlobal[frame.Header.Seq] = cdfEvidenceFrameIdentity{
			globalSequence: frame.Header.Seq, localSequence: localSequence,
			simTS: frame.Header.SimTS, clientID: frame.Header.ClientID,
			venueID: frame.Venue, route: filepath.ToSlash(route), eventName: eventName,
			payloadDigest: payloadDigest,
		}
		sourceEventCount++
		return nil
	}); err != nil {
		return fmt.Errorf("cdf activation: verify binary evidence source: %w", err)
	}
	if !sourceReader.Terminated() || sourceReader.Count() != attestation.StreamFrames || sourceEventCount != attestation.EventFrames {
		return fmt.Errorf("cdf activation: binary source counts or completion trailer disagree with attestation")
	}
	sourceDigest := sourceReader.ExecutionHash()
	if hex.EncodeToString(sourceDigest[:]) != attestation.ExecutionStreamHash {
		return fmt.Errorf("cdf activation: binary source hash disagrees with attestation")
	}
	var rendered cdfRenderedEvidenceAttestation
	if err := decodeSV1DJSONWithRequiredFields(snapshot.RenderedAttestationRaw, &rendered,
		"domain", "ordering", "source_execution_stream_hash", "source_event_frames",
		"source_stream_frames", "rendered_digest", "global_sequence_included"); err != nil {
		return fmt.Errorf("cdf activation: decode rendered evidence attestation: %w", err)
	}
	if rendered.Domain != "rendered_binary_evidence" ||
		rendered.Ordering != "venue_sequence_files_with_global_frame_identity" ||
		rendered.SourceExecutionHash != attestation.ExecutionStreamHash ||
		rendered.SourceEventFrames != attestation.EventFrames ||
		rendered.SourceStreamFrames != attestation.StreamFrames ||
		!rendered.GlobalSequenceIncluded || !isCDFHex(rendered.RenderedDigest, sha256.Size) {
		return fmt.Errorf("cdf activation: rendered evidence attestation is not bound to the binary source")
	}
	actualRenderedDigest, err := digestRenderedEvidenceSnapshot(snapshot.RenderedFiles)
	if err != nil {
		return fmt.Errorf("cdf activation: digest rendered evidence: %w", err)
	}
	if actualRenderedDigest != rendered.RenderedDigest {
		return fmt.Errorf("cdf activation: rendered evidence digest mismatch")
	}
	renderedByGlobal := make(map[uint64]cdfEvidenceFrameIdentity)
	paths := make([]string, 0, len(snapshot.RenderedFiles))
	for path := range snapshot.RenderedFiles {
		if strings.HasPrefix(path, "venues/") && strings.HasSuffix(path, ".jsonl") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		relative := strings.TrimPrefix(path, "venues/")
		parts := strings.Split(relative, "/")
		if len(parts) < 2 || parts[0] == "" {
			return fmt.Errorf("cdf activation: rendered event path is not venue-qualified")
		}
		venueID := parts[0]
		route := strings.Join(parts[1:], "/")
		scanner := bufio.NewScanner(bytes.NewReader(snapshot.RenderedFiles[path]))
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			var envelope envelope
			if err := json.Unmarshal(line, &envelope); err != nil {
				return fmt.Errorf("cdf activation: parse rendered evidence %s: %w", relative, err)
			}
			var data dataLayer
			if err := json.Unmarshal(envelope.Data, &data); err != nil {
				return fmt.Errorf("cdf activation: parse rendered evidence data %s: %w", relative, err)
			}
			if data.GlobalSequence == 0 || data.Sequence == 0 || data.VenueID == "" || data.VenueID != venueID || envelope.Event == "" {
				return fmt.Errorf("cdf activation: rendered event has incomplete global/local identity")
			}
			identity := cdfEvidenceFrameIdentity{
				globalSequence: data.GlobalSequence, localSequence: data.Sequence,
				simTS: envelope.SimTS, clientID: envelope.ClientID, venueID: data.VenueID,
				route: route, eventName: envelope.Event, payloadDigest: sha256.Sum256(data.Payload),
			}
			if _, duplicate := renderedByGlobal[data.GlobalSequence]; duplicate {
				return fmt.Errorf("cdf activation: rendered evidence repeats global frame sequence %d", data.GlobalSequence)
			}
			renderedByGlobal[data.GlobalSequence] = identity
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("cdf activation: scan rendered evidence %s: %w", relative, err)
		}
	}
	if uint64(len(renderedByGlobal)) != attestation.EventFrames {
		return fmt.Errorf("cdf activation: rendered identity count %d does not match binary event frames %d", len(renderedByGlobal), attestation.EventFrames)
	}
	for sequence, sourceIdentity := range sourceByGlobal {
		renderedIdentity, exists := renderedByGlobal[sequence]
		if !exists || renderedIdentity != sourceIdentity {
			return fmt.Errorf("cdf activation: rendered event identity does not match binary source frame %d", sequence)
		}
	}
	return nil
}

func (r *CDFActivationAudit) validateConfiguration(config cdfActivationConfig, metadata cdfActivationMetadata, contract CDFActivationContract) {
	if config.HypothesisID != contract.HypothesisID || config.ExperimentID != contract.ExperimentID || config.Seed != contract.Seed ||
		metadata.SimulatedHorizon != contract.Horizon || metadata.SimulationStartNano != contract.SimulationStartNano ||
		metadata.SimulationEndNano != contract.SimulationEndNano {
		r.addCheck(CDFActivationCheck{Failure: "run identity does not match the registered activation contract"})
	}
	if !sameCDFStrings(config.VenueIDs, contract.VenueIDs) {
		r.addCheck(CDFActivationCheck{Failure: "venue roster does not match the registered activation contract"})
	}
	if config.LogMode != "full" || config.EvidenceFormat != "evstream_v3" || config.EvidenceContractVersion != 2 || !config.StrictPopulationAccounting ||
		!config.StrictRiskContract || config.AutoBorrowSpot == nil || *config.AutoBorrowSpot || !config.CrossAssetSpotGraph ||
		config.CrossAssetCollateralMarks || !config.RecordElasticLiquiditySupplierDecisions ||
		!config.RecordMarketDataReceipts || !containsCDFString(config.MarketDataReceiptRoles, "cdf_elastic_supplier") {
		r.addCheck(CDFActivationCheck{Failure: "successor strict-risk or evidence configuration is incomplete"})
	}
	if config.ElasticSupplierCount != contract.HistoricalSupplierCountPerVenue {
		r.addCheck(CDFActivationCheck{Failure: "historical supplier count differs from the registered predecessor population"})
	}
	expected := make(map[string]CDFSupplierContract, len(contract.Suppliers))
	for _, supplier := range contract.Suppliers {
		expected[supplier.Role] = supplier
	}
	if len(config.ElasticLiquiditySuppliers) != len(expected) {
		r.addCheck(CDFActivationCheck{Failure: "configured CDF supplier roster has the wrong size"})
	}
	seen := make(map[string]struct{}, len(config.ElasticLiquiditySuppliers))
	for _, actual := range config.ElasticLiquiditySuppliers {
		want, exists := expected[actual.Role]
		if !exists || actual != want {
			r.addCheck(CDFActivationCheck{Role: actual.Role, Failure: "configured CDF supplier differs from the registered finite roster"})
		}
		if _, duplicate := seen[actual.Role]; duplicate {
			r.addCheck(CDFActivationCheck{Role: actual.Role, Failure: "duplicate configured CDF supplier role"})
		}
		seen[actual.Role] = struct{}{}
	}
	for role := range expected {
		if _, exists := seen[role]; !exists {
			r.addCheck(CDFActivationCheck{Role: role, Failure: "registered CDF supplier role is missing from config"})
		}
	}
}

func loadCDFReceiptIndex(dir string) (*cdfReceiptIndex, *MarketDataReceiptAudit, error) {
	audit, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, nil, err
	}
	manifestRaw, err := readSV1DRegularFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, nil, err
	}
	var manifest marketDataEvidenceManifest
	if err := decodeSV1DJSONWithRequiredFields(manifestRaw, &manifest,
		"schema_version", "domain", "ordering", "terminal_at", "schedules", "receipts", "decisions", "links", "symbols"); err != nil {
		return nil, nil, err
	}
	receiptsRaw, digestMatches, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, nil, err
	}
	if !digestMatches {
		return nil, nil, fmt.Errorf("receipt digest mismatch")
	}
	decisionsRaw, digestMatches, err := readEvidenceFile(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, nil, err
	}
	if !digestMatches {
		return nil, nil, fmt.Errorf("decision digest mismatch")
	}
	links := make(map[uint32]struct {
		sourceVenue string
		role        string
	}, len(manifest.Links))
	for _, row := range manifest.Links {
		links[row.ID] = struct {
			sourceVenue string
			role        string
		}{row.SourceVenue, row.Role}
	}
	symbols := make(map[uint32]string, len(manifest.Symbols))
	for _, row := range manifest.Symbols {
		symbols[row.ID] = row.Symbol
	}
	index := &cdfReceiptIndex{
		receipts:  make(map[cdfReceiptKey]cdfReceiptProof, manifest.Receipts.Records),
		decisions: make(map[cdfRequestKey]cdfGatewayDecision, manifest.Decisions.Records),
	}
	frontiers := make(map[linkKey][16]byte)
	for offset := 0; offset < len(receiptsRaw); offset += marketDataReceiptRecordBytes {
		raw := receiptsRaw[offset : offset+marketDataReceiptRecordBytes]
		record := decodeObservation(raw)
		key := linkKey{clientID: record.clientID, linkID: record.linkID}
		chain := sha256.New()
		previous := frontiers[key]
		_, _ = chain.Write(previous[:])
		_, _ = chain.Write(raw)
		var digest [16]byte
		copy(digest[:], chain.Sum(nil))
		frontiers[key] = digest
		catalog := links[record.linkID]
		proofKey := cdfReceiptKey{record.clientID, record.linkID, record.ordinal}
		if _, duplicate := index.receipts[proofKey]; duplicate {
			return nil, nil, fmt.Errorf("duplicate receipt identity for client %d link %d ordinal %d", record.clientID, record.linkID, record.ordinal)
		}
		index.receipts[proofKey] = cdfReceiptProof{
			sourceVenue: catalog.sourceVenue, role: catalog.role,
			symbol: symbols[record.symbolID], record: record, digest: digest,
		}
	}
	for offset := 0; offset < len(decisionsRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(decisionsRaw[offset : offset+marketDataDecisionRecordBytes])
		catalog := links[record.linkID]
		key := cdfRequestKey{catalog.sourceVenue, record.clientID, record.requestID}
		if _, duplicate := index.decisions[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate gateway request identity for client %d request %d", record.clientID, record.requestID)
		}
		index.decisions[key] = cdfGatewayDecision{sourceVenue: catalog.sourceVenue, symbol: symbols[record.symbolID], record: record}
	}
	return index, audit, nil
}

func (r *CDFActivationAudit) indexCDFAccounts(report Report, config cdfActivationConfig, contract CDFActivationContract) map[cdfParticipantKey]*cdfSupplierState {
	configured := make(map[string]CDFSupplierContract, len(config.ElasticLiquiditySuppliers))
	for _, supplier := range config.ElasticLiquiditySuppliers {
		configured[supplier.Role] = supplier
	}
	expectedVenues := make(map[string]struct{}, len(contract.VenueIDs))
	for _, venueID := range contract.VenueIDs {
		expectedVenues[venueID] = struct{}{}
	}
	states := make(map[cdfParticipantKey]*cdfSupplierState, len(contract.VenueIDs)*len(contract.Suppliers))
	historicalInitial := make(map[string]map[string]struct{}, len(contract.VenueIDs))
	historicalTerminal := make(map[string]map[string]struct{}, len(contract.VenueIDs))
	initialRoleOwners := make(map[string]map[string]uint64, len(contract.VenueIDs))
	terminalRoleOwners := make(map[string]map[string]uint64, len(contract.VenueIDs))
	for _, row := range report.InitialAccounts {
		if isNumberedRole(row.Role, "elastic_supplier_") {
			registerCDFHistoricalRole(r, historicalInitial, row, expectedVenues)
			continue
		}
		if !strings.HasPrefix(row.Role, "cdf_elastic_supplier_") {
			continue
		}
		key := cdfParticipantKey{row.VenueID, row.ClientID}
		if _, duplicate := states[key]; duplicate {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate initial CDF supplier account"})
			continue
		}
		supplier, exists := configured[row.Role]
		if !exists {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "CDF supplier account is outside the configured roster"})
			continue
		}
		state := &cdfSupplierState{
			contract: supplier, initialAccountSeen: true,
			reconstructedReference: supplier.ReferencePrice,
			reconstructedRiskMark:  supplier.ReferencePrice,
			reconstructedEquity:    row.Account.Equity,
			reconstructedPeak:      row.Account.Equity,
			equityStateSet:         true,
			audit: CDFSupplierActivationAudit{
				VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID,
				InitialEquity: row.Account.Equity, MinPosition: math.MaxInt64, MaxPosition: math.MinInt64,
			},
		}
		states[key] = state
		registerCDFRoleOwner(r, initialRoleOwners, row, "initial")
		if _, expected := expectedVenues[row.VenueID]; !expected {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "CDF supplier account is outside the registered venue roster"})
		}
		r.validateSupplierAccount(row, supplier, true)
		state.initialBaseBalance, _ = cdfAccountNetBalance(row.Account.SpotBalances, supplier.BaseAsset)
		state.initialQuoteBalance, _ = cdfAccountNetBalance(row.Account.SpotBalances, supplier.QuoteAsset)
	}
	terminalSeen := make(map[cdfParticipantKey]struct{})
	for _, row := range report.TerminalAccounts {
		if isNumberedRole(row.Role, "elastic_supplier_") {
			registerCDFHistoricalRole(r, historicalTerminal, row, expectedVenues)
			continue
		}
		if !strings.HasPrefix(row.Role, "cdf_elastic_supplier_") {
			continue
		}
		key := cdfParticipantKey{row.VenueID, row.ClientID}
		if _, duplicate := terminalSeen[key]; duplicate {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate terminal CDF supplier account"})
			continue
		}
		terminalSeen[key] = struct{}{}
		state := states[key]
		if state == nil || state.audit.Role != row.Role {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "terminal CDF supplier has no matching initial account"})
			continue
		}
		state.terminalAccountSeen = true
		registerCDFRoleOwner(r, terminalRoleOwners, row, "terminal")
		state.audit.TerminalEquity = row.Account.Equity
		pnl, ok := checkedCDFSub(row.Account.Equity, state.audit.InitialEquity)
		if !ok {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier PnL overflows"})
		} else {
			state.audit.PnL = pnl
		}
		r.validateSupplierAccount(row, state.contract, false)
		state.terminalBaseBalance, _ = cdfAccountNetBalance(row.Account.SpotBalances, state.contract.BaseAsset)
		state.terminalQuoteBalance, _ = cdfAccountNetBalance(row.Account.SpotBalances, state.contract.QuoteAsset)
	}
	for _, venueID := range contract.VenueIDs {
		for _, supplier := range contract.Suppliers {
			found := false
			for _, state := range states {
				if state.audit.VenueID == venueID && state.audit.Role == supplier.Role {
					found = true
					break
				}
			}
			if !found {
				r.addCheck(CDFActivationCheck{VenueID: venueID, Role: supplier.Role, Failure: "registered CDF supplier account is missing"})
			}
		}
		validateCDFHistoricalRoster(r, historicalInitial[venueID], venueID, contract.HistoricalSupplierCountPerVenue, "initial")
		validateCDFHistoricalRoster(r, historicalTerminal[venueID], venueID, contract.HistoricalSupplierCountPerVenue, "terminal")
	}
	for _, state := range states {
		if !state.terminalAccountSeen {
			r.addCheck(CDFActivationCheck{VenueID: state.audit.VenueID, Role: state.audit.Role, ClientID: state.audit.ClientID, Failure: "CDF supplier is missing terminal account evidence"})
		}
	}
	r.SupplierCount = len(states)
	return states
}

func registerCDFHistoricalRole(result *CDFActivationAudit, roster map[string]map[string]struct{}, row AccountRow, venues map[string]struct{}) {
	if _, expected := venues[row.VenueID]; !expected {
		result.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "historical supplier is outside the registered venue roster"})
	}
	if roster[row.VenueID] == nil {
		roster[row.VenueID] = make(map[string]struct{})
	}
	if _, duplicate := roster[row.VenueID][row.Role]; duplicate {
		result.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "duplicate historical supplier role"})
	}
	roster[row.VenueID][row.Role] = struct{}{}
}

func registerCDFRoleOwner(result *CDFActivationAudit, owners map[string]map[string]uint64, row AccountRow, phase string) {
	if owners[row.VenueID] == nil {
		owners[row.VenueID] = make(map[string]uint64)
	}
	if previous, duplicate := owners[row.VenueID][row.Role]; duplicate && previous != row.ClientID {
		result.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " CDF supplier role is owned by multiple clients"})
	}
	owners[row.VenueID][row.Role] = row.ClientID
}

func validateCDFHistoricalRoster(result *CDFActivationAudit, roles map[string]struct{}, venueID string, expected int, phase string) {
	if len(roles) != expected {
		result.addCheck(CDFActivationCheck{VenueID: venueID, Failure: fmt.Sprintf("%s historical supplier roster has %d roles, want %d", phase, len(roles), expected)})
	}
	for ordinal := 1; ordinal <= expected; ordinal++ {
		role := fmt.Sprintf("elastic_supplier_%d", ordinal)
		if _, exists := roles[role]; !exists {
			result.addCheck(CDFActivationCheck{VenueID: venueID, Role: role, Failure: phase + " historical supplier role is missing"})
		}
	}
}

func (r *CDFActivationAudit) validateSupplierAccount(row AccountRow, supplier CDFSupplierContract, initial bool) {
	phase := "terminal"
	if initial {
		phase = "initial"
	}
	if row.Account.Equity == 0 {
		r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier equity is absent or zero"})
	}
	balances := make(map[string]Balance, len(row.Account.SpotBalances))
	for _, balance := range row.Account.SpotBalances {
		if balance.Asset == "" {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier has an unnamed spot balance"})
			continue
		}
		if _, duplicate := balances[balance.Asset]; duplicate {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier has duplicate spot balance assets"})
		}
		balances[balance.Asset] = balance
		if balance.Borrowed != 0 {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier carries borrowed spot debt"})
		}
	}
	if initial {
		if balances[supplier.BaseAsset].NetAsset != supplier.InitialBaseBalance || balances[supplier.QuoteAsset].NetAsset != supplier.InitialQuoteBalance {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "supplier initial balances differ from the finite registered endowment"})
		}
	} else {
		base, basePresent := balances[supplier.BaseAsset]
		quote, quotePresent := balances[supplier.QuoteAsset]
		baseDisplacement, displacementOK := checkedCDFSub(base.NetAsset, supplier.InitialBaseBalance)
		if !basePresent || !quotePresent || base.NetAsset < 0 || base.NetAsset > supplier.MaxInventory ||
			quote.NetAsset < 0 || !displacementOK || cdfAbsExceeds(baseDisplacement, supplier.MaxPosition) {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: "terminal supplier balances exceed finite inventory or cash limits"})
		}
	}
	for _, balance := range row.Account.PerpBalances {
		if balance.NetAsset != 0 || balance.Borrowed != 0 {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier has nonzero derivative collateral"})
		}
	}
	for _, position := range row.Account.Positions {
		if position.Size != 0 {
			r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier has a derivative position"})
		}
	}
	markedEquity, markedEquityOK := cdfMarkedAccountEquity(row, supplier)
	if !markedEquityOK || markedEquity != row.Account.Equity {
		r.addCheck(CDFActivationCheck{VenueID: row.VenueID, Role: row.Role, ClientID: row.ClientID, Failure: phase + " supplier equity does not reconcile to finite marked spot balances"})
	}
}

func (r *CDFActivationAudit) indexCDFSnapshots(run *Run) (map[cdfSnapshotKey]cdfSnapshotProof, map[string][]cdfDepthObservation, error) {
	proofs := make(map[cdfSnapshotKey]cdfSnapshotProof)
	sequences := make(map[string]map[uint64][16]byte)
	depth := make(map[string][]cdfDepthObservation)
	for _, path := range run.Files() {
		if symbolFromPath(path) != cdfActivationLogName {
			continue
		}
		var callbackFailure error
		err := run.Scan(ScanOptions{Events: []string{"BookSnapshot"}, Files: []string{path}, FilesSelected: true, Workers: 1}, func(event Event) {
			if callbackFailure != nil {
				return
			}
			var snapshot cdfPublicSnapshotEvidence
			if err := decodeRequiredJSON(event.Raw(), &snapshot, "bids", "asks", "source_sequence", "public_bids", "public_asks"); err != nil {
				r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed public CDF snapshot: " + err.Error()})
				return
			}
			if snapshot.SourceSequence == 0 || snapshot.Bids == nil || snapshot.Asks == nil || snapshot.PublicBids == nil || snapshot.PublicAsks == nil || !validCDFSnapshotProjection(snapshot) {
				r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF snapshot lacks explicit sequence or side presence"})
				return
			}
			message := &etypes.MarketDataMsg{
				Type: etypes.MDSnapshot, Symbol: cdfActivationSymbol,
				SeqNum: snapshot.SourceSequence, Timestamp: event.SimTS,
				Data: &etypes.BookSnapshot{Bids: snapshot.PublicBids, Asks: snapshot.PublicAsks},
			}
			fingerprint, err := etypes.MarketDataFingerprint(message)
			if err != nil {
				callbackFailure = fmt.Errorf("fingerprint CDF snapshot: %w", err)
				return
			}
			key := cdfSnapshotKey{event.VenueID, snapshot.SourceSequence, fingerprint}
			venueSequences := sequences[event.VenueID]
			if venueSequences == nil {
				venueSequences = make(map[uint64][16]byte)
				sequences[event.VenueID] = venueSequences
			}
			if priorFingerprint, exists := venueSequences[snapshot.SourceSequence]; exists && priorFingerprint != fingerprint {
				r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF snapshot publication sequence is reused with a different fingerprint"})
				return
			}
			venueSequences[snapshot.SourceSequence] = fingerprint
			if _, duplicate := proofs[key]; duplicate {
				r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "duplicate public CDF snapshot identity"})
				return
			}
			proofs[key] = cdfSnapshotProof{
				publishedAt: event.SimTS, globalSequence: event.GlobalSequence,
				bids: snapshot.PublicBids, asks: snapshot.PublicAsks,
			}
		})
		if err != nil {
			return nil, nil, fmt.Errorf("cdf activation: scan public snapshots in %s: %w", path, err)
		}
		if callbackFailure != nil {
			return nil, nil, callbackFailure
		}
	}
	if len(proofs) == 0 {
		r.addCheck(CDFActivationCheck{Failure: "no public CDF snapshot evidence"})
	}
	return proofs, depth, nil
}

var cdfOrderedEventNames = []string{
	"elastic_liquidity_supplier_decision", "elastic_liquidity_supplier_fill", "balance_snapshot", "borrow",
	"BookSnapshot", "BookDelta", "Trade", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "OrderCancelRejected",
}

func collectCDFOrderedEvents(run *Run) ([]Event, error) {
	if run == nil {
		return nil, fmt.Errorf("cdf activation: nil evidence run")
	}
	events := make([]Event, 0)
	if err := run.Scan(ScanOptions{Events: cdfOrderedEventNames, Workers: 1}, func(event Event) {
		if filepath.Base(event.File) == "general.jsonl" {
			switch event.Name {
			case "elastic_liquidity_supplier_decision", "elastic_liquidity_supplier_fill", "balance_snapshot", "borrow", "OrderCancelRejected":
				events = append(events, event)
			}
			return
		}
		if symbolFromPath(event.File) != cdfActivationLogName {
			return
		}
		switch event.Name {
		case "BookSnapshot", "BookDelta", "Trade", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "OrderCancelRejected":
			events = append(events, event)
		}
	}); err != nil {
		return nil, fmt.Errorf("cdf activation: collect ordered evidence: %w", err)
	}
	for _, event := range events {
		if event.GlobalSequence == 0 {
			return nil, fmt.Errorf("cdf activation: strict evidence event %s/%s#%d has no global frame sequence", event.VenueID, event.Name, event.Ordinal)
		}
	}
	sort.Slice(events, func(left, right int) bool {
		if events[left].GlobalSequence != events[right].GlobalSequence {
			return events[left].GlobalSequence < events[right].GlobalSequence
		}
		if events[left].File != events[right].File {
			return events[left].File < events[right].File
		}
		return events[left].Ordinal < events[right].Ordinal
	})
	for index := 1; index < len(events); index++ {
		if events[index].GlobalSequence == events[index-1].GlobalSequence {
			return nil, fmt.Errorf("cdf activation: duplicate global frame sequence %d in selected evidence", events[index].GlobalSequence)
		}
	}
	return events, nil
}

func (r *CDFActivationAudit) scanCDFOrdered(
	events []Event,
	states map[cdfParticipantKey]*cdfSupplierState,
	receipts *cdfReceiptIndex,
	snapshots map[cdfSnapshotKey]cdfSnapshotProof,
	submissions map[cdfRequestKey]*cdfSubmission,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
	observedFills map[cdfFillKey]cdfFillEvidence,
	orders map[cdfOrderKey]*cdfOrderState,
	actualFills map[cdfFillKey]cdfOrderFillEvidence,
	depth map[string][]cdfDepthObservation,
) error {
	publicDepth := make(map[string]*cdfPublicDepthState)
	pendingDepth := make(map[string][]cdfPendingDepthObservation)
	bookSnapshotCount := 0
	for _, event := range events {
		r.flushCDFPendingDepthBeforeEvent(event, states, orders, depth, pendingDepth)
		switch event.Name {
		case "elastic_liquidity_supplier_decision":
			r.processCDFDecision(event, states, receipts, snapshots, submissions, withdrawals)
		case "elastic_liquidity_supplier_fill":
			r.processCDFFill(event, states, observedFills)
		case "balance_snapshot":
			r.processCDFBalanceSnapshot(event, states)
		case "borrow":
			r.processCDFBorrow(event, states)
		case "BookSnapshot":
			bookSnapshotCount++
			r.processCDFDepthSnapshot(event, states, orders, depth, publicDepth, pendingDepth)
		case "BookDelta":
			r.processCDFDepthDelta(event, states, orders, depth, publicDepth, pendingDepth)
		case "Trade":
			r.processCDFTrade(event)
		case "OrderAccepted":
			r.processCDFAccepted(event, states, submissions, orders)
		case "OrderRejected":
			r.processCDFRejected(event, states, submissions)
		case "OrderFill":
			r.processCDFOrderFill(event, states, orders, actualFills)
		case "OrderCancelled":
			r.processCDFCancelled(event, states, withdrawals, orders, depth, publicDepth)
			r.flushCDFPendingDepth(event.VenueID, states, orders, depth, pendingDepth, true)
		case "OrderCancelRejected":
			r.processCDFCancelRejected(event, states, withdrawals, orders)
		}
	}
	r.flushAllCDFPendingDepth(states, orders, depth, pendingDepth)
	if bookSnapshotCount == 0 {
		r.addCheck(CDFActivationCheck{Failure: "no rendered CDF/USD book evidence"})
	}
	return nil
}

func (r *CDFActivationAudit) scanCDFGeneral(
	run *Run,
	states map[cdfParticipantKey]*cdfSupplierState,
	receipts *cdfReceiptIndex,
	snapshots map[cdfSnapshotKey]cdfSnapshotProof,
	submissions map[cdfRequestKey]*cdfSubmission,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
	observedFills map[cdfFillKey]cdfFillEvidence,
) error {
	for _, path := range run.Files() {
		if filepath.Base(path) != "general.jsonl" {
			continue
		}
		lastTimestamp := int64(math.MinInt64)
		err := run.Scan(ScanOptions{
			Events: []string{"elastic_liquidity_supplier_decision", "elastic_liquidity_supplier_fill", "balance_snapshot", "borrow"},
			Files:  []string{path}, FilesSelected: true, Workers: 1,
		}, func(event Event) {
			if event.SimTS < lastTimestamp {
				r.addCheck(CDFActivationCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "CDF general evidence timestamps regress"})
			}
			lastTimestamp = event.SimTS
			switch event.Name {
			case "elastic_liquidity_supplier_decision":
				r.processCDFDecision(event, states, receipts, snapshots, submissions, withdrawals)
			case "elastic_liquidity_supplier_fill":
				r.processCDFFill(event, states, observedFills)
			case "balance_snapshot":
				r.processCDFBalanceSnapshot(event, states)
			case "borrow":
				r.processCDFBorrow(event, states)
			}
		})
		if err != nil {
			return fmt.Errorf("cdf activation: scan supplier evidence in %s: %w", path, err)
		}
	}
	return nil
}

func (r *CDFActivationAudit) processCDFDecision(
	event Event,
	states map[cdfParticipantKey]*cdfSupplierState,
	receipts *cdfReceiptIndex,
	snapshots map[cdfSnapshotKey]cdfSnapshotProof,
	submissions map[cdfRequestKey]*cdfSubmission,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
) {
	var decision cdfDecisionEvidence
	required := []string{
		"role", "client_id", "symbol", "decision_time", "decision_phase_offset_nanos",
		"observation_time", "observation_age", "observation_sequence", "observation_link_id",
		"observation_ordinal", "observation_delivered_at", "observation_fingerprint", "observation_digest",
		"best_bid", "best_bid_qty", "best_ask", "best_ask_qty", "mark_price", "risk_mark_price",
		"local_book_mode", "quote_price_source", "risk_mark_source", "reference_price", "position",
		"target_position", "inventory_limit", "initial_base_balance", "gross_inventory",
		"gross_inventory_limit", "action", "reason", "minimum_qualifying_qty",
		"registered_minimum_executable_qty", "quote_cash_reserved", "initial_equity_quote",
		"equity_quote", "peak_equity_quote", "loss_from_initial_quote", "drawdown_quote",
		"max_loss_quote", "equity_available", "risk_limit_triggered",
	}
	if r.strictMechanics {
		required = append(required, "risk_mark_current")
	}
	if err := decodeRequiredJSON(event.Raw(), &decision, required...); err != nil {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed CDF decision: " + err.Error()})
		return
	}
	key := cdfParticipantKey{event.VenueID, event.ClientID}
	state := states[key]
	if state == nil || decision.ClientID != event.ClientID || decision.Role != state.audit.Role || decision.Symbol != cdfActivationSymbol {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Role: decision.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "CDF decision identity is outside the registered participant roster"})
		return
	}
	state.audit.DecisionCount++
	r.DecisionCount++
	if decision.DecisionTime != event.SimTS || decision.DecisionPhaseOffset != state.contract.DecisionPhaseOffset {
		r.addEventCheck(event, state, "CDF decision timestamp or phase offset disagrees with the registered schedule")
	}
	if decision.InventoryLimit != state.contract.MaxPosition || decision.InitialBaseBalance != state.contract.InitialBaseBalance ||
		decision.GrossInventoryLimit != state.contract.MaxInventory || decision.MinimumQualifyingQty != state.contract.MinimumQualifyingQty ||
		decision.RegisteredMinimumExecutableQty != state.contract.RegisteredMinimumExecutableQty || decision.MaxLossQuote != state.contract.MaxLossQuote {
		r.addEventCheck(event, state, "CDF decision limits disagree with the registered finite roster")
	}
	grossInventory, ok := checkedCDFAdd(state.contract.InitialBaseBalance, decision.Position)
	if !ok || grossInventory != decision.GrossInventory || grossInventory < 0 || grossInventory > state.contract.MaxInventory ||
		cdfAbsExceeds(decision.Position, state.contract.MaxPosition) || cdfAbsExceeds(decision.TargetPosition, state.contract.MaxPosition) ||
		decision.Position != state.currentPosition {
		r.addEventCheck(event, state, "CDF decision inventory or position exceeds finite limits")
	}
	if decision.GrossInventory > state.audit.MaxGrossInventory {
		state.audit.MaxGrossInventory = decision.GrossInventory
	}
	if decision.Position < state.audit.MinPosition {
		state.audit.MinPosition = decision.Position
	}
	if decision.Position > state.audit.MaxPosition {
		state.audit.MaxPosition = decision.Position
	}
	if decision.LossFromInitialQuote < 0 || decision.DrawdownQuote < 0 || decision.MaxLossQuote <= 0 ||
		decision.QuoteCashAvailable < 0 || decision.QuoteCashReserved < 0 {
		r.addEventCheck(event, state, "CDF decision carries invalid finite cash or loss state")
	}
	joinedSnapshot := r.validateCDFObservation(event, state, decision, receipts, snapshots)
	if joinedSnapshot {
		state.audit.EligibleObservationCount++
		if !r.strictMechanics && (decision.ReferencePrice <= 0 || !cdfBetweenInclusive(decision.ReferencePrice, state.reconstructedReference, decision.MarkPrice)) {
			r.addEventCheck(event, state, "CDF private reference moved outside its prior value and delayed local anchor")
		} else if !r.strictMechanics {
			state.reconstructedReference = decision.ReferencePrice
		}
	}
	mechanicsValid := true
	if r.strictMechanics {
		mechanicsValid = r.validateCDFDecisionMechanics(event, state, decision)
	}
	expectedPeak := decision.PeakEquityQuote
	if r.strictMechanics {
		expectedPeak = r.validateCDFDecisionEquity(event, state, decision, mechanicsValid)
	}
	expectedLoss, lossOK := checkedCDFSub(state.audit.InitialEquity, decision.EquityQuote)
	if expectedLoss < 0 {
		expectedLoss = 0
	}
	drawdownPeak := decision.PeakEquityQuote
	if r.strictMechanics {
		drawdownPeak = expectedPeak
	}
	expectedDrawdown, drawdownOK := checkedCDFSub(drawdownPeak, decision.EquityQuote)
	if expectedDrawdown < 0 {
		expectedDrawdown = 0
	}
	if decision.InitialEquityQuote != state.audit.InitialEquity || decision.PeakEquityQuote < decision.EquityQuote ||
		!lossOK || !drawdownOK || decision.LossFromInitialQuote != expectedLoss || decision.DrawdownQuote != expectedDrawdown ||
		decision.PeakEquityQuote != expectedPeak ||
		decision.RiskMarkCurrent && (!decision.EquityAvailable || decision.RiskMarkPrice <= 0) {
		r.addEventCheck(event, state, "CDF decision equity and loss evidence is internally inconsistent")
	}
	if !isCDFAction(decision.Action) || decision.Reason == "" {
		r.addEventCheck(event, state, "CDF decision has an unknown action or empty reason")
	}
	if r.strictMechanics && (!cdfDecisionReasonAllowed(decision.Action, decision.Reason) ||
		!cdfDecisionReasonPredicate(decision, state)) {
		r.addEventCheck(event, state, "CDF decision reason is not a registered economic lifecycle reason")
	}
	r.recordCDFPostFillResponse(event, state, decision)
	requestKey := cdfRequestKey{event.VenueID, event.ClientID, decision.QuoteRequestID}
	switch decision.Action {
	case "submit":
		if !joinedSnapshot || decision.QuoteRequestID == 0 || decision.QuoteSubmittedAt != decision.DecisionTime ||
			(decision.Side != "BUY" && decision.Side != "SELL") || decision.QuotePrice <= 0 ||
			decision.QuotePrice%state.contract.TickSize != 0 || decision.QuoteQty < state.contract.MinimumExecutableQty ||
			decision.QuoteQty > state.contract.MaxQuoteQty || decision.QuoteCashRequired < 0 {
			r.addEventCheck(event, state, "CDF submit decision is not a bounded executable passive quote")
		}
		r.validateCDFSubmitEconomics(event, state, decision)
		if _, duplicate := submissions[requestKey]; duplicate {
			r.addEventCheck(event, state, "duplicate CDF submission request identity")
		} else {
			submissions[requestKey] = &cdfSubmission{event: event, decision: decision}
		}
		gateway, exists := receipts.decisions[requestKey]
		if !exists || gateway.symbol != cdfActivationSymbol || gateway.record.decisionAt != decision.DecisionTime ||
			gateway.record.frontierOrdinal != decision.ObservationOrdinal || gateway.record.frontierDeliveredAt != decision.ObservationDeliveredAt ||
			hex.EncodeToString(gateway.record.frontierDigest[:]) != decision.ObservationDigest ||
			gateway.record.price != decision.QuotePrice || gateway.record.qty != decision.QuoteQty ||
			gateway.record.side != cdfSideCode(decision.Side) {
			r.addEventCheck(event, state, "CDF submit decision does not match its actor-gateway decision record")
		}
	case "cancel", "withdraw":
		if decision.QuoteOrderID == 0 || decision.CancelRequestID == 0 {
			r.addEventCheck(event, state, "CDF cancellation or withdrawal decision lacks order or request identity")
			break
		}
		cancelKey := cdfRequestKey{event.VenueID, event.ClientID, decision.CancelRequestID}
		if _, duplicate := withdrawals[cancelKey]; duplicate {
			r.addEventCheck(event, state, "duplicate CDF cancellation or withdrawal request identity")
		} else {
			withdrawals[cancelKey] = &cdfWithdrawal{event: event, decision: decision}
		}
	}
	state.lastDecision = decision
	state.hasLastDecision = true
}

func (r *CDFActivationAudit) validateCDFObservation(
	event Event,
	state *cdfSupplierState,
	decision cdfDecisionEvidence,
	receipts *cdfReceiptIndex,
	snapshots map[cdfSnapshotKey]cdfSnapshotProof,
) bool {
	if decision.ObservationSequence == 0 || decision.ObservationLinkID == 0 || decision.ObservationOrdinal == 0 {
		if decision.Action != "wait" || decision.Reason != "subscribe" &&
			decision.Reason != "stale_or_missing_observation" && decision.Reason != "equity_unavailable" {
			r.addEventCheck(event, state, "actionable CDF decision has no delivered observation frontier")
		}
		return false
	}
	if !isCDFHex(decision.ObservationFingerprint, 16) || !isCDFHex(decision.ObservationDigest, 16) {
		r.addEventCheck(event, state, "CDF decision has an empty or malformed observation fingerprint/frontier digest")
		return false
	}
	proof, exists := receipts.receipts[cdfReceiptKey{event.ClientID, decision.ObservationLinkID, decision.ObservationOrdinal}]
	if !exists || proof.sourceVenue != event.VenueID || proof.role != "cdf_elastic_supplier" || proof.symbol != cdfActivationSymbol ||
		proof.record.sequence != decision.ObservationSequence || proof.record.publishedAt != decision.ObservationTime ||
		proof.record.deliveredAt != decision.ObservationDeliveredAt || hex.EncodeToString(proof.record.fingerprint[:]) != decision.ObservationFingerprint ||
		hex.EncodeToString(proof.digest[:]) != decision.ObservationDigest {
		r.addEventCheck(event, state, "CDF decision does not join its exact delayed receipt frontier")
		return false
	}
	if decision.ObservationTime > decision.ObservationDeliveredAt || decision.ObservationDeliveredAt > decision.DecisionTime ||
		decision.ObservationAge != decision.DecisionTime-decision.ObservationTime || decision.ObservationAge < 0 {
		r.addEventCheck(event, state, "CDF decision observation timing is non-causal")
		return false
	}
	if (decision.Action == "submit" || decision.Action == "rest") && decision.ObservationAge > state.contract.MaxObservationAge {
		r.addEventCheck(event, state, "CDF quote uses an observation older than the registered maximum")
		return false
	}
	fingerprintBytes, _ := hex.DecodeString(decision.ObservationFingerprint)
	var fingerprint [16]byte
	copy(fingerprint[:], fingerprintBytes)
	snapshot, exists := snapshots[cdfSnapshotKey{event.VenueID, decision.ObservationSequence, fingerprint}]
	if !exists || snapshot.publishedAt != decision.ObservationTime {
		r.addEventCheck(event, state, "CDF receipt fingerprint does not join a public CDF snapshot")
		return false
	}
	if r.strictMechanics && (event.GlobalSequence == 0 || snapshot.globalSequence == 0 || snapshot.globalSequence >= event.GlobalSequence) {
		r.addEventCheck(event, state, "CDF decision does not follow the globally ordered source snapshot frame")
		return false
	}
	bestBid, bestBidQty := cdfBestBid(snapshot.bids)
	bestAsk, bestAskQty := cdfBestAsk(snapshot.asks)
	if decision.BestBid != bestBid || decision.BestBidQty != bestBidQty || decision.BestAsk != bestAsk || decision.BestAskQty != bestAskQty {
		r.addEventCheck(event, state, "CDF decision touch differs from its delayed public snapshot")
		return false
	}
	expectedMode := cdfLocalBookMode(bestBid, bestBidQty, bestAsk, bestAskQty, state.contract.TickSize)
	if cdfDecisionIsUncomputed(decision) {
		return true
	}
	if decision.LocalBookMode != expectedMode {
		r.addEventCheck(event, state, "CDF decision local-book mode differs from its delayed public snapshot")
		return false
	}
	if expectedMode == "one_sided" {
		r.OneSidedDecisionCount++
		if decision.Action == "submit" && decision.QuotePriceSource == "one_sided_missing_side_blended" &&
			!validCDFMissingSideQuote(decision, state.contract) {
			r.addEventCheck(event, state, "one-sided CDF quote violates the registered missing-side price contract")
			return false
		}
	}
	return true
}

func cdfLocalBookMode(bestBid, bestBidQty, bestAsk, bestAskQty, tickSize int64) string {
	if bestBid > 0 && bestAsk > 0 && bestBid < bestAsk {
		return "two_sided"
	}
	if (bestBid > 0 && bestBidQty > 0 && bestAsk == 0 && bestAskQty == 0 && cdfPositiveTickPrice(bestBid, tickSize)) ||
		(bestAsk > 0 && bestAskQty > 0 && bestBid == 0 && bestBidQty == 0 && cdfPositiveTickPrice(bestAsk, tickSize)) {
		return "one_sided"
	}
	return ""
}

func cdfPositiveTickPrice(price, tickSize int64) bool {
	return price > 0 && tickSize > 0 && price%tickSize == 0
}

func cdfLocalAnchor(decision cdfDecisionEvidence, contract CDFSupplierContract) (int64, bool) {
	if decision.LocalBookMode == "two_sided" {
		if decision.BestBid <= 0 || decision.BestAsk <= 0 || decision.BestBid >= decision.BestAsk {
			return 0, false
		}
		return etypes.Midpoint(decision.BestBid, decision.BestAsk), true
	}
	if decision.LocalBookMode == "one_sided" {
		if decision.BestBid > 0 && decision.BestBidQty > 0 && decision.BestAsk == 0 && decision.BestAskQty == 0 && cdfPositiveTickPrice(decision.BestBid, contract.TickSize) {
			return decision.BestBid, true
		}
		if decision.BestAsk > 0 && decision.BestAskQty > 0 && decision.BestBid == 0 && decision.BestBidQty == 0 && cdfPositiveTickPrice(decision.BestAsk, contract.TickSize) {
			return decision.BestAsk, true
		}
	}
	return 0, false
}

func advanceCDFReference(reference, updatedAt int64, updateSet bool, anchor, now, halfLife int64) (int64, int64, bool) {
	if anchor <= 0 || halfLife <= 0 {
		return reference, updatedAt, updateSet
	}
	if !updateSet {
		return reference, now, true
	}
	elapsedNanos, ok := checkedCDFSub(now, updatedAt)
	updatedAt = now
	if !ok {
		return reference, updatedAt, true
	}
	elapsedSeconds := float64(elapsedNanos) / 1_000_000_000.0
	if elapsedSeconds <= 0 {
		return reference, updatedAt, true
	}
	halfLifeSeconds := float64(halfLife) / 1_000_000_000.0
	alpha := 1 - math.Exp(-math.Ln2*elapsedSeconds/halfLifeSeconds)
	revised := float64(reference) + alpha*(float64(anchor)-float64(reference))
	if math.IsNaN(revised) || math.IsInf(revised, 0) || revised <= 0 {
		return reference, updatedAt, true
	}
	return int64(revised), updatedAt, true
}

func cdfTargetPosition(reference, anchor int64, contract CDFSupplierContract) int64 {
	if anchor <= 0 || reference <= 0 {
		return contract.BaseHolding
	}
	percentAbove := (float64(anchor)/float64(reference) - 1) * 100
	target := float64(contract.BaseHolding) - percentAbove*float64(contract.ElasticityPerPercent)
	if math.IsNaN(target) || math.IsInf(target, 0) {
		return contract.BaseHolding
	}
	minimumPosition, maximumPosition := -contract.MaxPosition, contract.MaxPosition
	if contract.MaxInventory > 0 {
		minimumPosition = maxCDFInt64(minimumPosition, -contract.InitialBaseBalance)
		maximumPosition = minCDFInt64(maximumPosition, contract.MaxInventory-contract.InitialBaseBalance)
	}
	return int64(math.Max(float64(minimumPosition), math.Min(float64(maximumPosition), target)))
}

func maxCDFInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func minCDFInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func (r *CDFActivationAudit) validateCDFDecisionMechanics(event Event, state *cdfSupplierState, decision cdfDecisionEvidence) bool {
	valid := true
	if decision.ReferencePrice <= 0 {
		r.addEventCheck(event, state, "CDF decision has a non-positive private reference")
		valid = false
	}
	expectedReference := state.reconstructedReference
	expectedUpdatedAt := state.referenceUpdatedAt
	expectedUpdateSet := state.referenceUpdateSet
	anchor, hasAnchor := cdfLocalAnchor(decision, state.contract)
	if cdfDecisionIsUncomputed(decision) {
		if decision.ReferencePrice != expectedReference {
			r.addEventCheck(event, state, "CDF private reference does not match the last computed actor state")
			valid = false
		}
		if decision.RiskMarkPrice < 0 || decision.RiskMarkPrice != 0 && decision.RiskMarkPrice != state.reconstructedRiskMark {
			r.addEventCheck(event, state, "CDF uncomputed decision does not preserve the last coherent risk mark")
			valid = false
		}
		return valid
	}
	if hasAnchor {
		expectedReference, expectedUpdatedAt, expectedUpdateSet = advanceCDFReference(
			expectedReference, expectedUpdatedAt, expectedUpdateSet, anchor, decision.DecisionTime, state.contract.ReferenceHalfLife,
		)
	}
	if decision.ReferencePrice != expectedReference {
		r.addEventCheck(event, state, "CDF private reference does not match the registered delayed-anchor update")
		valid = false
	}
	if decision.ReferencePrice == expectedReference {
		state.reconstructedReference = expectedReference
		state.referenceUpdatedAt = expectedUpdatedAt
		state.referenceUpdateSet = expectedUpdateSet
	}

	if !hasAnchor {
		if decision.MarkPrice != 0 || decision.TargetPosition != 0 {
			r.addEventCheck(event, state, "CDF decision reports mark or target without a valid local anchor")
			valid = false
		}
		if decision.RiskMarkCurrent || decision.RiskMarkSource != "" || decision.RiskMarkPrice < 0 ||
			decision.RiskMarkPrice != 0 && decision.RiskMarkPrice != state.reconstructedRiskMark {
			r.addEventCheck(event, state, "CDF decision risk mark is unavailable without a valid local anchor")
			valid = false
		}
		return valid
	}
	if decision.MarkPrice != anchor {
		r.addEventCheck(event, state, "CDF decision mark does not match its independently reconstructed local anchor")
		valid = false
	}
	expectedTarget := cdfTargetPosition(expectedReference, anchor, state.contract)
	if decision.TargetPosition != expectedTarget {
		r.addEventCheck(event, state, "CDF decision target does not match the registered inventory elasticity")
		valid = false
	}
	expectedRiskMark := anchor
	expectedRiskSource := "two_sided_midpoint"
	if decision.LocalBookMode == "one_sided" {
		grossInventory, ok := checkedCDFAdd(state.contract.InitialBaseBalance, decision.Position)
		if !ok || grossInventory < 0 {
			r.addEventCheck(event, state, "CDF one-sided risk mark cannot establish finite gross inventory")
			return false
		}
		if decision.BestAsk > 0 && grossInventory > 0 {
			expectedRiskMark = 0
			expectedRiskSource = "one_sided_ask_unavailable"
		} else if grossInventory == 0 {
			expectedRiskSource = "one_sided_bid_zero_inventory"
			if decision.BestAsk > 0 {
				expectedRiskSource = "one_sided_ask_zero_inventory"
			}
		} else {
			expectedRiskSource = "one_sided_bid"
		}
	}
	if decision.RiskMarkPrice != expectedRiskMark || decision.RiskMarkSource != expectedRiskSource {
		r.addEventCheck(event, state, "CDF decision risk mark does not match the registered local-book risk rule")
		valid = false
	}
	if decision.RiskMarkCurrent && expectedRiskMark <= 0 {
		r.addEventCheck(event, state, "CDF decision claims a current risk mark that the local book cannot provide")
		valid = false
	}
	if valid && decision.RiskMarkCurrent && expectedRiskMark > 0 {
		state.reconstructedRiskMark = expectedRiskMark
	}
	return valid
}

func cdfDecisionIsUncomputed(decision cdfDecisionEvidence) bool {
	return !decision.RiskMarkCurrent && decision.MarkPrice == 0 && decision.TargetPosition == 0 &&
		decision.QuotePriceSource == "" && decision.RiskMarkSource == ""
}

func (r *CDFActivationAudit) validateCDFDecisionEquity(event Event, state *cdfSupplierState, decision cdfDecisionEvidence, mechanicsValid bool) int64 {
	if !state.equityStateSet {
		state.reconstructedEquity = state.audit.InitialEquity
		state.reconstructedPeak = state.audit.InitialEquity
		state.equityStateSet = true
	}
	expectedEquity := state.reconstructedEquity
	canReconstruct := mechanicsValid && decision.RiskMarkCurrent && decision.EquityAvailable && decision.RiskMarkPrice > 0 && decision.RiskMarkSource != ""
	expectedCash, cashOK := checkedCDFAdd(state.initialQuoteBalance, state.fillQuoteDelta)
	reportedCash, reportedCashOK := checkedCDFAdd(decision.QuoteCashAvailable, decision.QuoteCashReserved)
	if !cashOK || !reportedCashOK || expectedCash != reportedCash {
		r.addEventCheck(event, state, "CDF decision quote cash does not match the actor-visible fill ledger")
	}
	if canReconstruct {
		var ok bool
		expectedEquity, ok = cdfMarkedPositionEquity(decision.Position, decision.RiskMarkPrice,
			expectedCash, 0, state.contract)
		if !ok || expectedEquity != decision.EquityQuote {
			r.addEventCheck(event, state, "CDF available equity does not match the marked position and quote cash")
			expectedEquity = state.reconstructedEquity
		}
	} else if decision.EquityQuote != state.reconstructedEquity {
		r.addEventCheck(event, state, "CDF unavailable equity changed without a verifiable mark")
	}
	expectedPeak := state.reconstructedPeak
	if canReconstruct && expectedEquity > expectedPeak {
		expectedPeak = expectedEquity
	}
	if decision.PeakEquityQuote != expectedPeak {
		r.addEventCheck(event, state, "CDF peak equity is not the reconstructed monotone peak")
	}
	if canReconstruct {
		state.reconstructedEquity = expectedEquity
	}
	state.reconstructedPeak = expectedPeak
	return expectedPeak
}

func (r *CDFActivationAudit) recordCDFPostFillResponse(event Event, state *cdfSupplierState, decision cdfDecisionEvidence) {
	for index := range state.fillResponses {
		response := &state.fillResponses[index]
		if response.responded || !cdfEventAfter(event, response.fillAt, response.fillGlobalSeq) || decision.Position != response.positionAfter {
			continue
		}
		if !r.strictMechanics {
			response.responded = true
			state.audit.PostFillResponsiveCount++
			continue
		}
		if response.requiresFreshObservation && !cdfDecisionUsesFreshObservation(response.preFillDecision, decision, response.fillAt) {
			continue
		}
		marketStateUnchanged := cdfDecisionMarketStateEqual(response.preFillDecision, decision)
		privateInventoryStateUnchanged := response.preFillDecision.ReferencePrice == decision.ReferencePrice &&
			response.preFillDecision.TargetPosition == decision.TargetPosition
		if response.preFillKnown && marketStateUnchanged && privateInventoryStateUnchanged && decision.Action == "wait" && decision.Reason == "inventory_at_target" &&
			decision.TargetPosition == decision.Position && decision.TargetPosition == response.preFillDecision.TargetPosition {
			response.responded = true
		} else if response.preFillKnown && marketStateUnchanged && privateInventoryStateUnchanged &&
			(decision.Action == "submit" || decision.Action == "cancel" || decision.Action == "withdraw") {
			quoteChanged := decision.Side != response.preFillDecision.Side || decision.QuotePrice != response.preFillDecision.QuotePrice ||
				decision.QuoteQty != response.preFillDecision.QuoteQty
			response.responded = quoteChanged || decision.Action == "withdraw" && cdfPostFillWithdrawalJustified(decision)
		}
		if response.responded {
			state.audit.PostFillResponsiveCount++
		}
	}
}

func cdfDecisionUsesFreshObservation(previous, current cdfDecisionEvidence, fillAt int64) bool {
	if previous.ObservationSequence == 0 || current.ObservationSequence == 0 {
		return false
	}
	if current.ObservationSequence < previous.ObservationSequence {
		return false
	}
	if current.ObservationSequence == previous.ObservationSequence && current.ObservationDeliveredAt <= previous.ObservationDeliveredAt {
		return false
	}
	return current.ObservationDeliveredAt > fillAt ||
		(current.ObservationDeliveredAt == fillAt && current.ObservationSequence > previous.ObservationSequence)
}

func cdfPostFillWithdrawalJustified(decision cdfDecisionEvidence) bool {
	switch decision.Reason {
	case "loss_limit", "equity_unavailable", "stale_or_missing_observation", "one_sided_or_locked_book",
		"limit_or_touch_unavailable", "below_minimum_executable_qty", "position_gap_overflow",
		"inventory_at_target", "quote_cash_limit":
		return true
	default:
		return false
	}
}

func cdfDecisionMarketStateEqual(previous, current cdfDecisionEvidence) bool {
	// Compare the delayed public market and coherent risk mark, not the
	// supplier's private reference or target. Those private fields are the
	// response variables under test; treating their normal evolution as a
	// market change would let target-only replays evade this gate.
	return previous.BestBid == current.BestBid && previous.BestBidQty == current.BestBidQty &&
		previous.BestAsk == current.BestAsk && previous.BestAskQty == current.BestAskQty &&
		previous.MarkPrice == current.MarkPrice && previous.RiskMarkPrice == current.RiskMarkPrice &&
		previous.RiskMarkCurrent == current.RiskMarkCurrent && previous.LocalBookMode == current.LocalBookMode
}

func cdfEventAfter(event Event, timestamp int64, globalSequence uint64) bool {
	if globalSequence != 0 && event.GlobalSequence != 0 {
		return event.GlobalSequence > globalSequence
	}
	return event.SimTS > timestamp
}

func (r *CDFActivationAudit) validateCDFSubmitEconomics(event Event, state *cdfSupplierState, decision cdfDecisionEvidence) {
	gap, gapOK := checkedCDFSub(decision.TargetPosition, decision.Position)
	if !gapOK || gap == 0 || decision.QuoteQty > cdfAbs(gap) ||
		gap > 0 && decision.Side != "BUY" || gap < 0 && decision.Side != "SELL" {
		r.addEventCheck(event, state, "CDF submit side or quantity does not follow its finite inventory target gap")
	}
	switch decision.QuotePriceSource {
	case "two_sided_touch":
		if decision.LocalBookMode != "two_sided" || decision.Side == "BUY" && decision.QuotePrice != decision.BestBid ||
			decision.Side == "SELL" && decision.QuotePrice != decision.BestAsk {
			r.addEventCheck(event, state, "two-sided CDF quote does not rest at its delayed local touch")
		}
	case "one_sided_missing_side_blended":
		if decision.LocalBookMode != "one_sided" || !validCDFMissingSideQuote(decision, state.contract) {
			r.addEventCheck(event, state, "one-sided CDF quote does not follow the registered missing-side rule")
		}
	case "one_sided_present_touch":
		if decision.LocalBookMode != "one_sided" || decision.Side == "BUY" && decision.QuotePrice != decision.BestBid ||
			decision.Side == "SELL" && decision.QuotePrice != decision.BestAsk {
			r.addEventCheck(event, state, "one-sided CDF quote does not rest at its delayed present touch")
		}
	default:
		r.addEventCheck(event, state, "CDF submit has an unknown local quote-price source")
	}
	if decision.Side == "BUY" {
		notional, notionalOK := cdfActivationNotional(decision.QuotePrice, decision.QuoteQty, state.contract.BasePrecision)
		fee, feeOK := cdfActivationFee(notional, state.contract.MakerFeeBps)
		required, requiredOK := checkedCDFAdd(notional, fee)
		if !notionalOK || !feeOK || !requiredOK || decision.QuoteCashRequired != required || required > decision.QuoteCashAvailable {
			r.addEventCheck(event, state, "CDF buy quote exceeds or misstates finite quote cash")
		}
	} else if decision.QuoteCashRequired != 0 {
		r.addEventCheck(event, state, "CDF sell quote carries a nonzero quote-cash requirement")
	}
}

func (r *CDFActivationAudit) processCDFFill(event Event, states map[cdfParticipantKey]*cdfSupplierState, observed map[cdfFillKey]cdfFillEvidence) {
	var fill cdfFillEvidence
	if err := decodeRequiredJSON(event.Raw(), &fill,
		"role", "client_id", "symbol", "order_id", "trade_id", "timestamp", "side", "price", "qty",
		"fee_amount", "fee_asset", "is_full", "position_before", "position_after"); err != nil {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "malformed CDF supplier fill: " + err.Error()})
		return
	}
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil || fill.ClientID != event.ClientID || fill.Role != state.audit.Role || fill.Symbol != cdfActivationSymbol || fill.Timestamp != event.SimTS {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Role: fill.Role, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "CDF supplier fill identity is invalid"})
		return
	}
	key := cdfFillKey{event.VenueID, event.ClientID, fill.OrderID, fill.TradeID}
	if _, duplicate := observed[key]; duplicate {
		r.addEventCheck(event, state, "duplicate CDF supplier fill identity")
		return
	}
	if fill.OrderID == 0 || fill.TradeID == 0 || fill.Price <= 0 || fill.Qty <= 0 || fill.FeeAmount < 0 ||
		fill.FeeAsset != state.contract.QuoteAsset {
		r.addEventCheck(event, state, "CDF supplier fill has invalid order, quantity, price, or fee")
		return
	}
	expectedPosition := fill.PositionBefore
	var ok bool
	if fill.Side == "BUY" {
		expectedPosition, ok = checkedCDFAdd(fill.PositionBefore, fill.Qty)
	} else if fill.Side == "SELL" {
		expectedPosition, ok = checkedCDFSub(fill.PositionBefore, fill.Qty)
	} else {
		ok = false
	}
	expectedGrossInventory, grossOK := checkedCDFAdd(state.contract.InitialBaseBalance, fill.PositionAfter)
	if !ok || !grossOK || fill.PositionBefore != state.currentPosition || expectedPosition != fill.PositionAfter ||
		cdfAbsExceeds(fill.PositionAfter, state.contract.MaxPosition) || expectedGrossInventory < 0 || expectedGrossInventory > state.contract.MaxInventory {
		r.addEventCheck(event, state, "CDF supplier fill position transition is invalid")
		return
	}
	notional, notionalOK := cdfActivationNotional(fill.Price, fill.Qty, state.contract.BasePrecision)
	expectedFee, feeOK := cdfActivationFee(notional, state.contract.MakerFeeBps)
	if !notionalOK || !feeOK || fill.FeeAmount != expectedFee || fill.FeeAsset != state.contract.QuoteAsset {
		r.addEventCheck(event, state, "CDF supplier fill fee does not match the registered maker fee")
		return
	}
	baseDelta, quoteDelta, deltaOK := cdfExchangeFillBalanceDelta(fill.Side, notional, fill.Qty, fill.FeeAmount)
	updatedBase, baseDeltaOK := checkedCDFAdd(state.fillBaseDelta, baseDelta)
	updatedQuote, quoteDeltaOK := checkedCDFAdd(state.fillQuoteDelta, quoteDelta)
	if !deltaOK || !baseDeltaOK || !quoteDeltaOK {
		r.addEventCheck(event, state, "CDF supplier fill balance delta overflows")
		return
	}
	state.fillBaseDelta = updatedBase
	state.fillQuoteDelta = updatedQuote
	observed[key] = fill
	if r.observedFillGlobal == nil {
		r.observedFillGlobal = make(map[cdfFillKey]uint64)
	}
	r.observedFillGlobal[key] = event.GlobalSequence
	preFillDecision := state.lastDecision
	preFillKnown := state.hasLastDecision
	requiresFreshObservation := preFillKnown && fill.IsFull &&
		preFillDecision.LocalBookMode == "one_sided" &&
		preFillDecision.QuotePriceSource == "one_sided_missing_side_blended" &&
		(preFillDecision.QuoteOrderID == 0 || preFillDecision.QuoteOrderID == fill.OrderID)
	state.audit.FillCount++
	state.lastFillAt = event.SimTS
	state.lastFillPosition = fill.PositionAfter
	state.currentPosition = fill.PositionAfter
	state.fillResponses = append(state.fillResponses, cdfFillResponseWindow{
		fillAt: event.SimTS, fillGlobalSeq: event.GlobalSequence, positionAfter: fill.PositionAfter,
		preFillDecision: preFillDecision, preFillKnown: preFillKnown,
		requiresFreshObservation: requiresFreshObservation,
	})
	r.FillCount++
}

func (r *CDFActivationAudit) processCDFBalanceSnapshot(event Event, states map[cdfParticipantKey]*cdfSupplierState) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var snapshot cdfBalanceSnapshotEvidence
	if err := decodeRequiredJSON(event.Raw(), &snapshot, "timestamp", "client_id", "spot_balances", "perp_balances", "borrowed"); err != nil {
		r.addEventCheck(event, state, "malformed supplier balance snapshot: "+err.Error())
		return
	}
	if snapshot.ClientID != event.ClientID || snapshot.Timestamp != event.SimTS || snapshot.SpotBalances == nil || snapshot.PerpBalances == nil || snapshot.Borrowed == nil {
		r.addEventCheck(event, state, "supplier balance snapshot identity or presence contract is invalid")
		return
	}
	foundBase, foundQuote := false, false
	seenAssets := make(map[string]struct{}, len(snapshot.SpotBalances))
	for _, balance := range snapshot.SpotBalances {
		if balance.Asset == "" {
			r.addEventCheck(event, state, "supplier balance snapshot contains an unnamed asset")
			continue
		}
		if _, duplicate := seenAssets[balance.Asset]; duplicate {
			r.addEventCheck(event, state, "supplier balance snapshot contains duplicate asset rows")
		}
		seenAssets[balance.Asset] = struct{}{}
		if balance.Asset == state.contract.BaseAsset {
			foundBase = true
		}
		if balance.Asset == state.contract.QuoteAsset {
			foundQuote = true
		}
		net, ok := checkedCDFAdd(balance.Free, balance.Locked)
		if ok {
			net, ok = checkedCDFSub(net, balance.Borrowed)
		}
		if ok {
			net, ok = checkedCDFSub(net, balance.Interest)
		}
		if !ok || net != balance.NetAsset || balance.Free < 0 || balance.Locked < 0 || balance.Borrowed != 0 || balance.Interest != 0 {
			r.addEventCheck(event, state, "supplier spot balance snapshot violates no-debt arithmetic")
		}
		if balance.Asset != state.contract.BaseAsset && balance.Asset != state.contract.QuoteAsset && balance.NetAsset != 0 {
			r.addEventCheck(event, state, "supplier balance snapshot contains unregistered nonzero asset")
		}
	}
	baseDelta, quoteDelta := state.exchangeBaseDelta, state.exchangeQuoteDelta
	if !r.strictMechanics {
		baseDelta, quoteDelta = state.fillBaseDelta, state.fillQuoteDelta
	}
	expectedBase, baseOK := checkedCDFAdd(state.initialBaseBalance, baseDelta)
	expectedQuote, quoteOK := checkedCDFAdd(state.initialQuoteBalance, quoteDelta)
	actualBase, actualBaseOK := cdfAccountNetBalanceFromCDFBalances(snapshot.SpotBalances, state.contract.BaseAsset)
	actualQuote, actualQuoteOK := cdfAccountNetBalanceFromCDFBalances(snapshot.SpotBalances, state.contract.QuoteAsset)
	if !baseOK || !quoteOK || !actualBaseOK || !actualQuoteOK || actualBase != expectedBase || actualQuote != expectedQuote {
		r.addEventCheck(event, state, "supplier balance snapshot does not reconcile to prior exchange-matched fills")
	}
	for _, balance := range snapshot.PerpBalances {
		if balance.Free != 0 || balance.Locked != 0 || balance.Borrowed != 0 || balance.Interest != 0 || balance.NetAsset != 0 {
			r.addEventCheck(event, state, "supplier balance snapshot has nonzero derivative collateral")
		}
	}
	for _, amount := range snapshot.Borrowed {
		if amount != 0 {
			r.addEventCheck(event, state, "supplier balance snapshot has aggregate debt")
		}
	}
	if !foundBase || !foundQuote {
		r.addEventCheck(event, state, "supplier balance snapshot lacks configured assets")
	}
	state.audit.BalanceSnapshotCount++
	postFill := false
	if len(state.fillResponses) > 0 {
		lastFill := state.fillResponses[len(state.fillResponses)-1]
		postFill = cdfEventAfter(event, lastFill.fillAt, lastFill.fillGlobalSeq)
	}
	if postFill {
		state.audit.PostFillBalanceSnapshotCount++
	}
}

func cdfAccountNetBalanceFromCDFBalances(balances []cdfBalanceEvidence, asset string) (int64, bool) {
	for _, balance := range balances {
		if balance.Asset == asset {
			return balance.NetAsset, true
		}
	}
	return 0, false
}

func (r *CDFActivationAudit) processCDFBorrow(event Event, states map[cdfParticipantKey]*cdfSupplierState) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var borrow cdfBorrowEvidence
	if err := decodeRequiredJSON(event.Raw(), &borrow, "client_id", "asset", "amount"); err != nil {
		r.addEventCheck(event, state, "malformed supplier borrow evidence: "+err.Error())
		return
	}
	if borrow.ClientID != event.ClientID || borrow.Amount != 0 {
		r.addEventCheck(event, state, "CDF supplier used borrowing-backed liquidity")
	}
}

func (r *CDFActivationAudit) scanCDFBooks(
	run *Run,
	states map[cdfParticipantKey]*cdfSupplierState,
	submissions map[cdfRequestKey]*cdfSubmission,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
	orders map[cdfOrderKey]*cdfOrderState,
	actualFills map[cdfFillKey]cdfOrderFillEvidence,
	depth map[string][]cdfDepthObservation,
) error {
	bookCount := 0
	publicDepth := make(map[string]*cdfPublicDepthState)
	pendingDepth := make(map[string][]cdfPendingDepthObservation)
	for _, path := range run.Files() {
		if symbolFromPath(path) != cdfActivationLogName {
			continue
		}
		bookCount++
		lastTimestamp := int64(math.MinInt64)
		err := run.Scan(ScanOptions{
			Events: []string{"BookSnapshot", "BookDelta", "Trade", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "OrderCancelRejected"},
			Files:  []string{path}, FilesSelected: true, Workers: 1,
		}, func(event Event) {
			if event.SimTS < lastTimestamp {
				r.addCheck(CDFActivationCheck{VenueID: event.VenueID, ClientID: event.ClientID, Ordinal: event.Ordinal, Failure: "CDF book evidence timestamps regress"})
			}
			lastTimestamp = event.SimTS
			r.flushCDFPendingDepthBeforeEvent(event, states, orders, depth, pendingDepth)
			switch event.Name {
			case "BookSnapshot":
				r.processCDFDepthSnapshot(event, states, orders, depth, publicDepth, pendingDepth)
			case "BookDelta":
				r.processCDFDepthDelta(event, states, orders, depth, publicDepth, pendingDepth)
			case "Trade":
				r.processCDFTrade(event)
			case "OrderAccepted":
				r.processCDFAccepted(event, states, submissions, orders)
			case "OrderRejected":
				r.processCDFRejected(event, states, submissions)
			case "OrderFill":
				r.processCDFOrderFill(event, states, orders, actualFills)
			case "OrderCancelled":
				r.processCDFCancelled(event, states, withdrawals, orders, depth, publicDepth)
				r.flushCDFPendingDepth(event.VenueID, states, orders, depth, pendingDepth, true)
			case "OrderCancelRejected":
				r.processCDFCancelRejected(event, states, withdrawals, orders)
			}
		})
		r.flushAllCDFPendingDepth(states, orders, depth, pendingDepth)
		if err != nil {
			return fmt.Errorf("cdf activation: scan CDF book in %s: %w", path, err)
		}
	}
	if bookCount == 0 {
		r.addCheck(CDFActivationCheck{Failure: "no rendered CDF/USD book evidence"})
	}
	return nil
}

func (r *CDFActivationAudit) processCDFAccepted(event Event, states map[cdfParticipantKey]*cdfSupplierState, submissions map[cdfRequestKey]*cdfSubmission, orders map[cdfOrderKey]*cdfOrderState) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var accepted cdfAcceptedEvidence
	if err := decodeRequiredJSON(event.Raw(), &accepted, "order_id", "client_id", "request_id", "side", "type", "time_in_force", "post_only", "price", "qty"); err != nil {
		r.addEventCheck(event, state, "malformed CDF OrderAccepted evidence: "+err.Error())
		return
	}
	requestKey := cdfRequestKey{event.VenueID, event.ClientID, accepted.RequestID}
	submission := submissions[requestKey]
	if submission == nil || accepted.ClientID != event.ClientID || accepted.OrderID == 0 || accepted.Type != "LIMIT" ||
		accepted.TimeInForce != "GTC" || !accepted.PostOnly || accepted.Side != submission.decision.Side ||
		accepted.Price != submission.decision.QuotePrice || accepted.Qty != submission.decision.QuoteQty ||
		(r.strictMechanics && !cdfEventAfter(event, submission.event.SimTS, submission.event.GlobalSequence)) ||
		(!r.strictMechanics && event.SimTS < submission.event.SimTS) {
		r.addEventCheck(event, state, "accepted CDF order does not match a prior bounded passive submission")
		return
	}
	if submission.accepted || submission.rejected {
		r.addEventCheck(event, state, "CDF submission has duplicate acceptance/rejection outcomes")
		return
	}
	orderKey := cdfOrderKey{event.VenueID, event.ClientID, accepted.OrderID}
	if _, duplicate := orders[orderKey]; duplicate {
		r.addEventCheck(event, state, "duplicate accepted CDF order identity")
		return
	}
	participantKey := cdfParticipantKey{event.VenueID, event.ClientID}
	if r.liveOrderBySupplier == nil {
		r.liveOrderBySupplier = make(map[cdfParticipantKey]cdfOrderKey)
	}
	if liveOrder, duplicate := r.liveOrderBySupplier[participantKey]; duplicate {
		r.addEventCheck(event, state, fmt.Sprintf("CDF supplier accepted order %d while order %d was still live", accepted.OrderID, liveOrder.orderID))
		return
	}
	for liveOrder := range orders {
		if liveOrder.venueID == event.VenueID && liveOrder.clientID == event.ClientID {
			r.addEventCheck(event, state, fmt.Sprintf("CDF supplier has multiple live orders %d and %d", liveOrder.orderID, accepted.OrderID))
			return
		}
	}
	if r.terminalOrders != nil {
		if _, duplicate := r.terminalOrders[orderKey]; duplicate {
			r.addEventCheck(event, state, "CDF order identity was reused after a terminal outcome")
			return
		}
	}
	if state.pendingReprice {
		if r.strictMechanics {
			if submission.decision.ReplacesOrderID != state.pendingRepriceOrderID {
				r.addEventCheck(event, state, "CDF replacement acceptance lacks the matching replaced-order identity")
			} else {
				if !(accepted.Side == state.pendingRepriceSide && accepted.Price == state.pendingRepricePrice && accepted.Qty == state.pendingRepriceQty) &&
					!incrementCDFCounter(&state.audit.CompletedRepriceCount) {
					r.addEventCheck(event, state, "completed CDF reprice counter overflows")
				}
				state.pendingReprice = false
				state.pendingRepriceOrderID = 0
				state.pendingRepriceSide = ""
				state.pendingRepricePrice = 0
				state.pendingRepriceQty = 0
			}
		} else {
			if accepted.Side != state.pendingRepriceSide || accepted.Price != state.pendingRepricePrice || accepted.Qty != state.pendingRepriceQty {
				if !incrementCDFCounter(&state.audit.CompletedRepriceCount) {
					r.addEventCheck(event, state, "completed CDF reprice counter overflows")
				}
			}
			state.pendingReprice = false
			state.pendingRepriceOrderID = 0
			state.pendingRepriceSide = ""
			state.pendingRepricePrice = 0
			state.pendingRepriceQty = 0
		}
	}
	submission.accepted = true
	oneSidedCandidate := submission.decision.LocalBookMode == "one_sided" &&
		submission.decision.QuotePriceSource == "one_sided_missing_side_blended" &&
		submission.decision.QuoteQty >= submission.decision.MinimumQualifyingQty
	orders[orderKey] = &cdfOrderState{
		requestID: accepted.RequestID, side: accepted.Side, price: accepted.Price,
		originalQty: accepted.Qty, remainingQty: accepted.Qty, acceptedAt: event.SimTS,
		acceptedGlobalSeq: event.GlobalSequence, decisionAction: submission.decision.Action,
		decisionReason:    submission.decision.Reason,
		oneSidedCandidate: oneSidedCandidate, minimumQualifying: submission.decision.MinimumQualifyingQty,
	}
	r.liveOrderBySupplier[participantKey] = orderKey
	state.audit.AcceptedOrderCount++
	r.AcceptedOrderCount++
}

func (r *CDFActivationAudit) processCDFRejected(event Event, states map[cdfParticipantKey]*cdfSupplierState, submissions map[cdfRequestKey]*cdfSubmission) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var rejected cdfRejectedEvidence
	if err := decodeRequiredJSON(event.Raw(), &rejected, "request_id", "success", "error"); err != nil {
		r.addEventCheck(event, state, "malformed CDF OrderRejected evidence: "+err.Error())
		return
	}
	submission := submissions[cdfRequestKey{event.VenueID, event.ClientID, rejected.RequestID}]
	if submission == nil || rejected.Success || rejected.Error == "" || event.SimTS < submission.event.SimTS {
		r.addEventCheck(event, state, "CDF order rejection has no matching prior submission")
		return
	}
	if submission.accepted || submission.rejected {
		r.addEventCheck(event, state, "CDF submission has duplicate acceptance/rejection outcomes")
		return
	}
	submission.rejected = true
}

func (r *CDFActivationAudit) processCDFOrderFill(event Event, states map[cdfParticipantKey]*cdfSupplierState, orders map[cdfOrderKey]*cdfOrderState, actual map[cdfFillKey]cdfOrderFillEvidence) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var fill cdfOrderFillEvidence
	if err := decodeRequiredJSON(event.Raw(), &fill,
		"order_id", "trade_id", "side", "price", "qty", "fee_amount", "fee_asset", "filled_qty", "remaining_qty", "is_full"); err != nil {
		r.addEventCheck(event, state, "malformed exchange OrderFill evidence: "+err.Error())
		return
	}
	orderKey := cdfOrderKey{event.VenueID, event.ClientID, fill.OrderID}
	order := orders[orderKey]
	if order == nil || fill.TradeID == 0 || fill.Side != order.side || fill.Price != order.price || fill.Qty <= 0 ||
		fill.Qty > order.remainingQty || fill.FilledQty <= 0 || fill.RemainingQty < 0 ||
		(r.strictMechanics && !cdfEventAfter(event, order.acceptedAt, order.acceptedGlobalSeq)) ||
		(!r.strictMechanics && event.SimTS < order.acceptedAt) {
		r.addEventCheck(event, state, "exchange OrderFill does not match a live accepted CDF order")
		return
	}
	key := cdfFillKey{event.VenueID, event.ClientID, fill.OrderID, fill.TradeID}
	if _, duplicate := actual[key]; duplicate {
		r.addEventCheck(event, state, "duplicate exchange OrderFill identity")
		return
	}
	expectedRemaining, remainingOK := checkedCDFSub(order.remainingQty, fill.Qty)
	expectedFilled, filledOK := checkedCDFAdd(order.filledQty, fill.Qty)
	filledWithRemaining, totalOK := checkedCDFAdd(fill.FilledQty, fill.RemainingQty)
	if !remainingOK || !filledOK || !totalOK || expectedRemaining != fill.RemainingQty ||
		expectedFilled != fill.FilledQty || filledWithRemaining != order.originalQty ||
		fill.IsFull != (fill.RemainingQty == 0) {
		r.addEventCheck(event, state, "exchange OrderFill remaining quantity is inconsistent")
		return
	}
	if fill.FeeAmount < 0 || fill.FeeAsset != state.contract.QuoteAsset {
		r.addEventCheck(event, state, "exchange OrderFill carries an invalid maker fee")
		return
	}
	notional, notionalOK := cdfActivationNotional(fill.Price, fill.Qty, state.contract.BasePrecision)
	expectedFee, feeOK := cdfActivationFee(notional, state.contract.MakerFeeBps)
	if !notionalOK || !feeOK || fill.FeeAmount != expectedFee {
		r.addEventCheck(event, state, "exchange OrderFill fee does not match the registered maker fee")
		return
	}
	baseDelta, quoteDelta, deltaOK := cdfExchangeFillBalanceDelta(fill.Side, notional, fill.Qty, fill.FeeAmount)
	if !deltaOK {
		r.addEventCheck(event, state, "exchange OrderFill balance delta overflows")
		return
	}
	updatedBase, baseDeltaOK := checkedCDFAdd(state.exchangeBaseDelta, baseDelta)
	updatedQuote, quoteDeltaOK := checkedCDFAdd(state.exchangeQuoteDelta, quoteDelta)
	if !baseDeltaOK || !quoteDeltaOK {
		r.addEventCheck(event, state, "exchange OrderFill aggregate balance delta overflows")
		return
	}
	updatedFillCount, fillCountOK := checkedCDFAdd(order.fillCount, 1)
	if !fillCountOK {
		r.addEventCheck(event, state, "CDF order fill count overflows")
		return
	}
	actual[key] = fill
	if r.actualFillGlobal == nil {
		r.actualFillGlobal = make(map[cdfFillKey]uint64)
	}
	r.actualFillGlobal[key] = event.GlobalSequence
	state.exchangeBaseDelta = updatedBase
	state.exchangeQuoteDelta = updatedQuote
	order.remainingQty = expectedRemaining
	order.filledQty = expectedFilled
	order.fillCount = updatedFillCount
	if order.firstFillAt == 0 {
		order.firstFillAt = event.SimTS
	}
	if order.remainingQty == 0 {
		if !r.recordCDFOrderClosure(state, order, event.SimTS, "filled") {
			r.addEventCheck(event, state, "filled CDF order has an invalid or overflowing quote lifetime")
		}
		order.terminalState = "filled"
		r.rememberCDFTerminalOrder(orderKey, order)
		delete(orders, orderKey)
		r.forgetCDFLiveOrder(cdfParticipantKey{event.VenueID, event.ClientID}, orderKey)
	}
}

func cdfExchangeFillBalanceDelta(side string, notional, quantity, fee int64) (int64, int64, bool) {
	if notional < 0 || quantity <= 0 || fee < 0 {
		return 0, 0, false
	}
	switch side {
	case "BUY":
		cashOut, ok := checkedCDFAdd(notional, fee)
		if !ok || cashOut <= 0 {
			return 0, 0, false
		}
		return quantity, -cashOut, true
	case "SELL":
		cashIn, ok := checkedCDFSub(notional, fee)
		if !ok {
			return 0, 0, false
		}
		return -quantity, cashIn, true
	default:
		return 0, 0, false
	}
}

func (r *CDFActivationAudit) processCDFCancelled(
	event Event,
	states map[cdfParticipantKey]*cdfSupplierState,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
	orders map[cdfOrderKey]*cdfOrderState,
	depth map[string][]cdfDepthObservation,
	publicDepth map[string]*cdfPublicDepthState,
) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var cancelled cdfCancelledEvidence
	if err := decodeRequiredJSON(event.Raw(), &cancelled, "order_id", "remaining_qty"); err != nil {
		r.addEventCheck(event, state, "malformed CDF OrderCancelled evidence: "+err.Error())
		return
	}
	orderKey := cdfOrderKey{event.VenueID, event.ClientID, cancelled.OrderID}
	order := orders[orderKey]
	if order == nil || cancelled.OrderID == 0 || cancelled.RemainingQty != order.remainingQty ||
		(r.strictMechanics && cancelled.RequestID == 0 && cancelled.Reason == "") {
		r.addEventCheck(event, state, "CDF cancellation does not close a live order with a valid terminal reason")
		return
	}
	if cancelled.RequestID == 0 {
		if !strings.HasPrefix(cancelled.Reason, "EXCHANGE_FORCED_") {
			r.addEventCheck(event, state, "requestless CDF cancellation lacks an exchange-forced reason")
			return
		}
		if r.strictMechanics && !cdfEventAfter(event, order.acceptedAt, order.acceptedGlobalSeq) {
			r.addEventCheck(event, state, "forced CDF cancellation precedes order acceptance")
			return
		}
		if !r.recordCDFOrderClosure(state, order, event.SimTS, "forced_cancelled") {
			r.addEventCheck(event, state, "forced CDF cancellation has an invalid or overflowing quote lifetime")
		}
		order.terminalState = "forced_cancelled"
		r.rememberCDFTerminalOrder(orderKey, order)
		delete(orders, orderKey)
		r.forgetCDFLiveOrder(cdfParticipantKey{event.VenueID, event.ClientID}, orderKey)
		if r.strictMechanics && publicDepth[event.VenueID] != nil {
			r.recordCDFDepthObservation(event, states, orders, depth, publicDepth[event.VenueID], false)
		}
		return
	}
	withdrawal := withdrawals[cdfRequestKey{event.VenueID, event.ClientID, cancelled.RequestID}]
	if withdrawal == nil || withdrawal.decision.QuoteOrderID != cancelled.OrderID ||
		(r.strictMechanics && !cdfEventAfter(event, withdrawal.event.SimTS, withdrawal.event.GlobalSequence)) ||
		(!r.strictMechanics && event.SimTS < withdrawal.event.SimTS) {
		r.addEventCheck(event, state, "CDF cancellation does not close a live order and prior withdrawal decision")
		return
	}
	if withdrawal.closed || withdrawal.cancelRejected {
		r.addEventCheck(event, state, "duplicate CDF cancellation outcome")
		return
	}
	if !r.recordCDFOrderClosure(state, order, event.SimTS, "cancelled") {
		r.addEventCheck(event, state, "cancelled CDF order has an invalid or overflowing quote lifetime")
	}
	withdrawal.closed = true
	delete(orders, orderKey)
	order.terminalState = "cancelled"
	r.rememberCDFTerminalOrder(orderKey, order)
	r.forgetCDFLiveOrder(cdfParticipantKey{event.VenueID, event.ClientID}, orderKey)
	if !incrementCDFCounter(&state.audit.WithdrawalCount) {
		r.addEventCheck(event, state, "CDF withdrawal counter overflows")
	}
	if !incrementCDFCounter(&r.WithdrawalCount) {
		r.addEventCheck(event, state, "aggregate CDF withdrawal counter overflows")
	}
	switch withdrawal.decision.Action {
	case "withdraw":
		if !incrementCDFCounter(&state.audit.SuccessfulWithdrawalCount) {
			r.addEventCheck(event, state, "CDF successful withdrawal counter overflows")
		}
	case "cancel":
		if withdrawal.decision.Reason == "reprice_for_inventory_or_touch" {
			if !incrementCDFCounter(&state.audit.RepriceCancelCount) {
				r.addEventCheck(event, state, "CDF reprice cancellation counter overflows")
			}
			if state.pendingReprice {
				r.addEventCheck(event, state, "CDF reprice cancellation arrived before the prior replacement")
			}
			state.pendingReprice = true
			state.pendingRepriceOrderID = orderKey.orderID
			state.pendingRepriceSide = order.side
			state.pendingRepricePrice = order.price
			state.pendingRepriceQty = order.remainingQty
		}
	}
	if r.strictMechanics && publicDepth[event.VenueID] != nil {
		r.recordCDFDepthObservation(event, states, orders, depth, publicDepth[event.VenueID], false)
	}
}

func (r *CDFActivationAudit) processCDFCancelRejected(
	event Event,
	states map[cdfParticipantKey]*cdfSupplierState,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
	orders map[cdfOrderKey]*cdfOrderState,
) {
	state := states[cdfParticipantKey{event.VenueID, event.ClientID}]
	if state == nil {
		return
	}
	var rejected cdfCancelRejectedEvidence
	if err := decodeRequiredJSON(event.Raw(), &rejected, "order_id", "request_id", "success", "error"); err != nil {
		r.addEventCheck(event, state, "malformed CDF OrderCancelRejected evidence: "+err.Error())
		return
	}
	withdrawal := withdrawals[cdfRequestKey{event.VenueID, event.ClientID, rejected.RequestID}]
	if withdrawal == nil || rejected.OrderID == 0 || rejected.RequestID == 0 || rejected.Success || rejected.Error == "" ||
		withdrawal.decision.QuoteOrderID != rejected.OrderID ||
		(r.strictMechanics && !cdfEventAfter(event, withdrawal.event.SimTS, withdrawal.event.GlobalSequence)) ||
		(!r.strictMechanics && event.SimTS < withdrawal.event.SimTS) {
		r.addEventCheck(event, state, "CDF cancellation rejection has no matching prior withdrawal decision")
		return
	}
	if withdrawal.closed || withdrawal.cancelRejected {
		r.addEventCheck(event, state, "duplicate CDF cancellation rejection outcome")
		return
	}
	orderKey := cdfOrderKey{event.VenueID, event.ClientID, rejected.OrderID}
	if order := orders[orderKey]; order != nil {
		if rejected.Error == etypes.RejectOrderNotFound || rejected.Error == etypes.RejectOrderAlreadyFilled {
			r.addEventCheck(event, state, "CDF cancellation rejection contradicts a still-live order")
			return
		}
	} else if terminal := r.terminalOrders[orderKey]; terminal == nil {
		r.addEventCheck(event, state, "CDF cancellation rejection has no live or previously resolved order")
		return
	} else if terminal.terminalState != "filled" && terminal.terminalState != "forced_cancelled" ||
		(rejected.Error != etypes.RejectOrderNotFound && rejected.Error != etypes.RejectOrderAlreadyFilled) {
		r.addEventCheck(event, state, "CDF cancellation rejection does not match the resolved order terminal state")
		return
	}
	withdrawal.cancelRejected = true
	if !incrementCDFCounter(&state.audit.CancelRejectedCount) {
		r.addEventCheck(event, state, "CDF cancellation rejection counter overflows")
	}
}

func incrementCDFCounter(counter *int64) bool {
	updated, ok := checkedCDFAdd(*counter, 1)
	if ok {
		*counter = updated
	}
	return ok
}

func (r *CDFActivationAudit) rememberCDFTerminalOrder(key cdfOrderKey, order *cdfOrderState) {
	if r.terminalOrders == nil {
		r.terminalOrders = make(map[cdfOrderKey]*cdfOrderState)
	}
	if _, duplicate := r.terminalOrders[key]; !duplicate {
		r.terminalOrders[key] = order
	}
}

func (r *CDFActivationAudit) forgetCDFLiveOrder(participant cdfParticipantKey, key cdfOrderKey) {
	if r.liveOrderBySupplier == nil {
		return
	}
	if liveOrder, exists := r.liveOrderBySupplier[participant]; exists && liveOrder == key {
		delete(r.liveOrderBySupplier, participant)
	}
}

func (r *CDFActivationAudit) recordCDFOrderClosure(state *cdfSupplierState, order *cdfOrderState, closedAt int64, terminalKind string) bool {
	if state == nil || order == nil || order.terminalState != "" {
		return false
	}
	duration, durationOK := checkedCDFSub(closedAt, order.acceptedAt)
	if !durationOK || duration < 0 {
		return false
	}
	totalLifetime, totalOK := checkedCDFAdd(state.audit.TotalQuoteLifetimeNano, duration)
	if !totalOK {
		return false
	}
	maxLifetime := state.audit.MaxQuoteLifetimeNano
	if duration > maxLifetime {
		maxLifetime = duration
	}
	nextFilled, nextCancelled, nextForced, nextCensored := state.audit.FilledOrderCount, state.audit.CancelledOrderCount, state.audit.ForcedCancelCount, state.audit.CensoredOrderCount
	var censoredLifetime int64
	switch terminalKind {
	case "filled":
		if !incrementCDFCounter(&nextFilled) {
			return false
		}
	case "cancelled":
		if !incrementCDFCounter(&nextCancelled) {
			return false
		}
	case "forced_cancelled":
		if !incrementCDFCounter(&nextForced) {
			return false
		}
	case "censored":
		if !incrementCDFCounter(&nextCensored) {
			return false
		}
		var censoredOK bool
		censoredLifetime, censoredOK = checkedCDFAdd(state.audit.CensoredQuoteLifetimeNano, duration)
		if !censoredOK {
			return false
		}
	default:
		return false
	}
	state.audit.TotalQuoteLifetimeNano = totalLifetime
	state.audit.MaxQuoteLifetimeNano = maxLifetime
	state.audit.FilledOrderCount = nextFilled
	state.audit.CancelledOrderCount = nextCancelled
	state.audit.ForcedCancelCount = nextForced
	state.audit.CensoredOrderCount = nextCensored
	if terminalKind == "censored" {
		state.audit.CensoredQuoteLifetimeNano = censoredLifetime
	}
	return true
}

func (r *CDFActivationAudit) processCDFTrade(event Event) {
	var trade cdfTradeEvidence
	required := []string{"trade_id", "price", "qty", "side"}
	if r.strictMechanics {
		required = append(required, "maker_order_id", "taker_order_id")
	}
	if err := decodeRequiredJSON(event.Raw(), &trade, required...); err != nil {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed CDF trade evidence: " + err.Error()})
		return
	}
	if trade.TradeID == 0 || trade.Price <= 0 || trade.Qty <= 0 || (trade.Side != "BUY" && trade.Side != "SELL") ||
		(r.strictMechanics && (trade.MakerOrderID == 0 || trade.TakerOrderID == 0 || trade.MakerOrderID == trade.TakerOrderID)) {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF trade evidence has invalid identity, price, quantity, or side"})
		return
	}
	if r.trades == nil {
		r.trades = make(map[cdfTradeKey]cdfTradeEvidence)
	}
	if r.totalVolumeByVenue == nil {
		r.totalVolumeByVenue = make(map[string]int64)
	}
	tradeKey := cdfTradeKey{venueID: event.VenueID, tradeID: trade.TradeID}
	if _, duplicate := r.trades[tradeKey]; duplicate {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "duplicate CDF trade identity"})
		return
	}
	trade.globalSequence = event.GlobalSequence
	r.trades[tradeKey] = trade
	if r.tradeGlobal == nil {
		r.tradeGlobal = make(map[cdfTradeKey]uint64)
	}
	r.tradeGlobal[tradeKey] = event.GlobalSequence
	updatedTotal, totalOK := checkedCDFAdd(r.TotalVolumeQty, trade.Qty)
	updatedVenue, venueOK := checkedCDFAdd(r.totalVolumeByVenue[event.VenueID], trade.Qty)
	if !totalOK || !venueOK {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "aggregate CDF trade volume overflows"})
		return
	}
	r.TotalVolumeQty = updatedTotal
	r.totalVolumeByVenue[event.VenueID] = updatedVenue
}

func oppositeCDFSide(side string) string {
	if side == "BUY" {
		return "SELL"
	}
	if side == "SELL" {
		return "BUY"
	}
	return ""
}

func (r *CDFActivationAudit) processCDFDepthSnapshot(event Event, states map[cdfParticipantKey]*cdfSupplierState, orders map[cdfOrderKey]*cdfOrderState, depth map[string][]cdfDepthObservation, publicDepth map[string]*cdfPublicDepthState, pendingDepth map[string][]cdfPendingDepthObservation) {
	var snapshot cdfPublicSnapshotEvidence
	if err := decodeRequiredJSON(event.Raw(), &snapshot, "bids", "asks", "source_sequence", "public_bids", "public_asks"); err != nil {
		return
	}
	if snapshot.SourceSequence == 0 || snapshot.Bids == nil || snapshot.Asks == nil || snapshot.PublicBids == nil || snapshot.PublicAsks == nil || !validCDFSnapshotProjection(snapshot) {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF snapshot lacks a verified public projection"})
		return
	}
	state, ok := newCDFPublicDepthState(snapshot.PublicBids, snapshot.PublicAsks)
	if !ok {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF depth is negative or overflows"})
		return
	}
	publicDepth[event.VenueID] = state
	if r.strictMechanics && len(pendingDepth[event.VenueID]) > 0 {
		// A new snapshot is a complete public state boundary. If the previous
		// aggregate reduction still has no matching lifecycle transition, fail
		// that transition against the state visible before this snapshot rather
		// than letting a later replacement order rewrite its history.
		r.flushCDFPendingDepth(event.VenueID, states, orders, depth, pendingDepth, true)
	}
	r.recordCDFDepthObservation(event, states, orders, depth, state, true)
}

func (r *CDFActivationAudit) processCDFDepthDelta(event Event, states map[cdfParticipantKey]*cdfSupplierState, orders map[cdfOrderKey]*cdfOrderState, depth map[string][]cdfDepthObservation, publicDepth map[string]*cdfPublicDepthState, pendingDepth map[string][]cdfPendingDepthObservation) {
	state := publicDepth[event.VenueID]
	if state == nil || !state.initialized {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF BookDelta precedes a complete public snapshot"})
		return
	}
	var delta cdfBookDeltaEvidence
	if err := decodeRequiredJSON(event.Raw(), &delta, "side", "price", "visible_qty", "hidden_qty"); err != nil {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed public CDF delta: " + err.Error()})
		return
	}
	if delta.Price <= 0 || delta.VisibleQty < 0 || delta.HiddenQty < 0 || delta.Side != "BUY" && delta.Side != "SELL" {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF delta has invalid side, price, or quantity"})
		return
	}
	levels := state.bids
	if delta.Side == "SELL" {
		levels = state.asks
	}
	previousVisible := levels[delta.Price]
	if delta.VisibleQty == 0 {
		delete(levels, delta.Price)
	} else {
		levels[delta.Price] = delta.VisibleQty
	}
	// The exchange may publish an aggregate level reduction before the matching
	// cancellation frame. At that instant the supplier order map still contains
	// the order that caused the public reduction. Defer only a transition whose
	// currently reconstructed supplier depth would exceed the new public level;
	// unrelated reductions can be reconciled immediately. The deferred record
	// retains the level transition so a later event cannot satisfy it merely by
	// changing some other order at the same price.
	if r.strictMechanics && delta.VisibleQty < previousVisible {
		causalOrders := cdfOrdersAtDepthLevel(orders, states, event.VenueID, delta.Side, delta.Price)
		if cdfSupplierDepthAtLevel(causalOrders) > delta.VisibleQty {
			r.deferCDFDepthObservation(event, state, pendingDepth, causalOrders, delta.Side, delta.Price, previousVisible, delta.VisibleQty, false)
			return
		}
	}
	r.recordCDFDepthObservation(event, states, orders, depth, state, false)
}

func cloneCDFPublicDepthState(state *cdfPublicDepthState) cdfPublicDepthState {
	clone := cdfPublicDepthState{initialized: state != nil && state.initialized,
		bids: make(map[int64]int64), asks: make(map[int64]int64)}
	if state == nil {
		return clone
	}
	for price, quantity := range state.bids {
		clone.bids[price] = quantity
	}
	for price, quantity := range state.asks {
		clone.asks[price] = quantity
	}
	return clone
}

func (r *CDFActivationAudit) deferCDFDepthObservation(event Event, state *cdfPublicDepthState, pending map[string][]cdfPendingDepthObservation, causalOrders map[cdfOrderKey]int64, side string, price, previousVisible, newVisible int64, isSnapshot bool) {
	if state == nil {
		return
	}
	pending[event.VenueID] = append(pending[event.VenueID], cdfPendingDepthObservation{
		event: event, state: cloneCDFPublicDepthState(state), causalOrders: causalOrders,
		side: side, price: price, previousVisible: previousVisible, newVisible: newVisible,
		isSnapshot: isSnapshot,
	})
}

func cdfOrdersAtDepthLevel(orders map[cdfOrderKey]*cdfOrderState, states map[cdfParticipantKey]*cdfSupplierState, venueID, side string, price int64) map[cdfOrderKey]int64 {
	causalOrders := make(map[cdfOrderKey]int64)
	for key, order := range orders {
		if key.venueID != venueID || order == nil || order.side != side || order.price != price || order.remainingQty <= 0 {
			continue
		}
		if states[cdfParticipantKey{venueID: key.venueID, clientID: key.clientID}] == nil {
			continue
		}
		causalOrders[key] = order.remainingQty
	}
	return causalOrders
}

func cdfSupplierDepthAtLevel(orders map[cdfOrderKey]int64) int64 {
	var total int64
	for _, quantity := range orders {
		updated, ok := checkedCDFAdd(total, quantity)
		if !ok {
			return math.MaxInt64
		}
		total = updated
	}
	return total
}

func cdfPendingDepthResolved(observation cdfPendingDepthObservation, orders map[cdfOrderKey]*cdfOrderState) bool {
	levelReduction, ok := checkedCDFSub(observation.previousVisible, observation.newVisible)
	if !ok || levelReduction <= 0 {
		return false
	}
	changedOrders := 0
	matchingReduction := false
	for key, previousQuantity := range observation.causalOrders {
		current, exists := orders[key]
		currentQuantity := int64(0)
		if exists && current != nil {
			if current.side != observation.side || current.price != observation.price {
				return false
			}
			currentQuantity = current.remainingQty
		}
		if currentQuantity == previousQuantity {
			continue
		}
		changedOrders++
		quantityReduction, reductionOK := checkedCDFSub(previousQuantity, currentQuantity)
		if reductionOK && quantityReduction == levelReduction {
			matchingReduction = true
		}
	}
	return changedOrders == 1 && matchingReduction
}

func cdfPendingDepthCancellationMatches(observation cdfPendingDepthObservation, event Event, orders map[cdfOrderKey]*cdfOrderState) bool {
	if event.Name != "OrderCancelled" {
		return false
	}
	var cancelled cdfCancelledEvidence
	if err := decodeRequiredJSON(event.Raw(), &cancelled, "order_id", "remaining_qty"); err != nil {
		return false
	}
	key := cdfOrderKey{venueID: event.VenueID, clientID: event.ClientID, orderID: cancelled.OrderID}
	previousQuantity, causal := observation.causalOrders[key]
	if !causal {
		return false
	}
	order := orders[key]
	if order == nil || order.side != observation.side || order.price != observation.price ||
		cancelled.RemainingQty != order.remainingQty {
		return false
	}
	levelReduction, levelOK := checkedCDFSub(observation.previousVisible, observation.newVisible)
	return levelOK && levelReduction > 0 && previousQuantity == cancelled.RemainingQty &&
		previousQuantity == levelReduction
}

func (r *CDFActivationAudit) flushCDFPendingDepthBeforeEvent(event Event, states map[cdfParticipantKey]*cdfSupplierState, orders map[cdfOrderKey]*cdfOrderState, depth map[string][]cdfDepthObservation, pending map[string][]cdfPendingDepthObservation) {
	observations := pending[event.VenueID]
	if len(observations) == 0 {
		return
	}
	remaining := observations[:0]
	for _, observation := range observations {
		if cdfPendingDepthCancellationMatches(observation, event, orders) {
			remaining = append(remaining, observation)
			continue
		}
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: observation.event.Ordinal, Failure: "CDF deferred depth reduction lacks the exact next causal cancellation"})
		state := observation.state
		r.recordCDFDepthObservation(observation.event, states, orders, depth, &state, observation.isSnapshot)
	}
	if len(remaining) == 0 {
		delete(pending, event.VenueID)
	} else {
		pending[event.VenueID] = remaining
	}
}

func (r *CDFActivationAudit) flushCDFPendingDepth(
	venueID string,
	states map[cdfParticipantKey]*cdfSupplierState,
	orders map[cdfOrderKey]*cdfOrderState,
	depth map[string][]cdfDepthObservation,
	pending map[string][]cdfPendingDepthObservation,
	force ...bool,
) {
	observations := pending[venueID]
	if len(observations) == 0 {
		return
	}
	forceFlush := len(force) > 0 && force[0]
	remaining := observations[:0]
	for _, observation := range observations {
		if !forceFlush && !cdfPendingDepthResolved(observation, orders) {
			remaining = append(remaining, observation)
			continue
		}
		if forceFlush && !cdfPendingDepthResolved(observation, orders) {
			r.addCheck(CDFActivationCheck{VenueID: venueID, Ordinal: observation.event.Ordinal, Failure: "CDF deferred depth reduction never received an exact causal order transition"})
		}
		state := observation.state
		r.recordCDFDepthObservation(observation.event, states, orders, depth, &state, observation.isSnapshot)
	}
	if len(remaining) == 0 {
		delete(pending, venueID)
	} else {
		pending[venueID] = remaining
	}
}

func (r *CDFActivationAudit) flushAllCDFPendingDepth(
	states map[cdfParticipantKey]*cdfSupplierState,
	orders map[cdfOrderKey]*cdfOrderState,
	depth map[string][]cdfDepthObservation,
	pending map[string][]cdfPendingDepthObservation,
) {
	venues := make([]string, 0, len(pending))
	for venueID := range pending {
		venues = append(venues, venueID)
	}
	sort.Strings(venues)
	for _, venueID := range venues {
		r.flushCDFPendingDepth(venueID, states, orders, depth, pending, true)
	}
}

func (r *CDFActivationAudit) recordCDFDepthObservation(event Event, states map[cdfParticipantKey]*cdfSupplierState, orders map[cdfOrderKey]*cdfOrderState, depth map[string][]cdfDepthObservation, publicDepth *cdfPublicDepthState, allowRestoration bool) {
	bidDepth, bidOK := totalCDFDepthMap(publicDepth.bids)
	askDepth, askOK := totalCDFDepthMap(publicDepth.asks)
	if !bidOK || !askOK {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF depth overflows"})
		return
	}
	observation := cdfDepthObservation{
		at: event.SimTS, globalSequence: event.GlobalSequence, snapshot: allowRestoration,
		bidDepth: bidDepth, askDepth: askDepth,
		supplierDepthByKey: make(map[cdfParticipantKey]cdfSupplierDepth),
	}
	for key, order := range orders {
		if key.venueID != event.VenueID || states[cdfParticipantKey{key.venueID, key.clientID}] == nil {
			continue
		}
		participantKey := cdfParticipantKey{venueID: key.venueID, clientID: key.clientID}
		if order.side == "BUY" {
			observation.supplierBid, bidOK = checkedCDFAdd(observation.supplierBid, order.remainingQty)
			depthForSupplier := observation.supplierDepthByKey[participantKey]
			depthForSupplier.bid, bidOK = checkedCDFAdd(depthForSupplier.bid, order.remainingQty)
			observation.supplierDepthByKey[participantKey] = depthForSupplier
		} else {
			observation.supplierAsk, askOK = checkedCDFAdd(observation.supplierAsk, order.remainingQty)
			depthForSupplier := observation.supplierDepthByKey[participantKey]
			depthForSupplier.ask, askOK = checkedCDFAdd(depthForSupplier.ask, order.remainingQty)
			observation.supplierDepthByKey[participantKey] = depthForSupplier
		}
		if !bidOK || !askOK {
			r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "supplier resting depth overflows"})
			return
		}
		if allowRestoration && order.oneSidedCandidate && !order.restored && event.SimTS > order.acceptedAt &&
			cdfDepthRestoresOrder(publicDepth, order.side, order.price, order.minimumQualifying) {
			order.restored = true
			r.OneSidedRestorationCount++
		}
	}
	if observation.supplierBid > bidDepth || observation.supplierAsk > askDepth {
		r.addCheck(CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "reconstructed supplier depth exceeds public displayed depth"})
	}
	depth[event.VenueID] = append(depth[event.VenueID], observation)
}

func (r *CDFActivationAudit) reconcileCDFFills(
	states map[cdfParticipantKey]*cdfSupplierState,
	observed map[cdfFillKey]cdfFillEvidence,
	actual map[cdfFillKey]cdfOrderFillEvidence,
	orders map[cdfOrderKey]*cdfOrderState,
	terminalTimes ...int64,
) {
	for key, supplierFill := range observed {
		exchangeFill, exists := actual[key]
		state := states[cdfParticipantKey{key.venueID, key.clientID}]
		if state == nil {
			r.addCheck(CDFActivationCheck{VenueID: key.venueID, ClientID: key.clientID, Failure: "supplier fill is outside the registered CDF supplier roster"})
			continue
		}
		if !exists || exchangeFill.Side != supplierFill.Side || exchangeFill.Price != supplierFill.Price ||
			exchangeFill.Qty != supplierFill.Qty || exchangeFill.FeeAmount != supplierFill.FeeAmount ||
			exchangeFill.FeeAsset != supplierFill.FeeAsset || exchangeFill.IsFull != supplierFill.IsFull {
			r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: "supplier fill does not match exchange OrderFill evidence"})
		}
	}
	for key, fill := range actual {
		state := states[cdfParticipantKey{key.venueID, key.clientID}]
		if state == nil {
			r.addCheck(CDFActivationCheck{VenueID: key.venueID, ClientID: key.clientID, Failure: "exchange OrderFill is outside the registered CDF supplier roster"})
			continue
		}
		if _, exists := observed[key]; !exists {
			r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: "exchange OrderFill has no supplier inventory transition"})
		}
		tradeMatches := false
		trade, tradeExists := r.trades[cdfTradeKey{venueID: key.venueID, tradeID: key.tradeID}]
		if tradeExists {
			tradeMatches = trade.Price == fill.Price && trade.Qty == fill.Qty &&
				trade.Side == oppositeCDFSide(fill.Side) && trade.MakerOrderID == fill.OrderID &&
				trade.TakerOrderID != 0 && trade.TakerOrderID != fill.OrderID
		}
		producerOrderingValid := true
		if r.strictMechanics {
			if !tradeMatches {
				r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: "exchange OrderFill does not match a unique opposite-side CDF trade and maker order"})
			}
			if r.tradeGlobal != nil && r.actualFillGlobal != nil && r.observedFillGlobal != nil {
				tradeSequence := r.tradeGlobal[cdfTradeKey{venueID: key.venueID, tradeID: key.tradeID}]
				exchangeFillSequence := r.actualFillGlobal[key]
				supplierFillSequence := r.observedFillGlobal[key]
				if tradeSequence == 0 || exchangeFillSequence == 0 || supplierFillSequence == 0 ||
					!(tradeSequence < exchangeFillSequence && exchangeFillSequence < supplierFillSequence) {
					producerOrderingValid = false
					r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: "CDF fill producer ordering is not Trade < exchange OrderFill < supplier fill"})
				}
			}
		}
		if (!r.strictMechanics || tradeMatches) && producerOrderingValid {
			if !r.attributeCDFSupplierFill(state, fill) {
				r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: "supplier volume attribution overflows"})
			}
		}
	}
	for key, order := range orders {
		state := states[cdfParticipantKey{key.venueID, key.clientID}]
		if state == nil || order.remainingQty <= 0 || order.remainingQty > order.originalQty || order.filledQty < 0 {
			role := ""
			if state != nil {
				role = state.audit.Role
			}
			r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: role, ClientID: key.clientID, Failure: fmt.Sprintf("accepted CDF order %d has invalid terminal state with quantity %d", key.orderID, order.remainingQty)})
			continue
		}
		if r.strictMechanics {
			terminalAt := order.acceptedAt
			if len(terminalTimes) > 0 {
				terminalAt = terminalTimes[0]
			}
			if !r.recordCDFOrderClosure(state, order, terminalAt, "censored") {
				r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: fmt.Sprintf("accepted CDF order %d has an invalid or overflowing censored quote lifetime", key.orderID)})
			}
			order.terminalState = "censored"
			r.rememberCDFTerminalOrder(key, order)
			r.forgetCDFLiveOrder(cdfParticipantKey{key.venueID, key.clientID}, key)
			state.audit.OpenOrderCount++
			openOrderQty, ok := checkedCDFAdd(state.audit.OpenOrderQty, order.remainingQty)
			if !ok {
				r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: "terminal open CDF order quantity overflows"})
				continue
			}
			state.audit.OpenOrderQty = openOrderQty
			continue
		}
		r.addCheck(CDFActivationCheck{VenueID: key.venueID, Role: state.audit.Role, ClientID: key.clientID, Failure: fmt.Sprintf("accepted CDF order %d remains unresolved with quantity %d", key.orderID, order.remainingQty)})
	}
}

func (r *CDFActivationAudit) attributeCDFSupplierFill(state *cdfSupplierState, fill cdfOrderFillEvidence) bool {
	notional, notionalOK := cdfActivationNotional(fill.Price, fill.Qty, state.contract.BasePrecision)
	if !notionalOK {
		return false
	}
	tradeCount, tradeCountOK := checkedCDFAdd(state.audit.TradeCount, 1)
	volumeQty, volumeQtyOK := checkedCDFAdd(state.audit.VolumeQty, fill.Qty)
	volumeNotional, volumeNotionalOK := checkedCDFAdd(state.audit.VolumeNotionalQuote, notional)
	feesPaid, feesPaidOK := checkedCDFAdd(state.audit.FeesPaidQuote, fill.FeeAmount)
	globalVolume, globalVolumeOK := checkedCDFAdd(r.SupplierVolumeQty, fill.Qty)
	globalNotional, globalNotionalOK := checkedCDFAdd(r.SupplierVolumeNotional, notional)
	globalFees, globalFeesOK := checkedCDFAdd(r.SupplierFeesPaid, fill.FeeAmount)
	if !tradeCountOK || !volumeQtyOK || !volumeNotionalOK || !feesPaidOK || !globalVolumeOK || !globalNotionalOK || !globalFeesOK {
		return false
	}
	state.audit.TradeCount = tradeCount
	state.audit.VolumeQty = volumeQty
	state.audit.VolumeNotionalQuote = volumeNotional
	state.audit.FeesPaidQuote = feesPaid
	r.SupplierVolumeQty = globalVolume
	r.SupplierVolumeNotional = globalNotional
	r.SupplierFeesPaid = globalFees
	return true
}

func (r *CDFActivationAudit) finalizeCDFActivation(
	states map[cdfParticipantKey]*cdfSupplierState,
	submissions map[cdfRequestKey]*cdfSubmission,
	withdrawals map[cdfRequestKey]*cdfWithdrawal,
	depth map[string][]cdfDepthObservation,
	terminalAt int64,
	contract CDFActivationContract,
) {
	for _, submission := range submissions {
		if !submission.accepted && !submission.rejected {
			r.addCheck(CDFActivationCheck{VenueID: submission.event.VenueID, ClientID: submission.event.ClientID, Ordinal: submission.event.Ordinal, Failure: "CDF submission has no terminal acceptance/rejection outcome"})
		}
	}
	for _, withdrawal := range withdrawals {
		if !withdrawal.closed && !withdrawal.cancelRejected {
			r.addCheck(CDFActivationCheck{VenueID: withdrawal.event.VenueID, ClientID: withdrawal.event.ClientID, Ordinal: withdrawal.event.Ordinal, Failure: "CDF cancellation decision has no terminal OrderCancelled outcome"})
		}
	}
	allSuppliersActivated := len(states) == len(contract.VenueIDs)*len(contract.Suppliers)
	r.VolumeQtyByVenue = make(map[string]int64, len(r.totalVolumeByVenue))
	for venueID, volume := range r.totalVolumeByVenue {
		r.VolumeQtyByVenue[venueID] = volume
	}
	keys := make([]cdfParticipantKey, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venueID != keys[j].venueID {
			return keys[i].venueID < keys[j].venueID
		}
		return keys[i].clientID < keys[j].clientID
	})
	for _, key := range keys {
		state := states[key]
		if state.audit.MinPosition == math.MaxInt64 {
			state.audit.MinPosition = 0
			state.audit.MaxPosition = 0
		}
		// The preregistration requires every supplier/venue instance to trade and
		// show a later inventory response, but requires a lifecycle withdrawal or
		// reprice globally. Keep those scopes distinct; making every supplier
		// cancel would be an unregistered post-hoc strengthening.
		state.audit.ActivationSatisfied = state.initialAccountSeen && state.terminalAccountSeen &&
			state.audit.EligibleObservationCount > 0 && state.audit.AcceptedOrderCount > 0 &&
			state.audit.FillCount > 0 && state.audit.PostFillBalanceSnapshotCount > 0 &&
			state.audit.PostFillResponsiveCount > 0
		allSuppliersActivated = allSuppliersActivated && state.audit.ActivationSatisfied
		expectedBase, baseOK := checkedCDFAdd(state.initialBaseBalance, state.exchangeBaseDelta)
		expectedQuote, quoteOK := checkedCDFAdd(state.initialQuoteBalance, state.exchangeQuoteDelta)
		if !baseOK || !quoteOK || expectedBase != state.terminalBaseBalance || expectedQuote != state.terminalQuoteBalance {
			r.addCheck(CDFActivationCheck{VenueID: state.audit.VenueID, Role: state.audit.Role, ClientID: state.audit.ClientID, Failure: "terminal supplier balances do not reconcile to finite initial capital and exchange-matched fills"})
		}
		state.audit.VenueVolumeDenominatorQty = r.VolumeQtyByVenue[state.audit.VenueID]
		if state.audit.VenueVolumeDenominatorQty > 0 {
			state.audit.VenueVolumeShare = float64(state.audit.VolumeQty) / float64(state.audit.VenueVolumeDenominatorQty)
		}
		if r.TotalVolumeQty > 0 {
			state.audit.GlobalVolumeShare = float64(state.audit.VolumeQty) / float64(r.TotalVolumeQty)
		}
		venueObservations := depth[key.venueID]
		if r.strictMechanics {
			venueObservations = prependCDFInitialObservation(venueObservations, contract)
		}
		depthMetrics := measureCDFSupplierDepth(key, venueObservations, terminalAt, state.contract, contract)
		state.audit.DepthObservationCount = depthMetrics.observationCount
		state.audit.BidDepthTimeWeightedShare = depthMetrics.bidTimeWeightedShare
		state.audit.AskDepthTimeWeightedShare = depthMetrics.askTimeWeightedShare
		state.audit.BidQualifyingDepthShare = depthMetrics.bidQualifyingShare
		state.audit.AskQualifyingDepthShare = depthMetrics.askQualifyingShare
		state.audit.BidDepthDominanceTimeFraction = depthMetrics.bidDominanceTimeFraction
		state.audit.AskDepthDominanceTimeFraction = depthMetrics.askDominanceTimeFraction
		state.audit.BidRemovalDurationNano = depthMetrics.bidRemovalDurationNano
		state.audit.AskRemovalDurationNano = depthMetrics.askRemovalDurationNano
		state.audit.BidRemovalTimeFraction = depthMetrics.bidRemovalTimeFraction
		state.audit.AskRemovalTimeFraction = depthMetrics.askRemovalTimeFraction
		r.Suppliers = append(r.Suppliers, state.audit)
	}
	if r.TotalVolumeQty <= 0 || r.SupplierVolumeQty < 0 || r.SupplierVolumeQty > r.TotalVolumeQty {
		r.addCheck(CDFActivationCheck{Failure: "CDF volume denominator is missing or inconsistent with supplier fills"})
	} else {
		r.SupplierVolumeShare = float64(r.SupplierVolumeQty) / float64(r.TotalVolumeQty)
	}
	allVenueConcentrationSatisfied := true
	for _, venueID := range contract.VenueIDs {
		venueObservations := depth[venueID]
		if r.strictMechanics {
			r.Checks = append(r.Checks, validateCDFObservationCadence(venueID, venueObservations, contract.SimulationStartNano, terminalAt, contract.ObservationIntervalNano)...)
			venueObservations = prependCDFInitialObservation(venueObservations, contract)
		}
		venue := measureCDFVenueConcentration(venueID, venueObservations, terminalAt, contract)
		allVenueConcentrationSatisfied = allVenueConcentrationSatisfied && venue.ConcentrationSatisfied
		r.Venues = append(r.Venues, venue)
	}
	allSupplierConcentrationSatisfied := true
	for _, supplier := range r.Suppliers {
		if !cdfSupplierConcentrationSatisfied(supplier, contract) {
			allSupplierConcentrationSatisfied = false
		}
	}
	for index := range r.Suppliers {
		r.Suppliers[index].EvidenceValid = !r.hasParticipantCheck(r.Suppliers[index].VenueID, r.Suppliers[index].ClientID)
	}
	r.EvidenceValid = len(r.Checks) == 0
	r.ActivationSatisfied = r.EvidenceValid && allSuppliersActivated &&
		r.OneSidedDecisionCount > 0 && r.OneSidedRestorationCount > 0 && r.WithdrawalCount > 0
	r.AntiCheatingSatisfied = r.EvidenceValid && r.SupplierVolumeShare <= contract.MaximumSupplierVolumeShare &&
		allVenueConcentrationSatisfied && allSupplierConcentrationSatisfied
	r.Valid = r.EvidenceValid && r.ActivationSatisfied && r.AntiCheatingSatisfied
}

func cdfSupplierConcentrationSatisfied(supplier CDFSupplierActivationAudit, contract CDFActivationContract) bool {
	if supplier.VenueVolumeDenominatorQty <= 0 || supplier.VolumeQty < 0 ||
		supplier.VolumeQty > supplier.VenueVolumeDenominatorQty {
		return false
	}
	venueVolumeShare := float64(supplier.VolumeQty) / float64(supplier.VenueVolumeDenominatorQty)
	return supplier.DepthObservationCount > 0 &&
		venueVolumeShare <= contract.MaximumSupplierVolumeShare &&
		supplier.BidDepthDominanceTimeFraction <= contract.MaximumDepthDominanceTimeFraction &&
		supplier.AskDepthDominanceTimeFraction <= contract.MaximumDepthDominanceTimeFraction
}

func measureCDFVenueConcentration(venueID string, observations []cdfDepthObservation, terminalAt int64, contract CDFActivationContract) CDFVenueConcentrationAudit {
	result := CDFVenueConcentrationAudit{VenueID: venueID, SnapshotCount: int64(len(observations)), TerminalBookMode: "unobserved"}
	if len(observations) == 0 {
		return result
	}
	ordered := append([]cdfDepthObservation(nil), observations...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].at != ordered[j].at {
			return ordered[i].at < ordered[j].at
		}
		return ordered[i].globalSequence < ordered[j].globalSequence
	})
	currentNonTwoSidedDuration := int64(0)
	previousIntervalEnd := int64(0)
	hasPreviousInterval := false
	for index, observation := range ordered {
		if observation.at <= terminalAt {
			result.TerminalBookMode = cdfBookMode(observation.bidDepth, observation.askDepth)
		}
		end := terminalAt
		if index+1 < len(ordered) && ordered[index+1].at < end {
			end = ordered[index+1].at
		}
		if end <= observation.at {
			continue
		}
		duration := end - observation.at
		mode := cdfBookMode(observation.bidDepth, observation.askDepth)
		if mode == "two_sided" {
			if currentNonTwoSidedDuration > result.MaxUninterruptedNonTwoSidedDurationNano {
				result.MaxUninterruptedNonTwoSidedDurationNano = currentNonTwoSidedDuration
			}
			currentNonTwoSidedDuration = 0
		} else if hasPreviousInterval && observation.at == previousIntervalEnd {
			currentNonTwoSidedDuration += duration
		} else {
			if currentNonTwoSidedDuration > result.MaxUninterruptedNonTwoSidedDurationNano {
				result.MaxUninterruptedNonTwoSidedDurationNano = currentNonTwoSidedDuration
			}
			currentNonTwoSidedDuration = duration
		}
		if observation.bidDepth > 0 {
			result.BidActiveDurationNano += duration
			if float64(observation.supplierBid)/float64(observation.bidDepth) > contract.MaximumSupplierDepthShare {
				result.BidDominantDurationNano += duration
			}
		}
		if observation.askDepth > 0 {
			result.AskActiveDurationNano += duration
			if float64(observation.supplierAsk)/float64(observation.askDepth) > contract.MaximumSupplierDepthShare {
				result.AskDominantDurationNano += duration
			}
		}
		switch mode {
		case "bid_only":
			result.BidOnlyDurationNano += duration
		case "ask_only":
			result.AskOnlyDurationNano += duration
		case "empty":
			result.EmptyBookDurationNano += duration
		}
		previousIntervalEnd = end
		hasPreviousInterval = true
	}
	if currentNonTwoSidedDuration > result.MaxUninterruptedNonTwoSidedDurationNano {
		result.MaxUninterruptedNonTwoSidedDurationNano = currentNonTwoSidedDuration
	}
	result.OneSidedDurationNano = result.BidOnlyDurationNano + result.AskOnlyDurationNano
	result.NonTwoSidedDurationNano = result.OneSidedDurationNano + result.EmptyBookDurationNano
	if result.BidActiveDurationNano > 0 {
		result.BidDominanceTimeFraction = float64(result.BidDominantDurationNano) / float64(result.BidActiveDurationNano)
	}
	if result.AskActiveDurationNano > 0 {
		result.AskDominanceTimeFraction = float64(result.AskDominantDurationNano) / float64(result.AskActiveDurationNano)
	}
	result.ConcentrationSatisfied = result.BidActiveDurationNano > 0 && result.AskActiveDurationNano > 0 &&
		result.BidDominanceTimeFraction <= contract.MaximumDepthDominanceTimeFraction &&
		result.AskDominanceTimeFraction <= contract.MaximumDepthDominanceTimeFraction
	return result
}

func cdfBookMode(bidDepth, askDepth int64) string {
	switch {
	case bidDepth > 0 && askDepth > 0:
		return "two_sided"
	case bidDepth > 0:
		return "bid_only"
	case askDepth > 0:
		return "ask_only"
	default:
		return "empty"
	}
}

type cdfSupplierDepthMetrics struct {
	observationCount         int64
	bidTimeWeightedShare     float64
	askTimeWeightedShare     float64
	bidQualifyingShare       float64
	askQualifyingShare       float64
	bidDominanceTimeFraction float64
	askDominanceTimeFraction float64
	bidRemovalDurationNano   int64
	askRemovalDurationNano   int64
	bidRemovalTimeFraction   float64
	askRemovalTimeFraction   float64
}

// measureCDFSupplierDepth computes event-time, supplier-specific diagnostics
// from the reconstructed public book. A removal interval is one in which the
// supplier's qualifying side is the only reason the displayed side remains at
// or above the registered qualifying threshold. The qualifying share uses the
// same intervals and weights depth quantity rather than snapshot count.
func measureCDFSupplierDepth(
	key cdfParticipantKey,
	observations []cdfDepthObservation,
	terminalAt int64,
	contract CDFSupplierContract,
	activation CDFActivationContract,
) cdfSupplierDepthMetrics {
	metrics := cdfSupplierDepthMetrics{observationCount: int64(len(observations))}
	ordered := append([]cdfDepthObservation(nil), observations...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].at != ordered[j].at {
			return ordered[i].at < ordered[j].at
		}
		return ordered[i].globalSequence < ordered[j].globalSequence
	})
	var bidActive, askActive int64
	var bidTimeNumerator, askTimeNumerator float64
	var bidQualifyingNumerator, bidQualifyingDenominator float64
	var askQualifyingNumerator, askQualifyingDenominator float64
	for index, observation := range ordered {
		end := terminalAt
		if index+1 < len(ordered) {
			end = ordered[index+1].at
		}
		if end <= observation.at {
			continue
		}
		duration := end - observation.at
		if duration <= 0 {
			continue
		}
		depthForSupplier := observation.supplierDepthByKey[key]
		if observation.bidDepth > 0 {
			bidActive += duration
			share := float64(depthForSupplier.bid) / float64(observation.bidDepth)
			bidTimeNumerator += float64(duration) * share
			if share > activation.MaximumSupplierDepthShare {
				metrics.bidDominanceTimeFraction += float64(duration)
			}
			if depthForSupplier.bid >= contract.MinimumQualifyingQty {
				bidQualifyingNumerator += float64(depthForSupplier.bid)
				bidQualifyingDenominator += float64(observation.bidDepth)
				withoutSupplier, ok := checkedCDFSub(observation.bidDepth, depthForSupplier.bid)
				if ok && withoutSupplier < contract.MinimumQualifyingQty {
					metrics.bidRemovalDurationNano += duration
				}
			}
		}
		if observation.askDepth > 0 {
			askActive += duration
			share := float64(depthForSupplier.ask) / float64(observation.askDepth)
			askTimeNumerator += float64(duration) * share
			if share > activation.MaximumSupplierDepthShare {
				metrics.askDominanceTimeFraction += float64(duration)
			}
			if depthForSupplier.ask >= contract.MinimumQualifyingQty {
				askQualifyingNumerator += float64(depthForSupplier.ask)
				askQualifyingDenominator += float64(observation.askDepth)
				withoutSupplier, ok := checkedCDFSub(observation.askDepth, depthForSupplier.ask)
				if ok && withoutSupplier < contract.MinimumQualifyingQty {
					metrics.askRemovalDurationNano += duration
				}
			}
		}
	}
	if bidActive > 0 {
		metrics.bidTimeWeightedShare = bidTimeNumerator / float64(bidActive)
		metrics.bidDominanceTimeFraction /= float64(bidActive)
		metrics.bidRemovalTimeFraction = float64(metrics.bidRemovalDurationNano) / float64(bidActive)
	}
	if askActive > 0 {
		metrics.askTimeWeightedShare = askTimeNumerator / float64(askActive)
		metrics.askDominanceTimeFraction /= float64(askActive)
		metrics.askRemovalTimeFraction = float64(metrics.askRemovalDurationNano) / float64(askActive)
	}
	if bidQualifyingDenominator > 0 {
		metrics.bidQualifyingShare = bidQualifyingNumerator / bidQualifyingDenominator
	}
	if askQualifyingDenominator > 0 {
		metrics.askQualifyingShare = askQualifyingNumerator / askQualifyingDenominator
	}
	return metrics
}

func validCDFMissingSideQuote(decision cdfDecisionEvidence, contract CDFSupplierContract) bool {
	if decision.QuoteQty < contract.MinimumQualifyingQty {
		return false
	}
	expectedPrice, ok := expectedCDFMissingSideQuote(decision, contract)
	return ok && decision.QuotePrice == expectedPrice
}

func expectedCDFMissingSideQuote(decision cdfDecisionEvidence, contract CDFSupplierContract) (int64, bool) {
	if decision.ReferencePrice <= 0 || contract.TickSize <= 0 {
		return 0, false
	}
	switch {
	case decision.BestBid > 0 && decision.BestBidQty > 0 && decision.BestAsk == 0 && decision.BestAskQty == 0 && decision.Side == "SELL":
		lowerBound, ok := checkedCDFAdd(decision.BestBid, contract.TickSize)
		if !ok {
			return 0, false
		}
		candidate := maxCDFInt64(etypes.Midpoint(decision.ReferencePrice, decision.BestBid), lowerBound)
		return ceilCDFToTick(candidate, contract.TickSize)
	case decision.BestAsk > 0 && decision.BestAskQty > 0 && decision.BestBid == 0 && decision.BestBidQty == 0 && decision.Side == "BUY":
		upperBound, ok := checkedCDFSub(decision.BestAsk, contract.TickSize)
		if !ok || upperBound <= 0 {
			return 0, false
		}
		candidate := minCDFInt64(etypes.Midpoint(decision.ReferencePrice, decision.BestAsk), upperBound)
		return floorCDFToTick(candidate, contract.TickSize)
	default:
		return 0, false
	}
}

func floorCDFToTick(price, tick int64) (int64, bool) {
	if price <= 0 || tick <= 0 {
		return 0, false
	}
	quotient := price / tick
	if quotient <= 0 || quotient > math.MaxInt64/tick {
		return 0, false
	}
	return quotient * tick, true
}

func ceilCDFToTick(price, tick int64) (int64, bool) {
	if price <= 0 || tick <= 0 {
		return 0, false
	}
	quotient := price / tick
	if price%tick != 0 {
		if quotient == math.MaxInt64 {
			return 0, false
		}
		quotient++
	}
	if quotient <= 0 || quotient > math.MaxInt64/tick {
		return 0, false
	}
	return quotient * tick, true
}

// validCDFSnapshotProjection proves that the explicit public view is the
// visible projection of the complete snapshot. The proof is intentionally
// strict: the complete snapshot is capped by the venue's level limit, so a
// hidden-only level consuming that limit would make the public tail
// unprovable from the god view. Rejecting that record is safer than silently
// inferring a public book from incomplete evidence.
func validCDFSnapshotProjection(snapshot cdfPublicSnapshotEvidence) bool {
	fullBids, bidsOK := cdfVisibleProjection(snapshot.Bids)
	fullAsks, asksOK := cdfVisibleProjection(snapshot.Asks)
	publicBids, publicBidsOK := cdfPublicProjection(snapshot.PublicBids)
	publicAsks, publicAsksOK := cdfPublicProjection(snapshot.PublicAsks)
	return bidsOK && asksOK && publicBidsOK && publicAsksOK &&
		sameCDFPriceLevels(fullBids, publicBids) &&
		sameCDFPriceLevels(fullAsks, publicAsks)
}

func cdfVisibleProjection(levels []etypes.PriceLevel) ([]etypes.PriceLevel, bool) {
	projected := make([]etypes.PriceLevel, 0, len(levels))
	seenPrices := make(map[int64]struct{}, len(levels))
	for _, level := range levels {
		if level.Price <= 0 || level.VisibleQty < 0 || level.HiddenQty < 0 {
			return nil, false
		}
		if _, duplicate := seenPrices[level.Price]; duplicate {
			return nil, false
		}
		seenPrices[level.Price] = struct{}{}
		levelQuantity, ok := checkedCDFAdd(level.VisibleQty, level.HiddenQty)
		if !ok || levelQuantity <= 0 {
			return nil, false
		}
		if level.VisibleQty > 0 {
			projected = append(projected, etypes.PriceLevel{Price: level.Price, VisibleQty: level.VisibleQty})
		}
	}
	return projected, true
}

func cdfPublicProjection(levels []etypes.PriceLevel) ([]etypes.PriceLevel, bool) {
	projected := make([]etypes.PriceLevel, 0, len(levels))
	seenPrices := make(map[int64]struct{}, len(levels))
	for _, level := range levels {
		if level.Price <= 0 || level.VisibleQty <= 0 || level.HiddenQty != 0 {
			return nil, false
		}
		if _, duplicate := seenPrices[level.Price]; duplicate {
			return nil, false
		}
		seenPrices[level.Price] = struct{}{}
		projected = append(projected, level)
	}
	return projected, true
}

func sameCDFPriceLevels(left, right []etypes.PriceLevel) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Price != right[index].Price ||
			left[index].VisibleQty != right[index].VisibleQty ||
			left[index].HiddenQty != right[index].HiddenQty {
			return false
		}
	}
	return true
}

func newCDFPublicDepthState(bids, asks []etypes.PriceLevel) (*cdfPublicDepthState, bool) {
	state := &cdfPublicDepthState{initialized: true, bids: make(map[int64]int64, len(bids)), asks: make(map[int64]int64, len(asks))}
	for _, side := range []struct {
		levels []etypes.PriceLevel
		book   map[int64]int64
	}{{bids, state.bids}, {asks, state.asks}} {
		for _, level := range side.levels {
			if level.Price <= 0 || level.VisibleQty < 0 || level.HiddenQty < 0 {
				return nil, false
			}
			quantity, ok := checkedCDFAdd(side.book[level.Price], level.VisibleQty)
			if !ok {
				return nil, false
			}
			if quantity > 0 {
				side.book[level.Price] = quantity
			}
		}
	}
	return state, true
}

func totalCDFDepthMap(levels map[int64]int64) (int64, bool) {
	var total int64
	for price, quantity := range levels {
		if price <= 0 || quantity < 0 {
			return 0, false
		}
		var ok bool
		total, ok = checkedCDFAdd(total, quantity)
		if !ok {
			return 0, false
		}
	}
	return total, true
}

func cdfDepthRestoresOrder(depth *cdfPublicDepthState, side string, price, minimumQty int64) bool {
	levels := depth.bids
	if side == "SELL" {
		levels = depth.asks
	}
	return levels[price] >= minimumQty
}

func cdfAccountNetBalance(balances []Balance, asset string) (int64, bool) {
	for _, balance := range balances {
		if balance.Asset == asset {
			return balance.NetAsset, true
		}
	}
	return 0, false
}

func cdfMarkedAccountEquity(row AccountRow, supplier CDFSupplierContract) (int64, bool) {
	var total int64
	for _, balance := range row.Account.SpotBalances {
		if balance.NetAsset == 0 {
			continue
		}
		var precision int64
		switch balance.Asset {
		case supplier.BaseAsset:
			precision = supplier.BasePrecision
		case supplier.QuoteAsset:
			precision = supplier.QuotePrecision
		default:
			return 0, false
		}
		mark := row.Marks[balance.Asset]
		if mark <= 0 || precision <= 0 {
			return 0, false
		}
		value := new(big.Int).Mul(big.NewInt(balance.NetAsset), big.NewInt(mark))
		value.Quo(value, big.NewInt(precision))
		if !value.IsInt64() {
			return 0, false
		}
		var ok bool
		total, ok = checkedCDFAdd(total, value.Int64())
		if !ok {
			return 0, false
		}
	}
	return total, true
}

func cdfMarkedPositionEquity(position, riskMark, quoteAvailable, quoteReserved int64, supplier CDFSupplierContract) (int64, bool) {
	if riskMark <= 0 || quoteAvailable < 0 || quoteReserved < 0 || supplier.BasePrecision <= 0 {
		return 0, false
	}
	grossInventory, ok := checkedCDFAdd(supplier.InitialBaseBalance, position)
	if !ok || grossInventory < 0 {
		return 0, false
	}
	notional := new(big.Int).Mul(big.NewInt(grossInventory), big.NewInt(riskMark))
	notional.Quo(notional, big.NewInt(supplier.BasePrecision))
	cash, ok := checkedCDFAdd(quoteAvailable, quoteReserved)
	if !ok {
		return 0, false
	}
	equity := new(big.Int).Add(notional, big.NewInt(cash))
	if !equity.IsInt64() {
		return 0, false
	}
	return equity.Int64(), true
}

func cdfActivationNotional(price, quantity, basePrecision int64) (int64, bool) {
	if price <= 0 || quantity <= 0 || basePrecision <= 0 {
		return 0, false
	}
	notional := new(big.Int).Mul(big.NewInt(price), big.NewInt(quantity))
	notional.Quo(notional, big.NewInt(basePrecision))
	if !notional.IsInt64() {
		return 0, false
	}
	return notional.Int64(), true
}

func cdfActivationFee(notional, basisPoints int64) (int64, bool) {
	if notional < 0 || basisPoints < 0 || basisPoints > 10_000 {
		return 0, false
	}
	fee := new(big.Int).Mul(big.NewInt(notional), big.NewInt(basisPoints))
	fee.Quo(fee, big.NewInt(10_000))
	if !fee.IsInt64() {
		return 0, false
	}
	return fee.Int64(), true
}

func cdfBestBid(levels []etypes.PriceLevel) (int64, int64) {
	var price, quantity int64
	for _, level := range levels {
		if level.Price <= 0 || level.VisibleQty <= 0 || level.Price < price {
			continue
		}
		if level.Price > price {
			price, quantity = level.Price, level.VisibleQty
			continue
		}
		quantity, _ = checkedCDFAdd(quantity, level.VisibleQty)
	}
	return price, quantity
}

func cdfBestAsk(levels []etypes.PriceLevel) (int64, int64) {
	var price, quantity int64
	for _, level := range levels {
		if level.Price <= 0 || level.VisibleQty <= 0 || price != 0 && level.Price > price {
			continue
		}
		if price == 0 || level.Price < price {
			price, quantity = level.Price, level.VisibleQty
			continue
		}
		quantity, _ = checkedCDFAdd(quantity, level.VisibleQty)
	}
	return price, quantity
}

func canonicalCDFActivationJSON(raw []byte) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("cdf activation: canonicalize JSON: %w", err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("cdf activation: encode canonical JSON: %w", err)
	}
	return string(canonical), nil
}

func isCDFHex(value string, bytes int) bool {
	if len(value) != bytes*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sameCDFStrings(left, right []string) bool {
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

func sameCDFPath(left, right string) bool {
	leftAbsolute, leftErr := filepath.Abs(left)
	rightAbsolute, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return filepath.Clean(leftAbsolute) == filepath.Clean(rightAbsolute)
}

func containsCDFString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isNumberedRole(role, prefix string) bool {
	if !strings.HasPrefix(role, prefix) || len(role) == len(prefix) {
		return false
	}
	for _, character := range role[len(prefix):] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return role[len(prefix):] != "0"
}

func isCDFAction(action string) bool {
	switch action {
	case "wait", "submit", "rest", "cancel", "withdraw":
		return true
	default:
		return false
	}
}

func cdfDecisionReasonAllowed(action, reason string) bool {
	switch action {
	case "wait":
		switch reason {
		case "subscribe", "order_pending", "cancel_pending", "awaiting_fresh_observation_after_close",
			"loss_limit", "equity_unavailable", "stale_or_missing_observation", "one_sided_or_locked_book",
			"limit_or_touch_unavailable", "below_minimum_executable_qty", "inventory_at_target", "quote_cash_limit":
			return true
		}
	case "submit":
		return reason == "inventory_target_gap"
	case "rest":
		return reason == "quote_unchanged"
	case "cancel":
		return reason == "reprice_for_inventory_or_touch"
	case "withdraw":
		switch reason {
		case "loss_limit", "equity_unavailable", "stale_or_missing_observation", "one_sided_or_locked_book",
			"limit_or_touch_unavailable", "below_minimum_executable_qty", "position_gap_overflow",
			"inventory_at_target", "quote_cash_limit":
			return true
		}
	}
	return false
}

// cdfDecisionReasonPredicate checks the observable state that makes a
// lifecycle reason economically possible. The reason string is actor output,
// but the associated IDs, inventory, quote, mark, and loss fields are already
// independently reconstructed by the strict audit. Reasons without an
// independently testable predicate remain rejected instead of becoming a
// self-attested escape hatch.
func cdfDecisionReasonPredicate(decision cdfDecisionEvidence, state *cdfSupplierState) bool {
	if state == nil {
		return false
	}
	hasQuote := decision.QuoteOrderID != 0
	switch decision.Reason {
	case "subscribe":
		return decision.Action == "wait" && !hasQuote && decision.ObservationSequence == 0
	case "order_pending":
		return decision.Action == "wait" && !hasQuote && decision.QuoteRequestID != 0 && decision.QuoteSubmittedAt > 0
	case "cancel_pending":
		return decision.Action == "wait" && hasQuote && decision.CancelRequestID != 0
	case "awaiting_fresh_observation_after_close":
		return decision.Action == "wait" && !hasQuote && decision.LocalBookMode == "one_sided" &&
			decision.TargetPosition != decision.Position
	case "loss_limit":
		return (decision.Action == "wait" || decision.Action == "withdraw") && decision.RiskLimitTriggered &&
			decision.MaxLossQuote > 0 && (decision.LossFromInitialQuote >= decision.MaxLossQuote || decision.DrawdownQuote >= decision.MaxLossQuote)
	case "equity_unavailable":
		return (decision.Action == "wait" || decision.Action == "withdraw") && !decision.EquityAvailable &&
			(!decision.RiskMarkCurrent || decision.RiskMarkPrice == 0 || strings.HasSuffix(decision.RiskMarkSource, "_unavailable"))
	case "stale_or_missing_observation":
		return (decision.Action == "wait" || decision.Action == "withdraw") &&
			(decision.ObservationTime == 0 || decision.ObservationAge > state.contract.MaxObservationAge)
	case "one_sided_or_locked_book":
		return (decision.Action == "wait" || decision.Action == "withdraw") && decision.LocalBookMode == ""
	case "limit_or_touch_unavailable":
		if decision.Action != "wait" && decision.Action != "withdraw" {
			return false
		}
		if decision.QuotePrice <= 0 || decision.QuoteQty <= 0 {
			return true
		}
		// Early fail-closed producer paths start from baseDecision, which may
		// retain the old live quote's positive terms. In that case the absence
		// of a selected side/source plus an invalid local touch is the
		// independently observable failure predicate.
		if decision.Side != "" || decision.QuotePriceSource != "" {
			return false
		}
		if decision.LocalBookMode == "" || decision.LocalBookMode == "one_sided" {
			return true
		}
		return decision.LocalBookMode == "two_sided" &&
			(decision.BestBid <= 0 || decision.BestAsk <= 0 ||
				decision.BestBid%state.contract.TickSize != 0 || decision.BestAsk%state.contract.TickSize != 0)
	case "below_minimum_executable_qty":
		return (decision.Action == "wait" || decision.Action == "withdraw") && decision.QuoteQty > 0 &&
			decision.QuoteQty < state.contract.MinimumExecutableQty
	case "inventory_at_target":
		return (decision.Action == "wait" || decision.Action == "withdraw") && decision.TargetPosition == decision.Position
	case "quote_cash_limit":
		return (decision.Action == "wait" || decision.Action == "withdraw") && decision.QuoteCashAvailable >= 0 &&
			decision.QuoteCashRequired > decision.QuoteCashAvailable
	case "inventory_target_gap":
		return decision.Action == "submit" && decision.TargetPosition != decision.Position
	case "quote_unchanged":
		return decision.Action == "rest" && hasQuote && decision.QuotePrice > 0 && decision.QuoteQty > 0
	case "reprice_for_inventory_or_touch":
		if decision.Action != "cancel" || !hasQuote || decision.CancelRequestID == 0 || !state.hasLastDecision {
			return false
		}
		return decision.Side != state.lastDecision.Side || decision.QuotePrice != state.lastDecision.QuotePrice ||
			decision.QuoteQty != state.lastDecision.QuoteQty
	default:
		return false
	}
}

func cdfSideCode(side string) uint8 {
	if side == "BUY" {
		return uint8(etypes.Buy)
	}
	if side == "SELL" {
		return uint8(etypes.Sell)
	}
	return math.MaxUint8
}

func checkedCDFAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right || right < 0 && left < math.MinInt64-right {
		return 0, false
	}
	return left + right, true
}

func checkedCDFSub(left, right int64) (int64, bool) {
	if right == math.MinInt64 {
		if left >= 0 {
			return 0, false
		}
		return left - right, true
	}
	return checkedCDFAdd(left, -right)
}

func cdfAbsExceeds(value, limit int64) bool {
	if limit < 0 || value == math.MinInt64 {
		return true
	}
	if value < 0 {
		value = -value
	}
	return value > limit
}

func cdfAbs(value int64) int64 {
	if value == math.MinInt64 {
		return math.MaxInt64
	}
	if value < 0 {
		return -value
	}
	return value
}

func cdfBetweenInclusive(value, first, second int64) bool {
	if first > second {
		first, second = second, first
	}
	return value >= first && value <= second
}

func (r *CDFActivationAudit) addCheck(check CDFActivationCheck) {
	r.Checks = append(r.Checks, check)
}

func (r *CDFActivationAudit) addEventCheck(event Event, state *cdfSupplierState, failure string) {
	r.addCheck(CDFActivationCheck{
		VenueID: event.VenueID, Role: state.audit.Role, ClientID: event.ClientID,
		Ordinal: event.Ordinal, Failure: failure,
	})
}

func (r *CDFActivationAudit) hasParticipantCheck(venueID string, clientID uint64) bool {
	for _, check := range r.Checks {
		if check.VenueID == venueID && check.ClientID == clientID {
			return true
		}
	}
	return false
}
