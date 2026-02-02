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
