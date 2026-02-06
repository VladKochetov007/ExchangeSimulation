package math

import (
	"testing"
)

func TestRollingVolatility(t *testing.T) {
	scale := int64(10000)
	rv := NewRollingVolatility(10, scale)

	prices := []int64{10000, 10100, 10200, 10150, 10050, 10100, 10200, 10300, 10250, 10200}

	for _, price := range prices {
		rv.AddPrice(price)
	}

	vol := rv.Volatility()
	if vol < 0 {
		t.Errorf("Volatility should be non-negative, got %d", vol)
	}

	if rv.Size() != 9 {
		t.Errorf("Expected 9 returns (10 prices), got %d", rv.Size())
	}
}

func TestRollingVolatilityEmpty(t *testing.T) {
	rv := NewRollingVolatility(10, 10000)

	vol := rv.Volatility()
	if vol != 0 {
		t.Errorf("Volatility of empty buffer should be 0, got %d", vol)
	}
}

func TestRollingVolatilityConstantPrices(t *testing.T) {
	rv := NewRollingVolatility(5, 10000)

	for i := 0; i < 10; i++ {
		rv.AddPrice(10000)
	}

	vol := rv.Volatility()
	if vol != 0 {
		t.Errorf("Volatility of constant prices should be 0, got %d", vol)
	}
}
