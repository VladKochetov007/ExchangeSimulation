package exchange

import "testing"

func TestSimplePriceOracle_ReturnsZeroForUnmapped(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	o := NewSimplePriceOracle(ex)
	if p := o.GetPrice("BTC"); p != 0 {
		t.Errorf("expected 0 for unmapped asset, got %d", p)
	}
}

func TestSimplePriceOracle_ReturnsZeroForEmptyBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	o := NewSimplePriceOracle(ex)
	o.MapAssetToSymbol("BTC", "BTC/USD")
	if p := o.GetPrice("BTC"); p != 0 {
		t.Errorf("expected 0 for empty book, got %d", p)
	}
}

func TestSimplePriceOracle_ReturnsMidPrice(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))

	book := ex.Books["BTC/USD"]
	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: SATOSHI, Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	ask := &Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: SATOSHI, Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.addOrder(bid)
	book.Asks.addOrder(ask)

	o := NewSimplePriceOracle(ex)
	o.MapAssetToSymbol("BTC", "BTC/USD")

	mid := o.GetPrice("BTC")
	expected := (PriceUSD(49_000, DOLLAR_TICK) + PriceUSD(51_000, DOLLAR_TICK)) / 2
	if mid != expected {
		t.Errorf("mid price: expected %d, got %d", expected, mid)
	}
}
