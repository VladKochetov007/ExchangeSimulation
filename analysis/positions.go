package analysis

import (
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
}

// ReconstructedPosition is one participant's final position in one contract,
// rebuilt from the venue's position updates rather than read from its report.
type ReconstructedPosition struct {
	VenueID    string `json:"venue_id"`
	ClientID   uint64 `json:"client_id"`
	Symbol     string `json:"symbol"`
	Size       int64  `json:"size"`
	EntryPrice int64  `json:"entry_price"`
	Updates    int    `json:"updates"`
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
	// that would change hands if every holder closed there.
	MarkPrice int64 `json:"mark_price"`
	OpenValue int64 `json:"open_value"`
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
}

type positionKey struct {
	venue    string
	clientID uint64
	symbol   string
}

type markKey struct {
	venue  string
	symbol string
}

// MeasurePositions rebuilds final positions and marks from the event streams.
func (r *Run) MeasurePositions(opts PositionOptions) (*PositionReconstruction, error) {
	type positionPayload struct {
		Timestamp     int64  `json:"timestamp"`
		ClientID      uint64 `json:"client_id"`
		Symbol        string `json:"symbol"`
		NewSize       int64  `json:"new_size"`
		NewEntryPrice int64  `json:"new_entry_price"`
	}
	type markPayload struct {
		Timestamp int64  `json:"timestamp"`
		Symbol    string `json:"symbol"`
		MarkPrice int64  `json:"mark_price"`
	}

	var mu sync.Mutex
	type positionState struct {
		size, entry int64
		at          int64
		updates     int
	}
	positions := make(map[positionKey]*positionState)
	type markState struct {
		price int64
		at    int64
	}
	marks := make(map[markKey]*markState)

	scan := ScanOptions{
		Events:        []string{"position_update", "mark_price_update"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "position_update":
			var payload positionPayload
			if event.Decode(&payload) != nil || payload.Symbol == "" {
				return
			}
			key := positionKey{event.VenueID, payload.ClientID, payload.Symbol}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			mu.Lock()
			state := positions[key]
			if state == nil {
				state = &positionState{}
				positions[key] = state
			}
			state.updates++
			// Files are scanned concurrently, so the latest update wins by
			// timestamp rather than by arrival.
			if at >= state.at {
				state.size, state.entry, state.at = payload.NewSize, payload.NewEntryPrice, at
			}
			mu.Unlock()
		case "mark_price_update":
			var payload markPayload
			if event.Decode(&payload) != nil || payload.Symbol == "" || payload.MarkPrice <= 0 {
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
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

	precision := opts.BasePrecision
	if precision <= 0 {
		precision = 1
	}
	result := &PositionReconstruction{}
	contracts := make(map[markKey]*ContractBalance)
	for key, state := range positions {
		if state.size == 0 {
			continue
		}
		contractKey := markKey{key.venue, key.symbol}
		contract := contracts[contractKey]
		if contract == nil {
			contract = &ContractBalance{VenueID: key.venue, Symbol: key.symbol}
			if mark := marks[contractKey]; mark != nil {
				contract.MarkPrice = mark.price
			}
			contracts[contractKey] = contract
		}
		contract.NetSize += state.size
		gross := state.size
		if gross < 0 {
			gross = -gross
		}
		contract.GrossSize += gross
		contract.Holders++
		if contract.MarkPrice > 0 {
			contract.OpenValue += (contract.MarkPrice - state.entry) / 1 * state.size / precision
		}
	}

	for _, contract := range contracts {
		if contract.NetSize != 0 {
			result.NonZeroNetContracts++
		}
		if !isOptionSymbol(contract.Symbol) {
			result.OpenLinearValue += contract.OpenValue
		}
		result.Contracts = append(result.Contracts, *contract)
	}
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
			result.ReportedLinearValue += position.UnrealizedPnL
		}
	}
	result.Disagreement = result.OpenLinearValue - result.ReportedLinearValue
	return result, nil
}
