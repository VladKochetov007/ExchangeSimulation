package analysis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"

	"exchange_sim/exchange"
)

// TermCarryAudit independently checks the P3 local-source, exact-term-cost,
// and ordinary order-chain contract. It deliberately imports neither the P3
// actor nor the multivenue implementation.
type TermCarryAudit struct {
	Decisions                 int64            `json:"decisions"`
	Submitted                 int64            `json:"submitted"`
	Deferred                  int64            `json:"deferred"`
	Accepted                  int64            `json:"accepted"`
	Rejected                  int64            `json:"rejected"`
	Fills                     int64            `json:"fills"`
	Cancelled                 int64            `json:"cancelled"`
	ReceiptAuditValid         bool             `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors     int64            `json:"receipt_evidence_errors"`
	SourceMismatches          int64            `json:"source_mismatches"`
	FutureSourceUse           int64            `json:"future_source_use"`
	InvalidDecisionRecords    int64            `json:"invalid_decision_records"`
	DecisionFieldMismatches   int64            `json:"decision_field_mismatches"`
	ArithmeticMismatches      int64            `json:"arithmetic_mismatches"`
	MissingGatewayDecisions   int64            `json:"missing_gateway_decisions"`
	GatewayDecisionMismatches int64            `json:"gateway_decision_mismatches"`
	MissingVenueOutcomes      int64            `json:"missing_venue_outcomes"`
	DuplicateVenueOutcomes    int64            `json:"duplicate_venue_outcomes"`
	MissingActorOutcomes      int64            `json:"missing_actor_outcomes"`
	ActorOutcomeMismatches    int64            `json:"actor_outcome_mismatches"`
	ActionCounts              map[string]int64 `json:"action_counts"`
	Checks                    []TermCarryCheck `json:"checks,omitempty"`
	Valid                     bool             `json:"valid"`
}

// TermCarryCheck names one independently detected evidence failure.
type TermCarryCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id"`
	Failure   string `json:"failure"`
}

type termCarryPolicyConfig struct {
	Enabled             bool   `json:"enabled"`
	SpotSymbol          string `json:"spot_symbol"`
	PerpSymbol          string `json:"perp_symbol"`
	DecisionPeriod      int64  `json:"decision_period"`
	CommitmentIntervals int64  `json:"commitment_intervals"`
	MaxFundingAge       int64  `json:"max_funding_age"`
	TakerFeeBps         int64  `json:"taker_fee_bps"`
	LongSpotFundingBps  int64  `json:"long_spot_funding_bps"`
	ShortSpotBorrowBps  int64  `json:"short_spot_borrow_bps"`
	BalanceSheetBps     int64  `json:"balance_sheet_bps"`
	MarginRiskBps       int64  `json:"margin_risk_bps"`
	LegRiskBps          int64  `json:"leg_risk_bps"`
	MinNetCarryBps      int64  `json:"min_net_carry_bps"`
	MaxPosition         int64  `json:"max_position"`
	LotQty              int64  `json:"lot_qty"`
	MinOrderSize        int64  `json:"min_order_size"`
	SpotTick            int64  `json:"spot_tick"`
	PerpTick            int64  `json:"perp_tick"`
}

type termCarryManifest struct {
	Config struct {
		TakerFeeBps        int64                  `json:"taker_fee_bps"`
		TermCarryAllocator *termCarryPolicyConfig `json:"term_carry_allocator"`
	} `json:"config"`
}

type termCarryDecision struct {
	VenueID                     string `json:"venue_id"`
	Desk                        string `json:"desk"`
	ClientID                    uint64 `json:"client_id"`
	PolicyVersion               string `json:"policy_version"`
	DecisionTime                int64  `json:"decision_time"`
	Enabled                     bool   `json:"enabled"`
	Subscribed                  bool   `json:"subscribed"`
	Pending                     bool   `json:"pending"`
	State                       string `json:"state"`
	Action                      string `json:"action_or_defer_reason"`
	SpotSymbol                  string `json:"spot_symbol"`
	PerpSymbol                  string `json:"perp_symbol"`
	SpotPosition                int64  `json:"spot_position"`
	PerpPosition                int64  `json:"perp_position"`
	TargetSpot                  int64  `json:"target_spot_position"`
	TargetPerp                  int64  `json:"target_perp_position"`
	EntryAt                     int64  `json:"entry_at"`
	TermEnd                     int64  `json:"term_end"`
	CommitmentIntervals         int64  `json:"commitment_intervals"`
	HasSpotBook                 bool   `json:"has_spot_book"`
	SpotPublishedAt             int64  `json:"spot_published_at"`
	SpotSequence                uint64 `json:"spot_sequence"`
	HasSpotBid                  bool   `json:"has_spot_bid"`
	SpotBid                     int64  `json:"spot_bid"`
	SpotBidQty                  int64  `json:"spot_bid_qty"`
	HasSpotAsk                  bool   `json:"has_spot_ask"`
	SpotAsk                     int64  `json:"spot_ask"`
	SpotAskQty                  int64  `json:"spot_ask_qty"`
	HasPerpBook                 bool   `json:"has_perp_book"`
	PerpPublishedAt             int64  `json:"perp_published_at"`
	PerpSequence                uint64 `json:"perp_sequence"`
	HasPerpBid                  bool   `json:"has_perp_bid"`
	PerpBid                     int64  `json:"perp_bid"`
	PerpBidQty                  int64  `json:"perp_bid_qty"`
	HasPerpAsk                  bool   `json:"has_perp_ask"`
	PerpAsk                     int64  `json:"perp_ask"`
	PerpAskQty                  int64  `json:"perp_ask_qty"`
	HasFunding                  bool   `json:"has_funding"`
	FundingRateBps              int64  `json:"funding_rate_bps"`
	FundingPublishedAt          int64  `json:"funding_published_at"`
	FundingSequence             uint64 `json:"funding_sequence"`
	FundingNextAt               int64  `json:"funding_next_at"`
	FundingIntervalSeconds      int64  `json:"funding_interval_seconds"`
	FundingAgeNanos             int64  `json:"funding_age_nanos"`
	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`
	ExpectedFundingBps          string `json:"expected_funding_bps"`
	ExecutionFeeBps             string `json:"execution_fee_bps"`
	FinancingBpsNumerator       string `json:"financing_bps_numerator"`
	NetCarryBpsNumerator        string `json:"net_carry_bps_numerator"`
	RationalDenominator         string `json:"rational_denominator"`
	FinancingDirection          string `json:"financing_direction"`
	Leg                         string `json:"leg"`
	Side                        string `json:"side"`
	LimitPrice                  int64  `json:"limit_price"`
	RequestedQty                int64  `json:"requested_qty"`
	RequestID                   uint64 `json:"request_id"`
}

type termCarryOutcome struct {
	VenueID      string `json:"venue_id"`
	ClientID     uint64 `json:"client_id"`
	Event        string `json:"event"`
	RequestID    uint64 `json:"request_id"`
	OrderID      uint64 `json:"order_id"`
	TradeID      uint64 `json:"trade_id"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	Qty          int64  `json:"qty"`
	Price        int64  `json:"price"`
	FeeAmount    int64  `json:"fee_amount"`
	FeeAsset     string `json:"fee_asset"`
	RemainingQty int64  `json:"remaining_qty"`
	RejectReason string `json:"reject_reason"`
}

// MeasureTermCarry replays only persisted evidence. A pass means the declared
// local inputs, exact entry economics, and submitted-order chain are
// reconstructible; it is not a funding or basis conclusion.
func (r *Run) MeasureTermCarry() (*TermCarryAudit, error) {
	policy, err := loadTermCarryPolicy(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validTermCarryPolicy(policy); err != nil {
		return nil, err
	}
	sources, frontiers, gateways, receiptAudit, receiptErr := fundingCarryReceipts(r.Dir)
	result := &TermCarryAudit{ActionCounts: make(map[string]int64)}
	check := func(venue string, client, request uint64, failure string) {
		result.Checks = append(result.Checks, TermCarryCheck{VenueID: venue, ClientID: client, RequestID: request, Failure: failure})
	}
	if receiptErr != nil || receiptAudit == nil || !receiptAudit.Valid {
		result.ReceiptEvidenceErrors++
	} else {
		result.ReceiptAuditValid = true
	}
	var decisions []termCarryDecision
	var outcomes []termCarryOutcome
	accepted := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	rejected := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	fills := make(map[fundingCarryOrderKey][]fundingCarryVenueFill)
	cancels := make(map[fundingCarryOrderKey]int)
	err = r.Scan(ScanOptions{Events: []string{"term_carry_decision", "term_carry_leg_outcome", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "term_carry_decision":
			var decision termCarryDecision
			if event.Decode(&decision) != nil || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID || r.Role(event.VenueID, event.ClientID) != "term_carry_allocator" {
				result.InvalidDecisionRecords++
				check(event.VenueID, event.ClientID, 0, "invalid_decision_record")
				return
			}
			decisions = append(decisions, decision)
		case "term_carry_leg_outcome":
			var outcome termCarryOutcome
			if event.Decode(&outcome) != nil || outcome.VenueID != event.VenueID || outcome.ClientID != event.ClientID {
				result.ActorOutcomeMismatches++
				check(event.VenueID, event.ClientID, 0, "invalid_actor_outcome")
				return
			}
			outcomes = append(outcomes, outcome)
		case "OrderAccepted", "OrderRejected":
			var order fundingCarryVenueOrder
			if event.Decode(&order) != nil || order.RequestID == 0 || r.Role(event.VenueID, event.ClientID) != "term_carry_allocator" {
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
			if event.Decode(&fill) != nil || fill.OrderID == 0 || r.Role(event.VenueID, event.ClientID) != "term_carry_allocator" {
				return
			}
			fill.VenueID, fill.ClientID = event.VenueID, event.ClientID
			fills[fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}] = append(fills[fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}], fill)
		case "OrderCancelled":
			var cancellation struct {
				OrderID uint64 `json:"order_id"`
			}
			if event.Decode(&cancellation) == nil && cancellation.OrderID != 0 && r.Role(event.VenueID, event.ClientID) == "term_carry_allocator" {
				cancels[fundingCarryOrderKey{event.VenueID, event.ClientID, cancellation.OrderID}]++
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("term carry audit: scan: %w", err)
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
		result.InvalidDecisionRecords++
		check("", 0, 0, "missing_term_carry_decisions")
	}
	byRequest := make(map[fundingCarryKey][]termCarryOutcome)
	for _, outcome := range outcomes {
		byRequest[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}] = append(byRequest[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}], outcome)
	}
	seen := make(map[fundingCarryKey]struct{})
	for _, decision := range decisions {
		result.Decisions++
		result.ActionCounts[decision.Action]++
		if err := validateTermCarryDecision(policy, decision, sources, frontiers); err != nil {
			classifyTermCarryFailure(result, err)
			check(decision.VenueID, decision.ClientID, decision.RequestID, err.Error())
		}
		if !termCarrySubmission(decision.Action) {
			result.Deferred++
			continue
		}
		result.Submitted++
		key := fundingCarryKey{decision.VenueID, decision.ClientID, decision.RequestID}
		if _, duplicate := seen[key]; duplicate {
			result.DecisionFieldMismatches++
			check(decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_submitted_decision")
		}
		seen[key] = struct{}{}
		gateway, found := gateways[key]
		if !found {
			result.MissingGatewayDecisions++
			check(decision.VenueID, decision.ClientID, decision.RequestID, "missing_gateway_decision")
		} else if !termCarryGatewayMatches(decision, gateway) {
			result.GatewayDecisionMismatches++
			check(decision.VenueID, decision.ClientID, decision.RequestID, "gateway_decision_mismatch")
		}
		acceptRows, rejectRows := accepted[key], rejected[key]
		if len(acceptRows)+len(rejectRows) == 0 {
			result.MissingVenueOutcomes++
			check(decision.VenueID, decision.ClientID, decision.RequestID, "missing_venue_outcome")
		} else if len(acceptRows)+len(rejectRows) != 1 {
			result.DuplicateVenueOutcomes++
			check(decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_venue_outcome")
		} else if len(acceptRows) == 1 {
			result.Accepted++
			if !termCarryVenueOrderMatches(decision, acceptRows[0]) {
				result.GatewayDecisionMismatches++
				check(decision.VenueID, decision.ClientID, decision.RequestID, "accepted_order_mismatch")
			}
			if !termCarryHasOutcome(byRequest[key], "ORDER_ACCEPTED", acceptRows[0].OrderID) {
				result.MissingActorOutcomes++
				check(decision.VenueID, decision.ClientID, decision.RequestID, "missing_actor_acceptance")
			}
			for _, fill := range fills[fundingCarryOrderKey{key.venue, key.client, acceptRows[0].OrderID}] {
				result.Fills++
				if !termCarryHasFill(byRequest[key], fill) {
					result.ActorOutcomeMismatches++
					check(decision.VenueID, decision.ClientID, decision.RequestID, "actor_fill_mismatch")
				}
			}
			if cancels[fundingCarryOrderKey{key.venue, key.client, acceptRows[0].OrderID}] > 0 {
				result.Cancelled++
				if !termCarryHasOutcome(byRequest[key], "ORDER_CANCELLED", acceptRows[0].OrderID) {
					result.MissingActorOutcomes++
					check(decision.VenueID, decision.ClientID, decision.RequestID, "missing_actor_cancellation")
				}
			}
		} else {
			result.Rejected++
			if !termCarryVenueOrderMatches(decision, rejectRows[0]) {
				result.GatewayDecisionMismatches++
				check(decision.VenueID, decision.ClientID, decision.RequestID, "rejected_order_mismatch")
			}
			if !termCarryHasRejected(byRequest[key], rejectRows[0].Error) {
				result.MissingActorOutcomes++
				check(decision.VenueID, decision.ClientID, decision.RequestID, "missing_actor_rejection")
			}
		}
	}
	result.Valid = result.ReceiptAuditValid && result.ReceiptEvidenceErrors == 0 && result.SourceMismatches == 0 && result.FutureSourceUse == 0 && result.InvalidDecisionRecords == 0 && result.DecisionFieldMismatches == 0 && result.ArithmeticMismatches == 0 && result.MissingGatewayDecisions == 0 && result.GatewayDecisionMismatches == 0 && result.MissingVenueOutcomes == 0 && result.DuplicateVenueOutcomes == 0 && result.MissingActorOutcomes == 0 && result.ActorOutcomeMismatches == 0
	return result, nil
}

func loadTermCarryPolicy(dir string) (termCarryPolicyConfig, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return termCarryPolicyConfig{}, fmt.Errorf("read term-carry manifest: %w", err)
	}
	var manifest termCarryManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return termCarryPolicyConfig{}, fmt.Errorf("decode term-carry manifest: %w", err)
	}
	if manifest.Config.TermCarryAllocator == nil {
		return termCarryPolicyConfig{}, fmt.Errorf("term-carry policy missing from manifest")
	}
	policy := *manifest.Config.TermCarryAllocator
	if policy.TakerFeeBps != manifest.Config.TakerFeeBps {
		return termCarryPolicyConfig{}, fmt.Errorf("term-carry policy/exchange fee mismatch")
	}
	return policy, nil
}

func validTermCarryPolicy(policy termCarryPolicyConfig) error {
	if policy.SpotSymbol != "ABC/USD" || policy.PerpSymbol != "ABC-PERP" || policy.DecisionPeriod <= 0 || policy.CommitmentIntervals <= 0 || policy.MaxFundingAge <= 0 || policy.MaxPosition <= 0 || policy.LotQty <= 0 || policy.MinOrderSize < 0 || policy.SpotTick <= 0 || policy.PerpTick <= 0 {
		return fmt.Errorf("invalid term-carry policy manifest")
	}
	for _, value := range []int64{policy.TakerFeeBps, policy.LongSpotFundingBps, policy.ShortSpotBorrowBps, policy.BalanceSheetBps, policy.MarginRiskBps, policy.LegRiskBps, policy.MinNetCarryBps} {
		if value < 0 {
			return fmt.Errorf("negative term-carry bps policy")
		}
	}
	return nil
}

func validateTermCarryDecision(policy termCarryPolicyConfig, decision termCarryDecision, sources map[fundingCarrySourceKey][]observationRecord, frontiers map[fundingCarryReceiptKey]auditedFrontier) error {
	if decision.VenueID == "" || decision.ClientID == 0 || decision.Desk == "" || decision.PolicyVersion != "v2_5_p3_term_carry_v1" || decision.DecisionTime == 0 || decision.Action == "" || decision.SpotSymbol != policy.SpotSymbol || decision.PerpSymbol != policy.PerpSymbol || decision.CommitmentIntervals != policy.CommitmentIntervals || !termCarryKnownAction(decision.Action) {
		return fmt.Errorf("invalid_decision_fields")
	}
	if termCarrySubmission(decision.Action) && (decision.RequestID == 0 || decision.Leg == "" || decision.Side == "" || decision.RequestedQty <= 0) {
		return fmt.Errorf("invalid_submission")
	}
	if !termCarrySubmission(decision.Action) && decision.RequestID != 0 {
		return fmt.Errorf("deferred_action_has_request_id")
	}
	frontierDecision := fundingCarryDecision{ClientID: decision.ClientID, DecisionTime: decision.DecisionTime, DecisionFrontierLinkID: decision.DecisionFrontierLinkID, DecisionFrontierOrdinal: decision.DecisionFrontierOrdinal, DecisionFrontierDeliveredAt: decision.DecisionFrontierDeliveredAt, DecisionFrontierDigest: decision.DecisionFrontierDigest}
	if decision.DecisionFrontierOrdinal == 0 {
		if decision.DecisionFrontierLinkID == 0 || decision.DecisionFrontierDeliveredAt != 0 || decision.DecisionFrontierDigest != "00000000000000000000000000000000" || decision.HasSpotBook || decision.HasPerpBook || decision.HasFunding {
			return fmt.Errorf("receipt_frontier_mismatch")
		}
	} else if err := fundingCarrySourceInFrontier(frontierDecision, sources, exchange.MDSnapshot, decision.SpotSequence, decision.SpotPublishedAt, frontiers); decision.HasSpotBook && err != nil {
		return err
	}
	if decision.HasPerpBook {
		if err := fundingCarrySourceInFrontier(frontierDecision, sources, exchange.MDSnapshot, decision.PerpSequence, decision.PerpPublishedAt, frontiers); err != nil {
			return err
		}
	}
	if decision.HasFunding {
		if err := fundingCarrySourceInFrontier(frontierDecision, sources, exchange.MDFunding, decision.FundingSequence, decision.FundingPublishedAt, frontiers); err != nil {
			return err
		}
	}
	if decision.Action == "NET_CARRY_BELOW_MINIMUM" || decision.Action == "SUBMIT_ENTRY_SPOT_IOC" {
		return validateTermCarryEntryEconomics(policy, decision)
	}
	if termCarrySubmission(decision.Action) {
		return validateTermCarrySubmission(policy, decision)
	}
	return nil
}

func validateTermCarryEntryEconomics(policy termCarryPolicyConfig, decision termCarryDecision) error {
	if !decision.HasSpotBook || !decision.HasPerpBook || !decision.HasFunding || !decision.HasSpotBid || !decision.HasSpotAsk || !decision.HasPerpBid || !decision.HasPerpAsk || decision.FundingPublishedAt > decision.DecisionTime || decision.FundingNextAt <= decision.DecisionTime || decision.FundingIntervalSeconds <= 0 {
		return fmt.Errorf("arithmetic_missing_input")
	}
	if decision.SpotBid <= 0 || decision.SpotAsk <= 0 || decision.PerpBid <= 0 || decision.PerpAsk <= 0 || decision.SpotBid > decision.SpotAsk || decision.PerpBid > decision.PerpAsk {
		return fmt.Errorf("arithmetic_price_domain")
	}
	age := decision.DecisionTime - decision.FundingPublishedAt
	if age != decision.FundingAgeNanos || age > policy.MaxFundingAge {
		return fmt.Errorf("arithmetic_funding_age")
	}
	spot, perp := signedMidpoint(decision.SpotBid, decision.SpotAsk), signedMidpoint(decision.PerpBid, decision.PerpAsk)
	if spot == perp {
		return fmt.Errorf("arithmetic_zero_premium")
	}
	direction := int64(1)
	if perp < spot {
		direction = -1
	}
	financials, ok := termCarryAuditFinancials(policy, decision, direction)
	if !ok {
		return fmt.Errorf("arithmetic_financials")
	}
	if decision.TermEnd != financials.termEnd || decision.ExpectedFundingBps != financials.funding.String() || decision.ExecutionFeeBps != financials.fees.String() || decision.FinancingBpsNumerator != financials.financing.String() || decision.NetCarryBpsNumerator != financials.net.String() || decision.RationalDenominator != financials.denominator.String() || decision.FinancingDirection != financials.direction {
		return fmt.Errorf("arithmetic_financial_mismatch")
	}
	minimum := new(big.Int).Mul(big.NewInt(policy.MinNetCarryBps), financials.denominator)
	if decision.Action == "NET_CARRY_BELOW_MINIMUM" && financials.net.Cmp(minimum) >= 0 {
		return fmt.Errorf("arithmetic_defer_mismatch")
	}
	if decision.Action == "SUBMIT_ENTRY_SPOT_IOC" {
		if financials.net.Cmp(minimum) < 0 || decision.TargetSpot != direction*policy.MaxPosition || decision.TargetPerp != -decision.TargetSpot || decision.State != "ENTRY_SPOT" {
			return fmt.Errorf("arithmetic_entry_mismatch")
		}
	}
	return nil
}

type termCarryAuditFinancial struct {
	termEnd                                    int64
	funding, fees, financing, net, denominator *big.Int
	direction                                  string
}

func termCarryAuditFinancials(policy termCarryPolicyConfig, decision termCarryDecision, direction int64) (termCarryAuditFinancial, bool) {
	if direction == 0 || decision.FundingNextAt <= decision.DecisionTime || decision.FundingIntervalSeconds <= 0 {
		return termCarryAuditFinancial{}, false
	}
	end := new(big.Int).SetInt64(decision.FundingNextAt)
	extra := new(big.Int).Mul(big.NewInt(policy.CommitmentIntervals-1), big.NewInt(decision.FundingIntervalSeconds))
	extra.Mul(extra, big.NewInt(1_000_000_000))
	end.Add(end, extra)
	if !end.IsInt64() {
		return termCarryAuditFinancial{}, false
	}
	holding := new(big.Int).Sub(end, big.NewInt(decision.DecisionTime))
	if holding.Sign() <= 0 {
		return termCarryAuditFinancial{}, false
	}
	denominator := big.NewInt(365 * 24 * 60 * 60 * 1_000_000_000)
	funding := new(big.Int).Mul(big.NewInt(decision.FundingRateBps), big.NewInt(policy.CommitmentIntervals))
	funding.Mul(funding, big.NewInt(direction))
	fees := new(big.Int).Mul(big.NewInt(policy.TakerFeeBps), big.NewInt(4))
	financingRate, financingDirection := policy.LongSpotFundingBps, "LONG_SPOT_CASH_FINANCING"
	if direction < 0 {
		financingRate, financingDirection = policy.ShortSpotBorrowBps, "SHORT_SPOT_ASSET_BORROW"
	}
	financing := new(big.Int).Mul(big.NewInt(financingRate), holding)
	net := new(big.Int).Mul(funding, denominator)
	net.Sub(net, new(big.Int).Mul(fees, denominator))
	net.Sub(net, financing)
	fixed := new(big.Int).Add(big.NewInt(policy.BalanceSheetBps), big.NewInt(policy.MarginRiskBps))
	fixed.Add(fixed, big.NewInt(policy.LegRiskBps))
	net.Sub(net, new(big.Int).Mul(fixed, denominator))
	return termCarryAuditFinancial{termEnd: end.Int64(), funding: funding, fees: fees, financing: financing, net: net, denominator: denominator, direction: financingDirection}, true
}

func termCarryKnownAction(action string) bool {
	switch action {
	case "NOT_SUBSCRIBED", "POLICY_DISABLED", "REQUEST_PENDING", "FUNDING_UNAVAILABLE", "FUNDING_IDENTITY_UNAVAILABLE", "FUNDING_PUBLICATION_FUTURE", "FUNDING_AGE_UNREPRESENTABLE", "FUNDING_STALE", "FUNDING_REFERENCE_UNAVAILABLE", "LOCAL_REFERENCE_UNAVAILABLE", "LOCAL_PRICE_OUTSIDE_DOMAIN", "TERM_HORIZON_CENSORED", "ZERO_PREMIUM", "TERM_FINANCIALS_UNAVAILABLE", "NET_CARRY_BELOW_MINIMUM", "ENTRY_PLAN_UNAVAILABLE", "ENTRY_PERP_IOC", "TERM_ACTIVE", "UNWIND_PERP_IOC", "UNWIND_SPOT_IOC", "UNWIND_PRICE_UNAVAILABLE", "UNWIND_PRICE_OUTSIDE_DOMAIN", "TERM_CLOSED", "SPOT_TARGET_UNREPRESENTABLE", "PERP_TARGET_UNREPRESENTABLE", "PERP_GAP_UNREPRESENTABLE", "UNWIND_PERP_GAP_UNREPRESENTABLE", "UNWIND_SPOT_GAP_UNREPRESENTABLE", "ORDER_QUANTITY_UNREPRESENTABLE", "ORDER_ZERO_QUANTITY", "EXECUTABLE_SIZE_UNAVAILABLE", "SPOT_ENTRY_PRICE_UNAVAILABLE", "SPOT_ENTRY_PRICE_OUTSIDE_DOMAIN", "PERP_ENTRY_PRICE_UNAVAILABLE", "PERP_ENTRY_PRICE_OUTSIDE_DOMAIN", "UNKNOWN_LIFECYCLE_STATE", "ACTIVE_PLAN_UNAVAILABLE":
		return true
	default:
		return action == "SUBMIT_ENTRY_SPOT_IOC" || action == "SUBMIT_ENTRY_PERP_IOC" || action == "SUBMIT_UNWIND_PERP_IOC" || action == "SUBMIT_UNWIND_SPOT_IOC"
	}
}

func termCarrySubmission(action string) bool {
	return action == "SUBMIT_ENTRY_SPOT_IOC" || action == "SUBMIT_ENTRY_PERP_IOC" || action == "SUBMIT_UNWIND_PERP_IOC" || action == "SUBMIT_UNWIND_SPOT_IOC"
}

func validateTermCarrySubmission(policy termCarryPolicyConfig, decision termCarryDecision) error {
	var bookBid, bookAsk, available, tick int64
	perp := decision.Leg == "ENTRY_PERP_IOC" || decision.Leg == "UNWIND_PERP_IOC"
	if perp {
		bookBid, bookAsk, tick = decision.PerpBid, decision.PerpAsk, policy.PerpTick
	} else {
		bookBid, bookAsk, tick = decision.SpotBid, decision.SpotAsk, policy.SpotTick
	}
	gap := int64(0)
	switch decision.Leg {
	case "ENTRY_PERP_IOC":
		gap = -decision.SpotPosition - decision.PerpPosition
	case "UNWIND_PERP_IOC":
		gap = -decision.PerpPosition
	case "UNWIND_SPOT_IOC":
		gap = -decision.SpotPosition
	case "ENTRY_SPOT_IOC":
		gap = decision.TargetSpot - decision.SpotPosition
	default:
		return fmt.Errorf("submission_unknown_leg")
	}
	if gap == 0 {
		return fmt.Errorf("submission_zero_gap")
	}
	wantSide := exchange.Buy.String()
	price := bookAsk
	available = decision.PerpAskQty
	if !perp {
		available = decision.SpotAskQty
	}
	if gap < 0 {
		wantSide, price = exchange.Sell.String(), bookBid
		if perp {
			available = decision.PerpBidQty
		} else {
			available = decision.SpotBidQty
		}
	}
	wantQty, ok := fundingCarryAuditSizedQty(gap, policy.LotQty, available, policy.MinOrderSize)
	if !ok || decision.Side != wantSide || decision.LimitPrice != price || decision.RequestedQty != wantQty || !fundingCarryAuditPositiveGrid(price, tick) {
		return fmt.Errorf("submission_mismatch")
	}
	return nil
}

func termCarryGatewayMatches(decision termCarryDecision, gateway fundingCarryGatewayDecision) bool {
	return gateway.requestID == decision.RequestID && gateway.symbol == termCarryAuditSymbol(decision) && gateway.decisionAt == decision.DecisionTime && gateway.price == decision.LimitPrice && gateway.qty == decision.RequestedQty && gateway.side == uint8(exchangeSide(decision.Side)) && gateway.orderType == uint8(exchange.LimitOrder) && gateway.tif == uint8(exchange.IOC)
}
func termCarryVenueOrderMatches(decision termCarryDecision, order fundingCarryVenueOrder) bool {
	return order.Side == decision.Side && order.Type == exchange.LimitOrder.String() && order.TimeInForce == exchange.IOC.String() && order.Price == decision.LimitPrice && order.Qty == decision.RequestedQty
}
func termCarryAuditSymbol(decision termCarryDecision) string {
	if decision.Leg == "ENTRY_PERP_IOC" || decision.Leg == "UNWIND_PERP_IOC" {
		return decision.PerpSymbol
	}
	return decision.SpotSymbol
}
func termCarryHasOutcome(outcomes []termCarryOutcome, event string, orderID uint64) bool {
	for _, outcome := range outcomes {
		if outcome.Event == event && outcome.OrderID == orderID {
			return true
		}
	}
	return false
}
func termCarryHasRejected(outcomes []termCarryOutcome, reason string) bool {
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_REJECTED" && outcome.RejectReason == reason {
			return true
		}
	}
	return false
}
func termCarryHasFill(outcomes []termCarryOutcome, fill fundingCarryVenueFill) bool {
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_FILL" && outcome.OrderID == fill.OrderID && outcome.TradeID == fill.TradeID && outcome.Symbol == fill.Symbol && outcome.Side == fill.Side && outcome.Qty == fill.Qty && outcome.Price == fill.Price && outcome.FeeAmount == fill.FeeAmount && outcome.FeeAsset == fill.FeeAsset {
			return true
		}
	}
	return false
}

func classifyTermCarryFailure(result *TermCarryAudit, err error) {
	if err == nil {
		return
	}
	message := err.Error()
	if stringsHasPrefix(message, "source_") || stringsHasPrefix(message, "receipt_") {
		result.SourceMismatches++
	} else if message == "future_receipt" {
		result.FutureSourceUse++
	} else if stringsHasPrefix(message, "arithmetic_") {
		result.ArithmeticMismatches++
	} else {
		result.DecisionFieldMismatches++
	}
}
