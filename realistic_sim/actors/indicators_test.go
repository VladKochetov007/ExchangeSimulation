package actors

import (
	"testing"
)

func TestCircularBuffer_Add(t *testing.T) {
	cb := NewCircularBuffer(3)

	if cb.Count() != 0 {
		t.Errorf("Expected count 0, got %d", cb.Count())
	}

	cb.Add(10)
	if cb.Count() != 1 {
		t.Errorf("Expected count 1, got %d", cb.Count())
	}

	cb.Add(20)
	cb.Add(30)
	if cb.Count() != 3 {
		t.Errorf("Expected count 3, got %d", cb.Count())
	}
	if !cb.IsFull() {
		t.Error("Expected buffer to be full")
	}

	// Overflow - should wrap around
	cb.Add(40)
	if cb.Count() != 3 {
		t.Errorf("Expected count to remain 3, got %d", cb.Count())
	}
}

func TestCircularBuffer_SMA(t *testing.T) {
	tests := []struct {
		name   string
		values []int64
		want   int64
	}{
		{
			name:   "empty buffer",
			values: []int64{},
			want:   0,
		},
		{
			name:   "single value",
			values: []int64{100},
			want:   100,
		},
		{
			name:   "three values",
			values: []int64{10, 20, 30},
			want:   20, // (10+20+30)/3 = 20
		},
		{
			name:   "overflow wrap",
			values: []int64{10, 20, 30, 40}, // Buffer size 3, last 3 are 20,30,40
			want:   30,                      // (20+30+40)/3 = 30
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircularBuffer(3)
			for _, v := range tt.values {
				cb.Add(v)
			}
			got := cb.SMA()
			if got != tt.want {
				t.Errorf("SMA() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCircularBuffer_Get(t *testing.T) {
	cb := NewCircularBuffer(4)
	cb.Add(10)
	cb.Add(20)
	cb.Add(30)

	tests := []struct {
		index int
		want  int64
	}{
		{0, 10}, // oldest
		{1, 20},
		{2, 30}, // newest
		{3, 0},  // out of range
		{-1, 0}, // negative
	}

	for _, tt := range tests {
		got := cb.Get(tt.index)
		if got != tt.want {
			t.Errorf("Get(%d) = %d, want %d", tt.index, got, tt.want)
		}
	}
}

func TestCircularBuffer_Latest(t *testing.T) {
	cb := NewCircularBuffer(3)

	if cb.Latest() != 0 {
		t.Error("Expected 0 for empty buffer")
	}

	cb.Add(10)
	if cb.Latest() != 10 {
		t.Errorf("Expected 10, got %d", cb.Latest())
	}

	cb.Add(20)
	if cb.Latest() != 20 {
		t.Errorf("Expected 20, got %d", cb.Latest())
	}
}

func TestCircularBuffer_GetAfterWrap(t *testing.T) {
	cb := NewCircularBuffer(3)
	cb.Add(10)
	cb.Add(20)
	cb.Add(30)
	cb.Add(40) // Wraps around, buffer now contains [20, 30, 40]

	tests := []struct {
		index int
		want  int64
	}{
		{0, 20}, // oldest after wrap
		{1, 30},
		{2, 40}, // newest
	}

	for _, tt := range tests {
		got := cb.Get(tt.index)
		if got != tt.want {
			t.Errorf("Get(%d) after wrap = %d, want %d", tt.index, got, tt.want)
		}
	}
}

func TestCircularBuffer_StdDev(t *testing.T) {
	tests := []struct {
		name   string
		values []int64
		want   int64
	}{
		{
			name:   "empty buffer",
			values: []int64{},
			want:   0,
		},
		{
			name:   "no variance",
			values: []int64{10, 10, 10},
			want:   0,
		},
		{
			name:   "simple variance",
			values: []int64{10, 20, 30},
			want:   8, // stddev ~= 8.16, truncates to 8
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircularBuffer(3)
			for _, v := range tt.values {
				cb.Add(v)
			}
			mean := cb.SMA()
			got := cb.StdDev(mean)
			if got != tt.want {
				t.Errorf("StdDev() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestZScore(t *testing.T) {
	tests := []struct {
		name   string
		value  int64
		mean   int64
		stdDev int64
		want   int64
	}{
		{
			name:   "zero stddev",
			value:  100,
			mean:   100,
			stdDev: 0,
			want:   0,
		},
		{
			name:   "value equals mean",
			value:  100,
			mean:   100,
			stdDev: 10,
			want:   0,
		},
		{
			name:   "one stddev above",
			value:  110,
			mean:   100,
			stdDev: 10,
			want:   10000, // z-score = 1.0 = 10000 scaled
		},
		{
			name:   "two stddev below",
			value:  80,
			mean:   100,
			stdDev: 10,
			want:   -20000, // z-score = -2.0 = -20000 scaled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ZScore(tt.value, tt.mean, tt.stdDev)
			if got != tt.want {
				t.Errorf("ZScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalculateRatio(t *testing.T) {
	tests := []struct {
		name   string
		price1 int64
		price2 int64
		want   int64
	}{
		{
			name:   "equal prices",
			price1: 100,
			price2: 100,
			want:   10000, // ratio = 1.0 = 10000 scaled
		},
		{
			name:   "double price",
			price1: 200,
			price2: 100,
			want:   20000, // ratio = 2.0 = 20000 scaled
		},
		{
			name:   "half price",
			price1: 100,
			price2: 200,
			want:   5000, // ratio = 0.5 = 5000 scaled
		},
		{
			name:   "zero denominator",
			price1: 100,
			price2: 0,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateRatio(tt.price1, tt.price2)
			if got != tt.want {
				t.Errorf("CalculateRatio() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBPSChange(t *testing.T) {
	tests := []struct {
		name     string
		oldPrice int64
		newPrice int64
		want     int64
	}{
		{
			name:     "no change",
			oldPrice: 100,
			newPrice: 100,
			want:     0,
		},
		{
			name:     "1% increase",
			oldPrice: 100,
			newPrice: 101,
			want:     100, // 1% = 100 bps
		},
		{
			name:     "1% decrease",
			oldPrice: 100,
			newPrice: 99,
			want:     -100,
		},
		{
			name:     "10% increase",
			oldPrice: 100,
			newPrice: 110,
			want:     1000, // 10% = 1000 bps
		},
		{
			name:     "zero old price",
			oldPrice: 0,
			newPrice: 100,
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BPSChange(tt.oldPrice, tt.newPrice)
			if got != tt.want {
				t.Errorf("BPSChange() = %d, want %d", got, tt.want)
			}
		})
	}
}
