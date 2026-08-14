package simulation

import (
	"sync"
	"testing"
	"time"
)

// Cancelling a repeating event from inside its own callback (or concurrently
// while it is mid-fire) must stop it: the event is out of the heap at that
// moment, so Cancel can only flag it, and the re-push must honor the flag.
func TestRegressionCancelRepeatingWhileFiring(t *testing.T) {
	clk := NewSimulatedClock(0)
	sched := NewEventScheduler(clk)
	clk.SetScheduler(sched)

	fires := 0
	var id uint64
	id = sched.ScheduleRepeating(10, func() {
		fires++
		sched.Cancel(id)
	})

	clk.Advance(100 * time.Nanosecond)
	if fires != 1 {
		t.Fatalf("self-cancelled repeating event fired %d times, want 1", fires)
	}
}

// A non-positive interval must not hang ProcessUntil in an infinite loop.
func TestRegressionZeroIntervalRepeatingIsClamped(t *testing.T) {
	clk := NewSimulatedClock(0)
	sched := NewEventScheduler(clk)
	clk.SetScheduler(sched)

	fires := 0
	sched.ScheduleRepeating(0, func() { fires++ })

	done := make(chan struct{})
	go func() {
		clk.Advance(10 * time.Nanosecond)
		close(done)
	}()
	select {
	case <-done:
		if fires == 0 || fires > 11 {
			t.Fatalf("clamped zero-interval event fired %d times over 10ns, want 1..11", fires)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessUntil hung on zero-interval repeating event")
	}
}

// Concurrent Advance calls must compose additively: each gets its own time
// window, matching the old current += delta semantics.
func TestRegressionConcurrentAdvanceIsAdditive(t *testing.T) {
	clk := NewSimulatedClock(0)
	sched := NewEventScheduler(clk)
	clk.SetScheduler(sched)

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clk.Advance(10 * time.Millisecond)
		}()
	}
	wg.Wait()

	if got := clk.NowUnixNano(); got != int64(20*time.Millisecond) {
		t.Fatalf("two concurrent Advance(10ms) ended at %dns, want %dns", got, int64(20*time.Millisecond))
	}
}

// Stop racing a concurrently advancing clock must not panic (send on closed
// channel) and must actually silence the ticker. Run with -race.
func TestRegressionTickerStopConcurrentWithAdvance(t *testing.T) {
	clk := NewSimulatedClock(0)
	sched := NewEventScheduler(clk)
	clk.SetScheduler(sched)
	factory := NewSimTimerFactory(sched)

	ticker := factory.NewTicker(time.Microsecond)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 500 {
			clk.Advance(2 * time.Microsecond)
		}
	}()

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			select {
			case <-ticker.C():
			case <-done:
				return
			}
		}
	}()

	time.Sleep(time.Millisecond)
	ticker.Stop()
	<-done
	<-drained

	for len(ticker.C()) > 0 {
		<-ticker.C()
	}
	clk.Advance(10 * time.Microsecond)
	if len(ticker.C()) != 0 {
		t.Fatal("stopped ticker delivered another tick")
	}
}

func TestSimTimerFactoryWaitsForTickAcknowledgement(t *testing.T) {
	clk := NewSimulatedClock(0)
	sched := NewEventScheduler(clk)
	clk.SetScheduler(sched)
	factory := NewSimTimerFactory(sched)

	ticker := factory.NewTicker(time.Millisecond)
	defer ticker.Stop()

	clk.Advance(time.Millisecond)
	if factory.Idle() {
		t.Fatal("factory reported idle with a delivered tick")
	}
	<-ticker.C()
	if factory.Idle() {
		t.Fatal("factory reported idle after a tick was received but before processing completed")
	}

	acknowledger, ok := ticker.(interface{ Acknowledge() })
	if !ok {
		t.Fatal("simulation ticker does not expose acknowledgement")
	}
	acknowledger.Acknowledge()
	if !factory.Idle() {
		t.Fatal("factory remained non-idle after the delivered tick was acknowledged")
	}
}

func TestRegressionNewTickerPanicsOnNonPositiveInterval(t *testing.T) {
	clk := NewSimulatedClock(0)
	sched := NewEventScheduler(clk)
	clk.SetScheduler(sched)
	factory := NewSimTimerFactory(sched)

	defer func() {
		if recover() == nil {
			t.Fatal("NewTicker(0) did not panic; a zero-interval ticker hangs the simulation")
		}
	}()
	factory.NewTicker(0)
}
