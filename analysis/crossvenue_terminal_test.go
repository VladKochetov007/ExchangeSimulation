package analysis

import (
	"testing"

	etypes "exchange_sim/types"
)

func crossVenueTerminalFixture(t *testing.T) (Report, *CrossVenuePublicReplay, map[string]CrossVenueAccountDelta, CrossVenueTerminalConvention) {
	t.Helper()
	report, fills := crossVenueAccountFixture()
	venues := [2]string{"north", "south"}
	clients := map[string]uint64{"north": 7, "south": 8}
	deltas, err := ReconcileCrossVenueRouterAccounts(report, venues, clients, fills, CrossVenueAccountConvention{
		Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, TakerFeeBps: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	replay := &CrossVenuePublicReplay{
		Transitions: []CrossVenuePublicTransition{
			{VenueID: "north", SimTS: 15, GlobalSequence: 1},
			{VenueID: "south", SimTS: 16, GlobalSequence: 2},
		},
		Terminal: map[string]CrossVenueDisplayedBook{
			"north": {Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 5}}},
			"south": {Asks: []etypes.PriceLevel{{Price: 111, VisibleQty: 5}}},
		},
	}
	return report, replay, deltas, CrossVenueTerminalConvention{
		Venues: venues, Clients: clients, HorizonNano: 20, MaxBookEvidenceAgeNanos: 5,
		BasePrecision: 1, TakerFeeBps: 20,
	}
}

func TestCrossVenueTerminalValuationBindsBooksAndAccountsToHorizon(t *testing.T) {
	report, replay, deltas, convention := crossVenueTerminalFixture(t)
	result, err := ValueCrossVenueTerminalState(report, replay, deltas, convention)
	if err != nil || !result.Available || result.Value != -13 {
		t.Fatalf("terminal value = %#v, %v; want available -13", result, err)
	}
	if result.Venues[0].BookEvidenceAgeNanos != 5 || result.Venues[1].BookEvidenceAgeNanos != 4 {
		t.Fatalf("book evidence ages = %#v", result.Venues)
	}
}

func TestCrossVenueTerminalZeroBaseChangeDoesNotRequireFreshPrice(t *testing.T) {
	report, replay, deltas, convention := crossVenueTerminalFixture(t)
	for index := range report.TerminalAccounts {
		report.TerminalAccounts[index].Account.SpotBalances = append([]Balance(nil), report.InitialAccounts[index].Account.SpotBalances...)
	}
	for _, venue := range convention.Venues {
		row := deltas[venue]
		row.BaseDelta = 0
		row.QuoteDelta = 0
		deltas[venue] = row
	}
	convention.MaxBookEvidenceAgeNanos = 1
	result, err := ValueCrossVenueTerminalState(report, replay, deltas, convention)
	if err != nil || !result.Available || result.Value != 0 {
		t.Fatalf("zero-base terminal value = %#v, %v", result, err)
	}
	replay.Transitions = nil
	replay.Terminal = map[string]CrossVenueDisplayedBook{"north": {}, "south": {}}
	result, err = ValueCrossVenueTerminalState(report, replay, deltas, convention)
	if err != nil || !result.Available || result.Value != 0 {
		t.Fatalf("zero-base terminal value without price update = %#v, %v", result, err)
	}
}

func TestCrossVenueTerminalValuationRejectsMismatchedState(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Report, *CrossVenuePublicReplay, map[string]CrossVenueAccountDelta, *CrossVenueTerminalConvention)
	}{
		{"account-before-horizon", func(report *Report, _ *CrossVenuePublicReplay, _ map[string]CrossVenueAccountDelta, _ *CrossVenueTerminalConvention) {
			report.TerminalAccounts[0].Account.Timestamp--
		}},
		{"future-book", func(_ *Report, replay *CrossVenuePublicReplay, _ map[string]CrossVenueAccountDelta, _ *CrossVenueTerminalConvention) {
			replay.Transitions[1].SimTS = 21
		}},
		{"missing-book", func(_ *Report, replay *CrossVenuePublicReplay, _ map[string]CrossVenueAccountDelta, _ *CrossVenueTerminalConvention) {
			delete(replay.Terminal, "south")
		}},
		{"unbound-delta", func(_ *Report, _ *CrossVenuePublicReplay, deltas map[string]CrossVenueAccountDelta, _ *CrossVenueTerminalConvention) {
			deltas["north"] = CrossVenueAccountDelta{VenueID: "north", ClientID: 9, BaseDelta: 5, QuoteDelta: -501}
		}},
		{"missing-book-transition", func(_ *Report, replay *CrossVenuePublicReplay, _ map[string]CrossVenueAccountDelta, _ *CrossVenueTerminalConvention) {
			replay.Transitions = replay.Transitions[:1]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, replay, deltas, convention := crossVenueTerminalFixture(t)
			test.mutate(&report, replay, deltas, &convention)
			if _, err := ValueCrossVenueTerminalState(report, replay, deltas, convention); err == nil {
				t.Fatal("mismatched terminal state accepted")
			}
		})
	}
}

func TestCrossVenueTerminalValuationClassifiesStaleAndInsufficientDepth(t *testing.T) {
	report, replay, deltas, convention := crossVenueTerminalFixture(t)
	convention.MaxBookEvidenceAgeNanos = 4
	result, err := ValueCrossVenueTerminalState(report, replay, deltas, convention)
	if err != nil || result.Available || result.Venues[0].Closeout.Reason != "STALE_TERMINAL_BOOK" {
		t.Fatalf("stale terminal book = %#v, %v", result, err)
	}
	convention.MaxBookEvidenceAgeNanos = 5
	replay.Terminal["south"] = CrossVenueDisplayedBook{Asks: []etypes.PriceLevel{{Price: 111, VisibleQty: 4}}}
	result, err = ValueCrossVenueTerminalState(report, replay, deltas, convention)
	if err != nil || result.Available || result.Venues[1].Closeout.Reason != "INSUFFICIENT_VISIBLE_DEPTH" {
		t.Fatalf("insufficient terminal depth = %#v, %v", result, err)
	}
}
