package exchange

import "testing"

func TestProRata_SingleRestingOrderFullFill(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 100, 100, 100))

	incoming := mkBuyOrder(2, 200, LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Fatal("single resting order must fully fill incoming")
	}
	if len(result.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].Qty != 100 {
		t.Errorf("execution qty: want 100, got %d", result.Executions[0].Qty)
	}
}

func mkSellOrder(id, clientID uint64, price, qty int64) *Order {
	o := getOrder()
	o.ID = id
	o.ClientID = clientID
	o.Side = Sell
	o.Type = LimitOrder
	o.Price = price
	o.Qty = qty
	return o
}

func mkBuyOrder(id, clientID uint64, typ OrderType, price, qty int64) *Order {
	o := getOrder()
	o.ID = id
	o.ClientID = clientID
	o.Side = Buy
	o.Type = typ
	o.Price = price
	o.Qty = qty
	return o
}

func TestProRata_ProportionalSplit(t *testing.T) {
	// Three makers [600,300,100] total 1000, incoming 100 → floor shares [60,30,10], leftover=0.
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 100, 600))
	asks.AddOrder(mkSellOrder(2, 20, 100, 300))
	asks.AddOrder(mkSellOrder(3, 30, 100, 100))

	incoming := mkBuyOrder(4, 40, LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Fatal("should be fully filled")
	}
	if len(result.Executions) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(result.Executions))
	}

	fills := execByMaker(result.Executions)
	wantFills := map[uint64]int64{1: 60, 2: 30, 3: 10}
	for makerID, want := range wantFills {
		if fills[makerID] != want {
			t.Errorf("maker %d: want fill %d, got %d", makerID, want, fills[makerID])
		}
	}
}

func TestProRata_LeftoverFIFO(t *testing.T) {
	// Three equal makers [100,100,100] total 300, incoming 100:
	// floor shares = [33,33,33] sum=99, leftover 1 → FIFO-first maker → [34,33,33].
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 100, 100))
	asks.AddOrder(mkSellOrder(2, 20, 100, 100))
	asks.AddOrder(mkSellOrder(3, 30, 100, 100))

	incoming := mkBuyOrder(4, 40, LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Fatal("should be fully filled")
	}

	fills := execByMaker(result.Executions)
	wantFills := map[uint64]int64{1: 34, 2: 33, 3: 33}
	for makerID, want := range wantFills {
		if fills[makerID] != want {
			t.Errorf("maker %d: want fill %d, got %d", makerID, want, fills[makerID])
		}
	}
}

func TestProRata_PricePriorityFillsBestFirst(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 99, 100))  // best ask
	asks.AddOrder(mkSellOrder(2, 20, 101, 100)) // worse ask

	incoming := mkBuyOrder(3, 30, LimitOrder, 101, 100)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Fatal("should be fully filled at best level")
	}
	if len(result.Executions) != 1 {
		t.Fatalf("only one execution expected (from best level), got %d", len(result.Executions))
	}
	if result.Executions[0].Price != 99 {
		t.Errorf("execution price: want 99, got %d", result.Executions[0].Price)
	}
	if asks.Best == nil || asks.Best.Price != 101 {
		t.Errorf("worse-price ask should still be resting")
	}
}

func TestProRata_LimitBuyDoesNotMatchAbovePrice(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 110, 100)) // ask above buy limit

	incoming := mkBuyOrder(2, 20, LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)

	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("limit buy should not match ask above its price")
	}
}

func TestProRata_LimitSellDoesNotMatchBelowPrice(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	bid := getOrder()
	bid.ID = 1
	bid.ClientID = 10
	bid.Side = Buy
	bid.Type = LimitOrder
	bid.Price = 90
	bid.Qty = 100
	bids.AddOrder(bid)

	incoming := getOrder() // limit sell at 100, bid only at 90
	incoming.ID = 2
	incoming.ClientID = 20
	incoming.Side = Sell
	incoming.Type = LimitOrder
	incoming.Price = 100
	incoming.Qty = 100
	result := m.Match(bids, asks, incoming)

	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("limit sell should not match bid below its price")
	}
}

func TestProRata_SelfTradeSkipped(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 99, 100, 100)) // same clientID as incoming

	incoming := mkBuyOrder(2, 99, LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)

	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("self-trade must be skipped")
	}
}

func TestProRata_MarketOrderAlwaysMatches(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 999, 100)) // very high ask price

	incoming := mkBuyOrder(2, 20, Market, 0, 100)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Error("market order must match any resting ask price")
	}
}

func TestProRata_PartialFill(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 100, 40)) // only 40 available

	incoming := mkBuyOrder(2, 20, LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)

	if result.FullyFilled {
		t.Error("should not be fully filled")
	}
	if incoming.Status != PartialFill {
		t.Errorf("status: want PartialFill, got %v", incoming.Status)
	}
	if incoming.FilledQty != 40 {
		t.Errorf("filled qty: want 40, got %d", incoming.FilledQty)
	}
}

func TestProRata_MultiLevelFill(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 100, 60)) // best level: 60
	asks.AddOrder(mkSellOrder(2, 20, 101, 60)) // next level: 60

	incoming := mkBuyOrder(3, 30, LimitOrder, 101, 100)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Fatal("should be fully filled across two levels")
	}
	if len(result.Executions) != 2 {
		t.Fatalf("expected 2 executions (one per level), got %d", len(result.Executions))
	}
	if result.Executions[0].Qty+result.Executions[1].Qty != 100 {
		t.Errorf("total filled: want 100, got %d", result.Executions[0].Qty+result.Executions[1].Qty)
	}
}

func TestProRata_FilledRestingOrderRemovedFromBook(t *testing.T) {
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 100, 100))

	incoming := mkBuyOrder(2, 20, LimitOrder, 100, 100)
	m.Match(bids, asks, incoming)

	if asks.Best != nil {
		t.Error("fully-filled resting order must be removed from the book")
	}
	if _, exists := asks.Orders[1]; exists {
		t.Error("order map must not retain the filled order")
	}
}

func TestProRata_ZeroShareMakersEmitNoExecution(t *testing.T) {
	// Makers [1,999] total 1000, incoming 1:
	// floor(1*1/1000)=0, floor(1*999/1000)=0 → leftover=1 → FIFO-first (ID 1) gets it.
	m := NewProRataMatcher()
	bids := newBook(Buy)
	asks := newBook(Sell)

	asks.AddOrder(mkSellOrder(1, 10, 100, 1))
	asks.AddOrder(mkSellOrder(2, 20, 100, 999))

	incoming := mkBuyOrder(3, 30, LimitOrder, 100, 1)
	result := m.Match(bids, asks, incoming)

	if !result.FullyFilled {
		t.Fatal("should be fully filled")
	}
	if len(result.Executions) != 1 {
		t.Fatalf("only 1 execution expected, got %d", len(result.Executions))
	}
	if result.Executions[0].MakerOrderID != 1 {
		t.Errorf("leftover should go to FIFO-first maker (ID 1), got ID %d", result.Executions[0].MakerOrderID)
	}
}

func execByMaker(execs []*Execution) map[uint64]int64 {
	m := make(map[uint64]int64, len(execs))
	for _, e := range execs {
		m[e.MakerOrderID] += e.Qty
	}
	return m
}
