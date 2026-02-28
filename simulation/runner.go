package simulation

import (
	"context"
	"time"

	"exchange_sim/actor"
)

type RunnerConfig struct {
	Duration   time.Duration // wall-clock limit (0 = ctx-only)
	Iterations int           // simulated clock steps (0 = ctx-only)
	Step       time.Duration // step size per iteration for SimulatedClock (default 1ms)
}

type Runner struct {
	clock  Clock
	venues []*Venue
	actors []actor.Actor
	config RunnerConfig
}

func NewRunner(clock Clock, config RunnerConfig) *Runner {
	if config.Step == 0 {
		config.Step = time.Millisecond
	}
	return &Runner{
		clock:  clock,
		venues: make([]*Venue, 0),
		actors: make([]actor.Actor, 0),
		config: config,
	}
}

func (r *Runner) AddVenue(v *Venue) {
	r.venues = append(r.venues, v)
}

func (r *Runner) AddActor(a actor.Actor) {
	r.actors = append(r.actors, a)
}

func (r *Runner) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, a := range r.actors {
		if err := a.Start(ctx); err != nil {
			return err
		}
	}

	if r.config.Duration > 0 {
		go func() {
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
		go func() {
			if advanceable, ok := r.clock.(Advanceable); ok {
				for i := 0; i < r.config.Iterations; i++ {
					select {
					case <-ctx.Done():
						return
					default:
						advanceable.Advance(r.config.Step)
						time.Sleep(time.Microsecond)
					}
				}
			}
			cancel()
		}()
	}

	<-ctx.Done()

	for _, a := range r.actors {
		a.Stop()
	}
	for _, v := range r.venues {
		v.Shutdown()
	}

	return nil
}
