package exchange

import "testing"

func TestNewExchange(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	if ex == nil {
		t.Fatal("Exchange should not be nil")
	}
	if len(ex.Clients) != 0 {
		t.Error("Clients should be empty")
	}
}

func TestAddInstrument(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, 100, 1000)
	ex.AddInstrument(instrument)

	if ex.Instruments["BTC/USD"] == nil {
		t.Error("Instrument should be added")
	}
	if ex.Books["BTC/USD"] == nil {
		t.Error("OrderBook should be created")
	}
}

func TestConnectClient(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, 100, 1000)
	ex.AddInstrument(instrument)

	feePlan := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	balances := map[string]int64{"USD": USDAmount(100000)}
	gateway := ex.ConnectClient(1, balances, feePlan)

	if gateway == nil {
		t.Fatal("Gateway should not be nil")
	}
	if ex.Clients[1] == nil {
		t.Error("Client should be added")
	}
	if ex.Clients[1].Balances["USD"] != USDAmount(100000) {
		t.Error("Client balance should be set")
	}

	ex.DisconnectClient(1)
}

func TestPlaceOrderDirect(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, USD_PRECISION, USD_PRECISION/100)
	ex.AddInstrument(instrument)

	feePlan := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	balances := map[string]int64{"USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, feePlan)

	orderReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, SATOSHI),
		Qty:         SATOSHI / 100, // 0.01 BTC
		TimeInForce: GTC,
		Visibility:  Normal,
	}

	resp := ex.placeOrder(1, orderReq)
	if !resp.Success {
		t.Fatalf("Order should succeed, got error: %v", resp.Error)
	}

	client := ex.Clients[1]
	notional := int64((orderReq.Qty * orderReq.Price) / SATOSHI)
	if client.Reserved["USD"] != notional {
		t.Errorf("Should have reserved %d USD, got %d", notional, client.Reserved["USD"])
	}
}

func TestMatchingAndSettlement(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, 100, 1000)
	ex.AddInstrument(instrument)

	feePlan := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}

	balances1 := map[string]int64{"USD": USDAmount(100000)}
	ex.ConnectClient(1, balances1, feePlan)

	balances2 := map[string]int64{"BTC": BTCAmount(10)}
	ex.ConnectClient(2, balances2, feePlan)

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       50000 * 100,
		Qty:         SATOSHI,
		TimeInForce: GTC,
		Visibility:  Normal,
	}
	ex.placeOrder(2, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000 * 100,
		Qty:         SATOSHI,
		TimeInForce: GTC,
		Visibility:  Normal,
	}
	ex.placeOrder(1, buyReq)

	client1 := ex.Clients[1]
	client2 := ex.Clients[2]

	if client1.Balances["BTC"] != SATOSHI {
		t.Errorf("Client 1 should have 1 BTC, got %d", client1.Balances["BTC"])
	}
	if client2.Balances["BTC"] != 9*SATOSHI {
		t.Errorf("Client 2 should have 9 BTC, got %d", client2.Balances["BTC"])
	}
}
