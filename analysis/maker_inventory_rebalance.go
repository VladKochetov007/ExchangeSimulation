package analysis

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"sort"
)

// MakerInventoryRebalanceAudit independently reconstructs V2-3 P2's local
// CDF/USD IOC policy. It never reads the actor or exchange implementation: the
// inputs are persisted decision rows, V2-0 receipts, and exchange outcomes.
// A zero numeric price is not an absence signal anywhere in this audit.
type MakerInventoryRebalanceAudit struct {
	Decisions         int64 `json:"decisions"`
	DisabledDecisions int64 `json:"disabled_decisions"`
	EnabledDecisions  int64 `json:"enabled_decisions"`
	Submitted         int64 `json:"submitted"`
	Deferred          int64 `json:"deferred"`
	Accepted          int64 `json:"accepted"`
	Rejected          int64 `json:"rejected"`
	HorizonCensored   int64 `json:"horizon_censored"`
	Fills             int64 `json:"fills"`
	FilledQty         int64 `json:"filled_qty"`
	CancelledIOC      int64 `json:"cancelled_ioc"`

	ReceiptAuditValid     bool  `json:"receipt_audit_valid"`
	ReceiptEvidenceErrors int64 `json:"receipt_evidence_errors"`
	ReceiptMatches        int64 `json:"receipt_matches"`
	MissingReceipts       int64 `json:"missing_receipts"`
	AmbiguousReceipts     int64 `json:"ambiguous_receipts"`
	ReceiptMismatches     int64 `json:"receipt_mismatches"`
	FutureReceiptUse      int64 `json:"future_receipt_use"`

	InvalidDecisionRecords    int64            `json:"invalid_decision_records"`
	DecisionFieldMismatches   int64            `json:"decision_field_mismatches"`
	DisabledSubmissions       int64            `json:"disabled_submissions"`
	DuplicateDecisions        int64            `json:"duplicate_decisions"`
	MissingOutcomes           int64            `json:"missing_outcomes"`
	DuplicateOutcomes         int64            `json:"duplicate_outcomes"`
	CensoredOutcomeDeliveries int64            `json:"censored_outcome_deliveries"`
	OutcomeFieldMismatches    int64            `json:"outcome_field_mismatches"`
	MissingIOCTerminals       int64            `json:"missing_ioc_terminals"`
	DuplicateIOCTerminals     int64            `json:"duplicate_ioc_terminals"`
	FillQuantityMismatches    int64            `json:"fill_quantity_mismatches"`
	MissingFillEvidence       int64            `json:"missing_fill_evidence"`
	UnexpectedFillEvidence    int64            `json:"unexpected_fill_evidence"`
	FillEvidenceMismatches    int64            `json:"fill_evidence_mismatches"`
	NonReducingFills          int64            `json:"non_reducing_fills"`
	UnknownCounterparties     int64            `json:"unknown_counterparties"`
	SelfFills                 int64            `json:"self_fills"`
	NonTakerFills             int64            `json:"non_taker_fills"`
	NonPositiveFees           int64            `json:"non_positive_fees"`
	FeeMismatches             int64            `json:"fee_mismatches"`
	ActionCounts              map[string]int64 `json:"action_counts,omitempty"`

	Checks []MakerInventoryRebalanceCheck `json:"checks,omitempty"`
	Valid  bool                           `json:"valid"`
}

// MakerInventoryRebalanceCheck is a stable, specific evidence failure rather
// than an aggregate counter with no provenance.
type MakerInventoryRebalanceCheck struct {
	VenueID   string `json:"venue_id"`
	ClientID  uint64 `json:"client_id"`
	RequestID uint64 `json:"request_id,omitempty"`
	OrderID   uint64 `json:"order_id,omitempty"`
	Failure   string `json:"failure"`
}

type makerInventoryRebalanceDecision struct {
	VenueID              string `json:"venue_id"`
	Maker                string `json:"maker"`
	ClientID             uint64 `json:"client_id"`
	Symbol               string `json:"symbol"`
	DecisionTime         int64  `json:"decision_time"`
	Enabled              bool   `json:"enabled"`
	Subscribed           bool   `json:"subscribed"`
	RequestPending       bool   `json:"request_pending"`
	Action               string `json:"action_or_defer_reason"`
	Inventory            int64  `json:"inventory"`
	RiskBandQty          int64  `json:"risk_band_qty"`
	TargetBandQty        int64  `json:"target_band_qty"`
	LastBookSourceTime   int64  `json:"last_book_source_time"`
	LastBookReceivedTime int64  `json:"last_book_received_time"`
	LastBookSequence     uint64 `json:"last_book_sequence"`
	BidPrice             int64  `json:"bid_price"`
	BidVisibleQty        int64  `json:"bid_visible_qty"`
	AskPrice             int64  `json:"ask_price"`
	AskVisibleQty        int64  `json:"ask_visible_qty"`
	Side                 string `json:"side"`
	DesiredReduction     int64  `json:"desired_reduction"`
	ParticipationCap     int64  `json:"participation_cap"`
	MaxRequestQty        int64  `json:"max_request_qty"`
	ParticipationBps     int64  `json:"participation_bps"`
	SlippageBps          int64  `json:"slippage_bps"`
	EvaluationInterval   int64  `json:"evaluation_interval"`
	Cooldown             int64  `json:"cooldown"`
	LimitPrice           int64  `json:"limit_price"`
	RequestedQty         int64  `json:"requested_qty"`
	TakerFeeBps          int64  `json:"taker_fee_bps"`
	RequestID            uint64 `json:"request_id"`
	CooldownUntil        int64  `json:"cooldown_until"`
	OutcomeExpectation   string `json:"outcome_expectation"`
	CensorReason         string `json:"censor_reason"`
}

type makerInventoryRebalanceOrder struct {
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

type makerInventoryRebalanceFill struct {
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

type makerInventoryRebalanceCancellation struct {
	OrderID      uint64 `json:"order_id"`
	RemainingQty int64  `json:"remaining_qty"`
}

type makerInventoryRebalanceTrade struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type makerInventoryRebalanceFillEvidence struct {
	VenueID       string `json:"venue_id"`
	Maker         string `json:"maker"`
	ClientID      uint64 `json:"client_id"`
	Symbol        string `json:"symbol"`
	Timestamp     int64  `json:"timestamp"`
	OrderID       uint64 `json:"order_id"`
	TradeID       uint64 `json:"trade_id"`
	Side          string `json:"side"`
	Qty           int64  `json:"qty"`
	Price         int64  `json:"price"`
	FeeAmount     int64  `json:"fee_amount"`
	FeeAsset      string `json:"fee_asset"`
	PreInventory  int64  `json:"pre_inventory"`
	PostInventory int64  `json:"post_inventory"`
}

type makerInventoryRebalanceKey struct {
	venueID  string
	clientID uint64
	request  uint64
}

type makerInventoryRebalanceOrderKey struct {
	venueID string
	orderID uint64
}

type makerInventoryRebalanceOutcome struct {
	accepted bool
	order    makerInventoryRebalanceOrder
}

type makerInventoryRebalanceReceiptKey struct {
	venueID   string
	clientID  uint64
	symbol    string
	sequence  uint64
	published int64
}

type makerInventoryRebalanceReceipt struct {
	deliveredAt int64
}

// MeasureMakerInventoryRebalance audits the complete P2 evidence relation.
// It does not infer a successful intervention from a lower terminal inventory:
// only an exact local-policy decision, accepted IOC, external fill, positive
// fee, and local pre/post reduction qualify.
func (r *Run) MeasureMakerInventoryRebalance() (*MakerInventoryRebalanceAudit, error) {
	result := &MakerInventoryRebalanceAudit{ActionCounts: make(map[string]int64)}
	receipts, receiptAudit, receiptErr := makerInventoryRebalanceReceipts(r.Dir)
	if receiptErr != nil {
		result.ReceiptEvidenceErrors++
	} else {
		result.ReceiptAuditValid = receiptAudit.Valid
		if !receiptAudit.Valid {
			result.ReceiptEvidenceErrors++
		}
	}

	expected := make(map[makerInventoryRebalanceKey]makerInventoryRebalanceDecision)
	addCheck := func(venue string, client, request, order uint64, failure string) {
		result.Checks = append(result.Checks, MakerInventoryRebalanceCheck{VenueID: venue, ClientID: client, RequestID: request, OrderID: order, Failure: failure})
	}
	err := r.Scan(ScanOptions{Events: []string{"maker_inventory_rebalance_decision"}, Workers: 1}, func(event Event) {
		var decision makerInventoryRebalanceDecision
		if event.Decode(&decision) != nil || decision.ClientID == 0 || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID ||
			decision.Symbol != "CDF/USD" || decision.Maker == "" || r.Role(event.VenueID, event.ClientID) != "cdf_spot_maker" {
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
		if makerInventoryRebalanceUsesLocalBook(decision.Action) {
			makerInventoryRebalanceCheckReceipt(result, receipts, receiptErr, decision, addCheck)
		}
		if !validMakerInventoryRebalanceDecision(decision) {
			result.DecisionFieldMismatches++
			addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "decision_policy_mismatch")
		}
		if !decision.Enabled {
			if decision.Action != "POLICY_DISABLED" || decision.RequestID != 0 || decision.RequestedQty != 0 {
				result.DisabledSubmissions++
				addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "disabled_policy_submitted")
			}
			return
		}
		if decision.Action != "SUBMIT_IOC" {
			result.Deferred++
			if decision.RequestID != 0 || decision.RequestedQty != 0 {
				result.DecisionFieldMismatches++
				addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "deferred_policy_has_request")
			}
			return
		}
		result.Submitted++
		if decision.RequestID == 0 {
			result.InvalidDecisionRecords++
			addCheck(event.VenueID, event.ClientID, 0, 0, "submission_without_request_id")
			return
		}
		key := makerInventoryRebalanceKey{venueID: event.VenueID, clientID: event.ClientID, request: decision.RequestID}
		if _, duplicate := expected[key]; duplicate {
			result.DuplicateDecisions++
			addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "duplicate_submission_decision")
			return
		}
		expected[key] = decision
	})
	if err != nil {
		return nil, err
	}

	decodeOrder := func(event Event) (makerInventoryRebalanceOrder, bool) {
		var order makerInventoryRebalanceOrder
		if event.Decode(&order) != nil || order.RequestID == 0 {
			return makerInventoryRebalanceOrder{}, false
		}
		if order.ClientID == 0 {
			order.ClientID = event.ClientID
		}
		if order.Symbol == "" {
			order.Symbol = event.Symbol
		}
		// Spot acceptance payloads omit the redundant symbol. The event file
		// identity is the only safe fallback for this P2-specific audit.
		if order.Symbol == "" && pathHasSymbol(event.File, event.VenueID, "CDF-USD") {
			order.Symbol = "CDF/USD"
		}
		return order, true
	}

	outcomes := make(map[makerInventoryRebalanceKey][]makerInventoryRebalanceOutcome, len(expected))
	err = r.Scan(ScanOptions{Events: []string{"OrderAccepted", "OrderRejected"}, Workers: 1}, func(event Event) {
		order, ok := decodeOrder(event)
		if !ok {
			return
		}
		key := makerInventoryRebalanceKey{venueID: event.VenueID, clientID: event.ClientID, request: order.RequestID}
		if _, expected := expected[key]; expected {
			outcomes[key] = append(outcomes[key], makerInventoryRebalanceOutcome{accepted: event.Name == "OrderAccepted", order: order})
		}
	})
	if err != nil {
		return nil, err
	}

	accepted := make(map[makerInventoryRebalanceOrderKey]makerInventoryRebalanceDecision)
	orders := make(map[makerInventoryRebalanceOrderKey]makerInventoryRebalanceOrder)
	for key, decision := range expected {
		outcome := outcomes[key]
		censored, validCensor := validMakerInventoryRebalanceCensor(decision)
		if !validCensor {
			result.DecisionFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, 0, "invalid_outcome_expectation")
		}
		if censored {
			if len(outcome) == 0 {
				result.HorizonCensored++
				continue
			}
			result.CensoredOutcomeDeliveries += int64(len(outcome))
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
		if !validMakerInventoryRebalanceOutcome(decision, got.order) {
			result.OutcomeFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, got.order.OrderID, "request_fields_mismatch")
		}
		if !got.accepted {
			result.Rejected++
			continue
		}
		result.Accepted++
		if got.order.OrderID == 0 {
			result.OutcomeFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, 0, "accepted_without_order_id")
			continue
		}
		orderKey := makerInventoryRebalanceOrderKey{venueID: key.venueID, orderID: got.order.OrderID}
		accepted[orderKey] = decision
		orders[orderKey] = got.order
	}

	fills := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceFill)
	cancels := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceCancellation)
	trades := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceTrade)
	fillEvidence := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceFillEvidence)
	counterpartyOrders := make(map[makerInventoryRebalanceOrderKey]struct{})
	err = r.Scan(ScanOptions{Events: []string{"maker_inventory_rebalance_fill", "OrderFill", "OrderCancelled", "Trade"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "OrderFill":
			var fill makerInventoryRebalanceFill
			if event.Decode(&fill) == nil && fill.OrderID != 0 && fill.Qty > 0 {
				key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: fill.OrderID}
				if _, relevant := accepted[key]; relevant {
					fill.Timestamp = event.SimTS
					fills[key] = append(fills[key], fill)
				}
			}
		case "OrderCancelled":
			var cancel makerInventoryRebalanceCancellation
			if event.Decode(&cancel) == nil && cancel.OrderID != 0 {
				key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: cancel.OrderID}
				if _, relevant := accepted[key]; relevant {
					cancels[key] = append(cancels[key], cancel)
				}
			}
		case "Trade":
			var trade makerInventoryRebalanceTrade
			if event.Decode(&trade) != nil || trade.Qty <= 0 {
				return
			}
			for _, ownOrderID := range []uint64{trade.TakerOrderID, trade.MakerOrderID} {
				if ownOrderID == 0 {
					continue
				}
				ownKey := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: ownOrderID}
				if _, relevant := accepted[ownKey]; !relevant {
					continue
				}
				trades[ownKey] = append(trades[ownKey], trade)
				otherOrderID := trade.MakerOrderID
				if ownOrderID == trade.MakerOrderID {
					otherOrderID = trade.TakerOrderID
				}
				if otherOrderID != 0 && otherOrderID != ownOrderID {
					counterpartyOrders[makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: otherOrderID}] = struct{}{}
				}
			}
		case "maker_inventory_rebalance_fill":
			var evidence makerInventoryRebalanceFillEvidence
			if event.Decode(&evidence) != nil || evidence.OrderID == 0 || evidence.ClientID != event.ClientID || evidence.VenueID != event.VenueID {
				result.UnexpectedFillEvidence++
				addCheck(event.VenueID, event.ClientID, 0, evidence.OrderID, "invalid_fill_evidence")
				return
			}
			key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: evidence.OrderID}
			if _, relevant := accepted[key]; relevant {
				fillEvidence[key] = append(fillEvidence[key], evidence)
				return
			}
			result.UnexpectedFillEvidence++
			addCheck(event.VenueID, event.ClientID, 0, evidence.OrderID, "fill_evidence_without_accepted_p2_order")
		}
	})
	if err != nil {
		return nil, err
	}

	err = r.Scan(ScanOptions{Events: []string{"OrderAccepted"}, Workers: 1}, func(event Event) {
		order, ok := decodeOrder(event)
		if !ok || order.OrderID == 0 {
			return
		}
		key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: order.OrderID}
		if _, relevant := counterpartyOrders[key]; relevant {
			orders[key] = order
		}
	})
	if err != nil {
		return nil, err
	}

	matchedFillEvidence := make(map[makerInventoryRebalanceOrderKey]map[int]bool)
	for key, decision := range accepted {
		order := orders[key]
		orderFills := fills[key]
		orderCancels := cancels[key]
		var filled int64
		for _, fill := range orderFills {
			filled += fill.Qty
			result.Fills++
			result.FilledQty += fill.Qty
			if fill.Role != "taker" {
				result.NonTakerFills++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "rebalance_fill_not_taker")
			}
			if fill.FeeAmount <= 0 || fill.FeeAsset != "USD" {
				result.NonPositiveFees++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "non_positive_or_wrong_asset_fee")
			}
			if want, ok := makerInventoryRebalanceFee(fill.Qty, fill.Price, decision.TakerFeeBps); !ok || want != fill.FeeAmount {
				result.FeeMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "fee_formula_mismatch")
			}
			if !makerInventoryRebalanceHasExternalCounterparty(trades[key], orders, key, fill) {
				result.UnknownCounterparties++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_external_counterparty")
			} else if makerInventoryRebalanceHasSelfCounterparty(trades[key], orders, key, fill, order.ClientID) {
				result.SelfFills++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "self_fill")
			}
			matched := false
			for index, evidence := range fillEvidence[key] {
				if matchedFillEvidence[key] != nil && matchedFillEvidence[key][index] {
					continue
				}
				if evidence.ClientID == order.ClientID && evidence.Symbol == fill.Symbol && evidence.Side == fill.Side && evidence.Qty == fill.Qty && evidence.Price == fill.Price && evidence.FeeAmount == fill.FeeAmount && evidence.FeeAsset == fill.FeeAsset && evidence.TradeID == fill.TradeID && evidence.Timestamp == fill.Timestamp {
					if matchedFillEvidence[key] == nil {
						matchedFillEvidence[key] = make(map[int]bool)
					}
					matchedFillEvidence[key][index] = true
					matched = true
					if !makerInventoryRebalanceReduces(evidence) {
						result.NonReducingFills++
						addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "fill_does_not_reduce_actor_inventory")
					}
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
			if !matchedFillEvidence[key][index] {
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
			if len(orderCancels) == 0 {
				result.MissingIOCTerminals++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_ioc_remainder_cancellation")
			} else if len(orderCancels) != 1 || orderCancels[0].RemainingQty != order.Qty-filled {
				result.DuplicateIOCTerminals++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "invalid_ioc_remainder_cancellation")
			} else {
				result.CancelledIOC++
			}
		} else if len(orderCancels) != 0 {
			result.DuplicateIOCTerminals++
			addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "full_ioc_has_cancellation")
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
	result.Valid = result.Decisions > 0 && result.ReceiptEvidenceErrors == 0 && result.MissingReceipts == 0 && result.AmbiguousReceipts == 0 && result.ReceiptMismatches == 0 && result.FutureReceiptUse == 0 &&
		result.InvalidDecisionRecords == 0 && result.DecisionFieldMismatches == 0 && result.DisabledSubmissions == 0 && result.DuplicateDecisions == 0 && result.MissingOutcomes == 0 && result.DuplicateOutcomes == 0 && result.CensoredOutcomeDeliveries == 0 && result.OutcomeFieldMismatches == 0 &&
		result.MissingIOCTerminals == 0 && result.DuplicateIOCTerminals == 0 && result.FillQuantityMismatches == 0 && result.MissingFillEvidence == 0 && result.UnexpectedFillEvidence == 0 && result.FillEvidenceMismatches == 0 && result.NonReducingFills == 0 && result.UnknownCounterparties == 0 && result.SelfFills == 0 && result.NonTakerFills == 0 && result.NonPositiveFees == 0 && result.FeeMismatches == 0
	return result, nil
}

func (r *Run) measureMakerInventoryRebalanceBuffered() (*MakerInventoryRebalanceAudit, error) {
	result := &MakerInventoryRebalanceAudit{ActionCounts: make(map[string]int64)}
	receipts, receiptAudit, receiptErr := makerInventoryRebalanceReceipts(r.Dir)
	if receiptErr != nil {
		result.ReceiptEvidenceErrors++
	} else {
		result.ReceiptAuditValid = receiptAudit.Valid
		if !receiptAudit.Valid {
			result.ReceiptEvidenceErrors++
		}
	}

	expected := make(map[makerInventoryRebalanceKey]makerInventoryRebalanceDecision)
	outcomes := make(map[makerInventoryRebalanceKey][]makerInventoryRebalanceOutcome)
	orders := make(map[makerInventoryRebalanceOrderKey]makerInventoryRebalanceOrder)
	fills := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceFill)
	cancels := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceCancellation)
	trades := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceTrade)
	fillEvidence := make(map[makerInventoryRebalanceOrderKey][]makerInventoryRebalanceFillEvidence)
	addCheck := func(venue string, client, request, order uint64, failure string) {
		result.Checks = append(result.Checks, MakerInventoryRebalanceCheck{VenueID: venue, ClientID: client, RequestID: request, OrderID: order, Failure: failure})
	}

	err := r.Scan(ScanOptions{Events: []string{
		"maker_inventory_rebalance_decision", "maker_inventory_rebalance_fill", "OrderAccepted", "OrderRejected", "OrderFill", "OrderCancelled", "Trade",
	}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "maker_inventory_rebalance_decision":
			var decision makerInventoryRebalanceDecision
			if event.Decode(&decision) != nil || decision.ClientID == 0 || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID ||
				decision.Symbol != "CDF/USD" || decision.Maker == "" || r.Role(event.VenueID, event.ClientID) != "cdf_spot_maker" {
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
			if makerInventoryRebalanceUsesLocalBook(decision.Action) {
				makerInventoryRebalanceCheckReceipt(result, receipts, receiptErr, decision, addCheck)
			}
			if !validMakerInventoryRebalanceDecision(decision) {
				result.DecisionFieldMismatches++
				addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "decision_policy_mismatch")
			}
			if !decision.Enabled {
				if decision.Action != "POLICY_DISABLED" || decision.RequestID != 0 || decision.RequestedQty != 0 {
					result.DisabledSubmissions++
					addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "disabled_policy_submitted")
				}
				return
			}
			if decision.Action != "SUBMIT_IOC" {
				result.Deferred++
				if decision.RequestID != 0 || decision.RequestedQty != 0 {
					result.DecisionFieldMismatches++
					addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "deferred_policy_has_request")
				}
				return
			}
			result.Submitted++
			if decision.RequestID == 0 {
				result.InvalidDecisionRecords++
				addCheck(event.VenueID, event.ClientID, 0, 0, "submission_without_request_id")
				return
			}
			key := makerInventoryRebalanceKey{venueID: event.VenueID, clientID: event.ClientID, request: decision.RequestID}
			if _, duplicate := expected[key]; duplicate {
				result.DuplicateDecisions++
				addCheck(event.VenueID, event.ClientID, decision.RequestID, 0, "duplicate_submission_decision")
				return
			}
			expected[key] = decision
		case "OrderAccepted", "OrderRejected":
			var order makerInventoryRebalanceOrder
			if event.Decode(&order) != nil || order.RequestID == 0 {
				return
			}
			if order.ClientID == 0 {
				order.ClientID = event.ClientID
			}
			if order.Symbol == "" {
				order.Symbol = event.Symbol
			}
			// Spot order-acceptance payloads intentionally omit the redundant
			// symbol because each persisted book file is instrument-specific.
			// P2 knows only CDF/USD, whose declared filename identity is
			// CDF-USD.jsonl. Recover that exact book identity rather than treating
			// a missing payload field as a different or fabricated instrument. Do
			// not generalize this into a scanner-wide fallback: other analyzers may
			// scan general logs, whose filename is not a market symbol.
			if order.Symbol == "" && pathHasSymbol(event.File, event.VenueID, "CDF-USD") {
				order.Symbol = "CDF/USD"
			}
			key := makerInventoryRebalanceKey{venueID: event.VenueID, clientID: event.ClientID, request: order.RequestID}
			outcomes[key] = append(outcomes[key], makerInventoryRebalanceOutcome{accepted: event.Name == "OrderAccepted", order: order})
			if event.Name == "OrderAccepted" && order.OrderID != 0 {
				orders[makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: order.OrderID}] = order
			}
		case "OrderFill":
			var fill makerInventoryRebalanceFill
			if event.Decode(&fill) == nil && fill.OrderID != 0 && fill.Qty > 0 {
				fill.Timestamp = event.SimTS
				fills[makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: fill.OrderID}] = append(fills[makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: fill.OrderID}], fill)
			}
		case "OrderCancelled":
			var cancel makerInventoryRebalanceCancellation
			if event.Decode(&cancel) == nil && cancel.OrderID != 0 {
				cancels[makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: cancel.OrderID}] = append(cancels[makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: cancel.OrderID}], cancel)
			}
		case "Trade":
			var trade makerInventoryRebalanceTrade
			if event.Decode(&trade) == nil && trade.Qty > 0 {
				if trade.TakerOrderID != 0 {
					key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: trade.TakerOrderID}
					trades[key] = append(trades[key], trade)
				}
				if trade.MakerOrderID != 0 {
					key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: trade.MakerOrderID}
					trades[key] = append(trades[key], trade)
				}
			}
		case "maker_inventory_rebalance_fill":
			var evidence makerInventoryRebalanceFillEvidence
			if event.Decode(&evidence) != nil || evidence.OrderID == 0 || evidence.ClientID != event.ClientID || evidence.VenueID != event.VenueID {
				result.UnexpectedFillEvidence++
				addCheck(event.VenueID, event.ClientID, 0, evidence.OrderID, "invalid_fill_evidence")
				return
			}
			key := makerInventoryRebalanceOrderKey{venueID: event.VenueID, orderID: evidence.OrderID}
			fillEvidence[key] = append(fillEvidence[key], evidence)
		}
	})
	if err != nil {
		return nil, err
	}

	accepted := make(map[makerInventoryRebalanceOrderKey]makerInventoryRebalanceDecision)
	for key, decision := range expected {
		outcome := outcomes[key]
		censored, validCensor := validMakerInventoryRebalanceCensor(decision)
		if !validCensor {
			result.DecisionFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, 0, "invalid_outcome_expectation")
		}
		if censored {
			if len(outcome) == 0 {
				result.HorizonCensored++
				continue
			}
			result.CensoredOutcomeDeliveries += int64(len(outcome))
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
		if !validMakerInventoryRebalanceOutcome(decision, got.order) {
			result.OutcomeFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, got.order.OrderID, "request_fields_mismatch")
		}
		if !got.accepted {
			result.Rejected++
			continue
		}
		result.Accepted++
		if got.order.OrderID == 0 {
			result.OutcomeFieldMismatches++
			addCheck(key.venueID, key.clientID, key.request, 0, "accepted_without_order_id")
			continue
		}
		accepted[makerInventoryRebalanceOrderKey{venueID: key.venueID, orderID: got.order.OrderID}] = decision
	}

	matchedFillEvidence := make(map[makerInventoryRebalanceOrderKey]map[int]bool)
	for key, decision := range accepted {
		order := orders[key]
		orderFills := fills[key]
		orderCancels := cancels[key]
		var filled int64
		for _, fill := range orderFills {
			filled += fill.Qty
			result.Fills++
			result.FilledQty += fill.Qty
			if fill.Role != "taker" {
				result.NonTakerFills++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "rebalance_fill_not_taker")
			}
			if fill.FeeAmount <= 0 || fill.FeeAsset != "USD" {
				result.NonPositiveFees++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "non_positive_or_wrong_asset_fee")
			}
			if want, ok := makerInventoryRebalanceFee(fill.Qty, fill.Price, decision.TakerFeeBps); !ok || want != fill.FeeAmount {
				result.FeeMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "fee_formula_mismatch")
			}
			if !makerInventoryRebalanceHasExternalCounterparty(trades[key], orders, key, fill) {
				result.UnknownCounterparties++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_external_counterparty")
			} else if makerInventoryRebalanceHasSelfCounterparty(trades[key], orders, key, fill, order.ClientID) {
				result.SelfFills++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "self_fill")
			}
			matched := false
			for index, evidence := range fillEvidence[key] {
				if matchedFillEvidence[key] != nil && matchedFillEvidence[key][index] {
					continue
				}
				if evidence.ClientID == order.ClientID && evidence.Symbol == fill.Symbol && evidence.Side == fill.Side && evidence.Qty == fill.Qty && evidence.Price == fill.Price && evidence.FeeAmount == fill.FeeAmount && evidence.FeeAsset == fill.FeeAsset && evidence.TradeID == fill.TradeID && evidence.Timestamp == fill.Timestamp {
					if matchedFillEvidence[key] == nil {
						matchedFillEvidence[key] = make(map[int]bool)
					}
					matchedFillEvidence[key][index] = true
					matched = true
					if !makerInventoryRebalanceReduces(evidence) {
						result.NonReducingFills++
						addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "fill_does_not_reduce_actor_inventory")
					}
					break
				}
			}
			if !matched {
				result.MissingFillEvidence++
				result.FillEvidenceMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_exact_fill_evidence")
			}
		}
		for index, evidence := range fillEvidence[key] {
			if !matchedFillEvidence[key][index] {
				result.UnexpectedFillEvidence++
				result.FillEvidenceMismatches++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "unexpected_fill_evidence")
				_ = evidence
			}
		}
		if filled > order.Qty {
			result.FillQuantityMismatches++
			addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "filled_quantity_exceeds_request")
		}
		if filled < order.Qty {
			if len(orderCancels) == 0 {
				result.MissingIOCTerminals++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "missing_ioc_remainder_cancellation")
			} else if len(orderCancels) != 1 || orderCancels[0].RemainingQty != order.Qty-filled {
				result.DuplicateIOCTerminals++
				addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "invalid_ioc_remainder_cancellation")
			} else {
				result.CancelledIOC++
			}
		} else if len(orderCancels) != 0 {
			result.DuplicateIOCTerminals++
			addCheck(key.venueID, order.ClientID, order.RequestID, key.orderID, "full_ioc_has_cancellation")
		}
	}
	for key, evidenceRows := range fillEvidence {
		if _, known := accepted[key]; known {
			continue
		}
		for range evidenceRows {
			result.UnexpectedFillEvidence++
			addCheck(key.venueID, 0, 0, key.orderID, "fill_evidence_without_accepted_p2_order")
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
	result.Valid = result.Decisions > 0 && result.ReceiptEvidenceErrors == 0 && result.MissingReceipts == 0 && result.AmbiguousReceipts == 0 && result.ReceiptMismatches == 0 && result.FutureReceiptUse == 0 &&
		result.InvalidDecisionRecords == 0 && result.DecisionFieldMismatches == 0 && result.DisabledSubmissions == 0 && result.DuplicateDecisions == 0 && result.MissingOutcomes == 0 && result.DuplicateOutcomes == 0 && result.CensoredOutcomeDeliveries == 0 && result.OutcomeFieldMismatches == 0 &&
		result.MissingIOCTerminals == 0 && result.DuplicateIOCTerminals == 0 && result.FillQuantityMismatches == 0 && result.MissingFillEvidence == 0 && result.UnexpectedFillEvidence == 0 && result.FillEvidenceMismatches == 0 && result.NonReducingFills == 0 && result.UnknownCounterparties == 0 && result.SelfFills == 0 && result.NonTakerFills == 0 && result.NonPositiveFees == 0 && result.FeeMismatches == 0
	return result, nil
}

func makerInventoryRebalanceCheckReceipt(result *MakerInventoryRebalanceAudit, receipts map[makerInventoryRebalanceReceiptKey][]makerInventoryRebalanceReceipt, receiptErr error, decision makerInventoryRebalanceDecision, add func(string, uint64, uint64, uint64, string)) {
	if receiptErr != nil {
		result.MissingReceipts++
		add(decision.VenueID, decision.ClientID, decision.RequestID, 0, "market_data_receipt_evidence_unavailable")
		return
	}
	if decision.LastBookSourceTime == 0 || decision.LastBookSequence == 0 || decision.LastBookReceivedTime == 0 {
		result.MissingReceipts++
		add(decision.VenueID, decision.ClientID, decision.RequestID, 0, "decision_missing_local_book_receipt_identity")
		return
	}
	key := makerInventoryRebalanceReceiptKey{venueID: decision.VenueID, clientID: decision.ClientID, symbol: decision.Symbol, sequence: decision.LastBookSequence, published: decision.LastBookSourceTime}
	matches := receipts[key]
	if len(matches) == 0 {
		result.MissingReceipts++
		add(decision.VenueID, decision.ClientID, decision.RequestID, 0, "missing_matching_snapshot_receipt")
		return
	}
	if len(matches) != 1 {
		result.AmbiguousReceipts++
		add(decision.VenueID, decision.ClientID, decision.RequestID, 0, "ambiguous_matching_snapshot_receipt")
		return
	}
	if matches[0].deliveredAt != decision.LastBookReceivedTime {
		result.ReceiptMismatches++
		add(decision.VenueID, decision.ClientID, decision.RequestID, 0, "snapshot_receipt_delivery_mismatch")
	}
	if decision.LastBookReceivedTime > decision.DecisionTime {
		result.FutureReceiptUse++
		add(decision.VenueID, decision.ClientID, decision.RequestID, 0, "future_snapshot_used_by_decision")
	}
	result.ReceiptMatches++
}

// makerInventoryRebalanceUsesLocalBook is deliberately narrower than
// "the row happens to contain book fields". Disabled, unsubscribed, pending,
// cooldown, and in-band evaluations do not consult the snapshot and therefore
// cannot require a receipt to prove their decision. Any action depending on
// source time, touch depth, or touch price must have an exact delivered receipt.
func makerInventoryRebalanceUsesLocalBook(action string) bool {
	switch action {
	case "LOCAL_BOOK_SOURCE_FUTURE", "LOCAL_BOOK_STALE", "LOCAL_CONTRA_TOUCH_UNAVAILABLE", "INVALID_OUTWARD_LIMIT", "REQUEST_QUANTITY_UNAVAILABLE", "COOLDOWN_OVERFLOW", "SUBMIT_IOC":
		return true
	default:
		return false
	}
}

func makerInventoryRebalanceReceipts(dir string) (map[makerInventoryRebalanceReceiptKey][]makerInventoryRebalanceReceipt, *MarketDataReceiptAudit, error) {
	audit, err := AuditMarketDataReceipts(dir)
	if err != nil {
		return nil, nil, err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		return nil, audit, err
	}
	var manifest marketDataEvidenceManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, audit, err
	}
	raw, _, err := readEvidenceFile(dir, manifest.Receipts.File, marketDataReceiptRecordBytes, manifest.Receipts.Records, manifest.Receipts.Digest)
	if err != nil {
		return nil, audit, err
	}
	links := make(map[uint32]struct{ venue, role string }, len(manifest.Links))
	for _, link := range manifest.Links {
		links[link.ID] = struct{ venue, role string }{link.SourceVenue, link.Role}
	}
	symbols := make(map[uint32]string, len(manifest.Symbols))
	for _, symbol := range manifest.Symbols {
		symbols[symbol.ID] = symbol.Symbol
	}
	index := make(map[makerInventoryRebalanceReceiptKey][]makerInventoryRebalanceReceipt)
	for offset := 0; offset < len(raw); offset += marketDataReceiptRecordBytes {
		record := decodeObservation(raw[offset : offset+marketDataReceiptRecordBytes])
		link, ok := links[record.linkID]
		if !ok || link.role != "cdf_spot_maker" || record.mdType != 0 || symbols[record.symbolID] != "CDF/USD" {
			continue
		}
		key := makerInventoryRebalanceReceiptKey{venueID: link.venue, clientID: record.clientID, symbol: "CDF/USD", sequence: record.sequence, published: record.publishedAt}
		index[key] = append(index[key], makerInventoryRebalanceReceipt{deliveredAt: record.deliveredAt})
	}
	return index, audit, nil
}

func validMakerInventoryRebalanceDecision(d makerInventoryRebalanceDecision) bool {
	// P2 is a fixed, preregistered screen rather than a generic policy metric.
	// Requiring every declared constant here prevents a run from re-labelling a
	// different policy as P2 after the fact.
	if d.RiskBandQty != 10_000_000_000 || d.TargetBandQty != 5_000_000_000 || d.MaxRequestQty != 500_000_000 || d.ParticipationBps != 1_000 || d.SlippageBps != 50 || d.EvaluationInterval != 10_000_000_000 || d.Cooldown != 30_000_000_000 || d.TakerFeeBps != 5 {
		return false
	}
	if !d.Enabled {
		return d.Action == "POLICY_DISABLED" && d.RequestID == 0 && d.RequestedQty == 0 && !d.RequestPending && d.CooldownUntil == 0
	}
	if !d.Subscribed {
		return d.Action == "NOT_SUBSCRIBED" && d.RequestID == 0 && d.RequestedQty == 0 && !d.RequestPending && d.CooldownUntil == 0
	}
	if d.RequestPending {
		return d.Action == "REQUEST_PENDING" && d.RequestID == 0 && d.RequestedQty == 0 && d.CooldownUntil == 0
	}
	if d.Action == "COOLDOWN" {
		return d.CooldownUntil > d.DecisionTime && d.RequestID == 0 && d.RequestedQty == 0
	}
	if d.CooldownUntil != 0 && d.Action != "SUBMIT_IOC" {
		return false
	}
	magnitude := new(big.Int).Abs(big.NewInt(d.Inventory))
	if magnitude.Cmp(big.NewInt(d.RiskBandQty)) < 0 {
		return d.Action == "IN_BAND" && d.RequestID == 0 && d.RequestedQty == 0
	}
	if d.LastBookSourceTime == 0 {
		return d.Action == "LOCAL_BOOK_UNAVAILABLE" && d.RequestID == 0 && d.RequestedQty == 0
	}
	age := new(big.Int).Sub(big.NewInt(d.DecisionTime), big.NewInt(d.LastBookSourceTime))
	if age.Sign() < 0 {
		return d.Action == "LOCAL_BOOK_SOURCE_FUTURE" && d.RequestID == 0 && d.RequestedQty == 0
	}
	if age.Cmp(big.NewInt(d.EvaluationInterval)) > 0 {
		return d.Action == "LOCAL_BOOK_STALE" && d.RequestID == 0 && d.RequestedQty == 0
	}
	if magnitude.Cmp(big.NewInt(d.TargetBandQty)) <= 0 {
		return d.Action == "IN_BAND" && d.RequestID == 0 && d.RequestedQty == 0
	}
	desired := new(big.Int).Sub(magnitude, big.NewInt(d.TargetBandQty))
	if !desired.IsInt64() || desired.Int64() != d.DesiredReduction || d.DesiredReduction <= 0 || d.LastBookReceivedTime == 0 || d.LastBookSequence == 0 {
		return false
	}
	touch, visible := d.BidPrice, d.BidVisibleQty
	wantSide := "SELL"
	if d.Inventory < 0 {
		touch, visible, wantSide = d.AskPrice, d.AskVisibleQty, "BUY"
	}
	if d.Side != wantSide {
		return false
	}
	cap := new(big.Int).Mul(big.NewInt(visible), big.NewInt(d.ParticipationBps))
	cap.Quo(cap, big.NewInt(10_000))
	if !cap.IsInt64() || cap.Int64() != d.ParticipationCap {
		return false
	}
	wantPrice, ok := makerInventoryRebalanceLimit(touch, d.SlippageBps, 100_000, d.Side)
	if (!ok && d.LimitPrice != 0) || (ok && wantPrice != d.LimitPrice) {
		return false
	}
	if d.ParticipationCap <= 0 {
		return d.Action == "LOCAL_CONTRA_TOUCH_UNAVAILABLE" && d.RequestID == 0 && d.RequestedQty == 0
	}
	if !ok || d.LimitPrice <= 0 {
		return d.Action == "INVALID_OUTWARD_LIMIT" && d.RequestID == 0 && d.RequestedQty == 0
	}
	wantQty := big.NewInt(d.DesiredReduction)
	for _, cap := range []int64{d.MaxRequestQty, d.ParticipationCap} {
		if wantQty.Cmp(big.NewInt(cap)) > 0 {
			wantQty.SetInt64(cap)
		}
	}
	if !wantQty.IsInt64() {
		return false
	}
	if wantQty.Sign() <= 0 {
		return d.Action == "REQUEST_QUANTITY_UNAVAILABLE" && d.RequestID == 0 && d.RequestedQty == 0
	}
	if d.RequestedQty != wantQty.Int64() {
		return false
	}
	wantCooldown := new(big.Int).Add(big.NewInt(d.DecisionTime), big.NewInt(d.Cooldown))
	if !wantCooldown.IsInt64() {
		return d.Action == "COOLDOWN_OVERFLOW" && d.RequestID == 0 && d.RequestedQty == 0
	}
	return d.Action == "SUBMIT_IOC" && d.RequestID != 0 && d.CooldownUntil == wantCooldown.Int64()
}

func makerInventoryRebalanceLimit(touch, bps, tick int64, side string) (int64, bool) {
	if touch <= 0 || bps < 0 || tick <= 0 {
		return 0, false
	}
	value := new(big.Int).Mul(big.NewInt(touch), big.NewInt(bps))
	value.Quo(value, big.NewInt(10_000))
	if !value.IsInt64() {
		return 0, false
	}
	if side == "BUY" {
		value.Add(value, big.NewInt(touch))
	} else if side == "SELL" {
		value.Sub(big.NewInt(touch), value)
	} else {
		return 0, false
	}
	if !value.IsInt64() || value.Sign() <= 0 {
		return 0, false
	}
	price := value
	rem := new(big.Int).Rem(new(big.Int).Set(price), big.NewInt(tick))
	if rem.Sign() == 0 {
		return price.Int64(), true
	}
	if side == "BUY" {
		price.Add(price, new(big.Int).Sub(big.NewInt(tick), rem))
	} else {
		price.Sub(price, rem)
	}
	if !price.IsInt64() || price.Sign() <= 0 {
		return 0, false
	}
	return price.Int64(), true
}

func validMakerInventoryRebalanceCensor(d makerInventoryRebalanceDecision) (bool, bool) {
	switch d.OutcomeExpectation {
	case "VENUE_OUTCOME_REQUIRED":
		return false, d.CensorReason == ""
	case "SIMULATION_HORIZON_CENSORED":
		return true, d.CensorReason == "terminal_horizon_before_venue_ingress"
	default:
		return false, false
	}
}

func validMakerInventoryRebalanceOutcome(d makerInventoryRebalanceDecision, order makerInventoryRebalanceOrder) bool {
	return order.Symbol == d.Symbol && order.Side == d.Side && order.Type == "LIMIT" && order.TimeInForce == "IOC" && !order.PostOnly && order.Price == d.LimitPrice && order.Qty == d.RequestedQty
}

func makerInventoryRebalanceFee(qty, price, bps int64) (int64, bool) {
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

func makerInventoryRebalanceHasExternalCounterparty(trades []makerInventoryRebalanceTrade, orders map[makerInventoryRebalanceOrderKey]makerInventoryRebalanceOrder, own makerInventoryRebalanceOrderKey, fill makerInventoryRebalanceFill) bool {
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
		if _, ok := orders[makerInventoryRebalanceOrderKey{venueID: own.venueID, orderID: other}]; ok {
			return true
		}
	}
	return false
}

func makerInventoryRebalanceHasSelfCounterparty(trades []makerInventoryRebalanceTrade, orders map[makerInventoryRebalanceOrderKey]makerInventoryRebalanceOrder, own makerInventoryRebalanceOrderKey, fill makerInventoryRebalanceFill, clientID uint64) bool {
	for _, trade := range trades {
		if trade.TradeID != fill.TradeID || trade.Qty != fill.Qty || trade.Price != fill.Price {
			continue
		}
		other := trade.MakerOrderID
		if other == own.orderID {
			other = trade.TakerOrderID
		}
		if order, ok := orders[makerInventoryRebalanceOrderKey{venueID: own.venueID, orderID: other}]; ok && order.ClientID == clientID {
			return true
		}
	}
	return false
}

func makerInventoryRebalanceReduces(fill makerInventoryRebalanceFillEvidence) bool {
	if fill.Qty <= 0 || (fill.Side != "BUY" && fill.Side != "SELL") {
		return false
	}
	want := new(big.Int).SetInt64(fill.PreInventory)
	if fill.Side == "BUY" {
		want.Add(want, big.NewInt(fill.Qty))
	} else {
		want.Sub(want, big.NewInt(fill.Qty))
	}
	if !want.IsInt64() || want.Int64() != fill.PostInventory {
		return false
	}
	before := new(big.Int).Abs(big.NewInt(fill.PreInventory))
	after := new(big.Int).Abs(big.NewInt(fill.PostInventory))
	return after.Cmp(before) < 0
}
