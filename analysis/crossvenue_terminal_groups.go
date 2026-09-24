package analysis

import "fmt"

type CrossVenueFirstAttemptTerminal struct {
	Generation                uint64
	ExchangeOutcome           string
	ActorOutcome              string
	MissingFillReceipts       int
	BufferedEarlyFillReceipts int
}

// ReconcileCrossVenueFirstAttemptTerminal distinguishes exchange settlement
// from what the delayed router inbox knew by the horizon. It relies on the
// independently audited submission, placement/fill and receipt inputs.
func ReconcileCrossVenueFirstAttemptTerminal(groups []CrossVenueSubmissionGroup, results []CrossVenuePlacementResult, fills []CrossVenueExchangeFill, counters CrossVenueRouterEvidenceCounters, lotQty int64) (*CrossVenueFirstAttemptTerminal, error) {
	if lotQty <= 0 || len(groups) > 1 || len(results) != 2*len(groups) || counters.SubmittedGroups != len(groups) ||
		counters.CompletedGroups < 0 || counters.FailedGroups < 0 || counters.PendingGroups < 0 ||
		counters.CompletedGroups+counters.FailedGroups+counters.PendingGroups != counters.SubmittedGroups {
		return nil, fmt.Errorf("cross-venue first-attempt terminal: incomplete terminal counters or placement inventory")
	}
	if len(groups) == 0 {
		if len(fills) != 0 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: fills without attempt")
		}
		return nil, nil
	}
	group := groups[0]
	usedResults := make(map[int]struct{}, 2)
	knownLegs, acceptedLegs, missingFillReceipts, bufferedEarlyFillReceipts := 0, 0, 0, 0
	usedFills := make(map[int]struct{}, len(fills))
	for _, leg := range []CrossVenueSubmissionLeg{group.Buy, group.Sell} {
		placement := leg.Placement
		resultIndex := -1
		for index, candidate := range results {
			if candidate.VenueID == placement.Event.VenueID && candidate.ClientID == placement.Event.ClientID && candidate.RequestID == placement.RequestID {
				if resultIndex >= 0 {
					return nil, fmt.Errorf("cross-venue first-attempt terminal: duplicate placement result")
				}
				resultIndex = index
			}
		}
		if resultIndex < 0 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: submitted leg lacks exchange result")
		}
		if _, used := usedResults[resultIndex]; used {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: result used by two legs")
		}
		usedResults[resultIndex] = struct{}{}
		result := results[resultIndex]
		if (result.InboxAt == nil) != (result.InboxFrame == 0) {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: incomplete inbox acknowledgement identity")
		}
		if result.Kind != placement.Kind || result.OrderID != placement.OrderID || result.Side != placement.Side ||
			result.ExchangeAt != placement.Event.SimTS {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: result contradicts exchange placement")
		}
		if result.Kind == "REJECTED" {
			if result.FilledQty != 0 || result.Reason != placement.Reason {
				return nil, fmt.Errorf("cross-venue first-attempt terminal: rejected leg has fill or changed reason")
			}
			if result.InboxAt != nil {
				knownLegs++
			}
			continue
		}
		if result.Kind != "ACCEPTED" || result.FilledQty != lotQty || result.OrderID == 0 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: accepted FOK lacks complete exchange fill")
		}
		acceptedLegs++
		legFillCount, legMissingReceipts, legBufferedEarlyReceipts := 0, 0, 0
		for fillIndex, fill := range fills {
			if fill.Event.VenueID != result.VenueID || fill.Event.ClientID != result.ClientID || fill.OrderID != result.OrderID {
				continue
			}
			if _, used := usedFills[fillIndex]; used {
				return nil, fmt.Errorf("cross-venue first-attempt terminal: duplicate fill assignment")
			}
			usedFills[fillIndex] = struct{}{}
			legFillCount++
			if fill.Receipt == nil {
				legMissingReceipts++
			} else if result.InboxFrame != 0 && fill.Receipt.Event.GlobalSequence < result.InboxFrame {
				legBufferedEarlyReceipts++
			}
		}
		if legFillCount == 0 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: accepted leg lacks fill evidence")
		}
		missingFillReceipts += legMissingReceipts
		bufferedEarlyFillReceipts += legBufferedEarlyReceipts
		// BaseActor buffers fill notifications received before the accepted
		// order identity and replays them when the acceptance is delivered.
		if result.InboxFrame != 0 && legMissingReceipts == 0 {
			knownLegs++
		}
	}
	if len(usedResults) != len(results) || len(usedFills) != len(fills) {
		return nil, fmt.Errorf("cross-venue first-attempt terminal: unclaimed exchange outcome")
	}
	terminal := &CrossVenueFirstAttemptTerminal{Generation: group.Generation, MissingFillReceipts: missingFillReceipts, BufferedEarlyFillReceipts: bufferedEarlyFillReceipts}
	if acceptedLegs == 2 {
		terminal.ExchangeOutcome = "MATCHED_FOK"
	} else {
		terminal.ExchangeOutcome = "UNMATCHED_FOK"
	}
	if knownLegs != 2 {
		terminal.ActorOutcome = "ACTOR_TERMINAL_UNOBSERVED"
		if counters.PendingGroups != 1 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: actor claims terminal outcome without required inbox evidence")
		}
	} else if acceptedLegs == 2 {
		terminal.ActorOutcome = "COMPLETE"
		if counters.CompletedGroups != 1 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: actor complete counter contradicts received fills")
		}
	} else {
		terminal.ActorOutcome = "FAILED"
		if counters.FailedGroups != 1 {
			return nil, fmt.Errorf("cross-venue first-attempt terminal: actor failed counter contradicts terminal legs")
		}
	}
	return terminal, nil
}
