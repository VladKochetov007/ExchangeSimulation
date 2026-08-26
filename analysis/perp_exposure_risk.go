package analysis

import (
	"fmt"
	"sort"
)

// PerpExposureRiskOptions declares the independently replayed contract for a
// directional perpetual participant.  This is deliberately separate from
// MarginCheckOptions: P7d permits a quote-margin loan, whereas the older
// margin audit is only valid for accounts with no debt or cross-instrument
// movement.
type PerpExposureRiskOptions struct {
	Role                 string
	Symbol               string
	QuoteAsset           string
	BasePrecision        int64
	MaintenanceMarginBps int64
}

// DefaultPerpExposureRiskOptions is the P7d ABC-PERP risk contract.  The
// values are explicit inputs to the audit, not values inferred from logged
// liquidation_check fields.
func DefaultPerpExposureRiskOptions() PerpExposureRiskOptions {
	return PerpExposureRiskOptions{
		Role: "perp_exposure_hedger", Symbol: "ABC-PERP", QuoteAsset: "USD",
		BasePrecision: 100_000_000, MaintenanceMarginBps: 500,
	}
}

// PerpExposureRiskAudit is an independent replay of the participant-specific
// mark/equity/maintenance path.  A valid result means that all relevant
// evidence was reconstructible under the declared one-perpetual-plus-borrow
// contract; it does not claim that a risk event must occur.
type PerpExposureRiskAudit struct {
	Role                 string `json:"role"`
	Symbol               string `json:"symbol"`
	QuoteAsset           string `json:"quote_asset"`
	BasePrecision        int64  `json:"base_precision"`
	MaintenanceMarginBps int64  `json:"maintenance_margin_bps"`

	Candidates              int `json:"candidates"`
	MarkUpdates             int `json:"mark_updates"`
	DuplicateMarkTimestamps int `json:"duplicate_mark_timestamps"`
	ExpectedBreaches        int `json:"expected_breaches"`
	ObservedChecks          int `json:"observed_checks"`
	MissingChecks           int `json:"missing_checks"`
	UnexpectedChecks        int `json:"unexpected_checks"`
	DuplicateChecks         int `json:"duplicate_checks"`
	FieldChecks             int `json:"field_checks"`
	FieldMismatches         int `json:"field_mismatches"`
	MarkMismatches          int `json:"mark_mismatches"`
	BalanceMismatches       int `json:"balance_mismatches"`
	ContributionMismatches  int `json:"contribution_mismatches"`
	EquityMismatches        int `json:"equity_mismatches"`
	NotionalMismatches      int `json:"notional_mismatches"`
	MaintenanceMismatches   int `json:"maintenance_mismatches"`
	MarkDomainFailures      int `json:"mark_domain_failures"`
	ArithmeticFailures      int `json:"arithmetic_failures"`
	PositionChainFailures   int `json:"position_chain_failures"`
	BalanceChainFailures    int `json:"balance_chain_failures"`
	CrossFileAmbiguities    int `json:"cross_file_ambiguities"`
	MalformedRecords        int `json:"malformed_records"`

	BorrowEvents             int `json:"borrow_events"`
	BorrowAmountMismatches   int `json:"borrow_amount_mismatches"`
	UnexpectedAutoBorrows    int `json:"unexpected_auto_borrows"`
	NegativeDebtObservations int `json:"negative_debt_observations"`

	ParticipantLiquidations    int   `json:"participant_liquidations"`
	UnexpectedLiquidations     int   `json:"unexpected_liquidations"`
	ParticipantMarginCalls     int   `json:"participant_margin_calls"`
	ParticipantDeficits        int   `json:"participant_deficits"`
	TotalDeficit               int64 `json:"total_deficit"`
	ParticipantInsuranceEvents int   `json:"participant_insurance_events"`
	InsuranceMismatches        int   `json:"insurance_mismatches"`
	PositionPathFailures       int   `json:"position_path_failures"`
	TerminalStateMismatches    int   `json:"terminal_state_mismatches"`

	Participants []PerpExposureRiskParticipant `json:"participants,omitempty"`
	Checks       []PerpExposureRiskCheck       `json:"checks,omitempty"`
	Valid        bool                          `json:"valid"`
}

// PerpExposureRiskParticipant is a compact per-venue/account endpoint.  It
// keeps the three venues separate so one healthy account cannot hide another
// account's missing debt or liquidation evidence.
type PerpExposureRiskParticipant struct {
	VenueID          string `json:"venue_id"`
	ClientID         uint64 `json:"client_id"`
	InitialPosition  int64  `json:"initial_position"`
	TerminalPosition int64  `json:"terminal_position"`
	InitialWallet    int64  `json:"initial_wallet"`
	TerminalWallet   int64  `json:"terminal_wallet"`
	TerminalDebt     int64  `json:"terminal_debt"`
	MarkObservations int    `json:"mark_observations"`
	ExpectedBreaches int    `json:"expected_breaches"`
	ObservedChecks   int    `json:"observed_checks"`
	Liquidations     int    `json:"liquidations"`
	MarginCalls      int    `json:"margin_calls"`
	Deficit          int64  `json:"deficit"`
	BorrowedQuote    int64  `json:"borrowed_quote"`
}

// PerpExposureRiskCheck records the exact state used for one independent
// maintenance comparison.  Debt is included even though the venue's
// liquidation_check wire record only exposes gross balance.
type PerpExposureRiskCheck struct {
	VenueID      string `json:"venue_id"`
	ClientID     uint64 `json:"client_id"`
	Timestamp    int64  `json:"timestamp"`
	Mark         int64  `json:"mark"`
	Position     int64  `json:"position"`
	Entry        int64  `json:"entry"`
	Wallet       int64  `json:"wallet"`
	Debt         int64  `json:"debt"`
	Contribution int64  `json:"contribution"`
	Equity       int64  `json:"equity"`
	Notional     int64  `json:"notional"`
	Maintenance  int64  `json:"maintenance"`
	Expected     bool   `json:"expected"`
	Observed     bool   `json:"observed"`
	FieldsMatch  bool   `json:"fields_match"`
}

type perpRiskParticipantKey struct {
	venue  string
	client uint64
}

type perpRiskTimeKey struct {
	participant perpRiskParticipantKey
	ts          int64
}

type perpRiskMark struct {
	venue string
	ts    int64
	mark  int64
}

type perpRiskState struct {
	wallet int64
	debt   int64
	pos    int64
	entry  int64
}

type perpRiskSnapshot struct {
	ts   int64
	pre  perpRiskState
	post perpRiskState
}

type perpRiskPositionUpdate struct {
	file    string
	ordinal int64
	ts      int64
	payload perpRiskPositionPayload
}

type perpRiskTimelineEvent struct {
	file    string
	ordinal int64
	ts      int64
	name    string
	balance *perpRiskBalancePayload
	pos     *perpRiskPositionPayload
}

type perpRiskPositionPayload struct {
	Symbol        string `json:"symbol"`
	OldSize       int64  `json:"old_size"`
	OldEntryPrice int64  `json:"old_entry_price"`
	NewSize       int64  `json:"new_size"`
	NewEntryPrice int64  `json:"new_entry_price"`
}

type perpRiskBalancePayload struct {
	Changes []struct {
		Asset      string `json:"asset"`
		Wallet     string `json:"wallet"`
		OldBalance int64  `json:"old_balance"`
		NewBalance int64  `json:"new_balance"`
		Delta      int64  `json:"delta"`
	} `json:"changes"`
}

type perpRiskMarkPayload struct {
	MarkPrice int64 `json:"mark_price"`
}

type perpRiskCheckPayload struct {
	Symbol                 string `json:"symbol"`
	MarkPrice              int64  `json:"mark_price"`
	Balance                int64  `json:"balance"`
	DerivativeContribution int64  `json:"derivative_equity_contribution"`
	Equity                 int64  `json:"equity"`
	Notional               int64  `json:"notional"`
	MaintenanceMargin      int64  `json:"maintenance_margin"`
}

type perpRiskLiquidationPayload struct {
	Symbol        string `json:"symbol"`
	PositionSize  int64  `json:"position_size"`
	FillPrice     int64  `json:"fill_price"`
	RemainingDebt int64  `json:"remaining_debt"`
}

type perpRiskBorrowPayload struct {
	ClientID uint64 `json:"client_id"`
	Asset    string `json:"asset"`
	Amount   int64  `json:"amount"`
	Reason   string `json:"reason"`
}

type perpRiskInsurancePayload struct {
	Symbol string `json:"symbol"`
	Delta  int64  `json:"delta"`
}

type perpRiskMarginCallPayload struct {
	Symbol string `json:"symbol"`
}

// MeasurePerpExposureRisk independently replays the P7d participant risk
// contract. State is grouped by simulated timestamp: a liquidation_check is
// compared with the state before that timestamp's derivative mutations. This
// is the boundary emitted by CheckLiquidations before forced-close fills, and
// avoids pretending that separately persisted general/derivative files have a
// total order they do not carry.
func (r *Run) MeasurePerpExposureRisk(opts PerpExposureRiskOptions) (*PerpExposureRiskAudit, error) {
	if opts.Role == "" || opts.Symbol == "" || opts.QuoteAsset == "" || opts.BasePrecision <= 0 || opts.MaintenanceMarginBps < 0 || opts.MaintenanceMarginBps > 10_000 {
		return nil, fmt.Errorf("perpetual exposure risk: invalid contract")
	}
	result := &PerpExposureRiskAudit{
		Role: opts.Role, Symbol: opts.Symbol, QuoteAsset: opts.QuoteAsset,
		BasePrecision: opts.BasePrecision, MaintenanceMarginBps: opts.MaintenanceMarginBps,
	}

	candidates := make(map[perpRiskParticipantKey]perpRiskState)
	participantRows := make(map[perpRiskParticipantKey]*PerpExposureRiskParticipant)
	for _, row := range r.Report.InitialAccounts {
		if RoleGroup(row.Role) != opts.Role {
			continue
		}
		key := perpRiskParticipantKey{venue: row.VenueID, client: row.ClientID}
		if _, exists := candidates[key]; exists {
			result.MalformedRecords++
			continue
		}
		state := perpRiskState{}
		foundQuote := false
		for _, balance := range row.Account.PerpBalances {
			if balance.Asset != opts.QuoteAsset {
				continue
			}
			foundQuote = true
			gross, ok := exactAdd(balance.NetAsset, balance.Borrowed)
			if !ok {
				result.ArithmeticFailures++
				continue
			}
			state.wallet, state.debt = gross, balance.Borrowed
			if state.debt < 0 {
				result.NegativeDebtObservations++
			}
		}
		if !foundQuote {
			result.MalformedRecords++
		}
		for _, position := range row.Account.Positions {
			if position.Symbol != opts.Symbol && position.Size != 0 {
				result.MalformedRecords++
				continue
			}
			if position.Symbol == opts.Symbol && position.Size != 0 {
				state.pos, state.entry = position.Size, position.EntryPrice
			}
		}
		candidates[key] = state
		participantRows[key] = &PerpExposureRiskParticipant{
			VenueID: key.venue, ClientID: key.client,
			InitialPosition: state.pos, InitialWallet: state.wallet,
		}
		result.Candidates++
	}
	if len(candidates) == 0 {
		return result, nil
	}

	marks := make(map[string]map[int64][]perpRiskMark)
	checks := make(map[perpRiskTimeKey][]perpRiskCheckPayload)
	liquidations := make(map[perpRiskParticipantKey][]struct {
		file    string
		ordinal int64
		ts      int64
		payload perpRiskLiquidationPayload
	})
	marginCalls := make(map[perpRiskParticipantKey]int)
	borrows := make(map[perpRiskTimeKey][]perpRiskBorrowPayload)
	insurance := make(map[perpRiskTimeKey][]perpRiskInsurancePayload)
	timeline := make(map[perpRiskParticipantKey][]perpRiskTimelineEvent)

	events := []string{"mark_price_update", "position_update", "balance_change", "liquidation_check", "liquidation", "margin_call", "margincall", "margin_check", "borrow", "insurance_fund"}
	if err := r.Scan(ScanOptions{Workers: 1, Events: events}, func(event Event) {
		key := perpRiskParticipantKey{venue: event.VenueID, client: event.ClientID}
		if event.Name == "mark_price_update" {
			if event.Symbol != opts.Symbol {
				return
			}
			var payload perpRiskMarkPayload
			if event.Decode(&payload) != nil {
				result.MalformedRecords++
				return
			}
			if marks[event.VenueID] == nil {
				marks[event.VenueID] = make(map[int64][]perpRiskMark)
			}
			marks[event.VenueID][event.SimTS] = append(marks[event.VenueID][event.SimTS], perpRiskMark{venue: event.VenueID, ts: event.SimTS, mark: payload.MarkPrice})
			return
		}
		if event.Name == "insurance_fund" {
			var payload perpRiskInsurancePayload
			if event.Decode(&payload) != nil || payload.Symbol != opts.Symbol {
				return
			}
			insurance[perpRiskTimeKey{participant: perpRiskParticipantKey{venue: event.VenueID}, ts: event.SimTS}] = append(insurance[perpRiskTimeKey{participant: perpRiskParticipantKey{venue: event.VenueID}, ts: event.SimTS}], payload)
			return
		}
		if _, candidate := candidates[key]; !candidate {
			return
		}
		switch event.Name {
		case "position_update":
			var payload perpRiskPositionPayload
			if event.Decode(&payload) != nil || payload.Symbol != opts.Symbol {
				result.MalformedRecords++
				return
			}
			copy := payload
			timeline[key] = append(timeline[key], perpRiskTimelineEvent{file: event.File, ordinal: event.Ordinal, ts: event.SimTS, name: event.Name, pos: &copy})
		case "balance_change":
			var payload perpRiskBalancePayload
			if event.Decode(&payload) != nil {
				result.MalformedRecords++
				return
			}
			relevant := false
			for _, change := range payload.Changes {
				if change.Asset == opts.QuoteAsset && (change.Wallet == "perp" || change.Wallet == "borrowed") {
					relevant = true
					break
				}
			}
			if !relevant {
				return
			}
			copy := payload
			timeline[key] = append(timeline[key], perpRiskTimelineEvent{file: event.File, ordinal: event.Ordinal, ts: event.SimTS, name: event.Name, balance: &copy})
		case "liquidation_check":
			var payload perpRiskCheckPayload
			if event.Decode(&payload) != nil || payload.Symbol != opts.Symbol {
				result.MalformedRecords++
				return
			}
			checks[perpRiskTimeKey{participant: key, ts: event.SimTS}] = append(checks[perpRiskTimeKey{participant: key, ts: event.SimTS}], payload)
		case "liquidation":
			var payload perpRiskLiquidationPayload
			if event.Decode(&payload) != nil || payload.Symbol != opts.Symbol {
				result.MalformedRecords++
				return
			}
			liquidations[key] = append(liquidations[key], struct {
				file    string
				ordinal int64
				ts      int64
				payload perpRiskLiquidationPayload
			}{file: event.File, ordinal: event.Ordinal, ts: event.SimTS, payload: payload})
		case "margin_call", "margincall", "margin_check":
			// MarginCall is delivered to a handler in the simulator and is not
			// normally persisted. If a gateway does persist one of these names,
			// count it as participant evidence rather than silently ignoring it.
			var payload perpRiskMarginCallPayload
			if event.Decode(&payload) == nil && (payload.Symbol == "" || payload.Symbol == opts.Symbol) {
				marginCalls[key]++
			}
		case "borrow":
			var payload perpRiskBorrowPayload
			if event.Decode(&payload) != nil || payload.ClientID == 0 || payload.ClientID != event.ClientID || payload.Asset == "" || payload.Amount <= 0 {
				result.MalformedRecords++
				return
			}
			if payload.Reason != "auto_perp" {
				return
			}
			borrows[perpRiskTimeKey{participant: key, ts: event.SimTS}] = append(borrows[perpRiskTimeKey{participant: key, ts: event.SimTS}], payload)
		case "insurance_fund":
			// Handled above; kept for exhaustiveness.
		}
	}); err != nil {
		return nil, fmt.Errorf("perpetual exposure risk: evidence scan: %w", err)
	}

	// Reconstruct each participant's wallet/debt/position state in deterministic
	// file/ordinal order. Cross-file same-timestamp mutations of one field are
	// rejected as ambiguous rather than given an invented causal order.
	snapshots := make(map[perpRiskParticipantKey][]perpRiskSnapshot)
	positionUpdates := make(map[perpRiskParticipantKey][]perpRiskPositionUpdate)
	borrowDebtDeltas := make(map[perpRiskTimeKey]int64)
	for key, rows := range timeline {
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].ts != rows[j].ts {
				return rows[i].ts < rows[j].ts
			}
			if rows[i].file != rows[j].file {
				return rows[i].file < rows[j].file
			}
			return rows[i].ordinal < rows[j].ordinal
		})
		state := candidates[key]
		for index := 0; index < len(rows); {
			ts := rows[index].ts
			if ts <= initialTimestamp(r, key) {
				for index < len(rows) && rows[index].ts == ts {
					index++
				}
				continue
			}
			end := index
			for end < len(rows) && rows[end].ts == ts {
				end++
			}
			pre := state
			var borrowedDelta int64
			pending := append([]perpRiskTimelineEvent(nil), rows[index:end]...)
			for len(pending) > 0 {
				ready := make([]int, 0, len(pending))
				for pendingIndex, row := range pending {
					if perpRiskEventReady(row, state, opts.QuoteAsset) {
						ready = append(ready, pendingIndex)
					}
				}
				selected := 0
				if len(ready) > 0 {
					selected = ready[0]
					// A unique continuity match gives an evidence-derived order
					// even when records live in different files. Multiple ready
					// events touching the same state field are genuinely
					// ambiguous and are rejected rather than guessed about.
					for _, candidate := range ready[1:] {
						if perpRiskEventsOverlap(pending[selected], pending[candidate], opts.QuoteAsset) {
							result.CrossFileAmbiguities++
						}
					}
				} else {
					// Preserve the deterministic physical order for diagnostics,
					// but record the broken continuity before applying it.
					result.BalanceChainFailures++
				}
				row := pending[selected]
				if row.pos != nil {
					payload := *row.pos
					if payload.OldSize != state.pos || payload.OldEntryPrice != state.entry {
						result.PositionChainFailures++
					}
					state.pos, state.entry = payload.NewSize, payload.NewEntryPrice
					positionUpdates[key] = append(positionUpdates[key], perpRiskPositionUpdate{file: row.file, ordinal: row.ordinal, ts: ts, payload: payload})
				}
				if row.balance != nil {
					for _, change := range row.balance.Changes {
						if change.Asset != opts.QuoteAsset || (change.Wallet != "perp" && change.Wallet != "borrowed") {
							continue
						}
						if change.Wallet == "perp" {
							if change.OldBalance != state.wallet {
								result.BalanceChainFailures++
							}
							state.wallet = change.NewBalance
						} else {
							if change.OldBalance != state.debt {
								result.BalanceChainFailures++
							}
							delta, ok := exactSub(change.NewBalance, change.OldBalance)
							if !ok {
								result.ArithmeticFailures++
							} else if next, ok := exactAdd(borrowedDelta, delta); ok {
								borrowedDelta = next
							} else {
								result.ArithmeticFailures++
							}
							state.debt = change.NewBalance
							if state.debt < 0 {
								result.NegativeDebtObservations++
							}
						}
					}
				}
				pending = append(pending[:selected], pending[selected+1:]...)
			}
			borrowDebtDeltas[perpRiskTimeKey{participant: key, ts: ts}] = borrowedDelta
			snapshots[key] = append(snapshots[key], perpRiskSnapshot{ts: ts, pre: pre, post: state})
			index = end
		}
	}

	stateAt := func(key perpRiskParticipantKey, ts int64) perpRiskState {
		state := candidates[key]
		for _, snapshot := range snapshots[key] {
			if snapshot.ts > ts {
				break
			}
			if snapshot.ts == ts {
				return snapshot.pre
			}
			state = snapshot.post
		}
		return state
	}

	// Independently predict every maintenance breach from published marks.
	expected := make(map[perpRiskTimeKey]replayedMarginCheck)
	for venue, byTime := range marks {
		for ts, rows := range byTime {
			result.MarkUpdates += len(rows)
			if len(rows) != 1 {
				result.DuplicateMarkTimestamps += len(rows) - 1
				continue
			}
			mark := rows[0].mark
			if mark <= 0 {
				result.MarkDomainFailures++
				continue
			}
			for key := range candidates {
				if key.venue != venue {
					continue
				}
				state := stateAt(key, ts)
				participant := participantRows[key]
				if participant != nil {
					participant.MarkObservations++
				}
				if state.pos == 0 {
					continue
				}
				check, ok := independentPerpExposureMarginCheck(state.wallet, state.debt, state.pos, state.entry, mark, opts)
				if !ok {
					result.ArithmeticFailures++
					continue
				}
				if check.equity < check.maintenance {
					expected[perpRiskTimeKey{participant: key, ts: ts}] = check
					result.ExpectedBreaches++
					if participant != nil {
						participant.ExpectedBreaches++
					}
				}
			}
		}
	}

	for key, rows := range checks {
		state := stateAt(key.participant, key.ts)
		marksAt := marks[key.participant.venue][key.ts]
		if len(rows) > 1 {
			result.DuplicateChecks += len(rows) - 1
		}
		if len(rows) == 0 {
			continue
		}
		result.ObservedChecks += len(rows)
		if participant := participantRows[key.participant]; participant != nil {
			participant.ObservedChecks += len(rows)
		}
		check, expectedBreach := expected[key]
		if !expectedBreach {
			result.UnexpectedChecks += len(rows)
			for range rows {
				result.Checks = append(result.Checks, PerpExposureRiskCheck{VenueID: key.participant.venue, ClientID: key.participant.client, Timestamp: key.ts, Observed: true, FieldsMatch: false})
			}
			continue
		}
		if len(marksAt) != 1 {
			result.MarkMismatches += len(rows)
		}
		for index, row := range rows {
			result.FieldChecks++
			observed := PerpExposureRiskCheck{
				VenueID: key.participant.venue, ClientID: key.participant.client, Timestamp: key.ts,
				Mark: row.MarkPrice, Position: state.pos, Entry: state.entry,
				Wallet: state.wallet, Debt: state.debt, Contribution: check.contribution,
				Equity: check.equity, Notional: check.notional, Maintenance: check.maintenance,
				Expected: true, Observed: true,
			}
			if len(marksAt) == 1 && row.MarkPrice != marksAt[0].mark {
				result.MarkMismatches++
			}
			if row.Balance != state.wallet {
				result.BalanceMismatches++
			}
			if row.DerivativeContribution != check.contribution {
				result.ContributionMismatches++
			}
			if row.Equity != check.equity {
				result.EquityMismatches++
			}
			if row.Notional != check.notional {
				result.NotionalMismatches++
			}
			if row.MaintenanceMargin != check.maintenance {
				result.MaintenanceMismatches++
			}
			observed.FieldsMatch = row.MarkPrice == check.mark && row.Balance == state.wallet && row.DerivativeContribution == check.contribution && row.Equity == check.equity && row.Notional == check.notional && row.MaintenanceMargin == check.maintenance
			if !observed.FieldsMatch {
				result.FieldMismatches++
			}
			if index == 0 {
				result.Checks = append(result.Checks, observed)
			}
		}
	}
	for key := range expected {
		if len(checks[key]) == 0 {
			result.MissingChecks++
		}
	}

	// Borrow events are matched to the independently reconstructed debt delta
	// at the same participant/timestamp. This catches a log-only borrow or a
	// debt balance mutation that was not justified by the declared event.
	for key, rows := range borrows {
		var amount int64
		for _, row := range rows {
			result.BorrowEvents++
			if row.Asset != opts.QuoteAsset {
				result.UnexpectedAutoBorrows++
			}
			if row.Amount > 0 && amount <= (1<<63-1)-row.Amount {
				amount += row.Amount
			} else {
				result.BorrowAmountMismatches++
			}
		}
		if borrowDebtDeltas[key] != amount {
			result.BorrowAmountMismatches++
		}
		if participant := participantRows[key.participant]; participant != nil {
			if next, ok := exactAdd(participant.BorrowedQuote, amount); ok {
				participant.BorrowedQuote = next
			} else {
				result.ArithmeticFailures++
			}
		}
	}

	// Link participant liquidations to the same-timestamp risk breach and to a
	// real reducing position path in the derivative evidence.
	for key, rows := range liquidations {
		for _, row := range rows {
			result.ParticipantLiquidations++
			if _, found := expected[perpRiskTimeKey{participant: key, ts: row.ts}]; !found {
				result.UnexpectedLiquidations++
			}
			if participant := participantRows[key]; participant != nil {
				participant.Liquidations++
				if row.payload.RemainingDebt > 0 {
					participant.Deficit += row.payload.RemainingDebt
				}
			}
			if row.payload.RemainingDebt < 0 || row.payload.PositionSize == 0 {
				result.MalformedRecords++
				continue
			}
			if row.payload.RemainingDebt > 0 {
				result.ParticipantDeficits++
				if total, ok := exactAdd(result.TotalDeficit, row.payload.RemainingDebt); ok {
					result.TotalDeficit = total
				} else {
					result.ArithmeticFailures++
				}
				insuranceRows := insurance[perpRiskTimeKey{participant: perpRiskParticipantKey{venue: key.venue}, ts: row.ts}]
				matched := false
				for _, insuranceRow := range insuranceRows {
					if insuranceRow.Delta == -row.payload.RemainingDebt {
						matched = true
						break
					}
				}
				if !matched {
					result.InsuranceMismatches++
				} else {
					result.ParticipantInsuranceEvents++
				}
			}
			pathFound := false
			for _, update := range positionUpdates[key] {
				if update.file != row.file || update.ts != row.ts || update.ordinal >= row.ordinal || update.payload.OldSize != row.payload.PositionSize {
					continue
				}
				if reducedSameSideRisk(update.payload.OldSize, update.payload.NewSize) {
					pathFound = true
					break
				}
			}
			if !pathFound {
				result.PositionPathFailures++
			}
		}
	}
	for key, count := range marginCalls {
		result.ParticipantMarginCalls += count
		if participant := participantRows[key]; participant != nil {
			participant.MarginCalls += count
		}
	}

	for key, participant := range participantRows {
		state := candidates[key]
		if rows := snapshots[key]; len(rows) > 0 {
			state = rows[len(rows)-1].post
		}
		participant.TerminalPosition = state.pos
		participant.TerminalWallet = state.wallet
		participant.TerminalDebt = state.debt
		terminal, ok := terminalAccountFor(r, key, opts)
		if !ok || terminal.pos != state.pos || terminal.entry != state.entry || terminal.wallet != state.wallet || terminal.debt != state.debt {
			result.TerminalStateMismatches++
		}
		result.Participants = append(result.Participants, *participant)
	}
	sort.Slice(result.Participants, func(i, j int) bool {
		if result.Participants[i].VenueID != result.Participants[j].VenueID {
			return result.Participants[i].VenueID < result.Participants[j].VenueID
		}
		return result.Participants[i].ClientID < result.Participants[j].ClientID
	})
	sort.Slice(result.Checks, func(i, j int) bool {
		if result.Checks[i].VenueID != result.Checks[j].VenueID {
			return result.Checks[i].VenueID < result.Checks[j].VenueID
		}
		if result.Checks[i].ClientID != result.Checks[j].ClientID {
			return result.Checks[i].ClientID < result.Checks[j].ClientID
		}
		return result.Checks[i].Timestamp < result.Checks[j].Timestamp
	})
	result.Valid = result.Candidates > 0 && result.MalformedRecords == 0 && result.DuplicateMarkTimestamps == 0 && result.MarkDomainFailures == 0 && result.ArithmeticFailures == 0 && result.PositionChainFailures == 0 && result.BalanceChainFailures == 0 && result.CrossFileAmbiguities == 0 && result.BorrowAmountMismatches == 0 && result.UnexpectedAutoBorrows == 0 && result.NegativeDebtObservations == 0 && result.MissingChecks == 0 && result.UnexpectedChecks == 0 && result.DuplicateChecks == 0 && result.FieldMismatches == 0 && result.PositionPathFailures == 0 && result.UnexpectedLiquidations == 0 && result.InsuranceMismatches == 0 && result.TerminalStateMismatches == 0
	return result, nil
}

func initialTimestamp(r *Run, key perpRiskParticipantKey) int64 {
	for _, row := range r.Report.InitialAccounts {
		if row.VenueID == key.venue && row.ClientID == key.client {
			return row.Account.Timestamp
		}
	}
	return 0
}

func terminalAccountFor(r *Run, key perpRiskParticipantKey, opts PerpExposureRiskOptions) (perpRiskState, bool) {
	for _, row := range r.Report.TerminalAccounts {
		if row.VenueID != key.venue || row.ClientID != key.client {
			continue
		}
		state := perpRiskState{}
		found := false
		for _, balance := range row.Account.PerpBalances {
			if balance.Asset == opts.QuoteAsset {
				gross, ok := exactAdd(balance.NetAsset, balance.Borrowed)
				if !ok {
					return perpRiskState{}, false
				}
				state.wallet, state.debt, found = gross, balance.Borrowed, true
			}
		}
		if !found {
			return perpRiskState{}, false
		}
		for _, position := range row.Account.Positions {
			if position.Symbol == opts.Symbol && position.Size != 0 {
				state.pos, state.entry = position.Size, position.EntryPrice
			}
		}
		return state, true
	}
	return perpRiskState{}, false
}

func independentPerpExposureMarginCheck(wallet, debt, size, entry, mark int64, opts PerpExposureRiskOptions) (replayedMarginCheck, bool) {
	if mark <= 0 || size == 0 {
		return replayedMarginCheck{}, false
	}
	contribution, ok := exactMulDiv(size, mark-entry, opts.BasePrecision)
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
	equity, ok := exactSub(wallet, debt)
	if !ok {
		return replayedMarginCheck{}, false
	}
	equity, ok = exactAdd(equity, contribution)
	if !ok {
		return replayedMarginCheck{}, false
	}
	return replayedMarginCheck{mark: mark, balance: wallet, contribution: contribution, equity: equity, notional: notional, maintenance: maintenance}, true
}

func reducedSameSideRisk(oldSize, newSize int64) bool {
	if oldSize == 0 || oldSize == -1<<63 || newSize == -1<<63 {
		return false
	}
	if oldSize > 0 {
		return newSize >= 0 && newSize < oldSize
	}
	return newSize <= 0 && newSize > oldSize
}

func perpRiskEventReady(row perpRiskTimelineEvent, state perpRiskState, quoteAsset string) bool {
	if row.pos != nil {
		return row.pos.OldSize == state.pos && row.pos.OldEntryPrice == state.entry
	}
	if row.balance == nil {
		return false
	}
	tmp := state
	relevant := false
	for _, change := range row.balance.Changes {
		if change.Asset != quoteAsset || (change.Wallet != "perp" && change.Wallet != "borrowed") {
			continue
		}
		relevant = true
		if change.Wallet == "perp" {
			if change.OldBalance != tmp.wallet {
				return false
			}
			tmp.wallet = change.NewBalance
		} else {
			if change.OldBalance != tmp.debt {
				return false
			}
			tmp.debt = change.NewBalance
		}
	}
	return relevant
}

func perpRiskEventsOverlap(a, b perpRiskTimelineEvent, quoteAsset string) bool {
	if a.pos != nil || b.pos != nil {
		return a.pos != nil && b.pos != nil
	}
	fields := func(row perpRiskTimelineEvent) map[string]struct{} {
		out := make(map[string]struct{})
		if row.balance != nil {
			for _, change := range row.balance.Changes {
				if change.Asset == quoteAsset && (change.Wallet == "perp" || change.Wallet == "borrowed") {
					out[change.Wallet] = struct{}{}
				}
			}
		}
		return out
	}
	left, right := fields(a), fields(b)
	for field := range left {
		if _, ok := right[field]; ok {
			return true
		}
	}
	return false
}
