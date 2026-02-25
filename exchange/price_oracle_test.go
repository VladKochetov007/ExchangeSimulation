package exchange

import "testing"

func TestMidPriceOracle_ReturnsZeroForUnmapped(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	o := NewMidPriceOracle(ex)
	if p := o.GetPrice("BTC"); p != 0 {
		t.Errorf("expected 0 for unmapped asset, got %d", p)
	}
}

func TestMidPriceOracle_ReturnsZeroForEmptyBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	o := NewMidPriceOracle(ex)
	o.MapSymbol("BTC", "BTC/USD")
	if p := o.GetPrice("BTC"); p != 0 {
		t.Errorf("expected 0 for empty book, got %d", p)
	}
}

func TestMidPriceOracle_ReturnsMidPrice(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))

	book := ex.Books["BTC/USD"]
	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: BTC_PRECISION, Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	ask := &Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: BTC_PRECISION, Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.AddOrder(bid)
	book.Asks.AddOrder(ask)

	o := NewMidPriceOracle(ex)
	o.MapSymbol("BTC", "BTC/USD")

	expected := (PriceUSD(49_000, DOLLAR_TICK) + PriceUSD(51_000, DOLLAR_TICK)) / 2
	if mid := o.GetPrice("BTC"); mid != expected {
		t.Errorf("mid price: expected %d, got %d", expected, mid)
	}
}
