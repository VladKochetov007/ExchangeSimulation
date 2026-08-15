package latencylab

import (
	"context"
	"fmt"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

const (
	initialUSD = int64(200)
	initialABC = int64(1)
)

// Config holds the physical latency assignment. Client IDs stay fixed across a
// label swap, so the experiment cannot mistake deterministic ingress priority
// for an information-speed effect.
type Config struct {
	AlphaLatency  time.Duration
	BetaLatency   time.Duration
	ReverseActors bool
	Duration      time.Duration
}

func DefaultConfig() Config {
	return Config{AlphaLatency: time.Millisecond, BetaLatency: 5 * time.Millisecond, Duration: 50 * time.Millisecond}
}

func (c *Config) normalize() error {
	if c.Duration == 0 {
		c.Duration = 50 * time.Millisecond
	}
	if c.AlphaLatency <= 0 || c.BetaLatency <= 0 || c.AlphaLatency == c.BetaLatency {
		return fmt.Errorf("latencylab: racers require unequal positive latency")
	}
	if c.Duration < 40*time.Millisecond || c.Duration%time.Millisecond != 0 {
		return fmt.Errorf("latencylab: duration must be a millisecond multiple of at least 40ms")
	}
	return nil
}

type Result struct {
	Alpha RacerReport `json:"alpha"`
	Beta  RacerReport `json:"beta"`
}

func (r Result) Winner() string {
	if r.Alpha.PairComplete == r.Beta.PairComplete {
		return ""
	}
	if r.Alpha.PairComplete {
		return alphaName
	}
	return betaName
}

type Sim struct {
	Runner *simulation.Runner
	alpha  *raceActor
	beta   *raceActor
	ex     *exchange.Exchange
}

func newLatencyMount(ex *exchange.Exchange, scheduler *simulation.EventScheduler, clock *simulation.SimulatedClock, latency time.Duration) *simulation.Mount {
	return simulation.NewMount(ex, simulation.LatencyConfig{
		Request:    simulation.NewConstantLatency(latency),
		Response:   simulation.NewConstantLatency(latency),
		MarketData: simulation.NewConstantLatency(latency),
		Scheduler:  scheduler,
		Clock:      clock,
	})
}

func NewSim(cfg Config) (*Sim, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	timers := simulation.NewSimTimerFactory(scheduler)
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		Clock: clock, TickerFactory: timers, DeterministicIngress: true, DeterministicPhases: true,
	})
	for _, symbol := range []string{signalSymbol, buySymbol, sellSymbol} {
		base := "ABC"
		if symbol == signalSymbol {
			base = "SIG"
		}
		ex.AddInstrument(exchange.NewSpotInstrument(symbol, base, "USD", 1, 1, 1, 1))
	}

	direct := simulation.NewMount(ex, simulation.LatencyConfig{})
	liquidity := direct.ConnectNewClient(1, map[string]int64{"ABC": 2, "SIG": 1, "USD": 1_000}, &exchange.FixedFee{})
	signalGateway := direct.ConnectNewClient(2, map[string]int64{"USD": 1}, &exchange.FixedFee{})
	if err := seedBook(ex); err != nil {
		return nil, err
	}

	alphaMount := newLatencyMount(ex, scheduler, clock, cfg.AlphaLatency)
	betaMount := newLatencyMount(ex, scheduler, clock, cfg.BetaLatency)
	alphaGateway := alphaMount.ConnectNewClient(alphaClientID, map[string]int64{"ABC": initialABC, "USD": initialUSD}, &exchange.FixedFee{})
	betaGateway := betaMount.ConnectNewClient(betaClientID, map[string]int64{"ABC": initialABC, "USD": initialUSD}, &exchange.FixedFee{})
	alpha := newRaceActor(alphaName, alphaClientID, alphaGateway, clock)
	beta := newRaceActor(betaName, betaClientID, betaGateway, clock)
	passive := newPassiveActor(1, liquidity)
	signal := newSignalActor(2, signalGateway)
	for _, candidate := range []interface{ SetTickerFactory(exchange.TickerFactory) }{alpha, beta, signal, passive} {
		candidate.SetTickerFactory(timers)
	}

	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: int(cfg.Duration / time.Millisecond), Step: time.Millisecond, DeterministicPhases: true,
	})
	runner.AddIdler(timers)
	for _, mount := range []*simulation.Mount{direct, alphaMount, betaMount} {
		runner.AddMount(mount)
	}
	runner.AddActor(passive)
	runner.AddActor(signal)
	if cfg.ReverseActors {
		runner.AddActor(beta)
		runner.AddActor(alpha)
	} else {
		runner.AddActor(alpha)
		runner.AddActor(beta)
	}
	return &Sim{Runner: runner, alpha: alpha, beta: beta, ex: ex}, nil
}

func seedBook(ex *exchange.Exchange) error {
	orders := []exchange.OrderRequest{
		{RequestID: 1, Symbol: signalSymbol, Side: exchange.Sell, Type: exchange.LimitOrder, Price: 1, Qty: raceQty, TimeInForce: exchange.GTC, Visibility: exchange.Normal},
		{RequestID: 2, Symbol: buySymbol, Side: exchange.Sell, Type: exchange.LimitOrder, Price: 99, Qty: raceQty, TimeInForce: exchange.GTC, Visibility: exchange.Normal},
		{RequestID: 3, Symbol: sellSymbol, Side: exchange.Buy, Type: exchange.LimitOrder, Price: 101, Qty: raceQty, TimeInForce: exchange.GTC, Visibility: exchange.Normal},
	}
	for index := range orders {
		response := ex.PlaceOrder(1, &orders[index])
		if !response.Success {
			return fmt.Errorf("latencylab: seed order %d rejected: %s", index, response.Error)
		}
	}
	return nil
}

func (s *Sim) Run(ctx context.Context) (Result, error) {
	var result Result
	var invariantErr error
	s.Runner.SetShutdownHook(func() {
		result.Alpha = s.alpha.Report()
		result.Beta = s.beta.Report()
		for _, report := range []*RacerReport{&result.Alpha, &result.Beta} {
			client := s.ex.Clients[report.ClientID]
			report.AccountUSDDelta = client.Balances["USD"] - initialUSD
			report.AccountABCDelta = client.Balances["ABC"] - initialABC
			if report.AccountUSDDelta != report.ObservedCashflow {
				invariantErr = fmt.Errorf("latencylab: %s cashflow %d disagrees with ledger %d", report.Name, report.ObservedCashflow, report.AccountUSDDelta)
				return
			}
			if report.PairComplete && report.AccountABCDelta != 0 {
				invariantErr = fmt.Errorf("latencylab: completed %s pair has base residual %d", report.Name, report.AccountABCDelta)
				return
			}
		}
	})
	if err := s.Runner.Run(ctx); err != nil {
		return Result{}, err
	}
	return result, invariantErr
}

var _ actor.Actor = (*raceActor)(nil)
var _ actor.Actor = (*signalActor)(nil)
var _ actor.Actor = (*passiveActor)(nil)
