package exchange_test

import (
	. "exchange_sim/exchange"
	"sync"
	"testing"
	"time"
)

// multiLevelLogger captures all events during multi-level execution
type multiLevelLogger struct {
	mu              sync.Mutex
	positionUpdates []PositionUpdateEvent
	realizedPnL     []RealizedPnLEvent
	openInterest    []OpenInterestEvent
	feeRevenue      []FeeRevenueEvent
	balanceChanges  []BalanceChangeEvent
}

func (l *multiLevelLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch eventName {
	case "position_update":
		if e, ok := event.(PositionUpdateEvent); ok {
			l.positionUpdates = append(l.positionUpdates, e)
		}
	case "realized_pnl":
		if e, ok := event.(RealizedPnLEvent); ok {
			l.realizedPnL = append(l.realizedPnL, e)
		}
	case "open_interest":
		if e, ok := event.(OpenInterestEvent); ok {
			l.openInterest = append(l.openInterest, e)
		}
	case "fee_revenue":
		if e, ok := event.(FeeRevenueEvent); ok {
			l.feeRevenue = append(l.feeRevenue, e)
		}
	case "balance_change":
		if e, ok := event.(BalanceChangeEvent); ok {
			l.balanceChanges = append(l.balanceChanges, e)
		}
	}
}

// TestMultiLevelExecution demonstrates what happens when a taker order
// executes against multiple price levels (real exchange behavior)
func TestMultiLevelExecution(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(16, clock)
	logger := &multiLevelLogger{}
	ex.SetLogger("_global", logger)

	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	maker1 := ex.ConnectClient(1, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(1, "USD", 1000000*USD_PRECISION)

	maker2 := ex.ConnectClient(2, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(2, "USD", 1000000*USD_PRECISION)

	maker3 := ex.ConnectClient(3, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(3, "USD", 1000000*USD_PRECISION)

	taker := ex.ConnectClient(4, map[string]int64{}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.AddPerpBalance(4, "USD", 10000000*USD_PRECISION)

	t.Logf("\n=== Building Order Book ===")
	req1 := &OrderRequest{
		RequestID: 1,
		Side:      Sell,
		Type:      LimitOrder,
		Price:     PriceUSD(50000, DOLLAR_TICK),
		Qty:       100 * BTC_PRECISION,
		Symbol:    "BTC-PERP",
	}
	maker1.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req1}
	<-maker1.ResponseCh
	t.Logf("Level 1: 100 BTC @ $50,000 (Maker1)")

	req2 := &OrderRequest{
		RequestID: 2,
		Side:      Sell,
		Type:      LimitOrder,
		Price:     PriceUSD(50100, DOLLAR_TICK),
		Qty:       50 * BTC_PRECISION,
		Symbol:    "BTC-PERP",
	}
	maker2.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req2}
	<-maker2.ResponseCh
	t.Logf("Level 2: 50 BTC @ $50,100 (Maker2)")

	req3 := &OrderRequest{
		RequestID: 3,
		Side:      Sell,
		Type:      LimitOrder,
		Price:     PriceUSD(50200, DOLLAR_TICK),
		Qty:       30 * BTC_PRECISION,
		Symbol:    "BTC-PERP",
	}
	maker3.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: req3}
	<-maker3.ResponseCh
	t.Logf("Level 3: 30 BTC @ $50,200 (Maker3)")

	time.Sleep(10 * time.Millisecond)
	logger.mu.Lock()
	logger.positionUpdates = nil
	logger.openInterest = nil
	logger.feeRevenue = nil
	logger.mu.Unlock()

	t.Logf("\n=== Taker Order: BUY 160 BTC ===")

	reqTaker := &OrderRequest{
		RequestID: 4,
		Side:      Buy,
		Type:      LimitOrder,
		Price:     PriceUSD(50200, DOLLAR_TICK),
		Qty:       160 * BTC_PRECISION,
		Symbol:    "BTC-PERP",
	}
	taker.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: reqTaker}
	<-taker.ResponseCh

	time.Sleep(10 * time.Millisecond)

	logger.mu.Lock()
	posUpdates := make([]PositionUpdateEvent, len(logger.positionUpdates))
	copy(posUpdates, logger.positionUpdates)
	oiEvents := make([]OpenInterestEvent, len(logger.openInterest))
	copy(oiEvents, logger.openInterest)
	nFeeRevenue := len(logger.feeRevenue)
	logger.mu.Unlock()

	t.Logf("\n=== EXECUTION ANALYSIS ===")
	t.Logf("Position updates: %d", len(posUpdates))

	if len(posUpdates) != 6 {
		t.Errorf("Expected 6 position updates, got %d", len(posUpdates))
	}

	positionsByClient := make(map[uint64][]PositionUpdateEvent)
	for _, pu := range posUpdates {
		positionsByClient[pu.ClientID] = append(positionsByClient[pu.ClientID], pu)
	}

	takerUpdates := positionsByClient[4]
	if len(takerUpdates) != 3 {
		t.Errorf("Expected 3 position updates for taker, got %d", len(takerUpdates))
	}
	if takerUpdates[0].OldSize != 0 || takerUpdates[0].NewSize != 100*BTC_PRECISION {
		t.Errorf("Expected 0 -> 100 BTC, got %d -> %d",
			takerUpdates[0].OldSize/BTC_PRECISION, takerUpdates[0].NewSize/BTC_PRECISION)
	}
	if takerUpdates[1].OldSize != 100*BTC_PRECISION || takerUpdates[1].NewSize != 150*BTC_PRECISION {
		t.Errorf("Expected 100 -> 150 BTC, got %d -> %d",
			takerUpdates[1].OldSize/BTC_PRECISION, takerUpdates[1].NewSize/BTC_PRECISION)
	}
	if takerUpdates[2].OldSize != 150*BTC_PRECISION || takerUpdates[2].NewSize != 160*BTC_PRECISION {
		t.Errorf("Expected 150 -> 160 BTC, got %d -> %d",
			takerUpdates[2].OldSize/BTC_PRECISION, takerUpdates[2].NewSize/BTC_PRECISION)
	}

	t.Logf("Open interest updates: %d", len(oiEvents))
	if len(oiEvents) != 6 {
		t.Errorf("Expected 6 OI updates, got %d", len(oiEvents))
	}

	finalOI := oiEvents[len(oiEvents)-1].OpenInterest
	if finalOI != 320*BTC_PRECISION {
		t.Errorf("Expected final OI 320 BTC, got %d BTC", finalOI/BTC_PRECISION)
	}

	t.Logf("Fee revenue events: %d", nFeeRevenue)
	if nFeeRevenue != 3 {
		t.Errorf("Expected 3 fee revenue events, got %d", nFeeRevenue)
	}

	avgPrice := (100*50000 + 50*50100 + 10*50200) / 160
	t.Logf("Weighted average execution price: $%d", avgPrice)

	ex.Shutdown()
}
