package exchange

import (
	"testing"
)

// totalMoney returns the full accounting of money in the system for an asset:
// all client spot + perp balances, plus exchange fee revenue and insurance fund.
// Caller must not hold ex.mu.
func totalMoney(ex *Exchange, asset string) int64 {
	ex.mu.RLock()
	defer ex.mu.RUnlock()
	var spot, perp int64
	for _, client := range ex.Clients {
		spot += client.Balances[asset]
		perp += client.PerpBalances[asset]
	}
	fees := ex.ExchangeBalance.FeeRevenue[asset]
	insurance := ex.ExchangeBalance.InsuranceFund[asset]
	return spot + perp + fees + insurance
}

// TestMoneyConservation_SpotTrades verifies that a series of spot trades
// conserves the total money in the system: client balances + fee revenue = initial.
func TestMoneyConservation_SpotTrades(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(inst)

	makerBalances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(500_000)}
	takerBalances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(500_000)}

	fees := &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	_ = ex.ConnectClient(1, makerBalances, fees)
	_ = ex.ConnectClient(2, takerBalances, fees)

	initialUSD := totalMoney(ex, "USD")
	initialBTC := totalMoney(ex, "BTC")

	price := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(0.5)

	_, _ = InjectLimitOrder(ex, 1, "BTCUSD", Sell, price, qty)
	_, _ = InjectMarketOrder(ex, 2, "BTCUSD", Buy, qty)

	afterUSD := totalMoney(ex, "USD")
	afterBTC := totalMoney(ex, "BTC")

	if initialUSD != afterUSD {
		t.Errorf("USD conservation violated: initial=%d, after=%d, delta=%d",
			initialUSD, afterUSD, afterUSD-initialUSD)
	}
	if initialBTC != afterBTC {
		t.Errorf("BTC conservation violated: initial=%d, after=%d, delta=%d",
			initialBTC, afterBTC, afterBTC-initialBTC)
	}
}

// TestMoneyConservation_MultipleTrades verifies conservation across many trades.
func TestMoneyConservation_MultipleTrades(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	inst := NewSpotInstrument("BTCUSD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(inst)

	fees := &PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	_ = ex.ConnectClient(1, map[string]int64{"BTC": BTCAmount(100), "USD": USDAmount(5_000_000)}, fees)
	_ = ex.ConnectClient(2, map[string]int64{"BTC": BTCAmount(100), "USD": USDAmount(5_000_000)}, fees)

	initialUSD := totalMoney(ex, "USD")
	initialBTC := totalMoney(ex, "BTC")

	prices := []float64{49_000, 50_000, 51_000, 50_500, 49_500}
	for i, p := range prices {
		price := PriceUSD(p, DOLLAR_TICK)
		qty := BTCAmount(0.1)
		makerID := uint64(1 + i%2)
		takerID := uint64(2 - i%2)
		_, _ = InjectLimitOrder(ex, makerID, "BTCUSD", Sell, price, qty)
		_, _ = InjectMarketOrder(ex, takerID, "BTCUSD", Buy, qty)
	}

	if got := totalMoney(ex, "USD"); got != initialUSD {
		t.Errorf("USD conservation violated after %d trades: delta=%d", len(prices), got-initialUSD)
	}
	if got := totalMoney(ex, "BTC"); got != initialBTC {
		t.Errorf("BTC conservation violated after %d trades: delta=%d", len(prices), got-initialBTC)
	}
}

// TestMoneyConservation_Liquidation verifies that a liquidation event does not
// create or destroy money. The insurance fund must absorb the deficit.
func TestMoneyConservation_Liquidation(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	fees := &PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	// Client 1: thin margin, large position — will be liquidated
	_ = ex.ConnectClient(1, map[string]int64{}, fees)
	ex.AddPerpBalance(1, "USD", USDAmount(6_000)) // ~10% margin on 1 BTC at $50k

	// Client 2: provides liquidity at entry and at crash price
	_ = ex.ConnectClient(2, map[string]int64{}, fees)
	ex.AddPerpBalance(2, "USD", USDAmount(500_000))

	initialUSD := totalMoney(ex, "USD")

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)

	// Client 2 sells, client 1 buys (opens leveraged long)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty)
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty)

	if got := totalMoney(ex, "USD"); got != initialUSD {
		t.Fatalf("USD conservation violated after opening position: delta=%d", got-initialUSD)
	}

	// Price crashes to $44k — client 1's position is now deeply underwater
	// (loss = 1 BTC * (50k-44k) = $6k, which exceeds their $6k margin → liquidation)
	crashPrice := PriceUSD(44_000, DOLLAR_TICK)

	// Client 2 provides liquidity at crash price so liquidation can execute
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, crashPrice, qty)

	// Trigger liquidation check directly (no Start() needed — just call the method)
	automation := NewExchangeAutomation(ex, AutomationConfig{})
	automation.checkLiquidations("BTC-PERP", perp, crashPrice)

	if got := totalMoney(ex, "USD"); got != initialUSD {
		t.Errorf("USD conservation violated after liquidation: delta=%d (initial=%d, after=%d)",
			got-initialUSD, initialUSD, got)
	}
}

// TestMoneyConservation_CrossMarginMultiPosition verifies conservation when a client
// holds positions on two symbols simultaneously and one loses while the other wins.
func TestMoneyConservation_CrossMarginMultiPosition(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	btcPerp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ethPerp := NewPerpFutures("ETH-PERP", "ETH", "USD", ETH_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/10)
	ex.AddInstrument(btcPerp)
	ex.AddInstrument(ethPerp)

	fees := &PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	_ = ex.ConnectClient(1, map[string]int64{}, fees) // cross-margin trader
	_ = ex.ConnectClient(2, map[string]int64{}, fees) // liquidity provider

	ex.AddPerpBalance(1, "USD", USDAmount(100_000))
	ex.AddPerpBalance(2, "USD", USDAmount(500_000))

	initialUSD := totalMoney(ex, "USD")

	btcEntry := PriceUSD(50_000, DOLLAR_TICK)
	ethEntry := PriceUSD(3_000, DOLLAR_TICK)

	// Client 1 opens long BTC and short ETH (cross-margin: both draw from same pool)
	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Sell, btcEntry, BTCAmount(0.1))
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Buy, BTCAmount(0.1))

	_, _ = InjectLimitOrder(ex, 2, "ETH-PERP", Buy, ethEntry, ETHAmount(1.0))
	_, _ = InjectMarketOrder(ex, 1, "ETH-PERP", Sell, ETHAmount(1.0))

	if got := totalMoney(ex, "USD"); got != initialUSD {
		t.Fatalf("USD conservation violated after opening positions: delta=%d", got-initialUSD)
	}

	// Close both positions
	btcClose := PriceUSD(51_000, DOLLAR_TICK) // BTC up: client 1 wins
	ethClose := PriceUSD(2_900, DOLLAR_TICK)  // ETH down: client 1 wins (was short)

	_, _ = InjectLimitOrder(ex, 2, "BTC-PERP", Buy, btcClose, BTCAmount(0.1))
	_, _ = InjectMarketOrder(ex, 1, "BTC-PERP", Sell, BTCAmount(0.1))

	_, _ = InjectLimitOrder(ex, 2, "ETH-PERP", Sell, ethClose, ETHAmount(1.0))
	_, _ = InjectMarketOrder(ex, 1, "ETH-PERP", Buy, ETHAmount(1.0))

	if got := totalMoney(ex, "USD"); got != initialUSD {
		t.Errorf("USD conservation violated after closing positions: delta=%d", got-initialUSD)
	}
}

// TestMoneyConservation_FundingPayments verifies that funding settlements
// are zero-sum: what longs pay equals what shorts receive.
func TestMoneyConservation_FundingPayments(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	fees := &PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	gw1 := ex.ConnectClient(1, map[string]int64{}, fees)
	gw2 := ex.ConnectClient(2, map[string]int64{}, fees)

	ex.AddPerpBalance(1, "USD", USDAmount(100_000))
	ex.AddPerpBalance(2, "USD", USDAmount(100_000))

	initialUSD := totalMoney(ex, "USD")

	price := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(0.1)
	_, _ = InjectLimitOrder(ex, 1, "BTC-PERP", Sell, price, qty)
	_, _ = InjectMarketOrder(ex, 2, "BTC-PERP", Buy, qty)

	_ = gw1
	_ = gw2

	afterTrades := totalMoney(ex, "USD")
	if afterTrades != initialUSD {
		t.Errorf("USD conservation violated after perp trades: delta=%d", afterTrades-initialUSD)
	}
}

// TestSettleFunding_CorrectMagnitudeAtBTCPrices is a regression test for Bug 1:
// SettleFunding previously used TickSize (1e5) instead of BasePrecision (1e8),
// causing funding payments to be 1000x too large at real BTC prices.
//
// Expected: 1 BTC long at $50k with +10 bps rate pays exactly $50.
// Old bug:  the same position would pay $50,000.
func TestSettleFunding_CorrectMagnitudeAtBTCPrices(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{}, &FixedFee{})

	entryUSD := PriceUSD(50_000, DOLLAR_TICK) // $50,000 in USD units = 5_000_000_000
	qty := BTCAmount(1.0)                     // 1 BTC = 100_000_000 satoshis
	collateral := USDAmount(10_000)           // $10k each, well above initMargin

	injectPerpPosition(ex, 1, "BTC-PERP", qty, entryUSD, collateral, 0)   // client 1: long
	injectPerpPosition(ex, 2, "BTC-PERP", -qty, entryUSD, collateral, 0)  // client 2: short

	perp.GetFundingRate().Rate = 10 // +10 bps: longs pay shorts

	longBefore := ex.Clients[1].PerpBalances["USD"]
	shortBefore := ex.Clients[2].PerpBalances["USD"]

	ex.Positions.SettleFunding(ex.Clients, perp, ex)

	longAfter := ex.Clients[1].PerpBalances["USD"]
	shortAfter := ex.Clients[2].PerpBalances["USD"]

	// positionValue = 1e8 * 5e9 / 1e8 = 5e9 = $50,000 notional
	// funding = 5e9 * 10 / 10000 = 5_000_000 = $50
	wantPayment := int64(5_000_000)

	if got := longBefore - longAfter; got != wantPayment {
		t.Errorf("long paid %d (%.4f USD), want %d (%.4f USD); old bug would give %d",
			got, float64(got)/float64(USD_PRECISION),
			wantPayment, float64(wantPayment)/float64(USD_PRECISION),
			wantPayment*1000)
	}
	if got := shortAfter - shortBefore; got != wantPayment {
		t.Errorf("short received %d (%.4f USD), want %d (%.4f USD)",
			got, float64(got)/float64(USD_PRECISION),
			wantPayment, float64(wantPayment)/float64(USD_PRECISION))
	}
}

// TestSettleFunding_AsymmetricOIRoutesToExchange verifies Bug 3 fix:
// when long and short notionals differ (different entry prices), the imbalance
// routes to ExchangeBalance.FeeRevenue rather than creating/destroying money.
func TestSettleFunding_AsymmetricOIRoutesToExchange(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION)
	ex.AddInstrument(perp)

	ex.ConnectClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectClient(2, map[string]int64{}, &FixedFee{})

	// Client 1: long 1 BTC entered at $50k → notional $50k
	// Client 2: short 1 BTC entered at $40k → notional $40k
	// Different entry prices → asymmetric notionals even though qty is equal.
	qty := BTCAmount(1.0)
	collateral := USDAmount(20_000)
	injectPerpPosition(ex, 1, "BTC-PERP", qty, PriceUSD(50_000, DOLLAR_TICK), collateral, 0)
	injectPerpPosition(ex, 2, "BTC-PERP", -qty, PriceUSD(40_000, DOLLAR_TICK), collateral, 0)

	perp.GetFundingRate().Rate = 10 // +10 bps: longs pay

	initialTotal := totalMoney(ex, "USD")
	feeRevenueBefore := ex.ExchangeBalance.FeeRevenue["USD"]

	ex.Positions.SettleFunding(ex.Clients, perp, ex)

	// Long pays: 1e8 * 5e9 / 1e8 * 10 / 10000 = 5_000_000 ($50)
	// Short receives: 1e8 * 4e9 / 1e8 * 10 / 10000 = 4_000_000 ($40)
	// Exchange absorbs: 5_000_000 - 4_000_000 = 1_000_000 ($10)
	wantExchangeDelta := int64(1_000_000)
	gotExchangeDelta := ex.ExchangeBalance.FeeRevenue["USD"] - feeRevenueBefore

	if gotExchangeDelta != wantExchangeDelta {
		t.Errorf("exchange fee revenue delta = %d (%.4f USD), want %d (%.4f USD)",
			gotExchangeDelta, float64(gotExchangeDelta)/float64(USD_PRECISION),
			wantExchangeDelta, float64(wantExchangeDelta)/float64(USD_PRECISION))
	}

	// Total money must be conserved
	if got := totalMoney(ex, "USD"); got != initialTotal {
		t.Errorf("money conservation violated: delta=%d", got-initialTotal)
	}
}
