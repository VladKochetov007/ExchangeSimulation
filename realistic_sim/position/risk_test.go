package position

import (
	"exchange_sim/exchange"
	"testing"
	"time"
)

func TestDrawdownFilter(t *testing.T) {
	df := NewDrawdownFilter(500, 1*time.Minute, 30*time.Second)

	now := time.Now()
	df.CheckDrawdown(10000, now)
	df.CheckDrawdown(9000, now.Add(10*time.Second))
	df.CheckDrawdown(8500, now.Add(20*time.Second))

	drawdownBps := (10000 - 8500) * 10000 / 10000
	if drawdownBps <= 500 {
		t.Error("Test setup error: drawdown should exceed 500 bps")
	}

	order := &exchange.Order{
		Side: exchange.Buy,
		Qty:  100,
	}

	if !df.ShouldBlock(order) {
		t.Error("DrawdownFilter should block new positions during penalty period")
	}

	df.blockedUntil = now.Add(-1 * time.Second)
	if df.ShouldBlock(order) {
		t.Error("DrawdownFilter should not block after penalty period expires")
	}
}

func TestDrawdownFilterBelowThreshold(t *testing.T) {
	df := NewDrawdownFilter(500, 1*time.Minute, 30*time.Second)

	now := time.Now()
	df.CheckDrawdown(10000, now)
	df.CheckDrawdown(9950, now.Add(10*time.Second))

	order := &exchange.Order{
		Side: exchange.Buy,
		Qty:  100,
	}

	if df.ShouldBlock(order) {
		t.Error("DrawdownFilter should not block when drawdown is below threshold")
	}
}

func TestPositionFlipFilter(t *testing.T) {
	pf := NewPositionFlipFilter()
	pf.UpdatePosition(100)

	buyOrder := &exchange.Order{
		Side: exchange.Buy,
		Qty:  50,
	}
	if pf.ShouldBlock(buyOrder) {
		t.Error("PositionFlipFilter should not block same-direction orders")
	}

	sellOrder := &exchange.Order{
		Side: exchange.Sell,
		Qty:  150,
	}
	if !pf.ShouldBlock(sellOrder) {
		t.Error("PositionFlipFilter should block orders that flip position sign")
	}

	smallSellOrder := &exchange.Order{
		Side: exchange.Sell,
		Qty:  50,
	}
	if pf.ShouldBlock(smallSellOrder) {
		t.Error("PositionFlipFilter should not block reducing orders")
	}
}

func TestPositionFlipFilterFromZero(t *testing.T) {
	pf := NewPositionFlipFilter()
	pf.UpdatePosition(0)

	order := &exchange.Order{
		Side: exchange.Buy,
		Qty:  100,
	}
	if pf.ShouldBlock(order) {
		t.Error("PositionFlipFilter should not block from zero position")
	}
}

func TestCompositeFilter(t *testing.T) {
	df := NewDrawdownFilter(500, 1*time.Minute, 30*time.Second)
	pf := NewPositionFlipFilter()

	cf := NewCompositeFilter(df, pf)

	now := time.Now()
	df.CheckDrawdown(10000, now)
	df.CheckDrawdown(8000, now.Add(10*time.Second))

	order := &exchange.Order{
		Side: exchange.Buy,
		Qty:  100,
	}

	if !cf.ShouldBlock(order) {
		t.Error("CompositeFilter should block if any filter blocks")
	}

	df.blockedUntil = now.Add(-1 * time.Second)
	if cf.ShouldBlock(order) {
		t.Error("CompositeFilter should not block if no filter blocks")
	}
}
