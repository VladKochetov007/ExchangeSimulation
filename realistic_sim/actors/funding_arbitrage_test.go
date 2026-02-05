package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func TestFundingArbConfig_Defaults(t *testing.T) {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := FundingArbConfig{
		SpotSymbol:      "BTC/USD",
		PerpSymbol:      "BTC-PERP",
		SpotInstrument:  spotInst,
		PerpInstrument:  perpInst,
		MinFundingRate:  10,  // 0.1% min to enter
		ExitFundingRate: -10, // Exit if negative
		MaxPositionSize: exchange.BTC_PRECISION, // 1 BTC
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	fa := NewFundingArbitrage(1, gateway, config)

	// Check defaults
	if fa.config.MonitorInterval != 5*time.Second {
		t.Errorf("Expected MonitorInterval 5s, got %v", fa.config.MonitorInterval)
	}
	if fa.config.HedgeRatio != 10000 {
		t.Errorf("Expected HedgeRatio 10000, got %d", fa.config.HedgeRatio)
	}
	if fa.config.RebalanceThreshold != 100 {
		t.Errorf("Expected RebalanceThreshold 100, got %d", fa.config.RebalanceThreshold)
	}
}

func TestFundingArbActor_Creation(t *testing.T) {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := FundingArbConfig{
		SpotSymbol:         "BTC/USD",
		PerpSymbol:         "BTC-PERP",
		SpotInstrument:     spotInst,
		PerpInstrument:     perpInst,
		MinFundingRate:     20,
		ExitFundingRate:    5,
		HedgeRatio:         10000,
		MaxPositionSize:    exchange.BTC_PRECISION,
		MonitorInterval:    3 * time.Second,
		RebalanceThreshold: 50,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	fa := NewFundingArbitrage(1, gateway, config)

	if fa.ID() != 1 {
		t.Errorf("Expected ID 1, got %d", fa.ID())
	}
	if fa.config.SpotSymbol != "BTC/USD" {
		t.Errorf("Expected SpotSymbol BTC/USD, got %s", fa.config.SpotSymbol)
	}
	if fa.config.PerpSymbol != "BTC-PERP" {
		t.Errorf("Expected PerpSymbol BTC-PERP, got %s", fa.config.PerpSymbol)
	}
	if fa.config.MinFundingRate != 20 {
		t.Errorf("Expected MinFundingRate 20, got %d", fa.config.MinFundingRate)
	}
}

func TestFundingArbActor_Start(t *testing.T) {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := FundingArbConfig{
		SpotSymbol:      "BTC/USD",
		PerpSymbol:      "BTC-PERP",
		SpotInstrument:  spotInst,
		PerpInstrument:  perpInst,
		MinFundingRate:  10,
		ExitFundingRate: -10,
		MaxPositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	fa := NewFundingArbitrage(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := fa.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start FundingArbitrage: %v", err)
	}

	// Let it run briefly
	time.Sleep(10 * time.Millisecond)

	err = fa.Stop()
	if err != nil {
		t.Fatalf("Failed to stop FundingArbitrage: %v", err)
	}
}

func TestFundingArbActor_BookSnapshot(t *testing.T) {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := FundingArbConfig{
		SpotSymbol:      "BTC/USD",
		PerpSymbol:      "BTC-PERP",
		SpotInstrument:  spotInst,
		PerpInstrument:  perpInst,
		MinFundingRate:  10,
		ExitFundingRate: -10,
		MaxPositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	fa := NewFundingArbitrage(1, gateway, config)

	// Simulate spot book snapshot
	fa.onBookSnapshot(actor.BookSnapshotEvent{
		Symbol: "BTC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{
				{Price: 50000 * exchange.USD_PRECISION, VisibleQty: 100},
			},
			Asks: []exchange.PriceLevel{
				{Price: 50100 * exchange.USD_PRECISION, VisibleQty: 100},
			},
		},
	})

	expectedSpotMid := int64((50000*exchange.USD_PRECISION + 50100*exchange.USD_PRECISION) / 2)
	if fa.spotMid != expectedSpotMid {
		t.Errorf("Expected spotMid %d, got %d", expectedSpotMid, fa.spotMid)
	}

	// Simulate perp book snapshot
	fa.onBookSnapshot(actor.BookSnapshotEvent{
		Symbol: "BTC-PERP",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{
				{Price: 50050 * exchange.USD_PRECISION, VisibleQty: 100},
			},
			Asks: []exchange.PriceLevel{
				{Price: 50150 * exchange.USD_PRECISION, VisibleQty: 100},
			},
		},
	})

	expectedPerpMid := int64((50050*exchange.USD_PRECISION + 50150*exchange.USD_PRECISION) / 2)
	if fa.perpMid != expectedPerpMid {
		t.Errorf("Expected perpMid %d, got %d", expectedPerpMid, fa.perpMid)
	}
}

func TestFundingArbActor_FundingUpdate(t *testing.T) {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := FundingArbConfig{
		SpotSymbol:      "BTC/USD",
		PerpSymbol:      "BTC-PERP",
		SpotInstrument:  spotInst,
		PerpInstrument:  perpInst,
		MinFundingRate:  10,
		ExitFundingRate: -10,
		MaxPositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	fa := NewFundingArbitrage(1, gateway, config)

	// Simulate funding update
	nowNano := time.Now().UnixNano()
	fa.onFundingUpdate(actor.FundingUpdateEvent{
		Symbol: "BTC-PERP",
		FundingRate: &exchange.FundingRate{
			Symbol:      "BTC-PERP",
			Rate:        25, // 0.25% positive funding
			NextFunding: nowNano + int64(8*time.Hour),
			Interval:    28800, // 8 hours
			MarkPrice:   50100 * exchange.USD_PRECISION,
			IndexPrice:  50000 * exchange.USD_PRECISION,
		},
		Timestamp: nowNano,
	})

	if fa.fundingRate != 25 {
		t.Errorf("Expected fundingRate 25, got %d", fa.fundingRate)
	}
	if fa.nextFunding != nowNano+int64(8*time.Hour) {
		t.Errorf("Expected nextFunding %d, got %d", nowNano+int64(8*time.Hour), fa.nextFunding)
	}
}

func TestFundingArbActor_PositionState(t *testing.T) {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	perpInst := exchange.NewPerpFutures(
		"BTC-PERP",
		"BTC",
		"USD",
		exchange.BTC_PRECISION,
		exchange.USD_PRECISION,
		exchange.DOLLAR_TICK,
		exchange.SATOSHI/100,
	)

	config := FundingArbConfig{
		SpotSymbol:      "BTC/USD",
		PerpSymbol:      "BTC-PERP",
		SpotInstrument:  spotInst,
		PerpInstrument:  perpInst,
		MinFundingRate:  10,
		ExitFundingRate: -10,
		MaxPositionSize: exchange.BTC_PRECISION,
	}

	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(10, clock)
	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	balances := map[string]int64{
		"BTC": 10 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}

	gateway := ex.ConnectClient(1, balances, &exchange.FixedFee{})

	fa := NewFundingArbitrage(1, gateway, config)

	// Initially no position
	if fa.isActive {
		t.Error("Expected isActive to be false initially")
	}
	if fa.spotPosition != 0 {
		t.Errorf("Expected spotPosition 0, got %d", fa.spotPosition)
	}
	if fa.perpPosition != 0 {
		t.Errorf("Expected perpPosition 0, got %d", fa.perpPosition)
	}
}
