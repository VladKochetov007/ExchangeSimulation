package actors

import (
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func captureSubmit() (actor.OrderSubmitter, *[]exchange.Side) {
	var sides []exchange.Side
	fn := func(_ string, side exchange.Side, _ exchange.OrderType, _, _ int64) uint64 {
		sides = append(sides, side)
		return 0
	}
	return fn, &sides
}

func newTriArb(threshold int64) *TriangleArbitrage {
	return NewTriangleArbitrage(TriangleArbConfig{
		ActorID:      7,
		BaseSymbol:   "BTC/USD",
		CrossSymbol:  "ETH/USD",
		DirectSymbol: "ETH/BTC",
		ThresholdBps: threshold,
		MaxTradeSize: 100,
	})
}

func snapEvent(symbol string, bid, ask int64) *actor.Event {
	return &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: symbol,
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: bid}},
				Asks: []exchange.PriceLevel{{Price: ask}},
			},
		},
	}
}

func TestTriangleArb_Identity(t *testing.T) {
	ta := newTriArb(5)
	if ta.GetID() != 7 {
		t.Errorf("GetID: want 7, got %d", ta.GetID())
	}
	syms := ta.GetSymbols()
	if len(syms) != 3 {
		t.Fatalf("GetSymbols: want 3, got %d", len(syms))
	}
}

func TestTriangleArb_SnapshotUpdatesBase(t *testing.T) {
	ta := newTriArb(5)
	ctx := actor.NewSharedContext()
	submit, _ := captureSubmit()
	ta.OnEvent(snapEvent("BTC/USD", 99, 101), ctx, submit)
	// mid = 99 + (101-99)/2 = 100
	if ta.baseMid != 100 {
		t.Errorf("baseMid: want 100, got %d", ta.baseMid)
	}
}

func TestTriangleArb_SnapshotUpdatesCross(t *testing.T) {
	ta := newTriArb(5)
	ctx := actor.NewSharedContext()
	submit, _ := captureSubmit()
	ta.OnEvent(snapEvent("ETH/USD", 199, 201), ctx, submit)
	if ta.crossMid != 200 {
		t.Errorf("crossMid: want 200, got %d", ta.crossMid)
	}
}

func TestTriangleArb_SnapshotUpdatesDirect(t *testing.T) {
	ta := newTriArb(5)
	ctx := actor.NewSharedContext()
	submit, _ := captureSubmit()
	ta.OnEvent(snapEvent("ETH/BTC", 149, 151), ctx, submit)
	if ta.directMid != 150 {
		t.Errorf("directMid: want 150, got %d", ta.directMid)
	}
}

func TestTriangleArb_ProfitableArbSubmitsThreeOrders(t *testing.T) {
	// Correct formula: impliedDirect = baseMid * precision / crossMid
	// baseMid=100_000_000, crossMid=50_000_000, directMid=150_000_000, precision=100_000_000
	// impliedDirect = 100_000_000 * 100_000_000 / 50_000_000 = 200_000_000
	// profitBps = ((200_000_000 - 150_000_000) * 10000) / 150_000_000 ≈ 3333
	// TakerFeeBps=0 (not set), threshold=10 → 3333 > 10 → fires
	ta := newTriArb(10)
	ta.baseMid = 100_000_000
	ta.crossMid = 50_000_000
	ta.directMid = 150_000_000

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 3 {
		t.Fatalf("want 3 orders, got %d", len(*sides))
	}
	// Legs: Buy DirectSymbol, Buy CrossSymbol, Sell BaseSymbol.
	if (*sides)[0] != exchange.Buy {
		t.Errorf("order[0] (Direct buy): want Buy, got %v", (*sides)[0])
	}
	if (*sides)[1] != exchange.Buy {
		t.Errorf("order[1] (Cross buy): want Buy, got %v", (*sides)[1])
	}
	if (*sides)[2] != exchange.Sell {
		t.Errorf("order[2] (Base sell): want Sell, got %v", (*sides)[2])
	}
}

func TestTriangleArb_UnprofitableArbNoOrders(t *testing.T) {
	// All mids equal → profitBps = 0 → not above fee+threshold
	ta := newTriArb(10)
	ta.baseMid = 100_000_000
	ta.crossMid = 100_000_000
	ta.directMid = 100_000_000

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 0 {
		t.Errorf("want no orders for balanced prices, got %d", len(*sides))
	}
}

func TestTriangleArb_IncompleteDataNoOrders(t *testing.T) {
	ta := newTriArb(5)
	ta.baseMid = 100_000_000
	ta.crossMid = 200_000_000
	// ta.directMid == 0

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 0 {
		t.Errorf("want no orders with incomplete market data, got %d", len(*sides))
	}
}
