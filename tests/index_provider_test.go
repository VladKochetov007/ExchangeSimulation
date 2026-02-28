package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func TestStaticPriceOracle(t *testing.T) {
	oracle := NewStaticPriceOracle(map[string]int64{
		"BTC-PERP": 50000 * BTC_PRECISION,
		"ETH-PERP": 3000 * BTC_PRECISION,
	})

	if p := oracle.Price("BTC-PERP"); p != 50000*BTC_PRECISION {
		t.Errorf("expected %d, got %d", 50000*BTC_PRECISION, p)
	}
	if p := oracle.Price("ETH-PERP"); p != 3000*BTC_PRECISION {
		t.Errorf("expected %d, got %d", 3000*BTC_PRECISION, p)
	}
	if p := oracle.Price("DOGE-PERP"); p != 0 {
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
	spotBook.Bids.AddOrder(&Order{ID: 1, ClientID: 1, Price: 49900 * BTC_PRECISION, Qty: BTC_PRECISION, Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()})
	spotBook.Asks.AddOrder(&Order{ID: 2, ClientID: 1, Price: 50100 * BTC_PRECISION, Qty: BTC_PRECISION, Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()})

	expected := int64((49900*BTC_PRECISION + 50100*BTC_PRECISION) / 2)
	if p := oracle.Price("BTC-PERP"); p != expected {
		t.Errorf("expected %d, got %d", expected, p)
	}
}

func TestMidPriceOracle_UnmappedSymbol(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	oracle := NewMidPriceOracle(ex)
	if p := oracle.Price("BTC-PERP"); p != 0 {
		t.Errorf("expected 0 for unmapped symbol, got %d", p)
	}
}

func TestMidPriceOracle_MissingBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	oracle := NewMidPriceOracle(ex)
	oracle.MapSymbol("BTC-PERP", "BTC/USD")
	if p := oracle.Price("BTC-PERP"); p != 0 {
		t.Errorf("expected 0 for missing book, got %d", p)
	}
}
