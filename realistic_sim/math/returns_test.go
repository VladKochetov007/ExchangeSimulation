package math

import (
	"math"
	"testing"
)

func TestLogReturn(t *testing.T) {
	scale := int64(10000)

	tests := []struct {
		price_t         int64
		price_t_minus_1 int64
		expectedReturn  float64
	}{
		{11000, 10000, math.Log(1.1)},
		{10000, 10000, 0.0},
		{9000, 10000, math.Log(0.9)},
		{20000, 10000, math.Log(2.0)},
		{5000, 10000, math.Log(0.5)},
	}

	for _, tt := range tests {
		result := LogReturn(tt.price_t, tt.price_t_minus_1, scale)
		expected := int64(tt.expectedReturn * float64(scale))
		diff := result - expected
		if diff < 0 {
			diff = -diff
		}

		tolerance := int64(300)
		if diff > tolerance {
			t.Errorf("LogReturn(%d, %d) = %d, expected %d (diff %d)",
				tt.price_t, tt.price_t_minus_1, result, expected, diff)
		}
	}
}

func TestLogReturnEdgeCases(t *testing.T) {
	scale := int64(10000)

	if result := LogReturn(10000, 0, scale); result != 0 {
		t.Errorf("LogReturn with zero denominator should return 0, got %d", result)
	}

	if result := LogReturn(10000, 10000, scale); result != 0 {
		t.Errorf("LogReturn(x, x) should return 0, got %d", result)
	}
}
