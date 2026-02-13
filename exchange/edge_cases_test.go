package exchange

import "testing"

func TestMatchingPriority(t *testing.T) {
	matcher := NewDefaultMatcher()
	priority := matcher.Priority()

	if priority.Primary != PriorityPrice {
		t.Errorf("Expected primary priority to be PriorityPrice, got %v", priority.Primary)
	}
	if priority.Secondary != PriorityVisibility {
		t.Errorf("Expected secondary priority to be PriorityVisibility, got %v", priority.Secondary)
	}
	if priority.Tertiary != PriorityTime {
		t.Errorf("Expected tertiary priority to be PriorityTime, got %v", priority.Tertiary)
	}
}

func TestMinFunction(t *testing.T) {
	if min(10, 20) != 10 {
		t.Errorf("min(10, 20) should be 10")
	}
	if min(20, 10) != 10 {
		t.Errorf("min(20, 10) should be 10")
	}
	if min(15, 15) != 15 {
		t.Errorf("min(15, 15) should be 15")
	}
}

func TestCanMatchSellSide(t *testing.T) {
	matcher := NewDefaultMatcher()

	sellOrder := &Order{
		Side:  Sell,
		Type:  LimitOrder,
		Price: 50000,
	}

	limit := &Limit{Price: 49000}

	if matcher.canMatch(sellOrder, limit) {
		t.Errorf("Sell at 50000 should not match bid at 49000")
	}

	limit.Price = 50000
	if !matcher.canMatch(sellOrder, limit) {
		t.Errorf("Sell at 50000 should match bid at 50000")
	}

	limit.Price = 51000
	if !matcher.canMatch(sellOrder, limit) {
		t.Errorf("Sell at 50000 should match bid at 51000")
	}
}

func TestPlaceOrderInvalidPrice(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, 100, 1)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50001,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if resp.Success {
		t.Errorf("Should reject invalid price")
	}
	if resp.Error != RejectInvalidPrice {
		t.Errorf("Expected RejectInvalidPrice, got %v", resp.Error)
	}
}

func TestPlaceOrderInvalidQty(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, 1, 100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         0,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if resp.Success {
		t.Errorf("Should reject invalid qty")
	}
	if resp.Error != RejectInvalidQty {
		t.Errorf("Expected RejectInvalidQty, got %v", resp.Error)
	}
}

func TestPlaceOrderUnknownInstrument(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	balances := map[string]int64{"BTC": BTCAmount(10)}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "UNKNOWN/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000,
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if resp.Success {
		t.Errorf("Should reject unknown instrument")
	}
	if resp.Error != RejectUnknownInstrument {
		t.Errorf("Expected RejectUnknownInstrument, got %v", resp.Error)
	}
}

func TestPlaceOrderInsufficientBalanceBuy(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"USD": 1000}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, SATOSHI),
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if resp.Success {
		t.Errorf("Should reject insufficient balance")
	}
	if resp.Error != RejectInsufficientBalance {
		t.Errorf("Expected RejectInsufficientBalance, got %v", resp.Error)
	}
}

func TestPlaceOrderInsufficientBalanceSell(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": 100}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if resp.Success {
		t.Errorf("Should reject insufficient balance")
	}
	if resp.Error != RejectInsufficientBalance {
		t.Errorf("Expected RejectInsufficientBalance, got %v", resp.Error)
	}
}

func TestPlaceOrderMarketBuyInsufficientBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"USD": 1000, "BTC": BTCAmount(10)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	sellReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, SATOSHI),
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}
	ex.placeOrder(2, sellReq)

	buyReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        Market,
		Qty:         SATOSHI,
		TimeInForce: IOC,
	}

	resp := ex.placeOrder(1, buyReq)
	if resp.Success {
		t.Errorf("Market buy should fail with insufficient balance")
	}
}

func TestCancelOrderDifferentClient(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})
	ex.ConnectClient(2, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         SATOSHI,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if !resp.Success {
		t.Fatalf("Order placement failed")
	}
	orderID := resp.Data.(uint64)

	cancelReq := &CancelRequest{
		RequestID: 2,
		OrderID:   orderID,
	}
	cancelResp := ex.cancelOrder(2, cancelReq)
	if cancelResp.Success {
		t.Errorf("Should not allow canceling another client's order")
	}
}

func TestBookCancelOrderRemovesLimit(t *testing.T) {
	book := newBook(Buy)

	order := getOrder()
	order.ID = 1
	order.ClientID = 100
	order.Price = 50000
	order.Qty = 100
	book.addOrder(order)

	if len(book.Limits) != 1 {
		t.Fatalf("Expected 1 limit, got %d", len(book.Limits))
	}

	book.cancelOrder(1)

	if len(book.Limits) != 0 {
		t.Errorf("Expected 0 limits after canceling last order, got %d", len(book.Limits))
	}
	if book.Best != nil {
		t.Errorf("Best should be nil after removing last limit")
	}
}

func TestBookRemoveLimitUpdatesBest(t *testing.T) {
	book := newBook(Buy)

	order1 := getOrder()
	order1.ID = 1
	order1.ClientID = 100
	order1.Price = 50000
	order1.Qty = 100
	book.addOrder(order1)

	order2 := getOrder()
	order2.ID = 2
	order2.ClientID = 101
	order2.Price = 49000
	order2.Qty = 100
	book.addOrder(order2)

	if book.Best.Price != 50000 {
		t.Fatalf("Best should be 50000, got %d", book.Best.Price)
	}

	book.cancelOrder(1)

	if book.Best.Price != 49000 {
		t.Errorf("Best should be updated to 49000, got %d", book.Best.Price)
	}
}

func TestDisconnectClient(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	balances := map[string]int64{"BTC": BTCAmount(10)}
	ex.ConnectClient(1, balances, &FixedFee{})

	if ex.Gateways[1] == nil {
		t.Fatalf("Gateway should exist")
	}

	ex.DisconnectClient(1)

	if ex.Gateways[1] != nil {
		t.Errorf("Gateway should be removed")
	}
}

func TestShutdownStopsExchange(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.running = true

	ex.Shutdown()

	if ex.running {
		t.Errorf("Exchange should not be running after shutdown")
	}
}

func TestSettleFundingWithMissingClient(t *testing.T) {
	pm := NewPositionManager(&RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, 1, 100)

	clients := make(map[uint64]*Client)

	pm.UpdatePosition(1, "BTC-PERP", SATOSHI, 50000*SATOSHI, Buy)

	perp.UpdateFundingRate(50000*SATOSHI, 50100*SATOSHI)

	pm.SettleFunding(clients, perp, nil)
}
