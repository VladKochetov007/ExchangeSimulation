package analysis

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	etypes "exchange_sim/types"
)

type CrossVenuePublicReplayOptions struct {
	Venues         [2]string
	Symbol         string
	InitiallyEmpty bool
}

type CrossVenueDisplayedBook struct {
	Bids []etypes.PriceLevel
	Asks []etypes.PriceLevel
}

type CrossVenuePublicReplay struct {
	Transitions []CrossVenuePublicTransition
	Terminal    map[string]CrossVenueDisplayedBook
}

type crossVenueDepthState struct {
	bids, asks map[int64]int64
	known      bool
}

type crossVenueSnapshotEvidence struct {
	Bids           []etypes.PriceLevel `json:"bids"`
	Asks           []etypes.PriceLevel `json:"asks"`
	SourceSequence uint64              `json:"source_sequence"`
	PublicBids     []etypes.PriceLevel `json:"public_bids"`
	PublicAsks     []etypes.PriceLevel `json:"public_asks"`
}

type crossVenueDeltaEvidence struct {
	Side       string `json:"side"`
	Price      int64  `json:"price"`
	VisibleQty int64  `json:"visible_qty"`
	HiddenQty  int64  `json:"hidden_qty"`
	TotalQty   int64  `json:"total_qty"`
}

// CollectCrossVenuePublicEvents selects the two spot logs and orders their
// records by the binary renderer's global frame identity, never file walk
// order. The caller must supply an already verified rendered evidence tree.
func (r *Run) CollectCrossVenuePublicEvents(venues [2]string, symbol string) ([]Event, error) {
	if r == nil || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || symbol == "" {
		return nil, fmt.Errorf("cross-venue public events: invalid selection")
	}
	spotLogName := strings.ReplaceAll(symbol, "/", "-")
	files := make([]string, 0, 2)
	for _, venue := range venues {
		selected := r.BookFiles(venue, spotLogName)
		if len(selected) != 1 {
			return nil, fmt.Errorf("cross-venue public events: venue %s has %d selected book files", venue, len(selected))
		}
		files = append(files, selected[0])
	}
	var events []Event
	if err := r.Scan(ScanOptions{Events: []string{"BookSnapshot", "BookDelta"}, Files: files, FilesSelected: true, Workers: 1}, func(event Event) {
		events = append(events, event)
	}); err != nil {
		return nil, err
	}
	sort.Slice(events, func(left, right int) bool { return events[left].GlobalSequence < events[right].GlobalSequence })
	for index, event := range events {
		if event.GlobalSequence == 0 || index > 0 && event.GlobalSequence == events[index-1].GlobalSequence {
			return nil, fmt.Errorf("cross-venue public events: missing or duplicate global frame identity")
		}
	}
	return events, nil
}

// ReplayCrossVenuePublicEvents consumes only BookSnapshot and BookDelta events
// in canonical global-frame order. InitiallyEmpty is a declared genesis
// assumption; without it, a delta before a snapshot is unpriceable evidence.
func ReplayCrossVenuePublicEvents(events []Event, options CrossVenuePublicReplayOptions) (*CrossVenuePublicReplay, error) {
	if options.Symbol == "" || options.Venues[0] == "" || options.Venues[1] == "" || options.Venues[0] == options.Venues[1] {
		return nil, fmt.Errorf("cross-venue public replay: invalid venue or symbol contract")
	}
	spotLogName := strings.ReplaceAll(options.Symbol, "/", "-")
	states := make(map[string]*crossVenueDepthState, 2)
	seenEvents := make(map[string]int, 2)
	for _, venue := range options.Venues {
		states[venue] = &crossVenueDepthState{bids: make(map[int64]int64), asks: make(map[int64]int64), known: options.InitiallyEmpty}
	}
	result := &CrossVenuePublicReplay{Terminal: make(map[string]CrossVenueDisplayedBook, 2)}
	var previousSequence uint64
	var previousTS int64
	for index, event := range events {
		if event.Name != "BookSnapshot" && event.Name != "BookDelta" {
			return nil, fmt.Errorf("cross-venue public replay: unsupported event %q", event.Name)
		}
		if event.GlobalSequence == 0 || index > 0 && event.GlobalSequence <= previousSequence || event.SimTS < 0 || index > 0 && event.SimTS < previousTS {
			return nil, fmt.Errorf("cross-venue public replay: event %d is out of global order", index)
		}
		previousSequence, previousTS = event.GlobalSequence, event.SimTS
		state := states[event.VenueID]
		if state == nil || event.Symbol != options.Symbol && symbolFromPath(event.File) != spotLogName {
			return nil, fmt.Errorf("cross-venue public replay: event %d is outside registered venue/symbol", index)
		}
		seenEvents[event.VenueID]++
		if event.Name == "BookSnapshot" {
			var snapshot crossVenueSnapshotEvidence
			if err := decodeCrossVenueRequired(event.Raw(), &snapshot, "bids", "asks", "source_sequence", "public_bids", "public_asks"); err != nil {
				return nil, fmt.Errorf("cross-venue public replay: snapshot %d: %w", index, err)
			}
			bids, err := validateCrossVenueProjection(snapshot.Bids, snapshot.PublicBids)
			if err != nil {
				return nil, fmt.Errorf("cross-venue public replay: snapshot %d bids: %w", index, err)
			}
			asks, err := validateCrossVenueProjection(snapshot.Asks, snapshot.PublicAsks)
			if err != nil {
				return nil, fmt.Errorf("cross-venue public replay: snapshot %d asks: %w", index, err)
			}
			state.bids, state.asks, state.known = bids, asks, true
		} else {
			if !state.known {
				return nil, fmt.Errorf("cross-venue public replay: delta %d precedes known venue book", index)
			}
			var delta crossVenueDeltaEvidence
			if err := decodeRequiredJSON(event.Raw(), &delta, "side", "price", "visible_qty", "hidden_qty", "total_qty"); err != nil {
				return nil, fmt.Errorf("cross-venue public replay: delta %d: %w", index, err)
			}
			total, ok := etypes.TryAdd(delta.VisibleQty, delta.HiddenQty)
			if !ok || delta.VisibleQty < 0 || delta.HiddenQty < 0 || total != delta.TotalQty || delta.Side != "BUY" && delta.Side != "SELL" {
				return nil, fmt.Errorf("cross-venue public replay: delta %d has invalid side or quantity", index)
			}
			levels := state.bids
			if delta.Side == "SELL" {
				levels = state.asks
			}
			if delta.VisibleQty == 0 {
				delete(levels, delta.Price)
			} else {
				levels[delta.Price] = delta.VisibleQty
			}
		}
		result.Transitions = append(result.Transitions, CrossVenuePublicTransition{
			SimTS: event.SimTS, GlobalSequence: event.GlobalSequence, VenueID: event.VenueID, Touch: state.touch(),
		})
	}
	for _, venue := range options.Venues {
		state := states[venue]
		if !state.known || seenEvents[venue] == 0 {
			return nil, fmt.Errorf("cross-venue public replay: venue %s has no observed book state", venue)
		}
		result.Terminal[venue] = CrossVenueDisplayedBook{Bids: crossVenueLevels(state.bids, true), Asks: crossVenueLevels(state.asks, false)}
	}
	return result, nil
}

func decodeCrossVenueRequired(raw json.RawMessage, target any, required ...string) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return fmt.Errorf("payload is not an object")
	}
	for _, name := range required {
		value, present := fields[name]
		if !present || name == "source_sequence" && string(value) == "null" {
			return fmt.Errorf("missing required field %q", name)
		}
	}
	return nil
}

func validateCrossVenueProjection(full, public []etypes.PriceLevel) (map[int64]int64, error) {
	visible := make(map[int64]int64, len(full))
	seen := make(map[int64]struct{}, len(full))
	for _, level := range full {
		if level.VisibleQty < 0 || level.HiddenQty < 0 {
			return nil, fmt.Errorf("negative full depth")
		}
		if _, duplicate := seen[level.Price]; duplicate {
			return nil, fmt.Errorf("duplicate full price")
		}
		seen[level.Price] = struct{}{}
		total, ok := etypes.TryAdd(level.VisibleQty, level.HiddenQty)
		if !ok || total <= 0 {
			return nil, fmt.Errorf("invalid full level total")
		}
		if level.VisibleQty > 0 {
			visible[level.Price] = level.VisibleQty
		}
	}
	projected := make(map[int64]int64, len(public))
	for _, level := range public {
		if level.VisibleQty <= 0 || level.HiddenQty != 0 {
			return nil, fmt.Errorf("invalid displayed level")
		}
		if _, duplicate := projected[level.Price]; duplicate {
			return nil, fmt.Errorf("duplicate displayed price")
		}
		projected[level.Price] = level.VisibleQty
	}
	if len(visible) != len(projected) {
		return nil, fmt.Errorf("displayed depth disagrees with full projection")
	}
	for price, quantity := range visible {
		if projected[price] != quantity {
			return nil, fmt.Errorf("displayed depth disagrees with full projection")
		}
	}
	return projected, nil
}

func (state *crossVenueDepthState) touch() CrossVenueTouch {
	var touch CrossVenueTouch
	for price, quantity := range state.bids {
		if quantity > 0 && (!touch.HasBid || price > touch.Bid) {
			touch.Bid, touch.BidQty, touch.HasBid = price, quantity, true
		}
	}
	for price, quantity := range state.asks {
		if quantity > 0 && (!touch.HasAsk || price < touch.Ask) {
			touch.Ask, touch.AskQty, touch.HasAsk = price, quantity, true
		}
	}
	return touch
}

func crossVenueLevels(depth map[int64]int64, bids bool) []etypes.PriceLevel {
	levels := make([]etypes.PriceLevel, 0, len(depth))
	for price, quantity := range depth {
		levels = append(levels, etypes.PriceLevel{Price: price, VisibleQty: quantity})
	}
	slices.SortFunc(levels, func(left, right etypes.PriceLevel) int {
		if bids {
			return compareInt64(right.Price, left.Price)
		}
		return compareInt64(left.Price, right.Price)
	})
	return levels
}
