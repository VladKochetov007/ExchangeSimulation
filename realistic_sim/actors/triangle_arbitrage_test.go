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
	if ta.baseBid != 99 || ta.baseAsk != 101 {
		t.Errorf("base bid/ask: want 99/101, got %d/%d", ta.baseBid, ta.baseAsk)
	}
}

func TestTriangleArb_SnapshotUpdatesCross(t *testing.T) {
	ta := newTriArb(5)
	ctx := actor.NewSharedContext()
	submit, _ := captureSubmit()
	ta.OnEvent(snapEvent("ETH/USD", 199, 201), ctx, submit)
	if ta.crossBid != 199 || ta.crossAsk != 201 {
		t.Errorf("cross bid/ask: want 199/201, got %d/%d", ta.crossBid, ta.crossAsk)
	}
}

func TestTriangleArb_SnapshotUpdatesDirect(t *testing.T) {
	ta := newTriArb(5)
	ctx := actor.NewSharedContext()
	submit, _ := captureSubmit()
	ta.OnEvent(snapEvent("ETH/BTC", 149, 151), ctx, submit)
	if ta.directBid != 149 || ta.directAsk != 151 {
		t.Errorf("direct bid/ask: want 149/151, got %d/%d", ta.directBid, ta.directAsk)
	}
}

func TestTriangleArb_ProfitableForwardArbSubmitsThreeOrders(t *testing.T) {
	// Forward arb: baseBid*precision > directAsk*crossAsk
	// baseBid=100e6, crossAsk=50e6, directAsk=150e6, precision=100e6
	// forwardNumer = 100e6*100e6 - 150e6*50e6 = 1e16 - 7.5e15 = 2.5e15 > 0
	// profit_bps ≈ 3333 >> threshold=10 → fires forward direction
	ta := newTriArb(10)
	ta.baseBid = 100_000_000
	ta.baseAsk = 100_000_000
	ta.crossBid = 50_000_000
	ta.crossAsk = 50_000_000
	ta.directBid = 150_000_000
	ta.directAsk = 150_000_000

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 3 {
		t.Fatalf("want 3 orders, got %d", len(*sides))
	}
	// Forward: buy direct, buy cross, sell base.
	if (*sides)[0] != exchange.Buy {
		t.Errorf("order[0] (direct buy): want Buy, got %v", (*sides)[0])
	}
	if (*sides)[1] != exchange.Buy {
		t.Errorf("order[1] (cross buy): want Buy, got %v", (*sides)[1])
	}
	if (*sides)[2] != exchange.Sell {
		t.Errorf("order[2] (base sell): want Sell, got %v", (*sides)[2])
	}
}

func TestTriangleArb_ProfitableReverseArbSubmitsThreeOrders(t *testing.T) {
	// Reverse arb: directBid*crossBid > baseAsk*precision
	// directBid=200e6, crossBid=50e6, baseAsk=100e6, precision=100e6
	// reverseNumer = 200e6*50e6 - 100e6*100e6 = 1e16 - 1e16 = 0... let me tweak:
	// directBid=210e6 → reverseNumer = 210e6*50e6 - 100e6*100e6 = 10.5e15 - 10e15 = 5e14 > 0
	ta := newTriArb(10)
	ta.baseBid = 100_000_000
	ta.baseAsk = 100_000_000
	ta.crossBid = 50_000_000
	ta.crossAsk = 50_000_000
	ta.directBid = 210_000_000
	ta.directAsk = 210_000_000

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 3 {
		t.Fatalf("want 3 orders, got %d", len(*sides))
	}
	// Reverse: buy base, sell cross, sell direct.
	if (*sides)[0] != exchange.Buy {
		t.Errorf("order[0] (base buy): want Buy, got %v", (*sides)[0])
	}
	if (*sides)[1] != exchange.Sell {
		t.Errorf("order[1] (cross sell): want Sell, got %v", (*sides)[1])
	}
	if (*sides)[2] != exchange.Sell {
		t.Errorf("order[2] (direct sell): want Sell, got %v", (*sides)[2])
	}
}

func TestTriangleArb_UnprofitableArbNoOrders(t *testing.T) {
	// Balanced prices: baseBid*precision == directAsk*crossAsk → no arb in either direction.
	ta := newTriArb(10)
	ta.baseBid = 100_000_000
	ta.baseAsk = 100_000_000
	ta.crossBid = 100_000_000
	ta.crossAsk = 100_000_000
	ta.directBid = 100_000_000
	ta.directAsk = 100_000_000

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 0 {
		t.Errorf("want no orders for balanced prices, got %d", len(*sides))
	}
}

func TestTriangleArb_IncompleteDataNoOrders(t *testing.T) {
	ta := newTriArb(5)
	ta.baseBid = 100_000_000
	ta.baseAsk = 100_000_000
	ta.crossBid = 200_000_000
	ta.crossAsk = 200_000_000
	// directBid/directAsk == 0

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()
	ta.evaluateArbitrage(ctx, submit)

	if len(*sides) != 0 {
		t.Errorf("want no orders with incomplete market data, got %d", len(*sides))
	}
}

func TestTriangleArb_ExecutingFlagPreventsReentry(t *testing.T) {
	// After a profitable arb fires via OnEvent, a subsequent OnEvent must not
	// submit another round while executing=true.
	ta := newTriArb(10)
	ta.baseBid = 100_000_000
	ta.baseAsk = 100_000_000
	ta.crossBid = 50_000_000
	ta.crossAsk = 50_000_000
	ta.directBid = 150_000_000
	ta.directAsk = 150_000_000

	ctx := actor.NewSharedContext()
	submit, sides := captureSubmit()

	// First OnEvent triggers the arb (prices already profitable before this snapshot).
	ta.OnEvent(snapEvent("BTC/USD", 100_000_000, 100_000_000), ctx, submit)
	if len(*sides) != 3 {
		t.Fatalf("first fire: want 3 orders, got %d", len(*sides))
	}
	if !ta.executing {
		t.Fatal("executing should be true after first fire")
	}

	// Second snapshot while executing: OnEvent guard must block re-entry.
	ta.OnEvent(snapEvent("BTC/USD", 100_000_000, 100_000_000), ctx, submit)
	if len(*sides) != 3 {
		t.Errorf("re-entry prevention failed: got %d orders, want 3", len(*sides))
	}
}
