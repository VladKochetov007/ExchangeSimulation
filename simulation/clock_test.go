package simulation

import (
	"testing"
	"time"
)

func TestRealClock(t *testing.T) {
	clock := &RealClock{}
	before := time.Now().UnixNano()
	ts := clock.NowUnixNano()
	after := time.Now().UnixNano()

	if ts < before || ts > after {
		t.Errorf("RealClock.NowUnixNano() returned timestamp outside expected range")
	}

	unixBefore := time.Now().Unix()
	unixTs := clock.NowUnix()
	unixAfter := time.Now().Unix()

	if unixTs < unixBefore || unixTs > unixAfter {
		t.Errorf("RealClock.NowUnix() returned timestamp outside expected range")
	}
}

func TestSimulatedClock(t *testing.T) {
	start := int64(1000000000000000000)
	clock := NewSimulatedClock(start)

	if clock.NowUnixNano() != start {
		t.Errorf("Expected initial time %d, got %d", start, clock.NowUnixNano())
	}

	expectedUnix := start / 1e9
	if clock.NowUnix() != expectedUnix {
		t.Errorf("Expected Unix time %d, got %d", expectedUnix, clock.NowUnix())
	}
}

func TestSimulatedClockAdvance(t *testing.T) {
	start := int64(1000000000000000000)
	clock := NewSimulatedClock(start)

	delta := time.Second
	clock.Advance(delta)

	expected := start + int64(delta)
	if clock.NowUnixNano() != expected {
		t.Errorf("Expected time %d after advance, got %d", expected, clock.NowUnixNano())
	}
}

func TestSimulatedClockSetTime(t *testing.T) {
	clock := NewSimulatedClock(0)
	newTime := int64(5000000000000000000)
	clock.SetTime(newTime)

	if clock.NowUnixNano() != newTime {
		t.Errorf("Expected time %d after SetTime, got %d", newTime, clock.NowUnixNano())
	}
}
