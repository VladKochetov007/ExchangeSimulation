package repeatedspot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"slices"

	"exchange_sim/analysis"
	"exchange_sim/exchange"
	"exchange_sim/experiment/executionpilot"
	worldspot "exchange_sim/simulations/repeatedspot"
	"exchange_sim/types"
)

type replayContract struct {
	SchemaVersion                    int                        `json:"schema_version"`
	VenueID                          string                     `json:"venue_id"`
	Instrument                       worldspot.InstrumentConfig `json:"instrument"`
	StartUnixNano                    int64                      `json:"start_unix_nano"`
	Step                             int64                      `json:"step_ns"`
	Iterations                       int                        `json:"iterations"`
	ForbidBorrowing                  bool                       `json:"forbid_borrowing"`
	RecordSnapshotProjectionEvidence bool                       `json:"record_snapshot_projection_evidence"`
	Participants                     []replayParticipant        `json:"participants"`
}

type replayParticipant struct {
	ActorID  uint64           `json:"actor_id"`
	ClientID uint64           `json:"client_id"`
	Role     string           `json:"role"`
	Balances map[string]int64 `json:"balances"`
	Latency  struct {
		RequestNanos  int64  `json:"request_latency_ns"`
		ResponseNanos *int64 `json:"response_latency_ns"`
	} `json:"latency"`
	Policy struct {
		Name       string          `json:"name"`
		Parameters json.RawMessage `json:"parameters"`
	} `json:"policy"`
}

type AccountResult struct {
	ActorID        uint64                `json:"actor_id"`
	ClientID       uint64                `json:"client_id"`
	Role           string                `json:"role"`
	Initial        map[string]int64      `json:"initial"`
	Terminal       map[string]int64      `json:"terminal"`
	FeePaidQuote   int64                 `json:"fee_paid_quote"`
	FillCount      int                   `json:"fill_count"`
	BenchmarkGain  *int64                `json:"benchmark_gain_quote,omitempty"`
	Outbound       OutboundSummary       `json:"outbound"`
	LocalResponses *MakerResponseSummary `json:"maker_local_responses,omitempty"`
	InventoryRisk  AccountInventoryRisk  `json:"inventory_risk"`
	RestingDepth   RestingDepthSummary   `json:"resting_depth"`
	MakerEnvelope  *MakerEnvelopeSummary `json:"maker_envelope,omitempty"`
}

type EconomicReplay struct {
	ContractSHA256     string                           `json:"contract_sha256"`
	Evidence           EvidenceIdentity                 `json:"evidence"`
	Accounts           []AccountResult                  `json:"accounts"`
	VenueFeeRevenue    map[string]int64                 `json:"venue_fee_revenue"`
	TradeCount         int                              `json:"trade_count"`
	TerminalMarkStatus string                           `json:"terminal_mark_status"`
	TerminalMidQuote   *int64                           `json:"terminal_mid_quote,omitempty"`
	InformationAudit   *analysis.MarketDataReceiptAudit `json:"information_audit"`
	Market             MarketSummary                    `json:"market"`
	MeasurementWindow  MeasurementWindow                `json:"measurement_window"`
}

type accountState struct {
	actorID         uint64
	clientID        uint64
	role            string
	initial         map[string]int64
	current         map[string]int64
	feePaidQuote    int64
	fillCount       int
	startSeen       bool
	endSeen         bool
	workingLimit    int64
	requestDelay    int64
	responseDelay   int64
	maker           *makerParameters
	variance        deliveredVariance
	decisions       int64
	pendingSends    []*sentOrder
	outbound        OutboundSummary
	localResponses  MakerResponseSummary
	acceptedLocally map[uint64]bool
	exchangeRisk    *inventoryRiskSeries
	localRisk       *inventoryRiskSeries
	inventoryRisk   AccountInventoryRisk
	restingDepth    *restingDepthSeries
	restingSummary  RestingDepthSummary
	envelope        *makerEnvelopeReplay
	envelopeSummary *MakerEnvelopeSummary
}

type replayState struct {
	contract            replayContract
	contractHash        string
	phase               string
	accounts            map[uint64]*accountState
	venue               map[string]int64
	venueSequence       uint64
	lastTimestamp       int64
	terminalBook        bool
	terminalMid         *int64
	markStatus          string
	market              *publicBookSeries
	trades              *tradeAudit
	publicSnapshots     map[uint64]publicSnapshot
	makerObservations   []makerObservationRecord
	latestMakerSnapshot map[uint64]worldspot.MakerObservation
	sentRequests        map[requestKey]*sentOrder
	makerReceipts       map[makerReceiptKey]*makerReceipt
	measurementWindow   MeasurementWindow
	resting             *restingBook
}

type publicSnapshot struct {
	timestamp int64
	bids      []types.PriceLevel
	asks      []types.PriceLevel
}

type makerObservationRecord struct {
	clientID    uint64
	observation worldspot.MakerObservation
}

func Replay(contractBytes []byte, stream io.Reader, identity EvidenceIdentity, receiptDir string) (*EconomicReplay, error) {
	return replayWithWindow(contractBytes, stream, identity, receiptDir, nil)
}

// ReplayWindow verifies the full stream while measuring inventory exposure
// only inside the caller's prospectively declared observation window.
func ReplayWindow(contractBytes []byte, stream io.Reader, identity EvidenceIdentity, receiptDir string, window MeasurementWindow) (*EconomicReplay, error) {
	return replayWithWindow(contractBytes, stream, identity, receiptDir, &window)
}

func replayWithWindow(contractBytes []byte, stream io.Reader, identity EvidenceIdentity, receiptDir string, requested *MeasurementWindow) (*EconomicReplay, error) {
	if err := executionpilot.ValidateStrictJSON(contractBytes); err != nil {
		return nil, fmt.Errorf("repeated spot: invalid contract JSON: %w", err)
	}
	var contract replayContract
	if err := json.Unmarshal(contractBytes, &contract); err != nil {
		return nil, err
	}
	if contract.SchemaVersion != 2 || contract.VenueID == "" || contract.Instrument.Symbol == "" || !contract.RecordSnapshotProjectionEvidence ||
		contract.Instrument.BasePrecision <= 0 || contract.Instrument.QuotePrecision <= 0 ||
		contract.Instrument.TickSize <= 0 || contract.Instrument.MinOrderSize <= 0 ||
		contract.Instrument.BaseAsset == "" || contract.Instrument.QuoteAsset == "" ||
		contract.StartUnixNano < 0 || contract.Step <= 0 || contract.Iterations <= 0 || !contract.ForbidBorrowing ||
		len(contract.Participants) == 0 || int64(contract.Iterations) > (math.MaxInt64-contract.StartUnixNano)/contract.Step {
		return nil, errors.New("repeated spot: unsupported or invalid E0 contract")
	}
	terminalAt := contract.StartUnixNano + int64(contract.Iterations)*contract.Step
	window := MeasurementWindow{StartAt: contract.StartUnixNano, EndAt: terminalAt}
	if requested != nil {
		window = *requested
	}
	if window.StartAt < contract.StartUnixNano || window.EndAt > terminalAt || window.EndAt <= window.StartAt {
		return nil, errors.New("repeated spot: invalid measurement window")
	}
	hash := sha256.Sum256(contractBytes)
	state := &replayState{contract: contract, contractHash: hex.EncodeToString(hash[:]), phase: "before_begin",
		accounts: make(map[uint64]*accountState), venue: make(map[string]int64), trades: newTradeAudit(contract.Instrument),
		market:        newPublicBookSeries(contract.StartUnixNano, contract.Instrument.TickSize),
		sentRequests:  make(map[requestKey]*sentOrder),
		makerReceipts: make(map[makerReceiptKey]*makerReceipt), measurementWindow: window,
		resting:         &restingBook{orders: make(map[uint64]*restingOrder)},
		publicSnapshots: make(map[uint64]publicSnapshot), latestMakerSnapshot: make(map[uint64]worldspot.MakerObservation)}
	for _, participant := range contract.Participants {
		if participant.ClientID == 0 || participant.ActorID == 0 || participant.Role == "" ||
			participant.Latency.RequestNanos < 0 || participant.Latency.ResponseNanos == nil ||
			*participant.Latency.ResponseNanos < 0 ||
			state.accounts[participant.ClientID] != nil {
			return nil, errors.New("repeated spot: duplicate or incomplete participant identity")
		}
		initial := make(map[string]int64, len(participant.Balances))
		for asset, balance := range participant.Balances {
			if (asset != contract.Instrument.BaseAsset && asset != contract.Instrument.QuoteAsset) || balance < 0 {
				return nil, fmt.Errorf("repeated spot: invalid initial balance for client %d", participant.ClientID)
			}
			initial[asset] = balance
		}
		maker, err := parseMakerParameters(participant)
		if err != nil {
			return nil, fmt.Errorf("repeated spot: invalid maker parameters for client %d: %w", participant.ClientID, err)
		}
		workingLimit := int64(0)
		variance := deliveredVariance{}
		if maker != nil {
			workingLimit = maker.workingLimit
			variance.value = maker.initialVariance
		}
		state.accounts[participant.ClientID] = &accountState{actorID: participant.ActorID, clientID: participant.ClientID,
			role: participant.Role, initial: initial, current: copyBalances(initial), workingLimit: workingLimit,
			maker: maker, variance: variance, requestDelay: participant.Latency.RequestNanos,
			responseDelay:   *participant.Latency.ResponseNanos,
			acceptedLocally: make(map[uint64]bool),
			exchangeRisk:    newInventoryRiskSeries(contract.StartUnixNano, window, workingLimit),
			restingDepth:    newRestingDepthSeries(contract.StartUnixNano, window)}
		if maker != nil {
			state.accounts[participant.ClientID].localRisk = newInventoryRiskSeries(contract.StartUnixNano, window, workingLimit)
			state.accounts[participant.ClientID].envelope = newMakerEnvelopeReplay(contract.StartUnixNano, window, workingLimit)
		}
	}
	if err := WalkEvidence(stream, identity, state.visit); err != nil {
		return nil, err
	}
	if state.phase != "ended" || !state.terminalBook {
		return nil, errors.New("repeated spot: incomplete world evidence")
	}
	for _, account := range state.accounts {
		if !account.startSeen || !account.endSeen {
			return nil, fmt.Errorf("repeated spot: missing boundary account snapshot for client %d", account.clientID)
		}
		if account.maker != nil && account.decisions != int64(contract.Iterations)*contract.Step/account.maker.quoteIntervalNanos {
			return nil, fmt.Errorf("repeated spot: incomplete maker decision schedule for client %d", account.clientID)
		}
		if len(account.pendingSends) != 0 {
			return nil, fmt.Errorf("repeated spot: maker %d has unassigned outbound requests", account.clientID)
		}
	}
	if err := state.trades.finish(state); err != nil {
		return nil, err
	}
	if err := state.finishMakerResponses(); err != nil {
		return nil, err
	}
	if err := state.finishInventoryRisk(terminalAt); err != nil {
		return nil, err
	}
	if err := state.finishRestingDepth(terminalAt); err != nil {
		return nil, err
	}
	for _, account := range state.accounts {
		if account.envelope == nil {
			continue
		}
		summary, err := account.envelope.finish(terminalAt)
		if err != nil {
			return nil, fmt.Errorf("repeated spot: maker %d envelope: %w", account.clientID, err)
		}
		if account.envelope.filled != account.localResponses.NetProcessedFillBase {
			return nil, fmt.Errorf("repeated spot: maker %d envelope fill differs from local responses", account.clientID)
		}
		account.envelopeSummary = &summary
	}
	if err := state.checkConservation(); err != nil {
		return nil, err
	}
	info, err := analysis.AuditMarketDataReceipts(receiptDir)
	if err != nil || info == nil || !info.Valid || info.TerminalAt != state.lastTimestamp {
		return nil, fmt.Errorf("repeated spot: invalid information-boundary sidecar: %v", err)
	}
	if err := state.verifyMakerReceipts(receiptDir); err != nil {
		return nil, err
	}
	for key, sent := range state.sentRequests {
		if !sent.resolved {
			account := state.accounts[key.clientID]
			end := contract.StartUnixNano + int64(contract.Iterations)*contract.Step
			if account.requestDelay <= end-sent.at {
				return nil, fmt.Errorf("repeated spot: client %d request %d reached its venue window without outcome", key.clientID, key.requestID)
			}
			account.outbound.UnresolvedAtEnd++
		}
	}
	market := state.market.summary()
	tradeVolume, tradeNotional := state.trades.totals()
	market.TradeVolumeBaseUnits = tradeVolume
	market.TradeNotionalQuoteUnits = tradeNotional
	report := &EconomicReplay{ContractSHA256: state.contractHash, Evidence: identity,
		VenueFeeRevenue: copyBalances(state.venue), TradeCount: len(state.trades.trades),
		TerminalMarkStatus: state.markStatus, TerminalMidQuote: state.terminalMid, InformationAudit: info,
		Market: market, MeasurementWindow: window}
	ids := make([]uint64, 0, len(state.accounts))
	for clientID := range state.accounts {
		ids = append(ids, clientID)
	}
	slices.Sort(ids)
	for _, clientID := range ids {
		account := state.accounts[clientID]
		result := AccountResult{ActorID: account.actorID, ClientID: clientID, Role: account.role,
			Initial: copyBalances(account.initial), Terminal: copyBalances(account.current),
			FeePaidQuote: account.feePaidQuote, FillCount: account.fillCount, Outbound: account.outbound,
		}
		if account.maker != nil {
			result.LocalResponses = &account.localResponses
			result.MakerEnvelope = account.envelopeSummary
		}
		result.InventoryRisk = account.inventoryRisk
		result.RestingDepth = account.restingSummary
		if state.terminalMid != nil {
			gain, err := benchmarkGain(account, contract.Instrument, *state.terminalMid)
			if err != nil {
				return nil, err
			}
			result.BenchmarkGain = &gain
		}
		report.Accounts = append(report.Accounts, result)
	}
	return report, nil
}

func (state *replayState) visit(event Event) error {
	if event.Timestamp < state.lastTimestamp || event.Timestamp < state.contract.StartUnixNano {
		return errors.New("repeated spot: event timestamp regressed")
	}
	state.lastTimestamp = event.Timestamp
	if event.Source == "control" {
		return state.control(event)
	}
	if state.phase == "opening_snapshots" || state.phase == "closing_snapshots" {
		if event.Source != "exchange" || event.Name != "balance_snapshot" || event.Route != "_global" {
			return fmt.Errorf("repeated spot: unexpected %s/%s at account boundary", event.Source, event.Name)
		}
		return state.snapshot(event)
	}
	if state.phase != "running" {
		return fmt.Errorf("repeated spot: event %s/%s outside world run", event.Source, event.Name)
	}
	if event.Source == "actor" && event.Name == "maker_decision" {
		return state.makerDecision(event)
	}
	if event.Source == "actor" && event.Name == "maker_observation" {
		return state.makerObservation(event)
	}
	if event.Source == "gateway" && event.Name == "maker_response_receipt" {
		return state.makerResponseReceipt(event)
	}
	if event.Source == "actor" && event.Name == "maker_processed_response" {
		return state.makerProcessedResponse(event)
	}
	if event.Source == "actor" && event.Name == "order_send" {
		return state.orderSend(event)
	}
	if event.Source != "exchange" || event.Route != state.contract.Instrument.Symbol && event.Route != "_global" {
		return fmt.Errorf("repeated spot: unexpected event source/route %s/%s", event.Source, event.Route)
	}
	switch event.Name {
	case "balance_change":
		return state.balanceChange(event)
	case "venue_balance_change":
		return state.venueChange(event)
	case "balance_snapshot":
		return errors.New("repeated spot: unregistered mid-run balance snapshot")
	case "BookSnapshot", "BookDelta", "OrderAccepted", "OrderRejected", "OrderCancelled", "OrderCancelRejected", "Trade", "OrderFill", "fee_revenue":
		if event.Name == "BookSnapshot" {
			if err := state.publicSnapshot(event); err != nil {
				return err
			}
		} else if event.Name == "BookDelta" {
			if err := state.publicDelta(event); err != nil {
				return err
			}
		}
		if err := state.checkOrderOutcome(event); err != nil {
			return err
		}
		if err := state.trades.visit(event); err != nil {
			return err
		}
		if event.Name == "OrderFill" {
			if err := state.exchangeInventoryFill(event); err != nil {
				return err
			}
		}
		return state.restingOrderEvent(event)
	default:
		return fmt.Errorf("repeated spot: unsupported exchange event %q", event.Name)
	}
}

func (state *replayState) control(event Event) error {
	if event.ClientID != 0 || event.Route != "" {
		return errors.New("repeated spot: control event has client or route")
	}
	switch event.Name {
	case "world_begin":
		if state.phase != "before_begin" || event.Timestamp != state.contract.StartUnixNano {
			return errors.New("repeated spot: duplicate or mistimed world begin")
		}
		var payload struct {
			ContractSHA256 string `json:"contract_sha256"`
		}
		if err := decodePayload(event.Payload, &payload); err != nil || payload.ContractSHA256 != state.contractHash {
			return errors.New("repeated spot: world begin contract digest mismatch")
		}
		state.phase = "opening_snapshots"
	case "world_run_start":
		if state.phase != "opening_snapshots" || event.Timestamp != state.contract.StartUnixNano {
			return errors.New("repeated spot: invalid world-run start")
		}
		for _, account := range state.accounts {
			if !account.startSeen {
				return errors.New("repeated spot: missing initial account snapshot")
			}
		}
		state.phase = "running"
	case "world_run_end":
		if state.phase != "running" || event.Timestamp != state.contract.StartUnixNano+int64(state.contract.Iterations)*state.contract.Step {
			return errors.New("repeated spot: incomplete or mistimed world run")
		}
		state.phase = "closing_snapshots"
	case "terminal_book":
		if state.phase != "closing_snapshots" || state.terminalBook ||
			event.Timestamp != state.contract.StartUnixNano+int64(state.contract.Iterations)*state.contract.Step {
			return errors.New("repeated spot: duplicate or misplaced terminal book")
		}
		for _, account := range state.accounts {
			if !account.endSeen {
				return errors.New("repeated spot: terminal account snapshot missing before book")
			}
		}
		if err := state.readTerminalBook(event.Timestamp, event.Payload); err != nil {
			return err
		}
		state.terminalBook = true
	case "world_end":
		if state.phase != "closing_snapshots" || !state.terminalBook {
			return errors.New("repeated spot: invalid world completion")
		}
		state.phase = "ended"
	default:
		return fmt.Errorf("repeated spot: incomplete or unknown control event %q", event.Name)
	}
	return nil
}

func (state *replayState) snapshot(event Event) error {
	var snapshot exchange.BalanceSnapshot
	if err := decodePayload(event.Payload, &snapshot); err != nil {
		return err
	}
	account := state.accounts[event.ClientID]
	if account == nil || snapshot.ClientID != event.ClientID || snapshot.Timestamp != event.Timestamp ||
		len(snapshot.PerpBalances) != 0 || len(snapshot.Borrowed) != 0 {
		return errors.New("repeated spot: malformed one-book account snapshot")
	}
	if state.phase == "opening_snapshots" {
		if account.startSeen || event.Timestamp != state.contract.StartUnixNano {
			return errors.New("repeated spot: duplicate or mistimed initial account snapshot")
		}
		account.startSeen = true
	} else {
		if account.endSeen {
			return errors.New("repeated spot: duplicate terminal account snapshot")
		}
		account.endSeen = true
	}
	seen := make(map[string]bool)
	for _, balance := range snapshot.SpotBalances {
		if seen[balance.Asset] || balance.Free < 0 || balance.Locked < 0 || balance.Borrowed != 0 || balance.Interest != 0 ||
			(balance.Asset != state.contract.Instrument.BaseAsset && balance.Asset != state.contract.Instrument.QuoteAsset) {
			return errors.New("repeated spot: invalid spot balance snapshot row")
		}
		seen[balance.Asset] = true
		total, ok := checkedAdd(balance.Free, balance.Locked)
		if !ok || total != account.current[balance.Asset] || balance.NetAsset != total {
			return errors.New("repeated spot: account snapshot disagrees with independent ledger")
		}
	}
	for asset := range account.current {
		if !seen[asset] {
			return errors.New("repeated spot: account snapshot omits an endowed asset")
		}
	}
	return nil
}

func (state *replayState) balanceChange(event Event) error {
	var change exchange.BalanceChangeEvent
	if err := decodePayload(event.Payload, &change); err != nil {
		return err
	}
	account := state.accounts[event.ClientID]
	if account == nil || event.Route != state.contract.Instrument.Symbol || change.ClientID != event.ClientID ||
		change.Timestamp != event.Timestamp || change.Symbol != state.contract.Instrument.Symbol ||
		change.Reason != "trade_settlement" || len(change.Changes) != 2 {
		return errors.New("repeated spot: unsupported or malformed balance change")
	}
	seen := make(map[string]bool)
	for _, delta := range change.Changes {
		if delta.Wallet != "spot" || seen[delta.Asset] ||
			(delta.Asset != state.contract.Instrument.BaseAsset && delta.Asset != state.contract.Instrument.QuoteAsset) ||
			delta.OldBalance != account.current[delta.Asset] || delta.NewBalance < 0 {
			return errors.New("repeated spot: invalid spot settlement ledger row")
		}
		computed, ok := checkedAdd(delta.OldBalance, delta.Delta)
		if !ok || computed != delta.NewBalance {
			return errors.New("repeated spot: inconsistent balance delta")
		}
		seen[delta.Asset] = true
		account.current[delta.Asset] = delta.NewBalance
	}
	if !seen[state.contract.Instrument.BaseAsset] || !seen[state.contract.Instrument.QuoteAsset] {
		return errors.New("repeated spot: settlement omitted base or quote")
	}
	return state.trades.settlement(event, change)
}

func (state *replayState) venueChange(event Event) error {
	var change exchange.VenueBalanceEvent
	if err := decodePayload(event.Payload, &change); err != nil {
		return err
	}
	if event.ClientID != 0 || event.Route != state.contract.Instrument.Symbol || change.Timestamp != event.Timestamp ||
		change.Symbol != state.contract.Instrument.Symbol || change.Bucket != exchange.VenueFeeRevenue ||
		change.Asset != state.contract.Instrument.QuoteAsset ||
		change.Sequence != state.venueSequence+1 || change.OldBalance != state.venue[change.Asset] {
		return errors.New("repeated spot: invalid or reordered venue fee ledger")
	}
	updated, ok := checkedAdd(change.OldBalance, change.Delta)
	if !ok || updated != change.NewBalance {
		return errors.New("repeated spot: venue fee delta disagrees with balance")
	}
	state.venueSequence = change.Sequence
	state.venue[change.Asset] = updated
	return state.trades.venueFee(change)
}

func (state *replayState) makerDecision(event Event) error {
	var decision worldspot.MakerDecision
	if err := decodePayload(event.Payload, &decision); err != nil {
		return err
	}
	account := state.accounts[event.ClientID]
	if account == nil || decision.ActorID != account.actorID || decision.DecisionAt != event.Timestamp ||
		decision.ScheduledAt > decision.DecisionAt || decision.ScheduledAt < state.contract.StartUnixNano ||
		decision.BookSeen && decision.LatestBookSourceAt > decision.DecisionAt ||
		decision.BookSeen && decision.LatestBookProcessedAt > decision.DecisionAt ||
		decision.WorkingLower > decision.FilledInventory || decision.WorkingUpper < decision.FilledInventory ||
		decision.BestBid < 0 || decision.BestAsk < 0 || account.workingLimit <= 0 ||
		decision.WorkingLower < -account.workingLimit || decision.WorkingUpper > account.workingLimit ||
		!validMakerAction(decision.Action) {
		return errors.New("repeated spot: malformed or future-dated maker decision")
	}
	horizon := int64(state.contract.Iterations) * state.contract.Step
	if account.decisions >= horizon/account.maker.quoteIntervalNanos {
		return errors.New("repeated spot: extra maker decision outside registered schedule")
	}
	expectedScheduled := state.contract.StartUnixNano + (account.decisions+1)*account.maker.quoteIntervalNanos
	if decision.ScheduledAt != expectedScheduled {
		return errors.New("repeated spot: missing, duplicate or mistimed maker decision")
	}
	latest, seen := state.latestMakerSnapshot[event.ClientID]
	if decision.BookSeen != seen || seen && (decision.LatestBookSourceAt != latest.SourceAt ||
		decision.LatestBookSequence != latest.SourceSequence || decision.LatestBookProcessedAt != latest.ProcessedAt ||
		decision.BestBid != latest.BestBid || decision.BestAsk != latest.BestAsk) {
		return errors.New("repeated spot: maker decision disagrees with its processed local snapshot")
	}
	if err := account.variance.verifyDecision(decision); err != nil {
		return err
	}
	if decision.FilledInventory != account.localResponses.NetProcessedFillBase {
		return errors.New("repeated spot: maker decision used inventory absent from processed fill responses")
	}
	if err := account.envelope.decision(event.Timestamp, decision, account.pendingSends, account.maker); err != nil {
		return err
	}
	switch decision.Action {
	case "keep_quotes", "cancel_quotes", "evaluate_placements", "no_usable_quote":
		bid, ask, usable := expectedMakerQuote(account.maker, decision)
		if (decision.Action == "no_usable_quote" && usable) ||
			(decision.Action == "keep_quotes" || decision.Action == "evaluate_placements") && !usable ||
			decision.TargetBid != bid || decision.TargetAsk != ask {
			return fmt.Errorf("repeated spot: maker target quote differs from independent policy calculation: client=%d at=%d action=%s book=%d/%d filled=%d target=%d/%d expected=%d/%d usable=%t",
				event.ClientID, event.Timestamp, decision.Action, decision.BestBid, decision.BestAsk,
				decision.FilledInventory, decision.TargetBid, decision.TargetAsk, bid, ask, usable)
		}
	case "subscribe", "await_response":
		if decision.TargetBid != 0 || decision.TargetAsk != 0 {
			return errors.New("repeated spot: maker reported a target while no quote was calculated")
		}
	}
	if err := account.verifyMakerSends(decision, event.Timestamp); err != nil {
		return err
	}
	account.decisions++
	return nil
}

func (state *replayState) makerObservation(event Event) error {
	var observation worldspot.MakerObservation
	if err := decodePayload(event.Payload, &observation); err != nil {
		return err
	}
	account := state.accounts[event.ClientID]
	if account == nil || account.workingLimit <= 0 || observation.ActorID != account.actorID ||
		observation.Symbol != state.contract.Instrument.Symbol || observation.ProcessedAt != event.Timestamp ||
		observation.SourceAt > observation.ProcessedAt ||
		(observation.Kind != "snapshot" && observation.Kind != "trade") {
		return errors.New("repeated spot: malformed or future-dated maker observation")
	}
	if observation.Kind == "snapshot" {
		source, exists := state.publicSnapshots[observation.SourceSequence]
		if !exists || source.timestamp != observation.SourceAt ||
			observation.BestBid != firstLevelPrice(source.bids) || observation.BestAsk != firstLevelPrice(source.asks) {
			return errors.New("repeated spot: maker snapshot disagrees with published public book")
		}
		state.latestMakerSnapshot[event.ClientID] = observation
	} else {
		trade := state.trades.trades[observation.TradeID]
		if trade == nil || trade.timestamp != observation.SourceAt ||
			trade.trade.Price != observation.TradePrice || trade.trade.Qty != observation.TradeQty ||
			trade.trade.Side != observation.TradeSide {
			return errors.New("repeated spot: maker trade observation disagrees with exchange trade")
		}
		if err := account.variance.observe(account.maker, observation); err != nil {
			return err
		}
	}
	state.makerObservations = append(state.makerObservations, makerObservationRecord{event.ClientID, observation})
	return nil
}

func (state *replayState) publicSnapshot(event Event) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &fields); err != nil {
		return err
	}
	for _, required := range []string{"source_sequence", "public_bids", "public_asks"} {
		if _, present := fields[required]; !present {
			return fmt.Errorf("repeated spot: public snapshot omitted %s", required)
		}
	}
	var snapshot struct {
		SourceSequence uint64             `json:"source_sequence"`
		PublicBids     []types.PriceLevel `json:"public_bids"`
		PublicAsks     []types.PriceLevel `json:"public_asks"`
	}
	if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
		return errors.New("repeated spot: missing public snapshot projection")
	}
	_, duplicate := state.publicSnapshots[snapshot.SourceSequence]
	if duplicate {
		return errors.New("repeated spot: missing or duplicate public snapshot projection")
	}
	if err := state.market.verifySnapshot(event.Timestamp, snapshot.PublicBids, snapshot.PublicAsks); err != nil {
		return err
	}
	if err := state.resting.verifySnapshot(snapshot.PublicBids, snapshot.PublicAsks); err != nil {
		return err
	}
	state.publicSnapshots[snapshot.SourceSequence] = publicSnapshot{event.Timestamp, snapshot.PublicBids, snapshot.PublicAsks}
	return nil
}

func (state *replayState) publicDelta(event Event) error {
	var delta struct {
		Side       string `json:"side"`
		Price      int64  `json:"price"`
		VisibleQty int64  `json:"visible_qty"`
		HiddenQty  int64  `json:"hidden_qty"`
		TotalQty   int64  `json:"total_qty"`
	}
	if err := decodePayload(event.Payload, &delta); err != nil {
		return err
	}
	return state.market.delta(event.Timestamp, delta.Side, delta.Price,
		delta.VisibleQty, delta.HiddenQty, delta.TotalQty)
}

func firstLevelPrice(levels []types.PriceLevel) int64 {
	if len(levels) == 0 {
		return 0
	}
	return levels[0].Price
}

func validMakerAction(action string) bool {
	switch action {
	case "fault", "subscribe", "await_response", "keep_quotes", "cancel_quotes", "evaluate_placements", "no_usable_quote":
		return true
	default:
		return false
	}
}

func (state *replayState) readTerminalBook(at int64, payload json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return err
	}
	for _, required := range []string{"symbol", "bids", "asks"} {
		if _, present := fields[required]; !present {
			return fmt.Errorf("repeated spot: terminal book omitted %s", required)
		}
	}
	var book struct {
		Symbol string                `json:"symbol"`
		Bids   []exchange.PriceLevel `json:"bids"`
		Asks   []exchange.PriceLevel `json:"asks"`
	}
	if err := decodePayload(payload, &book); err != nil {
		return err
	}
	if book.Symbol != state.contract.Instrument.Symbol {
		return errors.New("repeated spot: terminal book symbol mismatch")
	}
	if err := state.market.verifyTerminal(at, book.Bids, book.Asks); err != nil {
		return err
	}
	if err := state.resting.verifySnapshot(book.Bids, book.Asks); err != nil {
		return err
	}
	for index, level := range book.Bids {
		if level.Price <= 0 || level.VisibleQty <= 0 || level.HiddenQty < 0 || index > 0 && level.Price >= book.Bids[index-1].Price {
			return errors.New("repeated spot: invalid terminal bid levels")
		}
	}
	for index, level := range book.Asks {
		if level.Price <= 0 || level.VisibleQty <= 0 || level.HiddenQty < 0 || index > 0 && level.Price <= book.Asks[index-1].Price {
			return errors.New("repeated spot: invalid terminal ask levels")
		}
	}
	state.markStatus = "UNAVAILABLE_ONE_SIDED_OR_EMPTY"
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return nil
	}
	if book.Bids[0].Price >= book.Asks[0].Price {
		return errors.New("repeated spot: crossed terminal spot book")
	}
	state.markStatus = "AVAILABLE_TWO_SIDED_MID"
	mid := book.Bids[0].Price + (book.Asks[0].Price-book.Bids[0].Price)/2
	state.terminalMid = &mid
	return nil
}

func (state *replayState) checkConservation() error {
	for _, asset := range []string{state.contract.Instrument.BaseAsset, state.contract.Instrument.QuoteAsset} {
		initial := big.NewInt(0)
		terminal := big.NewInt(state.venue[asset])
		for _, account := range state.accounts {
			initial.Add(initial, big.NewInt(account.initial[asset]))
			terminal.Add(terminal, big.NewInt(account.current[asset]))
		}
		if initial.Cmp(terminal) != 0 {
			return fmt.Errorf("repeated spot: %s asset ledger does not conserve with venue fees", asset)
		}
	}
	return nil
}

func benchmarkGain(account *accountState, instrument worldspot.InstrumentConfig, mark int64) (int64, error) {
	baseChange := new(big.Int).Sub(big.NewInt(account.current[instrument.BaseAsset]), big.NewInt(account.initial[instrument.BaseAsset]))
	baseValue := new(big.Int).Mul(baseChange, big.NewInt(mark))
	baseValue.Quo(baseValue, big.NewInt(instrument.BasePrecision))
	cashChange := new(big.Int).Sub(big.NewInt(account.current[instrument.QuoteAsset]), big.NewInt(account.initial[instrument.QuoteAsset]))
	result := new(big.Int).Add(baseValue, cashChange)
	if !result.IsInt64() {
		return 0, errors.New("repeated spot: benchmark gain overflows int64")
	}
	return result.Int64(), nil
}

func checkedAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func copyBalances(original map[string]int64) map[string]int64 {
	copy := make(map[string]int64, len(original))
	for asset, balance := range original {
		copy[asset] = balance
	}
	return copy
}

func decodePayload(raw json.RawMessage, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("repeated spot: trailing event payload content")
	}
	return nil
}
