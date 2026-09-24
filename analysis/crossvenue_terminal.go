package analysis

import "fmt"

type CrossVenueTerminalConvention struct {
	Venues                  [2]string
	Clients                 map[string]uint64
	HorizonNano             int64
	MaxBookEvidenceAgeNanos int64
	BasePrecision           int64
	TakerFeeBps             int64
}

type CrossVenueTerminalVenue struct {
	VenueID              string
	ClientID             uint64
	AccountAt            int64
	BookEvidenceAt       int64
	BookEvidenceSequence uint64
	BookEvidenceAgeNanos int64
	Closeout             VenueLocalCloseout
}

type CrossVenueTerminalValue struct {
	Available bool
	Value     int64
	Venues    [2]CrossVenueTerminalVenue
}

// ValueCrossVenueTerminalState binds independently reconciled account deltas
// and a complete public-book replay to one declared terminal horizon. Callers
// must first verify the raw evidence and fill/account reconciliation; this
// function does not prove that a still-pending order cannot later settle.
func ValueCrossVenueTerminalState(report Report, replay *CrossVenuePublicReplay, deltas map[string]CrossVenueAccountDelta, convention CrossVenueTerminalConvention) (CrossVenueTerminalValue, error) {
	var result CrossVenueTerminalValue
	venues := convention.Venues
	if replay == nil || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] ||
		len(convention.Clients) != 2 || len(deltas) != 2 || convention.HorizonNano <= 0 ||
		convention.MaxBookEvidenceAgeNanos <= 0 || convention.BasePrecision <= 0 || convention.TakerFeeBps < 0 {
		return result, fmt.Errorf("cross-venue terminal: invalid convention or input")
	}
	latest := make(map[string]CrossVenuePublicTransition, 2)
	var previousSequence uint64
	var previousTime int64
	for index, transition := range replay.Transitions {
		if transition.GlobalSequence == 0 || index > 0 && transition.GlobalSequence <= previousSequence ||
			transition.SimTS < 0 || index > 0 && transition.SimTS < previousTime || transition.SimTS > convention.HorizonNano ||
			transition.VenueID != venues[0] && transition.VenueID != venues[1] {
			return result, fmt.Errorf("cross-venue terminal: invalid public transition %d", index)
		}
		previousSequence, previousTime = transition.GlobalSequence, transition.SimTS
		latest[transition.VenueID] = transition
	}
	closeouts := make(map[string]VenueLocalCloseout, 2)
	for index, venue := range venues {
		clientID := convention.Clients[venue]
		delta, hasDelta := deltas[venue]
		book, hasBook := replay.Terminal[venue]
		transition, hasTransition := latest[venue]
		if clientID == 0 || !hasDelta || delta.VenueID != venue || delta.ClientID != clientID || !hasBook || delta.BaseDelta != 0 && !hasTransition {
			return result, fmt.Errorf("cross-venue terminal: missing or mismatched %s evidence", venue)
		}
		account, err := oneCrossVenueAccount(report.TerminalAccounts, venue, clientID, "terminal_post_mark")
		if err != nil {
			return result, err
		}
		if account.Account.Timestamp != convention.HorizonNano {
			return result, fmt.Errorf("cross-venue terminal: %s account time differs from horizon", venue)
		}
		age := convention.HorizonNano - transition.SimTS
		closeout := VenueLocalCloseout{Reason: "STALE_TERMINAL_BOOK"}
		if delta.BaseDelta == 0 || age <= convention.MaxBookEvidenceAgeNanos {
			closeout = ValueVenueLocalInventory(VenueLocalCloseoutInput{
				BaseDelta: delta.BaseDelta, QuoteDelta: delta.QuoteDelta,
				BasePrecision: convention.BasePrecision, TakerFeeBps: convention.TakerFeeBps,
				Bids: book.Bids, Asks: book.Asks,
			})
		}
		result.Venues[index] = CrossVenueTerminalVenue{
			VenueID: venue, ClientID: clientID, AccountAt: account.Account.Timestamp,
			BookEvidenceAt: transition.SimTS, BookEvidenceSequence: transition.GlobalSequence,
			BookEvidenceAgeNanos: age, Closeout: closeout,
		}
		closeouts[venue] = closeout
	}
	if !closeouts[venues[0]].Available || !closeouts[venues[1]].Available {
		return result, nil
	}
	value, err := SumVenueLocalCloseouts(closeouts)
	if err != nil {
		return result, err
	}
	result.Available, result.Value = true, value
	return result, nil
}
