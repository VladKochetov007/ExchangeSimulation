package signals

import (
	"testing"
	"time"
)

func TestHorizonTracker(t *testing.T) {
	horizons := []time.Duration{30 * time.Second, 3 * time.Minute}
	ht := NewHorizonTracker(horizons, 10000)

	symbol := "BTC/USD"
	baseTime := time.Now()
	basePrice := int64(50000 * 10000)

	for i := 0; i < 100; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		price := basePrice + int64(i*100*10000)
		ht.AddPrice(symbol, price, timestamp)
	}

	if !ht.IsReady(symbol, 30*time.Second) {
		t.Error("30s horizon should be ready after 100 samples")
	}

	ret30s := ht.GetReturn(symbol, 30*time.Second)
	if ret30s <= 0 {
		t.Errorf("Expected positive return for increasing prices, got %d", ret30s)
	}
}

func TestHorizonTrackerMultipleSymbols(t *testing.T) {
	horizons := []time.Duration{30 * time.Second}
	ht := NewHorizonTracker(horizons, 10000)

	baseTime := time.Now()

	for i := 0; i < 70; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		ht.AddPrice("BTC/USD", 50000*10000+int64(i*100*10000), timestamp)
		ht.AddPrice("ETH/USD", 3000*10000-int64(i*10*10000), timestamp)
	}

	retBTC := ht.GetReturn("BTC/USD", 30*time.Second)
	retETH := ht.GetReturn("ETH/USD", 30*time.Second)

	if retBTC <= 0 {
		t.Error("BTC should have positive return")
	}

	if retETH >= 0 {
		t.Error("ETH should have negative return")
	}
}

func TestHorizonTrackerNotReady(t *testing.T) {
	horizons := []time.Duration{1 * time.Hour}
	ht := NewHorizonTracker(horizons, 10000)

	baseTime := time.Now()
	for i := 0; i < 10; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Minute)
		ht.AddPrice("BTC/USD", 50000*10000, timestamp)
	}

	if ht.IsReady("BTC/USD", 1*time.Hour) {
		t.Error("1h horizon should not be ready with only 10 minutes of data")
	}

	ret := ht.GetReturn("BTC/USD", 1*time.Hour)
	if ret != 0 {
		t.Errorf("GetReturn should return 0 when not ready, got %d", ret)
	}
}
