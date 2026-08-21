package analysis

import (
	"sort"
	"sync"
)

// ratePayload is a published funding rate.
type ratePayload struct {
	Timestamp int64 `json:"timestamp"`
	Rate      int64 `json:"rate"`
}

// DerivativeAuditOptions configures the funding and exercise audits.
type DerivativeAuditOptions struct {
	Files         []string
	FilesSelected bool
	BasePrecision int64
}

// FundingInstantCheck is one venue's funding settlement, recomputed.
type FundingInstantCheck struct {
	VenueID   string `json:"venue_id"`
	Timestamp int64  `json:"timestamp"`
	// Payers and Receivers are account counts, and Paid the total charged.
	Payers    int   `json:"payers"`
	Receivers int   `json:"receivers"`
	Paid      int64 `json:"paid"`
	Received  int64 `json:"received"`
	// Residual is Paid plus Received, which must be zero: funding is a
	// transfer between the two sides of one contract, not a payment by the
	// venue.
	Residual int64 `json:"residual"`
	// LongsPaid records the direction: with a positive funding rate the longs
	// pay, so a run in which the sign is reversed is a sign error in the
	// mechanism rather than a market outcome.
	Rate      int64 `json:"rate"`
	LongsPaid bool  `json:"longs_paid"`
	// SignConsistent is false when the direction contradicts the rate.
	SignConsistent bool `json:"sign_consistent"`
}

// ExerciseCheck is one option's settlement, recomputed from its own terms.
type ExerciseCheck struct {
	VenueID         string `json:"venue_id"`
	Symbol          string `json:"symbol"`
	Strike          int64  `json:"strike"`
	IsCall          bool   `json:"is_call"`
	SettlementPrice int64  `json:"settlement_price"`
	// Intrinsic is max(S-K,0) for a call and max(K-S,0) for a put, per
	// contract, in quote units.
	Intrinsic int64 `json:"intrinsic"`
	Holders   int   `json:"holders"`
	NetSize   int64 `json:"net_size"`
	// ExpectedPayout is the intrinsic value times the net long position, which
	// is zero, so the sum across holders must be zero for every option however
	// deep in the money it is: one side's gain is the other's loss.
	ExpectedPayout int64 `json:"expected_payout"`
	PaidOut        int64 `json:"paid_out"`
	Residual       int64 `json:"residual"`
	// OutOfMoneyPaid flags an option that paid something while worthless.
	OutOfMoneyPaid bool `json:"out_of_money_paid"`
}

// DerivativeSemantics is the audit of funding and exercise.
type DerivativeSemantics struct {
	Funding          []FundingInstantCheck `json:"funding"`
	Exercises        []ExerciseCheck       `json:"exercises"`
	FundingBroken    int                   `json:"funding_broken"`
	FundingSignWrong int                   `json:"funding_sign_wrong"`
	ExerciseBroken   int                   `json:"exercise_broken"`
	WorthlessPaid    int                   `json:"worthless_paid"`
}

// MeasureDerivativeSemantics audits perpetual funding and option exercise.
//
// Funding is checked as a transfer: every instant must net to zero across
// accounts, and the side that pays must be the side the published rate says
// pays. Exercise is checked against the contract's own terms: an option's
// payoff is its intrinsic value at the settlement price, one holder's gain is
// another's loss, and an option that finished out of the money must pay
// nothing at all.
func (r *Run) MeasureDerivativeSemantics(opts DerivativeAuditOptions) (*DerivativeSemantics, error) {
	type instrumentPayload struct {
		Symbol          string `json:"symbol"`
		InstrumentType  string `json:"instrument_type"`
		Strike          int64  `json:"strike"`
		IsCall          bool   `json:"is_call"`
		SettlementPrice int64  `json:"settlement_price"`
		ExpiryNano      int64  `json:"expiry_nano"`
	}
	type positionPayload struct {
		Timestamp int64  `json:"timestamp"`
		ClientID  uint64 `json:"client_id"`
		Symbol    string `json:"symbol"`
		NewSize   int64  `json:"new_size"`
	}

	var mu sync.Mutex
	options := make(map[markKey]instrumentPayload)
	rates := make(map[string][]ratePayload)
	type fundingBucket struct {
		payers, receivers int
		paid, received    int64
	}
	funding := make(map[instantKey]*fundingBucket)
	optionPaid := make(map[markKey]struct {
		amount   int64
		accounts int
	})
	type positionState struct {
		size, at int64
	}
	perpPositions := make(map[positionKey]*positionState)
	optionPositions := make(map[positionKey]*positionState)
	expiries := make(map[markKey]int64)

	// First pass: contract terms and expiry instants.
	if err := r.Scan(ScanOptions{Events: []string{"instrument_settled"}, Files: opts.Files, FilesSelected: opts.FilesSelected}, func(event Event) {
		var payload instrumentPayload
		if event.Decode(&payload) != nil || payload.InstrumentType != "OPTION" {
			return
		}
		mu.Lock()
		options[markKey{event.VenueID, payload.Symbol}] = payload
		expiries[markKey{event.VenueID, payload.Symbol}] = payload.ExpiryNano
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	scan := ScanOptions{
		Events:        []string{"funding_rate_update", "balance_change", "position_update"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "funding_rate_update":
			var payload ratePayload
			if event.Decode(&payload) != nil {
				return
			}
			mu.Lock()
			rates[event.VenueID] = append(rates[event.VenueID], payload)
			mu.Unlock()
		case "position_update":
			var payload positionPayload
			if event.Decode(&payload) != nil || payload.Symbol == "" {
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			key := positionKey{event.VenueID, payload.ClientID, payload.Symbol}
			mu.Lock()
			if isOptionSymbol(payload.Symbol) {
				if expiry, known := expiries[markKey{event.VenueID, payload.Symbol}]; known && at > expiry {
					mu.Unlock()
					return
				}
				state := optionPositions[key]
				if state == nil || at >= state.at {
					optionPositions[key] = &positionState{size: payload.NewSize, at: at}
				}
				mu.Unlock()
				return
			}
			state := perpPositions[key]
			if state == nil || at >= state.at {
				perpPositions[key] = &positionState{size: payload.NewSize, at: at}
			}
			mu.Unlock()
		case "balance_change":
			var record balanceChangeRecord
			if event.Decode(&record) != nil {
				return
			}
			total := int64(0)
			for _, change := range record.Changes {
				total += change.Delta
			}
			instant := record.Timestamp
			if instant == 0 {
				instant = event.SimTS
			}
			mu.Lock()
			defer mu.Unlock()
			switch {
			case record.Reason == "funding_settlement":
				key := instantKey{event.VenueID, instant, "USD"}
				bucket := funding[key]
				if bucket == nil {
					bucket = &fundingBucket{}
					funding[key] = bucket
				}
				if total >= 0 {
					bucket.receivers++
					bucket.received += total
					return
				}
				bucket.payers++
				bucket.paid += total
			case record.Reason == "expiry_settlement" && isOptionSymbol(record.Symbol):
				key := markKey{event.VenueID, record.Symbol}
				entry := optionPaid[key]
				entry.amount += total
				entry.accounts++
				optionPaid[key] = entry
			}
		}
	}); err != nil {
		return nil, err
	}

	precision := opts.BasePrecision
	if precision <= 0 {
		precision = 1
	}
	result := &DerivativeSemantics{}

	for key, bucket := range funding {
		check := FundingInstantCheck{
			VenueID: key.venue, Timestamp: key.timestamp,
			Payers: bucket.payers, Receivers: bucket.receivers,
			Paid: bucket.paid, Received: bucket.received,
			Residual: bucket.paid + bucket.received,
		}
		check.Rate = rateAt(rates[key.venue], key.timestamp)
		check.LongsPaid = bucket.paid < 0
		// With a positive rate the longs pay, so somebody must have been
		// charged; a zero rate should charge nobody.
		check.SignConsistent = (check.Rate > 0 && bucket.payers > 0) ||
			(check.Rate < 0 && bucket.receivers > 0) ||
			(check.Rate == 0 && bucket.payers == 0 && bucket.receivers == 0)
		if check.Residual != 0 {
			result.FundingBroken++
		}
		if !check.SignConsistent {
			result.FundingSignWrong++
		}
		result.Funding = append(result.Funding, check)
	}
	sort.Slice(result.Funding, func(i, j int) bool {
		if result.Funding[i].VenueID != result.Funding[j].VenueID {
			return result.Funding[i].VenueID < result.Funding[j].VenueID
		}
		return result.Funding[i].Timestamp < result.Funding[j].Timestamp
	})

	for key, terms := range options {
		check := ExerciseCheck{
			VenueID: key.venue, Symbol: key.symbol, Strike: terms.Strike,
			IsCall: terms.IsCall, SettlementPrice: terms.SettlementPrice,
			Intrinsic: intrinsicValue(terms.SettlementPrice, terms.Strike, terms.IsCall),
		}
		for holderKey, state := range optionPositions {
			if holderKey.venue != key.venue || holderKey.symbol != key.symbol || state.size == 0 {
				continue
			}
			check.Holders++
			check.NetSize += state.size
			check.ExpectedPayout += mulDiv(check.Intrinsic, state.size, precision)
		}
		entry := optionPaid[key]
		check.PaidOut = entry.amount
		check.Residual = check.PaidOut - check.ExpectedPayout
		check.OutOfMoneyPaid = check.Intrinsic == 0 && entry.amount != 0
		if check.Residual != 0 {
			result.ExerciseBroken++
		}
		if check.OutOfMoneyPaid {
			result.WorthlessPaid++
		}
		result.Exercises = append(result.Exercises, check)
	}
	sort.Slice(result.Exercises, func(i, j int) bool {
		if result.Exercises[i].VenueID != result.Exercises[j].VenueID {
			return result.Exercises[i].VenueID < result.Exercises[j].VenueID
		}
		return result.Exercises[i].Symbol < result.Exercises[j].Symbol
	})
	return result, nil
}

// intrinsicValue is what an option is worth at settlement, per contract.
func intrinsicValue(settlement, strike int64, isCall bool) int64 {
	if isCall {
		if settlement > strike {
			return settlement - strike
		}
		return 0
	}
	if strike > settlement {
		return strike - settlement
	}
	return 0
}

// rateAt is the last published funding rate at or before an instant.
func rateAt(published []ratePayload, at int64) int64 {
	best := int64(0)
	bestAt := int64(-1)
	for _, point := range published {
		if point.Timestamp <= at && point.Timestamp > bestAt {
			best, bestAt = point.Rate, point.Timestamp
		}
	}
	return best
}
