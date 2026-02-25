package exchange

import (
	"testing"
)

func TestLastPriceCalculator(t *testing.T) {
	calc := NewLastPriceCalculator()

	// Create order book with last trade
	book := &OrderBook{
		Symbol: "BTC/USD",
		Bids:   newBook(Buy),
		Asks:   newBook(Sell),
		LastTrade: &Trade{
			TradeID: 1,
			Price:   50000 * SATOSHI,
			Qty:     SATOSHI,
		},
	}

	price := calc.Calculate(book)
	if price != 50000*SATOSHI {
		t.Errorf("Expected %d, got %d", 50000*SATOSHI, price)
	}
}

func TestLastPriceCalculatorNoTrade(t *testing.T) {
	calc := NewLastPriceCalculator()

	// Order book with no trades
	book := &OrderBook{
		Symbol:    "BTC/USD",
		Bids:      newBook(Buy),
		Asks:      newBook(Sell),
		LastTrade: nil,
	}

	price := calc.Calculate(book)
	if price != 0 {
		t.Errorf("Expected 0, got %d", price)
	}
}

func TestMidPriceCalculator(t *testing.T) {
	calc := NewMidPriceCalculator()
	clock := &RealClock{}

	// Create exchange and instrument
	ex := NewExchange(10, clock)
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(inst)

	book := ex.Books["BTC/USD"]

	// Add orders to create bid/ask
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

	book.Bids.addOrder(bidOrder)
	book.Asks.addOrder(askOrder)

	// Mid price should be average
	price := calc.Calculate(book)
	expected := int64((49900*SATOSHI + 50100*SATOSHI) / 2)
	if price != expected {
		t.Errorf("Expected %d, got %d", expected, price)
	}
}

func TestMidPriceCalculatorEmptyBook(t *testing.T) {
	calc := NewMidPriceCalculator()

	// Empty order book with last trade
	book := &OrderBook{
		Symbol: "BTC/USD",
		Bids:   newBook(Buy),
		Asks:   newBook(Sell),
		LastTrade: &Trade{
			Price: 50000 * SATOSHI,
		},
	}

	// Should fallback to last trade
	price := calc.Calculate(book)
	if price != 50000*SATOSHI {
		t.Errorf("Expected %d (fallback to last), got %d", 50000*SATOSHI, price)
	}
}

func TestWeightedMidPriceCalculator(t *testing.T) {
	calc := NewWeightedMidPriceCalculator()
	clock := &RealClock{}

	// Create exchange and instrument
	ex := NewExchange(10, clock)
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(inst)

	book := ex.Books["BTC/USD"]

	// Add orders with different quantities
	bidOrder := &Order{
		ID:        1,
		ClientID:  1,
		Price:     49900 * SATOSHI,
		Qty:       2 * SATOSHI, // More qty on bid
		Side:      Buy,
		Type:      LimitOrder,
		Timestamp: clock.NowUnixNano(),
	}

	askOrder := &Order{
		ID:        2,
		ClientID:  1,
		Price:     50100 * SATOSHI,
		Qty:       SATOSHI, // Less qty on ask
		Side:      Sell,
		Type:      LimitOrder,
		Timestamp: clock.NowUnixNano(),
	}

	book.Bids.addOrder(bidOrder)
	book.Asks.addOrder(askOrder)

	// Weighted mid should favor bid side (more liquidity)
	price := calc.Calculate(book)

	bidQty := int64(2 * SATOSHI)
	askQty := int64(1 * SATOSHI)
	bidPrice := int64(49900 * SATOSHI)
	askPrice := int64(50100 * SATOSHI)
	expected := (bidPrice*askQty + askPrice*bidQty) / (bidQty + askQty)

	if price != expected {
		t.Errorf("Expected %d, got %d", expected, price)
	}
}

func TestOrderBookGetters(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100)
	ex.AddInstrument(inst)

	book := ex.Books["BTC/USD"]

	// Initially empty
	if book.GetBestBid() != 0 {
		t.Error("Expected 0 for empty bids")
	}
	if book.GetBestAsk() != 0 {
		t.Error("Expected 0 for empty asks")
	}
	if book.GetLastPrice() != 0 {
		t.Error("Expected 0 for no trades")
	}
	if book.GetMidPrice() != 0 {
		t.Error("Expected 0 for empty book")
	}

	// Add orders
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

	book.Bids.addOrder(bidOrder)
	book.Asks.addOrder(askOrder)

	// Check getters
	if book.GetBestBid() != 49900*SATOSHI {
		t.Errorf("Expected %d, got %d", 49900*SATOSHI, book.GetBestBid())
	}
	if book.GetBestAsk() != 50100*SATOSHI {
		t.Errorf("Expected %d, got %d", 50100*SATOSHI, book.GetBestAsk())
	}

	expectedMid := int64((49900*SATOSHI + 50100*SATOSHI) / 2)
	if book.GetMidPrice() != expectedMid {
		t.Errorf("Expected %d, got %d", expectedMid, book.GetMidPrice())
	}

	// Add last trade
	book.LastTrade = &Trade{
		Price: 50000 * SATOSHI,
	}

	if book.GetLastPrice() != 50000*SATOSHI {
		t.Errorf("Expected %d, got %d", 50000*SATOSHI, book.GetLastPrice())
	}
}
