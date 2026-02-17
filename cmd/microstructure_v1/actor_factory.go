package main

import (
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/actors"
)

type ActorFactory struct {
	marketConfig *MarketConfig
	nextActorID  uint64
}

func NewActorFactory(marketConfig *MarketConfig) *ActorFactory {
	return &ActorFactory{
		marketConfig: marketConfig,
		nextActorID:  1,
	}
}

func (af *ActorFactory) nextID() uint64 {
	id := af.nextActorID
	af.nextActorID++
	return id
}

func (af *ActorFactory) CreateAvellanedaStoikovMakers(gateway *exchange.ClientGateway) []actor.Actor {
	configs := []struct {
		Gamma            int64
		RequoteInterval  time.Duration
		VolatilityWindow int
	}{
		{100, 500 * time.Millisecond, 20},
		{200, 750 * time.Millisecond, 15},
		{300, 1000 * time.Millisecond, 10},
		{400, 1250 * time.Millisecond, 25},
		{500, 1500 * time.Millisecond, 30},
	}

	allSymbols := getAllSymbols(af.marketConfig.Instruments)
	result := make([]actor.Actor, 0, len(allSymbols)*len(configs))

	for _, symbol := range allSymbols {
		maxInv := int64(10 * ASSET_PRECISION)
		if symbol[len(symbol)-4:] == "PERP" {
			maxInv = 5 * ASSET_PRECISION
		}

		for _, cfg := range configs {
			asActor := actors.NewAvellanedaStoikov(af.nextID(), gateway, actors.AvellanedaStoikovConfig{
				Symbol:           symbol,
				Gamma:            cfg.Gamma,
				K:                10000,
				T:                3600,
				QuoteQty:         ASSET_PRECISION / 10,
				MaxInventory:     maxInv,
				VolatilityWindow: cfg.VolatilityWindow,
				RequoteInterval:  cfg.RequoteInterval,
			})
			result = append(result, asActor)
		}
	}

	return result
}

func (af *ActorFactory) CreatePureMarketMakers(gateway *exchange.ClientGateway) []actor.Actor {
	configs := []struct {
		SpreadBps       int64
		MonitorInterval time.Duration
	}{
		{5, 50 * time.Millisecond},
		{10, 100 * time.Millisecond},
		{20, 150 * time.Millisecond},
		{35, 175 * time.Millisecond},
		{50, 200 * time.Millisecond},
	}

	allSymbols := getAllSymbols(af.marketConfig.Instruments)
	result := make([]actor.Actor, 0, len(allSymbols)*len(configs))

	for _, symbol := range allSymbols {
		inst := af.marketConfig.Instruments[symbol]
		maxInv := int64(10 * ASSET_PRECISION)
		if symbol[len(symbol)-4:] == "PERP" {
			maxInv = 5 * ASSET_PRECISION
		}

		for _, cfg := range configs {
			pmm := actors.NewPureMarketMaker(af.nextID(), gateway, actors.PureMarketMakerConfig{
				Symbol:           symbol,
				Instrument:       inst,
				SpreadBps:        cfg.SpreadBps,
				QuoteSize:        ASSET_PRECISION / 10,
				MaxInventory:     maxInv,
				RequoteThreshold: 5,
				MonitorInterval:  cfg.MonitorInterval,
				BootstrapPrice:   af.marketConfig.BootstrapPrices[symbol],
			})
			result = append(result, pmm)
		}
	}

	return result
}

func (af *ActorFactory) CreateMultiSymbolMakers(gateway *exchange.ClientGateway) []actor.Actor {
	symbolsByType := GetSymbolsByType()
	result := make([]actor.Actor, 0, 10)

	usdSpotInst := make(map[string]exchange.Instrument)
	for _, sym := range symbolsByType["usd_spot"] {
		usdSpotInst[sym] = af.marketConfig.Instruments[sym]
	}
	mm1 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          symbolsByType["usd_spot"],
		Instruments:      usdSpotInst,
		SpreadBps:        10,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     10 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  50 * time.Millisecond,
	})
	result = append(result, mm1)

	mm2 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          symbolsByType["usd_spot"],
		Instruments:      usdSpotInst,
		SpreadBps:        15,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     10 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  75 * time.Millisecond,
	})
	result = append(result, mm2)

	perpInst := make(map[string]exchange.Instrument)
	for _, sym := range symbolsByType["perps"] {
		perpInst[sym] = af.marketConfig.Instruments[sym]
	}
	mm3 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          symbolsByType["perps"],
		Instruments:      perpInst,
		SpreadBps:        15,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     5 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  50 * time.Millisecond,
	})
	result = append(result, mm3)

	mm4 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          symbolsByType["perps"],
		Instruments:      perpInst,
		SpreadBps:        20,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     5 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  100 * time.Millisecond,
	})
	result = append(result, mm4)

	abcSpotFirst := symbolsByType["abc_spot"][:2]
	abcInst1 := make(map[string]exchange.Instrument)
	for _, sym := range abcSpotFirst {
		abcInst1[sym] = af.marketConfig.Instruments[sym]
	}
	mm5 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          abcSpotFirst,
		Instruments:      abcInst1,
		SpreadBps:        20,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     10 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  50 * time.Millisecond,
	})
	result = append(result, mm5)

	mm6 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          abcSpotFirst,
		Instruments:      abcInst1,
		SpreadBps:        25,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     10 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  75 * time.Millisecond,
	})
	result = append(result, mm6)

	abcSpotLast := symbolsByType["abc_spot"][2:]
	abcInst2 := make(map[string]exchange.Instrument)
	for _, sym := range abcSpotLast {
		abcInst2[sym] = af.marketConfig.Instruments[sym]
	}
	mm7 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          abcSpotLast,
		Instruments:      abcInst2,
		SpreadBps:        20,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     10 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  50 * time.Millisecond,
	})
	result = append(result, mm7)

	mm8 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          abcSpotLast,
		Instruments:      abcInst2,
		SpreadBps:        25,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     10 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  75 * time.Millisecond,
	})
	result = append(result, mm8)

	mixedABC := []string{"ABC/USD", "ABC-PERP"}
	mixedInst1 := make(map[string]exchange.Instrument)
	for _, sym := range mixedABC {
		mixedInst1[sym] = af.marketConfig.Instruments[sym]
	}
	mm9 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          mixedABC,
		Instruments:      mixedInst1,
		SpreadBps:        12,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     8 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  60 * time.Millisecond,
	})
	result = append(result, mm9)

	mixedBCD := []string{"BCD/USD", "BCD-PERP"}
	mixedInst2 := make(map[string]exchange.Instrument)
	for _, sym := range mixedBCD {
		mixedInst2[sym] = af.marketConfig.Instruments[sym]
	}
	mm10 := actor.NewMultiSymbolMM(af.nextID(), gateway, actor.MultiSymbolMMConfig{
		Symbols:          mixedBCD,
		Instruments:      mixedInst2,
		SpreadBps:        12,
		QuoteSize:        ASSET_PRECISION / 10,
		MaxInventory:     8 * ASSET_PRECISION,
		RequoteThreshold: 5,
		MonitorInterval:  60 * time.Millisecond,
	})
	result = append(result, mm10)

	return result
}

func (af *ActorFactory) CreateFundingArbActors(gateway *exchange.ClientGateway) []*actor.CompositeActor {
	result := make([]*actor.CompositeActor, 0, 5)

	for _, asset := range af.marketConfig.Assets {
		spotSymbol := asset + "/USD"
		perpSymbol := asset + "-PERP"

		subActor := actors.NewInternalFundingArb(actors.InternalFundingArbConfig{
			ActorID:         af.nextID(),
			SpotSymbol:      spotSymbol,
			PerpSymbol:      perpSymbol,
			SpotInstrument:  af.marketConfig.Instruments[spotSymbol],
			PerpInstrument:  af.marketConfig.Instruments[perpSymbol],
			MinFundingRate:  20,
			ExitFundingRate: 5,
			MaxPositionSize: ASSET_PRECISION,
		})

		composite := actor.NewCompositeActor(af.nextID(), gateway, []actor.SubActor{subActor})
		composite.InitializeBalances(map[string]int64{asset: 10 * ASSET_PRECISION}, 1_000_000*USD_PRECISION)

		result = append(result, composite)
	}

	return result
}

func (af *ActorFactory) CreateTriangleArbActor(gateway *exchange.ClientGateway) *actor.CompositeActor {
	triangles := []struct {
		base   string
		cross  string
		direct string
	}{
		{"ABC/USD", "BCD/ABC", "BCD/USD"},
		{"ABC/USD", "CDE/ABC", "CDE/USD"},
		{"ABC/USD", "DEF/ABC", "DEF/USD"},
		{"ABC/USD", "EFG/ABC", "EFG/USD"},
	}

	subActors := make([]actor.SubActor, len(triangles))
	for i, tri := range triangles {
		subActors[i] = actors.NewTriangleArbitrage(actors.TriangleArbConfig{
			ActorID:          af.nextID(),
			BaseSymbol:       tri.base,
			CrossSymbol:      tri.cross,
			DirectSymbol:     tri.direct,
			BaseInstrument:   af.marketConfig.Instruments[tri.base],
			CrossInstrument:  af.marketConfig.Instruments[tri.cross],
			DirectInstrument: af.marketConfig.Instruments[tri.direct],
			ThresholdBps:     20,
			MaxTradeSize:     ASSET_PRECISION / 10,
		})
	}

	composite := actor.NewCompositeActor(af.nextID(), gateway, subActors)
	composite.InitializeBalances(map[string]int64{}, 1_000_000*USD_PRECISION)

	return composite
}

func (af *ActorFactory) CreateRandomizedTakers(gateway *exchange.ClientGateway) []actor.Actor {
	symbolsByType := GetSymbolsByType()
	result := make([]actor.Actor, 0, 20)

	for _, symbol := range symbolsByType["usd_spot"] {
		rt1 := actors.NewRandomizedTaker(af.nextID(), gateway, actors.RandomizedTakerConfig{
			Symbol:         symbol,
			Interval:       1000 * time.Millisecond,
			MinQty:         ASSET_PRECISION / 100,
			MaxQty:         ASSET_PRECISION / 2,
			BasePrecision:  ASSET_PRECISION,
			QuotePrecision: USD_PRECISION,
		})
		result = append(result, rt1)

		rt2 := actors.NewRandomizedTaker(af.nextID(), gateway, actors.RandomizedTakerConfig{
			Symbol:         symbol,
			Interval:       2000 * time.Millisecond,
			MinQty:         ASSET_PRECISION / 100,
			MaxQty:         ASSET_PRECISION / 2,
			BasePrecision:  ASSET_PRECISION,
			QuotePrecision: USD_PRECISION,
		})
		result = append(result, rt2)
	}

	for _, symbol := range symbolsByType["perps"] {
		rt1 := actors.NewRandomizedTaker(af.nextID(), gateway, actors.RandomizedTakerConfig{
			Symbol:         symbol,
			Interval:       1500 * time.Millisecond,
			MinQty:         ASSET_PRECISION / 100,
			MaxQty:         ASSET_PRECISION / 2,
			BasePrecision:  ASSET_PRECISION,
			QuotePrecision: USD_PRECISION,
		})
		result = append(result, rt1)

		rt2 := actors.NewRandomizedTaker(af.nextID(), gateway, actors.RandomizedTakerConfig{
			Symbol:         symbol,
			Interval:       2500 * time.Millisecond,
			MinQty:         ASSET_PRECISION / 100,
			MaxQty:         ASSET_PRECISION / 2,
			BasePrecision:  ASSET_PRECISION,
			QuotePrecision: USD_PRECISION,
		})
		result = append(result, rt2)
	}

	return result
}

func getAllSymbols(instruments map[string]exchange.Instrument) []string {
	symbols := make([]string, 0, len(instruments))
	for sym := range instruments {
		symbols = append(symbols, sym)
	}
	return symbols
}
