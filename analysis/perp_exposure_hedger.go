package analysis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"exchange_sim/exchange"
)

// PerpExposureHedgerAudit independently replays V2-5 P2's finite physical
// exposure policy from evidence. It deliberately does not import the actor or
// multivenue package: the persisted configuration, receipt sidecars, gateway
// decisions, and venue events are the only sources of this verdict.
type PerpExposureHedgerAudit struct {
	Decisions                 int64  `json:"decisions"`
	EnabledDecisions          int64  `json:"enabled_decisions"`
	DisabledDecisions         int64  `json:"disabled_decisions"`
	StateUpdates              int64  `json:"state_updates"`
	Submitted                 int64  `json:"submitted"`
	Deferred                  int64  `json:"deferred"`
	Accepted                  int64  `json:"accepted"`
	Rejected                  int64  `json:"rejected"`
	HorizonCensored           int64  `json:"horizon_censored"`
	Fills                     int64  `json:"fills"`
	FilledQty                 int64  `json:"filled_qty"`
	CancelledIOC              int64  `json:"cancelled_ioc"`
	AutoPerpBorrowEvents      int64  `json:"auto_perp_borrow_events"`
	AutoPerpBorrowedQuote     string `json:"auto_perp_borrowed_quote"`
	UnexpectedAutoPerpBorrows int64  `json:"unexpected_auto_perp_borrows"`
	InvalidBorrowEvents       int64  `json:"invalid_borrow_events"`
	AbsoluteGapSum            string `json:"absolute_gap_sum"`
	GapSamples                int64  `json:"gap_samples"`

	ReceiptAuditValid       bool  `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors   int64 `json:"receipt_evidence_errors"`
	ReceiptMatches          int64 `json:"receipt_matches"`
	MissingReceipts         int64 `json:"missing_receipts"`
	AmbiguousReceipts       int64 `json:"ambiguous_receipts"`
	ReceiptMismatches       int64 `json:"receipt_mismatches"`
	FutureReceiptUse        int64 `json:"future_receipt_use"`
	MissingGatewayDecisions int64 `json:"missing_gateway_decisions"`
	GatewayMismatches       int64 `json:"gateway_mismatches"`

	InvalidDecisionRecords int64 `json:"invalid_decision_records"`
	StateMismatches        int64 `json:"state_mismatches"`
	DecisionMismatches     int64 `json:"decision_mismatches"`
	DisabledSubmissions    int64 `json:"disabled_submissions"`
	DuplicateDecisions     int64 `json:"duplicate_decisions"`
	MissingOutcomes        int64 `json:"missing_outcomes"`
	DuplicateOutcomes      int64 `json:"duplicate_outcomes"`
	OutcomeMismatches      int64 `json:"outcome_mismatches"`
	MissingIOCTerminals    int64 `json:"missing_ioc_terminals"`
	DuplicateIOCTerminals  int64 `json:"duplicate_ioc_terminals"`
	FillQuantityMismatches int64 `json:"fill_quantity_mismatches"`
	MissingFillEvidence    int64 `json:"missing_fill_evidence"`
	UnexpectedFillEvidence int64 `json:"unexpected_fill_evidence"`
	FillEvidenceMismatches int64 `json:"fill_evidence_mismatches"`
	NonReducingFills       int64 `json:"non_reducing_fills"`
	UnknownCounterparties  int64 `json:"unknown_counterparties"`
	SelfFills              int64 `json:"self_fills"`
	NonTakerFills          int64 `json:"non_taker_fills"`
	FeeMismatches          int64 `json:"fee_mismatches"`

	ActionCounts map[string]int64           `json:"action_counts,omitempty"`
	Hedgers      []PerpExposureHedgerBucket `json:"hedgers,omitempty"`
	Checks       []PerpExposureHedgerCheck  `json:"checks,omitempty"`
	Valid        bool                       `json:"valid"`
}

// PerpExposureHedgerBucket retains per-account activation rather than
// averaging independently funded venue-local participants into one actor.
type PerpExposureHedgerBucket struct {
	VenueID             string `json:"venue_id"`
	ClientID            uint64 `json:"client_id"`
	Decisions           int64  `json:"decisions"`
	StateUpdates        int64  `json:"state_updates"`
	Submitted           int64  `json:"submitted"`
	Accepted            int64  `json:"accepted"`
	Fills               int64  `json:"fills"`
	ReducingFills       int64  `json:"reducing_fills"`
	AbsoluteGapSum      string `json:"absolute_gap_sum"`
	GapSamples          int64  `json:"gap_samples"`
	TerminalAbsoluteGap string `json:"terminal_absolute_gap"`
}

// PerpExposureHedgerCheck is an independently predicted evidence failure.
type PerpExposureHedgerCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id,omitempty"`
	OrderID   uint64 `json:"order_id,omitempty"`
	Failure   string `json:"failure"`
}

type perpExposurePolicyConfig struct {
	Enabled                   bool   `json:"enabled"`
	Symbol                    string `json:"symbol"`
	ExposureMode              string `json:"exposure_mode"`
	InitialPhysicalExposure   int64  `json:"initial_physical_exposure"`
	InitialTargetPerpPosition int64  `json:"initial_target_perp_position"`
	AutoBorrowPerp            bool   `json:"auto_borrow_perp"`
	DecisionInterval          int64  `json:"decision_interval"`
	ExposureInterval          int64  `json:"exposure_interval"`
	ExposureStepQty           int64  `json:"exposure_step_qty"`
	MaxAbsExposure            int64  `json:"max_abs_exposure"`
	MaxRequestQty             int64  `json:"max_request_qty"`
	TickSize                  int64  `json:"tick_size"`
	InitialQuoteBalance       int64  `json:"initial_quote_balance"`
	InitialMargin             int64  `json:"initial_margin"`
}

type perpExposureRunConfig struct {
	Seed               int64                     `json:"seed"`
	VenueIDs           []string                  `json:"venue_ids"`
	TakerFeeBps        int64                     `json:"taker_fee_bps"`
	PerpExposureHedger *perpExposurePolicyConfig `json:"perp_exposure_hedger"`
}

type perpExposureManifest struct {
	Config perpExposureRunConfig `json:"config"`
}

type perpExposureDecision struct {
	VenueID               string `json:"venue_id"`
	Hedger                string `json:"hedger"`
	ClientID              uint64 `json:"client_id"`
	PolicyVersion         string `json:"policy_version"`
	ExposureMode          string `json:"exposure_mode"`
	Symbol                string `json:"symbol"`
	DecisionTime          int64  `json:"decision_time"`
	Enabled               bool   `json:"enabled"`
	Subscribed            bool   `json:"subscribed"`
	RequestPending        bool   `json:"request_pending"`
	Action                string `json:"action_or_defer_reason"`
	PhysicalBefore        int64  `json:"physical_exposure_before"`
	PhysicalAfter         int64  `json:"physical_exposure_after"`
	PhysicalStep          int64  `json:"physical_exposure_step"`
	PhysicalExposureLimit int64  `json:"physical_exposure_limit"`
	FilledPerpPosition    int64  `json:"filled_perp_position"`
	TargetPerpPosition    int64  `json:"target_perp_position"`
	HedgeGap              int64  `json:"hedge_gap"`
	DecisionInterval      int64  `json:"decision_interval"`
	ExposureInterval      int64  `json:"exposure_interval"`
	HasSnapshot           bool   `json:"has_snapshot"`
	BookPublishedAt       int64  `json:"book_published_at"`
	BookSequence          uint64 `json:"book_sequence"`
	BookFingerprint       string `json:"book_fingerprint"`
	HasBid                bool   `json:"has_bid"`
	BidPrice              int64  `json:"bid_price"`
	BidVisibleQty         int64  `json:"bid_visible_qty"`
	HasAsk                bool   `json:"has_ask"`
	AskPrice              int64  `json:"ask_price"`
	AskVisibleQty         int64  `json:"ask_visible_qty"`
	DecisionFrontierLink  uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrd   uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierAt    int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierHash  string `json:"decision_frontier_digest"`
	Side                  string `json:"side"`
	LimitPrice            int64  `json:"limit_price"`
	RequestedQty          int64  `json:"requested_qty"`
	RequestID             uint64 `json:"request_id"`
	TakerFeeBps           int64  `json:"taker_fee_bps"`
	OutcomeExpectation    string `json:"outcome_expectation"`
	CensorReason          string `json:"censor_reason"`
}

type perpExposureFillEvidence struct {
	VenueID      string `json:"venue_id"`
	Hedger       string `json:"hedger"`
	ClientID     uint64 `json:"client_id"`
	Symbol       string `json:"symbol"`
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

type perpExposureOrder struct {
	ClientID    uint64 `json:"client_id"`
	RequestID   uint64 `json:"request_id"`
	OrderID     uint64 `json:"order_id"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	PostOnly    bool   `json:"post_only"`
	Price       int64  `json:"price"`
	Qty         int64  `json:"qty"`
}

type perpExposureFill struct {
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

type perpExposureCancellation struct {
	OrderID      uint64 `json:"order_id"`
	RemainingQty int64  `json:"remaining_qty"`
}

type perpExposureTrade struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type perpExposureKey struct {
	venue           string
	client, request uint64
}
type perpExposureOrderKey struct {
	venue string
	order uint64
}
type perpExposureReceiptKey struct {
	client  uint64
	link    uint32
	ordinal uint64
}
type perpExposureSourceKey struct {
	client    uint64
	link      uint32
	mdType    uint8
	sequence  uint64
	published int64
}

type perpExposureGatewayDecision struct {
	requestID            uint64
	symbol               string
	decisionAt           int64
	price, qty           int64
	side, orderType, tif uint8
}

type perpExposureStateEvent struct {
	venue    string
	client   uint64
	file     string
	ordinal  int64
	decision *perpExposureDecision
	fill     *perpExposureFillEvidence
}

type perpExposureReplayState struct {
	rng                                      *rand.Rand
	physical, position, lastUpdate, lastTick int64
	target                                   int64
	seenFirst                                bool
	entryComplete                            bool
}

const perpExposurePolicyVersion = "v2_5_p2_perp_exposure_v1"
const fixedLiabilityExposureMode = "fixed_liability"
const fixedLiabilityPolicyVersion = "v2_7_fixed_liability_v1"
const fixedDirectionalExposureMode = "fixed_directional"
const fixedDirectionalPolicyVersion = "v2_7_fixed_directional_v1"

func perpExposurePolicyVersionFor(mode string) string {
	if mode == fixedLiabilityExposureMode {
		return fixedLiabilityPolicyVersion
	}
	if mode == fixedDirectionalExposureMode {
		return fixedDirectionalPolicyVersion
	}
	return perpExposurePolicyVersion
}

func fixedExposureMode(mode string) bool {
	return mode == fixedLiabilityExposureMode || mode == fixedDirectionalExposureMode
}

func fixedExposureHeldAction(mode string) string {
	if mode == fixedDirectionalExposureMode {
		return "FIXED_DIRECTIONAL_HELD"
	}
	return "FIXED_LIABILITY_HELD"
}

func perpExposureTarget(state *perpExposureReplayState, policy *perpExposurePolicyConfig) int64 {
	if policy.ExposureMode == fixedDirectionalExposureMode {
		return state.target
	}
	return -state.physical
}

// MeasurePerpExposureHedger verifies the complete P2 local state/action
// chain. A valid outcome proves evidence coherence and ordinary execution; it
// does not assert that funding, basis, or market realism changed.
func (r *Run) MeasurePerpExposureHedger() (*PerpExposureHedgerAudit, error) {
	config, err := loadPerpExposureRunConfig(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validPerpExposureRunConfig(config); err != nil {
		return nil, err
	}
	policy := config.PerpExposureHedger
	result := &PerpExposureHedgerAudit{ActionCounts: make(map[string]int64)}
	add := func(venue string, client, request, order uint64, failure string) {
		result.Checks = append(result.Checks, PerpExposureHedgerCheck{VenueID: venue, ClientID: client, RequestID: request, OrderID: order, Failure: failure})
	}

	sources, frontiers, gateway, sourceVenues, receiptAudit, receiptErr := perpExposureEvidence(r.Dir)
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

	var stateEvents []perpExposureStateEvent
	outcomes := make(map[perpExposureKey][]struct {
		accepted bool
		order    perpExposureOrder
	})
	orders := make(map[perpExposureOrderKey]perpExposureOrder)
	fills := make(map[perpExposureOrderKey][]perpExposureFill)
	cancels := make(map[perpExposureOrderKey][]perpExposureCancellation)
	trades := make(map[perpExposureOrderKey][]perpExposureTrade)
	fillEvidence := make(map[perpExposureOrderKey][]perpExposureFillEvidence)
	autoPerpBorrowed := new(big.Int)
	err = r.Scan(ScanOptions{Events: []string{"perp_exposure_hedger_decision", "perp_exposure_hedger_fill", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "Trade", "borrow"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "perp_exposure_hedger_decision":
			var d perpExposureDecision
			if event.Decode(&d) != nil || d.ClientID == 0 || d.VenueID != event.VenueID || d.ClientID != event.ClientID || d.Hedger == "" || d.Symbol != "ABC-PERP" || r.Role(event.VenueID, event.ClientID) != "perp_exposure_hedger" {
				result.InvalidDecisionRecords++
				add(event.VenueID, event.ClientID, 0, 0, "invalid_decision_record")
				return
			}
			result.Decisions++
			result.ActionCounts[d.Action]++
			if d.Enabled {
				result.EnabledDecisions++
			} else {
				result.DisabledDecisions++
			}
			copy := d
			stateEvents = append(stateEvents, perpExposureStateEvent{venue: event.VenueID, client: event.ClientID, file: event.File, ordinal: event.Ordinal, decision: &copy})
		case "perp_exposure_hedger_fill":
			var f perpExposureFillEvidence
			if event.Decode(&f) != nil || f.ClientID == 0 || f.VenueID != event.VenueID || f.ClientID != event.ClientID || f.Hedger == "" || f.Symbol != "ABC-PERP" || f.OrderID == 0 || f.Qty <= 0 {
				result.UnexpectedFillEvidence++
				add(event.VenueID, event.ClientID, 0, f.OrderID, "invalid_fill_evidence")
				return
			}
			key := perpExposureOrderKey{event.VenueID, f.OrderID}
			fillEvidence[key] = append(fillEvidence[key], f)
			copy := f
			stateEvents = append(stateEvents, perpExposureStateEvent{venue: event.VenueID, client: event.ClientID, file: event.File, ordinal: event.Ordinal, fill: &copy})
		case "OrderAccepted", "OrderRejected":
			var order perpExposureOrder
			if event.Decode(&order) != nil || order.RequestID == 0 {
				return
			}
			if order.ClientID == 0 {
				order.ClientID = event.ClientID
			}
			key := perpExposureKey{event.VenueID, event.ClientID, order.RequestID}
			outcomes[key] = append(outcomes[key], struct {
				accepted bool
				order    perpExposureOrder
			}{event.Name == "OrderAccepted", order})
			if event.Name == "OrderAccepted" && order.OrderID != 0 {
				orders[perpExposureOrderKey{event.VenueID, order.OrderID}] = order
			}
		case "OrderFill":
			var fill perpExposureFill
			if event.Decode(&fill) == nil && fill.OrderID != 0 && fill.Qty > 0 {
				fill.Timestamp = event.SimTS
				key := perpExposureOrderKey{event.VenueID, fill.OrderID}
				fills[key] = append(fills[key], fill)
			}
		case "OrderCancelled":
			var cancel perpExposureCancellation
			if event.Decode(&cancel) == nil && cancel.OrderID != 0 {
				key := perpExposureOrderKey{event.VenueID, cancel.OrderID}
				cancels[key] = append(cancels[key], cancel)
			}
		case "Trade":
			var trade perpExposureTrade
			if event.Decode(&trade) != nil || trade.Qty <= 0 {
				return
			}
			if trade.TakerOrderID != 0 {
				key := perpExposureOrderKey{event.VenueID, trade.TakerOrderID}
				trades[key] = append(trades[key], trade)
			}
			if trade.MakerOrderID != 0 {
				key := perpExposureOrderKey{event.VenueID, trade.MakerOrderID}
				trades[key] = append(trades[key], trade)
			}
		case "borrow":
			var borrow exchange.BorrowEvent
			if event.Decode(&borrow) != nil || borrow.ClientID == 0 || borrow.Asset == "" || borrow.Amount <= 0 {
				result.InvalidBorrowEvents++
				return
			}
			if borrow.Reason != "auto_perp" {
				return
			}
			result.AutoPerpBorrowEvents++
			autoPerpBorrowed.Add(autoPerpBorrowed, big.NewInt(borrow.Amount))
			if !policy.AutoBorrowPerp || borrow.Asset != "USD" || r.Role(event.VenueID, event.ClientID) != "perp_exposure_hedger" {
				result.UnexpectedAutoPerpBorrows++
			}
		}
	})
	if err != nil {
		return nil, err
	}
	result.AutoPerpBorrowedQuote = autoPerpBorrowed.String()
	if result.Decisions == 0 {
		result.InvalidDecisionRecords++
		add("", 0, 0, 0, "missing_perp_exposure_decisions")
	}

	sort.Slice(stateEvents, func(i, j int) bool {
		if stateEvents[i].venue != stateEvents[j].venue {
			return stateEvents[i].venue < stateEvents[j].venue
		}
		if stateEvents[i].client != stateEvents[j].client {
			return stateEvents[i].client < stateEvents[j].client
		}
		if stateEvents[i].file != stateEvents[j].file {
			return stateEvents[i].file < stateEvents[j].file
		}
		return stateEvents[i].ordinal < stateEvents[j].ordinal
	})
	venueIndex := make(map[string]int, len(config.VenueIDs))
	for index, venue := range config.VenueIDs {
		venueIndex[venue] = index
	}
	states := make(map[Participant]*perpExposureReplayState)
	buckets := make(map[Participant]*PerpExposureHedgerBucket)
	filesByParticipant := make(map[Participant]string)
	seenTicks := make(map[Participant]map[int64]bool)
	gapSums := make(map[Participant]*big.Int)
	gapCounts := make(map[Participant]int64)
	expected := make(map[perpExposureKey]perpExposureDecision)
	for _, event := range stateEvents {
		participant := Participant{VenueID: event.venue, ClientID: event.client}
		state := states[participant]
		if state == nil {
			index, ok := venueIndex[event.venue]
			if !ok {
				result.StateMismatches++
				add(event.venue, event.client, 0, 0, "unknown_venue")
				continue
			}
			state = &perpExposureReplayState{rng: rand.New(rand.NewSource(perpExposureFlowSeed(config.Seed, index, 0, 16)))}
			switch policy.ExposureMode {
			case fixedLiabilityExposureMode:
				state.physical = policy.InitialPhysicalExposure
			case fixedDirectionalExposureMode:
				state.target = policy.InitialTargetPerpPosition
			}
			states[participant] = state
			buckets[participant] = &PerpExposureHedgerBucket{VenueID: event.venue, ClientID: event.client}
			seenTicks[participant] = make(map[int64]bool)
		}
		if previous, ok := filesByParticipant[participant]; ok && previous != event.file {
			result.StateMismatches++
			add(event.venue, event.client, 0, 0, "state_evidence_spans_multiple_causal_files")
		} else {
			filesByParticipant[participant] = event.file
		}
		if event.fill != nil {
			reducesGap := perpExposureFillReducesGap(*event.fill, state, policy)
			if !perpExposureApplyFill(*event.fill, state) {
				result.FillEvidenceMismatches++
				add(event.venue, event.client, 0, event.fill.OrderID, "actor_position_transition_mismatch")
			} else if !reducesGap {
				result.NonReducingFills++
				add(event.venue, event.client, 0, event.fill.OrderID, "fill_does_not_reduce_hedge_gap")
			} else {
				buckets[participant].ReducingFills++
			}
			if fixedExposureMode(policy.ExposureMode) && state.position == perpExposureTarget(state, policy) {
				state.entryComplete = true
			}
			continue
		}
		d := *event.decision
		bucket := buckets[participant]
		bucket.Decisions++
		if seenTicks[participant][d.DecisionTime] {
			result.DuplicateDecisions++
			add(d.VenueID, d.ClientID, d.RequestID, 0, "duplicate_decision_tick")
		}
		seenTicks[participant][d.DecisionTime] = true
		valid, updated, submitted := validatePerpExposureDecision(d, state, policy, config.TakerFeeBps, terminalAt)
		if updated {
			result.StateUpdates++
			bucket.StateUpdates++
		}
		if !valid {
			result.DecisionMismatches++
			add(d.VenueID, d.ClientID, d.RequestID, 0, "decision_policy_or_state_mismatch")
		}
		if d.HasSnapshot {
			source, err := perpExposureSourceInFrontier(d, sources, frontiers, sourceVenues)
			if err != nil {
				perpExposureReceiptFailure(result, d, err, add)
			} else {
				result.ReceiptMatches++
				if d.BookFingerprint != hex.EncodeToString(source.fingerprint[:]) {
					result.ReceiptMismatches++
					add(d.VenueID, d.ClientID, d.RequestID, 0, "cached_book_fingerprint_mismatch")
				}
			}
		} else if err := perpExposureValidateEmptyFrontier(d, frontiers, sourceVenues); err != nil {
			perpExposureReceiptFailure(result, d, err, add)
		}
		if !d.Enabled && d.RequestID != 0 {
			result.DisabledSubmissions++
			add(d.VenueID, d.ClientID, d.RequestID, 0, "disabled_policy_submitted")
		}
		if !submitted {
			result.Deferred++
			if d.Action == "SIMULATION_HORIZON_CENSORED" {
				result.HorizonCensored++
			}
			if d.RequestID != 0 {
				result.DecisionMismatches++
				add(d.VenueID, d.ClientID, d.RequestID, 0, "deferred_action_has_request_id")
			}
		} else {
			result.Submitted++
			bucket.Submitted++
			key := perpExposureKey{d.VenueID, d.ClientID, d.RequestID}
			if _, exists := expected[key]; exists {
				result.DuplicateDecisions++
				add(d.VenueID, d.ClientID, d.RequestID, 0, "duplicate_submission")
			}
			expected[key] = d
			if gatewayDecision, found := gateway[key]; !found {
				result.MissingGatewayDecisions++
				add(d.VenueID, d.ClientID, d.RequestID, 0, "missing_gateway_decision")
			} else if !perpExposureGatewayMatches(d, gatewayDecision) {
				result.GatewayMismatches++
				add(d.VenueID, d.ClientID, d.RequestID, 0, "gateway_decision_mismatch")
			}
		}
		sum := gapSums[participant]
		if sum == nil {
			sum = new(big.Int)
			gapSums[participant] = sum
		}
		sum.Add(sum, new(big.Int).Abs(big.NewInt(d.HedgeGap)))
		gapCounts[participant]++
	}

	accepted := make(map[perpExposureOrderKey]perpExposureDecision)
	for key, d := range expected {
		rows := outcomes[key]
		if len(rows) == 0 {
			result.MissingOutcomes++
			add(key.venue, key.client, key.request, 0, "missing_venue_outcome")
			continue
		}
		if len(rows) != 1 {
			result.DuplicateOutcomes++
			add(key.venue, key.client, key.request, 0, "duplicate_venue_outcome")
			continue
		}
		row := rows[0]
		if !perpExposureOutcomeMatches(d, row.order) {
			result.OutcomeMismatches++
			add(key.venue, key.client, key.request, row.order.OrderID, "venue_outcome_mismatch")
		}
		if !row.accepted {
			result.Rejected++
			continue
		}
		result.Accepted++
		if bucket := buckets[Participant{VenueID: key.venue, ClientID: key.client}]; bucket != nil {
			bucket.Accepted++
		}
		if row.order.OrderID == 0 {
			result.OutcomeMismatches++
			add(key.venue, key.client, key.request, 0, "accepted_without_order_id")
			continue
		}
		accepted[perpExposureOrderKey{key.venue, row.order.OrderID}] = d
	}
	matchedEvidence := make(map[perpExposureOrderKey]map[int]bool)
	for key, d := range accepted {
		order, known := orders[key]
		if !known {
			result.OutcomeMismatches++
			add(key.venue, 0, 0, key.order, "accepted_order_missing_from_order_index")
			continue
		}
		var total int64
		for _, fill := range fills[key] {
			total += fill.Qty
			result.Fills++
			result.FilledQty += fill.Qty
			if bucket := buckets[Participant{VenueID: key.venue, ClientID: order.ClientID}]; bucket != nil {
				bucket.Fills++
			}
			if fill.Role != "taker" {
				result.NonTakerFills++
				add(key.venue, order.ClientID, order.RequestID, key.order, "perp_hedge_fill_not_taker")
			}
			fee, feeOK := perpExposureFee(fill.Qty, fill.Price, d.TakerFeeBps)
			assetOK := (fee == 0 && fill.FeeAsset == "") || (fee > 0 && fill.FeeAsset == "USD")
			if !feeOK || fee != fill.FeeAmount || !assetOK {
				result.FeeMismatches++
				add(key.venue, order.ClientID, order.RequestID, key.order, "fee_formula_mismatch")
			}
			if !perpExposureHasExternalCounterparty(trades[key], orders, key, fill) {
				result.UnknownCounterparties++
				add(key.venue, order.ClientID, order.RequestID, key.order, "missing_external_counterparty")
			} else if perpExposureSelfFill(trades[key], orders, key, fill, order.ClientID) {
				result.SelfFills++
				add(key.venue, order.ClientID, order.RequestID, key.order, "self_fill")
			}
			matched := false
			for index, evidence := range fillEvidence[key] {
				if matchedEvidence[key] != nil && matchedEvidence[key][index] {
					continue
				}
				if evidence.ClientID == order.ClientID && evidence.Timestamp == fill.Timestamp && evidence.TradeID == fill.TradeID && evidence.Side == fill.Side && evidence.Qty == fill.Qty && evidence.Price == fill.Price && evidence.FeeAmount == fill.FeeAmount && evidence.FeeAsset == fill.FeeAsset {
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
				add(key.venue, order.ClientID, order.RequestID, key.order, "missing_exact_actor_fill_evidence")
			}
		}
		for index := range fillEvidence[key] {
			if !matchedEvidence[key][index] {
				result.UnexpectedFillEvidence++
				result.FillEvidenceMismatches++
				add(key.venue, order.ClientID, order.RequestID, key.order, "unexpected_actor_fill_evidence")
			}
		}
		if total > order.Qty {
			result.FillQuantityMismatches++
			add(key.venue, order.ClientID, order.RequestID, key.order, "filled_qty_exceeds_ioc")
		}
		if total < order.Qty {
			rows := cancels[key]
			if len(rows) == 0 {
				result.MissingIOCTerminals++
				add(key.venue, order.ClientID, order.RequestID, key.order, "missing_ioc_cancellation")
			} else if len(rows) != 1 || rows[0].RemainingQty != order.Qty-total {
				result.DuplicateIOCTerminals++
				add(key.venue, order.ClientID, order.RequestID, key.order, "invalid_ioc_cancellation")
			} else {
				result.CancelledIOC++
			}
		} else if len(cancels[key]) != 0 {
			result.DuplicateIOCTerminals++
			add(key.venue, order.ClientID, order.RequestID, key.order, "full_ioc_cancelled")
		}
	}
	for key, rows := range fillEvidence {
		if _, known := accepted[key]; !known {
			for range rows {
				result.UnexpectedFillEvidence++
				add(key.venue, 0, 0, key.order, "actor_fill_without_accepted_p2_order")
			}
		}
	}

	totalGap := new(big.Int)
	for participant, bucket := range buckets {
		if sum := gapSums[participant]; sum != nil {
			bucket.AbsoluteGapSum = sum.String()
			totalGap.Add(totalGap, sum)
		} else {
			bucket.AbsoluteGapSum = "0"
		}
		bucket.GapSamples = gapCounts[participant]
		result.GapSamples += bucket.GapSamples
		if state := states[participant]; state != nil {
			bucket.TerminalAbsoluteGap = new(big.Int).Abs(new(big.Int).Sub(big.NewInt(perpExposureTarget(state, policy)), big.NewInt(state.position))).String()
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
		a, b := result.Checks[i], result.Checks[j]
		if a.VenueID != b.VenueID {
			return a.VenueID < b.VenueID
		}
		if a.ClientID != b.ClientID {
			return a.ClientID < b.ClientID
		}
		if a.RequestID != b.RequestID {
			return a.RequestID < b.RequestID
		}
		if a.OrderID != b.OrderID {
			return a.OrderID < b.OrderID
		}
		return a.Failure < b.Failure
	})
	result.Valid = result.Decisions > 0 && result.ReceiptAuditValid && result.ReceiptEvidenceErrors == 0 && result.MissingReceipts == 0 && result.AmbiguousReceipts == 0 && result.ReceiptMismatches == 0 && result.FutureReceiptUse == 0 && result.MissingGatewayDecisions == 0 && result.GatewayMismatches == 0 && result.InvalidDecisionRecords == 0 && result.InvalidBorrowEvents == 0 && result.UnexpectedAutoPerpBorrows == 0 && result.StateMismatches == 0 && result.DecisionMismatches == 0 && result.DisabledSubmissions == 0 && result.DuplicateDecisions == 0 && result.MissingOutcomes == 0 && result.DuplicateOutcomes == 0 && result.OutcomeMismatches == 0 && result.MissingIOCTerminals == 0 && result.DuplicateIOCTerminals == 0 && result.FillQuantityMismatches == 0 && result.MissingFillEvidence == 0 && result.UnexpectedFillEvidence == 0 && result.FillEvidenceMismatches == 0 && result.NonReducingFills == 0 && result.UnknownCounterparties == 0 && result.SelfFills == 0 && result.NonTakerFills == 0 && result.FeeMismatches == 0
	return result, nil
}

func loadPerpExposureRunConfig(dir string) (perpExposureRunConfig, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return perpExposureRunConfig{}, fmt.Errorf("read P2 manifest: %w", err)
	}
	var manifest perpExposureManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return perpExposureRunConfig{}, fmt.Errorf("decode P2 manifest: %w", err)
	}
	return manifest.Config, nil
}

func validPerpExposureRunConfig(c perpExposureRunConfig) error {
	p := c.PerpExposureHedger
	if p == nil || len(c.VenueIDs) == 0 || p.Symbol != "ABC-PERP" || (p.ExposureMode != "" && p.ExposureMode != fixedLiabilityExposureMode && p.ExposureMode != fixedDirectionalExposureMode) || p.DecisionInterval <= 0 || p.ExposureInterval <= 0 || p.ExposureInterval%p.DecisionInterval != 0 || p.ExposureStepQty <= 0 || p.MaxAbsExposure < p.ExposureStepQty || p.MaxAbsExposure > math.MaxInt64-p.ExposureStepQty || p.MaxRequestQty <= 0 || p.TickSize <= 0 || p.InitialQuoteBalance <= 0 || p.InitialMargin <= 0 || c.TakerFeeBps < 0 || c.TakerFeeBps > 10_000 {
		return fmt.Errorf("unsupported P2 policy/configuration")
	}
	if p.ExposureMode == fixedLiabilityExposureMode && (p.InitialPhysicalExposure == 0 || p.InitialPhysicalExposure > p.MaxAbsExposure || p.InitialPhysicalExposure < -p.MaxAbsExposure) {
		return fmt.Errorf("unsupported fixed-liability exposure")
	}
	if p.ExposureMode == fixedDirectionalExposureMode {
		if (p.Enabled && p.InitialTargetPerpPosition == 0) || p.InitialTargetPerpPosition > p.MaxAbsExposure || p.InitialTargetPerpPosition < -p.MaxAbsExposure || !p.AutoBorrowPerp {
			return fmt.Errorf("unsupported fixed-directional exposure")
		}
	} else if p.AutoBorrowPerp {
		return fmt.Errorf("unsupported perpetual auto-borrow policy")
	}
	seen := make(map[string]bool, len(c.VenueIDs))
	for _, venue := range c.VenueIDs {
		if venue == "" || seen[venue] {
			return fmt.Errorf("invalid P2 venue IDs")
		}
		seen[venue] = true
	}
	return nil
}

func validatePerpExposureDecision(d perpExposureDecision, state *perpExposureReplayState, p *perpExposurePolicyConfig, expectedTakerFeeBps, terminalAt int64) (bool, bool, bool) {
	if d.PolicyVersion != perpExposurePolicyVersionFor(p.ExposureMode) || d.ExposureMode != p.ExposureMode || d.Symbol != p.Symbol || d.DecisionTime <= 0 || d.Enabled != p.Enabled || d.PhysicalExposureLimit != p.MaxAbsExposure || d.DecisionInterval != p.DecisionInterval || d.ExposureInterval != p.ExposureInterval || d.TakerFeeBps < 0 {
		return false, false, false
	}
	if d.TakerFeeBps != expectedTakerFeeBps {
		return false, false, false
	}
	if !perpExposureValidBook(d) {
		return false, false, false
	}
	if !state.seenFirst {
		state.seenFirst, state.lastTick = true, d.DecisionTime
		return !d.Subscribed && d.Action == "NOT_SUBSCRIBED" && d.PhysicalBefore == state.physical && d.PhysicalAfter == state.physical && d.PhysicalStep == 0 && d.FilledPerpPosition == 0 && d.TargetPerpPosition == 0 && d.HedgeGap == 0 && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "" && !d.HasSnapshot, false, false
	}
	if d.DecisionTime-state.lastTick != p.DecisionInterval || !d.Subscribed || d.PhysicalBefore != state.physical || d.FilledPerpPosition != state.position {
		return false, false, false
	}
	state.lastTick = d.DecisionTime
	updated := false
	if fixedExposureMode(p.ExposureMode) {
		if d.PhysicalStep != 0 || d.PhysicalAfter != state.physical {
			return false, false, false
		}
	} else if state.lastUpdate == 0 || d.DecisionTime-state.lastUpdate >= p.ExposureInterval {
		step, next, ok := perpExposureNextStep(state, p)
		if !ok || d.PhysicalStep != step || d.PhysicalAfter != next {
			return false, false, false
		}
		state.physical, state.lastUpdate, updated = next, d.DecisionTime, true
	} else if d.PhysicalStep != 0 || d.PhysicalAfter != state.physical {
		return false, false, false
	}
	target := big.NewInt(perpExposureTarget(state, p))
	gap := new(big.Int).Sub(target, big.NewInt(state.position))
	if !target.IsInt64() || !gap.IsInt64() || d.TargetPerpPosition != target.Int64() || d.HedgeGap != gap.Int64() {
		return false, updated, false
	}
	if !p.Enabled {
		return d.Enabled == false && d.Action == "POLICY_DISABLED" && d.RequestID == 0, updated, false
	}
	if !d.Enabled {
		return false, updated, false
	}
	if fixedExposureMode(p.ExposureMode) && state.entryComplete {
		return d.Action == fixedExposureHeldAction(p.ExposureMode) && d.RequestID == 0 && d.RequestedQty == 0 && d.Side == "", updated, false
	}
	if d.RequestPending {
		return d.Action == "REQUEST_PENDING" && d.RequestID == 0, updated, false
	}
	if gap.Sign() == 0 {
		return d.Action == "IN_BAND" && d.RequestID == 0, updated, false
	}
	if perpExposureCensored(d.DecisionTime, p.DecisionInterval, terminalAt) {
		return d.Action == "SIMULATION_HORIZON_CENSORED" && d.RequestID == 0 && d.OutcomeExpectation == "SIMULATION_HORIZON_CENSORED" && d.CensorReason == "terminal_horizon_before_round_trip", updated, false
	}
	if !d.HasSnapshot {
		return d.Action == "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE" && d.RequestID == 0, updated, false
	}
	if d.BookPublishedAt > d.DecisionTime {
		return d.Action == "LOCAL_BOOK_PUBLICATION_FUTURE" && d.RequestID == 0, updated, false
	}
	qty := new(big.Int).Abs(gap)
	if qty.Cmp(big.NewInt(p.MaxRequestQty)) > 0 {
		qty.SetInt64(p.MaxRequestQty)
	}
	if !qty.IsInt64() || qty.Sign() <= 0 {
		return d.Action == "ZERO_REQUEST_QUANTITY" && d.RequestID == 0, updated, false
	}
	side, hasTouch, price := exchange.Sell.String(), d.HasBid, d.BidPrice
	if gap.Sign() > 0 {
		side, hasTouch, price = exchange.Buy.String(), d.HasAsk, d.AskPrice
	}
	if !hasTouch {
		return d.Action == "LOCAL_EXECUTABLE_PRICE_UNAVAILABLE" && d.RequestID == 0 && d.Side == side && d.RequestedQty == qty.Int64(), updated, false
	}
	if price <= 0 || price%p.TickSize != 0 {
		return d.Action == "PERP_PRICE_OUTSIDE_DOMAIN" && d.RequestID == 0 && d.Side == side && d.LimitPrice == price && d.RequestedQty == qty.Int64(), updated, false
	}
	return d.Action == "SUBMIT_IOC" && d.Side == side && d.LimitPrice == price && d.RequestedQty == qty.Int64() && d.RequestID != 0 && d.OutcomeExpectation == "VENUE_OUTCOME_REQUIRED" && d.CensorReason == "", updated, true
}

func perpExposureValidBook(d perpExposureDecision) bool {
	if !d.HasSnapshot {
		return d.BookPublishedAt == 0 && d.BookSequence == 0 && d.BookFingerprint == hex.EncodeToString(make([]byte, 16)) && !d.HasBid && d.BidPrice == 0 && d.BidVisibleQty == 0 && !d.HasAsk && d.AskPrice == 0 && d.AskVisibleQty == 0
	}
	return len(d.BookFingerprint) == 32 && (!d.HasBid && d.BidPrice == 0 && d.BidVisibleQty == 0 || d.HasBid) && (!d.HasAsk && d.AskPrice == 0 && d.AskVisibleQty == 0 || d.HasAsk)
}

func perpExposureNextStep(s *perpExposureReplayState, p *perpExposurePolicyConfig) (int64, int64, bool) {
	step := p.ExposureStepQty
	if s.rng.Intn(2) == 0 {
		step = -step
	}
	next := new(big.Int).Add(big.NewInt(s.physical), big.NewInt(step))
	limit := big.NewInt(p.MaxAbsExposure)
	lower := new(big.Int).Neg(new(big.Int).Set(limit))
	if next.Cmp(limit) > 0 || next.Cmp(lower) < 0 {
		step = -step
		next.SetInt64(s.physical)
		next.Add(next, big.NewInt(step))
	}
	if !next.IsInt64() || next.Cmp(limit) > 0 || next.Cmp(lower) < 0 || step == 0 {
		return 0, s.physical, false
	}
	return step, next.Int64(), true
}

func perpExposureCensored(now, interval, terminal int64) bool {
	if terminal == 0 {
		return false
	}
	deadline := new(big.Int).Add(big.NewInt(now), big.NewInt(interval))
	deadline.Add(deadline, big.NewInt(interval))
	return !deadline.IsInt64() || deadline.Int64() > terminal
}

func perpExposureApplyFill(f perpExposureFillEvidence, s *perpExposureReplayState) bool {
	if !s.seenFirst || f.PrePosition != s.position || f.Qty <= 0 || (f.Side != "BUY" && f.Side != "SELL") {
		return false
	}
	next := new(big.Int).SetInt64(s.position)
	if f.Side == "BUY" {
		next.Add(next, big.NewInt(f.Qty))
	} else {
		next.Sub(next, big.NewInt(f.Qty))
	}
	if !next.IsInt64() || next.Int64() != f.PostPosition {
		return false
	}
	s.position = f.PostPosition
	return true
}

func perpExposureFillReducesGap(f perpExposureFillEvidence, s *perpExposureReplayState, p *perpExposurePolicyConfig) bool {
	if !s.seenFirst || f.PrePosition != s.position {
		return false
	}
	// The replay state stores the explicit directional target for fixed
	// mandates and derives the opposite hedge target for physical-liability
	// policies. Both paths remain independent of actor memory.
	target := big.NewInt(perpExposureTarget(s, p))
	before := new(big.Int).Sub(target, big.NewInt(f.PrePosition))
	after := new(big.Int).Sub(target, big.NewInt(f.PostPosition))
	return new(big.Int).Abs(after).Cmp(new(big.Int).Abs(before)) < 0
}

func perpExposureEvidence(dir string) (map[perpExposureSourceKey][]observationRecord, map[perpExposureReceiptKey]auditedFrontier, map[perpExposureKey]perpExposureGatewayDecision, map[uint32]string, *MarketDataReceiptAudit, error) {
	audit, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, nil, nil, nil, audit, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, nil, nil, nil, audit, err
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, nil, nil, nil, audit, err
	}
	receiptRaw, _, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, nil, nil, nil, audit, err
	}
	decisionRaw, _, err := readEvidenceFile(dir, manifest.Decisions.File, marketDataDecisionRecordBytes, manifest.Decisions.Records, manifest.Decisions.Digest)
	if err != nil {
		return nil, nil, nil, nil, audit, err
	}
	linkVenue := make(map[uint32]string)
	linkRole := make(map[uint32]string)
	for _, link := range manifest.Links {
		linkVenue[link.ID], linkRole[link.ID] = link.SourceVenue, link.Role
	}
	symbols := make(map[uint32]string)
	for _, symbol := range manifest.Symbols {
		symbols[symbol.ID] = symbol.Symbol
	}
	sources := make(map[perpExposureSourceKey][]observationRecord)
	for offset := 0; offset < len(receiptRaw); offset += marketDataReceiptRecordBytes {
		record := decodeObservation(receiptRaw[offset : offset+marketDataReceiptRecordBytes])
		if linkRole[record.linkID] == "perp_exposure_hedger" && symbols[record.symbolID] == "ABC-PERP" {
			key := perpExposureSourceKey{record.clientID, record.linkID, record.mdType, record.sequence, record.publishedAt}
			sources[key] = append(sources[key], record)
		}
	}
	frontiers := make(map[perpExposureReceiptKey]auditedFrontier)
	for key, frontier := range reconstructReceiptHistory(receiptRaw) {
		frontiers[perpExposureReceiptKey{key.clientID, key.linkID, key.ordinal}] = frontier
	}
	gateways := make(map[perpExposureKey]perpExposureGatewayDecision)
	for offset := 0; offset < len(decisionRaw); offset += marketDataDecisionRecordBytes {
		record := decodeDecision(decisionRaw[offset : offset+marketDataDecisionRecordBytes])
		if linkRole[record.linkID] == "perp_exposure_hedger" && symbols[record.symbolID] == "ABC-PERP" && record.requestID != 0 {
			gateways[perpExposureKey{linkVenue[record.linkID], record.clientID, record.requestID}] = perpExposureGatewayDecision{record.requestID, symbols[record.symbolID], record.decisionAt, record.price, record.qty, record.side, record.orderType, record.tif}
		}
	}
	return sources, frontiers, gateways, linkVenue, audit, nil
}

func perpExposureValidateEmptyFrontier(d perpExposureDecision, frontiers map[perpExposureReceiptKey]auditedFrontier, sourceVenues map[uint32]string) error {
	if d.DecisionFrontierOrd != 0 {
		return nil
	}
	if d.DecisionFrontierLink == 0 || d.DecisionFrontierAt != 0 || d.DecisionFrontierHash != hex.EncodeToString(make([]byte, 16)) {
		return fmt.Errorf("empty_frontier_mismatch")
	}
	if sourceVenues[d.DecisionFrontierLink] != d.VenueID {
		return fmt.Errorf("source_venue_mismatch")
	}
	return nil
}

func perpExposureSourceInFrontier(d perpExposureDecision, sources map[perpExposureSourceKey][]observationRecord, frontiers map[perpExposureReceiptKey]auditedFrontier, sourceVenues map[uint32]string) (observationRecord, error) {
	if d.DecisionFrontierLink == 0 || d.DecisionFrontierOrd == 0 {
		return observationRecord{}, fmt.Errorf("source_frontier_missing")
	}
	if sourceVenues[d.DecisionFrontierLink] != d.VenueID {
		return observationRecord{}, fmt.Errorf("source_venue_mismatch")
	}
	frontier, found := frontiers[perpExposureReceiptKey{d.ClientID, d.DecisionFrontierLink, d.DecisionFrontierOrd}]
	if !found || d.DecisionFrontierAt != frontier.deliveredAt || d.DecisionFrontierHash != hex.EncodeToString(frontier.digest[:]) {
		return observationRecord{}, fmt.Errorf("source_frontier_mismatch")
	}
	if d.DecisionFrontierAt > d.DecisionTime {
		return observationRecord{}, fmt.Errorf("future_receipt")
	}
	rows := sources[perpExposureSourceKey{d.ClientID, d.DecisionFrontierLink, uint8(exchange.MDSnapshot), d.BookSequence, d.BookPublishedAt}]
	if len(rows) == 0 {
		return observationRecord{}, fmt.Errorf("source_missing")
	}
	if len(rows) != 1 {
		return observationRecord{}, fmt.Errorf("source_ambiguous")
	}
	record := rows[0]
	if record.ordinal > d.DecisionFrontierOrd {
		return observationRecord{}, fmt.Errorf("source_after_frontier")
	}
	if record.publishedAt > d.DecisionTime || record.deliveredAt > d.DecisionTime {
		return observationRecord{}, fmt.Errorf("future_receipt")
	}
	return record, nil
}

func perpExposureReceiptFailure(result *PerpExposureHedgerAudit, d perpExposureDecision, err error, add func(string, uint64, uint64, uint64, string)) {
	switch err.Error() {
	case "source_missing", "source_frontier_missing":
		result.MissingReceipts++
	case "source_ambiguous":
		result.AmbiguousReceipts++
	case "future_receipt":
		result.FutureReceiptUse++
	default:
		result.ReceiptMismatches++
	}
	add(d.VenueID, d.ClientID, d.RequestID, 0, err.Error())
}

func perpExposureGatewayMatches(d perpExposureDecision, g perpExposureGatewayDecision) bool {
	return g.requestID == d.RequestID && g.symbol == d.Symbol && g.decisionAt == d.DecisionTime && g.price == d.LimitPrice && g.qty == d.RequestedQty && g.side == uint8(perpExposureSide(d.Side)) && g.orderType == uint8(exchange.LimitOrder) && g.tif == uint8(exchange.IOC)
}
func perpExposureSide(side string) exchange.Side {
	if side == exchange.Sell.String() {
		return exchange.Sell
	}
	return exchange.Buy
}
func perpExposureOutcomeMatches(d perpExposureDecision, order perpExposureOrder) bool {
	return order.Side == d.Side && order.Type == exchange.LimitOrder.String() && order.TimeInForce == exchange.IOC.String() && !order.PostOnly && order.Price == d.LimitPrice && order.Qty == d.RequestedQty
}

func perpExposureFee(qty, price, bps int64) (int64, bool) {
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

func perpExposureSelfFill(trades []perpExposureTrade, orders map[perpExposureOrderKey]perpExposureOrder, own perpExposureOrderKey, fill perpExposureFill, client uint64) bool {
	for _, trade := range trades {
		if trade.TradeID != fill.TradeID || trade.Qty != fill.Qty || trade.Price != fill.Price {
			continue
		}
		other := trade.MakerOrderID
		if other == own.order {
			other = trade.TakerOrderID
		}
		if order, ok := orders[perpExposureOrderKey{own.venue, other}]; ok && order.ClientID == client {
			return true
		}
	}
	return false
}

func perpExposureHasExternalCounterparty(trades []perpExposureTrade, orders map[perpExposureOrderKey]perpExposureOrder, own perpExposureOrderKey, fill perpExposureFill) bool {
	for _, trade := range trades {
		if trade.TradeID != fill.TradeID || trade.Qty != fill.Qty || trade.Price != fill.Price {
			continue
		}
		other := trade.MakerOrderID
		if other == own.order {
			other = trade.TakerOrderID
		}
		if other == 0 || other == own.order {
			continue
		}
		if _, ok := orders[perpExposureOrderKey{venue: own.venue, order: other}]; ok {
			return true
		}
	}
	return false
}

func perpExposureFlowSeed(master int64, venueIndex, participant, flowClass int) int64 {
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
