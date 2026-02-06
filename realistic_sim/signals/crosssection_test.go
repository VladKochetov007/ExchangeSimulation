package signals

import (
	"testing"
	"time"
)

func TestCrossSectionalSignals(t *testing.T) {
	horizons := []time.Duration{30 * time.Second}
	ht := NewHorizonTracker(horizons, 10000)
	cs := NewCrossSectionalSignals(ht)

	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
	baseTime := time.Now()

	for i := 0; i < 70; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		ht.AddPrice("BTC/USD", 50000*10000+int64(i*200*10000), timestamp)
		ht.AddPrice("ETH/USD", 3000*10000+int64(i*50*10000), timestamp)
		ht.AddPrice("SOL/USD", 100*10000-int64(i*2*10000), timestamp)
	}

	signals := cs.Calculate(symbols, 30*time.Second)

	if len(signals) != 3 {
		t.Fatalf("Expected 3 signals, got %d", len(signals))
	}

	if signals["SOL/USD"] >= 0 {
		t.Error("SOL (worst performer) should have negative signal (mean reversion)")
	}

	if signals["BTC/USD"] <= 0 {
		t.Error("BTC (best performer) should have positive signal")
	}

	signalSum := int64(0)
	for _, signal := range signals {
		signalSum += signal
	}

	if signalSum < -1000 || signalSum > 1000 {
		t.Errorf("Signals should sum to approximately 0 (neutrality), got %d", signalSum)
	}
}

func TestCrossSectionalSignalsNotReady(t *testing.T) {
	horizons := []time.Duration{1 * time.Hour}
	ht := NewHorizonTracker(horizons, 10000)
	cs := NewCrossSectionalSignals(ht)

	symbols := []string{"BTC/USD", "ETH/USD"}
	baseTime := time.Now()

	for i := 0; i < 10; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Minute)
		ht.AddPrice("BTC/USD", 50000*10000, timestamp)
		ht.AddPrice("ETH/USD", 3000*10000, timestamp)
	}

	signals := cs.Calculate(symbols, 1*time.Hour)

	if len(signals) != 0 {
		t.Errorf("Expected no signals when data not ready, got %d", len(signals))
	}
}

func TestCrossSectionalSignalsPartialReady(t *testing.T) {
	horizons := []time.Duration{30 * time.Second}
	ht := NewHorizonTracker(horizons, 10000)
	cs := NewCrossSectionalSignals(ht)

	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
	baseTime := time.Now()

	for i := 0; i < 70; i++ {
		timestamp := baseTime.Add(time.Duration(i) * time.Second)
		ht.AddPrice("BTC/USD", 50000*10000, timestamp)
		ht.AddPrice("ETH/USD", 3000*10000, timestamp)
	}

	signals := cs.Calculate(symbols, 30*time.Second)

	if len(signals) != 2 {
		t.Errorf("Expected 2 signals (only BTC and ETH ready), got %d", len(signals))
	}

	if _, ok := signals["SOL/USD"]; ok {
		t.Error("SOL/USD should not have signal (no data)")
	}
}
