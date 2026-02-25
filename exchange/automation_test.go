package exchange

import (
	"testing"
)

// setupPerpExchange creates an exchange with a BTC-PERP instrument and two connected clients.
// Client 1 receives perpUSD in the perp wallet. Client 2 receives perpUSD as liquidity provider.
// Both clients use zero-fee plans so arithmetic in tests is exact.
func setupPerpExchange(client1PerpUSD, client2PerpUSD int64) (*Exchange, *PerpFutures) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(perp)

	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{}, &FixedFee{})

	if client1PerpUSD > 0 {
		ex.AddPerpBalance(1, "USD", client1PerpUSD)
	}
	if client2PerpUSD > 0 {
		ex.AddPerpBalance(2, "USD", client2PerpUSD)
	}

	return ex, perp
}

// injectBorrowing directly sets Borrowed and PerpBalances to mirror what BorrowMargin does,
// without needing a price oracle or full BorrowingConfig setup.
func injectBorrowing(ex *Exchange, clientID uint64, asset string, amount int64) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	client := ex.Clients[clientID]
	client.Borrowed[asset] += amount
	client.PerpBalances[asset] += amount
}

// TestChargeCollateralInterest_DebitsPerpWallet verifies that interest is charged
// from the perp wallet, leaving the spot wallet completely unchanged.
func TestChargeCollateralInterest_DebitsPerpWallet(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(100_000))

	spotDeposit := USDAmount(50_000)
	ex.ConnectClient(3, map[string]int64{"USD": spotDeposit}, &FixedFee{})

	borrowedAmount := USDAmount(10_000)
	injectBorrowing(ex, 3, "USD", borrowedAmount)

	perpBefore := ex.Clients[3].PerpBalances["USD"]
	spotBefore := ex.Clients[3].Balances["USD"]

	automation := NewExchangeAutomation(ex, AutomationConfig{CollateralRate: 500})
	automation.chargeCollateralInterest()

	perpAfter := ex.Clients[3].PerpBalances["USD"]
	spotAfter := ex.Clients[3].Balances["USD"]

	if perpAfter >= perpBefore {
		t.Errorf("perp balance should decrease after interest charge: before=%d, after=%d", perpBefore, perpAfter)
	}
	if spotAfter != spotBefore {
		t.Errorf("spot balance must not change during interest charge: before=%d, after=%d", spotBefore, spotAfter)
	}
}

// TestChargeCollateralInterest_FeeRevenueReceivesInterest verifies that the amount
// debited from the client's perp wallet goes to ExchangeBalance.FeeRevenue.
func TestChargeCollateralInterest_FeeRevenueReceivesInterest(t *testing.T) {
	ex, _ := setupPerpExchange(0, 0)

	borrowedAmount := USDAmount(10_000)
	injectBorrowing(ex, 1, "USD", borrowedAmount)

	feeRevenueBefore := ex.ExchangeBalance.FeeRevenue["USD"]
	perpBefore := ex.Clients[1].PerpBalances["USD"]

	automation := NewExchangeAutomation(ex, AutomationConfig{CollateralRate: 500})
	automation.chargeCollateralInterest()

	perpAfter := ex.Clients[1].PerpBalances["USD"]
	feeRevenueAfter := ex.ExchangeBalance.FeeRevenue["USD"]

	interestCharged := perpBefore - perpAfter
	feeRevenueDelta := feeRevenueAfter - feeRevenueBefore

	if interestCharged <= 0 {
		t.Fatalf("expected positive interest charge, got perp delta=%d", interestCharged)
	}
	if feeRevenueDelta != interestCharged {
		t.Errorf("fee revenue delta (%d) must equal interest charged (%d)", feeRevenueDelta, interestCharged)
	}
}

// TestChargeCollateralInterest_Conservation verifies that perp_balance + fee_revenue
// is invariant across an interest charge cycle.
func TestChargeCollateralInterest_Conservation(t *testing.T) {
	ex, _ := setupPerpExchange(0, USDAmount(500_000))

	borrowedAmount := USDAmount(50_000)
	injectBorrowing(ex, 1, "USD", borrowedAmount)

	totalBefore := totalMoney(ex, "USD")

	automation := NewExchangeAutomation(ex, AutomationConfig{CollateralRate: 500})
	automation.chargeCollateralInterest()

	totalAfter := totalMoney(ex, "USD")

	if totalBefore != totalAfter {
		t.Errorf("money not conserved after interest charge: before=%d, after=%d, delta=%d",
			totalBefore, totalAfter, totalAfter-totalBefore)
	}
}

// TestChargeCollateralInterest_ZeroRateChargesNothing verifies that a zero collateral
// rate results in no interest being charged.
func TestChargeCollateralInterest_ZeroRateChargesNothing(t *testing.T) {
	ex, _ := setupPerpExchange(0, 0)

	borrowedAmount := USDAmount(100_000)
	injectBorrowing(ex, 1, "USD", borrowedAmount)

	perpBefore := ex.Clients[1].PerpBalances["USD"]
	feeBefore := ex.ExchangeBalance.FeeRevenue["USD"]

	automation := NewExchangeAutomation(ex, AutomationConfig{CollateralRate: 1})
	automation.collateralRate = 0
	automation.chargeCollateralInterest()

	if ex.Clients[1].PerpBalances["USD"] != perpBefore {
		t.Errorf("perp balance changed with zero rate: before=%d, after=%d",
			perpBefore, ex.Clients[1].PerpBalances["USD"])
	}
	if ex.ExchangeBalance.FeeRevenue["USD"] != feeBefore {
		t.Errorf("fee revenue changed with zero rate: before=%d, after=%d",
			feeBefore, ex.ExchangeBalance.FeeRevenue["USD"])
	}
}

// TestChargeCollateralInterest_ProportionalToBorrowed verifies that interest scales
// linearly with the borrowed amount: doubling borrowed doubles interest charged.
func TestChargeCollateralInterest_ProportionalToBorrowed(t *testing.T) {
	ex1, _ := setupPerpExchange(0, 0)
	ex2, _ := setupPerpExchange(0, 0)

	injectBorrowing(ex1, 1, "USD", USDAmount(10_000))
	injectBorrowing(ex2, 1, "USD", USDAmount(20_000))

	ex1.Clients[1].PerpBalances["USD"] = USDAmount(10_000)
	ex2.Clients[1].PerpBalances["USD"] = USDAmount(20_000)

	auto1 := NewExchangeAutomation(ex1, AutomationConfig{CollateralRate: 500})
	auto2 := NewExchangeAutomation(ex2, AutomationConfig{CollateralRate: 500})

	auto1.chargeCollateralInterest()
	auto2.chargeCollateralInterest()

	interest1 := ex1.ExchangeBalance.FeeRevenue["USD"]
	interest2 := ex2.ExchangeBalance.FeeRevenue["USD"]

	if interest1 == 0 {
		t.Fatal("expected non-zero interest for non-zero borrow")
	}
	if interest2 != 2*interest1 {
		t.Errorf("interest should be proportional to borrowed: 10k->%d, 20k->%d (expected 2x)", interest1, interest2)
	}
}

// TestChargeCollateralInterest_NoBorrowNoCharge verifies that clients with no
// outstanding debt are not charged any interest.
func TestChargeCollateralInterest_NoBorrowNoCharge(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(100_000))

	perpBefore1 := ex.Clients[1].PerpBalances["USD"]
	perpBefore2 := ex.Clients[2].PerpBalances["USD"]

	automation := NewExchangeAutomation(ex, AutomationConfig{CollateralRate: 500})
	automation.chargeCollateralInterest()

	if ex.Clients[1].PerpBalances["USD"] != perpBefore1 {
		t.Errorf("client 1 without borrow should not be charged: before=%d, after=%d",
			perpBefore1, ex.Clients[1].PerpBalances["USD"])
	}
	if ex.Clients[2].PerpBalances["USD"] != perpBefore2 {
		t.Errorf("client 2 without borrow should not be charged: before=%d, after=%d",
			perpBefore2, ex.Clients[2].PerpBalances["USD"])
	}
}

// TestLiquidation_AtMaintenanceMarginBoundary verifies that a position exactly at
// the maintenance margin threshold is liquidated. The position entry is at $50k,
// maintenance margin is 500 bps (5%). A 5% adverse move takes equity exactly to
// the maintenance boundary, triggering liquidation.
func TestLiquidation_AtMaintenanceMarginBoundary(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(6_000), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty)

	initialTotal := totalMoney(ex, "USD")

	// MaintenanceMarginRate = 500 bps = 5%.
	// Initial margin = 1 BTC * $50000 * 10% = $5000.
	// marginRatio = equity * 10000 / initMargin < 500 → liquidation.
	// At $46k: unrealizedPnL = -$4000; with $6k balance, equity = $2000.
	// marginRatio = 2000 * 10000 / 5000 = 4000 bps > 500, so not yet liquidated.
	// At $43k: unrealizedPnL = -$7000; equity = $6k - $7k = -$1000 (negative) → liquidation.
	// Use $44k as a price that triggers maintenance margin breach.
	crashPrice := PriceUSD(44_000, DOLLAR_TICK)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, crashPrice, qty)

	automation := NewExchangeAutomation(ex, AutomationConfig{})
	automation.checkLiquidations("BTC-PERP", perp, crashPrice)

	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	positionSize := int64(0)
	if pos != nil {
		positionSize = pos.Size
	}
	if positionSize != 0 {
		t.Errorf("position should be zero after liquidation at maintenance boundary, got size=%d", positionSize)
	}

	afterTotal := totalMoney(ex, "USD")
	if initialTotal != afterTotal {
		t.Errorf("money not conserved during boundary liquidation: before=%d, after=%d, delta=%d",
			initialTotal, afterTotal, afterTotal-initialTotal)
	}
}

// TestLiquidation_InsuranceFundAbsorbsDeficit verifies that when liquidation proceeds
// cannot cover the client's position loss, InsuranceFund goes negative (absorbs the
// deficit) and the client's perp balances are zeroed out.
// Client 1 has just enough margin to open a 1 BTC long at $50k (10% = $5000 required,
// client has $5200). A crash to $35k realizes a -$15k loss against $5.2k balance →
// deficit of ~$9.8k which the insurance fund must cover.
func TestLiquidation_InsuranceFundAbsorbsDeficit(t *testing.T) {
	// $5200: enough to open 1 BTC at $50k with 10% margin ($5000 needed), thin buffer.
	ex, perp := setupPerpExchange(USDAmount(5_200), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty)

	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	if pos == nil || pos.Size == 0 {
		t.Fatal("position not opened: check that client1 has enough margin for 1 BTC at $50k")
	}

	initialTotal := totalMoney(ex, "USD")

	// Crash to $35k: loss = $15k >> $5.2k balance → deep deficit.
	crashPrice := PriceUSD(35_000, DOLLAR_TICK)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, crashPrice, qty)

	automation := NewExchangeAutomation(ex, AutomationConfig{})
	automation.checkLiquidations("BTC-PERP", perp, crashPrice)

	client1 := ex.Clients[1]
	if client1.PerpBalances["USD"] != 0 {
		t.Errorf("client perp balance should be zeroed after insolvent liquidation, got=%d",
			client1.PerpBalances["USD"])
	}
	if client1.PerpReserved["USD"] != 0 {
		t.Errorf("client perp reserved should be zeroed after insolvent liquidation, got=%d",
			client1.PerpReserved["USD"])
	}

	insuranceFund := ex.ExchangeBalance.InsuranceFund["USD"]
	if insuranceFund >= 0 {
		t.Errorf("insurance fund should go negative to absorb deficit, got=%d", insuranceFund)
	}

	afterTotal := totalMoney(ex, "USD")
	if initialTotal != afterTotal {
		t.Errorf("money not conserved during deficit liquidation: before=%d, after=%d, delta=%d",
			initialTotal, afterTotal, afterTotal-initialTotal)
	}
}

// TestLiquidation_PartialFillConservation verifies that when there is insufficient
// counterparty liquidity to close the entire position, the liquidation settles
// only the filled portion without creating or destroying money.
func TestLiquidation_PartialFillConservation(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(6_000), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	fullQty := BTCAmount(1.0)
	partialLiquidityQty := BTCAmount(0.3) // counterparty provides only 0.3 BTC

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, fullQty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, fullQty)

	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	if pos == nil || pos.Size == 0 {
		t.Fatal("position not opened: setup error")
	}

	initialTotal := totalMoney(ex, "USD")

	// Provide only partial liquidity at crash price — liquidation fills partially.
	crashPrice := PriceUSD(44_000, DOLLAR_TICK)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, crashPrice, partialLiquidityQty)

	automation := NewExchangeAutomation(ex, AutomationConfig{})
	automation.checkLiquidations("BTC-PERP", perp, crashPrice)

	afterTotal := totalMoney(ex, "USD")
	if initialTotal != afterTotal {
		t.Errorf("money not conserved during partial liquidation: before=%d, after=%d, delta=%d",
			initialTotal, afterTotal, afterTotal-initialTotal)
	}
}

// TestEstimateLiquidationPrice_Long verifies the formula for a long position:
// liqPrice = entryPrice - available * precision / size
func TestEstimateLiquidationPrice_Long(t *testing.T) {
	// $10k balance: $5k used as initial margin (10% of 1 BTC at $50k), $5k remains available.
	ex, perp := setupPerpExchange(USDAmount(10_000), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty)

	auto := NewExchangeAutomation(ex, AutomationConfig{})

	ex.mu.RLock()
	client := ex.Clients[1]
	ex.mu.RUnlock()

	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	if pos == nil || pos.Size == 0 {
		t.Fatal("position not opened")
	}

	liqPrice := auto.estimateLiquidationPrice(pos, client, perp, BTC_PRECISION)

	available := client.PerpAvailable("USD")
	expected := pos.EntryPrice - available*BTC_PRECISION/pos.Size
	if liqPrice != expected {
		t.Errorf("long liqPrice: expected %d, got %d", expected, liqPrice)
	}
	if liqPrice >= entryPrice {
		t.Errorf("long liqPrice must be below entry: entry=%d, liq=%d", entryPrice, liqPrice)
	}
}

// TestEstimateLiquidationPrice_Short verifies the formula for a short position:
// liqPrice = entryPrice + available * precision / (-size)
func TestEstimateLiquidationPrice_Short(t *testing.T) {
	// $10k balance: $5k used as initial margin (10% of 1 BTC at $50k), $5k remains available.
	ex, perp := setupPerpExchange(USDAmount(10_000), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Sell, qty)

	auto := NewExchangeAutomation(ex, AutomationConfig{})

	ex.mu.RLock()
	client := ex.Clients[1]
	ex.mu.RUnlock()

	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	if pos == nil || pos.Size == 0 {
		t.Fatal("position not opened")
	}

	liqPrice := auto.estimateLiquidationPrice(pos, client, perp, BTC_PRECISION)

	if pos.Size >= 0 {
		t.Fatalf("expected short (negative) position size, got %d", pos.Size)
	}
	available := client.PerpAvailable("USD")
	expected := pos.EntryPrice + available*BTC_PRECISION/(-pos.Size)
	if liqPrice != expected {
		t.Errorf("short liqPrice: expected %d, got %d", expected, liqPrice)
	}
	if liqPrice <= entryPrice {
		t.Errorf("short liqPrice must be above entry: entry=%d, liq=%d", entryPrice, liqPrice)
	}
}

// TestEstimateLiquidationPrice_ZeroSize verifies that a zero-size position returns 0.
func TestEstimateLiquidationPrice_ZeroSize(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(10_000), 0)
	auto := NewExchangeAutomation(ex, AutomationConfig{})

	ex.mu.RLock()
	client := ex.Clients[1]
	ex.mu.RUnlock()

	pos := &Position{ClientID: 1, Symbol: "BTC-PERP", Size: 0, EntryPrice: PriceUSD(50_000, DOLLAR_TICK)}
	liqPrice := auto.estimateLiquidationPrice(pos, client, perp, BTC_PRECISION)
	if liqPrice != 0 {
		t.Errorf("expected 0 for zero-size position, got %d", liqPrice)
	}
}

// TestCheckAndSettleFunding_TriggersWhenDue verifies that funding is settled when
// clock time >= NextFunding. A freshly created perp has NextFunding=0, so any
// positive clock time qualifies.
func TestCheckAndSettleFunding_TriggersWhenDue(t *testing.T) {
	ex, _ := setupPerpExchange(USDAmount(100_000), USDAmount(100_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(0.1)
	_, _ = InjectLimitOrder(ex, 1, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 2, "BTC-PERP", Buy, qty)

	totalBefore := totalMoney(ex, "USD")

	auto := NewExchangeAutomation(ex, AutomationConfig{})
	auto.checkAndSettleFunding()

	totalAfter := totalMoney(ex, "USD")
	if totalBefore != totalAfter {
		t.Errorf("funding settlement violated conservation: before=%d, after=%d, delta=%d",
			totalBefore, totalAfter, totalAfter-totalBefore)
	}

	perp := ex.Instruments["BTC-PERP"].(*PerpFutures)
	if perp.GetFundingRate().NextFunding <= 0 {
		t.Error("NextFunding should have advanced after settlement")
	}
}

// TestCheckAndSettleFunding_SkipsWhenNotDue verifies that funding is not settled when
// NextFunding is in the future.
func TestCheckAndSettleFunding_SkipsWhenNotDue(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(100_000), USDAmount(100_000))

	// Set NextFunding far in the future so settlement must not fire.
	// 9e18 ns ≈ year 2255; current wall-clock (~1.7e18) is safely below.
	perp.fundingRate.NextFunding = int64(9e18)

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(0.1)
	_, _ = InjectLimitOrder(ex, 1, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 2, "BTC-PERP", Buy, qty)

	auto := NewExchangeAutomation(ex, AutomationConfig{})
	auto.checkAndSettleFunding()

	// NextFunding must be unchanged — settlement did not run.
	if perp.GetFundingRate().NextFunding != int64(9e18) {
		t.Error("NextFunding changed even though settlement was not due")
	}
}

// TestMidPriceOracle_EmptyBookReturnsZero verifies that GetPrice returns 0
// when the spot instrument exists but has no resting orders (no bid or ask).
func TestMidPriceOracle_EmptyBookReturnsZero(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	spotInst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	perpInst := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	oracle := NewMidPriceOracle(ex)
	oracle.MapSymbol("BTC-PERP", "BTC/USD")

	if price := oracle.GetPrice("BTC-PERP"); price != 0 {
		t.Errorf("expected 0 from empty spot book, got %d", price)
	}
}

// TestMidPriceOracle_UpdatesWithNewOrders verifies that after a new best bid/ask
// is posted, GetPrice returns the updated mid-price reflecting the new market.
func TestMidPriceOracle_UpdatesWithNewOrders(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(10, clock)

	spotInst := NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	perpInst := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, SATOSHI)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	oracle := NewMidPriceOracle(ex)
	oracle.MapSymbol("BTC-PERP", "BTC/USD")

	spotBook := ex.Books["BTC/USD"]

	bid1 := &Order{
		ID: 1, ClientID: 1,
		Price: PriceUSD(49_000, DOLLAR_TICK), Qty: SATOSHI,
		Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano(),
	}
	ask1 := &Order{
		ID: 2, ClientID: 1,
		Price: PriceUSD(51_000, DOLLAR_TICK), Qty: SATOSHI,
		Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano(),
	}
	spotBook.Bids.addOrder(bid1)
	spotBook.Asks.addOrder(ask1)

	firstMid := oracle.GetPrice("BTC-PERP")
	expectedFirst := (PriceUSD(49_000, DOLLAR_TICK) + PriceUSD(51_000, DOLLAR_TICK)) / 2
	if firstMid != expectedFirst {
		t.Errorf("first mid-price: expected %d, got %d", expectedFirst, firstMid)
	}

	spotBook.Bids.cancelOrder(1)
	spotBook.Asks.cancelOrder(2)

	bid2 := &Order{
		ID: 3, ClientID: 1,
		Price: PriceUSD(52_000, DOLLAR_TICK), Qty: SATOSHI,
		Side: Buy, Type: LimitOrder, Timestamp: clock.NowUnixNano(),
	}
	ask2 := &Order{
		ID: 4, ClientID: 1,
		Price: PriceUSD(54_000, DOLLAR_TICK), Qty: SATOSHI,
		Side: Sell, Type: LimitOrder, Timestamp: clock.NowUnixNano(),
	}
	spotBook.Bids.addOrder(bid2)
	spotBook.Asks.addOrder(ask2)

	secondMid := oracle.GetPrice("BTC-PERP")
	expectedSecond := (PriceUSD(52_000, DOLLAR_TICK) + PriceUSD(54_000, DOLLAR_TICK)) / 2
	if secondMid != expectedSecond {
		t.Errorf("updated mid-price: expected %d, got %d", expectedSecond, secondMid)
	}
	if secondMid <= firstMid {
		t.Errorf("index price should have increased after order update: first=%d, second=%d", firstMid, secondMid)
	}
}
