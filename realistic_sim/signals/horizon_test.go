package signals

import (
	"testing"

	"exchange_sim/exchange"
)

func TestPriceHistoryBasic(t *testing.T) {
	ph := NewPriceHistory(10, 10000)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	for i := 0; i < 10; i++ {
		price := basePrice + int64(i)*1000*exchange.USD_PRECISION
		ph.AddPrice(price)
	}

	if !ph.IsReady() {
		t.Error("Price history should be ready after 10 samples")
	}

	ret := ph.GetReturn()
	if ret == 0 {
		t.Error("Return should be non-zero with increasing prices")
	}

	t.Logf("Return over 10 samples: %d", ret)
}

func TestPriceHistoryNotReady(t *testing.T) {
	ph := NewPriceHistory(10, 10000)

	for i := 0; i < 5; i++ {
		ph.AddPrice(50000 * exchange.USD_PRECISION)
	}

	if ph.IsReady() {
		t.Error("Price history should not be ready with only 5 samples")
	}

	ret := ph.GetReturn()
	if ret != 0 {
		t.Error("Return should be 0 when not ready")
	}
}

func TestPriceHistoryRolling(t *testing.T) {
	ph := NewPriceHistory(5, 10000)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	for i := 0; i < 5; i++ {
		ph.AddPrice(basePrice + int64(i)*1000*exchange.USD_PRECISION)
	}

	ret1 := ph.GetReturn()

	ph.AddPrice(basePrice + 10000*exchange.USD_PRECISION)
	ret2 := ph.GetReturn()

	if ret2 <= ret1 {
		t.Errorf("Return should increase with larger price jump: ret1=%d, ret2=%d", ret1, ret2)
	}
}
