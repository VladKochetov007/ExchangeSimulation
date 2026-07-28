package exchange_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"exchange_sim/simulation"
)

// After processing events that fired before the target, Advance must still
// rest the clock at the requested target time; events past the target stay
// queued and later fire at their own instant.
func TestRegressionAdvanceRestsAtTargetAfterEarlierEvents(t *testing.T) {
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	var earlyAt, lateAt int64 = -1, -1
	scheduler.Schedule(int64(time.Millisecond), func() { earlyAt = clock.NowUnixNano() })
	scheduler.Schedule(int64(20*time.Millisecond), func() { lateAt = clock.NowUnixNano() })

	clock.Advance(10 * time.Millisecond)
	if earlyAt != int64(time.Millisecond) {
		t.Fatalf("callback at 1ms observed %dns", earlyAt)
	}
	if lateAt != -1 {
		t.Fatalf("event scheduled at 20ms fired during an advance to 10ms (at %dns)", lateAt)
	}
	if now := clock.NowUnixNano(); now != int64(10*time.Millisecond) {
		t.Fatalf("clock rested at %dns after Advance, want the 10ms target", now)
	}

	clock.Advance(15 * time.Millisecond)
	if lateAt != int64(20*time.Millisecond) {
		t.Fatalf("callback at 20ms observed %dns", lateAt)
	}
	if now := clock.NowUnixNano(); now != int64(25*time.Millisecond) {
		t.Fatalf("clock rested at %dns after second Advance, want 25ms", now)
	}
}

// Each tick of a repeating event must observe its own scheduled instant, not
// the end of the enclosing Advance jump.
func TestRegressionRepeatingCallbacksObserveSuccessiveInstants(t *testing.T) {
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	var ticks []int64
	scheduler.ScheduleRepeating(int64(time.Millisecond), func() {
		ticks = append(ticks, clock.NowUnixNano())
	})

	clock.Advance(3 * time.Millisecond)

	want := []int64{int64(time.Millisecond), int64(2 * time.Millisecond), int64(3 * time.Millisecond)}
	if len(ticks) != len(want) {
		t.Fatalf("got %d ticks %v, want %d", len(ticks), ticks, len(want))
	}
	for i := range want {
		if ticks[i] != want[i] {
			t.Fatalf("tick %d observed %dns, want %dns (all ticks: %v)", i, ticks[i], want[i], ticks)
		}
	}
}

// An event whose scheduled time is already in the past fires at the current
// time; it must never rewind the clock.
func TestRegressionPastDueEventDoesNotRewindClock(t *testing.T) {
	clock := simulation.NewSimulatedClock(int64(5 * time.Millisecond))
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	var observed int64 = -1
	scheduler.Schedule(int64(time.Millisecond), func() { observed = clock.NowUnixNano() })

	clock.Advance(5 * time.Millisecond)

	if observed != int64(5*time.Millisecond) {
		t.Fatalf("past-due callback observed %dns, want the 5ms present (no rewind)", observed)
	}
	if now := clock.NowUnixNano(); now != int64(10*time.Millisecond) {
		t.Fatalf("clock rested at %dns, want the 10ms target", now)
	}
}

// A scheduler constructed without a clock still fires due events; the
// clock-sync step is skipped rather than dereferencing nil.
func TestRegressionProcessUntilWithNilClockFiresEvents(t *testing.T) {
	scheduler := simulation.NewEventScheduler(nil)

	fired := false
	scheduler.Schedule(1, func() { fired = true })

	scheduler.ProcessUntil(10)

	if !fired {
		t.Fatal("due event did not fire with a nil clock")
	}
}

// Schedule races against Advance/ProcessUntil on the shared heap and clock.
// Run with -race: every concurrently scheduled event must eventually fire
// exactly once, and observed clock time must never move backwards.
func TestRegressionSchedulerConcurrentScheduleDuringAdvance(t *testing.T) {
	clock := simulation.NewSimulatedClock(0)
	scheduler := simulation.NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	const producers = 4
	const eventsPerProducer = 200
	var fired atomic.Int64
	var monotonicViolation atomic.Bool
	stopReader := make(chan struct{})

	var readerWG sync.WaitGroup
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		last := int64(0)
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			now := clock.NowUnixNano()
			if now < last {
				monotonicViolation.Store(true)
				return
			}
			last = now
		}
	}()

	var producerWG sync.WaitGroup
	for p := 0; p < producers; p++ {
		producerWG.Add(1)
		go func(offset int64) {
			defer producerWG.Done()
			for i := int64(0); i < eventsPerProducer; i++ {
				scheduler.Schedule(offset+i*int64(time.Microsecond), func() { fired.Add(1) })
			}
		}(int64(p) * int64(time.Millisecond))
	}

	for step := 0; step < 50; step++ {
		clock.Advance(time.Millisecond)
	}
	producerWG.Wait()
	clock.Advance(time.Second)

	close(stopReader)
	readerWG.Wait()

	if monotonicViolation.Load() {
		t.Fatal("clock time moved backwards during concurrent Schedule/Advance")
	}
	if got := fired.Load(); got != producers*eventsPerProducer {
		t.Fatalf("fired %d of %d events scheduled concurrently with Advance", got, producers*eventsPerProducer)
	}
	wantFinal := int64(50*time.Millisecond) + int64(time.Second)
	if now := clock.NowUnixNano(); now != wantFinal {
		t.Fatalf("clock rested at %dns, want %dns", now, wantFinal)
	}
}
