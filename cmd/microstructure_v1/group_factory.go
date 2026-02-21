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

func (gf *GroupFactory) CreatePerpMMGroups() []*actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 100 * ASSET_PRECISION,
		"BCD": 200 * ASSET_PRECISION,
		"CDE": 500 * ASSET_PRECISION,
		"DEF": 1000 * ASSET_PRECISION,
		"EFG": 5000 * ASSET_PRECISION,
		"USD": 100_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	perpSymbols := []string{"ABC-PERP", "BCD-PERP", "CDE-PERP", "DEF-PERP", "EFG-PERP"}
	groups := []*actor.CompositeActor{}

	for _, symbol := range perpSymbols {
		for _, spreadBps := range []int64{5, 10, 15} {
			gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
			for asset, balance := range initialBalances {
				gf.ex.AddPerpBalance(gf.nextClientID, asset, balance)
			}
			gf.nextClientID++

			latencyConfig := simulation.LatencyConfig{
				Mode:              simulation.LatencyMarketData,
				MarketDataLatency: simulation.NewUniformRandomLatency(1*time.Millisecond, 2*time.Millisecond, int64(gf.nextClientID)),
			}
			delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
			delayedGateway.Start()
			wrappedGateway := delayedGateway.ToClientGateway()

			sub := actors.NewPureMMSubActor(gf.nextActorID, symbol, actors.PureMMSubActorConfig{
				SpreadBps:        spreadBps,
				QuoteSize:        ASSET_PRECISION * 5, // Increased volume
				MaxInventory:     100 * ASSET_PRECISION,
				RequoteThreshold: gf.marketConfig.BootstrapPrices[symbol] / 10000,
				Precision:        ASSET_PRECISION,
				BootstrapPrice:   gf.marketConfig.BootstrapPrices[symbol],
			})
			gf.nextActorID++

			composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
			gf.nextActorID++
			composite.InitializeBalances(initialBalances, 100_000_000*USD_PRECISION)
			groups = append(groups, composite)
		}
	}

	return groups
}

func (gf *GroupFactory) CreateSpotMMGroups() []*actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 100 * ASSET_PRECISION,
		"BCD": 200 * ASSET_PRECISION,
		"CDE": 500 * ASSET_PRECISION,
		"DEF": 1000 * ASSET_PRECISION,
		"EFG": 5000 * ASSET_PRECISION,
		"USD": 100_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	spotSymbols := []string{"ABC/USD", "BCD/USD", "CDE/USD", "DEF/USD", "EFG/USD"}
	groups := []*actor.CompositeActor{}

	for _, symbol := range spotSymbols {
		for _, spreadBps := range []int64{5, 10, 15} {
			gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
			gf.nextClientID++

			latencyConfig := simulation.LatencyConfig{
				Mode:              simulation.LatencyMarketData,
				MarketDataLatency: simulation.NewUniformRandomLatency(500*time.Microsecond, 1*time.Millisecond, int64(gf.nextClientID)),
			}
			delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
			delayedGateway.Start()
			wrappedGateway := delayedGateway.ToClientGateway()

			sub := actors.NewPureMMSubActor(gf.nextActorID, symbol, actors.PureMMSubActorConfig{
				SpreadBps:        spreadBps,
				QuoteSize:        ASSET_PRECISION * 5, // Increased volume
				MaxInventory:     100 * ASSET_PRECISION,
				RequoteThreshold: gf.marketConfig.BootstrapPrices[symbol] / 10000,
				Precision:        ASSET_PRECISION,
				BootstrapPrice:   gf.marketConfig.BootstrapPrices[symbol],
			})
			gf.nextActorID++

			composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
			gf.nextActorID++
			composite.InitializeBalances(initialBalances, 100_000_000*USD_PRECISION)
			groups = append(groups, composite)
		}
	}

	return groups
}

func (gf *GroupFactory) CreateSpotABCMMGroups() []*actor.CompositeActor {
	initialBalances := map[string]int64{
		"BCD": 500 * ASSET_PRECISION,
		"CDE": 500 * ASSET_PRECISION,
		"DEF": 500 * ASSET_PRECISION,
		"EFG": 500 * ASSET_PRECISION,
		"ABC": 10_000 * ASSET_PRECISION, // High quote balance for ABC bids
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	abcSymbols := []string{"BCD/ABC", "CDE/ABC", "DEF/ABC", "EFG/ABC"}
	groups := []*actor.CompositeActor{}

	for _, symbol := range abcSymbols {
		for _, spreadBps := range []int64{5, 10, 15} {
			gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
			gf.nextClientID++

			latencyConfig := simulation.LatencyConfig{
				Mode:              simulation.LatencyMarketData,
				MarketDataLatency: simulation.NewUniformRandomLatency(500*time.Microsecond, 1*time.Millisecond, int64(gf.nextClientID)),
			}
			delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
			delayedGateway.Start()
			wrappedGateway := delayedGateway.ToClientGateway()

			sub := actors.NewPureMMSubActor(gf.nextActorID, symbol, actors.PureMMSubActorConfig{
				SpreadBps:        spreadBps,
				QuoteSize:        ASSET_PRECISION * 5,
				MaxInventory:     100 * ASSET_PRECISION,
				RequoteThreshold: gf.marketConfig.BootstrapPrices[symbol] / 500,
				Precision:        ASSET_PRECISION,
				BootstrapPrice:   gf.marketConfig.BootstrapPrices[symbol],
			})
			gf.nextActorID++

			composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
			gf.nextActorID++
			composite.InitializeBalances(initialBalances, 10_000*ASSET_PRECISION)
			groups = append(groups, composite)
		}
	}

	return groups
}

func (gf *GroupFactory) CreateSpotTakerGroups() []*actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 10_000 * ASSET_PRECISION,
		"BCD": 20_000 * ASSET_PRECISION,
		"CDE": 50_000 * ASSET_PRECISION,
		"DEF": 100_000 * ASSET_PRECISION,
		"EFG": 500_000 * ASSET_PRECISION,
		"USD": 100_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 10, TakerBps: 15, InQuote: true}
	spotSymbols := []string{"ABC/USD", "BCD/USD", "CDE/USD", "DEF/USD", "EFG/USD"}
	groups := []*actor.CompositeActor{}

	seed := int64(gf.nextActorID)
	for _, symbol := range spotSymbols {
		inst := gf.marketConfig.Instruments[symbol]
		for i := range 2 {
			gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
			gf.nextClientID++

			latencyConfig := simulation.LatencyConfig{
				Mode:              simulation.LatencyMarketData,
				MarketDataLatency: simulation.NewUniformRandomLatency(5*time.Millisecond, 10*time.Millisecond, int64(gf.nextClientID)),
			}
			delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
			delayedGateway.Start()
			wrappedGateway := delayedGateway.ToClientGateway()

			sub := actors.NewRandomTakerSubActor(gf.nextActorID, symbol, actors.RandomTakerSubActorConfig{
				Interval:    time.Duration(1000+i*500) * time.Millisecond,
				MinQty:      ASSET_PRECISION / 5,
				MaxQty:      2 * ASSET_PRECISION,
				Precision:   ASSET_PRECISION,
				Instrument:  inst,
				TakerFeeBps: 15,
			}, seed)
			gf.nextActorID++
			seed++

			composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
			gf.nextActorID++
			composite.InitializeBalances(initialBalances, 100_000_000*USD_PRECISION)
			groups = append(groups, composite)
		}
	}

	return groups
}

func (gf *GroupFactory) CreateSpotABCTakerGroups() []*actor.CompositeActor {
	initialBalances := map[string]int64{
		"BCD": 20_000 * ASSET_PRECISION,
		"CDE": 50_000 * ASSET_PRECISION,
		"DEF": 100_000 * ASSET_PRECISION,
		"EFG": 500_000 * ASSET_PRECISION,
		"ABC": 100_000 * ASSET_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 10, TakerBps: 15, InQuote: true}
	abcSymbols := []string{"BCD/ABC", "CDE/ABC", "DEF/ABC", "EFG/ABC"}
	groups := []*actor.CompositeActor{}

	seed := int64(gf.nextActorID + 500)
	for _, symbol := range abcSymbols {
		inst := gf.marketConfig.Instruments[symbol]
		for i := range 2 {
			gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
			gf.nextClientID++

			latencyConfig := simulation.LatencyConfig{
				Mode:              simulation.LatencyMarketData,
				MarketDataLatency: simulation.NewUniformRandomLatency(5*time.Millisecond, 10*time.Millisecond, int64(gf.nextClientID)),
			}
			delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
			delayedGateway.Start()
			wrappedGateway := delayedGateway.ToClientGateway()

			sub := actors.NewRandomTakerSubActor(gf.nextActorID, symbol, actors.RandomTakerSubActorConfig{
				Interval:    time.Duration(1000+i*500) * time.Millisecond,
				MinQty:      ASSET_PRECISION / 5,
				MaxQty:      2 * ASSET_PRECISION,
				Precision:   ASSET_PRECISION,
				Instrument:  inst,
				TakerFeeBps: 15,
			}, seed)
			gf.nextActorID++
			seed++

			composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
			gf.nextActorID++
			composite.InitializeBalances(initialBalances, 100_000*ASSET_PRECISION)
			groups = append(groups, composite)
		}
	}

	return groups
}

func (gf *GroupFactory) CreatePerpTakerGroups() []*actor.CompositeActor {
	initialBalances := map[string]int64{
		"ABC": 10_000 * ASSET_PRECISION,
		"BCD": 20_000 * ASSET_PRECISION,
		"CDE": 50_000 * ASSET_PRECISION,
		"DEF": 100_000 * ASSET_PRECISION,
		"EFG": 500_000 * ASSET_PRECISION,
		"USD": 100_000_000 * USD_PRECISION,
	}

	fees := &exchange.PercentageFee{MakerBps: 10, TakerBps: 15, InQuote: true}
	perpSymbols := []string{"ABC-PERP", "BCD-PERP", "CDE-PERP", "DEF-PERP", "EFG-PERP"}
	groups := []*actor.CompositeActor{}

	seed := int64(gf.nextActorID)
	for _, symbol := range perpSymbols {
		inst := gf.marketConfig.Instruments[symbol]
		for i := range 2 {
			gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
			for asset, balance := range initialBalances {
				gf.ex.AddPerpBalance(gf.nextClientID, asset, balance)
			}
			gf.nextClientID++

			latencyConfig := simulation.LatencyConfig{
				Mode:              simulation.LatencyMarketData,
				MarketDataLatency: simulation.NewUniformRandomLatency(5*time.Millisecond, 10*time.Millisecond, int64(gf.nextClientID)),
			}
			delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
			delayedGateway.Start()
			wrappedGateway := delayedGateway.ToClientGateway()

			sub := actors.NewRandomTakerSubActor(gf.nextActorID, symbol, actors.RandomTakerSubActorConfig{
				Interval:    time.Duration(1000+i*500) * time.Millisecond,
				MinQty:      ASSET_PRECISION / 5,
				MaxQty:      2 * ASSET_PRECISION,
				Precision:   ASSET_PRECISION,
				Instrument:  inst,
				TakerFeeBps: 15,
			}, seed)
			gf.nextActorID++
			seed++

			composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
			gf.nextActorID++
			composite.InitializeBalances(initialBalances, 100_000_000*USD_PRECISION)
			groups = append(groups, composite)
		}
	}

	return groups
}

func (gf *GroupFactory) CreateFundingArbGroups() []*actor.CompositeActor {
	type assetCfg struct {
		asset   string
		baseQty int64
		usdBal  int64
	}
	perAsset := []assetCfg{
		{"ABC", 1000 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"BCD", 2000 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"CDE", 5000 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"DEF", 10000 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"EFG", 50000 * ASSET_PRECISION, 20_000_000 * USD_PRECISION},
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 2, InQuote: true}
	groups := make([]*actor.CompositeActor, 0, len(perAsset))

	for _, cfg := range perAsset {
		initialBalances := map[string]int64{
			cfg.asset: cfg.baseQty,
			"USD":     cfg.usdBal,
		}
		gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
		gf.ex.AddPerpBalance(gf.nextClientID, "USD", cfg.usdBal) // For perp side
		gf.nextClientID++

		latencyConfig := simulation.LatencyConfig{
			Mode:              simulation.LatencyMarketData,
			MarketDataLatency: simulation.NewUniformRandomLatency(2*time.Millisecond, 3*time.Millisecond, int64(gf.nextClientID)),
		}
		delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
		delayedGateway.Start()
		wrappedGateway := delayedGateway.ToClientGateway()

		spotSymbol := cfg.asset + "/USD"
		perpSymbol := cfg.asset + "-PERP"
		sub := actors.NewInternalFundingArb(actors.InternalFundingArbConfig{
			ActorID:         gf.nextActorID,
			SpotSymbol:      spotSymbol,
			PerpSymbol:      perpSymbol,
			SpotInstrument:  gf.marketConfig.Instruments[spotSymbol],
			PerpInstrument:  gf.marketConfig.Instruments[perpSymbol],
			MinFundingRate:  3,
			ExitFundingRate: 1,
			MaxPositionSize: 100 * ASSET_PRECISION, // Increased limit
		})
		gf.nextActorID++

		composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
		gf.nextActorID++
		composite.InitializeBalances(map[string]int64{cfg.asset: cfg.baseQty}, cfg.usdBal)
		groups = append(groups, composite)
	}

	return groups
}

func (gf *GroupFactory) CreateTriangleArbGroups() []*actor.CompositeActor {
	type assetCfg struct {
		asset   string
		baseQty int64
		abcQty  int64
		usdBal  int64
	}
	perAsset := []assetCfg{
		{"BCD", 1000 * ASSET_PRECISION, 100 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"CDE", 1000 * ASSET_PRECISION, 100 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"DEF", 1000 * ASSET_PRECISION, 100 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
		{"EFG", 1000 * ASSET_PRECISION, 100 * ASSET_PRECISION, 10_000_000 * USD_PRECISION},
	}

	fees := &exchange.PercentageFee{MakerBps: 0, TakerBps: 1, InQuote: true}
	groups := make([]*actor.CompositeActor, 0, len(perAsset))

	for _, cfg := range perAsset {
		initialBalances := map[string]int64{
			cfg.asset: cfg.baseQty,
			"ABC":     cfg.abcQty,
			"USD":     cfg.usdBal,
		}
		gateway := gf.ex.ConnectClient(gf.nextClientID, initialBalances, fees)
		gf.nextClientID++

		latencyConfig := simulation.LatencyConfig{
			Mode:              simulation.LatencyMarketData,
			MarketDataLatency: simulation.NewUniformRandomLatency(1*time.Millisecond, 2*time.Millisecond, int64(gf.nextClientID)),
		}
		delayedGateway := simulation.NewDelayedGateway(gateway, latencyConfig)
		delayedGateway.Start()
		wrappedGateway := delayedGateway.ToClientGateway()

		baseSymbol := cfg.asset + "/USD"
		crossSymbol := cfg.asset + "/ABC"
		directSymbol := "ABC/USD"

		sub := actors.NewTriangleArbitrage(actors.TriangleArbConfig{
			ActorID:          gf.nextActorID,
			BaseSymbol:       baseSymbol,
			CrossSymbol:      crossSymbol,
			DirectSymbol:     directSymbol,
			BaseInstrument:   gf.marketConfig.Instruments[baseSymbol],
			CrossInstrument:  gf.marketConfig.Instruments[crossSymbol],
			DirectInstrument: gf.marketConfig.Instruments[directSymbol],
			ThresholdBps:     1,
			MaxTradeSize:     ASSET_PRECISION * 5, // Increased volume
			TakerFeeBps:      3,
		})
		gf.nextActorID++

		composite := actor.NewCompositeActor(gf.nextActorID, wrappedGateway, []actor.SubActor{sub})
		gf.nextActorID++
		composite.InitializeBalances(map[string]int64{cfg.asset: cfg.baseQty, "ABC": cfg.abcQty}, cfg.usdBal)
		groups = append(groups, composite)
	}

	return groups
}
