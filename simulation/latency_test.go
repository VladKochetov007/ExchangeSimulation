package simulation

import (
	"testing"
	"time"
)

func TestConstantLatency(t *testing.T) {
	delay := 100 * time.Millisecond
	provider := NewConstantLatency(delay)

	for i := 0; i < 10; i++ {
		if d := provider.Delay(); d != delay {
			t.Errorf("Expected constant delay %v, got %v", delay, d)
		}
	}
}

func TestUniformRandomLatency(t *testing.T) {
	min := 10 * time.Millisecond
	max := 100 * time.Millisecond
	provider := NewUniformRandomLatency(min, max, 42)

	for i := 0; i < 100; i++ {
		delay := provider.Delay()
		if delay < min || delay > max {
			t.Errorf("Delay %v outside range [%v, %v]", delay, min, max)
		}
	}
}

func TestNormalLatency(t *testing.T) {
	mean := 50 * time.Millisecond
	stddev := 10 * time.Millisecond
	provider := NewNormalLatency(mean, stddev, 42)

	sum := time.Duration(0)
	samples := 1000
	for i := 0; i < samples; i++ {
		delay := provider.Delay()
		if delay < 0 {
			t.Errorf("Negative delay %v", delay)
		}
		sum += delay
	}

	avgDelay := sum / time.Duration(samples)
	tolerance := 5 * time.Millisecond
	if avgDelay < mean-tolerance || avgDelay > mean+tolerance {
		t.Errorf("Average delay %v not close to mean %v (tolerance %v)", avgDelay, mean, tolerance)
	}
}

func TestLogNormalLatency_AlwaysAboveFloor(t *testing.T) {
	min := 50 * time.Microsecond
	provider := NewLogNormalLatency(min, 200*time.Microsecond, 0.5, 42)
	for i := 0; i < 10000; i++ {
		if d := provider.Delay(); d <= min {
			t.Fatalf("delay %v not strictly above floor %v", d, min)
		}
	}
}

func TestLogNormalLatency_MedianMatchesParam(t *testing.T) {
	min := 50 * time.Microsecond
	medianAbove := 200 * time.Microsecond
	provider := NewLogNormalLatency(min, medianAbove, 0.5, 42)

	// For a correct median parameter, ~50% of samples fall below min+medianAbove.
	// Binomial(10000, 0.5) has std≈50; ±5% band is >10σ — essentially impossible to fail.
	n := 10000
	below := 0
	for i := 0; i < n; i++ {
		if provider.Delay() < min+medianAbove {
			below++
		}
	}
	if below < n*45/100 || below > n*55/100 {
		t.Errorf("%d/%d samples below median threshold, want ≈50%%", below, n)
	}
}

func TestLogNormalLatency_MeanExceedsMedian(t *testing.T) {
	min := 50 * time.Microsecond
	medianAbove := 200 * time.Microsecond
	provider := NewLogNormalLatency(min, medianAbove, 0.5, 42)

	n := 10000
	sum := time.Duration(0)
	for i := 0; i < n; i++ {
		sum += provider.Delay()
	}
	mean := sum / time.Duration(n)

	// Log-normal is right-skewed: mean = min + medianAbove*exp(σ²/2) > min + medianAbove.
	if mean <= min+medianAbove {
		t.Errorf("mean %v should exceed min+median %v for right-skewed distribution", mean, min+medianAbove)
	}
}

func TestHawkesLatency_FloorAtRest(t *testing.T) {
	min := 100 * time.Microsecond
	h := NewHawkesLatency(min, 50*time.Microsecond, 10.0)
	// No events recorded; excitation is zero regardless of elapsed time (0 * exp(-β*dt) = 0).
	if d := h.Delay(); d != min {
		t.Errorf("expected minLatency %v before any events, got %v", min, d)
	}
}

func TestHawkesLatency_RisesAfterEvent(t *testing.T) {
	min := 100 * time.Microsecond
	h := NewHawkesLatency(min, 50*time.Microsecond, 10.0)
	h.RecordEvent()
	if d := h.Delay(); d <= min {
		t.Errorf("delay %v should exceed minLatency %v immediately after event", d, min)
	}
}

func TestHawkesLatency_NeverBelowMin(t *testing.T) {
	min := 100 * time.Microsecond
	h := NewHawkesLatency(min, 50*time.Microsecond, 10.0)
	for i := 0; i < 1000; i++ {
		h.RecordEvent()
		if d := h.Delay(); d < min {
			t.Fatalf("delay %v below minLatency %v after %d events", d, min, i+1)
		}
	}
}

func TestHawkesLatency_BurstHigherThanSingle(t *testing.T) {
	min := 100 * time.Microsecond
	jump := 10 * time.Microsecond

	single := NewHawkesLatency(min, jump, 10.0)
	single.RecordEvent()
	singleDelay := single.Delay()

	burst := NewHawkesLatency(min, jump, 10.0)
	for i := 0; i < 10; i++ {
		burst.RecordEvent()
	}
	burstDelay := burst.Delay()

	if burstDelay <= singleDelay {
		t.Errorf("burst delay %v should exceed single-event delay %v", burstDelay, singleDelay)
	}
}

func TestHawkesLatency_DecaysTowardMin(t *testing.T) {
	min := 100 * time.Microsecond
	// β=1000/s → half-life ≈ 0.7ms; after 20ms ≈ 29 half-lives → excitation < 1ns.
	h := NewHawkesLatency(min, 100*time.Microsecond, 1000.0)
	h.RecordEvent()

	if h.Delay() <= min {
		t.Fatal("expected excitation above min immediately after event")
	}

	time.Sleep(20 * time.Millisecond)

	if d := h.Delay(); d != min {
		t.Errorf("delay %v should equal minLatency %v after full decay (got excess %v)", d, min, d-min)
	}
}
