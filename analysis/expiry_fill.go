package analysis

import (
	"sort"
	"sync"
)

// ExpiryFillCheck records one expired future or option and every persisted fill
// record that arrived after the contractual expiry embedded in its settlement
// announcement.  The announcement's ExpiryNano is the contract term, not the
// time at which an implementation happened to delist the book.
type ExpiryFillCheck struct {
	VenueID          string `json:"venue_id"`
	Symbol           string `json:"symbol"`
	InstrumentType   string `json:"instrument_type"`
	ExpiryNano       int64  `json:"expiry_nano"`
	SettlementNano   int64  `json:"settlement_nano"`
	FillRecords      int    `json:"fill_records"`
	FillsAfterExpiry int    `json:"fills_after_expiry"`
}

// ExpiryFillAudit verifies the contractual lifetime boundary for every
// persisted expirable instrument.  It purposely does not infer expiry from
// the settlement event's simulation timestamp: a delayed expiry implementation
// would make that circular and conceal exactly the bug being tested.
//
// FillRecords count participant-side OrderFill records, so a matched trade
// usually contributes two records.  The invariant is the zero count of those
// records after the independently announced contractual expiry.
type ExpiryFillAudit struct {
	Contracts             int               `json:"contracts"`
	Futures               int               `json:"futures"`
	Options               int               `json:"options"`
	MissingExpiryMetadata int               `json:"missing_expiry_metadata"`
	FillRecords           int               `json:"fill_records"`
	FillsAfterExpiry      int               `json:"fills_after_expiry"`
	Checks                []ExpiryFillCheck `json:"checks"`
}

type expiryFillKey struct {
	venue, symbol string
}

type expiryFillContract struct {
	kind, symbol string
	expiry       int64
	settlement   int64
}

// MeasureExpiryFills independently joins settled contract metadata to every
// persisted OrderFill.  It covers both dated futures and European options;
// payout correctness remains the responsibility of the narrower settlement
// and exercise audits.
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
	if err := r.Scan(ScanOptions{Events: []string{"instrument_settled"}}, func(event Event) {
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
		contracts[key] = expiryFillContract{
			kind: payload.InstrumentType, symbol: payload.Symbol,
			expiry: payload.ExpiryNano, settlement: event.SimTS,
		}
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

	result := &ExpiryFillAudit{MissingExpiryMetadata: len(missing)}
	for key, contract := range contracts {
		counts := fillCounts[key]
		result.Contracts++
		if contract.kind == "FUTURE" {
			result.Futures++
		} else {
			result.Options++
		}
		result.FillRecords += counts.fills
		result.FillsAfterExpiry += counts.after
		result.Checks = append(result.Checks, ExpiryFillCheck{
			VenueID: key.venue, Symbol: contract.symbol, InstrumentType: contract.kind,
			ExpiryNano: contract.expiry, SettlementNano: contract.settlement,
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
