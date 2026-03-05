package randomwalk

import (
	"os"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

const (
	btcPrecision   = exchange.BTC_PRECISION
	bootstrapPrice = 50_000 * exchange.DOLLAR_TICK
)

type Sim struct {
	Runner  *simulation.Runner
	MM      *MarketMaker
	Taker   *RandomTaker
	Arb     *BasisArbActor
	Loggers []*JSONLinesLogger
	ex      *exchange.Exchange
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

func (s *Sim) Close() {
	for _, l := range s.Loggers {
		l.Close()
	}
}

func NewSim() (*Sim, error) {
	simClock := simulation.NewSimulatedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients:        6,
		Clock:                   simClock,
		TickerFactory:           timerFact,
		SnapshotInterval:        time.Second,
		BalanceSnapshotInterval: 10 * time.Second,
	})

	spotInst := exchange.NewSpotInstrument("ABC-USD", "ABC", "USD",
		btcPrecision, exchange.USD_PRECISION, exchange.DOLLAR_TICK, btcPrecision/100)
	ex.AddInstrument(spotInst)

	perp := exchange.NewPerpFutures("ABC-PERP", "ABC", "USD",
		btcPrecision, exchange.USD_PRECISION, exchange.DOLLAR_TICK, btcPrecision/100)
	perp.GetFundingRate().Interval = 120 // 2-minute funding in 900s sim → ~7 settlements
	ex.AddInstrument(perp)

	indexOracle := exchange.NewMidPriceOracle(ex)
	indexOracle.MapSymbol("ABC-PERP", "ABC-USD")
	ex.ConfigureAutomation(exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewWeightedMidPriceCalculator(),
		IndexProvider:       indexOracle,
		PriceUpdateInterval: 30 * time.Second,
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
	logSpot, err := NewJSONLinesLogger("logs/randomwalk/spot/ABC-USD.jsonl")
	if err != nil {
		return nil, err
	}
	logPerp, err := NewJSONLinesLogger("logs/randomwalk/perp/ABC-PERP.jsonl")
	if err != nil {
		return nil, err
	}
	ex.SetLogger("_global", logGlobal)
	ex.SetLogger("ABC-USD", logSpot)
	ex.SetLogger("ABC-PERP", logPerp)

	initBalances := map[string]int64{
		"ABC": 1_000 * btcPrecision,
		"USD": 100_000_000 * exchange.USD_PRECISION,
	}
	zeroFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 0, InQuote: true}
	takerFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 10, InQuote: true}
	arbFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}

	mount := simulation.NewMount(ex, simulation.LatencyConfig{})
	mmGw := mount.ConnectNewClient(1, initBalances, zeroFee)
	takerGw := mount.ConnectNewClient(2, initBalances, takerFee)
	arbGw := mount.ConnectNewClient(3, initBalances, arbFee)
	for _, id := range []uint64{1, 2, 3} {
		ex.AddPerpBalance(id, "USD", 10_000_000*exchange.USD_PRECISION)
	}

	mm := NewMarketMaker(1, mmGw, MMConfig{
		Symbols:         []string{"ABC-USD", "ABC-PERP"},
		BootstrapPrice:  bootstrapPrice,
		Levels:          5,
		LevelSpacing:    2,
		LevelSize:       btcPrecision,
		TickSize:        exchange.DOLLAR_TICK,
		RefreshInterval: 100 * time.Millisecond,
	})
	mm.SetTickerFactory(timerFact)

	taker := NewRandomTaker(2, takerGw, TakerConfig{
		Symbols:      []string{"ABC-USD", "ABC-PERP"},
		OrderQty:     btcPrecision * 2 / 5, // 0.4 ABC
		TakeInterval: 100 * time.Millisecond,
		Seed:         42,
	})
	taker.SetTickerFactory(timerFact)

	arb := NewBasisArbActor(3, arbGw, BasisArbConfig{
		SpotSymbol:  "ABC-USD",
		PerpSymbol:  "ABC-PERP",
		ThresholdBps: 1,            // 1 bps ≈ $5 at $50k
		LotSize:     btcPrecision,  // 1 BTC per lot
		MaxPosition: 30,            // 30 BTC max → up to $30 per-leg price impact
	})
	arb.SetTickerFactory(timerFact)

	// 900 seconds sim-time @ 1ms/step = 900k iterations
	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations: 900_000,
		Step:       time.Millisecond,
	})
	runner.AddMount(mount)
	runner.AddActor(mm)
	runner.AddActor(taker)
	runner.AddActor(arb)

	return &Sim{
		Runner:  runner,
		MM:      mm,
		Taker:   taker,
		Arb:     arb,
		Loggers: []*JSONLinesLogger{logGlobal, logSpot, logPerp},
		ex:      ex,
	}, nil
}
