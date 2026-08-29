package analysis

import (
	"encoding/json"
	"path/filepath"
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

// perpSideAt returns the position size an account held in the particular
// funded perpetual at a given instant. Funding is contract-specific; summing
// dated-future or another perpetual exposure on the venue would make a valid
// payment look correctly directed for the wrong reason.
func perpSideAt(history map[positionKey][]posPoint, venue string, client uint64, symbol string, at int64, file string, ordinal int64) (int64, bool) {
	points := history[positionKey{venue: venue, clientID: client, symbol: symbol}]
	return sizeAt(points, at, file, ordinal)
}

// sizeAt is the last position published before an evidence record. Same-file
// same-timestamp points before the record are eligible; points after it, and
// same-timestamp points from a physically separate file without a global order,
// are deliberately excluded.
func sizeAt(points []posPoint, at int64, file string, ordinal int64) (int64, bool) {
	var size int64
	known := false
	var selected evidenceOrder
	use := evidenceOrder{timestamp: at, file: file, ordinal: ordinal}
	for _, point := range points {
		pointOrder := evidenceOrder{timestamp: point.at, file: point.file, ordinal: point.ordinal}
		if point.at > at || !evidenceAfter(use, pointOrder) {
			continue
		}
		if !known || point.at > selected.timestamp || (point.at == selected.timestamp && evidenceBefore(selected, pointOrder)) {
			size = point.size
			selected = pointOrder
			known = true
		}
	}
	return size, known
}

// ratePayload is a published funding rate.
type ratePayload struct {
	Timestamp   int64  `json:"timestamp"`
	Symbol      string `json:"symbol"`
	Rate        int64  `json:"rate"`
	NextFunding int64  `json:"next_funding"`
	Interval    int64  `json:"interval"`
}

type fundingRatePoint struct {
	ratePayload
	order evidenceOrder
}

// DerivativeAuditOptions configures the funding and exercise audits.
type DerivativeAuditOptions struct {
	Files         []string
	FilesSelected bool
	BasePrecision int64
	// RequireExactReplay enables the r5 fail-closed funding evidence contract.
	// Legacy callers may retain timestamp-only funding semantics explicitly.
	RequireExactReplay bool
	// ExpectedFundingIntervalSeconds is the registered venue cadence. When set,
	// strict replay verifies both the published interval field and the complete
	// sequence of observed funding deadlines, including settlement instants.
	ExpectedFundingIntervalSeconds int64
}

// FundingInstantCheck is one venue's funding settlement, recomputed.
type FundingInstantCheck struct {
	VenueID   string `json:"venue_id"`
	Symbol    string `json:"symbol"`
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
	Residual         int64 `json:"residual"`
	Rate             int64 `json:"rate"`
	RateTimestamp    int64 `json:"rate_timestamp"`
	NextFunding      int64 `json:"next_funding"`
	RateInterval     int64 `json:"rate_interval"`
	RateAvailable    bool  `json:"rate_available"`
	TimingConsistent bool  `json:"timing_consistent"`
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
	// DuplicatePayments is the number of nonzero settlement postings in excess
	// of one per funded account at this contract and instant. It detects a
	// repeated funding operation even when repeated debits and credits still
	// net globally and point in the right direction.
	DuplicatePayments int `json:"duplicate_payments"`
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
	HoldersMispaid       int   `json:"holders_mispaid"`
	WorstHolderGap       int64 `json:"worst_holder_gap"`
	MissingPayouts       int   `json:"missing_payouts"`
	ExtraPayouts         int   `json:"extra_payouts"`
	DuplicatePayouts     int   `json:"duplicate_payouts"`
	UnknownPayoutHolders int   `json:"unknown_payout_holders"`
	// OutOfMoneyPaid flags an option that paid something while worthless.
	OutOfMoneyPaid bool `json:"out_of_money_paid"`
	// Unrepresentable means an exact signed integer recomputation could not be
	// represented in the artifact's int64 field. This is not a zero payout and
	// not a passing exercise check.
	Unrepresentable bool `json:"unrepresentable"`
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
	FundingMisdirected        int `json:"funding_misdirected"`
	FundingUndirected         int `json:"funding_undirected"`
	FundingDuplicatePayments  int `json:"funding_duplicate_payments"`
	FundingMissingRates       int `json:"funding_missing_rates"`
	FundingTimingFailures     int `json:"funding_timing_failures"`
	FundingEvidenceFailures   int `json:"funding_evidence_failures"`
	FundingArithmeticFailures int `json:"funding_arithmetic_failures"`
	ExerciseBroken            int `json:"exercise_broken"`
	// ExerciseTimingFailures counts option terminal announcements or payout
	// postings that were not emitted at the contract's declared expiry. A
	// payout amount can be arithmetically correct while still being a lifecycle
	// violation if it is credited early or late.
	ExerciseTimingFailures int `json:"exercise_timing_failures"`
	// HoldersMispaid is the count across every contract of holders whose own
	// payout did not match their own position.
	HoldersMispaid int `json:"holders_mispaid"`
	WorthlessPaid  int `json:"worthless_paid"`
	// ExerciseArithmeticFailures counts contracts the independent replay could
	// not represent exactly. They remain explicitly unresolved rather than
	// being compared after an overflow was rewritten as zero.
	ExerciseArithmeticFailures int `json:"exercise_arithmetic_failures"`
	ExerciseEvidenceFailures   int `json:"exercise_evidence_failures"`
	ExerciseMissingPayouts     int `json:"exercise_missing_payouts"`
	ExerciseExtraPayouts       int `json:"exercise_extra_payouts"`
	ExerciseDuplicatePayouts   int `json:"exercise_duplicate_payouts"`
	ExerciseUnknownPayouts     int `json:"exercise_unknown_payouts"`
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
		Action          string `json:"action"`
		Symbol          string `json:"symbol"`
		InstrumentType  string `json:"instrument_type"`
		Strike          int64  `json:"strike"`
		IsCall          bool   `json:"is_call"`
		SettlementPrice int64  `json:"settlement_price"`
		ExpiryNano      int64  `json:"expiry_nano"`
		Timestamp       int64  `json:"timestamp"`
	}
	type positionPayload struct {
		Timestamp int64  `json:"timestamp"`
		ClientID  uint64 `json:"client_id"`
		Symbol    string `json:"symbol"`
		NewSize   int64  `json:"new_size"`
	}
	files := opts.Files
	filesSelected := opts.FilesSelected
	if !filesSelected && files == nil {
		filesSelected = true
		for _, file := range r.files {
			if filepath.Base(file) == "derivatives.jsonl" {
				files = append(files, file)
			}
		}
	}

	var mu sync.Mutex
	options := make(map[markKey]instrumentPayload)
	rates := make(map[markKey][]fundingRatePoint)
	type fundingBucket struct {
		payers, receivers, movements int
		paid, received               int64
	}
	funding := make(map[instantKey]*fundingBucket)
	fundingEvidenceFailures := 0
	fundingArithmeticFailures := 0
	// Per-account funding movements, so the direction of the transfer can be
	// checked against each account's own side rather than against the sign of
	// the total, which is the same under any sign convention.
	fundingDeltas := make(map[instantKey]map[uint64]int64)
	type fundingCursor struct {
		timestamp int64
		file      string
		ordinal   int64
	}
	// The first balance record at a funding instant marks the event boundary.
	// All affected accounts are settled by that operation before later records
	// at the same timestamp. Keeping its physical position avoids attributing a
	// post-funding trade to the funded side.
	fundingCursors := make(map[instantKey]fundingCursor)
	fundingTimingFailures := 0
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
	optionPaymentCounts := make(map[markKey]map[uint64]int)
	type positionState struct {
		size, at int64
	}
	optionPositions := make(map[positionKey]*positionState)
	expiries := make(map[markKey]int64)
	exerciseTimingFailures := 0
	exerciseEvidenceFailures := 0

	// First pass: contract terms and expiry instants.
	if err := r.Scan(ScanOptions{Events: []string{"instrument_settled"}, Files: files, FilesSelected: filesSelected}, func(event Event) {
		var payload instrumentPayload
		if err := event.Decode(&payload); err != nil {
			if opts.RequireExactReplay {
				mu.Lock()
				exerciseEvidenceFailures++
				mu.Unlock()
			}
			return
		}
		if opts.RequireExactReplay && decodeRequiredJSON(event.Raw(), &payload,
			"action", "symbol", "instrument_type", "settlement_price", "expiry_nano", "timestamp") != nil {
			mu.Lock()
			exerciseEvidenceFailures++
			mu.Unlock()
			return
		}
		if opts.RequireExactReplay && (payload.Action != "settled" || payload.Symbol == "" ||
			(event.Symbol != "" && payload.Symbol != event.Symbol) || payload.InstrumentType == "") {
			mu.Lock()
			exerciseEvidenceFailures++
			mu.Unlock()
			return
		}
		if payload.InstrumentType != "OPTION" {
			return
		}
		if opts.RequireExactReplay && decodeRequiredJSON(event.Raw(), &payload, "strike", "is_call") != nil {
			mu.Lock()
			exerciseEvidenceFailures++
			mu.Unlock()
			return
		}
		mu.Lock()
		if payload.ExpiryNano <= 0 || payload.Timestamp == 0 || payload.Timestamp != event.SimTS || event.SimTS != payload.ExpiryNano {
			exerciseTimingFailures++
		}
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
		Files:         files,
		FilesSelected: filesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "funding_rate_update":
			var payload ratePayload
			if err := event.Decode(&payload); err != nil {
				if opts.RequireExactReplay {
					mu.Lock()
					fundingEvidenceFailures++
					mu.Unlock()
				}
				return
			}
			if opts.RequireExactReplay && decodeRequiredJSON(event.Raw(), &payload,
				"timestamp", "symbol", "rate", "next_funding", "interval") != nil {
				mu.Lock()
				fundingEvidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp <= 0 || payload.Timestamp != event.SimTS ||
				payload.Symbol == "" || (event.Symbol != "" && payload.Symbol != event.Symbol) ||
				payload.NextFunding <= event.SimTS || payload.Interval <= 0) {
				mu.Lock()
				fundingEvidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Symbol == "" {
				payload.Symbol = event.Symbol
			}
			if payload.Symbol == "" {
				return
			}
			mu.Lock()
			rateKey := markKey{event.VenueID, payload.Symbol}
			rates[rateKey] = append(rates[rateKey], fundingRatePoint{
				ratePayload: payload,
				order:       evidenceOrder{timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal},
			})
			mu.Unlock()
		case "position_update":
			var payload positionPayload
			if err := event.Decode(&payload); err != nil {
				if opts.RequireExactReplay {
					mu.Lock()
					exerciseEvidenceFailures++
					mu.Unlock()
				}
				return
			}
			if opts.RequireExactReplay && decodeRequiredJSON(event.Raw(), &payload,
				"timestamp", "client_id", "symbol", "new_size") != nil {
				mu.Lock()
				exerciseEvidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS ||
				payload.ClientID == 0 || event.ClientID != payload.ClientID ||
				(event.Symbol != "" && payload.Symbol != event.Symbol)) {
				mu.Lock()
				exerciseEvidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Symbol == "" {
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			key := positionKey{venue: event.VenueID, clientID: payload.ClientID, symbol: payload.Symbol}
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
			if err := event.Decode(&fill); err != nil {
				if opts.RequireExactReplay {
					mu.Lock()
					exerciseEvidenceFailures++
					mu.Unlock()
				}
				return
			}
			if opts.RequireExactReplay && decodeRequiredJSON(event.Raw(), &fill, "symbol", "qty", "side") != nil {
				mu.Lock()
				exerciseEvidenceFailures++
				mu.Unlock()
				return
			}
			if fill.Qty <= 0 || (opts.RequireExactReplay && (fill.Symbol == "" || (event.Symbol != "" && event.Symbol != fill.Symbol) ||
				(event.ClientID == 0) || (fill.Side != "BUY" && fill.Side != "SELL"))) {
				if opts.RequireExactReplay {
					mu.Lock()
					exerciseEvidenceFailures++
					mu.Unlock()
				}
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
			key := positionKey{venue: event.VenueID, clientID: event.ClientID, symbol: symbol}
			state := optionPositions[key]
			if state == nil {
				state = &positionState{}
				optionPositions[key] = state
			}
			nextSize, ok := exactAdd(state.size, signed)
			if !ok {
				if opts.RequireExactReplay {
					exerciseEvidenceFailures++
				}
				mu.Unlock()
				return
			}
			state.size = nextSize
			state.at = event.SimTS
			mu.Unlock()
		case "balance_change":
			var record balanceChangeRecord
			if err := event.Decode(&record); err != nil {
				if opts.RequireExactReplay {
					mu.Lock()
					fundingEvidenceFailures++
					exerciseEvidenceFailures++
					mu.Unlock()
				}
				return
			}
			if opts.RequireExactReplay && decodeRequiredJSON(event.Raw(), &record, "reason", "changes") != nil {
				mu.Lock()
				fundingEvidenceFailures++
				exerciseEvidenceFailures++
				mu.Unlock()
				return
			}
			relevantSettlement := record.Reason == "funding_settlement" ||
				record.Reason == "expiry_settlement"
			if opts.RequireExactReplay && relevantSettlement {
				if decodeRequiredJSON(event.Raw(), &record, "timestamp", "client_id", "symbol") != nil {
					mu.Lock()
					fundingEvidenceFailures++
					exerciseEvidenceFailures++
					mu.Unlock()
					return
				}
				var rawRecord struct {
					Changes []json.RawMessage `json:"changes"`
				}
				if json.Unmarshal(event.Raw(), &rawRecord) != nil {
					mu.Lock()
					fundingEvidenceFailures++
					exerciseEvidenceFailures++
					mu.Unlock()
					return
				}
				for _, rawChange := range rawRecord.Changes {
					var change balanceDeltaRecord
					if decodeRequiredJSON(rawChange, &change, "asset", "wallet", "old_balance", "new_balance", "delta") != nil {
						mu.Lock()
						fundingEvidenceFailures++
						exerciseEvidenceFailures++
						mu.Unlock()
						return
					}
				}
			}
			total := int64(0)
			for _, change := range record.Changes {
				next, ok := exactAdd(total, change.Delta)
				if !ok {
					if opts.RequireExactReplay {
						mu.Lock()
						fundingArithmeticFailures++
						mu.Unlock()
					}
					return
				}
				total = next
			}
			instant := record.Timestamp
			if instant == 0 {
				instant = event.SimTS
			}
			mu.Lock()
			defer mu.Unlock()
			if opts.RequireExactReplay && (record.Reason == "funding_settlement" || record.Reason == "expiry_settlement") && record.Symbol == "" {
				fundingEvidenceFailures++
				exerciseEvidenceFailures++
				return
			}
			switch {
			case record.Reason == "funding_settlement":
				if record.Symbol == "" || (opts.RequireExactReplay &&
					(record.Timestamp == 0 || record.Timestamp != event.SimTS ||
						(event.Symbol != "" && record.Symbol != event.Symbol) ||
						record.ClientID == 0 || record.ClientID != event.ClientID)) {
					if opts.RequireExactReplay {
						fundingEvidenceFailures++
					}
					return
				}
				key := instantKey{event.VenueID, instant, record.Symbol}
				cursor, seen := fundingCursors[key]
				if !seen || (cursor.file == event.File && event.Ordinal < cursor.ordinal) {
					fundingCursors[key] = fundingCursor{timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal}
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
				bucket.movements++
				holder := record.ClientID
				if holder == 0 {
					holder = event.ClientID
				}
				perAccount := fundingDeltas[key]
				if perAccount == nil {
					perAccount = make(map[uint64]int64)
					fundingDeltas[key] = perAccount
				}
				previous := perAccount[holder]
				next, ok := exactAdd(previous, total)
				if !ok {
					if opts.RequireExactReplay {
						fundingArithmeticFailures++
					}
					return
				}
				perAccount[holder] = next
				if total > 0 {
					bucket.receivers++
					next, ok := exactAdd(bucket.received, total)
					if !ok {
						if opts.RequireExactReplay {
							fundingArithmeticFailures++
						}
						return
					}
					bucket.received = next
					return
				}
				bucket.payers++
				paidTotal, ok := exactAdd(bucket.paid, total)
				if !ok {
					if opts.RequireExactReplay {
						fundingArithmeticFailures++
					}
					return
				}
				bucket.paid = paidTotal
			case record.Reason == "expiry_settlement" && isOptionSymbol(record.Symbol):
				key := markKey{event.VenueID, record.Symbol}
				if opts.RequireExactReplay && (record.Timestamp == 0 || record.Timestamp != event.SimTS ||
					(event.Symbol != "" && event.Symbol != record.Symbol) || record.ClientID == 0 || record.ClientID != event.ClientID) {
					exerciseEvidenceFailures++
					return
				}
				expiry, known := expiries[key]
				if !known || expiry <= 0 || instant != expiry || event.SimTS != expiry || (record.Timestamp != 0 && record.Timestamp != expiry) {
					exerciseTimingFailures++
				}
				entry := optionPaid[key]
				if total == 0 {
					return
				}
				nextAmount, ok := exactAdd(entry.amount, total)
				if !ok {
					exerciseEvidenceFailures++
					return
				}
				entry.amount = nextAmount
				entry.accounts++
				optionPaid[key] = entry
				holder := record.ClientID
				if holder == 0 {
					holder = event.ClientID
				}
				holderKey := positionKey{venue: event.VenueID, clientID: holder, symbol: record.Symbol}
				previous := optionPaidPerHolder[holderKey]
				nextHolderAmount, ok := exactAdd(previous, total)
				if !ok {
					exerciseEvidenceFailures++
					return
				}
				optionPaidPerHolder[holderKey] = nextHolderAmount
				payments := optionPaymentCounts[key]
				if payments == nil {
					payments = make(map[uint64]int)
					optionPaymentCounts[key] = payments
				}
				payments[holder]++
			}
		}
	}); err != nil {
		return nil, err
	}

	for key := range perpHistory {
		points := perpHistory[key]
		sort.Slice(points, func(i, j int) bool {
			if points[i].at != points[j].at {
				return points[i].at < points[j].at
			}
			if points[i].file != points[j].file {
				return points[i].file < points[j].file
			}
			return points[i].ordinal < points[j].ordinal
		})
		perpHistory[key] = points
	}
	if opts.RequireExactReplay && opts.ExpectedFundingIntervalSeconds > 0 {
		expectedSeconds := opts.ExpectedFundingIntervalSeconds
		expectedNanos, intervalOK := exactMul(expectedSeconds, 1_000_000_000)
		if !intervalOK {
			fundingArithmeticFailures++
		} else {
			for _, points := range rates {
				deadlines := make([]int64, 0, len(points))
				for _, point := range points {
					if point.Interval != expectedSeconds {
						fundingTimingFailures++
					}
					if point.NextFunding <= 0 {
						fundingTimingFailures++
						continue
					}
					deadlines = append(deadlines, point.NextFunding)
				}
				sort.Slice(deadlines, func(i, j int) bool { return deadlines[i] < deadlines[j] })
				uniqueDeadlines := deadlines[:0]
				for _, deadline := range deadlines {
					if len(uniqueDeadlines) > 0 && uniqueDeadlines[len(uniqueDeadlines)-1] == deadline {
						fundingTimingFailures++
						continue
					}
					uniqueDeadlines = append(uniqueDeadlines, deadline)
				}
				for index := 1; index < len(uniqueDeadlines); index++ {
					gap, gapOK := exactSub(uniqueDeadlines[index], uniqueDeadlines[index-1])
					if !gapOK || gap != expectedNanos {
						fundingTimingFailures++
					}
				}
			}
			byContract := make(map[markKey][]int64)
			for key := range funding {
				byContract[markKey{venue: key.venue, symbol: key.asset}] = append(byContract[markKey{venue: key.venue, symbol: key.asset}], key.timestamp)
			}
			for _, instants := range byContract {
				sort.Slice(instants, func(i, j int) bool { return instants[i] < instants[j] })
				for index := 1; index < len(instants); index++ {
					gap, gapOK := exactSub(instants[index], instants[index-1])
					if !gapOK || gap != expectedNanos {
						fundingTimingFailures++
					}
				}
			}
		}
	}

	precision := opts.BasePrecision
	if precision <= 0 {
		precision = 1
	}
	result := &DerivativeSemantics{}
	result.ExerciseTimingFailures = exerciseTimingFailures
	result.FundingEvidenceFailures = fundingEvidenceFailures
	result.FundingArithmeticFailures = fundingArithmeticFailures
	result.FundingTimingFailures = fundingTimingFailures
	result.ExerciseEvidenceFailures = exerciseEvidenceFailures

	selectFundingRate := func(points []fundingRatePoint, instant int64, cursor fundingCursor, cursorKnown bool) (fundingRatePoint, bool) {
		var selected fundingRatePoint
		found := false
		for _, point := range points {
			if opts.RequireExactReplay {
				if !cursorKnown || point.NextFunding != instant || point.Timestamp > instant ||
					!evidenceAfter(evidenceOrder{timestamp: cursor.timestamp, file: cursor.file, ordinal: cursor.ordinal}, point.order) {
					continue
				}
			} else if point.Timestamp > instant {
				continue
			}
			if !found || point.Timestamp > selected.Timestamp ||
				(point.Timestamp == selected.Timestamp && evidenceBefore(selected.order, point.order)) {
				selected = point
				found = true
			}
		}
		return selected, found
	}

	for key, bucket := range funding {
		cursor, cursorKnown := fundingCursors[key]
		check := FundingInstantCheck{
			VenueID: key.venue, Symbol: key.asset, Timestamp: key.timestamp,
			Payers: bucket.payers, Receivers: bucket.receivers,
			Paid: bucket.paid, Received: bucket.received,
		}
		if residual, ok := exactAdd(bucket.paid, bucket.received); ok {
			check.Residual = residual
		} else {
			result.FundingArithmeticFailures++
		}
		selectedRate, rateAvailable := selectFundingRate(rates[markKey{key.venue, key.asset}], key.timestamp, cursor, cursorKnown)
		check.RateAvailable = rateAvailable
		check.TimingConsistent = rateAvailable
		if rateAvailable {
			check.Rate = selectedRate.Rate
			check.RateTimestamp = selectedRate.Timestamp
			check.NextFunding = selectedRate.NextFunding
			check.RateInterval = selectedRate.Interval
			if opts.RequireExactReplay && opts.ExpectedFundingIntervalSeconds > 0 && selectedRate.Interval != opts.ExpectedFundingIntervalSeconds {
				check.TimingConsistent = false
				result.FundingTimingFailures++
			}
		} else if opts.RequireExactReplay {
			result.FundingMissingRates++
			result.FundingTimingFailures++
		}
		check.DuplicatePayments = bucket.movements - len(fundingDeltas[key])
		longsDebited, longsCredited := 0, 0
		for holder, delta := range fundingDeltas[key] {
			if delta == 0 {
				continue
			}
			if !cursorKnown {
				check.Undirected++
				continue
			}
			size, known := perpSideAt(perpHistory, key.venue, holder, key.asset, key.timestamp, cursor.file, cursor.ordinal)
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
		check.SignConsistent = structural && check.Misdirected == 0 && check.RateAvailable && check.TimingConsistent
		result.FundingMisdirected += check.Misdirected
		result.FundingUndirected += check.Undirected
		result.FundingDuplicatePayments += check.DuplicatePayments
		// Each account's share is an integer division, so a settlement across
		// n accounts can lose up to n units to truncation. Anything beyond
		// that is not rounding.
		bound := int64(bucket.payers + bucket.receivers)
		if check.Residual > bound || check.Residual < -bound || check.DuplicatePayments != 0 {
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
		if result.Funding[i].Symbol != result.Funding[j].Symbol {
			return result.Funding[i].Symbol < result.Funding[j].Symbol
		}
		return result.Funding[i].Timestamp < result.Funding[j].Timestamp
	})

	for key, terms := range options {
		intrinsic, intrinsicOK := intrinsicValue(terms.SettlementPrice, terms.Strike, terms.IsCall)
		check := ExerciseCheck{
			VenueID: key.venue, Symbol: key.symbol, Strike: terms.Strike,
			IsCall: terms.IsCall, SettlementPrice: terms.SettlementPrice,
			Intrinsic: intrinsic,
		}
		if !intrinsicOK {
			check.Unrepresentable = true
			result.ExerciseArithmeticFailures++
			result.Exercises = append(result.Exercises, check)
			continue
		}
		for holderKey, state := range optionPositions {
			if holderKey.venue != key.venue || holderKey.symbol != key.symbol || state.size == 0 {
				continue
			}
			check.Holders++
			netSize, ok := exactAdd(check.NetSize, state.size)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			check.NetSize = netSize
			expected, ok := mulDiv(check.Intrinsic, state.size, precision)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			expectedPayout, ok := exactAdd(check.ExpectedPayout, expected)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			check.ExpectedPayout = expectedPayout
			gap, ok := exactSub(optionPaidPerHolder[holderKey], expected)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			gapMagnitude, magnitudeOK := absoluteAuditInt64(gap)
			if !magnitudeOK {
				check.Unrepresentable = true
				continue
			}
			if gapMagnitude > 1 {
				check.HoldersMispaid++
				worstMagnitude, worstOK := absoluteAuditInt64(check.WorstHolderGap)
				if !worstOK || gapMagnitude > worstMagnitude {
					check.WorstHolderGap = gap
				}
			}
			payments := optionPaymentCounts[key][holderKey.clientID]
			if expected != 0 {
				switch {
				case payments == 0:
					check.MissingPayouts++
				case payments > 1:
					check.DuplicatePayouts += payments - 1
				}
			} else if payments > 0 {
				check.ExtraPayouts += payments
			}
		}
		for holder, payments := range optionPaymentCounts[key] {
			holderKey := positionKey{venue: key.venue, clientID: holder, symbol: key.symbol}
			state, known := optionPositions[holderKey]
			if (!known || state.size == 0) && payments > 0 {
				check.UnknownPayoutHolders += payments
			}
		}
		entry := optionPaid[key]
		check.PaidOut = entry.amount
		if residual, ok := exactSub(check.PaidOut, check.ExpectedPayout); ok {
			check.Residual = residual
		} else {
			check.Unrepresentable = true
		}
		if check.Unrepresentable {
			result.ExerciseArithmeticFailures++
			result.Exercises = append(result.Exercises, check)
			continue
		}
		// A worthless option may still show a unit or two of rounding dust, so
		// the test is against the number of accounts paid rather than zero.
		paidMagnitude, magnitudeOK := absoluteAuditInt64(entry.amount)
		if !magnitudeOK {
			check.Unrepresentable = true
			result.ExerciseArithmeticFailures++
			result.Exercises = append(result.Exercises, check)
			continue
		}
		check.OutOfMoneyPaid = check.Intrinsic == 0 && paidMagnitude > int64(entry.accounts)
		// Each holder's payout is an integer division, so a contract with n
		// holders may lose up to n units to truncation.
		bound := int64(check.Holders)
		if entry.accounts > check.Holders {
			bound = int64(entry.accounts)
		}
		if check.Residual > bound || check.Residual < -bound || check.MissingPayouts > 0 || check.ExtraPayouts > 0 || check.DuplicatePayouts > 0 || check.UnknownPayoutHolders > 0 {
			result.ExerciseBroken++
		}
		if check.OutOfMoneyPaid {
			result.WorthlessPaid++
		}
		if check.HoldersMispaid > 0 {
			result.HoldersMispaid += check.HoldersMispaid
		}
		result.ExerciseMissingPayouts += check.MissingPayouts
		result.ExerciseExtraPayouts += check.ExtraPayouts
		result.ExerciseDuplicatePayouts += check.DuplicatePayouts
		result.ExerciseUnknownPayouts += check.UnknownPayoutHolders
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
func intrinsicValue(settlement, strike int64, isCall bool) (int64, bool) {
	if isCall {
		if settlement > strike {
			return exactSub(settlement, strike)
		}
		return 0, true
	}
	if strike > settlement {
		return exactSub(strike, settlement)
	}
	return 0, true
}

func exactMul(a, b int64) (int64, bool) {
	return exactMulDiv(a, b, 1)
}
