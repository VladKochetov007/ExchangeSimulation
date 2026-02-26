package simulation

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventSchedulerFiresEventsInOrder(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	fired := []int{}

	scheduler.Schedule(300*int64(time.Millisecond), func() { fired = append(fired, 3) })
	scheduler.Schedule(100*int64(time.Millisecond), func() { fired = append(fired, 1) })
	scheduler.Schedule(200*int64(time.Millisecond), func() { fired = append(fired, 2) })

	clock.Advance(1 * time.Second)

	if len(fired) != 3 {
		t.Fatalf("expected 3 events, got %d", len(fired))
	}
	if fired[0] != 1 || fired[1] != 2 || fired[2] != 3 {
		t.Errorf("events fired out of order: %v", fired)
	}
}

func TestEventSchedulerRepeating(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	count := 0
	scheduler.ScheduleRepeating(100*int64(time.Millisecond), func() { count++ })

	clock.Advance(250 * time.Millisecond)

	if count != 2 {
		t.Errorf("expected 2 ticks, got %d", count)
	}

	clock.Advance(150 * time.Millisecond)

	if count != 4 {
		t.Errorf("expected 4 ticks after more time, got %d", count)
	}
}

func TestEventSchedulerCancel(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	fired := false
	id := scheduler.Schedule(100*int64(time.Millisecond), func() { fired = true })

	scheduler.Cancel(id)
	clock.Advance(200 * time.Millisecond)

	if fired {
		t.Error("event fired after being cancelled")
	}
}

func TestEventSchedulerCancelRepeating(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	count := 0
	id := scheduler.ScheduleRepeating(100*int64(time.Millisecond), func() { count++ })

	clock.Advance(250 * time.Millisecond)
	if count != 2 {
		t.Fatalf("expected 2 ticks before cancel, got %d", count)
	}

	scheduler.Cancel(id)
	clock.Advance(300 * time.Millisecond)

	if count != 2 {
		t.Errorf("expected no more ticks after cancel, got %d", count)
	}
}

func TestSimTickerBasicOperation(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	factory := NewSimTimerFactory(scheduler)

	ticker := factory.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	ticks := 0
	done := make(chan struct{})

	go func() {
		for i := 0; i < 5; i++ {
			<-ticker.C()
			ticks++
		}
		close(done)
	}()

	// Advance time in steps
	for i := 0; i < 10; i++ {
		clock.Advance(50 * time.Millisecond)
		time.Sleep(time.Millisecond) // Let goroutine run
	}

	<-done
	if ticks != 5 {
		t.Errorf("expected 5 ticks, got %d", ticks)
	}
}

func TestSimTickerStop(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	factory := NewSimTimerFactory(scheduler)

	ticker := factory.NewTicker(100 * time.Millisecond)

	ticks := 0
	stopped := make(chan struct{})

	go func() {
		for range ticker.C() {
			ticks++
			if ticks >= 3 {
				ticker.Stop()
				close(stopped)
				return
			}
		}
	}()

	// Advance time
	for i := 0; i < 10; i++ {
		clock.Advance(50 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}

	<-stopped

	// Verify no more ticks after stop
	initialTicks := ticks
	clock.Advance(500 * time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	if ticks != initialTicks {
		t.Errorf("ticker continued after stop: %d -> %d", initialTicks, ticks)
	}
}

func TestMultipleTickersIndependent(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	factory := NewSimTimerFactory(scheduler)

	ticker1 := factory.NewTicker(100 * time.Millisecond)
	ticker2 := factory.NewTicker(250 * time.Millisecond)
	defer ticker1.Stop()
	defer ticker2.Stop()

	var ticks1, ticks2 atomic.Int64
	done := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ticker1.C():
				ticks1.Add(1)
			case <-done:
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ticker2.C():
				ticks2.Add(1)
			case <-done:
				return
			}
		}
	}()

	// Advance in smaller steps to let goroutines process
	for i := 0; i < 50; i++ {
		clock.Advance(10 * time.Millisecond)
		time.Sleep(time.Millisecond)
	}
	close(done)
	wg.Wait()

	if got := ticks1.Load(); got != 5 {
		t.Errorf("ticker1: expected 5 ticks, got %d", got)
	}
	if got := ticks2.Load(); got != 2 {
		t.Errorf("ticker2: expected 2 ticks, got %d", got)
	}
}

func TestSchedulerWithZeroTime(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	fired := false
	scheduler.Schedule(0, func() { fired = true })

	clock.Advance(1 * time.Nanosecond)

	if !fired {
		t.Error("event at time 0 did not fire")
	}
}

func TestSchedulerLargeTimeJump(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)

	count := 0
	scheduler.ScheduleRepeating(1*int64(time.Second), func() { count++ })

	// Jump 1 hour forward
	clock.Advance(1 * time.Hour)

	if count != 3600 {
		t.Errorf("expected 3600 ticks for 1 hour, got %d", count)
	}
}
