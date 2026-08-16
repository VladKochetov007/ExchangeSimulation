package feesim

import (
	"fmt"
	"os"
	"strings"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/matching"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
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
	// NoiseTraderCount creates independently seeded random-flow actors across
	// all books. Zero keeps the historical one-taker baseline.
	NoiseTraderCount int
	Seed             int64 // base RNG seed for taker and latency draws, default 42

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

	// Deterministic enables the runner-owned phase runtime. It defines request,
	// response, market-data, timer, and configured-latency delivery order in
	// simulation time; it is required for reproducible research runs.
	Deterministic bool

	// StepSleepUs is the runner's per-step goroutine drain pause in µs
	// (0 = runner default 1µs, negative = no sleep). Larger values trade
	// wall time for scheduling determinism.
	StepSleepUs int64

	// ValueTraderBandBps enables one value trader per USD spot market that
	// mean-reverts price toward its own opinion of value when the mid deviates
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
	// public book update instead of on the polling ticker. Polling entrants
	// tie regardless of latency (a 100ms decision timer swamps any network
	// spread — the gen-6 negative result); reactive entrants race on actual
	// executable displayed quotes.
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
	// RaceArbImproveTicks posts the second leg this many ticks inside the
	// touch, buying queue priority ahead of the resident market maker.
	RaceArbImproveTicks int64

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
func (c *SimConfig) normalize() error {
	if c.TickScaleNum == 0 {
		c.TickScaleNum = 1
	}
	if c.TickScaleDen == 0 {
		c.TickScaleDen = 1
	}
	if c.TakerIntervalMs == 0 {
		c.TakerIntervalMs = 100
	}
	if c.NoiseTraderCount == 0 {
		c.NoiseTraderCount = 1
	}
	if c.NoiseTraderCount < 1 {
		return fmt.Errorf("feesim: noise trader count must be positive")
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
	return nil
}

// scaleDur scales a duration by a float factor, clamping at 1ns.
func scaleDur(d time.Duration, factor float64) time.Duration {
	scaled := time.Duration(float64(d) * factor)
	if scaled < 1 {
		return 1
	}
	return scaled
}

// latencySeed derives an independent, reproducible stream for one client and
// channel. A shared mutable latency provider makes the draw sequence depend on
// which actor the host happens to schedule first, defeating common-random-
// number experiments.
func latencySeed(master int64, clientID, channel uint64) int64 {
	x := uint64(master) + 0x9e3779b97f4a7c15
	x ^= clientID * 0xbf58476d1ce4e5b9
	x ^= channel * 0x94d049bb133111eb
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return int64(x & ((uint64(1) << 63) - 1))
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
	Takers       []*RandomTaker
	BasisArbs    []*FeeAwareBasisArb
	FundingArbs  []*FeeAwareFundingArb
	TriArb       *FeeAwareTriArb
	RaceArbs     []*FeeAwareBasisArb
	RaceArbTiers []float64
	RaceArbIDs   []uint64
	ValueTraders []*ValueTrader
	Loggers      []*JSONLinesLogger
	ex           *exchange.Exchange
}

func (s *Sim) Exchange() *exchange.Exchange { return s.ex }

// RaceArbTerminalReport joins an observed fill ledger to a strict,
// exchange-owned terminal account valuation. A missing two-sided asset mark
// returns an error rather than turning an unresolved position into zero PnL.
type RaceArbTerminalReport struct {
	Tier                     float64                      `json:"tier"`
	ClientID                 uint64                       `json:"client_id"`
	Fills                    BasisArbReport               `json:"fills"`
	InitialEquityUSD         int64                        `json:"initial_equity_usd"`
	PassiveTerminalEquityUSD int64                        `json:"passive_terminal_equity_usd"`
	TerminalAccount          etypes.MarkedAccountSnapshot `json:"terminal_account"`
	// StrategyEquityChangeUSD subtracts the value the actor's initial
	// ABC/Q/USD inventory would have at the same terminal marks. It therefore
	// isolates execution, fees, and derivative PnL from passive spot beta.
	StrategyEquityChangeUSD int64 `json:"strategy_equity_change_usd"`
}

// RaceArbTerminalReports preserves configured-tier order and values every
// race client in USD using strict two-sided ABC/USD and Q/USD terminal marks.
func (s *Sim) RaceArbTerminalReports() ([]RaceArbTerminalReport, error) {
	if len(s.RaceArbs) != len(s.RaceArbTiers) || len(s.RaceArbs) != len(s.RaceArbIDs) {
		return nil, fmt.Errorf("feesim: inconsistent race-arb metadata")
	}
	abcMark, ok := s.ex.TwoSidedMidPrice("ABC/USD")
	if !ok {
		return nil, fmt.Errorf("feesim: missing terminal two-sided ABC/USD mark")
	}
	qMark, ok := s.ex.TwoSidedMidPrice("Q/USD")
	if !ok {
		return nil, fmt.Errorf("feesim: missing terminal two-sided Q/USD mark")
	}
	spec := etypes.AccountValuationSpec{
		ReportAsset: "USD", ReportPrecision: usdPrecision,
		AssetMarks: map[string]etypes.AssetValuationMark{
			"USD": {Price: usdPrecision, Precision: usdPrecision},
			"ABC": {Price: abcMark, Precision: abcPrecision},
			"Q":   {Price: qMark, Precision: qPrecision},
		},
	}
	initial := raceInitialEquityUSD()
	passive, err := racePassiveTerminalEquityUSD(abcMark, qMark)
	if err != nil {
		return nil, err
	}
	reports := make([]RaceArbTerminalReport, 0, len(s.RaceArbs))
	for index, arb := range s.RaceArbs {
		account, err := s.ex.MarkedAccount(s.RaceArbIDs[index], spec)
		if err != nil {
			return nil, fmt.Errorf("feesim: mark race tier %g: %w", s.RaceArbTiers[index], err)
		}
		change, ok := etypes.TrySub(account.Equity, passive)
		if !ok {
			return nil, fmt.Errorf("feesim: race tier %g strategy equity delta overflows", s.RaceArbTiers[index])
		}
		reports = append(reports, RaceArbTerminalReport{
			Tier: s.RaceArbTiers[index], ClientID: s.RaceArbIDs[index], Fills: arb.Report(),
			InitialEquityUSD: initial, PassiveTerminalEquityUSD: passive,
			TerminalAccount: account, StrategyEquityChangeUSD: change,
		})
	}
	return reports, nil
}

func (s *Sim) Close() {
	for _, l := range s.Loggers {
		l.Close()
	}
}

func NewSim(simTime time.Duration, cfg SimConfig) (*Sim, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	spotTakerBps := cfg.SpotTakerBps
	perpTakerBps := cfg.PerpTakerBps
	logDir := cfg.LogDir
	simClock := simulation.NewSimulatedClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	estimatedClients := 5*cfg.MMCount + cfg.NoiseTraderCount + 2 + 2 + 1 + len(cfg.RaceArbTiers)
	if cfg.ValueTraderBandBps > 0 {
		estimatedClients += 2
	}
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		EstimatedClients:        estimatedClients,
		Clock:                   simClock,
		TickerFactory:           timerFact,
		SnapshotInterval:        time.Second,
		BalanceSnapshotInterval: 10 * time.Second,
		DeterministicIngress:    cfg.Deterministic,
		DeterministicPhases:     cfg.Deterministic,
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
	latencyFor := func(clientID uint64, factor float64) simulation.LatencyConfig {
		return simulation.LatencyConfig{
			Request:    simulation.NewLogNormalLatency(scaleDur(latMin, factor), scaleDur(latMedian, factor), cfg.LatencySigma, latencySeed(cfg.Seed, clientID, 1)),
			Response:   simulation.NewLogNormalLatency(scaleDur(latMin, factor), scaleDur(latMedian, factor), cfg.LatencySigma, latencySeed(cfg.Seed, clientID, 2)),
			MarketData: simulation.NewLogNormalLatency(scaleDur(latMin, factor), scaleDur(latMedian, factor), cfg.LatencySigma, latencySeed(cfg.Seed, clientID, 3)),
			// Deliver at exact sim timestamps; wall-clock sleeps are unrelated to
			// simulation time and would distort latency by orders of magnitude.
			Scheduler: scheduler,
			Clock:     simClock,
		}
	}

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
	var actorMounts []*simulation.Mount

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
		mount := simulation.NewMount(ex, latencyFor(nextClient, 1))
		actorMounts = append(actorMounts, mount)
		gw := mount.ConnectNewClient(nextClient, initBalances, fee)
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

	// Random takers across all five symbols. The first remains available
	// through Sim.Taker for existing callers; every additional participant has
	// a stable independent flow seed and latency stream.
	// Pre-computed target qtys: ~$2000 notional for USD pairs, smaller for cross.
	allSymbols := []string{"ABC/USD", "Q/USD", "ABC-PERP", "Q-PERP", "Q/ABC"}
	targetQtys := map[string]int64{
		"ABC/USD":  4_000_000, // 0.04 BTC ≈ $2000
		"Q/USD":    700_000,   // 0.7 Q ≈ $2100
		"ABC-PERP": 4_000_000, // 0.04 BTC ≈ $2000
		"Q-PERP":   700_000,   // 0.7 Q ≈ $2100
		"Q/ABC":    200_000,   // 0.2 Q (thinner cross market)
	}
	takers := make([]*RandomTaker, 0, cfg.NoiseTraderCount)
	for participant := 0; participant < cfg.NoiseTraderCount; participant++ {
		takerGw := connectActor(spotMakerFee, true)
		seed := cfg.Seed
		if participant != 0 {
			seed = flowSeed(cfg.Seed, uint64(participant), 1)
		}
		taker := NewRandomTaker(nextClient, takerGw, TakerConfig{
			Symbols:           allSymbols,
			TargetQtys:        targetQtys,
			TakeInterval:      time.Duration(cfg.TakerIntervalMs) * time.Millisecond,
			Seed:              seed,
			ImbalanceCoupling: cfg.TakerImbalanceCoupling,
			ExciteAlpha:       cfg.TakerExciteAlpha,
			ExciteBetaPerSec:  cfg.TakerExciteBetaPerSec,
		})
		taker.SetTickerFactory(timerFact)
		takers = append(takers, taker)
	}

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
			BasePrecision: instrumentBasePrecision(spec.spotSym),
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
	var raceArbTiers []float64
	var raceArbIDs []uint64
	var raceMounts []*simulation.Mount
	for _, tier := range cfg.RaceArbTiers {
		nextClient++
		tierMount := simulation.NewMount(ex, latencyFor(nextClient, tier))
		raceMounts = append(raceMounts, tierMount)
		gw := tierMount.ConnectNewClient(nextClient, initBalances, spotMakerFee)
		ex.AddPerpBalance(nextClient, "USD", 10_000_000*usdPrecision)
		arb := NewFeeAwareBasisArb(nextClient, gw, BasisArbConfig{
			SpotSymbol:       "ABC/USD",
			PerpSymbol:       "ABC-PERP",
			SpotFeeBps:       spotTakerBps,
			PerpFeeBps:       perpTakerBps,
			LotSize:          raceLotSize,
			BasePrecision:    abcPrecision,
			MaxPosition:      raceMaxPosition,
			CheckInterval:    100 * time.Millisecond,
			Reactive:         cfg.RaceArbReactive,
			HedgeResidual:    cfg.RaceArbHedge,
			SequentialLegs:   cfg.RaceArbSequential,
			PostSecondLeg:    cfg.RaceArbPostLeg,
			SecondLegTimeout: time.Duration(cfg.RaceArbLegTimeoutMs) * time.Millisecond,
			PostImproveTicks: cfg.RaceArbImproveTicks,
			TickSize:         abcUSDTick,
		})
		arb.SetTickerFactory(timerFact)
		raceArbs = append(raceArbs, arb)
		raceArbTiers = append(raceArbTiers, tier)
		raceArbIDs = append(raceArbIDs, nextClient)
	}

	// Optional value traders. Each holds its own opinion of value; nothing in the
	// simulation validates that opinion, and only their trading moves the price.
	var valueTraders []*ValueTrader
	if cfg.ValueTraderBandBps > 0 {
		type valueSpec struct {
			symbol        string
			believedValue int64
			lotQty        int64
		}
		valueSpecs := []valueSpec{
			{"ABC/USD", abcBootstrapUSD, targetQtys["ABC/USD"]},
			{"Q/USD", qBootstrapUSD, targetQtys["Q/USD"]},
		}
		for _, spec := range valueSpecs {
			gw := connectActor(spotMakerFee, false)
			vt := NewValueTrader(nextClient, gw, ValueTraderConfig{
				Symbol:        spec.symbol,
				BelievedValue: spec.believedValue,
				BandBps:       cfg.ValueTraderBandBps,
				LotQty:        spec.lotQty,
				MaxPosition:   cfg.ValueTraderMaxLots * spec.lotQty,
				Interval:      time.Duration(cfg.ValueTraderIntervalMs) * time.Millisecond,
			})
			vt.SetTickerFactory(timerFact)
			valueTraders = append(valueTraders, vt)
		}
	}

	// ---------- Runner ----------

	const step = time.Millisecond
	runner := simulation.NewRunner(simClock, simulation.RunnerConfig{
		Iterations:          int(simTime / step),
		Step:                step,
		StepSleep:           time.Duration(cfg.StepSleepUs) * time.Microsecond,
		Quiesce:             cfg.Deterministic,
		DeterministicPhases: cfg.Deterministic,
	})
	runner.AddIdler(timerFact)
	runner.AddMount(mmMount)
	for _, m := range actorMounts {
		runner.AddMount(m)
	}
	for _, m := range raceMounts {
		runner.AddMount(m)
	}
	for _, mm := range mms {
		runner.AddActor(mm)
	}
	for _, taker := range takers {
		runner.AddActor(taker)
	}
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
		Taker:        takers[0],
		Takers:       takers,
		BasisArbs:    basisArbs,
		FundingArbs:  fundingArbs,
		TriArb:       triArb,
		RaceArbs:     raceArbs,
		RaceArbTiers: raceArbTiers,
		RaceArbIDs:   raceArbIDs,
		ValueTraders: valueTraders,
		Loggers:      allLoggers,
		ex:           ex,
	}, nil
}

func flowSeed(master int64, participant, stream uint64) int64 {
	value := uint64(master) + 0x9e3779b97f4a7c15
	value ^= participant * 0xbf58476d1ce4e5b9
	value ^= stream * 0x94d049bb133111eb
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31
	return int64(value & ((uint64(1) << 63) - 1))
}

func raceInitialEquityUSD() int64 {
	abcValue := int64(1_000) * abcBootstrapUSD
	qValue := int64(100_000) * qBootstrapUSD
	spotUSD := int64(100_000_000) * usdPrecision
	perpUSD := int64(10_000_000) * usdPrecision
	value := mustAdd(abcValue, qValue, "race initial ABC/Q equity")
	value = mustAdd(value, spotUSD, "race initial spot USD equity")
	return mustAdd(value, perpUSD, "race initial perp USD equity")
}

// racePassiveTerminalEquityUSD values the static inventory granted to every
// race participant at the supplied terminal marks. Race reports subtract it
// so a common ABC/Q directional move is never labelled latency-arb PnL.
func racePassiveTerminalEquityUSD(abcMark, qMark int64) (int64, error) {
	if abcMark <= 0 || qMark <= 0 {
		return 0, fmt.Errorf("feesim: passive race valuation requires positive ABC and Q marks")
	}
	abcValue, ok := etypes.TryMulDiv(int64(1_000)*abcPrecision, abcMark, abcPrecision)
	if !ok {
		return 0, fmt.Errorf("feesim: passive ABC valuation overflows")
	}
	qValue, ok := etypes.TryMulDiv(int64(100_000)*qPrecision, qMark, qPrecision)
	if !ok {
		return 0, fmt.Errorf("feesim: passive Q valuation overflows")
	}
	value, ok := etypes.TryAdd(abcValue, qValue)
	if !ok {
		return 0, fmt.Errorf("feesim: passive ABC/Q valuation overflows")
	}
	value, ok = etypes.TryAdd(value, int64(100_000_000)*usdPrecision)
	if !ok {
		return 0, fmt.Errorf("feesim: passive spot USD valuation overflows")
	}
	value, ok = etypes.TryAdd(value, int64(10_000_000)*usdPrecision)
	if !ok {
		return 0, fmt.Errorf("feesim: passive perp USD valuation overflows")
	}
	return value, nil
}

func instrumentBasePrecision(symbol string) int64 {
	switch symbol {
	case "ABC/USD":
		return abcPrecision
	case "Q/USD":
		return qPrecision
	default:
		panic("feesim: missing basis-arb base precision for " + symbol)
	}
}
