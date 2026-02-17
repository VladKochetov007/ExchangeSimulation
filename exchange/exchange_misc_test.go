package exchange

import (
	"testing"
	"time"
)

func TestGetBestLiquidity_UnknownSymbol(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	bid, ask := ex.GetBestLiquidity("UNKNOWN")
	if bid != 0 || ask != 0 {
		t.Errorf("expected (0,0) for unknown symbol, got (%d,%d)", bid, ask)
	}
}

func TestGetBestLiquidity_WithOrders(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))

	book := ex.Books["BTC/USD"]
	bidQty := int64(2 * SATOSHI)
	askQty := int64(3 * SATOSHI)
	book.Bids.addOrder(&Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: bidQty, Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()})
	book.Asks.addOrder(&Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: askQty, Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()})

	bid, ask := ex.GetBestLiquidity("BTC/USD")
	if bid != bidQty {
		t.Errorf("bid qty: expected %d, got %d", bidQty, bid)
	}
	if ask != askQty {
		t.Errorf("ask qty: expected %d, got %d", askQty, ask)
	}
}

func TestEnablePeriodicSnapshots(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI))
	// Just verifies EnablePeriodicSnapshots doesn't panic.
	ex.EnablePeriodicSnapshots(100 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	ex.Shutdown()
}

func TestRealClock_NowUnix(t *testing.T) {
	c := &RealClock{}
	unix := c.NowUnix()
	if unix <= 0 {
		t.Errorf("expected positive unix timestamp, got %d", unix)
	}
}

func TestInstrumentType_Spot(t *testing.T) {
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	if inst.InstrumentType() != "SPOT" {
		t.Errorf("expected \"SPOT\", got %q", inst.InstrumentType())
	}
	if inst.QuotePrecision() != USD_PRECISION {
		t.Errorf("QuotePrecision: expected %d, got %d", USD_PRECISION, inst.QuotePrecision())
	}
}

func TestInstrumentType_Perp(t *testing.T) {
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	if perp.InstrumentType() != "PERP" {
		t.Errorf("expected \"PERP\", got %q", perp.InstrumentType())
	}
}

func TestSetFundingCalculator(t *testing.T) {
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	custom := &SimpleFundingCalc{BaseRate: 10, Damping: 1, MaxRate: 100}
	perp.SetFundingCalculator(custom)
	perp.UpdateFundingRate(PriceUSD(50_000, DOLLAR_TICK), PriceUSD(50_100, DOLLAR_TICK))
}

func TestReservedSpotDelta(t *testing.T) {
	d := reservedSpotDelta("USD", 1_000, 900)
	if d.Asset != "USD" || d.OldBalance != 1_000 || d.NewBalance != 900 {
		t.Errorf("unexpected delta: %+v", d)
	}
}
