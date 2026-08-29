package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// PositionOptions configures the independent position reconstruction.
type PositionOptions struct {
	Files         []string
	FilesSelected bool
	// BasePrecision converts a position size into contracts, which is what
	// turns a price difference into cash.
	BasePrecision int64
	// RequireExactReplay selects the strict r5 evidence contract. Historical
	// streams may omit trade inputs; strict runs must reject that fallback.
	RequireExactReplay bool
}

// ReconstructedPosition is one participant's final position in one contract,
// rebuilt from the venue's position updates rather than read from its report.
type ReconstructedPosition struct {
	VenueID      string `json:"venue_id"`
	ClientID     uint64 `json:"client_id"`
	Symbol       string `json:"symbol"`
	PositionSide string `json:"position_side,omitempty"`
	Size         int64  `json:"size"`
	EntryPrice   int64  `json:"entry_price"`
	Updates      int    `json:"updates"`
}

// ContractBalance is the net open interest of one contract at one venue.
type ContractBalance struct {
	VenueID string `json:"venue_id"`
	Symbol  string `json:"symbol"`
	// NetSize must be zero: every contract is somebody's long and somebody
	// else's short.
	NetSize int64 `json:"net_size"`
	// GrossSize is the open interest, which is what makes a zero net
	// meaningful rather than vacuous.
	GrossSize int64 `json:"gross_size"`
	Holders   int   `json:"holders"`
	// MarkPrice is the venue's last published mark, and OpenValue the cash
	// that would change hands if every holder closed there. MarkAvailable keeps
	// a present zero/negative mark distinct from no mark publication.
	MarkPrice     int64 `json:"mark_price"`
	MarkAvailable bool  `json:"mark_available"`
	OpenValue     int64 `json:"open_value"`
	// DisplayOpenValue retains the public-entry reconstruction as a diagnostic;
	// it is expected to differ from OpenValue after exact basis rounding.
	DisplayOpenValue int64 `json:"display_open_value"`
}

// PositionReconstruction is the independent rebuild of every open position.
//
// It exists because a conservation audit that reads the same account report it
// is auditing proves only that the report agrees with itself. These positions
// come from the venue's position-update stream and the marks from its
// mark-price stream, so a disagreement with the report is informative.
type PositionReconstruction struct {
	Positions []ReconstructedPosition `json:"positions,omitempty"`
	Contracts []ContractBalance       `json:"contracts"`
	// NonZeroNetContracts counts contracts whose longs and shorts do not
	// cancel, which is a broken market rather than a small residual.
	NonZeroNetContracts int `json:"non_zero_net_contracts"`
	// OpenLinearValue is the summed unrealised cash across non-option
	// contracts, computed from reconstructed positions and published marks.
	OpenLinearValue int64 `json:"open_linear_value"`
	// ReportedLinearValue is the same quantity as the run's own report gives
	// it, and Disagreement the difference.
	ReportedLinearValue int64 `json:"reported_linear_value"`
	Disagreement        int64 `json:"disagreement"`
	// SettledContractsExcluded counts positions the stream still shows as open
	// in a contract that has already settled. They are closed by definition,
	// and counting them was overstating the open value by the settled amount.
	SettledContractsExcluded int   `json:"settled_contracts_excluded"`
	SettledSizeExcluded      int64 `json:"settled_size_excluded"`
	// UnrepresentableOpenValues counts signed price changes whose exact
	// position cash flow exceeded int64. They are not rewritten as zero.
	UnrepresentableOpenValues int `json:"unrepresentable_open_values"`
	// Exact replay is fail-closed. A malformed, ambiguous, or inconsistent
	// trade update never falls back silently to the rounded public entry price.
	ExactReplayChecks    int `json:"exact_replay_checks"`
	ExactReplayFailures  int `json:"exact_replay_failures"`
	EvidenceFailures     int `json:"evidence_failures"`
	MissingMarks         int `json:"missing_marks"`
	MarkIdentityFailures int `json:"mark_identity_failures"`
	// Terminal position reconciliation prevents one matching aggregate PnL from
	// hiding an omitted or invented holder in the independently persisted report.
	MissingTerminalPositions    int `json:"missing_terminal_positions"`
	UnexpectedTerminalPositions int `json:"unexpected_terminal_positions"`
	TerminalPositionMismatches  int `json:"terminal_position_mismatches"`
	TerminalTimestampFailures   int `json:"terminal_timestamp_failures"`
	PostTerminalPositionUpdates int `json:"post_terminal_position_updates"`
	// RealizedPnLChecks binds each nonzero exact replay transition to the
	// independent realized_pnl event emitted by settlement.
	RealizedPnLChecks   int `json:"realized_pnl_checks"`
	RealizedPnLFailures int `json:"realized_pnl_failures"`
	// DisplayFormulaGap is diagnostic only: it measures information lost by
	// valuing with the rounded public EntryPrice.
	DisplayFormulaGap  int64 `json:"display_formula_gap"`
	DisplayLinearValue int64 `json:"display_linear_value"`
}

type positionKey struct {
	venue        string
	clientID     uint64
	symbol       string
	positionSide string
}

type markKey struct {
	venue  string
	symbol string
}

type realizedPnLKey struct {
	venue, symbol, side string
	clientID            uint64
	timestamp           int64
	closedQty           int64
	entryPrice          int64
	exitPrice           int64
	pnl                 int64
}

func realizedClosedQuantity(oldSize, tradeQty int64, tradeSide string) (int64, bool) {
	if oldSize == 0 {
		return 0, true
	}
	delta := tradeQty
	if tradeSide == "SELL" {
		delta = -tradeQty
	}
	if tradeSide != "BUY" && tradeSide != "SELL" {
		return 0, false
	}
	if (oldSize > 0 && delta >= 0) || (oldSize < 0 && delta <= 0) {
		return 0, true
	}
	oldMagnitude := exactMagnitude(oldSize)
	tradeMagnitude := exactMagnitude(delta)
	if tradeMagnitude > oldMagnitude {
		return oldMagnitude, true
	}
	return tradeMagnitude, true
}

// MeasurePositions rebuilds final positions and marks from the event streams.
func (r *Run) MeasurePositions(opts PositionOptions) (*PositionReconstruction, error) {
	if opts.BasePrecision <= 0 {
		return nil, fmt.Errorf("analysis: position base precision must be positive, got %d", opts.BasePrecision)
	}
	type positionPayload struct {
		Timestamp     int64  `json:"timestamp"`
		ClientID      uint64 `json:"client_id"`
		Symbol        string `json:"symbol"`
		PositionSide  string `json:"position_side"`
		BasePrecision int64  `json:"base_precision"`
		OldSize       int64  `json:"old_size"`
		OldEntryPrice int64  `json:"old_entry_price"`
		NewSize       int64  `json:"new_size"`
		NewEntryPrice int64  `json:"new_entry_price"`
		TradeQty      int64  `json:"trade_qty"`
		TradePrice    int64  `json:"trade_price"`
		TradeSide     string `json:"trade_side"`
		Reason        string `json:"reason"`
	}
	type markPayload struct {
		Timestamp int64  `json:"timestamp"`
		Symbol    string `json:"symbol"`
		MarkPrice int64  `json:"mark_price"`
	}
	type realizedPnLPayload struct {
		Timestamp  int64  `json:"timestamp"`
		ClientID   uint64 `json:"client_id"`
		Symbol     string `json:"symbol"`
		ClosedQty  int64  `json:"closed_qty"`
		EntryPrice int64  `json:"entry_price"`
		ExitPrice  int64  `json:"exit_price"`
		PnL        int64  `json:"pnl"`
		Side       string `json:"side"`
	}
	type instrumentPayload struct {
		Action        string `json:"action"`
		Symbol        string `json:"symbol"`
		BasePrecision int64  `json:"base_precision"`
		Timestamp     int64  `json:"timestamp"`
	}

	var mu sync.Mutex
	terminalAt, terminalKnown := terminalAccountTimestamp(r.Report)
	type positionState struct {
		size, entry int64
		at          int64
		updates     int
		precision   int64
		exact       *exactPositionReplay
		exactFailed bool
	}
	positions := make(map[positionKey]*positionState)
	precision := opts.BasePrecision
	if precision <= 0 {
		precision = 1
	}
	exactReplayChecks := 0
	exactReplayFailures := 0
	evidenceFailures := 0
	postTerminalPositionUpdates := 0
	realizedExpected := make(map[realizedPnLKey]int)
	realizedObserved := make(map[realizedPnLKey]int)
	realizedPnLChecks := 0
	realizedPnLFailures := 0
	type markState struct {
		price int64
		at    int64
	}
	marks := make(map[markKey]*markState)
	markIdentityFailures := 0

	// A contract that settled is closed by definition, whatever the position
	// stream last said about it. Without this the reconstruction carries every
	// expired dated future forward at its last mark, and the open value it
	// reports is overstated by exactly the amount that was settled in cash.
	settled := make(map[markKey]bool)
	lifecyclePrecisions := make(map[markKey]int64)
	lifecycleListings := make(map[markKey][]evidenceOrder)
	settledScan := ScanOptions{
		Events:        []string{"instrument_listed", "instrument_settled"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
		Workers:       1,
	}
	if err := r.Scan(settledScan, func(event Event) {
		var payload instrumentPayload
		if err := event.Decode(&payload); err != nil {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		if payload.Symbol == "" {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS || payload.BasePrecision <= 0) {
			mu.Lock()
			evidenceFailures++
			mu.Unlock()
			return
		}
		mu.Lock()
		key := markKey{event.VenueID, payload.Symbol}
		if opts.RequireExactReplay && ((event.Name == "instrument_listed" && payload.Action != "listed") ||
			(event.Name == "instrument_settled" && payload.Action != "settled")) {
			evidenceFailures++
			mu.Unlock()
			return
		}
		if event.Name == "instrument_listed" {
			lifecycleListings[key] = append(lifecycleListings[key], evidenceOrder{
				timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
			})
			if payload.BasePrecision > 0 {
				if previous, exists := lifecyclePrecisions[key]; exists && previous != payload.BasePrecision {
					evidenceFailures++
				} else {
					lifecyclePrecisions[key] = payload.BasePrecision
				}
			}
		} else if opts.RequireExactReplay {
			_, listed := latestCausalPrerequisite(lifecycleListings[key], evidenceOrder{
				timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
			})
			if !listed {
				evidenceFailures++
			}
		}
		if event.Name == "instrument_settled" {
			settled[key] = true
		}
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	scan := ScanOptions{
		Events:        []string{"position_update", "mark_price_update", "realized_pnl"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
		Workers:       1,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "position_update":
			var payload positionPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Symbol == "" {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS) {
				mu.Lock()
				exactReplayFailures++
				evidenceFailures++
				mu.Unlock()
				return
			}
			expectedPrecision := precision
			lifecycleKey := markKey{event.VenueID, payload.Symbol}
			if lifecyclePrecision := lifecyclePrecisions[lifecycleKey]; lifecyclePrecision > 0 {
				expectedPrecision = lifecyclePrecision
			} else if opts.RequireExactReplay {
				mu.Lock()
				exactReplayFailures++
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Reason == "trade" &&
				(payload.ClientID != event.ClientID || (event.Symbol != "" && payload.Symbol != event.Symbol) ||
					(opts.RequireExactReplay && (event.Symbol == "" || !validPositionSide(payload.PositionSide) || payload.BasePrecision != expectedPrecision)) ||
					(!opts.RequireExactReplay && payload.PositionSide != "" && !validPositionSide(payload.PositionSide))) {
				// The event envelope and its payload are two independent links.
				// A trade whose links disagree cannot be assigned to a position
				// without silently changing the account being audited.
				mu.Lock()
				exactReplayFailures++
				evidenceFailures++
				mu.Unlock()
				return
			}
			key := positionKey{venue: event.VenueID, clientID: payload.ClientID, symbol: payload.Symbol, positionSide: payload.PositionSide}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			if opts.RequireExactReplay && terminalKnown && at > terminalAt {
				mu.Lock()
				exactReplayFailures++
				evidenceFailures++
				postTerminalPositionUpdates++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay {
				_, listed := latestCausalPrerequisite(lifecycleListings[lifecycleKey], evidenceOrder{
					timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
				})
				if !listed {
					mu.Lock()
					exactReplayFailures++
					evidenceFailures++
					mu.Unlock()
					return
				}
			}
			mu.Lock()
			state := positions[key]
			if state == nil {
				state = &positionState{}
				positions[key] = state
			}
			state.updates++
			if payload.Reason == "trade" {
				if state.exact == nil {
					state.exact = &exactPositionReplay{}
				}
				if !state.exactFailed {
					tradePrecision := precision
					if payload.BasePrecision > 0 {
						tradePrecision = payload.BasePrecision
					}
					if state.precision == 0 {
						state.precision = tradePrecision
					}
					exactReplayChecks++
					if state.precision != tradePrecision {
						state.exactFailed = true
						exactReplayFailures++
					} else {
						trade := exactPositionTrade{
							OldSize: payload.OldSize, OldEntryPrice: payload.OldEntryPrice,
							NewSize: payload.NewSize, NewEntryPrice: payload.NewEntryPrice,
							TradeQty: payload.TradeQty, TradePrice: payload.TradePrice,
							TradeSide: payload.TradeSide, PositionSide: payload.PositionSide,
						}
						realizedCash, err := state.exact.apply(trade, tradePrecision)
						if err != nil {
							state.exactFailed = true
							exactReplayFailures++
						} else if opts.RequireExactReplay && realizedCash != 0 {
							closedQty, closedQtyOK := realizedClosedQuantity(payload.OldSize, payload.TradeQty, payload.TradeSide)
							if !closedQtyOK || closedQty == 0 {
								realizedPnLFailures++
							} else {
								key := realizedPnLKey{
									venue: event.VenueID, clientID: payload.ClientID, symbol: payload.Symbol,
									timestamp: at, closedQty: closedQty, entryPrice: payload.OldEntryPrice,
									exitPrice: payload.TradePrice, pnl: realizedCash, side: payload.TradeSide,
								}
								realizedExpected[key]++
								realizedPnLChecks++
							}
						}
					}
				}
			} else if state.exact != nil || opts.RequireExactReplay {
				if state.exact == nil {
					state.exact = &exactPositionReplay{}
				}
				// Once exact replay has started, a mutation without the
				// registered trade inputs makes the account path ambiguous.
				if !state.exactFailed {
					state.exactFailed = true
					exactReplayFailures++
				}
			} else if payload.Reason != "" {
				// Mutations without the registered trade inputs cannot be
				// safely inferred from public state.
				state.exact = &exactPositionReplay{}
				state.exactFailed = true
				exactReplayFailures++
			}
			// This scan is deliberately single-worker: exact basis replay uses
			// physical order within each persisted derivatives stream.
			if at >= state.at {
				state.size, state.entry, state.at = payload.NewSize, payload.NewEntryPrice, at
			}
			mu.Unlock()
		case "realized_pnl":
			var payload realizedPnLPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				realizedPnLFailures++
				mu.Unlock()
				return
			}
			valid := payload.Timestamp != 0 && payload.Timestamp == event.SimTS &&
				payload.ClientID == event.ClientID && payload.Symbol != "" &&
				(event.Symbol == "" || event.Symbol == payload.Symbol) && payload.ClosedQty > 0 &&
				payload.PnL != 0 && (payload.Side == "BUY" || payload.Side == "SELL")
			if opts.RequireExactReplay {
				_, listed := latestCausalPrerequisite(lifecycleListings[markKey{event.VenueID, payload.Symbol}], evidenceOrder{
					timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
				})
				valid = valid && listed
			}
			if !valid {
				mu.Lock()
				realizedPnLFailures++
				mu.Unlock()
				return
			}
			key := realizedPnLKey{
				venue: event.VenueID, clientID: payload.ClientID, symbol: payload.Symbol,
				timestamp: payload.Timestamp, closedQty: payload.ClosedQty,
				entryPrice: payload.EntryPrice, exitPrice: payload.ExitPrice,
				pnl: payload.PnL, side: payload.Side,
			}
			mu.Lock()
			realizedObserved[key]++
			mu.Unlock()
		case "mark_price_update":
			var payload markPayload
			if err := event.Decode(&payload); err != nil {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if payload.Symbol == "" {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (payload.Timestamp == 0 || payload.Timestamp != event.SimTS) {
				mu.Lock()
				evidenceFailures++
				mu.Unlock()
				return
			}
			if opts.RequireExactReplay && (event.Symbol == "" || event.Symbol != payload.Symbol) {
				mu.Lock()
				markIdentityFailures++
				evidenceFailures++
				mu.Unlock()
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			if opts.RequireExactReplay && terminalKnown && at > terminalAt {
				return
			}
			if opts.RequireExactReplay {
				_, listed := latestCausalPrerequisite(lifecycleListings[markKey{event.VenueID, payload.Symbol}], evidenceOrder{
					timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
				})
				if !listed {
					mu.Lock()
					markIdentityFailures++
					evidenceFailures++
					mu.Unlock()
					return
				}
			}
			mu.Lock()
			key := markKey{event.VenueID, payload.Symbol}
			state := marks[key]
			if state == nil || at >= state.at {
				marks[key] = &markState{price: payload.MarkPrice, at: at}
			}
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}
	for key, expected := range realizedExpected {
		observed := realizedObserved[key]
		if observed != expected {
			if observed > expected {
				realizedPnLFailures += observed - expected
			} else {
				realizedPnLFailures += expected - observed
			}
		}
	}
	for key, observed := range realizedObserved {
		if _, expected := realizedExpected[key]; !expected {
			realizedPnLFailures += observed
		}
	}

	result := &PositionReconstruction{}
	terminalSeen := make(map[positionKey]int)
	if opts.RequireExactReplay {
		for _, row := range r.Report.TerminalAccounts {
			for _, position := range row.Account.Positions {
				if isOptionSymbol(position.Symbol) || position.Size == 0 {
					continue
				}
				side, present, valid := reportPositionSide(position.PositionSide)
				if !present || !valid {
					result.TerminalPositionMismatches++
					evidenceFailures++
					continue
				}
				key := positionKey{venue: row.VenueID, clientID: row.ClientID, symbol: position.Symbol, positionSide: side}
				terminalSeen[key]++
				state := positions[key]
				if terminalSeen[key] != 1 || state == nil || state.exact == nil || state.exactFailed ||
					state.size != position.Size || state.entry != position.EntryPrice {
					result.TerminalPositionMismatches++
					continue
				}
				mark := marks[markKey{row.VenueID, position.Symbol}]
				if position.MarkPrice == nil || !position.UnrealizedPnLPresent || mark == nil || mark.at > terminalAt || *position.MarkPrice != mark.price {
					result.TerminalPositionMismatches++
					continue
				}
				statePrecision := state.precision
				if statePrecision <= 0 {
					statePrecision = precision
				}
				expectedPnL, replayOK := state.exact.unrealizedPnL(*position.MarkPrice, statePrecision)
				if !replayOK || expectedPnL != position.UnrealizedPnL {
					result.TerminalPositionMismatches++
				}
			}
		}
	}
	activePositions := make(map[positionKey]bool)
	contracts := make(map[markKey]*ContractBalance)
	for key, state := range positions {
		if state.exact != nil && !state.exactFailed && state.exact.size != state.size {
			state.exactFailed = true
			exactReplayFailures++
		}
		if state.size == 0 {
			continue
		}
		contractKey := markKey{key.venue, key.symbol}
		if settled[contractKey] {
			result.SettledContractsExcluded++
			excludedSize, ok := exactAdd(result.SettledSizeExcluded, state.size)
			if !ok {
				result.UnrepresentableOpenValues++
			} else {
				result.SettledSizeExcluded = excludedSize
			}
			continue
		}
		activePositions[key] = true
		result.Positions = append(result.Positions, ReconstructedPosition{
			VenueID: key.venue, ClientID: key.clientID, Symbol: key.symbol,
			PositionSide: key.positionSide, Size: state.size, EntryPrice: state.entry,
			Updates: state.updates,
		})
		contract := contracts[contractKey]
		if contract == nil {
			contract = &ContractBalance{VenueID: key.venue, Symbol: key.symbol}
			if mark := marks[contractKey]; mark != nil {
				contract.MarkPrice = mark.price
				contract.MarkAvailable = true
			}
			contracts[contractKey] = contract
		}
		netSize, ok := exactAdd(contract.NetSize, state.size)
		if !ok {
			result.UnrepresentableOpenValues++
			continue
		}
		contract.NetSize = netSize
		gross := state.size
		if gross == -1<<63 {
			result.UnrepresentableOpenValues++
			continue
		}
		if gross < 0 {
			gross = -gross
		}
		grossSize, ok := exactAdd(contract.GrossSize, gross)
		if !ok {
			result.UnrepresentableOpenValues++
			continue
		}
		contract.GrossSize = grossSize
		contract.Holders++
		if contract.MarkAvailable {
			statePrecision := state.precision
			if statePrecision <= 0 {
				statePrecision = precision
			}
			change, ok := exactSub(contract.MarkPrice, state.entry)
			if !ok {
				result.UnrepresentableOpenValues++
				continue
			}
			displayValue, ok := exactMulDiv(change, state.size, statePrecision)
			if !ok {
				result.UnrepresentableOpenValues++
				continue
			}
			displayTotal, ok := exactAdd(contract.DisplayOpenValue, displayValue)
			if !ok {
				result.UnrepresentableOpenValues++
				continue
			}
			contract.DisplayOpenValue = displayTotal
			value := displayValue
			if state.exact != nil {
				if state.exactFailed {
					continue
				}
				exactValue, exactOK := state.exact.unrealizedPnL(contract.MarkPrice, statePrecision)
				if !exactOK {
					result.UnrepresentableOpenValues++
					continue
				}
				value = exactValue
			}
			next, ok := exactAdd(contract.OpenValue, value)
			if !ok {
				result.UnrepresentableOpenValues++
				continue
			}
			contract.OpenValue = next
		}
	}

	for _, contract := range contracts {
		if contract.NetSize != 0 {
			result.NonZeroNetContracts++
		}
		if !isOptionSymbol(contract.Symbol) {
			if opts.RequireExactReplay {
				if !contract.MarkAvailable {
					result.MissingMarks++
				} else if !terminalKnown || marks[markKey{contract.VenueID, contract.Symbol}].at > terminalAt {
					result.MissingMarks++
					result.MarkIdentityFailures++
				}
			}
			openValue, ok := exactAdd(result.OpenLinearValue, contract.OpenValue)
			if !ok {
				result.UnrepresentableOpenValues++
			} else {
				result.OpenLinearValue = openValue
			}
			displayValue, ok := exactAdd(result.DisplayLinearValue, contract.DisplayOpenValue)
			if !ok {
				result.UnrepresentableOpenValues++
			} else {
				result.DisplayLinearValue = displayValue
			}
		}
		result.Contracts = append(result.Contracts, *contract)
	}
	if opts.RequireExactReplay {
		for key := range activePositions {
			if terminalSeen[key] == 0 {
				result.MissingTerminalPositions++
			}
		}
		for key, count := range terminalSeen {
			if count != 1 || !activePositions[key] {
				result.UnexpectedTerminalPositions++
			}
		}
		if len(activePositions) > 0 && !terminalKnown {
			result.TerminalTimestampFailures++
		}
	}
	sort.Slice(result.Positions, func(i, j int) bool {
		if result.Positions[i].VenueID != result.Positions[j].VenueID {
			return result.Positions[i].VenueID < result.Positions[j].VenueID
		}
		if result.Positions[i].ClientID != result.Positions[j].ClientID {
			return result.Positions[i].ClientID < result.Positions[j].ClientID
		}
		if result.Positions[i].Symbol != result.Positions[j].Symbol {
			return result.Positions[i].Symbol < result.Positions[j].Symbol
		}
		return result.Positions[i].PositionSide < result.Positions[j].PositionSide
	})
	sort.Slice(result.Contracts, func(i, j int) bool {
		if result.Contracts[i].VenueID != result.Contracts[j].VenueID {
			return result.Contracts[i].VenueID < result.Contracts[j].VenueID
		}
		return result.Contracts[i].Symbol < result.Contracts[j].Symbol
	})

	for _, row := range r.Report.TerminalAccounts {
		for _, position := range row.Account.Positions {
			if isOptionSymbol(position.Symbol) {
				continue
			}
			reportedValue, ok := exactAdd(result.ReportedLinearValue, position.UnrealizedPnL)
			if !ok {
				result.UnrepresentableOpenValues++
			} else {
				result.ReportedLinearValue = reportedValue
			}
		}
	}
	if disagreement, ok := exactSub(result.OpenLinearValue, result.ReportedLinearValue); ok {
		result.Disagreement = disagreement
	} else {
		result.UnrepresentableOpenValues++
	}
	if displayGap, ok := exactSub(result.OpenLinearValue, result.DisplayLinearValue); ok {
		result.DisplayFormulaGap = displayGap
	} else {
		result.UnrepresentableOpenValues++
	}
	result.ExactReplayChecks = exactReplayChecks
	result.ExactReplayFailures = exactReplayFailures
	result.EvidenceFailures = evidenceFailures
	result.PostTerminalPositionUpdates = postTerminalPositionUpdates
	result.RealizedPnLChecks = realizedPnLChecks
	result.RealizedPnLFailures = realizedPnLFailures
	result.MarkIdentityFailures += markIdentityFailures
	return result, nil
}

func reportPositionSide(raw json.RawMessage) (string, bool, bool) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", false, false
	}
	var numeric uint8
	if err := json.Unmarshal(raw, &numeric); err == nil {
		switch numeric {
		case 0:
			return "BOTH", true, true
		case 1:
			return "LONG", true, true
		case 2:
			return "SHORT", true, true
		default:
			return "", true, false
		}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return "", true, false
	}
	return text, true, validPositionSide(text)
}
