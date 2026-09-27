package simulation

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestIntermediateDrainProcessesEveryEarlierEventTimeBeforeBoundary(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	var order []string
	scheduler.Schedule(int64(500*time.Microsecond), func() {
		order = append(order, "event-500")
		scheduler.Schedule(int64(750*time.Microsecond), func() {
			order = append(order, "event-750")
		})
	})
	scheduler.Schedule(int64(time.Millisecond), func() {
		order = append(order, "event-1000")
	})
	err := clock.advanceWithIntermediateDrain(time.Millisecond, func(atNano int64) error {
		switch atNano {
		case int64(500 * time.Microsecond):
			order = append(order, "drain-500")
		case int64(750 * time.Microsecond):
			order = append(order, "drain-750")
		default:
			t.Fatalf("unexpected intermediate time %d", atNano)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"event-500", "drain-500", "event-750", "drain-750", "event-1000"}
	if !reflect.DeepEqual(order, want) || clock.NowUnixNano() != int64(time.Millisecond) {
		t.Fatalf("event and phase order = %v at %d, want %v", order, clock.NowUnixNano(), want)
	}
}

func TestIntermediateDrainRevisitsSameTimeEventsScheduledByPhase(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	var order []string
	scheduler.Schedule(int64(500*time.Microsecond), func() { order = append(order, "first-event") })
	drainCount := 0
	err := clock.advanceWithIntermediateDrain(time.Millisecond, func(atNano int64) error {
		drainCount++
		order = append(order, "drain")
		if drainCount == 1 {
			scheduler.Schedule(atNano, func() { order = append(order, "same-time-event") })
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first-event", "drain", "same-time-event", "drain"}) {
		t.Fatalf("same-time scheduler event escaped phase drain: %v", order)
	}
}

func TestIntermediateDrainErrorLeavesLaterEventsUnfired(t *testing.T) {
	clock := NewSimulatedClock(0)
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	scheduler.Schedule(int64(500*time.Microsecond), func() {})
	laterFired := false
	scheduler.Schedule(int64(time.Millisecond), func() { laterFired = true })
	stop := errors.New("intermediate phase failed")
	if err := clock.advanceWithIntermediateDrain(time.Millisecond, func(int64) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("lost intermediate phase error: %v", err)
	}
	if laterFired || clock.NowUnixNano() != int64(500*time.Microsecond) || clock.goal != clock.current {
		t.Fatalf("later event fired or clock advanced after failure: later=%v current=%d goal=%d",
			laterFired, clock.current, clock.goal)
	}
}

func TestIntermediateDrainRejectsClockMutationInsideCallbackOrPhase(t *testing.T) {
	for _, mutateIn := range []string{"scheduler-callback", "phase-drain"} {
		t.Run(mutateIn, func(t *testing.T) {
			clock := NewSimulatedClock(0)
			scheduler := NewEventScheduler(clock)
			clock.SetScheduler(scheduler)
			scheduler.Schedule(int64(500*time.Microsecond), func() {
				if mutateIn == "scheduler-callback" {
					clock.SetTime(int64(750 * time.Microsecond))
				}
			})
			laterFired := false
			scheduler.Schedule(int64(time.Millisecond), func() { laterFired = true })
			err := clock.advanceWithIntermediateDrain(time.Millisecond, func(int64) error {
				if mutateIn == "phase-drain" {
					clock.SetTime(int64(750 * time.Microsecond))
				}
				return nil
			})
			if err == nil || laterFired || clock.goal != clock.current {
				t.Fatalf("clock mutation escaped fail-closed check: err=%v later=%v current=%d goal=%d",
					err, laterFired, clock.current, clock.goal)
			}
		})
	}
}
