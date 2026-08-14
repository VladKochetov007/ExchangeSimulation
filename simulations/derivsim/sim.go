package derivsim

import (
	"os"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
	"exchange_sim/simulation"
	"exchange_sim/simulations/feesim"
)

const (
	basePrecision = exchange.BTC_PRECISION
	usdPrecision  = exchange.USD_PRECISION
	bootstrapUSD  = 50_000 * usdPrecision
)

// SimConfig wires an underlying spot ecology (reused from feesim actors) with
// a derivatives board: scheduled futures and option chains, market openers,
// arbitrage bots, and option flow whose direction controls the dealer's gamma
// sign.
type SimConfig struct {
	LogDir string
	Seed   int64

	// Underlying ecology.
	MMCount           int   // spot MMs (default 3)
	TakerIntervalMs   int64 // hawkes-lite taker cadence (default 100)
	ValueTraderBand   int64 // bps (default 10, deep anchor)
	ValueTraderLots   int64 // default 500
	TakerExciteAlpha  float64
	TakerExciteBeta   float64
	StepSleepUs       int64
	FundingArbsAbsent bool // reserved

	// Listing board.
	TenorSec       int64 // option tenor (default 300 = 5 sim-min)
	FutTenorSec    int64 // futures tenor (default = TenorSec; desync to isolate expiries)
	StrikesPerSide int   // option chain half-width (default 2)
	StrikeStepUSD  int64 // default 1000
	IV             float64

	// Option dealer (market opener + hedger).
	DealerSpreadBps  int64 // default 30
	DealerSkewBps    int64 // default 5
	DealerHedge      bool
	DealerHedgeMs    int64 // default 500
	DealerHedgeBand  int64 // base units (default lot/2)
	OptionQuoteQty   int64 // default 0.5 BTC
	FuturesSpreadBps int64 // default 4

	// Option flow (gamma-sign control).
	OptionFlow       bool
	OptionPBuy       float64 // 0.8 = dealer short gamma; 0.2 = long gamma
	OptionLotQty     int64   // default 0.1 BTC
	OptionIntervalMs int64   // default 400

	// Arbitrage bots.
	CarryArb       bool
	CarryEdgeBps   int64 // default 10
	CarryScaleEdge bool  // sqrt(TTE/tenor) edge scaling
	ParityArb      bool
	ParityEdgeBps  int64 // default 15

	// Untethered futures regime: futures MM quotes around its own last trade
	// and the taker also hits futures books.
	FuturesSelfAnchored bool
	TakeFutures         bool
}

func (c *SimConfig) normalize() {
	if c.Seed == 0 {
		c.Seed = 42
	}
	if c.MMCount == 0 {
		c.MMCount = 3
	}
	if c.TakerIntervalMs == 0 {
		c.TakerIntervalMs = 100
	}
	if c.ValueTraderBand == 0 {
		c.ValueTraderBand = 10
	}
	if c.ValueTraderLots == 0 {
		c.ValueTraderLots = 500
	}
	if c.TenorSec == 0 {
		c.TenorSec = 300
	}
	if c.FutTenorSec == 0 {
		c.FutTenorSec = c.TenorSec
	}
	if c.StrikesPerSide == 0 {
		c.StrikesPerSide = 2
	}
	if c.StrikeStepUSD == 0 {
		c.StrikeStepUSD = 1000
	}
	if c.IV == 0 {
		c.IV = 0.8
	}
	if c.DealerSpreadBps == 0 {
		c.DealerSpreadBps = 30
	}
	if c.DealerSkewBps == 0 {
		c.DealerSkewBps = 5
	}
	if c.DealerHedgeMs == 0 {
		c.DealerHedgeMs = 500
	}
	if c.OptionQuoteQty == 0 {
		c.OptionQuoteQty = basePrecision / 2
	}
	if c.FuturesSpreadBps == 0 {
		c.FuturesSpreadBps = 4
	}
	if c.OptionPBuy == 0 {
		c.OptionPBuy = 0.5
	}
	if c.OptionLotQty == 0 {
		c.OptionLotQty = basePrecision / 10
	}
	if c.OptionIntervalMs == 0 {
		c.OptionIntervalMs = 400
	}
	if c.DealerHedgeBand == 0 {
		c.DealerHedgeBand = c.OptionLotQty / 2
	}
	if c.CarryEdgeBps == 0 {
		c.CarryEdgeBps = 10
	}
	if c.ParityEdgeBps == 0 {
		c.ParityEdgeBps = 15
	}
}

type Sim struct {
	Runner    *simulation.Runner
	Dealer    *OptionMarketMaker
	FutMM     *FuturesMarketMaker
	CarryBot  *CashCarryArb
	ParityBot *ParityArb
	Loggers   []*feesim.JSONLinesLogger
	ex        *exchange.Exchange
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

func (s *Sim) Close() {
	for _, l := range s.Loggers {
		l.Close()
	}
}

func NewSim(simTime time.Duration, cfg SimConfig) (*Sim, error) {
	cfg.normalize()

	simClock := simulation.NewSimulatedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients:        16,
		Clock:                   simClock,
		TickerFactory:           timerFact,
		DeterministicIngress:    true,
		SnapshotInterval:        200 * time.Millisecond,
		BalanceSnapshotInterval: 10 * time.Second,
	})

	if err := os.MkdirAll(cfg.LogDir+"/spot", 0755); err != nil {
		return nil, err
	}
	logGlobal, err := feesim.NewJSONLinesLogger(cfg.LogDir + "/general.jsonl")
	if err != nil {
		return nil, err
	}
	ex.SetLogger("_global", logGlobal)
	logSpot, err := feesim.NewJSONLinesLogger(cfg.LogDir + "/spot/ABC-USD.jsonl")
	if err != nil {
		return nil, err
	}
	ex.SetLogger("ABC/USD", logSpot)
	loggers := []*feesim.JSONLinesLogger{logGlobal, logSpot}

	tick := int64(10 * usdPrecision)
	spot := exchange.NewSpotInstrument("ABC/USD", "ABC", "USD",
		basePrecision, usdPrecision, tick, basePrecision/100)
	ex.AddInstrument(spot)

	// Listing board: one futures ladder + one option chain, same tenor.
	tenor := cfg.TenorSec * int64(time.Second)
	futTenor := cfg.FutTenorSec * int64(time.Second)
	spec := einstrument.ContractSpec{
		Base: "ABC", Quote: "USD",
		BasePrecision: basePrecision, QuotePrecision: usdPrecision,
		TickSize: tick, MinOrderSize: 1,
	}
	optSpec := spec
	optSpec.TickSize = usdPrecision // $1 premium tick
	indexOracle := exchange.NewMidPriceOracle(ex)
	ex.ConfigureAutomation(exchange.AutomationConfig{
		IndexProvider:       indexOracle,
		PriceUpdateInterval: 5 * time.Second,
		ListingPolicies: []exchange.ListingPolicy{
			&einstrument.DatedFuturesLister{
				Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{futTenor},
			},
			&einstrument.OptionChainLister{
				Underlying: "ABC/USD", Spec: optSpec, TenorsNano: []int64{tenor},
				StrikeStep:     cfg.StrikeStepUSD * usdPrecision,
				StrikesPerSide: cfg.StrikesPerSide,
				IV:             cfg.IV,
			},
		},
	})

	mmMount := simulation.NewMount(ex, simulation.LatencyConfig{})

	zeroFee := &exchange.FixedFee{}
	spotBalances := map[string]int64{
		"ABC": 5_000 * basePrecision,
		"USD": 500_000_000 * usdPrecision,
	}
	nextClient := uint64(0)
	connect := func(perpUSD int64) actor.Gateway {
		nextClient++
		gw := mmMount.ConnectNewClient(nextClient, spotBalances, zeroFee)
		if perpUSD > 0 {
			ex.AddPerpBalance(nextClient, "USD", perpUSD)
		}
		return gw
	}

	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations: int(simTime / time.Millisecond),
		Step:       time.Millisecond,
		StepSleep:  time.Duration(cfg.StepSleepUs) * time.Microsecond,
		Quiesce:    true,
	})
	runner.AddMount(mmMount)
	runner.AddIdler(timerFact)

	// --- Underlying ecology (reused feesim actors) ---
	for i := 0; i < cfg.MMCount; i++ {
		gw := connect(0)
		mm := feesim.NewMarketMaker(nextClient, gw, feesim.MMConfig{
			Symbol:         "ABC/USD",
			BootstrapPrice: bootstrapUSD,
			Levels:         5,
			LevelSpacing:   2,
			LevelSize:      basePrecision / 10,
			TickSize:       tick,
			MidPriceMode:   feesim.MidFromWeightedMid,
			BaseInterval:   10*time.Millisecond + time.Duration(i)*time.Millisecond,
			MaxInterval:    100*time.Millisecond + time.Duration(i)*10*time.Millisecond,
		})
		mm.SetTickerFactory(timerFact)
		runner.AddActor(mm)
	}

	takerGw := connect(10_000_000 * usdPrecision)
	taker := feesim.NewRandomTaker(nextClient, takerGw, feesim.TakerConfig{
		Symbols:          []string{"ABC/USD"},
		TargetQtys:       map[string]int64{"ABC/USD": 4_000_000},
		TakeInterval:     time.Duration(cfg.TakerIntervalMs) * time.Millisecond,
		Seed:             cfg.Seed,
		ExciteAlpha:      cfg.TakerExciteAlpha,
		ExciteBetaPerSec: cfg.TakerExciteBeta,
	})
	taker.SetTickerFactory(timerFact)
	runner.AddActor(taker)

	vtGw := connect(0)
	vt := feesim.NewValueTrader(nextClient, vtGw, feesim.ValueTraderConfig{
		Symbol:      "ABC/USD",
		Fundamental: bootstrapUSD,
		BandBps:     cfg.ValueTraderBand,
		LotQty:      4_000_000,
		MaxPosition: cfg.ValueTraderLots * 4_000_000,
		Interval:    100 * time.Millisecond,
	})
	vt.SetTickerFactory(timerFact)
	runner.AddActor(vt)

	// --- Derivatives board actors ---
	sim := &Sim{Runner: runner, Loggers: loggers, ex: ex}

	dealerGw := connect(100_000_000 * usdPrecision)
	sim.Dealer = NewOptionMarketMaker(nextClient, dealerGw, OptionMMConfig{
		Underlying:    "ABC/USD",
		IV:            cfg.IV,
		SpreadBps:     cfg.DealerSpreadBps,
		SkewPerLotBps: cfg.DealerSkewBps,
		QuoteQty:      cfg.OptionQuoteQty,
		LotQty:        cfg.OptionLotQty,
		PremiumTick:   usdPrecision,
		QuoteInterval: 200 * time.Millisecond,
		HedgeEnabled:  cfg.DealerHedge,
		HedgeInterval: time.Duration(cfg.DealerHedgeMs) * time.Millisecond,
		HedgeBandQty:  cfg.DealerHedgeBand,
		BasePrecision: basePrecision,
	})
	sim.Dealer.SetTickerFactory(timerFact)
	runner.AddActor(sim.Dealer)

	futGw := connect(50_000_000 * usdPrecision)
	sim.FutMM = NewFuturesMarketMaker(nextClient, futGw, FuturesMMConfig{
		Underlying:    "ABC/USD",
		SpreadBps:     cfg.FuturesSpreadBps,
		QuoteQty:      basePrecision / 2,
		Tick:          tick,
		QuoteInterval: 100 * time.Millisecond,
		SelfAnchored:  cfg.FuturesSelfAnchored,
	})
	sim.FutMM.SetTickerFactory(timerFact)
	runner.AddActor(sim.FutMM)

	if cfg.OptionFlow {
		gw := connect(50_000_000 * usdPrecision)
		ot := NewOptionTaker(nextClient, gw, OptionTakerConfig{
			Underlying:     "ABC/USD",
			PBuy:           cfg.OptionPBuy,
			LotQty:         cfg.OptionLotQty,
			Interval:       time.Duration(cfg.OptionIntervalMs) * time.Millisecond,
			Seed:           cfg.Seed + 100,
			IncludeFutures: cfg.TakeFutures,
		})
		ot.SetTickerFactory(timerFact)
		runner.AddActor(ot)
	}

	if cfg.CarryArb {
		gw := connect(50_000_000 * usdPrecision)
		carryTenor := int64(0)
		if cfg.CarryScaleEdge {
			carryTenor = futTenor
		}
		sim.CarryBot = NewCashCarryArb(nextClient, gw, CarryArbConfig{
			Underlying:    "ABC/USD",
			EdgeBps:       cfg.CarryEdgeBps,
			LotQty:        basePrecision / 10,
			MaxPosPerSym:  5 * basePrecision,
			CheckInterval: 200 * time.Millisecond,
			TenorNano:     carryTenor,
		})
		sim.CarryBot.SetTickerFactory(timerFact)
		runner.AddActor(sim.CarryBot)
	}

	if cfg.ParityArb {
		gw := connect(50_000_000 * usdPrecision)
		sim.ParityBot = NewParityArb(nextClient, gw, ParityArbConfig{
			Underlying:    "ABC/USD",
			EdgeBps:       cfg.ParityEdgeBps,
			LotQty:        basePrecision / 20,
			MaxTrades:     500,
			CheckInterval: 500 * time.Millisecond,
		})
		sim.ParityBot.SetTickerFactory(timerFact)
		runner.AddActor(sim.ParityBot)
	}

	return sim, nil
}
