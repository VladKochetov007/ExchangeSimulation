package exchange

import (
	"testing"
	"time"
)

// sendRequest sends any Request through the gateway and waits for the matching Response.
func sendRequest(gateway *ClientGateway, req Request, reqID uint64) Response {
	gateway.RequestCh <- req
	timeout := time.After(2 * time.Second)
	for {
		select {
		case resp := <-gateway.ResponseCh:
			if resp.RequestID == reqID {
				return resp
			}
		case <-timeout:
			panic("test timeout waiting for response")
		}
	}
}

// setupSpotExchange creates a minimal spot exchange with two clients.
func setupSpotExchange() *Exchange {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(100_000), "BTC": BTCAmount(10)}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{"USD": USDAmount(100_000), "BTC": BTCAmount(10)}, &FixedFee{})
	return ex
}

func TestPlaceOrder_RejectsUnknownInstrument(t *testing.T) {
	ex := setupSpotExchange()
	_, reason := InjectLimitOrder(ex, 1, "ETH/USD", Buy, PriceUSD(2_000, DOLLAR_TICK), BTCAmount(1))
	if reason != RejectUnknownInstrument {
		t.Errorf("expected RejectUnknownInstrument, got %v", reason)
	}
}

func TestPlaceOrder_RejectsInvalidPrice(t *testing.T) {
	ex := setupSpotExchange()
	// Price not aligned to tick size (DOLLAR_TICK = USD_PRECISION, so 50_000.5 USD is invalid)
	badPrice := PriceUSD(50_000, DOLLAR_TICK) + 1 // +1 unit breaks tick alignment
	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Buy, badPrice, BTCAmount(1))
	if reason != RejectInvalidPrice {
		t.Errorf("expected RejectInvalidPrice, got %v", reason)
	}
}

func TestPlaceOrder_RejectsInvalidQty(t *testing.T) {
	ex := setupSpotExchange()
	// MinOrderSize for BTC is SATOSHI = 1 BTC satoshi; use 0 which is below min
	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), 0)
	if reason != RejectInvalidQty {
		t.Errorf("expected RejectInvalidQty, got %v", reason)
	}
}

func TestPlaceOrder_RejectsSpotMarketBuyInsufficientBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	// Give only $1 spot — not enough to buy 1 BTC at $50k
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(1)}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})

	// Seed the ask side so the check has a reference price
	_, _ = InjectLimitOrder(ex, 2, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))

	_, reason := InjectMarketOrder(ex, 1, "BTC/USD", Buy, BTCAmount(1))
	if reason != RejectInsufficientBalance {
		t.Errorf("expected RejectInsufficientBalance, got %v", reason)
	}
}

func TestPlaceOrder_RejectsSpotMarketSellInsufficientBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	// Give only 0.0001 BTC — not enough to sell 1 BTC
	ex.ConnectClient(1, map[string]int64{"BTC": BTCAmount(0.0001)}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{"USD": USDAmount(100_000)}, &FixedFee{})

	// Seed the bid side
	_, _ = InjectLimitOrder(ex, 2, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))

	_, reason := InjectMarketOrder(ex, 1, "BTC/USD", Sell, BTCAmount(1))
	if reason != RejectInsufficientBalance {
		t.Errorf("expected RejectInsufficientBalance, got %v", reason)
	}
}

func TestPlaceOrder_RejectsSpotLimitBuyInsufficientBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	// $10 — not enough to reserve for 1 BTC limit buy at $50k
	ex.ConnectClient(1, map[string]int64{"USD": USDAmount(10)}, &FixedFee{})

	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Buy, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	if reason != RejectInsufficientBalance {
		t.Errorf("expected RejectInsufficientBalance, got %v", reason)
	}
}

func TestPlaceOrder_RejectsSpotLimitSellInsufficientBalance(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	// 0.0001 BTC — not enough to reserve for a 1 BTC limit sell
	ex.ConnectClient(1, map[string]int64{"BTC": BTCAmount(0.0001)}, &FixedFee{})

	_, reason := InjectLimitOrder(ex, 1, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	if reason != RejectInsufficientBalance {
		t.Errorf("expected RejectInsufficientBalance, got %v", reason)
	}
}

func TestPlaceOrder_FOKNotFilled(t *testing.T) {
	ex := setupSpotExchange()

	const reqID = uint64(9999)
	gateway := ex.Gateways[1]

	// Place a FOK buy for 1 BTC but the ask book is empty — cannot fill fully.
	// Client has $100k so can afford the $50k reserve, but no counterparty exists.
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			Symbol:      "BTC/USD",
			TimeInForce: FOK,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectFOKNotFilled {
		t.Errorf("expected RejectFOKNotFilled, got success=%v error=%v", resp.Success, resp.Error)
	}
}

func TestPlaceOrder_FOKPerp_NotFilled(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(100_000))
	const reqID = uint64(9998)
	gateway := ex.Gateways[1]

	// Place a FOK buy for 1 BTC perp but ask book is empty — cannot fill
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			Symbol:      "BTC-PERP",
			TimeInForce: FOK,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectFOKNotFilled {
		t.Errorf("expected RejectFOKNotFilled for perp, got success=%v error=%v", resp.Success, resp.Error)
	}
}

func TestPlaceOrder_FOKSell_NotFilled(t *testing.T) {
	ex := setupSpotExchange()
	const reqID = uint64(9997)
	gateway := ex.Gateways[1]

	// FOK sell — no bids in book, cannot fill
	req := Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:   reqID,
			Side:        Sell,
			Type:        LimitOrder,
			Price:       PriceUSD(50_000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			Symbol:      "BTC/USD",
			TimeInForce: FOK,
			Visibility:  Normal,
		},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectFOKNotFilled {
		t.Errorf("expected RejectFOKNotFilled for sell, got success=%v error=%v", resp.Success, resp.Error)
	}
}

func TestCancelOrder_OrderNotOwned(t *testing.T) {
	ex := setupSpotExchange()

	// Client 2 places a sell order
	orderID, _ := InjectLimitOrder(ex, 2, "BTC/USD", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))
	if orderID == 0 {
		t.Fatal("order placement failed")
	}

	// Client 1 tries to cancel client 2's order
	const reqID = uint64(8888)
	gateway := ex.Gateways[1]
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: orderID},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectOrderNotOwned {
		t.Errorf("expected RejectOrderNotOwned, got success=%v error=%v", resp.Success, resp.Error)
	}
}

func TestCancelOrder_OrderNotFound(t *testing.T) {
	ex := setupSpotExchange()
	const reqID = uint64(8887)
	gateway := ex.Gateways[1]
	req := Request{
		Type:      ReqCancelOrder,
		CancelReq: &CancelRequest{RequestID: reqID, OrderID: 99999},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectOrderNotFound {
		t.Errorf("expected RejectOrderNotFound, got success=%v error=%v", resp.Success, resp.Error)
	}
}

func TestSubscribe_UnknownInstrument(t *testing.T) {
	ex := setupSpotExchange()
	const reqID = uint64(7777)
	gateway := ex.Gateways[1]
	req := Request{
		Type:     ReqSubscribe,
		QueryReq: &QueryRequest{RequestID: reqID, Symbol: "NONEXISTENT"},
	}
	resp := sendRequest(gateway, req, reqID)
	if resp.Success || resp.Error != RejectUnknownInstrument {
		t.Errorf("expected RejectUnknownInstrument from subscribe, got success=%v error=%v", resp.Success, resp.Error)
	}
}

func TestPlaceOrder_PerpMarketInsufficientBalance(t *testing.T) {
	// Client with tiny perp balance tries to open a large position
	ex, _ := setupPerpExchange(USDAmount(1), USDAmount(500_000))

	// Seed asks so market order has a reference price and the balance check fires
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, PriceUSD(50_000, DOLLAR_TICK), BTCAmount(1))

	_, reason := InjectMarketOrder(ex, 1, "BTC-PERP", Buy, BTCAmount(1))
	if reason != RejectInsufficientBalance {
		t.Errorf("expected RejectInsufficientBalance for perp market order, got %v", reason)
	}
}
