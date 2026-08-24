package analysis

import (
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
)

// MarginCheckOptions states the independent contract used for the limited
// liquidation-trigger replay. The frozen multivenue ABC perpetual uses these
// values; callers must supply the contract explicitly rather than infer it
// from a liquidation_check record.
type MarginCheckOptions struct {
	Role                 string
	Symbol               string
	QuoteAsset           string
	BasePrecision        int64
	MaintenanceMarginBps int64
}

// DefaultMarginCheckOptions is the documented risk contract of the frozen
// ABC-PERP stress population. It is intentionally not a universal cross-margin
// replay: option marks, other contracts, and borrowed collateral require
// additional independently persisted inputs.
func DefaultMarginCheckOptions() MarginCheckOptions {
	return MarginCheckOptions{
		Role:                 "noise_flow",
		Symbol:               "ABC-PERP",
		QuoteAsset:           "USD",
		BasePrecision:        100_000_000,
		MaintenanceMarginBps: 500,
	}
}

// MarginCheckAudit independently replays the mark-time liquidation condition
// for the homogeneous, single-perpetual stress population. It does not use the
// engine's logged equity as its input. An account is excluded, rather than
// approximated, as soon as it has another derivative, option activity, a
// borrowed-balance movement, or a balance movement whose cross-file order
// cannot be established from preserved evidence.
//
// This is deliberately a coverage-qualified audit. A zero mismatch says the
// observable simple account path agreed with the stated margin formula; it
// does not certify the full multi-instrument or collateral model.
type MarginCheckAudit struct {
	Role                 string `json:"role"`
	Symbol               string `json:"symbol"`
	QuoteAsset           string `json:"quote_asset"`
	BasePrecision        int64  `json:"base_precision"`
	MaintenanceMarginBps int64  `json:"maintenance_margin_bps"`

	Candidates         int `json:"candidates"`
	EligibleCandidates int `json:"eligible_candidates"`
	ExcludedCandidates int `json:"excluded_candidates"`
	MarkUpdates        int `json:"mark_updates"`
	// UnsupportedMarkDomain counts published marks outside this frozen
	// positive-price perpetual contract. They are present observations, not
	// missing marks; affected candidates are excluded rather than replayed with
	// a numeric sentinel.
	UnsupportedMarkDomain int `json:"unsupported_mark_domain"`
	// AmbiguousMarkTimestampCollisions counts repeated ABC-PERP mark records at
	// one venue/time. General and derivative logs have no shared ordinal, so a
	// liquidation_check at that time could not be matched to one of those marks.
	// Every affected candidate is excluded rather than guessed about.
	AmbiguousMarkTimestampCollisions int `json:"ambiguous_mark_timestamp_collisions"`
	ActiveMarkChecks                 int `json:"active_mark_checks"`
	ExpectedBreaches                 int `json:"expected_breaches"`
	ObservedChecks                   int `json:"observed_checks"`
	MissingChecks                    int `json:"missing_checks"`
	UnexpectedChecks                 int `json:"unexpected_checks"`
	DuplicateChecks                  int `json:"duplicate_checks"`
	FieldChecks                      int `json:"field_checks"`
	FieldMismatches                  int `json:"field_mismatches"`
	MarkMismatches                   int `json:"mark_mismatches"`
	BalanceMismatches                int `json:"balance_mismatches"`
	ContributionMismatches           int `json:"contribution_mismatches"`
	EquityMismatches                 int `json:"equity_mismatches"`
	NotionalMismatches               int `json:"notional_mismatches"`
	MaintenanceMismatches            int `json:"maintenance_mismatches"`

	PositionChainFailures int                    `json:"position_chain_failures"`
	BalanceChainFailures  int                    `json:"balance_chain_failures"`
	ArithmeticFailures    int                    `json:"arithmetic_failures"`
	Exclusions            []MarginCheckExclusion `json:"exclusions"`
}

// MarginCheckExclusion records why a candidate could not be independently
// replayed from the preserved evidence. It is evidence of limited coverage,
// not a failed margin check.
type MarginCheckExclusion struct {
	VenueID  string   `json:"venue_id"`
	ClientID uint64   `json:"client_id"`
	Reasons  []string `json:"reasons"`
}

type marginReplayState struct {
	participant Participant
	initialTS   int64
	balance     int64
	size        int64
	entry       int64
	excluded    map[string]struct{}
}

type marginCheckKey struct {
	participant Participant
	timestamp   int64
}

type marginMarkTimestampKey struct {
	venueID   string
	timestamp int64
}

type replayedMarginCheck struct {
	mark, balance, contribution, equity, notional, maintenance int64
}

type loggedMarginCheck struct {
	mark, balance, contribution, equity, notional, maintenance int64
}

// MeasureMarginChecks replays the frozen ABC-PERP stress trigger for accounts
// satisfying the documented one-position/no-debt evidence contract.
func (r *Run) MeasureMarginChecks(opts MarginCheckOptions) (*MarginCheckAudit, error) {
	if opts.Role == "" || opts.Symbol == "" || opts.QuoteAsset == "" || opts.BasePrecision <= 0 || opts.MaintenanceMarginBps < 0 {
		return nil, fmt.Errorf("margin check audit: invalid contract")
	}
	result := &MarginCheckAudit{
		Role: opts.Role, Symbol: opts.Symbol, QuoteAsset: opts.QuoteAsset,
		BasePrecision: opts.BasePrecision, MaintenanceMarginBps: opts.MaintenanceMarginBps,
	}
	states := make(map[Participant]*marginReplayState)
	for _, row := range r.Report.InitialAccounts {
		if RoleGroup(row.Role) != opts.Role {
			continue
		}
		participant := Participant{VenueID: row.VenueID, ClientID: row.ClientID}
		state := &marginReplayState{participant: participant, initialTS: row.Account.Timestamp, excluded: make(map[string]struct{})}
		foundQuote := false
		for _, balance := range row.Account.PerpBalances {
			if balance.Asset != opts.QuoteAsset {
				continue
			}
			foundQuote = true
			state.balance = balance.NetAsset
			if balance.Borrowed != 0 {
				state.exclude("initial_borrowed_quote")
			}
		}
		if !foundQuote {
			state.exclude("missing_initial_quote_balance")
		}
		for _, position := range row.Account.Positions {
			if position.Size == 0 {
				continue
			}
			if position.Symbol != opts.Symbol {
				state.exclude("initial_other_position")
				continue
			}
			state.size, state.entry = position.Size, position.EntryPrice
		}
		states[participant] = state
		result.Candidates++
	}

	derivativeFiles := make([]string, 0, len(r.files))
	generalFiles := make([]string, 0, len(r.files))
	for _, file := range r.files {
		switch filepath.Base(file) {
		case "derivatives.jsonl":
			derivativeFiles = append(derivativeFiles, file)
		case "general.jsonl":
			generalFiles = append(generalFiles, file)
		}
	}
	if len(states) == 0 || len(derivativeFiles) == 0 {
		return result, nil
	}

	type positionPayload struct {
		Symbol   string `json:"symbol"`
		OldSize  int64  `json:"old_size"`
		NewSize  int64  `json:"new_size"`
		NewEntry int64  `json:"new_entry_price"`
	}
	type balancePayload struct {
		Changes []struct {
			Asset      string `json:"asset"`
			Wallet     string `json:"wallet"`
			OldBalance int64  `json:"old_balance"`
			NewBalance int64  `json:"new_balance"`
		} `json:"changes"`
	}
	type markPayload struct {
		MarkPrice int64 `json:"mark_price"`
	}
	type fillPayload struct {
		Symbol string `json:"symbol"`
	}

	expected := make(map[marginCheckKey]replayedMarginCheck)
	evaluations := make(map[Participant]int)
	seenMarks := make(map[marginMarkTimestampKey]struct{})
	marks := 0
	if err := r.Scan(ScanOptions{
		Files: derivativeFiles, FilesSelected: true, Workers: 1,
		Events: []string{"position_update", "balance_change", "mark_price_update", "OrderFill"},
	}, func(event Event) {
		participant := Participant{VenueID: event.VenueID, ClientID: event.ClientID}
		state := states[participant]
		switch event.Name {
		case "position_update":
			if state == nil {
				return
			}
			var payload positionPayload
			if event.Decode(&payload) != nil {
				state.exclude("malformed_position_update")
				return
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			if symbol != opts.Symbol {
				state.exclude("other_derivative_position")
				return
			}
			if payload.OldSize != state.size {
				result.PositionChainFailures++
				state.exclude("position_chain_mismatch")
			}
			state.size, state.entry = payload.NewSize, payload.NewEntry
		case "balance_change":
			if state == nil {
				return
			}
			var payload balancePayload
			if event.Decode(&payload) != nil {
				state.exclude("malformed_balance_change")
				return
			}
			for _, change := range payload.Changes {
				if change.Asset != opts.QuoteAsset {
					continue
				}
				switch change.Wallet {
				case "perp":
					if change.OldBalance != state.balance {
						result.BalanceChainFailures++
						state.exclude("perp_balance_chain_mismatch")
					}
					state.balance = change.NewBalance
				case "borrowed":
					state.exclude("borrowed_quote_movement")
				}
			}
		case "OrderFill":
			if state == nil {
				return
			}
			var payload fillPayload
			if event.Decode(&payload) != nil {
				state.exclude("malformed_fill")
				return
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			if symbol != opts.Symbol {
				state.exclude("other_instrument_fill")
			}
		case "mark_price_update":
			if event.Symbol != opts.Symbol {
				return
			}
			var payload markPayload
			if event.Decode(&payload) != nil {
				return
			}
			if payload.MarkPrice <= 0 {
				result.UnsupportedMarkDomain++
				for _, current := range states {
					if current.participant.VenueID == event.VenueID {
						current.exclude("unsupported_positive_mark_domain")
					}
				}
				return
			}
			markKey := marginMarkTimestampKey{venueID: event.VenueID, timestamp: event.SimTS}
			if _, duplicate := seenMarks[markKey]; duplicate {
				result.AmbiguousMarkTimestampCollisions++
				for _, current := range states {
					if current.participant.VenueID == event.VenueID {
						current.exclude("ambiguous_same_timestamp_mark")
					}
				}
				return
			}
			seenMarks[markKey] = struct{}{}
			marks++
			for _, current := range states {
				if current.isExcluded() || current.participant.VenueID != event.VenueID || current.size == 0 {
					continue
				}
				evaluations[current.participant]++
				check, ok := independentMarginCheck(current.balance, current.size, current.entry, payload.MarkPrice, opts)
				if !ok {
					result.ArithmeticFailures++
					current.exclude("unrepresentable_margin_arithmetic")
					continue
				}
				if check.equity < check.maintenance {
					expected[marginCheckKey{participant: current.participant, timestamp: event.SimTS}] = check
				}
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("margin check audit: derivative scan: %w", err)
	}

	type liquidationPayload struct {
		Symbol                 string `json:"symbol"`
		MarkPrice              int64  `json:"mark_price"`
		Balance                int64  `json:"balance"`
		DerivativeContribution int64  `json:"derivative_equity_contribution"`
		Equity                 int64  `json:"equity"`
		Notional               int64  `json:"notional"`
		MaintenanceMargin      int64  `json:"maintenance_margin"`
	}
	observed := make(map[marginCheckKey][]loggedMarginCheck)
	if err := r.Scan(ScanOptions{
		Files: generalFiles, FilesSelected: true, Workers: 1,
		Events: []string{"balance_change", "liquidation_check"},
	}, func(event Event) {
		participant := Participant{VenueID: event.VenueID, ClientID: event.ClientID}
		state := states[participant]
		if state == nil {
			return
		}
		switch event.Name {
		case "balance_change":
			// General logs have no total order relative to the derivative book.
			// Initial prefunding is represented in the report; a later relevant
			// movement makes the account conservatively out of scope.
			if event.SimTS == state.initialTS {
				return
			}
			var payload balancePayload
			if event.Decode(&payload) != nil {
				state.exclude("malformed_general_balance_change")
				return
			}
			for _, change := range payload.Changes {
				if change.Asset == opts.QuoteAsset && (change.Wallet == "perp" || change.Wallet == "borrowed") {
					state.exclude("cross_file_quote_or_borrow_movement")
				}
			}
		case "liquidation_check":
			var payload liquidationPayload
			if event.Decode(&payload) != nil || payload.Symbol != opts.Symbol {
				return
			}
			key := marginCheckKey{participant: participant, timestamp: event.SimTS}
			observed[key] = append(observed[key], loggedMarginCheck{
				mark: payload.MarkPrice, balance: payload.Balance, contribution: payload.DerivativeContribution,
				equity: payload.Equity, notional: payload.Notional, maintenance: payload.MaintenanceMargin,
			})
		}
	}); err != nil {
		return nil, fmt.Errorf("margin check audit: general scan: %w", err)
	}

	for _, state := range states {
		if state.isExcluded() {
			result.ExcludedCandidates++
			result.Exclusions = append(result.Exclusions, state.exclusion())
			continue
		}
		result.EligibleCandidates++
		result.ActiveMarkChecks += evaluations[state.participant]
	}
	result.MarkUpdates = marks

	for key, check := range expected {
		state := states[key.participant]
		if state == nil || state.isExcluded() {
			continue
		}
		result.ExpectedBreaches++
		rows := observed[key]
		if len(rows) == 0 {
			result.MissingChecks++
			continue
		}
		result.ObservedChecks += len(rows)
		if len(rows) > 1 {
			result.DuplicateChecks += len(rows) - 1
		}
		result.FieldChecks++
		if result.recordMarginDifferences(check, rows[0]) {
			result.FieldMismatches++
		}
	}
	for key, rows := range observed {
		state := states[key.participant]
		if state == nil || state.isExcluded() {
			continue
		}
		if _, exists := expected[key]; !exists {
			result.ObservedChecks += len(rows)
			result.UnexpectedChecks += len(rows)
		}
	}
	sort.Slice(result.Exclusions, func(i, j int) bool {
		if result.Exclusions[i].VenueID != result.Exclusions[j].VenueID {
			return result.Exclusions[i].VenueID < result.Exclusions[j].VenueID
		}
		return result.Exclusions[i].ClientID < result.Exclusions[j].ClientID
	})
	return result, nil
}

func (s *marginReplayState) exclude(reason string) { s.excluded[reason] = struct{}{} }

func (s *marginReplayState) isExcluded() bool { return len(s.excluded) != 0 }

func (s *marginReplayState) exclusion() MarginCheckExclusion {
	reasons := make([]string, 0, len(s.excluded))
	for reason := range s.excluded {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	return MarginCheckExclusion{VenueID: s.participant.VenueID, ClientID: s.participant.ClientID, Reasons: reasons}
}

func (r *MarginCheckAudit) recordMarginDifferences(expected replayedMarginCheck, observed loggedMarginCheck) bool {
	mismatch := false
	if expected.mark != observed.mark {
		r.MarkMismatches++
		mismatch = true
	}
	if expected.balance != observed.balance {
		r.BalanceMismatches++
		mismatch = true
	}
	if expected.contribution != observed.contribution {
		r.ContributionMismatches++
		mismatch = true
	}
	if expected.equity != observed.equity {
		r.EquityMismatches++
		mismatch = true
	}
	if expected.notional != observed.notional {
		r.NotionalMismatches++
		mismatch = true
	}
	if expected.maintenance != observed.maintenance {
		r.MaintenanceMismatches++
		mismatch = true
	}
	return mismatch
}

func independentMarginCheck(balance, size, entry, mark int64, opts MarginCheckOptions) (replayedMarginCheck, bool) {
	if mark <= 0 || size == -1<<63 {
		return replayedMarginCheck{}, false
	}
	quantity := size
	priceDelta, ok := exactSub(mark, entry)
	if !ok {
		return replayedMarginCheck{}, false
	}
	if quantity < 0 {
		quantity = -quantity
		priceDelta, ok = exactSub(entry, mark)
		if !ok {
			return replayedMarginCheck{}, false
		}
	}
	contribution, ok := exactMulDiv(quantity, priceDelta, opts.BasePrecision)
	if !ok {
		return replayedMarginCheck{}, false
	}
	notional, ok := exactMulDiv(absMarginSize(size), mark, opts.BasePrecision)
	if !ok {
		return replayedMarginCheck{}, false
	}
	maintenance, ok := exactMulDiv(notional, opts.MaintenanceMarginBps, 10_000)
	if !ok {
		return replayedMarginCheck{}, false
	}
	equity, ok := exactAdd(balance, contribution)
	if !ok {
		return replayedMarginCheck{}, false
	}
	return replayedMarginCheck{
		mark: mark, balance: balance, contribution: contribution, equity: equity,
		notional: notional, maintenance: maintenance,
	}, true
}

// exactMulDiv uses math/big rather than the simulator's arithmetic helper.
// The validator therefore has an independent integer implementation of the
// multiplication/division used in the risk condition.
func exactMulDiv(a, b, denominator int64) (int64, bool) {
	if denominator <= 0 {
		return 0, false
	}
	var product big.Int
	product.Mul(big.NewInt(a), big.NewInt(b))
	product.Quo(&product, big.NewInt(denominator))
	if !product.IsInt64() {
		return 0, false
	}
	return product.Int64(), true
}

func exactAdd(a, b int64) (int64, bool) {
	var sum big.Int
	sum.Add(big.NewInt(a), big.NewInt(b))
	if !sum.IsInt64() {
		return 0, false
	}
	return sum.Int64(), true
}

func exactSub(a, b int64) (int64, bool) {
	var difference big.Int
	difference.Sub(big.NewInt(a), big.NewInt(b))
	if !difference.IsInt64() {
		return 0, false
	}
	return difference.Int64(), true
}

func absMarginSize(value int64) int64 {
	if value >= 0 {
		return value
	}
	return -value
}
