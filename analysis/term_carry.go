package analysis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	LifecycleViolations       int64            `json:"lifecycle_violations"`
	PositionContinuityErrors  int64            `json:"position_continuity_errors"`
	TerminalPerpMismatches    int64            `json:"terminal_perp_mismatches"`
	TerminalSpotMismatches    int64            `json:"terminal_spot_mismatches"`
	FirstExposureMismatches   int64            `json:"first_exposure_mismatches"`
	ActiveTerms               int64            `json:"active_terms"`
	ClosedTerms               int64            `json:"closed_terms"`
	OpenTerms                 int64            `json:"open_terms"`
	ActiveTermFunding         int64            `json:"active_term_funding_settlements"`
	OutsideTermFunding        int64            `json:"outside_term_funding_settlements"`
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

const (
	termCarryPolicyV1 = "v2_5_p3_term_carry_v1"
	termCarryPolicyV2 = "v2_5_p3_term_carry_v2"
	termCarryPolicyV3 = "v2_5_p3_term_carry_v3"
)

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
	MandateEndAtNano    int64  `json:"mandate_end_at_nano"`
	MaxPosition         int64  `json:"max_position"`
	LotQty              int64  `json:"lot_qty"`
	MinOrderSize        int64  `json:"min_order_size"`
	UnwindMinOrderSize  *int64 `json:"unwind_min_order_size,omitempty"`
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
	VenueID         string `json:"venue_id"`
	Desk            string `json:"desk"`
	ClientID        uint64 `json:"client_id"`
	PolicyVersion   string `json:"policy_version"`
	DecisionTime    int64  `json:"decision_time"`
	Enabled         bool   `json:"enabled"`
	Subscribed      bool   `json:"subscribed"`
	Pending         bool   `json:"pending"`
	State           string `json:"state"`
	Action          string `json:"action_or_defer_reason"`
	SpotSymbol      string `json:"spot_symbol"`
	PerpSymbol      string `json:"perp_symbol"`
	SpotPosition    int64  `json:"spot_position"`
	PerpPosition    int64  `json:"perp_position"`
	TargetSpot      int64  `json:"target_spot_position"`
	TargetPerp      int64  `json:"target_perp_position"`
	PlanCreatedAt   int64  `json:"plan_created_at"`
	FirstExposureAt int64  `json:"first_exposure_at"`
	// EntryAt is the legacy v1 plan-creation timestamp. V2 replaces it with
	// explicit plan_created_at and first_exposure_at evidence.
	EntryAt                     int64  `json:"entry_at"`
	TermEnd                     int64  `json:"term_end"`
	MandateEndAt                int64  `json:"mandate_end_at"`
	CommitmentIntervals         int64  `json:"commitment_intervals"`
	UnwindMinOrderSize          *int64 `json:"unwind_min_order_size,omitempty"`
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
	VenueID            string `json:"venue_id"`
	Desk               string `json:"desk"`
	ClientID           uint64 `json:"client_id"`
	DecisionTime       int64  `json:"decision_time"`
	ExecutionTime      int64  `json:"execution_time"`
	State              string `json:"state"`
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

type termCarryFundingSettlement struct {
	VenueID  string
	ClientID uint64
	At       int64
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
	var settlements []termCarryFundingSettlement
	accepted := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	rejected := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	fills := make(map[fundingCarryOrderKey][]fundingCarryVenueFill)
	cancels := make(map[fundingCarryOrderKey]int)
	err = r.Scan(ScanOptions{Events: []string{"term_carry_decision", "term_carry_leg_outcome", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "balance_change"}, Workers: 1}, func(event Event) {
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
			if event.Decode(&outcome) != nil || outcome.VenueID != event.VenueID || outcome.ClientID != event.ClientID || outcome.Desk == "" || r.Role(event.VenueID, event.ClientID) != "term_carry_allocator" {
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
		case "balance_change":
			var balance balanceChangeRecord
			if event.Decode(&balance) != nil || balance.Symbol != policy.PerpSymbol || balance.Reason != "funding_settlement" || r.Role(event.VenueID, event.ClientID) != "term_carry_allocator" {
				return
			}
			at := balance.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			settlements = append(settlements, termCarryFundingSettlement{VenueID: event.VenueID, ClientID: event.ClientID, At: at})
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
	auditTermCarryLifecycle(r, policy, decisions, outcomes, settlements, result, check)
	result.Valid = result.ReceiptAuditValid && result.ReceiptEvidenceErrors == 0 && result.SourceMismatches == 0 && result.FutureSourceUse == 0 && result.InvalidDecisionRecords == 0 && result.DecisionFieldMismatches == 0 && result.ArithmeticMismatches == 0 && result.MissingGatewayDecisions == 0 && result.GatewayDecisionMismatches == 0 && result.MissingVenueOutcomes == 0 && result.DuplicateVenueOutcomes == 0 && result.MissingActorOutcomes == 0 && result.ActorOutcomeMismatches == 0 && result.LifecycleViolations == 0 && result.PositionContinuityErrors == 0 && result.TerminalPerpMismatches == 0 && result.TerminalSpotMismatches == 0 && result.FirstExposureMismatches == 0 && result.OutsideTermFunding == 0
	return result, nil
}

// termCarryParticipant is deliberately local to the replay rather than the
// simulator's actor identity. The analyzer must establish lifecycle facts from
// persisted evidence alone.
type termCarryParticipant struct {
	venue  string
	client uint64
}

type termCarryLifecycleTerm struct {
	owner           termCarryParticipant
	policyVersion   string
	planCreatedAt   int64
	firstExposureAt int64
	entryAt         int64 // v1 compatibility only; use firstExposureAt for v2.
	termEnd         int64
	activeAt        int64
	closedAt        int64
	aborted         bool
	active          bool
	closed          bool
}

type termCarryLifecycleItem struct {
	at       int64
	index    int
	decision *termCarryDecision
	outcome  *termCarryOutcome
}

// auditTermCarryLifecycle reconstructs the actor's declared finite ownership
// term from its decisions and actor-side response attestations. It never reads
// TermCarryAllocator. Open terms are reported, not called invalid here: P3a
// and P3b deliberately stop before the registered commitment horizon. P3c
// separately requires eventual closure.
func auditTermCarryLifecycle(run *Run, policy termCarryPolicyConfig, decisions []termCarryDecision, outcomes []termCarryOutcome, settlements []termCarryFundingSettlement, result *TermCarryAudit, check func(string, uint64, uint64, string)) {
	byParticipantDecisions := make(map[termCarryParticipant][]termCarryDecision)
	byParticipantOutcomes := make(map[termCarryParticipant][]termCarryOutcome)
	participants := make(map[termCarryParticipant]struct{})
	for _, decision := range decisions {
		key := termCarryParticipant{venue: decision.VenueID, client: decision.ClientID}
		byParticipantDecisions[key] = append(byParticipantDecisions[key], decision)
		participants[key] = struct{}{}
	}
	for _, outcome := range outcomes {
		key := termCarryParticipant{venue: outcome.VenueID, client: outcome.ClientID}
		byParticipantOutcomes[key] = append(byParticipantOutcomes[key], outcome)
		participants[key] = struct{}{}
	}

	baseAsset, ok := termCarryBaseAsset(policy.SpotSymbol)
	if !ok {
		result.LifecycleViolations++
		check("", 0, 0, "invalid_spot_symbol_base_asset")
		return
	}

	var terms []*termCarryLifecycleTerm
	terminalPerp := make(map[termCarryParticipant]int64)
	terminalFound := make(map[termCarryParticipant]bool)
	terminalSpot := make(map[termCarryParticipant]int64)
	terminalSpotFound := make(map[termCarryParticipant]bool)
	initialSpot := make(map[termCarryParticipant]int64)
	initialSpotFound := make(map[termCarryParticipant]bool)
	for _, row := range run.Report.InitialAccounts {
		key := termCarryParticipant{venue: row.VenueID, client: row.ClientID}
		if run.Role(row.VenueID, row.ClientID) != "term_carry_allocator" {
			continue
		}
		balance, found := termCarrySpotBalance(row.Account.SpotBalances, baseAsset)
		if found {
			initialSpot[key] = balance
			initialSpotFound[key] = true
		}
	}
	for _, row := range run.Report.TerminalAccounts {
		key := termCarryParticipant{venue: row.VenueID, client: row.ClientID}
		if run.Role(row.VenueID, row.ClientID) != "term_carry_allocator" {
			continue
		}
		terminalFound[key] = true
		balance, found := termCarrySpotBalance(row.Account.SpotBalances, baseAsset)
		if found {
			terminalSpot[key] = balance
			terminalSpotFound[key] = true
		}
		for _, position := range row.Account.Positions {
			if position.Symbol == policy.PerpSymbol {
				terminalPerp[key] = position.Size
			}
		}
	}

	participantKeys := make([]termCarryParticipant, 0, len(participants))
	for participant := range participants {
		participantKeys = append(participantKeys, participant)
	}
	sort.Slice(participantKeys, func(i, j int) bool {
		if participantKeys[i].venue != participantKeys[j].venue {
			return participantKeys[i].venue < participantKeys[j].venue
		}
		return participantKeys[i].client < participantKeys[j].client
	})
	for _, participant := range participantKeys {
		items := make([]termCarryLifecycleItem, 0, len(byParticipantDecisions[participant])+len(byParticipantOutcomes[participant]))
		submitted := make(map[uint64]termCarryDecision)
		for index := range byParticipantDecisions[participant] {
			decision := &byParticipantDecisions[participant][index]
			items = append(items, termCarryLifecycleItem{at: decision.DecisionTime, index: index, decision: decision})
			if termCarrySubmission(decision.Action) {
				submitted[decision.RequestID] = *decision
			}
		}
		for index := range byParticipantOutcomes[participant] {
			outcome := &byParticipantOutcomes[participant][index]
			at := outcome.DecisionTime
			if outcome.Event == "ORDER_FILL" {
				at = outcome.ExecutionTime
			}
			items = append(items, termCarryLifecycleItem{at: at, index: index, outcome: outcome})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].at != items[j].at {
				return items[i].at < items[j].at
			}
			// A decision is causally before its same-timestamp response. There is
			// no need to infer a cross-file global order for a single participant.
			if items[i].decision != nil || items[j].decision != nil {
				return items[i].decision != nil
			}
			return items[i].index < items[j].index
		})

		spotPosition, perpPosition := int64(0), int64(0)
		var current *termCarryLifecycleTerm
		for _, item := range items {
			if item.at <= 0 {
				result.LifecycleViolations++
				check(participant.venue, participant.client, 0, "nonpositive_lifecycle_timestamp")
				continue
			}
			if item.decision != nil {
				decision := item.decision
				if decision.SpotPosition != spotPosition || decision.PerpPosition != perpPosition {
					result.PositionContinuityErrors++
					check(participant.venue, participant.client, decision.RequestID, "decision_position_discontinuity")
				}
				if violation := validateTermCarryLifecycleDecision(policy, *decision, spotPosition, perpPosition, &current, &terms); violation != "" {
					result.LifecycleViolations++
					if violation == "first_exposure_mismatch" || violation == "first_exposure_without_plan" || violation == "first_exposure_before_plan" || violation == "active_term_without_first_exposure" {
						result.FirstExposureMismatches++
					}
					check(participant.venue, participant.client, decision.RequestID, violation)
				}
				continue
			}

			outcome := item.outcome
			if submittedDecision, found := submitted[outcome.RequestID]; !found || outcome.DecisionTime != submittedDecision.DecisionTime || outcome.State != submittedDecision.State || outcome.Leg != submittedDecision.Leg || outcome.Symbol != termCarryAuditSymbol(submittedDecision) {
				result.LifecycleViolations++
				check(participant.venue, participant.client, outcome.RequestID, "outcome_without_matching_submission")
			}
			spotPosition, perpPosition = applyTermCarryOutcome(policy, *outcome, spotPosition, perpPosition, result, check)
			if current != nil && termCarryUsesV2Lifecycle(current.policyVersion) && current.firstExposureAt == 0 && (spotPosition != 0 || perpPosition != 0) {
				if outcome.Event != "ORDER_FILL" || outcome.ExecutionTime < current.planCreatedAt {
					result.LifecycleViolations++
					result.FirstExposureMismatches++
					check(participant.venue, participant.client, outcome.RequestID, "first_exposure_mismatch")
				} else {
					current.firstExposureAt = outcome.ExecutionTime
				}
			}
			if current != nil && termCarryUsesV2Lifecycle(current.policyVersion) && current.firstExposureAt == 0 && spotPosition == 0 && perpPosition == 0 && (outcome.Event == "ORDER_REJECTED" || outcome.Event == "ORDER_CANCELLED") {
				current.aborted = true
				current = nil
			}
		}
		if current != nil {
			result.OpenTerms++
		}
		if !terminalFound[participant] || terminalPerp[participant] != perpPosition {
			result.TerminalPerpMismatches++
			check(participant.venue, participant.client, 0, "terminal_perp_position_mismatch")
		}
		spotDelta, validSpotDelta := fundingCarryAuditSub(terminalSpot[participant], initialSpot[participant])
		if !initialSpotFound[participant] || !terminalSpotFound[participant] || !validSpotDelta || spotDelta != spotPosition {
			result.TerminalSpotMismatches++
			check(participant.venue, participant.client, 0, "terminal_spot_balance_mismatch")
		}
	}

	for _, term := range terms {
		if term.active {
			result.ActiveTerms++
		}
		if term.closed {
			result.ClosedTerms++
		}
	}
	for _, settlement := range settlements {
		matches := 0
		for _, term := range terms {
			if term.owner != (termCarryParticipant{venue: settlement.VenueID, client: settlement.ClientID}) || !term.active {
				continue
			}
			if settlement.At >= term.activeAt && settlement.At <= term.termEnd && (term.closedAt == 0 || settlement.At <= term.closedAt) {
				matches++
			}
		}
		if matches == 1 {
			result.ActiveTermFunding++
			continue
		}
		result.OutsideTermFunding++
		check(settlement.VenueID, settlement.ClientID, 0, "funding_settlement_outside_active_term")
		if matches > 1 {
			result.LifecycleViolations++
			check(settlement.VenueID, settlement.ClientID, 0, "funding_settlement_matches_overlapping_terms")
		}
	}
}

// termCarryBaseAsset returns the spot asset whose terminal inventory must
// match the independently reconstructed term-carry spot position.
func termCarryBaseAsset(symbol string) (string, bool) {
	base, quote, found := strings.Cut(symbol, "/")
	return base, found && base != "" && quote != ""
}

// termCarrySpotBalance returns exactly one declared balance. Missing and
// duplicate balances are evidence failures, never implicit numeric zero.
func termCarrySpotBalance(balances []Balance, asset string) (int64, bool) {
	var value int64
	found := false
	for _, balance := range balances {
		if balance.Asset != asset {
			continue
		}
		if found {
			return 0, false
		}
		value, found = balance.NetAsset, true
	}
	return value, found
}

func validateTermCarryLifecycleDecision(policy termCarryPolicyConfig, decision termCarryDecision, spotPosition, perpPosition int64, current **termCarryLifecycleTerm, terms *[]*termCarryLifecycleTerm) string {
	if termCarryUsesV2Lifecycle(decision.PolicyVersion) {
		return validateTermCarryLifecycleDecisionV2(policy, decision, spotPosition, perpPosition, current, terms)
	}
	// A rejected economic candidate has a calculated TermEnd but no EntryAt:
	// it is a projection used to explain NET_CARRY_BELOW_MINIMUM, not an owned
	// term. Ownership begins only with the first submitted spot-entry request.
	// validateTermCarryEntryEconomics independently checks the projected end.
	if decision.EntryAt == 0 && decision.TermEnd != 0 && decision.Action != "NET_CARRY_BELOW_MINIMUM" {
		return "term_end_without_ownership_term"
	}
	hasTerm := decision.EntryAt != 0
	if hasTerm && decision.TermEnd <= decision.EntryAt {
		return "invalid_term_bounds"
	}
	if *current != nil && (decision.EntryAt != (*current).entryAt || decision.TermEnd != (*current).termEnd) {
		return "term_identity_changed_while_open"
	}
	if *current == nil && hasTerm && decision.Action != "SUBMIT_ENTRY_SPOT_IOC" {
		return "term_metadata_without_open_term"
	}
	switch decision.Action {
	case "SUBMIT_ENTRY_SPOT_IOC":
		if decision.State != "ENTRY_SPOT" && decision.State != "ENTRY_PERP" {
			return "entry_spot_wrong_state"
		}
		if decision.TargetSpot == 0 || decision.TargetSpot != -decision.TargetPerp || (decision.TargetSpot != policy.MaxPosition && decision.TargetSpot != -policy.MaxPosition) {
			return "entry_spot_invalid_targets"
		}
		if *current == nil {
			if decision.EntryAt != decision.DecisionTime {
				return "entry_spot_entry_time_mismatch"
			}
			*current = &termCarryLifecycleTerm{owner: termCarryParticipant{venue: decision.VenueID, client: decision.ClientID}, policyVersion: termCarryPolicyV1, entryAt: decision.EntryAt, termEnd: decision.TermEnd}
			*terms = append(*terms, *current)
		}
	case "SUBMIT_ENTRY_PERP_IOC":
		if *current == nil || decision.State != "ENTRY_PERP" {
			return "entry_perp_without_entry_state"
		}
	case "TERM_ACTIVE":
		if *current == nil || decision.State != "ACTIVE_TERM" || spotPosition == 0 || perpPosition != -spotPosition || decision.DecisionTime >= (*current).termEnd {
			return "active_term_state_mismatch"
		}
		if !(*current).active {
			(*current).active, (*current).activeAt = true, decision.DecisionTime
		}
	case "SUBMIT_UNWIND_PERP_IOC":
		if *current == nil || decision.State != "UNWIND_PERP" || decision.DecisionTime < (*current).termEnd || perpPosition == 0 {
			return "perp_unwind_state_mismatch"
		}
	case "SUBMIT_UNWIND_SPOT_IOC":
		if *current == nil || decision.State != "UNWIND_SPOT" || decision.DecisionTime < (*current).termEnd || perpPosition != 0 || spotPosition == 0 {
			return "spot_unwind_state_mismatch"
		}
	case "UNWIND_PRICE_UNAVAILABLE", "UNWIND_PRICE_OUTSIDE_DOMAIN", "UNWIND_PERP_GAP_UNREPRESENTABLE", "UNWIND_SPOT_GAP_UNREPRESENTABLE":
		if *current == nil || (decision.State != "UNWIND_PERP" && decision.State != "UNWIND_SPOT") || decision.DecisionTime < (*current).termEnd {
			return "unwind_defer_state_mismatch"
		}
	case "TERM_CLOSED":
		if *current == nil || decision.State != "IDLE" || decision.DecisionTime < (*current).termEnd || spotPosition != 0 || perpPosition != 0 {
			return "term_close_state_mismatch"
		}
		if (*current).closed {
			return "duplicate_term_close"
		}
		(*current).closed, (*current).closedAt = true, decision.DecisionTime
		*current = nil
	default:
		if *current == nil && decision.State != "IDLE" {
			return "nonidle_state_without_open_term"
		}
	}
	return ""
}

// validateTermCarryLifecycleDecisionV2 distinguishes an executable entry plan
// from the owned term. The latter begins only at the canonical first fill.
func validateTermCarryLifecycleDecisionV2(policy termCarryPolicyConfig, decision termCarryDecision, spotPosition, perpPosition int64, current **termCarryLifecycleTerm, terms *[]*termCarryLifecycleTerm) string {
	if decision.PlanCreatedAt == 0 {
		if decision.FirstExposureAt != 0 {
			return "first_exposure_without_plan"
		}
	} else if decision.TermEnd <= decision.PlanCreatedAt {
		return "invalid_term_bounds"
	} else if decision.FirstExposureAt != 0 && (decision.FirstExposureAt < decision.PlanCreatedAt || decision.FirstExposureAt > decision.DecisionTime) {
		return "first_exposure_before_plan"
	}
	if *current != nil {
		if decision.PlanCreatedAt != (*current).planCreatedAt || decision.TermEnd != (*current).termEnd {
			return "term_identity_changed_while_open"
		}
		if decision.FirstExposureAt != (*current).firstExposureAt {
			return "first_exposure_mismatch"
		}
	}
	switch decision.Action {
	case "SUBMIT_ENTRY_SPOT_IOC":
		if *current != nil || decision.State != "ENTRY_SPOT" || decision.PlanCreatedAt != decision.DecisionTime || decision.FirstExposureAt != 0 || decision.TargetSpot == 0 || decision.TargetSpot != -decision.TargetPerp || (decision.TargetSpot != policy.MaxPosition && decision.TargetSpot != -policy.MaxPosition) {
			return "entry_spot_invalid_plan"
		}
		*current = &termCarryLifecycleTerm{owner: termCarryParticipant{venue: decision.VenueID, client: decision.ClientID}, policyVersion: decision.PolicyVersion, planCreatedAt: decision.PlanCreatedAt, termEnd: decision.TermEnd}
		*terms = append(*terms, *current)
	case "SUBMIT_ENTRY_PERP_IOC":
		if *current == nil || decision.State != "ENTRY_PERP" {
			return "entry_perp_without_entry_state"
		}
	case "TERM_ACTIVE":
		if *current == nil || decision.State != "ACTIVE_TERM" || (*current).firstExposureAt == 0 || spotPosition == 0 || perpPosition != -spotPosition || decision.DecisionTime >= (*current).termEnd {
			return "active_term_without_first_exposure"
		}
		if !(*current).active {
			(*current).active, (*current).activeAt = true, decision.DecisionTime
		}
	case "SUBMIT_UNWIND_PERP_IOC":
		if *current == nil || decision.State != "UNWIND_PERP" || decision.DecisionTime < (*current).termEnd || perpPosition == 0 {
			return "perp_unwind_state_mismatch"
		}
	case "SUBMIT_UNWIND_SPOT_IOC":
		if *current == nil || decision.State != "UNWIND_SPOT" || decision.DecisionTime < (*current).termEnd || perpPosition != 0 || spotPosition == 0 {
			return "spot_unwind_state_mismatch"
		}
	case "UNWIND_PRICE_UNAVAILABLE", "UNWIND_PRICE_OUTSIDE_DOMAIN", "UNWIND_PERP_GAP_UNREPRESENTABLE", "UNWIND_SPOT_GAP_UNREPRESENTABLE":
		if *current == nil || (decision.State != "UNWIND_PERP" && decision.State != "UNWIND_SPOT") || decision.DecisionTime < (*current).termEnd {
			return "unwind_defer_state_mismatch"
		}
	case "TERM_CLOSED":
		if *current == nil || decision.State != "IDLE" || decision.DecisionTime < (*current).termEnd || spotPosition != 0 || perpPosition != 0 {
			return "term_close_state_mismatch"
		}
		if (*current).closed {
			return "duplicate_term_close"
		}
		(*current).closed, (*current).closedAt = true, decision.DecisionTime
		*current = nil
	default:
		if *current == nil && decision.State != "IDLE" {
			return "nonidle_state_without_open_term"
		}
	}
	return ""
}

func applyTermCarryOutcome(policy termCarryPolicyConfig, outcome termCarryOutcome, spotPosition, perpPosition int64, result *TermCarryAudit, check func(string, uint64, uint64, string)) (int64, int64) {
	fail := func(reason string) {
		result.PositionContinuityErrors++
		check(outcome.VenueID, outcome.ClientID, outcome.RequestID, reason)
	}
	if outcome.DecisionTime <= 0 || outcome.State == "" || outcome.Leg == "" || outcome.RequestID == 0 || outcome.SpotPositionBefore != spotPosition || outcome.PerpPositionBefore != perpPosition {
		fail("outcome_position_before_mismatch")
	}
	switch outcome.Event {
	case "ORDER_ACCEPTED", "ORDER_REJECTED", "ORDER_CANCELLED":
		if outcome.SpotPositionAfter != spotPosition || outcome.PerpPositionAfter != perpPosition {
			fail("nonfill_changed_position")
		}
		return spotPosition, perpPosition
	case "ORDER_FILL":
		if outcome.ExecutionTime <= 0 || outcome.Qty <= 0 || (outcome.Symbol != policy.SpotSymbol && outcome.Symbol != policy.PerpSymbol) || (outcome.Side != exchange.Buy.String() && outcome.Side != exchange.Sell.String()) {
			fail("invalid_fill_outcome")
			return spotPosition, perpPosition
		}
		delta := outcome.Qty
		if outcome.Side == exchange.Sell.String() {
			delta = -delta
		}
		if outcome.Symbol == policy.SpotSymbol {
			next, ok := fundingCarryAuditAdd(spotPosition, delta)
			if !ok || outcome.SpotPositionAfter != next || outcome.PerpPositionAfter != perpPosition {
				fail("spot_fill_position_mismatch")
				return spotPosition, perpPosition
			}
			return next, perpPosition
		}
		next, ok := fundingCarryAuditAdd(perpPosition, delta)
		if !ok || outcome.SpotPositionAfter != spotPosition || outcome.PerpPositionAfter != next {
			fail("perp_fill_position_mismatch")
			return spotPosition, perpPosition
		}
		return spotPosition, next
	default:
		fail("unknown_term_carry_outcome")
		return spotPosition, perpPosition
	}
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
	if policy.SpotSymbol != "ABC/USD" || policy.PerpSymbol != "ABC-PERP" || policy.DecisionPeriod <= 0 || policy.CommitmentIntervals <= 0 || policy.MaxFundingAge <= 0 || policy.MandateEndAtNano < 0 || policy.MaxPosition <= 0 || policy.LotQty <= 0 || policy.MinOrderSize < 0 || policy.SpotTick <= 0 || policy.PerpTick <= 0 {
		return fmt.Errorf("invalid term-carry policy manifest")
	}
	if policy.UnwindMinOrderSize != nil && *policy.UnwindMinOrderSize < 0 {
		return fmt.Errorf("negative explicit term-carry unwind minimum")
	}
	for _, value := range []int64{policy.TakerFeeBps, policy.LongSpotFundingBps, policy.ShortSpotBorrowBps, policy.BalanceSheetBps, policy.MarginRiskBps, policy.LegRiskBps, policy.MinNetCarryBps} {
		if value < 0 {
			return fmt.Errorf("negative term-carry bps policy")
		}
	}
	return nil
}

func validateTermCarryDecision(policy termCarryPolicyConfig, decision termCarryDecision, sources map[fundingCarrySourceKey][]observationRecord, frontiers map[fundingCarryReceiptKey]auditedFrontier) error {
	if decision.VenueID == "" || decision.ClientID == 0 || decision.Desk == "" || !termCarryKnownPolicyVersion(decision.PolicyVersion) || decision.DecisionTime == 0 || decision.Action == "" || decision.SpotSymbol != policy.SpotSymbol || decision.PerpSymbol != policy.PerpSymbol || decision.MandateEndAt != policy.MandateEndAtNano || decision.CommitmentIntervals != policy.CommitmentIntervals || !termCarryKnownAction(decision.Action) {
		return fmt.Errorf("invalid_decision_fields")
	}
	if err := validateTermCarryPolicyEvidence(policy, decision); err != nil {
		return err
	}
	if termCarryUsesV2Lifecycle(decision.PolicyVersion) && (decision.EntryAt != 0 || decision.FirstExposureAt < 0 || decision.PlanCreatedAt < 0 || (decision.FirstExposureAt != 0 && decision.PlanCreatedAt == 0)) {
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

func termCarryKnownPolicyVersion(version string) bool {
	return version == termCarryPolicyV1 || version == termCarryPolicyV2 || version == termCarryPolicyV3
}

func termCarryUsesV2Lifecycle(version string) bool {
	return version == termCarryPolicyV2 || version == termCarryPolicyV3
}

func validateTermCarryPolicyEvidence(policy termCarryPolicyConfig, decision termCarryDecision) error {
	if policy.UnwindMinOrderSize == nil {
		if decision.PolicyVersion == termCarryPolicyV3 || decision.UnwindMinOrderSize != nil {
			return fmt.Errorf("term_carry_unwind_policy_mismatch")
		}
		return nil
	}
	if decision.PolicyVersion != termCarryPolicyV3 || decision.UnwindMinOrderSize == nil || *decision.UnwindMinOrderSize != *policy.UnwindMinOrderSize {
		return fmt.Errorf("term_carry_unwind_policy_mismatch")
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
		if termCarryUsesV2Lifecycle(decision.PolicyVersion) && (decision.PlanCreatedAt != decision.DecisionTime || decision.FirstExposureAt != 0) {
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
	minimum := policy.MinOrderSize
	if decision.PolicyVersion == termCarryPolicyV3 && (decision.Leg == "UNWIND_PERP_IOC" || decision.Leg == "UNWIND_SPOT_IOC") {
		if policy.UnwindMinOrderSize == nil {
			return fmt.Errorf("submission_unwind_policy_missing")
		}
		minimum = *policy.UnwindMinOrderSize
	}
	wantQty, ok := fundingCarryAuditSizedQty(gap, policy.LotQty, available, minimum)
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
