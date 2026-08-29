package analysis

import (
	"path/filepath"
	"sort"
	"strings"
)

// FillPositionAudit independently pairs every persisted linear (perpetual or
// dated-future) fill with the position update it claims to have caused. A
// balance-conservation check alone
// cannot catch a fill settled twice: the extra long and short still cancel,
// and the final account report can agree with the duplicated position stream.
//
// The comparison is deliberately between persisted outcomes. It does not read
// the matching engine's order state or the terminal account report. Options
// are intentionally excluded: the frozen raw evidence does not emit their
// per-fill position-update records, a separate documented measurement gap
// rather than a zero result.
type FillPositionAudit struct {
	LinearFills              int `json:"linear_fills"`
	TradePositionUpdates     int `json:"trade_position_updates"`
	Matched                  int `json:"matched"`
	MissingPositionUpdate    int `json:"missing_position_update"`
	UnexpectedPositionUpdate int `json:"unexpected_position_update"`
	PriceMismatches          int `json:"price_mismatches"`
	MalformedFillRecords     int `json:"malformed_fill_records"`
	MalformedPositionUpdates int `json:"malformed_position_updates"`
	// PositionChainChecks compares a trade update's old size with the prior
	// persisted new size for that account/contract in physical file order.
	PositionChainChecks   int                 `json:"position_chain_checks"`
	PositionChainFailures int                 `json:"position_chain_failures"`
	ByVenue               []FillPositionVenue `json:"by_venue"`
}

// FillPositionVenue is the audit result for one venue.
type FillPositionVenue struct {
	VenueID                  string `json:"venue_id"`
	LinearFills              int    `json:"linear_fills"`
	TradePositionUpdates     int    `json:"trade_position_updates"`
	Matched                  int    `json:"matched"`
	MissingPositionUpdate    int    `json:"missing_position_update"`
	UnexpectedPositionUpdate int    `json:"unexpected_position_update"`
	PriceMismatches          int    `json:"price_mismatches"`
	MalformedFillRecords     int    `json:"malformed_fill_records"`
	MalformedPositionUpdates int    `json:"malformed_position_updates"`
	PositionChainChecks      int    `json:"position_chain_checks"`
	PositionChainFailures    int    `json:"position_chain_failures"`
}

type fillPositionKey struct {
	venue, file, symbol, side string
	timestamp                 int64
	clientID                  uint64
	qty, newSize, price       int64
}

type fillPositionBaseKey struct {
	venue, file, symbol, side string
	timestamp                 int64
	clientID                  uint64
	qty, newSize              int64
}

type fillPositionChainKey struct {
	file, symbol string
	clientID     uint64
}

// MeasureFillPositions checks one-to-one linear fill/position evidence for
// every derivatives.jsonl stream. Same-timestamp records are keyed by all outcome
// fields and counted as multisets, because a participant can legitimately
// receive several fills at one scheduler instant.
func (r *Run) MeasureFillPositions() (*FillPositionAudit, error) {
	files := make([]string, 0, len(r.files))
	for _, file := range r.files {
		if filepath.Base(file) == "derivatives.jsonl" {
			files = append(files, file)
		}
	}
	result := &FillPositionAudit{}
	if len(files) == 0 {
		return result, nil
	}
	listedTypes := make(map[string]string)
	if err := r.Scan(ScanOptions{Events: []string{"instrument_listed"}, Workers: 1}, func(event Event) {
		var payload struct {
			Symbol string `json:"symbol"`
			Type   string `json:"instrument_type"`
		}
		if event.Decode(&payload) == nil && payload.Symbol != "" && payload.Type != "" {
			listedTypes[event.VenueID+"\x00"+payload.Symbol] = payload.Type
		}
	}); err != nil {
		return nil, err
	}

	type fillPayload struct {
		Symbol  string `json:"symbol"`
		Qty     int64  `json:"qty"`
		Side    string `json:"side"`
		NewSize int64  `json:"new_size"`
		Price   int64  `json:"price"`
	}
	type positionPayload struct {
		Symbol     string `json:"symbol"`
		OldSize    int64  `json:"old_size"`
		NewSize    int64  `json:"new_size"`
		TradeQty   int64  `json:"trade_qty"`
		TradeSide  string `json:"trade_side"`
		Reason     string `json:"reason"`
		TradePrice int64  `json:"trade_price"`
	}

	fills := make(map[fillPositionKey]int)
	updates := make(map[fillPositionKey]int)
	fillsByBase := make(map[fillPositionBaseKey]int)
	updatesByBase := make(map[fillPositionBaseKey]int)
	matchedByBase := make(map[fillPositionBaseKey]int)
	chains := make(map[fillPositionChainKey]int64)
	chainSeen := make(map[fillPositionChainKey]bool)
	venues := make(map[string]*FillPositionVenue)
	venue := func(id string) *FillPositionVenue {
		row := venues[id]
		if row == nil {
			row = &FillPositionVenue{VenueID: id}
			venues[id] = row
		}
		return row
	}

	err := r.Scan(ScanOptions{
		Files: files, FilesSelected: true, Workers: 1,
		Events: []string{"OrderFill", "position_update"},
	}, func(event Event) {
		switch event.Name {
		case "OrderFill":
			var payload fillPayload
			if err := decodeRequiredJSON(event.Raw(), &payload, "symbol", "qty", "side", "new_size", "price"); err != nil || !isLinearSymbol(event.VenueID, payload.Symbol, listedTypes) || payload.Qty <= 0 || payload.Side == "" {
				result.MalformedFillRecords++
				return
			}
			base := fillPositionBaseKey{event.VenueID, event.File, payload.Symbol, payload.Side, event.SimTS, event.ClientID, payload.Qty, payload.NewSize}
			key := fillPositionKey{venue: base.venue, file: base.file, symbol: base.symbol, side: base.side, timestamp: base.timestamp, clientID: base.clientID, qty: base.qty, newSize: base.newSize, price: payload.Price}
			fills[key]++
			fillsByBase[base]++
			result.LinearFills++
			venue(event.VenueID).LinearFills++
		case "position_update":
			var payload positionPayload
			if err := decodeRequiredJSON(event.Raw(), &payload, "symbol", "old_size", "new_size", "trade_qty", "trade_price", "trade_side", "reason"); err != nil || !isLinearSymbol(event.VenueID, payload.Symbol, listedTypes) || payload.Reason != "trade" || payload.TradeQty <= 0 || payload.TradeSide == "" {
				result.MalformedPositionUpdates++
				return
			}
			base := fillPositionBaseKey{event.VenueID, event.File, payload.Symbol, payload.TradeSide, event.SimTS, event.ClientID, payload.TradeQty, payload.NewSize}
			key := fillPositionKey{venue: base.venue, file: base.file, symbol: base.symbol, side: base.side, timestamp: base.timestamp, clientID: base.clientID, qty: base.qty, newSize: base.newSize, price: payload.TradePrice}
			updates[key]++
			updatesByBase[base]++
			result.TradePositionUpdates++
			row := venue(event.VenueID)
			row.TradePositionUpdates++
			chainKey := fillPositionChainKey{event.File, payload.Symbol, event.ClientID}
			if chainSeen[chainKey] {
				result.PositionChainChecks++
				row.PositionChainChecks++
				if chains[chainKey] != payload.OldSize {
					result.PositionChainFailures++
					row.PositionChainFailures++
				}
			}
			chains[chainKey] = payload.NewSize
			chainSeen[chainKey] = true
		}
	})
	if err != nil {
		return nil, err
	}

	keys := make(map[fillPositionKey]struct{}, len(fills)+len(updates))
	for key := range fills {
		keys[key] = struct{}{}
	}
	for key := range updates {
		keys[key] = struct{}{}
	}
	for key := range keys {
		fillCount, updateCount := fills[key], updates[key]
		matched := min(fillCount, updateCount)
		result.Matched += matched
		matchedByBase[fillPositionBaseKey{venue: key.venue, file: key.file, symbol: key.symbol, side: key.side, timestamp: key.timestamp, clientID: key.clientID, qty: key.qty, newSize: key.newSize}] += matched
		row := venue(key.venue)
		row.Matched += matched
		if fillCount > updateCount {
			delta := fillCount - updateCount
			result.MissingPositionUpdate += delta
			row.MissingPositionUpdate += delta
		}
		if updateCount > fillCount {
			delta := updateCount - fillCount
			result.UnexpectedPositionUpdate += delta
			row.UnexpectedPositionUpdate += delta
		}
	}
	baseKeys := make(map[fillPositionBaseKey]struct{}, len(fillsByBase)+len(updatesByBase))
	for key := range fillsByBase {
		baseKeys[key] = struct{}{}
	}
	for key := range updatesByBase {
		baseKeys[key] = struct{}{}
	}
	for key := range baseKeys {
		priceMismatch := min(fillsByBase[key], updatesByBase[key]) - matchedByBase[key]
		if priceMismatch <= 0 {
			continue
		}
		result.PriceMismatches += priceMismatch
		venue(key.venue).PriceMismatches += priceMismatch
	}
	for _, row := range venues {
		result.ByVenue = append(result.ByVenue, *row)
	}
	sort.Slice(result.ByVenue, func(i, j int) bool { return result.ByVenue[i].VenueID < result.ByVenue[j].VenueID })
	return result, nil
}

func isLinearSymbol(venue, symbol string, listedTypes map[string]string) bool {
	return listedTypes[venue+"\x00"+symbol] == "FUTURE" || strings.HasSuffix(symbol, "-PERP")
}
