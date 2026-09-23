package executionpilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"

	"exchange_sim/exchange"
	"exchange_sim/types"
)

type OutcomeStatus string

const (
	OutcomeNoObservedOpportunity OutcomeStatus = "NO_OBSERVED_OPPORTUNITY"
	OutcomeRejected              OutcomeStatus = "REJECTED"
	OutcomeAcceptedUnfilled      OutcomeStatus = "ACCEPTED_UNFILLED"
	OutcomePartiallyFilled       OutcomeStatus = "PARTIALLY_FILLED"
	OutcomeFullyFilled           OutcomeStatus = "FULLY_FILLED"
)

type ReconstructedOutcome struct {
	Status                 OutcomeStatus `json:"status"`
	ClientID               uint64        `json:"client_id"`
	TargetQty              int64         `json:"target_qty"`
	DecisionAt             int64         `json:"decision_at"`
	DecisionMid            int64         `json:"decision_mid"`
	DeliveredSnapshotAt    int64         `json:"delivered_snapshot_at"`
	PublishedSnapshotAt    int64         `json:"published_snapshot_at"`
	DeliveredSnapshotSeq   uint64        `json:"delivered_snapshot_seq"`
	ProcessedSnapshotAt    int64         `json:"processed_snapshot_at,omitempty"`
	LatestMessageAt        int64         `json:"latest_message_at"`
	LatestMessageSeq       uint64        `json:"latest_message_seq"`
	LatestMessageTwoSided  bool          `json:"latest_message_two_sided"`
	RetainedAfterOneSided  bool          `json:"retained_after_one_sided"`
	DeliveredTouchAskQty   int64         `json:"delivered_touch_ask_qty"`
	DeliveredFiveAskQty    int64         `json:"delivered_five_ask_qty"`
	DeliveredSpread        int64         `json:"delivered_spread"`
	MechanicalSweepQty     int64         `json:"mechanical_sweep_qty"`
	MechanicalSweepCost    int64         `json:"mechanical_sweep_cost"`
	RequestID              uint64        `json:"request_id"`
	OrderID                uint64        `json:"order_id"`
	OrderSentAt            int64         `json:"order_sent_at"`
	VenueArrivalAt         int64         `json:"venue_arrival_at"`
	RejectReason           string        `json:"reject_reason,omitempty"`
	FilledQty              int64         `json:"filled_qty"`
	UnfilledQty            int64         `json:"unfilled_qty"`
	FirstVenueFillAt       int64         `json:"first_venue_fill_at"`
	LastVenueFillAt        int64         `json:"last_venue_fill_at"`
	FillCount              int           `json:"fill_count"`
	Notional               int64         `json:"notional"`
	QuoteFees              int64         `json:"quote_fees"`
	CancelledResidual      int64         `json:"cancelled_residual"`
	CancelReason           string        `json:"cancel_reason,omitempty"`
	TerminalMid            int64         `json:"terminal_mid"`
	TerminalMarkAvailable  bool          `json:"terminal_mark_available"`
	FilledShortfall        int64         `json:"filled_shortfall"`
	FilledShortfallBps     float64       `json:"filled_shortfall_bps"`
	TargetShortfall        int64         `json:"target_shortfall"`
	TargetShortfallBps     float64       `json:"target_shortfall_bps"`
	TargetShortfallDefined bool          `json:"target_shortfall_defined"`
	InitialABC             int64         `json:"initial_abc"`
	InitialUSD             int64         `json:"initial_usd"`
	TerminalABC            int64         `json:"terminal_abc"`
	TerminalUSD            int64         `json:"terminal_usd"`
	BalanceSnapshotSeen    bool          `json:"balance_snapshot_seen"`
}

type analysisContract struct {
	Config struct {
		RecordSnapshotProjectionEvidence bool `json:"RecordSnapshotProjectionEvidence"`
	} `json:"config"`
	Instrument struct {
		Symbol        string `json:"symbol"`
		BaseAsset     string `json:"base_asset"`
		QuoteAsset    string `json:"quote_asset"`
		BasePrecision int64  `json:"base_precision"`
	} `json:"instrument"`
	Accounts []struct {
		ClientID        uint64           `json:"client_id"`
		Role            string           `json:"role"`
		InitialBalances map[string]int64 `json:"initial_balances"`
		Fee             struct {
			MakerBps int64 `json:"MakerBps"`
			TakerBps int64 `json:"TakerBps"`
			InQuote  bool  `json:"InQuote"`
		} `json:"fee"`
	} `json:"accounts"`
	Parents []struct {
		ClientID   uint64 `json:"client_id"`
		Latency    int64  `json:"latency_nanos"`
		Deployment *struct {
			MarketDataLatency int64 `json:"market_data_latency_nanos"`
			RequestLatency    int64 `json:"request_latency_nanos"`
			ResponseLatency   int64 `json:"response_latency_nanos"`
			ProcessingDelay   int64 `json:"processing_delay_nanos"`
		} `json:"deployment,omitempty"`
		Config struct {
			Symbol        string `json:"Symbol"`
			Side          string `json:"Side"`
			TargetQty     int64  `json:"TargetQty"`
			DecisionAfter int64  `json:"DecisionAfter"`
			PollInterval  int64  `json:"PollInterval"`
		} `json:"config"`
	} `json:"parents"`
	Runner struct {
		Iterations int   `json:"iterations"`
		Step       int64 `json:"step_nanos"`
	} `json:"runner"`
}

func analysisInputs(plan LockedPlan) (analysisContract, uint64, error) {
	var contract analysisContract
	digest, err := planDigest(plan)
	if err != nil || digest != plan.TypedPlanSHA256 || plan.SchemaVersion != PlanSchemaVersion {
		return contract, 0, errors.New("execution pilot: invalid typed analysis plan")
	}
	if err := json.Unmarshal(plan.EffectiveWorld, &contract); err != nil {
		return contract, 0, fmt.Errorf("execution pilot: decode analysis contract: %w", err)
	}
	if len(contract.Parents) != 1 || contract.Instrument.Symbol == "" ||
		contract.Instrument.BasePrecision <= 0 || contract.Parents[0].Config.Side != "BUY" ||
		contract.Parents[0].Config.TargetQty != plan.Cell.TargetQty ||
		contract.Parents[0].Config.Symbol != contract.Instrument.Symbol ||
		contract.Runner.Iterations != 4000 || contract.Runner.Step != 1_000_000 ||
		contract.Parents[0].Config.PollInterval != 1_000_000 ||
		contract.Parents[0].Config.DecisionAfter != 1_000_000_000 ||
		contract.Parents[0].Latency != 1_000_000 || !contract.Config.RecordSnapshotProjectionEvidence {
		return contract, 0, errors.New("execution pilot: unsupported or inconsistent focal contract")
	}
	clientID := contract.Parents[0].ClientID
	for _, account := range contract.Accounts {
		if account.ClientID == clientID && account.Role == "parent" {
			if account.InitialBalances[contract.Instrument.BaseAsset] <= 0 || account.InitialBalances[contract.Instrument.QuoteAsset] <= 0 {
				return contract, 0, errors.New("execution pilot: missing focal initial endowment")
			}
			if !account.Fee.InQuote || account.Fee.TakerBps != 5 || account.Fee.MakerBps != 0 {
				return contract, 0, errors.New("execution pilot: unsupported focal fee contract")
			}
			return contract, clientID, nil
		}
	}
	return contract, 0, errors.New("execution pilot: focal account missing from effective world")
}

type snapshotWire struct {
	Symbol    string `json:"Symbol"`
	Timestamp int64  `json:"Timestamp"`
	SeqNum    uint64 `json:"SeqNum"`
	Snapshot  *struct {
		Bids []exchange.PriceLevel `json:"bids"`
		Asks []exchange.PriceLevel `json:"asks"`
	} `json:"Snapshot"`
}

type publicationWire struct {
	SourceSequence uint64                `json:"source_sequence"`
	PublicBids     []exchange.PriceLevel `json:"public_bids"`
	PublicAsks     []exchange.PriceLevel `json:"public_asks"`
}

type publishedSnapshot struct {
	Timestamp int64
	Payload   publicationWire
}

type publicationOutcomeWire struct {
	ClientID  uint64 `json:"client_id"`
	Symbol    string `json:"symbol"`
	Type      int    `json:"type"`
	Sequence  uint64 `json:"sequence"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`
}

type orderWire struct {
	RequestID   uint64 `json:"request_id"`
	OrderID     uint64 `json:"order_id"`
	ClientID    uint64 `json:"client_id"`
	Symbol      string `json:"symbol"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	Visibility  string `json:"visibility"`
	Price       int64  `json:"price"`
	PostOnly    bool   `json:"post_only"`
	ReduceOnly  bool   `json:"reduce_only"`
	Qty         int64  `json:"qty"`
	FilledQty   int64  `json:"filled_qty"`
	Timestamp   int64  `json:"timestamp"`
	Error       string `json:"error"`
}

type fillWire struct {
	OrderID      uint64 `json:"order_id"`
	Symbol       string `json:"symbol"`
	Qty          int64  `json:"qty"`
	Price        int64  `json:"price"`
	Side         string `json:"side"`
	TradeID      uint64 `json:"trade_id"`
	FeeAmount    int64  `json:"fee_amount"`
	FeeAsset     string `json:"fee_asset"`
	FilledQty    int64  `json:"filled_qty"`
	RemainingQty int64  `json:"remaining_qty"`
	Timestamp    int64  `json:"timestamp"`
}

type tradeWire struct {
	TradeID      uint64 `json:"trade_id"`
	TakerOrderID uint64 `json:"taker_order_id"`
	Qty          int64  `json:"qty"`
	Price        int64  `json:"price"`
}

type balanceChangeWire struct {
	Timestamp int64  `json:"timestamp"`
	ClientID  uint64 `json:"client_id"`
	Reason    string `json:"reason"`
	Changes   []struct {
		Asset      string `json:"asset"`
		Wallet     string `json:"wallet"`
		OldBalance int64  `json:"old_balance"`
		NewBalance int64  `json:"new_balance"`
		Delta      int64  `json:"delta"`
	} `json:"changes"`
}

type balanceSnapshotWire struct {
	ClientID     uint64 `json:"client_id"`
	SpotBalances []struct {
		Asset    string `json:"asset"`
		Free     int64  `json:"free"`
		Locked   int64  `json:"locked"`
		Borrowed int64  `json:"borrowed"`
	} `json:"spot_balances"`
}

type reconstructionState struct {
	contract              analysisContract
	clientID              uint64
	result                ReconstructedOutcome
	lastSnapshot          snapshotWire
	lastReceipt           int64
	lastSnapshotEventSeq  uint64
	lastProcessedAt       int64
	latestMessage         snapshotWire
	latestReceipt         int64
	latestMessageEventSeq uint64
	eligibleTick          int64
	wasSent               bool
	wasAccepted           bool
	wasRejected           bool
	wasCancelled          bool
	cancelledAt           int64
	terminalSeen          bool
	acceptedSeen          bool
	responded             bool
	balances              map[string]int64
	fillByTrade           map[uint64]fillWire
	tradeByID             map[uint64]tradeWire
	receipts              map[uint64]bool
	ledgerABC             int64
	ledgerUSD             int64
	feeBps                int64
	lastEventTS           int64
	decisionTicks         int64
	publications          map[uint64]publishedSnapshot
	publicationOutcomes   map[uint64]publicationOutcomeWire
	deliveredSnapshots    map[uint64]bool
	lastDeliveredSeq      uint64
	lastProcessedSeq      uint64
	pendingProcessing     map[uint64]pendingReconstructionSnapshot
	processingDelay       int64
	marketDataLatency     int64
	requestLatency        int64
	responseLatency       int64
	latencyEvidence       bool
	cancelReceiptCount    int
}

type pendingReconstructionSnapshot struct {
	snapshot        snapshotWire
	receivedAt      int64
	receiptEventSeq uint64
}

func Reconstruct(input io.Reader, evidence EvidenceIdentity, plan LockedPlan) (ReconstructedOutcome, error) {
	contract, clientID, err := analysisInputs(plan)
	if err != nil {
		return ReconstructedOutcome{}, err
	}
	state := newReconstructionState(contract, clientID, plan.Cell.TargetQty)
	if err := WalkEvidence(input, evidence, state.consume); err != nil {
		return ReconstructedOutcome{}, err
	}
	if err := state.finish(); err != nil {
		return ReconstructedOutcome{}, err
	}
	return state.result, nil
}

// ReconstructLatency independently replays the versioned deployment evidence.
// Callers must bind effectiveWorld to a verified, pinned plan before invoking it.
func ReconstructLatency(input io.Reader, evidence EvidenceIdentity, effectiveWorld json.RawMessage, targetQty int64) (ReconstructedOutcome, error) {
	var contract analysisContract
	if err := json.Unmarshal(effectiveWorld, &contract); err != nil {
		return ReconstructedOutcome{}, fmt.Errorf("latency pilot: decode effective world: %w", err)
	}
	if len(contract.Parents) != 1 || contract.Parents[0].Deployment == nil || contract.Parents[0].Latency != 0 ||
		contract.Parents[0].Config.TargetQty != targetQty || targetQty <= 0 || contract.Parents[0].Config.Side != "BUY" ||
		contract.Parents[0].Config.Symbol != contract.Instrument.Symbol || contract.Instrument.BasePrecision <= 0 ||
		contract.Runner.Iterations != 4000 || contract.Runner.Step != 1_000_000 ||
		contract.Parents[0].Config.PollInterval != 1_000_000 || contract.Parents[0].Config.DecisionAfter != 1_000_000_000 ||
		!contract.Config.RecordSnapshotProjectionEvidence {
		return ReconstructedOutcome{}, errors.New("latency pilot: unsupported or inconsistent focal contract")
	}
	deployment := contract.Parents[0].Deployment
	if deployment.MarketDataLatency < 0 || deployment.RequestLatency < 0 || deployment.ResponseLatency < 0 || deployment.ProcessingDelay < 0 {
		return ReconstructedOutcome{}, errors.New("latency pilot: negative deployment component")
	}
	clientID := contract.Parents[0].ClientID
	accountFound := false
	for _, account := range contract.Accounts {
		if account.ClientID == clientID && account.Role == "parent" {
			accountFound = account.InitialBalances[contract.Instrument.BaseAsset] > 0 && account.InitialBalances[contract.Instrument.QuoteAsset] > 0 &&
				account.Fee.InQuote && account.Fee.TakerBps == 5 && account.Fee.MakerBps == 0
		}
	}
	if !accountFound {
		return ReconstructedOutcome{}, errors.New("latency pilot: focal account or fee contract missing")
	}
	state := newReconstructionState(contract, clientID, targetQty)
	state.marketDataLatency = deployment.MarketDataLatency
	state.requestLatency = deployment.RequestLatency
	state.responseLatency = deployment.ResponseLatency
	state.processingDelay = deployment.ProcessingDelay
	state.latencyEvidence = true
	if err := WalkLatencyEvidence(input, evidence, state.consume); err != nil {
		return ReconstructedOutcome{}, err
	}
	if err := state.finish(); err != nil {
		return ReconstructedOutcome{}, err
	}
	return state.result, nil
}

func newReconstructionState(contract analysisContract, clientID uint64, targetQty int64) *reconstructionState {
	state := &reconstructionState{
		contract: contract, clientID: clientID,
		result:   ReconstructedOutcome{ClientID: clientID, TargetQty: targetQty},
		balances: map[string]int64{}, fillByTrade: map[uint64]fillWire{},
		tradeByID: map[uint64]tradeWire{}, receipts: map[uint64]bool{},
		publications:        map[uint64]publishedSnapshot{},
		publicationOutcomes: map[uint64]publicationOutcomeWire{}, deliveredSnapshots: map[uint64]bool{},
		pendingProcessing: map[uint64]pendingReconstructionSnapshot{},
		marketDataLatency: contract.Parents[0].Latency,
		requestLatency:    contract.Parents[0].Latency,
		responseLatency:   contract.Parents[0].Latency,
	}
	for _, account := range contract.Accounts {
		if account.ClientID == clientID {
			state.feeBps = account.Fee.TakerBps
			for asset, balance := range account.InitialBalances {
				state.balances[asset] = balance
			}
			state.result.InitialABC = account.InitialBalances[contract.Instrument.BaseAsset]
			state.result.InitialUSD = account.InitialBalances[contract.Instrument.QuoteAsset]
		}
	}
	return state
}

func decodePayload[T any](raw json.RawMessage) (T, error) {
	var value T
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, fmt.Errorf("execution pilot: malformed required event: %w", err)
	}
	return value, nil
}

func requireEventField(raw json.RawMessage, field string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	value, present := fields[field]
	if !present || string(value) == "null" {
		return fmt.Errorf("execution pilot: required event field %s is absent", field)
	}
	return nil
}

func (s *reconstructionState) consume(event RecordedEvent) error {
	if event.Timestamp < s.lastEventTS || event.Timestamp < 0 || event.Timestamp > int64(s.contract.Runner.Iterations)*s.contract.Runner.Step {
		return errors.New("execution pilot: nonmonotone or out-of-horizon evidence time")
	}
	s.lastEventTS = event.Timestamp
	if event.Source == "actor" && event.ClientID == s.clientID {
		return s.consumeActor(event)
	}
	if event.Source == "transport" && event.ClientID == s.clientID {
		return s.consumeTransport(event)
	}
	if event.Source == "exchange" {
		return s.consumeExchange(event)
	}
	return nil
}

func (s *reconstructionState) consumeActor(event RecordedEvent) error {
	switch event.Name {
	case "book_snapshot_receipt":
		snapshot, err := decodePayload[snapshotWire](event.Payload)
		if err != nil {
			return err
		}
		if snapshot.Symbol != s.contract.Instrument.Symbol || snapshot.Timestamp > event.Timestamp || snapshot.Snapshot == nil {
			return errors.New("execution pilot: invalid delivered snapshot identity or time")
		}
		publication, published := s.publications[snapshot.SeqNum]
		outcome, accounted := s.publicationOutcomes[snapshot.SeqNum]
		if !published || snapshot.SeqNum <= s.lastDeliveredSeq ||
			!accounted || outcome.Status != "enqueued" ||
			snapshot.Timestamp != publication.Timestamp ||
			event.Timestamp != publication.Timestamp+s.marketDataLatency ||
			!reflect.DeepEqual(snapshot.Snapshot.Bids, publication.Payload.PublicBids) ||
			!reflect.DeepEqual(snapshot.Snapshot.Asks, publication.Payload.PublicAsks) {
			return errors.New("execution pilot: local snapshot lacks matching prior venue publication or delivery latency")
		}
		s.lastDeliveredSeq = snapshot.SeqNum
		s.deliveredSnapshots[snapshot.SeqNum] = true
		s.latestMessage, s.latestReceipt, s.latestMessageEventSeq = snapshot, event.Timestamp, event.Sequence
		if s.processingDelay > 0 {
			s.pendingProcessing[snapshot.SeqNum] = pendingReconstructionSnapshot{snapshot, event.Timestamp, event.Sequence}
		} else if len(snapshot.Snapshot.Bids) != 0 && len(snapshot.Snapshot.Asks) != 0 {
			s.lastSnapshot, s.lastReceipt, s.lastSnapshotEventSeq = snapshot, event.Timestamp, event.Sequence
			if s.latencyEvidence {
				s.lastProcessedAt = event.Timestamp
			}
		}
	case "snapshot_processing_complete":
		if !s.latencyEvidence || s.processingDelay == 0 {
			return errors.New("latency pilot: unexpected processing completion")
		}
		processed, err := decodePayload[struct {
			SeqNum      uint64 `json:"seq_num"`
			ReceivedAt  int64  `json:"received_at"`
			ProcessedAt int64  `json:"processed_at"`
			BestBid     int64  `json:"best_bid"`
			BestAsk     int64  `json:"best_ask"`
			TwoSided    bool   `json:"two_sided"`
		}](event.Payload)
		if err != nil {
			return err
		}
		pending, exists := s.pendingProcessing[processed.SeqNum]
		if !exists || processed.SeqNum <= s.lastProcessedSeq || processed.ReceivedAt != pending.receivedAt ||
			processed.ProcessedAt != event.Timestamp || event.Timestamp < pending.receivedAt+s.processingDelay ||
			event.Timestamp%s.contract.Runner.Step != 0 ||
			processed.TwoSided != (len(pending.snapshot.Snapshot.Bids) != 0 && len(pending.snapshot.Snapshot.Asks) != 0) {
			return errors.New("latency pilot: unmatched, misordered or premature processing completion")
		}
		if event.Timestamp-s.contract.Runner.Step >= pending.receivedAt+s.processingDelay {
			return errors.New("latency pilot: processing completion skipped an eligible poll")
		}
		bid, ask := int64(0), int64(0)
		if processed.TwoSided {
			bid, ask = pending.snapshot.Snapshot.Bids[0].Price, pending.snapshot.Snapshot.Asks[0].Price
		}
		if processed.BestBid != bid || processed.BestAsk != ask {
			return errors.New("latency pilot: processed quote differs from received snapshot")
		}
		delete(s.pendingProcessing, processed.SeqNum)
		s.lastProcessedSeq = processed.SeqNum
		if processed.TwoSided {
			s.lastSnapshot, s.lastReceipt, s.lastSnapshotEventSeq = pending.snapshot, pending.receivedAt, pending.receiptEventSeq
			s.lastProcessedAt = event.Timestamp
		}
	case "decision_tick":
		if event.Timestamp != (s.decisionTicks+1)*s.contract.Runner.Step {
			return errors.New("execution pilot: missing, duplicate or misphased focal decision tick")
		}
		s.decisionTicks++
		tick, err := decodePayload[struct {
			BestBid        int64 `json:"best_bid"`
			BestAsk        int64 `json:"best_ask"`
			AlreadyDecided bool  `json:"already_decided"`
		}](event.Payload)
		if err != nil {
			return err
		}
		bid, ask := int64(0), int64(0)
		if s.lastSnapshot.Snapshot != nil && len(s.lastSnapshot.Snapshot.Bids) != 0 && len(s.lastSnapshot.Snapshot.Asks) != 0 {
			bid, ask = s.lastSnapshot.Snapshot.Bids[0].Price, s.lastSnapshot.Snapshot.Asks[0].Price
		}
		if tick.BestBid != bid || tick.BestAsk != ask || tick.AlreadyDecided != s.wasSent {
			return errors.New("execution pilot: actor decision frontier differs from delivered snapshot")
		}
		if !s.wasSent && event.Timestamp >= s.contract.Parents[0].Config.DecisionAfter && bid > 0 && ask > 0 {
			if s.eligibleTick == 0 {
				s.eligibleTick = event.Timestamp
			}
		}
	case "order_send":
		return s.consumeSend(event)
	case "order_accepted_receipt":
		response, err := decodePayload[orderWire](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasAccepted || s.responded || response.RequestID != s.result.RequestID || response.OrderID != s.result.OrderID ||
			event.Timestamp != s.result.VenueArrivalAt+s.responseLatency {
			return errors.New("execution pilot: unmatched or duplicate acceptance receipt")
		}
		s.responded = true
	case "order_rejected_receipt":
		response, err := decodePayload[struct {
			RequestID uint64 `json:"request_id"`
			Reason    string `json:"reason"`
		}](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasRejected || s.responded || response.RequestID != s.result.RequestID || response.Reason != s.result.RejectReason ||
			event.Timestamp != s.result.VenueArrivalAt+s.responseLatency {
			return errors.New("execution pilot: unmatched or duplicate rejection receipt")
		}
		s.responded = true
	case "order_fill_receipt":
		fill, err := decodePayload[fillWire](event.Payload)
		if err != nil {
			return err
		}
		original, ok := s.fillByTrade[fill.TradeID]
		if !ok || s.receipts[fill.TradeID] || event.Timestamp != original.Timestamp+s.responseLatency ||
			fill.OrderID != original.OrderID || fill.Qty != original.Qty || fill.Price != original.Price ||
			fill.FeeAmount != original.FeeAmount || fill.FeeAsset != original.FeeAsset ||
			fill.Timestamp != original.Timestamp {
			return errors.New("execution pilot: fill receipt lacks matching earlier exchange execution")
		}
		s.receipts[fill.TradeID] = true
	case "order_cancelled_receipt":
		if err := requireEventField(event.Payload, "request_id"); err != nil {
			return err
		}
		cancel, err := decodePayload[struct {
			OrderID      uint64 `json:"order_id"`
			RequestID    uint64 `json:"request_id"`
			RemainingQty int64  `json:"remaining_qty"`
		}](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasCancelled || s.cancelReceiptCount != 0 || cancel.OrderID != s.result.OrderID || cancel.RequestID != 0 ||
			cancel.RemainingQty != s.result.CancelledResidual ||
			event.Timestamp != s.cancelledAt+s.responseLatency {
			return errors.New("execution pilot: cancellation receipt mismatch")
		}
		s.cancelReceiptCount++
	}
	return nil
}

func (s *reconstructionState) consumeTransport(event RecordedEvent) error {
	if event.Name != "snapshot_publish_outcome" {
		return nil
	}
	if err := requireEventField(event.Payload, "type"); err != nil {
		return err
	}
	outcome, err := decodePayload[publicationOutcomeWire](event.Payload)
	if err != nil {
		return err
	}
	if event.Route != s.contract.Instrument.Symbol || outcome.Symbol != event.Route ||
		outcome.ClientID != s.clientID || outcome.Sequence == 0 || outcome.Timestamp != event.Timestamp ||
		outcome.Type != int(exchange.MDSnapshot) {
		return errors.New("execution pilot: invalid focal snapshot publication outcome identity")
	}
	switch outcome.Status {
	case "enqueued", "dropped", "not_subscribed", "not_interested", "gateway_stopped":
	default:
		return errors.New("execution pilot: unknown snapshot publication outcome")
	}
	if _, exists := s.publicationOutcomes[outcome.Sequence]; exists {
		return errors.New("execution pilot: duplicate focal snapshot publication outcome")
	}
	s.publicationOutcomes[outcome.Sequence] = outcome
	return nil
}

func (s *reconstructionState) consumeSend(event RecordedEvent) error {
	request, err := decodePayload[struct {
		Type     string    `json:"Type"`
		OrderReq orderWire `json:"OrderReq"`
	}](event.Payload)
	if err != nil {
		return err
	}
	if s.wasSent || event.Timestamp != s.eligibleTick || request.Type != "place_order" ||
		request.OrderReq.RequestID == 0 || request.OrderReq.Symbol != s.contract.Instrument.Symbol ||
		request.OrderReq.Side != "BUY" || request.OrderReq.Type != "MARKET" ||
		request.OrderReq.Qty != s.result.TargetQty || request.OrderReq.Price != 0 ||
		request.OrderReq.TimeInForce != "GTC" || request.OrderReq.Visibility != "NORMAL" ||
		request.OrderReq.PostOnly || request.OrderReq.ReduceOnly {
		return errors.New("execution pilot: focal order send does not match eligible decision and locked contract")
	}
	s.wasSent = true
	s.result.DecisionAt = event.Timestamp
	s.result.OrderSentAt = event.Timestamp
	s.result.RequestID = request.OrderReq.RequestID
	s.result.DecisionMid = s.lastSnapshot.Snapshot.Bids[0].Price +
		(s.lastSnapshot.Snapshot.Asks[0].Price-s.lastSnapshot.Snapshot.Bids[0].Price)/2
	if s.lastSnapshot.Snapshot.Bids[0].Price > s.lastSnapshot.Snapshot.Asks[0].Price || s.result.DecisionMid <= 0 {
		return errors.New("execution pilot: invalid delivered two-sided price")
	}
	s.result.DeliveredSnapshotAt = s.lastReceipt
	s.result.PublishedSnapshotAt = s.lastSnapshot.Timestamp
	s.result.DeliveredSnapshotSeq = s.lastSnapshot.SeqNum
	s.result.ProcessedSnapshotAt = s.lastProcessedAt
	s.result.LatestMessageAt = s.latestReceipt
	s.result.LatestMessageSeq = s.latestMessage.SeqNum
	s.result.LatestMessageTwoSided = len(s.latestMessage.Snapshot.Bids) != 0 && len(s.latestMessage.Snapshot.Asks) != 0
	s.result.RetainedAfterOneSided = s.latestMessageEventSeq > s.lastSnapshotEventSeq && !s.result.LatestMessageTwoSided
	s.result.DeliveredTouchAskQty = s.lastSnapshot.Snapshot.Asks[0].VisibleQty
	s.result.DeliveredSpread = s.lastSnapshot.Snapshot.Asks[0].Price - s.lastSnapshot.Snapshot.Bids[0].Price
	for index, level := range s.lastSnapshot.Snapshot.Asks {
		if index >= 5 {
			break
		}
		if level.Price <= 0 || level.VisibleQty < 0 ||
			(index > 0 && level.Price < s.lastSnapshot.Snapshot.Asks[index-1].Price) ||
			!addSafe(s.result.DeliveredFiveAskQty, level.VisibleQty) {
			return errors.New("execution pilot: malformed delivered ask curve")
		}
		s.result.DeliveredFiveAskQty += level.VisibleQty
		remaining := s.result.TargetQty - s.result.MechanicalSweepQty
		if remaining <= 0 {
			continue
		}
		quantity := min(remaining, level.VisibleQty)
		notional, ok := types.TryMulDiv(quantity, level.Price, s.contract.Instrument.BasePrecision)
		if !ok || quantity < 0 || !addSafe(s.result.MechanicalSweepCost, notional) {
			return errors.New("execution pilot: invalid delivered ask sweep arithmetic")
		}
		s.result.MechanicalSweepQty += quantity
		s.result.MechanicalSweepCost += notional
	}
	return nil
}

func (s *reconstructionState) consumeExchange(event RecordedEvent) error {
	if event.Name == "BookSnapshot" {
		if event.Route != s.contract.Instrument.Symbol {
			return errors.New("execution pilot: venue snapshot routed from wrong symbol")
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &fields); err != nil {
			return err
		}
		for _, key := range []string{"source_sequence", "public_bids", "public_asks"} {
			if _, present := fields[key]; !present {
				return errors.New("execution pilot: venue snapshot lacks public projection identity")
			}
		}
		publication, err := decodePayload[publicationWire](event.Payload)
		if err != nil {
			return err
		}
		if publication.SourceSequence != 0 {
			if _, exists := s.publications[publication.SourceSequence]; exists {
				return errors.New("execution pilot: duplicate venue snapshot sequence")
			}
			s.publications[publication.SourceSequence] = publishedSnapshot{Timestamp: event.Timestamp, Payload: publication}
		}
		return nil
	}
	if event.Name == "Trade" {
		if event.Route != s.contract.Instrument.Symbol {
			return errors.New("execution pilot: venue trade routed from wrong symbol")
		}
		trade, err := decodePayload[tradeWire](event.Payload)
		if err != nil {
			return err
		}
		if trade.TakerOrderID == s.result.OrderID && s.result.OrderID != 0 {
			if trade.Qty <= 0 || trade.Price <= 0 {
				return errors.New("execution pilot: malformed focal venue trade")
			}
			if _, exists := s.tradeByID[trade.TradeID]; exists {
				return errors.New("execution pilot: duplicate focal trade identity")
			}
			s.tradeByID[trade.TradeID] = trade
		}
		return nil
	}
	if event.Name == "terminal_book" {
		if event.Route != s.contract.Instrument.Symbol {
			return errors.New("execution pilot: terminal book routed from wrong symbol")
		}
		if !s.result.BalanceSnapshotSeen || event.Timestamp != int64(s.contract.Runner.Iterations)*s.contract.Runner.Step {
			return errors.New("execution pilot: terminal book is not at shutdown after ledger snapshot")
		}
		terminal, err := decodePayload[struct {
			Symbol string `json:"symbol"`
			Bid    struct {
				Price    int64 `json:"price"`
				TotalQty int64 `json:"total_qty"`
			} `json:"bid"`
			Ask struct {
				Price    int64 `json:"price"`
				TotalQty int64 `json:"total_qty"`
			} `json:"ask"`
			Valid bool `json:"valid"`
		}](event.Payload)
		if err != nil {
			return err
		}
		if s.terminalSeen || terminal.Symbol != s.contract.Instrument.Symbol {
			return errors.New("execution pilot: duplicate or wrong terminal book")
		}
		s.terminalSeen = true
		if terminal.Valid {
			if terminal.Bid.Price <= 0 || terminal.Ask.Price <= 0 || terminal.Bid.Price > terminal.Ask.Price ||
				terminal.Bid.TotalQty <= 0 || terminal.Ask.TotalQty <= 0 {
				return errors.New("execution pilot: invalid terminal two-sided book")
			}
			s.result.TerminalMid = terminal.Bid.Price + (terminal.Ask.Price-terminal.Bid.Price)/2
			s.result.TerminalMarkAvailable = true
		} else if terminal.Bid.Price != 0 || terminal.Ask.Price != 0 || terminal.Bid.TotalQty != 0 || terminal.Ask.TotalQty != 0 {
			return errors.New("execution pilot: unavailable terminal book carries nonempty quote")
		}
		return nil
	}
	if event.ClientID != s.clientID {
		return nil
	}
	if event.Name == "balance_snapshot" {
		if event.Route != "_global" {
			return errors.New("execution pilot: terminal balance snapshot routed incorrectly")
		}
	} else if event.Name == "OrderAccepted" || event.Name == "OrderRejected" || event.Name == "OrderFill" ||
		event.Name == "OrderCancelled" || event.Name == "balance_change" {
		if event.Route != s.contract.Instrument.Symbol {
			return errors.New("execution pilot: focal venue event routed from wrong symbol")
		}
	}
	switch event.Name {
	case "OrderAccepted":
		accepted, err := decodePayload[orderWire](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasSent || s.acceptedSeen || accepted.RequestID != s.result.RequestID || accepted.ClientID != s.clientID ||
			accepted.OrderID == 0 || accepted.Qty != s.result.TargetQty || accepted.Side != "BUY" || accepted.Type != "MARKET" ||
			accepted.Price != 0 || accepted.FilledQty != 0 || accepted.TimeInForce != "GTC" ||
			accepted.Visibility != "NORMAL" || accepted.PostOnly ||
			event.Timestamp != s.result.OrderSentAt+s.requestLatency {
			return errors.New("execution pilot: accepted order does not match focal intent")
		}
		if accepted.Timestamp != event.Timestamp {
			return errors.New("execution pilot: accepted order timestamp differs from venue event")
		}
		s.acceptedSeen, s.wasAccepted = true, true
		s.result.OrderID, s.result.VenueArrivalAt = accepted.OrderID, event.Timestamp
	case "OrderRejected":
		rejected, err := decodePayload[orderWire](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasSent || s.acceptedSeen || rejected.RequestID != s.result.RequestID || rejected.Error == "" ||
			rejected.Qty != s.result.TargetQty || rejected.Symbol != s.contract.Instrument.Symbol ||
			rejected.Side != "BUY" || rejected.Type != "MARKET" || rejected.Price != 0 ||
			rejected.TimeInForce != "GTC" || rejected.PostOnly ||
			event.Timestamp != s.result.OrderSentAt+s.requestLatency {
			return errors.New("execution pilot: rejected order does not match focal intent")
		}
		s.acceptedSeen, s.wasRejected = true, true
		s.result.VenueArrivalAt, s.result.RejectReason = event.Timestamp, rejected.Error
	case "OrderFill":
		fill, err := decodePayload[fillWire](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasAccepted || s.wasCancelled || fill.OrderID != s.result.OrderID || fill.Symbol != s.contract.Instrument.Symbol ||
			fill.Side != "BUY" || fill.Qty <= 0 || fill.Price <= 0 ||
			fill.FeeAsset != s.contract.Instrument.QuoteAsset || fill.FeeAmount < 0 ||
			fill.FilledQty != s.result.FilledQty+fill.Qty || fill.RemainingQty != s.result.TargetQty-fill.FilledQty ||
			event.Timestamp < s.result.VenueArrivalAt {
			return errors.New("execution pilot: unjoined or inconsistent focal fill")
		}
		if _, exists := s.fillByTrade[fill.TradeID]; exists {
			return errors.New("execution pilot: duplicate focal fill trade ID")
		}
		notional, ok := types.TryMulDiv(fill.Qty, fill.Price, s.contract.Instrument.BasePrecision)
		expectedFee, feeOK := types.TryMulBps(notional, s.feeBps)
		if !ok || !feeOK || fill.FeeAmount != expectedFee ||
			!addSafe(s.result.Notional, notional) || !addSafe(s.result.QuoteFees, fill.FeeAmount) {
			return errors.New("execution pilot: focal fill arithmetic overflow")
		}
		fill.Timestamp = event.Timestamp
		s.fillByTrade[fill.TradeID] = fill
		s.result.FillCount++
		s.result.FilledQty += fill.Qty
		s.result.Notional += notional
		s.result.QuoteFees += fill.FeeAmount
		if s.result.FirstVenueFillAt == 0 {
			s.result.FirstVenueFillAt = event.Timestamp
		}
		s.result.LastVenueFillAt = event.Timestamp
	case "OrderCancelled":
		if err := requireEventField(event.Payload, "request_id"); err != nil {
			return err
		}
		cancel, err := decodePayload[struct {
			OrderID      uint64 `json:"order_id"`
			RequestID    uint64 `json:"request_id"`
			RemainingQty int64  `json:"remaining_qty"`
			Reason       string `json:"reason"`
		}](event.Payload)
		if err != nil {
			return err
		}
		if !s.wasAccepted || s.wasCancelled || cancel.OrderID != s.result.OrderID ||
			cancel.RequestID != s.result.RequestID || cancel.RemainingQty <= 0 || cancel.Reason != "NO_LIQUIDITY" {
			return errors.New("execution pilot: unmatched focal cancellation")
		}
		s.wasCancelled = true
		s.cancelledAt = event.Timestamp
		s.result.CancelledResidual = cancel.RemainingQty
		s.result.CancelReason = cancel.Reason
	case "balance_change":
		return s.consumeBalanceChange(event)
	case "balance_snapshot":
		return s.consumeBalanceSnapshot(event)
	}
	return nil
}

func addSafe(left, right int64) bool {
	return (right >= 0 && left <= math.MaxInt64-right) || (right < 0 && left >= math.MinInt64-right)
}

func (s *reconstructionState) consumeBalanceChange(event RecordedEvent) error {
	change, err := decodePayload[balanceChangeWire](event.Payload)
	if err != nil {
		return err
	}
	if change.ClientID != s.clientID || change.Timestamp != event.Timestamp || len(change.Changes) == 0 {
		return errors.New("execution pilot: invalid focal balance change identity")
	}
	for _, delta := range change.Changes {
		if delta.Wallet != "spot" || delta.Asset == "" || !addSafe(delta.OldBalance, delta.Delta) || delta.NewBalance != delta.OldBalance+delta.Delta || s.balances[delta.Asset] != delta.OldBalance {
			return errors.New("execution pilot: broken focal balance delta chain")
		}
		s.balances[delta.Asset] = delta.NewBalance
		if delta.Asset == s.contract.Instrument.BaseAsset {
			if !addSafe(s.ledgerABC, delta.Delta) {
				return errors.New("execution pilot: base ledger sum overflow")
			}
			s.ledgerABC += delta.Delta
		}
		if delta.Asset == s.contract.Instrument.QuoteAsset {
			if !addSafe(s.ledgerUSD, delta.Delta) {
				return errors.New("execution pilot: quote ledger sum overflow")
			}
			s.ledgerUSD += delta.Delta
		}
	}
	return nil
}

func (s *reconstructionState) consumeBalanceSnapshot(event RecordedEvent) error {
	snapshot, err := decodePayload[balanceSnapshotWire](event.Payload)
	if err != nil {
		return err
	}
	if s.result.BalanceSnapshotSeen || snapshot.ClientID != s.clientID || event.Timestamp != int64(s.contract.Runner.Iterations)*s.contract.Runner.Step || s.terminalSeen {
		return errors.New("execution pilot: duplicate or mismatched focal balance snapshot")
	}
	s.result.BalanceSnapshotSeen = true
	seen := map[string]bool{}
	for _, balance := range snapshot.SpotBalances {
		if seen[balance.Asset] || balance.Borrowed != 0 || !addSafe(balance.Free, balance.Locked) ||
			balance.Free+balance.Locked != s.balances[balance.Asset] {
			return errors.New("execution pilot: terminal balance differs from replayed ledger")
		}
		seen[balance.Asset] = true
	}
	if !seen[s.contract.Instrument.BaseAsset] || !seen[s.contract.Instrument.QuoteAsset] {
		return errors.New("execution pilot: missing terminal asset balance")
	}
	s.result.TerminalABC = s.balances[s.contract.Instrument.BaseAsset]
	s.result.TerminalUSD = s.balances[s.contract.Instrument.QuoteAsset]
	return nil
}

func (s *reconstructionState) finish() error {
	for sequence, publication := range s.publications {
		outcome, present := s.publicationOutcomes[sequence]
		if !present || outcome.Timestamp != publication.Timestamp {
			return errors.New("execution pilot: unaccounted focal snapshot publication")
		}
		if outcome.Status == "enqueued" &&
			publication.Timestamp+s.marketDataLatency <= int64(s.contract.Runner.Iterations)*s.contract.Runner.Step &&
			!s.deliveredSnapshots[sequence] {
			return errors.New("execution pilot: enqueued focal snapshot has no actor receipt")
		}
	}
	for sequence := range s.publicationOutcomes {
		if _, published := s.publications[sequence]; !published {
			return errors.New("execution pilot: focal delivery outcome lacks venue publication")
		}
	}
	if s.decisionTicks != int64(s.contract.Runner.Iterations) {
		return errors.New("execution pilot: incomplete focal decision clock evidence")
	}
	if s.processingDelay > 0 {
		for _, pending := range s.pendingProcessing {
			if pending.receivedAt+s.processingDelay <= int64(s.contract.Runner.Iterations)*s.contract.Runner.Step {
				return errors.New("latency pilot: missing due processing completion")
			}
		}
	}
	if !s.terminalSeen || !s.result.BalanceSnapshotSeen {
		return errors.New("execution pilot: missing terminal venue or ledger evidence")
	}
	if !s.wasSent {
		if s.eligibleTick != 0 {
			return errors.New("execution pilot: immediate policy omitted order despite eligible local opportunity")
		}
		if s.ledgerABC != 0 || s.ledgerUSD != 0 {
			return errors.New("execution pilot: focal ledger changed without an order")
		}
		s.result.Status = OutcomeNoObservedOpportunity
		return nil
	}
	if !s.acceptedSeen || !s.responded || (s.wasAccepted == s.wasRejected) {
		return errors.New("execution pilot: missing or ambiguous focal admission response")
	}
	if s.wasCancelled && s.cancelReceiptCount != 1 || !s.wasCancelled && s.cancelReceiptCount != 0 {
		return errors.New("execution pilot: missing or extra focal cancellation receipt")
	}
	if s.wasRejected {
		if s.result.FillCount != 0 || s.result.OrderID != 0 || s.wasCancelled || s.ledgerABC != 0 || s.ledgerUSD != 0 {
			return errors.New("execution pilot: rejected request has execution")
		}
		s.result.Status = OutcomeRejected
		s.result.UnfilledQty = s.result.TargetQty
		return s.finishArithmetic()
	}
	if s.result.FilledQty > s.result.TargetQty || s.result.FilledQty < 0 ||
		s.result.FilledQty+s.result.CancelledResidual != s.result.TargetQty {
		return errors.New("execution pilot: admitted quantity does not equal fills plus terminal residual")
	}
	for tradeID, fill := range s.fillByTrade {
		trade, exists := s.tradeByID[tradeID]
		if !exists || trade.TakerOrderID != s.result.OrderID || trade.Qty != fill.Qty || trade.Price != fill.Price || !s.receipts[tradeID] {
			return errors.New("execution pilot: focal trade, fill or receipt join failed")
		}
	}
	if len(s.tradeByID) != len(s.fillByTrade) || len(s.receipts) != len(s.fillByTrade) {
		return errors.New("execution pilot: unmatched focal trade or receipt")
	}
	settledCost, ok := types.TryAdd(s.result.Notional, s.result.QuoteFees)
	if !ok || s.ledgerABC != s.result.FilledQty || s.ledgerUSD != -settledCost {
		return errors.New("execution pilot: cash/asset ledger disagrees with executions and fees")
	}
	s.result.UnfilledQty = s.result.TargetQty - s.result.FilledQty
	switch {
	case s.result.FilledQty == 0:
		s.result.Status = OutcomeAcceptedUnfilled
	case s.result.UnfilledQty == 0:
		s.result.Status = OutcomeFullyFilled
	default:
		s.result.Status = OutcomePartiallyFilled
	}
	return s.finishArithmetic()
}

func (s *reconstructionState) finishArithmetic() error {
	if s.result.DecisionMid <= 0 {
		return errors.New("execution pilot: missing positive decision midpoint")
	}
	filledReference, ok := types.TryMulDiv(s.result.FilledQty, s.result.DecisionMid, s.contract.Instrument.BasePrecision)
	if !ok {
		return errors.New("execution pilot: filled reference overflow")
	}
	priceDifference, ok := types.TrySub(s.result.Notional, filledReference)
	if !ok {
		return errors.New("execution pilot: filled price shortfall overflow")
	}
	s.result.FilledShortfall, ok = types.TryAdd(priceDifference, s.result.QuoteFees)
	if !ok {
		return errors.New("execution pilot: filled all-in shortfall overflow")
	}
	if filledReference > 0 {
		s.result.FilledShortfallBps = float64(s.result.FilledShortfall) * 10000 / float64(filledReference)
	}
	if !s.result.TerminalMarkAvailable {
		return nil
	}
	residualCost, ok := types.TryMulDiv(s.result.UnfilledQty, s.result.TerminalMid, s.contract.Instrument.BasePrecision)
	if !ok {
		return errors.New("execution pilot: terminal residual overflow")
	}
	targetReference, ok := types.TryMulDiv(s.result.TargetQty, s.result.DecisionMid, s.contract.Instrument.BasePrecision)
	if !ok || targetReference <= 0 {
		return errors.New("execution pilot: target reference unavailable")
	}
	completionCost, ok := types.TryAdd(s.result.Notional, residualCost)
	if !ok {
		return errors.New("execution pilot: completion cost overflow")
	}
	targetDifference, ok := types.TrySub(completionCost, targetReference)
	if !ok {
		return errors.New("execution pilot: target price shortfall overflow")
	}
	s.result.TargetShortfall, ok = types.TryAdd(targetDifference, s.result.QuoteFees)
	if !ok {
		return errors.New("execution pilot: target all-in shortfall overflow")
	}
	s.result.TargetShortfallBps = float64(s.result.TargetShortfall) * 10000 / float64(targetReference)
	s.result.TargetShortfallDefined = true
	return nil
}
