package randomwalk

import (
	"os"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

const btcPrecision = exchange.BTC_PRECISION

type assetSpec struct {
	name         string
	price        int64
	tickSize     int64
	levelSpacing int64
}

var assets = []assetSpec{
	{name: "ABC", price: 50_000 * exchange.DOLLAR_TICK, tickSize: exchange.DOLLAR_TICK, levelSpacing: 2},
	{name: "DEF", price: 3_000 * exchange.DOLLAR_TICK, tickSize: exchange.DOLLAR_TICK, levelSpacing: 1},
	{name: "GHI", price: 150 * exchange.DOLLAR_TICK, tickSize: exchange.DOLLAR_TICK, levelSpacing: 1},
}

type Sim struct {
	Runner      *simulation.Runner
	MMs         []*MarketMaker
	Taker       *RandomTaker
	Arbs        []*BasisArbActor
	FundingArbs []*FundingArbActor
	Loggers     []*JSONLinesLogger
	ex          *exchange.Exchange
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

func (s *Sim) Close() {
	for _, l := range s.Loggers {
		l.Close()
	}
}

func NewSim(simTime time.Duration) (*Sim, error) {
	simClock := simulation.NewSimulatedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients:        10,
		Clock:                   simClock,
		TickerFactory:           timerFact,
		SnapshotInterval:        time.Second,
		BalanceSnapshotInterval: 10 * time.Second,
	})

	if err := os.MkdirAll("logs/randomwalk/spot", 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll("logs/randomwalk/perp", 0755); err != nil {
		return nil, err
	}

	logGlobal, err := NewJSONLinesLogger("logs/randomwalk/general.jsonl")
	if err != nil {
		return nil, err
	}
	ex.SetLogger("_global", logGlobal)

	allLoggers := []*JSONLinesLogger{logGlobal}

	// Register all instruments and loggers; build a single oracle for all perps.
	indexOracle := exchange.NewMidPriceOracle(ex)
	var allSymbols []string
	for _, a := range assets {
		spotSym := a.name + "-USD"
		perpSym := a.name + "-PERP"

		spotInst := exchange.NewSpotInstrument(spotSym, a.name, "USD",
			btcPrecision, exchange.USD_PRECISION, a.tickSize, btcPrecision/100)
		ex.AddInstrument(spotInst)

		perp := exchange.NewPerpFutures(perpSym, a.name, "USD",
			btcPrecision, exchange.USD_PRECISION, a.tickSize, btcPrecision/100)
		perp.GetFundingRate().Interval = 120 // 2-min funding → ~750 settlements per 25h
		ex.AddInstrument(perp)

		indexOracle.MapSymbol(perpSym, spotSym)

		logSpot, err := NewJSONLinesLogger("logs/randomwalk/spot/" + spotSym + ".jsonl")
		if err != nil {
			return nil, err
		}
		logPerp, err := NewJSONLinesLogger("logs/randomwalk/perp/" + perpSym + ".jsonl")
		if err != nil {
			return nil, err
		}
		ex.SetLogger(spotSym, logSpot)
		ex.SetLogger(perpSym, logPerp)
		allLoggers = append(allLoggers, logSpot, logPerp)
		allSymbols = append(allSymbols, spotSym, perpSym)
	}

	ex.ConfigureAutomation(exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewWeightedMidPriceCalculator(),
		IndexProvider:       indexOracle,
		PriceUpdateInterval: 30 * time.Second,
	})

	initBalances := map[string]int64{
		"ABC": 1_000 * btcPrecision,
		"DEF": 1_000 * btcPrecision,
		"GHI": 1_000 * btcPrecision,
		"USD": 100_000_000 * exchange.USD_PRECISION,
	}
	zeroFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 0, InQuote: true}
	takerFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 10, InQuote: true}
	arbFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}

	mount := simulation.NewMount(ex, simulation.LatencyConfig{})

	// Clients 1-3: one MM per asset, quoting spot+perp.
	var mms []*MarketMaker
	for i, a := range assets {
		clientID := uint64(i + 1)
		mmGw := mount.ConnectNewClient(clientID, initBalances, zeroFee)
		ex.AddPerpBalance(clientID, "USD", 10_000_000*exchange.USD_PRECISION)
		mm := NewMarketMaker(clientID, mmGw, MMConfig{
			Symbols:         []string{a.name + "-USD", a.name + "-PERP"},
			BootstrapPrice:  a.price,
			Levels:          5,
			LevelSpacing:    a.levelSpacing,
			LevelSize:       btcPrecision,
			TickSize:        a.tickSize,
			RefreshInterval: 100 * time.Millisecond,
		})
		mm.SetTickerFactory(timerFact)
		mms = append(mms, mm)
	}

	// Client 4: one random taker across all 6 symbols.
	takerGw := mount.ConnectNewClient(4, initBalances, takerFee)
	ex.AddPerpBalance(4, "USD", 10_000_000*exchange.USD_PRECISION)
	taker := NewRandomTaker(4, takerGw, TakerConfig{
		Symbols:       allSymbols,
		QuoteNotional: 1_200 * exchange.USD_PRECISION, // ~$1,200 per order
		BasePrecision: btcPrecision,
		TakeInterval:  100 * time.Millisecond,
		Seed:          42,
	})
	taker.SetTickerFactory(timerFact)

	// Clients 5-7: one basis arb per asset pair.
	var arbs []*BasisArbActor
	for i, a := range assets {
		clientID := uint64(5 + i)
		arbGw := mount.ConnectNewClient(clientID, initBalances, arbFee)
		ex.AddPerpBalance(clientID, "USD", 10_000_000*exchange.USD_PRECISION)
		arb := NewBasisArbActor(clientID, arbGw, BasisArbConfig{
			SpotSymbol:   a.name + "-USD",
			PerpSymbol:   a.name + "-PERP",
			ThresholdBps: 1,
			LotSize:      btcPrecision,
			MaxPosition:  500,
		})
		arb.SetTickerFactory(timerFact)
		arbs = append(arbs, arb)
	}

	// Clients 8-10: one funding arb per asset pair.
	var fundingArbs []*FundingArbActor
	for i, a := range assets {
		clientID := uint64(8 + i)
		arbGw := mount.ConnectNewClient(clientID, initBalances, arbFee)
		ex.AddPerpBalance(clientID, "USD", 10_000_000*exchange.USD_PRECISION)
		arb := NewFundingArbActor(clientID, arbGw, FundingArbConfig{
			SpotSymbol:        a.name + "-USD",
			PerpSymbol:        a.name + "-PERP",
			OpenThresholdBps:  1,
			CloseThresholdBps: 0,
			LotSize:           btcPrecision,
			MaxPosition:       10,
			EntryWindow:       60 * time.Second,
		})
		arb.SetTickerFactory(timerFact)
		fundingArbs = append(fundingArbs, arb)
	}

	const step = time.Millisecond
	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations: int(simTime / step),
		Step:       step,
	})
	runner.AddMount(mount)
	for _, mm := range mms {
		runner.AddActor(mm)
	}
	runner.AddActor(taker)
	for _, arb := range arbs {
		runner.AddActor(arb)
	}
	for _, arb := range fundingArbs {
		runner.AddActor(arb)
	}

	return &Sim{
		Runner:      runner,
		MMs:         mms,
		Taker:       taker,
		Arbs:        arbs,
		FundingArbs: fundingArbs,
		Loggers:     allLoggers,
		ex:          ex,
	}, nil
}
