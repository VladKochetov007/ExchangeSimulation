package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-019. Margin is aggregated across every book the account touches;
// liquidation is not. buildAccountMarginProfile walks all books in sorted
// symbol order and adds each position's unrealized PnL to equity, but
// CheckLiquidations is entered per symbol from that symbol's mark update and,
// when the account breaches, closes only the positions in that symbol.
//
// So an account can be under maintenance because of instrument B and have
// instrument A confiscated, because A is the book that happened to tick. Which
// of an actor's positions is taken then depends on the tick order of
// instruments it does not control, and the exposure that caused the loss
// survives with no margin behind it.
//
// This test measures what actually happens. It does not prescribe a
// liquidation policy: partial-close ordering and cross-book seizure rules are
// scientific economics and the owner's to decide.
func TestAuditCrossBookLiquidationClosesTheTickingBookNotTheLosingOne(t *testing.T) {
	const (
		perpSymbol = "ABC-PERP"
		futSymbol  = "ABC-FUT"
	)

	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures(perpSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	sibling := NewPerpFutures(futSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	ex.AddInstrument(sibling)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler
	ex.LiquidationFeeBps = 0

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(900))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))

	// Client 1 is long 1 unit of the perp and long 10 of the sibling, both
	// entered at 100. Hand-derived below rather than read back from the margin
	// engine, so the engine is not its own oracle.
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, perpSymbol, &Position{
		ClientID: 1, Symbol: perpSymbol, PositionSide: PositionBoth,
		Size: BTCAmount(1), EntryPrice: USDAmount(100),
	})
	pm.InjectPosition(1, futSymbol, &Position{
		ClientID: 1, Symbol: futSymbol, PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	// The sibling collapses; the perp does not move.
	//
	//   sibling uPnL = 10 * (5 - 100)     = -950 USD
	//   perp    uPnL =  1 * (100 - 100)   =    0 USD
	//   equity       = 900 - 950 + 0      =  -50 USD, below any maintenance
	//
	// The deficit is entirely the sibling's. The perp is exactly at its entry.
	if err := sibling.UpdateFundingRate(USDAmount(5), USDAmount(5)); err != nil {
		t.Fatalf("sibling mark: %v", err)
	}

	// Liquidity so the perp position can actually be closed.
	if _, reject := InjectLimitOrder(ex, 2, perpSymbol, Buy, USDAmount(100), BTCAmount(1)); reject != "" {
		t.Fatalf("perp liquidity rejected: %s", reject)
	}

	// A mark update arrives on the perp — the healthy book.
	ex.CheckLiquidations(perpSymbol, perp, USDAmount(100))

	perpPos := ex.Positions.GetPosition(1, perpSymbol)
	futPos := ex.Positions.GetPosition(1, futSymbol)
	perpSize := int64(0)
	if perpPos != nil {
		perpSize = perpPos.Size
	}
	futSize := int64(0)
	if futPos != nil {
		futSize = futPos.Size
	}
	t.Logf("after a mark update on the healthy book: liquidations=%d, %s size=%d, %s size=%d, perp cash=%d",
		handler.liquidations, perpSymbol, perpSize, futSymbol, futSize, ex.Clients[1].PerpBalances["USD"])

	if handler.liquidations == 0 {
		t.Fatalf("H-019 not exercised: the account was not liquidated at all, so the aggregation asymmetry was never reached")
	}

	// Measured, and pinned rather than prescribed: the healthy position is the
	// one taken. A change in either direction should surface here.
	if perpSize != 0 {
		t.Errorf("the perp position survived the liquidation it triggered: size %d", perpSize)
	}
	if futSize != BTCAmount(10) {
		t.Errorf("the liquidation reached the sibling exposure: %s is now %d, was %d. "+
			"That would be an improvement, but it changes what this test documents",
			futSymbol, futSize, BTCAmount(10))
	}

	if futSize == 0 {
		return
	}

	// How bad is it? Two questions, in order.
	//
	// First: is the account still findable through the door it was found by?
	before := handler.liquidations
	ex.CheckLiquidations(perpSymbol, perp, USDAmount(100))
	if handler.liquidations == before {
		t.Logf("a second check on %s finds nothing: the trigger symbol has no positions left, so the account "+
			"can only be re-examined through a mark update on %s", perpSymbol, futSymbol)
	}

	// Second, and this is what decides the severity: does a mark update on the
	// losing book itself reach the exposure? If it does, the defect is that the
	// WRONG position was taken first and the account was briefly carried
	// unmargined. If it does not, the deficit is permanent.
	if _, reject := InjectLimitOrder(ex, 2, futSymbol, Buy, USDAmount(5), BTCAmount(10)); reject != "" {
		t.Fatalf("sibling liquidity rejected: %s", reject)
	}
	before = handler.liquidations
	ex.CheckLiquidations(futSymbol, sibling, USDAmount(5))
	after := ex.Positions.GetPosition(1, futSymbol)
	afterSize := int64(0)
	if after != nil {
		afterSize = after.Size
	}
	t.Logf("after a mark update on the losing book: liquidations=%d (was %d), %s size=%d, perp cash=%d, insurance fund=%d",
		handler.liquidations, before, futSymbol, afterSize, ex.Clients[1].PerpBalances["USD"],
		ex.ExchangeBalance.InsuranceFund["USD"])
	if afterSize != 0 {
		t.Errorf("the losing exposure survives even a mark update on its own book: %d units still open", afterSize)
	}
	// The deficit is therefore transient, not permanent, and the fund absorbs
	// exactly the hand-derived shortfall: equity was 900 - 950 = -50.
	if got, want := ex.ExchangeBalance.InsuranceFund["USD"], -USDAmount(50); got != want {
		t.Errorf("insurance fund absorbed %d, want the hand-derived deficit %d", got, want)
	}
	if got := ex.Clients[1].PerpBalances["USD"]; got != 0 {
		t.Errorf("bankrupt perp cash = %d, want 0 after the write-down", got)
	}
}
