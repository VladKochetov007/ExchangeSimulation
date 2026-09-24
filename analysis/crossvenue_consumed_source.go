package analysis

import (
	"fmt"

	etypes "exchange_sim/types"
)

type CrossVenueConsumedSource struct {
	Generation       uint64
	VenueID          string
	SourceSequence   uint64
	PublicationFrame uint64
	PublishedAt      int64
	EvaluatedAt      int64
	Reason           string
}

// MatchCrossVenueEvaluationSources reconstructs each consumed public message
// from the exchange's canonical book stream, then independently applies only
// those messages to the actor's local book. Publication is not consumption:
// intervening public updates cannot affect the local cache until a matching
// evaluation confirms that the actor processed them.
func MatchCrossVenueEvaluationSources(records []CrossVenueEvaluationRecord, publicEvents []Event, symbol string, lotQty, basePrecision, feeBps int64) ([]CrossVenueConsumedSource, error) {
	if symbol == "" || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 {
		return nil, fmt.Errorf("cross-venue consumed source: invalid convention")
	}
	byVenue := make(map[string][]Event, 2)
	var lastPublicFrame uint64
	for _, event := range publicEvents {
		if event.GlobalSequence == 0 || event.GlobalSequence <= lastPublicFrame {
			return nil, fmt.Errorf("cross-venue consumed source: public messages are not in unique global frame order")
		}
		lastPublicFrame = event.GlobalSequence
		byVenue[event.VenueID] = append(byVenue[event.VenueID], event)
	}
	cursors := make(map[string]int, 2)
	local := make(map[string]*crossVenueDepthState, 2)
	var matches []CrossVenueConsumedSource
	for rowIndex, item := range records {
		row := item.Payload
		if len(row.Books) != 2 || len(row.Feeds) != 2 || item.Event.GlobalSequence == 0 {
			return nil, fmt.Errorf("cross-venue consumed source: row %d has incomplete evaluation evidence", rowIndex)
		}
		venueEvents := byVenue[row.TriggerVenueID]
		var source Event
		var message *etypes.MarketDataMsg
		matchedCursor := -1
		for cursor := cursors[row.TriggerVenueID]; cursor < len(venueEvents) && venueEvents[cursor].GlobalSequence < item.Event.GlobalSequence; cursor++ {
			candidate := venueEvents[cursor]
			if candidate.SimTS != row.TriggerPublishedAt || !crossVenueSourceTypeMatches(candidate.Name, row.TriggerType) {
				continue
			}
			decoded, err := crossVenueSourceMessage(candidate, symbol, row.TriggerSequence)
			if err != nil {
				return nil, fmt.Errorf("cross-venue consumed source: row %d: %w", rowIndex, err)
			}
			if decoded == nil {
				continue
			}
			fingerprint, err := etypes.MarketDataFingerprint(decoded)
			if err != nil {
				return nil, fmt.Errorf("cross-venue consumed source: row %d fingerprint: %w", rowIndex, err)
			}
			if fingerprint != row.TriggerDigest {
				continue
			}
			if matchedCursor >= 0 {
				return nil, fmt.Errorf("cross-venue consumed source: row %d has ambiguous earlier publications", rowIndex)
			}
			source, message, matchedCursor = candidate, decoded, cursor
		}
		if matchedCursor < 0 {
			return nil, fmt.Errorf("cross-venue consumed source: row %d has no earlier matching publication", rowIndex)
		}
		cursors[row.TriggerVenueID] = matchedCursor + 1
		state := local[row.TriggerVenueID]
		if state == nil {
			state = &crossVenueDepthState{bids: make(map[int64]int64), asks: make(map[int64]int64)}
			local[row.TriggerVenueID] = state
		}
		if err := applyCrossVenueConsumedMessage(state, message); err != nil {
			return nil, fmt.Errorf("cross-venue consumed source: row %d: %w", rowIndex, err)
		}
		if err := verifyCrossVenueLocalBooks(row, local, lotQty, basePrecision, feeBps); err != nil {
			return nil, fmt.Errorf("cross-venue consumed source: row %d: %w", rowIndex, err)
		}
		matches = append(matches, CrossVenueConsumedSource{
			Generation: row.Generation, VenueID: row.TriggerVenueID, SourceSequence: row.TriggerSequence,
			PublicationFrame: source.GlobalSequence, PublishedAt: source.SimTS, EvaluatedAt: item.Event.SimTS, Reason: row.Reason,
		})
	}
	return matches, nil
}

func crossVenueSourceTypeMatches(name string, kind etypes.MDType) bool {
	return name == "BookSnapshot" && kind == etypes.MDSnapshot || name == "BookDelta" && kind == etypes.MDDelta
}

func crossVenueSourceMessage(event Event, symbol string, sequence uint64) (*etypes.MarketDataMsg, error) {
	if event.Name == "BookSnapshot" {
		var snapshot crossVenueSnapshotEvidence
		if err := decodeCrossVenueRequired(event.Raw(), &snapshot, "bids", "asks", "source_sequence", "public_bids", "public_asks"); err != nil {
			return nil, err
		}
		if snapshot.SourceSequence != sequence {
			return nil, nil
		}
		if _, err := validateCrossVenueProjection(snapshot.Bids, snapshot.PublicBids); err != nil {
			return nil, err
		}
		if _, err := validateCrossVenueProjection(snapshot.Asks, snapshot.PublicAsks); err != nil {
			return nil, err
		}
		return &etypes.MarketDataMsg{
			Type: etypes.MDSnapshot, Symbol: symbol, SeqNum: sequence, Timestamp: event.SimTS,
			Data: &etypes.BookSnapshot{Bids: snapshot.PublicBids, Asks: snapshot.PublicAsks},
		}, nil
	}
	if event.Name != "BookDelta" {
		return nil, fmt.Errorf("unsupported public message %s", event.Name)
	}
	var delta crossVenueDeltaEvidence
	if err := decodeRequiredJSON(event.Raw(), &delta, "side", "price", "visible_qty", "hidden_qty", "total_qty"); err != nil {
		return nil, err
	}
	total, ok := etypes.TryAdd(delta.VisibleQty, delta.HiddenQty)
	if !ok || delta.VisibleQty < 0 || delta.HiddenQty < 0 || total != delta.TotalQty {
		return nil, fmt.Errorf("invalid delta quantity")
	}
	side := etypes.Buy
	if delta.Side == "SELL" {
		side = etypes.Sell
	} else if delta.Side != "BUY" {
		return nil, fmt.Errorf("invalid delta side")
	}
	return &etypes.MarketDataMsg{
		Type: etypes.MDDelta, Symbol: symbol, SeqNum: sequence, Timestamp: event.SimTS,
		Data: &etypes.BookDelta{Side: side, Price: delta.Price, VisibleQty: delta.VisibleQty},
	}, nil
}

func applyCrossVenueConsumedMessage(state *crossVenueDepthState, message *etypes.MarketDataMsg) error {
	if message.Type == etypes.MDSnapshot {
		snapshot := message.Data.(*etypes.BookSnapshot)
		state.bids, state.asks = make(map[int64]int64, len(snapshot.Bids)), make(map[int64]int64, len(snapshot.Asks))
		for _, level := range snapshot.Bids {
			state.bids[level.Price] = level.VisibleQty
		}
		for _, level := range snapshot.Asks {
			state.asks[level.Price] = level.VisibleQty
		}
		state.known = true
		return nil
	}
	if !state.known {
		return fmt.Errorf("delta consumed before an anchored snapshot")
	}
	delta := message.Data.(*etypes.BookDelta)
	levels := state.bids
	if delta.Side == etypes.Sell {
		levels = state.asks
	}
	if delta.VisibleQty == 0 {
		delete(levels, delta.Price)
	} else {
		levels[delta.Price] = delta.VisibleQty
	}
	return nil
}

func verifyCrossVenueLocalBooks(row CrossVenueEvaluationPayload, local map[string]*crossVenueDepthState, lotQty, basePrecision, feeBps int64) error {
	var venues [2]string
	var books [2]CrossVenueTouch
	for index, observed := range row.Books {
		venues[index] = observed.VenueID
		books[index] = CrossVenueTouch{Bid: observed.Bid, BidQty: observed.BidQty, HasBid: observed.HasBid, Ask: observed.Ask, AskQty: observed.AskQty, HasAsk: observed.HasAsk}
		var reconstructed CrossVenueTouch
		if state := local[observed.VenueID]; state != nil && state.known {
			reconstructed = state.touch()
		}
		if books[index] != reconstructed {
			return fmt.Errorf("actor-local book does not follow consumed messages on %s", observed.VenueID)
		}
	}
	edge := EvaluateCrossVenueOneLotEdge(venues, books, lotQty, basePrecision, feeBps, true)
	switch row.Reason {
	case "SUBMIT":
		if edge.Status != "POSITIVE_EDGE" || edge.BuyVenue != row.SelectedBuy || edge.SellVenue != row.SelectedSell || edge.Edge != row.QuotedEdge {
			return fmt.Errorf("submitted route disagrees with independent local fee/depth calculation")
		}
	case "NO_POSITIVE_POLICY_EDGE":
		if edge.Status == "POSITIVE_EDGE" {
			return fmt.Errorf("actor abstained despite a positive local policy edge")
		}
	case "INCOMPLETE_FRONTIER":
		for _, feed := range row.Feeds {
			if feed.Frontier.Ordinal == 0 {
				return nil
			}
		}
		return fmt.Errorf("frontier was complete despite incomplete-frontier reason")
	case "DUPLICATE_GENERATION":
		return fmt.Errorf("duplicate generation is unreachable under this candidate")
	}
	return nil
}
