package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func crossVenuePlacementFixture(t *testing.T, northRows ...map[string]any) *Run {
	t.Helper()
	root := t.TempDir()
	files := make([]string, 0, 2)
	for _, venue := range []string{"north", "south"} {
		path := filepath.Join(root, "venues", venue, "spot", "ABC-USD.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		var content []byte
		if venue == "north" {
			for _, row := range northRows {
				encoded, err := json.Marshal(row)
				if err != nil {
					t.Fatal(err)
				}
				content = append(content, encoded...)
				content = append(content, '\n')
			}
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	return &Run{files: files}
}

func crossVenuePlacementRow(sequence, requestID, orderID uint64, name, tif string) map[string]any {
	payload := map[string]any{
		"request_id": requestID, "side": "BUY", "type": "MARKET", "time_in_force": tif,
		"price": int64(0), "qty": int64(5),
	}
	if name == "OrderAccepted" {
		payload["order_id"] = orderID
	} else if name == "OrderRejected" {
		payload["success"] = false
		payload["error"] = "FOK_NOT_FILLED"
	}
	return map[string]any{
		"sim_ts": 10, "client_id": 7, "event": name,
		"data": map[string]any{"venue_id": "north", "symbol": "ABC/USD", "global_sequence": sequence, "payload": payload},
	}
}

func TestCrossVenuePlacementsCollectAcceptedAndRejectedFOK(t *testing.T) {
	run := crossVenuePlacementFixture(t,
		crossVenuePlacementRow(3, 11, 21, "OrderAccepted", "FOK"),
		crossVenuePlacementRow(5, 12, 0, "OrderRejected", "FOK"),
	)
	rows, err := run.CollectCrossVenuePlacements([2]string{"north", "south"}, map[string]uint64{"north": 7, "south": 8}, "ABC/USD", 5)
	if err != nil || len(rows) != 2 || rows[0].RequestID != 11 || rows[0].OrderID != 21 || rows[0].Kind != "ACCEPTED" || rows[1].RequestID != 12 || rows[1].Kind != "REJECTED" || rows[1].Reason != "FOK_NOT_FILLED" {
		t.Fatalf("placement outcomes = %#v, %v", rows, err)
	}
}

func TestCrossVenuePlacementsRejectCorruptedOutcome(t *testing.T) {
	wrongVenue := crossVenuePlacementRow(3, 11, 21, "OrderAccepted", "FOK")
	wrongVenue["data"].(map[string]any)["venue_id"] = "south"
	missingQuantity := crossVenuePlacementRow(3, 11, 21, "OrderAccepted", "FOK")
	delete(missingQuantity["data"].(map[string]any)["payload"].(map[string]any), "qty")
	for _, test := range []struct {
		name string
		rows []map[string]any
	}{
		{"duplicate-request", []map[string]any{crossVenuePlacementRow(3, 11, 21, "OrderAccepted", "FOK"), crossVenuePlacementRow(5, 11, 0, "OrderRejected", "FOK")}},
		{"duplicate-order", []map[string]any{crossVenuePlacementRow(3, 11, 21, "OrderAccepted", "FOK"), crossVenuePlacementRow(5, 12, 21, "OrderAccepted", "FOK")}},
		{"wrong-tif", []map[string]any{crossVenuePlacementRow(3, 11, 21, "OrderAccepted", "IOC")}},
		{"missing-request", []map[string]any{crossVenuePlacementRow(3, 0, 21, "OrderAccepted", "FOK")}},
		{"missing-order", []map[string]any{crossVenuePlacementRow(3, 11, 0, "OrderAccepted", "FOK")}},
		{"unordered", []map[string]any{crossVenuePlacementRow(5, 11, 21, "OrderAccepted", "FOK"), crossVenuePlacementRow(3, 12, 0, "OrderRejected", "FOK")}},
		{"wrong-file-venue", []map[string]any{wrongVenue}},
		{"missing-quantity", []map[string]any{missingQuantity}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := crossVenuePlacementFixture(t, test.rows...)
			if _, err := run.CollectCrossVenuePlacements([2]string{"north", "south"}, map[string]uint64{"north": 7, "south": 8}, "ABC/USD", 5); err == nil {
				t.Fatal("corrupted placement outcome accepted")
			}
		})
	}
}
