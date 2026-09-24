package analysis

import "testing"

func crossVenueTerminalGroupFixture(t *testing.T, matched bool, actorSawOutcome bool) ([]CrossVenueSubmissionGroup, []CrossVenuePlacementResult, []CrossVenueExchangeFill, CrossVenueRouterEvidenceCounters) {
	t.Helper()
	evaluations, decisions, placements := crossVenueSubmissionFixture(t)
	if matched {
		placements[1].Kind, placements[1].Reason, placements[1].OrderID = "ACCEPTED", "", 22
	}
	groups, err := BindCrossVenueSubmissionOutcomes(evaluations, decisions, placements, "ABC/USD", 5, 1)
	if err != nil {
		t.Fatal(err)
	}
	results := []CrossVenuePlacementResult{
		{VenueID: "north", ClientID: 7, RequestID: 1, OrderID: 21, Kind: "ACCEPTED", Side: "BUY", ExchangeAt: 11, FilledQty: 5},
		{VenueID: "south", ClientID: 8, RequestID: 1, OrderID: placements[1].OrderID, Kind: placements[1].Kind, Reason: placements[1].Reason, Side: "SELL", ExchangeAt: 12},
	}
	fills := []CrossVenueExchangeFill{{Event: Event{VenueID: "north", ClientID: 7}, OrderID: 21, Qty: 5}}
	if matched {
		results[1].FilledQty = 5
		fills = append(fills, CrossVenueExchangeFill{Event: Event{VenueID: "south", ClientID: 8}, OrderID: 22, Qty: 5})
	}
	counters := CrossVenueRouterEvidenceCounters{SubmittedGroups: 1}
	if actorSawOutcome {
		receipt := &CrossVenueResponseReceiptRecord{Event: Event{GlobalSequence: 17}}
		acceptedAt := int64(13)
		results[0].InboxAt = &acceptedAt
		results[0].InboxFrame = 13
		for index := range fills {
			fills[index].Receipt = receipt
		}
		if matched {
			results[1].InboxAt = &acceptedAt
			results[1].InboxFrame = 14
			counters.CompletedGroups = 1
		} else {
			at := int64(13)
			results[1].InboxAt = &at
			results[1].InboxFrame = 14
			counters.FailedGroups = 1
		}
	} else {
		counters.PendingGroups = 1
	}
	return groups, results, fills, counters
}

func TestCrossVenueFirstAttemptTerminalSeparatesExchangeAndActorState(t *testing.T) {
	for _, test := range []struct {
		name      string
		matched   bool
		actorSaw  bool
		wantVenue string
		wantActor string
	}{
		{"matched-received", true, true, "MATCHED_FOK", "COMPLETE"},
		{"one-leg-received", false, true, "UNMATCHED_FOK", "FAILED"},
		{"matched-inbox-unobserved", true, false, "MATCHED_FOK", "ACTOR_TERMINAL_UNOBSERVED"},
		{"one-leg-inbox-unobserved", false, false, "UNMATCHED_FOK", "ACTOR_TERMINAL_UNOBSERVED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			groups, results, fills, counters := crossVenueTerminalGroupFixture(t, test.matched, test.actorSaw)
			terminal, err := ReconcileCrossVenueFirstAttemptTerminal(groups, results, fills, counters, 5)
			if err != nil || terminal == nil || terminal.ExchangeOutcome != test.wantVenue || terminal.ActorOutcome != test.wantActor {
				t.Fatalf("terminal classification = %#v, %v", terminal, err)
			}
		})
	}
	terminal, err := ReconcileCrossVenueFirstAttemptTerminal(nil, nil, nil, CrossVenueRouterEvidenceCounters{}, 5)
	if err != nil || terminal != nil {
		t.Fatalf("zero-attempt terminal = %#v, %v", terminal, err)
	}
}

func TestCrossVenueFirstAttemptTerminalRejectsInflatedOrIncompleteReport(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*[]CrossVenuePlacementResult, *[]CrossVenueExchangeFill, *CrossVenueRouterEvidenceCounters)
	}{
		{"false-actor-complete", func(_ *[]CrossVenuePlacementResult, _ *[]CrossVenueExchangeFill, counters *CrossVenueRouterEvidenceCounters) {
			counters.PendingGroups, counters.CompletedGroups = 0, 1
		}},
		{"fill-receipts-without-acceptance", func(results *[]CrossVenuePlacementResult, fills *[]CrossVenueExchangeFill, counters *CrossVenueRouterEvidenceCounters) {
			receipt := &CrossVenueResponseReceiptRecord{}
			(*fills)[0].Receipt = receipt
			counters.PendingGroups, counters.FailedGroups = 0, 1
			(*results)[0].InboxAt = nil
		}},
		{"missing-exchange-result", func(results *[]CrossVenuePlacementResult, _ *[]CrossVenueExchangeFill, _ *CrossVenueRouterEvidenceCounters) {
			*results = (*results)[:1]
		}},
		{"orphan-fill", func(_ *[]CrossVenuePlacementResult, fills *[]CrossVenueExchangeFill, _ *CrossVenueRouterEvidenceCounters) {
			*fills = append(*fills, CrossVenueExchangeFill{Event: Event{VenueID: "north", ClientID: 7}, OrderID: 99, Qty: 5})
		}},
		{"false-exchange-full", func(results *[]CrossVenuePlacementResult, _ *[]CrossVenueExchangeFill, _ *CrossVenueRouterEvidenceCounters) {
			(*results)[0].FilledQty = 4
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			groups, results, fills, counters := crossVenueTerminalGroupFixture(t, false, false)
			test.mutate(&results, &fills, &counters)
			if _, err := ReconcileCrossVenueFirstAttemptTerminal(groups, results, fills, counters, 5); err == nil {
				t.Fatal("unsupported terminal status accepted")
			}
		})
	}
}

func TestCrossVenueFirstAttemptTerminalReplaysBufferedFillAfterAcceptedInbox(t *testing.T) {
	groups, results, fills, counters := crossVenueTerminalGroupFixture(t, true, true)
	fills[0].Receipt = &CrossVenueResponseReceiptRecord{Event: Event{GlobalSequence: results[0].InboxFrame - 1}}
	terminal, err := ReconcileCrossVenueFirstAttemptTerminal(groups, results, fills, counters, 5)
	if err != nil || terminal.ActorOutcome != "COMPLETE" || terminal.BufferedEarlyFillReceipts != 1 {
		t.Fatalf("buffered early fill not reconciled: %#v, %v", terminal, err)
	}
	results[0].InboxAt = nil
	results[0].InboxFrame = 0
	counters.CompletedGroups, counters.PendingGroups = 0, 1
	terminal, err = ReconcileCrossVenueFirstAttemptTerminal(groups, results, fills, counters, 5)
	if err != nil || terminal.ActorOutcome != "ACTOR_TERMINAL_UNOBSERVED" {
		t.Fatalf("fill without acceptance treated as actor-observed: %#v, %v", terminal, err)
	}
	counters.CompletedGroups, counters.PendingGroups = 1, 0
	if _, err := ReconcileCrossVenueFirstAttemptTerminal(groups, results, fills, counters, 5); err == nil {
		t.Fatal("actor complete counter accepted without delivered order identity")
	}
}
