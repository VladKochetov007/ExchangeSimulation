package actor

import (
	"exchange_sim/exchange"
	"testing"
)

const SATOSHI = 100_000_000

func TestNettingOMSNewPosition(t *testing.T) {
	oms := NewNettingOMS()

	fill := OrderFillEvent{
		Side:      exchange.Buy,
		Price:     50000 * exchange.SATOSHI,
		Qty:       exchange.SATOSHI,
		FeeAmount: 0,
	}

	oms.OnFill("BTC/USD", fill, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", exchange.SATOSHI, pos.Qty)
	}
	if pos.AvgPrice != 50000*exchange.SATOSHI {
		t.Fatalf("Expected avg price %d, got %d", 50000*exchange.SATOSHI, pos.AvgPrice)
	}
}

func TestNettingOMSIncreasePosition(t *testing.T) {
	oms := NewNettingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != 2*exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", 2*exchange.SATOSHI, pos.Qty)
	}

	expectedAvg := int64((50000 + 51000) / 2 * exchange.SATOSHI)
	if pos.AvgPrice != expectedAvg {
		t.Fatalf("Expected avg price %d, got %d", expectedAvg, pos.AvgPrice)
	}
}

func TestNettingOMSReducePosition(t *testing.T) {
	oms := NewNettingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   2 * exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", exchange.SATOSHI, pos.Qty)
	}

	expectedPnL := int64((51000 - 50000) * exchange.SATOSHI)
	if pos.RealizedPnL != expectedPnL {
		t.Fatalf("Expected realized PnL %d, got %d", expectedPnL, pos.RealizedPnL)
	}
}

func TestNettingOMSClosePosition(t *testing.T) {
	oms := NewNettingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != 0 {
		t.Fatalf("Expected position qty 0, got %d", pos.Qty)
	}

	expectedPnL := int64((51000 - 50000) * exchange.SATOSHI)
	if pos.RealizedPnL != expectedPnL {
		t.Fatalf("Expected realized PnL %d, got %d", expectedPnL, pos.RealizedPnL)
	}
}

func TestNettingOMSFlipPosition(t *testing.T) {
	oms := NewNettingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * exchange.SATOSHI,
		Qty:   2 * exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != -exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", -exchange.SATOSHI, pos.Qty)
	}
	if pos.AvgPrice != 51000*exchange.SATOSHI {
		t.Fatalf("Expected avg price %d (flip price), got %d", 51000*exchange.SATOSHI, pos.AvgPrice)
	}

	expectedPnL := int64((51000 - 50000) * exchange.SATOSHI)
	if pos.RealizedPnL != expectedPnL {
		t.Fatalf("Expected realized PnL %d, got %d", expectedPnL, pos.RealizedPnL)
	}
}

func TestNettingOMSShortPosition(t *testing.T) {
	oms := NewNettingOMS()

	fill := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != -exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", -exchange.SATOSHI, pos.Qty)
	}
	if pos.AvgPrice != 50000*exchange.SATOSHI {
		t.Fatalf("Expected avg price %d, got %d", 50000*exchange.SATOSHI, pos.AvgPrice)
	}
}

func TestNettingOMSGetNetPosition(t *testing.T) {
	oms := NewNettingOMS()

	fill := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill, exchange.SATOSHI)

	net := oms.GetNetPosition("BTC/USD")
	if net != exchange.SATOSHI {
		t.Fatalf("Expected net position %d, got %d", exchange.SATOSHI, net)
	}

	netEmpty := oms.GetNetPosition("ETH/USD")
	if netEmpty != 0 {
		t.Fatalf("Expected net position 0 for unknown instrument, got %d", netEmpty)
	}
}

func TestHedgingOMSMultiplePositions(t *testing.T) {
	oms := NewHedgingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	positions := oms.GetPositions("BTC/USD")
	if len(positions) != 2 {
		t.Fatalf("Expected 2 positions, got %d", len(positions))
	}

	net := oms.GetNetPosition("BTC/USD")
	if net != 0 {
		t.Fatalf("Expected net position 0 (long and short cancel), got %d", net)
	}
}

func TestHedgingOMSIncreaseLongPosition(t *testing.T) {
	oms := NewHedgingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	positions := oms.GetPositions("BTC/USD")
	if len(positions) != 1 {
		t.Fatalf("Expected 1 position, got %d", len(positions))
	}

	if positions[0].Qty != 2*exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", 2*exchange.SATOSHI, positions[0].Qty)
	}

	expectedAvg := int64((50000 + 51000) / 2 * exchange.SATOSHI)
	if positions[0].AvgPrice != expectedAvg {
		t.Fatalf("Expected avg price %d, got %d", expectedAvg, positions[0].AvgPrice)
	}
}

func TestOMSReset(t *testing.T) {
	oms := NewNettingOMS()

	fill := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != exchange.SATOSHI {
		t.Fatalf("Expected position before reset, got %d", pos.Qty)
	}

	oms.Reset("BTC/USD")

	posAfter := oms.GetPosition("BTC/USD")
	if posAfter.Qty != 0 {
		t.Fatalf("Expected position 0 after reset, got %d", posAfter.Qty)
	}
}

func TestNettingOMSGetPositions(t *testing.T) {
	oms := NewNettingOMS()

	fill := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill, exchange.SATOSHI)

	positions := oms.GetPositions("BTC/USD")
	if len(positions) != 1 {
		t.Fatalf("Expected 1 position, got %d", len(positions))
	}
	if positions[0].Qty != exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", exchange.SATOSHI, positions[0].Qty)
	}

	emptyPositions := oms.GetPositions("ETH/USD")
	if len(emptyPositions) != 0 {
		t.Fatalf("Expected 0 positions for unknown instrument, got %d", len(emptyPositions))
	}
}

func TestHedgingOMSGetPosition(t *testing.T) {
	oms := NewHedgingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	pos := oms.GetPosition("BTC/USD")
	if pos.Qty != exchange.SATOSHI {
		t.Fatalf("Expected position qty %d, got %d", exchange.SATOSHI, pos.Qty)
	}

	emptyPos := oms.GetPosition("ETH/USD")
	if emptyPos.Qty != 0 {
		t.Fatalf("Expected empty position for unknown instrument, got %d", emptyPos.Qty)
	}
}

func TestHedgingOMSReset(t *testing.T) {
	oms := NewHedgingOMS()

	fill1 := OrderFillEvent{
		Side:  exchange.Buy,
		Price: 50000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill1, exchange.SATOSHI)

	fill2 := OrderFillEvent{
		Side:  exchange.Sell,
		Price: 51000 * exchange.SATOSHI,
		Qty:   exchange.SATOSHI,
	}
	oms.OnFill("BTC/USD", fill2, exchange.SATOSHI)

	positions := oms.GetPositions("BTC/USD")
	if len(positions) != 2 {
		t.Fatalf("Expected 2 positions before reset, got %d", len(positions))
	}

	oms.Reset("BTC/USD")

	positionsAfter := oms.GetPositions("BTC/USD")
	if positionsAfter != nil {
		t.Fatalf("Expected nil positions after reset, got %d positions", len(positionsAfter))
	}
}
