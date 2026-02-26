package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
)

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestNewPerpFutures(t *testing.T) {
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, BTC_PRECISION, BTC_PRECISION/100)

	if perp.Symbol() != "BTC-PERP" {
		t.Errorf("Expected symbol BTC-PERP, got %s", perp.Symbol())
	}
	if !perp.IsPerp() {
		t.Errorf("IsPerp() should return true")
	}
	if perp.BaseAsset() != "BTC" {
		t.Errorf("Expected base BTC, got %s", perp.BaseAsset())
	}
	if perp.QuoteAsset() != "USD" {
		t.Errorf("Expected quote USD, got %s", perp.QuoteAsset())
	}

	fr := perp.GetFundingRate()
	if fr == nil {
		t.Fatalf("Funding rate should not be nil")
	}
	if fr.Symbol != "BTC-PERP" {
		t.Errorf("Expected funding rate symbol BTC-PERP, got %s", fr.Symbol)
	}
	if fr.Interval != 28800 {
		t.Errorf("Expected interval 28800, got %d", fr.Interval)
	}
}

func TestPerpFuturesUpdateFundingRate(t *testing.T) {
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, BTC_PRECISION, BTC_PRECISION/100)

	indexPrice := int64(50000 * BTC_PRECISION)
	markPrice := int64(50100 * BTC_PRECISION)

	perp.UpdateFundingRate(indexPrice, markPrice)

	fr := perp.GetFundingRate()
	if fr.IndexPrice != indexPrice {
		t.Errorf("Expected index price %d, got %d", indexPrice, fr.IndexPrice)
	}
	if fr.MarkPrice != markPrice {
		t.Errorf("Expected mark price %d, got %d", markPrice, fr.MarkPrice)
	}
	if fr.Rate == 0 {
		t.Errorf("Rate should be calculated, got 0")
	}
}

func TestSimpleFundingCalcPositivePremium(t *testing.T) {
	calc := &SimpleFundingCalc{
		BaseRate: 10,
		Damping:  100,
		MaxRate:  75,
	}

	indexPrice := int64(50000 * BTC_PRECISION)
	markPrice := int64(50100 * BTC_PRECISION)

	rate := calc.Calculate(indexPrice, markPrice)

	premium := ((markPrice - indexPrice) * 10000) / indexPrice
	expectedRate := int64(10 + (premium * 100 / 100))

	if rate != expectedRate {
		t.Errorf("Expected rate %d, got %d", expectedRate, rate)
	}
}

func TestSimpleFundingCalcNegativePremium(t *testing.T) {
	calc := &SimpleFundingCalc{
		BaseRate: 10,
		Damping:  100,
		MaxRate:  75,
	}

	indexPrice := int64(50000 * BTC_PRECISION)
	markPrice := int64(49900 * BTC_PRECISION)

	rate := calc.Calculate(indexPrice, markPrice)

	if rate >= 10 {
		t.Errorf("Expected rate < 10 for negative premium, got %d", rate)
	}
}

func TestSimpleFundingCalcMaxRateCap(t *testing.T) {
	calc := &SimpleFundingCalc{
		BaseRate: 10,
		Damping:  100,
		MaxRate:  75,
	}

	indexPrice := int64(50000 * BTC_PRECISION)
	markPrice := int64(60000 * BTC_PRECISION)

	rate := calc.Calculate(indexPrice, markPrice)

	if rate > calc.MaxRate {
		t.Errorf("Rate should be capped at MaxRate %d, got %d", calc.MaxRate, rate)
	}
	if rate != calc.MaxRate {
		t.Errorf("Expected rate to be capped at %d, got %d", calc.MaxRate, rate)
	}
}

func TestSimpleFundingCalcMinRateCap(t *testing.T) {
	calc := &SimpleFundingCalc{
		BaseRate: 10,
		Damping:  100,
		MaxRate:  75,
	}

	indexPrice := int64(50000 * BTC_PRECISION)
	markPrice := int64(40000 * BTC_PRECISION)

	rate := calc.Calculate(indexPrice, markPrice)

	if rate < -calc.MaxRate {
		t.Errorf("Rate should be capped at -MaxRate %d, got %d", -calc.MaxRate, rate)
	}
	if rate != -calc.MaxRate {
		t.Errorf("Expected rate to be capped at %d, got %d", -calc.MaxRate, rate)
	}
}

func TestSimpleFundingCalcZeroIndex(t *testing.T) {
	calc := &SimpleFundingCalc{
		BaseRate: 10,
		Damping:  100,
		MaxRate:  75,
	}

	rate := calc.Calculate(0, 50000*BTC_PRECISION)

	if rate != 0 {
		t.Errorf("Expected rate 0 for zero index price, got %d", rate)
	}
}

func TestPositionManagerGetPosition(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pos := pm.GetPosition(1, "BTC-PERP")
	if pos != nil {
		t.Errorf("Expected nil for non-existent position")
	}

	pm.UpdatePosition(1, "BTC-PERP", 100, 50000*BTC_PRECISION, Buy)

	pos = pm.GetPosition(1, "BTC-PERP")
	if pos == nil {
		t.Fatalf("Position should exist")
	}
	if pos.Size != 100 {
		t.Errorf("Expected size 100, got %d", pos.Size)
	}
}

func TestPositionManagerUpdatePositionNewLong(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pm.UpdatePosition(1, "BTC-PERP", 100, 50000*BTC_PRECISION, Buy)

	pos := pm.GetPosition(1, "BTC-PERP")
	if pos.Size != 100 {
		t.Errorf("Expected size 100, got %d", pos.Size)
	}
	if pos.EntryPrice != 50000*BTC_PRECISION {
		t.Errorf("Expected entry price %d, got %d", 50000*BTC_PRECISION, pos.EntryPrice)
	}
}

func TestPositionManagerUpdatePositionNewShort(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pm.UpdatePosition(1, "BTC-PERP", 100, 50000*BTC_PRECISION, Sell)

	pos := pm.GetPosition(1, "BTC-PERP")
	if pos.Size != -100 {
		t.Errorf("Expected size -100, got %d", pos.Size)
	}
	if pos.EntryPrice != 50000*BTC_PRECISION {
		t.Errorf("Expected entry price %d, got %d", 50000*BTC_PRECISION, pos.EntryPrice)
	}
}

func TestPositionManagerUpdatePositionIncreaseLong(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pm.UpdatePosition(1, "BTC-PERP", 100, 50000*BTC_PRECISION, Buy)
	pm.UpdatePosition(1, "BTC-PERP", 100, 51000*BTC_PRECISION, Buy)

	pos := pm.GetPosition(1, "BTC-PERP")
	if pos.Size != 200 {
		t.Errorf("Expected size 200, got %d", pos.Size)
	}

	expectedEntry := int64((100*50000*BTC_PRECISION + 100*51000*BTC_PRECISION) / 200)
	if pos.EntryPrice != expectedEntry {
		t.Errorf("Expected average entry price %d, got %d", expectedEntry, pos.EntryPrice)
	}
}

func TestPositionManagerUpdatePositionClosePosition(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pm.UpdatePosition(1, "BTC-PERP", 100, 50000*BTC_PRECISION, Buy)
	pm.UpdatePosition(1, "BTC-PERP", 100, 51000*BTC_PRECISION, Sell)

	pos := pm.GetPosition(1, "BTC-PERP")
	if pos.Size != 0 {
		t.Errorf("Expected size 0 (closed), got %d", pos.Size)
	}
	if pos.EntryPrice != 0 {
		t.Errorf("Expected entry price 0 (closed), got %d", pos.EntryPrice)
	}
}

func TestPositionManagerUpdatePositionFlipLongToShort(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pm.UpdatePosition(1, "BTC-PERP", 100, 50000*BTC_PRECISION, Buy)
	pm.UpdatePosition(1, "BTC-PERP", 150, 51000*BTC_PRECISION, Sell)

	pos := pm.GetPosition(1, "BTC-PERP")
	if pos.Size != -50 {
		t.Errorf("Expected size -50 (flipped to short), got %d", pos.Size)
	}
	if pos.EntryPrice != 51000*BTC_PRECISION {
		t.Errorf("Expected entry price from flip trade, got %d", pos.EntryPrice)
	}
}

func TestPositionManagerSettleFundingLongPosition(t *testing.T) {
	pm := NewPositionManager(&RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, BTC_PRECISION, BTC_PRECISION/100)

	clients := make(map[uint64]*Client)
	clients[1] = NewClient(1, &FixedFee{})
	clients[1].PerpBalances["USD"] = 10000 * USD_PRECISION

	pm.UpdatePosition(1, "BTC-PERP", BTC_PRECISION, 50000*BTC_PRECISION, Buy)

	perp.UpdateFundingRate(50000*BTC_PRECISION, 50100*BTC_PRECISION)

	balanceBefore := clients[1].PerpBalances["USD"]

	pm.SettleFunding(clients, perp, nil)

	balanceAfter := clients[1].PerpBalances["USD"]

	if balanceAfter >= balanceBefore {
		t.Errorf("Long position should pay funding, balance should decrease")
	}
}

func TestPositionManagerSettleFundingShortPosition(t *testing.T) {
	pm := NewPositionManager(&RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, BTC_PRECISION, BTC_PRECISION/100)

	clients := make(map[uint64]*Client)
	clients[1] = NewClient(1, &FixedFee{})
	clients[1].PerpBalances["USD"] = 10000 * USD_PRECISION

	pm.UpdatePosition(1, "BTC-PERP", BTC_PRECISION, 50000*BTC_PRECISION, Sell)

	perp.UpdateFundingRate(50000*BTC_PRECISION, 50100*BTC_PRECISION)

	balanceBefore := clients[1].PerpBalances["USD"]

	pm.SettleFunding(clients, perp, nil)

	balanceAfter := clients[1].PerpBalances["USD"]

	if balanceAfter <= balanceBefore {
		t.Errorf("Short position should receive funding when mark > index, balance should increase")
	}
}

func TestPositionManagerSettleFundingNoPosition(t *testing.T) {
	pm := NewPositionManager(&RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, BTC_PRECISION, BTC_PRECISION/100)

	clients := make(map[uint64]*Client)
	clients[1] = NewClient(1, &FixedFee{})
	clients[1].PerpBalances["USD"] = 10000 * USD_PRECISION

	balanceBefore := clients[1].PerpBalances["USD"]

	pm.SettleFunding(clients, perp, nil)

	balanceAfter := clients[1].PerpBalances["USD"]

	if balanceAfter != balanceBefore {
		t.Errorf("Balance should not change with no position")
	}
}

func TestAbsFunction(t *testing.T) {
	if abs(100) != 100 {
		t.Errorf("abs(100) should be 100")
	}
	if abs(-100) != 100 {
		t.Errorf("abs(-100) should be 100")
	}
	if abs(0) != 0 {
		t.Errorf("abs(0) should be 0")
	}
}

func TestInstrumentTickSize(t *testing.T) {
	spot := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, 100, 1000)

	if spot.TickSize() != 100 {
		t.Errorf("Expected tick size 100, got %d", spot.TickSize())
	}
}

func TestInstrumentMinOrderSize(t *testing.T) {
	spot := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, 100, 1000)

	if spot.MinOrderSize() != 1000 {
		t.Errorf("Expected min order size 1000, got %d", spot.MinOrderSize())
	}
}

func TestCalculateOpenInterest(t *testing.T) {
	pm := NewPositionManager(&RealClock{})

	pm.UpdatePosition(1, "BTC-PERP", BTC_PRECISION, PriceUSD(50000, CENT_TICK), Buy)
	pm.UpdatePosition(2, "BTC-PERP", -BTC_PRECISION/2, PriceUSD(50000, CENT_TICK), Sell)

	oi := pm.CalculateOpenInterest("BTC-PERP")
	expected := int64(BTC_PRECISION + BTC_PRECISION/2)
	if oi != expected {
		t.Errorf("Expected open interest %d, got %d", expected, oi)
	}
}

func TestPublishOpenInterest(t *testing.T) {
	mdp := NewMDPublisher()
	gateway := NewClientGateway(1)

	types := []MDType{MDOpenInterest}
	mdp.Subscribe(1, "BTC-PERP", types, gateway)

	oi := &OpenInterest{
		Symbol:         "BTC-PERP",
		TotalContracts: 1000000,
		Timestamp:      123456,
	}
	mdp.PublishOpenInterest("BTC-PERP", oi, 123456)

	select {
	case msg := <-gateway.MarketData:
		if msg.Type != MDOpenInterest {
			t.Errorf("Expected MDOpenInterest, got %v", msg.Type)
		}
		receivedOI := msg.Data.(*OpenInterest)
		if receivedOI.TotalContracts != 1000000 {
			t.Errorf("Expected OI 1000000, got %d", receivedOI.TotalContracts)
		}
	default:
		t.Error("Should receive open interest message")
	}
}
