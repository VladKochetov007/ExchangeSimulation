package exchange

import (
	"encoding/json"
	"testing"
	"time"
)

type TestLogger struct {
	events []map[string]any
}

func (t *TestLogger) LogEvent(simTime int64, clientID uint64, eventName string, event any) {
	entry := map[string]any{
		"sim_time":  simTime,
		"client_id": clientID,
		"event":     eventName,
	}
	if event != nil {
		eventBytes, _ := json.Marshal(event)
		var eventFields map[string]any
		json.Unmarshal(eventBytes, &eventFields)
		for k, v := range eventFields {
			entry[k] = v
		}
	}
	t.events = append(t.events, entry)
}

func TestPeriodicSnapshots(t *testing.T) {
	// Setup exchange with a logger
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	
	// Create a test logger
	logger := &TestLogger{
		events: make([]map[string]any, 0),
	}
	ex.Loggers["BTCUSD"] = logger

	// Add instrument
	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(inst)

	// Add some orders to make the snapshot interesting
	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 1000000 * USD_PRECISION,
	}
	gw := ex.ConnectClient(1, balances, &PercentageFee{})
	
	// Place a buy order
	gw.RequestCh <- Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			Symbol:      "BTCUSD",
			Side:        Buy,
			Type:        LimitOrder,
			Price:       PriceUSD(50000, DOLLAR_TICK),
			Qty:         BTCAmount(1),
			TimeInForce: GTC,
			Visibility:  Normal,
		},
	}
	<-gw.ResponseCh
	
	// Enable periodic snapshots with a short interval
	interval := 100 * time.Millisecond
	ex.EnablePeriodicSnapshots(interval)
	
	// Wait for a few intervals
	time.Sleep(350 * time.Millisecond)
	
	ex.Shutdown()
	
	// Count snapshots
	snapshotCount := 0
	for _, event := range logger.events {
		if event["event"] == "BookSnapshot" {
			snapshotCount++
		}
	}
	
	// Should have at least 3 snapshots (one initial maybe? no, subscribe does initial, periodic loop does 3 in 350ms for 100ms interval)
	// We didn't subscribe, so no initial snapshot from subscription.
	// So we expect ~3 snapshots from the loop.
	if snapshotCount < 3 {
		t.Errorf("expected at least 3 snapshots, got %d", snapshotCount)
	}

	// Verify snapshot content
	var lastSnapshot map[string]any
	for _, event := range logger.events {
		if event["event"] == "BookSnapshot" {
			lastSnapshot = event
		}
	}
	
	if lastSnapshot == nil {
		t.Fatal("no snapshot found")
	}

	// Helper to extract PriceLevel from map
	getPriceLevels := func(data any) []PriceLevel {
		var levels []PriceLevel
		bytes, _ := json.Marshal(data)
		json.Unmarshal(bytes, &levels)
		return levels
	}
	
	bids := getPriceLevels(lastSnapshot["bids"])
	if len(bids) != 1 {
		t.Errorf("expected 1 bid level, got %d", len(bids))
	} else {
		expectedPrice := PriceUSD(50000, DOLLAR_TICK)
		if bids[0].Price != expectedPrice {
			t.Errorf("expected bid price %d, got %d", expectedPrice, bids[0].Price)
		}
	}
}

// Reuse TestLogger from logging_test.go if available, or redefine here if it's not exported
// Since we are in the same package (exchange), we can use TestLogger from logging_test.go IF it is defined there.
// Let me double check logging_test.go content.
