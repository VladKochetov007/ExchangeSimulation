package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validCrossVenueResponseReceipt(t *testing.T) CrossVenueResponseReceiptRecord {
	t.Helper()
	payload := CrossVenueResponseReceiptPayload{
		RouterID: 1, ActorID: 5, ClientID: 11, VenueID: "north", ReceivedAt: 20,
		Kind: "FILL", Success: true, OrderID: 3, TradeID: 9, Symbol: "ABC/USD",
		Side: "BUY", Qty: 100, Price: 101, FeeAmount: 1, FeeAsset: "USD", ExchangeAt: 15,
	}
	event := crossVenueReplayEvent(t, "cross_venue_arb_response_receipt", "north", 7, 20, payload)
	event.ClientID = 11
	return CrossVenueResponseReceiptRecord{Event: event, Payload: payload}
}

func TestCrossVenueResponseReceiptRejectsIdentityAndTimingMutations(t *testing.T) {
	valid := validCrossVenueResponseReceipt(t)
	venues := [2]string{"north", "south"}
	if err := validateCrossVenueResponseReceipt(valid, venues, 1); err != nil {
		t.Fatal(err)
	}
	firstTrade := valid
	firstTrade.Payload.TradeID = 0 // The exchange starts each book's trade sequence at zero.
	if err := validateCrossVenueResponseReceipt(firstTrade, venues, 1); err != nil {
		t.Fatalf("first trade rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*CrossVenueResponseReceiptRecord)
	}{
		{"wrong-client", func(row *CrossVenueResponseReceiptRecord) { row.Payload.ClientID++ }},
		{"wrong-venue", func(row *CrossVenueResponseReceiptRecord) { row.Payload.VenueID = "south" }},
		{"receipt-before-match", func(row *CrossVenueResponseReceiptRecord) { row.Payload.ExchangeAt = 21 }},
		{"invalid-side", func(row *CrossVenueResponseReceiptRecord) { row.Payload.Side = "OTHER" }},
		{"invalid-quantity", func(row *CrossVenueResponseReceiptRecord) { row.Payload.Qty = 0 }},
		{"unknown-kind", func(row *CrossVenueResponseReceiptRecord) { row.Payload.Kind = "MIRACLE_FILL" }},
		{"status-conflict", func(row *CrossVenueResponseReceiptRecord) { row.Payload.Success = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			row := valid
			test.mutate(&row)
			if err := validateCrossVenueResponseReceipt(row, venues, 1); err == nil {
				t.Fatal("corrupted response receipt accepted")
			}
		})
	}
}

func TestCollectCrossVenueResponseReceiptsRejectsDroppedAndMalformedRows(t *testing.T) {
	root := t.TempDir()
	valid := validCrossVenueResponseReceipt(t)
	paths := make([]string, 0, 2)
	for _, venue := range []string{"north", "south"} {
		path := filepath.Join(root, "venues", venue, "general.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	writeRow := func(payload any) {
		t.Helper()
		line, err := json.Marshal(map[string]any{
			"sim_ts": valid.Event.SimTS, "client_id": valid.Event.ClientID, "event": valid.Event.Name,
			"data": map[string]any{"venue_id": valid.Event.VenueID, "global_sequence": valid.Event.GlobalSequence, "payload": payload},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths[0], append(line, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths[1], nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := &Run{files: paths}
	writeRow(valid.Payload)
	if rows, err := run.CollectCrossVenueResponseReceipts([2]string{"north", "south"}, 1, 1); err != nil || len(rows) != 1 {
		t.Fatalf("valid receipt = %d, %v", len(rows), err)
	}
	if _, err := run.CollectCrossVenueResponseReceipts([2]string{"north", "south"}, 1, 2); err == nil || !strings.Contains(err.Error(), "terminal count") {
		t.Fatalf("dropped final receipt error = %v", err)
	}
	var malformed map[string]any
	raw, err := json.Marshal(valid.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &malformed); err != nil {
		t.Fatal(err)
	}
	delete(malformed, "exchange_at")
	writeRow(malformed)
	if _, err := run.CollectCrossVenueResponseReceipts([2]string{"north", "south"}, 1, 1); err == nil || !strings.Contains(err.Error(), "exchange_at") {
		t.Fatalf("missing match timestamp error = %v", err)
	}
}
