package exchange

import (
	"errors"
	"reflect"
	"testing"

	"exchange_sim/instrument"
)

const (
	fundingSettlementSecondNano = int64(1_000_000_000)
	fundingSettlementTestNano   = int64(25*60*fundingSettlementSecondNano + 10)
)

type fundingSettlementEpochAppendFunc func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error)

func (appendEpoch fundingSettlementEpochAppendFunc) AppendFundingSettlementEpoch(inputs []FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
	return appendEpoch(inputs)
}

func successfulFundingSettlementAppender(inputs *[]FundingSettlementAppendInput) fundingSettlementEpochAppendFunc {
	return func(appendInputs []FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		if inputs != nil {
			*inputs = append([]FundingSettlementAppendInput(nil), appendInputs...)
		}
		frontier := uint64(0)
		for _, input := range appendInputs {
			if input.Evidence.FrontierEventSequence > frontier {
				frontier = input.Evidence.FrontierEventSequence
			}
			for _, sequence := range input.Evidence.SourceEventSequences {
				if sequence > frontier {
					frontier = sequence
				}
			}
		}
		sequence := frontier
		receipts := make([]FundingSettlementReceipt, 0, len(appendInputs))
		for _, input := range appendInputs {
			sequence++
			receipt := FundingSettlementReceipt{AttemptEventSequence: sequence,
				BalanceEventSequences: make([]uint64, 0, len(input.BalanceChanges))}
			for range input.BalanceChanges {
				sequence++
				receipt.BalanceEventSequences = append(receipt.BalanceEventSequences, sequence)
			}
			if input.ReserveMovement != nil {
				sequence++
				receipt.ReserveMovementEventSeq = sequence
			}
			sequence++
			receipt.OutcomeEventSequence = sequence
			receipts = append(receipts, receipt)
		}
		return receipts, nil
	}
}

func newFundingSettlementVenue(t *testing.T, venueID string, payerCash int64) (*DefaultExchange, *fundingSourceClock) {
	t.Helper()
	ex, clock := fundingBookPairFixture(t, true)
	ex.ID = venueID
	setFundingSettlementFixtureCash(t, ex, 1, payerCash)
	endowment := fundingReserveTestEndowment()
	endowment.VenueID = venueID
	if venueID == "S" {
		endowment.Calendar = instrument.FundingCalendar{EpochNano: 10, IntervalSeconds: 480, PhaseSeconds: 60}
	}
	journal := fundingEndowmentAppendFunc(func(endowment FundingReserveEndowment, _ VenueBalanceEvent) (FundingEndowmentReceipt, error) {
		if endowment.VenueID == "S" {
			return FundingEndowmentReceipt{EndowmentEventSeq: 19, MovementEventSeq: 20}, nil
		}
		return FundingEndowmentReceipt{EndowmentEventSeq: 17, MovementEventSeq: 18}, nil
	})
	if err := ex.EndowFundingRoundingReserve(endowment, journal); err != nil {
		t.Fatal(err)
	}
	ex.Positions.UpdatePosition(1, "ABC-PERP", 1_000, 100, Buy, PositionBoth)
	ex.Positions.UpdatePosition(4, "ABC-PERP", 1_000, 100, Sell, PositionBoth)
	return ex, clock
}

func setFundingSettlementFixtureCash(t *testing.T, ex *DefaultExchange, clientID uint64, cash int64) {
	t.Helper()
	ex.mu.Lock()
	defer ex.mu.Unlock()
	client := ex.Clients[clientID]
	if client == nil {
		t.Fatalf("missing funding fixture client %d", clientID)
	}
	oldCash := client.PerpBalances["USD"]
	client.PerpBalances["USD"] = cash
	ex.conservation.record([]BalanceDelta{perpDelta("USD", oldCash, cash)})
}

func fundingSettlementRegistrations(venues ...*DefaultExchange) []FundingSettlementRegistration {
	registrations := make([]FundingSettlementRegistration, 0, len(venues))
	for _, venue := range venues {
		registrations = append(registrations, FundingSettlementRegistration{
			Exchange: venue, PerpSymbol: "ABC-PERP",
		})
	}
	return registrations
}

func fundingSettlementSequenceOffset(timestampNano int64, venueID string) uint64 {
	windowStartSeconds := (timestampNano - 60*fundingSettlementSecondNano) / fundingSettlementSecondNano
	venueOrder := uint64(0)
	if venueID == "S" {
		venueOrder = 1
	}
	return 100_000 + uint64(windowStartSeconds*2) + venueOrder
}

func fundingSettlementTestRequest(ex *DefaultExchange, venueID string, timestampNano int64,
	sequenceOffset uint64, unavailableIndex int) FundingSettlementAttemptRequest {
	contract := ex.fundingStates["ABC-PERP"].rateContract
	window := FundingSourceWindow{
		Sources: make([]FundingSourceRecord, 0, contract.SampleCount),
		Samples: make([]instrument.FundingWindowSample, 0, contract.SampleCount),
	}
	for index := range contract.SampleCount {
		at := timestampNano - int64(contract.SampleCount-index)*contract.SampleSpacingNano
		request := FundingBookPairRequest{VenueID: venueID, SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP", TimestampNano: at}
		sequence := sequenceOffset + uint64(index*2)
		observation := FundingBookObservation{Version: FundingBookObservationVersion,
			VenueID: venueID, SpotSymbol: request.SpotSymbol, PerpSymbol: request.PerpSymbol, TimestampNano: at}
		if index == unavailableIndex {
			observation.Reason = string(FundingBookNoDisplayedSide)
			window.Unavailable = append(window.Unavailable, observation)
		} else {
			pair := fundingWindowTestPair(request)
			observation.Available = true
			observation.Pair = &pair
			window.Samples = append(window.Samples, pair.RateWindowSample())
		}
		window.Sources = append(window.Sources, FundingSourceRecord{Observation: observation, EventSeq: sequence})
	}
	if len(window.Unavailable) != 0 {
		window.Samples = nil
	}
	currentRequest := FundingBookPairRequest{VenueID: venueID, SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
		TimestampNano: timestampNano}
	currentPair := fundingWindowTestPair(currentRequest)
	current := FundingSourceRecord{EventSeq: sequenceOffset + uint64(contract.SampleCount*2),
		Observation: FundingBookObservation{Version: FundingBookObservationVersion,
			VenueID: venueID, SpotSymbol: currentRequest.SpotSymbol, PerpSymbol: currentRequest.PerpSymbol,
			TimestampNano: timestampNano, Available: true, Pair: &currentPair}}
	accountRequest := fundingAccountSourceRequest()
	accountRequest.VenueID = venueID
	accountRequest.TimestampNano = timestampNano
	return FundingSettlementAttemptRequest{Exchange: ex, AccountRequest: accountRequest,
		SourceWindow: window, CurrentObservation: current}
}

func fundingSettlementRequestsAt(timestampNano int64, venues map[string]*DefaultExchange, venueIDs ...string) []FundingSettlementAttemptRequest {
	requests := make([]FundingSettlementAttemptRequest, 0, len(venueIDs))
	for _, venueID := range venueIDs {
		requests = append(requests, fundingSettlementTestRequest(venues[venueID], venueID, timestampNano,
			fundingSettlementSequenceOffset(timestampNano, venueID), -1))
	}
	return requests
}

func runFundingSettlementPrelude(t *testing.T, north, south *DefaultExchange,
	northClock, southClock *fundingSourceClock) ([]FundingSettlementRegistration, map[string]*DefaultExchange) {
	t.Helper()
	registrations := fundingSettlementRegistrations(north, south)
	venues := map[string]*DefaultExchange{"N": north, "S": south}
	appender := successfulFundingSettlementAppender(nil)
	prelude := []struct {
		timestampNano int64
		venueIDs      []string
	}{
		{5*60*fundingSettlementSecondNano + 10, []string{"N"}},
		{9*60*fundingSettlementSecondNano + 10, []string{"S"}},
		{10*60*fundingSettlementSecondNano + 10, []string{"N"}},
		{15*60*fundingSettlementSecondNano + 10, []string{"N"}},
		{17*60*fundingSettlementSecondNano + 10, []string{"S"}},
		{20*60*fundingSettlementSecondNano + 10, []string{"N"}},
	}
	for _, epoch := range prelude {
		northClock.now, southClock.now = epoch.timestampNano, epoch.timestampNano
		requests := fundingSettlementRequestsAt(epoch.timestampNano, venues, epoch.venueIDs...)
		result, err := SettleFundingEpoch(registrations, requests, appender)
		if err != nil || len(result.Attempts) != len(epoch.venueIDs) {
			t.Fatalf("scheduled funding prelude at %d failed: result=%+v err=%v", epoch.timestampNano, result, err)
		}
		for _, attempt := range result.Attempts {
			if attempt.Status != FundingSettlementPosted {
				t.Fatalf("solvent deterministic prelude produced %s at %s", attempt.Status, attempt.VenueID)
			}
		}
	}
	return registrations, venues
}

func TestSettleFundingEpochPostsCoincidentVenuePaymentsIndependentOfInputOrder(t *testing.T) {
	run := func(reverse bool) (FundingSettlementEpochResult, []FundingSettlementAppendInput, []*DefaultExchange) {
		north, northClock := newFundingSettlementVenue(t, "N", 100_000)
		south, southClock := newFundingSettlementVenue(t, "S", 100_000)
		registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
		northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
		requests := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "N", "S")
		if reverse {
			requests[0], requests[1] = requests[1], requests[0]
		}
		var appended []FundingSettlementAppendInput
		result, err := SettleFundingEpoch(registrations, requests, successfulFundingSettlementAppender(&appended))
		if err != nil {
			t.Fatalf("coincident funding epoch failed: %v", err)
		}
		return result, appended, []*DefaultExchange{north, south}
	}

	forward, forwardInput, forwardVenues := run(false)
	reverse, reverseInput, reverseVenues := run(true)
	if !reflect.DeepEqual(forward, reverse) || !reflect.DeepEqual(forwardInput, reverseInput) {
		t.Fatalf("venue request permutation changed the result or canonical append order:\nforward=%+v\nreverse=%+v", forward, reverse)
	}
	if len(forward.Attempts) != 2 || forward.Attempts[0].VenueID != "N" || forward.Attempts[1].VenueID != "S" ||
		forward.Attempts[0].Status != FundingSettlementPosted || forward.Attempts[1].Status != FundingSettlementPosted ||
		len(forward.Terminal) != 0 || len(forwardInput) != 2 || forwardInput[0].Evidence.VenueID != "N" || forwardInput[1].Evidence.VenueID != "S" {
		t.Fatalf("coincident venue attempts were not ordered and posted: result=%+v evidence=%+v", forward, forwardInput)
	}
	for _, venues := range [][]*DefaultExchange{forwardVenues, reverseVenues} {
		for _, venue := range venues {
			if len(venue.VerifyConservation()) != 0 {
				t.Fatalf("funding posting broke per-asset conservation at %s: %+v", venue.ID, venue.VerifyConservation())
			}
		}
	}
	for index, expectedCumulativeDelta := range []int64{-40, -39} {
		venue := forwardVenues[index]
		if got := venue.Clients[1].PerpBalances["USD"]; got != 100_000+expectedCumulativeDelta {
			t.Fatalf("%s payer cash = %d, want %d", venue.ID, got, 100_000+expectedCumulativeDelta)
		}
		if got := venue.Clients[4].PerpBalances["USD"]; got != 100_000-expectedCumulativeDelta {
			t.Fatalf("%s receiver cash = %d, want %d", venue.ID, got, 100_000-expectedCumulativeDelta)
		}
		if venue.ExchangeBalance.FundingRoundingReserves["ABC-PERP"].Balance != 4 {
			t.Fatalf("%s reserve changed despite exactly matched opposite claims", venue.ID)
		}
	}
}

func TestSettleFundingEpochAllowsSolventVenueAfterOtherVenueShortfall(t *testing.T) {
	north, northClock := newFundingSettlementVenue(t, "N", 32)
	south, southClock := newFundingSettlementVenue(t, "S", 100_000)
	registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
	northBefore, southBefore := north.Clients[1].PerpBalances["USD"], south.Clients[1].PerpBalances["USD"]
	northAccrual := north.fundingStates["ABC-PERP"].remainders[1]
	northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
	requests := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "S", "N")
	var appended []FundingSettlementAppendInput
	result, err := SettleFundingEpoch(registrations, requests, successfulFundingSettlementAppender(&appended))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 2 || result.Attempts[0].VenueID != "N" ||
		result.Attempts[0].Status != FundingSettlementPayerShortfall ||
		!reflect.DeepEqual(result.Attempts[0].PayerShortfallClientIDs, []uint64{1}) ||
		result.Attempts[1].VenueID != "S" || result.Attempts[1].Status != FundingSettlementPosted ||
		len(result.Terminal) != 1 || result.Terminal[0].VenueID != "N" || len(appended) != 2 {
		t.Fatalf("venue-local shortfall suppressed the solvent coincident payment: %+v", result)
	}
	if north.Clients[1].PerpBalances["USD"] != northBefore || north.fundingStates["ABC-PERP"].remainders[1] != northAccrual ||
		south.Clients[1].PerpBalances["USD"] != southBefore-13 {
		t.Fatalf("shortfall mutated N or suppressed solvent S payment: N=%d S=%d",
			north.Clients[1].PerpBalances["USD"], south.Clients[1].PerpBalances["USD"])
	}
}

func TestSettleFundingEpochUnavailableWindowAdvancesWithoutInventedTerms(t *testing.T) {
	ex, clock := newFundingSettlementVenue(t, "N", 100_000)
	clock.now = 5*60*fundingSettlementSecondNano + 10
	registration := fundingSettlementRegistrations(ex)
	request := fundingSettlementTestRequest(ex, "N", clock.now, fundingSettlementSequenceOffset(clock.now, "N"), 4)
	prior := ex.fundingStates["ABC-PERP"].remainders[1]
	var appended []FundingSettlementAppendInput
	result, err := SettleFundingEpoch(registration, []FundingSettlementAttemptRequest{request}, successfulFundingSettlementAppender(&appended))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].Status != FundingSettlementRateUnavailable ||
		len(result.Terminal) != 0 || len(appended) != 1 || appended[0].Evidence.NotionalMarkPrice != 0 ||
		appended[0].Evidence.RateUnitsPerBp != 0 || len(appended[0].BalanceChanges) != 0 || appended[0].ReserveMovement != nil {
		t.Fatalf("unavailable prior book was converted into a priced/posted payment: result=%+v input=%+v", result, appended)
	}
	if ex.Clients[1].PerpBalances["USD"] != 100_000 || ex.fundingStates["ABC-PERP"].remainders[1] != prior ||
		!ex.fundingStates["ABC-PERP"].hasAttempt || ex.fundingStates["ABC-PERP"].lastAttemptCycle != 1 ||
		ex.fundingStates["ABC-PERP"].lastAttemptNano != clock.now {
		t.Fatal("rate-unavailable attempt changed money/fraction or failed to advance exactly one calendar cycle")
	}
}

func TestSettleFundingEpochRejectsForgedUnavailableReasonBeforeAppend(t *testing.T) {
	ex, clock := newFundingSettlementVenue(t, "N", 100_000)
	clock.now = 5*60*fundingSettlementSecondNano + 10
	request := fundingSettlementTestRequest(ex, "N", clock.now,
		fundingSettlementSequenceOffset(clock.now, "N"), 4)
	lastUnavailable := &request.SourceWindow.Sources[4].Observation
	lastUnavailable.Reason = string(FundingBookNoDisplayedSide) + ": tick violation"
	request.SourceWindow.Unavailable[0] = *lastUnavailable
	appendCalled := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appendCalled = true
		return nil, nil
	})
	beforeCash := ex.Clients[1].PerpBalances["USD"]
	beforeAccrual := ex.fundingStates["ABC-PERP"].remainders[1]
	if _, err := SettleFundingEpoch(fundingSettlementRegistrations(ex),
		[]FundingSettlementAttemptRequest{request}, appender); err == nil {
		t.Fatal("structural tick contradiction was accepted as an economically unavailable book")
	}
	if appendCalled || ex.Clients[1].PerpBalances["USD"] != beforeCash ||
		ex.fundingStates["ABC-PERP"].remainders[1] != beforeAccrual ||
		ex.fundingStates["ABC-PERP"].settlementFailure == nil {
		t.Fatal("forged unavailable reason reached evidence append or mutated/re-enabled settlement")
	}
}

func TestSettleFundingEpochRejectsMissingOrMalformedReserveBeforeAppend(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*DefaultExchange)
	}{
		{"nil-ledger", func(ex *DefaultExchange) { ex.ExchangeBalance = nil }},
		{"missing-reserve", func(ex *DefaultExchange) {
			delete(ex.ExchangeBalance.FundingRoundingReserves, "ABC-PERP")
		}},
		{"malformed-reserve-receipt", func(ex *DefaultExchange) {
			reserve := ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"]
			reserve.EndowmentMovementEventSeq = reserve.EndowmentEventSeq
			ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"] = reserve
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ex, clock := newFundingSettlementVenue(t, "N", 100_000)
			clock.now = 5*60*fundingSettlementSecondNano + 10
			request := fundingSettlementTestRequest(ex, "N", clock.now,
				fundingSettlementSequenceOffset(clock.now, "N"), -1)
			beforeCash := ex.Clients[1].PerpBalances["USD"]
			beforeAccrual := ex.fundingStates["ABC-PERP"].remainders[1]
			ex.mu.Lock()
			test.mutate(ex)
			ex.mu.Unlock()
			appendCalled := false
			appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
				appendCalled = true
				return nil, nil
			})
			if _, err := SettleFundingEpoch(fundingSettlementRegistrations(ex),
				[]FundingSettlementAttemptRequest{request}, appender); err == nil {
				t.Fatal("missing or malformed reserve state was accepted")
			}
			if appendCalled || ex.Clients[1].PerpBalances["USD"] != beforeCash ||
				ex.fundingStates["ABC-PERP"].remainders[1] != beforeAccrual ||
				ex.fundingStates["ABC-PERP"].settlementFailure == nil {
				t.Fatal("invalid reserve reached evidence append or mutated/re-enabled settlement")
			}
		})
	}
}

func TestSettleFundingEpochRejectsSkippedCalendarCycleWithoutCatchup(t *testing.T) {
	ex, clock := newFundingSettlementVenue(t, "N", 100_000)
	clock.now = 10*60*fundingSettlementSecondNano + 10
	request := fundingSettlementTestRequest(ex, "N", clock.now, fundingSettlementSequenceOffset(clock.now, "N"), -1)
	appended := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appended = true
		return nil, nil
	})
	_, err := SettleFundingEpoch(fundingSettlementRegistrations(ex), []FundingSettlementAttemptRequest{request}, appender)
	if err == nil || appended || ex.fundingStates["ABC-PERP"].settlementFailure == nil {
		t.Fatalf("skipped first funding cycle was caught up instead of failing closed: appended=%t err=%v", appended, err)
	}
}

func TestSettleFundingEpochRequiresEveryDueVenueAtCoincidence(t *testing.T) {
	north, northClock := newFundingSettlementVenue(t, "N", 100_000)
	south, southClock := newFundingSettlementVenue(t, "S", 100_000)
	registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
	northBefore, southBefore := north.Clients[1].PerpBalances["USD"], south.Clients[1].PerpBalances["USD"]
	northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
	request := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "N")
	appended := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appended = true
		return nil, nil
	})
	if _, err := SettleFundingEpoch(registrations, request, appender); err == nil || appended {
		t.Fatalf("coincident S attempt was silently omitted: appended=%t", appended)
	}
	if north.Clients[1].PerpBalances["USD"] != northBefore || south.Clients[1].PerpBalances["USD"] != southBefore {
		t.Fatal("incomplete coincident request set changed a venue balance")
	}
	for _, venue := range []*DefaultExchange{north, south} {
		if venue.fundingStates["ABC-PERP"].settlementFailure == nil {
			t.Fatalf("incomplete epoch left %s eligible to retry", venue.ID)
		}
	}
}

func TestSettleFundingEpochStructuralFailureAbortsCoincidentAppend(t *testing.T) {
	north, northClock := newFundingSettlementVenue(t, "N", 100_000)
	south, southClock := newFundingSettlementVenue(t, "S", 100_000)
	registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
	northBefore, southBefore := north.Clients[1].PerpBalances["USD"], south.Clients[1].PerpBalances["USD"]
	northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
	requests := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "N", "S")
	requests[0].CurrentObservation.EventSeq = requests[0].SourceWindow.Sources[len(requests[0].SourceWindow.Sources)-1].EventSeq
	appended := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appended = true
		return nil, nil
	})
	if _, err := SettleFundingEpoch(registrations, requests, appender); err == nil || appended {
		t.Fatalf("structural source contradiction reached append: appended=%t err=%v", appended, err)
	}
	if north.Clients[1].PerpBalances["USD"] != northBefore || south.Clients[1].PerpBalances["USD"] != southBefore {
		t.Fatal("invalid coincident epoch changed a balance")
	}
	for _, venue := range []*DefaultExchange{north, south} {
		if venue.fundingStates["ABC-PERP"].settlementFailure == nil {
			t.Fatalf("invalid epoch left %s eligible to retry", venue.ID)
		}
	}
}

func TestSettleFundingEpochAppendFailureIsNonEconomicAndNonRetryable(t *testing.T) {
	north, northClock := newFundingSettlementVenue(t, "N", 100_000)
	south, southClock := newFundingSettlementVenue(t, "S", 100_000)
	registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
	beforeNorth, beforeSouth := north.Clients[1].PerpBalances["USD"], south.Clients[1].PerpBalances["USD"]
	northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		return nil, errors.New("injected binary writer failure")
	})
	requests := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "N", "S")
	if _, err := SettleFundingEpoch(registrations, requests, appender); err == nil {
		t.Fatal("required evidence append failure was accepted")
	}
	if north.Clients[1].PerpBalances["USD"] != beforeNorth || south.Clients[1].PerpBalances["USD"] != beforeSouth {
		t.Fatal("evidence append failure partially posted a coincident venue")
	}
	for _, venue := range []*DefaultExchange{north, south} {
		if venue.fundingStates["ABC-PERP"].settlementFailure == nil {
			t.Fatalf("%s was left retryable after ambiguous evidence failure", venue.ID)
		}
	}
}

func TestSettleFundingEpochRejectsCrossVenueDuplicateSourceIdentity(t *testing.T) {
	north, northClock := newFundingSettlementVenue(t, "N", 100_000)
	south, southClock := newFundingSettlementVenue(t, "S", 100_000)
	registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
	northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
	requests := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "N", "S")
	requests[1].CurrentObservation.EventSeq = requests[0].CurrentObservation.EventSeq
	appended := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appended = true
		return nil, nil
	})
	if _, err := SettleFundingEpoch(registrations, requests, appender); err == nil || appended {
		t.Fatalf("reused canonical frame sequence passed joint preflight: appended=%t", appended)
	}
}

func TestSettleFundingEpochRequiresSourceWindowAfterReserveEndowment(t *testing.T) {
	ex, clock := newFundingSettlementVenue(t, "N", 100_000)
	clock.now = 5*60*fundingSettlementSecondNano + 10
	request := fundingSettlementTestRequest(ex, "N", clock.now, 16, -1)
	appended := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appended = true
		return nil, nil
	})
	if _, err := SettleFundingEpoch(fundingSettlementRegistrations(ex), []FundingSettlementAttemptRequest{request}, appender); err == nil || appended {
		t.Fatalf("source preceding the reserve endowment was accepted: appended=%t", appended)
	}
}

func TestSettleFundingEpochRejectsNonMonotonicGlobalAppendReceiptBeforeMutation(t *testing.T) {
	north, northClock := newFundingSettlementVenue(t, "N", 100_000)
	south, southClock := newFundingSettlementVenue(t, "S", 100_000)
	registrations, venues := runFundingSettlementPrelude(t, north, south, northClock, southClock)
	northClock.now, southClock.now = fundingSettlementTestNano, fundingSettlementTestNano
	requests := fundingSettlementRequestsAt(fundingSettlementTestNano, venues, "N", "S")
	badAppender := fundingSettlementEpochAppendFunc(func(inputs []FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		receipts, err := successfulFundingSettlementAppender(nil)(inputs)
		if err != nil {
			return nil, err
		}
		receipts[1].AttemptEventSequence = receipts[0].OutcomeEventSequence
		return receipts, nil
	})
	if _, err := SettleFundingEpoch(registrations, requests, badAppender); err == nil {
		t.Fatal("overlapping venue receipt ranges were accepted")
	}
	if north.Clients[1].PerpBalances["USD"] != 100_000-32 || south.Clients[1].PerpBalances["USD"] != 100_000-26 {
		t.Fatal("invalid append receipt changed economic state")
	}
}

func TestSettleFundingEpochRejectsNilPerpWalletBeforeEvidenceAppend(t *testing.T) {
	ex, clock := newFundingSettlementVenue(t, "N", 100_000)
	clock.now = 5*60*fundingSettlementSecondNano + 10
	ex.Clients[2].PerpBalances = nil
	request := fundingSettlementTestRequest(ex, "N", clock.now, fundingSettlementSequenceOffset(clock.now, "N"), -1)
	appended := false
	appender := fundingSettlementEpochAppendFunc(func([]FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		appended = true
		return nil, nil
	})
	if _, err := SettleFundingEpoch(fundingSettlementRegistrations(ex), []FundingSettlementAttemptRequest{request}, appender); err == nil || appended {
		t.Fatalf("nil account wallet was not rejected before canonical append: appended=%t", appended)
	}
}

func TestSettleFundingEpochRejectsClockAdvanceDuringEvidenceAppend(t *testing.T) {
	ex, clock := newFundingSettlementVenue(t, "N", 100_000)
	clock.now = 5*60*fundingSettlementSecondNano + 10
	request := fundingSettlementTestRequest(ex, "N", clock.now, fundingSettlementSequenceOffset(clock.now, "N"), -1)
	appender := fundingSettlementEpochAppendFunc(func(inputs []FundingSettlementAppendInput) ([]FundingSettlementReceipt, error) {
		receipts, err := successfulFundingSettlementAppender(nil)(inputs)
		clock.now++
		return receipts, err
	})
	if _, err := SettleFundingEpoch(fundingSettlementRegistrations(ex), []FundingSettlementAttemptRequest{request}, appender); err == nil {
		t.Fatal("runner clock advance during canonical append was accepted")
	}
	if ex.Clients[1].PerpBalances["USD"] != 100_000 || ex.fundingStates["ABC-PERP"].settlementFailure == nil {
		t.Fatal("clock race posted cash or left settlement retryable")
	}
}
