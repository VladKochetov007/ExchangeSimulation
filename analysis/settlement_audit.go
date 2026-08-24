package analysis

import (
	"sort"
	"sync"
)

// SettlementAuditOptions selects the contracts to audit.
type SettlementAuditOptions struct {
	Files         []string
	FilesSelected bool
	BasePrecision int64
	// Symbols, when non-empty, restricts the audit to these contracts.
	Symbols []string
}

// SettlementCheck is one contract's settlement, recomputed from the positions
// and the published settlement price rather than read from the payouts.
type SettlementCheck struct {
	VenueID         string `json:"venue_id"`
	Symbol          string `json:"symbol"`
	SettlementPrice int64  `json:"settlement_price"`
	ExpiryNano      int64  `json:"expiry_nano"`
	// Holders and NetSize describe the book at the moment before settlement.
	Holders int   `json:"holders"`
	NetSize int64 `json:"net_size"`
	// ExpectedPayout is what the settlement price and each holder's entry
	// price imply, and PaidOut is what the venue actually credited.
	ExpectedPayout int64 `json:"expected_payout"`
	PaidOut        int64 `json:"paid_out"`
	Residual       int64 `json:"residual"`
	// PaidAccounts is how many accounts received a settlement credit, which
	// must equal Holders: a holder that is not paid has been robbed, and an
	// account paid without a position has been given money.
	PaidAccounts int `json:"paid_accounts"`
	// TradesAfterExpiry counts fills recorded after the expiry instant, which
	// must be zero.
	TradesAfterExpiry int `json:"trades_after_expiry"`
	// Unrepresentable means the signed fixed-point payout cannot be represented
	// as int64. Such a contract is unresolved, never silently converted to a
	// zero payout and scored as a matching settlement.
	Unrepresentable bool `json:"unrepresentable"`
}

// SettlementAudit is the independent check of expiry semantics.
type SettlementAudit struct {
	Checks []SettlementCheck `json:"checks"`
	// Mismatched counts contracts whose recomputed payout disagrees with what
	// was paid, and Unpaid counts contracts where the number of paid accounts
	// differs from the number of holders.
	Mismatched int `json:"mismatched"`
	Unpaid     int `json:"unpaid"`
	// TotalTradesAfterExpiry is the count across all audited contracts.
	TotalTradesAfterExpiry int `json:"total_trades_after_expiry"`
	// ArithmeticFailures counts contracts for which the independent payout
	// reconstruction exceeded the artifact's int64 representation.
	ArithmeticFailures int `json:"arithmetic_failures"`
}

// MeasureSettlements recomputes every dated-future settlement.
//
// It reads three independent streams: the position updates that say who held
// what at expiry, the reference-data announcement that says what the contract
// settled at, and the balance changes that say who was paid. A venue could
// satisfy any two of those and fail the third.
//
// Two passes are needed rather than one. A holder's position is settled and
// zeroed after expiry, so the position that faced settlement is its last
// update at or before the expiry instant — and the expiry instant is only
// known once the settlement announcement has been read. A single streaming
// pass that keeps the latest update overall silently drops every holder whose
// close-out was logged after expiry, which is all of them.
func (r *Run) MeasureSettlements(opts SettlementAuditOptions) (*SettlementAudit, error) {
	type instrumentPayload struct {
		Action          string `json:"action"`
		Symbol          string `json:"symbol"`
		InstrumentType  string `json:"instrument_type"`
		ExpiryNano      int64  `json:"expiry_nano"`
		SettlementPrice int64  `json:"settlement_price"`
		Timestamp       int64  `json:"timestamp"`
	}
	type positionPayload struct {
		Timestamp     int64  `json:"timestamp"`
		ClientID      uint64 `json:"client_id"`
		Symbol        string `json:"symbol"`
		NewSize       int64  `json:"new_size"`
		NewEntryPrice int64  `json:"new_entry_price"`
	}
	type fillPayload struct {
		Symbol string `json:"symbol"`
		Qty    int64  `json:"qty"`
	}

	wanted := make(map[string]struct{}, len(opts.Symbols))
	for _, symbol := range opts.Symbols {
		wanted[symbol] = struct{}{}
	}
	interesting := func(symbol string) bool {
		if len(wanted) == 0 {
			return true
		}
		_, ok := wanted[symbol]
		return ok
	}

	var mu sync.Mutex
	type settledContract struct {
		price, expiry int64
	}
	settled := make(map[markKey]settledContract)
	type holding struct {
		size, entry, at int64
	}
	holdings := make(map[positionKey]*holding)
	expiries := make(map[markKey]int64)
	paid := make(map[markKey]struct {
		amount   int64
		accounts int
	})
	fillTimes := make(map[markKey][]int64)

	// First pass: learn every contract's expiry instant, so the second pass can
	// tell a pre-settlement position from a post-settlement close-out.
	expiryScan := ScanOptions{Events: []string{"instrument_settled"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(expiryScan, func(event Event) {
		var payload instrumentPayload
		if event.Decode(&payload) != nil || payload.InstrumentType != "FUTURE" || !interesting(payload.Symbol) {
			return
		}
		mu.Lock()
		expiries[markKey{event.VenueID, payload.Symbol}] = payload.ExpiryNano
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	scan := ScanOptions{
		Events:        []string{"instrument_settled", "position_update", "balance_change", "OrderFill"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		switch event.Name {
		case "instrument_settled":
			var payload instrumentPayload
			if event.Decode(&payload) != nil || payload.InstrumentType != "FUTURE" || !interesting(payload.Symbol) {
				return
			}
			mu.Lock()
			settled[markKey{event.VenueID, payload.Symbol}] = settledContract{payload.SettlementPrice, payload.ExpiryNano}
			mu.Unlock()
		case "position_update":
			var payload positionPayload
			if event.Decode(&payload) != nil || !interesting(payload.Symbol) {
				return
			}
			at := payload.Timestamp
			if at == 0 {
				at = event.SimTS
			}
			mu.Lock()
			// Only updates at or before the contract's expiry describe the
			// position that faced settlement.
			expiry, known := expiries[markKey{event.VenueID, payload.Symbol}]
			if known && at > expiry {
				mu.Unlock()
				return
			}
			key := positionKey{event.VenueID, payload.ClientID, payload.Symbol}
			state := holdings[key]
			if state == nil || at >= state.at {
				holdings[key] = &holding{size: payload.NewSize, entry: payload.NewEntryPrice, at: at}
			}
			mu.Unlock()
		case "balance_change":
			var record balanceChangeRecord
			if event.Decode(&record) != nil || record.Reason != "expiry_settlement" || !interesting(record.Symbol) {
				return
			}
			total := int64(0)
			for _, change := range record.Changes {
				total += change.Delta
			}
			mu.Lock()
			key := markKey{event.VenueID, record.Symbol}
			entry := paid[key]
			entry.amount += total
			entry.accounts++
			paid[key] = entry
			mu.Unlock()
		case "OrderFill":
			var payload fillPayload
			if event.Decode(&payload) != nil || payload.Qty <= 0 {
				return
			}
			symbol := event.Symbol
			if symbol == "" {
				symbol = payload.Symbol
			}
			if !interesting(symbol) {
				return
			}
			mu.Lock()
			key := markKey{event.VenueID, symbol}
			fillTimes[key] = append(fillTimes[key], event.SimTS)
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	precision := opts.BasePrecision
	if precision <= 0 {
		precision = 1
	}
	result := &SettlementAudit{}
	for key, contract := range settled {
		check := SettlementCheck{
			VenueID: key.venue, Symbol: key.symbol,
			SettlementPrice: contract.price, ExpiryNano: contract.expiry,
		}
		for holderKey, state := range holdings {
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
			change, ok := exactSub(contract.price, state.entry)
			if !ok {
				check.Unrepresentable = true
				continue
			}
			expected, ok := mulDiv(change, state.size, precision)
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
		}
		entry := paid[key]
		check.PaidOut = entry.amount
		check.PaidAccounts = entry.accounts
		if residual, ok := exactSub(check.PaidOut, check.ExpectedPayout); ok {
			check.Residual = residual
		} else {
			check.Unrepresentable = true
		}
		for _, at := range fillTimes[key] {
			if at > contract.expiry {
				check.TradesAfterExpiry++
			}
		}
		if check.Unrepresentable {
			result.ArithmeticFailures++
		} else if check.Residual != 0 {
			result.Mismatched++
		}
		if check.PaidAccounts != check.Holders {
			result.Unpaid++
		}
		result.TotalTradesAfterExpiry += check.TradesAfterExpiry
		result.Checks = append(result.Checks, check)
	}
	sort.Slice(result.Checks, func(i, j int) bool {
		if result.Checks[i].VenueID != result.Checks[j].VenueID {
			return result.Checks[i].VenueID < result.Checks[j].VenueID
		}
		return result.Checks[i].Symbol < result.Checks[j].Symbol
	})
	return result, nil
}
