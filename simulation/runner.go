package simulation

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type RunnerConfig struct {
	UseSimulatedClock bool
	Duration          time.Duration
	Iterations        int
}

type Runner struct {
	exchange *exchange.Exchange
	actors   []actor.Actor
	config   RunnerConfig
	clock    Clock
}

func NewRunner(config RunnerConfig) *Runner {
	var clock Clock
	if config.UseSimulatedClock {
		clock = NewSimulatedClock(time.Now().UnixNano())
	} else {
		clock = &RealClock{}
	}

	return &Runner{
		exchange: exchange.NewExchange(100, clock),
		actors:   make([]actor.Actor, 0),
		config:   config,
		clock:    clock,
	}
}

func (r *Runner) Exchange() *exchange.Exchange {
	return r.exchange
}

func (r *Runner) AddActor(a actor.Actor) {
	r.actors = append(r.actors, a)
}

func (r *Runner) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	for _, a := range r.actors {
		if err := a.Start(ctx); err != nil {
			return err
		}
	}

	var wg sync.WaitGroup
	if r.config.Duration > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			timer := time.NewTimer(r.config.Duration)
			defer timer.Stop()
			select {
			case <-timer.C:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	if r.config.Iterations > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if simClock, ok := r.clock.(*SimulatedClock); ok {
				for i := 0; i < r.config.Iterations; i++ {
					select {
					case <-ctx.Done():
						return
					default:
						simClock.Advance(time.Millisecond)
						time.Sleep(time.Microsecond)
					}
				}
				cancel()
			}
		}()
	}

	select {
	case <-sigCh:
		cancel()
	case <-ctx.Done():
	}

	for _, a := range r.actors {
		a.Stop()
	}

	wg.Wait()
	r.exchange.Shutdown()
	return nil
}
