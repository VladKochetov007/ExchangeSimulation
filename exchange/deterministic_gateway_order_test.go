package exchange

import "testing"

func gatewayIDs(gateways []*ClientGateway) []uint64 {
	ids := make([]uint64, 0, len(gateways))
	for _, gateway := range gateways {
		if gateway != nil {
			ids = append(ids, gateway.ClientID)
		}
	}
	return ids
}

func TestDeterministicGatewaySnapshotReconcilesDirectMapEdits(t *testing.T) {
	exchange := NewExchangeWithConfig(ExchangeConfig{
		Clock:                &RealClock{},
		DeterministicIngress: true,
		DeterministicPhases:  true,
	})
	defer exchange.Shutdown()

	first := exchange.ConnectNewClient(1, nil, &FixedFee{}).(*ClientGateway)
	defer first.Close()
	if got, want := gatewayIDs(exchange.deterministicGatewaySnapshot()), []uint64{1}; !equalUint64s(got, want) {
		t.Fatalf("initial gateway order = %v, want %v", got, want)
	}

	second := NewClientGateway(2)
	defer second.Close()
	exchange.Gateways[2] = second
	if got, want := gatewayIDs(exchange.deterministicGatewaySnapshot()), []uint64{1, 2}; !equalUint64s(got, want) {
		t.Fatalf("added direct gateway order = %v, want %v", got, want)
	}

	reconnected := NewClientGateway(2)
	defer reconnected.Close()
	exchange.Gateways[2] = reconnected
	snapshot := exchange.deterministicGatewaySnapshot()
	if got, want := gatewayIDs(snapshot), []uint64{1, 2}; !equalUint64s(got, want) {
		t.Fatalf("same-key reconnect gateway order = %v, want %v", got, want)
	}
	if snapshot[1] != reconnected {
		t.Fatal("gateway snapshot retained a stale pointer after same-key reconnect")
	}

	delete(exchange.Gateways, 1)
	if got, want := gatewayIDs(exchange.deterministicGatewaySnapshot()), []uint64{2}; !equalUint64s(got, want) {
		t.Fatalf("deleted direct gateway order = %v, want %v", got, want)
	}

	replacement := NewClientGateway(3)
	defer replacement.Close()
	delete(exchange.Gateways, 2)
	exchange.Gateways[3] = replacement
	snapshot = exchange.deterministicGatewaySnapshot()
	if got, want := gatewayIDs(snapshot), []uint64{3}; !equalUint64s(got, want) {
		t.Fatalf("same-cardinality direct replacement order = %v, want %v", got, want)
	}
	if snapshot[0] != replacement {
		t.Fatal("gateway snapshot retained a stale pointer after direct replacement")
	}
	if first.IsRunning() == false {
		t.Fatal("registered gateway unexpectedly stopped before shutdown")
	}
}

func TestDeterministicGatewaySnapshotTracksOfficialLifecycle(t *testing.T) {
	exchange := NewExchangeWithConfig(ExchangeConfig{
		Clock:                &RealClock{},
		DeterministicIngress: true,
		DeterministicPhases:  true,
	})
	defer exchange.Shutdown()

	first := exchange.ConnectNewClient(1, nil, &FixedFee{}).(*ClientGateway)
	second := exchange.ConnectNewClient(2, nil, &FixedFee{}).(*ClientGateway)
	if got, want := gatewayIDs(exchange.deterministicGatewaySnapshot()), []uint64{1, 2}; !equalUint64s(got, want) {
		t.Fatalf("connected gateway order = %v, want %v", got, want)
	}

	exchange.DisconnectClient(1)
	if first.IsRunning() {
		t.Fatal("official disconnect left the retired gateway running")
	}
	if got, want := gatewayIDs(exchange.deterministicGatewaySnapshot()), []uint64{2}; !equalUint64s(got, want) {
		t.Fatalf("disconnected gateway order = %v, want %v", got, want)
	}

	reconnected := exchange.ConnectNewClient(1, nil, &FixedFee{}).(*ClientGateway)
	if second.IsRunning() == false {
		t.Fatal("unrelated gateway stopped during reconnect")
	}
	if got, want := gatewayIDs(exchange.deterministicGatewaySnapshot()), []uint64{1, 2}; !equalUint64s(got, want) {
		t.Fatalf("reconnected gateway order = %v, want %v", got, want)
	}
	if exchange.deterministicGatewaySnapshot()[0] != reconnected {
		t.Fatal("reconnected gateway was not the authoritative snapshot pointer")
	}
	if reconnected.IsRunning() == false {
		t.Fatal("official reconnect returned a stopped gateway")
	}
}

func TestDeterministicIngressAndEgressPreserveClientOrder(t *testing.T) {
	exchange := NewExchangeWithConfig(ExchangeConfig{
		Clock:                &RealClock{},
		DeterministicIngress: true,
		DeterministicPhases:  true,
	})
	defer exchange.Shutdown()
	exchange.AddInstrument(NewSpotInstrument(
		"ABC-USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/100,
	))
	first := exchange.ConnectNewClient(1, map[string]int64{"USD": 1_000 * USD_PRECISION}, &FixedFee{}).(*ClientGateway)
	second := exchange.ConnectNewClient(2, map[string]int64{"ABC": BTC_PRECISION}, &FixedFee{}).(*ClientGateway)

	first.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 11, Symbol: "ABC-USD", Side: Buy, Type: LimitOrder,
		Price: PriceUSD(100, DOLLAR_TICK), Qty: BTC_PRECISION,
		TimeInForce: GTC, Visibility: Normal,
	}})
	second.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 22, Symbol: "ABC-USD", Side: Sell, Type: LimitOrder,
		Price: PriceUSD(100, DOLLAR_TICK), Qty: BTC_PRECISION,
		TimeInForce: GTC, Visibility: Normal,
	}})
	if !exchange.DrainIngress() {
		t.Fatal("deterministic ingress processed no requests")
	}
	if !exchange.DrainDeterministicEgress() {
		t.Fatal("deterministic egress delivered no responses")
	}

	firstResponses := []Response{<-first.ResponseCh, <-first.ResponseCh}
	secondResponses := []Response{<-second.ResponseCh, <-second.ResponseCh}
	if got := acceptedOrderID(firstResponses, 11); got != 2 {
		t.Fatalf("client 1 accepted order ID = %d, want 2; responses=%+v", got, firstResponses)
	}
	if got := acceptedOrderID(secondResponses, 22); got != 3 {
		t.Fatalf("client 2 accepted order ID = %d, want 3; responses=%+v", got, secondResponses)
	}
	if exchange.Books["ABC-USD"].Bids.Best != nil || exchange.Books["ABC-USD"].Asks.Best != nil {
		t.Fatal("crossing requests did not consume the resting liquidity")
	}
}

func acceptedOrderID(responses []Response, requestID uint64) uint64 {
	for _, response := range responses {
		if response.RequestID == requestID {
			orderID, ok := response.Data.(uint64)
			if ok {
				return orderID
			}
		}
	}
	return 0
}

func equalUint64s(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
