package feesim

import (
	"os"
	"strings"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/matching"
	"exchange_sim/simulation"
)

const (
	abcPrecision = exchange.BTC_PRECISION // 1e8
	qPrecision   = exchange.ETH_PRECISION // 1e6
	usdPrecision = exchange.USD_PRECISION // 1e5

	abcBootstrapUSD = 50_000 * usdPrecision // $50,000
	qBootstrapUSD   = 3_000 * usdPrecision  // $3,000
)

type SimConfig struct {
	SpotTakerBps int64
	PerpTakerBps int64
	// Maker fees for the taker/arb fee plan; negative = rebate.
	SpotMakerBps int64
	// Fees for the MM fee plan (default 0/0 = fee-exempt designated MM).
	MMMakerBps int64
	MMTakerBps int64
	LogDir     string

	// Tick sizes are multiplied by TickScaleNum/TickScaleDen (default 1/1).
	TickScaleNum int64
	TickScaleDen int64

	TakerIntervalMs int64 // default 100
	Seed            int64 // base RNG seed for taker and latency draws, default 42

	MMCount          int   // MMs per symbol, default 1
	MMLevels         int   // quote levels per MM, default 5
	MMBaseIntervalMs int64 // default 10
	MMMaxIntervalMs  int64 // default 100

	// Actor (taker/arb) network latency: log-normal with hard floor.
	LatencyMinUs    int64   // default 1000
	LatencyMedianUs int64   // default 3000 (median above min)
	LatencySigma    float64 // default 0.5

	FundingIntervalSec int64 // default 120

	// ProRata switches the exchange-wide matcher to pro-rata allocation.
	ProRata bool

	// MMSkewTicksPerLot enables Avellaneda-Stoikov style inventory skew in the
	// market makers (mid shifts against inventory). 0 = off.
	MMSkewTicksPerLot float64
	// MMSkewCapTicks bounds the skew shift in ticks (0 = unbounded).
	MMSkewCapTicks float64

	// TakerImbalanceCoupling tilts taker side toward book imbalance (herding).
	TakerImbalanceCoupling float64
	// TakerExciteAlpha / TakerExciteBetaPerSec: Hawkes-lite self-excited flow.
	TakerExciteAlpha      float64
	TakerExciteBetaPerSec float64

	// StepSleepUs is the runner's per-step goroutine drain pause in µs
	// (0 = runner default 1µs, negative = no sleep). Larger values trade
	// wall time for scheduling determinism.
	StepSleepUs int64

	// ValueTraderBandBps enables one value trader per USD spot market that
	// mean-reverts price to the bootstrap fundamental when the mid deviates
	// beyond this band. 0 = disabled.
	ValueTraderBandBps int64
	// ValueTraderMaxLots caps the value trader position in lots (default 20).
	ValueTraderMaxLots int64
	// ValueTraderIntervalMs is the decision cadence (default 200).
	ValueTraderIntervalMs int64

	// RaceArbTiers adds one extra ABC basis arb per entry, each with actor
	// latency scaled by the tier factor (1.0 = baseline latency, 0.1 = 10×
	// faster). All race entrants watch the same spot/perp pair, so their
	// relative fill shares measure the latency-race win concentration
	// (Aquilina-Budish-O'Neill style). Empty = no race.
	RaceArbTiers []float64

	// RaceArbReactive makes race entrants decide inside HandleEvent on every
	// trade print instead of on the polling ticker. Polling entrants tie
	// regardless of latency (a 100ms decision timer swamps any network
	// spread — the gen-6 negative result); reactive entrants race for real.
	RaceArbReactive bool

	// RaceArbMaxPosition caps each race entrant's inventory in lots (default
	// 5000). The accumulate threshold scales with |position|/MaxPosition, so
	// this is also the knob that decides how quickly a winning entrant
	// throttles itself — the suspected cause of speed advantage saturating.
	RaceArbMaxPosition int64

	// RaceArbLotSize overrides each race entrant's lot in base units
	// (default abcPrecision/10). Sizing to available depth is the discipline
	// that decides whether the second leg can actually keep up with the
	// first.
	RaceArbLotSize int64

	// RaceArbPostLeg rests the mirrored second leg at the near touch instead
	// of crossing for it, cancelling and unwinding the first leg if it has
	// not filled within RaceArbLegTimeoutMs (default: one decision interval).
	RaceArbPostLeg      bool
	RaceArbLegTimeoutMs int64

	// RaceArbSequential makes race entrants leg in sequentially: perp first,
	// spot mirrored to the actual fill. Removes the mismatch between two
	// independent market orders, which is the dominant source of naked delta.
	RaceArbSequential bool

	// RaceArbHedge makes race entrants flatten the residual delta between
	// their two legs. Unhedged, partial fills leave naked exposure an order
	// of magnitude larger than the basis edge, which buries any profit signal
	// under directional noise.
	RaceArbHedge bool
}

func DefaultSimConfig() SimConfig {
	return SimConfig{
		SpotTakerBps: 8,
		PerpTakerBps: 5,
		LogDir:       "logs/feesim",
	}
}

// normalize fills zero-valued fields with defaults so partial configs
// (e.g. unmarshalled from experiment JSON) behave like DefaultSimConfig.
func (c *SimConfig) normalize() {
	if c.TickScaleNum == 0 {
		c.TickScaleNum = 1
	}
	if c.TickScaleDen == 0 {
		c.TickScaleDen = 1
	}
	if c.TakerIntervalMs == 0 {
		c.TakerIntervalMs = 100
	}
	if c.Seed == 0 {
		c.Seed = 42
	}
	if c.MMCount == 0 {
		c.MMCount = 1
	}
	if c.MMLevels == 0 {
		c.MMLevels = 5
	}
	if c.MMBaseIntervalMs == 0 {
		c.MMBaseIntervalMs = 10
	}
	if c.MMMaxIntervalMs == 0 {
		c.MMMaxIntervalMs = 100
	}
	if c.LatencyMinUs == 0 {
		c.LatencyMinUs = 1000
	}
	if c.LatencyMedianUs == 0 {
		c.LatencyMedianUs = 3000
	}
	if c.LatencySigma == 0 {
		c.LatencySigma = 0.5
	}
	if c.FundingIntervalSec == 0 {
		c.FundingIntervalSec = 120
	}
	if c.ValueTraderMaxLots == 0 {
		c.ValueTraderMaxLots = 20
	}
	if c.ValueTraderIntervalMs == 0 {
		c.ValueTraderIntervalMs = 200
	}
}

// scaleDur scales a duration by a float factor, clamping at 1ns.
func scaleDur(d time.Duration, factor float64) time.Duration {
	scaled := time.Duration(float64(d) * factor)
	if scaled < 1 {
		return 1
	}
	return scaled
}

// scaleTick applies the configured tick scale, clamping at 1.
func (c *SimConfig) scaleTick(tick int64) int64 {
	scaled := tick * c.TickScaleNum / c.TickScaleDen
	if scaled < 1 {
		return 1
	}
	return scaled
}

type Sim struct {
	Runner       *simulation.Runner
	MMs          []*MarketMaker
	Taker        *RandomTaker
	BasisArbs    []*FeeAwareBasisArb
	FundingArbs  []*FeeAwareFundingArb
	TriArb       *FeeAwareTriArb
	RaceArbs     []*FeeAwareBasisArb
	ValueTraders []*ValueTrader
	Loggers      []*JSONLinesLogger
	ex           *exchange.Exchange
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

func (s *Sim) Close() {
	for _, l := range s.Loggers {
		l.Close()
	}
}

func NewSim(simTime time.Duration, cfg SimConfig) (*Sim, error) {
	cfg.normalize()
	spotTakerBps := cfg.SpotTakerBps
	perpTakerBps := cfg.PerpTakerBps
	logDir := cfg.LogDir
	simClock := simulation.NewSimulatedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients:        15,
		Clock:                   simClock,
		TickerFactory:           timerFact,
		SnapshotInterval:        time.Second,
		BalanceSnapshotInterval: 10 * time.Second,
	})
	if cfg.ProRata {
		ex.Matcher = matching.NewProRataMatcher(simClock)
	}

	if err := os.MkdirAll(logDir+"/spot", 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(logDir+"/perp", 0755); err != nil {
		return nil, err
	}

	logGlobal, err := NewJSONLinesLogger(logDir + "/general.jsonl")
	if err != nil {
		return nil, err
	}
	ex.SetLogger("_global", logGlobal)
	allLoggers := []*JSONLinesLogger{logGlobal}

	// ---------- Instruments ----------

	abcUSDTick := cfg.scaleTick(10 * usdPrecision) // $10 tick — 0.02% of price
	abcSpot := exchange.NewSpotInstrument("ABC/USD", "ABC", "USD",
		abcPrecision, usdPrecision, abcUSDTick, abcPrecision/100)
	ex.AddInstrument(abcSpot)

	qUSDTick := cfg.scaleTick(1 * usdPrecision) // $1 tick — 0.033% of price
	qSpot := exchange.NewSpotInstrument("Q/USD", "Q", "USD",
		qPrecision, usdPrecision, qUSDTick, qPrecision/100)
	ex.AddInstrument(qSpot)

	// ABC-PERP
	abcPerp := exchange.NewPerpFutures("ABC-PERP", "ABC", "USD",
		abcPrecision, usdPrecision, abcUSDTick, abcPrecision/100)
	abcPerp.GetFundingRate().Interval = cfg.FundingIntervalSec
	ex.AddInstrument(abcPerp)

	// Q-PERP
	qPerp := exchange.NewPerpFutures("Q-PERP", "Q", "USD",
		qPrecision, usdPrecision, qUSDTick, qPrecision/100)
	qPerp.GetFundingRate().Interval = cfg.FundingIntervalSec
	ex.AddInstrument(qPerp)

	// Q/ABC cross spot — price in ABC units
	// Bootstrap: Q/ABC = Q_USD / ABC_USD = 3000/50000 = 0.06 ABC
	// 0.06 ABC = 6_000_000 in BTC_PRECISION (1e8)
	// Tick: 0.0001 ABC = 10_000 in BTC_PRECISION
	qabcTick := cfg.scaleTick(10_000)
	qabcSpot := exchange.NewSpotInstrument("Q/ABC", "Q", "ABC",
		qPrecision, abcPrecision, qabcTick, qPrecision/100)
	ex.AddInstrument(qabcSpot)

	// Bootstrap cross price from arb pricing theory.
	qabcBootstrap := int64(qBootstrapUSD) * int64(abcPrecision) / int64(abcBootstrapUSD) // 6_000_000
	// Align to tick.
	qabcBootstrap = (qabcBootstrap / qabcTick) * qabcTick

	// ---------- Loggers per symbol ----------

	type logSpec struct {
		sym string
		dir string
	}
	logSpecs := []logSpec{
		{"ABC/USD", "spot"}, {"Q/USD", "spot"}, {"Q/ABC", "spot"},
		{"ABC-PERP", "perp"}, {"Q-PERP", "perp"},
	}
	for _, ls := range logSpecs {
		safeName := strings.ReplaceAll(ls.sym, "/", "-")
		lg, err := NewJSONLinesLogger(logDir + "/" + ls.dir + "/" + safeName + ".jsonl")
		if err != nil {
			return nil, err
		}
		ex.SetLogger(ls.sym, lg)
		allLoggers = append(allLoggers, lg)
	}

	// ---------- Automation (mark price, index, funding) ----------

	indexOracle := exchange.NewMidPriceOracle(ex)
	indexOracle.MapSymbol("ABC-PERP", "ABC/USD")
	indexOracle.MapSymbol("Q-PERP", "Q/USD")

	ex.ConfigureAutomation(exchange.AutomationConfig{
		MarkPriceCalc:       exchange.NewWeightedMidPriceCalculator(),
		IndexProvider:       indexOracle,
		PriceUpdateInterval: 30 * time.Second,
	})

	// ---------- Latency ----------

	// MMs get zero latency (co-located). Takers/arbs get realistic latency.
	mmMount := simulation.NewMount(ex, simulation.LatencyConfig{})

	latMin := time.Duration(cfg.LatencyMinUs) * time.Microsecond
	latMedian := time.Duration(cfg.LatencyMedianUs) * time.Microsecond
	actorLatency := simulation.LatencyConfig{
		Request:    simulation.NewLogNormalLatency(latMin, latMedian, cfg.LatencySigma, cfg.Seed),
		Response:   simulation.NewLogNormalLatency(latMin, latMedian, cfg.LatencySigma, cfg.Seed+1),
		MarketData: simulation.NewLogNormalLatency(latMin, latMedian, cfg.LatencySigma, cfg.Seed+2),
		// Deliver at exact sim timestamps; wall-clock sleeps are unrelated to
		// simulation time and would distort latency by orders of magnitude.
		Scheduler: scheduler,
		Clock:     simClock,
	}
	actorMount := simulation.NewMount(ex, actorLatency)

	// ---------- Fee plans ----------

	spotMakerFee := &exchange.PercentageFee{MakerBps: cfg.SpotMakerBps, TakerBps: spotTakerBps, InQuote: true}
	mmFee := &exchange.PercentageFee{MakerBps: cfg.MMMakerBps, TakerBps: cfg.MMTakerBps, InQuote: true}

	// ---------- Common balances ----------

	initBalances := map[string]int64{
		"ABC": 1_000 * abcPrecision,
		"Q":   100_000 * qPrecision,
		"USD": 100_000_000 * usdPrecision,
	}

	// ---------- Actors ----------

	var mms []*MarketMaker

	nextClient := uint64(0)
	connectMM := func(fee exchange.FeeModel, addPerp bool) actor.Gateway {
		nextClient++
		gw := mmMount.ConnectNewClient(nextClient, initBalances, fee)
		if addPerp {
			ex.AddPerpBalance(nextClient, "USD", 10_000_000*usdPrecision)
		}
		return gw
	}
	connectActor := func(fee exchange.FeeModel, addPerp bool) actor.Gateway {
		nextClient++
		gw := actorMount.ConnectNewClient(nextClient, initBalances, fee)
		if addPerp {
			ex.AddPerpBalance(nextClient, "USD", 10_000_000*usdPrecision)
		}
		return gw
	}

	// Clients 1-5: one pure MM per instrument.
	type mmSpec struct {
		symbol    string
		bootstrap int64
		tickSize  int64
		levelSize int64
		isPerp    bool
		spacing   int64
	}
	mmSpecs := []mmSpec{
		{"ABC/USD", abcBootstrapUSD, abcUSDTick, abcPrecision / 10, false, 2},
		{"Q/USD", qBootstrapUSD, qUSDTick, qPrecision, false, 2},
		{"ABC-PERP", abcBootstrapUSD, abcUSDTick, abcPrecision / 10, true, 2},
		{"Q-PERP", qBootstrapUSD, qUSDTick, qPrecision, true, 2},
		{"Q/ABC", qabcBootstrap, qabcTick, qPrecision, false, 2},
	}
	mmBase := time.Duration(cfg.MMBaseIntervalMs) * time.Millisecond
	mmMax := time.Duration(cfg.MMMaxIntervalMs) * time.Millisecond
	for _, spec := range mmSpecs {
		for i := 0; i < cfg.MMCount; i++ {
			gw := connectMM(mmFee, spec.isPerp)
			mm := NewMarketMaker(nextClient, gw, MMConfig{
				Symbol:         spec.symbol,
				BootstrapPrice: spec.bootstrap,
				Levels:         cfg.MMLevels,
				LevelSpacing:   spec.spacing,
				LevelSize:      spec.levelSize,
				TickSize:       spec.tickSize,
				MidPriceMode:   MidFromWeightedMid,
				// Stagger refresh intervals so competing MMs do not move in lockstep.
				BaseInterval:    mmBase + time.Duration(i)*time.Millisecond,
				MaxInterval:     mmMax + time.Duration(i)*10*time.Millisecond,
				SkewTicksPerLot: cfg.MMSkewTicksPerLot,
				SkewCapTicks:    cfg.MMSkewCapTicks,
			})
			mm.SetTickerFactory(timerFact)
			mms = append(mms, mm)
		}
	}

	// Client 6: random taker across all 5 symbols.
	// Pre-computed target qtys: ~$2000 notional for USD pairs, smaller for cross.
	takerGw := connectActor(spotMakerFee, true)
	allSymbols := []string{"ABC/USD", "Q/USD", "ABC-PERP", "Q-PERP", "Q/ABC"}
	targetQtys := map[string]int64{
		"ABC/USD":  4_000_000, // 0.04 BTC ≈ $2000
		"Q/USD":    700_000,   // 0.7 Q ≈ $2100
		"ABC-PERP": 4_000_000, // 0.04 BTC ≈ $2000
		"Q-PERP":   700_000,   // 0.7 Q ≈ $2100
		"Q/ABC":    200_000,   // 0.2 Q (thinner cross market)
	}
	taker := NewRandomTaker(nextClient, takerGw, TakerConfig{
		Symbols:           allSymbols,
		TargetQtys:        targetQtys,
		TakeInterval:      time.Duration(cfg.TakerIntervalMs) * time.Millisecond,
		Seed:              cfg.Seed,
		ImbalanceCoupling: cfg.TakerImbalanceCoupling,
		ExciteAlpha:       cfg.TakerExciteAlpha,
		ExciteBetaPerSec:  cfg.TakerExciteBetaPerSec,
	})
	taker.SetTickerFactory(timerFact)

	// Clients 7-8: fee-aware basis arbs.
	type basisSpec struct {
		spotSym     string
		perpSym     string
		lotSize     int64
		maxPosition int64
	}
	basisSpecs := []basisSpec{
		{"ABC/USD", "ABC-PERP", abcPrecision / 10, 5000},
		{"Q/USD", "Q-PERP", qPrecision, 5000},
	}
	var basisArbs []*FeeAwareBasisArb
	for _, spec := range basisSpecs {
		gw := connectActor(spotMakerFee, true)
		arb := NewFeeAwareBasisArb(nextClient, gw, BasisArbConfig{
			SpotSymbol:    spec.spotSym,
			PerpSymbol:    spec.perpSym,
			SpotFeeBps:    spotTakerBps,
			PerpFeeBps:    perpTakerBps,
			LotSize:       spec.lotSize,
			MaxPosition:   spec.maxPosition,
			CheckInterval: 100 * time.Millisecond,
		})
		arb.SetTickerFactory(timerFact)
		basisArbs = append(basisArbs, arb)
	}

	// Clients 9-10: fee-aware funding arbs.
	var fundingArbs []*FeeAwareFundingArb
	for _, spec := range basisSpecs {
		gw := connectActor(spotMakerFee, true)
		arb := NewFeeAwareFundingArb(nextClient, gw, FundingArbConfig{
			SpotSymbol:  spec.spotSym,
			PerpSymbol:  spec.perpSym,
			SpotFeeBps:  spotTakerBps,
			PerpFeeBps:  perpTakerBps,
			LotSize:     spec.lotSize,
			MaxPosition: 100,
			EntryWindow: 60 * time.Second,
		})
		arb.SetTickerFactory(timerFact)
		fundingArbs = append(fundingArbs, arb)
	}

	// Client 11: fee-aware triangle arb (Q/USD ↔ Q/ABC ↔ ABC/USD).
	triGw := connectActor(spotMakerFee, false)
	triArb := NewFeeAwareTriArb(nextClient, triGw, TriArbConfig{
		QUSDSymbol:     "Q/USD",
		ABCUSDSymbol:   "ABC/USD",
		QABCSymbol:     "Q/ABC",
		TakerFeeBps:    spotTakerBps,
		TargetNotional: 500 * usdPrecision,
		QPrecision:     qPrecision,
		ABCPrecision:   abcPrecision,
		CheckInterval:  100 * time.Millisecond,
	})
	triArb.SetTickerFactory(timerFact)

	// Optional latency-race arbs: same signal, tiered speed.
	raceMaxPosition := cfg.RaceArbMaxPosition
	if raceMaxPosition == 0 {
		raceMaxPosition = 5000
	}
	raceLotSize := cfg.RaceArbLotSize
	if raceLotSize == 0 {
		raceLotSize = abcPrecision / 10
	}
	var raceArbs []*FeeAwareBasisArb
	var raceMounts []*simulation.Mount
	for _, tier := range cfg.RaceArbTiers {
		tierLatency := simulation.LatencyConfig{
			Request:    simulation.NewLogNormalLatency(scaleDur(latMin, tier), scaleDur(latMedian, tier), cfg.LatencySigma, cfg.Seed+10),
			Response:   simulation.NewLogNormalLatency(scaleDur(latMin, tier), scaleDur(latMedian, tier), cfg.LatencySigma, cfg.Seed+11),
			MarketData: simulation.NewLogNormalLatency(scaleDur(latMin, tier), scaleDur(latMedian, tier), cfg.LatencySigma, cfg.Seed+12),
			Scheduler:  scheduler,
			Clock:      simClock,
		}
		tierMount := simulation.NewMount(ex, tierLatency)
		raceMounts = append(raceMounts, tierMount)
		nextClient++
		gw := tierMount.ConnectNewClient(nextClient, initBalances, spotMakerFee)
		ex.AddPerpBalance(nextClient, "USD", 10_000_000*usdPrecision)
		arb := NewFeeAwareBasisArb(nextClient, gw, BasisArbConfig{
			SpotSymbol:       "ABC/USD",
			PerpSymbol:       "ABC-PERP",
			SpotFeeBps:       spotTakerBps,
			PerpFeeBps:       perpTakerBps,
			LotSize:          abcPrecision / 10,
			MaxPosition:      raceMaxPosition,
			CheckInterval:    100 * time.Millisecond,
			Reactive:         cfg.RaceArbReactive,
			HedgeResidual:    cfg.RaceArbHedge,
			SequentialLegs:   cfg.RaceArbSequential,
			PostSecondLeg:    cfg.RaceArbPostLeg,
			SecondLegTimeout: time.Duration(cfg.RaceArbLegTimeoutMs) * time.Millisecond,
		})
		arb.SetTickerFactory(timerFact)
		raceArbs = append(raceArbs, arb)
	}

	// Optional value traders: fundamental anchor for the price level.
	var valueTraders []*ValueTrader
	if cfg.ValueTraderBandBps > 0 {
		type valueSpec struct {
			symbol      string
			fundamental int64
			lotQty      int64
		}
		valueSpecs := []valueSpec{
			{"ABC/USD", abcBootstrapUSD, targetQtys["ABC/USD"]},
			{"Q/USD", qBootstrapUSD, targetQtys["Q/USD"]},
		}
		for _, spec := range valueSpecs {
			gw := connectActor(spotMakerFee, false)
			vt := NewValueTrader(nextClient, gw, ValueTraderConfig{
				Symbol:      spec.symbol,
				Fundamental: spec.fundamental,
				BandBps:     cfg.ValueTraderBandBps,
				LotQty:      spec.lotQty,
				MaxPosition: cfg.ValueTraderMaxLots * spec.lotQty,
				Interval:    time.Duration(cfg.ValueTraderIntervalMs) * time.Millisecond,
			})
			vt.SetTickerFactory(timerFact)
			valueTraders = append(valueTraders, vt)
		}
	}

	// ---------- Runner ----------

	const step = time.Millisecond
	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations: int(simTime / step),
		Step:       step,
		StepSleep:  time.Duration(cfg.StepSleepUs) * time.Microsecond,
	})
	runner.AddMount(mmMount)
	runner.AddMount(actorMount)
	for _, m := range raceMounts {
		runner.AddMount(m)
	}
	for _, mm := range mms {
		runner.AddActor(mm)
	}
	runner.AddActor(taker)
	for _, arb := range basisArbs {
		runner.AddActor(arb)
	}
	for _, arb := range fundingArbs {
		runner.AddActor(arb)
	}
	runner.AddActor(triArb)
	for _, arb := range raceArbs {
		runner.AddActor(arb)
	}
	for _, vt := range valueTraders {
		runner.AddActor(vt)
	}

	return &Sim{
		Runner:       runner,
		MMs:          mms,
		Taker:        taker,
		BasisArbs:    basisArbs,
		FundingArbs:  fundingArbs,
		TriArb:       triArb,
		RaceArbs:     raceArbs,
		ValueTraders: valueTraders,
		Loggers:      allLoggers,
		ex:           ex,
	}, nil
}
