package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// H-020. Failing closed on the measurement can fail open on the action.
//
// buildAccountMarginProfile returns an error when any of the account's
// positions sits on a settlement-pending contract, for a stated and sound
// reason: "Retained pending exposure is not an economic zero. No valid mark
// exists, so fail the whole account profile closed instead of allowing active
// sibling risk to ignore it."
//
// CheckLiquidations receives that error, reports it as a price-unavailable
// diagnostic, and continues to the next client. So the account is not measured
// and, as a consequence, is not liquidated either — however far underwater its
// other positions are.
//
// A dated contract enters that state whenever it reaches expiry with no
// settlement price, and the retry policy is retry-forever, so it can stay there
// while its positions are retained. Two actors with identical losing positions
// then get different treatment, and the difference is whether one of them
// happens to hold an expired contract awaiting a price.
func TestAuditPendingSiblingSuspendsLiquidationOfTheWholeAccount(t *testing.T) {
	const (
		perpSymbol = "ABC-PERP"
		futSymbol  = "ABC-FUT"
	)

	clock := &testClock{now: time.Now().UnixNano()}
	ex := NewExchange(4, clock)
	perp := NewPerpFutures(perpSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	future := NewExpiringFutures(futSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1,
		clock.now+int64(time.Minute))
	ex.AddInstrument(perp)
	ex.AddInstrument(future)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler
	ex.LiquidationFeeBps = 0

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))

	// Client 1 is long 10 of the perp at 100 and holds one unit of the future.
	// The perp alone is enough to bankrupt the account when the mark halves:
	//
	//   perp uPnL = 10 * (50 - 100) = -500 USD against 100 USD of cash.
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, perpSymbol, &Position{
		ClientID: 1, Symbol: perpSymbol, PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.InjectPosition(1, futSymbol, &Position{
		ClientID: 1, Symbol: futSymbol, PositionSide: PositionBoth,
		Size: BTCAmount(1), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	// Liquidity so a liquidation could execute if one were attempted.
	if _, reject := InjectLimitOrder(ex, 2, perpSymbol, Buy, USDAmount(50), BTCAmount(10)); reject != "" {
		t.Fatalf("perp liquidity rejected: %s", reject)
	}

	// The future expires with no settlement price and enters settlement-pending.
	clock.now += int64(2 * time.Minute)
	ex.CheckExpiries()
	if ex.Instruments[futSymbol] == nil {
		t.Fatalf("the future settled; this test needs it to stay pending")
	}
	if pos := ex.Positions.GetPosition(1, futSymbol); pos == nil || pos.Size == 0 {
		t.Fatalf("the pending contract's position was not retained, so the premise does not hold")
	}

	// The perp is now deeply underwater and there is liquidity to close it.
	ex.CheckLiquidations(perpSymbol, perp, USDAmount(50))

	perpPos := ex.Positions.GetPosition(1, perpSymbol)
	perpSize := int64(0)
	if perpPos != nil {
		perpSize = perpPos.Size
	}
	t.Logf("while the sibling is settlement-pending: liquidations=%d, %s size=%d, perp cash=%d",
		handler.liquidations, perpSymbol, perpSize, ex.Clients[1].PerpBalances["USD"])

	if handler.liquidations != 0 || perpSize != BTCAmount(10) {
		t.Fatalf("the account was liquidated (%d events, %s size %d) despite a pending sibling; "+
			"H-020 no longer holds and this test documents behaviour that changed",
			handler.liquidations, perpSymbol, perpSize)
	}

	// The severity turns on what else the account can do while unmeasurable.
	// It is frozen, not privileged: the admission path refuses every order from
	// a client with settlement-pending exposure, including one that would
	// reduce the position. So the actor cannot add risk — and cannot shed it
	// either.
	frozen := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 99, Symbol: perpSymbol, Side: Sell, Type: LimitOrder,
		Price: USDAmount(50), Qty: BTCAmount(10), TimeInForce: GTC,
	})
	if frozen.Success {
		t.Errorf("the account could still trade while unliquidatable: it can add risk no one can measure")
	} else if frozen.Error != RejectSettlementPendingExposure {
		t.Errorf("order refused with %q, want %q: the freeze this test relies on comes from somewhere else",
			frozen.Error, RejectSettlementPendingExposure)
	}

	// What the freeze costs: the position keeps moving while nobody can close
	// it, so the deficit the insurance fund eventually absorbs grows with the
	// market rather than being capped at the liquidation point.
	//
	//   at mark 50: equity = 100 + 10*(50-100)  = -400 USD
	//   at mark 25: equity = 100 + 10*(25-100)  = -650 USD
	//
	// A liquidation exists to stop that growth. Suspending it transfers the
	// difference from the defaulter to the fund.

	// The freeze ends when the contract settles, which bounds the severity.
	future.ObserveSettlement(USDAmount(100), clock.now)
	ex.CheckExpiries()
	if ex.Instruments[futSymbol] != nil {
		t.Fatalf("the future did not settle after a price was observed")
	}
	ex.CheckLiquidations(perpSymbol, perp, USDAmount(50))
	after := ex.Positions.GetPosition(1, perpSymbol)
	afterSize := int64(0)
	if after != nil {
		afterSize = after.Size
	}
	t.Logf("after the sibling settles: liquidations=%d, %s size=%d, perp cash=%d, insurance fund=%d",
		handler.liquidations, perpSymbol, afterSize, ex.Clients[1].PerpBalances["USD"],
		ex.ExchangeBalance.InsuranceFund["USD"])
	if afterSize != 0 {
		t.Errorf("the position survived even after the sibling settled: %d still open", afterSize)
	}
	// Hand-derived: the position closes against the resting bid at 50, so
	// realized PnL is 10*(50-100) = -500 against 100 USD of cash, and the fund
	// absorbs 400.
	if got, want := ex.ExchangeBalance.InsuranceFund["USD"], -USDAmount(400); got != want {
		t.Errorf("insurance fund absorbed %d, want the hand-derived %d", got, want)
	}
}

// What the suspension costs, isolated. The account above is closed against a
// bid at 50 because that bid was already resting. This one differs in one
// respect: the only liquidity available when the freeze lifts is at 25, which
// is what a delayed liquidation actually faces after the market has moved.
// The fund absorbs the difference.
func TestAuditSuspendedLiquidationCostsTheFundWhatTheDelayCosts(t *testing.T) {
	const (
		perpSymbol = "ABC-PERP"
		futSymbol  = "ABC-FUT"
	)
	fundAfterCloseAt := func(t *testing.T, closePrice int64) int64 {
		t.Helper()
		clock := &testClock{now: time.Now().UnixNano()}
		ex := NewExchange(4, clock)
		perp := NewPerpFutures(perpSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
		future := NewExpiringFutures(futSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1,
			clock.now+int64(time.Minute))
		ex.AddInstrument(perp)
		ex.AddInstrument(future)
		ex.LiquidationHandler = &fundEventRecorder{}
		ex.LiquidationFeeBps = 0
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(1, "USD", USDAmount(100))
		ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))

		pm := ex.Positions.(*PositionManager)
		pm.Lock()
		pm.InjectPosition(1, perpSymbol, &Position{
			ClientID: 1, Symbol: perpSymbol, PositionSide: PositionBoth,
			Size: BTCAmount(10), EntryPrice: USDAmount(100),
		})
		pm.InjectPosition(1, futSymbol, &Position{
			ClientID: 1, Symbol: futSymbol, PositionSide: PositionBoth,
			Size: BTCAmount(1), EntryPrice: USDAmount(100),
		})
		pm.Unlock()

		clock.now += int64(2 * time.Minute)
		ex.CheckExpiries()
		future.ObserveSettlement(USDAmount(100), clock.now)
		ex.CheckExpiries()

		if _, reject := InjectLimitOrder(ex, 2, perpSymbol, Buy, closePrice, BTCAmount(10)); reject != "" {
			t.Fatalf("liquidity at %d rejected: %s", closePrice, reject)
		}
		ex.CheckLiquidations(perpSymbol, perp, closePrice)
		if pos := ex.Positions.GetPosition(1, perpSymbol); pos != nil && pos.Size != 0 {
			t.Fatalf("the position was not closed at %d: %d still open", closePrice, pos.Size)
		}
		return ex.ExchangeBalance.InsuranceFund["USD"]
	}

	atBreach := fundAfterCloseAt(t, USDAmount(50))
	afterDelay := fundAfterCloseAt(t, USDAmount(25))
	t.Logf("fund absorbs %d when the position closes at 50, %d when it closes at 25", atBreach, afterDelay)

	// Hand-derived: 10*(50-100) = -500 against 100 cash leaves 400; 10*(25-100)
	// = -750 against 100 leaves 650.
	if atBreach != -USDAmount(400) {
		t.Errorf("closing at 50 left the fund %d, want %d", atBreach, -USDAmount(400))
	}
	if afterDelay != -USDAmount(650) {
		t.Errorf("closing at 25 left the fund %d, want %d", afterDelay, -USDAmount(650))
	}
	if afterDelay >= atBreach {
		t.Errorf("delay did not increase the fund's loss: %d then %d", atBreach, afterDelay)
	}
}
