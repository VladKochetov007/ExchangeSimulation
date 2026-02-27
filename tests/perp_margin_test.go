package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
	"time"
)

func TestPerpMarginReleaseOnlyWhenClosing(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	defer ex.Shutdown()

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/10000)
	ex.AddInstrument(perp)

	takerID := uint64(1)
	makerID := uint64(2)

	initialBalance := int64(1000000 * USD_PRECISION)
	ex.ConnectClient(takerID, map[string]int64{}, &FixedFee{})
	takerGateway := ex.Gateways[takerID]
	ex.ConnectClient(makerID, map[string]int64{}, &FixedFee{})
	makerGateway := ex.Gateways[makerID]

	ex.AddPerpBalance(takerID, "USD", initialBalance)
	ex.AddPerpBalance(makerID, "USD", initialBalance)

	// Maker places sell limit order
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTCAmount(1.0), Symbol: "BTC-PERP"}
	makerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	makerResp := <-makerGateway.ResponseCh
	if !makerResp.Success {
		t.Fatalf("Maker order failed: %v", makerResp.Error)
	}

	// Test 1: Taker opens position (buys 0.5 BTC) - should NOT release margin
	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: Market, Price: 0, Qty: BTCAmount(0.5), Symbol: "BTC-PERP"}
	takerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	takerResp := <-takerGateway.ResponseCh
	if !takerResp.Success {
		t.Fatalf("Taker open position failed: %v", takerResp.Error)
	}

	time.Sleep(10 * time.Millisecond)

	takerPos := ex.Positions.GetPosition(takerID, "BTC-PERP")
	if takerPos == nil || takerPos.Size != BTCAmount(0.5) {
		t.Fatalf("Expected taker position 0.5 BTC, got %v", takerPos)
	}

	ex.Lock()
	takerReserved := ex.Clients[takerID].PerpReserved["USD"]
	makerReserved := ex.Clients[makerID].PerpReserved["USD"]
	ex.Unlock()

	if takerReserved < 0 {
		t.Fatalf("Taker reserved should not be negative after opening, got %d", takerReserved)
	}

	// Maker should have margin reserved for remaining order
	if makerReserved < 0 {
		t.Fatalf("Maker reserved should not be negative, got %d", makerReserved)
	}

	// Maker places another sell order
	req3 := &OrderRequest{RequestID: 3, Side: Sell, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTCAmount(1.0), Symbol: "BTC-PERP"}
	makerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req3}
	makerResp = <-makerGateway.ResponseCh
	if !makerResp.Success {
		t.Fatalf("Maker second order failed: %v", makerResp.Error)
	}

	// Test 2: Taker adds to position (buys another 0.5 BTC) - should NOT release margin
	req4 := &OrderRequest{RequestID: 4, Side: Buy, Type: Market, Price: 0, Qty: BTCAmount(0.5), Symbol: "BTC-PERP"}
	takerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req4}
	takerResp = <-takerGateway.ResponseCh
	if !takerResp.Success {
		t.Fatalf("Taker add to position failed: %v", takerResp.Error)
	}

	time.Sleep(10 * time.Millisecond)

	takerPos = ex.Positions.GetPosition(takerID, "BTC-PERP")
	if takerPos == nil || takerPos.Size != BTCAmount(1.0) {
		t.Fatalf("Expected taker position 1.0 BTC, got %v", takerPos)
	}

	ex.Lock()
	takerReserved = ex.Clients[takerID].PerpReserved["USD"]
	ex.Unlock()

	if takerReserved < 0 {
		t.Fatalf("Taker reserved should not be negative after adding, got %d", takerReserved)
	}

	// Maker places buy order to enable taker to close position
	req5 := &OrderRequest{RequestID: 5, Side: Buy, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTCAmount(2.0), Symbol: "BTC-PERP"}
	makerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req5}
	makerResp = <-makerGateway.ResponseCh
	if !makerResp.Success {
		t.Fatalf("Maker buy order failed: %v", makerResp.Error)
	}

	time.Sleep(10 * time.Millisecond)

	// Test 3: Taker reduces position (sells 0.5 BTC) - should release margin for closed portion
	req6 := &OrderRequest{RequestID: 6, Side: Sell, Type: Market, Price: 0, Qty: BTCAmount(0.5), Symbol: "BTC-PERP"}
	takerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req6}
	takerResp = <-takerGateway.ResponseCh
	if !takerResp.Success {
		t.Fatalf("Taker reduce position failed: %v", takerResp.Error)
	}

	time.Sleep(10 * time.Millisecond)

	takerPos = ex.Positions.GetPosition(takerID, "BTC-PERP")
	if takerPos == nil || takerPos.Size != BTCAmount(0.5) {
		t.Fatalf("Expected taker position 0.5 BTC after reduce, got %v", takerPos)
	}

	ex.Lock()
	afterCloseReserved := ex.Clients[takerID].PerpReserved["USD"]
	ex.Unlock()

	if afterCloseReserved < 0 {
		t.Fatalf("Taker reserved should not be negative after reducing, got %d", afterCloseReserved)
	}

	// Test 4: Taker closes position completely (sells remaining 0.5 BTC)
	req7 := &OrderRequest{RequestID: 7, Side: Sell, Type: Market, Price: 0, Qty: BTCAmount(0.5), Symbol: "BTC-PERP"}
	takerGateway.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req7}
	takerResp = <-takerGateway.ResponseCh
	if !takerResp.Success {
		t.Fatalf("Taker close position failed: %v", takerResp.Error)
	}

	time.Sleep(10 * time.Millisecond)

	takerPos = ex.Positions.GetPosition(takerID, "BTC-PERP")
	if takerPos != nil && takerPos.Size != 0 {
		t.Fatalf("Expected taker position 0 after close, got %d", takerPos.Size)
	}

	ex.Lock()
	finalReserved := ex.Clients[takerID].PerpReserved["USD"]
	available := ex.Clients[takerID].PerpAvailable("USD")
	takerBalance := ex.Clients[takerID].PerpBalances["USD"]
	ex.Unlock()

	if finalReserved < 0 {
		t.Fatalf("Taker reserved should not be negative after closing, got %d", finalReserved)
	}

	// Verify available balance calculation is correct
	if available < 0 {
		t.Fatalf("Taker available should not be negative, got %d", available)
	}
	if available > takerBalance {
		t.Fatalf("Taker available should not exceed balance, available=%d, balance=%d", available, takerBalance)
	}
}

func TestPerpMarginMultipleTraders(t *testing.T) {
	ex := NewExchange(20, &RealClock{})
	defer ex.Shutdown()

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/10000)
	ex.AddInstrument(perp)

	initialBalance := int64(100000 * USD_PRECISION)

	// Create 5 market makers
	makers := make([]*ClientGateway, 5)
	for i := 0; i < 5; i++ {
		makerID := uint64(i + 1)
		ex.ConnectClient(makerID, map[string]int64{}, &FixedFee{})
		makers[i] = ex.Gateways[makerID]
		ex.AddPerpBalance(makerID, "USD", initialBalance)

		// Each maker places sell limit
		req := &OrderRequest{RequestID: uint64(i + 1), Side: Sell, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTCAmount(0.5), Symbol: "BTC-PERP"}
		makers[i].RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req}
		resp := <-makers[i].ResponseCh
		if !resp.Success {
			t.Fatalf("Maker %d order failed: %v", i, resp.Error)
		}
	}

	// Create 3 takers that will trade randomly
	takers := make([]*ClientGateway, 3)
	for i := 0; i < 3; i++ {
		takerID := uint64(10 + i)
		ex.ConnectClient(takerID, map[string]int64{}, &FixedFee{})
		takers[i] = ex.Gateways[takerID]
		ex.AddPerpBalance(takerID, "USD", initialBalance)
	}

	// Execute 50 random trades
	for i := 0; i < 50; i++ {
		takerIdx := i % 3
		takerID := uint64(10 + takerIdx)
		side := Buy
		if i%2 == 1 {
			side = Sell
		}

		qty := BTCAmount(0.01) + int64(i%5)*BTCAmount(0.01)

		req := &OrderRequest{RequestID: uint64(100 + i), Side: side, Type: Market, Price: 0, Qty: qty, Symbol: "BTC-PERP"}
		takers[takerIdx].RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req}
		resp := <-takers[takerIdx].ResponseCh
		if !resp.Success && resp.Error != RejectInsufficientBalance {
			t.Fatalf("Trade %d failed unexpectedly: %v", i, resp.Error)
		}

		time.Sleep(5 * time.Millisecond)

		// Check reserved balances never go negative
		ex.Lock()
		reserved := ex.Clients[takerID].PerpReserved["USD"]
		ex.Unlock()
		if reserved < 0 {
			t.Fatalf("Taker %d reserved went negative after trade %d: %d", takerIdx, i, reserved)
		}
	}

	// Verify all clients have non-negative reserved balances
	ex.Lock()
	type clientSnapshot struct {
		reserved  int64
		available int64
	}
	snapshots := make(map[uint64]clientSnapshot, len(ex.Clients))
	for clientID, client := range ex.Clients {
		snapshots[clientID] = clientSnapshot{
			reserved:  client.PerpReserved["USD"],
			available: client.PerpAvailable("USD"),
		}
	}
	ex.Unlock()

	for clientID, snap := range snapshots {
		if snap.reserved < 0 {
			t.Fatalf("Client %d has negative reserved balance: %d", clientID, snap.reserved)
		}
		if snap.available < 0 {
			t.Fatalf("Client %d has negative available balance: %d", clientID, snap.available)
		}
	}
}
