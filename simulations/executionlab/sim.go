package executionlab

import (
	"context"
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	"exchange_sim/simulations/feesim"
)

const (
	basePrecision  = int64(100_000_000)
	quotePrecision = int64(100_000)
	bootstrapPrice = int64(50_000) * quotePrecision
	priceTick      = int64(10) * quotePrecision
)

// SimConfig controls a single-world execution experiment. To compare
// policies, construct separate worlds with identical seed and every field
// except Parent.Policy. The endogenous paths may diverge after the parent
// executes; that divergence is the treatment effect, not a seed mismatch.
type SimConfig struct {
	Seed              int64
	Duration          time.Duration
	MMCount           int
	NoiseTraderCount  int
	BackgroundLatency time.Duration
	ExecutionLatency  time.Duration
	// ParentDeployment is an explicit directed transport/actor-processing
	// assignment. Nil preserves the legacy symmetric ExecutionLatency path.
	ParentDeployment                 *ParentDeployment
	RecordSnapshotProjectionEvidence bool
	// ParentCount schedules independent parent-order clients using the same
	// policy and deterministic side schedule. One preserves the original
	// single-parent experiment.
	ParentCount int
	// ParentClientID overrides the first parent's account ID for controlled
	// identity-swap fixtures. Zero uses the next sequential ID.
	ParentClientID uint64
	// ParentInterval separates consecutive parent decisions. It is required
	// when ParentCount is greater than one so a study cannot accidentally send
	// a simultaneous, inseparable parent-order burst.
	ParentInterval time.Duration
	Parent         ParentOrderConfig
}

type ParentDeployment struct {
	MarketDataLatency time.Duration `json:"market_data_latency_nanos"`
	RequestLatency    time.Duration `json:"request_latency_nanos"`
	ResponseLatency   time.Duration `json:"response_latency_nanos"`
	ProcessingDelay   time.Duration `json:"processing_delay_nanos"`
}

func DefaultSimConfig(policy Policy) SimConfig {
	return SimConfig{
		Seed:              42,
		Duration:          4 * time.Second,
		MMCount:           4,
		NoiseTraderCount:  8,
		BackgroundLatency: 2 * time.Millisecond,
		ExecutionLatency:  time.Millisecond,
		ParentCount:       1,
		ParentInterval:    time.Second,
		Parent: ParentOrderConfig{
			Symbol:        "ABC/USD",
			Side:          exchange.Buy,
			TargetQty:     2 * basePrecision,
			BasePrecision: basePrecision,
			QuoteAsset:    "USD",
			Policy:        policy,
			DecisionAfter: time.Second,
			SliceInterval: 200 * time.Millisecond,
			SliceCount:    5,
			PollInterval:  time.Millisecond,
		},
	}
}

func (c *SimConfig) normalize() error {
	if c.Seed == 0 {
		c.Seed = 42
	}
	if c.MMCount == 0 {
		c.MMCount = 4
	}
	if c.NoiseTraderCount == 0 {
		c.NoiseTraderCount = 8
	}
	if c.Duration == 0 {
		c.Duration = 4 * time.Second
	}
	if c.ParentCount == 0 {
		c.ParentCount = 1
	}
	if c.ParentCount < 1 {
		return fmt.Errorf("executionlab: parent count must be positive")
	}
	if c.ParentClientID != 0 && c.ParentClientID <= uint64(c.MMCount+c.NoiseTraderCount) {
		return fmt.Errorf("executionlab: parent client ID must not collide with a background account")
	}
	if c.ParentClientID > ^uint64(0)-uint64(c.ParentCount-1) {
		return fmt.Errorf("executionlab: parent client ID range overflows")
	}
	if c.MMCount < 0 || c.NoiseTraderCount < 0 {
		return fmt.Errorf("executionlab: background account counts must be non-negative")
	}
	if c.ParentCount > 1 && c.ParentInterval <= 0 {
		return fmt.Errorf("executionlab: parent interval must be positive when parent count exceeds one")
	}
	if c.BackgroundLatency < 0 || c.ExecutionLatency < 0 {
		return fmt.Errorf("executionlab: latency must be non-negative")
	}
	if c.ParentDeployment != nil {
		deployment := c.ParentDeployment
		if c.ExecutionLatency != 0 || deployment.MarketDataLatency < 0 || deployment.RequestLatency < 0 ||
			deployment.ResponseLatency < 0 || deployment.ProcessingDelay < 0 {
			return fmt.Errorf("executionlab: explicit parent deployment requires zero legacy latency and non-negative components")
		}
	}
	if err := c.Parent.validate(); err != nil {
		return err
	}
	lastChild := c.Parent.DecisionAfter
	if c.Parent.Policy == TWAP {
		lastChild += time.Duration(c.Parent.SliceCount-1) * c.Parent.SliceInterval
	}
	lastChild += time.Duration(c.ParentCount-1) * c.ParentInterval
	// Constant latency gives this experiment a finite causal horizon. A
	// log-normal tail has no finite drain bound and would turn unprocessed
	// children at shutdown into fabricated execution failures.
	requestLatency, responseLatency, processingDelay := c.ExecutionLatency, c.ExecutionLatency, time.Duration(0)
	if c.ParentDeployment != nil {
		requestLatency, responseLatency, processingDelay = c.ParentDeployment.RequestLatency, c.ParentDeployment.ResponseLatency, c.ParentDeployment.ProcessingDelay
	}
	minimumDuration := lastChild + processingDelay + requestLatency + responseLatency + 2*c.Parent.PollInterval
	if c.Duration < minimumDuration {
		return fmt.Errorf("executionlab: duration %s ends before final child can arrive and be observed (%s)", c.Duration, minimumDuration)
	}
	return nil
}

type Sim struct {
	Runner   *simulation.Runner
	Parent   *executionAgent
	Parents  []*executionAgent
	exchange *exchange.Exchange
	clock    *simulation.SimulatedClock
	mounts   []*simulation.Mount
	actors   []actor.Actor
	contract WorldContract
	observe  func(EvidenceObservation)
}

func (s *Sim) WorldContract() WorldContract { return s.contract.clone() }

func newLatencyMount(ex *exchange.Exchange, scheduler *simulation.EventScheduler, clock *simulation.SimulatedClock, delay time.Duration) *simulation.Mount {
	if delay == 0 {
		return simulation.NewMount(ex, simulation.LatencyConfig{})
	}
	return simulation.NewMount(ex, simulation.LatencyConfig{
		Request:    simulation.NewConstantLatency(delay),
		Response:   simulation.NewConstantLatency(delay),
		MarketData: simulation.NewConstantLatency(delay),
		Scheduler:  scheduler,
		Clock:      clock,
	})
}

func newDirectedLatencyMount(ex *exchange.Exchange, scheduler *simulation.EventScheduler, clock *simulation.SimulatedClock, deployment ParentDeployment) *simulation.Mount {
	if deployment.MarketDataLatency == 0 && deployment.RequestLatency == 0 && deployment.ResponseLatency == 0 {
		return simulation.NewMount(ex, simulation.LatencyConfig{})
	}
	return simulation.NewMount(ex, simulation.LatencyConfig{
		Request:    simulation.NewConstantLatency(deployment.RequestLatency),
		Response:   simulation.NewConstantLatency(deployment.ResponseLatency),
		MarketData: simulation.NewConstantLatency(deployment.MarketDataLatency),
		Scheduler:  scheduler, Clock: clock,
	})
}

func NewSim(cfg SimConfig) (*Sim, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	runnerContract := RunnerContract{
		Iterations: int(cfg.Duration / time.Millisecond), Step: time.Millisecond,
		DeterministicIngress: true, DeterministicPhases: true,
	}
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock:                            clock,
		TickerFactory:                    timers,
		DeterministicIngress:             runnerContract.DeterministicIngress,
		DeterministicPhases:              runnerContract.DeterministicPhases,
		RecordSnapshotProjectionEvidence: cfg.RecordSnapshotProjectionEvidence,
	})
	instrument := InstrumentContract{
		Symbol: cfg.Parent.Symbol, BaseAsset: "ABC", QuoteAsset: cfg.Parent.QuoteAsset,
		BasePrecision: basePrecision, QuotePrecision: quotePrecision,
		TickSize: priceTick, LotSize: basePrecision / 100,
	}
	ex.AddInstrument(exchange.NewSpotInstrument(
		instrument.Symbol, instrument.BaseAsset, instrument.QuoteAsset,
		instrument.BasePrecision, instrument.QuotePrecision, instrument.TickSize, instrument.LotSize,
	))

	fee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	mmFee := &exchange.PercentageFee{}
	balances := map[string]int64{
		"ABC": 100_000 * basePrecision,
		"USD": 100_000_000 * quotePrecision,
	}
	contract := WorldContract{
		SchemaVersion: 1,
		Config:        cfg,
		Instrument:    instrument,
		Bootstrap:     bootstrapPrice,
		Runner:        runnerContract,
	}
	addAccount := func(id uint64, role string, feeModel *exchange.PercentageFee) {
		contract.Accounts = append(contract.Accounts, AccountContract{
			ClientID: id, Role: role, InitialBalances: balances, Fee: *feeModel,
		})
	}

	directMount := simulation.NewMount(ex, simulation.LatencyConfig{})
	mounts := []*simulation.Mount{directMount}
	actors := make([]actor.Actor, 0, cfg.MMCount+cfg.NoiseTraderCount+cfg.ParentCount)
	clientID := uint64(0)
	for i := 0; i < cfg.MMCount; i++ {
		clientID++
		gateway := directMount.ConnectNewClient(clientID, balances, mmFee)
		makerConfig := feesim.MMConfig{
			Symbol:         cfg.Parent.Symbol,
			BootstrapPrice: bootstrapPrice,
			Levels:         5,
			LevelSpacing:   2,
			LevelSize:      basePrecision / 4,
			TickSize:       priceTick,
			MidPriceMode:   feesim.MidFromWeightedMid,
			BaseInterval:   10*time.Millisecond + time.Duration(i)*time.Millisecond,
			MaxInterval:    30*time.Millisecond + time.Duration(i)*time.Millisecond,
		}
		mm := feesim.NewMarketMaker(clientID, gateway, makerConfig)
		addAccount(clientID, "maker", mmFee)
		contract.Makers = append(contract.Makers, MakerContract{
			ClientID: clientID, Config: makerConfig,
			RealizedLevelCadences: mm.RealizedLevelCadences(),
		})
		mm.SetTickerFactory(timers)
		actors = append(actors, mm)
	}
	for i := 0; i < cfg.NoiseTraderCount; i++ {
		clientID++
		mount := newLatencyMount(ex, scheduler, clock, cfg.BackgroundLatency)
		mounts = append(mounts, mount)
		gateway := mount.ConnectNewClient(clientID, balances, fee)
		noiseConfig := feesim.TakerConfig{
			Symbols:      []string{cfg.Parent.Symbol},
			TargetQtys:   map[string]int64{cfg.Parent.Symbol: basePrecision / 20},
			TakeInterval: 25 * time.Millisecond,
			Seed:         cfg.Seed + int64(i) + 1,
		}
		noise := feesim.NewRandomTaker(clientID, gateway, noiseConfig)
		addAccount(clientID, "random_taker", fee)
		contract.Noise = append(contract.Noise, NoiseContract{
			ClientID: clientID, Symbol: cfg.Parent.Symbol,
			TargetQty:    noiseConfig.TargetQtys[cfg.Parent.Symbol],
			TakeInterval: noiseConfig.TakeInterval, DecisionPhaseOffset: noiseConfig.DecisionPhaseOffset,
			Seed: noiseConfig.Seed, Latency: cfg.BackgroundLatency,
			ImbalanceCoupling: noiseConfig.ImbalanceCoupling,
			ExciteAlpha:       noiseConfig.ExciteAlpha, ExciteBetaPerSec: noiseConfig.ExciteBetaPerSec,
			SizeParetoAlpha: noiseConfig.SizeParetoAlpha, SizeCapMultiple: noiseConfig.SizeCapMultiple,
		})
		noise.SetTickerFactory(timers)
		actors = append(actors, noise)
	}
	parents := make([]*executionAgent, 0, cfg.ParentCount)
	for i := 0; i < cfg.ParentCount; i++ {
		clientID++
		if i == 0 && cfg.ParentClientID != 0 {
			clientID = cfg.ParentClientID
		}
		var executionMount *simulation.Mount
		if cfg.ParentDeployment != nil {
			executionMount = newDirectedLatencyMount(ex, scheduler, clock, *cfg.ParentDeployment)
		} else {
			executionMount = newLatencyMount(ex, scheduler, clock, cfg.ExecutionLatency)
		}
		mounts = append(mounts, executionMount)
		parentCfg := cfg.Parent
		parentCfg.DecisionAfter += time.Duration(i) * cfg.ParentInterval
		// Alternate side so a long study creates persistent two-sided parent
		// demand rather than an artificial one-way inventory drain. The policy
		// comparison gets the identical side schedule in its paired world.
		if i%2 == 1 {
			parentCfg.Side = opposite(parentCfg.Side)
		}
		parentGateway := executionMount.ConnectNewClient(clientID, balances, fee)
		parent, err := newExecutionAgent(clientID, parentGateway, parentCfg)
		if err != nil {
			return nil, err
		}
		parent.SetTickerFactory(timers)
		parent.observationTime = clock.NowUnixNano
		if cfg.ParentDeployment != nil {
			parent.processingDelay = cfg.ParentDeployment.ProcessingDelay
			parent.emitProcessingCompletion = true
		}
		addAccount(clientID, "parent", fee)
		contract.Parents = append(contract.Parents, ParentContract{
			ClientID: clientID, Config: parentCfg, Latency: cfg.ExecutionLatency,
			Deployment: cfg.ParentDeployment,
		})
		parents = append(parents, parent)
		actors = append(actors, parent)
	}

	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations:          runnerContract.Iterations,
		Step:                runnerContract.Step,
		DeterministicPhases: runnerContract.DeterministicPhases,
	})
	runner.AddIdler(timers)
	for _, mount := range mounts {
		runner.AddMount(mount)
	}
	for _, candidate := range actors {
		runner.AddActor(candidate)
	}
	return &Sim{
		Runner: runner, Parent: parents[0], Parents: parents,
		exchange: ex, clock: clock, mounts: mounts, actors: actors, contract: contract.clone(),
	}, nil
}

func (s *Sim) Run(ctx context.Context) (ExecutionReport, error) {
	reports, err := s.RunMany(ctx)
	if err != nil {
		return ExecutionReport{}, err
	}
	return reports[0], nil
}

// RunMany runs every scheduled parent and returns reports in deterministic
// decision/client order. Run remains available for the original one-parent
// callers and returns the first report.
func (s *Sim) RunMany(ctx context.Context) ([]ExecutionReport, error) {
	var terminalMid int64
	s.Runner.SetShutdownHook(func() {
		// The runner invokes its shutdown hook after the final deterministic
		// fixed point and before venue shutdown. The value is therefore a
		// terminal exchange observation, not a delayed actor market-data view.
		terminalMid, _ = s.exchange.TwoSidedMidPrice(s.Parent.cfg.Symbol)
		if s.observe != nil {
			s.exchange.LogAllBalances()
			bid, ask, valid := s.exchange.TwoSidedTopOfBook(s.Parent.cfg.Symbol)
			s.observe(EvidenceObservation{
				Timestamp: s.clock.NowUnixNano(), Source: "exchange", Name: "terminal_book", Route: s.Parent.cfg.Symbol,
				Payload: TerminalBook{Symbol: s.Parent.cfg.Symbol, Bid: bid, Ask: ask, Valid: valid},
			})
		}
	})
	if err := s.Runner.Run(ctx); err != nil {
		return nil, err
	}
	reports := make([]ExecutionReport, 0, len(s.Parents))
	for _, parent := range s.Parents {
		reports = append(reports, parent.reportWithTerminalMark(terminalMid, "two_sided_book_mid"))
	}
	return reports, nil
}

func opposite(side exchange.Side) exchange.Side {
	if side == exchange.Buy {
		return exchange.Sell
	}
	return exchange.Buy
}
