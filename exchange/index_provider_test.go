package exchange

import (
	"testing"
)

func TestFixedIndexProvider(t *testing.T) {
	provider := NewFixedIndexProvider()

	// Set fixed prices
	provider.SetPrice("BTC-PERP", 50000*SATOSHI)
	provider.SetPrice("ETH-PERP", 3000*SATOSHI)

	// Get prices
	btcPrice := provider.GetIndexPrice("BTC-PERP", 0)
	if btcPrice != 50000*SATOSHI {
		t.Errorf("Expected %d, got %d", 50000*SATOSHI, btcPrice)
	}

	ethPrice := provider.GetIndexPrice("ETH-PERP", 0)
	if ethPrice != 3000*SATOSHI {
		t.Errorf("Expected %d, got %d", 3000*SATOSHI, ethPrice)
	}

	// Unknown symbol returns 0
	unknownPrice := provider.GetIndexPrice("DOGE-PERP", 0)
	if unknownPrice != 0 {
		t.Errorf("Expected 0 for unknown symbol, got %d", unknownPrice)
	}
}

func TestSpotIndexProvider(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)

	// Add spot instrument
	spotInst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(spotInst)

	// Add perp instrument
	perpInst := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(perpInst)

	// Create provider and map perp to spot
	provider := NewSpotIndexProvider(ex)
	provider.MapPerpToSpot("BTC-PERP", "BTC/USD")

	spotBook := ex.Books["BTC/USD"]

	// Add orders to spot book
	bidOrder := &Order{
		ID:        1,
		ClientID:  1,
		Price:     49900 * SATOSHI,
		Qty:       SATOSHI,
		Side:      Buy,
		Type:      LimitOrder,
		Timestamp: clock.NowUnixNano(),
	}

	askOrder := &Order{
		ID:        2,
		ClientID:  1,
		Price:     50100 * SATOSHI,
		Qty:       SATOSHI,
		Side:      Sell,
		Type:      LimitOrder,
		Timestamp: clock.NowUnixNano(),
	}

	spotBook.Bids.addOrder(bidOrder)
	spotBook.Asks.addOrder(askOrder)

	// Get index price (should use spot mid price)
	indexPrice := provider.GetIndexPrice("BTC-PERP", clock.NowUnixNano())
	expectedMid := int64((49900*SATOSHI + 50100*SATOSHI) / 2)

	if indexPrice != expectedMid {
		t.Errorf("Expected %d, got %d", expectedMid, indexPrice)
	}
}

func TestSpotIndexProviderUnmappedSymbol(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	provider := NewSpotIndexProvider(ex)

	// Request unmapped symbol
	price := provider.GetIndexPrice("BTC-PERP", 0)
	if price != 0 {
		t.Errorf("Expected 0 for unmapped symbol, got %d", price)
	}
}

func TestSpotIndexProviderMissingSpot(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	provider := NewSpotIndexProvider(ex)

	// Map to non-existent spot
	provider.MapPerpToSpot("BTC-PERP", "BTC/USD")

	// Request price (spot doesn't exist)
	price := provider.GetIndexPrice("BTC-PERP", 0)
	if price != 0 {
		t.Errorf("Expected 0 for missing spot, got %d", price)
	}
}

func TestDynamicIndexProvider(t *testing.T) {
	// Custom calculator that returns timestamp-based price
	calculator := func(symbol string, timestamp int64) int64 {
		if symbol == "BTC-PERP" {
			return 50000*SATOSHI + (timestamp % 1000)
		}
		return 0
	}

	provider := NewDynamicIndexProvider(calculator)

	// Test with different timestamps
	price1 := provider.GetIndexPrice("BTC-PERP", 100)
	expected1 := int64(50000*SATOSHI + 100)
	if price1 != expected1 {
		t.Errorf("Expected %d, got %d", expected1, price1)
	}

	price2 := provider.GetIndexPrice("BTC-PERP", 500)
	expected2 := int64(50000*SATOSHI + 500)
	if price2 != expected2 {
		t.Errorf("Expected %d, got %d", expected2, price2)
	}

	// Unknown symbol
	price3 := provider.GetIndexPrice("ETH-PERP", 100)
	if price3 != 0 {
		t.Errorf("Expected 0 for unknown symbol, got %d", price3)
	}
}
