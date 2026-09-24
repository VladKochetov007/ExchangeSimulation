package analysis

import "testing"

func crossVenuePlacementJoinFixture(t *testing.T) ([]CrossVenuePlacement, []CrossVenueResponseReceiptRecord, []CrossVenueExchangeFill) {
	t.Helper()
	accepted := CrossVenuePlacement{
		Event: Event{VenueID: "north", ClientID: 7, SimTS: 10, GlobalSequence: 2},
		Kind:  "ACCEPTED", RequestID: 11, OrderID: 21, Side: "BUY", Qty: 5,
	}
	rejected := CrossVenuePlacement{
		Event: Event{VenueID: "south", ClientID: 8, SimTS: 11, GlobalSequence: 5},
		Kind:  "REJECTED", RequestID: 12, Side: "SELL", Qty: 5, Reason: "FOK_NOT_FILLED",
	}
	ack := validCrossVenueResponseReceipt(t)
	ack.Event.VenueID, ack.Event.ClientID, ack.Event.SimTS, ack.Event.GlobalSequence = "north", 7, 13, 7
	ack.Payload.VenueID, ack.Payload.ClientID, ack.Payload.ActorID, ack.Payload.ReceivedAt = "north", 7, 1, 13
	ack.Payload.Kind, ack.Payload.RequestID, ack.Payload.OrderID = "ORDER_ACCEPTED", 11, 21
	ack.Payload.Qty, ack.Payload.Symbol = 0, ""
	ack.Payload.ExchangeAt = 0
	rejection := validCrossVenueResponseReceipt(t)
	rejection.Event.VenueID, rejection.Event.ClientID, rejection.Event.SimTS, rejection.Event.GlobalSequence = "south", 8, 14, 8
	rejection.Payload.VenueID, rejection.Payload.ClientID, rejection.Payload.ActorID, rejection.Payload.ReceivedAt = "south", 8, 2, 14
	rejection.Payload.Kind, rejection.Payload.RequestID, rejection.Payload.OrderID = "REJECTED", 12, 0
	rejection.Payload.Success, rejection.Payload.Error = false, "FOK_NOT_FILLED"
	rejection.Payload.Qty, rejection.Payload.Symbol = 0, ""
	rejection.Payload.ExchangeAt = 0
	fill := CrossVenueExchangeFill{
		Event:   Event{VenueID: "north", ClientID: 7, SimTS: 10, GlobalSequence: 4},
		OrderID: 21, TradeID: 0, Symbol: "ABC/USD", Side: "BUY", Qty: 5, Price: 100,
		FeeAmount: 1, FeeAsset: "USD", Role: "taker",
	}
	return []CrossVenuePlacement{accepted, rejected}, []CrossVenueResponseReceiptRecord{ack, rejection}, []CrossVenueExchangeFill{fill}
}

func TestCrossVenuePlacementJoinKeepsExchangeAndInboxTimelinesSeparate(t *testing.T) {
	placements, receipts, fills := crossVenuePlacementJoinFixture(t)
	rows, err := ReconcileCrossVenuePlacementReceipts(placements, receipts, fills, 20, 5)
	if err != nil || len(rows) != 2 || rows[0].FilledQty != 5 || rows[0].ExchangeAt != 10 || rows[0].InboxAt == nil || *rows[0].InboxAt != 13 || rows[1].Reason != "FOK_NOT_FILLED" {
		t.Fatalf("placement join = %#v, %v", rows, err)
	}
	rows, err = ReconcileCrossVenuePlacementReceipts(placements, receipts[1:], fills, 20, 5)
	if err != nil || rows[0].InboxAt != nil || rows[0].FilledQty != 5 {
		t.Fatalf("late acknowledgement misclassified: %#v, %v", rows, err)
	}
	fills[0].Qty = 2
	secondFill := fills[0]
	secondFill.TradeID, secondFill.Qty, secondFill.Event.GlobalSequence = 1, 3, 6
	rows, err = ReconcileCrossVenuePlacementReceipts(placements, receipts, append(fills, secondFill), 20, 5)
	if err != nil || rows[0].FilledQty != 5 {
		t.Fatalf("split full FOK fill = %#v, %v", rows, err)
	}
}

func TestCrossVenuePlacementJoinRejectsCorruptFOKAndReceiptLinks(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*[]CrossVenuePlacement, *[]CrossVenueResponseReceiptRecord, *[]CrossVenueExchangeFill)
	}{
		{"unanchored-ack", func(_ *[]CrossVenuePlacement, receipts *[]CrossVenueResponseReceiptRecord, _ *[]CrossVenueExchangeFill) {
			(*receipts)[0].Payload.RequestID = 99
		}},
		{"wrong-order", func(_ *[]CrossVenuePlacement, receipts *[]CrossVenueResponseReceiptRecord, _ *[]CrossVenueExchangeFill) {
			(*receipts)[0].Payload.OrderID = 22
		}},
		{"early-ack", func(_ *[]CrossVenuePlacement, receipts *[]CrossVenueResponseReceiptRecord, _ *[]CrossVenueExchangeFill) {
			(*receipts)[0].Event.GlobalSequence = 1
		}},
		{"wrong-reason", func(_ *[]CrossVenuePlacement, receipts *[]CrossVenueResponseReceiptRecord, _ *[]CrossVenueExchangeFill) {
			(*receipts)[1].Payload.Error = "OTHER"
		}},
		{"partial-fok", func(_ *[]CrossVenuePlacement, _ *[]CrossVenueResponseReceiptRecord, fills *[]CrossVenueExchangeFill) {
			(*fills)[0].Qty = 4
		}},
		{"unknown-fill", func(_ *[]CrossVenuePlacement, _ *[]CrossVenueResponseReceiptRecord, fills *[]CrossVenueExchangeFill) {
			(*fills)[0].OrderID = 22
		}},
		{"duplicate-receipt", func(_ *[]CrossVenuePlacement, receipts *[]CrossVenueResponseReceiptRecord, _ *[]CrossVenueExchangeFill) {
			*receipts = append(*receipts, (*receipts)[0])
		}},
		{"receipt-after-horizon", func(_ *[]CrossVenuePlacement, receipts *[]CrossVenueResponseReceiptRecord, _ *[]CrossVenueExchangeFill) {
			(*receipts)[0].Payload.ReceivedAt, (*receipts)[0].Event.SimTS = 21, 21
		}},
		{"fill-after-horizon", func(_ *[]CrossVenuePlacement, _ *[]CrossVenueResponseReceiptRecord, fills *[]CrossVenueExchangeFill) {
			(*fills)[0].Event.SimTS = 21
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			placements, receipts, fills := crossVenuePlacementJoinFixture(t)
			test.mutate(&placements, &receipts, &fills)
			if _, err := ReconcileCrossVenuePlacementReceipts(placements, receipts, fills, 20, 5); err == nil {
				t.Fatal("corrupt FOK or receipt join accepted")
			}
		})
	}
}
