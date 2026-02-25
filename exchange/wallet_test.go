package exchange

import (
	"testing"
)

func TestSpotLimitBuyLocksQuoteInSpotReserved(t *testing.T) {
	ex := newPerpTestExchange()
	spot := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(spot)

	gw := ex.ConnectClient(1, map[string]int64{"USD": USDAmount(100000)}, &FixedFee{})
	defer ex.Shutdown()

	price := PriceUSD(50000, DOLLAR_TICK)
	req := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTCUSD", Side: Buy, Type: LimitOrder,
		Price: price, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw.RequestCh <- req
	resp := <-gw.ResponseCh
	if !resp.Success {
		t.Fatalf("order rejected: %v", resp.Error)
	}

	client := ex.Clients[1]
	expectedReserved := (BTC_PRECISION * price) / BTC_PRECISION
	if client.Reserved["USD"] != expectedReserved {
		t.Errorf("SpotReserved USD = %d, want %d", client.Reserved["USD"], expectedReserved)
	}
	if client.PerpReserved["USD"] != 0 {
		t.Errorf("PerpReserved should be 0, got %d", client.PerpReserved["USD"])
	}
}

func TestSpotLimitSellLocksBaseInSpotReserved(t *testing.T) {
	ex := newPerpTestExchange()
	spot := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(spot)

	gw := ex.ConnectClient(1, map[string]int64{"BTC": 2 * BTC_PRECISION, "USD": USDAmount(10000)}, &FixedFee{})
	defer ex.Shutdown()

	req := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTCUSD", Side: Sell, Type: LimitOrder,
		Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw.RequestCh <- req
	resp := <-gw.ResponseCh
	if !resp.Success {
		t.Fatalf("order rejected: %v", resp.Error)
	}

	client := ex.Clients[1]
	if client.Reserved["BTC"] != BTC_PRECISION {
		t.Errorf("Reserved BTC = %d, want %d", client.Reserved["BTC"], BTC_PRECISION)
	}
	if client.PerpReserved["USD"] != 0 {
		t.Errorf("PerpReserved should be 0 for spot order")
	}
}

func TestPerpLimitOrderLocksInPerpReserved(t *testing.T) {
	ex := newPerpTestExchange()
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	gw := ex.ConnectClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100000))
	defer ex.Shutdown()

	price := PriceUSD(50000, DOLLAR_TICK)
	initialMargin := (BTC_PRECISION * price / BTC_PRECISION) * perp.MarginRate / 10000

	req := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: price, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw.RequestCh <- req
	resp := <-gw.ResponseCh
	if !resp.Success {
		t.Fatalf("perp order rejected: %v", resp.Error)
	}

	client := ex.Clients[1]
	if client.PerpReserved["USD"] != initialMargin {
		t.Errorf("PerpReserved USD = %d, want %d", client.PerpReserved["USD"], initialMargin)
	}
	if client.Reserved["USD"] != 0 {
		t.Errorf("Spot Reserved should be 0 for perp order")
	}
}

func TestPerpCloseReleasesMarginAndSettlesPnL(t *testing.T) {
	ex := newPerpTestExchange()
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	// Client 1: long BTC-PERP
	gw1 := ex.ConnectClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100000))
	// Client 2: short (maker providing liquidity for close)
	gw2 := ex.ConnectClient(2, nil, &FixedFee{})
	ex.AddPerpBalance(2, "USD", USDAmount(100000))
	defer ex.Shutdown()

	entryPrice := PriceUSD(50000, DOLLAR_TICK)
	exitPrice := PriceUSD(51000, DOLLAR_TICK)

	// Open: client 1 buys, client 2 sells (limit sell first as maker)
	openSell := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder,
		Price: entryPrice, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw2.RequestCh <- openSell
	<-gw2.ResponseCh // accepted

	openBuy := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 2, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: entryPrice, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw1.RequestCh <- openBuy
	<-gw1.ResponseCh // accepted

	// Drain fill notifications
	<-gw1.ResponseCh // fill
	<-gw2.ResponseCh // fill

	// Now close: client 1 sells at higher price (profit)
	closeSell := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 3, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder,
		Price: exitPrice, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw1.RequestCh <- closeSell
	<-gw1.ResponseCh // accepted

	closeBuy := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 4, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: exitPrice, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw2.RequestCh <- closeBuy
	<-gw2.ResponseCh // accepted
	<-gw1.ResponseCh // fill for sell
	<-gw2.ResponseCh // fill for buy

	// PnL for client 1 (long → closed): (exitPrice - entryPrice) * qty / precision
	expectedPnL := (exitPrice - entryPrice) * BTC_PRECISION / BTC_PRECISION

	client1 := ex.Clients[1]
	startBalance := USDAmount(100000)
	// PerpReserved should be released back to 0
	if client1.PerpReserved["USD"] != 0 {
		t.Errorf("PerpReserved after close = %d, want 0", client1.PerpReserved["USD"])
	}
	// PerpBalances should reflect PnL gain
	gained := client1.PerpBalances["USD"] - startBalance
	if gained != expectedPnL {
		t.Errorf("PnL = %d, want %d", gained, expectedPnL)
	}
}

func TestCrossMarketIsolation(t *testing.T) {
	ex := newPerpTestExchange()
	spot := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(spot)
	ex.AddInstrument(perp)

	gw := ex.ConnectClient(1, map[string]int64{"BTC": 2 * BTC_PRECISION, "USD": USDAmount(200000)}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(50000))
	defer ex.Shutdown()

	// Place spot order — should only affect spot wallet
	spotReq := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTCUSD", Side: Buy, Type: LimitOrder,
		Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw.RequestCh <- spotReq
	resp := <-gw.ResponseCh
	if !resp.Success {
		t.Fatalf("spot order rejected: %v", resp.Error)
	}

	client := ex.Clients[1]
	if client.PerpReserved["USD"] != 0 {
		t.Errorf("Perp wallet affected by spot order: PerpReserved=%d", client.PerpReserved["USD"])
	}
	if client.PerpBalances["USD"] != USDAmount(50000) {
		t.Errorf("Perp balance changed by spot order")
	}
}

func TestTransferSpotToPerp(t *testing.T) {
	ex := newPerpTestExchange()
	gw := ex.ConnectClient(1, map[string]int64{"USD": USDAmount(10000)}, &FixedFee{})
	_ = gw
	defer ex.Shutdown()

	if err := ex.Transfer(1, "spot", "perp", "USD", USDAmount(3000)); err != nil {
		t.Fatalf("transfer failed: %v", err)
	}

	client := ex.Clients[1]
	if client.Balances["USD"] != USDAmount(7000) {
		t.Errorf("Spot USD = %d, want %d", client.Balances["USD"], USDAmount(7000))
	}
	if client.PerpBalances["USD"] != USDAmount(3000) {
		t.Errorf("Perp USD = %d, want %d", client.PerpBalances["USD"], USDAmount(3000))
	}
}

func TestTransferPerpToSpot(t *testing.T) {
	ex := newPerpTestExchange()
	gw := ex.ConnectClient(1, map[string]int64{"USD": USDAmount(5000)}, &FixedFee{})
	_ = gw
	ex.AddPerpBalance(1, "USD", USDAmount(8000))
	defer ex.Shutdown()

	if err := ex.Transfer(1, "perp", "spot", "USD", USDAmount(2000)); err != nil {
		t.Fatalf("transfer failed: %v", err)
	}

	client := ex.Clients[1]
	if client.Balances["USD"] != USDAmount(7000) {
		t.Errorf("Spot USD = %d, want %d", client.Balances["USD"], USDAmount(7000))
	}
	if client.PerpBalances["USD"] != USDAmount(6000) {
		t.Errorf("Perp USD = %d, want %d", client.PerpBalances["USD"], USDAmount(6000))
	}
}

func TestPerpFundingUsesPerpWallet(t *testing.T) {
	pm := NewPositionManager(&RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, BTC_PRECISION, BTC_PRECISION/100)

	clients := map[uint64]*Client{
		1: NewClient(1, &FixedFee{}),
	}
	clients[1].PerpBalances["USD"] = USDAmount(10000)
	clients[1].Balances["USD"] = USDAmount(10000)

	pm.UpdatePosition(1, "BTC-PERP", BTC_PRECISION, PriceUSD(50000, BTC_PRECISION), Buy)
	perp.UpdateFundingRate(PriceUSD(50000, BTC_PRECISION), PriceUSD(50100, BTC_PRECISION))

	spotBefore := clients[1].Balances["USD"]
	pm.SettleFunding(clients, perp, nil)

	// Spot wallet must not change
	if clients[1].Balances["USD"] != spotBefore {
		t.Error("Funding incorrectly debited from spot wallet")
	}
	// Perp wallet must decrease (long pays positive funding)
	if clients[1].PerpBalances["USD"] >= USDAmount(10000) {
		t.Error("Funding not debited from perp wallet")
	}
}

func TestPerpOrderInsufficientPerpBalance(t *testing.T) {
	ex := newPerpTestExchange()
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	// Give lots of SPOT balance but no PERP balance
	gw := ex.ConnectClient(1, map[string]int64{"USD": USDAmount(1000000)}, &FixedFee{})
	defer ex.Shutdown()

	req := Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: 1, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder,
		Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
	}}
	gw.RequestCh <- req
	resp := <-gw.ResponseCh
	if resp.Success {
		t.Error("Perp order should be rejected when perp wallet is empty")
	}
	if resp.Error != RejectInsufficientBalance {
		t.Errorf("Expected RejectInsufficientBalance, got %v", resp.Error)
	}
}

func newPerpTestExchange() *Exchange {
	return NewExchange(10, &RealClock{})
}
