package exchange

import (
	"errors"
	"sync"
	"testing"
	"time"

	einstrument "exchange_sim/instrument"
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

type mixedEpochLiquidationCounter struct {
	count int
}

func (c *mixedEpochLiquidationCounter) OnMarginCall(*MarginCallEvent)       {}
func (c *mixedEpochLiquidationCounter) OnInsuranceFund(*InsuranceFundEvent) {}
func (c *mixedEpochLiquidationCounter) OnLiquidation(*LiquidationEvent)     { c.count++ }

func TestDerivativeRefreshCommitsCompleteMixedRiskEpoch(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()

	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	option := NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, clock.now+int64(365*24*time.Hour), true)
	ex.AddInstrument(spot)
	ex.AddInstrument(perp)
	ex.AddInstrument(option)
	addTwoSidedQuote(t, ex.Books[spot.Symbol()], 100, 120)
	perp.UpdateMarkReferences(110, 110)

	liquidations := &mixedEpochLiquidationCounter{}
	ex.LiquidationHandler = liquidations
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(2, "USD", 1_000_000)
	positions := ex.Positions.(*PositionManager)
	positions.Lock()
	positions.InjectPosition(1, perp.Symbol(), &Position{
		ClientID: 1, Symbol: perp.Symbol(), PositionSide: PositionBoth,
		Size: 1, EntryPrice: 110,
	})
	positions.InjectPosition(1, option.Symbol(), &Position{
		ClientID: 1, Symbol: option.Symbol(), PositionSide: PositionBoth,
		Size: -1, EntryPrice: 1,
	})
	positions.Unlock()

	ex.UpdateDerivativeMarks()
	ex.mu.RLock()
	epoch, coherent := ex.accountMarkEpochLocked(1, "USD")
	profile, profileErr := ex.buildAccountMarginProfileAtEpoch(1, "USD", "", 0, epoch)
	ex.mu.RUnlock()
	if !coherent || epoch == 0 {
		t.Fatalf("derivative refresh did not commit a complete mixed mark epoch: epoch=%d coherent=%v", epoch, coherent)
	}
	if profileErr != nil {
		t.Fatalf("mixed profile rejected after complete derivative refresh: %v", profileErr)
	}
	if profile.Maintenance == 0 {
		t.Fatal("mixed profile lost option/perp maintenance")
	}

	premium, err := option.MarkPremium()
	if err != nil {
		t.Fatalf("option mark unavailable after derivative refresh: %v", err)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		Symbol: option.Symbol(), Side: Sell, Type: LimitOrder,
		Price: premium, Qty: 1, TimeInForce: GTC,
	})
	if !response.Success {
		t.Fatalf("option liquidation liquidity rejected: %v", response.Error)
	}
	ex.CheckPositionMarginerLiquidations()
	if liquidations.count == 0 {
		t.Fatal("complete mixed mark epoch still suppressed the expected option risk action")
	}
}

type blockingMarkCalculator struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	price   int64
}

func (c *blockingMarkCalculator) Calculate(*OrderBook) (int64, error) {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return c.price, nil
}

func TestMarkProducersSerializeRiskEpochCommit(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	ex.AddInstrument(perp)
	calculator := &blockingMarkCalculator{
		started: make(chan struct{}), release: make(chan struct{}), price: 100,
	}
	ex.ConfigureAutomation(AutomationConfig{MarkPriceCalc: calculator})

	priceDone := make(chan struct{})
	go func() {
		ex.UpdatePerpPrices()
		close(priceDone)
	}()
	select {
	case <-calculator.started:
	case <-time.After(time.Second):
		t.Fatal("mark calculator did not start")
	}

	derivativeDone := make(chan struct{})
	go func() {
		ex.UpdateDerivativeMarks()
		close(derivativeDone)
	}()
	select {
	case <-derivativeDone:
		t.Fatal("derivative refresh overtook an in-flight mark/risk pass")
	case <-time.After(100 * time.Millisecond):
	}
	close(calculator.release)
	select {
	case <-priceDone:
	case <-time.After(time.Second):
		t.Fatal("mark pass did not finish after release")
	}
	select {
	case <-derivativeDone:
	case <-time.After(time.Second):
		t.Fatal("derivative refresh did not finish after mark pass")
	}
}

type legacyPositionMarginer struct {
	*einstrument.SpotInstrument
	mark        int64
	maintenance int64
}

func (m *legacyPositionMarginer) PositionMark() (int64, error) { return m.mark, nil }

func (m *legacyPositionMarginer) MaintenanceForPosition(size, _ int64) int64 {
	if size == 0 {
		return 0
	}
	return m.maintenance
}

func TestCoherentEpochKeepsLegacyPositionMarginerExtensible(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	custom := &legacyPositionMarginer{
		SpotInstrument: einstrument.NewSpotInstrument("ABC-CUSTOM", "ABC", "USD", 1, 1, 1, 1),
		mark:           7, maintenance: 3,
	}
	ex.AddInstrument(perp)
	ex.AddInstrument(custom)
	perp.UpdateMarkReferences(100, 100)
	positions := ex.Positions.(*PositionManager)
	positions.Lock()
	positions.InjectPosition(1, perp.Symbol(), &Position{ClientID: 1, Symbol: perp.Symbol(), Size: 1, EntryPrice: 100})
	positions.InjectPosition(1, custom.Symbol(), &Position{ClientID: 1, Symbol: custom.Symbol(), Size: 1, EntryPrice: 7})
	positions.Unlock()

	epoch, err := ex.CommitMarkEpoch([]string{perp.Symbol(), custom.Symbol()})
	if err != nil {
		t.Fatalf("legacy position marginer epoch rejected: %v", err)
	}
	ex.mu.RLock()
	profile, profileErr := ex.buildAccountMarginProfileAtEpoch(1, "USD", "", 0, epoch)
	ex.mu.RUnlock()
	if profileErr != nil {
		t.Fatalf("legacy position marginer cannot join strict profile: %v", profileErr)
	}
	if profile.Maintenance < custom.maintenance {
		t.Fatalf("legacy position marginer maintenance was lost: got %d", profile.Maintenance)
	}
}
