package analysis

import (
	"sort"
	"sync"
)

// posPoint is one published perpetual position size and the physical evidence
// position at which it became effective. Funding can share a timestamp with a
// later fill, so SimTS alone is not enough to recover its contemporaneous side.
type posPoint struct {
	at, size int64
	file     string
	ordinal  int64
}

// perpSideAt returns the position size an account held in any perpetual on a
// venue at a given instant, and whether any position of theirs was published
// at or before it. An account holding several perpetuals on one venue is
// summed, because funding for the venue settles them together in the ledger.
func perpSideAt(history map[positionKey][]posPoint, venue string, client uint64, at int64, file string, ordinal int64) (int64, bool) {
	total := int64(0)
	known := false
	for key, points := range history {
		if key.venue != venue || key.clientID != client {
			continue
		}
		size, ok := sizeAt(points, at, file, ordinal)
		if !ok {
			continue
		}
		known = true
		total += size
	}
	return total, known
}

// sizeAt is the last position published before an evidence record. Same-file
// same-timestamp points before the record are eligible; points after it, and
// same-timestamp points from a physically separate file without a global order,
// are deliberately excluded.
func sizeAt(points []posPoint, at int64, file string, ordinal int64) (int64, bool) {
	firstAt := sort.Search(len(points), func(i int) bool { return points[i].at >= at })
	var size int64
	known := false
	if firstAt > 0 {
		size = points[firstAt-1].size
		known = true
	}
	lastAt := sort.Search(len(points), func(i int) bool { return points[i].at > at })
	latestOrdinal := int64(-1)
	for i := firstAt; i < lastAt; i++ {
		point := points[i]
		if point.file != file || point.ordinal >= ordinal || point.ordinal <= latestOrdinal {
			continue
		}
		size = point.size
		known = true
		latestOrdinal = point.ordinal
	}
	return size, known
}

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
	// Residual is Paid plus Received. Funding is a transfer between the two
	// sides of one contract, so it must be zero up to the truncation of each
	// account's integer share: a settlement across n accounts may lose up to n
	// units and no more.
	Residual int64 `json:"residual"`
	Rate     int64 `json:"rate"`
	// Directed counts the accounts whose perpetual side at this instant is
	// known from the position stream, and Misdirected those among them charged
	// the wrong way for the published rate: with a positive rate a long must
	// be debited and a short credited, and with a negative rate the reverse.
	// Counting only accounts whose side is known keeps the check honest --
	// an account whose position was never published is not evidence either
	// way and is reported separately as Undirected.
	Directed    int `json:"directed"`
	Misdirected int `json:"misdirected"`
	Undirected  int `json:"undirected"`
	// LongsPaid is true when the accounts that were long at this instant were
	// the ones debited. It is derived from the reconstructed sides, not from
	// the sign of the total, which carries no direction information.
	LongsPaid bool `json:"longs_paid"`
	// SignConsistent is false when any directed account was charged against
	// the published rate.
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
	// HoldersMispaid counts holders whose own payout does not match their own
	// position. The summed residual cannot see a compensating pair — one
	// holder overpaid and another underpaid by the same amount nets to zero —
	// which is precisely the error a settlement bug produces.
	HoldersMispaid int   `json:"holders_mispaid"`
	WorstHolderGap int64 `json:"worst_holder_gap"`
	// OutOfMoneyPaid flags an option that paid something while worthless.
	OutOfMoneyPaid bool `json:"out_of_money_paid"`
}

// DerivativeSemantics is the audit of funding and exercise.
type DerivativeSemantics struct {
	Funding          []FundingInstantCheck `json:"funding"`
	Exercises        []ExerciseCheck       `json:"exercises"`
	FundingBroken    int                   `json:"funding_broken"`
	FundingSignWrong int                   `json:"funding_sign_wrong"`
	// FundingMisdirected is the total number of account-instants charged
	// against the published rate, and FundingUndirected the number whose side
	// could not be established from the position stream. A large undirected
	// count means the direction check is weak on that run and must be said so
	// rather than read as a pass.
	FundingMisdirected int `json:"funding_misdirected"`
	FundingUndirected  int `json:"funding_undirected"`
	ExerciseBroken     int `json:"exercise_broken"`
	// HoldersMispaid is the count across every contract of holders whose own
	// payout did not match their own position.
	HoldersMispaid int `json:"holders_mispaid"`
	WorthlessPaid  int `json:"worthless_paid"`
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
	// Per-account funding movements, so the direction of the transfer can be
	// checked against each account's own side rather than against the sign of
	// the total, which is the same under any sign convention.
	fundingDeltas := make(map[instantKey]map[uint64]int64)
	type fundingCursor struct {
		file    string
		ordinal int64
	}
	// The first balance record at a funding instant marks the event boundary.
	// All affected accounts are settled by that operation before later records
	// at the same timestamp. Keeping its physical position avoids attributing a
	// post-funding trade to the funded side.
	fundingCursors := make(map[instantKey]fundingCursor)
	// Every published perpetual position, kept as a time series per account so
	// the side held at a funding instant can be read off it. Positions arrive
	// out of order because the scan is concurrent, so they are sorted after
	// the pass rather than assumed ordered during it.
	perpHistory := make(map[positionKey][]posPoint)
	optionPaid := make(map[markKey]struct {
		amount   int64
		accounts int
	})
	optionPaidPerHolder := make(map[positionKey]int64)
	type positionState struct {
		size, at int64
	}
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

	// Option positions are not published as position updates — only linear
	// contracts are — so an option holding has to be rebuilt from the fills
	// themselves. That is the more independent reconstruction anyway: it uses
	// the trades rather than the venue's own bookkeeping of them.
	type optionFill struct {
		Symbol string `json:"symbol"`
		Qty    int64  `json:"qty"`
		Side   string `json:"side"`
	}
	scan := ScanOptions{
		Events:        []string{"funding_rate_update", "balance_change", "position_update", "OrderFill"},
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
				// Options are rebuilt from fills above; a position update for
				// one would be the venue's own bookkeeping and is not used.
				mu.Unlock()
				return
			}
			perpHistory[key] = append(perpHistory[key], posPoint{at: at, size: payload.NewSize, file: event.File, ordinal: event.Ordinal})
			mu.Unlock()
		case "OrderFill":
			var fill optionFill
			if event.Decode(&fill) != nil || fill.Qty <= 0 {
				return
			}
			symbol := event.Symbol
			if symbol == "" {
				symbol = fill.Symbol
			}
			if !isOptionSymbol(symbol) {
				return
			}
			signed := fill.Qty
			if fill.Side == "SELL" {
				signed = -fill.Qty
			}
			mu.Lock()
			if expiry, known := expiries[markKey{event.VenueID, symbol}]; known && event.SimTS >= expiry {
				mu.Unlock()
				return
			}
			key := positionKey{event.VenueID, event.ClientID, symbol}
			state := optionPositions[key]
			if state == nil {
				state = &positionState{}
				optionPositions[key] = state
			}
			state.size += signed
			state.at = event.SimTS
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
				cursor, seen := fundingCursors[key]
				if !seen || (cursor.file == event.File && event.Ordinal < cursor.ordinal) {
					fundingCursors[key] = fundingCursor{file: event.File, ordinal: event.Ordinal}
				}
				bucket := funding[key]
				if bucket == nil {
					bucket = &fundingBucket{}
					funding[key] = bucket
				}
				// An account charged nothing is not a payer or a receiver. It
				// is a holder of no position at the settlement instant, and
				// counting it makes a zero-rate settlement look misdirected.
				if total == 0 {
					return
				}
				holder := record.ClientID
				if holder == 0 {
					holder = event.ClientID
				}
				perAccount := fundingDeltas[key]
				if perAccount == nil {
					perAccount = make(map[uint64]int64)
					fundingDeltas[key] = perAccount
				}
				perAccount[holder] += total
				if total > 0 {
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
				holder := record.ClientID
				if holder == 0 {
					holder = event.ClientID
				}
				optionPaidPerHolder[positionKey{event.VenueID, holder, record.Symbol}] += total
			}
		}
	}); err != nil {
		return nil, err
	}

	for key := range perpHistory {
		points := perpHistory[key]
		sort.Slice(points, func(i, j int) bool { return points[i].at < points[j].at })
		perpHistory[key] = points
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
		longsDebited, longsCredited := 0, 0
		for holder, delta := range fundingDeltas[key] {
			if delta == 0 {
				continue
			}
			cursor, cursorKnown := fundingCursors[key]
			if !cursorKnown {
				check.Undirected++
				continue
			}
			size, known := perpSideAt(perpHistory, key.venue, holder, key.timestamp, cursor.file, cursor.ordinal)
			if !known || size == 0 {
				check.Undirected++
				continue
			}
			check.Directed++
			// A positive rate charges the long side. Expected sign of the
			// account's movement is therefore -sign(rate)*sign(size).
			expectDebit := (check.Rate > 0) == (size > 0)
			if check.Rate == 0 {
				// Nothing should move at a zero rate; a movement is itself
				// the defect, and its direction is not meaningful.
				check.Misdirected++
				continue
			}
			if (delta < 0) != expectDebit {
				check.Misdirected++
			}
			if size > 0 {
				if delta < 0 {
					longsDebited++
				} else {
					longsCredited++
				}
			}
		}
		check.LongsPaid = longsDebited > longsCredited
		// Two independent conditions. The structural one holds without any
		// position information: a non-zero rate has to move money between two
		// sides, and a zero rate must move none. The directional one needs the
		// reconstructed sides and is the check that a reversed sign fails.
		structural := (check.Rate != 0 && bucket.payers > 0 && bucket.receivers > 0) ||
			(check.Rate == 0 && bucket.payers == 0 && bucket.receivers == 0)
		check.SignConsistent = structural && check.Misdirected == 0
		result.FundingMisdirected += check.Misdirected
		result.FundingUndirected += check.Undirected
		// Each account's share is an integer division, so a settlement across
		// n accounts can lose up to n units to truncation. Anything beyond
		// that is not rounding.
		bound := int64(bucket.payers + bucket.receivers)
		if check.Residual > bound || check.Residual < -bound {
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
			expected := mulDiv(check.Intrinsic, state.size, precision)
			check.ExpectedPayout += expected
			gap := optionPaidPerHolder[holderKey] - expected
			if gap > 1 || gap < -1 {
				check.HoldersMispaid++
				if absInt64(gap) > absInt64(check.WorstHolderGap) {
					check.WorstHolderGap = gap
				}
			}
		}
		entry := optionPaid[key]
		check.PaidOut = entry.amount
		check.Residual = check.PaidOut - check.ExpectedPayout
		// A worthless option may still show a unit or two of rounding dust, so
		// the test is against the number of accounts paid rather than zero.
		check.OutOfMoneyPaid = check.Intrinsic == 0 && absInt64(entry.amount) > int64(entry.accounts)
		// Each holder's payout is an integer division, so a contract with n
		// holders may lose up to n units to truncation.
		bound := int64(check.Holders)
		if entry.accounts > check.Holders {
			bound = int64(entry.accounts)
		}
		if check.Residual > bound || check.Residual < -bound {
			result.ExerciseBroken++
		}
		if check.OutOfMoneyPaid {
			result.WorthlessPaid++
		}
		if check.HoldersMispaid > 0 {
			result.HoldersMispaid += check.HoldersMispaid
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

// absInt64 is the magnitude of a signed amount.
func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
