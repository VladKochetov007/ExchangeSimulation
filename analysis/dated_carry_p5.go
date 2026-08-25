package analysis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"

	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

const (
	p5DatedCarryPolicyVersion = "v2_5_p5_dated_term_carry_v1"
	p5DatedCarryYearNanos     = int64(365 * 24 * 60 * 60 * 1_000_000_000)
)

// DatedCarryP5Audit independently reconstructs P5's delivered-information and
// exact-cost links. Actor arithmetic is never accepted as a source value.
// Canonical execution, lifecycle, and the paired basis endpoint are layered on
// this audit rather than allowed to rescue a failed information/economic link.
type DatedCarryP5Audit struct {
	TradeEnabled bool `json:"trade_enabled"`

	Decisions          int64            `json:"decisions"`
	CandidateDecisions int64            `json:"candidate_decisions"`
	EligibleDecisions  int64            `json:"eligible_decisions"`
	ShadowEligible     int64            `json:"shadow_eligible"`
	TargetChanges      int64            `json:"target_changes"`
	Submitted          int64            `json:"submitted"`
	ActionCounts       map[string]int64 `json:"action_counts"`

	ReceiptAuditValid     bool                `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors int64               `json:"receipt_evidence_errors"`
	RoleLinksActive       bool                `json:"role_links_active"`
	ListingMatches        int64               `json:"listing_matches"`
	BookMatches           int64               `json:"book_matches"`
	SourceMismatches      int64               `json:"source_mismatches"`
	FutureSourceUse       int64               `json:"future_source_use"`
	FrontierMismatches    int64               `json:"frontier_mismatches"`
	TermMismatches        int64               `json:"term_mismatches"`
	ArithmeticMismatches  int64               `json:"arithmetic_mismatches"`
	PolicyMismatches      int64               `json:"policy_mismatches"`
	Checks                []DatedCarryP5Check `json:"checks,omitempty"`
	Valid                 bool                `json:"valid"`
}

type DatedCarryP5Check struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	At       int64  `json:"decision_time"`
	Failure  string `json:"failure"`
}

type datedCarryP5Config struct {
	Enabled                     bool   `json:"enabled"`
	TradeEnabled                bool   `json:"trade_enabled"`
	SpotSymbol                  string `json:"spot_symbol"`
	TargetTenor                 int64  `json:"target_tenor_nanos"`
	DecisionPeriod              int64  `json:"decision_period_nanos"`
	DecisionPhase               int64  `json:"decision_phase_offset_nanos"`
	MaxMarketAge                int64  `json:"max_market_age_nanos"`
	MinTimeToExpiry             int64  `json:"min_time_to_expiry_nanos"`
	TakerFeeBps                 int64  `json:"taker_fee_bps"`
	LongSpotFundingBps          int64  `json:"long_spot_funding_bps"`
	ShortSpotBorrowBps          int64  `json:"short_spot_borrow_bps"`
	BalanceSheetBps             int64  `json:"balance_sheet_bps"`
	MarginRiskBps               int64  `json:"margin_risk_bps"`
	LegRiskBps                  int64  `json:"leg_risk_bps"`
	SettlementMismatchBps       int64  `json:"settlement_mismatch_bps"`
	PostSettlementExitBps       int64  `json:"post_settlement_exit_bps"`
	MinNetCarryBps              int64  `json:"min_net_carry_bps"`
	MaxPosition                 int64  `json:"max_position"`
	LotQty                      int64  `json:"lot_qty"`
	MinOrderSize                int64  `json:"min_order_size"`
	SpotTick                    int64  `json:"spot_tick"`
	FutureTick                  int64  `json:"future_tick"`
	PassiveExitSliceQty         int64  `json:"passive_exit_slice_qty"`
	ExitDeadlineAfterSettlement int64  `json:"exit_deadline_after_settlement_nanos"`
}

type datedMandateP5Config struct {
	Enabled           bool   `json:"enabled"`
	Underlying        string `json:"underlying"`
	TargetTenor       int64  `json:"target_tenor_nanos"`
	Side              string `json:"side"`
	ParentQty         int64  `json:"parent_qty"`
	ChildQty          int64  `json:"child_qty"`
	StartDelay        int64  `json:"start_delay_nanos"`
	ExecutionDuration int64  `json:"execution_duration_nanos"`
	DecisionPeriod    int64  `json:"decision_period_nanos"`
	DecisionPhase     int64  `json:"decision_phase_offset_nanos"`
	MaxMarketAge      int64  `json:"max_market_age_nanos"`
	SlippageBps       int64  `json:"slippage_bps"`
	TickSize          int64  `json:"tick_size"`
}

type datedCarryP5Manifest struct {
	Config struct {
		TakerFeeBps                 int64                 `json:"taker_fee_bps"`
		ReceiptRoles                []string              `json:"market_data_receipt_roles"`
		DatedExecutionMandate       *datedMandateP5Config `json:"dated_future_execution_mandate"`
		DatedTermCarry              *datedCarryP5Config   `json:"dated_term_carry_allocator"`
		RecordDatedMandateDecisions bool                  `json:"record_dated_execution_mandate_decisions"`
		RecordDatedCarryDecisions   bool                  `json:"record_dated_term_carry_decisions"`
		OptionFlowIncludeFutures    *bool                 `json:"option_flow_include_futures"`
		StrictPopulationAccounting  bool                  `json:"strict_population_accounting"`
		ShortFutureTenor            int64                 `json:"short_future_tenor"`
		SnapshotInterval            int64                 `json:"snapshot_interval"`
	} `json:"config"`
}

type datedCarryP5Decision struct {
	VenueID       string `json:"venue_id"`
	Desk          string `json:"desk"`
	ClientID      uint64 `json:"client_id"`
	PolicyVersion string `json:"policy_version"`
	DecisionTime  int64  `json:"decision_time"`
	Action        string `json:"action_or_defer_reason"`
	Enabled       bool   `json:"enabled"`
	TradeEnabled  bool   `json:"trade_enabled"`
	Subscribed    bool   `json:"subscribed"`
	State         string `json:"state"`

	SpotSymbol           string `json:"spot_symbol"`
	FutureSymbol         string `json:"future_symbol"`
	ListedNano           int64  `json:"listed_nano"`
	ExpiryNano           int64  `json:"expiry_nano"`
	OriginalTenorNanos   int64  `json:"original_tenor_nanos"`
	ListingPublishedAt   int64  `json:"listing_published_at"`
	ListingSequence      uint64 `json:"listing_sequence"`
	ListingFingerprint   string `json:"listing_fingerprint"`
	TimeToExpiryNanos    int64  `json:"time_to_expiry_nanos"`
	SettlementObservedAt int64  `json:"settlement_observed_at"`
	ExitDeadlineAt       int64  `json:"exit_deadline_at"`

	SpotPosition         int64 `json:"spot_position"`
	FuturePosition       int64 `json:"future_position"`
	TargetSpot           int64 `json:"target_spot_position"`
	TargetFuture         int64 `json:"target_future_position"`
	ProposedTargetSpot   int64 `json:"proposed_target_spot_position"`
	ProposedTargetFuture int64 `json:"proposed_target_future_position"`
	TargetChangedAt      int64 `json:"target_changed_at"`

	HasSpotBook       bool   `json:"has_spot_book"`
	SpotPublishedAt   int64  `json:"spot_published_at"`
	SpotSequence      uint64 `json:"spot_sequence"`
	HasSpotBid        bool   `json:"has_spot_bid"`
	SpotBid           int64  `json:"spot_bid"`
	SpotBidQty        int64  `json:"spot_bid_qty"`
	HasSpotAsk        bool   `json:"has_spot_ask"`
	SpotAsk           int64  `json:"spot_ask"`
	SpotAskQty        int64  `json:"spot_ask_qty"`
	HasFutureBook     bool   `json:"has_future_book"`
	FuturePublishedAt int64  `json:"future_published_at"`
	FutureSequence    uint64 `json:"future_sequence"`
	HasFutureBid      bool   `json:"has_future_bid"`
	FutureBid         int64  `json:"future_bid"`
	FutureBidQty      int64  `json:"future_bid_qty"`
	HasFutureAsk      bool   `json:"has_future_ask"`
	FutureAsk         int64  `json:"future_ask"`
	FutureAskQty      int64  `json:"future_ask_qty"`
	SpotAgeNanos      int64  `json:"spot_age_nanos"`
	FutureAgeNanos    int64  `json:"future_age_nanos"`

	Direction                   string `json:"direction"`
	SpotExecutionReference      int64  `json:"spot_execution_reference"`
	FutureExecutionReference    int64  `json:"future_execution_reference"`
	GrossLockedSpreadRaw        string `json:"gross_locked_spread_raw"`
	GrossLockedBpsNumerator     string `json:"gross_locked_bps_numerator"`
	ExecutionFeeBpsNumerator    string `json:"execution_fee_bps_numerator"`
	FinancingBpsNumerator       string `json:"financing_bps_numerator"`
	BalanceSheetBpsNumerator    string `json:"balance_sheet_bps_numerator"`
	MarginRiskBpsNumerator      string `json:"margin_risk_bps_numerator"`
	LegRiskBpsNumerator         string `json:"leg_risk_bps_numerator"`
	SettlementMismatchNumerator string `json:"settlement_mismatch_bps_numerator"`
	PostSettlementExitNumerator string `json:"post_settlement_exit_bps_numerator"`
	NetCarryBpsNumerator        string `json:"net_carry_bps_numerator"`
	MinimumNetBpsNumerator      string `json:"minimum_net_bps_numerator"`
	RationalDenominator         string `json:"rational_denominator"`
	FinancingDirection          string `json:"financing_direction"`

	Leg                         string `json:"leg"`
	Side                        string `json:"side"`
	OrderType                   string `json:"order_type"`
	TimeInForce                 string `json:"time_in_force"`
	PostOnly                    *bool  `json:"post_only"`
	LimitPrice                  int64  `json:"limit_price"`
	RequestedQty                int64  `json:"requested_qty"`
	RequestID                   uint64 `json:"request_id"`
	CancelOrderID               uint64 `json:"cancel_order_id"`
	CancelRequestID             uint64 `json:"cancel_request_id"`
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`
}

type p5EvidenceKey struct {
	client    uint64
	link      uint32
	mdType    uint8
	symbol    string
	sequence  uint64
	published int64
}

type p5Evidence struct {
	sources      map[p5EvidenceKey][]observationRecord
	frontiers    map[fundingCarryReceiptKey]auditedFrontier
	linkRoles    map[uint32]string
	linkVenues   map[uint32]string
	roleReceipts map[string]int64
	audit        *MarketDataReceiptAudit
}

func p5RequiredRoleLinksActive(evidence *p5Evidence) bool {
	if evidence == nil || evidence.audit == nil {
		return false
	}
	seen := map[string]int{"dated_execution_mandate": 0, "dated_term_carry_allocator": 0}
	for _, activity := range evidence.audit.LinkActivity {
		if _, required := seen[activity.Role]; !required {
			continue
		}
		seen[activity.Role]++
		if activity.Receipts == 0 {
			return false
		}
	}
	return seen["dated_execution_mandate"] > 0 && seen["dated_term_carry_allocator"] > 0
}

type p5ListingKey struct {
	venue, symbol string
}

type p5Listing struct {
	at           int64
	announcement etypes.InstrumentAnnouncement
}

type p5Financials struct {
	direction, financingDirection                        string
	spotReference, futureReference                       int64
	grossSpread, gross, fees, financing                  *big.Int
	balance, margin, leg, settlement, exit, net, minimum *big.Int
	denominator                                          *big.Int
	eligible                                             bool
}

func loadDatedCarryP5Manifest(dir string) (datedCarryP5Manifest, error) {
	var manifest datedCarryP5Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return manifest, fmt.Errorf("read P5 manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, fmt.Errorf("decode P5 manifest: %w", err)
	}
	if manifest.Config.DatedTermCarry == nil || manifest.Config.DatedExecutionMandate == nil {
		return manifest, fmt.Errorf("P5 policies missing from manifest")
	}
	return manifest, nil
}

func validateDatedCarryP5Manifest(manifest datedCarryP5Manifest) error {
	p := manifest.Config.DatedTermCarry
	m := manifest.Config.DatedExecutionMandate
	if !p.Enabled || p.SpotSymbol != "ABC/USD" || p.TargetTenor <= 0 || p.DecisionPeriod <= 0 || p.MaxMarketAge <= 0 || p.MinTimeToExpiry <= 0 || p.MaxPosition <= 0 || p.LotQty <= 0 || p.MinOrderSize <= 0 || p.SpotTick <= 0 || p.FutureTick <= 0 || p.PassiveExitSliceQty <= 0 || p.ExitDeadlineAfterSettlement <= 0 {
		return fmt.Errorf("invalid P5 dated-carry policy")
	}
	if p.TakerFeeBps != manifest.Config.TakerFeeBps || p.TargetTenor != manifest.Config.ShortFutureTenor || p.DecisionPhase < 0 || p.DecisionPhase >= p.DecisionPeriod {
		return fmt.Errorf("P5 dated-carry policy disagrees with environment")
	}
	for _, value := range []int64{p.TakerFeeBps, p.LongSpotFundingBps, p.ShortSpotBorrowBps, p.BalanceSheetBps, p.MarginRiskBps, p.LegRiskBps, p.SettlementMismatchBps, p.PostSettlementExitBps, p.MinNetCarryBps} {
		if value < 0 {
			return fmt.Errorf("negative P5 cost component")
		}
	}
	if !m.Enabled || m.Underlying != p.SpotSymbol || m.TargetTenor != p.TargetTenor || m.ParentQty <= 0 || m.ChildQty <= 0 || m.ExecutionDuration <= 0 || m.DecisionPeriod <= 0 || m.MaxMarketAge <= 0 || m.TickSize <= 0 || m.Side != exchange.Buy.String() {
		return fmt.Errorf("invalid P5 dated-execution mandate")
	}
	if !manifest.Config.RecordDatedCarryDecisions || !manifest.Config.RecordDatedMandateDecisions || !manifest.Config.StrictPopulationAccounting || manifest.Config.OptionFlowIncludeFutures == nil || *manifest.Config.OptionFlowIncludeFutures {
		return fmt.Errorf("P5 evidence/population contract not enabled")
	}
	wantRoles := map[string]bool{"dated_execution_mandate": false, "dated_term_carry_allocator": false}
	for _, role := range manifest.Config.ReceiptRoles {
		if _, ok := wantRoles[role]; !ok {
			return fmt.Errorf("unexpected P5 receipt role %q", role)
		}
		wantRoles[role] = true
	}
	for role, found := range wantRoles {
		if !found {
			return fmt.Errorf("missing P5 receipt role %q", role)
		}
	}
	return nil
}

func loadP5Evidence(dir string) (*p5Evidence, error) {
	audit, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, err
	}
	rawManifest, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, err
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, err
	}
	receipts, _, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, err
	}
	symbols := make(map[uint32]string, len(manifest.Symbols))
	for _, row := range manifest.Symbols {
		symbols[row.ID] = row.Symbol
	}
	evidence := &p5Evidence{
		sources: make(map[p5EvidenceKey][]observationRecord), frontiers: make(map[fundingCarryReceiptKey]auditedFrontier),
		linkRoles: make(map[uint32]string), linkVenues: make(map[uint32]string), roleReceipts: make(map[string]int64), audit: audit,
	}
	for _, link := range manifest.Links {
		evidence.linkRoles[link.ID], evidence.linkVenues[link.ID] = link.Role, link.SourceVenue
	}
	for offset := 0; offset < len(receipts); offset += marketDataReceiptRecordBytes {
		record := decodeObservation(receipts[offset : offset+marketDataReceiptRecordBytes])
		key := p5EvidenceKey{record.clientID, record.linkID, record.mdType, symbols[record.symbolID], record.sequence, record.publishedAt}
		evidence.sources[key] = append(evidence.sources[key], record)
		evidence.roleReceipts[evidence.linkRoles[record.linkID]]++
	}
	for key, frontier := range reconstructReceiptHistory(receipts) {
		evidence.frontiers[fundingCarryReceiptKey{key.clientID, key.linkID, key.ordinal}] = frontier
	}
	return evidence, nil
}

func p5DecisionFrontier(decision datedCarryP5Decision, evidence *p5Evidence) (auditedFrontier, error) {
	if decision.DecisionFrontierLinkID == 0 {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_missing")
	}
	if decision.DecisionFrontierOrdinal == 0 {
		if decision.DecisionFrontierDeliveredAt != 0 || decision.DecisionFrontierDigest != "00000000000000000000000000000000" {
			return auditedFrontier{}, fmt.Errorf("decision_frontier_mismatch")
		}
		if evidence.linkRoles[decision.DecisionFrontierLinkID] != "dated_term_carry_allocator" || evidence.linkVenues[decision.DecisionFrontierLinkID] != decision.VenueID {
			return auditedFrontier{}, fmt.Errorf("decision_frontier_wrong_link")
		}
		return auditedFrontier{}, nil
	}
	frontier, ok := evidence.frontiers[fundingCarryReceiptKey{decision.ClientID, decision.DecisionFrontierLinkID, decision.DecisionFrontierOrdinal}]
	if !ok {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_missing")
	}
	if decision.DecisionFrontierDeliveredAt != frontier.deliveredAt || decision.DecisionFrontierDigest != hex.EncodeToString(frontier.digest[:]) {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_mismatch")
	}
	if frontier.deliveredAt > decision.DecisionTime {
		return auditedFrontier{}, fmt.Errorf("future_receipt")
	}
	if evidence.linkRoles[decision.DecisionFrontierLinkID] != "dated_term_carry_allocator" || evidence.linkVenues[decision.DecisionFrontierLinkID] != decision.VenueID {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_wrong_link")
	}
	return frontier, nil
}

func p5SourceInFrontier(decision datedCarryP5Decision, evidence *p5Evidence, mdType exchange.MDType, symbol string, sequence uint64, published int64, fingerprint string) error {
	frontier, err := p5DecisionFrontier(decision, evidence)
	if err != nil {
		return err
	}
	rows := evidence.sources[p5EvidenceKey{decision.ClientID, decision.DecisionFrontierLinkID, uint8(mdType), symbol, sequence, published}]
	if fingerprint != "" {
		filtered := make([]observationRecord, 0, len(rows))
		for _, row := range rows {
			if fingerprint == hex.EncodeToString(row.fingerprint[:]) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	if len(rows) == 0 {
		return fmt.Errorf("source_missing")
	}
	if len(rows) != 1 {
		return fmt.Errorf("source_identity_ambiguous")
	}
	record := rows[0]
	if fingerprint != "" && fingerprint != hex.EncodeToString(record.fingerprint[:]) {
		return fmt.Errorf("source_fingerprint_mismatch")
	}
	if record.ordinal > frontier.ordinal {
		return fmt.Errorf("source_after_frontier")
	}
	if record.publishedAt > decision.DecisionTime || record.deliveredAt > decision.DecisionTime {
		return fmt.Errorf("future_receipt")
	}
	return nil
}

func validateP5Listing(decision datedCarryP5Decision, listing p5Listing, found bool) error {
	if !found {
		return fmt.Errorf("canonical_listing_missing")
	}
	announcement := listing.announcement
	if announcement.Action != "listed" || announcement.Symbol != decision.FutureSymbol || announcement.InstrumentType != "FUTURE" || announcement.Underlying != decision.SpotSymbol || announcement.ListedNano == nil {
		return fmt.Errorf("canonical_listing_contract_mismatch")
	}
	if listing.at != decision.ListedNano || announcement.Timestamp != decision.ListedNano || decision.ListingPublishedAt < listing.at || *announcement.ListedNano != decision.ListedNano || announcement.ExpiryNano != decision.ExpiryNano || announcement.SettlementPrice != nil {
		return fmt.Errorf("canonical_listing_term_mismatch")
	}
	// A subscriber that joins after listing receives a deterministic replay:
	// the original ListedNano and contract fields are retained, while both the
	// message publication and descriptor timestamp become the replay time.
	// Reconstruct that public message from canonical listing evidence rather
	// than accepting the actor's fingerprint as self-authenticating.
	announcement.Timestamp = decision.ListingPublishedAt
	fingerprint, err := etypes.MarketDataFingerprint(&etypes.MarketDataMsg{
		Type: exchange.MDInstrument, Symbol: etypes.InstrumentFeedSymbol,
		SeqNum: decision.ListingSequence, Timestamp: decision.ListingPublishedAt, Data: &announcement,
	})
	if err != nil {
		return fmt.Errorf("canonical_listing_fingerprint: %w", err)
	}
	if decision.ListingFingerprint != hex.EncodeToString(fingerprint[:]) {
		return fmt.Errorf("canonical_listing_fingerprint_mismatch")
	}
	return nil
}

func recomputeP5Financials(policy datedCarryP5Config, decision datedCarryP5Decision) (p5Financials, error) {
	if !decision.HasSpotBook || !decision.HasFutureBook || decision.TimeToExpiryNanos <= 0 || decision.TimeToExpiryNanos < policy.MinTimeToExpiry {
		return p5Financials{}, fmt.Errorf("candidate_input_unavailable")
	}
	if decision.SpotPublishedAt > decision.DecisionTime || decision.FuturePublishedAt > decision.DecisionTime || decision.DecisionTime-decision.SpotPublishedAt != decision.SpotAgeNanos || decision.DecisionTime-decision.FuturePublishedAt != decision.FutureAgeNanos || decision.SpotAgeNanos > policy.MaxMarketAge || decision.FutureAgeNanos > policy.MaxMarketAge {
		return p5Financials{}, fmt.Errorf("candidate_book_time_mismatch")
	}
	candidates := make([]p5Financials, 0, 2)
	if decision.HasSpotBid && decision.HasSpotAsk && decision.SpotBid > decision.SpotAsk {
		return p5Financials{}, fmt.Errorf("invalid_spot_book")
	}
	if decision.HasFutureBid && decision.HasFutureAsk && decision.FutureBid > decision.FutureAsk {
		return p5Financials{}, fmt.Errorf("invalid_future_book")
	}
	if decision.HasSpotAsk && decision.HasFutureBid && decision.SpotAsk > 0 {
		candidates = append(candidates, p5FinancialsFor(policy, "RICH_FUTURE", decision.SpotAsk, decision.FutureBid, decision.TimeToExpiryNanos))
	}
	if decision.HasSpotBid && decision.HasFutureAsk && decision.SpotBid > 0 {
		candidates = append(candidates, p5FinancialsFor(policy, "CHEAP_FUTURE", decision.SpotBid, decision.FutureAsk, decision.TimeToExpiryNanos))
	}
	if len(candidates) == 0 {
		return p5Financials{}, fmt.Errorf("executable_touch_unavailable")
	}
	sort.Slice(candidates, func(i, j int) bool {
		if byNet := candidates[j].net.Cmp(candidates[i].net); byNet != 0 {
			return byNet < 0
		}
		return candidates[i].direction < candidates[j].direction
	})
	best := candidates[0]
	best.eligible = best.net.Cmp(best.minimum) >= 0
	return best, nil
}

func p5FinancialsFor(policy datedCarryP5Config, direction string, spotReference, futureReference, tte int64) p5Financials {
	spot := big.NewInt(spotReference)
	denominator := new(big.Int).Mul(new(big.Int).Set(spot), big.NewInt(p5DatedCarryYearNanos))
	grossSpread := new(big.Int).Sub(big.NewInt(futureReference), spot)
	financingRate, financingDirection := policy.LongSpotFundingBps, "LONG_SPOT_CASH_FINANCING"
	if direction == "CHEAP_FUTURE" {
		grossSpread.Neg(grossSpread)
		financingRate, financingDirection = policy.ShortSpotBorrowBps, "SHORT_SPOT_ASSET_BORROW"
	}
	gross := new(big.Int).Mul(new(big.Int).Set(grossSpread), big.NewInt(10_000))
	gross.Mul(gross, big.NewInt(p5DatedCarryYearNanos))
	component := func(bps int64) *big.Int { return new(big.Int).Mul(big.NewInt(bps), denominator) }
	fees := component(4 * policy.TakerFeeBps)
	financing := new(big.Int).Mul(big.NewInt(financingRate), big.NewInt(tte))
	financing.Mul(financing, spot)
	balance, margin, leg := component(policy.BalanceSheetBps), component(policy.MarginRiskBps), component(policy.LegRiskBps)
	settlement, exit := component(policy.SettlementMismatchBps), component(policy.PostSettlementExitBps)
	net := new(big.Int).Set(gross)
	for _, cost := range []*big.Int{fees, financing, balance, margin, leg, settlement, exit} {
		net.Sub(net, cost)
	}
	minimum := component(policy.MinNetCarryBps)
	return p5Financials{direction: direction, financingDirection: financingDirection, spotReference: spotReference, futureReference: futureReference,
		grossSpread: grossSpread, gross: gross, fees: fees, financing: financing, balance: balance, margin: margin, leg: leg,
		settlement: settlement, exit: exit, net: net, minimum: minimum, denominator: denominator}
}

func validateP5FinancialAttestation(decision datedCarryP5Decision, f p5Financials) error {
	wants := []struct{ name, got, want string }{
		{"direction", decision.Direction, f.direction}, {"financing_direction", decision.FinancingDirection, f.financingDirection},
		{"gross_spread", decision.GrossLockedSpreadRaw, f.grossSpread.String()}, {"gross", decision.GrossLockedBpsNumerator, f.gross.String()},
		{"fees", decision.ExecutionFeeBpsNumerator, f.fees.String()}, {"financing", decision.FinancingBpsNumerator, f.financing.String()},
		{"balance", decision.BalanceSheetBpsNumerator, f.balance.String()}, {"margin", decision.MarginRiskBpsNumerator, f.margin.String()},
		{"leg", decision.LegRiskBpsNumerator, f.leg.String()}, {"settlement", decision.SettlementMismatchNumerator, f.settlement.String()},
		{"exit", decision.PostSettlementExitNumerator, f.exit.String()}, {"net", decision.NetCarryBpsNumerator, f.net.String()},
		{"minimum", decision.MinimumNetBpsNumerator, f.minimum.String()}, {"denominator", decision.RationalDenominator, f.denominator.String()},
	}
	for _, row := range wants {
		if row.got != row.want {
			return fmt.Errorf("%s_attestation_mismatch", row.name)
		}
	}
	if decision.SpotExecutionReference != f.spotReference || decision.FutureExecutionReference != f.futureReference {
		return fmt.Errorf("execution_reference_mismatch")
	}
	return nil
}

func validateP5CandidatePolicy(policy datedCarryP5Config, decision datedCarryP5Decision, f p5Financials) error {
	direction := int64(1)
	if f.direction == "CHEAP_FUTURE" {
		direction = -1
	}
	wantSpot, wantFuture := direction*policy.MaxPosition, -direction*policy.MaxPosition
	if !f.eligible {
		if decision.Action != "NET_CARRY_BELOW_MINIMUM" || decision.ProposedTargetSpot != 0 || decision.ProposedTargetFuture != 0 || decision.TargetSpot != 0 || decision.TargetFuture != 0 || decision.RequestID != 0 {
			return fmt.Errorf("ineligible_action_mismatch")
		}
		return nil
	}
	if decision.ProposedTargetSpot != wantSpot || decision.ProposedTargetFuture != wantFuture {
		return fmt.Errorf("proposed_target_mismatch")
	}
	if !policy.TradeEnabled {
		if decision.Action != "SHADOW_ELIGIBLE" || decision.TradeEnabled || decision.TargetSpot != 0 || decision.TargetFuture != 0 || decision.TargetChangedAt != 0 || decision.RequestID != 0 {
			return fmt.Errorf("shadow_policy_mismatch")
		}
		return nil
	}
	if decision.Action != "SUBMIT_ENTRY_SPOT_IOC" || !decision.TradeEnabled || decision.TargetSpot != wantSpot || decision.TargetFuture != wantFuture || decision.TargetChangedAt != decision.DecisionTime || decision.RequestID == 0 {
		return fmt.Errorf("active_target_mismatch")
	}
	wantSide, wantPrice := exchange.Buy.String(), decision.SpotAsk
	if direction < 0 {
		wantSide, wantPrice = exchange.Sell.String(), decision.SpotBid
	}
	wantQty := policy.LotQty
	available := decision.SpotAskQty
	if direction < 0 {
		available = decision.SpotBidQty
	}
	if available > 0 && wantQty > available {
		wantQty = available
	}
	postOnly := decision.PostOnly != nil && *decision.PostOnly
	if decision.Leg != "ENTRY_SPOT_IOC" || decision.Side != wantSide || decision.LimitPrice != wantPrice || decision.RequestedQty != wantQty || wantQty < policy.MinOrderSize || decision.OrderType != exchange.LimitOrder.String() || decision.TimeInForce != exchange.IOC.String() || decision.PostOnly == nil || postOnly {
		return fmt.Errorf("active_submission_mismatch")
	}
	return nil
}

// MeasureDatedCarryP5 audits P5 links 1--3 and the declared shadow/active
// target boundary. It deliberately fails closed until every candidate source
// and exact actor attestation is independently reconstructed.
func (r *Run) MeasureDatedCarryP5() (*DatedCarryP5Audit, error) {
	manifest, err := loadDatedCarryP5Manifest(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validateDatedCarryP5Manifest(manifest); err != nil {
		return nil, err
	}
	evidence, evidenceErr := loadP5Evidence(r.Dir)
	result := &DatedCarryP5Audit{TradeEnabled: manifest.Config.DatedTermCarry.TradeEnabled, ActionCounts: make(map[string]int64)}
	check := func(d datedCarryP5Decision, failure string) {
		result.Checks = append(result.Checks, DatedCarryP5Check{VenueID: d.VenueID, ClientID: d.ClientID, At: d.DecisionTime, Failure: failure})
	}
	if evidenceErr != nil || evidence == nil || evidence.audit == nil || !evidence.audit.Valid {
		result.ReceiptEvidenceErrors++
	} else {
		result.ReceiptAuditValid = true
		result.RoleLinksActive = p5RequiredRoleLinksActive(evidence)
		if !result.RoleLinksActive {
			result.ReceiptEvidenceErrors++
		}
	}
	var decisions []datedCarryP5Decision
	listings := make(map[p5ListingKey]p5Listing)
	if err := r.Scan(ScanOptions{Events: []string{"dated_term_carry_decision", "instrument_listed"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "dated_term_carry_decision":
			var decision datedCarryP5Decision
			if event.Decode(&decision) != nil || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID || decision.Desk == "" || r.Role(event.VenueID, event.ClientID) != "dated_term_carry_allocator" {
				result.PolicyMismatches++
				return
			}
			decisions = append(decisions, decision)
		case "instrument_listed":
			var announcement etypes.InstrumentAnnouncement
			if event.Decode(&announcement) != nil || announcement.Symbol == "" {
				return
			}
			key := p5ListingKey{event.VenueID, announcement.Symbol}
			if _, duplicate := listings[key]; duplicate {
				result.TermMismatches++
				return
			}
			listings[key] = p5Listing{at: event.SimTS, announcement: announcement}
		}
	}); err != nil {
		return nil, fmt.Errorf("P5 dated-carry scan: %w", err)
	}
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].DecisionTime != decisions[j].DecisionTime {
			return decisions[i].DecisionTime < decisions[j].DecisionTime
		}
		if decisions[i].VenueID != decisions[j].VenueID {
			return decisions[i].VenueID < decisions[j].VenueID
		}
		return decisions[i].ClientID < decisions[j].ClientID
	})
	for _, decision := range decisions {
		result.Decisions++
		result.ActionCounts[decision.Action]++
		if decision.PolicyVersion != p5DatedCarryPolicyVersion || decision.Enabled != manifest.Config.DatedTermCarry.Enabled || decision.TradeEnabled != manifest.Config.DatedTermCarry.TradeEnabled || decision.SpotSymbol != manifest.Config.DatedTermCarry.SpotSymbol {
			result.PolicyMismatches++
			check(decision, "decision_policy_mismatch")
		}
		if evidence == nil {
			continue
		}
		if _, err := p5DecisionFrontier(decision, evidence); err != nil {
			result.FrontierMismatches++
			if err.Error() == "future_receipt" {
				result.FutureSourceUse++
			}
			check(decision, err.Error())
			continue
		}
		if decision.FutureSymbol == "" {
			continue
		}
		if decision.OriginalTenorNanos != manifest.Config.DatedTermCarry.TargetTenor || decision.ExpiryNano-decision.ListedNano != decision.OriginalTenorNanos || decision.ExpiryNano-decision.DecisionTime != decision.TimeToExpiryNanos {
			result.TermMismatches++
			check(decision, "term_identity_mismatch")
		}
		if err := validateP5Listing(decision, listings[p5ListingKey{decision.VenueID, decision.FutureSymbol}], listings[p5ListingKey{decision.VenueID, decision.FutureSymbol}].announcement.Symbol != ""); err != nil {
			result.TermMismatches++
			check(decision, err.Error())
		}
		if err := p5SourceInFrontier(decision, evidence, exchange.MDInstrument, "_instruments", decision.ListingSequence, decision.ListingPublishedAt, decision.ListingFingerprint); err != nil {
			result.SourceMismatches++
			if err.Error() == "future_receipt" {
				result.FutureSourceUse++
			}
			check(decision, "listing_"+err.Error())
		} else {
			result.ListingMatches++
		}
		if decision.HasSpotBook {
			if err := p5SourceInFrontier(decision, evidence, exchange.MDSnapshot, decision.SpotSymbol, decision.SpotSequence, decision.SpotPublishedAt, ""); err != nil {
				result.SourceMismatches++
				if err.Error() == "future_receipt" {
					result.FutureSourceUse++
				}
				check(decision, "spot_"+err.Error())
			} else {
				result.BookMatches++
			}
		}
		if decision.HasFutureBook {
			if err := p5SourceInFrontier(decision, evidence, exchange.MDSnapshot, decision.FutureSymbol, decision.FutureSequence, decision.FuturePublishedAt, ""); err != nil {
				result.SourceMismatches++
				if err.Error() == "future_receipt" {
					result.FutureSourceUse++
				}
				check(decision, "future_"+err.Error())
			} else {
				result.BookMatches++
			}
		}
		if decision.Action != "NET_CARRY_BELOW_MINIMUM" && decision.Action != "SHADOW_ELIGIBLE" && decision.Action != "SUBMIT_ENTRY_SPOT_IOC" {
			continue
		}
		result.CandidateDecisions++
		financials, err := recomputeP5Financials(*manifest.Config.DatedTermCarry, decision)
		if err != nil {
			result.ArithmeticMismatches++
			check(decision, err.Error())
			continue
		}
		if err := validateP5FinancialAttestation(decision, financials); err != nil {
			result.ArithmeticMismatches++
			check(decision, err.Error())
		}
		if err := validateP5CandidatePolicy(*manifest.Config.DatedTermCarry, decision, financials); err != nil {
			result.PolicyMismatches++
			check(decision, err.Error())
		}
		if financials.eligible {
			result.EligibleDecisions++
			if decision.Action == "SHADOW_ELIGIBLE" {
				result.ShadowEligible++
			}
			if decision.TargetChangedAt == decision.DecisionTime && decision.TargetSpot != 0 && decision.TargetFuture != 0 {
				result.TargetChanges++
			}
			if decision.RequestID != 0 {
				result.Submitted++
			}
		}
	}
	if result.Decisions == 0 {
		result.PolicyMismatches++
		result.Checks = append(result.Checks, DatedCarryP5Check{Failure: "missing_dated_carry_decisions"})
	}
	result.Valid = result.ReceiptAuditValid && result.RoleLinksActive && result.ReceiptEvidenceErrors == 0 && result.SourceMismatches == 0 && result.FutureSourceUse == 0 && result.FrontierMismatches == 0 && result.TermMismatches == 0 && result.ArithmeticMismatches == 0 && result.PolicyMismatches == 0
	return result, nil
}
