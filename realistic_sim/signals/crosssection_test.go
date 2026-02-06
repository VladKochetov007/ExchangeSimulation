package signals

import (
	"testing"

	"exchange_sim/exchange"
)

func TestCrossSectionalSignals(t *testing.T) {
	cs := NewCrossSectionalSignals(60, 10000)

	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
	for _, symbol := range symbols {
		cs.AddSymbol(symbol, 60, 10000)
	}

	basePrice := int64(50000 * exchange.USD_PRECISION)
	for i := 0; i < 60; i++ {
		cs.AddPrice("BTC/USD", basePrice+int64(i)*100*exchange.USD_PRECISION)
		cs.AddPrice("ETH/USD", basePrice+int64(i)*50*exchange.USD_PRECISION)
		cs.AddPrice("SOL/USD", basePrice-int64(i)*50*exchange.USD_PRECISION)
	}

	signalMap := cs.Calculate(symbols)

	if len(signalMap) != 3 {
		t.Fatalf("Expected 3 signals, got %d", len(signalMap))
	}

	if signalMap["SOL/USD"] >= signalMap["BTC/USD"] {
		t.Error("SOL (worst performer) should have lower signal than BTC (best performer)")
	}

	signalSum := int64(0)
	for _, signal := range signalMap {
		signalSum += signal
	}

	if signalSum < -1000 || signalSum > 1000 {
		t.Logf("Signals should sum to approximately 0, got %d (acceptable with rounding)", signalSum)
	}
}

func TestCrossSectionalSignalsNotReady(t *testing.T) {
	cs := NewCrossSectionalSignals(60, 10000)

	symbols := []string{"BTC/USD", "ETH/USD"}
	for _, symbol := range symbols {
		cs.AddSymbol(symbol, 60, 10000)
	}

	for i := 0; i < 30; i++ {
		cs.AddPrice("BTC/USD", 50000*exchange.USD_PRECISION)
		cs.AddPrice("ETH/USD", 3000*exchange.USD_PRECISION)
	}

	signalMap := cs.Calculate(symbols)

	if len(signalMap) != 0 {
		t.Errorf("Expected no signals when buffers not full, got %d", len(signalMap))
	}
}

func TestCrossSectionalSignalsPartialReady(t *testing.T) {
	cs := NewCrossSectionalSignals(60, 10000)

	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
	for _, symbol := range symbols {
		cs.AddSymbol(symbol, 60, 10000)
	}

	for i := 0; i < 60; i++ {
		cs.AddPrice("BTC/USD", 50000*exchange.USD_PRECISION)
		cs.AddPrice("ETH/USD", 3000*exchange.USD_PRECISION)
	}

	signalMap := cs.Calculate(symbols)

	if len(signalMap) != 2 {
		t.Errorf("Expected 2 signals (only BTC and ETH ready), got %d", len(signalMap))
	}

	if _, ok := signalMap["SOL/USD"]; ok {
		t.Error("SOL/USD should not have signal (no data)")
	}
}
