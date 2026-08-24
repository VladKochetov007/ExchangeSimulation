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
)

// FundingCarryAudit independently checks V2-5 P0's local-information and
// economic-decision contract. It deliberately imports neither the actor nor
// multivenue implementation; all conclusions are reconstructed from retained
// JSON evidence, the frozen run manifest, and V2-0 receipt sidecars.
type FundingCarryAudit struct {
	Decisions int64 `json:"decisions"`
	Submitted int64 `json:"submitted"`
	Deferred  int64 `json:"deferred"`
	Accepted  int64 `json:"accepted"`
	Rejected  int64 `json:"rejected"`
	Fills     int64 `json:"fills"`
	Cancelled int64 `json:"cancelled"`

	ReceiptAuditValid     bool  `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors int64 `json:"receipt_evidence_errors"`
	FundingReceiptMatches int64 `json:"funding_receipt_matches"`
	BookReceiptMatches    int64 `json:"book_receipt_matches"`
	MissingFundingReceipt int64 `json:"missing_funding_receipt"`
	MissingBookReceipt    int64 `json:"missing_book_receipt"`
	ReceiptMismatches     int64 `json:"receipt_mismatches"`
	FutureReceiptUse      int64 `json:"future_receipt_use"`

	InvalidDecisionRecords      int64 `json:"invalid_decision_records"`
	MissingDecisionEvidence     int64 `json:"missing_decision_evidence"`
	DuplicateDecisions          int64 `json:"duplicate_decisions"`
	DecisionFieldMismatches     int64 `json:"decision_field_mismatches"`
	FundingArithmeticMismatches int64 `json:"funding_arithmetic_mismatches"`
	FundingSignMismatches       int64 `json:"funding_sign_mismatches"`
	MissingGatewayDecisions     int64 `json:"missing_gateway_decisions"`
	GatewayDecisionMismatches   int64 `json:"gateway_decision_mismatches"`
	MissingVenueOutcomes        int64 `json:"missing_venue_outcomes"`
	DuplicateVenueOutcomes      int64 `json:"duplicate_venue_outcomes"`
	MissingActorOutcomes        int64 `json:"missing_actor_outcomes"`
	ActorOutcomeMismatches      int64 `json:"actor_outcome_mismatches"`

	ActionCounts map[string]int64    `json:"action_counts,omitempty"`
	Checks       []FundingCarryCheck `json:"checks,omitempty"`
	Valid        bool                `json:"valid"`
}

// FundingCarryCheck identifies an exact persisted-evidence failure. It is
// deliberately granular so a failed P0 activation is not compressed into an
// apparent economic null.
type FundingCarryCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id"`
	Failure   string `json:"failure"`
}

type fundingCarryPolicyConfig struct {
	Enabled         bool   `json:"enabled"`
	SpotSymbol      string `json:"spot_symbol"`
	PerpSymbol      string `json:"perp_symbol"`
	DecisionPeriod  int64  `json:"decision_period"`
	FundingHorizon  int64  `json:"funding_horizon"`
	MaxFundingAge   int64  `json:"max_funding_age"`
	TakerFeeBps     int64  `json:"taker_fee_bps"`
	BorrowAnnualBps int64  `json:"borrow_annual_bps"`
	BalanceSheetBps int64  `json:"balance_sheet_bps"`
	MarginRiskBps   int64  `json:"margin_risk_bps"`
	LegRiskBps      int64  `json:"leg_risk_bps"`
	MinNetCarryBps  int64  `json:"min_net_carry_bps"`
	MaxPosition     int64  `json:"max_position"`
	LotQty          int64  `json:"lot_qty"`
	MinOrderSize    int64  `json:"min_order_size"`
	SpotTick        int64  `json:"spot_tick"`
	PerpTick        int64  `json:"perp_tick"`
}

type fundingCarryManifest struct {
	Config struct {
		FundingCarryArbitrageur *fundingCarryPolicyConfig `json:"funding_carry_arbitrageur"`
	} `json:"config"`
}

type fundingCarryDecision struct {
	VenueID             string `json:"venue_id"`
	Desk                string `json:"desk"`
	ClientID            uint64 `json:"client_id"`
	PolicyVersion       string `json:"policy_version"`
	DecisionTime        int64  `json:"decision_time"`
	Enabled             bool   `json:"enabled"`
	Subscribed          bool   `json:"subscribed"`
	Pending             bool   `json:"pending"`
	Action              string `json:"action_or_defer_reason"`
	SpotSymbol          string `json:"spot_symbol"`
	PerpSymbol          string `json:"perp_symbol"`
	SpotPosition        int64  `json:"spot_position"`
	PerpPosition        int64  `json:"perp_position"`
	DesiredSpotPosition int64  `json:"desired_spot_position"`
	DesiredPerpPosition int64  `json:"desired_perp_position"`

	HasSpotBook        bool   `json:"has_spot_book"`
	SpotPublishedAt    int64  `json:"spot_published_at"`
	SpotDeliveredAt    int64  `json:"spot_delivered_at"`
	SpotSequence       uint64 `json:"spot_sequence"`
	SpotReceiptLinkID  uint32 `json:"spot_receipt_link_id"`
	SpotReceiptOrdinal uint64 `json:"spot_receipt_ordinal"`
	HasSpotBid         bool   `json:"has_spot_bid"`
	SpotBid            int64  `json:"spot_bid"`
	SpotBidQty         int64  `json:"spot_bid_qty"`
	HasSpotAsk         bool   `json:"has_spot_ask"`
	SpotAsk            int64  `json:"spot_ask"`
	SpotAskQty         int64  `json:"spot_ask_qty"`

	HasPerpBook        bool   `json:"has_perp_book"`
	PerpPublishedAt    int64  `json:"perp_published_at"`
	PerpDeliveredAt    int64  `json:"perp_delivered_at"`
	PerpSequence       uint64 `json:"perp_sequence"`
	PerpReceiptLinkID  uint32 `json:"perp_receipt_link_id"`
	PerpReceiptOrdinal uint64 `json:"perp_receipt_ordinal"`
	HasPerpBid         bool   `json:"has_perp_bid"`
	PerpBid            int64  `json:"perp_bid"`
	PerpBidQty         int64  `json:"perp_bid_qty"`
	HasPerpAsk         bool   `json:"has_perp_ask"`
	PerpAsk            int64  `json:"perp_ask"`
	PerpAskQty         int64  `json:"perp_ask_qty"`

	HasFunding             bool   `json:"has_funding"`
	FundingRateBps         int64  `json:"funding_rate_bps"`
	FundingPublishedAt     int64  `json:"funding_published_at"`
	FundingDeliveredAt     int64  `json:"funding_delivered_at"`
	FundingSequence        uint64 `json:"funding_sequence"`
	FundingReceiptLinkID   uint32 `json:"funding_receipt_link_id"`
	FundingReceiptOrdinal  uint64 `json:"funding_receipt_ordinal"`
	FundingReceiptDigest   string `json:"funding_receipt_digest"`
	FundingNextAt          int64  `json:"funding_next_at"`
	FundingIntervalSeconds int64  `json:"funding_interval_seconds"`
	FundingMarkAvailable   bool   `json:"funding_mark_available"`
	FundingMarkPrice       int64  `json:"funding_mark_price"`
	FundingIndexAvailable  bool   `json:"funding_index_available"`
	FundingIndexPrice      int64  `json:"funding_index_price"`
	FundingAgeNanos        int64  `json:"funding_age_nanos"`
	FundingHorizon         int64  `json:"funding_horizon"`
	HoldingNanos           int64  `json:"holding_nanos"`

	SpotMid             int64  `json:"spot_mid"`
	PerpMid             int64  `json:"perp_mid"`
	PremiumBps          int64  `json:"premium_bps"`
	FundingIncomeBps    int64  `json:"funding_income_bps"`
	TakerFeeCostBps     int64  `json:"taker_fee_cost_bps"`
	BorrowCostBps       int64  `json:"borrow_cost_bps"`
	BalanceSheetCostBps int64  `json:"balance_sheet_cost_bps"`
	MarginRiskCostBps   int64  `json:"margin_risk_cost_bps"`
	LegRiskCostBps      int64  `json:"leg_risk_cost_bps"`
	NetCarryBps         int64  `json:"net_carry_bps"`
	MinNetCarryBps      int64  `json:"min_net_carry_bps"`
	Leg                 string `json:"leg"`
	Side                string `json:"side"`
	LimitPrice          int64  `json:"limit_price"`
	RequestedQty        int64  `json:"requested_qty"`
	RequestID           uint64 `json:"request_id"`
}

type fundingCarryOutcome struct {
	VenueID            string `json:"venue_id"`
	Desk               string `json:"desk"`
	ClientID           uint64 `json:"client_id"`
	DecisionTime       int64  `json:"decision_time"`
	ExecutionTime      int64  `json:"execution_time"`
	Event              string `json:"event"`
	Leg                string `json:"leg"`
	RequestID          uint64 `json:"request_id"`
	OrderID            uint64 `json:"order_id"`
	TradeID            uint64 `json:"trade_id"`
	Symbol             string `json:"symbol"`
	Side               string `json:"side"`
	Qty                int64  `json:"qty"`
	Price              int64  `json:"price"`
	FeeAmount          int64  `json:"fee_amount"`
	FeeAsset           string `json:"fee_asset"`
	RemainingQty       int64  `json:"remaining_qty"`
	RejectReason       string `json:"reject_reason"`
	SpotPositionBefore int64  `json:"spot_position_before"`
	SpotPositionAfter  int64  `json:"spot_position_after"`
	PerpPositionBefore int64  `json:"perp_position_before"`
	PerpPositionAfter  int64  `json:"perp_position_after"`
}

type fundingCarryGatewayDecision struct {
	clientID   uint64
	linkID     uint32
	requestID  uint64
	decisionAt int64
	price      int64
	qty        int64
	side       uint8
	orderType  uint8
	tif        uint8
}

type fundingCarryVenueOrder struct {
	VenueID     string
	ClientID    uint64
	RequestID   uint64 `json:"request_id"`
	OrderID     uint64 `json:"order_id"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
	Error       string `json:"error"`
}

type fundingCarryVenueFill struct {
	VenueID   string
	ClientID  uint64
	OrderID   uint64 `json:"order_id"`
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Qty       int64  `json:"qty"`
	Price     int64  `json:"price"`
	TradeID   uint64 `json:"trade_id"`
	FeeAmount int64  `json:"fee_amount"`
	FeeAsset  string `json:"fee_asset"`
}

type fundingCarryKey struct {
	venue           string
	client, request uint64
}
type fundingCarryOrderKey struct {
	venue         string
	client, order uint64
}
type fundingCarryReceiptKey struct {
	client  uint64
	link    uint32
	ordinal uint64
}

// MeasureFundingCarry independently audits V2-5 P0. A valid result means the
// recorder contract and declared calculation are internally evidenced; it is
// not a claim that funding anchors the market.
func (r *Run) MeasureFundingCarry() (*FundingCarryAudit, error) {
	policy, err := loadFundingCarryPolicy(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validFundingCarryPolicy(policy); err != nil {
		return nil, err
	}
	result := &FundingCarryAudit{ActionCounts: make(map[string]int64)}
	addCheck := func(venue string, client, request uint64, failure string) {
		result.Checks = append(result.Checks, FundingCarryCheck{VenueID: venue, ClientID: client, RequestID: request, Failure: failure})
	}

	receipts, frontiers, gateway, receiptAudit, receiptErr := fundingCarryReceipts(r.Dir)
	if receiptErr != nil {
		result.ReceiptEvidenceErrors++
	} else {
		result.ReceiptAuditValid = receiptAudit.Valid
		if !receiptAudit.Valid {
			result.ReceiptEvidenceErrors++
		}
	}

	decisions := make([]fundingCarryDecision, 0)
	outcomes := make([]fundingCarryOutcome, 0)
	accepted := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	rejected := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	fills := make(map[fundingCarryOrderKey][]fundingCarryVenueFill)
	cancels := make(map[fundingCarryOrderKey]int)
	err = r.Scan(ScanOptions{Events: []string{"funding_carry_decision", "funding_carry_leg_outcome", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "funding_carry_decision":
			var decision fundingCarryDecision
			if event.Decode(&decision) != nil {
				result.InvalidDecisionRecords++
				addCheck(event.VenueID, event.ClientID, 0, "invalid_decision_json")
				return
			}
			if decision.VenueID != event.VenueID || decision.ClientID != event.ClientID || r.Role(event.VenueID, event.ClientID) != "funding_carry_arb" {
				result.InvalidDecisionRecords++
				addCheck(event.VenueID, event.ClientID, decision.RequestID, "decision_envelope_mismatch")
				return
			}
			decisions = append(decisions, decision)
		case "funding_carry_leg_outcome":
			var outcome fundingCarryOutcome
			if event.Decode(&outcome) != nil {
				result.ActorOutcomeMismatches++
				addCheck(event.VenueID, event.ClientID, 0, "invalid_actor_outcome_json")
				return
			}
			if outcome.VenueID != event.VenueID || outcome.ClientID != event.ClientID {
				result.ActorOutcomeMismatches++
				addCheck(event.VenueID, event.ClientID, outcome.RequestID, "actor_outcome_envelope_mismatch")
				return
			}
			outcomes = append(outcomes, outcome)
		case "OrderAccepted", "OrderRejected":
			var order fundingCarryVenueOrder
			if event.Decode(&order) != nil || order.RequestID == 0 {
				return
			}
			order.VenueID, order.ClientID = event.VenueID, event.ClientID
			key := fundingCarryKey{event.VenueID, event.ClientID, order.RequestID}
			if event.Name == "OrderAccepted" {
				accepted[key] = append(accepted[key], order)
			} else {
				rejected[key] = append(rejected[key], order)
			}
		case "OrderFill":
			var fill fundingCarryVenueFill
			if event.Decode(&fill) != nil || fill.OrderID == 0 {
				return
			}
			fill.VenueID, fill.ClientID = event.VenueID, event.ClientID
			fills[fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}] = append(fills[fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}], fill)
		case "OrderCancelled":
			var cancellation struct {
				OrderID uint64 `json:"order_id"`
			}
			if event.Decode(&cancellation) == nil && cancellation.OrderID != 0 {
				cancels[fundingCarryOrderKey{event.VenueID, event.ClientID, cancellation.OrderID}]++
			}
		}
	})
	if err != nil {
		return nil, err
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
	if len(decisions) == 0 {
		result.MissingDecisionEvidence++
		addCheck("", 0, 0, "missing_funding_carry_decisions")
	}

	seenDecisions := make(map[fundingCarryKey]struct{})
	seenDecisionTicks := make(map[struct {
		venue  string
		client uint64
		time   int64
	}]struct{})
	for _, decision := range decisions {
		result.Decisions++
		result.ActionCounts[decision.Action]++
		tick := struct {
			venue  string
			client uint64
			time   int64
		}{decision.VenueID, decision.ClientID, decision.DecisionTime}
		if _, duplicate := seenDecisionTicks[tick]; duplicate {
			result.DuplicateDecisions++
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_decision_tick")
		}
		seenDecisionTicks[tick] = struct{}{}
		if err := validateFundingCarryDecision(policy, decision, receipts, frontiers); err != nil {
			result.DecisionFieldMismatches++
			if isFundingCarryArithmeticFailure(err) {
				result.FundingArithmeticMismatches++
			}
			if isFundingCarrySignFailure(err) {
				result.FundingSignMismatches++
			}
			if isFundingCarryReceiptFailure(err) {
				result.ReceiptMismatches++
			}
			if isFundingCarryFutureFailure(err) {
				result.FutureReceiptUse++
			}
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, err.Error())
		}
		if decision.HasFunding {
			if receipt, found := receipts[fundingCarryReceiptKey{decision.ClientID, decision.FundingReceiptLinkID, decision.FundingReceiptOrdinal}]; !found {
				result.MissingFundingReceipt++
				addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "missing_funding_receipt")
			} else if fundingCarryReceiptMatches(decision, receipt, frontiers[fundingCarryReceiptKey{decision.ClientID, decision.FundingReceiptLinkID, decision.FundingReceiptOrdinal}], exchange.MDFunding) {
				result.FundingReceiptMatches++
			} else {
				result.ReceiptMismatches++
				addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "funding_receipt_identity_mismatch")
			}
		}
		for _, book := range []struct {
			present              bool
			link                 uint32
			ordinal              uint64
			sequence             uint64
			published, delivered int64
		}{
			{decision.HasSpotBook, decision.SpotReceiptLinkID, decision.SpotReceiptOrdinal, decision.SpotSequence, decision.SpotPublishedAt, decision.SpotDeliveredAt},
			{decision.HasPerpBook, decision.PerpReceiptLinkID, decision.PerpReceiptOrdinal, decision.PerpSequence, decision.PerpPublishedAt, decision.PerpDeliveredAt},
		} {
			if !book.present {
				continue
			}
			receipt, found := receipts[fundingCarryReceiptKey{decision.ClientID, book.link, book.ordinal}]
			if !found {
				result.MissingBookReceipt++
				addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "missing_book_receipt")
				continue
			}
			if receipt.mdType != uint8(exchange.MDSnapshot) || receipt.sequence != book.sequence || receipt.publishedAt != book.published || receipt.deliveredAt != book.delivered || receipt.deliveredAt > decision.DecisionTime {
				result.ReceiptMismatches++
				if receipt.deliveredAt > decision.DecisionTime {
					result.FutureReceiptUse++
				}
				addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "book_receipt_identity_or_time_mismatch")
			} else {
				result.BookReceiptMatches++
			}
		}
		if !fundingCarrySubmission(decision.Action) {
			result.Deferred++
			continue
		}
		result.Submitted++
		key := fundingCarryKey{decision.VenueID, decision.ClientID, decision.RequestID}
		if _, duplicate := seenDecisions[key]; duplicate {
			result.DuplicateDecisions++
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_submitted_decision")
		}
		seenDecisions[key] = struct{}{}
		gatewayDecision, found := gateway[key]
		if !found {
			result.MissingGatewayDecisions++
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "missing_gateway_decision")
		} else if !fundingCarryGatewayMatches(decision, gatewayDecision) {
			result.GatewayDecisionMismatches++
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "gateway_decision_mismatch")
		}
		acceptedRows, rejectedRows := accepted[key], rejected[key]
		if len(acceptedRows)+len(rejectedRows) == 0 {
			result.MissingVenueOutcomes++
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "missing_venue_outcome")
		} else if len(acceptedRows)+len(rejectedRows) != 1 {
			result.DuplicateVenueOutcomes++
			addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_venue_outcome")
		} else if len(acceptedRows) == 1 {
			result.Accepted++
			if !fundingCarryVenueOrderMatches(decision, acceptedRows[0]) {
				result.GatewayDecisionMismatches++
				addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "accepted_order_mismatch")
			}
		} else {
			result.Rejected++
			if !fundingCarryVenueOrderMatches(decision, rejectedRows[0]) {
				result.GatewayDecisionMismatches++
				addCheck(decision.VenueID, decision.ClientID, decision.RequestID, "rejected_order_mismatch")
			}
		}
	}

	actorOutcomes := make(map[fundingCarryKey][]fundingCarryOutcome)
	for _, outcome := range outcomes {
		actorOutcomes[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}] = append(actorOutcomes[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}], outcome)
	}
	for key, acceptedRows := range accepted {
		if r.Role(key.venue, key.client) != "funding_carry_arb" || len(acceptedRows) != 1 {
			continue
		}
		order := acceptedRows[0]
		outcomeRows := actorOutcomes[key]
		if !fundingCarryHasActorOutcome(outcomeRows, "ORDER_ACCEPTED", order.OrderID) {
			result.MissingActorOutcomes++
			addCheck(key.venue, key.client, key.request, "missing_actor_acceptance")
		}
		for _, fill := range fills[fundingCarryOrderKey{key.venue, key.client, order.OrderID}] {
			result.Fills++
			if !fundingCarryHasActorFill(outcomeRows, fill) {
				result.ActorOutcomeMismatches++
				addCheck(key.venue, key.client, key.request, "actor_fill_mismatch")
			}
		}
		if cancels[fundingCarryOrderKey{key.venue, key.client, order.OrderID}] > 0 {
			result.Cancelled++
			if !fundingCarryHasActorOutcome(outcomeRows, "ORDER_CANCELLED", order.OrderID) {
				result.MissingActorOutcomes++
				addCheck(key.venue, key.client, key.request, "missing_actor_cancellation")
			}
		}
	}
	for key, rejectedRows := range rejected {
		if r.Role(key.venue, key.client) != "funding_carry_arb" || len(rejectedRows) != 1 {
			continue
		}
		if !fundingCarryHasActorRejected(actorOutcomes[key], rejectedRows[0].Error) {
			result.MissingActorOutcomes++
			addCheck(key.venue, key.client, key.request, "missing_actor_rejection")
		}
	}

	result.Valid = result.ReceiptAuditValid && result.ReceiptEvidenceErrors == 0 && result.MissingFundingReceipt == 0 && result.MissingBookReceipt == 0 && result.ReceiptMismatches == 0 && result.FutureReceiptUse == 0 && result.InvalidDecisionRecords == 0 && result.MissingDecisionEvidence == 0 && result.DuplicateDecisions == 0 && result.DecisionFieldMismatches == 0 && result.FundingArithmeticMismatches == 0 && result.FundingSignMismatches == 0 && result.MissingGatewayDecisions == 0 && result.GatewayDecisionMismatches == 0 && result.MissingVenueOutcomes == 0 && result.DuplicateVenueOutcomes == 0 && result.MissingActorOutcomes == 0 && result.ActorOutcomeMismatches == 0
	return result, nil
}

func loadFundingCarryPolicy(dir string) (fundingCarryPolicyConfig, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return fundingCarryPolicyConfig{}, fmt.Errorf("read funding-carry manifest: %w", err)
	}
	var manifest fundingCarryManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fundingCarryPolicyConfig{}, fmt.Errorf("decode funding-carry manifest: %w", err)
	}
	if manifest.Config.FundingCarryArbitrageur == nil {
		return fundingCarryPolicyConfig{}, fmt.Errorf("funding-carry policy missing from manifest")
	}
	return *manifest.Config.FundingCarryArbitrageur, nil
}

func validFundingCarryPolicy(policy fundingCarryPolicyConfig) error {
	if policy.SpotSymbol != "ABC/USD" || policy.PerpSymbol != "ABC-PERP" || policy.DecisionPeriod <= 0 || policy.FundingHorizon <= 0 || policy.MaxFundingAge <= 0 || policy.MaxPosition <= 0 || policy.LotQty <= 0 || policy.MinOrderSize < 0 || policy.SpotTick <= 0 || policy.PerpTick <= 0 {
		return fmt.Errorf("invalid funding-carry policy manifest")
	}
	for _, value := range []int64{policy.TakerFeeBps, policy.BorrowAnnualBps, policy.BalanceSheetBps, policy.MarginRiskBps, policy.LegRiskBps, policy.MinNetCarryBps} {
		if value < 0 {
			return fmt.Errorf("negative funding-carry bps policy")
		}
	}
	return nil
}

func fundingCarryReceipts(dir string) (map[fundingCarryReceiptKey]observationRecord, map[fundingCarryReceiptKey]auditedFrontier, map[fundingCarryKey]fundingCarryGatewayDecision, *MarketDataReceiptAudit, error) {
	audit, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, nil, nil, audit, err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, nil, nil, audit, err
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, nil, nil, audit, err
	}
	receiptRaw, _, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, nil, nil, audit, err
	}
	decisionRaw, _, err := readEvidenceFile(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, nil, nil, audit, err
	}
	receipts := make(map[fundingCarryReceiptKey]observationRecord, manifest.Receipts.Records)
	for offset := 0; offset < len(receiptRaw); offset += marketDataReceiptRecordBytes {
		record := decodeObservation(receiptRaw[offset : offset+marketDataReceiptRecordBytes])
		receipts[fundingCarryReceiptKey{record.clientID, record.linkID, record.ordinal}] = record
	}
	frontierHistory := reconstructReceiptHistory(receiptRaw)
	frontiers := make(map[fundingCarryReceiptKey]auditedFrontier, len(frontierHistory))
	for key, frontier := range frontierHistory {
		frontiers[fundingCarryReceiptKey{key.clientID, key.linkID, key.ordinal}] = frontier
	}
	linkVenue := make(map[uint32]string, len(manifest.Links))
	for _, link := range manifest.Links {
		linkVenue[link.ID] = link.SourceVenue
	}
	gateways := make(map[fundingCarryKey]fundingCarryGatewayDecision)
	for offset := 0; offset < len(decisionRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(decisionRaw[offset : offset+marketDataDecisionRecordBytes])
		gateways[fundingCarryKey{linkVenue[record.linkID], record.clientID, record.requestID}] = fundingCarryGatewayDecision{clientID: record.clientID, linkID: record.linkID, requestID: record.requestID, decisionAt: record.decisionAt, price: record.price, qty: record.qty, side: record.side, orderType: record.orderType, tif: record.tif}
	}
	return receipts, frontiers, gateways, audit, nil
}

func fundingCarryReceiptMatches(decision fundingCarryDecision, receipt observationRecord, frontier auditedFrontier, want exchange.MDType) bool {
	if receipt.mdType != uint8(want) || receipt.sequence != decision.FundingSequence || receipt.publishedAt != decision.FundingPublishedAt || receipt.deliveredAt != decision.FundingDeliveredAt || receipt.deliveredAt > decision.DecisionTime {
		return false
	}
	return frontier.ordinal == decision.FundingReceiptOrdinal && frontier.deliveredAt == decision.FundingDeliveredAt && decision.FundingReceiptDigest == hex.EncodeToString(frontier.digest[:])
}

func validateFundingCarryDecision(policy fundingCarryPolicyConfig, decision fundingCarryDecision, receipts map[fundingCarryReceiptKey]observationRecord, frontiers map[fundingCarryReceiptKey]auditedFrontier) error {
	if decision.VenueID == "" || decision.ClientID == 0 || decision.Desk == "" || decision.PolicyVersion != "v2_5_p0_funding_carry_v1" || decision.DecisionTime == 0 || decision.Action == "" || decision.SpotSymbol != policy.SpotSymbol || decision.PerpSymbol != policy.PerpSymbol || decision.FundingHorizon != policy.FundingHorizon || decision.MinNetCarryBps != policy.MinNetCarryBps {
		return fmt.Errorf("invalid_decision_fields")
	}
	if !fundingCarryKnownAction(decision.Action) {
		return fmt.Errorf("unknown_action")
	}
	if !fundingCarrySubmission(decision.Action) && decision.RequestID != 0 {
		return fmt.Errorf("deferred_action_has_request_id")
	}
	if decision.Action == "NOT_SUBSCRIBED" && decision.Subscribed {
		return fmt.Errorf("not_subscribed_action_mismatch")
	}
	if decision.Action == "POLICY_DISABLED" && (decision.Enabled || !decision.Subscribed) {
		return fmt.Errorf("policy_disabled_action_mismatch")
	}
	if decision.Action == "REQUEST_PENDING" && (!decision.Enabled || !decision.Subscribed || !decision.Pending) {
		return fmt.Errorf("pending_action_mismatch")
	}
	if fundingCarrySubmission(decision.Action) && (!decision.Enabled || !decision.Subscribed || decision.Pending) {
		return fmt.Errorf("submission_state_mismatch")
	}
	if decision.HasFunding {
		if decision.FundingDeliveredAt > decision.DecisionTime || decision.FundingPublishedAt > decision.DecisionTime {
			return fmt.Errorf("future_receipt")
		}
		receipt, found := receipts[fundingCarryReceiptKey{decision.ClientID, decision.FundingReceiptLinkID, decision.FundingReceiptOrdinal}]
		if !found {
			return fmt.Errorf("receipt_missing")
		}
		frontier, knownFrontier := frontiers[fundingCarryReceiptKey{decision.ClientID, decision.FundingReceiptLinkID, decision.FundingReceiptOrdinal}]
		if !knownFrontier || !fundingCarryReceiptMatches(decision, receipt, frontier, exchange.MDFunding) {
			return fmt.Errorf("receipt_mismatch")
		}
		if decision.FundingReceiptDigest == "" {
			return fmt.Errorf("receipt_digest_missing")
		}
	}
	if fundingCarrySubmission(decision.Action) {
		if decision.RequestID == 0 || decision.Leg == "" || decision.Side == "" || decision.RequestedQty <= 0 {
			return fmt.Errorf("invalid_submission")
		}
	}
	if decision.Action == "NET_CARRY_BELOW_MINIMUM" || decision.Action == "SUBMIT_SPOT_TARGET_IOC" || decision.Action == "AT_TARGET" {
		if err := validateFundingCarryArithmetic(policy, decision); err != nil {
			return err
		}
	}
	if err := validateFundingCarryAction(policy, decision); err != nil {
		return err
	}
	return nil
}

func fundingCarryKnownAction(action string) bool {
	switch action {
	case "NOT_SUBSCRIBED", "POLICY_DISABLED", "REQUEST_PENDING", "SIMULATION_HORIZON_CENSORED",
		"POSITION_MISMATCH_OVERFLOW", "FUNDING_UNAVAILABLE", "FUNDING_IDENTITY_UNAVAILABLE",
		"FUNDING_PUBLICATION_FUTURE", "FUNDING_AGE_UNREPRESENTABLE", "FUNDING_STALE",
		"FUNDING_REFERENCE_UNAVAILABLE", "LOCAL_REFERENCE_UNAVAILABLE", "LOCAL_PRICE_OUTSIDE_DOMAIN",
		"PREMIUM_UNREPRESENTABLE", "ZERO_PREMIUM", "FUNDING_HORIZON_UNAVAILABLE",
		"NET_CARRY_BELOW_MINIMUM", "TARGET_GAP_UNREPRESENTABLE", "AT_TARGET",
		"ORPHAN_REPAIR_UNREPRESENTABLE", "ORPHAN_REPAIR_ZERO_QUANTITY",
		"PERP_EXECUTABLE_PRICE_UNAVAILABLE", "PERP_PRICE_OUTSIDE_DOMAIN",
		"PERP_EXECUTABLE_SIZE_UNAVAILABLE", "SPOT_TARGET_UNREPRESENTABLE",
		"SPOT_TARGET_ZERO_QUANTITY", "SPOT_EXECUTABLE_PRICE_UNAVAILABLE",
		"SPOT_PRICE_OUTSIDE_DOMAIN", "SPOT_EXECUTABLE_SIZE_UNAVAILABLE",
		"SUBMIT_SPOT_TARGET_IOC", "SUBMIT_PERP_ORPHAN_REPAIR_IOC":
		return true
	default:
		return false
	}
}

func validateFundingCarryAction(policy fundingCarryPolicyConfig, decision fundingCarryDecision) error {
	switch decision.Action {
	case "NET_CARRY_BELOW_MINIMUM":
		if decision.RequestID != 0 || decision.DesiredSpotPosition != 0 || decision.DesiredPerpPosition != 0 {
			return fmt.Errorf("net_carry_defer_target_mismatch")
		}
	case "AT_TARGET", "SUBMIT_SPOT_TARGET_IOC":
		direction := int64(1)
		if decision.PremiumBps < 0 {
			direction = -1
		}
		desiredSpot := policy.MaxPosition * direction
		if decision.DesiredSpotPosition != desiredSpot || decision.DesiredPerpPosition != -desiredSpot {
			return fmt.Errorf("target_position_mismatch")
		}
		if decision.Action == "AT_TARGET" {
			if decision.SpotPosition != desiredSpot || decision.PerpPosition != -desiredSpot {
				return fmt.Errorf("at_target_position_mismatch")
			}
			return nil
		}
		return validateFundingCarrySpotSubmission(policy, decision, desiredSpot)
	case "SUBMIT_PERP_ORPHAN_REPAIR_IOC":
		return validateFundingCarryPerpRepair(policy, decision)
	}
	return nil
}

func validateFundingCarrySpotSubmission(policy fundingCarryPolicyConfig, decision fundingCarryDecision, desiredSpot int64) error {
	gap, ok := fundingCarryAuditSub(desiredSpot, decision.SpotPosition)
	if !ok || gap == 0 {
		return fmt.Errorf("spot_target_gap_mismatch")
	}
	if decision.Leg != "SPOT_TARGET_ADJUSTMENT" {
		return fmt.Errorf("spot_target_leg_mismatch")
	}
	var price, available int64
	wantSide := exchange.Buy.String()
	if gap > 0 {
		price, available = decision.SpotAsk, decision.SpotAskQty
	} else {
		price, available, wantSide = decision.SpotBid, decision.SpotBidQty, exchange.Sell.String()
	}
	wantQty, ok := fundingCarryAuditSizedQty(gap, policy.LotQty, available, policy.MinOrderSize)
	if !ok || decision.Side != wantSide || decision.LimitPrice != price || decision.RequestedQty != wantQty || !fundingCarryAuditPositiveGrid(price, policy.SpotTick) {
		return fmt.Errorf("spot_submission_mismatch")
	}
	return nil
}

func validateFundingCarryPerpRepair(policy fundingCarryPolicyConfig, decision fundingCarryDecision) error {
	mismatch, ok := fundingCarryAuditAdd(decision.SpotPosition, decision.PerpPosition)
	if !ok || mismatch == 0 {
		return fmt.Errorf("orphan_mismatch_invalid")
	}
	if decision.Leg != "PERP_ORPHAN_REPAIR" || decision.DesiredSpotPosition != decision.SpotPosition || decision.DesiredPerpPosition != -decision.SpotPosition {
		return fmt.Errorf("orphan_target_mismatch")
	}
	var price, available int64
	wantSide := exchange.Buy.String()
	if mismatch > 0 {
		price, available, wantSide = decision.PerpBid, decision.PerpBidQty, exchange.Sell.String()
	} else {
		price, available = decision.PerpAsk, decision.PerpAskQty
	}
	wantQty, ok := fundingCarryAuditSizedQty(mismatch, policy.LotQty, available, policy.MinOrderSize)
	if !ok || decision.Side != wantSide || decision.LimitPrice != price || decision.RequestedQty != wantQty || !fundingCarryAuditPositiveGrid(price, policy.PerpTick) {
		return fmt.Errorf("orphan_submission_mismatch")
	}
	return nil
}

func validateFundingCarryArithmetic(policy fundingCarryPolicyConfig, decision fundingCarryDecision) error {
	if !decision.HasFunding || !decision.HasSpotBook || !decision.HasPerpBook || !decision.HasSpotBid || !decision.HasSpotAsk || !decision.HasPerpBid || !decision.HasPerpAsk {
		return fmt.Errorf("arithmetic_missing_input")
	}
	if decision.SpotBid <= 0 || decision.SpotAsk <= 0 || decision.PerpBid <= 0 || decision.PerpAsk <= 0 || decision.SpotBid > decision.SpotAsk || decision.PerpBid > decision.PerpAsk {
		return fmt.Errorf("arithmetic_price_domain")
	}
	spotMid := signedMidpoint(decision.SpotBid, decision.SpotAsk)
	perpMid := signedMidpoint(decision.PerpBid, decision.PerpAsk)
	premium, ok := fundingCarryAuditBasisBps(perpMid, spotMid)
	if !ok || spotMid != decision.SpotMid || perpMid != decision.PerpMid || premium != decision.PremiumBps {
		return fmt.Errorf("arithmetic_premium_mismatch")
	}
	if decision.FundingPublishedAt > decision.DecisionTime {
		return fmt.Errorf("future_receipt")
	}
	age := decision.DecisionTime - decision.FundingPublishedAt
	if age != decision.FundingAgeNanos || age > policy.MaxFundingAge || decision.FundingNextAt <= decision.DecisionTime || decision.FundingIntervalSeconds <= 0 {
		return fmt.Errorf("arithmetic_funding_time_mismatch")
	}
	direction := int64(1)
	if premium < 0 {
		direction = -1
	}
	if premium == 0 {
		return fmt.Errorf("arithmetic_zero_premium")
	}
	holding, income, fees, borrow, net, ok := fundingCarryAuditFinancials(policy, decision, direction)
	if !ok || holding != decision.HoldingNanos || income != decision.FundingIncomeBps || fees != decision.TakerFeeCostBps || borrow != decision.BorrowCostBps || decision.BalanceSheetCostBps != policy.BalanceSheetBps || decision.MarginRiskCostBps != policy.MarginRiskBps || decision.LegRiskCostBps != policy.LegRiskBps || net != decision.NetCarryBps {
		return fmt.Errorf("arithmetic_financial_mismatch")
	}
	if income <= 0 && fundingCarrySubmission(decision.Action) {
		return fmt.Errorf("funding_sign_mismatch")
	}
	if decision.Action == "NET_CARRY_BELOW_MINIMUM" && net >= policy.MinNetCarryBps {
		return fmt.Errorf("net_carry_defer_mismatch")
	}
	if fundingCarrySubmission(decision.Action) && net < policy.MinNetCarryBps {
		return fmt.Errorf("net_carry_submission_mismatch")
	}
	return nil
}

func fundingCarryAuditFinancials(policy fundingCarryPolicyConfig, decision fundingCarryDecision, direction int64) (int64, int64, int64, int64, int64, bool) {
	holding := new(big.Int).Sub(big.NewInt(decision.FundingNextAt), big.NewInt(decision.DecisionTime))
	extra := new(big.Int).Mul(big.NewInt(policy.FundingHorizon-1), big.NewInt(decision.FundingIntervalSeconds))
	extra.Mul(extra, big.NewInt(1_000_000_000))
	holding.Add(holding, extra)
	income := new(big.Int).Mul(big.NewInt(decision.FundingRateBps), big.NewInt(policy.FundingHorizon))
	income.Mul(income, big.NewInt(direction))
	fees := new(big.Int).Mul(big.NewInt(policy.TakerFeeBps), big.NewInt(4))
	borrow := new(big.Int).Mul(big.NewInt(policy.BorrowAnnualBps), holding)
	borrow.Quo(borrow, big.NewInt(365*24*60*60*1_000_000_000))
	net := new(big.Int).Sub(income, fees)
	net.Sub(net, borrow)
	net.Sub(net, big.NewInt(policy.BalanceSheetBps))
	net.Sub(net, big.NewInt(policy.MarginRiskBps))
	net.Sub(net, big.NewInt(policy.LegRiskBps))
	if holding.Sign() <= 0 || !holding.IsInt64() || !income.IsInt64() || !fees.IsInt64() || !borrow.IsInt64() || !net.IsInt64() {
		return 0, 0, 0, 0, 0, false
	}
	return holding.Int64(), income.Int64(), fees.Int64(), borrow.Int64(), net.Int64(), true
}

func fundingCarryAuditBasisBps(perp, spot int64) (int64, bool) {
	if spot <= 0 || perp <= 0 {
		return 0, false
	}
	numerator := new(big.Int).Sub(big.NewInt(perp), big.NewInt(spot))
	numerator.Mul(numerator, big.NewInt(10_000))
	numerator.Quo(numerator, big.NewInt(spot))
	if !numerator.IsInt64() {
		return 0, false
	}
	return numerator.Int64(), true
}

func fundingCarryAuditAdd(left, right int64) (int64, bool) {
	value := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func fundingCarryAuditSub(left, right int64) (int64, bool) {
	value := new(big.Int).Sub(big.NewInt(left), big.NewInt(right))
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func fundingCarryAuditMagnitude(value int64) (int64, bool) {
	if value >= 0 {
		return value, true
	}
	if value == -value {
		return 0, false
	}
	return -value, true
}

func fundingCarryAuditSizedQty(gap, lot, available, minimum int64) (int64, bool) {
	quantity, ok := fundingCarryAuditMagnitude(gap)
	if !ok || quantity == 0 || lot <= 0 {
		return 0, false
	}
	if quantity > lot {
		quantity = lot
	}
	if available > 0 && quantity > available {
		quantity = available
	}
	if quantity <= 0 || minimum > 0 && quantity < minimum {
		return 0, false
	}
	return quantity, true
}

func fundingCarryAuditPositiveGrid(price, tick int64) bool {
	return price > 0 && tick > 0 && price%tick == 0
}

func signedMidpoint(left, right int64) int64 {
	value := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	value.Quo(value, big.NewInt(2))
	return value.Int64()
}

func fundingCarrySubmission(action string) bool {
	return action == "SUBMIT_SPOT_TARGET_IOC" || action == "SUBMIT_PERP_ORPHAN_REPAIR_IOC"
}

func fundingCarryGatewayMatches(decision fundingCarryDecision, gateway fundingCarryGatewayDecision) bool {
	return gateway.requestID == decision.RequestID && gateway.decisionAt == decision.DecisionTime && gateway.price == decision.LimitPrice && gateway.qty == decision.RequestedQty && gateway.side == uint8(exchangeSide(decision.Side)) && gateway.orderType == uint8(exchange.LimitOrder) && gateway.tif == uint8(exchange.IOC)
}

func fundingCarryVenueOrderMatches(decision fundingCarryDecision, order fundingCarryVenueOrder) bool {
	return order.Symbol == mapFundingCarryAuditSymbol(decision) && order.Side == decision.Side && order.Type == exchange.LimitOrder.String() && order.TimeInForce == exchange.IOC.String() && order.Price == decision.LimitPrice && order.Qty == decision.RequestedQty
}

func mapFundingCarryAuditSymbol(decision fundingCarryDecision) string {
	if decision.Leg == "PERP_ORPHAN_REPAIR" {
		return decision.PerpSymbol
	}
	return decision.SpotSymbol
}
func exchangeSide(side string) exchange.Side {
	if side == exchange.Sell.String() {
		return exchange.Sell
	}
	return exchange.Buy
}

func fundingCarryHasActorOutcome(outcomes []fundingCarryOutcome, event string, orderID uint64) bool {
	for _, outcome := range outcomes {
		if outcome.Event == event && outcome.OrderID == orderID {
			return true
		}
	}
	return false
}
func fundingCarryHasActorRejected(outcomes []fundingCarryOutcome, reason string) bool {
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_REJECTED" && outcome.RejectReason == reason {
			return true
		}
	}
	return false
}
func fundingCarryHasActorFill(outcomes []fundingCarryOutcome, fill fundingCarryVenueFill) bool {
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_FILL" && outcome.OrderID == fill.OrderID && outcome.TradeID == fill.TradeID && outcome.Symbol == fill.Symbol && outcome.Side == fill.Side && outcome.Qty == fill.Qty && outcome.Price == fill.Price && outcome.FeeAmount == fill.FeeAmount && outcome.FeeAsset == fill.FeeAsset {
			return true
		}
	}
	return false
}
func isFundingCarryReceiptFailure(err error) bool { return stringsHasPrefix(err.Error(), "receipt_") }
func isFundingCarryFutureFailure(err error) bool  { return err.Error() == "future_receipt" }
func isFundingCarryArithmeticFailure(err error) bool {
	return stringsHasPrefix(err.Error(), "arithmetic_")
}
func isFundingCarrySignFailure(err error) bool { return err.Error() == "funding_sign_mismatch" }
func stringsHasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}
