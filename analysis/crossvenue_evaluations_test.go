package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	etypes "exchange_sim/types"
)

func validCrossVenueEvaluationRecord(t *testing.T) CrossVenueEvaluationRecord {
	t.Helper()
	row := CrossVenueEvaluationPayload{
		RouterID: 1, TriggerActorID: 5, TriggerClientID: 11, TriggerVenueID: "north",
		TriggerType: etypes.MDSnapshot, TriggerSequence: 1, TriggerPublishedAt: 10,
		TriggerDigest: [16]byte{1}, Generation: 1, Reason: "INCOMPLETE_FRONTIER",
		Books: []CrossVenueEvaluationBook{
			{VenueID: "north", ClientID: 11, Bid: 99, BidQty: 1, HasBid: true, Ask: 101, AskQty: 1, HasAsk: true},
			{VenueID: "south", ClientID: 12},
		},
		Feeds: []CrossVenueEvaluationFeed{
			{VenueID: "north", ClientID: 11, Frontier: CrossVenueEvaluationFrontier{LinkID: 1, Ordinal: 1, DeliveredAt: 15, Digest: [16]byte{2}, Fingerprint: [16]byte{1}}},
			{VenueID: "south", ClientID: 12},
		},
	}
	event := crossVenueReplayEvent(t, "cross_venue_arb_evaluation", "north", 3, 20, row)
	event.ClientID = 11
	return CrossVenueEvaluationRecord{Event: event, Payload: row}
}

func TestCrossVenueEvaluationStructuralAuditRejectsMutations(t *testing.T) {
	venues := [2]string{"north", "south"}
	valid := validCrossVenueEvaluationRecord(t)
	if err := validateCrossVenueEvaluationRecord(valid, venues, 1, 1, make(map[string]uint64)); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*CrossVenueEvaluationRecord)
		want   string
	}{
		{"wrong-generation", func(row *CrossVenueEvaluationRecord) { row.Payload.Generation = 2 }, "identity mismatch"},
		{"wrong-trigger-client", func(row *CrossVenueEvaluationRecord) { row.Payload.TriggerClientID++ }, "identity mismatch"},
		{"missing-trigger-digest", func(row *CrossVenueEvaluationRecord) { row.Payload.TriggerDigest = [16]byte{} }, "consumed-message"},
		{"future-receipt", func(row *CrossVenueEvaluationRecord) { row.Payload.Feeds[0].Frontier.DeliveredAt = 21 }, "frontier"},
		{"missing-trigger-frontier", func(row *CrossVenueEvaluationRecord) { row.Payload.Feeds[0].Frontier = CrossVenueEvaluationFrontier{} }, "no delivered frontier"},
		{"duplicate-venue", func(row *CrossVenueEvaluationRecord) {
			row.Payload.Books[1].VenueID = "north"
			row.Payload.Feeds[1].VenueID = "north"
		}, "uniquely ordered"},
		{"bad-abstention", func(row *CrossVenueEvaluationRecord) { row.Payload.QuotedEdge = 1 }, "no-action"},
		{"unknown-reason", func(row *CrossVenueEvaluationRecord) { row.Payload.Reason = "SAVED_MARKET" }, "unknown evaluation reason"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := valid
			mutated.Payload.Books = append([]CrossVenueEvaluationBook(nil), valid.Payload.Books...)
			mutated.Payload.Feeds = append([]CrossVenueEvaluationFeed(nil), valid.Payload.Feeds...)
			test.mutate(&mutated)
			err := validateCrossVenueEvaluationRecord(mutated, venues, 1, 1, make(map[string]uint64))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCrossVenueEvaluationShapeRejectsTruncatedOrMissingNestedFields(t *testing.T) {
	valid := validCrossVenueEvaluationRecord(t)
	raw, err := json.Marshal(valid.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCrossVenueEvaluationJSONShape(raw); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"truncated-trigger-digest", func(row map[string]any) { row["trigger_digest"] = []any{1} }},
		{"missing-book-side-presence", func(row map[string]any) { delete(row["books"].([]any)[0].(map[string]any), "has_bid") }},
		{"missing-frontier-fingerprint", func(row map[string]any) {
			delete(row["feeds"].([]any)[0].(map[string]any)["frontier"].(map[string]any), "Fingerprint")
		}},
		{"null-feed", func(row map[string]any) { row["feeds"].([]any)[1] = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var row map[string]any
			if err := json.Unmarshal(raw, &row); err != nil {
				t.Fatal(err)
			}
			test.mutate(row)
			mutated, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateCrossVenueEvaluationJSONShape(mutated); err == nil {
				t.Fatal("malformed evaluation shape was accepted")
			}
		})
	}
}

func TestCollectCrossVenueEvaluationsRejectsDroppedFinalRow(t *testing.T) {
	root := t.TempDir()
	record := validCrossVenueEvaluationRecord(t)
	for _, venue := range []string{"north", "south"} {
		path := filepath.Join(root, "venues", venue, "general.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if venue == "north" {
			var err error
			raw, err = json.Marshal(map[string]any{
				"sim_ts": record.Event.SimTS, "client_id": record.Event.ClientID, "event": record.Event.Name,
				"data": map[string]any{"venue_id": record.Event.VenueID, "global_sequence": record.Event.GlobalSequence, "payload": record.Payload},
			})
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n')
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := &Run{files: []string{filepath.Join(root, "venues", "north", "general.jsonl"), filepath.Join(root, "venues", "south", "general.jsonl")}}
	rows, err := run.CollectCrossVenueEvaluations([2]string{"north", "south"}, 1, 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("retained evaluations = %#v, %v", rows, err)
	}
	if _, err := run.CollectCrossVenueEvaluations([2]string{"north", "south"}, 1, 2); err == nil || !strings.Contains(err.Error(), "terminal count") {
		t.Fatalf("missing final evaluation error = %v", err)
	}
}

func TestCrossVenueEvaluationReceiptJoinSeparatesConsumedMessageFromAheadFrontier(t *testing.T) {
	record := validCrossVenueEvaluationRecord(t)
	first := cdfReceiptProof{
		sourceVenue: "north", role: "cross_venue_router_tier", symbol: "ABC/USD", digest: [16]byte{2},
		record: observationRecord{clientID: 11, linkID: 1, mdType: uint8(etypes.MDSnapshot), sequence: 1,
			fingerprint: [16]byte{1}, publishedAt: 10, deliveredAt: 15, ordinal: 1},
	}
	second := cdfReceiptProof{
		sourceVenue: "north", role: "cross_venue_router_tier", symbol: "ABC/USD", digest: [16]byte{4},
		record: observationRecord{clientID: 11, linkID: 1, mdType: uint8(etypes.MDDelta), sequence: 2,
			fingerprint: [16]byte{5}, publishedAt: 12, deliveredAt: 18, ordinal: 2},
	}
	index := &cdfReceiptIndex{receipts: map[cdfReceiptKey]cdfReceiptProof{
		{clientID: 11, linkID: 1, ordinal: 1}: first,
		{clientID: 11, linkID: 1, ordinal: 2}: second,
	}}
	frontier := &record.Payload.Feeds[0].Frontier
	frontier.Ordinal, frontier.DeliveredAt, frontier.Digest, frontier.Fingerprint = 2, 18, [16]byte{4}, [16]byte{5}
	if err := verifyCrossVenueReceiptRows([]CrossVenueEvaluationRecord{record}, index, "ABC/USD"); err != nil {
		t.Fatalf("valid consumed trigger within ahead inbox frontier: %v", err)
	}
	badDigest := record
	badDigest.Payload.Feeds = append([]CrossVenueEvaluationFeed(nil), record.Payload.Feeds...)
	badDigest.Payload.Feeds[0].Frontier.Digest = [16]byte{9}
	if err := verifyCrossVenueReceiptRows([]CrossVenueEvaluationRecord{badDigest}, index, "ABC/USD"); err == nil || !strings.Contains(err.Error(), "mismatched delivered frontier") {
		t.Fatalf("wrong frontier digest error = %v", err)
	}
	badTrigger := record
	badTrigger.Payload.TriggerSequence = 3
	if err := verifyCrossVenueReceiptRows([]CrossVenueEvaluationRecord{badTrigger}, index, "ABC/USD"); err == nil || !strings.Contains(err.Error(), "not in its delivered prefix") {
		t.Fatalf("unreceived trigger error = %v", err)
	}
	lateTrigger := record
	lateTrigger.Event.SimTS = 14
	if err := verifyCrossVenueReceiptRows([]CrossVenueEvaluationRecord{lateTrigger}, index, "ABC/USD"); err == nil {
		t.Fatal("receipt after evaluation was accepted")
	}
}
