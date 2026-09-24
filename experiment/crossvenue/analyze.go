package crossvenue

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"exchange_sim/analysis"
	"exchange_sim/simulations/multivenue"
	etypes "exchange_sim/types"
)

type Contract struct {
	SchemaVersion           int       `json:"schema_version"`
	Arm                     string    `json:"arm"`
	Seed                    int64     `json:"seed"`
	SourceRevision          string    `json:"source_revision"`
	EffectiveConfigSHA256   string    `json:"effective_config_sha256"`
	Venues                  [2]string `json:"venues"`
	Symbol                  string    `json:"symbol"`
	BaseAsset               string    `json:"base_asset"`
	QuoteAsset              string    `json:"quote_asset"`
	HorizonNano             int64     `json:"horizon_nano"`
	LotQty                  int64     `json:"lot_qty"`
	BasePrecision           int64     `json:"base_precision"`
	TakerFeeBps             int64     `json:"taker_fee_bps"`
	MaxBookEvidenceAgeNanos int64     `json:"max_book_evidence_age_nanos"`
	InitialBasePerVenue     int64     `json:"initial_base_per_venue"`
	InitialQuotePerVenue    int64     `json:"initial_quote_per_venue"`
}

type Economics struct {
	ActualQuoteCashflow              int64            `json:"actual_quote_cashflow"`
	ActualLegFees                    int64            `json:"actual_leg_fees"`
	MatchedEdgeBeforeFees            *int64           `json:"matched_edge_before_fees"`
	MatchedEdgeAfterFees             *int64           `json:"matched_edge_after_fees"`
	GlobalResidualBaseQty            int64            `json:"global_residual_base_qty"`
	LocalBaseDeltaByVenue            map[string]int64 `json:"local_base_delta_by_venue"`
	HypotheticalLocalRestorationFlow *int64           `json:"hypothetical_local_restoration_cashflow"`
	FinalNetValue                    *int64           `json:"final_net_value"`
}

type RouterResult struct {
	RouterID  uint64                                   `json:"router_id"`
	Evidence  *analysis.CrossVenueFirstAttemptEvidence `json:"evidence"`
	Economics Economics                                `json:"economics"`
}

type Result struct {
	SchemaVersion        int                                 `json:"schema_version"`
	Arm                  string                              `json:"arm"`
	Seed                 int64                               `json:"seed"`
	Binding              analysis.CrossVenueRunBinding       `json:"binding"`
	Edge                 analysis.CrossVenueEdgeWorldSummary `json:"edge"`
	Conservation         *analysis.Conservation              `json:"conservation"`
	Router               *RouterResult                       `json:"router,omitempty"`
	TradeAttribution     string                              `json:"trade_attribution"`
	RandomStreamCoupling string                              `json:"random_stream_coupling"`
}

func DecodeContract(raw []byte) (Contract, error) {
	var contract Contract
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, fmt.Errorf("ME-005 contract: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Contract{}, fmt.Errorf("ME-005 contract: trailing content or malformed JSON")
	}
	if err := contract.Validate(); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func (contract Contract) Validate() error {
	if contract.SchemaVersion != 1 || contract.Arm != "ON" && contract.Arm != "OFF" ||
		contract.Seed == 0 || contract.SourceRevision == "" ||
		contract.EffectiveConfigSHA256 == "" || contract.Venues[0] == "" || contract.Venues[1] == "" ||
		contract.Venues[0] == contract.Venues[1] || contract.Symbol == "" || contract.BaseAsset == "" ||
		contract.QuoteAsset == "" || contract.BaseAsset == contract.QuoteAsset || contract.HorizonNano <= 0 ||
		contract.LotQty <= 0 || contract.BasePrecision <= 0 || contract.TakerFeeBps < 0 ||
		contract.MaxBookEvidenceAgeNanos <= 0 || contract.Arm == "ON" &&
		(contract.InitialBasePerVenue <= 0 || contract.InitialQuotePerVenue <= 0) || contract.Arm == "OFF" &&
		(contract.InitialBasePerVenue != 0 || contract.InitialQuotePerVenue != 0) {
		return fmt.Errorf("ME-005 contract: incomplete or unsupported registered fields")
	}
	return nil
}

// AnalyzeCompletedWorld reads a completed binary run; it never starts a
// simulator. The renderer verifies the complete stream before reconstruction.
// The caller owns a fresh renderedDir and must retain the raw evidence.
func AnalyzeCompletedWorld(rawDir, renderedDir string, contract Contract) (Result, error) {
	if rawDir == "" || renderedDir == "" {
		return Result{}, fmt.Errorf("ME-005 analysis: raw and rendered directories are required")
	}
	if err := contract.Validate(); err != nil {
		return Result{}, err
	}
	rendered, err := multivenue.RenderBinaryEvidence(rawDir, renderedDir)
	if err != nil {
		return Result{}, fmt.Errorf("ME-005 analysis: completed binary render: %w", err)
	}
	if err := copyTerminalReport(rawDir, renderedDir); err != nil {
		return Result{}, err
	}
	maxAttempts := 0
	if contract.Arm == "ON" {
		maxAttempts = 1
	}
	binding, err := analysis.VerifyCrossVenueRunBinding(rawDir, renderedDir, analysis.CrossVenueRunBindingExpectation{
		SourceRevision: contract.SourceRevision, EffectiveConfigSHA256: contract.EffectiveConfigSHA256,
		ExecutionStreamHash: rendered.ExecutionHash, RouterEnabled: contract.Arm == "ON",
		Seed: contract.Seed, Venues: contract.Venues, LotQty: contract.LotQty, MaxAttempts: maxAttempts,
		TakerFeeBps: contract.TakerFeeBps,
	})
	if err != nil {
		return Result{}, err
	}
	backgroundIdentity, err := backgroundIdentityFromManifest(filepath.Join(rawDir, "manifest.json"), binding.ManifestSHA256)
	if err != nil {
		return Result{}, err
	}
	run, err := analysis.Open(renderedDir)
	if err != nil {
		return Result{}, err
	}
	if err := checkTerminalHorizon(run.Report.TerminalAccounts, contract.HorizonNano); err != nil {
		return Result{}, err
	}
	conservation, err := run.MeasureConservation(analysis.ConservationOptions{})
	if err != nil {
		return Result{}, err
	}
	if err := validateConservation(conservation); err != nil {
		return Result{}, err
	}
	publicEvents, err := run.CollectCrossVenuePublicEvents(contract.Venues, contract.Symbol)
	if err != nil {
		return Result{}, err
	}
	replay, err := analysis.ReplayCrossVenuePublicEvents(publicEvents, analysis.CrossVenuePublicReplayOptions{
		Venues: contract.Venues, Symbol: contract.Symbol, InitiallyEmpty: true,
	})
	if err != nil {
		return Result{}, err
	}
	edge, err := analysis.SummarizeCrossVenueEdgeWorld(analysis.CrossVenueEdgeWorldInput{
		Seed: contract.Seed, Arm: contract.Arm, BackgroundIdentity: backgroundIdentity,
		Venues: contract.Venues, Transitions: replay.Transitions, HorizonNano: contract.HorizonNano,
		LotQty: contract.LotQty, BasePrecision: contract.BasePrecision, TakerFeeBps: contract.TakerFeeBps,
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{SchemaVersion: 1, Arm: contract.Arm, Seed: contract.Seed, Binding: binding, Edge: edge, Conservation: conservation,
		TradeAttribution: "NOT_IDENTIFIED", RandomStreamCoupling: "NOT_PROVEN_BY_EQUAL_SEED"}
	if contract.Arm == "OFF" {
		if len(run.Report.RouterReports) != 0 || hasRouterAccount(run.Report.InitialAccounts) ||
			hasRouterAccount(run.Report.TerminalAccounts) {
			return Result{}, fmt.Errorf("ME-005 analysis: router-off arm contains router state")
		}
		return result, nil
	}
	routerID, clients, err := selectedRouter(run.Report, contract.Venues)
	if err != nil {
		return Result{}, err
	}
	evidence, err := run.ReconstructCrossVenueFirstAttempt(analysis.CrossVenueFirstAttemptSpec{
		EvidenceDir: rawDir, Venues: contract.Venues, Clients: clients, RouterID: routerID,
		Symbol: contract.Symbol, BaseAsset: contract.BaseAsset, QuoteAsset: contract.QuoteAsset,
		HorizonNano: contract.HorizonNano, LotQty: contract.LotQty, BasePrecision: contract.BasePrecision,
		TakerFeeBps: contract.TakerFeeBps, MaxAttempts: 1,
		MaxBookEvidenceAgeNanos: contract.MaxBookEvidenceAgeNanos,
	})
	if err != nil {
		return Result{}, err
	}
	for _, venue := range contract.Venues {
		movement := evidence.Movements[venue]
		if movement.InitialBase != contract.InitialBasePerVenue || movement.InitialQuote != contract.InitialQuotePerVenue {
			return Result{}, fmt.Errorf("ME-005 analysis: %s router endowment differs from registered capital", venue)
		}
	}
	economics, err := SummarizeEconomics(evidence, contract.Venues)
	if err != nil {
		return Result{}, err
	}
	result.Router = &RouterResult{RouterID: routerID, Evidence: evidence, Economics: economics}
	return result, nil
}

func copyTerminalReport(rawDir, renderedDir string) error {
	raw, err := os.ReadFile(filepath.Join(rawDir, "greeks.json"))
	if err != nil {
		return fmt.Errorf("ME-005 analysis: read terminal report: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(renderedDir, "greeks.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("ME-005 analysis: new rendered report: %w", err)
	}
	written, err := file.Write(raw)
	if err != nil || written != len(raw) {
		_ = file.Close()
		return fmt.Errorf("ME-005 analysis: incomplete report copy: %d/%d bytes: %v", written, len(raw), err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func checkTerminalHorizon(accounts []analysis.AccountRow, horizonNano int64) error {
	if len(accounts) == 0 {
		return fmt.Errorf("ME-005 analysis: terminal accounts absent")
	}
	for _, account := range accounts {
		if account.Phase != "terminal_post_mark" || account.Account.Timestamp != horizonNano {
			return fmt.Errorf("ME-005 analysis: incomplete terminal horizon")
		}
	}
	return nil
}

func hasRouterAccount(accounts []analysis.AccountRow) bool {
	for _, account := range accounts {
		if strings.HasPrefix(account.Role, "cross_venue_router_tier_") {
			return true
		}
	}
	return false
}

func selectedRouter(report analysis.Report, venues [2]string) (uint64, map[string]uint64, error) {
	if len(report.RouterReports) != 1 || report.RouterReports[0].RouterID == 0 {
		return 0, nil, fmt.Errorf("ME-005 analysis: expected one instrumented router")
	}
	clients := make(map[string]uint64, 2)
	for _, account := range report.InitialAccounts {
		if !strings.HasPrefix(account.Role, "cross_venue_router_tier_") {
			continue
		}
		if account.VenueID != venues[0] && account.VenueID != venues[1] || clients[account.VenueID] != 0 || account.ClientID == 0 {
			return 0, nil, fmt.Errorf("ME-005 analysis: duplicate or foreign router account")
		}
		clients[account.VenueID] = account.ClientID
	}
	if len(clients) != 2 {
		return 0, nil, fmt.Errorf("ME-005 analysis: router lacks two venue-local accounts")
	}
	terminalCount := 0
	for _, account := range report.TerminalAccounts {
		if !strings.HasPrefix(account.Role, "cross_venue_router_tier_") {
			continue
		}
		if clients[account.VenueID] != account.ClientID || account.ClientID == 0 {
			return 0, nil, fmt.Errorf("ME-005 analysis: terminal router account differs from initial roster")
		}
		terminalCount++
	}
	if terminalCount != 2 {
		return 0, nil, fmt.Errorf("ME-005 analysis: incomplete terminal router roster")
	}
	return report.RouterReports[0].RouterID, clients, nil
}

func SummarizeEconomics(evidence *analysis.CrossVenueFirstAttemptEvidence, venues [2]string) (Economics, error) {
	if evidence == nil || len(evidence.Accounts) != 2 {
		return Economics{}, fmt.Errorf("ME-005 economics: missing audited venue accounts")
	}
	result := Economics{LocalBaseDeltaByVenue: make(map[string]int64, len(venues))}
	for _, venue := range venues {
		account, exists := evidence.Accounts[venue]
		if !exists || account.VenueID != venue {
			return Economics{}, fmt.Errorf("ME-005 economics: missing venue-local delta")
		}
		var ok bool
		result.ActualQuoteCashflow, ok = etypes.TryAdd(result.ActualQuoteCashflow, account.QuoteDelta)
		if !ok {
			return Economics{}, fmt.Errorf("ME-005 economics: quote cashflow overflow")
		}
		result.ActualLegFees, ok = etypes.TryAdd(result.ActualLegFees, account.QuoteFees)
		if !ok {
			return Economics{}, fmt.Errorf("ME-005 economics: fee sum overflow")
		}
		result.LocalBaseDeltaByVenue[venue] = account.BaseDelta
		result.GlobalResidualBaseQty, ok = etypes.TryAdd(result.GlobalResidualBaseQty, account.BaseDelta)
		if !ok {
			return Economics{}, fmt.Errorf("ME-005 economics: residual base overflow")
		}
	}
	if evidence.Terminal != nil && evidence.Terminal.ExchangeOutcome == "MATCHED_FOK" {
		beforeFees, ok := etypes.TryAdd(result.ActualQuoteCashflow, result.ActualLegFees)
		if !ok {
			return Economics{}, fmt.Errorf("ME-005 economics: before-fee edge overflow")
		}
		afterFees := result.ActualQuoteCashflow
		result.MatchedEdgeBeforeFees, result.MatchedEdgeAfterFees = &beforeFees, &afterFees
	}
	if evidence.TerminalValue.Available {
		adjustment, ok := etypes.TrySub(evidence.TerminalValue.Value, result.ActualQuoteCashflow)
		if !ok {
			return Economics{}, fmt.Errorf("ME-005 economics: local restoration adjustment overflow")
		}
		value := evidence.TerminalValue.Value
		result.HypotheticalLocalRestorationFlow, result.FinalNetValue = &adjustment, &value
	}
	return result, nil
}
