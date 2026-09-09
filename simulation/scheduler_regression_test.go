package simulation

import (
	"errors"
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

	var observedMu sync.Mutex
	var observed []int64
	sched.Schedule(int64(10*time.Millisecond), func() {
		observedMu.Lock()
		observed = append(observed, clk.NowUnixNano())
		observedMu.Unlock()
	})
	sched.Schedule(int64(20*time.Millisecond), func() {
		observedMu.Lock()
		observed = append(observed, clk.NowUnixNano())
		observedMu.Unlock()
	})

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
	observedMu.Lock()
	defer observedMu.Unlock()
	if got, want := observed, []int64{int64(10 * time.Millisecond), int64(20 * time.Millisecond)}; !equalInt64s(got, want) {
		t.Fatalf("concurrent advance callback times = %v, want %v", got, want)
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

func TestTimestampHookDrainsSameTimestampInsertionsBeforeReturning(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	var fired []string
	hookCalls := 0
	scheduler.Schedule(500, func() {
		fired = append(fired, "outer")
		scheduler.Schedule(500, func() { fired = append(fired, "inserted") })
	})
	if err := clock.AdvanceWithTimestampHook(time.Second, func() error {
		hookCalls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := fired, []string{"outer", "inserted"}; !equalStrings(got, want) {
		t.Fatalf("same-timestamp events = %v, want %v", got, want)
	}
	if hookCalls != 1 {
		t.Fatalf("timestamp hook calls = %d, want 1", hookCalls)
	}
}

func TestTimestampHookInvokesAtAdvanceEndpoint(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	var fired []string
	hookCalls := 0
	scheduler.Schedule(int64(time.Second), func() { fired = append(fired, "scheduled") })
	if err := clock.AdvanceWithTimestampHook(time.Second, func() error {
		hookCalls++
		if hookCalls == 1 {
			scheduler.Schedule(int64(time.Second), func() { fired = append(fired, "hook-inserted") })
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, want := fired, []string{"scheduled", "hook-inserted"}; !equalStrings(got, want) {
		t.Fatalf("endpoint events = %v, want %v", got, want)
	}
	if hookCalls != 2 {
		t.Fatalf("timestamp hook calls at step endpoint = %d, want 2", hookCalls)
	}
	if got := clock.NowUnixNano(); got != int64(time.Second) {
		t.Fatalf("clock after endpoint hook = %d, want %d", got, int64(time.Second))
	}
}

func TestTimestampHookErrorLeavesClockAtFailingTimestamp(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	var observed int64
	scheduler.Schedule(500, func() { return })
	scheduler.Schedule(600, func() { observed = clock.NowUnixNano() })
	wantErr := errors.New("phase failure")
	if err := clock.AdvanceWithTimestampHook(time.Second, func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("AdvanceWithTimestampHook error = %v, want %v", err, wantErr)
	}
	if got := clock.NowUnixNano(); got != 500 {
		t.Fatalf("clock after hook error = %d, want 500", got)
	}
	clock.Advance(100)
	if observed != 600 {
		t.Fatalf("retried event observed at %d, want 600", observed)
	}
}

func TestTimestampHookRejectsPastDueEventInsertion(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	scheduler.Schedule(500, func() {
		scheduler.Schedule(400, func() {})
	})
	hookCalls := 0
	if err := clock.AdvanceWithTimestampHook(time.Second, func() error {
		hookCalls++
		return nil
	}); err == nil {
		t.Fatal("past-due scheduler insertion was accepted")
	}
	if got := clock.NowUnixNano(); got != 500 {
		t.Fatalf("clock after past-due rejection = %d, want 500", got)
	}
	if hookCalls != 0 {
		t.Fatalf("timestamp hook ran after past-due insertion %d times, want 0", hookCalls)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalInt64s(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
