package analysis

import (
	"math/big"
	"sort"
)

// PerpQuoteReplenishmentAudit independently replays V2-3 P3's confirmed
// own-order residual policy. It reads only persisted actor-receipt evidence
// and independently logged venue lifecycle records; it does not inspect a
// maker, book, or maker_state target.
type PerpQuoteReplenishmentAudit struct {
	Decisions         int64            `json:"decisions"`
	EnabledDecisions  int64            `json:"enabled_decisions"`
	DisabledDecisions int64            `json:"disabled_decisions"`
	RefreshDue        int64            `json:"refresh_due"`
	NoRefresh         int64            `json:"no_refresh"`
	Accepted          int64            `json:"accepted"`
	Rejected          int64            `json:"rejected"`
	HorizonCensored   int64            `json:"horizon_censored"`
	LifecycleRows     int64            `json:"lifecycle_rows"`
	ActionCounts      map[string]int64 `json:"action_counts,omitempty"`

	InvalidDecisionRecords  int64 `json:"invalid_decision_records"`
	InvalidLifecycleRecords int64 `json:"invalid_lifecycle_records"`
	LifecycleMismatches     int64 `json:"lifecycle_mismatches"`
	ThresholdMismatches     int64 `json:"threshold_mismatches"`
	MissingOutcomes         int64 `json:"missing_outcomes"`
	DuplicateOutcomes       int64 `json:"duplicate_outcomes"`
	OutcomeFieldMismatches  int64 `json:"outcome_field_mismatches"`
	CensoredDeliveries      int64 `json:"censored_deliveries"`
	UnexpectedRefreshes     int64 `json:"unexpected_refreshes"`
	MissingRefreshes        int64 `json:"missing_refreshes"`

	Checks []PerpQuoteReplenishmentCheck `json:"checks,omitempty"`
	Valid  bool                          `json:"valid"`
}

// PerpQuoteReplenishmentCheck identifies a single evidence or policy failure
// without hiding the causal location inside aggregate counters.
type PerpQuoteReplenishmentCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id,omitempty"`
	OrderID   uint64 `json:"order_id,omitempty"`
	Failure   string `json:"failure"`
}

type perpQuoteReplenishmentDecision struct {
	VenueID             string `json:"venue_id"`
	Maker               string `json:"maker"`
	ClientID            uint64 `json:"client_id"`
	Symbol              string `json:"symbol"`
	DecisionTime        int64  `json:"decision_time"`
	Enabled             bool   `json:"enabled"`
	ThresholdBps        int64  `json:"threshold_bps"`
	BidOrderID          uint64 `json:"bid_order_id"`
	AskOrderID          uint64 `json:"ask_order_id"`
	BidTargetQty        int64  `json:"bid_target_qty"`
	AskTargetQty        int64  `json:"ask_target_qty"`
	BidKnownRestingQty  int64  `json:"bid_known_resting_qty"`
	AskKnownRestingQty  int64  `json:"ask_known_resting_qty"`
	BidReplenishmentDue bool   `json:"bid_replenishment_due"`
	AskReplenishmentDue bool   `json:"ask_replenishment_due"`
	RefreshDue          bool   `json:"refresh_due"`
	Reason              string `json:"reason"`
	BidPrice            int64  `json:"bid_price"`
	AskPrice            int64  `json:"ask_price"`
	BidRequestID        uint64 `json:"bid_request_id"`
	AskRequestID        uint64 `json:"ask_request_id"`
	OutcomeExpectation  string `json:"outcome_expectation"`
	CensorReason        string `json:"censor_reason"`
}

type perpQuoteReplenishmentLifecycle struct {
	VenueID           string `json:"venue_id"`
	Maker             string `json:"maker"`
	ClientID          uint64 `json:"client_id"`
	Symbol            string `json:"symbol"`
	ObservedAt        int64  `json:"observed_at"`
	ExchangeTimestamp int64  `json:"exchange_timestamp"`
	Transition        string `json:"transition"`
	Side              string `json:"side"`
	RequestID         uint64 `json:"request_id"`
	OrderID           uint64 `json:"order_id"`
	Qty               int64  `json:"qty"`
	TargetQty         int64  `json:"target_qty"`
	KnownRestingQty   int64  `json:"known_resting_qty"`
}

type perpQuoteVenueOrder struct {
	At          int64
	Ordinal     int64
	OrderID     uint64 `json:"order_id"`
	RequestID   uint64 `json:"request_id"`
	ClientID    uint64 `json:"client_id"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	PostOnly    bool   `json:"post_only"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
}

type perpQuoteVenueFill struct {
	At      int64
	Ordinal int64
	OrderID uint64 `json:"order_id"`
	Symbol  string `json:"symbol"`
	Side    string `json:"side"`
	Qty     int64  `json:"qty"`
}

type perpQuoteVenueCancel struct {
	At      int64
	Ordinal int64
	OrderID uint64 `json:"order_id"`
}

type perpQuoteParticipant struct {
	venue  string
	client uint64
}

type perpQuoteOrderKey struct {
	venue string
	order uint64
}

type perpQuoteRequestKey struct {
	venue   string
	client  uint64
	request uint64
}

type perpQuoteRecord struct {
	at        int64
	ordinal   int64
	decision  *perpQuoteReplenishmentDecision
	lifecycle *perpQuoteReplenishmentLifecycle
}

type perpQuoteState struct {
	side    string
	request uint64
	target  int64
	resting int64
}

// MeasurePerpQuoteReplenishment validates the declared P3 local policy. A
// passing result means the actor's own received lifecycle can be reconstructed
// and predicted from raw evidence; it says nothing about price stability or
// long-horizon exit capacity.
func (r *Run) MeasurePerpQuoteReplenishment() (*PerpQuoteReplenishmentAudit, error) {
	result := &PerpQuoteReplenishmentAudit{ActionCounts: make(map[string]int64)}
	records := make(map[perpQuoteParticipant][]perpQuoteRecord)
	accepted := make(map[perpQuoteRequestKey][]perpQuoteVenueOrder)
	rejected := make(map[perpQuoteRequestKey][]perpQuoteVenueOrder)
	fills := make(map[perpQuoteOrderKey][]perpQuoteVenueFill)
	cancels := make(map[perpQuoteOrderKey][]perpQuoteVenueCancel)
	addCheck := func(venue string, client, request, order uint64, failure string) {
		result.Checks = append(result.Checks, PerpQuoteReplenishmentCheck{VenueID: venue, ClientID: client, RequestID: request, OrderID: order, Failure: failure})
	}

	err := r.Scan(ScanOptions{Events: []string{
		"perp_quote_replenishment_decision", "perp_quote_replenishment_lifecycle", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled",
	}, Workers: 1}, func(event Event) {
		owner := perpQuoteParticipant{venue: event.VenueID, client: event.ClientID}
		switch event.Name {
		case "perp_quote_replenishment_decision":
			var decision perpQuoteReplenishmentDecision
			if event.Decode(&decision) != nil || !validPerpQuoteDecisionIdentity(r, event, decision) {
				result.InvalidDecisionRecords++
				addCheck(event.VenueID, event.ClientID, 0, 0, "invalid_decision_record")
				return
			}
			records[owner] = append(records[owner], perpQuoteRecord{at: decision.DecisionTime, ordinal: event.Ordinal, decision: &decision})
		case "perp_quote_replenishment_lifecycle":
			var lifecycle perpQuoteReplenishmentLifecycle
			if event.Decode(&lifecycle) != nil || !validPerpQuoteLifecycleIdentity(r, event, lifecycle) {
				result.InvalidLifecycleRecords++
				addCheck(event.VenueID, event.ClientID, lifecycle.RequestID, lifecycle.OrderID, "invalid_lifecycle_record")
				return
			}
			result.LifecycleRows++
			records[owner] = append(records[owner], perpQuoteRecord{at: lifecycle.ObservedAt, ordinal: event.Ordinal, lifecycle: &lifecycle})
		case "OrderAccepted", "OrderRejected":
			var order perpQuoteVenueOrder
			if event.Decode(&order) != nil || order.RequestID == 0 || r.Role(event.VenueID, event.ClientID) != "perp_maker" {
				return
			}
			order.At, order.Ordinal = event.SimTS, event.Ordinal
			if order.ClientID == 0 {
				order.ClientID = event.ClientID
			}
			if order.Symbol == "" {
				order.Symbol = event.Symbol
			}
			key := perpQuoteRequestKey{venue: event.VenueID, client: event.ClientID, request: order.RequestID}
			if event.Name == "OrderAccepted" {
				accepted[key] = append(accepted[key], order)
			} else {
				rejected[key] = append(rejected[key], order)
			}
		case "OrderFill":
			var fill perpQuoteVenueFill
			if event.Decode(&fill) != nil || fill.OrderID == 0 || fill.Qty <= 0 || r.Role(event.VenueID, event.ClientID) != "perp_maker" {
				return
			}
			fill.At, fill.Ordinal = event.SimTS, event.Ordinal
			if fill.Symbol == "" {
				fill.Symbol = event.Symbol
			}
			fills[perpQuoteOrderKey{venue: event.VenueID, order: fill.OrderID}] = append(fills[perpQuoteOrderKey{venue: event.VenueID, order: fill.OrderID}], fill)
		case "OrderCancelled":
			var cancellation perpQuoteVenueCancel
			if event.Decode(&cancellation) != nil || cancellation.OrderID == 0 || r.Role(event.VenueID, event.ClientID) != "perp_maker" {
				return
			}
			cancellation.At, cancellation.Ordinal = event.SimTS, event.Ordinal
			cancels[perpQuoteOrderKey{venue: event.VenueID, order: cancellation.OrderID}] = append(cancels[perpQuoteOrderKey{venue: event.VenueID, order: cancellation.OrderID}], cancellation)
		}
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		result.InvalidDecisionRecords++
		addCheck("", 0, 0, 0, "missing_replenishment_evidence")
	}
	for owner, items := range records {
		sort.Slice(items, func(i, j int) bool {
			if items[i].at != items[j].at {
				return items[i].at < items[j].at
			}
			// A delivery observed at the same simulated timestamp is known before
			// a quote decision in the runner's actor-inbox phase.
			if items[i].lifecycle != nil && items[j].decision != nil {
				return true
			}
			if items[i].decision != nil && items[j].lifecycle != nil {
				return false
			}
			return items[i].ordinal < items[j].ordinal
		})
		states := make(map[uint64]perpQuoteState)
		for _, item := range items {
			if item.lifecycle != nil {
				auditPerpQuoteLifecycle(item.lifecycle, owner, states, accepted, rejected, fills, cancels, result, addCheck)
				continue
			}
			auditPerpQuoteDecision(item.decision, owner, states, accepted, rejected, result, addCheck)
		}
	}
	sort.Slice(result.Checks, func(i, j int) bool {
		left, right := result.Checks[i], result.Checks[j]
		if left.VenueID != right.VenueID {
			return left.VenueID < right.VenueID
		}
		if left.ClientID != right.ClientID {
			return left.ClientID < right.ClientID
		}
		if left.RequestID != right.RequestID {
			return left.RequestID < right.RequestID
		}
		if left.OrderID != right.OrderID {
			return left.OrderID < right.OrderID
		}
		return left.Failure < right.Failure
	})
	result.Valid = result.Decisions > 0 && result.InvalidDecisionRecords == 0 && result.InvalidLifecycleRecords == 0 && result.LifecycleMismatches == 0 && result.ThresholdMismatches == 0 && result.MissingOutcomes == 0 && result.DuplicateOutcomes == 0 && result.OutcomeFieldMismatches == 0 && result.CensoredDeliveries == 0 && result.UnexpectedRefreshes == 0 && result.MissingRefreshes == 0
	return result, nil
}

func validPerpQuoteDecisionIdentity(run *Run, event Event, decision perpQuoteReplenishmentDecision) bool {
	return decision.VenueID == event.VenueID && decision.ClientID == event.ClientID && decision.Maker == "perp_maker" && decision.Symbol == "ABC-PERP" && decision.DecisionTime > 0 && run.Role(event.VenueID, event.ClientID) == "perp_maker"
}

func validPerpQuoteLifecycleIdentity(run *Run, event Event, lifecycle perpQuoteReplenishmentLifecycle) bool {
	if lifecycle.VenueID != event.VenueID || lifecycle.ClientID != event.ClientID || lifecycle.Maker != "perp_maker" || lifecycle.Symbol != "ABC-PERP" || lifecycle.ObservedAt <= 0 || run.Role(event.VenueID, event.ClientID) != "perp_maker" {
		return false
	}
	return lifecycle.Side == "BUY" || lifecycle.Side == "SELL"
}

func auditPerpQuoteLifecycle(lifecycle *perpQuoteReplenishmentLifecycle, owner perpQuoteParticipant, states map[uint64]perpQuoteState, accepted, rejected map[perpQuoteRequestKey][]perpQuoteVenueOrder, fills map[perpQuoteOrderKey][]perpQuoteVenueFill, cancels map[perpQuoteOrderKey][]perpQuoteVenueCancel, result *PerpQuoteReplenishmentAudit, addCheck func(string, uint64, uint64, uint64, string)) {
	requestKey := perpQuoteRequestKey{venue: owner.venue, client: owner.client, request: lifecycle.RequestID}
	orderKey := perpQuoteOrderKey{venue: owner.venue, order: lifecycle.OrderID}
	fail := func(name string) {
		result.LifecycleMismatches++
		addCheck(owner.venue, owner.client, lifecycle.RequestID, lifecycle.OrderID, name)
	}
	switch lifecycle.Transition {
	case "ACKNOWLEDGED":
		rows := accepted[requestKey]
		if len(rows) != 1 || lifecycle.OrderID == 0 || lifecycle.TargetQty <= 0 || lifecycle.KnownRestingQty != lifecycle.TargetQty || !perpQuoteAcceptedMatchesLifecycle(rows, lifecycle) {
			fail("acknowledgement_does_not_match_venue_order")
			return
		}
		if rows[0].At > lifecycle.ObservedAt || states[lifecycle.OrderID].target != 0 {
			fail("invalid_or_future_acknowledgement")
			return
		}
		states[lifecycle.OrderID] = perpQuoteState{side: lifecycle.Side, request: lifecycle.RequestID, target: lifecycle.TargetQty, resting: lifecycle.KnownRestingQty}
	case "REJECTED":
		rows := rejected[requestKey]
		if len(rows) != 1 || lifecycle.OrderID != 0 || lifecycle.Qty != 0 || lifecycle.KnownRestingQty != 0 || lifecycle.TargetQty <= 0 || rows[0].At > lifecycle.ObservedAt {
			fail("rejection_does_not_match_venue_outcome")
		}
	case "PARTIAL_FILL", "FULL_FILL":
		state, found := states[lifecycle.OrderID]
		if !found || lifecycle.Qty <= 0 || lifecycle.TargetQty != state.target || lifecycle.Side != state.side || lifecycle.ExchangeTimestamp <= 0 {
			fail("fill_without_matching_local_quote")
			return
		}
		if !perpQuoteHasVenueFill(fills[orderKey], lifecycle, owner) || lifecycle.ExchangeTimestamp > lifecycle.ObservedAt {
			fail("fill_does_not_match_venue_or_is_future")
			return
		}
		remaining := state.resting - lifecycle.Qty
		if lifecycle.Transition == "PARTIAL_FILL" {
			if remaining <= 0 || lifecycle.KnownRestingQty != remaining {
				fail("partial_fill_residual_mismatch")
				return
			}
			state.resting = remaining
			states[lifecycle.OrderID] = state
		} else {
			if remaining != 0 || lifecycle.KnownRestingQty != 0 {
				fail("full_fill_residual_mismatch")
				return
			}
			delete(states, lifecycle.OrderID)
		}
	case "CANCELLED":
		state, found := states[lifecycle.OrderID]
		if !found || lifecycle.Qty != 0 || lifecycle.TargetQty != state.target || lifecycle.Side != state.side || lifecycle.KnownRestingQty != 0 || !perpQuoteHasVenueCancellation(cancels[orderKey], lifecycle.ObservedAt) {
			fail("cancellation_does_not_match_venue_or_local_quote")
			return
		}
		delete(states, lifecycle.OrderID)
	default:
		fail("unknown_lifecycle_transition")
	}
}

func auditPerpQuoteDecision(decision *perpQuoteReplenishmentDecision, owner perpQuoteParticipant, states map[uint64]perpQuoteState, accepted, rejected map[perpQuoteRequestKey][]perpQuoteVenueOrder, result *PerpQuoteReplenishmentAudit, addCheck func(string, uint64, uint64, uint64, string)) {
	result.Decisions++
	result.ActionCounts[decision.Reason]++
	if decision.Enabled {
		result.EnabledDecisions++
	} else {
		result.DisabledDecisions++
	}
	failThreshold := func(name string) {
		result.ThresholdMismatches++
		addCheck(owner.venue, owner.client, 0, 0, name)
	}
	bid, bidFound := states[decision.BidOrderID]
	ask, askFound := states[decision.AskOrderID]
	if !bidFound || !askFound || bid.side != "BUY" || ask.side != "SELL" || decision.BidTargetQty != bid.target || decision.AskTargetQty != ask.target || decision.BidKnownRestingQty != bid.resting || decision.AskKnownRestingQty != ask.resting || decision.BidPrice <= 0 || decision.AskPrice <= decision.BidPrice {
		failThreshold("decision_does_not_match_confirmed_local_frontier")
		return
	}
	if decision.Enabled != (decision.ThresholdBps == 5_000) {
		failThreshold("unregistered_enable_or_threshold")
		return
	}
	bidDue := decision.Enabled && belowPerpQuoteFraction(bid.resting, bid.target, decision.ThresholdBps)
	askDue := decision.Enabled && belowPerpQuoteFraction(ask.resting, ask.target, decision.ThresholdBps)
	refreshDue := bidDue || askDue
	if decision.BidReplenishmentDue != bidDue || decision.AskReplenishmentDue != askDue || decision.RefreshDue != refreshDue || decision.Reason != perpQuoteReason(decision.Enabled, bidDue, askDue) {
		failThreshold("threshold_or_reason_mismatch")
		return
	}
	if !refreshDue {
		result.NoRefresh++
		if decision.BidRequestID != 0 || decision.AskRequestID != 0 || decision.OutcomeExpectation != "NO_VENUE_REQUEST" || decision.CensorReason != "" {
			result.UnexpectedRefreshes++
			addCheck(owner.venue, owner.client, decision.BidRequestID, 0, "non_due_decision_submitted_request")
		}
		return
	}
	result.RefreshDue++
	if decision.BidRequestID == 0 || decision.AskRequestID == 0 || decision.BidRequestID == decision.AskRequestID {
		result.MissingRefreshes++
		addCheck(owner.venue, owner.client, 0, 0, "due_decision_missing_request_ids")
		return
	}
	if decision.OutcomeExpectation == "SIMULATION_HORIZON_CENSORED" && decision.CensorReason == "terminal_horizon_before_venue_ingress" {
		if len(accepted[perpQuoteRequestKey{owner.venue, owner.client, decision.BidRequestID}])+len(rejected[perpQuoteRequestKey{owner.venue, owner.client, decision.BidRequestID}])+len(accepted[perpQuoteRequestKey{owner.venue, owner.client, decision.AskRequestID}])+len(rejected[perpQuoteRequestKey{owner.venue, owner.client, decision.AskRequestID}]) != 0 {
			result.CensoredDeliveries += 2
			addCheck(owner.venue, owner.client, decision.BidRequestID, 0, "terminal_censored_request_delivered")
		}
		result.HorizonCensored++
		return
	}
	if decision.OutcomeExpectation != "VENUE_OUTCOME_REQUIRED" || decision.CensorReason != "" {
		result.OutcomeFieldMismatches++
		addCheck(owner.venue, owner.client, decision.BidRequestID, 0, "invalid_refresh_outcome_expectation")
		return
	}
	auditPerpQuoteRequestOutcome(decision, owner, decision.BidRequestID, "BUY", decision.BidPrice, decision.BidTargetQty, accepted, rejected, result, addCheck)
	auditPerpQuoteRequestOutcome(decision, owner, decision.AskRequestID, "SELL", decision.AskPrice, decision.AskTargetQty, accepted, rejected, result, addCheck)
	// This is the actor's declared immediate state transition: it sends cancel
	// requests and clears both current IDs before new acknowledgements arrive.
	delete(states, decision.BidOrderID)
	delete(states, decision.AskOrderID)
}

func auditPerpQuoteRequestOutcome(decision *perpQuoteReplenishmentDecision, owner perpQuoteParticipant, request uint64, side string, price, qty int64, accepted, rejected map[perpQuoteRequestKey][]perpQuoteVenueOrder, result *PerpQuoteReplenishmentAudit, addCheck func(string, uint64, uint64, uint64, string)) {
	key := perpQuoteRequestKey{venue: owner.venue, client: owner.client, request: request}
	acceptRows, rejectRows := accepted[key], rejected[key]
	if len(acceptRows)+len(rejectRows) == 0 {
		result.MissingOutcomes++
		addCheck(owner.venue, owner.client, request, 0, "missing_refresh_request_outcome")
		return
	}
	if len(acceptRows)+len(rejectRows) != 1 {
		result.DuplicateOutcomes++
		addCheck(owner.venue, owner.client, request, 0, "duplicate_refresh_request_outcome")
		return
	}
	if len(acceptRows) == 1 {
		result.Accepted++
		order := acceptRows[0]
		if order.Symbol != "ABC-PERP" || order.Side != side || order.Type != "LIMIT" || order.TimeInForce != "GTC" || order.PostOnly || order.Price != price || order.Qty != qty || order.OrderID == 0 {
			result.OutcomeFieldMismatches++
			addCheck(owner.venue, owner.client, request, order.OrderID, "refresh_accepted_order_fields_mismatch")
		}
		return
	}
	result.Rejected++
	if rejectRows[0].At < decision.DecisionTime {
		result.OutcomeFieldMismatches++
		addCheck(owner.venue, owner.client, request, 0, "refresh_rejection_precedes_decision")
	}
}

func perpQuoteAcceptedMatchesLifecycle(rows []perpQuoteVenueOrder, lifecycle *perpQuoteReplenishmentLifecycle) bool {
	if len(rows) != 1 {
		return false
	}
	order := rows[0]
	return order.OrderID == lifecycle.OrderID && order.Symbol == "ABC-PERP" && order.Side == lifecycle.Side && order.Type == "LIMIT" && order.TimeInForce == "GTC" && !order.PostOnly && order.Qty == lifecycle.TargetQty && order.Qty > 0
}

func perpQuoteHasVenueFill(rows []perpQuoteVenueFill, lifecycle *perpQuoteReplenishmentLifecycle, owner perpQuoteParticipant) bool {
	matched := 0
	for _, row := range rows {
		if row.At == lifecycle.ExchangeTimestamp && row.At <= lifecycle.ObservedAt && row.Symbol == "ABC-PERP" && row.Side == lifecycle.Side && row.Qty == lifecycle.Qty {
			matched++
		}
	}
	return matched == 1
}

func perpQuoteHasVenueCancellation(rows []perpQuoteVenueCancel, observedAt int64) bool {
	matched := 0
	for _, row := range rows {
		if row.At <= observedAt {
			matched++
		}
	}
	return matched == 1
}

func belowPerpQuoteFraction(resting, target, bps int64) bool {
	if resting < 0 || target <= 0 || bps <= 0 {
		return false
	}
	left := new(big.Int).Mul(big.NewInt(resting), big.NewInt(10_000))
	right := new(big.Int).Mul(big.NewInt(target), big.NewInt(bps))
	return left.Cmp(right) < 0
}

func perpQuoteReason(enabled, bidDue, askDue bool) string {
	if !enabled {
		return "POLICY_DISABLED"
	}
	switch {
	case bidDue && askDue:
		return "BOTH_BELOW_THRESHOLD"
	case bidDue:
		return "BID_BELOW_THRESHOLD"
	case askDue:
		return "ASK_BELOW_THRESHOLD"
	default:
		return "ABOVE_THRESHOLD"
	}
}
