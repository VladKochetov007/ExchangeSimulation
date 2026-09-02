package analysis

import (
	"fmt"
	"sort"
)

// LiquidationAudit independently reconciles the observable liquidation,
// account-deficit, and insurance-fund streams. It does not infer that a
// liquidation was economically correct merely because an event was emitted.
type LiquidationAudit struct {
	Liquidations             int   `json:"liquidations"`
	LiquidationChecks        int   `json:"liquidation_checks"`
	AffectedAccounts         int   `json:"affected_accounts"`
	LiquidationsWithDeficit  int   `json:"liquidations_with_deficit"`
	TotalDeficit             int64 `json:"total_deficit"`
	InsuranceDeficit         int64 `json:"insurance_deficit"`
	BalanceDeficitCredit     int64 `json:"balance_deficit_credit"`
	DeficitInsuranceResidual int64 `json:"deficit_insurance_residual"`
	DeficitBalanceResidual   int64 `json:"deficit_balance_residual"`
	DeficitMismatchInstants  int   `json:"deficit_mismatch_instants"`
	InvalidLiquidations      int   `json:"invalid_liquidations"`
	// SignedOrZeroFillPrices counts present numeric fill prices that a
	// positive-domain liquidation policy might reject upstream. They are not
	// classified as absent/invalid evidence here: this generic reconstruction
	// has no instrument-domain declaration from which to make that judgment.
	SignedOrZeroFillPrices int `json:"signed_or_zero_fill_prices"`
	// PositionPathRecords are liquidations for which the same-file, same-time
	// position-update batch provides both the pre-close and post-close state.
	// The liquidation event records the pre-close size because liquidate holds a
	// defensive position copy; the batch is the independent execution evidence.
	PositionPathRecords  int `json:"position_path_records"`
	PositionPathMissing  int `json:"position_path_missing"`
	PositionPathFailures int `json:"position_path_failures"`
	// PositionConservation checks that all participant position deltas in the
	// forced-close batch net to zero. It is a per-close contract residual, not
	// a claim that realised-PnL cash postings are individually zero-sum.
	PositionConservationRecords  int                `json:"position_conservation_records"`
	PositionConservationMissing  int                `json:"position_conservation_missing"`
	PositionConservationFailures int                `json:"position_conservation_failures"`
	PositionConservationResidual int64              `json:"position_conservation_residual"`
	ByVenue                      []LiquidationVenue `json:"by_venue"`
}

// LiquidationVenue is one venue's independently reconciled liquidation path.
type LiquidationVenue struct {
	VenueID                  string `json:"venue_id"`
	Liquidations             int    `json:"liquidations"`
	AffectedAccounts         int    `json:"affected_accounts"`
	LiquidationsWithDeficit  int    `json:"liquidations_with_deficit"`
	TotalDeficit             int64  `json:"total_deficit"`
	InsuranceDeficit         int64  `json:"insurance_deficit"`
	BalanceDeficitCredit     int64  `json:"balance_deficit_credit"`
	DeficitInsuranceResidual int64  `json:"deficit_insurance_residual"`
	DeficitBalanceResidual   int64  `json:"deficit_balance_residual"`
}

type liquidationInstant struct {
	venue, symbol string
	timestamp     int64
}

type liquidationAccountInstant struct {
	liquidationInstant
	clientID uint64
}

type liquidationPositionKey struct {
	venue, file, symbol string
	clientID            uint64
}

type liquidationPositionBatch struct {
	timestamp         int64
	firstOld, lastNew int64
	firstOrdinal      int64
	lastOrdinal       int64
	continuous        bool
}

type liquidationPositionWindow struct {
	file      string
	timestamp int64
	updates   []liquidationPositionDelta
}

type liquidationPositionDelta struct {
	ordinal int64
	delta   int64
}

// MeasureLiquidations reconciles every logged liquidation deficit against two
// independent records: the liquidated account's balance change and the
// exchange insurance-fund movement. All three are required for a deficit; an
// event-only check would miss a transfer that silently failed to post.
func (r *Run) MeasureLiquidations() (*LiquidationAudit, error) {
	type liquidationPayload struct {
		Symbol        string `json:"symbol"`
		PositionSize  int64  `json:"position_size"`
		FillPrice     int64  `json:"fill_price"`
		RemainingDebt int64  `json:"remaining_debt"`
	}
	type balanceChange struct {
		Timestamp int64  `json:"timestamp"`
		ClientID  uint64 `json:"client_id"`
		Symbol    string `json:"symbol"`
		Reason    string `json:"reason"`
		Changes   []struct {
			Delta int64 `json:"delta"`
		} `json:"changes"`
	}
	type insuranceFund struct {
		Timestamp int64  `json:"timestamp"`
		Symbol    string `json:"symbol"`
		Delta     int64  `json:"delta"`
		Reason    string `json:"reason"`
	}
	type positionUpdate struct {
		Timestamp int64  `json:"timestamp"`
		ClientID  uint64 `json:"client_id"`
		Symbol    string `json:"symbol"`
		OldSize   int64  `json:"old_size"`
		NewSize   int64  `json:"new_size"`
	}

	result := &LiquidationAudit{}
	accounts := make(map[Participant]struct{})
	venueAccounts := make(map[string]map[Participant]struct{})
	venueRows := make(map[string]*LiquidationVenue)
	debtByInstant := make(map[liquidationInstant]int64)
	insuranceByInstant := make(map[liquidationInstant]int64)
	balanceByInstant := make(map[liquidationInstant]int64)
	positionBatches := make(map[liquidationPositionKey]liquidationPositionBatch)
	var positionWindow liquidationPositionWindow
	row := func(venue string) *LiquidationVenue {
		out := venueRows[venue]
		if out == nil {
			out = &LiquidationVenue{VenueID: venue}
			venueRows[venue] = out
		}
		return out
	}
	// Every event in this contract is emitted either by a derivative book or a
	// venue-global logger. Restricting the scan also avoids spending most of a
	// long audit reading unrelated spot-book evidence.
	files := make([]string, 0, len(r.files))
	for _, file := range r.files {
		name := logicalEventLogName(file)
		if name == "derivatives.jsonl" || name == "general.jsonl" {
			files = append(files, file)
		}
	}
	// Position evidence is meaningful only in physical file order. A single
	// worker preserves that order while still allowing independent runs to be
	// processed in parallel by the campaign.
	if err := r.Scan(ScanOptions{Workers: 1, Files: files, FilesSelected: true, Events: []string{"liquidation", "liquidation_check", "balance_change", "insurance_fund", "position_update"}}, func(event Event) {
		switch event.Name {
		case "liquidation_check":
			result.LiquidationChecks++
		case "position_update":
			var payload positionUpdate
			if event.Decode(&payload) != nil {
				return
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			clientID := payload.ClientID
			if clientID == 0 {
				clientID = event.ClientID
			}
			if symbol == "" || clientID == 0 {
				return
			}
			if positionWindow.file != event.File || positionWindow.timestamp != event.SimTS {
				positionWindow = liquidationPositionWindow{file: event.File, timestamp: event.SimTS}
			}
			positionWindow.updates = append(positionWindow.updates, liquidationPositionDelta{
				ordinal: event.Ordinal,
				delta:   payload.NewSize - payload.OldSize,
			})
			key := liquidationPositionKey{venue: event.VenueID, file: event.File, clientID: clientID, symbol: symbol}
			batch, exists := positionBatches[key]
			if !exists || batch.timestamp != event.SimTS {
				positionBatches[key] = liquidationPositionBatch{
					timestamp: event.SimTS, firstOld: payload.OldSize, lastNew: payload.NewSize,
					firstOrdinal: event.Ordinal, lastOrdinal: event.Ordinal, continuous: true,
				}
				return
			}
			batch.continuous = batch.continuous && payload.OldSize == batch.lastNew
			batch.lastNew = payload.NewSize
			batch.lastOrdinal = event.Ordinal
			positionBatches[key] = batch
		case "liquidation":
			var payload liquidationPayload
			if event.Decode(&payload) != nil {
				return
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			result.Liquidations++
			state := row(event.VenueID)
			state.Liquidations++
			participant := Participant{VenueID: event.VenueID, ClientID: event.ClientID}
			accounts[participant] = struct{}{}
			if venueAccounts[event.VenueID] == nil {
				venueAccounts[event.VenueID] = make(map[Participant]struct{})
			}
			venueAccounts[event.VenueID][participant] = struct{}{}
			if symbol == "" || payload.PositionSize == 0 || payload.RemainingDebt < 0 {
				result.InvalidLiquidations++
			}
			if payload.FillPrice <= 0 {
				result.SignedOrZeroFillPrices++
			}
			positionKey := liquidationPositionKey{venue: event.VenueID, file: event.File, clientID: event.ClientID, symbol: symbol}
			batch, found := positionBatches[positionKey]
			if !found || batch.timestamp != event.SimTS || batch.lastOrdinal >= event.Ordinal {
				result.PositionPathMissing++
			} else {
				result.PositionPathRecords++
				if !batch.continuous || batch.firstOld != payload.PositionSize || !reducedSameSide(batch.firstOld, batch.lastNew) {
					result.PositionPathFailures++
				}
				if positionWindow.file != event.File || positionWindow.timestamp != event.SimTS {
					result.PositionConservationMissing++
				} else {
					result.PositionConservationRecords++
					var residual int64
					for _, update := range positionWindow.updates {
						if update.ordinal >= batch.firstOrdinal && update.ordinal < event.Ordinal {
							residual += update.delta
						}
					}
					result.PositionConservationResidual += residual
					if residual != 0 {
						result.PositionConservationFailures++
					}
				}
			}
			// A second same-timestamp liquidation for this key cannot be
			// disambiguated without a position-side field, so classify it as
			// missing rather than reusing evidence from the first close.
			delete(positionBatches, positionKey)
			if payload.RemainingDebt > 0 {
				key := liquidationInstant{venue: event.VenueID, symbol: symbol, timestamp: event.SimTS}
				debtByInstant[key] += payload.RemainingDebt
				result.LiquidationsWithDeficit++
				result.TotalDeficit += payload.RemainingDebt
				state.LiquidationsWithDeficit++
				state.TotalDeficit += payload.RemainingDebt
			}
		case "balance_change":
			var payload balanceChange
			if event.Decode(&payload) != nil || payload.Reason != "liquidation_deficit" {
				return
			}
			timestamp := payload.Timestamp
			if timestamp == 0 {
				timestamp = event.SimTS
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			var credit int64
			for _, change := range payload.Changes {
				credit += change.Delta
			}
			balanceByInstant[liquidationInstant{venue: event.VenueID, symbol: symbol, timestamp: timestamp}] += credit
			result.BalanceDeficitCredit += credit
			row(event.VenueID).BalanceDeficitCredit += credit
		case "insurance_fund":
			var payload insuranceFund
			if event.Decode(&payload) != nil || payload.Reason != "liquidation_deficit" {
				return
			}
			timestamp := payload.Timestamp
			if timestamp == 0 {
				timestamp = event.SimTS
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			insuranceByInstant[liquidationInstant{venue: event.VenueID, symbol: symbol, timestamp: timestamp}] -= payload.Delta
			result.InsuranceDeficit -= payload.Delta
			row(event.VenueID).InsuranceDeficit -= payload.Delta
		}
	}); err != nil {
		return nil, fmt.Errorf("liquidation audit: scan: %w", err)
	}
	for key, debt := range debtByInstant {
		insurance := insuranceByInstant[key]
		balance := balanceByInstant[key]
		if debt != insurance || debt != balance {
			result.DeficitMismatchInstants++
		}
		result.DeficitInsuranceResidual += debt - insurance
		result.DeficitBalanceResidual += debt - balance
		state := row(key.venue)
		state.DeficitInsuranceResidual += debt - insurance
		state.DeficitBalanceResidual += debt - balance
	}
	for key, insurance := range insuranceByInstant {
		if _, exists := debtByInstant[key]; !exists && insurance != 0 {
			result.DeficitMismatchInstants++
			result.DeficitInsuranceResidual -= insurance
			row(key.venue).DeficitInsuranceResidual -= insurance
		}
	}
	for key, balance := range balanceByInstant {
		if _, exists := debtByInstant[key]; !exists && balance != 0 {
			result.DeficitMismatchInstants++
			result.DeficitBalanceResidual -= balance
			row(key.venue).DeficitBalanceResidual -= balance
		}
	}
	result.AffectedAccounts = len(accounts)
	for venue, state := range venueRows {
		state.AffectedAccounts = len(venueAccounts[venue])
		result.ByVenue = append(result.ByVenue, *state)
	}
	sort.Slice(result.ByVenue, func(i, j int) bool { return result.ByVenue[i].VenueID < result.ByVenue[j].VenueID })
	return result, nil
}

func reducedSameSide(before, after int64) bool {
	if before == 0 || absLiquidationSize(after) >= absLiquidationSize(before) {
		return false
	}
	return after == 0 || (before < 0) == (after < 0)
}

func absLiquidationSize(value int64) uint64 {
	if value >= 0 {
		return uint64(value)
	}
	// Negating MinInt64 overflows, whereas unsigned subtraction retains its
	// magnitude and keeps this validator total for malformed evidence too.
	return uint64(-(value + 1)) + 1
}
