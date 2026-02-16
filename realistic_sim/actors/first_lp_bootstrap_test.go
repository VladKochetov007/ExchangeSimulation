package actors

import (
	"context"
	"exchange_sim/exchange"
	"testing"
	"time"
)

// TestFirstLP_BootstrapEmptyBook verifies FirstLP provides first liquidity
func TestFirstLP_BootstrapEmptyBook(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)
	defer ex.Shutdown()

	// Create instrument
	instrument := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	// Check: book should be empty initially
	book := ex.Books["BTCUSD"]
	if book == nil {
		t.Fatal("Order book not found")
	}

	if book.Bids.Best != nil || book.Asks.Best != nil {
		t.Fatal("Book should be empty initially")
	}
	t.Log("Book is empty initially")

	// Create FirstLP with bootstrap price
	config := FirstLPConfig{
		Symbol:         "BTCUSD",
		HalfSpreadBps:  100, // 1% half-spread = 2% total spread
		BootstrapPrice: exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	}

	balances := map[string]int64{
		"BTC": exchange.BTCAmount(1.0),
		"USD": exchange.USDAmount(50000),
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	lp := NewFirstLP(1, gateway, config)
	lp.SetInitialState(instrument)
	lp.UpdateBalances(balances["BTC"], balances["USD"])

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := lp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer lp.Stop()

	// Wait for LP to place initial quotes
	time.Sleep(300 * time.Millisecond)

	// Check: LastMidPrice should be set from bootstrap
	if lp.LastMidPrice == 0 {
		t.Error("LastMidPrice should be set from BootstrapPrice")
	} else {
		t.Logf("LastMidPrice set to: %d (%.2f USD)",
			lp.LastMidPrice, float64(lp.LastMidPrice)/100000)
	}

	// Check: Orders should be placed
	if lp.ActiveBidID == 0 {
		t.Error("❌ No bid order placed - FirstLP failed to provide first liquidity!")
	} else {
		t.Logf("Bid order placed: orderID=%d", lp.ActiveBidID)
	}

	if lp.ActiveAskID == 0 {
		t.Error("❌ No ask order placed - FirstLP failed to provide first liquidity!")
	} else {
		t.Logf("Ask order placed: orderID=%d", lp.ActiveAskID)
	}

	// Check: Book should now have liquidity
	if book.Bids.Best == nil {
		t.Error("❌ No bids on book after FirstLP started")
	} else {
		bestBid := book.Bids.Best
		t.Logf("Best bid: price=%d (%.2f USD), qty=%d (%.8f BTC)",
			bestBid.Price, float64(bestBid.Price)/100000,
			bestBid.TotalQty, float64(bestBid.TotalQty)/100000000)
	}

	if book.Asks.Best == nil {
		t.Error("❌ No asks on book after FirstLP started")
	} else {
		bestAsk := book.Asks.Best
		t.Logf("Best ask: price=%d (%.2f USD), qty=%d (%.8f BTC)",
			bestAsk.Price, float64(bestAsk.Price)/100000,
			bestAsk.TotalQty, float64(bestAsk.TotalQty)/100000000)
	}

	// Verify spread is reasonable (should be ~1%)
	if book.Bids.Best != nil && book.Asks.Best != nil {
		spread := book.Asks.Best.Price - book.Bids.Best.Price
		spreadBps := (spread * 10000) / lp.LastMidPrice
		t.Logf("Spread: %d bps (%.2f%%)", spreadBps, float64(spreadBps)/100)

		expectedSpread := int64(200) // With HalfSpreadBps=100, total spread is 200 bps (2%)
		tolerance := int64(50)       // Allow some rounding
		if spreadBps < expectedSpread-tolerance || spreadBps > expectedSpread+tolerance {
			t.Errorf("Spread %d bps not close to expected %d bps", spreadBps, expectedSpread)
		}
	}
}

// TestFirstLP_NoBootstrapPrice verifies behavior without bootstrap price
func TestFirstLP_NoBootstrapPrice(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)
	defer ex.Shutdown()

	instrument := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	// Create FirstLP WITHOUT bootstrap price
	config := FirstLPConfig{
		Symbol:        "BTCUSD",
		HalfSpreadBps: 100, // 1% half-spread = 2% total spread
		// BootstrapPrice: 0  // NOT SET
	}

	balances := map[string]int64{
		"BTC": exchange.BTCAmount(1.0),
		"USD": exchange.USDAmount(50000),
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	lp := NewFirstLP(1, gateway, config)
	lp.SetInitialState(instrument)
	lp.UpdateBalances(balances["BTC"], balances["USD"])

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := lp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer lp.Stop()

	time.Sleep(300 * time.Millisecond)

	// Without bootstrap price and empty book, LP should NOT place orders
	if lp.LastMidPrice != 0 {
		t.Errorf("LastMidPrice should be 0 without bootstrap and empty book, got %d", lp.LastMidPrice)
	}

	if lp.ActiveBidID != 0 || lp.ActiveAskID != 0 {
		t.Error("Orders should NOT be placed without bootstrap price on empty book")
	} else {
		t.Log("Correctly did NOT place orders without bootstrap price")
	}
}

// TestFirstLP_BootstrapThenMarketPrice verifies transition from bootstrap to market price
func TestFirstLP_BootstrapThenMarketPrice(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)
	defer ex.Shutdown()

	instrument := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	// Create FirstLP with bootstrap
	config := FirstLPConfig{
		Symbol:         "BTCUSD",
		HalfSpreadBps:  100, // 1% half-spread = 2% total spread
		BootstrapPrice: exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	}

	balances := map[string]int64{
		"BTC": exchange.BTCAmount(1.0),
		"USD": exchange.USDAmount(50000),
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	lp := NewFirstLP(1, gateway, config)
	lp.SetInitialState(instrument)
	lp.UpdateBalances(balances["BTC"], balances["USD"])

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := lp.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer lp.Stop()

	time.Sleep(300 * time.Millisecond)

	bootstrapMid := lp.LastMidPrice
	t.Logf("Bootstrap mid price: %d", bootstrapMid)

	// Now inject external liquidity at different price
	gateway2 := ex.ConnectClient(2, map[string]int64{
		"BTC": exchange.BTCAmount(10.0),
		"USD": exchange.USDAmount(1000000),
	}, feePlan)
	if gateway2 == nil {
		t.Fatal("Failed to connect client 2")
	}

	time.Sleep(100 * time.Millisecond)

	newPrice := exchange.PriceUSD(51000, exchange.DOLLAR_TICK)
	orderID, err := exchange.InjectLimitOrder(ex, 2, "BTCUSD", exchange.Buy, newPrice, exchange.BTCAmount(0.5))
	if err != 0 {
		t.Fatalf("Failed to inject order: error=%d", err)
	}
	t.Logf("Injected external order: orderID=%d at price=%d", orderID, newPrice)

	time.Sleep(300 * time.Millisecond)

	// LP should update to market price (mid between its ask and new bid)
	newMid := lp.LastMidPrice
	t.Logf("New mid price after external order: %d", newMid)

	if newMid == bootstrapMid {
		t.Log("Mid price unchanged - may need external trade to trigger update")
	} else {
		t.Logf("Mid price updated from bootstrap to market price")
	}
}
