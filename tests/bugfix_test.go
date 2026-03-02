package exchange_test

import (
	. "exchange_sim/exchange"
	"testing"
	"time"
)

type testClock struct {
	now int64
}

func (c *testClock) NowUnixNano() int64 { return c.now }
func (c *testClock) NowUnix() int64     { return c.now / 1e9 }
func (c *testClock) SetTime(t int64)    { c.now = t }

func TestTotalQtyUpdatesOnPartialFill(t *testing.T) {
	clock := &testClock{now: 1000000000}
	ex := NewExchange(10, clock)

	inst := NewSpotInstrument("BTCUSDT", "BTC", "USDT", 100000000, 1000000, DOLLAR_TICK, BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	ex.ConnectNewClient(1, map[string]int64{"USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.ConnectNewClient(2, map[string]int64{"BTC": 100000000, "USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})

	makerResp := ex.PlaceOrder(1, &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !makerResp.Success {
		t.Fatalf("maker order failed: %v", makerResp.Error)
	}

	book := ex.Books["BTCUSDT"]
	limit := book.Bids.Best
	if limit == nil {
		t.Fatal("no best bid")
	}

	initialTotalQty := limit.TotalQty
	if initialTotalQty != 100000000 {
		t.Errorf("initial TotalQty = %d, want 100000000", initialTotalQty)
	}

	takerResp := ex.PlaceOrder(2, &OrderRequest{
		RequestID:   2,
		Side:        Sell,
		Type:        LimitOrder,
		Price:       5000000000000,
		Qty:         30000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !takerResp.Success {
		t.Fatalf("taker order failed: %v", takerResp.Error)
	}

	limit = book.Bids.Best
	if limit == nil {
		t.Fatal("best bid disappeared")
	}

	expectedTotalQty := initialTotalQty - 30000000
	if limit.TotalQty != expectedTotalQty {
		t.Errorf("after partial fill: TotalQty = %d, want %d", limit.TotalQty, expectedTotalQty)
	}

	makerOrder := book.Bids.Orders[makerResp.Data.(uint64)]
	if makerOrder == nil {
		t.Fatal("maker order not found after partial fill")
	}
	if makerOrder.FilledQty != 30000000 {
		t.Errorf("maker FilledQty = %d, want 30000000", makerOrder.FilledQty)
	}
	if makerOrder.Status != PartialFill {
		t.Errorf("maker Status = %v, want PartialFill", makerOrder.Status)
	}

	ex.Gateways[1].Close()
	ex.Gateways[2].Close()
}

func TestDeltasPublishedForMakerFills(t *testing.T) {
	clock := &testClock{now: 1000000000}
	ex := NewExchange(10, clock)

	inst := NewSpotInstrument("BTCUSDT", "BTC", "USDT", 100000000, 1000000, DOLLAR_TICK, BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	ex.ConnectNewClient(1, map[string]int64{}, &PercentageFee{MakerBps: 0, TakerBps: 0, InQuote: true})
	recorder := ex.Gateways[1]
	ex.MDPublisher.Subscribe(1, "BTCUSDT", []MDType{MDSnapshot, MDDelta, MDTrade}, recorder)

	ex.ConnectNewClient(2, map[string]int64{"USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.ConnectNewClient(3, map[string]int64{"BTC": 100000000, "USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})

	makerResp := ex.PlaceOrder(2, &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !makerResp.Success {
		t.Fatalf("maker order failed")
	}

	select {
	case msg := <-recorder.MarketData:
		if msg.Type != MDDelta {
			t.Errorf("expected MDDelta for maker order placement, got %v", msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for delta after maker placement")
	}

	takerResp := ex.PlaceOrder(3, &OrderRequest{
		RequestID:   2,
		Side:        Sell,
		Type:        Market,
		Price:       0,
		Qty:         30000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !takerResp.Success {
		t.Fatalf("taker order failed")
	}

	gotTrade := false
	gotDelta := false
readLoop:
	for i := 0; i < 10; i++ {
		select {
		case msg := <-recorder.MarketData:
			switch msg.Type {
			case MDTrade:
				gotTrade = true
			case MDDelta:
				delta := msg.Data.(*BookDelta)
				if delta.Side == Buy && delta.Price == 5000000000000 {
					gotDelta = true
					expectedQty := int64(70000000)
					if delta.VisibleQty != expectedQty {
						t.Errorf("delta VisibleQty = %d, want %d", delta.VisibleQty, expectedQty)
					}
				}
			}
		case <-time.After(100 * time.Millisecond):
			break readLoop
		}
	}

	if !gotTrade {
		t.Error("no trade published after match")
	}
	if !gotDelta {
		t.Error("no delta published for maker order partial fill")
	}

	ex.Gateways[1].Close()
	ex.Gateways[2].Close()
	ex.Gateways[3].Close()
}

func TestSequenceNumbersInEvents(t *testing.T) {
	clock := &testClock{now: 1000000000}
	ex := NewExchange(10, clock)

	inst := NewSpotInstrument("BTCUSDT", "BTC", "USDT", 100000000, 1000000, DOLLAR_TICK, BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	ex.ConnectNewClient(1, map[string]int64{"USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	client := ex.Gateways[1]

	client.RequestCh <- Request{
		Type: ReqSubscribe,
		QueryReq: &QueryRequest{
			RequestID: 1,
			Symbol:    "BTCUSDT",
		},
	}

	select {
	case <-client.ResponseCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for subscribe response")
	}

	select {
	case msg := <-client.MarketData:
		if msg.Type != MDSnapshot {
			t.Errorf("first message should be snapshot, got %v", msg.Type)
		}
		if msg.SeqNum == 0 {
			t.Error("snapshot SeqNum is 0, should be > 0")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for initial snapshot")
	}

	resp := ex.PlaceOrder(1, &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !resp.Success {
		t.Fatalf("order failed")
	}

	select {
	case msg := <-client.MarketData:
		if msg.Type != MDDelta {
			t.Errorf("expected delta, got %v", msg.Type)
		}
		if msg.SeqNum == 0 {
			t.Error("delta SeqNum is 0, should be > 0")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for delta")
	}

	client.Close()
}

func TestNoOrdersLeakedToPool(t *testing.T) {
	clock := &testClock{now: 1000000000}
	ex := NewExchange(10, clock)

	inst := NewSpotInstrument("BTCUSDT", "BTC", "USDT", 100000000, 1000000, DOLLAR_TICK, BTC_PRECISION/1000)
	ex.AddInstrument(inst)

	ex.ConnectNewClient(1, map[string]int64{"USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})
	ex.ConnectNewClient(2, map[string]int64{"BTC": 100000000, "USDT": 100000000000}, &PercentageFee{MakerBps: 10, TakerBps: 20, InQuote: true})

	makerResp := ex.PlaceOrder(1, &OrderRequest{
		RequestID:   1,
		Side:        Buy,
		Type:        LimitOrder,
		Price:       5000000000000,
		Qty:         100000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !makerResp.Success {
		t.Fatalf("maker order failed")
	}

	book := ex.Books["BTCUSDT"]
	makerOrderID := makerResp.Data.(uint64)

	makerOrder := book.Bids.Orders[makerOrderID]
	if makerOrder == nil {
		t.Fatal("maker order not in book")
	}
	if makerOrder.ID != makerOrderID {
		t.Errorf("maker order ID mismatch: got %d, want %d", makerOrder.ID, makerOrderID)
	}

	takerResp := ex.PlaceOrder(2, &OrderRequest{
		RequestID:   2,
		Side:        Sell,
		Type:        Market,
		Price:       0,
		Qty:         100000000,
		Symbol:      "BTCUSDT",
		TimeInForce: GTC,
		Visibility:  Normal,
	})
	if !takerResp.Success {
		t.Fatalf("taker order failed")
	}

	makerOrderAfterFill := book.Bids.Orders[makerOrderID]
	if makerOrderAfterFill != nil {
		t.Error("maker order still in book after being filled")
	}

	ex.Gateways[1].Close()
	ex.Gateways[2].Close()
}
