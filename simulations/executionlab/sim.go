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
	// ParentCount schedules independent parent-order clients using the same
	// policy and deterministic side schedule. One preserves the original
	// single-parent experiment.
	ParentCount int
	// ParentInterval separates consecutive parent decisions. It is required
	// when ParentCount is greater than one so a study cannot accidentally send
	// a simultaneous, inseparable parent-order burst.
	ParentInterval time.Duration
	Parent         ParentOrderConfig
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
	if c.ParentCount > 1 && c.ParentInterval <= 0 {
		return fmt.Errorf("executionlab: parent interval must be positive when parent count exceeds one")
	}
	if c.BackgroundLatency < 0 || c.ExecutionLatency < 0 {
		return fmt.Errorf("executionlab: latency must be non-negative")
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
	minimumDuration := lastChild + c.ExecutionLatency + c.ExecutionLatency + 2*c.Parent.PollInterval
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
}

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

func NewSim(cfg SimConfig) (*Sim, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock:                clock,
		TickerFactory:        timers,
		DeterministicIngress: true,
		DeterministicPhases:  true,
	})
	ex.AddInstrument(exchange.NewSpotInstrument(
		cfg.Parent.Symbol, "ABC", cfg.Parent.QuoteAsset, basePrecision, quotePrecision,
		priceTick, basePrecision/100,
	))

	fee := &exchange.PercentageFee{MakerBps: 0, TakerBps: 5, InQuote: true}
	mmFee := &exchange.PercentageFee{}
	balances := map[string]int64{
		"ABC": 100_000 * basePrecision,
		"USD": 100_000_000 * quotePrecision,
	}

	directMount := simulation.NewMount(ex, simulation.LatencyConfig{})
	mounts := []*simulation.Mount{directMount}
	actors := make([]actor.Actor, 0, cfg.MMCount+cfg.NoiseTraderCount+cfg.ParentCount)
	clientID := uint64(0)
	for i := 0; i < cfg.MMCount; i++ {
		clientID++
		gateway := directMount.ConnectNewClient(clientID, balances, mmFee)
		mm := feesim.NewMarketMaker(clientID, gateway, feesim.MMConfig{
			Symbol:         cfg.Parent.Symbol,
			BootstrapPrice: bootstrapPrice,
			Levels:         5,
			LevelSpacing:   2,
			LevelSize:      basePrecision / 4,
			TickSize:       priceTick,
			MidPriceMode:   feesim.MidFromWeightedMid,
			BaseInterval:   10*time.Millisecond + time.Duration(i)*time.Millisecond,
			MaxInterval:    30*time.Millisecond + time.Duration(i)*time.Millisecond,
		})
		mm.SetTickerFactory(timers)
		actors = append(actors, mm)
	}
	for i := 0; i < cfg.NoiseTraderCount; i++ {
		clientID++
		mount := newLatencyMount(ex, scheduler, clock, cfg.BackgroundLatency)
		mounts = append(mounts, mount)
		gateway := mount.ConnectNewClient(clientID, balances, fee)
		noise := feesim.NewRandomTaker(clientID, gateway, feesim.TakerConfig{
			Symbols:      []string{cfg.Parent.Symbol},
			TargetQtys:   map[string]int64{cfg.Parent.Symbol: basePrecision / 20},
			TakeInterval: 25 * time.Millisecond,
			Seed:         cfg.Seed + int64(i) + 1,
		})
		noise.SetTickerFactory(timers)
		actors = append(actors, noise)
	}
	parents := make([]*executionAgent, 0, cfg.ParentCount)
	for i := 0; i < cfg.ParentCount; i++ {
		clientID++
		executionMount := newLatencyMount(ex, scheduler, clock, cfg.ExecutionLatency)
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
		parents = append(parents, parent)
		actors = append(actors, parent)
	}

	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations:          int(cfg.Duration / time.Millisecond),
		Step:                time.Millisecond,
		DeterministicPhases: true,
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
		exchange: ex, clock: clock, mounts: mounts, actors: actors,
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
