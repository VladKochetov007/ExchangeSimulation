package analysis

import (
	"sort"
	"sync"
)

// ExpiryFillCheck records one expired future or option and every persisted fill
// record that arrived after the contractual expiry in its listing metadata.
// Settlement metadata is a consistency check, not the source of truth: a
// broken expiry path can omit it altogether.
type ExpiryFillCheck struct {
	VenueID          string `json:"venue_id"`
	Symbol           string `json:"symbol"`
	InstrumentType   string `json:"instrument_type"`
	ExpiryNano       int64  `json:"expiry_nano"`
	ListedNano       int64  `json:"listed_nano"`
	SettlementNano   int64  `json:"settlement_nano"`
	Settled          bool   `json:"settled"`
	FillRecords      int    `json:"fill_records"`
	FillsAfterExpiry int    `json:"fills_after_expiry"`
}

// ExpiryFillAudit verifies the contractual lifetime boundary for every
// persisted expirable instrument. It purposely does not infer expiry from the
// settlement event's simulation timestamp: a delayed or omitted expiry
// implementation would make that circular and conceal exactly the bug being
// tested.
//
// FillRecords count participant-side OrderFill records, so a matched trade
// usually contributes two records.  The invariant is the zero count of those
// records after the independently announced contractual expiry.
type ExpiryFillAudit struct {
	Contracts                 int               `json:"contracts"`
	Futures                   int               `json:"futures"`
	Options                   int               `json:"options"`
	ExpiredContracts          int               `json:"expired_contracts"`
	SettledContracts          int               `json:"settled_contracts"`
	ExpiredUnsettledContracts int               `json:"expired_unsettled_contracts"`
	SettlementWithoutListing  int               `json:"settlement_without_listing"`
	MetadataMismatches        int               `json:"metadata_mismatches"`
	MissingExpiryMetadata     int               `json:"missing_expiry_metadata"`
	FillRecords               int               `json:"fill_records"`
	FillsAfterExpiry          int               `json:"fills_after_expiry"`
	Checks                    []ExpiryFillCheck `json:"checks"`
}

type expiryFillKey struct {
	venue, symbol string
}

type expiryFillContract struct {
	kind, symbol     string
	expiry           int64
	listed           bool
	listing          int64
	settled          bool
	settlement       int64
	metadataMismatch bool
}

// MeasureExpiryFills independently joins contract listing metadata to every
// persisted OrderFill.  A settlement announcement confirms the contract was
// delisted, but is not the source of truth for which contracts must have been
// delisted: a broken expiry path could omit that announcement altogether. It
// covers both dated futures and European options; payout correctness remains
// the responsibility of the narrower settlement and exercise audits.
func (r *Run) MeasureExpiryFills() (*ExpiryFillAudit, error) {
	type instrumentPayload struct {
		Symbol         string `json:"symbol"`
		InstrumentType string `json:"instrument_type"`
		ExpiryNano     int64  `json:"expiry_nano"`
	}
	type fillPayload struct {
		Symbol string `json:"symbol"`
		Qty    int64  `json:"qty"`
	}

	contracts := make(map[expiryFillKey]expiryFillContract)
	missing := make(map[expiryFillKey]struct{})
	var mu sync.Mutex
	if err := r.Scan(ScanOptions{Events: []string{"instrument_listed", "instrument_settled"}}, func(event Event) {
		var payload instrumentPayload
		if event.Decode(&payload) != nil || payload.Symbol == "" {
			return
		}
		if payload.InstrumentType != "FUTURE" && payload.InstrumentType != "OPTION" {
			return
		}
		key := expiryFillKey{event.VenueID, payload.Symbol}
		mu.Lock()
		defer mu.Unlock()
		if payload.ExpiryNano <= 0 {
			missing[key] = struct{}{}
			return
		}
		contract := contracts[key]
		if contract.symbol != "" && (contract.kind != payload.InstrumentType || contract.expiry != payload.ExpiryNano) {
			contract.metadataMismatch = true
		}
		if contract.symbol == "" {
			contract.kind = payload.InstrumentType
			contract.symbol = payload.Symbol
			contract.expiry = payload.ExpiryNano
		}
		if event.Name == "instrument_listed" {
			contract.listed = true
			if contract.listing == 0 || event.SimTS < contract.listing {
				contract.listing = event.SimTS
			}
		} else {
			contract.settled = true
			contract.settlement = event.SimTS
		}
		contracts[key] = contract
	}); err != nil {
		return nil, err
	}

	type counts struct{ fills, after int }
	fillCounts := make(map[expiryFillKey]counts)
	if err := r.Scan(ScanOptions{Events: []string{"OrderFill"}}, func(event Event) {
		var payload fillPayload
		if event.Decode(&payload) != nil || payload.Qty <= 0 {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = payload.Symbol
		}
		key := expiryFillKey{event.VenueID, symbol}
		mu.Lock()
		defer mu.Unlock()
		contract, exists := contracts[key]
		if !exists {
			return
		}
		row := fillCounts[key]
		row.fills++
		if event.SimTS > contract.expiry {
			row.after++
		}
		fillCounts[key] = row
	}); err != nil {
		return nil, err
	}

	terminalNano := int64(0)
	for _, row := range r.Report.TerminalAccounts {
		if row.Account.Timestamp > terminalNano {
			terminalNano = row.Account.Timestamp
		}
	}
	result := &ExpiryFillAudit{MissingExpiryMetadata: len(missing)}
	for key, contract := range contracts {
		counts := fillCounts[key]
		result.Contracts++
		if contract.kind == "FUTURE" {
			result.Futures++
		} else {
			result.Options++
		}
		if contract.settled {
			result.SettledContracts++
		}
		if contract.settled && !contract.listed {
			result.SettlementWithoutListing++
		}
		if contract.metadataMismatch {
			result.MetadataMismatches++
		}
		expiredAtTerminal := terminalNano > 0 && contract.expiry <= terminalNano
		if expiredAtTerminal {
			result.ExpiredContracts++
			if !contract.settled {
				result.ExpiredUnsettledContracts++
			}
		}
		if !expiredAtTerminal && !contract.settled {
			continue
		}
		result.FillRecords += counts.fills
		result.FillsAfterExpiry += counts.after
		result.Checks = append(result.Checks, ExpiryFillCheck{
			VenueID: key.venue, Symbol: contract.symbol, InstrumentType: contract.kind,
			ExpiryNano: contract.expiry, ListedNano: contract.listing,
			SettlementNano: contract.settlement, Settled: contract.settled,
			FillRecords: counts.fills, FillsAfterExpiry: counts.after,
		})
	}
	sort.Slice(result.Checks, func(i, j int) bool {
		if result.Checks[i].VenueID != result.Checks[j].VenueID {
			return result.Checks[i].VenueID < result.Checks[j].VenueID
		}
		return result.Checks[i].Symbol < result.Checks[j].Symbol
	})
	return result, nil
}
