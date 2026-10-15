package me014composition_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/instrument"
	"exchange_sim/simulation"
)

type consumerFundingState struct {
	clock                *simulation.SimulatedClock
	venueID              string
	perpSymbol           string
	quoteAsset           string
	basePrecision        int64
	registeredClientIDs  []uint64
	initialRoundingAtoms int64
	currentRoundingAtoms int64
	accruals             map[uint64]exchange.ScaledFundingAccrualSnapshot
}

func makeConsumerFundingFixture(t *testing.T, initialCash int64) (*exposureFixture, *consumerFundingState) {
	t.Helper()
	clock := simulation.NewSimulatedClock(int64(time.Hour))
	fixture := makeExposureFixtureAtClock(t, "N", nil, []perpFixtureSpec{
		{symbol: "ABC-PERP", base: "ABC"},
	}, []positionFixture{
		{clientID: 1, symbol: "ABC-PERP", quantity: 1, entry: 100, side: exchange.Buy},
		{clientID: 2, symbol: "ABC-PERP", quantity: 1, entry: 100, side: exchange.Buy},
		{clientID: 3, symbol: "ABC-PERP", quantity: 2, entry: 100, side: exchange.Sell},
	}, clock)
	for _, clientID := range fixture.clients {
		fixture.market.Clients[clientID].MutatePerpBalance("USD", initialCash)
	}

	initialAccrual, err := exchange.NewScaledFundingAccrual(1, 1)
	if err != nil {
		t.Fatalf("create scaled funding accrual: %v", err)
	}
	accruals := make(map[uint64]exchange.ScaledFundingAccrualSnapshot, len(fixture.clients))
	for _, clientID := range fixture.clients {
		accruals[clientID] = initialAccrual.Snapshot()
	}
	state := &consumerFundingState{
		clock: clock, venueID: "N", perpSymbol: "ABC-PERP", quoteAsset: "USD", basePrecision: 1,
		registeredClientIDs:  append([]uint64(nil), fixture.clients...),
		initialRoundingAtoms: 3, currentRoundingAtoms: 3, accruals: accruals,
	}
	return fixture, state
}

// settleWithConsumerOwnedState models only a quiescent caller. The fixture does
// not run the phase scheduler, journal evidence, or provide atomic account and
// reserve mutation; those remain production integration requirements.
func (state *consumerFundingState) settleWithConsumerOwnedState(market *exchange.DefaultExchange, terms instrument.FundingSettlementTerms) (exchange.FundingBatchPreview, error) {
	request := exchange.FundingAccountSnapshotRequest{
		VenueID: state.venueID, PerpSymbol: state.perpSymbol,
		TimestampNano:     state.clock.NowUnixNano(),
		ExpectedClientIDs: append([]uint64(nil), state.registeredClientIDs...),
	}
	source, err := market.CaptureFundingAccountSnapshot(request)
	if err != nil {
		return exchange.FundingBatchPreview{}, fmt.Errorf("capture consumer funding source: %w", err)
	}
	if source.QuoteAsset != state.quoteAsset || source.BasePrecision != state.basePrecision {
		return exchange.FundingBatchPreview{}, errors.New("consumer funding source changed its instrument units")
	}

	input := exchange.FundingBatchInput{
		VenueID: source.VenueID, Symbol: source.PerpSymbol, QuoteAsset: source.QuoteAsset,
		BasePrecision: source.BasePrecision, Terms: terms,
		RegisteredClientIDs:    append([]uint64(nil), state.registeredClientIDs...),
		InitialRoundingReserve: state.initialRoundingAtoms,
		CurrentRoundingReserve: state.currentRoundingAtoms,
		Accounts:               make([]exchange.FundingBatchAccount, 0, len(source.Accounts)),
	}
	for _, account := range source.Accounts {
		accrual, found := state.accruals[account.ClientID]
		if !found {
			return exchange.FundingBatchPreview{}, fmt.Errorf("consumer funding state missing client %d accrual", account.ClientID)
		}
		input.Accounts = append(input.Accounts, exchange.FundingBatchAccount{
			ClientID: account.ClientID, NetPosition: account.NetPosition,
			PerpCash: account.PerpCash, Accrual: accrual,
		})
	}
	preview, err := exchange.PreviewFundingBatch(input)
	if err != nil {
		return exchange.FundingBatchPreview{}, err
	}

	current, err := market.CaptureFundingAccountSnapshot(request)
	if err != nil {
		return exchange.FundingBatchPreview{}, fmt.Errorf("recheck consumer funding source before commit: %w", err)
	}
	if !reflect.DeepEqual(source, current) || state.clock.NowUnixNano() != request.TimestampNano {
		return exchange.FundingBatchPreview{}, errors.New("consumer funding source changed between preview and commit")
	}
	for _, account := range preview.AccountCash {
		client := market.Clients[account.ClientID]
		if client == nil || client.PerpBalance(source.QuoteAsset) != account.CashBefore {
			return exchange.FundingBatchPreview{}, fmt.Errorf("consumer funding account %d changed before commit", account.ClientID)
		}
	}

	for _, account := range preview.AccountCash {
		market.Clients[account.ClientID].MutatePerpBalance(source.QuoteAsset, account.CashDelta)
		state.accruals[account.ClientID] = account.AccrualAfter
	}
	state.currentRoundingAtoms = preview.RoundingReserveAfter
	return preview, nil
}

func TestConsumerOwnedFundingStateComposesWithPublicSnapshotAndPreviewAcrossRepeatedTerms(t *testing.T) {
	fixture, state := makeConsumerFundingFixture(t, 100)
	rate, err := instrument.NewQuantizedFundingRate(1, 1)
	if err != nil {
		t.Fatalf("create quantized rate: %v", err)
	}
	terms := instrument.FundingSettlementTerms{Rate: rate, NotionalMarkPrice: 100}

	for settlement := 1; settlement <= 100; settlement++ {
		state.clock.Advance(time.Second)
		if _, err := state.settleWithConsumerOwnedState(fixture.market, terms); err != nil {
			t.Fatalf("settlement %d: %v", settlement, err)
		}
		if settlement == 50 {
			if state.currentRoundingAtoms != 2 || fixture.market.Clients[1].PerpBalance("USD") != 100 ||
				fixture.market.Clients[2].PerpBalance("USD") != 100 || fixture.market.Clients[3].PerpBalance("USD") != 101 {
				t.Fatalf("50-term cash/reserve state: reserve=%d accounts=[%d %d %d]",
					state.currentRoundingAtoms, fixture.market.Clients[1].PerpBalance("USD"),
					fixture.market.Clients[2].PerpBalance("USD"), fixture.market.Clients[3].PerpBalance("USD"))
			}
		}
	}

	if state.currentRoundingAtoms != 3 || fixture.market.Clients[1].PerpBalance("USD") != 99 ||
		fixture.market.Clients[2].PerpBalance("USD") != 99 || fixture.market.Clients[3].PerpBalance("USD") != 102 {
		t.Fatalf("100-term cash/reserve state: reserve=%d accounts=[%d %d %d]",
			state.currentRoundingAtoms, fixture.market.Clients[1].PerpBalance("USD"),
			fixture.market.Clients[2].PerpBalance("USD"), fixture.market.Clients[3].PerpBalance("USD"))
	}
	clientCash := fixture.market.Clients[1].PerpBalance("USD") + fixture.market.Clients[2].PerpBalance("USD") + fixture.market.Clients[3].PerpBalance("USD")
	if clientCash+state.currentRoundingAtoms != 303 {
		t.Fatalf("client cash plus consumer-owned finite reserve = %d, want 303", clientCash+state.currentRoundingAtoms)
	}
	for clientID, accrual := range state.accruals {
		if accrual.RemainderNumerator != "0" {
			t.Fatalf("client %d retained remainder after 100 exact accruals: %+v", clientID, accrual)
		}
	}
}

func TestConsumerFundingShortfallLeavesAccountsAndOwnedStateUnchanged(t *testing.T) {
	fixture, state := makeConsumerFundingFixture(t, 0)
	rate, err := instrument.NewQuantizedFundingRate(1, 1)
	if err != nil {
		t.Fatalf("create quantized rate: %v", err)
	}
	terms := instrument.FundingSettlementTerms{Rate: rate, NotionalMarkPrice: 10_000}
	initialState := *state
	initialState.registeredClientIDs = append([]uint64(nil), state.registeredClientIDs...)
	initialState.accruals = make(map[uint64]exchange.ScaledFundingAccrualSnapshot, len(state.accruals))
	for clientID, accrual := range state.accruals {
		initialState.accruals[clientID] = accrual
	}
	initialCash := []int64{
		fixture.market.Clients[1].PerpBalance("USD"),
		fixture.market.Clients[2].PerpBalance("USD"),
		fixture.market.Clients[3].PerpBalance("USD"),
	}

	_, err = state.settleWithConsumerOwnedState(fixture.market, terms)
	var shortfall *exchange.FundingPayerCashShortfall
	if !errors.As(err, &shortfall) {
		t.Fatalf("settlement error = %v, want a typed payer shortfall", err)
	}
	if !reflect.DeepEqual(shortfall.ClientIDs, []uint64{1, 2}) {
		t.Fatalf("shortfall clients = %v, want both funding payers [1 2]", shortfall.ClientIDs)
	}
	if !reflect.DeepEqual(*state, initialState) {
		t.Fatalf("owned accrual/reserve state changed after rejected preview: got=%+v want=%+v", *state, initialState)
	}
	for index, clientID := range state.registeredClientIDs {
		if got := fixture.market.Clients[clientID].PerpBalance("USD"); got != initialCash[index] {
			t.Fatalf("client %d cash changed on rejected preview: got=%d want=%d", clientID, got, initialCash[index])
		}
	}
}
