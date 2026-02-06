package math

import (
	"math"
	"testing"
)

func TestIntegerLog(t *testing.T) {
	tests := []struct {
		x        int64
		scale    int64
		expected float64
	}{
		{10000, 10000, math.Log(1.0)},
		{20000, 10000, math.Log(2.0)},
		{27183, 10000, math.Log(2.7183)},
		{100000, 10000, math.Log(10.0)},
		{1000000, 10000, math.Log(100.0)},
		{10, 10000, math.Log(0.001)},
		{100, 10000, math.Log(0.01)},
		{1000, 10000, math.Log(0.1)},
	}

	for _, tt := range tests {
		result := IntegerLog(tt.x, tt.scale)
		expected := int64(tt.expected * float64(tt.scale))
		diff := result - expected
		if diff < 0 {
			diff = -diff
		}

		tolerance := int64(100)
		if diff > tolerance {
			t.Errorf("IntegerLog(%d, %d) = %d, expected %d (diff %d > tolerance %d)",
				tt.x, tt.scale, result, expected, diff, tolerance)
		}
	}
}

func TestIntegerLogEdgeCases(t *testing.T) {
	if result := IntegerLog(0, 10000); result != 0 {
		t.Errorf("IntegerLog(0) should return 0, got %d", result)
	}

	if result := IntegerLog(-100, 10000); result != 0 {
		t.Errorf("IntegerLog(negative) should return 0, got %d", result)
	}

	if result := IntegerLog(10000, 10000); result != 0 {
		t.Errorf("IntegerLog(scale, scale) should return 0 (ln(1)=0), got %d", result)
	}
}

func TestIntegerLogAccuracy(t *testing.T) {
	scale := int64(10000)

	for x := int64(1000); x <= 1000000; x += 10000 {
		result := IntegerLog(x, scale)
		expected := int64(math.Log(float64(x)/float64(scale)) * float64(scale))
		diff := result - expected
		if diff < 0 {
			diff = -diff
		}

		percentError := float64(diff) / float64(expected+1) * 100
		if percentError > 2.0 {
			t.Errorf("IntegerLog(%d) accuracy error %.2f%% > 2.0%%", x, percentError)
		}
	}
}
