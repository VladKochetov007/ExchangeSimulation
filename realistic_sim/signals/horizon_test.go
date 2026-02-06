package signals

import (
	"testing"
	"time"

	"exchange_sim/exchange"
	simmath "exchange_sim/realistic_sim/math"
)

func TestPriceHistoryBasic(t *testing.T) {
	ph := NewPriceHistory(30*time.Second, 10000)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()

	for i := 0; i < 60; i++ {
		timestamp := baseTime + int64(i)*int64(time.Second)
		price := basePrice + int64(i)*1000*exchange.USD_PRECISION
		ph.AddPrice(price, timestamp)
	}

	currentTime := baseTime + 60*int64(time.Second)

	if !ph.IsReady(currentTime) {
		t.Error("Price history should be ready after 60 seconds of data")
	}

	ret := ph.GetReturn(currentTime)
	if ret == 0 {
		t.Error("Return should be non-zero with increasing prices")
	}

	t.Logf("Return over 30s lookback: %d", ret)
}

func TestPriceHistoryNotReady(t *testing.T) {
	ph := NewPriceHistory(30*time.Second, 10000)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()

	for i := 0; i < 15; i++ {
		timestamp := baseTime + int64(i)*int64(time.Second)
		ph.AddPrice(50000*exchange.USD_PRECISION, timestamp)
	}

	currentTime := baseTime + 15*int64(time.Second)

	if ph.IsReady(currentTime) {
		t.Error("Price history should not be ready with only 15 seconds of data")
	}

	ret := ph.GetReturn(currentTime)
	if ret != 0 {
		t.Error("Return should be 0 when not ready")
	}
}

func TestPriceHistoryTimeWindow(t *testing.T) {
	ph := NewPriceHistory(10*time.Second, 10000)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()

	ph.AddPrice(basePrice, baseTime)
	ph.AddPrice(basePrice+1000*exchange.USD_PRECISION, baseTime+5*int64(time.Second))
	ph.AddPrice(basePrice+2000*exchange.USD_PRECISION, baseTime+10*int64(time.Second))
	ph.AddPrice(basePrice+10000*exchange.USD_PRECISION, baseTime+15*int64(time.Second))

	currentTime := baseTime + 15*int64(time.Second)

	ret := ph.GetReturn(currentTime)

	t.Logf("Return from t=5s to t=15s: %d", ret)

	if ret == 0 {
		t.Error("Return should be non-zero")
	}
}

func TestPriceHistoryIncreasingPrices(t *testing.T) {
	ph := NewPriceHistory(30*time.Second, 10000)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()

	for i := 0; i < 60; i++ {
		timestamp := baseTime + int64(i)*int64(time.Second)
		price := basePrice + int64(i)*50*exchange.USD_PRECISION
		ph.AddPrice(price, timestamp)
	}

	currentTime := baseTime + 60*int64(time.Second)

	t.Logf("Total points: %d", len(ph.points))
	t.Logf("First point: ts=%d, price=%d", ph.points[0].Timestamp-baseTime, ph.points[0].Price/exchange.USD_PRECISION)
	t.Logf("Last point: ts=%d, price=%d", ph.points[len(ph.points)-1].Timestamp-baseTime, ph.points[len(ph.points)-1].Price/exchange.USD_PRECISION)

	cutoff := currentTime - int64(30*time.Second)
	t.Logf("Cutoff: %d seconds from base", (cutoff-baseTime)/int64(time.Second))

	oldestIdx := -1
	for i := 0; i < len(ph.points); i++ {
		if ph.points[i].Timestamp >= cutoff {
			oldestIdx = i
			break
		}
	}
	oldestPrice := ph.points[oldestIdx].Price
	newestPrice := ph.points[len(ph.points)-1].Price

	t.Logf("OldestIdx found: %d, timestamp=%d, price=%d (raw=%d)",
		oldestIdx,
		(ph.points[oldestIdx].Timestamp-baseTime)/int64(time.Second),
		oldestPrice/exchange.USD_PRECISION,
		oldestPrice)
	t.Logf("Newest: timestamp=%d, price=%d (raw=%d)",
		(ph.points[len(ph.points)-1].Timestamp-baseTime)/int64(time.Second),
		newestPrice/exchange.USD_PRECISION,
		newestPrice)

	ret := ph.GetReturn(currentTime)

	t.Logf("Return over 30s lookback: %d", ret)
	t.Logf("Expected positive return: ln(%d/%d) * 10000", newestPrice, oldestPrice)

	directRet := simmath.LogReturn(newestPrice, oldestPrice, 10000)
	t.Logf("Direct LogReturn call: %d", directRet)

	if ret <= 0 {
		t.Errorf("Return should be positive for increasing prices: got %d", ret)
	}
}
