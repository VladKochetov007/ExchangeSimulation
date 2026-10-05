package exchange

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"

	"exchange_sim/instrument"
)

type FundingSettlementStatus string

const (
	FundingSettlementPosted          FundingSettlementStatus = "POSTED"
	FundingSettlementRateUnavailable FundingSettlementStatus = "RATE_UNAVAILABLE"
	FundingSettlementPayerShortfall  FundingSettlementStatus = "FUNDING_PAYER_CASH_SHORTFALL"
)

type FundingSettlementAttemptRequest struct {
	Exchange           *DefaultExchange
	AccountRequest     FundingAccountSnapshotRequest
	SourceWindow       FundingSourceWindow
	CurrentObservation FundingSourceRecord
}

// FundingSettlementRegistration is one perpetual whose absolute settlement
// calendar is part of the frozen venue deployment. The registration list must
// be complete for every venue/perpetual covered by the epoch coordinator.
type FundingSettlementRegistration struct {
	Exchange   *DefaultExchange
	PerpSymbol string
}

type FundingSettlementAccount struct {
	ClientID      uint64                       `json:"client_id"`
	NetPosition   int64                        `json:"net_position"`
	CashBefore    int64                        `json:"cash_before"`
	CashDelta     int64                        `json:"cash_delta"`
	CashAfter     int64                        `json:"cash_after"`
	AccrualBefore ScaledFundingAccrualSnapshot `json:"accrual_before"`
	AccrualAfter  ScaledFundingAccrualSnapshot `json:"accrual_after"`
}

type FundingSettlementEvidence struct {
	Version                 uint16                                 `json:"version"`
	VenueID                 string                                 `json:"venue_id"`
	SpotSymbol              string                                 `json:"spot_symbol"`
	PerpSymbol              string                                 `json:"perp_symbol"`
	TimestampNano           int64                                  `json:"timestamp_nano"`
	Calendar                instrument.FundingCalendar             `json:"calendar"`
	RateContract            instrument.WindowedFundingRateContract `json:"rate_contract"`
	Status                  FundingSettlementStatus                `json:"status"`
	SourceEventSequences    []uint64                               `json:"source_event_sequences"`
	FrontierEventSequence   uint64                                 `json:"frontier_event_sequence"`
	UnavailableObservations []FundingBookObservation               `json:"unavailable_observations,omitempty"`
	RateSignedUnits         int64                                  `json:"rate_signed_units,omitempty"`
	RateUnitsPerBp          int64                                  `json:"rate_units_per_bp,omitempty"`
	NotionalMarkPrice       int64                                  `json:"notional_mark_price,omitempty"`
	Accounts                []FundingSettlementAccount             `json:"accounts"`
	RoundingReserveBefore   int64                                  `json:"rounding_reserve_before"`
	RoundingReserveDelta    int64                                  `json:"rounding_reserve_delta"`
	RoundingReserveAfter    int64                                  `json:"rounding_reserve_after"`
	PayerShortfallClientIDs []uint64                               `json:"payer_shortfall_client_ids,omitempty"`
}

type FundingSettlementAppendInput struct {
	Evidence        FundingSettlementEvidence
	BalanceChanges  []BalanceChangeEvent
	ReserveMovement *VenueBalanceEvent
}

type FundingSettlementReceipt struct {
	AttemptEventSequence    uint64
	BalanceEventSequences   []uint64
	ReserveMovementEventSeq uint64
	OutcomeEventSequence    uint64
}

// FundingSettlementEpochAppender appends every prepared venue record in input
// order under one canonical-writer boundary. A partial append must poison the
// required evidence stream and return an error; the caller never posts cash
// unless the complete epoch has valid receipts.
type FundingSettlementEpochAppender interface {
	AppendFundingSettlementEpoch([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error)
}

type FundingSettlementAttemptResult struct {
	VenueID                 string
	PerpSymbol              string
	TimestampNano           int64
	Status                  FundingSettlementStatus
	SourceEventSequences    []uint64
	FrontierEventSequence   uint64
	Terms                   *instrument.FundingSettlementTerms
	PayerShortfallClientIDs []uint64
	Preview                 FundingBatchPreview
	Receipt                 FundingSettlementReceipt
}

type FundingSettlementEpochResult struct {
	TimestampNano int64
	Attempts      []FundingSettlementAttemptResult
	Terminal      []FundingSettlementTerminalReason
}

type FundingSettlementTerminalReason struct {
	VenueID    string
	PerpSymbol string
	Reason     string
	ClientIDs  []uint64
}

type fundingSettlementPlan struct {
	request      FundingSettlementAttemptRequest
	cycle        int64
	input        FundingBatchInput
	result       FundingSettlementAttemptResult
	appendInput  FundingSettlementAppendInput
	reserveEvent *VenueBalanceEvent
}

// SettleFundingEpoch is called only by the deterministic runner's exclusive
// pre-instant phase. registrations is the frozen, complete set of venue-local
// perpetual calendars, including calendars that are not due at timestamp. The
// coordinator requires exactly the due request set, so a caller cannot silently
// omit one side of a coincident epoch. It also requires every scheduled attempt
// in sequence; a skipped interval is not converted into a later catch-up post.
// A source or arithmetic defect aborts the complete due epoch. Economically
// unavailable windows and payer shortfalls remain venue-local outcomes; a
// solvent coincident venue can still post.
func SettleFundingEpoch(registrations []FundingSettlementRegistration,
	requests []FundingSettlementAttemptRequest, appender FundingSettlementEpochAppender) (FundingSettlementEpochResult, error) {
	if len(registrations) == 0 || len(requests) == 0 || appender == nil {
		return FundingSettlementEpochResult{}, fmt.Errorf("funding epoch: missing frozen calendars, due attempts or required evidence appender")
	}
	orderedRegistrations := slices.Clone(registrations)
	slices.SortFunc(orderedRegistrations, compareFundingSettlementRegistration)
	orderedRequests := slices.Clone(requests)
	slices.SortFunc(orderedRequests, compareFundingSettlementAttempt)
	timestamp := orderedRequests[0].AccountRequest.TimestampNano
	for _, request := range orderedRequests {
		if request.Exchange == nil || request.AccountRequest.TimestampNano != timestamp ||
			request.AccountRequest.VenueID == "" || request.AccountRequest.PerpSymbol == "" ||
			request.AccountRequest.ExpectedClientIDs == nil {
			return FundingSettlementEpochResult{}, fmt.Errorf("funding epoch: incomplete or non-coincident venue request")
		}
	}
	exchanges, err := uniqueFundingSettlementExchanges(orderedRegistrations)
	if err != nil {
		return FundingSettlementEpochResult{}, err
	}
	for _, exchange := range exchanges {
		exchange.mu.Lock()
	}
	defer func() {
		for index := len(exchanges) - 1; index >= 0; index-- {
			exchanges[index].mu.Unlock()
		}
	}()

	_, err = validateFundingSettlementEpochRoster(orderedRegistrations, orderedRequests, timestamp)
	if err != nil {
		failFundingSettlementRegistrations(orderedRegistrations, err)
		return FundingSettlementEpochResult{}, err
	}

	plans := make([]fundingSettlementPlan, 0, len(orderedRequests))
	for _, request := range orderedRequests {
		plan, err := prepareFundingSettlementPlan(request)
		if err != nil {
			failFundingSettlementRegistrations(orderedRegistrations, err)
			return FundingSettlementEpochResult{}, err
		}
		plans = append(plans, plan)
	}

	sourceFrontier, err := fundingSettlementSourceFrontier(plans)
	if err != nil {
		failFundingSettlementRegistrations(orderedRegistrations, err)
		return FundingSettlementEpochResult{}, err
	}
	appendInputs := make([]FundingSettlementAppendInput, 0, len(plans))
	for _, plan := range plans {
		appendInputs = append(appendInputs, cloneFundingSettlementAppendInput(plan.appendInput))
	}
	receipts, err := appender.AppendFundingSettlementEpoch(appendInputs)
	if err != nil {
		failure := fmt.Errorf("required funding epoch evidence append failed: %w", err)
		failFundingSettlementRegistrations(orderedRegistrations, failure)
		return FundingSettlementEpochResult{}, failure
	}
	if len(receipts) != len(plans) {
		err = fmt.Errorf("funding epoch: evidence appender returned %d receipts for %d attempts", len(receipts), len(plans))
		failFundingSettlementRegistrations(orderedRegistrations, err)
		return FundingSettlementEpochResult{}, err
	}
	previousSequence := sourceFrontier
	for index := range plans {
		lastSequence, validationErr := validateFundingSettlementReceipt(&plans[index], receipts[index], previousSequence)
		if validationErr != nil {
			failFundingSettlementRegistrations(orderedRegistrations, validationErr)
			return FundingSettlementEpochResult{}, validationErr
		}
		plans[index].result.Receipt = cloneFundingSettlementReceipt(receipts[index])
		previousSequence = lastSequence
	}
	for _, exchange := range exchanges {
		if exchange.Clock == nil || exchange.Clock.NowUnixNano() != timestamp {
			err = fmt.Errorf("funding epoch: runner clock advanced during canonical append")
			failFundingSettlementRegistrations(orderedRegistrations, err)
			return FundingSettlementEpochResult{}, err
		}
	}

	epoch := FundingSettlementEpochResult{TimestampNano: timestamp,
		Attempts: make([]FundingSettlementAttemptResult, 0, len(plans))}
	for _, plan := range plans {
		owned := plan.request.Exchange.fundingStates[plan.request.AccountRequest.PerpSymbol]
		owned.lastAttemptNano, owned.lastAttemptCycle, owned.hasAttempt = timestamp, plan.cycle, true
		if plan.result.Status == FundingSettlementPosted {
			applyFundingSettlementPlan(&plan)
		}
		if plan.result.Status == FundingSettlementPayerShortfall {
			epoch.Terminal = append(epoch.Terminal, FundingSettlementTerminalReason{
				VenueID: plan.result.VenueID, PerpSymbol: plan.result.PerpSymbol,
				Reason:    string(FundingSettlementPayerShortfall),
				ClientIDs: slices.Clone(plan.result.PayerShortfallClientIDs),
			})
		}
		epoch.Attempts = append(epoch.Attempts, cloneFundingSettlementAttemptResult(plan.result))
	}
	return epoch, nil
}

type fundingSettlementKey struct {
	venueID    string
	perpSymbol string
}

func compareFundingSettlementRegistration(left, right FundingSettlementRegistration) int {
	leftVenue, rightVenue := "", ""
	if left.Exchange != nil {
		leftVenue = left.Exchange.ID
	}
	if right.Exchange != nil {
		rightVenue = right.Exchange.ID
	}
	if leftVenue < rightVenue {
		return -1
	}
	if leftVenue > rightVenue {
		return 1
	}
	if left.PerpSymbol < right.PerpSymbol {
		return -1
	}
	if left.PerpSymbol > right.PerpSymbol {
		return 1
	}
	return 0
}

func compareFundingSettlementAttempt(left, right FundingSettlementAttemptRequest) int {
	if left.AccountRequest.VenueID < right.AccountRequest.VenueID {
		return -1
	}
	if left.AccountRequest.VenueID > right.AccountRequest.VenueID {
		return 1
	}
	if left.AccountRequest.PerpSymbol < right.AccountRequest.PerpSymbol {
		return -1
	}
	if left.AccountRequest.PerpSymbol > right.AccountRequest.PerpSymbol {
		return 1
	}
	return 0
}

func uniqueFundingSettlementExchanges(registrations []FundingSettlementRegistration) ([]*DefaultExchange, error) {
	exchanges := make([]*DefaultExchange, 0, len(registrations))
	seenKeys := make(map[fundingSettlementKey]struct{}, len(registrations))
	venueExchange := make(map[string]*DefaultExchange, len(registrations))
	for _, registration := range registrations {
		if registration.Exchange == nil || registration.Exchange.ID == "" || registration.PerpSymbol == "" {
			return nil, fmt.Errorf("funding epoch: malformed frozen calendar registration")
		}
		key := fundingSettlementKey{venueID: registration.Exchange.ID, perpSymbol: registration.PerpSymbol}
		if _, exists := seenKeys[key]; exists {
			return nil, fmt.Errorf("funding epoch: duplicate frozen calendar registration %s/%s", key.venueID, key.perpSymbol)
		}
		seenKeys[key] = struct{}{}
		if existing, exists := venueExchange[key.venueID]; exists && existing != registration.Exchange {
			return nil, fmt.Errorf("funding epoch: venue %s is bound to multiple exchange instances", key.venueID)
		}
		venueExchange[key.venueID] = registration.Exchange
		if len(exchanges) == 0 || exchanges[len(exchanges)-1] != registration.Exchange {
			exchanges = append(exchanges, registration.Exchange)
		}
	}
	return exchanges, nil
}

func validateFundingSettlementEpochRoster(registrations []FundingSettlementRegistration,
	requests []FundingSettlementAttemptRequest, timestamp int64) ([]FundingSettlementRegistration, error) {
	due := make([]FundingSettlementRegistration, 0, len(registrations))
	registered := make(map[fundingSettlementKey]FundingSettlementRegistration, len(registrations))
	registeredSymbols := make(map[*DefaultExchange]map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		e := registration.Exchange
		if e.Clock == nil || e.Clock.NowUnixNano() != timestamp {
			return due, fmt.Errorf("funding epoch: registered venue clock is not at the common timestamp")
		}
		owned := e.fundingStates[registration.PerpSymbol]
		if owned == nil || owned.settlementFailure != nil {
			return due, fmt.Errorf("funding epoch: registered %s/%s settlement state is missing or failed", e.ID, registration.PerpSymbol)
		}
		_, isDue, err := nextFundingSettlementCycle(owned, timestamp)
		if err != nil {
			return due, err
		}
		if isDue {
			due = append(due, registration)
		}
		if err := owned.rateContract.Validate(owned.calendar.IntervalSeconds); err != nil ||
			owned.rateContract.RateUnitsPerBp != owned.rateUnitsPerBp {
			return due, fmt.Errorf("funding epoch: registered rate contract is invalid or differs from its reserve endowment")
		}
		registered[fundingSettlementKey{venueID: e.ID, perpSymbol: registration.PerpSymbol}] = registration
		if registeredSymbols[e] == nil {
			registeredSymbols[e] = make(map[string]struct{})
		}
		registeredSymbols[e][registration.PerpSymbol] = struct{}{}
	}
	for exchange, symbols := range registeredSymbols {
		for symbol := range exchange.fundingStates {
			if _, present := symbols[symbol]; !present {
				return due, fmt.Errorf("funding epoch: frozen registration omits an exchange-owned perpetual calendar")
			}
		}
	}
	if len(due) == 0 || len(requests) != len(due) {
		return due, fmt.Errorf("funding epoch: request set omits or adds a scheduled venue attempt")
	}
	for index, request := range requests {
		key := fundingSettlementKey{venueID: request.AccountRequest.VenueID, perpSymbol: request.AccountRequest.PerpSymbol}
		registration, exists := registered[key]
		if !exists || registration.Exchange != request.Exchange {
			return due, fmt.Errorf("funding epoch: request does not match frozen venue/perpetual registration")
		}
		if _, isDue := fundingSettlementRegistrationIn(due, key); !isDue {
			return due, fmt.Errorf("funding epoch: request includes a venue attempt that is not due")
		}
		if index > 0 && compareFundingSettlementAttempt(requests[index-1], request) >= 0 {
			return due, fmt.Errorf("funding epoch: duplicate or unstable venue request ordering")
		}
	}
	return due, nil
}

func fundingSettlementRegistrationIn(registrations []FundingSettlementRegistration, key fundingSettlementKey) (FundingSettlementRegistration, bool) {
	for _, registration := range registrations {
		if registration.Exchange.ID == key.venueID && registration.PerpSymbol == key.perpSymbol {
			return registration, true
		}
	}
	return FundingSettlementRegistration{}, false
}

func nextFundingSettlementCycle(owned *fundingOwnedState, timestamp int64) (int64, bool, error) {
	cycle := int64(1)
	if owned.hasAttempt {
		if owned.lastAttemptCycle <= 0 || owned.lastAttemptCycle == math.MaxInt64 {
			return 0, false, fmt.Errorf("funding epoch: settlement cycle cursor is invalid or exhausted")
		}
		cycle = owned.lastAttemptCycle + 1
	}
	expected, err := owned.calendar.ScheduledAt(cycle)
	if err != nil {
		return 0, false, err
	}
	if timestamp > expected {
		return cycle, false, fmt.Errorf("funding epoch: scheduled cycle %d at %d was skipped before %d", cycle, expected, timestamp)
	}
	return cycle, timestamp == expected, nil
}

func failFundingSettlementRegistrations(registrations []FundingSettlementRegistration, failure error) {
	for _, registration := range registrations {
		if registration.Exchange == nil {
			continue
		}
		if owned := registration.Exchange.fundingStates[registration.PerpSymbol]; owned != nil {
			owned.settlementFailure = failure
		}
	}
}

func prepareFundingSettlementPlan(request FundingSettlementAttemptRequest) (fundingSettlementPlan, error) {
	e := request.Exchange
	requestAt := request.AccountRequest.TimestampNano
	if e.Clock == nil || e.ID != request.AccountRequest.VenueID || e.Clock.NowUnixNano() != requestAt {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: venue %q clock is not at the requested t-minus frontier", request.AccountRequest.VenueID)
	}
	owned := e.fundingStates[request.AccountRequest.PerpSymbol]
	if owned == nil || owned.settlementFailure != nil || owned.hasAttempt && requestAt <= owned.lastAttemptNano {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: missing, failed or repeated owned settlement state at %d", requestAt)
	}
	cycle, isDue, err := nextFundingSettlementCycle(owned, requestAt)
	if err != nil || !isDue {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: timestamp %d is not the next scheduled instant for venue %q", requestAt, request.AccountRequest.VenueID)
	}
	if err := owned.rateContract.Validate(owned.calendar.IntervalSeconds); err != nil ||
		owned.rateContract.RateUnitsPerBp != owned.rateUnitsPerBp {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: owned rate contract is invalid or differs from its endowment")
	}
	samples, unavailable, sourceSequences, err := validateFundingSettlementWindow(request, owned.spotSymbol, owned.rateContract)
	if err != nil {
		return fundingSettlementPlan{}, err
	}
	if e.ExchangeBalance == nil {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: venue balance ledger is unavailable")
	}
	reserve, hasReserve := e.ExchangeBalance.FundingRoundingReserves[request.AccountRequest.PerpSymbol]
	if !hasReserve || reserve.SourceID != FundingReserveEndowmentSource ||
		reserve.EndowmentEventSeq == 0 || reserve.EndowmentMovementEventSeq <= reserve.EndowmentEventSeq {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: finite reserve endowment is missing or malformed")
	}
	if len(sourceSequences) == 0 || sourceSequences[0] <= reserve.EndowmentMovementEventSeq {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: source window predates its finite reserve endowment")
	}
	input, err := e.captureOwnedFundingBatchStateLocked(request.AccountRequest, owned.rateUnitsPerBp)
	if err != nil {
		return fundingSettlementPlan{}, fmt.Errorf("funding epoch: capture %s accounts: %w", request.AccountRequest.VenueID, err)
	}
	status := FundingSettlementRateUnavailable
	terms := input.Terms
	if len(unavailable) == 0 {
		terms, err = owned.rateContract.SettlementTermsAt(requestAt, owned.calendar.IntervalSeconds, samples)
		if err != nil {
			return fundingSettlementPlan{}, fmt.Errorf("funding epoch: derive %s terms: %w", request.AccountRequest.VenueID, err)
		}
		input.Terms = terms
		status = FundingSettlementPosted
	}
	result := FundingSettlementAttemptResult{
		VenueID: input.VenueID, PerpSymbol: input.Symbol, TimestampNano: requestAt,
		Status: status, SourceEventSequences: slices.Clone(sourceSequences),
		FrontierEventSequence: request.CurrentObservation.EventSeq,
	}
	if status == FundingSettlementPosted {
		result.Terms = &terms
		preview, previewErr := PreviewFundingBatch(input)
		if previewErr != nil {
			var shortfall *FundingPayerCashShortfall
			if !errors.As(previewErr, &shortfall) {
				return fundingSettlementPlan{}, fmt.Errorf("funding epoch: preflight %s batch: %w", input.VenueID, previewErr)
			}
			status = FundingSettlementPayerShortfall
			result.Status = status
			result.PayerShortfallClientIDs = slices.Clone(shortfall.ClientIDs)
		} else {
			result.Preview = preview
		}
	}
	evidence := buildFundingSettlementEvidence(request, input, result, terms, unavailable,
		owned.spotSymbol, owned.calendar, owned.rateContract)
	appendInput, reserveMovement, err := buildFundingSettlementAppendInput(e, evidence, result)
	if err != nil {
		return fundingSettlementPlan{}, err
	}
	return fundingSettlementPlan{request: request, cycle: cycle, input: input, result: result,
		appendInput: appendInput, reserveEvent: reserveMovement}, nil
}

func validateFundingSettlementWindow(request FundingSettlementAttemptRequest, spotSymbol string,
	contract instrument.WindowedFundingRateContract) ([]instrument.FundingWindowSample, []FundingBookObservation, []uint64, error) {
	if contract.SampleCount <= 0 || contract.SampleSpacingNano <= 0 ||
		len(request.SourceWindow.Sources) != contract.SampleCount ||
		request.CurrentObservation.EventSeq == 0 {
		return nil, nil, nil, fmt.Errorf("funding epoch: incomplete prior source window or current boundary identity")
	}
	priorSpan := new(big.Int).Mul(big.NewInt(int64(contract.SampleCount)), big.NewInt(contract.SampleSpacingNano))
	start := new(big.Int).Sub(big.NewInt(request.AccountRequest.TimestampNano), priorSpan)
	if !start.IsInt64() || start.Sign() < 0 {
		return nil, nil, nil, fmt.Errorf("funding epoch: source-window start is outside the timestamp domain")
	}
	samples := make([]instrument.FundingWindowSample, 0, contract.SampleCount)
	unavailable := make([]FundingBookObservation, 0)
	sequences := make([]uint64, 0, contract.SampleCount)
	previousSequence := uint64(0)
	for index, record := range request.SourceWindow.Sources {
		at := new(big.Int).Add(start, new(big.Int).Mul(big.NewInt(int64(index)), big.NewInt(contract.SampleSpacingNano)))
		if !at.IsInt64() || record.EventSeq == 0 || record.EventSeq <= previousSequence ||
			record.Observation.TimestampNano != at.Int64() {
			return nil, nil, nil, fmt.Errorf("funding epoch: missing, reordered or mistimed source record %d", index)
		}
		if err := validateFundingSettlementObservation(record.Observation, request, spotSymbol, at.Int64()); err != nil {
			return nil, nil, nil, err
		}
		sequences = append(sequences, record.EventSeq)
		previousSequence = record.EventSeq
		if record.Observation.Available {
			samples = append(samples, record.Observation.Pair.RateWindowSample())
		} else {
			unavailable = append(unavailable, record.Observation)
		}
	}
	if request.CurrentObservation.EventSeq <= previousSequence ||
		request.CurrentObservation.Observation.TimestampNano != request.AccountRequest.TimestampNano {
		return nil, nil, nil, fmt.Errorf("funding epoch: current source observation does not follow the prior window")
	}
	if err := validateFundingSettlementObservation(request.CurrentObservation.Observation, request, spotSymbol,
		request.AccountRequest.TimestampNano); err != nil {
		return nil, nil, nil, fmt.Errorf("funding epoch: current boundary: %w", err)
	}
	if len(unavailable) == 0 {
		if len(request.SourceWindow.Unavailable) != 0 || len(request.SourceWindow.Samples) != len(samples) {
			return nil, nil, nil, fmt.Errorf("funding epoch: numeric window disagrees with its source observations")
		}
		for index, sample := range samples {
			if sample != request.SourceWindow.Samples[index] {
				return nil, nil, nil, fmt.Errorf("funding epoch: numeric sample %d is not derived from its venue source", index)
			}
		}
	} else if len(request.SourceWindow.Samples) != 0 || len(request.SourceWindow.Unavailable) != len(unavailable) {
		return nil, nil, nil, fmt.Errorf("funding epoch: unavailable window has contradictory numeric samples")
	}
	for index, observation := range unavailable {
		if !reflectFundingObservationEqual(observation, request.SourceWindow.Unavailable[index]) {
			return nil, nil, nil, fmt.Errorf("funding epoch: unavailable-source list disagrees with canonical grid")
		}
	}
	return samples, unavailable, sequences, nil
}

func validateFundingSettlementObservation(observation FundingBookObservation, request FundingSettlementAttemptRequest,
	spotSymbol string, atNano int64) error {
	if observation.Version != FundingBookObservationVersion ||
		observation.VenueID != request.AccountRequest.VenueID ||
		observation.SpotSymbol != spotSymbol || observation.PerpSymbol != request.AccountRequest.PerpSymbol ||
		observation.TimestampNano != atNano {
		return fmt.Errorf("funding epoch: source observation identity contradicts the registered venue grid")
	}
	if !observation.Available {
		if observation.Pair != nil || !FundingBookUnavailableReason(observation.Reason).Valid() {
			return fmt.Errorf("funding epoch: unavailable observation is malformed")
		}
		return nil
	}
	if observation.Reason != "" || observation.Pair == nil {
		return fmt.Errorf("funding epoch: available observation is missing its venue-local pair")
	}
	pair := *observation.Pair
	if err := validateFundingObservedPair(FundingBookPairRequest{
		VenueID: observation.VenueID, SpotSymbol: spotSymbol,
		PerpSymbol: observation.PerpSymbol, TimestampNano: atNano,
	}, pair); err != nil {
		return err
	}
	return nil
}

func reflectFundingObservationEqual(left, right FundingBookObservation) bool {
	if left.Version != right.Version || left.VenueID != right.VenueID || left.SpotSymbol != right.SpotSymbol ||
		left.PerpSymbol != right.PerpSymbol || left.TimestampNano != right.TimestampNano ||
		left.Available != right.Available || left.Reason != right.Reason {
		return false
	}
	if left.Pair == nil || right.Pair == nil {
		return left.Pair == nil && right.Pair == nil
	}
	return *left.Pair == *right.Pair
}

func buildFundingSettlementEvidence(request FundingSettlementAttemptRequest, input FundingBatchInput,
	result FundingSettlementAttemptResult, terms instrument.FundingSettlementTerms,
	unavailable []FundingBookObservation, spotSymbol string, calendar instrument.FundingCalendar,
	rateContract instrument.WindowedFundingRateContract) FundingSettlementEvidence {
	evidence := FundingSettlementEvidence{
		Version: 1, VenueID: input.VenueID, SpotSymbol: spotSymbol,
		PerpSymbol: input.Symbol, TimestampNano: request.AccountRequest.TimestampNano,
		Calendar: calendar, RateContract: rateContract,
		Status: result.Status, SourceEventSequences: slices.Clone(result.SourceEventSequences),
		FrontierEventSequence:   request.CurrentObservation.EventSeq,
		UnavailableObservations: slices.Clone(unavailable),
		RoundingReserveBefore:   input.CurrentRoundingReserve,
		RoundingReserveAfter:    input.CurrentRoundingReserve,
		PayerShortfallClientIDs: slices.Clone(result.PayerShortfallClientIDs),
		Accounts:                make([]FundingSettlementAccount, 0, len(input.Accounts)),
	}
	if result.Status != FundingSettlementRateUnavailable {
		evidence.RateSignedUnits = terms.Rate.SignedUnits()
		evidence.RateUnitsPerBp = terms.Rate.UnitsPerBp()
		evidence.NotionalMarkPrice = terms.NotionalMarkPrice
	}
	previewByClient := make(map[uint64]FundingAccountCashPreview, len(result.Preview.AccountCash))
	for _, preview := range result.Preview.AccountCash {
		previewByClient[preview.ClientID] = preview
	}
	for _, account := range input.Accounts {
		row := FundingSettlementAccount{ClientID: account.ClientID, NetPosition: account.NetPosition,
			CashBefore: account.PerpCash, CashAfter: account.PerpCash,
			AccrualBefore: account.Accrual, AccrualAfter: account.Accrual}
		if result.Status == FundingSettlementPosted {
			if preview, present := previewByClient[account.ClientID]; present {
				row.CashDelta, row.CashAfter = preview.CashDelta, preview.CashAfter
				row.AccrualAfter = preview.AccrualAfter
			}
		}
		evidence.Accounts = append(evidence.Accounts, row)
	}
	if result.Status == FundingSettlementPosted {
		evidence.RoundingReserveDelta = result.Preview.RoundingReserveDelta
		evidence.RoundingReserveAfter = result.Preview.RoundingReserveAfter
	}
	return evidence
}

func buildFundingSettlementAppendInput(e *DefaultExchange, evidence FundingSettlementEvidence,
	result FundingSettlementAttemptResult) (FundingSettlementAppendInput, *VenueBalanceEvent, error) {
	appendInput := FundingSettlementAppendInput{Evidence: evidence}
	for _, account := range evidence.Accounts {
		if account.CashDelta == 0 {
			continue
		}
		appendInput.BalanceChanges = append(appendInput.BalanceChanges, BalanceChangeEvent{
			Timestamp: evidence.TimestampNano, ClientID: account.ClientID,
			Symbol: evidence.PerpSymbol, Reason: "funding_settlement",
			Changes: []BalanceDelta{{Asset: e.ExchangeBalance.FundingRoundingReserves[evidence.PerpSymbol].Asset,
				Wallet: "perp", OldBalance: account.CashBefore, NewBalance: account.CashAfter, Delta: account.CashDelta}},
		})
	}
	if result.Status != FundingSettlementPosted || evidence.RoundingReserveDelta == 0 {
		return appendInput, nil, nil
	}
	if e.venueBalanceSequence == ^uint64(0) {
		return FundingSettlementAppendInput{}, nil, fmt.Errorf("funding epoch: venue balance sequence overflows")
	}
	reserve := e.ExchangeBalance.FundingRoundingReserves[evidence.PerpSymbol]
	movement := VenueBalanceEvent{Timestamp: evidence.TimestampNano,
		Sequence: e.venueBalanceSequence + 1, Bucket: VenueFundingRoundingReserve,
		Asset: reserve.Asset, Symbol: evidence.PerpSymbol,
		Reason: "funding_rounding_residual", OldBalance: evidence.RoundingReserveBefore,
		NewBalance: evidence.RoundingReserveAfter, Delta: evidence.RoundingReserveDelta}
	appendInput.ReserveMovement = &movement
	return appendInput, &movement, nil
}

func fundingSettlementSourceFrontier(plans []fundingSettlementPlan) (uint64, error) {
	sequences := make(map[uint64]struct{})
	frontier := uint64(0)
	endowmentFrontier := uint64(0)
	for _, plan := range plans {
		reserve := plan.request.Exchange.ExchangeBalance.FundingRoundingReserves[plan.request.AccountRequest.PerpSymbol]
		if reserve.EndowmentEventSeq == 0 || reserve.EndowmentMovementEventSeq <= reserve.EndowmentEventSeq {
			return 0, fmt.Errorf("funding epoch: finite reserve lacks ordered canonical endowment identities")
		}
		for _, sequence := range []uint64{reserve.EndowmentEventSeq, reserve.EndowmentMovementEventSeq} {
			if _, duplicate := sequences[sequence]; duplicate {
				return 0, fmt.Errorf("funding epoch: reserve endowments reuse canonical frame sequence %d", sequence)
			}
			sequences[sequence] = struct{}{}
			if sequence > endowmentFrontier {
				endowmentFrontier = sequence
			}
		}
	}
	frontier = endowmentFrontier
	for _, plan := range plans {
		for _, sequence := range plan.result.SourceEventSequences {
			if sequence == 0 || sequence <= endowmentFrontier {
				return 0, fmt.Errorf("funding epoch: source is absent or predates a finite reserve endowment")
			}
			if _, duplicate := sequences[sequence]; duplicate {
				return 0, fmt.Errorf("funding epoch: source windows reuse canonical frame sequence %d", sequence)
			}
			sequences[sequence] = struct{}{}
			if sequence > frontier {
				frontier = sequence
			}
		}
		sequence := plan.result.FrontierEventSequence
		if sequence == 0 || sequence <= endowmentFrontier {
			return 0, fmt.Errorf("funding epoch: current boundary is absent or predates a finite reserve endowment")
		}
		if _, duplicate := sequences[sequence]; duplicate {
			return 0, fmt.Errorf("funding epoch: source windows reuse canonical frame sequence %d", sequence)
		}
		sequences[sequence] = struct{}{}
		if sequence > frontier {
			frontier = sequence
		}
	}
	return frontier, nil
}

func validateFundingSettlementReceipt(plan *fundingSettlementPlan, receipt FundingSettlementReceipt, afterSequence uint64) (uint64, error) {
	if receipt.AttemptEventSequence == 0 || receipt.OutcomeEventSequence <= receipt.AttemptEventSequence {
		return 0, fmt.Errorf("funding epoch: settlement appender returned missing or reversed completion identity")
	}
	if len(receipt.BalanceEventSequences) != len(plan.appendInput.BalanceChanges) {
		return 0, fmt.Errorf("funding epoch: settlement appender omitted or added account balance events")
	}
	if (plan.appendInput.ReserveMovement == nil) != (receipt.ReserveMovementEventSeq == 0) {
		return 0, fmt.Errorf("funding epoch: settlement appender reserve movement identity disagrees with plan")
	}
	lastSequence := afterSequence
	ordered := []uint64{receipt.AttemptEventSequence}
	ordered = append(ordered, receipt.BalanceEventSequences...)
	if receipt.ReserveMovementEventSeq != 0 {
		ordered = append(ordered, receipt.ReserveMovementEventSeq)
	}
	ordered = append(ordered, receipt.OutcomeEventSequence)
	for _, sequence := range ordered {
		if sequence <= lastSequence {
			return 0, fmt.Errorf("funding epoch: settlement evidence sequence is not strictly after its source/previous epoch record")
		}
		lastSequence = sequence
	}
	return lastSequence, nil
}

func applyFundingSettlementPlan(plan *fundingSettlementPlan) {
	e := plan.request.Exchange
	owned := e.fundingStates[plan.result.PerpSymbol]
	for _, account := range plan.result.Preview.AccountCash {
		client := e.Clients[account.ClientID]
		client.PerpBalances[plan.input.QuoteAsset] = account.CashAfter
		owned.remainders[account.ClientID] = account.AccrualAfter
		if account.CashDelta != 0 {
			e.conservation.record([]BalanceDelta{{Asset: plan.input.QuoteAsset,
				Wallet: "perp", OldBalance: account.CashBefore, NewBalance: account.CashAfter,
				Delta: account.CashDelta}})
		}
	}
	if plan.reserveEvent != nil {
		reserve := e.ExchangeBalance.FundingRoundingReserves[plan.result.PerpSymbol]
		reserve.Balance = plan.reserveEvent.NewBalance
		e.ExchangeBalance.FundingRoundingReserves[plan.result.PerpSymbol] = reserve
		e.venueBalanceSequence = plan.reserveEvent.Sequence
		e.conservation.recordVenue(plan.reserveEvent.Asset, plan.reserveEvent.Delta)
	}
}

func failFundingSettlementRequests(requests []FundingSettlementAttemptRequest, failure error) {
	for _, request := range requests {
		if request.Exchange == nil {
			continue
		}
		if owned := request.Exchange.fundingStates[request.AccountRequest.PerpSymbol]; owned != nil {
			owned.settlementFailure = failure
		}
	}
}

func cloneFundingSettlementAppendInput(input FundingSettlementAppendInput) FundingSettlementAppendInput {
	input.Evidence.SourceEventSequences = slices.Clone(input.Evidence.SourceEventSequences)
	input.Evidence.UnavailableObservations = slices.Clone(input.Evidence.UnavailableObservations)
	input.Evidence.Accounts = slices.Clone(input.Evidence.Accounts)
	input.Evidence.PayerShortfallClientIDs = slices.Clone(input.Evidence.PayerShortfallClientIDs)
	input.BalanceChanges = slices.Clone(input.BalanceChanges)
	for index := range input.BalanceChanges {
		input.BalanceChanges[index].Changes = slices.Clone(input.BalanceChanges[index].Changes)
	}
	if input.ReserveMovement != nil {
		movement := *input.ReserveMovement
		input.ReserveMovement = &movement
	}
	return input
}

func cloneFundingSettlementReceipt(receipt FundingSettlementReceipt) FundingSettlementReceipt {
	receipt.BalanceEventSequences = slices.Clone(receipt.BalanceEventSequences)
	return receipt
}

func cloneFundingSettlementAttemptResult(result FundingSettlementAttemptResult) FundingSettlementAttemptResult {
	result.SourceEventSequences = slices.Clone(result.SourceEventSequences)
	result.PayerShortfallClientIDs = slices.Clone(result.PayerShortfallClientIDs)
	result.Preview.AccountCash = slices.Clone(result.Preview.AccountCash)
	if result.Terms != nil {
		terms := *result.Terms
		result.Terms = &terms
	}
	result.Receipt = cloneFundingSettlementReceipt(result.Receipt)
	return result
}
