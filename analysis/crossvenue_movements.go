package analysis

import (
	"fmt"
	"sort"

	etypes "exchange_sim/types"
)

type CrossVenueRouterMovementAccount struct {
	VenueID              string
	ClientID             uint64
	InitialBase          int64
	InitialQuote         int64
	DepositFrame         uint64
	Settlements          int
	FirstSettlementFrame uint64
	LastSettlementFrame  uint64
}

type crossVenueRouterMovement struct {
	event  Event
	record balanceChangeRecord
}

type crossVenueExpectedSettlement struct {
	venueID    string
	timestamp  int64
	baseDelta  int64
	quoteDelta int64
}

// AuditCrossVenueRouterMovements scans all venue evidence files so a router
// transfer, borrow or financing change cannot hide in a non-spot log. It
// requires each post-deposit movement to match one canonical settled fill.
func (r *Run) AuditCrossVenueRouterMovements(venues [2]string, clients map[string]uint64, fills []CrossVenueExchangeFill, convention CrossVenueAccountConvention) (map[string]CrossVenueRouterMovementAccount, error) {
	if r == nil || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || len(clients) != 2 ||
		clients[venues[0]] == 0 || clients[venues[1]] == 0 {
		return nil, fmt.Errorf("cross-venue movements: invalid selected accounts")
	}
	if _, err := ReconcileCrossVenueRouterAccounts(r.Report, venues, clients, fills, convention); err != nil {
		return nil, fmt.Errorf("cross-venue movements: account/fill totals: %w", err)
	}
	expected, err := expectedCrossVenueSettlements(fills, convention)
	if err != nil {
		return nil, err
	}
	var movements []crossVenueRouterMovement
	var callbackErr error
	if err := r.Scan(ScanOptions{Events: []string{"balance_change"}, Workers: 1}, func(event Event) {
		if callbackErr != nil {
			return
		}
		var record balanceChangeRecord
		if err := decodeRequiredJSON(event.Raw(), &record, "timestamp", "client_id", "symbol", "reason", "changes"); err != nil {
			callbackErr = fmt.Errorf("cross-venue movements: malformed balance-change payload: %w", err)
			return
		}
		if record.ClientID != event.ClientID {
			callbackErr = fmt.Errorf("cross-venue movements: outer and payload client identities differ")
			return
		}
		if clients[event.VenueID] != event.ClientID {
			if event.ClientID == clients[venues[0]] || event.ClientID == clients[venues[1]] {
				callbackErr = fmt.Errorf("cross-venue movements: selected router client appears in an unregistered venue")
			}
			return
		}
		if event.GlobalSequence == 0 || event.SimTS <= 0 || record.Timestamp != event.SimTS || len(record.Changes) != 2 {
			callbackErr = fmt.Errorf("cross-venue movements: malformed router balance change")
			return
		}
		movements = append(movements, crossVenueRouterMovement{event: event, record: record})
	}); err != nil {
		return nil, err
	}
	if callbackErr != nil {
		return nil, callbackErr
	}
	sort.Slice(movements, func(left, right int) bool {
		return movements[left].event.GlobalSequence < movements[right].event.GlobalSequence
	})
	accounts := make(map[string]CrossVenueRouterMovementAccount, 2)
	balances := make(map[string]map[string]int64, 2)
	for _, venue := range venues {
		initial, err := oneCrossVenueAccount(r.Report.InitialAccounts, venue, clients[venue], "initial")
		if err != nil {
			return nil, err
		}
		initialBalances, err := crossVenueAccountBalances(initial.Account, convention.BaseAsset, convention.QuoteAsset)
		if err != nil {
			return nil, fmt.Errorf("cross-venue movements: invalid initial account: %w", err)
		}
		accounts[venue] = CrossVenueRouterMovementAccount{
			VenueID: venue, ClientID: clients[venue],
			InitialBase: initialBalances[convention.BaseAsset], InitialQuote: initialBalances[convention.QuoteAsset],
		}
		balances[venue] = make(map[string]int64, 2)
	}
	seenDeposit := make(map[string]bool, 2)
	var previousFrame uint64
	for _, movement := range movements {
		event, record := movement.event, movement.record
		if event.GlobalSequence <= previousFrame {
			return nil, fmt.Errorf("cross-venue movements: duplicate or unordered global frame")
		}
		previousFrame = event.GlobalSequence
		baseDelta, quoteDelta, err := applyCrossVenueRouterMovement(record, balances[event.VenueID], convention)
		if err != nil {
			return nil, err
		}
		account := accounts[event.VenueID]
		if !seenDeposit[event.VenueID] {
			initial, err := oneCrossVenueAccount(r.Report.InitialAccounts, event.VenueID, event.ClientID, "initial")
			if err != nil {
				return nil, err
			}
			if record.Reason != "initial_deposit" || record.Symbol != "" || event.SimTS != initial.Account.Timestamp ||
				balances[event.VenueID][convention.BaseAsset] != account.InitialBase ||
				balances[event.VenueID][convention.QuoteAsset] != account.InitialQuote {
				return nil, fmt.Errorf("cross-venue movements: initial deposit does not anchor router account")
			}
			seenDeposit[event.VenueID] = true
			account.DepositFrame = event.GlobalSequence
			accounts[event.VenueID] = account
			continue
		}
		if record.Reason != "trade_settlement" || record.Symbol != convention.Symbol {
			return nil, fmt.Errorf("cross-venue movements: undeclared router movement %q", record.Reason)
		}
		key := crossVenueExpectedSettlement{venueID: event.VenueID, timestamp: event.SimTS, baseDelta: baseDelta, quoteDelta: quoteDelta}
		if expected[key] == 0 {
			return nil, fmt.Errorf("cross-venue movements: settlement lacks matching canonical fill")
		}
		expected[key]--
		account.Settlements++
		if account.FirstSettlementFrame == 0 {
			account.FirstSettlementFrame = event.GlobalSequence
		}
		account.LastSettlementFrame = event.GlobalSequence
		accounts[event.VenueID] = account
	}
	for _, venue := range venues {
		if !seenDeposit[venue] {
			return nil, fmt.Errorf("cross-venue movements: venue %s lacks an initial deposit", venue)
		}
		terminal, err := oneCrossVenueAccount(r.Report.TerminalAccounts, venue, clients[venue], "terminal_post_mark")
		if err != nil {
			return nil, err
		}
		terminalBalances, err := crossVenueAccountBalances(terminal.Account, convention.BaseAsset, convention.QuoteAsset)
		if err != nil {
			return nil, err
		}
		if balances[venue][convention.BaseAsset] != terminalBalances[convention.BaseAsset] ||
			balances[venue][convention.QuoteAsset] != terminalBalances[convention.QuoteAsset] {
			return nil, fmt.Errorf("cross-venue movements: venue %s terminal account breaks movement chain", venue)
		}
	}
	for _, remaining := range expected {
		if remaining != 0 {
			return nil, fmt.Errorf("cross-venue movements: canonical fill lacks matching balance movement")
		}
	}
	return accounts, nil
}

func applyCrossVenueRouterMovement(record balanceChangeRecord, balances map[string]int64, convention CrossVenueAccountConvention) (int64, int64, error) {
	seen := make(map[string]bool, 2)
	var baseDelta, quoteDelta int64
	for _, change := range record.Changes {
		if change.Wallet != "spot" || seen[change.Asset] || change.Asset != convention.BaseAsset && change.Asset != convention.QuoteAsset ||
			change.OldBalance != balances[change.Asset] {
			return 0, 0, fmt.Errorf("cross-venue movements: unexpected wallet, asset or broken balance chain")
		}
		expectedBalance, ok := etypes.TryAdd(change.OldBalance, change.Delta)
		if !ok || expectedBalance != change.NewBalance || expectedBalance < 0 {
			return 0, 0, fmt.Errorf("cross-venue movements: inconsistent or negative account balance")
		}
		seen[change.Asset] = true
		balances[change.Asset] = change.NewBalance
		if change.Asset == convention.BaseAsset {
			baseDelta = change.Delta
		} else {
			quoteDelta = change.Delta
		}
	}
	if !seen[convention.BaseAsset] || !seen[convention.QuoteAsset] {
		return 0, 0, fmt.Errorf("cross-venue movements: incomplete base/quote movement")
	}
	return baseDelta, quoteDelta, nil
}

func expectedCrossVenueSettlements(fills []CrossVenueExchangeFill, convention CrossVenueAccountConvention) (map[crossVenueExpectedSettlement]int, error) {
	expected := make(map[crossVenueExpectedSettlement]int, len(fills))
	for _, fill := range fills {
		notional, ok := etypes.TryMulDiv(fill.Qty, fill.Price, convention.BasePrecision)
		if !ok {
			return nil, fmt.Errorf("cross-venue movements: fill notional overflows")
		}
		baseDelta, quoteDelta := fill.Qty, notional
		if fill.Side == "BUY" {
			cost, ok := etypes.TryAdd(notional, fill.FeeAmount)
			if !ok || cost < 0 {
				return nil, fmt.Errorf("cross-venue movements: buy cost overflows")
			}
			quoteDelta = -cost
		} else if fill.Side == "SELL" {
			baseDelta = -fill.Qty
			quoteDelta, ok = etypes.TrySub(notional, fill.FeeAmount)
			if !ok {
				return nil, fmt.Errorf("cross-venue movements: sell proceeds overflow")
			}
		} else {
			return nil, fmt.Errorf("cross-venue movements: unsupported fill side")
		}
		key := crossVenueExpectedSettlement{venueID: fill.Event.VenueID, timestamp: fill.Event.SimTS, baseDelta: baseDelta, quoteDelta: quoteDelta}
		expected[key]++
	}
	return expected, nil
}
