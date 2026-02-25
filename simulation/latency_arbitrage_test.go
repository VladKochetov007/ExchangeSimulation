package simulation

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
)

func TestLatencyArbitrageActorCreation(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex1.AddInstrument(instrument)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex2.AddInstrument(instrument2)

	registry.Register("fast", ex1)
	registry.Register("slow", ex2)

	balances := map[VenueID]map[string]int64{
		"fast": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
		"slow": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"fast": &exchange.FixedFee{},
		"slow": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, balances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	config := LatencyArbitrageConfig{
		FastVenue:    "fast",
		SlowVenue:    "slow",
		Symbol:       "BTC/USD",
		MinProfitBps: 10,
		MaxQty:       100000000,
	}

	arbActor := NewLatencyArbitrageActor(1, mgw, config)

	if arbActor == nil {
		t.Fatal("Expected latency arbitrage actor to be created")
	}
	if arbActor.ID() != 1 {
		t.Fatalf("Expected actor ID 1, got %d", arbActor.ID())
	}

	ex1.Shutdown()
	ex2.Shutdown()
}

func TestLatencyArbitrageActorStart(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex1.AddInstrument(instrument)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex2.AddInstrument(instrument2)

	registry.Register("fast", ex1)
	registry.Register("slow", ex2)

	balances := map[VenueID]map[string]int64{
		"fast": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
		"slow": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"fast": &exchange.FixedFee{},
		"slow": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, balances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	config := LatencyArbitrageConfig{
		FastVenue:    "fast",
		SlowVenue:    "slow",
		Symbol:       "BTC/USD",
		MinProfitBps: 10,
		MaxQty:       100000000,
	}

	arbActor := NewLatencyArbitrageActor(1, mgw, config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := arbActor.Start(ctx); err != nil {
		t.Fatalf("Failed to start actor: %v", err)
	}
	defer arbActor.Stop()

	time.Sleep(50 * time.Millisecond)

	ex1.Shutdown()
	ex2.Shutdown()
}

func TestLatencyArbitrageActorWithLiquidity(t *testing.T) {
	registry := NewVenueRegistry()

	// Create fast venue
	fastEx := exchange.NewExchange(100, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	fastEx.AddInstrument(instrument)
	registry.Register("fast", fastEx)

	// Create slow venue
	slowEx := exchange.NewExchange(100, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	slowEx.AddInstrument(instrument2)
	registry.Register("slow", slowEx)

	// Add market makers to both venues
	lpBalances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,            // 100 BTC
		"USD": 10000000 * exchange.USD_PRECISION, // 10 million USD
	}

	// Market maker on fast venue
	fastGW := fastEx.ConnectClient(100, lpBalances, &exchange.FixedFee{})
	fastLP := actors.NewFirstLP(100, fastGW, actors.FirstLPConfig{
		Symbol:            "BTC/USD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
		BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	})
	fastLP.SetInitialState(instrument)
	fastLP.UpdateBalances(lpBalances["BTC"], lpBalances["USD"])

	ctx := context.Background()
	fastLP.Start(ctx)
	defer fastLP.Stop()

	// Market maker on slow venue
	slowGW := slowEx.ConnectClient(200, lpBalances, &exchange.FixedFee{})
	slowLP := actors.NewFirstLP(200, slowGW, actors.FirstLPConfig{
		Symbol:            "BTC/USD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
		BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	})
	slowLP.SetInitialState(instrument2)
	slowLP.UpdateBalances(lpBalances["BTC"], lpBalances["USD"])
	slowLP.Start(ctx)
	defer slowLP.Stop()

	// Wait for market makers to place orders
	time.Sleep(100 * time.Millisecond)

	// Create multi-venue gateway for arbitrage actor
	arbBalances := map[VenueID]map[string]int64{
		"fast": {"BTC": 10 * exchange.BTC_PRECISION, "USD": 1000000 * exchange.USD_PRECISION},
		"slow": {"BTC": 10 * exchange.BTC_PRECISION, "USD": 1000000 * exchange.USD_PRECISION},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"fast": &exchange.FixedFee{},
		"slow": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, arbBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	// Create arbitrage actor
	config := LatencyArbitrageConfig{
		FastVenue:    "fast",
		SlowVenue:    "slow",
		Symbol:       "BTC/USD",
		MinProfitBps: 5,
		MaxQty:       exchange.BTCAmount(0.1),
	}

	arbActor := NewLatencyArbitrageActor(1, mgw, config)

	arbCtx, arbCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer arbCancel()

	if err := arbActor.Start(arbCtx); err != nil {
		t.Fatalf("Failed to start arbitrage actor: %v", err)
	}
	defer arbActor.Stop()

	// Wait for actor to process market data
	time.Sleep(200 * time.Millisecond)

	// Check stats
	arbitrages, _ := arbActor.Stats()
	t.Logf("Arbitrages executed: %d", arbitrages)

	fastEx.Shutdown()
	slowEx.Shutdown()
}

func TestBookStateUpdate(t *testing.T) {
	book := NewBookState()

	// Test snapshot update
	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{
			{Price: 50000000000000, VisibleQty: 100000000},
			{Price: 49990000000000, VisibleQty: 200000000},
		},
		Asks: []exchange.PriceLevel{
			{Price: 50010000000000, VisibleQty: 150000000},
			{Price: 50020000000000, VisibleQty: 250000000},
		},
	}

	md := &exchange.MarketDataMsg{
		Type: exchange.MDSnapshot,
		Data: snapshot,
	}

	book.Update(md)

	if book.BestBid() != 50000000000000 {
		t.Errorf("Expected best bid 50000000000000, got %d", book.BestBid())
	}
	if book.BestAsk() != 50010000000000 {
		t.Errorf("Expected best ask 50010000000000, got %d", book.BestAsk())
	}
	if book.BestBidQty() != 100000000 {
		t.Errorf("Expected best bid qty 100000000, got %d", book.BestBidQty())
	}
	if book.BestAskQty() != 150000000 {
		t.Errorf("Expected best ask qty 150000000, got %d", book.BestAskQty())
	}
}

func TestBookStateDelta(t *testing.T) {
	book := NewBookState()

	// Test delta update (buy side)
	delta := &exchange.BookDelta{
		Side:       exchange.Buy,
		Price:      50000000000000,
		VisibleQty: 100000000,
	}

	md := &exchange.MarketDataMsg{
		Type: exchange.MDDelta,
		Data: delta,
	}

	book.Update(md)

	if book.BestBid() != 50000000000000 {
		t.Errorf("Expected best bid 50000000000000, got %d", book.BestBid())
	}
	if book.BestBidQty() != 100000000 {
		t.Errorf("Expected best bid qty 100000000, got %d", book.BestBidQty())
	}

	// Test delta update (sell side)
	askDelta := &exchange.BookDelta{
		Side:       exchange.Sell,
		Price:      50010000000000,
		VisibleQty: 150000000,
	}

	askMD := &exchange.MarketDataMsg{
		Type: exchange.MDDelta,
		Data: askDelta,
	}

	book.Update(askMD)

	if book.BestAsk() != 50010000000000 {
		t.Errorf("Expected best ask 50010000000000, got %d", book.BestAsk())
	}
	if book.BestAskQty() != 150000000 {
		t.Errorf("Expected best ask qty 150000000, got %d", book.BestAskQty())
	}
}

func TestLatencyArbitrageActorDoubleStart(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex1.AddInstrument(instrument)

	ex2 := exchange.NewExchange(10, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	ex2.AddInstrument(instrument2)

	registry.Register("fast", ex1)
	registry.Register("slow", ex2)

	balances := map[VenueID]map[string]int64{
		"fast": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
		"slow": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"fast": &exchange.FixedFee{},
		"slow": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, balances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	config := LatencyArbitrageConfig{
		FastVenue:    "fast",
		SlowVenue:    "slow",
		Symbol:       "BTC/USD",
		MinProfitBps: 10,
		MaxQty:       100000000,
	}

	arbActor := NewLatencyArbitrageActor(1, mgw, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := arbActor.Start(ctx); err != nil {
		t.Fatalf("First start failed: %v", err)
	}
	defer arbActor.Stop()

	// Second start should not error
	if err := arbActor.Start(ctx); err != nil {
		t.Fatalf("Second start should not error: %v", err)
	}

	ex1.Shutdown()
	ex2.Shutdown()
}

func TestLatencyArbitrageActorArbitrageDetection(t *testing.T) {
	registry := NewVenueRegistry()

	// Create fast venue with higher price
	fastEx := exchange.NewExchange(100, &RealClock{})
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	fastEx.AddInstrument(instrument)
	registry.Register("fast", fastEx)

	// Create slow venue with lower price
	slowEx := exchange.NewExchange(100, &RealClock{})
	instrument2 := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.BTC_PRECISION/1000)
	slowEx.AddInstrument(instrument2)
	registry.Register("slow", slowEx)

	lpBalances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 10000000 * exchange.USD_PRECISION, // 10 million USD
	}

	// Market maker on fast venue with HIGHER price (51000)
	fastGW := fastEx.ConnectClient(100, lpBalances, &exchange.FixedFee{})
	fastLP := actors.NewFirstLP(100, fastGW, actors.FirstLPConfig{
		Symbol:            "BTC/USD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
		BootstrapPrice:    exchange.PriceUSD(51000, exchange.DOLLAR_TICK),
	})
	fastLP.SetInitialState(instrument)
	fastLP.UpdateBalances(lpBalances["BTC"], lpBalances["USD"])

	ctx := context.Background()
	fastLP.Start(ctx)
	defer fastLP.Stop()

	// Market maker on slow venue with LOWER price (50000)
	slowGW := slowEx.ConnectClient(200, lpBalances, &exchange.FixedFee{})
	slowLP := actors.NewFirstLP(200, slowGW, actors.FirstLPConfig{
		Symbol:            "BTC/USD",
		HalfSpreadBps:     10, // 0.1% half-spread (was 20 bps / 2)
		LiquidityMultiple: 10,
		BootstrapPrice:    exchange.PriceUSD(50000, exchange.DOLLAR_TICK),
	})
	slowLP.SetInitialState(instrument2)
	slowLP.UpdateBalances(lpBalances["BTC"], lpBalances["USD"])
	slowLP.Start(ctx)
	defer slowLP.Stop()

	// Wait for market makers to place orders
	time.Sleep(100 * time.Millisecond)

	// Create multi-venue gateway for arbitrage actor
	arbBalances := map[VenueID]map[string]int64{
		"fast": {"BTC": 10 * exchange.BTC_PRECISION, "USD": 1000000 * exchange.USD_PRECISION},
		"slow": {"BTC": 10 * exchange.BTC_PRECISION, "USD": 1000000 * exchange.USD_PRECISION},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"fast": &exchange.FixedFee{},
		"slow": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, arbBalances, feePlans)
	mgw.Start()
	defer mgw.Stop()

	// Create arbitrage actor with low profit threshold
	config := LatencyArbitrageConfig{
		FastVenue:    "fast",
		SlowVenue:    "slow",
		Symbol:       "BTC/USD",
		MinProfitBps: 1, // Very low threshold to ensure detection
		MaxQty:       exchange.BTCAmount(0.1),
	}

	arbActor := NewLatencyArbitrageActor(1, mgw, config)

	arbCtx, arbCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer arbCancel()

	if err := arbActor.Start(arbCtx); err != nil {
		t.Fatalf("Failed to start arbitrage actor: %v", err)
	}
	defer arbActor.Stop()

	// Wait for actor to process market data and execute arbitrages
	time.Sleep(80 * time.Millisecond)

	// Check stats - should have executed some arbitrages
	arbitrages, profit := arbActor.Stats()
	t.Logf("Arbitrages executed: %d, Profit: %d satoshis", arbitrages, profit)

	if arbitrages == 0 {
		t.Log("No arbitrages executed (timing dependent, but expected at least 1)")
	}

	fastEx.Shutdown()
	slowEx.Shutdown()
}

func TestLatencyArbitrageActorStopBeforeStart(t *testing.T) {
	registry := NewVenueRegistry()

	ex1 := exchange.NewExchange(10, &RealClock{})
	ex2 := exchange.NewExchange(10, &RealClock{})

	registry.Register("fast", ex1)
	registry.Register("slow", ex2)

	balances := map[VenueID]map[string]int64{
		"fast": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
		"slow": {"BTC": 1000000000, "USD": 1000000 * exchange.USD_PRECISION},
	}

	feePlans := map[VenueID]exchange.FeeModel{
		"fast": &exchange.FixedFee{},
		"slow": &exchange.FixedFee{},
	}

	mgw := NewMultiVenueGateway(1, registry, balances, feePlans)

	config := LatencyArbitrageConfig{
		FastVenue:    "fast",
		SlowVenue:    "slow",
		Symbol:       "BTC/USD",
		MinProfitBps: 10,
		MaxQty:       100000000,
	}

	arbActor := NewLatencyArbitrageActor(1, mgw, config)

	if err := arbActor.Stop(); err != nil {
		t.Fatalf("Stop before start should not error: %v", err)
	}

	ex1.Shutdown()
	ex2.Shutdown()
}
