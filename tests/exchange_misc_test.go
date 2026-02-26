package exchange_test

import (
	. "exchange_sim/exchange"
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
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))

	book := ex.Books["BTC/USD"]
	bidQty := int64(2 * BTC_PRECISION)
	askQty := int64(3 * BTC_PRECISION)
	book.Bids.AddOrder(&Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: bidQty, Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()})
	book.Asks.AddOrder(&Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: askQty, Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()})

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
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
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
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	if inst.InstrumentType() != "SPOT" {
		t.Errorf("expected \"SPOT\", got %q", inst.InstrumentType())
	}
	if inst.QuotePrecision() != USD_PRECISION {
		t.Errorf("QuotePrecision: expected %d, got %d", USD_PRECISION, inst.QuotePrecision())
	}
}

func TestInstrumentType_Perp(t *testing.T) {
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	if perp.InstrumentType() != "PERP" {
		t.Errorf("expected \"PERP\", got %q", perp.InstrumentType())
	}
}

func TestSetFundingCalculator(t *testing.T) {
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	custom := &SimpleFundingCalc{BaseRate: 10, Damping: 1, MaxRate: 100}
	perp.SetFundingCalculator(custom)
	perp.UpdateFundingRate(PriceUSD(50_000, DOLLAR_TICK), PriceUSD(50_100, DOLLAR_TICK))
}

func TestReservedSpotDelta(t *testing.T) {
	d := ReservedSpotDelta("USD", 1_000, 900)
	if d.Asset != "USD" || d.OldBalance != 1_000 || d.NewBalance != 900 {
		t.Errorf("unexpected delta: %+v", d)
	}
}

func TestUSDTAmount(t *testing.T) {
	if USDTAmount(1.0) != USDT_PRECISION {
		t.Errorf("USDTAmount(1.0) = %d, want %d", USDTAmount(1.0), USDT_PRECISION)
	}
}

func TestEnableBorrowing_NilOracleReturnsError(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	err := ex.EnableBorrowing(BorrowingConfig{Enabled: true, PriceOracle: nil})
	if err == nil {
		t.Error("expected error when enabling borrowing without price oracle")
	}
}

func TestWeightedMidPriceCalculator_BothSides(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	book := ex.Books["BTC/USD"]

	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: BTCAmount(2), Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	ask := &Order{ID: 2, ClientID: 1, Price: PriceUSD(51_000, DOLLAR_TICK), Qty: BTCAmount(1), Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.AddOrder(bid)
	book.Asks.AddOrder(ask)

	calc := NewWeightedMidPriceCalculator()
	mid := calc.Calculate(book)
	if mid <= 0 {
		t.Errorf("expected positive weighted mid, got %d", mid)
	}
}

func TestWeightedMidPriceCalculator_OnlyBid(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	book := ex.Books["BTC/USD"]

	bid := &Order{ID: 1, ClientID: 1, Price: PriceUSD(49_000, DOLLAR_TICK), Qty: BTCAmount(1), Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano()}
	book.Bids.AddOrder(bid)

	calc := NewWeightedMidPriceCalculator()
	// No ask — falls back to last trade price (0 here since no trades occurred)
	mid := calc.Calculate(book)
	if mid < 0 {
		t.Errorf("unexpected negative mid: %d", mid)
	}
}

func TestWeightedMidPriceCalculator_EmptyBook(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	book := ex.Books["BTC/USD"]

	calc := NewWeightedMidPriceCalculator()
	mid := calc.Calculate(book)
	if mid != 0 {
		t.Errorf("empty book: expected 0, got %d", mid)
	}
}

func TestPublishSnapshot_ViaSubscribe(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION))
	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	gateway := ex.Gateways[1]

	ex.MDPublisher.Subscribe(1, "BTC/USD", []MDType{MDSnapshot}, gateway)

	const reqID = uint64(6666)
	req := Request{
		Type:     ReqSubscribe,
		QueryReq: &QueryRequest{RequestID: reqID, Symbol: "BTC/USD"},
	}
	resp := sendRequest(gateway, req, reqID)
	if !resp.Success {
		t.Errorf("subscribe should succeed, got error=%v", resp.Error)
	}
}
