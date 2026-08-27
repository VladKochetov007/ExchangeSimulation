package analysis

import (
	"fmt"
	"math"
	"sort"
)

// EcologyRole summarizes marked wealth for one participant class. Equity is
// reported in the run's report asset precision; the return is dimensionless.
// It is an outcome description, not an attribution of skill, because marked
// wealth includes inventory revaluation as well as realised trading PnL.
type EcologyRole struct {
	Role                string  `json:"role"`
	Accounts            int     `json:"accounts"`
	InitialEquity       int64   `json:"initial_equity"`
	TerminalEquity      int64   `json:"terminal_equity"`
	EquityReturn        float64 `json:"equity_return"`
	InitialWealthShare  float64 `json:"initial_wealth_share"`
	TerminalWealthShare float64 `json:"terminal_wealth_share"`
}

// Ecology is the report-derived wealth and concentration view of a run. It is
// deliberately separate from RoleAudit: fills, turnover, and fees come from
// raw event evidence, while this result comes from independently saved account
// snapshots.
type Ecology struct {
	Roles                    []EcologyRole `json:"roles"`
	InitialEquity            int64         `json:"initial_equity"`
	TerminalEquity           int64         `json:"terminal_equity"`
	InitialConcentrationHHI  float64       `json:"initial_concentration_hhi"`
	TerminalConcentrationHHI float64       `json:"terminal_concentration_hhi"`
	InitialAccounts          int           `json:"initial_accounts"`
	TerminalAccounts         int           `json:"terminal_accounts"`
}

// MeasureEcology compares the initial and terminal account reports. It refuses
// to silently compare a changing account population, because that would turn
// an omitted participant into an apparently successful extinction event.
func (r *Run) MeasureEcology() (*Ecology, error) {
	initial := make(map[Participant]AccountRow, len(r.Report.InitialAccounts))
	for _, row := range r.Report.InitialAccounts {
		key := Participant{VenueID: row.VenueID, ClientID: row.ClientID}
		if _, exists := initial[key]; exists {
			return nil, fmt.Errorf("ecology: duplicate initial account %s/%d", key.VenueID, key.ClientID)
		}
		initial[key] = row
	}
	terminal := make(map[Participant]AccountRow, len(r.Report.TerminalAccounts))
	for _, row := range r.Report.TerminalAccounts {
		key := Participant{VenueID: row.VenueID, ClientID: row.ClientID}
		if _, exists := terminal[key]; exists {
			return nil, fmt.Errorf("ecology: duplicate terminal account %s/%d", key.VenueID, key.ClientID)
		}
		terminal[key] = row
	}
	if len(initial) == 0 || len(terminal) == 0 {
		return nil, fmt.Errorf("ecology: missing initial or terminal accounts")
	}
	if len(initial) != len(terminal) {
		return nil, fmt.Errorf("ecology: account population changed: initial=%d terminal=%d", len(initial), len(terminal))
	}

	type totals struct {
		accounts int
		initial  int64
		terminal int64
	}
	byRole := make(map[string]*totals)
	result := &Ecology{InitialAccounts: len(initial), TerminalAccounts: len(terminal)}
	for key, initialRow := range initial {
		terminalRow, exists := terminal[key]
		if !exists {
			return nil, fmt.Errorf("ecology: initial account %s/%d missing from terminal report", key.VenueID, key.ClientID)
		}
		role := RoleGroup(initialRow.Role)
		if terminalRole := RoleGroup(terminalRow.Role); terminalRole != role {
			return nil, fmt.Errorf("ecology: role changed for %s/%d: %q -> %q", key.VenueID, key.ClientID, role, terminalRole)
		}
		state := byRole[role]
		if state == nil {
			state = &totals{}
			byRole[role] = state
		}
		var err error
		if state.initial, err = addEquity(state.initial, initialRow.Account.Equity); err != nil {
			return nil, fmt.Errorf("ecology: %s initial: %w", role, err)
		}
		if state.terminal, err = addEquity(state.terminal, terminalRow.Account.Equity); err != nil {
			return nil, fmt.Errorf("ecology: %s terminal: %w", role, err)
		}
		if result.InitialEquity, err = addEquity(result.InitialEquity, initialRow.Account.Equity); err != nil {
			return nil, fmt.Errorf("ecology: total initial: %w", err)
		}
		if result.TerminalEquity, err = addEquity(result.TerminalEquity, terminalRow.Account.Equity); err != nil {
			return nil, fmt.Errorf("ecology: total terminal: %w", err)
		}
		state.accounts++
	}
	for key := range terminal {
		if _, exists := initial[key]; !exists {
			return nil, fmt.Errorf("ecology: terminal account %s/%d absent from initial report", key.VenueID, key.ClientID)
		}
	}
	roles := make([]string, 0, len(byRole))
	for role := range byRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	for _, role := range roles {
		state := byRole[role]
		row := EcologyRole{Role: role, Accounts: state.accounts, InitialEquity: state.initial, TerminalEquity: state.terminal}
		if state.initial != 0 {
			row.EquityReturn = float64(state.terminal-state.initial) / float64(state.initial)
		}
		if result.InitialEquity > 0 {
			row.InitialWealthShare = float64(state.initial) / float64(result.InitialEquity)
			result.InitialConcentrationHHI += row.InitialWealthShare * row.InitialWealthShare
		}
		if result.TerminalEquity > 0 {
			row.TerminalWealthShare = float64(state.terminal) / float64(result.TerminalEquity)
			result.TerminalConcentrationHHI += row.TerminalWealthShare * row.TerminalWealthShare
		}
		result.Roles = append(result.Roles, row)
	}
	return result, nil
}

func addEquity(left, right int64) (int64, error) {
	if right > 0 && left > math.MaxInt64-right {
		return 0, fmt.Errorf("int64 overflow")
	}
	if right < 0 && left < math.MinInt64-right {
		return 0, fmt.Errorf("int64 overflow")
	}
	return left + right, nil
}
