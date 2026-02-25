package exchange

import (
	"testing"
	"time"
)

func TestHandleClientRequestsCancelOrder(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	go ex.handleClientRequests(gateway)

	orderReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	gateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: orderReq}

	var orderID uint64
	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Fatalf("Order placement failed")
		}
		orderID = resp.Data.(uint64)
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for order response")
	}

	cancelReq := &CancelRequest{
		RequestID: 2,
		OrderID:   orderID,
	}
	gateway.RequestCh <- Request{Type: ReqCancelOrder, CancelReq: cancelReq}

	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Errorf("Cancel should succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for cancel response")
	}

	gateway.Close()
}

func TestHandleClientRequestsSubscribeUnsubscribe(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10)}
	gateway := ex.ConnectClient(1, balances, &FixedFee{})

	go ex.handleClientRequests(gateway)

	subscribeReq := &QueryRequest{
		RequestID: 1,
		Symbol:    "BTC/USD",
	}
	gateway.RequestCh <- Request{Type: ReqSubscribe, QueryReq: subscribeReq}

	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Fatalf("Subscribe should succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for subscribe response")
	}

	unsubscribeReq := &QueryRequest{
		RequestID: 2,
		Symbol:    "BTC/USD",
	}
	gateway.RequestCh <- Request{Type: ReqUnsubscribe, QueryReq: unsubscribeReq}

	select {
	case resp := <-gateway.ResponseCh:
		if !resp.Success {
			t.Fatalf("Unsubscribe should succeed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("Timeout waiting for unsubscribe response")
	}

	gateway.Close()
}

func TestHandleClientRequestsFullChannelSkipsResponse(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10)}
	gateway := &ClientGateway{
		ClientID:   1,
		RequestCh:  make(chan Request, 10),
		ResponseCh: make(chan Response),
		MarketData: make(chan *MarketDataMsg, 10),
	}

	ex.Clients[1] = NewClient(1, &FixedFee{})
	ex.Clients[1].Balances = balances
	ex.Gateways[1] = gateway

	go ex.handleClientRequests(gateway)

	queryReq := &QueryRequest{RequestID: 1}
	gateway.RequestCh <- Request{Type: ReqQueryBalance, QueryReq: queryReq}

	time.Sleep(50 * time.Millisecond)

	select {
	case <-gateway.ResponseCh:
		t.Errorf("Should not receive response when channel is full")
	default:
	}

	close(gateway.RequestCh)
}

func TestPlaceOrderSellReservesBase(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, CENT_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(2), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, CENT_TICK),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if !resp.Success {
		t.Fatalf("Order should succeed")
	}

	client := ex.Clients[1]
	reserved := client.GetReserved("BTC")
	if reserved != BTC_PRECISION {
		t.Errorf("Expected BTC reserved to be BTC_PRECISION, got %d", reserved)
	}
}

func TestPlaceOrderMarketSellNoAskBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(2), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        Market,
		Qty:         BTC_PRECISION,
		TimeInForce: IOC,
	}

	resp := ex.placeOrder(1, req)
	if !resp.Success {
		t.Errorf("Market sell should be allowed even with no bid book")
	}
}

func TestProcessExecutionsTakerSell(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true})
	ex.ConnectClient(2, balances, &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true})

	buyReq := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Buy,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, BTC_PRECISION),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}
	ex.placeOrder(1, buyReq)

	sellReq := &OrderRequest{
		RequestID:   2,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, BTC_PRECISION),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(2, sellReq)
	if !resp.Success {
		t.Fatalf("Sell order should match and succeed")
	}

	client1 := ex.Clients[1]
	if client1.MakerVolume == 0 {
		t.Errorf("Client 1 should have maker volume recorded")
	}

	client2 := ex.Clients[2]
	if client2.TakerVolume == 0 {
		t.Errorf("Client 2 should have taker volume recorded")
	}
}

func TestCancelOrderSellSide(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	instrument := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000)
	ex.AddInstrument(instrument)

	balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(100000)}
	ex.ConnectClient(1, balances, &FixedFee{})

	req := &OrderRequest{
		RequestID:   1,
		Symbol:      "BTC/USD",
		Side:        Sell,
		Type:        LimitOrder,
		Price:       PriceUSD(50000, BTC_PRECISION),
		Qty:         BTC_PRECISION,
		TimeInForce: GTC,
	}

	resp := ex.placeOrder(1, req)
	if !resp.Success {
		t.Fatalf("Order placement failed")
	}
	orderID := resp.Data.(uint64)

	client := ex.Clients[1]
	reservedBefore := client.GetReserved("BTC")

	cancelReq := &CancelRequest{
		RequestID: 2,
		OrderID:   orderID,
	}
	cancelResp := ex.cancelOrder(1, cancelReq)
	if !cancelResp.Success {
		t.Fatalf("Cancel should succeed")
	}

	reservedAfter := client.GetReserved("BTC")
	if reservedAfter >= reservedBefore {
		t.Errorf("Reserved BTC should be released")
	}
}

func TestBookInsertLimitSellSide(t *testing.T) {
	book := newBook(Sell)

	order1 := getOrder()
	order1.ID = 1
	order1.ClientID = 100
	order1.Price = 50000
	order1.Qty = 100
	book.AddOrder(order1)

	order2 := getOrder()
	order2.ID = 2
	order2.ClientID = 101
	order2.Price = 49000
	order2.Qty = 100
	book.AddOrder(order2)

	if book.Best.Price != 49000 {
		t.Errorf("Best ask should be 49000, got %d", book.Best.Price)
	}

	order3 := getOrder()
	order3.ID = 3
	order3.ClientID = 102
	order3.Price = 51000
	order3.Qty = 100
	book.AddOrder(order3)

	if book.ActiveTail.Price != 51000 {
		t.Errorf("Tail should be 51000, got %d", book.ActiveTail.Price)
	}
}

func TestBookRemoveLimitMiddle(t *testing.T) {
	book := newBook(Buy)

	order1 := getOrder()
	order1.ID = 1
	order1.ClientID = 100
	order1.Price = 50000
	order1.Qty = 100
	book.AddOrder(order1)

	order2 := getOrder()
	order2.ID = 2
	order2.ClientID = 101
	order2.Price = 49000
	order2.Qty = 100
	book.AddOrder(order2)

	order3 := getOrder()
	order3.ID = 3
	order3.ClientID = 102
	order3.Price = 48000
	order3.Qty = 100
	book.AddOrder(order3)

	book.CancelOrder(2)

	if len(book.Limits) != 2 {
		t.Errorf("Expected 2 limits after removing middle, got %d", len(book.Limits))
	}

	if book.Limits[49000] != nil {
		t.Errorf("49000 limit should be removed")
	}
}
