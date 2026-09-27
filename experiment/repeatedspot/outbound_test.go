package repeatedspot

import (
	"encoding/json"
	"testing"

	"exchange_sim/exchange"
	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestOutboundCancellationNeedsExactPriorActorRequest(t *testing.T) {
	state := &replayState{
		contract:     replayContract{Instrument: worldspot.InstrumentConfig{Symbol: "ABC/USD"}},
		accounts:     map[uint64]*accountState{1: {clientID: 1}},
		sentRequests: make(map[requestKey]*sentOrder),
	}
	request := exchange.Request{Type: exchange.ReqCancelOrder,
		CancelReq: &exchange.CancelRequest{RequestID: 7, OrderID: 42}}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.orderSend(Event{Timestamp: 5, ClientID: 1, Route: "ABC/USD", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	cancel := Event{Timestamp: 6, ClientID: 1, Name: "OrderCancelled",
		Payload: json.RawMessage(`{"order_id":42,"request_id":7,"remaining_qty":1}`)}
	if err := state.checkOrderOutcome(cancel); err != nil {
		t.Fatal(err)
	}
	if state.accounts[1].outbound.CancelSent != 1 || state.accounts[1].outbound.CancelAccepted != 1 {
		t.Fatalf("cancel funnel not counted: %+v", state.accounts[1].outbound)
	}
	if err := state.checkOrderOutcome(cancel); err == nil {
		t.Fatal("duplicate cancel outcome accepted")
	}
	wrong := Event{Timestamp: 7, ClientID: 1, Name: "OrderCancelRejected",
		Payload: json.RawMessage(`{"order_id":99,"request_id":8}`)}
	if err := state.checkOrderOutcome(wrong); err == nil {
		t.Fatal("unanchored cancel rejection accepted")
	}
	forced := Event{Timestamp: 8, ClientID: 1, Name: "OrderCancelled",
		Payload: json.RawMessage(`{"order_id":42,"remaining_qty":1,"reason":"EXCHANGE_FORCED_LIFECYCLE"}`)}
	if err := state.checkOrderOutcome(forced); err != nil {
		t.Fatalf("venue-forced cancellation should not require actor request: %v", err)
	}
	forced.Payload = json.RawMessage(`{"order_id":42,"remaining_qty":1,"reason":"UNREGISTERED_REASON"}`)
	if err := state.checkOrderOutcome(forced); err == nil {
		t.Fatal("unknown forced-cancel reason bypassed actor request join")
	}
}

func TestOutboundRejectedPlacementKeepsFailureInFunnel(t *testing.T) {
	state := &replayState{
		contract:     replayContract{Instrument: worldspot.InstrumentConfig{Symbol: "ABC/USD"}},
		accounts:     map[uint64]*accountState{3: {clientID: 3}},
		sentRequests: make(map[requestKey]*sentOrder),
	}
	request := exchange.Request{Type: exchange.ReqPlaceOrder, OrderReq: &exchange.OrderRequest{
		RequestID: 11, Symbol: "ABC/USD", Side: exchange.Buy, Type: exchange.LimitOrder,
		Price: 101, Qty: 1, TimeInForce: exchange.GTC, Visibility: exchange.Normal,
	}}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.orderSend(Event{Timestamp: 5, ClientID: 3, Route: "ABC/USD", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	rejected := Event{Timestamp: 6, ClientID: 3, Name: "OrderRejected",
		Payload: json.RawMessage(`{"request_id":11,"success":false,"error":"INSUFFICIENT_BALANCE","symbol":"ABC/USD","side":"BUY","type":"LIMIT","time_in_force":"GTC","post_only":false,"price":101,"qty":1}`)}
	if err := state.checkOrderOutcome(rejected); err != nil {
		t.Fatal(err)
	}
	if state.accounts[3].outbound.PlaceSent != 1 || state.accounts[3].outbound.PlaceRejected != 1 ||
		state.accounts[3].outbound.PlaceAccepted != 0 {
		t.Fatalf("rejected placement was lost or treated as admitted: %+v", state.accounts[3].outbound)
	}
	rejected.Payload = json.RawMessage(`{"request_id":11,"symbol":"ABC/USD","side":"SELL","type":"LIMIT","time_in_force":"GTC","post_only":false,"price":101,"qty":1}`)
	if err := state.checkOrderOutcome(rejected); err == nil {
		t.Fatal("duplicate/mismatched rejection accepted")
	}
}
