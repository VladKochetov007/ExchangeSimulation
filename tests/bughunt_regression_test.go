package exchange_test

import (
	"testing"
	"time"

	. "exchange_sim/exchange"
	"exchange_sim/simulation"
)

// These are executable regressions for defects found in the July 2026 audit.
// They intentionally fail against the current implementation and should be kept
// until the corresponding behavior is corrected.

type bughuntLiquidationHandler struct {
	liquidations int
}

func (h *bughuntLiquidationHandler) OnMarginCall(*MarginCallEvent)       {}
func (h *bughuntLiquidationHandler) OnInsuranceFund(*InsuranceFundEvent) {}
func (h *bughuntLiquidationHandler) OnLiquidation(*LiquidationEvent)     { h.liquidations++ }

func bughuntBorrowExchange(t *testing.T) *Exchange {
	t.Helper()

	ex := NewExchange(2, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		CollateralFactors: map[string]float64{"USD": 1},
		MaxBorrowPerAsset: map[string]int64{"USD": USDAmount(10)},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION}),
	}); err != nil {
		t.Fatalf("EnableBorrowing: %v", err)
	}
	if err := ex.BorrowMargin(1, "USD", USDAmount(1), "regression"); err != nil {
		t.Fatalf("BorrowMargin: %v", err)
	}
	return ex
}

// A loan credits the perp wallet, but it is a liability rather than equity.
// $100 collateral + $90 loan - $60 uPnL = $40 equity, below $47 maintenance.
func TestRegressionBorrowedDebtCountsAgainstLiquidationEquity(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	handler := &bughuntLiquidationHandler{}
	ex.LiquidationHandler = handler

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000))
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		CollateralFactors: map[string]float64{"USD": 1},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := ex.BorrowMargin(1, "USD", USDAmount(90), "regression"); err != nil {
		t.Fatal(err)
	}

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(10), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(94), BTCAmount(10)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if handler.liquidations == 0 {
		t.Fatal("borrowed USD was counted as equity: economic equity is $40, below $47 maintenance")
	}
}

// A perp order carries two quote costs: initial margin (reserved) and the fill
// fee (debited from the wallet). An account funded to the last cent of margin
// cannot pay the fee, so the exchange must reject the order up front rather than
// accept it and let the fee drive the balance negative. With margin + fee funded
// the order rests and settles solvent.
func TestRegressionPerpFeesCannotExceedAvailableMargin(t *testing.T) {
	// MarginRate 1000bps (10%) + taker fee 100bps (1%) of a 50k notional =
	// 5000 margin + 500 fee.
	const margin, fee = 5_000, 500
	price, qty := USDAmount(50_000), BTCAmount(1)
	fees := &PercentageFee{MakerBps: 100, TakerBps: 100, InQuote: true}

	newExchange := func() *Exchange {
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
		ex.ConnectNewClient(1, map[string]int64{}, fees)
		ex.ConnectNewClient(2, map[string]int64{}, fees)
		return ex
	}

	t.Run("margin-only funding is rejected", func(t *testing.T) {
		ex := newExchange()
		ex.AddPerpBalance(1, "USD", USDAmount(margin))
		response := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal})
		if response.Success {
			t.Fatalf("order funded to the last cent of margin was accepted; it cannot pay the %d fee", USDAmount(fee))
		}
	})

	t.Run("margin plus fee funding settles solvent", func(t *testing.T) {
		ex := newExchange()
		ex.AddPerpBalance(1, "USD", USDAmount(margin+fee))
		ex.AddPerpBalance(2, "USD", USDAmount(margin+fee))

		if response := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: "BTC-PERP", Side: Sell, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal}); !response.Success {
			t.Fatalf("maker rejected: %s", response.Error)
		}
		if response := ex.PlaceOrder(2, &OrderRequest{RequestID: 2, Symbol: "BTC-PERP", Side: Buy, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal}); !response.Success {
			t.Fatalf("taker rejected: %s", response.Error)
		}
		for _, clientID := range []uint64{1, 2} {
			if bal := ex.Clients[clientID].PerpBalance("USD"); bal < 0 {
				t.Fatalf("client %d perp balance went negative after fees: balance=%d", clientID, bal)
			}
			if available := ex.Clients[clientID].PerpAvailable("USD"); available < 0 {
				t.Fatalf("client %d available went negative after fees: available=%d", clientID, available)
			}
		}
	})
}

type bughuntThirdAssetFee struct{}

func (bughuntThirdAssetFee) CalculateFee(FillContext) Fee {
	return Fee{Asset: "BNB", Amount: 1}
}

func TestRegressionThirdAssetFeeRequiresFunds(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	fees := bughuntThirdAssetFee{}
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(1)}, fees)
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(50_000)}, fees)

	price, qty := USDAmount(50_000), BTCAmount(1)
	sell := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal})
	buy := ex.PlaceOrder(2, &OrderRequest{RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal})
	if sell.Success && buy.Success {
		t.Fatalf("trade succeeded without either participant holding the fee asset: seller BNB=%d buyer BNB=%d", ex.Clients[1].Balances["BNB"], ex.Clients[2].Balances["BNB"])
	}
}

// Reservation already treats a nil FeeModel as a zero-fee plan. Settlement
// must preserve that contract instead of panicking when the first fill arrives.
func TestRegressionNilFeePlanDoesNotPanicOnFill(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(1)}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(50_000)}, nil)

	price, qty := USDAmount(50_000), BTCAmount(1)
	if response := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal}); !response.Success {
		t.Fatalf("maker rejected: %s", response.Error)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("crossing order panicked with a nil zero-fee plan: %v", recovered)
		}
	}()
	if response := ex.PlaceOrder(2, &OrderRequest{RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder, Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal}); !response.Success {
		t.Fatalf("zero-fee taker rejected: %s", response.Error)
	}
}

func TestRegressionSchedulerRunsCallbacksAtDueTime(t *testing.T) {
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	var observed int64 = -1
	scheduler.Schedule(int64(time.Millisecond), func() {
		observed = clock.NowUnixNano()
	})
	clock.Advance(10 * time.Millisecond)

	if observed != int64(time.Millisecond) {
		t.Fatalf("callback scheduled for 1ms observed %dns", observed)
	}
}

// A delayed response scheduled by an event at t=1ms must use t=1ms as its
// origin, and therefore arrive during the same Advance(10ms) call at t=2ms.
func TestRegressionSchedulerPreservesNestedEventCausality(t *testing.T) {
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	nestedFired := false
	scheduler.Schedule(int64(time.Millisecond), func() {
		scheduler.Schedule(clock.NowUnixNano()+int64(time.Millisecond), func() {
			nestedFired = true
		})
	})
	clock.Advance(10 * time.Millisecond)

	if !nestedFired {
		t.Fatal("event scheduled from a 1ms callback was deferred past the 10ms advance")
	}
}

func TestRegressionNegativeRepayCannotIncreaseDebtAndBalance(t *testing.T) {
	ex := bughuntBorrowExchange(t)

	err := ex.RepayMargin(1, "USD", -USDAmount(10_000))
	if err == nil {
		client := ex.Clients[1]
		t.Fatalf("negative repayment accepted: perp balance=%d borrowed=%d", client.PerpBalances["USD"], client.Borrowed["USD"])
	}
}

func TestRegressionBorrowMarginRejectsNegativeAmount(t *testing.T) {
	ex := bughuntBorrowExchange(t)

	if err := ex.BorrowMargin(1, "USD", -USDAmount(10_000), "regression"); err == nil {
		client := ex.Clients[1]
		t.Fatalf("negative borrow accepted: perp balance=%d borrowed=%d", client.PerpBalances["USD"], client.Borrowed["USD"])
	}
}

func TestRegressionPerpNetAssetSubtractsBorrowedDebt(t *testing.T) {
	ex := bughuntBorrowExchange(t)
	client := ex.Clients[1]

	snapshot := client.GetBalanceSnapshot(0)
	for _, balance := range snapshot.PerpBalances {
		if balance.Asset != "USD" {
			continue
		}
		want := client.PerpBalances["USD"] - client.Borrowed["USD"]
		if balance.NetAsset != want {
			t.Fatalf("perp NetAsset=%d, want balance minus borrowed debt=%d", balance.NetAsset, want)
		}
		return
	}
	t.Fatal("USD perp balance missing from snapshot")
}

func TestRegressionRepayUnknownClientReturnsError(t *testing.T) {
	ex := bughuntBorrowExchange(t)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("RepayMargin panicked for an unknown client: %v", recovered)
		}
	}()
	if err := ex.RepayMargin(999, "USD", USDAmount(1)); err == nil {
		t.Fatal("RepayMargin accepted an unknown client")
	}
}

func TestRegressionReplacingInstrumentDoesNotStrandReservation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	inst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(inst)
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(200)}, &FixedFee{})

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: USDAmount(100), Qty: BTCAmount(1), TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("resting order rejected: %s", response.Error)
	}
	orderID := ex.Clients[1].OrderIDs[0]

	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	if cancel := ex.CancelOrder(1, &CancelRequest{RequestID: 2, OrderID: orderID}); !cancel.Success {
		t.Fatalf("replacing an instrument lost a live order and its reservation: %s", cancel.Error)
	}
}

type bughuntCustomPerp struct{ *PerpFutures }

func TestRegressionCustomPerpetualDoesNotPanicFunding(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	ex.AddInstrument(&bughuntCustomPerp{NewPerpFutures(
		"CUSTOM-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1,
	)})

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("funding panicked for a custom Instrument reporting IsPerp: %v", recovered)
		}
	}()
	ex.CheckAndSettleFunding()
}
