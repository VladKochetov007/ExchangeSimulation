package exchange

import "testing"


func TestInsufficientLiquidityLimitOrder(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", CENT_TICK, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI / 2,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(2, buyReq)
	if !resp.Success {
		t.Fatalf("Order should succeed even with partial fill")
	}
	orderID := resp.Data.(uint64)

	book := ex.Books["BTC/USD"]
	bidOrder := book.Bids.Orders[orderID]

	if bidOrder == nil {
		t.Fatalf("Partially filled limit order should remain on book")
	}

	if bidOrder.FilledQty != SATOSHI/2 {
		t.Errorf("Expected FilledQty %d, got %d", SATOSHI/2, bidOrder.FilledQty)
	}

	if bidOrder.Status != PartialFill {
		t.Errorf("Order status should be PartialFill, got %v", bidOrder.Status)
	}

	remainingQty := bidOrder.Qty - bidOrder.FilledQty
	if remainingQty != SATOSHI/2 {
		t.Errorf("Expected remaining qty %d, got %d", SATOSHI/2, remainingQty)
	}
}

func TestInsufficientLiquidityMarketOrder(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI / 2,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}

	resp := ex.placeOrder(2, buyReq)
	if !resp.Success {
		t.Fatalf("Market order should succeed with partial fill")
	}
	orderID := resp.Data.(uint64)

	book := ex.Books["BTC/USD"]
	bidOrder := book.Bids.Orders[orderID]

	if bidOrder != nil {
		t.Errorf("Market order should not remain on book (unfilled portion cancelled)")
	}

	client := ex.Clients[2]
	expectedBTC := int64(SATOSHI / 2)
	actualBTC := client.GetBalance("BTC")

	if actualBTC < expectedBTC {
		t.Errorf("Client should have received %d BTC from partial fill, got %d", expectedBTC, actualBTC)
	}
}

func TestFOKOrderNotImplemented(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", CENT_TICK, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI / 2,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: FOK,
	}

	resp := ex.placeOrder(2, buyReq)

	if !resp.Success {
		if resp.Error != RejectFOKNotFilled {
			t.Fatalf("FOK rejection expected RejectFOKNotFilled, got %v", resp.Error)
		}
	} else {
		t.Fatal("FOK order should reject on partial fill")
	}
}

func TestFOKOrderFullyFilled(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", CENT_TICK, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: FOK,
	}

	resp := ex.placeOrder(2, buyReq)
	if !resp.Success {
		t.Fatalf("FOK order with sufficient liquidity should succeed, got error %v", resp.Error)
	}
}

func TestIOCOrderPartialFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", CENT_TICK, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI / 2,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}

	resp := ex.placeOrder(2, buyReq)
	if !resp.Success {
		t.Fatalf("IOC order should accept partial fill, got error %v", resp.Error)
	}

	book := ex.Books["BTC/USD"]
	if book.Bids.Best != nil {
		t.Fatal("IOC order should not rest on book after partial fill")
	}

	client2 := ex.Clients[2]
	btcBalance := client2.Balances["BTC"]
	if btcBalance != SATOSHI/2+BTCAmount(10) {
		t.Fatalf("Expected client2 to receive 0.5 BTC from partial fill, got %d", btcBalance)
	}
}

func TestIOCOrderNoFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", CENT_TICK, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})

	buyReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}

	resp := ex.placeOrder(1, buyReq)
	if !resp.Success {
		t.Fatalf("IOC order with no match should still succeed, got error %v", resp.Error)
	}

	book := ex.Books["BTC/USD"]
	if book.Bids.Best != nil {
		t.Fatal("IOC order should not rest on book when no match")
	}
}

func TestEmptyBookMarketOrder(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"USD": 100000 * SATOSHI}
	ex.ConnectClient(1, balances, &FixedFee{})

	buyReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}

	resp := ex.placeOrder(1, buyReq)
	if !resp.Success {
		t.Fatalf("Market order against empty book should succeed (fills nothing)")
	}

	client := ex.Clients[1]
	btcBalance := client.GetBalance("BTC")
	if btcBalance != 0 {
		t.Errorf("Client should have 0 BTC (nothing executed), got %d", btcBalance)
	}
}

func TestPartialFillReleasesCorrectAmount(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", SATOSHI, SATOSHI/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       50000 * SATOSHI,
		Qty:         SATOSHI / 2,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, sellReq)

	client := ex.Clients[2]

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000 * SATOSHI,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(2, buyReq)
	if !resp.Success {
		t.Fatalf("Order should succeed")
	}

	actualReserved := client.GetReserved("USD")

	if actualReserved == 0 {
		t.Errorf("Should have reserved USD for unfilled portion")
	}

	actualAvailable := client.GetAvailable("USD")

	if actualAvailable < 0 {
		t.Errorf("Available balance should not be negative, got %d", actualAvailable)
	}

	btcReceived := client.GetBalance("BTC") - (10 * SATOSHI)
	if btcReceived != SATOSHI/2 {
		t.Errorf("Expected to receive %d BTC from partial fill, got %d", SATOSHI/2, btcReceived)
	}
}
