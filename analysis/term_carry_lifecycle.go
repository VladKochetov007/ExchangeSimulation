package analysis

import (
	"fmt"
	"sort"

	"exchange_sim/exchange"
)

const (
	LifecycleObserved      = "observed"
	LifecycleNotObserved   = "not_observed"
	LifecycleNotApplicable = "not_applicable"
	LifecycleNotExercised  = "not_exercised"
)

// TermCarryLifecycleOptions fixes the causal screen's analysis boundary.
type TermCarryLifecycleOptions struct {
	DeadlineAtNano int64
}

// TermCarryLifecycleEndpoint reports whether and when one lifecycle fact held.
type TermCarryLifecycleEndpoint struct {
	Status string `json:"status"`
	AtNano *int64 `json:"at_nano,omitempty"`
}

// TermCarryLifecycleQuantityEndpoint distinguishes an observed zero from an
// endpoint that was not applicable or exercised.
type TermCarryLifecycleQuantityEndpoint struct {
	Status   string `json:"status"`
	AtNano   *int64 `json:"at_nano,omitempty"`
	Quantity int64  `json:"quantity"`
}

// TermCarryLifecyclePosition is an independently replayed spot/perpetual pair.
type TermCarryLifecyclePosition struct {
	Status            string `json:"status"`
	AtNano            int64  `json:"at_nano"`
	Spot              int64  `json:"spot"`
	Perp              int64  `json:"perp"`
	ResidualMagnitude int64  `json:"residual_magnitude"`
}

// TermCarryLifecycleAggressiveEvidence records the local displayed-depth
// condition which made an ordinary IOC child legally unavailable.
type TermCarryLifecycleAggressiveEvidence struct {
	Status             string `json:"status"`
	Eligible           bool   `json:"eligible"`
	AtNano             *int64 `json:"at_nano,omitempty"`
	Leg                string `json:"leg,omitempty"`
	Side               string `json:"side,omitempty"`
	ContraDisplayedQty int64  `json:"contra_displayed_qty"`
	EffectiveMinimum   int64  `json:"effective_minimum"`
}

// TermCarryLifecycleRestingEvidence measures canonical acceptance to the first
// terminal order event or run censoring.
type TermCarryLifecycleRestingEvidence struct {
	Status        string `json:"status"`
	AcceptedAt    *int64 `json:"accepted_at_nano,omitempty"`
	EndedAt       *int64 `json:"ended_at_nano,omitempty"`
	DurationNanos *int64 `json:"duration_nanos,omitempty"`
	Censored      bool   `json:"censored"`
}

// TermCarryLifecycleCancellationEvidence joins the actor request identity to
// the canonical venue acknowledgement.
type TermCarryLifecycleCancellationEvidence struct {
	Status          string `json:"status"`
	OrderID         uint64 `json:"order_id"`
	CancelRequestID uint64 `json:"cancel_request_id"`
	RequestedAtNano *int64 `json:"requested_at_nano,omitempty"`
	AcknowledgedAt  *int64 `json:"acknowledged_at_nano,omitempty"`
}

// TermCarryLifecyclePassiveOrder is one ordinary P4 order chain.
type TermCarryLifecyclePassiveOrder struct {
	RequestID    uint64                                 `json:"request_id"`
	OrderID      uint64                                 `json:"order_id"`
	Leg          string                                 `json:"leg"`
	Side         string                                 `json:"side"`
	Price        int64                                  `json:"price"`
	RequestedQty int64                                  `json:"requested_qty"`
	Admission    TermCarryLifecycleEndpoint             `json:"admission"`
	Resting      TermCarryLifecycleRestingEvidence      `json:"resting"`
	FilledQty    int64                                  `json:"filled_qty"`
	PartialFill  TermCarryLifecycleQuantityEndpoint     `json:"partial_fill"`
	FullFill     TermCarryLifecycleQuantityEndpoint     `json:"full_fill"`
	Cancellation TermCarryLifecycleCancellationEvidence `json:"cancellation"`
}

// TermCarryLifecycleFundingPhase keeps funding timing separate from closure.
type TermCarryLifecycleFundingPhase struct {
	Settlements   int64 `json:"settlements"`
	NetQuoteDelta int64 `json:"net_quote_delta"`
}

// TermCarryLifecycleFundingEvidence independently classifies every attributed
// funding record and reports expected post-deadline records that were absent.
type TermCarryLifecycleFundingEvidence struct {
	BeforeTermEnd          TermCarryLifecycleFundingPhase `json:"before_term_end"`
	ResidualBeforeDeadline TermCarryLifecycleFundingPhase `json:"residual_before_deadline"`
	ResidualAfterDeadline  TermCarryLifecycleFundingPhase `json:"residual_after_deadline"`
	PostClose              TermCarryLifecycleFundingPhase `json:"post_close"`
	ExpectedAfterDeadline  int64                          `json:"expected_after_deadline"`
	MissingAfterDeadline   int64                          `json:"missing_after_deadline"`
}

// TermCarryLifecycleConservation exposes the independent canonical/actor and
// terminal-position identities for one term.
type TermCarryLifecycleConservation struct {
	CanonicalFillRecords int64 `json:"canonical_fill_records"`
	ActorFillRecords     int64 `json:"actor_fill_records"`
	CanonicalFilledQty   int64 `json:"canonical_filled_qty"`
	ActorFilledQty       int64 `json:"actor_filled_qty"`
	FillChainValid       bool  `json:"fill_chain_valid"`
	TerminalSpotAgrees   bool  `json:"terminal_spot_agrees"`
	TerminalPerpAgrees   bool  `json:"terminal_perp_agrees"`
}

// TermCarryLifecycleTerm is the complete independently graded record for one
// canonically owned term.
type TermCarryLifecycleTerm struct {
	TermID                 string                                 `json:"term_id"`
	VenueID                string                                 `json:"venue_id"`
	ClientID               uint64                                 `json:"client_id"`
	PolicyVersion          string                                 `json:"policy_version"`
	PlanCreatedAtNano      int64                                  `json:"plan_created_at_nano"`
	TermEndAtNano          int64                                  `json:"term_end_at_nano"`
	AnalysisDeadlineAtNano int64                                  `json:"analysis_deadline_at_nano"`
	Ownership              TermCarryLifecycleEndpoint             `json:"ownership"`
	Activation             TermCarryLifecycleEndpoint             `json:"activation"`
	AggressiveEligibility  TermCarryLifecycleAggressiveEvidence   `json:"aggressive_eligibility"`
	PassiveEligibility     TermCarryLifecycleEndpoint             `json:"passive_eligibility"`
	PassiveAdmission       TermCarryLifecycleEndpoint             `json:"passive_admission"`
	PassiveOrders          []TermCarryLifecyclePassiveOrder       `json:"passive_orders"`
	PassiveFilledQuantity  TermCarryLifecycleQuantityEndpoint     `json:"passive_filled_quantity"`
	PositionAtTermEnd      TermCarryLifecyclePosition             `json:"position_at_term_end"`
	FirstResidualReduction TermCarryLifecycleEndpoint             `json:"first_residual_reduction"`
	PositionAtDeadline     TermCarryLifecyclePosition             `json:"position_at_deadline"`
	TerminalPosition       TermCarryLifecyclePosition             `json:"terminal_position"`
	DeadlineState          TermCarryLifecycleEndpoint             `json:"deadline_state"`
	Cancellation           TermCarryLifecycleCancellationEvidence `json:"cancellation"`
	Flatness               TermCarryLifecycleEndpoint             `json:"flatness"`
	CloseTransitionCount   int64                                  `json:"close_transition_count"`
	CloseTransitionAtNano  *int64                                 `json:"close_transition_at_nano,omitempty"`
	ProvenClosedByDeadline bool                                   `json:"proven_closed_by_deadline"`
	MutatedAfterClose      bool                                   `json:"mutated_after_close"`
	Funding                TermCarryLifecycleFundingEvidence      `json:"funding"`
	Conservation           TermCarryLifecycleConservation         `json:"conservation"`
}

// TermCarryLifecycleAggregate contains the registered cell-level endpoints.
type TermCarryLifecycleAggregate struct {
	ExerciseStatus                   string   `json:"exercise_status"`
	OwnedTerms                       int64    `json:"owned_terms"`
	ActivatedTerms                   int64    `json:"activated_terms"`
	EligibleTerms                    int64    `json:"eligible_terms"`
	PassiveEligibleTerms             int64    `json:"passive_eligible_terms"`
	PassiveAdmittedTerms             int64    `json:"passive_admitted_terms"`
	ProvenClosedEligibleTerms        int64    `json:"proven_closed_eligible_terms"`
	ClosureFraction                  *float64 `json:"closure_fraction,omitempty"`
	AllEligibleTermsClosedByDeadline *bool    `json:"all_eligible_terms_closed_by_deadline,omitempty"`
	PassiveFilledQuantity            int64    `json:"passive_filled_quantity"`
	TerminalResidualMagnitude        int64    `json:"terminal_residual_magnitude"`
	ResidualFundingSettlements       int64    `json:"residual_funding_settlements"`
	ResidualFundingNetQuoteDelta     int64    `json:"residual_funding_net_quote_delta"`
}

// TermCarryLifecycleIntegrityFailure is aggregated by identity so a repeated
// bad decision cannot expand a full-cell result by hundreds of thousands of
// duplicate rows.
type TermCarryLifecycleIntegrityFailure struct {
	TermID    string `json:"term_id,omitempty"`
	VenueID   string `json:"venue_id,omitempty"`
	ClientID  uint64 `json:"client_id,omitempty"`
	RequestID uint64 `json:"request_id,omitempty"`
	Failure   string `json:"failure"`
	Count     int64  `json:"count"`
}

// TermCarryLifecycleAudit is the fail-closed P3e lifecycle evidence report.
type TermCarryLifecycleAudit struct {
	SchemaVersion          int                                  `json:"schema_version"`
	Arm                    string                               `json:"arm"`
	AnalysisDeadlineAtNano int64                                `json:"analysis_deadline_at_nano"`
	ObservationEndAtNano   int64                                `json:"observation_end_at_nano"`
	Terms                  []TermCarryLifecycleTerm             `json:"terms"`
	Aggregates             TermCarryLifecycleAggregate          `json:"aggregates"`
	IntegrityFailures      []TermCarryLifecycleIntegrityFailure `json:"integrity_failures"`
	IntegrityValid         bool                                 `json:"integrity_valid"`
}

type lifecycleDecisionEvidence struct {
	termCarryDecision
	at int64
}

type lifecycleOrderEvidence struct {
	at    int64
	order fundingCarryVenueOrder
}

type lifecycleFillEvidence struct {
	at         int64
	fill       fundingCarryVenueFill
	requestID  uint64
	decision   termCarryDecision
	spotBefore int64
	spotAfter  int64
	perpBefore int64
	perpAfter  int64
}

type lifecycleCancellationEvidence struct {
	at           int64
	cancellation termCarryVenueCancellation
}

type lifecycleFundingEvidence struct {
	at            int64
	netQuoteDelta int64
}

type lifecycleTermCandidate struct {
	participant   termCarryParticipant
	policyVersion string
	planCreatedAt int64
	termEnd       int64
	decisions     []termCarryDecision
	fills         []lifecycleFillEvidence
	actorOutcomes []termCarryOutcome
}

type lifecycleFailureCollector struct {
	rows map[string]*TermCarryLifecycleIntegrityFailure
}

func newLifecycleFailureCollector() *lifecycleFailureCollector {
	return &lifecycleFailureCollector{rows: make(map[string]*TermCarryLifecycleIntegrityFailure)}
}

func (c *lifecycleFailureCollector) add(termID, venue string, client, request uint64, failure string) {
	key := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", termID, venue, client, request, failure)
	if row := c.rows[key]; row != nil {
		row.Count++
		return
	}
	c.rows[key] = &TermCarryLifecycleIntegrityFailure{
		TermID: termID, VenueID: venue, ClientID: client, RequestID: request,
		Failure: failure, Count: 1,
	}
}

func (c *lifecycleFailureCollector) sorted() []TermCarryLifecycleIntegrityFailure {
	rows := make([]TermCarryLifecycleIntegrityFailure, 0, len(c.rows))
	for _, row := range c.rows {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Failure != rows[j].Failure {
			return rows[i].Failure < rows[j].Failure
		}
		if rows[i].TermID != rows[j].TermID {
			return rows[i].TermID < rows[j].TermID
		}
		if rows[i].VenueID != rows[j].VenueID {
			return rows[i].VenueID < rows[j].VenueID
		}
		if rows[i].ClientID != rows[j].ClientID {
			return rows[i].ClientID < rows[j].ClientID
		}
		return rows[i].RequestID < rows[j].RequestID
	})
	return rows
}

type lifecycleEvidence struct {
	decisions      []lifecycleDecisionEvidence
	outcomes       []termCarryOutcome
	accepted       map[fundingCarryKey][]lifecycleOrderEvidence
	rejected       map[fundingCarryKey][]lifecycleOrderEvidence
	fills          map[fundingCarryOrderKey][]lifecycleFillEvidence
	cancellations  map[fundingCarryOrderKey][]lifecycleCancellationEvidence
	funding        map[termCarryParticipant][]lifecycleFundingEvidence
	observationEnd int64
}

// MeasureTermCarryLifecycle independently grades finite-term ownership and
// closure from persisted evidence.
func (r *Run) MeasureTermCarryLifecycle(opts TermCarryLifecycleOptions) (*TermCarryLifecycleAudit, error) {
	if opts.DeadlineAtNano <= 0 {
		return nil, fmt.Errorf("term carry lifecycle: analysis deadline must be positive")
	}
	policy, err := loadTermCarryPolicy(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validTermCarryPolicy(policy); err != nil {
		return nil, err
	}
	result := &TermCarryLifecycleAudit{SchemaVersion: 1, AnalysisDeadlineAtNano: opts.DeadlineAtNano, Arm: "A"}
	failures := newLifecycleFailureCollector()
	if policy.PassiveExit != nil {
		result.Arm = "B"
		if policy.PassiveExit.DeadlineAtNano != opts.DeadlineAtNano {
			failures.add("", "", 0, 0, "passive_exit_deadline_mismatch")
		}
	}

	sources, frontiers, gateways, receiptAudit, receiptErr := fundingCarryReceipts(r.Dir)
	if receiptErr != nil || receiptAudit == nil || !receiptAudit.Valid {
		failures.add("", "", 0, 0, "receipt_audit_invalid")
	}
	evidence, err := r.scanTermCarryLifecycleEvidence(policy, failures)
	if err != nil {
		return nil, fmt.Errorf("term carry lifecycle: scan: %w", err)
	}
	result.ObservationEndAtNano = evidence.observationEnd
	if evidence.observationEnd < opts.DeadlineAtNano {
		failures.add("", "", 0, 0, "analysis_deadline_not_observed")
	}

	requestDecisions := validateLifecycleDecisionEvidence(policy, evidence.decisions, sources, frontiers, failures)
	acceptedOrders := validateLifecycleOrderChains(evidence, requestDecisions, gateways, failures)
	candidates := buildLifecycleTermCandidates(evidence, requestDecisions, acceptedOrders, failures)
	result.Terms = gradeLifecycleTerms(r, policy, opts, evidence, candidates, failures)
	result.Aggregates = aggregateLifecycleTerms(result.Terms)
	result.IntegrityFailures = failures.sorted()
	result.IntegrityValid = len(result.IntegrityFailures) == 0
	return result, nil
}

func (r *Run) scanTermCarryLifecycleEvidence(policy termCarryPolicyConfig, failures *lifecycleFailureCollector) (lifecycleEvidence, error) {
	evidence := lifecycleEvidence{
		accepted:      make(map[fundingCarryKey][]lifecycleOrderEvidence),
		rejected:      make(map[fundingCarryKey][]lifecycleOrderEvidence),
		fills:         make(map[fundingCarryOrderKey][]lifecycleFillEvidence),
		cancellations: make(map[fundingCarryOrderKey][]lifecycleCancellationEvidence),
		funding:       make(map[termCarryParticipant][]lifecycleFundingEvidence),
	}
	events := []string{"term_carry_decision", "term_carry_leg_outcome", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "balance_change"}
	err := r.Scan(ScanOptions{Events: events, Workers: 1}, func(event Event) {
		if event.SimTS > evidence.observationEnd {
			evidence.observationEnd = event.SimTS
		}
		if r.Role(event.VenueID, event.ClientID) != "term_carry_allocator" {
			return
		}
		switch event.Name {
		case "term_carry_decision":
			var decision termCarryDecision
			if event.Decode(&decision) != nil || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID {
				failures.add("", event.VenueID, event.ClientID, 0, "invalid_decision_record")
				return
			}
			evidence.decisions = append(evidence.decisions, lifecycleDecisionEvidence{termCarryDecision: decision, at: event.SimTS})
		case "term_carry_leg_outcome":
			var outcome termCarryOutcome
			if event.Decode(&outcome) != nil || outcome.VenueID != event.VenueID || outcome.ClientID != event.ClientID {
				failures.add("", event.VenueID, event.ClientID, 0, "invalid_actor_outcome")
				return
			}
			evidence.outcomes = append(evidence.outcomes, outcome)
		case "OrderAccepted", "OrderRejected":
			var order fundingCarryVenueOrder
			if event.Decode(&order) != nil || order.RequestID == 0 {
				failures.add("", event.VenueID, event.ClientID, 0, "invalid_canonical_order_record")
				return
			}
			order.VenueID, order.ClientID = event.VenueID, event.ClientID
			key := fundingCarryKey{event.VenueID, event.ClientID, order.RequestID}
			row := lifecycleOrderEvidence{at: event.SimTS, order: order}
			if event.Name == "OrderAccepted" {
				evidence.accepted[key] = append(evidence.accepted[key], row)
			} else {
				evidence.rejected[key] = append(evidence.rejected[key], row)
			}
		case "OrderFill":
			var fill fundingCarryVenueFill
			if event.Decode(&fill) != nil || fill.OrderID == 0 {
				failures.add("", event.VenueID, event.ClientID, 0, "invalid_canonical_fill_record")
				return
			}
			fill.VenueID, fill.ClientID = event.VenueID, event.ClientID
			key := fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}
			evidence.fills[key] = append(evidence.fills[key], lifecycleFillEvidence{at: event.SimTS, fill: fill})
		case "OrderCancelled":
			var cancellation termCarryVenueCancellation
			if event.Decode(&cancellation) != nil || cancellation.OrderID == 0 {
				failures.add("", event.VenueID, event.ClientID, 0, "invalid_canonical_cancellation_record")
				return
			}
			cancellation.VenueID, cancellation.ClientID = event.VenueID, event.ClientID
			key := fundingCarryOrderKey{event.VenueID, event.ClientID, cancellation.OrderID}
			evidence.cancellations[key] = append(evidence.cancellations[key], lifecycleCancellationEvidence{at: event.SimTS, cancellation: cancellation})
		case "balance_change":
			var balance balanceChangeRecord
			if event.Decode(&balance) != nil || balance.Symbol != policy.PerpSymbol || balance.Reason != "funding_settlement" {
				return
			}
			at := balance.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			net := int64(0)
			for _, change := range balance.Changes {
				want, ok := fundingCarryAuditSub(change.NewBalance, change.OldBalance)
				if !ok || want != change.Delta {
					failures.add("", event.VenueID, event.ClientID, 0, "funding_balance_delta_mismatch")
					continue
				}
				if change.Asset == "USD" {
					next, ok := fundingCarryAuditAdd(net, change.Delta)
					if !ok {
						failures.add("", event.VenueID, event.ClientID, 0, "funding_delta_overflow")
						continue
					}
					net = next
				}
			}
			participant := termCarryParticipant{venue: event.VenueID, client: event.ClientID}
			evidence.funding[participant] = append(evidence.funding[participant], lifecycleFundingEvidence{at: at, netQuoteDelta: net})
		}
	})
	return evidence, err
}

func validateLifecycleDecisionEvidence(policy termCarryPolicyConfig, decisions []lifecycleDecisionEvidence, sources map[fundingCarrySourceKey][]observationRecord, frontiers map[fundingCarryReceiptKey]auditedFrontier, failures *lifecycleFailureCollector) map[fundingCarryKey]termCarryDecision {
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
		failures.add("", "", 0, 0, "missing_term_carry_decisions")
	}
	requests := make(map[fundingCarryKey]termCarryDecision)
	ticks := make(map[string]struct{})
	for _, row := range decisions {
		decision := row.termCarryDecision
		termID := lifecycleTermID(decision.VenueID, decision.ClientID, decision.PlanCreatedAt)
		if row.at != decision.DecisionTime {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "decision_timestamp_mismatch")
		}
		if err := validateTermCarryDecision(policy, decision, sources, frontiers); err != nil {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, err.Error())
		}
		tick := fmt.Sprintf("%s/%d/%d", decision.VenueID, decision.ClientID, decision.DecisionTime)
		if _, exists := ticks[tick]; exists {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_decision_tick")
		}
		ticks[tick] = struct{}{}
		if !termCarrySubmission(decision.Action) {
			continue
		}
		key := fundingCarryKey{decision.VenueID, decision.ClientID, decision.RequestID}
		if _, exists := requests[key]; exists {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_submission_request")
		}
		requests[key] = decision
	}
	return requests
}

func validateLifecycleOrderChains(evidence lifecycleEvidence, requests map[fundingCarryKey]termCarryDecision, gateways map[fundingCarryKey]fundingCarryGatewayDecision, failures *lifecycleFailureCollector) map[fundingCarryOrderKey]lifecycleOrderEvidence {
	orders := make(map[fundingCarryOrderKey]lifecycleOrderEvidence)
	actorByRequest := make(map[fundingCarryKey][]termCarryOutcome)
	for _, outcome := range evidence.outcomes {
		key := fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}
		actorByRequest[key] = append(actorByRequest[key], outcome)
		if _, exists := requests[key]; !exists {
			failures.add("", outcome.VenueID, outcome.ClientID, outcome.RequestID, "actor_outcome_without_submission")
		}
	}
	for key, decision := range requests {
		termID := lifecycleTermID(decision.VenueID, decision.ClientID, decision.PlanCreatedAt)
		gateway, found := gateways[key]
		if !found {
			failures.add(termID, key.venue, key.client, key.request, "missing_gateway_decision")
		} else if !termCarryGatewayMatches(decision, gateway) {
			failures.add(termID, key.venue, key.client, key.request, "gateway_decision_mismatch")
		}
		accepted, rejected := evidence.accepted[key], evidence.rejected[key]
		if len(accepted)+len(rejected) != 1 {
			failure := "missing_canonical_admission"
			if len(accepted)+len(rejected) > 1 {
				failure = "duplicate_canonical_admission"
			}
			failures.add(termID, key.venue, key.client, key.request, failure)
			continue
		}
		if len(rejected) == 1 {
			if !termCarryVenueOrderMatches(decision, rejected[0].order) || !termCarryHasRejected(actorByRequest[key], rejected[0].order.Error) {
				failures.add(termID, key.venue, key.client, key.request, "rejection_chain_mismatch")
			}
			continue
		}
		acceptedOrder := accepted[0]
		if acceptedOrder.order.OrderID == 0 || !termCarryVenueOrderMatches(decision, acceptedOrder.order) {
			failures.add(termID, key.venue, key.client, key.request, "accepted_order_mismatch")
		}
		if !termCarryHasOutcome(actorByRequest[key], "ORDER_ACCEPTED", acceptedOrder.order.OrderID) {
			failures.add(termID, key.venue, key.client, key.request, "missing_actor_acceptance")
		}
		orderKey := fundingCarryOrderKey{key.venue, key.client, acceptedOrder.order.OrderID}
		if _, exists := orders[orderKey]; exists {
			failures.add(termID, key.venue, key.client, key.request, "duplicate_order_identity")
		}
		orders[orderKey] = acceptedOrder
		validateLifecycleFills(termID, decision, acceptedOrder, evidence.fills[orderKey], actorByRequest[key], failures)
		validateLifecycleCancellation(termID, decision, acceptedOrder, evidence.cancellations[orderKey], actorByRequest[key], evidence.decisions, failures)
	}
	for key, rows := range evidence.fills {
		if _, exists := orders[key]; !exists {
			for range rows {
				failures.add("", key.venue, key.client, 0, "canonical_fill_without_admission")
			}
		}
	}
	return orders
}

func validateLifecycleFills(termID string, decision termCarryDecision, accepted lifecycleOrderEvidence, fills []lifecycleFillEvidence, outcomes []termCarryOutcome, failures *lifecycleFailureCollector) {
	seen := make(map[uint64]struct{})
	total := int64(0)
	for _, row := range fills {
		fill := row.fill
		if _, exists := seen[fill.TradeID]; exists || fill.TradeID == 0 {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_canonical_fill")
		}
		seen[fill.TradeID] = struct{}{}
		if fill.Symbol != termCarryAuditSymbol(decision) || fill.Side != decision.Side || fill.Qty <= 0 || !termCarryHasFill(outcomes, fill) {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "canonical_actor_fill_mismatch")
		}
		next, ok := fundingCarryAuditAdd(total, fill.Qty)
		if !ok || next > accepted.order.Qty {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "fill_quantity_exceeds_order")
			continue
		}
		total = next
	}
	for _, outcome := range outcomes {
		if outcome.Event != "ORDER_FILL" {
			continue
		}
		matched := false
		for _, fill := range fills {
			if termCarryHasFill([]termCarryOutcome{outcome}, fill.fill) {
				matched = true
				break
			}
		}
		if !matched {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "actor_fill_without_canonical_fill")
		}
	}
}

func validateLifecycleCancellation(termID string, decision termCarryDecision, accepted lifecycleOrderEvidence, cancellations []lifecycleCancellationEvidence, outcomes []termCarryOutcome, decisions []lifecycleDecisionEvidence, failures *lifecycleFailureCollector) {
	if len(cancellations) > 1 {
		failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "duplicate_canonical_cancellation")
	}
	if decision.Action != "SUBMIT_UNWIND_PERP_POST_ONLY" && decision.Action != "SUBMIT_UNWIND_SPOT_POST_ONLY" {
		for _, row := range cancellations {
			if !termCarryHasOutcome(outcomes, "ORDER_CANCELLED", accepted.order.OrderID) {
				failures.add(termID, decision.VenueID, decision.ClientID, row.cancellation.RequestID, "missing_actor_cancellation")
			}
		}
		return
	}
	for _, row := range cancellations {
		var cancelDecision termCarryDecision
		found := false
		for _, candidate := range decisions {
			if candidate.Action == "CANCEL_PASSIVE_EXIT_AT_DEADLINE" && candidate.VenueID == decision.VenueID && candidate.ClientID == decision.ClientID && candidate.CancelRequestID == row.cancellation.RequestID {
				cancelDecision, found = candidate.termCarryDecision, true
				break
			}
		}
		if failure := validateTermCarryPassiveExitCancellationChain(row.cancellation, accepted.order.OrderID, cancelDecision, found, outcomes); failure != "" {
			failures.add(termID, decision.VenueID, decision.ClientID, row.cancellation.RequestID, failure)
		}
	}
}

func buildLifecycleTermCandidates(evidence lifecycleEvidence, requests map[fundingCarryKey]termCarryDecision, accepted map[fundingCarryOrderKey]lifecycleOrderEvidence, failures *lifecycleFailureCollector) []lifecycleTermCandidate {
	type candidateKey struct {
		participant termCarryParticipant
		plan        int64
	}
	candidates := make(map[candidateKey]*lifecycleTermCandidate)
	for _, row := range evidence.decisions {
		decision := row.termCarryDecision
		if decision.PlanCreatedAt == 0 {
			if decision.Action == "TERM_CLOSED" {
				failures.add("", decision.VenueID, decision.ClientID, 0, "close_transition_without_term_identity")
			}
			continue
		}
		key := candidateKey{termCarryParticipant{venue: decision.VenueID, client: decision.ClientID}, decision.PlanCreatedAt}
		candidate := candidates[key]
		if candidate == nil {
			candidate = &lifecycleTermCandidate{participant: key.participant, policyVersion: decision.PolicyVersion, planCreatedAt: decision.PlanCreatedAt, termEnd: decision.TermEnd}
			candidates[key] = candidate
		}
		if candidate.termEnd != decision.TermEnd || candidate.policyVersion != decision.PolicyVersion {
			failures.add(lifecycleTermID(decision.VenueID, decision.ClientID, decision.PlanCreatedAt), decision.VenueID, decision.ClientID, decision.RequestID, "term_identity_mutated")
		}
		candidate.decisions = append(candidate.decisions, decision)
	}
	for key, decision := range requests {
		if decision.PlanCreatedAt == 0 {
			continue
		}
		candidate := candidates[candidateKey{termCarryParticipant{venue: key.venue, client: key.client}, decision.PlanCreatedAt}]
		if candidate == nil {
			continue
		}
		for orderKey, order := range accepted {
			if orderKey.venue != key.venue || orderKey.client != key.client || order.order.RequestID != key.request {
				continue
			}
			for _, fill := range evidence.fills[orderKey] {
				fill.requestID, fill.decision = key.request, decision
				candidate.fills = append(candidate.fills, fill)
			}
		}
	}
	for _, outcome := range evidence.outcomes {
		decision, found := requests[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}]
		if !found || decision.PlanCreatedAt == 0 {
			continue
		}
		candidate := candidates[candidateKey{termCarryParticipant{venue: outcome.VenueID, client: outcome.ClientID}, decision.PlanCreatedAt}]
		if candidate != nil {
			candidate.actorOutcomes = append(candidate.actorOutcomes, outcome)
		}
	}
	result := make([]lifecycleTermCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		owned := false
		for _, fill := range candidate.fills {
			if fill.decision.Action == "SUBMIT_ENTRY_SPOT_IOC" || fill.decision.Action == "SUBMIT_ENTRY_PERP_IOC" {
				owned = true
				break
			}
		}
		if owned {
			result = append(result, *candidate)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].participant.venue != result[j].participant.venue {
			return result[i].participant.venue < result[j].participant.venue
		}
		if result[i].participant.client != result[j].participant.client {
			return result[i].participant.client < result[j].participant.client
		}
		return result[i].planCreatedAt < result[j].planCreatedAt
	})
	return result
}

func gradeLifecycleTerms(run *Run, policy termCarryPolicyConfig, opts TermCarryLifecycleOptions, evidence lifecycleEvidence, candidates []lifecycleTermCandidate, failures *lifecycleFailureCollector) []TermCarryLifecycleTerm {
	terms := make([]TermCarryLifecycleTerm, 0, len(candidates))
	for index := range candidates {
		candidate := &candidates[index]
		termID := lifecycleTermID(candidate.participant.venue, candidate.participant.client, candidate.planCreatedAt)
		timeline := replayLifecycleTermFills(policy, candidate, termID, failures)
		term := TermCarryLifecycleTerm{
			TermID: termID, VenueID: candidate.participant.venue, ClientID: candidate.participant.client,
			PolicyVersion: candidate.policyVersion, PlanCreatedAtNano: candidate.planCreatedAt,
			TermEndAtNano: candidate.termEnd, AnalysisDeadlineAtNano: opts.DeadlineAtNano,
			Ownership: endpointAt(timeline[0].at), Activation: endpointStatus(LifecycleNotObserved),
			FirstResidualReduction: endpointStatus(LifecycleNotObserved), Flatness: endpointStatus(LifecycleNotObserved),
		}
		term.Conservation = lifecycleTermConservation(run, policy, candidate, timeline, termID, failures)
		for _, fill := range timeline {
			if fill.spotAfter != 0 && fill.perpAfter == -fill.spotAfter && fill.at < candidate.termEnd {
				term.Activation = endpointAt(fill.at)
				break
			}
		}
		term.PositionAtTermEnd = lifecyclePositionAt(timeline, candidate.termEnd)
		term.PositionAtDeadline = lifecyclePositionAt(timeline, opts.DeadlineAtNano)
		term.TerminalPosition = lifecyclePositionAt(timeline, evidence.observationEnd)
		term.AggressiveEligibility = findLifecycleAggressiveIneligibility(policy, candidate, timeline)
		gradeLifecyclePassiveEvidence(policy, candidate, timeline, evidence, &term, failures)
		gradeLifecycleReductionAndClosure(candidate, timeline, opts.DeadlineAtNano, &term, failures)
		gradeLifecycleFunding(policy, candidate, timeline, evidence, &term, failures)
		validateLifecycleActorPositions(candidate, timeline, termID, failures)
		terms = append(terms, term)
	}
	return terms
}

func replayLifecycleTermFills(policy termCarryPolicyConfig, candidate *lifecycleTermCandidate, termID string, failures *lifecycleFailureCollector) []lifecycleFillEvidence {
	sort.Slice(candidate.fills, func(i, j int) bool {
		if candidate.fills[i].at != candidate.fills[j].at {
			return candidate.fills[i].at < candidate.fills[j].at
		}
		if candidate.fills[i].fill.OrderID != candidate.fills[j].fill.OrderID {
			return candidate.fills[i].fill.OrderID < candidate.fills[j].fill.OrderID
		}
		return candidate.fills[i].fill.TradeID < candidate.fills[j].fill.TradeID
	})
	spot, perp := int64(0), int64(0)
	for index := range candidate.fills {
		fill := &candidate.fills[index]
		fill.spotBefore, fill.perpBefore = spot, perp
		delta := fill.fill.Qty
		if fill.fill.Side == exchange.Sell.String() {
			delta = -delta
		} else if fill.fill.Side != exchange.Buy.String() {
			failures.add(termID, candidate.participant.venue, candidate.participant.client, fill.requestID, "invalid_fill_side")
			continue
		}
		var ok bool
		switch fill.fill.Symbol {
		case policy.SpotSymbol:
			spot, ok = fundingCarryAuditAdd(spot, delta)
		case policy.PerpSymbol:
			perp, ok = fundingCarryAuditAdd(perp, delta)
		default:
			failures.add(termID, candidate.participant.venue, candidate.participant.client, fill.requestID, "invalid_fill_symbol")
			continue
		}
		if !ok {
			failures.add(termID, candidate.participant.venue, candidate.participant.client, fill.requestID, "position_overflow")
		}
		fill.spotAfter, fill.perpAfter = spot, perp
	}
	return candidate.fills
}

func findLifecycleAggressiveIneligibility(policy termCarryPolicyConfig, candidate *lifecycleTermCandidate, timeline []lifecycleFillEvidence) TermCarryLifecycleAggressiveEvidence {
	result := TermCarryLifecycleAggressiveEvidence{Status: LifecycleNotExercised, EffectiveMinimum: lifecycleUnwindMinimum(policy)}
	sort.Slice(candidate.decisions, func(i, j int) bool { return candidate.decisions[i].DecisionTime < candidate.decisions[j].DecisionTime })
	for _, decision := range candidate.decisions {
		if decision.DecisionTime < candidate.termEnd {
			continue
		}
		spot, perp := lifecyclePositionsAt(timeline, decision.DecisionTime, true)
		leg, side, available, present, price, tick, gap := lifecycleAggressiveInputs(policy, decision, spot, perp)
		if gap == 0 || !present || !fundingCarryAuditPositiveGrid(price, tick) {
			continue
		}
		if _, executable := fundingCarryAuditSizedQty(gap, policy.LotQty, available, lifecycleUnwindMinimum(policy)); executable {
			continue
		}
		at := decision.DecisionTime
		result.Status, result.Eligible, result.AtNano, result.Leg, result.Side = LifecycleObserved, false, &at, leg, side
		result.ContraDisplayedQty = available
		return result
	}
	return result
}

func lifecycleAggressiveInputs(policy termCarryPolicyConfig, decision termCarryDecision, spot, perp int64) (string, string, int64, bool, int64, int64, int64) {
	position, leg, tick := perp, "UNWIND_PERP_IOC", policy.PerpTick
	hasBid, bid, bidQty, hasAsk, ask, askQty := decision.HasPerpBid, decision.PerpBid, decision.PerpBidQty, decision.HasPerpAsk, decision.PerpAsk, decision.PerpAskQty
	if perp == 0 {
		position, leg, tick = spot, "UNWIND_SPOT_IOC", policy.SpotTick
		hasBid, bid, bidQty, hasAsk, ask, askQty = decision.HasSpotBid, decision.SpotBid, decision.SpotBidQty, decision.HasSpotAsk, decision.SpotAsk, decision.SpotAskQty
	}
	gap, ok := fundingCarryAuditSub(0, position)
	if !ok || gap == 0 {
		return leg, "", 0, false, 0, tick, 0
	}
	if gap > 0 {
		return leg, exchange.Buy.String(), askQty, hasAsk, ask, tick, gap
	}
	return leg, exchange.Sell.String(), bidQty, hasBid, bid, tick, gap
}

func gradeLifecyclePassiveEvidence(policy termCarryPolicyConfig, candidate *lifecycleTermCandidate, timeline []lifecycleFillEvidence, evidence lifecycleEvidence, term *TermCarryLifecycleTerm, failures *lifecycleFailureCollector) {
	if policy.PassiveExit == nil {
		term.PassiveEligibility = endpointStatus(LifecycleNotApplicable)
		term.PassiveAdmission = endpointStatus(LifecycleNotApplicable)
		term.PassiveFilledQuantity = quantityEndpoint(LifecycleNotApplicable, 0)
		term.Cancellation.Status = LifecycleNotApplicable
		term.DeadlineState = endpointAt(term.AnalysisDeadlineAtNano)
		return
	}
	term.PassiveEligibility = endpointStatus(LifecycleNotExercised)
	term.PassiveAdmission = endpointStatus(LifecycleNotExercised)
	term.PassiveFilledQuantity = quantityEndpoint(LifecycleNotExercised, 0)
	term.Cancellation.Status = LifecycleNotExercised
	if term.AggressiveEligibility.Status != LifecycleObserved {
		term.DeadlineState = lifecycleDeadlineState(candidate, term.AnalysisDeadlineAtNano)
		return
	}
	term.PassiveEligibility = endpointAt(*term.AggressiveEligibility.AtNano)
	term.PassiveFilledQuantity = quantityEndpoint(LifecycleObserved, 0)
	for _, decision := range candidate.decisions {
		if decision.Action != "SUBMIT_UNWIND_PERP_POST_ONLY" && decision.Action != "SUBMIT_UNWIND_SPOT_POST_ONLY" {
			continue
		}
		if err := validateTermCarryPassiveExitSubmission(policy, decision); err != nil {
			failures.add(term.TermID, term.VenueID, term.ClientID, decision.RequestID, err.Error())
		}
		order := buildLifecyclePassiveOrder(decision, timeline, evidence, term, failures)
		term.PassiveOrders = append(term.PassiveOrders, order)
		if order.Admission.Status == LifecycleObserved && term.PassiveAdmission.Status != LifecycleObserved {
			term.PassiveAdmission = order.Admission
		}
		term.PassiveFilledQuantity.Quantity += order.FilledQty
		if order.Cancellation.Status == LifecycleObserved {
			term.Cancellation = order.Cancellation
		}
	}
	term.DeadlineState = lifecycleDeadlineState(candidate, term.AnalysisDeadlineAtNano)
}

func buildLifecyclePassiveOrder(decision termCarryDecision, timeline []lifecycleFillEvidence, evidence lifecycleEvidence, term *TermCarryLifecycleTerm, failures *lifecycleFailureCollector) TermCarryLifecyclePassiveOrder {
	result := TermCarryLifecyclePassiveOrder{
		RequestID: decision.RequestID, Leg: decision.Leg, Side: decision.Side,
		Price: decision.LimitPrice, RequestedQty: decision.RequestedQty,
		Admission:   endpointStatus(LifecycleNotObserved),
		PartialFill: quantityEndpoint(LifecycleObserved, 0), FullFill: quantityEndpoint(LifecycleNotObserved, 0),
		Cancellation: TermCarryLifecycleCancellationEvidence{Status: LifecycleNotObserved},
		Resting:      TermCarryLifecycleRestingEvidence{Status: LifecycleNotObserved},
	}
	key := fundingCarryKey{decision.VenueID, decision.ClientID, decision.RequestID}
	if len(evidence.accepted[key]) != 1 {
		return result
	}
	accepted := evidence.accepted[key][0]
	result.OrderID = accepted.order.OrderID
	result.Admission = endpointAt(accepted.at)
	result.Resting.Status, result.Resting.AcceptedAt = LifecycleObserved, int64Pointer(accepted.at)
	orderKey := fundingCarryOrderKey{decision.VenueID, decision.ClientID, accepted.order.OrderID}
	var terminalAt int64
	for _, fill := range evidence.fills[orderKey] {
		result.FilledQty += fill.fill.Qty
		if result.FilledQty < decision.RequestedQty {
			result.PartialFill.Quantity = result.FilledQty
			result.PartialFill.AtNano = int64Pointer(fill.at)
		}
		if result.FilledQty == decision.RequestedQty && result.FullFill.Status != LifecycleObserved {
			result.FullFill = quantityEndpointAt(LifecycleObserved, result.FilledQty, fill.at)
			terminalAt = fill.at
		}
		if result.FilledQty > decision.RequestedQty {
			failures.add(term.TermID, term.VenueID, term.ClientID, decision.RequestID, "passive_fill_exceeds_request")
		}
	}
	if len(evidence.cancellations[orderKey]) > 0 {
		cancel := evidence.cancellations[orderKey][0]
		result.Cancellation = TermCarryLifecycleCancellationEvidence{
			Status: LifecycleObserved, OrderID: accepted.order.OrderID,
			CancelRequestID: cancel.cancellation.RequestID, AcknowledgedAt: int64Pointer(cancel.at),
		}
		for _, candidateDecision := range evidence.decisions {
			if candidateDecision.VenueID == decision.VenueID && candidateDecision.ClientID == decision.ClientID && candidateDecision.CancelRequestID == cancel.cancellation.RequestID {
				result.Cancellation.RequestedAtNano = int64Pointer(candidateDecision.DecisionTime)
				break
			}
		}
		if terminalAt == 0 || cancel.at < terminalAt {
			terminalAt = cancel.at
		}
	}
	if terminalAt == 0 {
		terminalAt, result.Resting.Censored = evidence.observationEnd, true
	}
	duration, ok := fundingCarryAuditSub(terminalAt, accepted.at)
	if !ok || duration < 0 {
		failures.add(term.TermID, term.VenueID, term.ClientID, decision.RequestID, "negative_resting_duration")
	} else {
		result.Resting.EndedAt = int64Pointer(terminalAt)
		result.Resting.DurationNanos = int64Pointer(duration)
	}
	return result
}

func gradeLifecycleReductionAndClosure(candidate *lifecycleTermCandidate, timeline []lifecycleFillEvidence, deadline int64, term *TermCarryLifecycleTerm, failures *lifecycleFailureCollector) {
	baseline := term.PositionAtTermEnd.ResidualMagnitude
	for _, fill := range timeline {
		if fill.at <= candidate.termEnd {
			continue
		}
		magnitude, ok := lifecycleResidualMagnitude(fill.spotAfter, fill.perpAfter)
		if !ok {
			failures.add(term.TermID, term.VenueID, term.ClientID, fill.requestID, "residual_magnitude_overflow")
			continue
		}
		if term.FirstResidualReduction.Status != LifecycleObserved && magnitude < baseline {
			term.FirstResidualReduction = endpointAt(fill.at)
		}
		if term.Flatness.Status != LifecycleObserved && fill.spotAfter == 0 && fill.perpAfter == 0 {
			term.Flatness = endpointAt(fill.at)
		}
	}
	var closeTimes []int64
	for _, decision := range candidate.decisions {
		if decision.Action == "TERM_CLOSED" {
			closeTimes = append(closeTimes, decision.DecisionTime)
		}
	}
	term.CloseTransitionCount = int64(len(closeTimes))
	if len(closeTimes) == 1 {
		term.CloseTransitionAtNano = int64Pointer(closeTimes[0])
	}
	if len(closeTimes) > 1 {
		failures.add(term.TermID, term.VenueID, term.ClientID, 0, "duplicate_term_closed_transition")
	}
	if len(closeTimes) > 0 && (term.Flatness.Status != LifecycleObserved || closeTimes[0] <= *term.Flatness.AtNano) {
		failures.add(term.TermID, term.VenueID, term.ClientID, 0, "term_closed_without_prior_flatness")
	}
	if term.Flatness.Status == LifecycleObserved {
		for _, fill := range timeline {
			if fill.at > *term.Flatness.AtNano {
				term.MutatedAfterClose = true
				failures.add(term.TermID, term.VenueID, term.ClientID, fill.requestID, "term_mutated_after_flatness")
			}
		}
	}
	term.ProvenClosedByDeadline = term.Flatness.Status == LifecycleObserved && *term.Flatness.AtNano <= deadline && len(closeTimes) == 1 && closeTimes[0] > *term.Flatness.AtNano && closeTimes[0] <= deadline && !term.MutatedAfterClose
	if term.ProvenClosedByDeadline && term.DeadlineState.Status != LifecycleObserved {
		term.DeadlineState = endpointAt(deadline)
	}
}

func gradeLifecycleFunding(policy termCarryPolicyConfig, candidate *lifecycleTermCandidate, timeline []lifecycleFillEvidence, evidence lifecycleEvidence, term *TermCarryLifecycleTerm, failures *lifecycleFailureCollector) {
	settlements := evidence.funding[candidate.participant]
	settlementTimes := make(map[int64]struct{}, len(settlements))
	for _, settlement := range settlements {
		if settlement.at < *term.Ownership.AtNano {
			continue
		}
		settlementTimes[settlement.at] = struct{}{}
		_, perp := lifecyclePositionsAt(timeline, settlement.at, false)
		phase := &term.Funding.BeforeTermEnd
		switch {
		case term.Flatness.Status == LifecycleObserved && settlement.at > *term.Flatness.AtNano:
			phase = &term.Funding.PostClose
			failures.add(term.TermID, term.VenueID, term.ClientID, 0, "funding_attributed_after_close")
		case settlement.at > term.AnalysisDeadlineAtNano && perp != 0:
			phase = &term.Funding.ResidualAfterDeadline
		case settlement.at > term.TermEndAtNano && perp != 0:
			phase = &term.Funding.ResidualBeforeDeadline
		case settlement.at > term.TermEndAtNano:
			failures.add(term.TermID, term.VenueID, term.ClientID, 0, "funding_without_residual_position")
		}
		phase.Settlements++
		phase.NetQuoteDelta += settlement.netQuoteDelta
	}
	if policy.PassiveExit == nil {
		return
	}
	expected := make(map[int64]struct{})
	for _, decision := range candidate.decisions {
		at := decision.FundingNextAt
		if !decision.HasFunding || at <= term.AnalysisDeadlineAtNano || at > evidence.observationEnd {
			continue
		}
		_, perp := lifecyclePositionsAt(timeline, at, false)
		if perp != 0 {
			expected[at] = struct{}{}
		}
	}
	term.Funding.ExpectedAfterDeadline = int64(len(expected))
	for at := range expected {
		if _, found := settlementTimes[at]; !found {
			term.Funding.MissingAfterDeadline++
			failures.add(term.TermID, term.VenueID, term.ClientID, 0, "missing_post_deadline_funding")
		}
	}
}

func validateLifecycleActorPositions(candidate *lifecycleTermCandidate, timeline []lifecycleFillEvidence, termID string, failures *lifecycleFailureCollector) {
	for _, decision := range candidate.decisions {
		spot, perp := lifecyclePositionsAt(timeline, decision.DecisionTime, true)
		if decision.SpotPosition != spot || decision.PerpPosition != perp {
			failures.add(termID, decision.VenueID, decision.ClientID, decision.RequestID, "actor_decision_position_disagreement")
		}
	}
	for _, outcome := range candidate.actorOutcomes {
		if outcome.Event == "ORDER_FILL" {
			matched := false
			for _, fill := range timeline {
				if fill.fill.OrderID == outcome.OrderID && fill.fill.TradeID == outcome.TradeID {
					matched = true
					if outcome.SpotPositionBefore != fill.spotBefore || outcome.PerpPositionBefore != fill.perpBefore || outcome.SpotPositionAfter != fill.spotAfter || outcome.PerpPositionAfter != fill.perpAfter {
						failures.add(termID, outcome.VenueID, outcome.ClientID, outcome.RequestID, "actor_fill_position_disagreement")
					}
					break
				}
			}
			if !matched {
				failures.add(termID, outcome.VenueID, outcome.ClientID, outcome.RequestID, "actor_fill_without_canonical_fill")
			}
			continue
		}
		if outcome.SpotPositionBefore != outcome.SpotPositionAfter || outcome.PerpPositionBefore != outcome.PerpPositionAfter {
			failures.add(termID, outcome.VenueID, outcome.ClientID, outcome.RequestID, "nonfill_actor_outcome_mutated_position")
		}
	}
}

func lifecycleTermConservation(run *Run, policy termCarryPolicyConfig, candidate *lifecycleTermCandidate, timeline []lifecycleFillEvidence, termID string, failures *lifecycleFailureCollector) TermCarryLifecycleConservation {
	result := TermCarryLifecycleConservation{FillChainValid: true}
	for _, fill := range timeline {
		result.CanonicalFillRecords++
		result.CanonicalFilledQty += fill.fill.Qty
	}
	for _, outcome := range candidate.actorOutcomes {
		if outcome.Event == "ORDER_FILL" {
			result.ActorFillRecords++
			result.ActorFilledQty += outcome.Qty
		}
	}
	if result.CanonicalFillRecords != result.ActorFillRecords || result.CanonicalFilledQty != result.ActorFilledQty {
		result.FillChainValid = false
		failures.add(termID, candidate.participant.venue, candidate.participant.client, 0, "fill_conservation_mismatch")
	}
	baseAsset, validBase := termCarryBaseAsset(policy.SpotSymbol)
	if !validBase {
		failures.add(termID, candidate.participant.venue, candidate.participant.client, 0, "invalid_spot_symbol_base_asset")
		return result
	}
	var initialSpot, terminalSpot, terminalPerp int64
	initialFound, terminalSpotFound, terminalPerpFound := false, false, false
	for _, row := range run.Report.InitialAccounts {
		if row.VenueID == candidate.participant.venue && row.ClientID == candidate.participant.client {
			initialSpot, initialFound = termCarrySpotBalance(row.Account.SpotBalances, baseAsset)
		}
	}
	for _, row := range run.Report.TerminalAccounts {
		if row.VenueID != candidate.participant.venue || row.ClientID != candidate.participant.client {
			continue
		}
		terminalSpot, terminalSpotFound = termCarrySpotBalance(row.Account.SpotBalances, baseAsset)
		terminalPerpFound = true
		for _, position := range row.Account.Positions {
			if position.Symbol == policy.PerpSymbol {
				terminalPerp = position.Size
			}
		}
	}
	spotDelta, validSpot := fundingCarryAuditSub(terminalSpot, initialSpot)
	last := timeline[len(timeline)-1]
	result.TerminalSpotAgrees = initialFound && terminalSpotFound && validSpot && spotDelta == last.spotAfter
	result.TerminalPerpAgrees = terminalPerpFound && terminalPerp == last.perpAfter
	if !result.TerminalSpotAgrees {
		failures.add(termID, candidate.participant.venue, candidate.participant.client, 0, "terminal_spot_position_disagreement")
	}
	if !result.TerminalPerpAgrees {
		failures.add(termID, candidate.participant.venue, candidate.participant.client, 0, "terminal_perp_position_disagreement")
	}
	return result
}

func aggregateLifecycleTerms(terms []TermCarryLifecycleTerm) TermCarryLifecycleAggregate {
	result := TermCarryLifecycleAggregate{ExerciseStatus: LifecycleNotExercised}
	for _, term := range terms {
		result.OwnedTerms++
		if term.Activation.Status == LifecycleObserved {
			result.ActivatedTerms++
		}
		if term.AggressiveEligibility.Status == LifecycleObserved {
			result.EligibleTerms++
			if term.ProvenClosedByDeadline {
				result.ProvenClosedEligibleTerms++
			}
		}
		if term.PassiveEligibility.Status == LifecycleObserved {
			result.PassiveEligibleTerms++
		}
		if term.PassiveAdmission.Status == LifecycleObserved {
			result.PassiveAdmittedTerms++
		}
		result.PassiveFilledQuantity += term.PassiveFilledQuantity.Quantity
		result.TerminalResidualMagnitude += term.TerminalPosition.ResidualMagnitude
		result.ResidualFundingSettlements += term.Funding.ResidualBeforeDeadline.Settlements + term.Funding.ResidualAfterDeadline.Settlements
		result.ResidualFundingNetQuoteDelta += term.Funding.ResidualBeforeDeadline.NetQuoteDelta + term.Funding.ResidualAfterDeadline.NetQuoteDelta
	}
	if result.EligibleTerms > 0 {
		result.ExerciseStatus = LifecycleObserved
		fraction := float64(result.ProvenClosedEligibleTerms) / float64(result.EligibleTerms)
		allClosed := result.ProvenClosedEligibleTerms == result.EligibleTerms
		result.ClosureFraction, result.AllEligibleTermsClosedByDeadline = &fraction, &allClosed
	}
	return result
}

func lifecyclePositionAt(timeline []lifecycleFillEvidence, at int64) TermCarryLifecyclePosition {
	spot, perp := lifecyclePositionsAt(timeline, at, true)
	magnitude, _ := lifecycleResidualMagnitude(spot, perp)
	return TermCarryLifecyclePosition{Status: LifecycleObserved, AtNano: at, Spot: spot, Perp: perp, ResidualMagnitude: magnitude}
}

func lifecyclePositionsAt(timeline []lifecycleFillEvidence, at int64, inclusive bool) (int64, int64) {
	spot, perp := int64(0), int64(0)
	for _, fill := range timeline {
		if fill.at > at || (!inclusive && fill.at == at) {
			break
		}
		spot, perp = fill.spotAfter, fill.perpAfter
	}
	return spot, perp
}

func lifecycleResidualMagnitude(spot, perp int64) (int64, bool) {
	spotMagnitude, spotOK := fundingCarryAuditMagnitude(spot)
	perpMagnitude, perpOK := fundingCarryAuditMagnitude(perp)
	if !spotOK || !perpOK {
		return 0, false
	}
	return fundingCarryAuditAdd(spotMagnitude, perpMagnitude)
}

func lifecycleDeadlineState(candidate *lifecycleTermCandidate, deadline int64) TermCarryLifecycleEndpoint {
	for _, decision := range candidate.decisions {
		if decision.DecisionTime < deadline {
			continue
		}
		switch decision.Action {
		case "CANCEL_PASSIVE_EXIT_AT_DEADLINE", "PASSIVE_EXIT_CANCEL_PENDING", "PASSIVE_EXIT_DEADLINE_EXPIRED", "TERM_CLOSED":
			return endpointAt(decision.DecisionTime)
		}
	}
	return endpointStatus(LifecycleNotObserved)
}

func lifecycleUnwindMinimum(policy termCarryPolicyConfig) int64 {
	minimum := policy.MinOrderSize
	if policy.UnwindMinOrderSize != nil && *policy.UnwindMinOrderSize > minimum {
		minimum = *policy.UnwindMinOrderSize
	}
	return minimum
}

func lifecycleTermID(venue string, client uint64, planCreatedAt int64) string {
	if venue == "" || client == 0 || planCreatedAt == 0 {
		return ""
	}
	return fmt.Sprintf("%s/%d/%d", venue, client, planCreatedAt)
}

func endpointStatus(status string) TermCarryLifecycleEndpoint {
	return TermCarryLifecycleEndpoint{Status: status}
}

func endpointAt(at int64) TermCarryLifecycleEndpoint {
	return TermCarryLifecycleEndpoint{Status: LifecycleObserved, AtNano: int64Pointer(at)}
}

func quantityEndpoint(status string, quantity int64) TermCarryLifecycleQuantityEndpoint {
	return TermCarryLifecycleQuantityEndpoint{Status: status, Quantity: quantity}
}

func quantityEndpointAt(status string, quantity, at int64) TermCarryLifecycleQuantityEndpoint {
	return TermCarryLifecycleQuantityEndpoint{Status: status, AtNano: int64Pointer(at), Quantity: quantity}
}
