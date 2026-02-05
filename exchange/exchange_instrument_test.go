package exchange

import "testing"

func TestListInstruments(t *testing.T) {
	ex := NewExchange(10, &RealClock{})

	btcusd := NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, DOLLAR_TICK, SATOSHI/1000)
	ethusd := NewSpotInstrument("ETHUSD", "ETH", "USD", ETH_PRECISION, USD_PRECISION, ETH_PRECISION/100, ETH_PRECISION/1000)
	ethbtc := NewSpotInstrument("ETHBTC", "ETH", "BTC",
		ETH_PRECISION, BTC_PRECISION, BTC_PRECISION/100, ETH_PRECISION/1000)

	ex.AddInstrument(btcusd)
	ex.AddInstrument(ethusd)
	ex.AddInstrument(ethbtc)

	all := ex.ListInstruments("", "")
	if len(all) != 3 {
		t.Errorf("Expected 3 instruments, got %d", len(all))
	}

	usdQuoted := ex.ListInstruments("", "USD")
	if len(usdQuoted) != 2 {
		t.Errorf("Expected 2 USD-quoted instruments, got %d", len(usdQuoted))
	}

	btcBase := ex.ListInstruments("BTC", "")
	if len(btcBase) != 1 {
		t.Errorf("Expected 1 BTC-base instrument, got %d", len(btcBase))
	}

	ethBtc := ex.ListInstruments("ETH", "BTC")
	if len(ethBtc) != 1 {
		t.Errorf("Expected 1 ETH/BTC instrument, got %d", len(ethBtc))
	}
	if ethBtc[0].Symbol != "ETHBTC" {
		t.Errorf("Expected ETHBTC, got %s", ethBtc[0].Symbol)
	}
}

func TestCancelOrderValidationNotFound(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	btcusd := NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, DOLLAR_TICK, SATOSHI/1000)
	ex.AddInstrument(btcusd)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000}
	gateway := ex.ConnectClient(1, balances, &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

	req := Request{
		Type: ReqCancelOrder,
		CancelReq: &CancelRequest{
			RequestID: 1,
			OrderID:   999,
		},
	}
	gateway.RequestCh <- req

	resp := <-gateway.ResponseCh
	if resp.Success {
		t.Error("Expected cancel to fail for non-existent order")
	}
	if resp.Error != RejectOrderNotFound {
		t.Errorf("Expected RejectOrderNotFound, got %v", resp.Error)
	}
}

func TestCancelOrderValidationNotOwned(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	btcusd := NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, DOLLAR_TICK, SATOSHI/1000)
	ex.AddInstrument(btcusd)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000}
	gateway1 := ex.ConnectClient(1, balances, &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})
	gateway2 := ex.ConnectClient(2, balances, &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

	orderReq := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID: 1,
			Side:      Buy,
			Type:      LimitOrder,
			Price:     5000000000000,
			Qty:       100000000,
			Symbol:    "BTCUSD",
		},
	}
	gateway1.RequestCh <- orderReq
	orderResp := <-gateway1.ResponseCh
	if !orderResp.Success {
		t.Fatal("Order placement failed")
	}
	orderID := orderResp.Data.(uint64)

	cancelReq := Request{
		Type: ReqCancelOrder,
		CancelReq: &CancelRequest{
			RequestID: 2,
			OrderID:   orderID,
		},
	}
	gateway2.RequestCh <- cancelReq

	cancelResp := <-gateway2.ResponseCh
	if cancelResp.Success {
		t.Error("Expected cancel to fail for order owned by different client")
	}
	if cancelResp.Error != RejectOrderNotOwned {
		t.Errorf("Expected RejectOrderNotOwned, got %v", cancelResp.Error)
	}
}

func TestCancelOrderValidationAfterPartialFill(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	btcusd := NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, DOLLAR_TICK, SATOSHI/1000)
	ex.AddInstrument(btcusd)

	balances := map[string]int64{"BTC": 1000000000, "USD": 100000000000}
	gateway1 := ex.ConnectClient(1, balances, &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})
	gateway2 := ex.ConnectClient(2, balances, &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true})

	sellReq := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID: 1,
			Side:      Sell,
			Type:      LimitOrder,
			Price:     5000000000000,
			Qty:       200000000,
			Symbol:    "BTCUSD",
		},
	}
	gateway1.RequestCh <- sellReq
	sellResp := <-gateway1.ResponseCh
	if !sellResp.Success {
		t.Fatal("Sell order placement failed")
	}
	orderID := sellResp.Data.(uint64)

	buyReq := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID: 2,
			Side:      Buy,
			Type:      Market,
			Qty:       100000000,
			Symbol:    "BTCUSD",
		},
	}
	gateway2.RequestCh <- buyReq
	<-gateway2.ResponseCh

	// Consume the fill notification for the maker (gateway1)
	fillResp := <-gateway1.ResponseCh
	if _, ok := fillResp.Data.(*FillNotification); !ok {
		t.Fatalf("Expected FillNotification for maker, got %T", fillResp.Data)
	}

	cancelReq := Request{
		Type: ReqCancelOrder,
		CancelReq: &CancelRequest{
			RequestID: 3,
			OrderID:   orderID,
		},
	}
	gateway1.RequestCh <- cancelReq

	cancelResp := <-gateway1.ResponseCh
	if !cancelResp.Success {
		t.Errorf("Expected cancel to succeed for partially filled order, got error: %v", cancelResp.Error)
	}
	remainingQty := cancelResp.Data.(int64)
	if remainingQty != 100000000 {
		t.Errorf("Expected remaining qty 100000000, got %d", remainingQty)
	}
}
