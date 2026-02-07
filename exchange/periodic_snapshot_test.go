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
	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", 8, 2, 1, 100)
	ex.AddInstrument(inst)

	// Add some orders to make the snapshot interesting
	gw := ex.ConnectClient(1, map[string]int64{"USD": 100000000, "BTC": 100000000}, &PercentageFee{})
	
	// Place a buy order
	gw.RequestCh <- Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			Symbol: "BTCUSD",
			Side:   Buy,
			Type:   LimitOrder,
			Price:  5000000, // 50000.00
			Qty:    1000000, // 0.01 BTC
			TimeInForce: GTC,
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
		if bids[0].Price != 5000000 {
			t.Errorf("expected bid price 5000000, got %d", bids[0].Price)
		}
	}
}

// Reuse TestLogger from logging_test.go if available, or redefine here if it's not exported
// Since we are in the same package (exchange), we can use TestLogger from logging_test.go IF it is defined there.
// Let me double check logging_test.go content.
