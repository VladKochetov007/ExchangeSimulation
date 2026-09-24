package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	etypes "exchange_sim/types"
)

type CrossVenueEvaluationFrontier struct {
	LinkID      uint32   `json:"LinkID"`
	Ordinal     uint64   `json:"Ordinal"`
	DeliveredAt int64    `json:"DeliveredAt"`
	Digest      [16]byte `json:"Digest"`
	Fingerprint [16]byte `json:"Fingerprint"`
}

type CrossVenueEvaluationFeed struct {
	VenueID  string                       `json:"venue_id"`
	ClientID uint64                       `json:"client_id"`
	Frontier CrossVenueEvaluationFrontier `json:"frontier"`
}

type CrossVenueEvaluationBook struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Bid      int64  `json:"bid"`
	BidQty   int64  `json:"bid_qty"`
	Ask      int64  `json:"ask"`
	AskQty   int64  `json:"ask_qty"`
	HasBid   bool   `json:"has_bid"`
	HasAsk   bool   `json:"has_ask"`
}

type CrossVenueEvaluationPayload struct {
	RouterID           uint64                     `json:"router_id"`
	TriggerActorID     uint64                     `json:"trigger_actor_id"`
	TriggerClientID    uint64                     `json:"trigger_client_id"`
	TriggerVenueID     string                     `json:"trigger_venue_id"`
	TriggerType        etypes.MDType              `json:"trigger_type"`
	TriggerSequence    uint64                     `json:"trigger_sequence"`
	TriggerPublishedAt int64                      `json:"trigger_published_at"`
	TriggerDigest      [16]byte                   `json:"trigger_digest"`
	Generation         uint64                     `json:"generation"`
	Reason             string                     `json:"reason"`
	InFlightGroupID    uint64                     `json:"in_flight_group_id"`
	AttemptsUsed       int                        `json:"attempts_used"`
	SelectedBuy        string                     `json:"selected_buy"`
	SelectedSell       string                     `json:"selected_sell"`
	QuotedEdge         int64                      `json:"quoted_edge"`
	Books              []CrossVenueEvaluationBook `json:"books"`
	Feeds              []CrossVenueEvaluationFeed `json:"feeds"`
}

type CrossVenueEvaluationRecord struct {
	Event   Event
	Payload CrossVenueEvaluationPayload
}

// CollectCrossVenueEvaluations verifies the optional callback stream against
// the terminal router counter supplied by the immutable run report. It does
// not yet prove the receipt/source or order/fill joins.
func (r *Run) CollectCrossVenueEvaluations(venues [2]string, routerID, expectedCount uint64) ([]CrossVenueEvaluationRecord, error) {
	if r == nil || routerID == 0 || venues[0] == "" || venues[1] == "" || venues[0] == venues[1] {
		return nil, fmt.Errorf("cross-venue evaluations: invalid selection")
	}
	files, err := selectCrossVenueGeneralFiles(r, venues)
	if err != nil {
		return nil, fmt.Errorf("cross-venue evaluations: %w", err)
	}
	var records []CrossVenueEvaluationRecord
	var callbackErr error
	if err := r.Scan(ScanOptions{Events: []string{"cross_venue_arb_evaluation"}, Files: files, FilesSelected: true, Workers: 1}, func(event Event) {
		if callbackErr != nil {
			return
		}
		var payload CrossVenueEvaluationPayload
		if err := validateCrossVenueEvaluationJSONShape(event.Raw()); err != nil {
			callbackErr = fmt.Errorf("cross-venue evaluations: malformed row shape: %w", err)
			return
		}
		if err := decodeRequiredJSON(event.Raw(), &payload, "router_id", "trigger_actor_id", "trigger_client_id", "trigger_venue_id", "trigger_type", "trigger_sequence", "trigger_published_at", "trigger_digest", "generation", "reason", "attempts_used", "books", "feeds"); err != nil {
			callbackErr = fmt.Errorf("cross-venue evaluations: malformed row: %w", err)
			return
		}
		if payload.RouterID == routerID {
			records = append(records, CrossVenueEvaluationRecord{Event: event, Payload: payload})
		}
	}); err != nil {
		return nil, err
	}
	if callbackErr != nil {
		return nil, callbackErr
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].Event.GlobalSequence < records[right].Event.GlobalSequence
	})
	if uint64(len(records)) != expectedCount {
		return nil, fmt.Errorf("cross-venue evaluations: %d retained rows disagree with terminal count %d", len(records), expectedCount)
	}
	lastTrigger := make(map[string]uint64, 2)
	for index, record := range records {
		if index > 0 && record.Event.GlobalSequence <= records[index-1].Event.GlobalSequence {
			return nil, fmt.Errorf("cross-venue evaluations: duplicate or unordered global frame identity")
		}
		if err := validateCrossVenueEvaluationRecord(record, venues, routerID, uint64(index+1), lastTrigger); err != nil {
			return nil, fmt.Errorf("cross-venue evaluations: row %d: %w", index, err)
		}
	}
	return records, nil
}

func validateCrossVenueEvaluationJSONShape(raw json.RawMessage) error {
	fields, err := requireCrossVenueFields(raw, "router_id", "trigger_actor_id", "trigger_client_id", "trigger_venue_id", "trigger_type", "trigger_sequence", "trigger_published_at", "trigger_digest", "generation", "reason", "attempts_used", "books", "feeds")
	if err != nil {
		return err
	}
	if err := requireCrossVenueDigest(fields["trigger_digest"]); err != nil {
		return fmt.Errorf("trigger digest: %w", err)
	}
	var books, feeds []json.RawMessage
	if err := json.Unmarshal(fields["books"], &books); err != nil || len(books) != 2 {
		return fmt.Errorf("books must contain exactly two objects")
	}
	if err := json.Unmarshal(fields["feeds"], &feeds); err != nil || len(feeds) != 2 {
		return fmt.Errorf("feeds must contain exactly two objects")
	}
	for index := 0; index < 2; index++ {
		if _, err := requireCrossVenueFields(books[index], "venue_id", "client_id", "bid", "bid_qty", "ask", "ask_qty", "has_bid", "has_ask"); err != nil {
			return fmt.Errorf("book %d: %w", index, err)
		}
		feed, err := requireCrossVenueFields(feeds[index], "venue_id", "client_id", "frontier")
		if err != nil {
			return fmt.Errorf("feed %d: %w", index, err)
		}
		frontier, err := requireCrossVenueFields(feed["frontier"], "LinkID", "Ordinal", "DeliveredAt", "Digest", "Fingerprint")
		if err != nil {
			return fmt.Errorf("feed %d frontier: %w", index, err)
		}
		if err := requireCrossVenueDigest(frontier["Digest"]); err != nil {
			return fmt.Errorf("feed %d frontier digest: %w", index, err)
		}
		if err := requireCrossVenueDigest(frontier["Fingerprint"]); err != nil {
			return fmt.Errorf("feed %d frontier fingerprint: %w", index, err)
		}
	}
	return nil
}

func requireCrossVenueFields(raw json.RawMessage, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("expected an object")
	}
	for _, name := range names {
		value, present := fields[name]
		if !present || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("missing or null field %q", name)
		}
	}
	return fields, nil
}

func requireCrossVenueDigest(raw json.RawMessage) error {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil || len(elements) != 16 {
		return fmt.Errorf("expected exactly 16 digest bytes")
	}
	return nil
}

func validateCrossVenueEvaluationRecord(record CrossVenueEvaluationRecord, venues [2]string, routerID, generation uint64, lastTrigger map[string]uint64) error {
	event, row := record.Event, record.Payload
	if event.GlobalSequence == 0 || row.RouterID != routerID || row.Generation != generation || event.ClientID != row.TriggerClientID || event.VenueID != row.TriggerVenueID || row.TriggerActorID == 0 {
		return fmt.Errorf("frame, router, generation or actor identity mismatch")
	}
	if row.TriggerType != etypes.MDSnapshot && row.TriggerType != etypes.MDDelta || row.TriggerSequence == 0 || row.TriggerDigest == ([16]byte{}) || row.TriggerPublishedAt > event.SimTS {
		return fmt.Errorf("invalid consumed-message identity")
	}
	if row.TriggerSequence <= lastTrigger[row.TriggerVenueID] {
		return fmt.Errorf("trigger source sequence is not increasing")
	}
	lastTrigger[row.TriggerVenueID] = row.TriggerSequence
	if row.AttemptsUsed < 0 || len(row.Books) != 2 || len(row.Feeds) != 2 {
		return fmt.Errorf("incomplete evaluation state")
	}
	allowed := map[string]bool{"IN_FLIGHT": true, "ATTEMPT_LIMIT": true, "DUPLICATE_GENERATION": true, "INCOMPLETE_FRONTIER": true, "NO_POSITIVE_POLICY_EDGE": true, "SUBMIT": true}
	if !allowed[row.Reason] {
		return fmt.Errorf("unknown evaluation reason %q", row.Reason)
	}
	if row.Reason == "SUBMIT" {
		if row.SelectedBuy == "" || row.SelectedSell == "" || row.SelectedBuy == row.SelectedSell || row.QuotedEdge <= 0 ||
			row.SelectedBuy != venues[0] && row.SelectedBuy != venues[1] || row.SelectedSell != venues[0] && row.SelectedSell != venues[1] {
			return fmt.Errorf("submission lacks a positive selected route")
		}
	} else if row.SelectedBuy != "" || row.SelectedSell != "" || row.QuotedEdge != 0 {
		return fmt.Errorf("no-action row contains a selected route")
	}
	if row.Reason == "IN_FLIGHT" && row.InFlightGroupID == 0 || row.Reason != "IN_FLIGHT" && row.InFlightGroupID != 0 {
		return fmt.Errorf("in-flight group identity disagrees with abstention reason")
	}
	triggerPresent := false
	for index := 0; index < 2; index++ {
		book, feed := row.Books[index], row.Feeds[index]
		if book.VenueID != feed.VenueID || book.ClientID != feed.ClientID || book.ClientID == 0 || book.HasBid && book.BidQty <= 0 || book.HasAsk && book.AskQty <= 0 || !book.HasBid && book.BidQty != 0 || !book.HasAsk && book.AskQty != 0 {
			return fmt.Errorf("book/feed identity or displayed touch is malformed")
		}
		if index > 0 && row.Books[index-1].VenueID >= book.VenueID {
			return fmt.Errorf("book/feed venues are not uniquely ordered")
		}
		if book.VenueID != venues[0] && book.VenueID != venues[1] {
			return fmt.Errorf("book/feed belongs to an unregistered venue")
		}
		frontier := feed.Frontier
		if frontier.Ordinal == 0 {
			if frontier.LinkID != 0 || frontier.DeliveredAt != 0 || frontier.Digest != ([16]byte{}) || frontier.Fingerprint != ([16]byte{}) {
				return fmt.Errorf("zero frontier has partial identity")
			}
		} else if frontier.LinkID == 0 || frontier.Digest == ([16]byte{}) || frontier.Fingerprint == ([16]byte{}) || frontier.DeliveredAt > event.SimTS {
			return fmt.Errorf("invalid delivered frontier")
		}
		if book.VenueID == row.TriggerVenueID && book.ClientID == row.TriggerClientID {
			triggerPresent = frontier.Ordinal > 0
		}
	}
	if !triggerPresent {
		return fmt.Errorf("trigger feed has no delivered frontier")
	}
	return nil
}

type crossVenueSourceReceiptKey struct {
	clientID uint64
	linkID   uint32
	sequence uint64
	digest   [16]byte
}

// VerifyCrossVenueEvaluationReceipts checks the compact courier sidecars and
// binds each callback to a particular message that had reached its actor
// inbox. A frontier alone may be ahead of the callback currently processing;
// the trigger identity is checked separately against a receipt within it.
func VerifyCrossVenueEvaluationReceipts(records []CrossVenueEvaluationRecord, evidenceDir, symbol string) error {
	index, audit, err := loadCDFReceiptIndex(evidenceDir)
	if err != nil {
		return fmt.Errorf("cross-venue evaluation receipts: %w", err)
	}
	if !audit.Valid {
		return fmt.Errorf("cross-venue evaluation receipts: compact receipt audit is invalid")
	}
	return verifyCrossVenueReceiptRows(records, index, symbol)
}

func verifyCrossVenueReceiptRows(records []CrossVenueEvaluationRecord, index *cdfReceiptIndex, symbol string) error {
	if index == nil || symbol == "" {
		return fmt.Errorf("cross-venue evaluation receipts: missing receipt index or symbol")
	}
	bySource := make(map[crossVenueSourceReceiptKey]cdfReceiptProof)
	for _, proof := range index.receipts {
		if proof.role != "cross_venue_router_tier" || proof.symbol != symbol {
			continue
		}
		key := crossVenueSourceReceiptKey{proof.record.clientID, proof.record.linkID, proof.record.sequence, proof.record.fingerprint}
		if _, duplicate := bySource[key]; duplicate {
			return fmt.Errorf("cross-venue evaluation receipts: duplicate delivered source identity")
		}
		bySource[key] = proof
	}
	for rowIndex, item := range records {
		row := item.Payload
		verifiedTrigger := false
		for _, feed := range row.Feeds {
			frontier := feed.Frontier
			if frontier.Ordinal == 0 {
				continue
			}
			proof, ok := index.receipts[cdfReceiptKey{feed.ClientID, frontier.LinkID, frontier.Ordinal}]
			if !ok || proof.sourceVenue != feed.VenueID || proof.role != "cross_venue_router_tier" || proof.symbol != symbol ||
				proof.digest != frontier.Digest || proof.record.fingerprint != frontier.Fingerprint || proof.record.deliveredAt != frontier.DeliveredAt || proof.record.deliveredAt > item.Event.SimTS {
				return fmt.Errorf("cross-venue evaluation receipts: row %d has a mismatched delivered frontier", rowIndex)
			}
			if feed.VenueID != row.TriggerVenueID || feed.ClientID != row.TriggerClientID {
				continue
			}
			trigger, ok := bySource[crossVenueSourceReceiptKey{feed.ClientID, frontier.LinkID, row.TriggerSequence, row.TriggerDigest}]
			if !ok || trigger.sourceVenue != row.TriggerVenueID || trigger.record.mdType != uint8(row.TriggerType) ||
				trigger.record.publishedAt != row.TriggerPublishedAt || trigger.record.ordinal > frontier.Ordinal || trigger.record.deliveredAt > item.Event.SimTS {
				return fmt.Errorf("cross-venue evaluation receipts: row %d consumed message is not in its delivered prefix", rowIndex)
			}
			verifiedTrigger = true
		}
		if !verifiedTrigger {
			return fmt.Errorf("cross-venue evaluation receipts: row %d has no verified consumed message", rowIndex)
		}
	}
	return nil
}
