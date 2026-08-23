package analysis

import (
	"fmt"
	"sort"
	"sync"
)

// LiquidationAudit independently reconciles the observable liquidation,
// account-deficit, and insurance-fund streams. It does not infer that a
// liquidation was economically correct merely because an event was emitted.
type LiquidationAudit struct {
	Liquidations             int                `json:"liquidations"`
	LiquidationChecks        int                `json:"liquidation_checks"`
	AffectedAccounts         int                `json:"affected_accounts"`
	LiquidationsWithDeficit  int                `json:"liquidations_with_deficit"`
	TotalDeficit             int64              `json:"total_deficit"`
	InsuranceDeficit         int64              `json:"insurance_deficit"`
	BalanceDeficitCredit     int64              `json:"balance_deficit_credit"`
	DeficitInsuranceResidual int64              `json:"deficit_insurance_residual"`
	DeficitBalanceResidual   int64              `json:"deficit_balance_residual"`
	DeficitMismatchInstants  int                `json:"deficit_mismatch_instants"`
	InvalidLiquidations      int                `json:"invalid_liquidations"`
	ByVenue                  []LiquidationVenue `json:"by_venue"`
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

	var mu sync.Mutex
	result := &LiquidationAudit{}
	accounts := make(map[Participant]struct{})
	venueAccounts := make(map[string]map[Participant]struct{})
	venueRows := make(map[string]*LiquidationVenue)
	debtByInstant := make(map[liquidationInstant]int64)
	insuranceByInstant := make(map[liquidationInstant]int64)
	balanceByInstant := make(map[liquidationInstant]int64)
	row := func(venue string) *LiquidationVenue {
		out := venueRows[venue]
		if out == nil {
			out = &LiquidationVenue{VenueID: venue}
			venueRows[venue] = out
		}
		return out
	}
	if err := r.Scan(ScanOptions{Events: []string{"liquidation", "liquidation_check", "balance_change", "insurance_fund"}}, func(event Event) {
		switch event.Name {
		case "liquidation_check":
			mu.Lock()
			result.LiquidationChecks++
			mu.Unlock()
		case "liquidation":
			var payload liquidationPayload
			if event.Decode(&payload) != nil {
				return
			}
			symbol := payload.Symbol
			if symbol == "" {
				symbol = event.Symbol
			}
			mu.Lock()
			result.Liquidations++
			state := row(event.VenueID)
			state.Liquidations++
			participant := Participant{VenueID: event.VenueID, ClientID: event.ClientID}
			accounts[participant] = struct{}{}
			if venueAccounts[event.VenueID] == nil {
				venueAccounts[event.VenueID] = make(map[Participant]struct{})
			}
			venueAccounts[event.VenueID][participant] = struct{}{}
			if symbol == "" || payload.FillPrice <= 0 || payload.PositionSize == 0 || payload.RemainingDebt < 0 {
				result.InvalidLiquidations++
			}
			if payload.RemainingDebt > 0 {
				key := liquidationInstant{venue: event.VenueID, symbol: symbol, timestamp: event.SimTS}
				debtByInstant[key] += payload.RemainingDebt
				result.LiquidationsWithDeficit++
				result.TotalDeficit += payload.RemainingDebt
				state.LiquidationsWithDeficit++
				state.TotalDeficit += payload.RemainingDebt
			}
			mu.Unlock()
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
			mu.Lock()
			balanceByInstant[liquidationInstant{venue: event.VenueID, symbol: symbol, timestamp: timestamp}] += credit
			result.BalanceDeficitCredit += credit
			row(event.VenueID).BalanceDeficitCredit += credit
			mu.Unlock()
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
			mu.Lock()
			insuranceByInstant[liquidationInstant{venue: event.VenueID, symbol: symbol, timestamp: timestamp}] -= payload.Delta
			result.InsuranceDeficit -= payload.Delta
			row(event.VenueID).InsuranceDeficit -= payload.Delta
			mu.Unlock()
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
