package analysis

import (
	"fmt"

	etypes "exchange_sim/types"
)

type CrossVenueAccountDelta struct {
	VenueID       string
	ClientID      uint64
	BaseDelta     int64
	QuoteDelta    int64
	FilledBase    int64
	FillQuoteFlow int64
	QuoteFees     int64
	FillCount     int
}

type CrossVenueAccountConvention struct {
	Symbol        string
	BaseAsset     string
	QuoteAsset    string
	BasePrecision int64
	TakerFeeBps   int64
}

// ReconcileCrossVenueRouterAccounts requires settled spot fills to explain
// every base/quote change in each prefunded venue account. No borrowing, transfer,
// derivative position or undeclared asset may be hidden in this endpoint.
func ReconcileCrossVenueRouterAccounts(report Report, venues [2]string, clients map[string]uint64, fills []CrossVenueExchangeFill, convention CrossVenueAccountConvention) (map[string]CrossVenueAccountDelta, error) {
	if venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || len(clients) != 2 || clients[venues[0]] == 0 || clients[venues[1]] == 0 || convention.Symbol == "" || convention.BaseAsset == "" || convention.QuoteAsset == "" || convention.BaseAsset == convention.QuoteAsset || convention.BasePrecision <= 0 || convention.TakerFeeBps < 0 {
		return nil, fmt.Errorf("cross-venue accounts: invalid convention")
	}
	result := make(map[string]CrossVenueAccountDelta, 2)
	for _, venue := range venues {
		clientID := clients[venue]
		initial, err := oneCrossVenueAccount(report.InitialAccounts, venue, clientID, "initial")
		if err != nil {
			return nil, err
		}
		terminal, err := oneCrossVenueAccount(report.TerminalAccounts, venue, clientID, "terminal_post_mark")
		if err != nil {
			return nil, err
		}
		if initial.Role == "" || terminal.Role != initial.Role || initial.Account.Timestamp > terminal.Account.Timestamp {
			return nil, fmt.Errorf("cross-venue accounts: %s/%d has inconsistent role or account time", venue, clientID)
		}
		initialBalances, err := crossVenueAccountBalances(initial.Account, convention.BaseAsset, convention.QuoteAsset)
		if err != nil {
			return nil, fmt.Errorf("cross-venue accounts: %s/%d initial: %w", venue, clientID, err)
		}
		terminalBalances, err := crossVenueAccountBalances(terminal.Account, convention.BaseAsset, convention.QuoteAsset)
		if err != nil {
			return nil, fmt.Errorf("cross-venue accounts: %s/%d terminal: %w", venue, clientID, err)
		}
		baseDelta, baseOK := etypes.TrySub(terminalBalances[convention.BaseAsset], initialBalances[convention.BaseAsset])
		quoteDelta, quoteOK := etypes.TrySub(terminalBalances[convention.QuoteAsset], initialBalances[convention.QuoteAsset])
		if !baseOK || !quoteOK {
			return nil, fmt.Errorf("cross-venue accounts: %s/%d balance delta overflows", venue, clientID)
		}
		result[venue] = CrossVenueAccountDelta{VenueID: venue, ClientID: clientID, BaseDelta: baseDelta, QuoteDelta: quoteDelta}
	}
	for _, fill := range fills {
		row, selected := result[fill.Event.VenueID]
		if !selected || row.ClientID != fill.Event.ClientID || fill.Symbol != convention.Symbol || fill.Role != "taker" || fill.Qty <= 0 || fill.Price <= 0 || fill.FeeAsset != convention.QuoteAsset {
			return nil, fmt.Errorf("cross-venue accounts: fill outside prefunded spot router path")
		}
		notional, ok := etypes.TryMulDiv(fill.Qty, fill.Price, convention.BasePrecision)
		if !ok {
			return nil, fmt.Errorf("cross-venue accounts: fill notional overflows")
		}
		fee, ok := etypes.TryMulBps(notional, convention.TakerFeeBps)
		if !ok || fee != fill.FeeAmount {
			return nil, fmt.Errorf("cross-venue accounts: fill fee differs from declared quote taker fee")
		}
		baseFlow, quoteFlow := fill.Qty, int64(0)
		if fill.Side == "BUY" {
			quoteFlow, ok = etypes.TryAdd(notional, fee)
			if !ok {
				return nil, fmt.Errorf("cross-venue accounts: buy cashflow overflows")
			}
			quoteFlow = -quoteFlow
		} else if fill.Side == "SELL" {
			baseFlow = -baseFlow
			quoteFlow, ok = etypes.TrySub(notional, fee)
			if !ok {
				return nil, fmt.Errorf("cross-venue accounts: sell cashflow overflows")
			}
		} else {
			return nil, fmt.Errorf("cross-venue accounts: fill has unsupported side")
		}
		row.FilledBase, ok = etypes.TryAdd(row.FilledBase, baseFlow)
		if !ok {
			return nil, fmt.Errorf("cross-venue accounts: base flow overflows")
		}
		row.FillQuoteFlow, ok = etypes.TryAdd(row.FillQuoteFlow, quoteFlow)
		if !ok {
			return nil, fmt.Errorf("cross-venue accounts: quote flow overflows")
		}
		row.QuoteFees, ok = etypes.TryAdd(row.QuoteFees, fee)
		if !ok {
			return nil, fmt.Errorf("cross-venue accounts: fee sum overflows")
		}
		row.FillCount++
		result[fill.Event.VenueID] = row
	}
	for _, venue := range venues {
		row := result[venue]
		if row.BaseDelta != row.FilledBase || row.QuoteDelta != row.FillQuoteFlow {
			return nil, fmt.Errorf("cross-venue accounts: %s/%d terminal balances disagree with settled fills", venue, row.ClientID)
		}
	}
	return result, nil
}

func oneCrossVenueAccount(rows []AccountRow, venue string, clientID uint64, phase string) (AccountRow, error) {
	var selected *AccountRow
	for index := range rows {
		if rows[index].VenueID != venue || rows[index].ClientID != clientID {
			continue
		}
		if selected != nil || rows[index].Phase != phase {
			return AccountRow{}, fmt.Errorf("cross-venue accounts: duplicate or wrong-phase %s/%d %s account", venue, clientID, phase)
		}
		selected = &rows[index]
	}
	if selected == nil {
		return AccountRow{}, fmt.Errorf("cross-venue accounts: missing %s/%d %s account", venue, clientID, phase)
	}
	return *selected, nil
}

func crossVenueAccountBalances(account Account, baseAsset, quoteAsset string) (map[string]int64, error) {
	if len(account.PerpBalances) != 0 || len(account.Positions) != 0 {
		return nil, fmt.Errorf("router has derivative exposure")
	}
	for _, borrowed := range account.Borrowed {
		if borrowed != 0 {
			return nil, fmt.Errorf("router has account debt")
		}
	}
	balances := make(map[string]int64, 2)
	for _, row := range account.SpotBalances {
		if _, duplicate := balances[row.Asset]; duplicate || row.Borrowed != 0 || row.Interest != 0 {
			return nil, fmt.Errorf("duplicate asset or router financing")
		}
		if row.Locked != 0 {
			return nil, fmt.Errorf("router inventory remains locked")
		}
		total, ok := etypes.TryAdd(row.Free, row.Locked)
		if !ok || total != row.NetAsset {
			return nil, fmt.Errorf("spot wallet free/locked/net identity fails")
		}
		if row.Asset != baseAsset && row.Asset != quoteAsset && row.NetAsset != 0 {
			return nil, fmt.Errorf("router holds undeclared asset %s", row.Asset)
		}
		balances[row.Asset] = row.NetAsset
	}
	if _, ok := balances[baseAsset]; !ok {
		return nil, fmt.Errorf("missing %s wallet", baseAsset)
	}
	if _, ok := balances[quoteAsset]; !ok {
		return nil, fmt.Errorf("missing %s wallet", quoteAsset)
	}
	return balances, nil
}
