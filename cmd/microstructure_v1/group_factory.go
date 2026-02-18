package main

import (
	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
	"exchange_sim/simulation"
	"time"
)

type GroupFactory struct {
	ex           *exchange.Exchange
	marketConfig *MarketConfig
	nextActorID  uint64
	nextClientID uint64
}

func NewGroupFactory(ex *exchange.Exchange, marketConfig *MarketConfig) *GroupFactory {
	return &GroupFactory{
		ex:           ex,
		marketConfig: marketConfig,
		nextActorID:  1000,
		nextClientID: 1,
	}
}

func (gf *GroupFactory) CreatePerpMMGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 2 * ASSET_PRECISION,
		"BCD": 4 * ASSET_PRECISION,
		"CDE": 10 * ASSET_PRECISION,
		"DEF": 20 * ASSET_PRECISION,
		"EFG": 100 * ASSET_PRECISION,
		"USD": 1_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)

	for asset, balance := range initialBalances {
		gf.ex.AddPerpBalance(gf.nextClientID, asset, balance)
	}
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(1*time.Millisecond, 2*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	perpSymbols := []string{"ABC-PERP", "BCD-PERP", "CDE-PERP", "DEF-PERP", "EFG-PERP"}
	subActors := []actor.SubActor{}

	for _, symbol := range perpSymbols {
		for _, spreadBps := range []int64{5, 10, 15} {
			sub := actors.NewPureMMSubActor(gf.nextActorID, symbol, actors.PureMMSubActorConfig{
				SpreadBps:        spreadBps,
				QuoteSize:        ASSET_PRECISION / 10,
				MaxInventory:     5 * ASSET_PRECISION,
				RequoteThreshold: gf.marketConfig.BootstrapPrices[symbol] / 10000,
				Precision:        ASSET_PRECISION,
				BootstrapPrice:   gf.marketConfig.BootstrapPrices[symbol],
			})
			subActors = append(subActors, sub)
			gf.nextActorID++
		}
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"ABC": 2 * ASSET_PRECISION,
		"BCD": 4 * ASSET_PRECISION,
		"CDE": 10 * ASSET_PRECISION,
		"DEF": 20 * ASSET_PRECISION,
		"EFG": 100 * ASSET_PRECISION,
	}
	composite.InitializeBalances(baseBalances, 1_000_000*USD_PRECISION)

	return composite
}

func (gf *GroupFactory) CreateSpotMMGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 2 * ASSET_PRECISION,
		"BCD": 4 * ASSET_PRECISION,
		"CDE": 10 * ASSET_PRECISION,
		"DEF": 20 * ASSET_PRECISION,
		"EFG": 100 * ASSET_PRECISION,
		"USD": 1_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(500*time.Microsecond, 1*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	spotSymbols := []string{"ABC/USD", "BCD/USD", "CDE/USD", "DEF/USD", "EFG/USD"}
	subActors := []actor.SubActor{}

	for _, symbol := range spotSymbols {
		for _, spreadBps := range []int64{5, 10, 15} {
			sub := actors.NewPureMMSubActor(gf.nextActorID, symbol, actors.PureMMSubActorConfig{
				SpreadBps:        spreadBps,
				QuoteSize:        ASSET_PRECISION / 10,
				MaxInventory:     5 * ASSET_PRECISION,
				RequoteThreshold: gf.marketConfig.BootstrapPrices[symbol] / 10000,
				Precision:        ASSET_PRECISION,
				BootstrapPrice:   gf.marketConfig.BootstrapPrices[symbol],
			})
			subActors = append(subActors, sub)
			gf.nextActorID++
		}
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"ABC": 2 * ASSET_PRECISION,
		"BCD": 4 * ASSET_PRECISION,
		"CDE": 10 * ASSET_PRECISION,
		"DEF": 20 * ASSET_PRECISION,
		"EFG": 100 * ASSET_PRECISION,
	}
	composite.InitializeBalances(baseBalances, 1_000_000*USD_PRECISION)

	return composite
}

// CreateSpotABCMMGroup creates market makers for the four ABC-quoted cross pairs:
// BCD/ABC, CDE/ABC, DEF/ABC, EFG/ABC.
// The group's quoteBalance represents ABC (not USD).
func (gf *GroupFactory) CreateSpotABCMMGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"BCD": 50 * ASSET_PRECISION,
		"CDE": 50 * ASSET_PRECISION,
		"DEF": 50 * ASSET_PRECISION,
		"EFG": 50 * ASSET_PRECISION,
		"ABC": 200 * ASSET_PRECISION, // quote for ABC-denominated bids
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(500*time.Microsecond, 1*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	abcSymbols := []string{"BCD/ABC", "CDE/ABC", "DEF/ABC", "EFG/ABC"}
	subActors := []actor.SubActor{}

	for _, symbol := range abcSymbols {
		for _, spreadBps := range []int64{5, 10, 15} {
			sub := actors.NewPureMMSubActor(gf.nextActorID, symbol, actors.PureMMSubActorConfig{
				SpreadBps:        spreadBps,
				QuoteSize:        ASSET_PRECISION / 10,
				MaxInventory:     5 * ASSET_PRECISION,
				RequoteThreshold: gf.marketConfig.BootstrapPrices[symbol] / 1000,
				Precision:        ASSET_PRECISION, // both base and quote use ASSET_PRECISION
				BootstrapPrice:   gf.marketConfig.BootstrapPrices[symbol],
			})
			subActors = append(subActors, sub)
			gf.nextActorID++
		}
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"BCD": 50 * ASSET_PRECISION,
		"CDE": 50 * ASSET_PRECISION,
		"DEF": 50 * ASSET_PRECISION,
		"EFG": 50 * ASSET_PRECISION,
	}
	// quoteBalance here represents ABC (the quote asset for all symbols in this group).
	composite.InitializeBalances(baseBalances, 200*ASSET_PRECISION)

	return composite
}

func (gf *GroupFactory) CreateSpotTakerGroup() *actor.CompositeActor {
	// Large initial balances: each taker burns ~0.055 base/trade at 1-2s intervals.
	// 1000× headroom keeps base balances positive for the full 24h sim.
	initialBalances := map[string]int64{
		"ABC": 1000 * ASSET_PRECISION,
		"BCD": 2000 * ASSET_PRECISION,
		"CDE": 5000 * ASSET_PRECISION,
		"DEF": 10000 * ASSET_PRECISION,
		"EFG": 50000 * ASSET_PRECISION,
		"USD": 5_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 10, TakerBps: 15, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(5*time.Millisecond, 10*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	spotSymbols := []string{"ABC/USD", "BCD/USD", "CDE/USD", "DEF/USD", "EFG/USD"}
	subActors := []actor.SubActor{}

	seed := int64(gf.nextActorID)
	for _, symbol := range spotSymbols {
		inst := gf.marketConfig.Instruments[symbol]
		for i := range 2 {
			sub := actors.NewRandomTakerSubActor(gf.nextActorID, symbol, actors.RandomTakerSubActorConfig{
				Interval:    time.Duration(1000+i*500) * time.Millisecond,
				MinQty:      ASSET_PRECISION / 100,
				MaxQty:      ASSET_PRECISION / 10,
				Precision:   ASSET_PRECISION,
				Instrument:  inst,
				TakerFeeBps: 15,
			}, seed)
			subActors = append(subActors, sub)
			gf.nextActorID++
			seed++
		}
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"ABC": 1000 * ASSET_PRECISION,
		"BCD": 2000 * ASSET_PRECISION,
		"CDE": 5000 * ASSET_PRECISION,
		"DEF": 10000 * ASSET_PRECISION,
		"EFG": 50000 * ASSET_PRECISION,
	}
	composite.InitializeBalances(baseBalances, 5_000_000*USD_PRECISION)

	return composite
}

// CreateSpotABCTakerGroup creates random takers for the four ABC-quoted cross pairs.
// The group's quoteBalance represents ABC.
func (gf *GroupFactory) CreateSpotABCTakerGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"BCD": 2000 * ASSET_PRECISION,
		"CDE": 5000 * ASSET_PRECISION,
		"DEF": 10000 * ASSET_PRECISION,
		"EFG": 50000 * ASSET_PRECISION,
		"ABC": 10_000 * ASSET_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 10, TakerBps: 15, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(5*time.Millisecond, 10*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	abcSymbols := []string{"BCD/ABC", "CDE/ABC", "DEF/ABC", "EFG/ABC"}
	subActors := []actor.SubActor{}

	seed := int64(gf.nextActorID + 500)
	for _, symbol := range abcSymbols {
		inst := gf.marketConfig.Instruments[symbol]
		for i := range 2 {
			sub := actors.NewRandomTakerSubActor(gf.nextActorID, symbol, actors.RandomTakerSubActorConfig{
				Interval:    time.Duration(1000+i*500) * time.Millisecond,
				MinQty:      ASSET_PRECISION / 100,
				MaxQty:      ASSET_PRECISION / 10,
				Precision:   ASSET_PRECISION,
				Instrument:  inst,
				TakerFeeBps: 15,
			}, seed)
			subActors = append(subActors, sub)
			gf.nextActorID++
			seed++
		}
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"BCD": 2000 * ASSET_PRECISION,
		"CDE": 5000 * ASSET_PRECISION,
		"DEF": 10000 * ASSET_PRECISION,
		"EFG": 50000 * ASSET_PRECISION,
	}
	// quoteBalance represents ABC for this group.
	composite.InitializeBalances(baseBalances, 10_000*ASSET_PRECISION)

	return composite
}

func (gf *GroupFactory) CreatePerpTakerGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 1000 * ASSET_PRECISION,
		"BCD": 2000 * ASSET_PRECISION,
		"CDE": 5000 * ASSET_PRECISION,
		"DEF": 10000 * ASSET_PRECISION,
		"EFG": 50000 * ASSET_PRECISION,
		"USD": 5_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 10, TakerBps: 15, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)

	for asset, balance := range initialBalances {
		gf.ex.AddPerpBalance(gf.nextClientID, asset, balance)
	}
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(5*time.Millisecond, 10*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	perpSymbols := []string{"ABC-PERP", "BCD-PERP", "CDE-PERP", "DEF-PERP", "EFG-PERP"}
	subActors := []actor.SubActor{}

	seed := int64(gf.nextActorID)
	for _, symbol := range perpSymbols {
		inst := gf.marketConfig.Instruments[symbol]
		for i := range 2 {
			sub := actors.NewRandomTakerSubActor(gf.nextActorID, symbol, actors.RandomTakerSubActorConfig{
				Interval:    time.Duration(1000+i*500) * time.Millisecond,
				MinQty:      ASSET_PRECISION / 100,
				MaxQty:      ASSET_PRECISION / 10,
				Precision:   ASSET_PRECISION,
				Instrument:  inst,  // IsPerp() == true: skips spot base-balance sell check
				TakerFeeBps: 15,
			}, seed)
			subActors = append(subActors, sub)
			gf.nextActorID++
			seed++
		}
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"ABC": 1000 * ASSET_PRECISION,
		"BCD": 2000 * ASSET_PRECISION,
		"CDE": 5000 * ASSET_PRECISION,
		"DEF": 10000 * ASSET_PRECISION,
		"EFG": 50000 * ASSET_PRECISION,
	}
	composite.InitializeBalances(baseBalances, 5_000_000*USD_PRECISION)

	return composite
}

func (gf *GroupFactory) CreateFundingArbGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 3 * ASSET_PRECISION,
		"BCD": 6 * ASSET_PRECISION,
		"CDE": 15 * ASSET_PRECISION,
		"DEF": 30 * ASSET_PRECISION,
		"EFG": 150 * ASSET_PRECISION,
		"USD": 1_500_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 5, TakerBps: 8, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)

	for asset, balance := range initialBalances {
		gf.ex.AddPerpBalance(gf.nextClientID, asset, balance)
	}
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(2*time.Millisecond, 3*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	assets := []string{"ABC", "BCD", "CDE", "DEF", "EFG"}
	subActors := []actor.SubActor{}

	for _, asset := range assets {
		spotSymbol := asset + "/USD"
		perpSymbol := asset + "-PERP"

		sub := actors.NewInternalFundingArb(actors.InternalFundingArbConfig{
			ActorID:         gf.nextActorID,
			SpotSymbol:      spotSymbol,
			PerpSymbol:      perpSymbol,
			SpotInstrument:  gf.marketConfig.Instruments[spotSymbol],
			PerpInstrument:  gf.marketConfig.Instruments[perpSymbol],
			MinFundingRate:  20,
			ExitFundingRate: 5,
			MaxPositionSize: ASSET_PRECISION,
		})
		subActors = append(subActors, sub)
		gf.nextActorID++
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"ABC": 3 * ASSET_PRECISION,
		"BCD": 6 * ASSET_PRECISION,
		"CDE": 15 * ASSET_PRECISION,
		"DEF": 30 * ASSET_PRECISION,
		"EFG": 150 * ASSET_PRECISION,
	}
	composite.InitializeBalances(baseBalances, 1_500_000*USD_PRECISION)

	return composite
}

func (gf *GroupFactory) CreateTriangleArbGroup() *actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 40 * ASSET_PRECISION,
		"BCD": 80 * ASSET_PRECISION,
		"CDE": 200 * ASSET_PRECISION,
		"DEF": 400 * ASSET_PRECISION,
		"EFG": 2000 * ASSET_PRECISION,
		"USD": 2_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 5, TakerBps: 8, InQuote: true}
	gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
	gf.nextClientID++

	latencyConfig := simulation.LatencyConfig{
		MarketDataLatency: simulation.NewUniformRandomLatency(1*time.Millisecond, 2*time.Millisecond, int64(gf.nextClientID)),
	}
	delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
	delayedGateway.Start()
	wrappedGateway := delayedGateway.ToClientGateway()

	abcQuotedAssets := []string{"BCD", "CDE", "DEF", "EFG"}
	subActors := []actor.SubActor{}

	for _, asset := range abcQuotedAssets {
		baseSymbol := asset + "/USD"   // e.g. BCD/USD: sell BCD to close circuit
		crossSymbol := asset + "/ABC"  // e.g. BCD/ABC: buy BCD with ABC
		directSymbol := "ABC/USD"      // buy ABC with USD to open circuit

		sub := actors.NewTriangleArbitrage(actors.TriangleArbConfig{
			ActorID:          gf.nextActorID,
			BaseSymbol:       baseSymbol,
			CrossSymbol:      crossSymbol,
			DirectSymbol:     directSymbol,
			BaseInstrument:   gf.marketConfig.Instruments[baseSymbol],
			CrossInstrument:  gf.marketConfig.Instruments[crossSymbol],
			DirectInstrument: gf.marketConfig.Instruments[directSymbol],
			ThresholdBps:     5,
			MaxTradeSize:     ASSET_PRECISION / 10,
			TakerFeeBps:      24, // 8bps × 3 legs
		})
		subActors = append(subActors, sub)
		gf.nextActorID++
	}

	composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, subActors)
	gf.nextActorID++

	baseBalances := map[string]int64{
		"ABC": 40 * ASSET_PRECISION,
		"BCD": 80 * ASSET_PRECISION,
		"CDE": 200 * ASSET_PRECISION,
		"DEF": 400 * ASSET_PRECISION,
		"EFG": 2000 * ASSET_PRECISION,
	}
	composite.InitializeBalances(baseBalances, 2_000_000*USD_PRECISION)

	return composite
}
