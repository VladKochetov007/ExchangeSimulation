package exchange

import (
	"context"
	"sync"
	"testing"
	"time"
)

// completeLogger captures all three new event types
type completeLogger struct {
	mu           sync.Mutex
	fundingRates []FundingRateUpdateEvent
	openInterest []OpenInterestEvent
	feeRevenue   []FeeRevenueEvent
}

func (l *completeLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch eventName {
	case "funding_rate_update":
		if e, ok := event.(FundingRateUpdateEvent); ok {
			l.fundingRates = append(l.fundingRates, e)
		}
	case "open_interest":
		if e, ok := event.(OpenInterestEvent); ok {
			l.openInterest = append(l.openInterest, e)
		}
	case "fee_revenue":
		if e, ok := event.(FeeRevenueEvent); ok {
			l.feeRevenue = append(l.feeRevenue, e)
		}
	}
}

// Test 1: Funding Rate Logging
// Real exchanges log funding rates every time they update (typically with mark prices)
func TestFundingRateLogging(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &completeLogger{}
	ex.SetLogger("_global", logger)
	ex.SetLogger("BTC-PERP", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(perp)

	// Setup clients with liquidity
	client1 := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(1, "USD", 1000000*USD_PRECISION)
	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(2, "USD", 1000000*USD_PRECISION)

	// Add book liquidity for mark price calculation
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

	// Start automation with funding rate updates
	automation := NewExchangeAutomation(ex, AutomationConfig{
		MarkPriceCalc:       NewMidPriceCalculator(),
		IndexProvider:       NewStaticPriceOracle(map[string]int64{"BTC-PERP": PriceUSD(50000, DOLLAR_TICK)}),
		PriceUpdateInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	automation.Start(ctx)

	time.Sleep(250 * time.Millisecond)
	automation.Stop()

	logger.mu.Lock()
	fundingRates := make([]FundingRateUpdateEvent, len(logger.fundingRates))
	copy(fundingRates, logger.fundingRates)
	logger.mu.Unlock()

	// Should have logged funding rates (same frequency as mark prices)
	if len(fundingRates) < 3 {
		t.Fatalf("Expected at least 3 funding rate updates, got %d", len(fundingRates))
	}

	// Check funding rate event structure
	fr := fundingRates[0]
	if fr.Symbol != "BTC-PERP" {
		t.Errorf("Expected symbol BTC-PERP, got %s", fr.Symbol)
	}
	// Timestamp should be set
	if fr.Timestamp == 0 {
		t.Errorf("Expected timestamp to be set, got 0")
	}

	// Real exchange behavior: Funding rates logged with every mark price update
	t.Logf("Logged %d funding rate updates in 250ms", len(fundingRates))
	t.Logf("Funding rate: %d bps, NextFunding: %d", fr.Rate, fr.NextFunding)

	ex.Shutdown()
}

// Test 2: Open Interest Logging
// Real exchanges log open interest after every position change
func TestOpenInterestLogging(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &completeLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(perp)

	client1 := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Trade 1: Open positions (should log open interest twice: once for taker, once for maker)
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 10 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 10 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client1.ResponseCh

	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	oi := make([]OpenInterestEvent, len(logger.openInterest))
	copy(oi, logger.openInterest)
	logger.mu.Unlock()

	// Should have 2 open interest events (one for each position update)
	if len(oi) < 2 {
		t.Fatalf("Expected at least 2 open interest events, got %d", len(oi))
	}

	// First OI should be for first position
	oi1 := oi[0]
	if oi1.Symbol != "BTC-PERP" {
		t.Errorf("Expected symbol BTC-PERP, got %s", oi1.Symbol)
	}
	if oi1.OpenInterest != 10*BTC_PRECISION {
		t.Errorf("Expected OI %d after first position, got %d", 10*BTC_PRECISION, oi1.OpenInterest)
	}

	// Second OI should be total of both positions
	oi2 := oi[1]
	if oi2.OpenInterest != 20*BTC_PRECISION {
		t.Errorf("Expected OI %d after both positions, got %d", 20*BTC_PRECISION, oi2.OpenInterest)
	}

	// Trade 2: Close partial positions
	logger.mu.Lock()
	logger.openInterest = nil
	logger.mu.Unlock()

	req3 := &OrderRequest{RequestID: 3, Side: Buy, Type: LimitOrder, Price: 51000 * USD_PRECISION, Qty: 5 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req3}
	<-client2.ResponseCh

	req4 := &OrderRequest{RequestID: 4, Side: Sell, Type: LimitOrder, Price: 51000 * USD_PRECISION, Qty: 5 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req4}
	<-client1.ResponseCh

	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	oi3 := make([]OpenInterestEvent, len(logger.openInterest))
	copy(oi3, logger.openInterest)
	logger.mu.Unlock()

	// OI should decrease as positions close
	if len(oi3) < 2 {
		t.Fatalf("Expected at least 2 OI events after partial close, got %d", len(oi3))
	}

	// After closing 5 BTC from each side, OI should be 10 BTC (5+5)
	finalOI := oi3[len(oi3)-1]
	if finalOI.OpenInterest != 10*BTC_PRECISION {
		t.Errorf("Expected final OI %d, got %d", 10*BTC_PRECISION, finalOI.OpenInterest)
	}

	// Real exchange behavior: OI logged after every position change
	t.Logf("Open interest tracked correctly: 0 -> 20 -> 10 BTC")

	ex.Shutdown()
}

// Test 3: Fee Revenue Logging (Perp)
// Real exchanges log fee revenue per trade for accounting
func TestFeeRevenueLoggingPerp(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &completeLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(perp)

	// Client with 10 bps maker, 20 bps taker fees
	client1 := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Trade: 1 BTC @ $50,000
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 1 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: 50000 * USD_PRECISION, Qty: 1 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client1.ResponseCh

	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	feeRevenue := make([]FeeRevenueEvent, len(logger.feeRevenue))
	copy(feeRevenue, logger.feeRevenue)
	logger.mu.Unlock()

	// Should have 1 fee revenue event
	if len(feeRevenue) != 1 {
		t.Fatalf("Expected 1 fee revenue event, got %d", len(feeRevenue))
	}

	fee := feeRevenue[0]
	if fee.Symbol != "BTC-PERP" {
		t.Errorf("Expected symbol BTC-PERP, got %s", fee.Symbol)
	}
	if fee.Asset != "USD" {
		t.Errorf("Expected fee asset USD, got %s", fee.Asset)
	}

	// Calculate expected fees:
	// Notional = 1 BTC * $50,000 = $50,000 = 50000 * USD_PRECISION
	// Taker fee (20 bps) = 50000 * 0.002 = 100 USD = 100 * USD_PRECISION
	// Maker fee (10 bps) = 50000 * 0.001 = 50 USD = 50 * USD_PRECISION
	notional := int64(50000 * USD_PRECISION)
	expectedTakerFee := notional * 20 / 10000
	expectedMakerFee := notional * 10 / 10000

	if fee.TakerFee != expectedTakerFee {
		t.Errorf("Expected taker fee %d, got %d", expectedTakerFee, fee.TakerFee)
	}
	if fee.MakerFee != expectedMakerFee {
		t.Errorf("Expected maker fee %d, got %d", expectedMakerFee, fee.MakerFee)
	}

	totalFee := fee.TakerFee + fee.MakerFee
	t.Logf("Fee revenue: Taker=%d, Maker=%d, Total=%d (in USD precision units)", fee.TakerFee, fee.MakerFee, totalFee)

	ex.Shutdown()
}

// Test 4: Fee Revenue Logging (Spot)
// Real exchanges log fee revenue for spot trades too
func TestFeeRevenueLoggingSpot(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &completeLogger{}
	ex.SetLogger("_global", logger)

	spot := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(spot)

	// Client with fees
	client1 := ex.ConnectClient(1, map[string]int64{"BTC": 10 * BTC_PRECISION, "USD": 1000000 * USD_PRECISION}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	client2 := ex.ConnectClient(2, map[string]int64{"BTC": 10 * BTC_PRECISION, "USD": 1000000 * USD_PRECISION}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})

	// Spot buy: 0.5 BTC @ $50,000
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTC_PRECISION / 2, Symbol: "BTCUSD"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: BTC_PRECISION / 2, Symbol: "BTCUSD"}
	client1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client1.ResponseCh

	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	spotFees := make([]FeeRevenueEvent, len(logger.feeRevenue))
	copy(spotFees, logger.feeRevenue)
	logger.mu.Unlock()

	// Should have 1 fee revenue event for spot trade
	if len(spotFees) != 1 {
		t.Fatalf("Expected 1 fee revenue event for spot, got %d", len(spotFees))
	}

	fee := spotFees[0]
	if fee.Symbol != "BTCUSD" {
		t.Errorf("Expected symbol BTCUSD, got %s", fee.Symbol)
	}
	if fee.Asset != "USD" {
		t.Errorf("Expected fee asset USD, got %s", fee.Asset)
	}

	// Fees should be non-zero
	if fee.TakerFee == 0 {
		t.Errorf("Expected non-zero taker fee")
	}
	if fee.MakerFee == 0 {
		t.Errorf("Expected non-zero maker fee")
	}

	t.Logf("Spot fee revenue: Taker=%d, Maker=%d", fee.TakerFee, fee.MakerFee)

	ex.Shutdown()
}

// Test 5: All Events Together (Integration Test)
// Real exchanges log all events simultaneously during normal trading
func TestCompleteLoggingIntegration(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &completeLogger{}
	ex.SetLogger("_global", logger)
	ex.SetLogger("BTC-PERP", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(perp)

	client1 := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 100000*USD_PRECISION)

	client2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 100000*USD_PRECISION)

	// Execute a trade
	req1 := &OrderRequest{RequestID: 1, Side: Sell, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: 5 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-client2.ResponseCh

	req2 := &OrderRequest{RequestID: 2, Side: Buy, Type: LimitOrder, Price: PriceUSD(50000, DOLLAR_TICK), Qty: 5 * BTC_PRECISION, Symbol: "BTC-PERP"}
	client1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-client1.ResponseCh

	time.Sleep(10 * time.Millisecond)

	// Start automation to get funding rates
	automation := NewExchangeAutomation(ex, AutomationConfig{
		MarkPriceCalc:       NewMidPriceCalculator(),
		IndexProvider:       NewStaticPriceOracle(map[string]int64{"BTC-PERP": PriceUSD(50000, DOLLAR_TICK)}),
		PriceUpdateInterval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	automation.Start(ctx)

	time.Sleep(120 * time.Millisecond)
	automation.Stop()

	logger.mu.Lock()
	nOI := len(logger.openInterest)
	nFee := len(logger.feeRevenue)
	nFunding := len(logger.fundingRates)
	logger.mu.Unlock()

	// Verify all three types of events were logged
	if nOI < 2 {
		t.Errorf("Expected at least 2 open interest events, got %d", nOI)
	}
	if nFee != 1 {
		t.Errorf("Expected 1 fee revenue event, got %d", nFee)
	}
	if nFunding < 2 {
		t.Errorf("Expected at least 2 funding rate updates, got %d", nFunding)
	}

	// Summary
	t.Logf("Complete logging verified:")
	t.Logf("  - Open interest events: %d", nOI)
	t.Logf("  - Fee revenue events: %d", nFee)
	t.Logf("  - Funding rate updates: %d", nFunding)
	t.Logf("All critical exchange events are now logged (100%% coverage)")

	ex.Shutdown()
}
