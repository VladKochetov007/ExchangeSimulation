package exchange

import (
	"context"
	"testing"
	"time"
)

type positionPnLLogger struct {
	positionUpdates []PositionUpdateEvent
	realizedPnL     []RealizedPnLEvent
	markPrices      []MarkPriceUpdateEvent
}

func (l *positionPnLLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	switch eventName {
	case "position_update":
		if e, ok := event.(PositionUpdateEvent); ok {
			l.positionUpdates = append(l.positionUpdates, e)
		}
	case "realized_pnl":
		if e, ok := event.(RealizedPnLEvent); ok {
			l.realizedPnL = append(l.realizedPnL, e)
		}
	case "mark_price_update":
		if e, ok := event.(MarkPriceUpdateEvent); ok {
			l.markPrices = append(l.markPrices, e)
		}
	}
}

// Edge Case 1: Simple position open (no PnL)
func TestPositionUpdateOpenPosition(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	maker := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	taker := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Maker posts sell order
	req1 := &OrderRequest{
		RequestID: 1,
		Side:      Sell,
		Type:      LimitOrder,
		Price:     50000 * USD_PRECISION,
		Qty:       1 * BTC_PRECISION,
		Symbol:    "BTC-PERP",
	}
	maker.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-maker.ResponseCh

	// Taker buys (opens long position)
	req2 := &OrderRequest{
		RequestID: 2,
		Side:      Buy,
		Type:      LimitOrder,
		Price:     50000 * BTC_PRECISION,
		Qty:       1 * BTC_PRECISION,
		Symbol:    "BTC-PERP",
	}
	taker.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-taker.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Should have 2 position updates (taker and maker)
	if len(logger.positionUpdates) != 2 {
		t.Fatalf("Expected 2 position updates, got %d", len(logger.positionUpdates))
	}

	// Should have NO realized PnL (opening positions)
	if len(logger.realizedPnL) != 0 {
		t.Errorf("Expected 0 realized PnL events for opening positions, got %d", len(logger.realizedPnL))
	}

	// Check taker position update
	var takerUpdate *PositionUpdateEvent
	for i := range logger.positionUpdates {
		if logger.positionUpdates[i].ClientID == 2 {
			takerUpdate = &logger.positionUpdates[i]
			break
		}
	}
	if takerUpdate == nil {
		t.Fatal("Taker position update not found")
	}

	if takerUpdate.OldSize != 0 {
		t.Errorf("Expected old size 0, got %d", takerUpdate.OldSize)
	}
	if takerUpdate.NewSize != 1*BTC_PRECISION {
		t.Errorf("Expected new size %d, got %d", 1*BTC_PRECISION, takerUpdate.NewSize)
	}
	if takerUpdate.NewEntryPrice != 50000*USD_PRECISION {
		t.Errorf("Expected entry price %d, got %d", 50000*USD_PRECISION, takerUpdate.NewEntryPrice)
	}
	if takerUpdate.Reason != "trade" {
		t.Errorf("Expected reason 'trade', got '%s'", takerUpdate.Reason)
	}

	ex.Shutdown()
}

// Edge Case 2: Partial close with realized PnL
func TestPositionUpdatePartialClose(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	client := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Open long 100 BTC @ $50k
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)
	logger.positionUpdates = nil
	logger.realizedPnL = nil

	// Close 30 BTC @ $51k (profit)
	req3 := &OrderRequest{RequestID: 3, Side: Buy, Type: LimitOrder, Price: 51000 * USD_PRECISION, Qty: 30 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req3}
	<-client2.ResponseCh

	req4 := &OrderRequest{RequestID: 4, Side: Sell, Type: LimitOrder, Price: 51000 * USD_PRECISION, Qty: 30 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req4}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Should have 2 position updates
	if len(logger.positionUpdates) != 2 {
		t.Fatalf("Expected 2 position updates, got %d", len(logger.positionUpdates))
	}

	// Should have 2 realized PnL events (both sides closing 30 BTC)
	// Client1: long 100 → 70 (profit), Client2: short -100 → -70 (loss)
	if len(logger.realizedPnL) != 2 {
		t.Fatalf("Expected 2 realized PnL events (both sides closing), got %d", len(logger.realizedPnL))
	}

	// Find client1's PnL (profit)
	var client1PnL, client2PnL *RealizedPnLEvent
	for i := range logger.realizedPnL {
		if logger.realizedPnL[i].ClientID == 1 {
			client1PnL = &logger.realizedPnL[i]
		} else if logger.realizedPnL[i].ClientID == 2 {
			client2PnL = &logger.realizedPnL[i]
		}
	}

	if client1PnL == nil {
		t.Fatal("Client1 PnL not found")
	}
	if client2PnL == nil {
		t.Fatal("Client2 PnL not found")
	}

	// Check client1 (long profit)
	if client1PnL.ClosedQty != 30*BTC_PRECISION {
		t.Errorf("Expected client1 closed qty %d, got %d", 30*BTC_PRECISION, client1PnL.ClosedQty)
	}
	if client1PnL.EntryPrice != 50000*USD_PRECISION {
		t.Errorf("Expected client1 entry price %d, got %d", 50000*BTC_PRECISION, client1PnL.EntryPrice)
	}
	if client1PnL.ExitPrice != 51000*USD_PRECISION {
		t.Errorf("Expected client1 exit price %d, got %d", 51000*BTC_PRECISION, client1PnL.ExitPrice)
	}
	expectedPnL1 := int64(30 * 1000 * USD_PRECISION)
	if client1PnL.PnL != expectedPnL1 {
		t.Errorf("Expected client1 PnL %d, got %d", expectedPnL1, client1PnL.PnL)
	}

	// Check client2 (short loss)
	if client2PnL.ClosedQty != 30*BTC_PRECISION {
		t.Errorf("Expected client2 closed qty %d, got %d", 30*BTC_PRECISION, client2PnL.ClosedQty)
	}
	expectedPnL2 := int64(-30 * 1000 * USD_PRECISION)
	if client2PnL.PnL != expectedPnL2 {
		t.Errorf("Expected client2 PnL %d, got %d", expectedPnL2, client2PnL.PnL)
	}

	// Check position reduced from 100 to 70
	var posUpdate *PositionUpdateEvent
	for i := range logger.positionUpdates {
		if logger.positionUpdates[i].ClientID == 1 {
			posUpdate = &logger.positionUpdates[i]
			break
		}
	}
	if posUpdate == nil {
		t.Fatal("Position update not found")
	}

	if posUpdate.OldSize != 100*BTC_PRECISION {
		t.Errorf("Expected old size %d, got %d", 100*BTC_PRECISION, posUpdate.OldSize)
	}
	if posUpdate.NewSize != 70*BTC_PRECISION {
		t.Errorf("Expected new size %d, got %d", 70*BTC_PRECISION, posUpdate.NewSize)
	}

	ex.Shutdown()
}

// Edge Case 3: Position flip (long to short)
func TestPositionUpdateFlipLongToShort(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	client := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 200000*USD_PRECISION)

	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 200000*USD_PRECISION)

	// Open long 100 BTC @ $50k
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)
	logger.positionUpdates = nil
	logger.realizedPnL = nil

	// Sell 150 BTC @ $51k (close 100 + open 50 short)
	req3 := &OrderRequest{RequestID: 3, Side: Buy, Type: LimitOrder, Price: 51000 * USD_PRECISION, Qty: 150 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req3}
	<-client2.ResponseCh

	req4 := &OrderRequest{RequestID: 4, Side: Sell, Type: LimitOrder, Price: 51000 * USD_PRECISION, Qty: 150 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req4}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Should have position update
	if len(logger.positionUpdates) < 1 {
		t.Fatalf("Expected at least 1 position update, got %d", len(logger.positionUpdates))
	}

	// Should have 2 realized PnL events (both sides closing positions)
	// Client1: long 100 → short -50 (closes 100, profit), Client2: short -100 → long 50 (closes 100, loss)
	if len(logger.realizedPnL) != 2 {
		t.Fatalf("Expected 2 realized PnL events (both sides closing), got %d", len(logger.realizedPnL))
	}

	// Find client1's PnL (profit)
	var client1PnL, client2PnL *RealizedPnLEvent
	for i := range logger.realizedPnL {
		if logger.realizedPnL[i].ClientID == 1 {
			client1PnL = &logger.realizedPnL[i]
		} else if logger.realizedPnL[i].ClientID == 2 {
			client2PnL = &logger.realizedPnL[i]
		}
	}

	if client1PnL == nil {
		t.Fatal("Client1 PnL not found")
	}
	if client2PnL == nil {
		t.Fatal("Client2 PnL not found")
	}

	// Check client1 (closes 100 long, profit)
	if client1PnL.ClosedQty != 100*BTC_PRECISION {
		t.Errorf("Expected client1 closed qty %d (only closing portion), got %d", 100*BTC_PRECISION, client1PnL.ClosedQty)
	}
	expectedPnL1 := int64(100 * 1000 * USD_PRECISION)
	if client1PnL.PnL != expectedPnL1 {
		t.Errorf("Expected client1 PnL %d, got %d", expectedPnL1, client1PnL.PnL)
	}

	// Check client2 (closes 100 short, loss)
	if client2PnL.ClosedQty != 100*BTC_PRECISION {
		t.Errorf("Expected client2 closed qty %d, got %d", 100*BTC_PRECISION, client2PnL.ClosedQty)
	}
	expectedPnL2 := int64(-100 * 1000 * USD_PRECISION)
	if client2PnL.PnL != expectedPnL2 {
		t.Errorf("Expected client2 PnL %d, got %d", expectedPnL2, client2PnL.PnL)
	}

	// Check position flipped from +100 to -50
	var posUpdate *PositionUpdateEvent
	for i := range logger.positionUpdates {
		if logger.positionUpdates[i].ClientID == 1 {
			posUpdate = &logger.positionUpdates[i]
			break
		}
	}
	if posUpdate == nil {
		t.Fatal("Position update not found")
	}

	if posUpdate.OldSize != 100*BTC_PRECISION {
		t.Errorf("Expected old size %d, got %d", 100*BTC_PRECISION, posUpdate.OldSize)
	}
	if posUpdate.NewSize != -50*BTC_PRECISION {
		t.Errorf("Expected new size %d, got %d", -50*BTC_PRECISION, posUpdate.NewSize)
	}

	ex.Shutdown()
}

// Edge Case 4: Complete position close (size → 0)
func TestPositionUpdateCompleteClose(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	client := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Open long 100 BTC @ $50k
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)
	logger.positionUpdates = nil
	logger.realizedPnL = nil

	// Close entire position @ $49k (loss)
	req3 := &OrderRequest{RequestID: 3, Side: Buy, Type: LimitOrder, Price: 49000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req3}
	<-client2.ResponseCh

	req4 := &OrderRequest{RequestID: 4, Side: Sell, Type: LimitOrder, Price: 49000 * USD_PRECISION, Qty: 100 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req4}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Should have 2 realized PnL events (both sides closing entire positions)
	// Client1: long 100 → 0 (loss), Client2: short -100 → 0 (profit)
	if len(logger.realizedPnL) != 2 {
		t.Fatalf("Expected 2 realized PnL events (both sides closing), got %d", len(logger.realizedPnL))
	}

	// Find client1's PnL (loss)
	var client1PnL, client2PnL *RealizedPnLEvent
	for i := range logger.realizedPnL {
		if logger.realizedPnL[i].ClientID == 1 {
			client1PnL = &logger.realizedPnL[i]
		} else if logger.realizedPnL[i].ClientID == 2 {
			client2PnL = &logger.realizedPnL[i]
		}
	}

	if client1PnL == nil {
		t.Fatal("Client1 PnL not found")
	}
	if client2PnL == nil {
		t.Fatal("Client2 PnL not found")
	}

	// Check client1 (closes 100 long, loss)
	if client1PnL.ClosedQty != 100*BTC_PRECISION {
		t.Errorf("Expected client1 closed qty %d, got %d", 100*BTC_PRECISION, client1PnL.ClosedQty)
	}
	expectedPnL1 := int64(-100 * 1000 * USD_PRECISION)
	if client1PnL.PnL != expectedPnL1 {
		t.Errorf("Expected client1 PnL %d, got %d", expectedPnL1, client1PnL.PnL)
	}

	// Check client2 (closes 100 short, profit)
	if client2PnL.ClosedQty != 100*BTC_PRECISION {
		t.Errorf("Expected client2 closed qty %d, got %d", 100*BTC_PRECISION, client2PnL.ClosedQty)
	}
	expectedPnL2 := int64(100 * 1000 * USD_PRECISION)
	if client2PnL.PnL != expectedPnL2 {
		t.Errorf("Expected client2 PnL %d, got %d", expectedPnL2, client2PnL.PnL)
	}

	// Check position closed (NewSize = 0)
	var posUpdate *PositionUpdateEvent
	for i := range logger.positionUpdates {
		if logger.positionUpdates[i].ClientID == 1 {
			posUpdate = &logger.positionUpdates[i]
			break
		}
	}
	if posUpdate == nil {
		t.Fatal("Position update not found")
	}

	if posUpdate.NewSize != 0 {
		t.Errorf("Expected new size 0, got %d", posUpdate.NewSize)
	}

	ex.Shutdown()
}

// Edge Case 5: Mark price logging
func TestMarkPriceLogging(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	// Setup clients with liquidity
	client1 := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(1, "USD", 1000000*USD_PRECISION)
	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(2, "USD", 1000000*USD_PRECISION)

	// Add book liquidity (MidPriceCalculator needs bid/ask to calculate mid price)
	// Prices must be aligned to DOLLAR_TICK (BTC_PRECISION)
	req1 := &OrderRequest{RequestID: 1, Side: Buy, Type: LimitOrder, Price: PriceUSD(49900, DOLLAR_TICK), Qty: 10 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	resp1 := <-client1.ResponseCh
	if !resp1.Success {
		t.Fatalf("Order 1 rejected: %v", resp1.Error)
	}

	req2 := &OrderRequest{RequestID: 2, Side: Sell, Type: LimitOrder, Price: PriceUSD(50100, DOLLAR_TICK), Qty: 10 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	resp2 := <-client2.ResponseCh
	if !resp2.Success {
		t.Fatalf("Order 2 rejected: %v", resp2.Error)
	}

	time.Sleep(10 * time.Millisecond)

	// Verify book has orders before starting automation
	book := ex.Books["BTC-PERP"]
	if book.GetBestBid() == 0 || book.GetBestAsk() == 0 {
		t.Fatal("Book should have bid/ask before starting automation")
	}

	// Start automation with mark price updates
	automation := NewExchangeAutomation(ex, AutomationConfig{
		MarkPriceCalc:       NewMidPriceCalculator(),
		IndexProvider:       &FixedIndexProvider{prices: map[string]int64{"BTC-PERP": PriceUSD(50000, DOLLAR_TICK)}},
		PriceUpdateInterval: 50 * time.Millisecond, // Faster for testing
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	automation.Start(ctx)

	// Wait long enough for several ticks (50ms interval, wait 300ms = 6 ticks)
	time.Sleep(300 * time.Millisecond)
	automation.Stop()

	// Should have logged mark prices (at least 3 updates in 300ms with 50ms interval)
	if len(logger.markPrices) < 3 {
		t.Fatalf("Expected at least 3 mark price updates, got %d", len(logger.markPrices))
	}

	// Check mark price event structure
	mp := logger.markPrices[0]
	if mp.Symbol != "BTC-PERP" {
		t.Errorf("Expected symbol BTC-PERP, got %s", mp.Symbol)
	}
	if mp.IndexPrice != PriceUSD(50000, DOLLAR_TICK) {
		t.Errorf("Expected index price %d, got %d", PriceUSD(50000, DOLLAR_TICK), mp.IndexPrice)
	}
	// Mark price should be mid price: (PriceUSD(49900) + PriceUSD(50100)) / 2
	expectedMarkPrice := (PriceUSD(49900, DOLLAR_TICK) + PriceUSD(50100, DOLLAR_TICK)) / 2
	if mp.MarkPrice != expectedMarkPrice {
		t.Errorf("Expected mark price %d, got %d", expectedMarkPrice, mp.MarkPrice)
	}

	ex.Shutdown()
}

// Edge Case 6: Zero-size trade (IOC no fill) - should not log position
func TestPositionUpdateZeroSizeTrade(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	client := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	// IOC order with no matching orders (should not fill)
	req := &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       50000 * BTC_PRECISION,
		Qty:         1 * BTC_PRECISION,
		Symbol:      "BTC-PERP",
		TimeInForce: IOC,
	}
	client.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req}
	<-client.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Should have NO position updates (no fill)
	if len(logger.positionUpdates) != 0 {
		t.Errorf("Expected 0 position updates for unfilled IOC, got %d", len(logger.positionUpdates))
	}

	ex.Shutdown()
}

// Edge Case 7: Very small position (1 satoshi)
func TestPositionUpdateMinimumSize(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &positionPnLLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	maker := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	taker := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Trade 1 satoshi
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 1, Symbol: "BTC-PERP"}
	maker.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-maker.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 1, Symbol: "BTC-PERP"}
	taker.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-taker.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Should still log position updates
	if len(logger.positionUpdates) != 2 {
		t.Fatalf("Expected 2 position updates for 1 satoshi trade, got %d", len(logger.positionUpdates))
	}

	// Check trade qty
	var takerUpdate *PositionUpdateEvent
	for i := range logger.positionUpdates {
		if logger.positionUpdates[i].ClientID == 2 {
			takerUpdate = &logger.positionUpdates[i]
			break
		}
	}
	if takerUpdate == nil {
		t.Fatal("Taker position update not found")
	}

	if takerUpdate.TradeQty != 1 {
		t.Errorf("Expected trade qty 1, got %d", takerUpdate.TradeQty)
	}

	ex.Shutdown()
}
