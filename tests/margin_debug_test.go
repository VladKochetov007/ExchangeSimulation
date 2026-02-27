package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func TestMarginReleaseDebug(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	perp.MarginRate = 1000 // 10%
	ex.AddInstrument(perp)

	gw1 := ex.ConnectClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	gw2 := ex.ConnectClient(2, nil, &FixedFee{})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Open: buy 1 BTC @ $50,000
	gw2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder,
		Price: 50000 * USD_PRECISION, Qty: BTC_PRECISION,
	}}
	<-gw2.ResponseCh

	t.Logf("After client2 sell limit placed")
	t.Logf("Client 1 PerpReserved: %d", ex.Clients[1].PerpReserved["USD"])
	t.Logf("Client 2 PerpReserved: %d", ex.Clients[2].PerpReserved["USD"])

	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 2, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: 50000 * USD_PRECISION, Qty: BTC_PRECISION,
	}}
	<-gw1.ResponseCh
	<-gw1.ResponseCh // fill
	<-gw2.ResponseCh // fill

	t.Logf("After open trade (client1 long 1 BTC @ $50k)")
	t.Logf("Client 1 PerpReserved: %d (%.2f USD)", ex.Clients[1].PerpReserved["USD"], float64(ex.Clients[1].PerpReserved["USD"])/float64(USD_PRECISION))
	t.Logf("Client 1 Position: %+v", ex.Positions.GetAllPositions(1))

	// Close: sell 1 BTC @ $51,000
	gw1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 3, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder,
		Price: 51000 * USD_PRECISION, Qty: BTC_PRECISION,
	}}
	<-gw1.ResponseCh

	t.Logf("After client1 sell limit placed (should close position)")
	t.Logf("Client 1 PerpReserved: %d (%.2f USD)", ex.Clients[1].PerpReserved["USD"], float64(ex.Clients[1].PerpReserved["USD"])/float64(USD_PRECISION))

	gw2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 4, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: 51000 * USD_PRECISION, Qty: BTC_PRECISION,
	}}
	<-gw2.ResponseCh
	<-gw1.ResponseCh // fill
	<-gw2.ResponseCh // fill

	t.Logf("After close trade")
	t.Logf("Client 1 PerpReserved: %d (%.2f USD) - SHOULD BE 0", ex.Clients[1].PerpReserved["USD"], float64(ex.Clients[1].PerpReserved["USD"])/float64(USD_PRECISION))
	t.Logf("Client 1 Position: %+v", ex.Positions.GetAllPositions(1))
	
	if ex.Clients[1].PerpReserved["USD"] != 0 {
		t.Errorf("Expected PerpReserved = 0, got %d", ex.Clients[1].PerpReserved["USD"])
	}
}
