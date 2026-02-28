package abcusd

import (
	"os"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

const abcPrecision = exchange.BTC_PRECISION

// Sim holds all components wired by NewSim.
type Sim struct {
	Runner  *simulation.Runner
	MM      *PureMarketMaker
	Taker   *RandomTaker
	Arb     *FundingArbActor
	Loggers []*JSONLinesLogger
	ex      *exchange.Exchange
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

// Close flushes and closes all log files.
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
		EstimatedClients: 10,
		Clock:            simClock,
		TickerFactory:    timerFact,
		SnapshotInterval: 100 * time.Millisecond,
	})

	spotInst := exchange.NewSpotInstrument("ABC-USD", "ABC", "USD",
		abcPrecision, exchange.USD_PRECISION, exchange.DOLLAR_TICK, abcPrecision/100)
	perpInst := exchange.NewPerpFutures("ABC-PERP", "ABC", "USD",
		abcPrecision, exchange.USD_PRECISION, exchange.DOLLAR_TICK, abcPrecision/100)
	perpInst.GetFundingRate().Interval = 3600 // 1h funding interval in seconds

	ex.AddInstrument(spotInst)
	ex.AddInstrument(perpInst)

	indexOracle := exchange.NewMidPriceOracle(ex)
	indexOracle.MapSymbol("ABC-PERP", "ABC-USD")
	ex.ConfigureAutomation(exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewMidPriceCalculator(),
		IndexProvider:       indexOracle,
		PriceUpdateInterval: 60 * time.Second,
	})

	if err := os.MkdirAll("logs", 0755); err != nil {
		return nil, err
	}
	logGlobal, err := NewJSONLinesLogger("logs/global.jsonl")
	if err != nil {
		return nil, err
	}
	logSpot, err := NewJSONLinesLogger("logs/abc_usd.jsonl")
	if err != nil {
		return nil, err
	}
	logPerp, err := NewJSONLinesLogger("logs/abc_perp.jsonl")
	if err != nil {
		return nil, err
	}
	ex.SetLogger("_global", logGlobal)
	ex.SetLogger("ABC-USD", logSpot)
	ex.SetLogger("ABC-PERP", logPerp)

	initSpot := map[string]int64{
		"ABC": 100_000 * abcPrecision,
		"USD": 100_000_000 * exchange.USD_PRECISION,
	}
	zeroFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 0, InQuote: true}
	takerFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 10, InQuote: true}
	arbFee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}

	venue := simulation.NewExchangeVenue(ex, simulation.LatencyConfig{})
	mmGw := venue.ConnectClient(1, initSpot, zeroFee)
	takerGw := venue.ConnectClient(2, initSpot, takerFee)
	arbGw := venue.ConnectClient(3, initSpot, arbFee)
	for _, id := range []uint64{1, 2, 3} {
		ex.AddPerpBalance(id, "USD", 10_000_000*exchange.USD_PRECISION)
	}

	mm := NewPureMarketMaker(1, mmGw, MarketMakerConfig{
		Symbols:         []string{"ABC-USD", "ABC-PERP"},
		BootstrapPrice:  100 * exchange.DOLLAR_TICK,
		SpotSymbol:      "ABC-USD",
		PerpSymbol:      "ABC-PERP",
		Levels:          5,
		LevelSpacing:    2,
		LevelSize:       abcPrecision,
		TickSize:        exchange.DOLLAR_TICK,
		RefreshInterval: 50 * time.Millisecond,
		BasisAlpha:      0.3,
	})
	mm.SetTickerFactory(timerFact)

	taker := NewRandomTaker(2, takerGw, RandomTakerConfig{
		Symbols:      []string{"ABC-USD", "ABC-PERP"},
		LevelSize:    abcPrecision,
		MinQty:       abcPrecision / 100,
		SizeFraction: 0.4,
		TakeInterval: 200 * time.Millisecond,
		Seed:         42,
	})
	taker.SetTickerFactory(timerFact)

	arb := NewFundingArbActor(3, arbGw, FundingArbConfig{
		SpotSymbol:        "ABC-USD",
		PerpSymbol:        "ABC-PERP",
		OpenThresholdBps:  10,
		CloseThresholdBps: 2,
		PositionSize:      abcPrecision,
	})
	arb.SetTickerFactory(timerFact)

	// 25 hours @ 10ms step = 9,000,000 iterations
	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations: 9_000_000,
		Step:       10 * time.Millisecond,
	})
	runner.AddVenue(venue)
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
