package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// Independent reproduction of the risk-engine findings raised by the isolated
// performance branch, written against the scientific HEAD and driven entirely
// through the exported API. No performance code is imported or relied on here:
// each test builds the smallest account/instrument set that decides a
// liquidation and asserts the decision, so a reader can check the economics
// without reading the risk engine.
//
// Reference: research/v2-integrated-longrun-r5-risk-triage-2026-08-30.md.

// triageUnderwaterLong is the reference breach used by several tests below.
// Long 10 base at entry 100 with 100 quote of collateral, marked at 94:
//
//	uPnL        = 10 * (94 - 100)      = -60
//	equity      = 100 - 60             =  40
//	notional    = 10 * 94              = 940
//	maintenance = 940 * 500bps / 10000 =  47
//
// 40 < 47, so the account is below maintenance and must be liquidated when a
// covering bid exists.
const (
	triageEntry = 100
	triageMark  = 94
	triageQty   = 10
)

func triageInjectLong(t *testing.T, ex *DefaultExchange, clientID uint64, symbol string, qty int64) {
	t.Helper()
	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(clientID, symbol, &Position{
		ClientID: clientID, Symbol: symbol, PositionSide: PositionBoth,
		Size: BTCAmount(float64(qty)), EntryPrice: USDAmount(triageEntry),
	})
	pm.Unlock()
}

func triagePositionOpen(ex *DefaultExchange, clientID uint64, symbol string) bool {
	pos := ex.Positions.GetPosition(clientID, symbol)
	return pos != nil && pos.Size != 0
}

// triageExchange returns an exchange holding one marked perp plus the accounts
// the breach needs: `underwater` is long and below maintenance, and `covering`
// rests a bid deep enough to absorb the whole forced close.
func triageExchange(t *testing.T, underwater, covering uint64) (*DefaultExchange, *PerpFutures) {
	t.Helper()
	ex := NewExchange(6, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	ex.ConnectNewClient(underwater, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(underwater, "USD", USDAmount(triageEntry))
	triageInjectLong(t, ex, underwater, "BTC-PERP", triageQty)

	ex.ConnectNewClient(covering, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(covering, "USD", USDAmount(1_000_000))
	if _, reject := InjectLimitOrder(ex, covering, "BTC-PERP", Buy,
		USDAmount(triageMark), BTCAmount(triageQty)); reject != "" {
		t.Fatalf("covering bid rejected: %s", reject)
	}
	return ex, perp
}

// TestTriageControlUnderwaterAccountIsLiquidated establishes that the breach
// above really is a breach on this HEAD. Every finding below is a deviation
// from this control, so a control that stopped liquidating would invalidate
// the other tests rather than pass them.
func TestTriageControlUnderwaterAccountIsLiquidated(t *testing.T) {
	ex, perp := triageExchange(t, 1, 2)
	defer ex.Shutdown()

	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(triageMark))

	if triagePositionOpen(ex, 1, "BTC-PERP") {
		t.Fatal("control: account below maintenance was not liquidated")
	}
}

// TestTriageF1UnmarkedZeroExposureBookSuppressesLiquidation is finding F1.
// buildAccountMarginProfile resolves a mark for every same-quote margined book
// before it establishes whether the account holds anything in that book, so a
// book the account has no exposure to can fail the whole profile. A book the
// account does not hold contributes zero equity, zero notional and zero
// maintenance whatever its mark is, so its price is not an input to this
// account's solvency.
func TestTriageF1UnmarkedZeroExposureBookSuppressesLiquidation(t *testing.T) {
	ex, perp := triageExchange(t, 1, 2)
	defer ex.Shutdown()

	// A second USD-quoted margined book, never marked and with an empty book so
	// the one-sided reference policy cannot price it either. Client 1 holds no
	// position in it.
	unmarked := NewPerpFutures("ZZZ-PERP", "ZZZ", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(unmarked)
	if ex.Positions.GetPosition(1, "ZZZ-PERP") != nil {
		t.Fatal("fixture error: client 1 must hold no ZZZ-PERP exposure")
	}

	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(triageMark))

	if triagePositionOpen(ex, 1, "BTC-PERP") {
		t.Fatal("F1: an unmarked book the account has no exposure to suppressed its liquidation")
	}
}

// TestTriageF2ProfileFailureAbortsSweepForOtherAccounts is finding F2.
// CheckLiquidations returns rather than continues when an account's profile
// cannot be built, and accounts are visited in ascending ID order, so one
// account's genuinely unpriceable exposure cancels the liquidation decision for
// every higher-numbered account at that mark - including accounts with no
// exposure to the unpriceable instrument.
//
// The unpriceable exposure here is an option position, which is the one risk
// path that is already per-account correct: addPositionMarginerExposure reaches
// riskMark only for symbols the client actually holds. Client 1's own profile
// therefore fails for a defensible reason; client 3's disappearance is not
// defensible.
func TestTriageF2ProfileFailureAbortsSweepForOtherAccounts(t *testing.T) {
	build := func(t *testing.T, giveClient1OptionExposure bool) *DefaultExchange {
		t.Helper()
		ex, perp := triageExchange(t, 3, 5)

		option := NewEuropeanOption("ABC-OPT", "ABC", "USD", "ABC/USD",
			BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, USDAmount(100), 1<<62, true)
		ex.AddInstrument(option)

		// Client 1 sorts below the underwater client 3 and is comfortably
		// solvent on BTC-PERP; only its option leg is unpriceable.
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(1, "USD", USDAmount(1_000_000))
		triageInjectLong(t, ex, 1, "BTC-PERP", 1)
		if giveClient1OptionExposure {
			pm := ex.Positions.(*PositionManager)
			pm.Lock()
			pm.InjectPosition(1, "ABC-OPT", &Position{
				ClientID: 1, Symbol: "ABC-OPT", PositionSide: PositionBoth,
				Size: -BTCAmount(1), EntryPrice: USDAmount(1),
			})
			pm.Unlock()
		}

		ex.CheckLiquidations("BTC-PERP", perp, USDAmount(triageMark))
		return ex
	}

	// Control: the same three accounts, the same unmarked option book, but
	// nobody holds the option. Client 3 must be liquidated.
	control := build(t, false)
	defer control.Shutdown()
	if triagePositionOpen(control, 3, "BTC-PERP") {
		t.Fatal("control: underwater client 3 was not liquidated with no option exposure present")
	}

	ex := build(t, true)
	defer ex.Shutdown()
	if triagePositionOpen(ex, 3, "BTC-PERP") {
		t.Fatal("F2: a lower-ID account's unpriceable exposure skipped the liquidation decision for client 3")
	}
}

// triageFixedMark is a mark calculator that reports whatever the test last set,
// so the mark sequence under test is exactly the sequence the risk engine sees.
type triageFixedMark struct{ price int64 }

func (m *triageFixedMark) Calculate(*OrderBook) (int64, error) { return m.price, nil }

// TestTriageF3CrossMarginOutcomeDependsOnSymbolOrder is finding F3.
// updateAllPerpPrices interleaves mark application and the liquidation sweep
// per symbol, and buildAccountMarginProfile prices non-trigger symbols from
// their last stored mark. The sweep triggered by the first symbol in sort order
// therefore values every cross-margined sibling at the previous tick's mark.
//
// One account, long both legs, is solvent against the fully refreshed mark set
// in both arms:
//
//	equity      = 101 + 10*(140-100) + 10*(70-100) = 201
//	maintenance = (10*140 + 10*70) * 500bps/10000  = 105
//
// Only the order in which the two symbols sort differs between the arms, and
// instrument names carry no economic content.
func TestTriageF3CrossMarginOutcomeDependsOnSymbolOrder(t *testing.T) {
	// survives reports whether the account keeps both legs when the named
	// symbol is the one that falls. Both arms are the same economics.
	survives := func(t *testing.T, fallingSymbol string) bool {
		t.Helper()
		const (
			risePrice = 140
			fallPrice = 70
			startMark = 100
		)
		ex := NewExchange(4, &RealClock{})
		defer ex.Shutdown()

		calcs := map[string]MarkPriceCalculator{}
		perps := map[string]*PerpFutures{}
		for _, symbol := range []string{"AAA-PERP", "BBB-PERP"} {
			perp := NewPerpFutures(symbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
			ex.AddInstrument(perp)
			perps[symbol] = perp
			calcs[symbol] = &triageFixedMark{price: USDAmount(startMark)}
		}
		// An explicit calculator also turns off mark auto-anchoring, so the
		// per-symbol calculators above are the only mark source.
		ex.ConfigureAutomation(AutomationConfig{
			MarkPriceCalc:  &triageFixedMark{price: USDAmount(startMark)},
			MarkPriceCalcs: calcs,
		})

		// Warm both marks to 100 before anyone holds a position, so the stale
		// mark this test is about is a genuinely stored, available one rather
		// than the one-sided book reference the unmarked case falls back to.
		ex.UpdatePerpPrices()
		for _, symbol := range []string{"AAA-PERP", "BBB-PERP"} {
			if rate := perps[symbol].GetFundingRate(); !rate.MarkAvailable || rate.MarkPrice != USDAmount(startMark) {
				t.Fatalf("fixture error: %s mark = %d (available %v), want a stored %d",
					symbol, rate.MarkPrice, rate.MarkAvailable, USDAmount(startMark))
			}
		}

		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(1, "USD", USDAmount(101))
		for symbol := range perps {
			triageInjectLong(t, ex, 1, symbol, triageQty)
		}
		// Deep covering bids on both legs, so a liquidation that fires can fill.
		ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(2, "USD", USDAmount(10_000_000))
		for symbol := range perps {
			if _, reject := InjectLimitOrder(ex, 2, symbol, Buy,
				USDAmount(fallPrice-1), BTCAmount(triageQty*2)); reject != "" {
				t.Fatalf("covering bid on %s rejected: %s", symbol, reject)
			}
		}

		// Tick 2 moves the legs in opposite directions by the same notional.
		for symbol, calc := range calcs {
			price := float64(risePrice)
			if symbol == fallingSymbol {
				price = fallPrice
			}
			calc.(*triageFixedMark).price = USDAmount(price)
		}
		ex.UpdatePerpPrices()

		return triagePositionOpen(ex, 1, "AAA-PERP") && triagePositionOpen(ex, 1, "BBB-PERP")
	}

	fallerSortsFirst := survives(t, "AAA-PERP")
	riserSortsFirst := survives(t, "BBB-PERP")

	if fallerSortsFirst != riserSortsFirst {
		t.Fatalf("F3: renaming the legs changed the outcome - survives with faller first = %v, with riser first = %v",
			fallerSortsFirst, riserSortsFirst)
	}
}

// TestTriageF8BorrowInterestTruncationIsBounded pins finding F8 as measured
// behaviour rather than repairing it. ChargeCollateralInterest computes
// borrowed * rate * 60 / (31_536_000 * 10_000) in integer arithmetic with no
// remainder carry, so a debt below 10,512,000 units - 105.12 USD at
// USD_PRECISION - is charged nothing, every minute, forever.
//
// It is left as it stands for the registered r5 cells because the small-debt
// regime does not occur there once the run leaves warm-up. Measured over
// 6h30m of dev-607 seed 607 across three venues: 18,147 borrow events, 10,471
// interest charges totalling 1,196,203,822 units (11,962 USD), and 0 of 33
// (venue, client, asset) debts below the threshold - the smallest was
// 42,712,358 units (427 USD), four times the threshold. What remains is
// sub-unit rounding bounded below, worth at most 0.475 USD over a 24-hour run.
//
// This test therefore guards the bound, not the behaviour: if the arithmetic
// ever changes, it says what the old semantics were and what was relied on.
func TestTriageF8BorrowInterestTruncationIsBounded(t *testing.T) {
	// The exact threshold for one charged unit per minute at 500 bps:
	// ceil(365*24*3600 * 10000 / (500 * 60)).
	const threshold = 10_512_000
	const minutes = 60

	chargeOverAnHour := func(t *testing.T, debt int64) int64 {
		t.Helper()
		ex := NewExchange(2, &RealClock{})
		defer ex.Shutdown()
		ex.ConfigureAutomation(AutomationConfig{}) // CollateralRate defaults to 500 bps
		if ex.CollateralRate != 500 {
			t.Fatalf("fixture error: CollateralRate = %d, want the 500 bps default", ex.CollateralRate)
		}
		ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(1, "USD", USDAmount(10_000_000))
		ex.Lock()
		ex.Clients[1].Borrowed["USD"] = debt
		ex.Unlock()

		before := ex.Clients[1].PerpBalances["USD"]
		for range minutes {
			ex.ChargeCollateralInterest()
		}
		return before - ex.Clients[1].PerpBalances["USD"]
	}

	// The threshold itself, from both sides.
	if got := chargeOverAnHour(t, threshold); got != minutes {
		t.Fatalf("debt at the threshold charged %d over %d minutes, want %d", got, minutes, minutes)
	}
	if got := chargeOverAnHour(t, threshold-1); got != 0 {
		t.Fatalf("debt one unit below the threshold charged %d, want 0", got)
	}

	// The pinned small-debt behaviour: nothing accrues, and nothing carries.
	// The largest debt in the performance branch's 15-minute sample was
	// 3,557,722 units (35.58 USD) and sat in exactly this regime.
	const warmupScaleDebt = 3_557_722
	if got := chargeOverAnHour(t, warmupScaleDebt); got != 0 {
		t.Fatalf("small-debt regime changed: %d units charged %d over an hour, want 0 (no remainder carry)",
			warmupScaleDebt, got)
	}

	// The bound that makes it immaterial at r5 scale: the shortfall against
	// exact arithmetic is strictly less than one unit per charged minute.
	for _, debt := range []int64{42_712_358, 676_396_650, 17_799_145_124, 1_999_999_972_971} {
		charged := chargeOverAnHour(t, debt)
		// Exact per-minute interest as a rational, compared without floats.
		numerator := debt * 500 * 60
		denominator := int64(365*24*3600) * 10000
		exactPerMinute := numerator / denominator
		shortfall := minutes*numerator/denominator - charged
		if charged != minutes*exactPerMinute {
			t.Fatalf("debt %d charged %d, want %d (%d per minute)",
				debt, charged, minutes*exactPerMinute, exactPerMinute)
		}
		if shortfall < 0 || shortfall >= minutes {
			t.Fatalf("debt %d: truncation shortfall %d units over %d minutes, want strictly under one unit per minute",
				debt, shortfall, minutes)
		}
	}
}
