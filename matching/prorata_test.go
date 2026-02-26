package matching

import (
	"testing"

	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

func mkSell(id, clientID uint64, price, qty int64) *etypes.Order {
	return &etypes.Order{ID: id, ClientID: clientID, Side: etypes.Sell, Type: etypes.LimitOrder, Price: price, Qty: qty}
}

func mkBuy(id, clientID uint64, typ etypes.OrderType, price, qty int64) *etypes.Order {
	return &etypes.Order{ID: id, ClientID: clientID, Side: etypes.Buy, Type: typ, Price: price, Qty: qty}
}

func execByMaker(execs []*etypes.Execution) map[uint64]int64 {
	m := make(map[uint64]int64, len(execs))
	for _, e := range execs {
		m[e.MakerOrderID] += e.Qty
	}
	return m
}

func newProRata() *ProRataMatcher { return NewProRataMatcher(&eclock.RealClock{}) }

func TestProRata_SingleRestingOrderFullFill(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 100, 100, 100))
	result := m.Match(bids, asks, mkBuy(2, 200, etypes.LimitOrder, 100, 100))
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

func TestProRata_ProportionalSplit(t *testing.T) {
	// Three makers [600,300,100] total 1000, incoming 100 → floor shares [60,30,10], leftover=0.
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 100, 600))
	asks.AddOrder(mkSell(2, 20, 100, 300))
	asks.AddOrder(mkSell(3, 30, 100, 100))
	result := m.Match(bids, asks, mkBuy(4, 40, etypes.LimitOrder, 100, 100))
	if !result.FullyFilled {
		t.Fatal("should be fully filled")
	}
	if len(result.Executions) != 3 {
		t.Fatalf("expected 3 executions, got %d", len(result.Executions))
	}
	fills := execByMaker(result.Executions)
	for makerID, want := range map[uint64]int64{1: 60, 2: 30, 3: 10} {
		if fills[makerID] != want {
			t.Errorf("maker %d: want fill %d, got %d", makerID, want, fills[makerID])
		}
	}
}

func TestProRata_LeftoverFIFO(t *testing.T) {
	// Three equal makers [100,100,100] total 300, incoming 100:
	// floor shares = [33,33,33] sum=99, leftover 1 → FIFO-first maker → [34,33,33].
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 100, 100))
	asks.AddOrder(mkSell(2, 20, 100, 100))
	asks.AddOrder(mkSell(3, 30, 100, 100))
	result := m.Match(bids, asks, mkBuy(4, 40, etypes.LimitOrder, 100, 100))
	if !result.FullyFilled {
		t.Fatal("should be fully filled")
	}
	fills := execByMaker(result.Executions)
	for makerID, want := range map[uint64]int64{1: 34, 2: 33, 3: 33} {
		if fills[makerID] != want {
			t.Errorf("maker %d: want fill %d, got %d", makerID, want, fills[makerID])
		}
	}
}

func TestProRata_PricePriorityFillsBestFirst(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 99, 100))  // best ask
	asks.AddOrder(mkSell(2, 20, 101, 100)) // worse ask
	result := m.Match(bids, asks, mkBuy(3, 30, etypes.LimitOrder, 101, 100))
	if !result.FullyFilled {
		t.Fatal("should be fully filled at best level")
	}
	if len(result.Executions) != 1 {
		t.Fatalf("expected 1 execution (from best level), got %d", len(result.Executions))
	}
	if result.Executions[0].Price != 99 {
		t.Errorf("execution price: want 99, got %d", result.Executions[0].Price)
	}
	if asks.Best == nil || asks.Best.Price != 101 {
		t.Error("worse-price ask should still be resting")
	}
}

func TestProRata_LimitBuyDoesNotMatchAbovePrice(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 110, 100)) // ask above buy limit
	result := m.Match(bids, asks, mkBuy(2, 20, etypes.LimitOrder, 100, 100))
	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("limit buy should not match ask above its price")
	}
}

func TestProRata_LimitSellDoesNotMatchBelowPrice(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	bids.AddOrder(&etypes.Order{ID: 1, ClientID: 10, Side: etypes.Buy, Type: etypes.LimitOrder, Price: 90, Qty: 100})
	incoming := &etypes.Order{ID: 2, ClientID: 20, Side: etypes.Sell, Type: etypes.LimitOrder, Price: 100, Qty: 100}
	result := m.Match(bids, asks, incoming)
	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("limit sell should not match bid below its price")
	}
}

func TestProRata_SelfTradeSkipped(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 99, 100, 100)) // same clientID as incoming
	result := m.Match(bids, asks, mkBuy(2, 99, etypes.LimitOrder, 100, 100))
	if result.FullyFilled || len(result.Executions) != 0 {
		t.Error("self-trade must be skipped")
	}
}

func TestProRata_MarketOrderAlwaysMatches(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 999, 100)) // very high ask price
	result := m.Match(bids, asks, mkBuy(2, 20, etypes.Market, 0, 100))
	if !result.FullyFilled {
		t.Error("market order must match any resting ask price")
	}
}

func TestProRata_PartialFill(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 100, 40)) // only 40 available
	incoming := mkBuy(2, 20, etypes.LimitOrder, 100, 100)
	result := m.Match(bids, asks, incoming)
	if result.FullyFilled {
		t.Error("should not be fully filled")
	}
	if incoming.Status != etypes.PartialFill {
		t.Errorf("status: want PartialFill, got %v", incoming.Status)
	}
	if incoming.FilledQty != 40 {
		t.Errorf("filled qty: want 40, got %d", incoming.FilledQty)
	}
}

func TestProRata_MultiLevelFill(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 100, 60)) // best level: 60
	asks.AddOrder(mkSell(2, 20, 101, 60)) // next level: 60
	incoming := mkBuy(3, 30, etypes.LimitOrder, 101, 100)
	result := m.Match(bids, asks, incoming)
	if !result.FullyFilled {
		t.Fatal("should be fully filled across two levels")
	}
	if len(result.Executions) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(result.Executions))
	}
	total := result.Executions[0].Qty + result.Executions[1].Qty
	if total != 100 {
		t.Errorf("total filled: want 100, got %d", total)
	}
}

func TestProRata_FilledRestingOrderRemovedFromBook(t *testing.T) {
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 100, 100))
	m.Match(bids, asks, mkBuy(2, 20, etypes.LimitOrder, 100, 100))
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
	m := newProRata()
	bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
	asks.AddOrder(mkSell(1, 10, 100, 1))
	asks.AddOrder(mkSell(2, 20, 100, 999))
	result := m.Match(bids, asks, mkBuy(3, 30, etypes.LimitOrder, 100, 1))
	if !result.FullyFilled {
		t.Fatal("should be fully filled")
	}
	if len(result.Executions) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(result.Executions))
	}
	if result.Executions[0].MakerOrderID != 1 {
		t.Errorf("leftover should go to FIFO-first maker (ID 1), got ID %d", result.Executions[0].MakerOrderID)
	}
}
