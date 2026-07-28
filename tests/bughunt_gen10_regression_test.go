package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
)

// An auto-anchored mark calculator must die with its instrument: the map is
// keyed by symbol, so a contract relisted under the same symbol would
// otherwise inherit the dead contract's seeded basis EMA and mark tick-1 off
// a basis it never had — re-creating the cascade the anchoring prevents.
func TestRegressionRelistedSymbolGetsFreshMarkCalculator(t *testing.T) {
	clock := &derivClock{now: derivStart}
	ex := newDerivExchange(t, clock) // ABC/USD spot quoted 49,900 / 50,100
	defer ex.Shutdown()
	ex.ConfigureAutomation(AutomationConfig{}) // no explicit calc -> auto-anchor

	expiry := derivStart + int64(60*time.Second)
	futA := NewExpiringFutures("ABC-FUT-1", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, expiry)
	futA.Underlying = "ABC/USD"
	ex.AddInstrument(futA)

	// Blow futA's book far above the ~50k index and let the EMA seed on it.
	gw := connectDerivTrader(ex, 7, 10_000_000)
	placeLimit(t, gw, 1, "ABC-FUT-1", Buy, PriceUSD(59_990, DOLLAR_TICK), BTC_PRECISION)
	placeLimit(t, gw, 2, "ABC-FUT-1", Sell, PriceUSD(60_010, DOLLAR_TICK), BTC_PRECISION)
	ex.UpdatePerpPrices()
	if mark := futA.Perp().GetFundingRate().MarkPrice; mark <= PriceUSD(50_000, DOLLAR_TICK) {
		t.Fatalf("futA mark %d did not move above index; test setup broken", mark)
	}

	clock.now = expiry + 1
	ex.CheckExpiries() // settles and delists ABC-FUT-1
	// Drain the two forced-cancel notifications from expiry so the next
	// placeLimit reads its own accept, not a stale notification.
	for range 2 {
		select {
		case <-gw.ResponseCh:
		case <-time.After(2 * time.Second):
			t.Fatal("expiry forced-cancel notifications never arrived")
		}
	}

	// Relist the SAME symbol, quoted exactly at the index: true basis is 0.
	futB := NewExpiringFutures("ABC-FUT-1", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, expiry+int64(time.Hour))
	futB.Underlying = "ABC/USD"
	ex.AddInstrument(futB)
	placeLimit(t, gw, 3, "ABC-FUT-1", Buy, PriceUSD(49_990, DOLLAR_TICK), BTC_PRECISION)
	placeLimit(t, gw, 4, "ABC-FUT-1", Sell, PriceUSD(50_010, DOLLAR_TICK), BTC_PRECISION)
	ex.UpdatePerpPrices()

	mark := futB.Perp().GetFundingRate().MarkPrice
	index := PriceUSD(50_000, DOLLAR_TICK)
	// A fresh calculator seeds on futB's own ~0 basis: mark stays within a
	// few ticks of the index. The stale calculator marked +2.45% off it.
	if diff := mark - index; diff > USDAmount(100) || diff < -USDAmount(100) {
		t.Fatalf("relisted contract inherited stale basis: mark %d vs index %d (diff %d)", mark, index, diff)
	}
}

// When two clients breach maintenance on the same tick and the book has
// liquidity for only one, the liquidation winner must be deterministic
// (client-ID order), not Go map iteration order.
func TestRegressionLiquidationOrderIsDeterministic(t *testing.T) {
	for run := range 10 {
		ex := NewExchange(6, &RealClock{})
		perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
		ex.AddInstrument(perp)
		handler := &fundEventRecorder{}
		ex.LiquidationHandler = handler

		// Clients 1 and 2: identical underwater positions. Client 5: liquidity
		// for exactly ONE forced close.
		for _, id := range []uint64{1, 2} {
			ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
			ex.AddPerpBalance(id, "USD", USDAmount(100))
		}
		ex.ConnectNewClient(5, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(5, "USD", USDAmount(100_000))

		pm := ex.Positions.(*PositionManager)
		pm.Lock()
		for _, id := range []uint64{1, 2} {
			pm.InjectPosition(id, "BTC-PERP", &Position{
				ClientID: id, Symbol: "BTC-PERP", PositionSide: PositionBoth,
				Size: BTCAmount(10), EntryPrice: USDAmount(100),
			})
		}
		pm.Unlock()

		if _, reject := InjectLimitOrder(ex, 5, "BTC-PERP", Buy, USDAmount(94), BTCAmount(10)); reject != "" {
			t.Fatalf("liquidity order rejected: %s", reject)
		}
		ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

		// Exactly one close filled; it must be client 1 (lowest ID), every run.
		pos1 := ex.Positions.GetPosition(1, "BTC-PERP")
		pos2 := ex.Positions.GetPosition(2, "BTC-PERP")
		if pos1 != nil && pos1.Size != 0 {
			t.Fatalf("run %d: client 1 (lowest ID) was not the liquidation winner", run)
		}
		if pos2 == nil || pos2.Size == 0 {
			t.Fatalf("run %d: client 2 also closed; expected liquidity for only one", run)
		}
	}
}

// A thin book absorbs only part of a forced close; the clearance fee must
// bill the quantity that actually executed, not the attempted position size.
func TestRegressionClearanceFeeOnPartialLiquidationBillsFilledQty(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler
	ex.LiquidationFeeBps = 30

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(100_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	// Liquidity for only 4 of the 10 BTC.
	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(94), BTCAmount(4)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("underwater position not liquidated")
	}
	wantFee := MulDiv(BTCAmount(4), USDAmount(94), BTC_PRECISION) * 30 / 10000
	overFee := MulDiv(BTCAmount(10), USDAmount(94), BTC_PRECISION) * 30 / 10000
	got := ex.ExchangeBalance.InsuranceFund["USD"]
	if got == overFee {
		t.Fatalf("clearance fee billed the full attempted size (%d); want fee on the 4 BTC that filled (%d)", overFee, wantFee)
	}
	if got != wantFee {
		t.Fatalf("insurance fund = %d, want fee on filled qty %d", got, wantFee)
	}
}

// An account whose ONLY exposure is a short option must still be liquidated:
// option books never enter the perp mark loop (marginCore is nil), so without
// the PositionMarginer sweep a pure short-vol account was unliquidatable no
// matter how far underwater — the exact hole the gen-4 wiring claimed to
// close but only closed for accounts that also held a perp position.
func TestRegressionPureOptionsAccountGetsLiquidated(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	expiry := time.Now().Add(24 * time.Hour).UnixNano()
	opt := NewEuropeanOption("BTC-1-48000-C", "BTC", "USD", "BTC/USD",
		BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, PriceUSD(48_000, DOLLAR_TICK), expiry, true)
	ex.AddInstrument(opt)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{}) // short-vol, NO perp position
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{}) // option liquidity
	ex.AddPerpBalance(1, "USD", USDAmount(5_000))
	ex.AddPerpBalance(2, "USD", USDAmount(500_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-1-48000-C", &Position{
		ClientID: 1, Symbol: "BTC-1-48000-C", PositionSide: PositionBoth,
		Size: -BTCAmount(1), EntryPrice: USDAmount(3_000),
	})
	pm.Unlock()

	// Underlying rallied: maintenance = 7.5% x $50k + $3k premium = $6,750
	// against $5,000 equity. Resting ask supplies the forced buy-back.
	opt.SetMarks(USDAmount(50_000), USDAmount(3_000))
	if _, reject := InjectLimitOrder(ex, 2, "BTC-1-48000-C", Sell, USDAmount(3_000), BTCAmount(1)); reject != "" {
		t.Fatalf("liquidity ask rejected: %s", reject)
	}

	ex.CheckPositionMarginerLiquidations()

	if handler.liquidations == 0 {
		t.Fatal("pure-options short-vol account was never liquidated: option books invisible to the risk sweep")
	}
	pos := ex.Positions.GetPosition(1, "BTC-1-48000-C")
	if pos != nil && pos.Size != 0 {
		t.Fatalf("short option position not closed: size %d", pos.Size)
	}
}

// Before the first mark tick an option has no marks; a short's maintenance
// must floor at the buy-back cost at the marked-or-entry premium instead of
// reporting zero exposure during the window.
func TestRegressionUnmarkedShortOptionStillCarriesMaintenance(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	expiry := time.Now().Add(24 * time.Hour).UnixNano()
	opt := NewEuropeanOption("BTC-1-48000-C", "BTC", "USD", "BTC/USD",
		BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, PriceUSD(48_000, DOLLAR_TICK), expiry, true)
	ex.AddInstrument(opt)
	handler := &fundEventRecorder{}
	ex.LiquidationHandler = handler

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(2_000)) // below the $3k entry-premium floor
	ex.AddPerpBalance(2, "USD", USDAmount(500_000))

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-1-48000-C", &Position{
		ClientID: 1, Symbol: "BTC-1-48000-C", PositionSide: PositionBoth,
		Size: -BTCAmount(1), EntryPrice: USDAmount(3_000),
	})
	pm.Unlock()

	// NO SetMarks: the instrument has no underlying mark yet.
	if _, reject := InjectLimitOrder(ex, 2, "BTC-1-48000-C", Sell, USDAmount(3_000), BTCAmount(1)); reject != "" {
		t.Fatalf("liquidity ask rejected: %s", reject)
	}

	ex.CheckPositionMarginerLiquidations()

	if handler.liquidations == 0 {
		t.Fatal("unmarked short option reported zero maintenance: $2,000 equity vs $3,000 buy-back floor did not liquidate")
	}
}
