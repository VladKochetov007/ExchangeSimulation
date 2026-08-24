package analysis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
)

// LiabilityHedgerAudit independently reconstructs V2-4 L0's stated delivery
// obligation, actor-local executable-touch decision, exchange request, and
// local fill transition. It reads only persisted evidence and the preserved
// run configuration; it never imports or calls the actor implementation.
type LiabilityHedgerAudit struct {
	Decisions         int64 `json:"decisions"`
	DisabledDecisions int64 `json:"disabled_decisions"`
	EnabledDecisions  int64 `json:"enabled_decisions"`
	StateUpdates      int64 `json:"state_updates"`
	Submitted         int64 `json:"submitted"`
	Deferred          int64 `json:"deferred"`
	Accepted          int64 `json:"accepted"`
	Rejected          int64 `json:"rejected"`
	HorizonCensored   int64 `json:"horizon_censored"`
	Fills             int64 `json:"fills"`
	FilledQty         int64 `json:"filled_qty"`
	CancelledIOC      int64 `json:"cancelled_ioc"`
	// AbsoluteGapSum and GapSamples retain an exact decision-time aggregate.
	// The mean is intentionally reconstructed as sum/samples rather than a
	// floating approximation so paired L1 comparisons cannot hide rounding.
	AbsoluteGapSum string `json:"absolute_gap_sum"`
	GapSamples     int64  `json:"gap_samples"`

	ReceiptAuditValid       bool  `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors   int64 `json:"receipt_evidence_errors"`
	ReceiptMatches          int64 `json:"receipt_matches"`
	MissingReceipts         int64 `json:"missing_receipts"`
	AmbiguousReceipts       int64 `json:"ambiguous_receipts"`
	ReceiptMismatches       int64 `json:"receipt_mismatches"`
	FutureReceiptUse        int64 `json:"future_receipt_use"`
	MissingGatewayDecisions int64 `json:"missing_gateway_decisions"`
	GatewayDecisionMismatch int64 `json:"gateway_decision_mismatches"`

	InvalidDecisionRecords   int64                   `json:"invalid_decision_records"`
	StateTransitionMismatch  int64                   `json:"state_transition_mismatches"`
	DecisionFieldMismatches  int64                   `json:"decision_field_mismatches"`
	DisabledSubmissions      int64                   `json:"disabled_submissions"`
	DuplicateDecisions       int64                   `json:"duplicate_decisions"`
	MissingOutcomes          int64                   `json:"missing_outcomes"`
	DuplicateOutcomes        int64                   `json:"duplicate_outcomes"`
	CensoredOutcomeDelivery  int64                   `json:"censored_outcome_deliveries"`
	OutcomeFieldMismatches   int64                   `json:"outcome_field_mismatches"`
	MissingIOCTerminals      int64                   `json:"missing_ioc_terminals"`
	DuplicateIOCTerminals    int64                   `json:"duplicate_ioc_terminals"`
	FillQuantityMismatches   int64                   `json:"fill_quantity_mismatches"`
	MissingFillEvidence      int64                   `json:"missing_fill_evidence"`
	UnexpectedFillEvidence   int64                   `json:"unexpected_fill_evidence"`
	FillEvidenceMismatches   int64                   `json:"fill_evidence_mismatches"`
	NonReducingFills         int64                   `json:"non_reducing_fills"`
	RandomControlFills       int64                   `json:"random_control_fills"`
	RandomControlReducing    int64                   `json:"random_control_reducing_fills"`
	RandomControlNonReducing int64                   `json:"random_control_non_reducing_fills"`
	UnknownCounterparties    int64                   `json:"unknown_counterparties"`
	SelfFills                int64                   `json:"self_fills"`
	NonTakerFills            int64                   `json:"non_taker_fills"`
	NonPositiveFees          int64                   `json:"non_positive_fees"`
	FeeMismatches            int64                   `json:"fee_mismatches"`
	ActionCounts             map[string]int64        `json:"action_counts,omitempty"`
	PolicyMode               string                  `json:"policy_mode"`
	Hedgers                  []LiabilityHedgerBucket `json:"hedgers,omitempty"`

	Checks []LiabilityHedgerCheck `json:"checks,omitempty"`
	Valid  bool                   `json:"valid"`
}

// LiabilityHedgerBucket retains activation counts without averaging different
// venue-local accounts into one apparent participant.
type LiabilityHedgerBucket struct {
	VenueID             string `json:"venue_id"`
	ClientID            uint64 `json:"client_id"`
	Decisions           int64  `json:"decisions"`
	StateUpdates        int64  `json:"state_updates"`
	Submitted           int64  `json:"submitted"`
	Accepted            int64  `json:"accepted"`
	Fills               int64  `json:"fills"`
	AbsoluteGapSum      string `json:"absolute_gap_sum"`
	GapSamples          int64  `json:"gap_samples"`
	TerminalAbsoluteGap string `json:"terminal_absolute_gap"`
	ReducingFills       int64  `json:"reducing_fills"`
	NonReducingFills    int64  `json:"non_reducing_fills"`
}

// LiabilityHedgerCheck identifies a specific independent replay failure.
type LiabilityHedgerCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id,omitempty"`
	OrderID   uint64 `json:"order_id,omitempty"`
	Failure   string `json:"failure"`
}

type liabilityHedgerDecision struct {
	VenueID              string `json:"venue_id"`
	Hedger               string `json:"hedger"`
	ClientID             uint64 `json:"client_id"`
	Symbol               string `json:"symbol"`
	DecisionTime         int64  `json:"decision_time"`
	Enabled              bool   `json:"enabled"`
	PolicyMode           string `json:"policy_mode"`
	Subscribed           bool   `json:"subscribed"`
	RequestPending       bool   `json:"request_pending"`
	Action               string `json:"action_or_defer_reason"`
	ObligationBefore     int64  `json:"obligation_before"`
	ObligationAfter      int64  `json:"obligation_after"`
	ObligationStep       int64  `json:"obligation_step"`
	ObligationLimit      int64  `json:"obligation_limit"`
	PositionBefore       int64  `json:"position_before"`
	HedgeGap             int64  `json:"hedge_gap"`
	DecisionInterval     int64  `json:"decision_interval"`
	ObligationInterval   int64  `json:"obligation_interval"`
	LastBookSourceTime   int64  `json:"last_book_source_time"`
	LastBookReceivedTime int64  `json:"last_book_received_time"`
	LastBookSequence     uint64 `json:"last_book_sequence"`
	HasSnapshot          bool   `json:"has_snapshot"`
	HasBid               bool   `json:"has_bid"`
	BidPrice             int64  `json:"bid_price"`
	BidVisibleQty        int64  `json:"bid_visible_qty"`
	HasAsk               bool   `json:"has_ask"`
	AskPrice             int64  `json:"ask_price"`
	AskVisibleQty        int64  `json:"ask_visible_qty"`
	Side                 string `json:"side"`
	LimitPrice           int64  `json:"limit_price"`
	RequestedQty         int64  `json:"requested_qty"`
	RequestID            uint64 `json:"request_id"`
	TakerFeeBps          int64  `json:"taker_fee_bps"`
	OutcomeExpectation   string `json:"outcome_expectation"`
	CensorReason         string `json:"censor_reason"`
}

type liabilityHedgerFillEvidence struct {
	VenueID      string `json:"venue_id"`
	Hedger       string `json:"hedger"`
	ClientID     uint64 `json:"client_id"`
	Symbol       string `json:"symbol"`
	PolicyMode   string `json:"policy_mode"`
	Timestamp    int64  `json:"timestamp"`
	OrderID      uint64 `json:"order_id"`
	TradeID      uint64 `json:"trade_id"`
	Side         string `json:"side"`
	Qty          int64  `json:"qty"`
	Price        int64  `json:"price"`
	FeeAmount    int64  `json:"fee_amount"`
	FeeAsset     string `json:"fee_asset"`
	PrePosition  int64  `json:"pre_position"`
	PostPosition int64  `json:"post_position"`
}

type liabilityHedgerOrder struct {
	OrderID     uint64 `json:"order_id"`
	ClientID    uint64 `json:"client_id"`
	RequestID   uint64 `json:"request_id"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	PostOnly    bool   `json:"post_only"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
}

type liabilityHedgerFill struct {
	Timestamp int64  `json:"-"`
	OrderID   uint64 `json:"order_id"`
	TradeID   uint64 `json:"trade_id"`
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Qty       int64  `json:"qty"`
	Price     int64  `json:"price"`
	FeeAmount int64  `json:"fee_amount"`
	FeeAsset  string `json:"fee_asset"`
	Role      string `json:"role"`
}

type liabilityHedgerCancellation struct {
	OrderID      uint64 `json:"order_id"`
	RemainingQty int64  `json:"remaining_qty"`
}

type liabilityHedgerTrade struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type liabilityHedgerKey struct {
	venueID  string
	clientID uint64
	request  uint64
}

type liabilityHedgerOrderKey struct {
	venueID string
	orderID uint64
}

type liabilityHedgerReceiptKey struct {
	venueID   string
	clientID  uint64
	sequence  uint64
	published int64
}

type liabilityHedgerReceipt struct{ deliveredAt int64 }

type liabilityHedgerGatewayDecision struct {
	decisionAt  int64
	deliveredAt int64
	side        byte
	orderType   byte
	tif         byte
	price       int64
	qty         int64
}

type liabilityHedgerOutcome struct {
	accepted bool
	order    liabilityHedgerOrder
}

type liabilityHedgerStateEvent struct {
	venueID  string
	clientID uint64
	file     string
	ordinal  int64
	time     int64
	decision *liabilityHedgerDecision
	fill     *liabilityHedgerFillEvidence
}

type liabilityHedgerReplayState struct {
	rng        *rand.Rand
	policyRNG  *rand.Rand
	obligation int64
	position   int64
	lastUpdate int64
	lastTick   int64
	seenFirst  bool
}

type liabilityHedgerRunConfig struct {
	Seed               int64    `json:"seed"`
	VenueIDs           []string `json:"venue_ids"`
	CDFLiabilityHedger *struct {
		Enabled             bool   `json:"enabled"`
		PolicyMode          string `json:"policy_mode"`
		Symbol              string `json:"symbol"`
		DecisionInterval    int64  `json:"decision_interval"`
		ObligationInterval  int64  `json:"obligation_interval"`
		ObligationStepQty   int64  `json:"obligation_step_qty"`
		MaxAbsObligationQty int64  `json:"max_abs_obligation_qty"`
		MaxRequestQty       int64  `json:"max_request_qty"`
	} `json:"cdf_liability_hedger"`
}

const (
	liabilityHedgerDecisionInterval   = int64(2_000_000_000)
	liabilityHedgerObligationInterval = int64(10_000_000_000)
	liabilityHedgerStep               = int64(200_000_000)
	liabilityHedgerLimit              = int64(2_000_000_000)
	liabilityHedgerRequestCap         = int64(100_000_000)
	liabilityHedgerFeeBps             = int64(5)
	liabilityHedgerPolicyLiability    = "delivery_liability"
	liabilityHedgerPolicyRandom       = "random_side_control"
)

// MeasureLiabilityHedger audits the complete L0 evidence relation. It does
// not accept price movement, a lower inventory, or a valid-looking log row as
// a substitute for an exact local state/action/exchange replay.
func (r *Run) MeasureLiabilityHedger() (*LiabilityHedgerAudit, error) {
	config, err := loadLiabilityHedgerRunConfig(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validLiabilityHedgerRunConfig(config); err != nil {
		return nil, err
	}
	receipts, gatewayDecisions, receiptAudit, receiptErr := liabilityHedgerEvidence(r.Dir)
	policyMode := effectiveLiabilityHedgerPolicyMode(config.CDFLiabilityHedger.PolicyMode)
	result := &LiabilityHedgerAudit{ActionCounts: make(map[string]int64), PolicyMode: policyMode}
	terminalAt := int64(0)
	if receiptAudit != nil {
		terminalAt = receiptAudit.TerminalAt
	}
	if receiptErr != nil {
		result.ReceiptEvidenceErrors++
	} else {
		result.ReceiptAuditValid = receiptAudit.Valid
		if !receiptAudit.Valid {
			result.ReceiptEvidenceErrors++
		}
	}

	var stateEvents []liabilityHedgerStateEvent
	outcomes := make(map[liabilityHedgerKey][]liabilityHedgerOutcome)
	orders := make(map[liabilityHedgerOrderKey]liabilityHedgerOrder)
	fills := make(map[liabilityHedgerOrderKey][]liabilityHedgerFill)
	cancels := make(map[liabilityHedgerOrderKey][]liabilityHedgerCancellation)
	trades := make(map[liabilityHedgerOrderKey][]liabilityHedgerTrade)
	fillEvidence := make(map[liabilityHedgerOrderKey][]liabilityHedgerFillEvidence)
	addCheck := func(venue string, client, request, order uint64, failure string) {
		result.Checks = append(result.Checks, LiabilityHedgerCheck{VenueID: venue, ClientID: client, RequestID: request, OrderID: order, Failure: failure})
	}

	err = r.Scan(ScanOptions{Events: []string{
		"liability_hedger_decision", "liability_hedger_fill", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "Trade",
	}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "liability_hedger_decision":
			var decision liabilityHedgerDecision
			if event.Decode(&decision) != nil || decision.ClientID == 0 || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID ||
				decision.Hedger == "" || decision.Symbol != "CDF/USD" || r.Role(event.VenueID, event.ClientID) != "liability_hedger" {
				result.InvalidDecisionRecords++
				addCheck(event.VenueID, event.ClientID, 0, 0, "invalid_decision_record")
				return
			}
			result.Decisions++
			result.ActionCounts[decision.Action]++
			if decision.Enabled {
				result.EnabledDecisions++
			} else {
				result.DisabledDecisions++
			}
			stateEvents = append(stateEvents, liabilityHedgerStateEvent{venueID: event.VenueID, clientID: event.ClientID, file: event.File, ordinal: event.Ordinal, time: event.SimTS, decision: &decision})
		case "liability_hedger_fill":
			var evidence liabilityHedgerFillEvidence
			if event.Decode(&evidence) != nil || evidence.OrderID == 0 || evidence.ClientID == 0 || evidence.VenueID != event.VenueID || evidence.ClientID != event.ClientID || evidence.Symbol != "CDF/USD" || evidence.Hedger == "" {
				result.UnexpectedFillEvidence++
				addCheck(event.VenueID, event.ClientID, 0, evidence.OrderID, "invalid_fill_evidence")
				return
			}
			key := liabilityHedgerOrderKey{venueID: event.VenueID, orderID: evidence.OrderID}
			fillEvidence[key] = append(fillEvidence[key], evidence)
			copy := evidence
			stateEvents = append(stateEvents, liabilityHedgerStateEvent{venueID: event.VenueID, clientID: event.ClientID, file: event.File, ordinal: event.Ordinal, time: event.SimTS, fill: &copy})
		case "OrderAccepted", "OrderRejected":
			var order liabilityHedgerOrder
			if event.Decode(&order) != nil || order.RequestID == 0 {
				return
			}
			if order.ClientID == 0 {
				order.ClientID = event.ClientID
			}
			if order.Symbol == "" && pathHasSymbol(event.File, event.VenueID, "CDF-USD") {
				order.Symbol = "CDF/USD"
			}
			key := liabilityHedgerKey{venueID: event.VenueID, clientID: event.ClientID, request: order.RequestID}
			outcomes[key] = append(outcomes[key], liabilityHedgerOutcome{accepted: event.Name == "OrderAccepted", order: order})
			if event.Name == "OrderAccepted" && order.OrderID != 0 {
				orders[liabilityHedgerOrderKey{venueID: event.VenueID, orderID: order.OrderID}] = order
			}
		case "OrderFill":
			var fill liabilityHedgerFill
			if event.Decode(&fill) == nil && fill.OrderID != 0 && fill.Qty > 0 {
				fill.Timestamp = event.SimTS
				key := liabilityHedgerOrderKey{venueID: event.VenueID, orderID: fill.OrderID}
				fills[key] = append(fills[key], fill)
			}
		case "OrderCancelled":
			var cancel liabilityHedgerCancellation
			if event.Decode(&cancel) == nil && cancel.OrderID != 0 {
				key := liabilityHedgerOrderKey{venueID: event.VenueID, orderID: cancel.OrderID}
				cancels[key] = append(cancels[key], cancel)
			}
		case "Trade":
			var trade liabilityHedgerTrade
			if event.Decode(&trade) != nil || trade.Qty <= 0 {
				return
			}
			if trade.TakerOrderID != 0 {
				key := liabilityHedgerOrderKey{venueID: event.VenueID, orderID: trade.TakerOrderID}
				trades[key] = append(trades[key], trade)
			}
			if trade.MakerOrderID != 0 {
				key := liabilityHedgerOrderKey{venueID: event.VenueID, orderID: trade.MakerOrderID}
				trades[key] = append(trades[key], trade)
			}
		}
	})
	if err != nil {
		return nil, err
	}

	expected := make(map[liabilityHedgerKey]liabilityHedgerDecision)
	stateBuckets := make(map[Participant]*LiabilityHedgerBucket)
	stateFiles := make(map[Participant]string)
	seenDecisionTick := make(map[Participant]map[int64]bool)
	gapSums := make(map[Participant]*big.Int)
	gapCounts := make(map[Participant]int64)
	sort.Slice(stateEvents, func(i, j int) bool {
		left, right := stateEvents[i], stateEvents[j]
		if left.venueID != right.venueID {
			return left.venueID < right.venueID
		}
		if left.clientID != right.clientID {
			return left.clientID < right.clientID
		}
		if left.file != right.file {
			return left.file < right.file
		}
		return left.ordinal < right.ordinal
	})
	states := make(map[Participant]*liabilityHedgerReplayState)
	venueIndex := make(map[string]int, len(config.VenueIDs))
	for index, venueID := range config.VenueIDs {
		venueIndex[venueID] = index
	}
	for _, event := range stateEvents {
		participant := Participant{VenueID: event.venueID, ClientID: event.clientID}
		state := states[participant]
		if state == nil {
			index, ok := venueIndex[event.venueID]
			if !ok {
				result.StateTransitionMismatch++
				addCheck(event.venueID, event.clientID, 0, 0, "unknown_venue_in_liability_state")
				continue
			}
			state = &liabilityHedgerReplayState{
				rng:       rand.New(rand.NewSource(liabilityHedgerFlowSeed(config.Seed, index, 0, 14))),
				policyRNG: rand.New(rand.NewSource(liabilityHedgerFlowSeed(config.Seed, index, 0, 15))),
			}
			states[participant] = state
			stateBuckets[participant] = &LiabilityHedgerBucket{VenueID: event.venueID, ClientID: event.clientID}
			seenDecisionTick[participant] = make(map[int64]bool)
		}
		bucket := stateBuckets[participant]
		if previous, seen := stateFiles[participant]; seen && previous != event.file {
			result.StateTransitionMismatch++
			addCheck(event.venueID, event.clientID, 0, 0, "liability_state_has_no_single_causal_file")
		} else {
			stateFiles[participant] = event.file
		}
		if event.decision != nil {
			bucket.Decisions++
			if seenDecisionTick[participant][event.decision.DecisionTime] {
				result.DuplicateDecisions++
				addCheck(event.venueID, event.clientID, event.decision.RequestID, 0, "duplicate_liability_decision_tick")
			}
			seenDecisionTick[participant][event.decision.DecisionTime] = true
			valid, update, submitted := validateLiabilityHedgerDecision(*event.decision, state, terminalAt, policyMode)
			if update {
				result.StateUpdates++
				bucket.StateUpdates++
			}
			if !valid {
				result.DecisionFieldMismatches++
				addCheck(event.venueID, event.clientID, event.decision.RequestID, 0, "decision_policy_or_state_mismatch")
			} else {
				sum := gapSums[participant]
				if sum == nil {
					sum = new(big.Int)
					gapSums[participant] = sum
				}
				sum.Add(sum, new(big.Int).Abs(big.NewInt(event.decision.HedgeGap)))
				gapCounts[participant]++
			}
			if liabilityHedgerUsesLocalBook(*event.decision) {
				liabilityHedgerCheckReceipt(result, receipts, receiptErr, *event.decision, addCheck)
			}
			if !event.decision.Enabled {
				expectedAction := "POLICY_DISABLED"
				if !event.decision.Subscribed {
					expectedAction = "NOT_SUBSCRIBED"
				}
				if event.decision.Action != expectedAction || event.decision.RequestID != 0 || event.decision.RequestedQty != 0 {
					result.DisabledSubmissions++
					addCheck(event.venueID, event.clientID, event.decision.RequestID, 0, "disabled_policy_submitted")
				}
				continue
			}
			if !submitted {
				result.Deferred++
				if event.decision.Action == "SIMULATION_HORIZON_CENSORED" {
					result.HorizonCensored++
				}
				// A decision that selected a side and capped desired quantity may
				// still defer when the corresponding executable local touch is
				// absent. RequestedQty is deliberately retained in that evidence to
				// make the selected policy independently replayable; RequestID is
				// the boundary at which an actual venue request exists. The decision
				// validator above verifies the permitted unavailable-touch shape.
				if event.decision.RequestID != 0 {
					result.DecisionFieldMismatches++
					addCheck(event.venueID, event.clientID, event.decision.RequestID, 0, "deferred_policy_has_request_id")
				}
				continue
			}
			result.Submitted++
			bucket.Submitted++
			key := liabilityHedgerKey{venueID: event.venueID, clientID: event.clientID, request: event.decision.RequestID}
			if event.decision.RequestID == 0 {
				result.InvalidDecisionRecords++
				addCheck(event.venueID, event.clientID, 0, 0, "submission_without_request_id")
			} else if _, exists := expected[key]; exists {
				result.DuplicateDecisions++
				addCheck(event.venueID, event.clientID, event.decision.RequestID, 0, "duplicate_submission_decision")
			} else {
				expected[key] = *event.decision
				liabilityHedgerCheckGatewayDecision(result, gatewayDecisions, *event.decision, addCheck)
			}
			continue
		}
		if event.fill != nil {
			valid, reducesGap := validateLiabilityHedgerStateFill(*event.fill, state, policyMode)
			if !valid {
				result.FillEvidenceMismatches++
				addCheck(event.venueID, event.clientID, 0, event.fill.OrderID, "actor_local_position_transition_mismatch")
			} else if policyMode == liabilityHedgerPolicyRandom {
				result.RandomControlFills++
				if reducesGap {
					result.RandomControlReducing++
					bucket.ReducingFills++
				} else {
					result.RandomControlNonReducing++
					bucket.NonReducingFills++
				}
			} else if !reducesGap {
				result.NonReducingFills++
				bucket.NonReducingFills++
				addCheck(event.venueID, event.clientID, 0, event.fill.OrderID, "fill_does_not_reduce_actor_gap")
			} else {
				bucket.ReducingFills++
			}
		}
	}
	for participant, bucket := range stateBuckets {
		if bucket.StateUpdates == 0 {
			result.StateTransitionMismatch++
			addCheck(participant.VenueID, participant.ClientID, 0, 0, "liability_state_never_updated")
		}
	}

	accepted := make(map[liabilityHedgerOrderKey]liabilityHedgerDecision)
	for key, decision := range expected {
		censored, validCensor := validLiabilityHedgerCensor(decision)
		if !validCensor {
			result.DecisionFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, 0, "invalid_outcome_expectation")
		}
		outcome := outcomes[key]
		if censored {
			if len(outcome) == 0 {
				result.HorizonCensored++
				continue
			}
			result.CensoredOutcomeDelivery += int64(len(outcome))
			addCheck(key.venueID, key.clientID, key.request, 0, "terminal_censored_request_delivered")
			continue
		}
		if len(outcome) == 0 {
			result.MissingOutcomes++
			addCheck(key.venueID, key.clientID, key.request, 0, "missing_request_outcome")
			continue
		}
		if len(outcome) != 1 {
			result.DuplicateOutcomes++
			addCheck(key.venueID, key.clientID, key.request, 0, "duplicate_request_outcome")
			continue
		}
		got := outcome[0]
		if !validLiabilityHedgerOutcome(decision, got.order) {
			result.OutcomeFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, got.order.OrderID, "request_fields_mismatch")
		}
		if !got.accepted {
			result.Rejected++
			continue
		}
		result.Accepted++
		if bucket := stateBuckets[Participant{VenueID: key.venueID, ClientID: key.clientID}]; bucket != nil {
			bucket.Accepted++
		}
		if got.order.OrderID == 0 {
			result.OutcomeFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, 0, "accepted_without_order_id")
			continue
		}
		accepted[liabilityHedgerOrderKey{venueID: key.venueID, orderID: got.order.OrderID}] = decision
	}

	matchedEvidence := make(map[liabilityHedgerOrderKey]map[int]bool)
	for key, decision := range accepted {
		order := orders[key]
		var filled int64
		for _, fill := range fills[key] {
			filled += fill.Qty
			result.Fills++
			result.FilledQty += fill.Qty
			if bucket := stateBuckets[Participant{VenueID: key.venueID, ClientID: order.ClientID}]; bucket != nil {
				bucket.Fills++
			}
			if fill.Role != "taker" {
				result.NonTakerFills++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "liability_fill_not_taker")
			}
			if fill.FeeAmount <= 0 || fill.FeeAsset != "USD" {
				result.NonPositiveFees++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "non_positive_or_wrong_asset_fee")
			}
			if want, ok := liabilityHedgerFee(fill.Qty, fill.Price, decision.TakerFeeBps); !ok || want != fill.FeeAmount {
				result.FeeMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "fee_formula_mismatch")
			}
			if !liabilityHedgerHasExternalCounterparty(trades[key], orders, key, fill) {
				result.UnknownCounterparties++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_external_counterparty")
			} else if liabilityHedgerHasSelfCounterparty(trades[key], orders, key, fill, order.ClientID) {
				result.SelfFills++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "self_fill")
			}
			matched := false
			for index, evidence := range fillEvidence[key] {
				if matchedEvidence[key] != nil && matchedEvidence[key][index] {
					continue
				}
				if evidence.ClientID == order.ClientID && evidence.Symbol == fill.Symbol && evidence.Side == fill.Side && evidence.Qty == fill.Qty && evidence.Price == fill.Price && evidence.FeeAmount == fill.FeeAmount && evidence.FeeAsset == fill.FeeAsset && evidence.TradeID == fill.TradeID && evidence.Timestamp == fill.Timestamp {
					if matchedEvidence[key] == nil {
						matchedEvidence[key] = make(map[int]bool)
					}
					matchedEvidence[key][index] = true
					matched = true
					break
				}
			}
			if !matched {
				result.MissingFillEvidence++
				result.FillEvidenceMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_exact_fill_evidence")
			}
		}
		for index := range fillEvidence[key] {
			if !matchedEvidence[key][index] {
				result.UnexpectedFillEvidence++
				result.FillEvidenceMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "unexpected_fill_evidence")
			}
		}
		if filled > order.Qty {
			result.FillQuantityMismatches++
			addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "filled_quantity_exceeds_request")
		}
		if filled < order.Qty {
			terminal := cancels[key]
			if len(terminal) == 0 {
				result.MissingIOCTerminals++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_ioc_remainder_cancellation")
			} else if len(terminal) != 1 || terminal[0].RemainingQty != order.Qty-filled {
				result.DuplicateIOCTerminals++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "invalid_ioc_remainder_cancellation")
			} else {
				result.CancelledIOC++
			}
		} else if len(cancels[key]) != 0 {
			result.DuplicateIOCTerminals++
			addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "full_ioc_has_cancellation")
		}
	}
	for key, rows := range fillEvidence {
		if _, known := accepted[key]; known {
			continue
		}
		for range rows {
			result.UnexpectedFillEvidence++
			addCheck(key.venueID, 0, 0, key.orderID, "fill_evidence_without_accepted_l0_order")
		}
	}
	totalGap := new(big.Int)
	for participant, bucket := range stateBuckets {
		if sum := gapSums[participant]; sum != nil {
			bucket.AbsoluteGapSum = sum.String()
			totalGap.Add(totalGap, sum)
		} else {
			bucket.AbsoluteGapSum = "0"
		}
		bucket.GapSamples = gapCounts[participant]
		result.GapSamples += bucket.GapSamples
		if state := states[participant]; state != nil {
			bucket.TerminalAbsoluteGap = new(big.Int).Abs(new(big.Int).Sub(big.NewInt(state.obligation), big.NewInt(state.position))).String()
		} else {
			bucket.TerminalAbsoluteGap = "0"
		}
		result.Hedgers = append(result.Hedgers, *bucket)
	}
	result.AbsoluteGapSum = totalGap.String()

	sort.Slice(result.Hedgers, func(i, j int) bool {
		if result.Hedgers[i].VenueID != result.Hedgers[j].VenueID {
			return result.Hedgers[i].VenueID < result.Hedgers[j].VenueID
		}
		return result.Hedgers[i].ClientID < result.Hedgers[j].ClientID
	})
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
	result.Valid = result.Decisions > 0 && result.ReceiptEvidenceErrors == 0 && result.MissingReceipts == 0 && result.AmbiguousReceipts == 0 && result.ReceiptMismatches == 0 && result.FutureReceiptUse == 0 && result.MissingGatewayDecisions == 0 && result.GatewayDecisionMismatch == 0 &&
		result.InvalidDecisionRecords == 0 && result.StateTransitionMismatch == 0 && result.DecisionFieldMismatches == 0 && result.DisabledSubmissions == 0 && result.DuplicateDecisions == 0 && result.MissingOutcomes == 0 && result.DuplicateOutcomes == 0 && result.CensoredOutcomeDelivery == 0 && result.OutcomeFieldMismatches == 0 &&
		result.MissingIOCTerminals == 0 && result.DuplicateIOCTerminals == 0 && result.FillQuantityMismatches == 0 && result.MissingFillEvidence == 0 && result.UnexpectedFillEvidence == 0 && result.FillEvidenceMismatches == 0 && result.NonReducingFills == 0 && result.UnknownCounterparties == 0 && result.SelfFills == 0 && result.NonTakerFills == 0 && result.NonPositiveFees == 0 && result.FeeMismatches == 0
	return result, nil
}

func loadLiabilityHedgerRunConfig(dir string) (liabilityHedgerRunConfig, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "run-config.json"))
	if err != nil {
		return liabilityHedgerRunConfig{}, fmt.Errorf("read L0 run configuration: %w", err)
	}
	var config liabilityHedgerRunConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return liabilityHedgerRunConfig{}, fmt.Errorf("decode L0 run configuration: %w", err)
	}
	return config, nil
}

func validLiabilityHedgerRunConfig(config liabilityHedgerRunConfig) error {
	policy := config.CDFLiabilityHedger
	if policy == nil || len(config.VenueIDs) != 3 || policy.Symbol != "CDF/USD" || policy.DecisionInterval != liabilityHedgerDecisionInterval || policy.ObligationInterval != liabilityHedgerObligationInterval || policy.ObligationStepQty != liabilityHedgerStep || policy.MaxAbsObligationQty != liabilityHedgerLimit || policy.MaxRequestQty != liabilityHedgerRequestCap {
		return fmt.Errorf("unsupported L0/L1 policy/configuration")
	}
	if policy.PolicyMode != "" && policy.PolicyMode != liabilityHedgerPolicyLiability && policy.PolicyMode != liabilityHedgerPolicyRandom {
		return fmt.Errorf("unsupported L0/L1 policy mode %q", policy.PolicyMode)
	}
	seen := make(map[string]struct{}, len(config.VenueIDs))
	for _, venueID := range config.VenueIDs {
		if venueID == "" {
			return fmt.Errorf("L0 configuration has an empty venue ID")
		}
		if _, exists := seen[venueID]; exists {
			return fmt.Errorf("L0 configuration duplicates venue ID %q", venueID)
		}
		seen[venueID] = struct{}{}
	}
	return nil
}

// effectiveLiabilityHedgerPolicyMode preserves the L0 evidence contract:
// historical configs and decision rows did not carry the field, and their
// only supported behavior was delivery-liability direction. L1 configs must
// state a non-empty mode and its decision evidence is checked strictly below.
func effectiveLiabilityHedgerPolicyMode(mode string) string {
	if mode == "" {
		return liabilityHedgerPolicyLiability
	}
	return mode
}

func liabilityHedgerObservedPolicyModeMatches(observed, configured string) bool {
	if configured == liabilityHedgerPolicyLiability {
		// Accept legacy L0 rows without a field, plus new explicit treatment
		// rows. The random control is never allowed to omit its mode.
		return observed == "" || observed == liabilityHedgerPolicyLiability
	}
	return observed == configured
}

func liabilityHedgerEvidence(dir string) (map[liabilityHedgerReceiptKey][]liabilityHedgerReceipt, map[liabilityHedgerKey][]liabilityHedgerGatewayDecision, *MarketDataReceiptAudit, error) {
	audit, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	rawManifest, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, nil, audit, err
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil {
		return nil, nil, audit, err
	}
	receiptRaw, _, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, nil, audit, err
	}
	decisionRaw, _, err := readEvidenceFile(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, nil, audit, err
	}
	links := make(map[uint32]struct{ venue, role string }, len(manifest.Links))
	for _, link := range manifest.Links {
		links[link.ID] = struct{ venue, role string }{venue: link.SourceVenue, role: link.Role}
	}
	symbols := make(map[uint32]string, len(manifest.Symbols))
	for _, symbol := range manifest.Symbols {
		symbols[symbol.ID] = symbol.Symbol
	}
	receipts := make(map[liabilityHedgerReceiptKey][]liabilityHedgerReceipt)
	for offset := 0; offset < len(receiptRaw); offset += marketDataReceiptRecordBytes {
		record := decodeObservation(receiptRaw[offset : offset+marketDataReceiptRecordBytes])
		link, ok := links[record.linkID]
		if !ok || link.role != "liability_hedger" || record.mdType != 0 || symbols[record.symbolID] != "CDF/USD" {
			continue
		}
		key := liabilityHedgerReceiptKey{venueID: link.venue, clientID: record.clientID, sequence: record.sequence, published: record.publishedAt}
		receipts[key] = append(receipts[key], liabilityHedgerReceipt{deliveredAt: record.deliveredAt})
	}
	decisions := make(map[liabilityHedgerKey][]liabilityHedgerGatewayDecision)
	for offset := 0; offset < len(decisionRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(decisionRaw[offset : offset+marketDataDecisionRecordBytes])
		link, ok := links[record.linkID]
		if !ok || link.role != "liability_hedger" || symbols[record.symbolID] != "CDF/USD" || record.requestID == 0 {
			continue
		}
		key := liabilityHedgerKey{venueID: link.venue, clientID: record.clientID, request: record.requestID}
		decisions[key] = append(decisions[key], liabilityHedgerGatewayDecision{decisionAt: record.decisionAt, deliveredAt: record.frontierDeliveredAt, side: record.side, orderType: record.orderType, tif: record.tif, price: record.price, qty: record.qty})
	}
	return receipts, decisions, audit, nil
}

func validateLiabilityHedgerDecision(d liabilityHedgerDecision, state *liabilityHedgerReplayState, terminalAt int64, policyMode string) (valid bool, update bool, submitted bool) {
	if d.Symbol != "CDF/USD" || d.ObligationLimit != liabilityHedgerLimit || d.DecisionInterval != liabilityHedgerDecisionInterval || d.ObligationInterval != liabilityHedgerObligationInterval || d.TakerFeeBps != liabilityHedgerFeeBps || d.DecisionTime <= 0 {
		return false, false, false
	}
	if !liabilityHedgerObservedPolicyModeMatches(d.PolicyMode, policyMode) {
		return false, false, false
	}
	if !validLiabilityHedgerBookEvidence(d) {
		return false, false, false
	}
	if !state.seenFirst {
		state.seenFirst = true
		state.lastTick = d.DecisionTime
		if d.Subscribed || d.Action != "NOT_SUBSCRIBED" || d.ObligationBefore != 0 || d.ObligationAfter != 0 || d.ObligationStep != 0 || d.PositionBefore != 0 || d.HedgeGap != 0 || d.RequestID != 0 || d.RequestedQty != 0 || d.Side != "" || d.HasSnapshot || d.HasBid || d.HasAsk {
			return false, false, false
		}
		return true, false, false
	}
	if d.DecisionTime-state.lastTick != liabilityHedgerDecisionInterval {
		return false, false, false
	}
	state.lastTick = d.DecisionTime
	if !d.Subscribed || d.ObligationBefore != state.obligation || d.PositionBefore != state.position {
		return false, false, false
	}
	expectedUpdate := state.lastUpdate == 0 || d.DecisionTime-state.lastUpdate >= liabilityHedgerObligationInterval
	if expectedUpdate {
		step, next, ok := liabilityHedgerNextStep(state)
		if !ok || d.ObligationStep != step || d.ObligationAfter != next {
			return false, false, false
		}
		state.obligation, state.lastUpdate = next, d.DecisionTime
		update = true
	} else if d.ObligationStep != 0 || d.ObligationAfter != state.obligation {
		return false, false, false
	}
	gap := new(big.Int).Sub(big.NewInt(state.obligation), big.NewInt(state.position))
	if !gap.IsInt64() || d.HedgeGap != gap.Int64() {
		return false, update, false
	}
	if !d.Enabled {
		return d.Action == "POLICY_DISABLED" && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && d.LimitPrice == 0, update, false
	}
	if d.RequestPending {
		return d.Action == "REQUEST_PENDING" && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && d.LimitPrice == 0, update, false
	}
	if gap.Sign() == 0 {
		return d.Action == "IN_BAND" && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && d.LimitPrice == 0, update, false
	}
	if d.Action == "SIMULATION_HORIZON_CENSORED" {
		deadline, ok := liabilityHedgerTailDeadline(d.DecisionTime)
		return ok && terminalAt != 0 && deadline > terminalAt &&
			d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && d.LimitPrice == 0 &&
			d.OutcomeExpectation == "SIMULATION_HORIZON_CENSORED" && d.CensorReason == "terminal_horizon_before_round_trip", update, false
	}
	if !d.HasSnapshot {
		return d.Action == "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE" && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && d.LimitPrice == 0, update, false
	}
	if d.LastBookSourceTime > d.DecisionTime {
		return d.Action == "LOCAL_BOOK_SOURCE_FUTURE" && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && d.LimitPrice == 0, update, false
	}
	wantSide := "SELL"
	if policyMode == liabilityHedgerPolicyRandom {
		if state.policyRNG.Intn(2) == 0 {
			wantSide = "BUY"
		}
	} else if gap.Sign() > 0 {
		wantSide = "BUY"
	}
	hasTouch, touch := d.HasBid, d.BidPrice
	if wantSide == "BUY" {
		hasTouch, touch = d.HasAsk, d.AskPrice
	}
	quantity := new(big.Int).Abs(gap)
	if quantity.Cmp(big.NewInt(liabilityHedgerRequestCap)) > 0 {
		quantity.SetInt64(liabilityHedgerRequestCap)
	}
	if !quantity.IsInt64() || quantity.Sign() <= 0 {
		return d.Action == "ZERO_REQUEST_QUANTITY" && d.RequestID == 0 && d.RequestedQty == 0, update, false
	}
	if d.Side != wantSide || d.RequestedQty != quantity.Int64() {
		return false, update, false
	}
	if !hasTouch {
		return d.Action == "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE" && d.RequestID == 0 && d.LimitPrice == 0, update, false
	}
	if d.LimitPrice != touch || d.RequestID == 0 || d.Action != "SUBMIT_IOC" {
		return false, update, false
	}
	return true, update, true
}

func liabilityHedgerTailDeadline(decisionAt int64) (int64, bool) {
	deadline := big.NewInt(decisionAt)
	deadline.Add(deadline, big.NewInt(liabilityHedgerDecisionInterval))
	deadline.Add(deadline, big.NewInt(liabilityHedgerDecisionInterval))
	if !deadline.IsInt64() {
		return 0, false
	}
	return deadline.Int64(), true
}

// validLiabilityHedgerBookEvidence keeps side availability separate from every
// numeric price. A present zero-valued level is not reclassified as missing;
// only the explicit flags describe snapshot and side availability.
func validLiabilityHedgerBookEvidence(d liabilityHedgerDecision) bool {
	if !d.HasSnapshot {
		return d.LastBookSourceTime == 0 && d.LastBookReceivedTime == 0 && d.LastBookSequence == 0 &&
			!d.HasBid && d.BidPrice == 0 && d.BidVisibleQty == 0 &&
			!d.HasAsk && d.AskPrice == 0 && d.AskVisibleQty == 0
	}
	if !d.HasBid && (d.BidPrice != 0 || d.BidVisibleQty != 0) {
		return false
	}
	if !d.HasAsk && (d.AskPrice != 0 || d.AskVisibleQty != 0) {
		return false
	}
	return true
}

func liabilityHedgerNextStep(state *liabilityHedgerReplayState) (int64, int64, bool) {
	step := liabilityHedgerStep
	if state.rng.Intn(2) == 0 {
		step = -step
	}
	next := new(big.Int).Add(big.NewInt(state.obligation), big.NewInt(step))
	limit := big.NewInt(liabilityHedgerLimit)
	negativeLimit := new(big.Int).Neg(new(big.Int).Set(limit))
	if next.Cmp(limit) > 0 || next.Cmp(negativeLimit) < 0 {
		step = -step
		next.SetInt64(state.obligation)
		next.Add(next, big.NewInt(step))
	}
	if !next.IsInt64() || next.Cmp(limit) > 0 || next.Cmp(negativeLimit) < 0 || step == 0 {
		return 0, state.obligation, false
	}
	return step, next.Int64(), true
}

func liabilityHedgerUsesLocalBook(d liabilityHedgerDecision) bool {
	if !d.HasSnapshot {
		return false
	}
	switch d.Action {
	case "LOCAL_BOOK_SOURCE_FUTURE", "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE", "SUBMIT_IOC":
		return true
	default:
		return false
	}
}

func liabilityHedgerCheckReceipt(result *LiabilityHedgerAudit, receipts map[liabilityHedgerReceiptKey][]liabilityHedgerReceipt, receiptErr error, d liabilityHedgerDecision, add func(string, uint64, uint64, uint64, string)) {
	if receiptErr != nil {
		result.MissingReceipts++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "market_data_receipt_evidence_unavailable")
		return
	}
	if d.LastBookSequence == 0 || d.LastBookReceivedTime == 0 {
		result.MissingReceipts++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "decision_missing_local_book_receipt_identity")
		return
	}
	key := liabilityHedgerReceiptKey{venueID: d.VenueID, clientID: d.ClientID, sequence: d.LastBookSequence, published: d.LastBookSourceTime}
	matches := receipts[key]
	if len(matches) == 0 {
		result.MissingReceipts++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "missing_matching_snapshot_receipt")
		return
	}
	if len(matches) != 1 {
		result.AmbiguousReceipts++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "ambiguous_matching_snapshot_receipt")
		return
	}
	if matches[0].deliveredAt != d.LastBookReceivedTime {
		result.ReceiptMismatches++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "snapshot_receipt_delivery_mismatch")
	}
	if d.LastBookReceivedTime > d.DecisionTime {
		result.FutureReceiptUse++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "future_snapshot_used_by_decision")
	}
	result.ReceiptMatches++
}

func liabilityHedgerCheckGatewayDecision(result *LiabilityHedgerAudit, decisions map[liabilityHedgerKey][]liabilityHedgerGatewayDecision, d liabilityHedgerDecision, add func(string, uint64, uint64, uint64, string)) {
	key := liabilityHedgerKey{venueID: d.VenueID, clientID: d.ClientID, request: d.RequestID}
	rows := decisions[key]
	if len(rows) == 0 {
		result.MissingGatewayDecisions++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "missing_v2_gateway_decision")
		return
	}
	if len(rows) != 1 || rows[0].decisionAt != d.DecisionTime || rows[0].deliveredAt != d.LastBookReceivedTime || rows[0].orderType != 1 || rows[0].tif != 1 || rows[0].price != d.LimitPrice || rows[0].qty != d.RequestedQty || (d.Side == "BUY" && rows[0].side != 0) || (d.Side == "SELL" && rows[0].side != 1) {
		result.GatewayDecisionMismatch++
		add(d.VenueID, d.ClientID, d.RequestID, 0, "v2_gateway_decision_mismatch")
	}
}

func validateLiabilityHedgerStateFill(fill liabilityHedgerFillEvidence, state *liabilityHedgerReplayState, policyMode string) (valid bool, reducesGap bool) {
	if !state.seenFirst || fill.Qty <= 0 || (fill.Side != "BUY" && fill.Side != "SELL") || fill.PrePosition != state.position || !liabilityHedgerObservedPolicyModeMatches(fill.PolicyMode, policyMode) {
		return false, false
	}
	next := new(big.Int).SetInt64(state.position)
	if fill.Side == "BUY" {
		next.Add(next, big.NewInt(fill.Qty))
	} else {
		next.Sub(next, big.NewInt(fill.Qty))
	}
	if !next.IsInt64() || next.Int64() != fill.PostPosition {
		return false, false
	}
	beforeGap := new(big.Int).Sub(big.NewInt(state.obligation), big.NewInt(fill.PrePosition))
	afterGap := new(big.Int).Sub(big.NewInt(state.obligation), big.NewInt(fill.PostPosition))
	reducesGap = new(big.Int).Abs(afterGap).Cmp(new(big.Int).Abs(beforeGap)) < 0
	state.position = fill.PostPosition
	return true, reducesGap
}

func validLiabilityHedgerCensor(d liabilityHedgerDecision) (bool, bool) {
	switch d.OutcomeExpectation {
	case "VENUE_OUTCOME_REQUIRED":
		return false, d.CensorReason == ""
	case "SIMULATION_HORIZON_CENSORED":
		return true, d.CensorReason == "terminal_horizon_before_venue_ingress"
	default:
		return false, false
	}
}

func validLiabilityHedgerOutcome(d liabilityHedgerDecision, order liabilityHedgerOrder) bool {
	return order.Symbol == "CDF/USD" && order.Side == d.Side && order.Type == "LIMIT" && order.TimeInForce == "IOC" && !order.PostOnly && order.Price == d.LimitPrice && order.Qty == d.RequestedQty
}

func liabilityHedgerFee(qty, price, bps int64) (int64, bool) {
	if qty <= 0 || price <= 0 || bps < 0 {
		return 0, false
	}
	value := new(big.Int).Mul(big.NewInt(qty), big.NewInt(price))
	value.Quo(value, big.NewInt(100_000_000))
	value.Mul(value, big.NewInt(bps))
	value.Quo(value, big.NewInt(10_000))
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func liabilityHedgerHasExternalCounterparty(trades []liabilityHedgerTrade, orders map[liabilityHedgerOrderKey]liabilityHedgerOrder, own liabilityHedgerOrderKey, fill liabilityHedgerFill) bool {
	for _, trade := range trades {
		if trade.TradeID != fill.TradeID || trade.Qty != fill.Qty || trade.Price != fill.Price {
			continue
		}
		other := trade.MakerOrderID
		if other == own.orderID {
			other = trade.TakerOrderID
		}
		if other == 0 || other == own.orderID {
			continue
		}
		if _, ok := orders[liabilityHedgerOrderKey{venueID: own.venueID, orderID: other}]; ok {
			return true
		}
	}
	return false
}

func liabilityHedgerHasSelfCounterparty(trades []liabilityHedgerTrade, orders map[liabilityHedgerOrderKey]liabilityHedgerOrder, own liabilityHedgerOrderKey, fill liabilityHedgerFill, clientID uint64) bool {
	for _, trade := range trades {
		if trade.TradeID != fill.TradeID || trade.Qty != fill.Qty || trade.Price != fill.Price {
			continue
		}
		other := trade.MakerOrderID
		if other == own.orderID {
			other = trade.TakerOrderID
		}
		if order, ok := orders[liabilityHedgerOrderKey{venueID: own.venueID, orderID: other}]; ok && order.ClientID == clientID {
			return true
		}
	}
	return false
}

func liabilityHedgerFlowSeed(master int64, venueIndex, participant, flowClass int) int64 {
	value := uint64(master) + 0x9e3779b97f4a7c15
	value ^= uint64(venueIndex+1) * 0xbf58476d1ce4e5b9
	value ^= uint64(participant+1) * 0x94d049bb133111eb
	value ^= uint64(flowClass) * 0xd6e8feb86659fd93
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	return int64(value & ((uint64(1) << 63) - 1))
}
