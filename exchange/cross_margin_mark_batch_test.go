package exchange

import (
	"errors"
	"testing"
	"time"

	etypes "exchange_sim/types"
)

func addTwoSidedQuote(t *testing.T, book *OrderBook, bidPrice, askPrice int64) {
	t.Helper()
	if !book.Bids.AddOrder(&Order{ID: 1, Side: Buy, Price: bidPrice, Qty: 1, Visibility: Normal}) {
		t.Fatalf("could not seed bid on %s", book.Symbol)
	}
	if !book.Asks.AddOrder(&Order{ID: 2, Side: Sell, Price: askPrice, Qty: 1, Visibility: Normal}) {
		t.Fatalf("could not seed ask on %s", book.Symbol)
	}
}

func TestUpdatePerpPricesRefreshesOptionAndDatedMarksBeforeRisk(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, clock.now+int64(365*24*time.Hour))
	future.Underlying = spot.Symbol()
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, clock.now+int64(365*24*time.Hour), true)
	for _, instrument := range []Instrument{spot, perp, future, option} {
		ex.AddInstrument(instrument)
	}
	ex.ConfigureAutomation(AutomationConfig{MarkPriceCalc: NewMidPriceCalculator()})
	addTwoSidedQuote(t, ex.Books[spot.Symbol()], 100, 120)
	addTwoSidedQuote(t, ex.Books[perp.Symbol()], 99, 101)
	addTwoSidedQuote(t, ex.Books[future.Symbol()], 100, 102)
	option.SetMarks(90, 7)

	ex.UpdatePerpPrices()

	underlying, err := option.UnderlyingMark()
	if err != nil || underlying != 110 {
		t.Fatalf("option underlying mark = %d, %v; want refreshed 110", underlying, err)
	}
	premium, err := option.MarkPremium()
	if err != nil || premium == 7 {
		t.Fatalf("option premium mark = %d, %v; stale option mark survived the perp risk phase", premium, err)
	}
	funding := future.GetFundingRate()
	if !funding.IndexAvailable || funding.IndexPrice != 110 || !funding.MarkAvailable || funding.MarkPrice != 101 {
		t.Fatalf("dated future mark set = %#v, want index 110 and book mark 101", funding)
	}
}

func TestUpdatePerpPricesClearsStaleOptionMarkWhenUnderlyingUnavailable(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, clock.now+int64(365*24*time.Hour), true)
	ex.AddInstrument(spot)
	ex.AddInstrument(option)
	ex.ConfigureAutomation(AutomationConfig{MarkPriceCalc: NewMidPriceCalculator()})
	addTwoSidedQuote(t, ex.Books[spot.Symbol()], 100, 120)
	option.SetMarks(90, 7)
	ex.UpdatePerpPrices()
	if _, err := option.MarkPremium(); err != nil {
		t.Fatalf("initial option mark unavailable: %v", err)
	}

	ex.Books[spot.Symbol()].Bids.CancelOrder(1)
	ex.Books[spot.Symbol()].Asks.CancelOrder(2)
	ex.UpdatePerpPrices()

	if premium, err := option.MarkPremium(); !errors.Is(err, etypes.ErrNoPrice) || premium != 0 {
		t.Fatalf("stale option premium = (%d, %v), want unavailable", premium, err)
	}
	if _, err := riskMark(option, ex.Books[option.Symbol()]); !errors.Is(err, etypes.ErrNoPrice) {
		t.Fatalf("stale option risk mark error = %v, want ErrNoPrice", err)
	}
}

func TestCrossMarginOptionRiskUsesImmutableSnapshotInputs(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", "ABC/USD", 1, 1, 1, 1, 100, clock.now+int64(time.Hour), true)
	option.SetMarks(100, 10)
	ex.AddInstrument(option)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.ConnectNewClient(2, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 100)
	ex.AddPerpBalance(2, "USD", 10_000)
	if delta := ex.Positions.UpdatePosition(1, option.Symbol(), 1, 10, Sell, PositionBoth); delta.NewSize != -1 {
		t.Fatalf("option position delta = %#v", delta)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: option.Symbol(), Side: Sell, Type: LimitOrder,
		Price: 1_000, Qty: 1, TimeInForce: GTC, PositionSide: PositionBoth,
	})
	if !response.Success {
		t.Fatalf("option covering ask rejected: %s", response.Error)
	}
	epoch, err := ex.CommitMarkEpoch([]string{option.Symbol()})
	if err != nil {
		t.Fatalf("commit option risk snapshot: %v", err)
	}
	option.SetMarks(1_000, 1_000)
	option.Margin.MMBps = 10_000

	ex.checkPositionMarginerLiquidationsAtEpoch(epoch)
	position := ex.Positions.GetPosition(1, option.Symbol())
	if position == nil || position.Size != -1 {
		t.Fatalf("live option mark/configuration mutation changed snapshot risk: %#v", position)
	}
}

func TestUpdateDerivativeMarksStoresCompleteOptionRiskSnapshot(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()
	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, clock.now+int64(time.Hour), true)
	ex.AddInstrument(spot)
	ex.AddInstrument(option)
	addTwoSidedQuote(t, ex.Books[spot.Symbol()], 100, 120)

	epoch := ex.UpdateDerivativeMarks()
	if epoch == 0 {
		t.Fatal("derivative mark pass did not commit an option epoch")
	}
	snapshot, ok := ex.riskMarkSnapshots[option.Symbol()]
	if !ok {
		t.Fatal("option risk snapshot was not recorded")
	}
	underlying, err := option.UnderlyingMark()
	if err != nil {
		t.Fatalf("option underlying mark: %v", err)
	}
	premium, err := option.MarkPremium()
	if err != nil {
		t.Fatalf("option premium mark: %v", err)
	}
	if snapshot.underlying != underlying || snapshot.mark != premium || snapshot.maintenanceBps != option.Margin.MMBps || snapshot.timestamp != clock.now {
		t.Fatalf("option risk snapshot = %#v, want underlying=%d mark=%d maintenance=%d timestamp=%d", snapshot, underlying, premium, option.Margin.MMBps, clock.now)
	}
}

func TestCrossMarginRiskRejectsMixedSnapshotTimestamps(t *testing.T) {
	ex, a, b := seedCrossMarginLiquidationCase(t)
	defer ex.Shutdown()
	snapshot := ex.riskMarkSnapshots[b.Symbol()]
	snapshot.timestamp++
	ex.riskMarkSnapshots[b.Symbol()] = snapshot

	ex.checkLiquidationsAtEpoch(a.Symbol(), a, 50, ex.markEpoch)
	for _, symbol := range []string{a.Symbol(), b.Symbol()} {
		position := ex.Positions.GetPosition(1, symbol)
		if position == nil || position.Size != 10 {
			t.Fatalf("mixed snapshot timestamp changed %s position: %#v", symbol, position)
		}
	}
}
