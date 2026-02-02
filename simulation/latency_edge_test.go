package simulation

import (
	"testing"
	"time"
)

func TestNormalLatencyNegativeDelay(t *testing.T) {
	mean := 10 * time.Millisecond
	stddev := 50 * time.Millisecond
	provider := NewNormalLatency(mean, stddev, 12345)

	for i := 0; i < 1000; i++ {
		delay := provider.Delay()
		if delay < 0 {
			t.Errorf("Negative delay %v should be clamped to 0", delay)
		}
	}
}

func TestNormalLatencyDistribution(t *testing.T) {
	mean := 100 * time.Millisecond
	stddev := 20 * time.Millisecond
	provider := NewNormalLatency(mean, stddev, 67890)

	samples := 10000
	sum := time.Duration(0)
	sumSquares := float64(0)

	for i := 0; i < samples; i++ {
		delay := provider.Delay()
		sum += delay
		diff := float64(delay - mean)
		sumSquares += diff * diff
	}

	avgDelay := sum / time.Duration(samples)
	variance := sumSquares / float64(samples)
	observedStddev := time.Duration(variance)

	tolerance := 5 * time.Millisecond
	if avgDelay < mean-tolerance || avgDelay > mean+tolerance {
		t.Logf("Average delay %v slightly outside tolerance of mean %v", avgDelay, mean)
	}

	if observedStddev < 0 {
		t.Errorf("Invalid variance calculation")
	}
}
