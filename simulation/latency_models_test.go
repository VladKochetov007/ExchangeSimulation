package simulation

import (
	"math"
	"sort"
	"testing"
	"time"
)

func sampleDelays(provider LatencyProvider, n int) []time.Duration {
	delays := make([]time.Duration, n)
	for i := range delays {
		delays[i] = provider.Delay()
	}
	sort.Slice(delays, func(i, j int) bool { return delays[i] < delays[j] })
	return delays
}

// The reason to draw lognormal rather than normal is the tail: the median has
// to land where it was configured while the top percentile runs far past it.
func TestLognormalLatencyKeepsItsMedianAndHasATail(t *testing.T) {
	provider := NewLognormalLatency(time.Millisecond, 1.0, 0, 7)
	delays := sampleDelays(provider, 20_000)
	median := delays[len(delays)/2]
	if ratio := float64(median) / float64(time.Millisecond); ratio < 0.9 || ratio > 1.1 {
		t.Errorf("median = %v, want about 1ms", median)
	}
	p999 := delays[len(delays)*999/1000]
	if float64(p999) < 8*float64(median) {
		t.Errorf("p99.9 = %v against a median of %v, which is not a tail", p999, median)
	}
	normal := sampleDelays(NewNormalLatency(time.Millisecond, time.Millisecond/4, 7), 20_000)
	if float64(p999) <= float64(normal[len(normal)*999/1000]) {
		t.Error("the lognormal tail is no heavier than a normal one, which defeats the purpose")
	}
}

func TestLognormalLatencyTruncatesAtItsCap(t *testing.T) {
	cap := 2 * time.Millisecond
	provider := NewLognormalLatency(time.Millisecond, 1.5, cap, 11)
	for _, delay := range sampleDelays(provider, 5_000) {
		if delay > cap {
			t.Fatalf("delay %v exceeded the cap %v", delay, cap)
		}
	}
}

func TestLognormalLatencyDegeneratesSafely(t *testing.T) {
	if got := NewLognormalLatency(0, 1, 0, 1).Delay(); got != 0 {
		t.Errorf("delay = %v, want 0 for a zero median", got)
	}
	if got := NewLognormalLatency(time.Millisecond, 0, 0, 1).Delay(); got != time.Millisecond {
		t.Errorf("delay = %v, want the median for a zero sigma", got)
	}
}

// A fast link that stalls occasionally is not the same participant as one with
// a slightly wider average, and the spike has to be rare and large.
func TestSpikyLatencyIsFastExceptWhenItIsNot(t *testing.T) {
	provider := NewSpikyLatency(
		NewConstantLatency(time.Millisecond),
		NewConstantLatency(200*time.Millisecond),
		0.01, 5,
	)
	spikes := 0
	const samples = 100_000
	for i := 0; i < samples; i++ {
		if provider.Delay() > 100*time.Millisecond {
			spikes++
		}
	}
	rate := float64(spikes) / samples
	if math.Abs(rate-0.01) > 0.002 {
		t.Errorf("spike rate = %.4f, want about 0.01", rate)
	}
}

func TestSpikyLatencyWithoutASpikeIsItsBase(t *testing.T) {
	provider := NewSpikyLatency(NewConstantLatency(time.Millisecond), nil, 0.5, 3)
	for i := 0; i < 100; i++ {
		if got := provider.Delay(); got != time.Millisecond {
			t.Fatalf("delay = %v, want the base 1ms", got)
		}
	}
}
