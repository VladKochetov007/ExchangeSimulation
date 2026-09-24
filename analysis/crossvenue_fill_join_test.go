package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func crossVenueFillFixture(t *testing.T, tradeIncluded bool, duplicateFill bool) (*Run, []CrossVenueResponseReceiptRecord) {
	t.Helper()
	root := t.TempDir()
	files := make([]string, 0, 2)
	for _, venue := range []string{"north", "south"} {
		path := filepath.Join(root, "venues", venue, "spot", "ABC-USD.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	makeLine := func(sequence uint64, timestamp int64, clientID uint64, name string, payload any) []byte {
		t.Helper()
		line, err := json.Marshal(map[string]any{
			"sim_ts": timestamp, "client_id": clientID, "event": name,
			"data": map[string]any{"venue_id": "north", "symbol": "ABC/USD", "global_sequence": sequence, "payload": payload},
		})
		if err != nil {
			t.Fatal(err)
		}
		return append(line, '\n')
	}
	var north []byte
	if tradeIncluded {
		north = append(north, makeLine(1, 10, 0, "Trade", map[string]any{
			"trade_id": 0, "price": 100, "qty": 5, "side": "BUY", "taker_order_id": 3, "maker_order_id": 4,
		})...)
	}
	fill := map[string]any{
		"order_id": 3, "trade_id": 0, "symbol": "ABC/USD", "side": "BUY", "qty": 5,
		"price": 100, "fee_amount": 1, "fee_asset": "USD", "role": "taker",
	}
	north = append(north, makeLine(2, 10, 7, "OrderFill", fill)...)
	if duplicateFill {
		north = append(north, makeLine(3, 10, 7, "OrderFill", fill)...)
	}
	if err := os.WriteFile(files[0], north, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[1], nil, 0o644); err != nil {
		t.Fatal(err)
	}
	receipt := validCrossVenueResponseReceipt(t)
	receipt.Event.GlobalSequence = 4
	receipt.Event.ClientID = 7
	receipt.Payload.ClientID = 7
	receipt.Payload.OrderID = 3
	receipt.Payload.TradeID = 0
	receipt.Payload.Qty = 5
	receipt.Payload.Price = 100
	receipt.Payload.ExchangeAt = 10
	receipt.Payload.FeeAmount = 1
	return &Run{files: files}, []CrossVenueResponseReceiptRecord{receipt}
}

func TestCrossVenueExchangeFillJoinAcceptsZeroTradeIDAndPendingReceipt(t *testing.T) {
	run, receipts := crossVenueFillFixture(t, true, false)
	clients := map[string]uint64{"north": 7, "south": 8}
	filled, err := run.CollectCrossVenueExchangeFills([2]string{"north", "south"}, clients, "ABC/USD", receipts)
	if err != nil || len(filled) != 1 || filled[0].TradeID != 0 || filled[0].Receipt == nil {
		t.Fatalf("matched first trade = %#v, %v", filled, err)
	}
	filled, err = run.CollectCrossVenueExchangeFills([2]string{"north", "south"}, clients, "ABC/USD", nil)
	if err != nil || len(filled) != 1 || filled[0].Receipt != nil {
		t.Fatalf("undelivered exchange fill = %#v, %v", filled, err)
	}
}

func TestCrossVenueExchangeFillJoinRejectsUnanchoredAndMutatedNotifications(t *testing.T) {
	clients := map[string]uint64{"north": 7, "south": 8}
	run, receipts := crossVenueFillFixture(t, false, false)
	if _, err := run.CollectCrossVenueExchangeFills([2]string{"north", "south"}, clients, "ABC/USD", receipts); err == nil {
		t.Fatal("fill without prior Trade accepted")
	}
	run, receipts = crossVenueFillFixture(t, true, true)
	if _, err := run.CollectCrossVenueExchangeFills([2]string{"north", "south"}, clients, "ABC/USD", receipts); err == nil {
		t.Fatal("duplicate exchange fill accepted")
	}
	run, receipts = crossVenueFillFixture(t, true, false)
	receipts[0].Payload.FeeAmount++
	if _, err := run.CollectCrossVenueExchangeFills([2]string{"north", "south"}, clients, "ABC/USD", receipts); err == nil {
		t.Fatal("mismatched actor fill fee accepted")
	}
	run, receipts = crossVenueFillFixture(t, true, false)
	receipts = append(receipts, receipts[0])
	if _, err := run.CollectCrossVenueExchangeFills([2]string{"north", "south"}, clients, "ABC/USD", receipts); err == nil {
		t.Fatal("duplicate actor fill notification accepted")
	}
}
