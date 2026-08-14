package randomwalk

import (
	"os"
	"path/filepath"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

const btcPrecision = exchange.BTC_PRECISION

const (
	// All flows below are quote-notional based. Using one base unit per level
	// made the low-priced GHI book hundreds of times thinner than ABC.
	marketMakerLevelNotional = 5_000 * exchange.USD_PRECISION
	randomTakerNotional      = 1_200 * exchange.USD_PRECISION
)

type assetSpec struct {
	name         string
	price        int64
	tickSize     int64
	levelSpacing int64
}

var assets = []assetSpec{
	{name: "ABC", price: 50_000 * exchange.DOLLAR_TICK, tickSize: exchange.DOLLAR_TICK, levelSpacing: 2},
	{name: "DEF", price: 3_000 * exchange.DOLLAR_TICK, tickSize: 50 * exchange.CENT_TICK, levelSpacing: 2},
	{name: "GHI", price: 150 * exchange.DOLLAR_TICK, tickSize: 3 * exchange.CENT_TICK, levelSpacing: 2},
}

func quantityForUSDNotional(notional, price int64) int64 {
	qty, ok := etypes.TryMulDiv(notional, btcPrecision, price)
	if !ok || qty < btcPrecision/100 {
		panic("randomwalk: invalid notional/price quantity")
	}
	return qty
}

type Sim struct {
	Runner      *simulation.Runner
	MMs         []*MarketMaker
	Taker       *RandomTaker
	Arbs        []*BasisArbActor
	FundingArbs []*FundingArbActor
	CrossMM     *CrossPairMM
	TriArbs     []*TriArbActor
	Loggers     []*JSONLinesLogger
	ex          *exchange.Exchange
}

type SimConfig struct {
	LogDir       string
	SnapshotOnly bool
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

func (s *Sim) Close() {
	for _, l := range s.Loggers {
		l.Close()
	}
}

func NewSim(simTime time.Duration) (*Sim, error) {
	return NewSimWithConfig(simTime, SimConfig{LogDir: "logs/randomwalk"})
}

// NewSimWithLogDir constructs the standard random-walk ecology while keeping
// each experiment's artifacts in its own directory.
func NewSimWithLogDir(simTime time.Duration, logDir string) (*Sim, error) {
	return NewSimWithConfig(simTime, SimConfig{LogDir: logDir})
}

// NewSimWithConfig exposes logging controls for long visualization runs while
// preserving the model's fixed 1ms simulation-clock resolution.
func NewSimWithConfig(simTime time.Duration, cfg SimConfig) (*Sim, error) {
	if cfg.LogDir == "" {
		cfg.LogDir = "logs/randomwalk"
	}
	logDir := cfg.LogDir
	simClock := simulation.NewSimulatedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients:        15,
		Clock:                   simClock,
		TickerFactory:           timerFact,
		DeterministicIngress:    true,
		SnapshotInterval:        time.Second,
		BalanceSnapshotInterval: 10 * time.Second,
	})

	if err := os.MkdirAll(filepath.Join(logDir, "spot"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(logDir, "perp"), 0755); err != nil {
		return nil, err
	}

	newLogger := func(path string) (*JSONLinesLogger, error) {
		logger, err := NewJSONLinesLogger(path)
		if err != nil {
			return logger, err
		}
		logger.filterSnapshots = cfg.SnapshotOnly
		return logger, nil
	}
	logGlobal, err := newLogger(filepath.Join(logDir, "general.jsonl"))
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

		logSpot, err := newLogger(filepath.Join(logDir, "spot", spotSym+".jsonl"))
		if err != nil {
			return nil, err
		}
		logPerp, err := newLogger(filepath.Join(logDir, "perp", perpSym+".jsonl"))
		if err != nil {
			return nil, err
		}
		ex.SetLogger(spotSym, logSpot)
		ex.SetLogger(perpSym, logPerp)
		allLoggers = append(allLoggers, logSpot, logPerp)
		allSymbols = append(allSymbols, spotSym, perpSym)
	}

	// Register cross-asset spot instruments (DEF-ABC, GHI-ABC).
	// Price = basePrice * btcPrecision / abcPrice; tick sizes chosen for ~0.02% granularity.
	type crossSpec struct {
		symbol   string
		base     string
		tickSize int64
	}
	crossSpecs := []crossSpec{
		{"DEF-ABC", "DEF", 1_000},
		{"GHI-ABC", "GHI", 100},
	}
	for _, cs := range crossSpecs {
		inst := exchange.NewSpotInstrument(cs.symbol, cs.base, "ABC",
			btcPrecision, btcPrecision, cs.tickSize, btcPrecision/100)
		ex.AddInstrument(inst)
		logCross, err := newLogger(filepath.Join(logDir, "spot", cs.symbol+".jsonl"))
		if err != nil {
			return nil, err
		}
		ex.SetLogger(cs.symbol, logCross)
		allLoggers = append(allLoggers, logCross)
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
			LevelSize:       quantityForUSDNotional(marketMakerLevelNotional, a.price),
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
		QuoteNotional: randomTakerNotional,
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
			LotSize:      quantityForUSDNotional(randomTakerNotional, a.price),
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
			LotSize:           quantityForUSDNotional(randomTakerNotional, a.price),
			MaxPosition:       10,
			EntryWindow:       60 * time.Second,
		})
		arb.SetTickerFactory(timerFact)
		fundingArbs = append(fundingArbs, arb)
	}

	// Client 11: cross-pair market maker, quotes DEF-ABC and GHI-ABC derived from USD mids.
	crossMMGw := mount.ConnectNewClient(11, initBalances, zeroFee)
	crossMM := NewCrossPairMM(11, crossMMGw, CrossPairMMConfig{
		CrossSymbols:   []string{"DEF-ABC", "GHI-ABC"},
		BaseUSDSymbols: []string{"DEF-USD", "GHI-USD"},
		QuoteUSDSymbol: "ABC-USD",
		QuotePrecision: btcPrecision,
		TickSizes:      map[string]int64{"DEF-ABC": 1_000, "GHI-ABC": 100},
		LevelSizes: map[string]int64{
			"DEF-ABC": quantityForUSDNotional(marketMakerLevelNotional, assets[1].price),
			"GHI-ABC": quantityForUSDNotional(marketMakerLevelNotional, assets[2].price),
		},
		Levels:          5,
		LevelSpacing:    1,
		RefreshInterval: 100 * time.Millisecond,
	})
	crossMM.SetTickerFactory(timerFact)

	// Clients 12-13: triangular arb actors, one per cross pair.
	type triSpec struct {
		clientID   uint64
		crossSym   string
		baseUSDSym string
	}
	triArbSpecs := []triSpec{
		{12, "DEF-ABC", "DEF-USD"},
		{13, "GHI-ABC", "GHI-USD"},
	}
	var triArbs []*TriArbActor
	for _, spec := range triArbSpecs {
		triGw := mount.ConnectNewClient(spec.clientID, initBalances, arbFee)
		arb := NewTriArbActor(spec.clientID, triGw, TriArbConfig{
			CrossSymbol:    spec.crossSym,
			BaseUSDSymbol:  spec.baseUSDSym,
			QuoteUSDSymbol: "ABC-USD",
			TargetNotional: 1_000 * exchange.USD_PRECISION,
			MinProfitBps:   1,
			BasePrecision:  btcPrecision,
			CheckInterval:  100 * time.Millisecond,
		})
		arb.SetTickerFactory(timerFact)
		triArbs = append(triArbs, arb)
	}

	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations: int(simTime / time.Millisecond),
		Step:       time.Millisecond,
		Quiesce:    true,
	})
	runner.AddMount(mount)
	runner.AddIdler(timerFact)
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
	runner.AddActor(crossMM)
	for _, arb := range triArbs {
		runner.AddActor(arb)
	}

	return &Sim{
		Runner:      runner,
		MMs:         mms,
		Taker:       taker,
		Arbs:        arbs,
		FundingArbs: fundingArbs,
		CrossMM:     crossMM,
		TriArbs:     triArbs,
		Loggers:     allLoggers,
		ex:          ex,
	}, nil
}
