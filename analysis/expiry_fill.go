package analysis

import (
	"encoding/json"
	"sort"
	"sync"
)

// ExpiryFillCheck records one expired future or option and every persisted fill
// record that arrived at or after the contractual expiry in its listing metadata.
// Settlement metadata is a consistency check, not the source of truth: a
// broken expiry path can omit it altogether.
type ExpiryFillCheck struct {
	VenueID                 string `json:"venue_id"`
	Symbol                  string `json:"symbol"`
	InstrumentType          string `json:"instrument_type"`
	ExpiryNano              int64  `json:"expiry_nano"`
	ListedNano              int64  `json:"listed_nano"`
	SettlementNano          int64  `json:"settlement_nano"`
	Settled                 bool   `json:"settled"`
	SettlementBeforeListing bool   `json:"settlement_before_listing"`
	FillRecords             int    `json:"fill_records"`
	FillsBeforeListing      int    `json:"fills_before_listing"`
	FillsAfterExpiry        int    `json:"fills_after_expiry"`
	// SnapshotRecordsAfterExpiry counts persisted book publications after the
	// contractual boundary. NonEmptySnapshotsAfterExpiry is the stronger
	// observable: a deleted instrument must not retain quoted depth.
	SnapshotRecordsAfterExpiry   int `json:"snapshot_records_after_expiry"`
	SnapshotRecordsBeforeListing int `json:"snapshot_records_before_listing"`
	NonEmptySnapshotsAfterExpiry int `json:"nonempty_snapshots_after_expiry"`
}

// ExpiryFillAudit verifies the contractual lifetime boundary for every
// persisted expirable instrument. It purposely does not infer expiry from the
// settlement event's simulation timestamp: a delayed or omitted expiry
// implementation would make that circular and conceal exactly the bug being
// tested.
//
// FillRecords count participant-side OrderFill records, so a matched trade
// usually contributes two records.  The invariant is the zero count of those
// records at or after the independently announced contractual expiry.
type ExpiryFillAudit struct {
	Contracts                    int               `json:"contracts"`
	Futures                      int               `json:"futures"`
	Options                      int               `json:"options"`
	ExpiredContracts             int               `json:"expired_contracts"`
	SettledContracts             int               `json:"settled_contracts"`
	ExpiredUnsettledContracts    int               `json:"expired_unsettled_contracts"`
	SettlementWithoutListing     int               `json:"settlement_without_listing"`
	MetadataMismatches           int               `json:"metadata_mismatches"`
	MissingExpiryMetadata        int               `json:"missing_expiry_metadata"`
	FillRecords                  int               `json:"fill_records"`
	FillsBeforeListing           int               `json:"fills_before_listing"`
	FillsAfterExpiry             int               `json:"fills_after_expiry"`
	MalformedFillRecords         int               `json:"malformed_fill_records"`
	FillIdentityFailures         int               `json:"fill_identity_failures"`
	MalformedLifecycleRecords    int               `json:"malformed_lifecycle_records"`
	MalformedSnapshotRecords     int               `json:"malformed_snapshot_records"`
	SettlementBeforeListing      int               `json:"settlement_before_listing"`
	SnapshotRecordsAfterExpiry   int               `json:"snapshot_records_after_expiry"`
	SnapshotRecordsBeforeListing int               `json:"snapshot_records_before_listing"`
	NonEmptySnapshotsAfterExpiry int               `json:"nonempty_snapshots_after_expiry"`
	Checks                       []ExpiryFillCheck `json:"checks"`
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
	listingOrder     evidenceOrder
	settlementOrder  evidenceOrder
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
	malformedLifecycleRecords := 0
	var mu sync.Mutex
	if err := r.Scan(ScanOptions{Events: []string{"instrument_listed", "instrument_settled"}}, func(event Event) {
		var payload instrumentPayload
		if event.Decode(&payload) != nil || payload.Symbol == "" || payload.InstrumentType == "" {
			mu.Lock()
			malformedLifecycleRecords++
			mu.Unlock()
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
			listingOrder := evidenceOrder{timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal}
			if contract.listingOrder.ordinal == 0 || evidenceBefore(listingOrder, contract.listingOrder) {
				contract.listingOrder = listingOrder
			}
		} else {
			contract.settled = true
			contract.settlement = event.SimTS
			contract.settlementOrder = evidenceOrder{timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal}
		}
		contracts[key] = contract
	}); err != nil {
		return nil, err
	}

	type counts struct {
		fills, beforeListing, after, snapshotsBeforeListing, snapshotsAfter, nonEmptySnapshotsAfter int
	}
	fillCounts := make(map[expiryFillKey]counts)
	malformedFillRecords := 0
	fillIdentityFailures := 0
	malformedSnapshotRecords := 0
	if err := r.Scan(ScanOptions{Events: []string{"OrderFill"}}, func(event Event) {
		var payload fillPayload
		if err := event.Decode(&payload); err != nil {
			mu.Lock()
			malformedFillRecords++
			mu.Unlock()
			return
		}
		if payload.Qty <= 0 {
			mu.Lock()
			malformedFillRecords++
			mu.Unlock()
			return
		}
		symbol := event.Symbol
		eventKey := expiryFillKey{event.VenueID, event.Symbol}
		payloadKey := expiryFillKey{event.VenueID, payload.Symbol}
		_, eventIsExpirable := contracts[eventKey]
		_, payloadIsExpirable := contracts[payloadKey]
		if (eventIsExpirable || payloadIsExpirable) && (event.Symbol == "" || payload.Symbol == "" || event.Symbol != payload.Symbol) {
			mu.Lock()
			fillIdentityFailures++
			mu.Unlock()
			return
		}
		if symbol == "" {
			symbol = payload.Symbol
		}
		if symbol == "" {
			mu.Lock()
			fillIdentityFailures++
			mu.Unlock()
			return
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
		if _, listed := latestCausalPrerequisite([]evidenceOrder{contract.listingOrder}, evidenceOrder{
			timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
		}); !listed {
			row.beforeListing++
		}
		if event.SimTS >= contract.expiry {
			row.after++
		}
		fillCounts[key] = row
	}); err != nil {
		return nil, err
	}

	// A fill-free expired book is still a lifecycle defect if it continues to
	// publish executable depth. This uses the listing contract already read
	// above, not a later settlement announcement, for the expiry boundary.
	type snapshotPayload struct {
		Bids []json.RawMessage `json:"bids"`
		Asks []json.RawMessage `json:"asks"`
	}
	if err := r.Scan(ScanOptions{Events: []string{"BookSnapshot"}}, func(event Event) {
		key := expiryFillKey{event.VenueID, event.Symbol}
		contract, exists := contracts[key]
		if !exists {
			return
		}
		if _, listed := latestCausalPrerequisite([]evidenceOrder{contract.listingOrder}, evidenceOrder{
			timestamp: event.SimTS, file: event.File, ordinal: event.Ordinal,
		}); !listed {
			mu.Lock()
			row := fillCounts[key]
			row.snapshotsBeforeListing++
			fillCounts[key] = row
			mu.Unlock()
			return
		}
		var payload snapshotPayload
		if err := decodeRequiredJSON(event.Raw(), &payload, "bids", "asks"); err != nil {
			mu.Lock()
			malformedSnapshotRecords++
			mu.Unlock()
			return
		}
		nonEmpty := false
		for _, rawLevel := range append(append([]json.RawMessage{}, payload.Bids...), payload.Asks...) {
			var level struct {
				VisibleQty int64 `json:"visible_qty"`
				HiddenQty  int64 `json:"hidden_qty"`
			}
			if err := decodeRequiredJSON(rawLevel, &level, "visible_qty"); err != nil || level.VisibleQty < 0 {
				mu.Lock()
				malformedSnapshotRecords++
				mu.Unlock()
				return
			}
			if level.HiddenQty < 0 {
				mu.Lock()
				malformedSnapshotRecords++
				mu.Unlock()
				return
			}
			quantity, ok := addAuditInt64(level.VisibleQty, level.HiddenQty)
			if !ok {
				mu.Lock()
				malformedSnapshotRecords++
				mu.Unlock()
				return
			}
			if quantity > 0 {
				nonEmpty = true
			}
		}
		// The expiry instant is the boundary snapshot: the deterministic runtime
		// may publish the final pre-settlement book at that timestamp before the
		// expiry phase delists it. Only a later timestamp is post-expiry evidence.
		if event.SimTS <= contract.expiry {
			return
		}
		mu.Lock()
		row := fillCounts[key]
		row.snapshotsAfter++
		if nonEmpty {
			row.nonEmptySnapshotsAfter++
		}
		fillCounts[key] = row
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	terminalNano := int64(0)
	for _, row := range r.Report.TerminalAccounts {
		if row.Account.Timestamp > terminalNano {
			terminalNano = row.Account.Timestamp
		}
	}
	result := &ExpiryFillAudit{
		MissingExpiryMetadata:     len(missing),
		MalformedFillRecords:      malformedFillRecords,
		FillIdentityFailures:      fillIdentityFailures,
		MalformedLifecycleRecords: malformedLifecycleRecords,
		MalformedSnapshotRecords:  malformedSnapshotRecords,
	}
	for key, contract := range contracts {
		counts := fillCounts[key]
		settlementBeforeListing := false
		if contract.settled {
			_, listedBeforeSettlement := latestCausalPrerequisite([]evidenceOrder{contract.listingOrder}, contract.settlementOrder)
			settlementBeforeListing = !listedBeforeSettlement
		}
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
		if settlementBeforeListing {
			result.SettlementBeforeListing++
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
		result.FillsBeforeListing += counts.beforeListing
		result.FillsAfterExpiry += counts.after
		result.SnapshotRecordsBeforeListing += counts.snapshotsBeforeListing
		result.SnapshotRecordsAfterExpiry += counts.snapshotsAfter
		result.NonEmptySnapshotsAfterExpiry += counts.nonEmptySnapshotsAfter
		result.Checks = append(result.Checks, ExpiryFillCheck{
			VenueID: key.venue, Symbol: contract.symbol, InstrumentType: contract.kind,
			ExpiryNano: contract.expiry, ListedNano: contract.listing,
			SettlementNano: contract.settlement, Settled: contract.settled,
			SettlementBeforeListing: settlementBeforeListing,
			FillRecords:             counts.fills, FillsBeforeListing: counts.beforeListing, FillsAfterExpiry: counts.after,
			SnapshotRecordsAfterExpiry:   counts.snapshotsAfter,
			SnapshotRecordsBeforeListing: counts.snapshotsBeforeListing,
			NonEmptySnapshotsAfterExpiry: counts.nonEmptySnapshotsAfter,
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
