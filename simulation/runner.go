package simulation

import (
	"context"
	"fmt"
	"runtime"
	"strings"
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

	// DeterministicPhases uses a synchronous fixed-point runtime instead of
	// goroutine scheduling. Scheduler-backed latency wrappers are supported
	// only through the runner-owned deterministic courier.
	DeterministicPhases bool

	// PhaseMaxRounds bounds same-timestamp reaction chains. Zero defaults to
	// 100,000. Reaching it is a model error, never a silently truncated run.
	PhaseMaxRounds int
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

type phaseActor interface {
	EnableDeterministicPhases()
	SupportsDeterministicPhases() bool
	PumpDeterministicPhase(context.Context) bool
}

type phaseTimerController interface {
	EnableDeterministicPhases()
	DeterministicPhaseError() error
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

// backpressureOnly reports whether the only non-idle components are mounts
// holding messages an actor has not yet made room for.
func (r *Runner) backpressureOnly() bool {
	for _, a := range r.actors {
		if idler, ok := a.(Idler); ok && !idler.Idle() {
			return false
		}
	}
	for _, i := range r.idlers {
		if !i.Idle() {
			return false
		}
	}
	blocked := false
	for _, m := range r.mounts {
		if m.Idle() {
			continue
		}
		if !m.EgressBlocked() {
			return false
		}
		blocked = true
	}
	return blocked
}

func (r *Runner) deterministicPhasePending() string {
	pending := make([]string, 0)
	for _, a := range r.actors {
		if idler, ok := a.(Idler); ok && !idler.Idle() {
			pending = append(pending, fmt.Sprintf("actor %d", a.ID()))
		}
	}
	for i, m := range r.mounts {
		if !m.Idle() {
			pending = append(pending, fmt.Sprintf("mount %d %s", i, m.PendingDescription()))
		}
	}
	for i, idler := range r.idlers {
		if !idler.Idle() {
			pending = append(pending, fmt.Sprintf("idler %d", i))
		}
	}
	return strings.Join(pending, ", ")
}

func (r *Runner) prepareDeterministicPhases() error {
	if r.config.Iterations <= 0 {
		return fmt.Errorf("simulation: deterministic phases require a positive iteration count")
	}
	if _, ok := r.clock.(Advanceable); !ok {
		return fmt.Errorf("simulation: deterministic phases require an advanceable clock")
	}
	for _, m := range r.mounts {
		if err := m.EnableDeterministicPhases(); err != nil {
			return err
		}
		if err := m.ValidateDeterministicPhases(); err != nil {
			return err
		}
	}
	for _, idler := range r.idlers {
		if controller, ok := idler.(phaseTimerController); ok {
			controller.EnableDeterministicPhases()
		}
	}
	for _, a := range r.actors {
		phase, ok := a.(phaseActor)
		if !ok || !phase.SupportsDeterministicPhases() {
			return fmt.Errorf("simulation: actor %d does not support deterministic phases", a.ID())
		}
		phase.EnableDeterministicPhases()
	}
	return nil
}

func (r *Runner) deterministicPhaseError() error {
	for _, idler := range r.idlers {
		if controller, ok := idler.(phaseTimerController); ok {
			if err := controller.DeterministicPhaseError(); err != nil {
				return err
			}
		}
	}
	return nil
}

// phaseIdleConfirmations is how many consecutive rounds of no progress with
// work still queued are required before the runner calls it a deadlock.
const phaseIdleConfirmations = 64

// drainDeterministicPhases reaches a same-timestamp fixed point using a
// documented global order. Venue-owned scheduled jobs run first, then venue
// ingress, then FIFO egress, then actors in Runner.AddActor order. Every
// callback happens on this goroutine, so neither select fairness nor OS
// scheduling chooses who reacts first.
func (r *Runner) drainDeterministicPhases(ctx context.Context) error {
	limit := r.config.PhaseMaxRounds
	if limit <= 0 {
		limit = 100_000
	}
	noProgressRounds := 0
	for round := 0; round < limit; round++ {
		if err := r.deterministicPhaseError(); err != nil {
			return err
		}

		progressed := false
		for _, m := range r.mounts {
			if m.PumpDeterministicPhase() {
				progressed = true
			}
		}
		for _, m := range r.mounts {
			if m.Drain() {
				progressed = true
			}
		}
		for _, m := range r.mounts {
			if m.DrainDeterministicEgress() {
				progressed = true
			}
		}
		for _, a := range r.actors {
			if a.(phaseActor).PumpDeterministicPhase(ctx) {
				progressed = true
			}
		}

		if err := r.deterministicPhaseError(); err != nil {
			return err
		}
		if progressed {
			noProgressRounds = 0
		}
		if !progressed {
			// Idle can flip under us: the exchange publishes market data from
			// its own goroutine, so a component can report work pending for
			// the instant it takes to hand it over. A single observation of
			// "no progress and not idle" is therefore not evidence of a
			// deadlock; a run of them is. Yielding between observations lets
			// the other goroutine finish rather than spinning against it.
			if !r.systemIdle() {
				noProgressRounds++
				if noProgressRounds < phaseIdleConfirmations {
					runtime.Gosched()
					continue
				}
				// A consumer that is behind is not a deadlock. When every
				// remaining message is waiting on a full actor inbox, the
				// fixed point for this timestamp has been reached and the
				// messages are delivered once the actor drains on its own
				// clock; refusing to advance time here would report a slow
				// participant as a broken simulation.
				if !r.backpressureOnly() {
					return fmt.Errorf("simulation: deterministic phase stalled with queued work: %s", r.deterministicPhasePending())
				}
			}
			return nil
		}
	}
	return fmt.Errorf("simulation: deterministic phase exceeded %d same-timestamp rounds", limit)
}

func (r *Runner) runDeterministicPhases(ctx context.Context) error {
	advanceable := r.clock.(Advanceable)
	if err := r.drainDeterministicPhases(ctx); err != nil {
		return err
	}
	for i := 0; i < r.config.Iterations; i++ {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		advanceable.Advance(r.config.Step)
		if err := r.drainDeterministicPhases(ctx); err != nil {
			return err
		}
		if r.config.OnProgress != nil && r.config.ProgressEvery > 0 && (i+1)%r.config.ProgressEvery == 0 {
			r.config.OnProgress(i+1, r.config.Iterations)
		}
	}
	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if r.config.DeterministicPhases {
		if err := r.prepareDeterministicPhases(); err != nil {
			return err
		}
	}

	for _, a := range r.actors {
		if err := a.Start(ctx); err != nil {
			return err
		}
	}

	if r.config.DeterministicPhases {
		err := r.runDeterministicPhases(ctx)
		for _, a := range r.actors {
			a.Stop()
		}
		if r.shutdownHook != nil {
			r.shutdownHook()
		}
		for _, m := range r.mounts {
			m.Shutdown()
		}
		return err
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
