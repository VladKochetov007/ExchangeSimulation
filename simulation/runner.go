package simulation

import (
	"context"
	"runtime"
	"time"

	"exchange_sim/actor"
)

type RunnerConfig struct {
	Duration      time.Duration         // wall-clock limit (0 = ctx-only)
	Iterations    int                   // simulated clock steps (0 = ctx-only)
	Step          time.Duration         // step size per iteration for SimulatedClock (default 1ms)
	ProgressEvery int                   // call OnProgress every N iterations (0 = disabled)
	OnProgress    func(done, total int) // optional progress callback

	// StepSleep is the wall-clock pause after each clock advance, giving
	// actor and exchange goroutines time to drain before the next step.
	// Longer sleeps reduce scheduling nondeterminism at the cost of wall
	// time. 0 = default 1µs; negative = no sleep.
	StepSleep time.Duration

	// Quiesce advances simulated time only once every actor has finished
	// reacting to the previous step. A fixed StepSleep cannot do this: it
	// bets a wall-clock duration against however long the reaction chain
	// happens to take, so the same configuration and seed can produce
	// materially different runs depending on OS scheduling. Waiting for
	// quiescence makes the interleaving a property of the model rather than
	// of the machine, at the cost of wall time.
	Quiesce bool

	// QuiesceTimeout bounds the wait for a settled system so a wedged actor
	// cannot hang the run (default 2s).
	QuiesceTimeout time.Duration
}

// Idler is implemented by components that can report having no work queued
// and none in flight. Actors and mounts implement it; anything that does not
// is treated as always idle.
type Idler interface {
	Idle() bool
}

type Drainer interface {
	Drain() bool
}

type Runner struct {
	clock        Clock
	mounts       []*Mount
	actors       []actor.Actor
	idlers       []Idler
	config       RunnerConfig
	shutdownHook func()
}

func NewRunner(clock Clock, config RunnerConfig) *Runner {
	if config.Step == 0 {
		config.Step = time.Millisecond
	}
	if config.StepSleep == 0 {
		config.StepSleep = time.Microsecond
	}
	return &Runner{
		clock:  clock,
		mounts: make([]*Mount, 0),
		actors: make([]actor.Actor, 0),
		config: config,
	}
}

func (r *Runner) AddMount(m *Mount) {
	r.mounts = append(r.mounts, m)
}

func (r *Runner) AddActor(a actor.Actor) {
	r.actors = append(r.actors, a)
}

// AddIdler registers an extra component whose quiescence the runner must
// respect — a timer factory holding undelivered ticks, for instance.
func (r *Runner) AddIdler(i Idler) {
	r.idlers = append(r.idlers, i)
}

func (r *Runner) SetProgressCallback(every int, fn func(done, total int)) {
	r.config.ProgressEvery = every
	r.config.OnProgress = fn
}

func (r *Runner) SetShutdownHook(fn func()) {
	r.shutdownHook = fn
}

// waitQuiescent blocks until every actor and mount reports idle twice in a
// row, or the timeout expires. Two consecutive observations are required
// because one component can hand work to another between checks, so a single
// all-idle sample can catch the system mid-handoff.
func (r *Runner) waitQuiescent() {
	timeout := r.config.QuiesceTimeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	deadline := time.Now().Add(timeout)
	settled := 0
	for time.Now().Before(deadline) {
		for _, m := range r.mounts {
			if d, ok := any(m).(Drainer); ok {
				d.Drain()
			}
		}
		if r.systemIdle() {
			if settled++; settled >= 2 {
				return
			}
		} else {
			settled = 0
		}
		runtime.Gosched()
	}
}

func (r *Runner) systemIdle() bool {
	for _, a := range r.actors {
		if idler, ok := a.(Idler); ok && !idler.Idle() {
			return false
		}
	}
	for _, m := range r.mounts {
		if !m.Idle() {
			return false
		}
	}
	for _, i := range r.idlers {
		if !i.Idle() {
			return false
		}
	}
	return true
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
						if r.config.Quiesce {
							r.waitQuiescent()
						} else if r.config.StepSleep > 0 {
							time.Sleep(r.config.StepSleep)
						}
					}
					if r.config.OnProgress != nil && r.config.ProgressEvery > 0 && (i+1)%r.config.ProgressEvery == 0 {
						r.config.OnProgress(i+1, r.config.Iterations)
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
	if r.shutdownHook != nil {
		r.shutdownHook()
	}
	for _, m := range r.mounts {
		m.Shutdown()
	}

	return nil
}
