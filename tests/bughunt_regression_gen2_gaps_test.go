package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

func bughuntLimit(ex *Exchange, clientID, reqID uint64, symbol string, side Side, price, qty int64) Response {
	return ex.PlaceOrder(clientID, &OrderRequest{
		RequestID: reqID, Symbol: symbol, Side: side, Type: LimitOrder,
		Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal,
	})
}

func bughuntMarket(ex *Exchange, clientID, reqID uint64, symbol string, side Side, qty int64) Response {
	return ex.PlaceOrder(clientID, &OrderRequest{
		RequestID: reqID, Symbol: symbol, Side: side, Type: Market,
		Qty: qty, TimeInForce: GTC, Visibility: Normal,
	})
}

func bughuntRepayExchange(t *testing.T, spotUSD, perpUSD, borrowUSD int64) *Exchange {
	t.Helper()

	ex := NewExchange(2, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{"USD": spotUSD}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", perpUSD)
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		CollateralFactors: map[string]float64{"USD": 1},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION}),
	}); err != nil {
		t.Fatalf("EnableBorrowing: %v", err)
	}
	if err := ex.BorrowMargin(1, "USD", borrowUSD, "regression"); err != nil {
		t.Fatalf("BorrowMargin: %v", err)
	}
	return ex
}

// A borrow credits the wallet and records an equal debt, so it is
// equity-neutral: the estimated liquidation price must not move. Counting the
// loaned cash as collateral would report a safer (more distant) price than the
// account can actually sustain.
func TestRegressionEstimateLiquidationPriceNetsBorrowedDebt(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(10_000), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)
	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty); reject != "" {
		t.Fatalf("maker rejected: %s", reject)
	}
	if _, reject := InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty); reject != "" {
		t.Fatalf("taker rejected: %s", reject)
	}
	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	if pos == nil || pos.Size == 0 {
		t.Fatal("position not opened")
	}

	liqWithoutLoan := ex.EstimateLiquidationPrice(pos, 1, perp, BTC_PRECISION)
	injectBorrowing(ex, 1, "USD", USDAmount(2_000))
	liqWithLoan := ex.EstimateLiquidationPrice(pos, 1, perp, BTC_PRECISION)

	if liqWithLoan != liqWithoutLoan {
		t.Fatalf("equity-neutral loan moved liquidation price %d -> %d: borrowed cash counted as collateral",
			liqWithoutLoan, liqWithLoan)
	}

	ex.RLock()
	client := ex.Clients[1]
	ex.RUnlock()
	collateral := client.PerpBalance("USD") - client.Borrowed["USD"]
	expected := pos.EntryPrice - MulDiv(collateral, BTC_PRECISION, pos.Size)
	if liqWithLoan != expected {
		t.Fatalf("liqPrice = %d, want entry - (balance - debt) * precision / size = %d", liqWithLoan, expected)
	}
}

// checkMarketOrderFunds (Margined branch) must demand margin plus the
// worst-case taker fee: a market order funded to the last cent of margin fills
// and then cannot pay the fee.
func TestRegressionPerpMarketOrderRequiresFeeHeadroom(t *testing.T) {
	const marginUSD, feeUSD = 5_000, 500
	price, qty := USDAmount(50_000), BTCAmount(1)
	fees := &PercentageFee{MakerBps: 100, TakerBps: 100, InQuote: true}

	newPerpMarket := func(t *testing.T, takerUSD int64) *Exchange {
		t.Helper()
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
		ex.ConnectNewClient(1, map[string]int64{}, fees)
		ex.ConnectNewClient(2, map[string]int64{}, fees)
		ex.AddPerpBalance(1, "USD", takerUSD)
		ex.AddPerpBalance(2, "USD", USDAmount(100_000))
		if response := bughuntLimit(ex, 2, 1, "BTC-PERP", Sell, price, qty); !response.Success {
			t.Fatalf("maker rejected: %s", response.Error)
		}
		return ex
	}

	t.Run("margin-only funding is rejected", func(t *testing.T) {
		ex := newPerpMarket(t, USDAmount(marginUSD))
		response := bughuntMarket(ex, 1, 2, "BTC-PERP", Buy, qty)
		if response.Success {
			t.Fatalf("market order funded to the last cent of margin was accepted; it cannot pay the %d fee", USDAmount(feeUSD))
		}
		if response.Error != RejectInsufficientBalance {
			t.Fatalf("reject reason = %s, want %s", response.Error, RejectInsufficientBalance)
		}
	})

	t.Run("margin plus fee funding fills solvent", func(t *testing.T) {
		ex := newPerpMarket(t, USDAmount(marginUSD+feeUSD))
		if response := bughuntMarket(ex, 1, 2, "BTC-PERP", Buy, qty); !response.Success {
			t.Fatalf("fully funded market order rejected: %s", response.Error)
		}
		if pos := ex.Positions.GetPosition(1, "BTC-PERP"); pos == nil || pos.Size != qty {
			t.Fatalf("position after market fill: %+v, want size %d", pos, qty)
		}
		if balance := ex.Clients[1].PerpBalance("USD"); balance != USDAmount(marginUSD) {
			t.Fatalf("taker balance after fee = %d, want %d", balance, USDAmount(marginUSD))
		}
		if available := ex.Clients[1].PerpAvailable("USD"); available < 0 {
			t.Fatalf("taker available went negative after fees: %d", available)
		}
	})
}

// reserveLimitOrderFunds (OrderMarginer branch) must reserve the option
// premium plus the worst-case fee; both are debited from the perp wallet.
func TestRegressionOptionLimitPremiumRequiresFeeHeadroom(t *testing.T) {
	const premiumUSD, feeUSD = 3_000, 30
	premium := USDAmount(premiumUSD)
	fees := &PercentageFee{MakerBps: 100, TakerBps: 100, InQuote: true}

	newOptionExchange := func(buyerUSD int64) *Exchange {
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewEuropeanOption("TST-48000-C", "TST", "USD", "TST/USD",
			BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, PriceUSD(48_000, DOLLAR_TICK), int64(1)<<62, true))
		ex.ConnectNewClient(1, nil, fees)
		ex.AddPerpBalance(1, "USD", buyerUSD)
		return ex
	}

	t.Run("premium-only funding is rejected", func(t *testing.T) {
		ex := newOptionExchange(USDAmount(premiumUSD))
		response := bughuntLimit(ex, 1, 1, "TST-48000-C", Buy, premium, BTCAmount(1))
		if response.Success {
			t.Fatalf("buy funded to the last cent of premium was accepted; it cannot pay the %d fee", USDAmount(feeUSD))
		}
	})

	t.Run("premium plus fee funding reserves both", func(t *testing.T) {
		ex := newOptionExchange(USDAmount(premiumUSD + feeUSD))
		response := bughuntLimit(ex, 1, 1, "TST-48000-C", Buy, premium, BTCAmount(1))
		if !response.Success {
			t.Fatalf("fully funded buy rejected: %s", response.Error)
		}
		if got := ex.Clients[1].PerpReserved["USD"]; got != USDAmount(premiumUSD+feeUSD) {
			t.Fatalf("reserved %d, want premium plus fee = %d", got, USDAmount(premiumUSD+feeUSD))
		}
	})
}

// checkMarketOrderFunds (OrderMarginer branch) must demand premium plus the
// worst-case fee for an option market buy.
func TestRegressionOptionMarketOrderRequiresFeeHeadroom(t *testing.T) {
	const premiumUSD, feeUSD = 3_000, 30
	premium := USDAmount(premiumUSD)
	fees := &PercentageFee{MakerBps: 100, TakerBps: 100, InQuote: true}

	newOptionMarket := func(t *testing.T, buyerUSD int64) *Exchange {
		t.Helper()
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewEuropeanOption("TST-48000-C", "TST", "USD", "TST/USD",
			BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, PriceUSD(48_000, DOLLAR_TICK), int64(1)<<62, true))
		ex.ConnectNewClient(1, nil, fees)
		ex.ConnectNewClient(2, nil, fees)
		ex.AddPerpBalance(1, "USD", buyerUSD)
		ex.AddPerpBalance(2, "USD", USDAmount(10_000))
		if response := bughuntLimit(ex, 2, 1, "TST-48000-C", Sell, premium, BTCAmount(1)); !response.Success {
			t.Fatalf("seller rejected: %s", response.Error)
		}
		return ex
	}

	t.Run("premium-only funding is rejected", func(t *testing.T) {
		ex := newOptionMarket(t, USDAmount(premiumUSD))
		response := bughuntMarket(ex, 1, 2, "TST-48000-C", Buy, BTCAmount(1))
		if response.Success {
			t.Fatalf("market buy funded to the last cent of premium was accepted; it cannot pay the %d fee", USDAmount(feeUSD))
		}
	})

	t.Run("premium plus fee funding fills solvent", func(t *testing.T) {
		ex := newOptionMarket(t, USDAmount(premiumUSD+feeUSD))
		if response := bughuntMarket(ex, 1, 2, "TST-48000-C", Buy, BTCAmount(1)); !response.Success {
			t.Fatalf("fully funded market buy rejected: %s", response.Error)
		}
		if pos := ex.Positions.GetPosition(1, "TST-48000-C"); pos == nil || pos.Size != BTCAmount(1) {
			t.Fatalf("position after market fill: %+v, want size %d", pos, BTCAmount(1))
		}
		if balance := ex.Clients[1].PerpBalance("USD"); balance != 0 {
			t.Fatalf("buyer balance after premium and fee = %d, want exactly 0", balance)
		}
	})
}

// checkForeignFeeFunds on a margined instrument must look at the PERP wallet:
// perp fees settle there, so a BNB-denominated fee needs perp BNB.
func TestRegressionForeignFeeOnPerpRequiresPerpFeeBalance(t *testing.T) {
	price, qty := USDAmount(50_000), BTCAmount(1)
	fees := bughuntThirdAssetFee{}

	newForeignFeePerp := func() *Exchange {
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
		ex.ConnectNewClient(1, map[string]int64{}, fees)
		ex.ConnectNewClient(2, map[string]int64{}, fees)
		ex.AddPerpBalance(1, "USD", USDAmount(100_000))
		ex.AddPerpBalance(2, "USD", USDAmount(100_000))
		return ex
	}

	t.Run("limit order without the fee asset is rejected", func(t *testing.T) {
		ex := newForeignFeePerp()
		response := bughuntLimit(ex, 1, 1, "BTC-PERP", Sell, price, qty)
		if response.Success {
			t.Fatal("perp order accepted with zero balance in the BNB fee asset")
		}
		if response.Error != RejectInsufficientBalance {
			t.Fatalf("reject reason = %s, want %s", response.Error, RejectInsufficientBalance)
		}
	})

	t.Run("market order without the fee asset is rejected", func(t *testing.T) {
		ex := newForeignFeePerp()
		ex.AddPerpBalance(2, "BNB", 10)
		if response := bughuntLimit(ex, 2, 1, "BTC-PERP", Sell, price, qty); !response.Success {
			t.Fatalf("funded maker rejected: %s", response.Error)
		}
		if response := bughuntMarket(ex, 1, 2, "BTC-PERP", Buy, qty); response.Success {
			t.Fatal("perp market order accepted with zero balance in the BNB fee asset")
		}
	})

	t.Run("orders with the fee asset in the perp wallet trade and pay it", func(t *testing.T) {
		ex := newForeignFeePerp()
		ex.AddPerpBalance(1, "BNB", 10)
		ex.AddPerpBalance(2, "BNB", 10)
		if response := bughuntLimit(ex, 1, 1, "BTC-PERP", Sell, price, qty); !response.Success {
			t.Fatalf("funded maker rejected: %s", response.Error)
		}
		if response := bughuntLimit(ex, 2, 2, "BTC-PERP", Buy, price, qty); !response.Success {
			t.Fatalf("funded taker rejected: %s", response.Error)
		}
		for _, clientID := range []uint64{1, 2} {
			if got := ex.Clients[clientID].PerpBalances["BNB"]; got != 9 {
				t.Fatalf("client %d perp BNB after fill = %d, want 9", clientID, got)
			}
		}
	})
}

// The foreign-fee pre-check must not over-reject: clients holding the fee
// asset trade normally and settlement debits exactly the fee.
func TestRegressionSpotForeignFeeWithFundsTradesAndDebits(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	fees := bughuntThirdAssetFee{}
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(1), "BNB": 5}, fees)
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(50_000), "BNB": 5}, fees)

	price, qty := USDAmount(50_000), BTCAmount(1)
	if response := bughuntLimit(ex, 1, 1, "BTC/USD", Sell, price, qty); !response.Success {
		t.Fatalf("funded seller rejected: %s", response.Error)
	}
	if response := bughuntLimit(ex, 2, 2, "BTC/USD", Buy, price, qty); !response.Success {
		t.Fatalf("funded buyer rejected: %s", response.Error)
	}
	for _, clientID := range []uint64{1, 2} {
		if got := ex.Clients[clientID].Balances["BNB"]; got != 4 {
			t.Fatalf("client %d spot BNB after fill = %d, want 4", clientID, got)
		}
	}
}

// Foreign fee checks must reserve, not merely inspect, the fee wallet. Two
// resting orders with one BNB cannot both be accepted when each fill charges
// one BNB: otherwise the second fill debits BNB below zero.
func TestRegressionForeignFeeReservationsPreventOvercommit(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	fees := bughuntThirdAssetFee{}
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(2), "BNB": 1}, fees)
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(50_000)}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"USD": USDAmount(50_000)}, &FixedFee{})

	price, qty := USDAmount(50_000), BTCAmount(1)
	if response := bughuntLimit(ex, 1, 1, "BTC/USD", Sell, price, qty); !response.Success {
		t.Fatalf("first order rejected: %s", response.Error)
	}
	if response := bughuntLimit(ex, 1, 2, "BTC/USD", Sell, price, qty); response.Success {
		t.Fatal("second order accepted although its BNB fee was already committed")
	}

	if response := bughuntLimit(ex, 2, 3, "BTC/USD", Buy, price, qty); !response.Success {
		t.Fatalf("funded buyer rejected: %s", response.Error)
	}
	if got := ex.Clients[1].Balances["BNB"]; got != 0 {
		t.Fatalf("seller BNB after one fill = %d, want 0", got)
	}
	if got := ex.Clients[1].Reserved["BNB"]; got != 0 {
		t.Fatalf("seller BNB reservation after fill = %d, want 0", got)
	}
}

func TestRegressionMarketForeignFeeReservesEverySweepFill(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	fees := bughuntThirdAssetFee{}
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100_000), "BNB": 1}, fees)
	ex.ConnectNewClient(2, map[string]int64{"BTC": BTCAmount(1)}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"BTC": BTCAmount(1)}, &FixedFee{})

	if response := bughuntLimit(ex, 2, 1, "BTC/USD", Sell, USDAmount(50_000), BTCAmount(1)); !response.Success {
		t.Fatalf("first maker rejected: %s", response.Error)
	}
	if response := bughuntLimit(ex, 3, 2, "BTC/USD", Sell, USDAmount(50_001), BTCAmount(1)); !response.Success {
		t.Fatalf("second maker rejected: %s", response.Error)
	}
	if response := bughuntMarket(ex, 1, 3, "BTC/USD", Buy, BTCAmount(2)); response.Success {
		t.Fatal("market sweep accepted with one BNB despite two charged fills")
	}
	if got := ex.Clients[1].Balances["BNB"]; got != 1 {
		t.Fatalf("rejected market order changed BNB balance to %d, want 1", got)
	}
}

// Liquidation repayment comes from the perp wallet. Once it pays all
// perp-attributed debt, any excess must retire spot-attributed debt too;
// otherwise a later perp borrow is hidden from margin equity.
func TestRegressionLiquidationRepayPreservesDebtAttribution(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(200), USDAmount(10_000))
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		CollateralFactors: map[string]float64{"USD": 1},
		AssetPrecisions:   map[string]int64{"USD": USD_PRECISION},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": USD_PRECISION}),
	}); err != nil {
		t.Fatalf("EnableBorrowing: %v", err)
	}

	ex.Lock()
	client := ex.Clients[1]
	client.Balances["USD"] = USDAmount(1_000)
	client.Borrowed["USD"] = USDAmount(200)
	client.BorrowedSpot["USD"] = USDAmount(100)
	ex.Unlock()

	pm := ex.Positions.(*PositionManager)
	pm.Lock()
	pm.InjectPosition(1, "BTC-PERP", &Position{
		ClientID: 1, Symbol: "BTC-PERP", PositionSide: PositionBoth,
		Size: BTCAmount(12), EntryPrice: USDAmount(100),
	})
	pm.Unlock()

	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, USDAmount(94), BTCAmount(12)); reject != "" {
		t.Fatalf("liquidity order rejected: %s", reject)
	}
	ex.CheckLiquidations("BTC-PERP", perp, USDAmount(94))

	if got, want := client.Borrowed["USD"], USDAmount(72); got != want {
		t.Fatalf("debt after liquidation repayment = %d, want %d", got, want)
	}
	if got, want := client.BorrowedSpot["USD"], USDAmount(72); got != want {
		t.Fatalf("spot-attributed debt after excess perp repayment = %d, want %d", got, want)
	}
	if err := ex.BorrowMargin(1, "USD", USDAmount(10), "regression reborrow"); err != nil {
		t.Fatalf("perp reborrow: %v", err)
	}
	if got, want := client.BorrowedPerpPortion("USD"), USDAmount(10); got != want {
		t.Fatalf("perp debt after reborrow = %d, want %d", got, want)
	}
}

func TestRegressionExchangeShutdownIsTerminalAndIdempotent(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(1)}, &FixedFee{})
	ex.Shutdown()
	ex.Shutdown()

	if ex.IsRunning() {
		t.Fatal("exchange reports running after shutdown")
	}
	if gateway := ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(1)}, &FixedFee{}); gateway.IsRunning() {
		t.Fatal("shutdown exchange accepted a new live gateway")
	}
}

func TestRegressionBorrowMarginRejectsZeroAmount(t *testing.T) {
	ex := bughuntBorrowExchange(t)
	client := ex.Clients[1]
	perpBefore, debtBefore := client.PerpBalances["USD"], client.Borrowed["USD"]

	if err := ex.BorrowMargin(1, "USD", 0, "regression"); err == nil {
		t.Fatal("zero-amount borrow accepted")
	}
	if client.PerpBalances["USD"] != perpBefore || client.Borrowed["USD"] != debtBefore {
		t.Fatalf("rejected zero borrow mutated state: perp %d -> %d, borrowed %d -> %d",
			perpBefore, client.PerpBalances["USD"], debtBefore, client.Borrowed["USD"])
	}
}

func TestRegressionRepayMarginRejectsZeroAmount(t *testing.T) {
	ex := bughuntBorrowExchange(t)
	client := ex.Clients[1]
	perpBefore, debtBefore := client.PerpBalances["USD"], client.Borrowed["USD"]

	if err := ex.RepayMargin(1, "USD", 0); err == nil {
		t.Fatal("zero-amount repay accepted as a silent no-op")
	}
	if client.PerpBalances["USD"] != perpBefore || client.Borrowed["USD"] != debtBefore {
		t.Fatalf("rejected zero repay mutated state: perp %d -> %d, borrowed %d -> %d",
			perpBefore, client.PerpBalances["USD"], debtBefore, client.Borrowed["USD"])
	}
}

// The historical margin-repay path must be preserved: when the perp wallet can
// cover the repayment, it pays, and the spot wallet is untouched.
func TestRegressionRepayPrefersPerpWalletWhenBothFunded(t *testing.T) {
	ex := bughuntRepayExchange(t, USDAmount(50), USDAmount(100), USDAmount(10))
	client := ex.Clients[1]

	if err := ex.RepayMargin(1, "USD", USDAmount(10)); err != nil {
		t.Fatalf("repay with both wallets funded failed: %v", err)
	}
	if client.PerpBalances["USD"] != USDAmount(100) {
		t.Fatalf("perp wallet after repay = %d, want %d", client.PerpBalances["USD"], USDAmount(100))
	}
	if client.Balances["USD"] != USDAmount(50) {
		t.Fatalf("spot wallet touched by a perp-funded repay: %d, want %d", client.Balances["USD"], USDAmount(50))
	}
	if client.Borrowed["USD"] != 0 {
		t.Fatalf("debt not cleared: %d", client.Borrowed["USD"])
	}
}

// When the combined available across both wallets cannot cover the repay, the
// call must fail without touching any balance or the debt.
func TestRegressionRepayInsufficientInBothWalletsIsAtomic(t *testing.T) {
	ex := bughuntRepayExchange(t, USDAmount(3), USDAmount(100), USDAmount(10))
	ex.Lock()
	ex.Clients[1].PerpReserved["USD"] = USDAmount(108)
	ex.Unlock()
	client := ex.Clients[1]

	if err := ex.RepayMargin(1, "USD", USDAmount(6)); err == nil {
		t.Fatal("repay of 6 succeeded with only 2 available in perp and 3 in spot")
	}
	if client.Borrowed["USD"] != USDAmount(10) {
		t.Fatalf("failed repay changed debt: %d, want %d", client.Borrowed["USD"], USDAmount(10))
	}
	if client.PerpBalances["USD"] != USDAmount(110) || client.Balances["USD"] != USDAmount(3) {
		t.Fatalf("failed repay moved balances: perp=%d spot=%d, want 110/3 in USD units",
			client.PerpBalances["USD"], client.Balances["USD"])
	}
}

// Repay may split the debit across wallets — perp first, spot for the
// remainder — so debt is never stuck while the account holds enough cash
// spread over the two wallets.
func TestRegressionRepaySplitsAcrossWallets(t *testing.T) {
	ex := bughuntRepayExchange(t, USDAmount(3), USDAmount(100), USDAmount(10))
	ex.Lock()
	ex.Clients[1].PerpReserved["USD"] = USDAmount(108)
	ex.Unlock()
	client := ex.Clients[1]

	if err := ex.RepayMargin(1, "USD", USDAmount(5)); err != nil {
		t.Fatalf("split repay of 5 failed with 2+3 available across wallets: %v", err)
	}
	if client.Borrowed["USD"] != USDAmount(5) {
		t.Fatalf("debt after split repay: %d, want %d", client.Borrowed["USD"], USDAmount(5))
	}
	if client.PerpBalances["USD"] != USDAmount(108) || client.Balances["USD"] != 0 {
		t.Fatalf("split repay balances: perp=%d spot=%d, want 108/0 in USD units",
			client.PerpBalances["USD"], client.Balances["USD"])
	}
}

func TestRegressionPerpSnapshotReportsBorrowedAmount(t *testing.T) {
	ex := bughuntBorrowExchange(t)
	client := ex.Clients[1]

	snapshot := client.GetBalanceSnapshot(0)
	for _, balance := range snapshot.PerpBalances {
		if balance.Asset != "USD" {
			continue
		}
		if balance.Borrowed != client.Borrowed["USD"] {
			t.Fatalf("perp snapshot Borrowed = %d, want %d", balance.Borrowed, client.Borrowed["USD"])
		}
		return
	}
	t.Fatal("USD perp balance missing from snapshot")
}

// AddInstrument on an existing symbol must be a no-op: the live book (with its
// resting orders) and the originally listed instrument both stay in place.
func TestRegressionReAddInstrumentKeepsOriginalBook(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	original := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(original)
	originalBook := ex.GetBook("BTC/USD")

	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))

	book := ex.GetBook("BTC/USD")
	if book != originalBook {
		t.Fatal("re-listing an existing symbol swapped in a fresh order book")
	}
	if book.Instrument != Instrument(original) {
		t.Fatal("re-listing an existing symbol replaced the live instrument")
	}
}

// Settlement computes a fee for BOTH sides of a fill; the maker call site must
// treat a nil FeePlan as zero-fee just like the taker call site.
func TestRegressionNilFeePlanMakerDoesNotPanicOnFill(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(1, map[string]int64{"BTC": BTCAmount(1)}, nil)
	ex.ConnectNewClient(2, map[string]int64{"USD": USDAmount(50_000)}, &FixedFee{})

	price, qty := USDAmount(50_000), BTCAmount(1)
	if response := bughuntLimit(ex, 1, 1, "BTC/USD", Sell, price, qty); !response.Success {
		t.Fatalf("nil-fee maker rejected: %s", response.Error)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("fill against a nil-fee-plan maker panicked: %v", recovered)
		}
	}()
	if response := bughuntLimit(ex, 2, 2, "BTC/USD", Buy, price, qty); !response.Success {
		t.Fatalf("taker rejected: %s", response.Error)
	}
}
