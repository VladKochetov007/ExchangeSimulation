package analysis

import (
	"fmt"
	"strings"
	"sync"
)

// OptionLiabilityAudit is an independent audit of the O2 finite-liability
// participant. It joins actor intent to canonical venue outcomes by request,
// order, and trade identity; the actor's own counters are never trusted as
// execution evidence.
type OptionLiabilityAudit struct {
	Role                  string           `json:"role"`
	Participants          int              `json:"participants"`
	ParticipantsByVenue   map[string]int   `json:"participants_by_venue"`
	Decisions             int              `json:"decisions"`
	DecisionsByVenue      map[string]int   `json:"decisions_by_venue"`
	SubmitDecisions       int              `json:"submit_decisions"`
	DeferredDecisions     int              `json:"deferred_decisions"`
	Accepted              int              `json:"accepted"`
	Rejected              int              `json:"rejected"`
	CanonicalFills        int              `json:"canonical_fills"`
	CanonicalFillsByVenue map[string]int   `json:"canonical_fills_by_venue"`
	ActorFills            int              `json:"actor_fills"`
	ActorFillsByVenue     map[string]int   `json:"actor_fills_by_venue"`
	FilledQty             int64            `json:"filled_qty"`
	FilledQtyByVenue      map[string]int64 `json:"filled_qty_by_venue"`
	FutureObservationUse  int              `json:"future_observation_use"`
	DecodeErrors          int              `json:"decode_errors"`
	MissingOutcomes       int              `json:"missing_outcomes"`
	DuplicateOutcomes     int              `json:"duplicate_outcomes"`
	OrphanOutcomes        int              `json:"orphan_outcomes"`
	OutcomeMismatches     int              `json:"outcome_mismatches"`
	InvalidDecisions      int              `json:"invalid_decisions"`
	TargetReached         bool             `json:"target_reached"`
	Valid                 bool             `json:"valid"`
}

type optionLiabilityDecisionEvidence struct {
	ClientID             uint64 `json:"client_id"`
	DecisionTime         int64  `json:"decision_time"`
	Action               string `json:"action"`
	Reason               string `json:"reason"`
	TargetQty            int64  `json:"target_qty"`
	PositionBefore       int64  `json:"position_before"`
	OptionSymbol         string `json:"option_symbol"`
	HasAsk               bool   `json:"has_ask"`
	AskPrice             int64  `json:"ask_price"`
	AskSourceTime        int64  `json:"ask_source_time"`
	AskReceivedAt        int64  `json:"ask_received_at"`
	UnderlyingSourceTime int64  `json:"underlying_source_time"`
	UnderlyingReceivedAt int64  `json:"underlying_received_at"`
	RequestID            uint64 `json:"request_id"`
	RequestedQty         int64  `json:"requested_qty"`
	SideEvidence         string `json:"side"`
	OrderType            string `json:"order_type"`
	TimeInForce          string `json:"time_in_force"`
}

type optionLiabilityFillEvidence struct {
	OrderID uint64 `json:"order_id"`
	TradeID uint64 `json:"trade_id"`
	Qty     int64  `json:"qty"`
	Price   int64  `json:"price"`
	Side    string `json:"side"`
}

type optionOrderAcceptedEvidence struct {
	RequestID uint64 `json:"request_id"`
	OrderID   uint64 `json:"order_id"`
}

type optionOrderRejectedEvidence struct {
	RequestID uint64 `json:"request_id"`
}

type optionCanonicalFillEvidence struct {
	OrderID uint64 `json:"order_id"`
	TradeID uint64 `json:"trade_id"`
	Qty     int64  `json:"qty"`
	Price   int64  `json:"price"`
	Side    string `json:"side"`
}

type optionP6FillKey struct {
	VenueID  string
	ClientID uint64
	OrderID  uint64
	TradeID  uint64
	Qty      int64
	Price    int64
	Side     string
}

type optionP6RequestKey struct {
	VenueID   string
	ClientID  uint64
	RequestID uint64
}

// MeasureOptionLiability audits persisted O2 decision/fill rows and ordinary
// venue events. A zero decision count is intentionally not treated as a pass;
// callers classify inactive O0/O1 stages separately as NOT APPLICABLE.
func (r *Run) MeasureOptionLiability() (*OptionLiabilityAudit, error) {
	const role = "option_liability_user"
	result := &OptionLiabilityAudit{
		Role:                  role,
		ParticipantsByVenue:   make(map[string]int),
		DecisionsByVenue:      make(map[string]int),
		CanonicalFillsByVenue: make(map[string]int),
		ActorFillsByVenue:     make(map[string]int),
		FilledQtyByVenue:      make(map[string]int64),
	}
	decisionByRequest := make(map[optionP6RequestKey]optionLiabilityDecisionEvidence)
	acceptedByRequest := make(map[optionP6RequestKey]optionOrderAcceptedEvidence)
	acceptedCounts := make(map[optionP6RequestKey]int)
	rejectedByRequest := make(map[optionP6RequestKey]int)
	canonicalFills := make(map[optionP6FillKey]int)
	actorFills := make(map[optionP6FillKey]int)
	participants := make(map[Participant]struct{})
	var targetQty int64
	var mu sync.Mutex

	err := r.Scan(ScanOptions{Events: []string{
		"option_liability_user_decision", "option_liability_user_fill",
		"OrderAccepted", "OrderRejected", "OrderFill",
	}}, func(event Event) {
		if RoleGroup(r.Role(event.VenueID, event.ClientID)) != role {
			return
		}
		mu.Lock()
		participant := Participant{event.VenueID, event.ClientID}
		if _, seen := participants[participant]; !seen {
			participants[participant] = struct{}{}
			result.ParticipantsByVenue[event.VenueID]++
		}
		mu.Unlock()
		switch event.Name {
		case "option_liability_user_decision":
			var row optionLiabilityDecisionEvidence
			if err := event.Decode(&row); err != nil {
				mu.Lock()
				result.DecodeErrors++
				mu.Unlock()
				return
			}
			mu.Lock()
			result.Decisions++
			result.DecisionsByVenue[event.VenueID]++
			if row.Action == "SUBMIT_PUT_IOC" {
				result.SubmitDecisions++
				if targetQty == 0 {
					targetQty = row.TargetQty
				}
				if row.RequestID == 0 || row.RequestedQty <= 0 || row.OptionSymbol == "" || row.SideEvidence != "BUY" || row.OrderType != "LIMIT" || row.TimeInForce != "IOC" || !row.HasAsk || row.AskPrice <= 0 {
					result.InvalidDecisions++
				}
				decisionByRequest[optionP6RequestKey{VenueID: event.VenueID, ClientID: event.ClientID, RequestID: row.RequestID}] = row
			} else {
				result.DeferredDecisions++
			}
			if (row.AskSourceTime != 0 && row.AskSourceTime > row.DecisionTime) ||
				(row.AskReceivedAt != 0 && row.AskReceivedAt > row.DecisionTime) ||
				(row.UnderlyingSourceTime != 0 && row.UnderlyingSourceTime > row.DecisionTime) ||
				(row.UnderlyingReceivedAt != 0 && row.UnderlyingReceivedAt > row.DecisionTime) {
				result.FutureObservationUse++
			}
			mu.Unlock()
		case "option_liability_user_fill":
			var row optionLiabilityFillEvidence
			if err := event.Decode(&row); err != nil {
				mu.Lock()
				result.DecodeErrors++
				mu.Unlock()
				return
			}
			mu.Lock()
			result.ActorFills++
			result.FilledQty += row.Qty
			result.ActorFillsByVenue[event.VenueID]++
			result.FilledQtyByVenue[event.VenueID] += row.Qty
			actorFills[optionP6FillKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: row.OrderID, TradeID: row.TradeID, Qty: row.Qty, Price: row.Price, Side: row.Side}]++
			mu.Unlock()
		case "OrderAccepted":
			var row optionOrderAcceptedEvidence
			if err := event.Decode(&row); err != nil {
				mu.Lock()
				result.DecodeErrors++
				mu.Unlock()
				return
			}
			mu.Lock()
			key := optionP6RequestKey{VenueID: event.VenueID, ClientID: event.ClientID, RequestID: row.RequestID}
			acceptedByRequest[key] = row
			acceptedCounts[key]++
			mu.Unlock()
		case "OrderRejected":
			var row optionOrderRejectedEvidence
			if err := event.Decode(&row); err != nil {
				mu.Lock()
				result.DecodeErrors++
				mu.Unlock()
				return
			}
			mu.Lock()
			rejectedByRequest[optionP6RequestKey{VenueID: event.VenueID, ClientID: event.ClientID, RequestID: row.RequestID}]++
			mu.Unlock()
		case "OrderFill":
			var row optionCanonicalFillEvidence
			if err := event.Decode(&row); err != nil {
				mu.Lock()
				result.DecodeErrors++
				mu.Unlock()
				return
			}
			mu.Lock()
			result.CanonicalFills++
			result.CanonicalFillsByVenue[event.VenueID]++
			canonicalFills[optionP6FillKey{VenueID: event.VenueID, ClientID: event.ClientID, OrderID: row.OrderID, TradeID: row.TradeID, Qty: row.Qty, Price: row.Price, Side: row.Side}]++
			mu.Unlock()
		}
	})
	if err != nil {
		return nil, err
	}
	result.Participants = len(participants)
	for requestID := range decisionByRequest {
		if acceptedCounts[requestID] > 0 && acceptedByRequest[requestID].OrderID != 0 {
			result.Accepted++
		}
		if rejectedByRequest[requestID] > 0 {
			result.Rejected += rejectedByRequest[requestID]
		}
	}
	for requestID := range decisionByRequest {
		acceptedCount := acceptedCounts[requestID]
		rejected := rejectedByRequest[requestID]
		if (acceptedCount == 0) == (rejected == 0) {
			result.MissingOutcomes++
		}
		if acceptedCount > 1 {
			result.DuplicateOutcomes += acceptedCount - 1
		}
		if rejected > 1 {
			result.DuplicateOutcomes += rejected - 1
		}
	}
	for key, count := range acceptedCounts {
		if _, ok := decisionByRequest[key]; !ok {
			result.OrphanOutcomes += count
		}
	}
	for key, count := range rejectedByRequest {
		if _, ok := decisionByRequest[key]; !ok {
			result.OrphanOutcomes += count
		}
	}
	for key, count := range actorFills {
		if canonicalFills[key] < count {
			result.OutcomeMismatches += count - canonicalFills[key]
		}
	}
	for key, count := range canonicalFills {
		if actorFills[key] < count {
			result.OutcomeMismatches += count - actorFills[key]
		}
	}
	result.TargetReached = targetQty > 0 && result.FilledQty >= targetQty
	result.Valid = result.Decisions > 0 && result.SubmitDecisions > 0 && result.DecodeErrors == 0 && result.FutureObservationUse == 0 && result.InvalidDecisions == 0 && result.MissingOutcomes == 0 && result.DuplicateOutcomes == 0 && result.OrphanOutcomes == 0 && result.OutcomeMismatches == 0 && result.ActorFills == result.CanonicalFills
	return result, nil
}

func (a OptionLiabilityAudit) String() string {
	return fmt.Sprintf("option liability decisions=%d submits=%d fills=%d valid=%t", a.Decisions, a.SubmitDecisions, a.ActorFills, a.Valid)
}

// OptionRoleActivity is an execution-side activation audit for an explicit
// option participant. Canonical accepted/rejected/fill records are the source
// of truth; this does not trust an actor counter or model intention.
type OptionRoleActivity struct {
	Role         string                             `json:"role"`
	Participants int                                `json:"participants"`
	Decisions    int                                `json:"decisions"`
	Accepted     int                                `json:"accepted"`
	Rejected     int                                `json:"rejected"`
	Fills        int                                `json:"fills"`
	FilledQty    int64                              `json:"filled_qty"`
	ByVenue      map[string]OptionRoleVenueActivity `json:"by_venue"`
}

type OptionRoleVenueActivity struct {
	Decisions int   `json:"decisions"`
	Accepted  int   `json:"accepted"`
	Rejected  int   `json:"rejected"`
	Fills     int   `json:"fills"`
	FilledQty int64 `json:"filled_qty"`
}

type optionRoleOrderEvidence struct {
	Symbol string `json:"symbol"`
	Qty    int64  `json:"qty"`
}

// MeasureOptionRoleActivity counts only canonical venue outcomes for one
// role's option orders. An option symbol is identified by the persisted
// expiry/strike/call-put naming contract, never by the participant's model.
func (r *Run) MeasureOptionRoleActivity(role string) (*OptionRoleActivity, error) {
	result := &OptionRoleActivity{Role: role, ByVenue: make(map[string]OptionRoleVenueActivity)}
	participants := make(map[Participant]struct{})
	var mu sync.Mutex
	if err := r.Scan(ScanOptions{Events: []string{"OrderAccepted", "OrderRejected", "OrderFill"}}, func(event Event) {
		if r.Role(event.VenueID, event.ClientID) != role {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			var row optionRoleOrderEvidence
			if event.Decode(&row) != nil {
				return
			}
			symbol = row.Symbol
		}
		if !strings.HasSuffix(symbol, "-C") && !strings.HasSuffix(symbol, "-P") {
			return
		}
		var row optionRoleOrderEvidence
		if event.Name == "OrderFill" || event.Symbol == "" {
			if err := event.Decode(&row); err != nil {
				return
			}
		}
		mu.Lock()
		participants[Participant{event.VenueID, event.ClientID}] = struct{}{}
		venue := result.ByVenue[event.VenueID]
		switch event.Name {
		case "OrderAccepted":
			result.Decisions++
			result.Accepted++
			venue.Decisions++
			venue.Accepted++
		case "OrderRejected":
			result.Decisions++
			result.Rejected++
			venue.Decisions++
			venue.Rejected++
		case "OrderFill":
			if row.Qty <= 0 {
				mu.Unlock()
				return
			}
			result.Fills++
			result.FilledQty += row.Qty
			venue.Fills++
			venue.FilledQty += row.Qty
		}
		result.ByVenue[event.VenueID] = venue
		mu.Unlock()
	}); err != nil {
		return nil, err
	}
	result.Participants = len(participants)
	return result, nil
}
