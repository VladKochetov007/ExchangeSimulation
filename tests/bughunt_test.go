package exchange_test

// Regression tests for bugs found in the July 2026 exchange-logic hunt.

import (
	"testing"
	"time"

	ebook "exchange_sim/book"
	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
)

// forceClose used to allocate its market order ID as `orderID := e.NextOrderID;
// e.NextOrderID++` while PlaceOrder pre-increments, so the liquidation order
// reused the ID of the most recently placed order and fills were attributed to
// an innocent client's resting order.
func TestLiquidationOrderIDUnique(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex, perp := setupPerpAutomation(handler)

	injectPerpPosition(ex, 1, "BTC-PERP", BTCAmount(1.0), entry100(), 0, USDAmount(1))
	bidID, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, markBelow(), BTCAmount(1.0))
	if reject != "" {
		t.Fatalf("liquidity bid rejected: %s", reject)
	}

	ex.CheckLiquidations("BTC-PERP", perp, markBelow())

	if len(handler.liquidations) == 0 {
		t.Fatal("expected liquidation to fire")
	}
	trade := ex.GetBook("BTC-PERP").LastTrade
	if trade == nil {
		t.Fatal("expected liquidation trade")
	}
	if trade.TakerOrderID == bidID {
		t.Fatalf("liquidation order reused resting order ID %d", bidID)
	}
	if trade.MakerOrderID != bidID {
		t.Fatalf("liquidation should fill against bid %d, got maker %d", bidID, trade.MakerOrderID)
	}
}

// settlePositionMargin used to compute openedQty = exec.Qty − closedQty, which
// margins phantom quantity when a hedge-mode reduce overshoots and the position
// clamp discards the excess.
func TestHedgeReduceOvershootNoGhostMargin(t *testing.T) {
	clock := &RealClock{}
	pm := NewPositionManager(clock)
	perp := einstrument.NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)

	// Taker (client 1) holds 0.5 BTC hedge-long; sells 1 BTC on the Long side.
	pm.UpdatePosition(1, "BTC-PERP", BTCAmount(0.5), USDAmount(100), Buy, PositionLong)

	reserves := map[uint64]int64{}
	takerOrder := GetOrder()
	takerOrder.ClientID = 1
	takerOrder.Side = Sell
	takerOrder.PositionSide = PositionLong
	takerOrder.Type = Market
	takerOrder.Qty = BTCAmount(1.0)
	defer PutOrder(takerOrder)

	exec := GetExecution()
	exec.TakerClientID = 1
	exec.MakerClientID = 2
	exec.Qty = BTCAmount(1.0)
	exec.Price = USDAmount(100)
	exec.MakerSide = Buy
	exec.MakerPosSide = PositionBoth
	defer PutExecution(exec)

	ctx := SettlementContext{
		Exec:              exec,
		TakerOrder:        takerOrder,
		MakerPosSide:      PositionBoth,
		Positions:         pm,
		PerpBalance:       func(uint64, string) int64 { return 0 },
		MutatePerpBalance: func(uint64, string, int64) {},
		ReservePerp: func(clientID uint64, _ string, amount int64) bool {
			reserves[clientID] += amount
			return true
		},
		ReleasePerp:      func(uint64, string, int64) {},
		RecordFeeRevenue: func(string, int64, int64) {},
		LogBalanceChange: func(uint64, string, string, []BalanceDelta) {},
		BasePrecision:    BTC_PRECISION,
		Timestamp:        clock.NowUnixNano(),
		BookSymbol:       "BTC-PERP",
	}
	perp.Settle(ctx)

	pos := pm.GetPositionBySide(1, "BTC-PERP", PositionLong)
	if pos != nil && pos.Size != 0 {
		t.Fatalf("hedge long should be flat after clamped reduce, got %d", pos.Size)
	}
	if reserves[1] != 0 {
		t.Fatalf("taker overshoot must not reserve margin (position never opened), reserved %d", reserves[1])
	}
	wantMaker := perp.MarginRequired(BTCAmount(1.0), USDAmount(100), BTC_PRECISION)
	if reserves[2] != wantMaker {
		t.Fatalf("maker margin: want %d, got %d", wantMaker, reserves[2])
	}
}

// placeHedgeOrder submits a limit order with an explicit PositionSide.
func placeHedgeOrder(ex *Exchange, clientID uint64, symbol string, side Side, posSide PositionSide, price, qty int64) (uint64, RejectReason) {
	gateway := ex.Gateways[clientID]
	reqID := uint64(2000000 + ex.Clock.NowUnixNano()%1000000)
	gateway.RequestCh <- Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID:    reqID,
			Side:         side,
			PositionSide: posSide,
			Type:         LimitOrder,
			Price:        price,
			Qty:          qty,
			Symbol:       symbol,
			TimeInForce:  GTC,
		},
	}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case resp := <-gateway.ResponseCh:
			if resp.RequestID != reqID {
				continue
			}
			if !resp.Success {
				return 0, resp.Error
			}
			return resp.Data.(uint64), ""
		case <-timeout:
			return 0, "TIMEOUT"
		}
	}
}

// Hedge-mode reducing orders larger than the held position are rejected at
// placement (venue reduce-only semantics) instead of silently vanishing the
// overshoot at fill time.
func TestHedgeReduceOvershootRejected(t *testing.T) {
	ex, _ := setupPerpAutomation(&mockLiquidationHandler{})
	ex.AddPerpBalance(1, "USD", USDAmount(10_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionLong,
		Size: BTCAmount(1.0), EntryPrice: entry100(),
	})
	pm.Unlock()

	_, reject := placeHedgeOrder(ex, 1, "BTC-PERP", Sell, PositionLong, entry100(), BTCAmount(2.0))
	if reject != RejectExceedsPosition {
		t.Fatalf("want %s, got %q", RejectExceedsPosition, reject)
	}

	// Exact-size reduce is allowed.
	if _, reject := placeHedgeOrder(ex, 1, "BTC-PERP", Sell, PositionLong, entry100(), BTCAmount(1.0)); reject != "" {
		t.Fatalf("exact-size hedge reduce rejected: %s", reject)
	}

	// Opening the short hedge side is unaffected.
	if _, reject := placeHedgeOrder(ex, 1, "BTC-PERP", Sell, PositionShort, entry100(), BTCAmount(2.0)); reject != "" {
		t.Fatalf("hedge short open rejected: %s", reject)
	}

	// Netting-mode flips stay legal.
	if _, reject := placeHedgeOrder(ex, 1, "BTC-PERP", Sell, PositionBoth, entry100(), BTCAmount(2.0)); reject != "" {
		t.Fatalf("netting flip rejected: %s", reject)
	}
}

// CheckLiquidations used to compare full account cash against a single
// symbol's maintenance requirement, so the same dollar backed every symbol at
// once and a globally under-margined account passed each per-symbol check.
func TestCrossSymbolMaintenanceAggregation(t *testing.T) {
	handler := &mockLiquidationHandler{}
	ex := NewExchange(10, &RealClock{})
	btcPerp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ethPerp := NewPerpFutures("ETH-PERP", "ETH", "USD", ETH_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(btcPerp)
	ex.AddInstrument(ethPerp)
	ex.LiquidationHandler = handler

	for _, id := range []uint64{1, 2, 3} {
		ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
	}
	ex.AddPerpBalance(1, "USD", USDAmount(8))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000))
	ex.AddPerpBalance(3, "USD", USDAmount(8))

	// $100 notional on each symbol → maintenance $5 each at the 5% default.
	// Client 1 holds both (account maintenance $10 > $8 equity → breach);
	// client 3 holds only BTC (maintenance $5 < $8 equity → healthy).
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{ClientID: 1, Symbol: "BTC-PERP", Size: BTCAmount(1.0), EntryPrice: entry100()})
	pm.InjectPosition(1, "ETH-PERP", &Position{ClientID: 1, Symbol: "ETH-PERP", Size: ETHAmount(1.0), EntryPrice: entry100()})
	pm.InjectPosition(3, "BTC-PERP", &Position{ClientID: 3, Symbol: "BTC-PERP", Size: BTCAmount(1.0), EntryPrice: entry100()})
	pm.Unlock()
	ethPerp.UpdateFundingRate(entry100(), entry100())

	// Liquidity so client 1's forced close can fill at entry (uPnL stays 0).
	InjectLimitOrder(ex, 2, "BTC-PERP", Buy, entry100(), BTCAmount(2.0))

	ex.CheckLiquidations("BTC-PERP", btcPerp, entry100())

	if len(handler.liquidations) != 1 {
		t.Fatalf("want exactly 1 liquidation (client 1), got %d", len(handler.liquidations))
	}
	if handler.liquidations[0].ClientID != 1 {
		t.Fatalf("liquidated wrong client: %d", handler.liquidations[0].ClientID)
	}
	if pos := pm.GetPositionBySide(1, "BTC-PERP", PositionBoth); pos != nil && pos.Size != 0 {
		t.Fatalf("client 1 BTC position not closed: %d", pos.Size)
	}
	if pos := pm.GetPositionBySide(3, "BTC-PERP", PositionBoth); pos == nil || pos.Size != BTCAmount(1.0) {
		t.Fatal("healthy single-symbol client 3 must not be liquidated")
	}
}

// ResetOrder used to skip PositionSide, so a pool-recycled order could carry a
// stale hedge-mode side into any constructor that forgot to set it.
func TestResetOrderClearsPositionSide(t *testing.T) {
	o := &Order{PositionSide: PositionLong, Side: Sell, Qty: 5}
	ebook.ResetOrder(o)
	if o.PositionSide != PositionBoth {
		t.Fatalf("PositionSide not reset: %v", o.PositionSide)
	}
	if o.Qty != 0 || o.Side != Buy {
		t.Fatal("ResetOrder must zero all fields")
	}
}
