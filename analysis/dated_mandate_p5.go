package analysis

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"

	"exchange_sim/exchange"
	etypes "exchange_sim/types"
)

const p5DatedMandatePolicyVersion = "v2_5_p5_dated_execution_mandate_v1"

// DatedMandateP5Audit validates the finite long-future execution objective
// independently of the carry participant. Activity is not a basis claim: this
// audit establishes only a real parent, delayed local books, and ordinary IOC
// children whose fills reduce the finite parent one-for-one.
type DatedMandateP5Audit struct {
	Decisions       int64            `json:"decisions"`
	Submitted       int64            `json:"submitted"`
	Accepted        int64            `json:"accepted"`
	Rejected        int64            `json:"rejected"`
	Fills           int64            `json:"fills"`
	FilledQty       int64            `json:"filled_qty"`
	CompletedParent int64            `json:"completed_parents"`
	ActionCounts    map[string]int64 `json:"action_counts"`

	ReceiptAuditValid  bool                `json:"receipt_audit_valid"`
	RoleLinksActive    bool                `json:"role_links_active"`
	ListingMatches     int64               `json:"listing_matches"`
	BookMatches        int64               `json:"book_matches"`
	SourceErrors       int64               `json:"source_errors"`
	FrontierErrors     int64               `json:"frontier_errors"`
	PolicyErrors       int64               `json:"policy_errors"`
	ParentErrors       int64               `json:"parent_errors"`
	MissingGateway     int64               `json:"missing_gateway_decisions"`
	GatewayErrors      int64               `json:"gateway_errors"`
	VenueOutcomeErrors int64               `json:"venue_outcome_errors"`
	ActorOutcomeErrors int64               `json:"actor_outcome_errors"`
	Checks             []DatedCarryP5Check `json:"checks,omitempty"`
	Valid              bool                `json:"valid"`
}

type datedMandateP5Decision struct {
	VenueID       string `json:"venue_id"`
	Desk          string `json:"desk"`
	ClientID      uint64 `json:"client_id"`
	PolicyVersion string `json:"policy_version"`
	DecisionTime  int64  `json:"decision_time"`
	Action        string `json:"action_or_defer_reason"`
	Enabled       bool   `json:"enabled"`
	Subscribed    bool   `json:"subscribed"`

	Symbol             string `json:"symbol"`
	Underlying         string `json:"underlying"`
	Side               string `json:"side"`
	ListedNano         int64  `json:"listed_nano"`
	ExpiryNano         int64  `json:"expiry_nano"`
	OriginalTenorNanos int64  `json:"original_tenor_nanos"`
	ListingPublishedAt int64  `json:"listing_published_at"`
	ListingSequence    uint64 `json:"listing_sequence"`
	ListingFingerprint string `json:"listing_fingerprint"`
	ParentQty          int64  `json:"parent_qty"`
	FilledQty          int64  `json:"filled_qty"`
	RemainingQty       int64  `json:"remaining_qty"`
	StartAt            int64  `json:"start_at"`
	EndAt              int64  `json:"end_at"`
	HasBook            bool   `json:"has_book"`
	BookPublishedAt    int64  `json:"book_published_at"`
	BookSequence       uint64 `json:"book_sequence"`
	HasBid             bool   `json:"has_bid"`
	Bid                int64  `json:"bid"`
	BidQty             int64  `json:"bid_qty"`
	HasAsk             bool   `json:"has_ask"`
	Ask                int64  `json:"ask"`
	AskQty             int64  `json:"ask_qty"`
	MarketAgeNanos     int64  `json:"market_age_nanos"`
	LimitPrice         int64  `json:"limit_price"`
	RequestedQty       int64  `json:"requested_qty"`
	RequestID          uint64 `json:"request_id"`

	DecisionFrontierLinkID      uint32 `json:"decision_frontier_link_id"`
	DecisionFrontierOrdinal     uint64 `json:"decision_frontier_ordinal"`
	DecisionFrontierDeliveredAt int64  `json:"decision_frontier_delivered_at"`
	DecisionFrontierDigest      string `json:"decision_frontier_digest"`
}

type datedMandateP5Outcome struct {
	VenueID         string `json:"venue_id"`
	Desk            string `json:"desk"`
	ClientID        uint64 `json:"client_id"`
	DecisionTime    int64  `json:"decision_time"`
	ExecutionTime   int64  `json:"execution_time"`
	Event           string `json:"event"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	RequestID       uint64 `json:"request_id"`
	OrderID         uint64 `json:"order_id"`
	TradeID         uint64 `json:"trade_id"`
	Qty             int64  `json:"qty"`
	Price           int64  `json:"price"`
	FeeAmount       int64  `json:"fee_amount"`
	FeeAsset        string `json:"fee_asset"`
	RemainingQty    int64  `json:"remaining_qty"`
	RejectReason    string `json:"reject_reason"`
	HasSettlement   bool   `json:"has_settlement"`
	SettlementPrice int64  `json:"settlement_price"`
}

type p5MandateCanonicalFill struct {
	at            int64
	venue, symbol string
	client        uint64
	fill          fundingCarryVenueFill
}

func p5MandateFrontier(decision datedMandateP5Decision, evidence *p5Evidence) (auditedFrontier, error) {
	pseudo := datedCarryP5Decision{
		VenueID: decision.VenueID, ClientID: decision.ClientID, DecisionTime: decision.DecisionTime,
		DecisionFrontierLinkID: decision.DecisionFrontierLinkID, DecisionFrontierOrdinal: decision.DecisionFrontierOrdinal,
		DecisionFrontierDeliveredAt: decision.DecisionFrontierDeliveredAt, DecisionFrontierDigest: decision.DecisionFrontierDigest,
	}
	if decision.DecisionFrontierLinkID == 0 {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_missing")
	}
	if decision.DecisionFrontierOrdinal == 0 {
		if decision.DecisionFrontierDeliveredAt != 0 || decision.DecisionFrontierDigest != "00000000000000000000000000000000" ||
			evidence.linkRoles[decision.DecisionFrontierLinkID] != "dated_execution_mandate" || evidence.linkVenues[decision.DecisionFrontierLinkID] != decision.VenueID {
			return auditedFrontier{}, fmt.Errorf("decision_frontier_mismatch")
		}
		return auditedFrontier{}, nil
	}
	frontier, ok := evidence.frontiers[fundingCarryReceiptKey{pseudo.ClientID, pseudo.DecisionFrontierLinkID, pseudo.DecisionFrontierOrdinal}]
	if !ok || frontier.deliveredAt != pseudo.DecisionFrontierDeliveredAt || hex.EncodeToString(frontier.digest[:]) != pseudo.DecisionFrontierDigest {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_mismatch")
	}
	if frontier.deliveredAt > decision.DecisionTime {
		return auditedFrontier{}, fmt.Errorf("future_receipt")
	}
	if evidence.linkRoles[decision.DecisionFrontierLinkID] != "dated_execution_mandate" || evidence.linkVenues[decision.DecisionFrontierLinkID] != decision.VenueID {
		return auditedFrontier{}, fmt.Errorf("decision_frontier_wrong_link")
	}
	return frontier, nil
}

func p5MandateSource(decision datedMandateP5Decision, evidence *p5Evidence, mdType exchange.MDType, symbol string, sequence uint64, published int64, fingerprint string) error {
	frontier, err := p5MandateFrontier(decision, evidence)
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
	if len(rows) != 1 {
		if len(rows) == 0 {
			return fmt.Errorf("source_missing")
		}
		return fmt.Errorf("source_identity_ambiguous")
	}
	if rows[0].ordinal > frontier.ordinal {
		return fmt.Errorf("source_after_frontier")
	}
	if rows[0].publishedAt > decision.DecisionTime || rows[0].deliveredAt > decision.DecisionTime {
		return fmt.Errorf("future_receipt")
	}
	return nil
}

func p5ValidateMandateListing(decision datedMandateP5Decision, listing p5Listing, found bool) error {
	if !found || listing.announcement.ListedNano == nil {
		return fmt.Errorf("canonical_listing_missing")
	}
	ann := listing.announcement
	if ann.Action != "listed" || ann.Symbol != decision.Symbol || ann.InstrumentType != "FUTURE" || ann.Underlying != decision.Underlying ||
		listing.at != decision.ListedNano || ann.Timestamp != decision.ListedNano || *ann.ListedNano != decision.ListedNano || ann.ExpiryNano != decision.ExpiryNano || decision.ListingPublishedAt < listing.at {
		return fmt.Errorf("canonical_listing_mismatch")
	}
	ann.Timestamp = decision.ListingPublishedAt
	fingerprint, err := etypes.MarketDataFingerprint(&etypes.MarketDataMsg{Type: exchange.MDInstrument, Symbol: etypes.InstrumentFeedSymbol, SeqNum: decision.ListingSequence, Timestamp: decision.ListingPublishedAt, Data: &ann})
	if err != nil || decision.ListingFingerprint != hex.EncodeToString(fingerprint[:]) {
		return fmt.Errorf("canonical_listing_fingerprint_mismatch")
	}
	return nil
}

func p5MandateOutwardLimit(touch, bps, tick int64, side exchange.Side) (int64, bool) {
	if touch <= 0 || bps < 0 || tick <= 0 {
		return 0, false
	}
	concession := new(big.Int).Mul(big.NewInt(touch), big.NewInt(bps))
	concession.Quo(concession, big.NewInt(10_000))
	price := new(big.Int).SetInt64(touch)
	if side == exchange.Buy {
		price.Add(price, concession)
	} else {
		price.Sub(price, concession)
	}
	if !price.IsInt64() || price.Sign() <= 0 {
		return 0, false
	}
	value := price.Int64()
	remainder := value % tick
	if remainder == 0 {
		return value, true
	}
	if side == exchange.Buy {
		rounded := new(big.Int).Add(price, big.NewInt(tick-remainder))
		if !rounded.IsInt64() {
			return 0, false
		}
		return rounded.Int64(), true
	}
	return value - remainder, true
}

func validateP5MandateDecision(policy datedMandateP5Config, decision datedMandateP5Decision) error {
	if decision.PolicyVersion != p5DatedMandatePolicyVersion || decision.Enabled != policy.Enabled || decision.Underlying != policy.Underlying || decision.Side != policy.Side || decision.ParentQty != policy.ParentQty {
		return fmt.Errorf("decision_policy_mismatch")
	}
	if decision.Symbol == "" {
		if decision.RequestID != 0 || decision.Action != "NOT_SUBSCRIBED" && decision.Action != "NO_ELIGIBLE_CONTRACT" {
			return fmt.Errorf("empty_contract_action_mismatch")
		}
		return nil
	}
	if decision.OriginalTenorNanos != policy.TargetTenor || decision.ExpiryNano-decision.ListedNano != policy.TargetTenor || decision.StartAt-decision.ListedNano != policy.StartDelay || decision.EndAt-decision.StartAt != policy.ExecutionDuration {
		return fmt.Errorf("mandate_term_mismatch")
	}
	if decision.FilledQty < 0 || decision.FilledQty > policy.ParentQty || decision.RemainingQty != policy.ParentQty-decision.FilledQty {
		return fmt.Errorf("parent_quantity_mismatch")
	}
	if decision.Action != "SUBMIT_CHILD_IOC" {
		if decision.RequestID != 0 {
			return fmt.Errorf("deferred_action_has_request")
		}
		return nil
	}
	if decision.DecisionTime < decision.StartAt || decision.DecisionTime >= decision.EndAt || !decision.HasBook || decision.BookPublishedAt > decision.DecisionTime || decision.DecisionTime-decision.BookPublishedAt != decision.MarketAgeNanos || decision.MarketAgeNanos > policy.MaxMarketAge {
		return fmt.Errorf("submission_time_or_book_mismatch")
	}
	touch, available := decision.Ask, decision.AskQty
	side := exchange.Buy
	if policy.Side == exchange.Sell.String() {
		touch, available, side = decision.Bid, decision.BidQty, exchange.Sell
	}
	if side == exchange.Buy && !decision.HasAsk || side == exchange.Sell && !decision.HasBid {
		return fmt.Errorf("submission_touch_unavailable")
	}
	limit, ok := p5MandateOutwardLimit(touch, policy.SlippageBps, policy.TickSize, side)
	if !ok || decision.LimitPrice != limit {
		return fmt.Errorf("submission_limit_mismatch")
	}
	wantQty := policy.ChildQty
	if decision.RemainingQty < wantQty {
		wantQty = decision.RemainingQty
	}
	_ = available // The declared mandate deliberately does not cap by touch depth.
	if decision.RequestedQty != wantQty || wantQty <= 0 || decision.RequestID == 0 {
		return fmt.Errorf("submission_quantity_mismatch")
	}
	return nil
}

func p5MandateGatewayMatches(decision datedMandateP5Decision, gateway fundingCarryGatewayDecision) bool {
	return gateway.requestID == decision.RequestID && gateway.symbol == decision.Symbol && gateway.decisionAt == decision.DecisionTime &&
		gateway.price == decision.LimitPrice && gateway.qty == decision.RequestedQty && gateway.side == uint8(exchangeSide(decision.Side)) &&
		gateway.orderType == uint8(exchange.LimitOrder) && gateway.tif == uint8(exchange.IOC)
}

// MeasureDatedMandateP5 validates the independent dated-future demand channel.
func (r *Run) MeasureDatedMandateP5() (*DatedMandateP5Audit, error) {
	manifest, err := loadDatedCarryP5Manifest(r.Dir)
	if err != nil {
		return nil, err
	}
	if err := validateDatedCarryP5Manifest(manifest); err != nil {
		return nil, err
	}
	evidence, evidenceErr := loadP5Evidence(r.Dir)
	result := &DatedMandateP5Audit{ActionCounts: make(map[string]int64)}
	check := func(decision datedMandateP5Decision, failure string) {
		result.Checks = append(result.Checks, DatedCarryP5Check{VenueID: decision.VenueID, ClientID: decision.ClientID, At: decision.DecisionTime, Failure: failure})
	}
	if evidenceErr == nil && evidence != nil && evidence.audit != nil && evidence.audit.Valid {
		result.ReceiptAuditValid = true
		result.RoleLinksActive = p5RequiredRoleLinksActive(evidence)
	}
	var decisions []datedMandateP5Decision
	var outcomes []datedMandateP5Outcome
	listings := make(map[p5ListingKey]p5Listing)
	accepted := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	rejected := make(map[fundingCarryKey][]fundingCarryVenueOrder)
	fills := make(map[fundingCarryOrderKey][]fundingCarryVenueFill)
	var canonicalFills []p5MandateCanonicalFill
	if err := r.Scan(ScanOptions{Events: []string{"dated_execution_mandate_decision", "dated_execution_mandate_outcome", "instrument_listed", "OrderAccepted", "OrderRejected", "OrderFill"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "dated_execution_mandate_decision":
			var decision datedMandateP5Decision
			if event.Decode(&decision) != nil || decision.VenueID != event.VenueID || decision.ClientID != event.ClientID || r.Role(event.VenueID, event.ClientID) != "dated_execution_mandate" {
				result.PolicyErrors++
				return
			}
			decisions = append(decisions, decision)
		case "dated_execution_mandate_outcome":
			var outcome datedMandateP5Outcome
			if event.Decode(&outcome) != nil || outcome.VenueID != event.VenueID || outcome.ClientID != event.ClientID || r.Role(event.VenueID, event.ClientID) != "dated_execution_mandate" {
				result.ActorOutcomeErrors++
				return
			}
			outcomes = append(outcomes, outcome)
		case "instrument_listed":
			var announcement etypes.InstrumentAnnouncement
			if event.Decode(&announcement) == nil && announcement.Symbol != "" {
				listings[p5ListingKey{event.VenueID, announcement.Symbol}] = p5Listing{at: event.SimTS, announcement: announcement}
			}
		case "OrderAccepted", "OrderRejected":
			if r.Role(event.VenueID, event.ClientID) != "dated_execution_mandate" {
				return
			}
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
			if r.Role(event.VenueID, event.ClientID) != "dated_execution_mandate" {
				return
			}
			var fill fundingCarryVenueFill
			if event.Decode(&fill) != nil || fill.OrderID == 0 {
				return
			}
			fill.VenueID, fill.ClientID = event.VenueID, event.ClientID
			fills[fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}] = append(fills[fundingCarryOrderKey{event.VenueID, event.ClientID, fill.OrderID}], fill)
			canonicalFills = append(canonicalFills, p5MandateCanonicalFill{at: event.SimTS, venue: event.VenueID, symbol: fill.Symbol, client: event.ClientID, fill: fill})
		}
	}); err != nil {
		return nil, fmt.Errorf("P5 mandate scan: %w", err)
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
	byRequest := make(map[fundingCarryKey][]datedMandateP5Outcome)
	for _, outcome := range outcomes {
		byRequest[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}] = append(byRequest[fundingCarryKey{outcome.VenueID, outcome.ClientID, outcome.RequestID}], outcome)
	}
	filledBefore := func(decision datedMandateP5Decision) int64 {
		total := int64(0)
		for _, row := range canonicalFills {
			if row.venue == decision.VenueID && row.client == decision.ClientID && row.symbol == decision.Symbol && row.at <= decision.DecisionTime {
				total += row.fill.Qty
			}
		}
		return total
	}
	for _, decision := range decisions {
		result.Decisions++
		result.ActionCounts[decision.Action]++
		if err := validateP5MandateDecision(*manifest.Config.DatedExecutionMandate, decision); err != nil {
			result.PolicyErrors++
			check(decision, err.Error())
		}
		if evidence == nil {
			continue
		}
		if _, err := p5MandateFrontier(decision, evidence); err != nil {
			result.FrontierErrors++
			check(decision, err.Error())
			continue
		}
		if decision.Symbol == "" {
			continue
		}
		if reconstructed := filledBefore(decision); decision.FilledQty != reconstructed || reconstructed < 0 || reconstructed > manifest.Config.DatedExecutionMandate.ParentQty {
			result.ParentErrors++
			check(decision, "parent_fill_reconstruction_mismatch")
		}
		listing, found := listings[p5ListingKey{decision.VenueID, decision.Symbol}]
		if err := p5ValidateMandateListing(decision, listing, found); err != nil {
			result.SourceErrors++
			check(decision, err.Error())
		}
		if err := p5MandateSource(decision, evidence, exchange.MDInstrument, etypes.InstrumentFeedSymbol, decision.ListingSequence, decision.ListingPublishedAt, decision.ListingFingerprint); err != nil {
			result.SourceErrors++
			check(decision, "listing_"+err.Error())
		} else {
			result.ListingMatches++
		}
		if decision.HasBook {
			if err := p5MandateSource(decision, evidence, exchange.MDSnapshot, decision.Symbol, decision.BookSequence, decision.BookPublishedAt, ""); err != nil {
				result.SourceErrors++
				check(decision, "book_"+err.Error())
			} else {
				result.BookMatches++
			}
		}
		if decision.Action != "SUBMIT_CHILD_IOC" {
			continue
		}
		result.Submitted++
		key := fundingCarryKey{decision.VenueID, decision.ClientID, decision.RequestID}
		gateway, found := evidence.gateways[key]
		if !found || evidence.gatewayCount[key] == 0 {
			result.MissingGateway++
			check(decision, "missing_gateway_decision")
		} else if evidence.gatewayCount[key] != 1 || !p5MandateGatewayMatches(decision, gateway) {
			result.GatewayErrors++
			check(decision, "gateway_decision_mismatch")
		}
		acceptRows, rejectRows := accepted[key], rejected[key]
		if len(acceptRows)+len(rejectRows) != 1 {
			result.VenueOutcomeErrors++
			check(decision, "venue_outcome_cardinality")
			continue
		}
		actorRows := byRequest[key]
		if len(rejectRows) == 1 {
			result.Rejected++
			if !p5ActorRejectedMandate(actorRows, rejectRows[0].Error) {
				result.ActorOutcomeErrors++
				check(decision, "actor_rejection_mismatch")
			}
			continue
		}
		order := acceptRows[0]
		result.Accepted++
		if order.Side != decision.Side || order.Type != exchange.LimitOrder.String() || order.TimeInForce != exchange.IOC.String() || order.PostOnly || order.Price != decision.LimitPrice || order.Qty != decision.RequestedQty {
			result.VenueOutcomeErrors++
			check(decision, "accepted_order_mismatch")
		}
		if !p5ActorAcceptedMandate(actorRows, order.OrderID) {
			result.ActorOutcomeErrors++
			check(decision, "actor_acceptance_mismatch")
		}
		for _, fill := range fills[fundingCarryOrderKey{decision.VenueID, decision.ClientID, order.OrderID}] {
			result.Fills++
			result.FilledQty += fill.Qty
			if !p5ActorFillMandate(actorRows, fill) {
				result.ActorOutcomeErrors++
				check(decision, "actor_fill_mismatch")
			}
		}
	}
	completed := make(map[p5ListingKey]struct{})
	for _, decision := range decisions {
		if decision.FilledQty == manifest.Config.DatedExecutionMandate.ParentQty && decision.RemainingQty == 0 {
			completed[p5ListingKey{decision.VenueID, decision.Symbol}] = struct{}{}
		}
	}
	result.CompletedParent = int64(len(completed))
	if result.Decisions == 0 {
		result.PolicyErrors++
		result.Checks = append(result.Checks, DatedCarryP5Check{Failure: "missing_dated_mandate_decisions"})
	}
	result.Valid = result.ReceiptAuditValid && result.RoleLinksActive && result.SourceErrors == 0 && result.FrontierErrors == 0 && result.PolicyErrors == 0 && result.ParentErrors == 0 && result.MissingGateway == 0 && result.GatewayErrors == 0 && result.VenueOutcomeErrors == 0 && result.ActorOutcomeErrors == 0
	return result, nil
}

func p5ActorAcceptedMandate(outcomes []datedMandateP5Outcome, orderID uint64) bool {
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_ACCEPTED" && outcome.OrderID == orderID {
			return true
		}
	}
	return false
}
func p5ActorRejectedMandate(outcomes []datedMandateP5Outcome, reason string) bool {
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_REJECTED" && outcome.RejectReason == reason {
			return true
		}
	}
	return false
}
func p5ActorFillMandate(outcomes []datedMandateP5Outcome, fill fundingCarryVenueFill) bool {
	matches := 0
	for _, outcome := range outcomes {
		if outcome.Event == "ORDER_FILL" && outcome.OrderID == fill.OrderID && outcome.TradeID == fill.TradeID && outcome.Symbol == fill.Symbol && outcome.Side == fill.Side && outcome.Qty == fill.Qty && outcome.Price == fill.Price && outcome.FeeAmount == fill.FeeAmount && outcome.FeeAsset == fill.FeeAsset {
			matches++
		}
	}
	return matches == 1
}
