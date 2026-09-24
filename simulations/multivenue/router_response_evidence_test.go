package multivenue

import (
	"testing"

	"exchange_sim/exchange"
)

func TestCrossVenueResponseReceiptSeparatesMatchAndInboxTime(t *testing.T) {
	fill := &exchange.FillNotification{
		OrderID: 21, ClientID: 7, TradeID: 35, Symbol: "ABC/USD",
		Side: exchange.Buy, Qty: 250, Price: 10100, FeeAmount: 4,
		FeeAsset: "USD", Timestamp: 100,
	}
	receipt := newCrossVenueArbResponseReceipt(9, 4, 7, "north", exchange.Response{Success: true, Data: fill}, 130)
	if receipt.Kind != "FILL" || receipt.ExchangeAt != 100 || receipt.ReceivedAt != 130 || receipt.TradeID != 35 || receipt.FeeAmount != 4 {
		t.Fatalf("fill receipt lost exchange or actor identity: %#v", receipt)
	}
	accepted := newCrossVenueArbResponseReceipt(9, 4, 7, "north", exchange.Response{RequestID: 17, Success: true, Data: uint64(21)}, 140)
	if accepted.Kind != "ORDER_ACCEPTED" || accepted.RequestID != 17 || accepted.OrderID != 21 {
		t.Fatalf("acceptance receipt lost request-to-order link: %#v", accepted)
	}
	if receipt.ExchangeAt >= receipt.ReceivedAt || receipt.ReceivedAt >= accepted.ReceivedAt {
		t.Fatal("test did not exercise a delayed fill received before its acceptance")
	}
}

func TestCrossVenueResponseReceiptClassifiesRejectionAndForcedCancellation(t *testing.T) {
	rejected := newCrossVenueArbResponseReceipt(9, 4, 7, "north", exchange.Response{RequestID: 17, Success: false, Error: exchange.RejectFOKNotFilled}, 130)
	if rejected.Kind != "REJECTED" || rejected.RequestID != 17 || rejected.Error != exchange.RejectFOKNotFilled {
		t.Fatalf("rejection receipt = %#v", rejected)
	}
	cancelled := newCrossVenueArbResponseReceipt(9, 4, 7, "north", exchange.Response{Success: true, Data: &exchange.ForcedCancelNotification{OrderID: 21, RemainingQty: 250}}, 140)
	if cancelled.Kind != "FORCED_CANCELLED" || cancelled.OrderID != 21 || cancelled.RemainingQty != 250 {
		t.Fatalf("forced cancel receipt = %#v", cancelled)
	}
}
