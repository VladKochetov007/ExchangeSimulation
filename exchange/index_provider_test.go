package exchange

import "testing"

func TestStaticPriceOracle(t *testing.T) {
	oracle := NewStaticPriceOracle(map[string]int64{
		"BTC-PERP": 50000 * SATOSHI,
		"ETH-PERP": 3000 * SATOSHI,
	})

	if p := oracle.GetPrice("BTC-PERP"); p != 50000*SATOSHI {
		t.Errorf("expected %d, got %d", 50000*SATOSHI, p)
	}
	if p := oracle.GetPrice("ETH-PERP"); p != 3000*SATOSHI {
		t.Errorf("expected %d, got %d", 3000*SATOSHI, p)
	}
	if p := oracle.GetPrice("DOGE-PERP"); p != 0 {
		t.Errorf("expected 0 for unknown symbol, got %d", p)
	}
}

func TestMidPriceOracle_PerpToSpot(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100))
	ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/100))

	oracle := NewMidPriceOracle(ex)
	oracle.MapSymbol("BTC-PERP", "BTC/USD")

	spotBook := ex.Books["BTC/USD"]
	spotBook.Bids.addOrder(&Order{ID: 1, ClientID: 1, Price: 49900 * SATOSHI, Qty: SATOSHI, Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()})
	spotBook.Asks.addOrder(&Order{ID: 2, ClientID: 1, Price: 50100 * SATOSHI, Qty: SATOSHI, Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()})

	expected := int64((49900*SATOSHI + 50100*SATOSHI) / 2)
	if p := oracle.GetPrice("BTC-PERP"); p != expected {
		t.Errorf("expected %d, got %d", expected, p)
	}
}

func TestMidPriceOracle_UnmappedSymbol(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	oracle := NewMidPriceOracle(ex)
	if p := oracle.GetPrice("BTC-PERP"); p != 0 {
		t.Errorf("expected 0 for unmapped symbol, got %d", p)
	}
}

func TestMidPriceOracle_MissingBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	oracle := NewMidPriceOracle(ex)
	oracle.MapSymbol("BTC-PERP", "BTC/USD")
	if p := oracle.GetPrice("BTC-PERP"); p != 0 {
		t.Errorf("expected 0 for missing book, got %d", p)
	}
}

func TestDynamicPriceOracle(t *testing.T) {
	oracle := NewDynamicPriceOracle(func(symbol string) int64 {
		if symbol == "BTC-PERP" {
			return 50000 * SATOSHI
		}
		return 0
	})

	if p := oracle.GetPrice("BTC-PERP"); p != 50000*SATOSHI {
		t.Errorf("expected %d, got %d", 50000*SATOSHI, p)
	}
	if p := oracle.GetPrice("ETH-PERP"); p != 0 {
		t.Errorf("expected 0 for unknown symbol, got %d", p)
	}
}
