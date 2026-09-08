package exchange

import "testing"

func TestForcedFillNotificationMarksOnlySyntheticTaker(t *testing.T) {
	book := &OrderBook{Symbol: "ABC-PERP"}
	ctx := executionContext{
		book:   book,
		exec:   &Execution{Timestamp: 123},
		forced: true, liquidationID: 5,
	}
	takerGateway := NewClientGateway(1)
	makerGateway := NewClientGateway(2)
	taker := fillSide{clientID: 1, orderID: 11, role: "taker"}
	maker := fillSide{clientID: 2, orderID: 22, role: "maker"}

	sendFillNotification(takerGateway, ctx, 7, taker)
	sendFillNotification(makerGateway, ctx, 7, maker)

	takerResponse := <-takerGateway.ResponseCh
	takerFill, ok := takerResponse.Data.(*FillNotification)
	if !ok || !takerFill.Forced || takerFill.LiquidationID != 5 {
		t.Fatalf("synthetic taker notification = %#v, want forced fill", takerResponse.Data)
	}
	makerResponse := <-makerGateway.ResponseCh
	makerFill, ok := makerResponse.Data.(*FillNotification)
	if !ok || makerFill.Forced {
		t.Fatalf("resting maker notification = %#v, want ordinary fill", makerResponse.Data)
	}
}
