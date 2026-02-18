package actors

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

type staticOracle struct{ price int64 }

func (o *staticOracle) GetSignal(_ string, _ int64) int64 { return o.price }

func newInformedTrader(oracle PrivateSignalOracle) (*InformedTraderActor, *exchange.ClientGateway) {
	gw := exchange.NewClientGateway(1)
	config := InformedTraderConfig{
		Symbol:    "BTC/USD",
		Oracle:    oracle,
		OrderQty:  exchange.BTC_PRECISION,
		ThresholdBps: 10,
	}
	return NewInformedTrader(1, gw, config), gw
}

func TestInformedTrader_Defaults(t *testing.T) {
	gw := exchange.NewClientGateway(1)
	tr := NewInformedTrader(1, gw, InformedTraderConfig{Oracle: &staticOracle{}})
	if tr.config.ThresholdBps != 10 {
		t.Errorf("ThresholdBps: want 10, got %d", tr.config.ThresholdBps)
	}
	if tr.config.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval: want 500ms, got %v", tr.config.PollInterval)
	}
}

func TestInformedTrader_MidFromSnapshot(t *testing.T) {
	tr, _ := newInformedTrader(&staticOracle{})
	tr.OnEvent(&actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: "BTC/USD",
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: 99_000}},
				Asks: []exchange.PriceLevel{{Price: 101_000}},
			},
		},
	})
	// mid = 99_000 + (101_000-99_000)/2 = 100_000
	if tr.mid != 100_000 {
		t.Errorf("mid: want 100000, got %d", tr.mid)
	}
}

func TestInformedTrader_MidIgnoresEmptySnapshot(t *testing.T) {
	tr, _ := newInformedTrader(&staticOracle{})
	tr.mid = 50_000
	tr.OnEvent(&actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol:   "BTC/USD",
			Snapshot: &exchange.BookSnapshot{},
		},
	})
	if tr.mid != 50_000 {
		t.Errorf("mid should be unchanged on empty book, got %d", tr.mid)
	}
}

func TestInformedTrader_FullFillClearsInOrder(t *testing.T) {
	tr, _ := newInformedTrader(&staticOracle{})
	tr.inOrder = true
	tr.OnEvent(&actor.Event{
		Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{IsFull: true},
	})
	if tr.inOrder {
		t.Error("full fill must clear inOrder")
	}
}

func TestInformedTrader_PartialFillKeepsInOrder(t *testing.T) {
	tr, _ := newInformedTrader(&staticOracle{})
	tr.inOrder = true
	tr.OnEvent(&actor.Event{
		Type: actor.EventOrderFilled,
		Data: actor.OrderFillEvent{IsFull: false},
	})
	if !tr.inOrder {
		t.Error("partial fill must not clear inOrder")
	}
}

func TestInformedTrader_CancelClearsInOrder(t *testing.T) {
	tr, _ := newInformedTrader(&staticOracle{})
	tr.inOrder = true
	tr.OnEvent(&actor.Event{
		Type: actor.EventOrderCancelled,
		Data: actor.OrderCancelledEvent{},
	})
	if tr.inOrder {
		t.Error("cancel must clear inOrder")
	}
}

func TestInformedTrader_RejectClearsInOrder(t *testing.T) {
	tr, _ := newInformedTrader(&staticOracle{})
	tr.inOrder = true
	tr.OnEvent(&actor.Event{
		Type: actor.EventOrderRejected,
		Data: actor.OrderRejectedEvent{},
	})
	if tr.inOrder {
		t.Error("reject must clear inOrder")
	}
}

func TestInformedTrader_CheckSignal_SkipsWhenMidZero(t *testing.T) {
	tr, gw := newInformedTrader(&staticOracle{price: 200_000})
	tr.checkSignal()
	if tr.inOrder {
		t.Error("inOrder must stay false when mid is zero")
	}
	if len(gw.RequestCh) != 0 {
		t.Errorf("expected no requests, got %d", len(gw.RequestCh))
	}
}

func TestInformedTrader_CheckSignal_SkipsWhenInFlight(t *testing.T) {
	tr, gw := newInformedTrader(&staticOracle{price: 200_000})
	tr.mid = 100_000
	tr.inOrder = true
	tr.checkSignal()
	if len(gw.RequestCh) != 0 {
		t.Errorf("expected no requests while in flight, got %d", len(gw.RequestCh))
	}
}

func TestInformedTrader_CheckSignal_BuyOnBullishSignal(t *testing.T) {
	// mid=100_000, ThresholdBps=10, threshold=100_000*10/10000=100
	// signal=101_000 > 100_000+100=100_100 → buy
	tr, gw := newInformedTrader(&staticOracle{price: 101_000})
	tr.mid = 100_000
	tr.checkSignal()
	if !tr.inOrder {
		t.Error("should set inOrder after buy signal")
	}
	req := <-gw.RequestCh
	if req.OrderReq.Side != exchange.Buy {
		t.Errorf("expected Buy, got %v", req.OrderReq.Side)
	}
	if req.OrderReq.Type != exchange.Market {
		t.Errorf("expected Market order, got %v", req.OrderReq.Type)
	}
}

func TestInformedTrader_CheckSignal_SellOnBearishSignal(t *testing.T) {
	// mid=100_000, threshold=100, signal=98_000 < 99_900 → sell
	tr, gw := newInformedTrader(&staticOracle{price: 98_000})
	tr.mid = 100_000
	tr.checkSignal()
	if !tr.inOrder {
		t.Error("should set inOrder after sell signal")
	}
	req := <-gw.RequestCh
	if req.OrderReq.Side != exchange.Sell {
		t.Errorf("expected Sell, got %v", req.OrderReq.Side)
	}
}

func TestInformedTrader_CheckSignal_QuietWithinThreshold(t *testing.T) {
	// mid=100_000, threshold=100; signal=100_050 is within ±100 → no order
	tr, gw := newInformedTrader(&staticOracle{price: 100_050})
	tr.mid = 100_000
	tr.checkSignal()
	if tr.inOrder {
		t.Error("should not trade within threshold")
	}
	if len(gw.RequestCh) != 0 {
		t.Errorf("expected no requests, got %d", len(gw.RequestCh))
	}
}
